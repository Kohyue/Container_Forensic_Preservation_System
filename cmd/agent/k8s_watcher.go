package main

// Kubernetes pod lifecycle watcher.
//
// When kubernetes.enabled = true in config, this goroutine polls the cluster
// every N seconds via kubectl, detects pod anomalies (OOMKilled, CrashLoopBackOff,
// Failed), and runs a full forensic capture — logs, metadata, events, process list —
// storing the evidence in the same structured repository as Falco-triggered captures.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"forensic-preservation/internal/audit"
	"forensic-preservation/internal/collector"
	"forensic-preservation/internal/config"
	"forensic-preservation/internal/preserver"
	"forensic-preservation/internal/repository"
)

// ── Minimal Kubernetes API types ──────────────────────────────────────────────

type k8sPodList struct {
	Items []k8sPod `json:"items"`
}

type k8sPod struct {
	Metadata struct {
		Name      string            `json:"name"`
		Namespace string            `json:"namespace"`
		UID       string            `json:"uid"`
		Labels    map[string]string `json:"labels"`
	} `json:"metadata"`
	Spec struct {
		NodeName string `json:"nodeName"`
	} `json:"spec"`
	Status struct {
		Phase             string           `json:"phase"`
		ContainerStatuses []k8sContainerSts `json:"containerStatuses"`
	} `json:"status"`
}

