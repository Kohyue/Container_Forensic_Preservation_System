// Package preserver orchestrates the full preservation pipeline:
//  1. Write collected evidence files to a repository package.
//  2. Compute SHA-256 hashes for every artifact.
//  3. Write a signed manifest.json with hashes and RFC3339 timestamps.
//  4. Return the manifest hash for the audit log.
package preserver

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"forensic-preservation/internal/collector"
	"forensic-preservation/internal/repository"
)

// ManifestEntry records one artifact in the evidence manifest.
type ManifestEntry struct {
	Filename   string    `json:"filename"`
	SHA256     string    `json:"sha256"`
	SizeBytes  int64     `json:"size_bytes"`
	CapturedAt time.Time `json:"captured_at"`
}

// Manifest is the top-level manifest written as manifest.json inside the
// evidence package.
type Manifest struct {
	SchemaVersion string          `json:"schema_version"`
	ContainerID   string          `json:"container_id"`
	ContainerName string          `json:"container_name"`
	CapturedAt    time.Time       `json:"captured_at"`
	TriggerRule   string          `json:"trigger_rule"`
	TriggerPrio   string          `json:"trigger_priority"`
	// CaptureDurationMs is the total time in milliseconds from when the
	// capture pipeline started to when the manifest was finalised.
	CaptureDurationMs int64           `json:"capture_duration_ms,omitempty"`
	Artifacts         []ManifestEntry `json:"artifacts"`
	ManifestSHA256    string          `json:"manifest_sha256,omitempty"`
}

// Preserve writes evidence files to pkg, builds a manifest, and returns the
// manifest hash. captureStart is the timestamp recorded when the capture
// pipeline began; it is used to compute CaptureDurationMs.
func Preserve(pkg *repository.Package, ev *collector.Evidence, rule, priority string, captureStart time.Time) (string, error) {
	capturedAt := time.Now().UTC()

	// Write each artifact to disk.
	artifacts := []struct{ filename, content string }{
		{"metadata.json", ev.Metadata},
		{"process.txt", ev.ProcessList},
		{"logs.txt", ev.Logs},
		{"network.txt", ev.Network},
	}
	for _, a := range artifacts {
		if err := pkg.WriteFile(a.filename, a.content); err != nil {
			return "", fmt.Errorf("write artifact %q: %w", a.filename, err)
		}
	}

	// Hash every file that was written.
	files, err := pkg.ListFiles()
	if err != nil {
		return "", fmt.Errorf("list evidence files: %w", err)
	}

	manifest := Manifest{
		SchemaVersion: "1",
		ContainerID:   ev.ContainerID,
		ContainerName: ev.ContainerName,
		CapturedAt:    capturedAt,
		TriggerRule:   rule,
		TriggerPrio:   priority,
	}

	for _, path := range files {
		hash, err := HashFile(path)
		if err != nil {
			return "", fmt.Errorf("hash %q: %w", path, err)
		}
		info, err := statFile(path)
		if err != nil {
			return "", err
		}
		manifest.Artifacts = append(manifest.Artifacts, ManifestEntry{
			Filename:   filepath.Base(path),
			SHA256:     hash,
			SizeBytes:  info,
			CapturedAt: capturedAt,
		})
	}

	// Record how long the capture took up to this point.
	manifest.CaptureDurationMs = time.Since(captureStart).Milliseconds()

	// Serialise manifest without its own hash first.
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal manifest: %w", err)
	}

	// Self-hash the manifest content and embed it.
	manifestHash := HashString(string(manifestJSON))
	manifest.ManifestSHA256 = manifestHash

	finalJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal manifest (final): %w", err)
	}

	if err := pkg.WriteFile("manifest.json", string(finalJSON)); err != nil {
		return "", fmt.Errorf("write manifest: %w", err)
	}

	return manifestHash, nil
}
