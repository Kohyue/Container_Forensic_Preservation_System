package collector

import (
	"context"
	"fmt"
	"strings"
)

// collectProcessList returns a snapshot of all processes running inside the container.
// It first tries `docker top` (works for any container runtime).
// Falls back to `docker exec ps` for containers that have ps installed.
func collectProcessList(ctx context.Context, containerID string) (string, error) {
	// docker top shows the host-visible view of container processes.
	out, err := runCmd(ctx, "docker", "top", containerID, "-eo", "pid,ppid,user,pcpu,pmem,stat,start,time,comm,args")
	if err == nil {
		return addHeader("PROCESS LIST (docker top)", out), nil
	}
	topErr := err

	// Fallback: exec ps inside the container.
	out, err = runCmd(ctx, "docker", "exec", containerID, "ps", "aux")
	if err == nil {
		return addHeader("PROCESS LIST (ps aux inside container)", out), nil
	}

	return "", fmt.Errorf("docker top: %w; exec ps: %w", topErr, err)
}

func addHeader(header, content string) string {
	var sb strings.Builder
	sb.WriteString("=== ")
	sb.WriteString(header)
	sb.WriteString(" ===\n")
	sb.WriteString(content)
	return sb.String()
}
