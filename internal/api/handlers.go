package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"forensic-preservation/internal/audit"
	"forensic-preservation/internal/preserver"
)

const version = "0.1.0"

// ── helpers ──────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ── /api/v1/status ────────────────────────────────────────────────────────────

type statusResponse struct {
	Status        string    `json:"status"`
	StartedAt     time.Time `json:"started_at"`
	UptimeSeconds int64     `json:"uptime_seconds"`
	NodeName      string    `json:"node_name"`
	Version       string    `json:"version"`
	WebhookAddr   string    `json:"webhook_addr"`
	WebAddr       string    `json:"web_addr"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	node := s.cfg.Agent.NodeName
	if node == "" {
		node, _ = os.Hostname()
	}
	writeJSON(w, http.StatusOK, statusResponse{
		Status:        "running",
		StartedAt:     s.startedAt,
		UptimeSeconds: int64(time.Since(s.startedAt).Seconds()),
		NodeName:      node,
		Version:       version,
		WebhookAddr:   s.cfg.Detector.ListenAddr,
		WebAddr:       s.cfg.Web.Addr,
	})
}

// ── /api/v1/stats ─────────────────────────────────────────────────────────────

type statsResponse struct {
	TotalAlerts      int                `json:"total_alerts"`
	TotalEvidence    int                `json:"total_evidence"`
	AlertsByPriority map[string]int     `json:"alerts_by_priority"`
	RecentAlerts     []audit.Entry      `json:"recent_alerts"`
	RecentEvidence   []evidenceListItem `json:"recent_evidence"`
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	allAlerts := s.readAlertEntries()
	byPriority := make(map[string]int)
	for _, a := range allAlerts {
		byPriority[strings.ToLower(a.Priority)]++
	}

	recent := allAlerts
	if len(recent) > 5 {
		recent = recent[len(recent)-5:]
	}
	// reverse so newest first
	for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
		recent[i], recent[j] = recent[j], recent[i]
	}

	evidence := s.listEvidencePackages()
	recentEv := evidence
	if len(recentEv) > 5 {
		recentEv = recentEv[:5]
	}

	writeJSON(w, http.StatusOK, statsResponse{
		TotalAlerts:      len(allAlerts),
		TotalEvidence:    len(evidence),
		AlertsByPriority: byPriority,
		RecentAlerts:     recent,
		RecentEvidence:   recentEv,
	})
}

// ── /api/v1/alerts ────────────────────────────────────────────────────────────

type alertsResponse struct {
	Total  int           `json:"total"`
	Alerts []audit.Entry `json:"alerts"`
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	all := s.readAlertEntries()

	// Filter by priority
	if p := r.URL.Query().Get("priority"); p != "" {
		filtered := all[:0]
		for _, a := range all {
			if strings.EqualFold(a.Priority, p) {
				filtered = append(filtered, a)
			}
		}
		all = filtered
	}

	// Filter by search term (matches rule or container_id)
	if q := r.URL.Query().Get("search"); q != "" {
		q = strings.ToLower(q)
		filtered := all[:0]
		for _, a := range all {
			if strings.Contains(strings.ToLower(a.Rule), q) ||
				strings.Contains(strings.ToLower(a.ContainerID), q) {
				filtered = append(filtered, a)
			}
		}
		all = filtered
	}

	total := len(all)

	// Reverse so newest first
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}

	// Pagination
	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}
	if offset >= len(all) {
		all = nil
	} else {
		all = all[offset:]
		if len(all) > limit {
			all = all[:limit]
		}
	}

	writeJSON(w, http.StatusOK, alertsResponse{Total: total, Alerts: all})
}

// ── /api/v1/evidence ─────────────────────────────────────────────────────────

type evidenceListItem struct {
	ID                string    `json:"id"`
	ContainerID       string    `json:"container_id"`
	ContainerName     string    `json:"container_name"`
	CapturedAt        time.Time `json:"captured_at"`
	TriggerRule       string    `json:"trigger_rule"`
	TriggerPrio       string    `json:"trigger_priority"`
	ArtifactCount     int       `json:"artifact_count"`
	CaptureDurationMs int64     `json:"capture_duration_ms"`
	Dir               string    `json:"dir"`
}

type evidenceListResponse struct {
	Total    int                `json:"total"`
	Packages []evidenceListItem `json:"packages"`
}

func (s *Server) handleListEvidence(w http.ResponseWriter, r *http.Request) {
	pkgs := s.listEvidencePackages()
	total := len(pkgs)

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}
	if offset < len(pkgs) {
		pkgs = pkgs[offset:]
	} else {
		pkgs = nil
	}
	if len(pkgs) > limit {
		pkgs = pkgs[:limit]
	}

	writeJSON(w, http.StatusOK, evidenceListResponse{Total: total, Packages: pkgs})
}

// ── /api/v1/evidence/{id} and /api/v1/evidence/{id}/file/{name} ──────────────

type evidenceDetailResponse struct {
	evidenceListItem
	Manifest *preserver.Manifest `json:"manifest"`
	Files    []string            `json:"files"`
}

func (s *Server) handleEvidence(w http.ResponseWriter, r *http.Request) {
	// Strip prefix "/api/v1/evidence/"
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/evidence/")
	parts := strings.SplitN(rest, "/file/", 2)
	pkgID := parts[0]

	if !isSafeID(pkgID) {
		writeError(w, http.StatusBadRequest, "invalid evidence ID")
		return
	}

	pkgDir := filepath.Join(s.cfg.Repository.BasePath, pkgID)
	if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "evidence package not found")
		return
	}

	// /api/v1/evidence/{id}/file/{name}
	if len(parts) == 2 {
		filename := filepath.Base(parts[1])
		if !isSafeFilename(filename) {
			writeError(w, http.StatusBadRequest, "invalid filename")
			return
		}
		fullPath := filepath.Join(pkgDir, filename)
		if !strings.HasPrefix(fullPath, pkgDir+string(filepath.Separator)) &&
			fullPath != pkgDir {
			writeError(w, http.StatusBadRequest, "invalid path")
			return
		}
		content, err := os.ReadFile(fullPath)
		if err != nil {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
		return
	}

	// /api/v1/evidence/{id} — return manifest + file list
	manifest := readManifest(pkgDir)
	files := listDir(pkgDir)
	item := buildListItem(pkgID, pkgDir, manifest)

	writeJSON(w, http.StatusOK, evidenceDetailResponse{
		evidenceListItem: item,
		Manifest:         manifest,
		Files:            files,
	})
}

// ── /api/v1/config ────────────────────────────────────────────────────────────

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg)
}

// ── /api/v1/config/collector (POST) ──────────────────────────────────────────

type collectorUpdateRequest struct {
	CollectProcess  *bool `json:"collect_process"`
	CollectLogs     *bool `json:"collect_logs"`
	CollectNetwork  *bool `json:"collect_network"`
	CollectMetadata *bool `json:"collect_metadata"`
	CaptureOnExit   *bool `json:"capture_on_exit"`
	MaxLogLines     *int  `json:"max_log_lines"`
	TimeoutSeconds  *int  `json:"timeout_seconds"`
}

func (s *Server) handleUpdateCollector(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req collectorUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.mu.Lock()
	if req.CollectProcess != nil {
		s.cfg.Collector.CollectProcess = *req.CollectProcess
	}
	if req.CollectLogs != nil {
		s.cfg.Collector.CollectLogs = *req.CollectLogs
	}
	if req.CollectNetwork != nil {
		s.cfg.Collector.CollectNetwork = *req.CollectNetwork
	}
	if req.CollectMetadata != nil {
		s.cfg.Collector.CollectMetadata = *req.CollectMetadata
	}
	if req.CaptureOnExit != nil {
		s.cfg.Collector.CaptureOnExit = *req.CaptureOnExit
	}
	if req.MaxLogLines != nil && *req.MaxLogLines > 0 {
		s.cfg.Collector.MaxLogLines = *req.MaxLogLines
	}
	if req.TimeoutSeconds != nil && *req.TimeoutSeconds > 0 {
		s.cfg.Collector.TimeoutSeconds = *req.TimeoutSeconds
	}
	saveErr := s.cfg.Save()
	s.mu.Unlock()

	if saveErr != nil {
		s.logger.Warn("collector config updated in memory but could not persist to disk", "err", saveErr)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ── /api/v1/config/schedule (POST) ───────────────────────────────────────────

type scheduleUpdateRequest struct {
	Enabled         *bool `json:"enabled"`
	IntervalSeconds *int  `json:"interval_seconds"`
}

func (s *Server) handleUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req scheduleUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.mu.Lock()
	if req.Enabled != nil {
		s.cfg.Schedule.Enabled = *req.Enabled
	}
	if req.IntervalSeconds != nil && *req.IntervalSeconds >= 10 {
		s.cfg.Schedule.IntervalSeconds = *req.IntervalSeconds
	}
	saveErr := s.cfg.Save()
	s.mu.Unlock()

	if saveErr != nil {
		s.logger.Warn("schedule config updated in memory but could not persist to disk", "err", saveErr)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ── /api/v1/rules ─────────────────────────────────────────────────────────────

type ruleEntry struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Priority    string   `json:"priority"`
	Tags        []string `json:"tags"`
}

type rulesResponse struct {
	Path  string      `json:"path"`
	Rules []ruleEntry `json:"rules"`
}

func (s *Server) handleGetRules(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(s.cfg.Web.RulesPath)
	if err != nil {
		writeJSON(w, http.StatusOK, rulesResponse{Path: s.cfg.Web.RulesPath, Rules: nil})
		return
	}
	rules := parseFalcoRulesYAML(string(data))
	writeJSON(w, http.StatusOK, rulesResponse{Path: s.cfg.Web.RulesPath, Rules: rules})
}

// ── /api/v1/containers ───────────────────────────────────────────────────────

type monitoredContainer struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Image      string `json:"image"`
	State      string `json:"state"`
	Status     string `json:"status"`
	RunningFor string `json:"running_for"`
	AlertCount int    `json:"alert_count"`
}

type containersResponse struct {
	Containers []monitoredContainer `json:"containers"`
	Total      int                  `json:"total"`
}

func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request) {
	out, err := exec.Command("docker", "ps", "--format", "{{json .}}").Output()
	if err != nil {
		// Docker not accessible — return empty list gracefully.
		writeJSON(w, http.StatusOK, containersResponse{Containers: []monitoredContainer{}, Total: 0})
		return
	}

	alertCounts := s.alertCountByContainer()

	var containers []monitoredContainer
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw struct {
			ID         string `json:"ID"`
			Names      string `json:"Names"`
			Image      string `json:"Image"`
			State      string `json:"State"`
			Status     string `json:"Status"`
			RunningFor string `json:"RunningFor"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		name := strings.TrimPrefix(raw.Names, "/")
		// Exclude the agent, Falco, and Kubernetes infrastructure containers.
		if name == "forensic-agent" || name == "falco" ||
			strings.Contains(name, "control-plane") ||
			strings.Contains(name, "etcd") ||
			strings.HasSuffix(name, "-worker") {
			continue
		}
		shortID := raw.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		containers = append(containers, monitoredContainer{
			ID:         shortID,
			Name:       name,
			Image:      raw.Image,
			State:      raw.State,
			Status:     raw.Status,
			RunningFor: raw.RunningFor,
			AlertCount: alertCounts[shortID],
		})
	}
	if containers == nil {
		containers = []monitoredContainer{}
	}
	writeJSON(w, http.StatusOK, containersResponse{Containers: containers, Total: len(containers)})
}

