// Package repository manages the on-disk forensic evidence store.
//
// Layout on disk:
//
//	<base>/
//	  <timestamp>_<container-short-id>/
//	    metadata.json       — docker inspect output
//	    process.txt         — process list snapshot
//	    logs.txt            — container log tail
//	    network.txt         — network state
//	    manifest.json       — artifact list with SHA-256 hashes and timestamps
//	    audit.jsonl         — per-evidence audit trail (subset of global log)
package repository

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Package is an open evidence package directory for one capture event.
type Package struct {
	Dir       string
	CreatedAt time.Time
}

// Create allocates a new, empty evidence package directory and returns it.
func Create(basePath, containerID string, at time.Time) (*Package, error) {
	shortID := containerID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	// Format: 20060102T150405Z_<short-container-id>
	dirName := fmt.Sprintf("%s_%s", at.UTC().Format("20060102T150405Z"), shortID)
	dir := filepath.Join(basePath, dirName)

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create evidence dir %q: %w", dir, err)
	}
	return &Package{Dir: dir, CreatedAt: at}, nil
}

// WriteFile writes content to a named file inside the evidence package.
// It is a no-op (returns nil) when content is empty.
func (p *Package) WriteFile(name, content string) error {
	if content == "" {
		return nil
	}
	path := filepath.Join(p.Dir, name)
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}

// ListFiles returns the paths of all files inside the evidence directory.
func (p *Package) ListFiles() ([]string, error) {
	entries, err := os.ReadDir(p.Dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if !e.IsDir() {
			paths = append(paths, filepath.Join(p.Dir, e.Name()))
		}
	}
	return paths, nil
}

// EnsureBaseDir creates the base evidence directory if it does not exist.
func EnsureBaseDir(basePath string) error {
	return os.MkdirAll(basePath, 0o750)
}
