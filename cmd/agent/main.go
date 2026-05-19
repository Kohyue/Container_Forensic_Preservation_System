package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"forensic-preservation/internal/api"
	"forensic-preservation/internal/audit"
	"forensic-preservation/internal/collector"
	"forensic-preservation/internal/config"
	"forensic-preservation/internal/detector"
	"forensic-preservation/internal/preserver"
	"forensic-preservation/internal/repository"
)

// captureRequest holds a queued alert for scheduled batch processing.
type captureRequest struct {
	alert      *detector.FalcoAlert
	receivedAt time.Time
}

func main() {
	configPath := flag.String("config", "/etc/forensic-agent/config.yaml", "path to config file")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("failed to load config", "err", err, "path", *configPath)
		os.Exit(1)
	}

	if err := repository.EnsureBaseDir(cfg.Repository.BasePath); err != nil {
		logger.Error("failed to create evidence base dir", "err", err)
		os.Exit(1)
	}

	auditLog, err := audit.NewLogger(cfg.Audit.LogPath)
	if err != nil {
		// Fall back to stdout so we never silently lose audit events.
		logger.Warn("could not open audit log file, falling back to stdout", "err", err)
		auditLog = audit.NewStdoutLogger()
	}
	defer auditLog.Close()

	auditLog.Log(audit.Entry{Action: audit.ActionAgentStarted, Details: map[string]string{
		"config":    *configPath,
		"base_path": cfg.Repository.BasePath,
		"listen":    cfg.Detector.ListenAddr,
	}})

	startedAt := time.Now().UTC()

	// Buffered queue absorbs alert bursts in scheduled mode.
	captureQueue := make(chan captureRequest, 500)

	handler    := buildHandler(cfg, logger, auditLog, captureQueue)
	webhookSrv := detector.NewServer(cfg.Detector.ListenAddr, cfg.Detector.MinPriority, handler, logger)
	webSrv     := api.NewServer(cfg, logger, startedAt)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Scheduler goroutine: always running, no-op when schedule mode is disabled.
	go runScheduler(ctx, captureQueue, cfg, logger, auditLog)

	// Container exit watcher: captures volatile data when a container terminates.
	go runContainerWatcher(ctx, cfg, logger, auditLog)

	go func() {
		if err := webSrv.Start(ctx); err != nil {
			logger.Error("web server error", "err", err)
		}
	}()

	if err := webhookSrv.Start(ctx); err != nil {
		logger.Error("webhook server error", "err", err)
	}

	auditLog.Log(audit.Entry{Action: audit.ActionAgentStopped})
	logger.Info("agent stopped")
}

// runCapture executes the full evidence collection and preservation pipeline
// for a single alert. captureStart is measured at the top so the duration
// field in the manifest includes both collection and preservation steps.
func runCapture(ctx context.Context, cfg *config.Config, logger *slog.Logger, auditLog *audit.Logger, alert *detector.FalcoAlert) {
	containerID := alert.ContainerID()
	captureStart := time.Now()
	captureAt   := captureStart.UTC()

	pkg, err := repository.Create(cfg.Repository.BasePath, containerID, captureAt)
	if err != nil {
		logger.Error("failed to create evidence package", "container_id", containerID, "err", err)
		auditLog.Log(audit.CaptureFailure(containerID, fmt.Errorf("create package: %w", err)))
		return
	}
	auditLog.Log(audit.CaptureStarted(containerID, pkg.Dir))

	// Read collector options fresh each time so runtime Settings changes take effect.
	collectOpts := collector.Options{
		CollectProcess:  cfg.Collector.CollectProcess,
		CollectLogs:     cfg.Collector.CollectLogs,
		CollectNetwork:  cfg.Collector.CollectNetwork,
		CollectMetadata: cfg.Collector.CollectMetadata,
		MaxLogLines:     cfg.Collector.MaxLogLines,
	}

	collectCtx, cancel := context.WithTimeout(ctx, cfg.Collector.Timeout())
	defer cancel()

	ev, collectErr := collector.Collect(collectCtx, containerID, collectOpts)
	if collectErr != nil {
		// Partial evidence is still preserved — log but continue.
		logger.Warn("partial collection", "container_id", containerID, "err", collectErr)
	}

	manifestHash, err := preserver.Preserve(pkg, ev, alert.Rule, alert.Priority, captureStart)
	if err != nil {
		logger.Error("preservation failed", "container_id", containerID, "err", err)
		auditLog.Log(audit.CaptureFailure(containerID, err))
		return
	}

	auditLog.Log(audit.CaptureSuccess(containerID, pkg.Dir, map[string]string{
		"rule":     alert.Rule,
		"priority": alert.Priority,
	}))
	auditLog.Log(audit.EvidenceStored(containerID, pkg.Dir, manifestHash))

	logger.Info("evidence preserved",
		"container_id", containerID,
		"evidence_dir", pkg.Dir,
		"manifest_sha256", manifestHash,
		"duration_ms", time.Since(captureStart).Milliseconds(),
	)
}