// alertCountByContainer returns a map of short container ID → alert count.
func (s *Server) alertCountByContainer() map[string]int {
	counts := make(map[string]int)
	for _, e := range s.readAlertEntries() {
		id := e.ContainerID
		if len(id) > 12 {
			id = id[:12]
		}
		if id != "" {
			counts[id]++
		}
	}
	return counts
}

// ── /api/v1/metrics ──────────────────────────────────────────────────────────

type metricsResponse struct {
	AllocMB      float64 `json:"alloc_mb"`
	SysMB        float64 `json:"sys_mb"`
	HeapInUseMB  float64 `json:"heap_inuse_mb"`
	Goroutines   int     `json:"goroutines"`
	GCRuns       uint32  `json:"gc_runs"`
	BinarySizeKB int64   `json:"binary_size_kb"`
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	// Try to get binary size from /proc/self/exe (Linux only).
	var binaryKB int64
	if info, err := os.Stat("/proc/self/exe"); err == nil {
		binaryKB = info.Size() / 1024
	}

	writeJSON(w, http.StatusOK, metricsResponse{
		AllocMB:      float64(ms.Alloc) / 1024 / 1024,
		SysMB:        float64(ms.Sys) / 1024 / 1024,
		HeapInUseMB:  float64(ms.HeapInuse) / 1024 / 1024,
		Goroutines:   runtime.NumGoroutine(),
		GCRuns:       ms.NumGC,
		BinarySizeKB: binaryKB,
	})
}