type k8sContainerSts struct {
	Name         string `json:"name"`
	RestartCount int    `json:"restartCount"`
	Ready        bool   `json:"ready"`
	State        struct {
		Waiting *struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"waiting"`
		Terminated *struct {
			Reason   string `json:"reason"`
			ExitCode int    `json:"exitCode"`
			Message  string `json:"message"`
		} `json:"terminated"`
	} `json:"state"`
	LastState struct {
		Terminated *struct {
			Reason   string `json:"reason"`
			ExitCode int    `json:"exitCode"`
		} `json:"terminated"`
	} `json:"lastState"`
}

// ── Watcher entry point ───────────────────────────────────────────────────────

// runKubernetesWatcher polls Kubernetes pods for lifecycle anomalies and
// triggers forensic capture when pods crash, OOMKill, or CrashLoopBackOff.
// It is a no-op when kubernetes.enabled = false.
func runKubernetesWatcher(ctx context.Context, cfg *config.Config, logger *slog.Logger, auditLog *audit.Logger) {
	if !cfg.Kubernetes.Enabled {
		return
	}

	interval := time.Duration(cfg.Kubernetes.PollIntervalSeconds) * time.Second
	if interval < 10*time.Second {
		interval = 15 * time.Second
	}

	logger.Info("kubernetes watcher started",
		"namespace", cfg.Kubernetes.Namespace,
		"poll_interval", interval,
	)

	// seen tracks <namespace/pod/container:reason> already captured to avoid duplicates.
	seen := make(map[string]bool)
	var mu sync.Mutex

	// Run immediately on startup, then on each tick.
	checkKubernetesPods(ctx, cfg, logger, auditLog, seen, &mu)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("kubernetes watcher stopped")
			return
		case <-ticker.C:
			checkKubernetesPods(ctx, cfg, logger, auditLog, seen, &mu)
		}
	}
}

// ── Pod inspection ────────────────────────────────────────────────────────────

func checkKubernetesPods(
	ctx context.Context,
	cfg *config.Config,
	logger *slog.Logger,
	auditLog *audit.Logger,
	seen map[string]bool,
	mu *sync.Mutex,
) {
	args := []string{"get", "pods", "-o", "json"}
	if cfg.Kubernetes.Namespace != "" {
		args = append(args, "-n", cfg.Kubernetes.Namespace)
	} else {
		args = append(args, "--all-namespaces")
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	if cfg.Kubernetes.Kubeconfig != "" {
		cmd.Env = append(os.Environ(), "KUBECONFIG="+cfg.Kubernetes.Kubeconfig)
	}

	out, err := cmd.Output()
	if err != nil {
		logger.Warn("kubernetes watcher: kubectl get pods failed", "err", err)
		return
	}

	var podList k8sPodList
	if err := json.Unmarshal(out, &podList); err != nil {
		logger.Warn("kubernetes watcher: failed to parse pod list", "err", err)
		return
	}

	for _, pod := range podList.Items {
		ns := pod.Metadata.Namespace
		name := pod.Metadata.Name

		// ── Pod-level: phase = Failed ─────────────────────────────────────────
		if pod.Status.Phase == "Failed" {
			captureIfNew(ctx, cfg, logger, auditLog, seen, mu,
				ns+"/"+name+":Failed",
				ns, name, "Pod Failed", "error",
			)
		}

		// ── Container-level anomalies ─────────────────────────────────────────
		for _, cs := range pod.Status.ContainerStatuses {
			// OOMKilled — container was killed by the kernel due to memory exhaustion
			if cs.LastState.Terminated != nil && cs.LastState.Terminated.Reason == "OOMKilled" {
				captureIfNew(ctx, cfg, logger, auditLog, seen, mu,
					fmt.Sprintf("%s/%s/%s:OOMKilled", ns, name, cs.Name),
					ns, name, "Pod OOMKilled", "critical",
				)
			}

			// CrashLoopBackOff — container repeatedly crashes on restart
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				captureIfNew(ctx, cfg, logger, auditLog, seen, mu,
					fmt.Sprintf("%s/%s/%s:CrashLoopBackOff", ns, name, cs.Name),
					ns, name, "Pod CrashLoopBackOff", "error",
				)
			}

			// Non-zero exit code from a terminated container
			if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
				key := fmt.Sprintf("%s/%s/%s:Exit%d", ns, name, cs.Name, cs.State.Terminated.ExitCode)
				rule := fmt.Sprintf("Pod Container Exit Error (code %d)", cs.State.Terminated.ExitCode)
				captureIfNew(ctx, cfg, logger, auditLog, seen, mu,
					key, ns, name, rule, "warning",
				)
			}
		}
	}
}

// captureIfNew triggers a forensic capture the first time a given key is seen.
func captureIfNew(
	ctx context.Context,
	cfg *config.Config,
	logger *slog.Logger,
	auditLog *audit.Logger,
	seen map[string]bool,
	mu *sync.Mutex,
	key, namespace, podName, rule, priority string,
) {
	mu.Lock()
	if seen[key] {
		mu.Unlock()
		return
	}
	seen[key] = true
	mu.Unlock()

	logger.Info("kubernetes anomaly detected",
		"key", key,
		"pod", podName,
		"namespace", namespace,
		"rule", rule,
		"priority", priority,
	)
	go runKubernetesCapture(ctx, cfg, logger, auditLog, namespace, podName, rule, priority)
}

// ── Evidence capture ──────────────────────────────────────────────────────────

// runKubernetesCapture collects forensic evidence from a Kubernetes pod using
// kubectl commands, then stores it in the standard evidence repository with a
// SHA-256 manifest — identical structure to a Falco-triggered capture.
func runKubernetesCapture(
	ctx context.Context,
	cfg *config.Config,
	logger *slog.Logger,
	auditLog *audit.Logger,
	namespace, podName, rule, priority string,
) {
	captureStart := time.Now()
	podID := namespace + "/" + podName

	pkg, err := repository.Create(cfg.Repository.BasePath, podName, captureStart.UTC())
	if err != nil {
		logger.Error("k8s: failed to create evidence package", "pod", podID, "err", err)
		auditLog.Log(audit.CaptureFailure(podID, fmt.Errorf("create package: %w", err)))
		return
	}

	auditLog.Log(audit.AlertReceived(podID, rule, priority))
	auditLog.Log(audit.CaptureStarted(podID, pkg.Dir))

	// kubectl helper — returns output as string, embeds error message on failure.
	kctl := func(args ...string) string {
		finalArgs := args
		if cfg.Kubernetes.Kubeconfig != "" {
			finalArgs = append([]string{"--kubeconfig", cfg.Kubernetes.Kubeconfig}, args...)
		}
		cmd := exec.CommandContext(ctx, "kubectl", finalArgs...)
		out, err := cmd.Output()
		if err != nil {
			return fmt.Sprintf("# kubectl %s: error: %v\n", strings.Join(args[:2], " "), err)
		}
		return string(out)
	}

	ev := &collector.Evidence{
		ContainerID:   podID,
		ContainerName: podName,
	}

	// ── Metadata: pod spec + describe + node info ─────────────────────────────
	ev.Metadata = fmt.Sprintf(
		"=== Pod: %s  Namespace: %s ===\n"+
			"Rule: %s\n"+
			"Priority: %s\n"+
			"Captured At: %s\n\n"+
			"=== kubectl describe pod ===\n%s\n"+
			"=== kubectl get pod -o json ===\n%s",
		podName, namespace,
		rule, priority,
		captureStart.UTC().Format(time.RFC3339),
		kctl("describe", "pod", podName, "-n", namespace),
		kctl("get", "pod", podName, "-n", namespace, "-o", "json"),
	)

	// ── Logs: current run + previous (crashed) container ─────────────────────
	ev.Logs = "=== Current Container Logs (last 500 lines) ===\n" +
		kctl("logs", podName, "-n", namespace, "--tail=500") +
		"\n\n=== Previous Container Logs (pre-crash, last 500 lines) ===\n" +
		kctl("logs", podName, "-n", namespace, "--previous", "--tail=500")

	// ── Network: Kubernetes events + live connections (best effort) ───────────
	ev.Network = "=== Kubernetes Events for Pod ===\n" +
		kctl("get", "events", "-n", namespace,
			"--field-selector=involvedObject.name="+podName,
			"--sort-by=.lastTimestamp") +
		"\n\n=== Live Network Connections (best effort) ===\n" +
		kctl("exec", podName, "-n", namespace, "--",
			"sh", "-c", "netstat -an 2>/dev/null || ss -tuln 2>/dev/null || echo 'not available'")

	// ── Process list (best effort — may fail if pod is already terminated) ────
	procs := kctl("exec", podName, "-n", namespace, "--", "ps", "aux")
	if strings.HasPrefix(procs, "# kubectl exec:") {
		procs = fmt.Sprintf(
			"# Pod not running at capture time — process list unavailable.\n"+
				"# Rule:     %s\n"+
				"# Priority: %s\n"+
				"# Pod:      %s/%s\n",
			rule, priority, namespace, podName,
		)
	}
	ev.ProcessList = procs

	// ── Preserve: write files + SHA-256 manifest ──────────────────────────────
	manifestHash, err := preserver.Preserve(pkg, ev, rule, priority, captureStart)
	if err != nil {
		logger.Error("k8s: preservation failed", "pod", podID, "err", err)
		auditLog.Log(audit.CaptureFailure(podID, err))
		return
	}

	auditLog.Log(audit.CaptureSuccess(podID, pkg.Dir, map[string]string{
		"rule": rule, "priority": priority, "source": "kubernetes",
	}))
	auditLog.Log(audit.EvidenceStored(podID, pkg.Dir, manifestHash))

	logger.Info("kubernetes evidence preserved",
		"pod", podID,
		"rule", rule,
		"priority", priority,
		"evidence_dir", pkg.Dir,
		"manifest_sha256", manifestHash,
		"duration_ms", time.Since(captureStart).Milliseconds(),
	)
}
