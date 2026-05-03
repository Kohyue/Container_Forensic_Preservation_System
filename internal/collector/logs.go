package collector

import (
	"context"
	"fmt"
	"strconv"
)

// collectLogs captures the most recent maxLines log lines from the container.
// Timestamps are always included for forensic timeline reconstruction.
func collectLogs(ctx context.Context, containerID string, maxLines int) (string, error) {
	if maxLines <= 0 {
		maxLines = 1000
	}
	out, err := runCmd(ctx, "docker", "logs",
		"--timestamps",
		"--tail", strconv.Itoa(maxLines),
		containerID,
	)
	if err != nil {
		return "", fmt.Errorf("docker logs: %w", err)
	}
	return addHeader(fmt.Sprintf("CONTAINER LOGS (last %d lines, with timestamps)", maxLines), out), nil
}