// ── /api/v1/reset (POST) ─────────────────────────────────────────────────────

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var deletedPackages, deletedFiles int

	// Delete all evidence package directories.
	entries, err := os.ReadDir(s.cfg.Repository.BasePath)
	if err != nil && !os.IsNotExist(err) {
		writeError(w, http.StatusInternalServerError, "failed to read evidence dir: "+err.Error())
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(s.cfg.Repository.BasePath, entry.Name())
		// Count files inside before removing.
		if fis, err := os.ReadDir(dir); err == nil {
			deletedFiles += len(fis)
		}
		if err := os.RemoveAll(dir); err != nil {
			s.logger.Warn("failed to remove evidence package", "dir", dir, "err", err)
		} else {
			deletedPackages++
		}
	}

	// Truncate the audit log (preserves the file, clears contents).
	if err := os.WriteFile(s.cfg.Audit.LogPath, []byte{}, 0o640); err != nil {
		s.logger.Warn("failed to clear audit log", "err", err)
	}

	// Also clear client-side localStorage read-state by returning a reset token
	// (the JS will clear it after a successful response).
	s.logger.Info("system reset performed",
		"deleted_packages", deletedPackages,
		"deleted_files", deletedFiles,
	)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":           "ok",
		"deleted_packages": deletedPackages,
		"deleted_files":    deletedFiles,
	})
}

