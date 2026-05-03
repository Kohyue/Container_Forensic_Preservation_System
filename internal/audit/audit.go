// Package audit provides structured, append-only audit logging of all forensic actions.
// Each entry is a JSON line so logs can be parsed programmatically.
package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Action describes what the agent did.
type Action string

const (
	ActionAlertReceived  Action = "ALERT_RECEIVED"
	ActionCaptureStarted Action = "CAPTURE_STARTED"
	ActionCaptureSuccess Action = "CAPTURE_SUCCESS"
	ActionCaptureFailure Action = "CAPTURE_FAILURE"
	ActionEvidenceStored Action = "EVIDENCE_STORED"
	ActionHashComputed   Action = "HASH_COMPUTED"
	ActionAgentStarted   Action = "AGENT_STARTED"
	ActionAgentStopped   Action = "AGENT_STOPPED"
)

// Entry is one audit log record.
type Entry struct {
	Timestamp   time.Time         `json:"timestamp"`
	Action      Action            `json:"action"`
	ContainerID string            `json:"container_id,omitempty"`
	EvidenceDir string            `json:"evidence_dir,omitempty"`
	Rule        string            `json:"rule,omitempty"`
	Priority    string            `json:"priority,omitempty"`
	Details     map[string]string `json:"details,omitempty"`
	Error       string            `json:"error,omitempty"`
}

// Logger writes audit entries to a file in JSONL format.
type Logger struct {
	mu  sync.Mutex
	out io.WriteCloser
}

// NewLogger opens (or creates) the audit log at path.
func NewLogger(path string) (*Logger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, fmt.Errorf("open audit log %q: %w", path, err)
	}
	return &Logger{out: f}, nil
}

// NewStdoutLogger returns a Logger that writes to stdout (useful for testing).
func NewStdoutLogger() *Logger {
	return &Logger{out: os.Stdout}
}

func (l *Logger) Log(e Entry) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.out.Write(append(data, '\n'))
}

func (l *Logger) Close() error {
	return l.out.Close()
}

// Convenience constructors

func AlertReceived(containerID, rule, priority string) Entry {
	return Entry{Action: ActionAlertReceived, ContainerID: containerID, Rule: rule, Priority: priority}
}

func CaptureStarted(containerID, evidenceDir string) Entry {
	return Entry{Action: ActionCaptureStarted, ContainerID: containerID, EvidenceDir: evidenceDir}
}

func CaptureSuccess(containerID, evidenceDir string, details map[string]string) Entry {
	return Entry{Action: ActionCaptureSuccess, ContainerID: containerID, EvidenceDir: evidenceDir, Details: details}
}

func CaptureFailure(containerID string, err error) Entry {
	return Entry{Action: ActionCaptureFailure, ContainerID: containerID, Error: err.Error()}
}

func EvidenceStored(containerID, evidenceDir, manifestHash string) Entry {
	return Entry{
		Action:      ActionEvidenceStored,
		ContainerID: containerID,
		EvidenceDir: evidenceDir,
		Details:     map[string]string{"manifest_sha256": manifestHash},
	}
}
