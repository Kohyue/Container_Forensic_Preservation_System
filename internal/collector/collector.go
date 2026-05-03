// Package collector gathers volatile forensic artifacts from a running container.
// All operations use the Docker CLI / /proc filesystem so no heavy SDK is needed.
package collector

import (
	"context"
	"fmt"
)

// Evidence holds every artifact collected for one container at one point in time.
type Evidence struct {
	ContainerID   string
	ContainerName string
	// Each field is the raw text of the captured artifact (empty string = not collected).
	ProcessList string
	Logs        string
	Network     string
	Metadata    string
}

// Options controls which artifact types are collected.
type Options struct {
	CollectProcess  bool
	CollectLogs     bool
	CollectNetwork  bool
	CollectMetadata bool
	MaxLogLines     int
}

// Collect gathers all requested artifacts for the named container.
func Collect(ctx context.Context, containerID string, opts Options) (*Evidence, error) {
	ev := &Evidence{ContainerID: containerID}
	var errs []error

	if opts.CollectMetadata {
		meta, name, err := collectMetadata(ctx, containerID)
		if err != nil {
			errs = append(errs, fmt.Errorf("metadata: %w", err))
		} else {
			ev.Metadata = meta
			ev.ContainerName = name
		}
	}

	if opts.CollectProcess {
		procs, err := collectProcessList(ctx, containerID)
		if err != nil {
			errs = append(errs, fmt.Errorf("process list: %w", err))
		} else {
			ev.ProcessList = procs
		}
	}

	if opts.CollectLogs {
		logs, err := collectLogs(ctx, containerID, opts.MaxLogLines)
		if err != nil {
			errs = append(errs, fmt.Errorf("logs: %w", err))
		} else {
			ev.Logs = logs
		}
	}

	if opts.CollectNetwork {
		net, err := collectNetwork(ctx, containerID)
		if err != nil {
			errs = append(errs, fmt.Errorf("network: %w", err))
		} else {
			ev.Network = net
		}
	}

	if len(errs) > 0 {
		// Return partial evidence alongside errors so callers can preserve what was collected.
		return ev, fmt.Errorf("partial collection errors: %v", errs)
	}
	return ev, nil
}
