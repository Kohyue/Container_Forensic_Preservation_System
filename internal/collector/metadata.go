package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// collectMetadata runs `docker inspect` and returns the raw JSON plus the container name.
func collectMetadata(ctx context.Context, containerID string) (string, string, error) {
	out, err := runCmd(ctx, "docker", "inspect", containerID)
	if err != nil {
		return "", "", err
	}

	// Parse just enough to extract the container name for the Evidence struct.
	name := extractContainerName(out)
	return out, name, nil
}

// extractContainerName pulls the Name field from `docker inspect` JSON output.
func extractContainerName(inspectJSON string) string {
	var items []struct {
		Name string `json:"Name"`
	}
	if err := json.Unmarshal([]byte(inspectJSON), &items); err != nil || len(items) == 0 {
		return ""
	}
	return strings.TrimPrefix(items[0].Name, "/")
}

// runCmd executes a command and returns combined stdout. Stderr is included on error.
func runCmd(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("command %q %v failed: %w — stderr: %s", name, args, err, stderr.String())
	}
	return stdout.String(), nil
}
