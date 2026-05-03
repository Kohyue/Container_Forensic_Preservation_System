package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
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
	handler := buildHandler(cfg, logger, auditLog)

	webhookSrv := detector.NewServer(cfg.Detector.ListenAddr, cfg.Detector.MinPriority, handler, logger)
	webSrv     := api.NewServer(cfg, logger, startedAt)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start both servers concurrently; either failing logs an error.
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

// buildHandler returns the AlertHandler closure that runs the full preservation pipeline.
func buildHandler(cfg *config.Config, logger *slog.Logger, auditLog *audit.Logger) detector.AlertHandler {
	collectOpts := collector.Options{
		CollectProcess:  cfg.Collector.CollectProcess,
		CollectLogs:     cfg.Collector.CollectLogs,
		CollectNetwork:  cfg.Collector.CollectNetwork,
		CollectMetadata: cfg.Collector.CollectMetadata,
		MaxLogLines:     cfg.Collector.MaxLogLines,
	}

	return func(ctx context.Context, alert *detector.FalcoAlert) {
		containerID := alert.ContainerID()
		auditLog.Log(audit.AlertReceived(containerID, alert.Rule, alert.Priority))

		logger.Info("preservation triggered",
			"container_id", containerID,
			"rule", alert.Rule,
			"priority", alert.Priority,
		)

		captureAt := time.Now().UTC()

		// Create evidence package directory.
		pkg, err := repository.Create(cfg.Repository.BasePath, containerID, captureAt)
		if err != nil {
			logger.Error("failed to create evidence package", "container_id", containerID, "err", err)
			auditLog.Log(audit.CaptureFailure(containerID, fmt.Errorf("create package: %w", err)))
			return
		}

		auditLog.Log(audit.CaptureStarted(containerID, pkg.Dir))

		// Collect volatile evidence with a timeout.
		collectCtx, cancel := context.WithTimeout(ctx, cfg.Collector.Timeout())
		defer cancel()

		ev, collectErr := collector.Collect(collectCtx, containerID, collectOpts)
		if collectErr != nil {
			// Partial evidence is still preserved — log error but continue.
			logger.Warn("partial collection", "container_id", containerID, "err", collectErr)
		}

		// Write evidence files and build the integrity manifest.
		manifestHash, err := preserver.Preserve(pkg, ev, alert.Rule, alert.Priority)
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
		)
	}
}