// ── internal helpers ──────────────────────────────────────────────────────────

// readAlertEntries parses the audit log and returns all ALERT_RECEIVED entries.
func (s *Server) readAlertEntries() []audit.Entry {
	f, err := os.Open(s.cfg.Audit.LogPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var entries []audit.Entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e audit.Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		if e.Action == audit.ActionAlertReceived {
			entries = append(entries, e)
		}
	}
	return entries
}

// listEvidencePackages scans the base path and returns all valid evidence
// packages, newest first.
func (s *Server) listEvidencePackages() []evidenceListItem {
	entries, err := os.ReadDir(s.cfg.Repository.BasePath)
	if err != nil {
		return nil
	}

	var items []evidenceListItem
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pkgDir := filepath.Join(s.cfg.Repository.BasePath, e.Name())
		manifest := readManifest(pkgDir)
		items = append(items, buildListItem(e.Name(), pkgDir, manifest))
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].CapturedAt.After(items[j].CapturedAt)
	})
	return items
}

func buildListItem(id, dir string, m *preserver.Manifest) evidenceListItem {
	item := evidenceListItem{ID: id, Dir: dir}
	if m != nil {
		item.ContainerID       = m.ContainerID
		item.ContainerName     = m.ContainerName
		item.CapturedAt        = m.CapturedAt
		item.TriggerRule       = m.TriggerRule
		item.TriggerPrio       = m.TriggerPrio
		item.ArtifactCount     = len(m.Artifacts)
		item.CaptureDurationMs = m.CaptureDurationMs
	}
	return item
}

func readManifest(pkgDir string) *preserver.Manifest {
	data, err := os.ReadFile(filepath.Join(pkgDir, "manifest.json"))
	if err != nil {
		return nil
	}
	var m preserver.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return &m
}

func listDir(pkgDir string) []string {
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

func isSafeID(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func isSafeFilename(s string) bool {
	if s == "" || s == "." || s == ".." || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

// parseFalcoRulesYAML does a minimal parse of a Falco YAML rules file to
// extract rule metadata without pulling in a full YAML library.
func parseFalcoRulesYAML(content string) []ruleEntry {
	var rules []ruleEntry
	var current *ruleEntry

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- rule:") {
			if current != nil {
				rules = append(rules, *current)
			}
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, "- rule:"))
			current = &ruleEntry{Name: name}
		} else if current != nil {
			if strings.HasPrefix(trimmed, "desc:") {
				current.Description = strings.TrimSpace(strings.TrimPrefix(trimmed, "desc:"))
			} else if strings.HasPrefix(trimmed, "priority:") {
				current.Priority = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(trimmed, "priority:")))
			} else if strings.HasPrefix(trimmed, "tags:") {
				tagStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "tags:"))
				tagStr = strings.Trim(tagStr, "[]")
				for _, t := range strings.Split(tagStr, ",") {
					t = strings.TrimSpace(t)
					if t != "" {
						current.Tags = append(current.Tags, t)
					}
				}
			}
		}
	}
	if current != nil {
		rules = append(rules, *current)
	}
	return rules
}