// buildHandler returns the AlertHandler that either queues the alert for
// scheduled processing or runs the capture pipeline immediately.
func buildHandler(cfg *config.Config, logger *slog.Logger, auditLog *audit.Logger, queue chan<- captureRequest) detector.AlertHandler {
	return func(ctx context.Context, alert *detector.FalcoAlert) {
		containerID := alert.ContainerID()
		auditLog.Log(audit.AlertReceived(containerID, alert.Rule, alert.Priority))

		logger.Info("preservation triggered",
			"container_id", containerID,
			"rule", alert.Rule,
			"priority", alert.Priority,
		)

		if cfg.Schedule.Enabled {
			// Scheduled mode: enqueue and let the scheduler batch-process.
			select {
			case queue <- captureRequest{alert: alert, receivedAt: time.Now()}:
				logger.Info("alert queued for scheduled capture",
					"container_id", containerID,
					"queue_len", len(queue),
				)
			default:
				logger.Warn("capture queue full, alert dropped", "container_id", containerID)
			}
			return
		}

		// Automated mode: run immediately in a goroutine.
		go runCapture(ctx, cfg, logger, auditLog, alert)
	}
}

// runScheduler drains the capture queue at the configured interval.
// It polls every 10 seconds so interval changes from the Settings UI take
// effect without a restart. It is a no-op when schedule mode is disabled.
func runScheduler(ctx context.Context, queue <-chan captureRequest, cfg *config.Config, logger *slog.Logger, auditLog *audit.Logger) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Initialise lastRun so the first batch fires immediately if enabled.
	lastRun := time.Now().Add(-cfg.Schedule.Interval())

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !cfg.Schedule.Enabled {
				continue
			}
			if time.Since(lastRun) < cfg.Schedule.Interval() {
				continue
			}
			lastRun = time.Now()

			count := 0
		DRAIN:
			for {
				select {
				case req := <-queue:
					runCapture(ctx, cfg, logger, auditLog, req.alert)
					count++
				default:
					break DRAIN
				}
			}

			if count > 0 {
				logger.Info("scheduled capture batch complete", "count", count)
			}
		}
	}
}

// runContainerWatcher monitors Docker events and triggers evidence capture
// when a container terminates, preserving whatever volatile data is still
// available (logs and metadata remain accessible after exit).
func runContainerWatcher(ctx context.Context, cfg *config.Config, logger *slog.Logger, auditLog *audit.Logger) {
	for {
		if !cfg.Collector.CaptureOnExit {
			// Feature disabled — sleep and re-check in case it's toggled on later.
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
				continue
			}
		}

		if err := watchDockerEvents(ctx, cfg, logger, auditLog); err != nil {
			if ctx.Err() != nil {
				return // normal shutdown
			}
			logger.Warn("container watcher restarting after error", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func watchDockerEvents(ctx context.Context, cfg *config.Config, logger *slog.Logger, auditLog *audit.Logger) error {
	cmd := exec.CommandContext(ctx, "docker", "events",
		"--filter", "type=container",
		"--filter", "event=die",
		"--format", "{{json .}}")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start docker events: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		var ev struct {
			ID    string `json:"id"`
			Actor struct {
				Attributes map[string]string `json:"Attributes"`
			} `json:"Actor"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue
		}

		// Skip internal containers (the agent itself).
		name := ev.Actor.Attributes["name"]
		if name == "forensic-agent" || name == "falco" {
			continue
		}

		containerID := ev.ID
		logger.Info("container exit detected, capturing volatile data",
			"container_id", containerID,
			"container_name", name,
		)

		alert := &detector.FalcoAlert{
			Rule:     "Container Termination",
			Priority: "warning",
			Output:   fmt.Sprintf("Container %s (%s) terminated — volatile data captured at exit", containerID[:12], name),
			Time:     time.Now().UTC().Format(time.RFC3339Nano),
			Fields: map[string]interface{}{
				"container.id":   containerID,
				"container.name": name,
			},
		}

		// Log to audit so it appears on the Alerts page.
		auditLog.Log(audit.AlertReceived(containerID, alert.Rule, alert.Priority))
		go runCapture(ctx, cfg, logger, auditLog, alert)
	}

	return cmd.Wait()
}
