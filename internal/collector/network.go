package collector

import (
	"context"
	"fmt"
	"strings"
)

// collectNetwork captures the network state visible from inside the container.
// It tries several commands in order, combining whatever succeeds.
func collectNetwork(ctx context.Context, containerID string) (string, error) {
	var parts []string
	var firstErr error

	type netCmd struct {
		label string
		args  []string
	}
	cmds := []netCmd{
		{"ss -tulpn (socket statistics)", []string{"docker", "exec", containerID, "ss", "-tulpn"}},
		{"netstat -tulpn", []string{"docker", "exec", containerID, "netstat", "-tulpn"}},
		{"cat /proc/net/tcp", []string{"docker", "exec", containerID, "cat", "/proc/net/tcp"}},
		{"cat /proc/net/tcp6", []string{"docker", "exec", containerID, "cat", "/proc/net/tcp6"}},
		{"cat /proc/net/udp", []string{"docker", "exec", containerID, "cat", "/proc/net/udp"}},
	}

	for _, c := range cmds {
		out, err := runCmd(ctx, c.args[0], c.args[1:]...)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		parts = append(parts, addHeader("NETWORK: "+c.label, out))
	}

	if len(parts) == 0 {
		return "", fmt.Errorf("all network commands failed; first error: %w", firstErr)
	}
	return strings.Join(parts, "\n"), nil
}
