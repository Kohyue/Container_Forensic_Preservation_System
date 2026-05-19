// Settings section
const Settings = (() => {
  async function load() {
    try {
      const [cfg, rules, status] = await Promise.all([API.config(), API.rules(), API.status()]);
      renderConfig(cfg);
      renderRules(rules.rules || []);
      renderInfo(status);
      renderCollection(cfg.collector || {});
      renderCaptureMode(cfg.schedule || {});
    } catch (e) {
      Toast.error('Failed to load settings: ' + e.message);
    }
  }

  // ── Agent Configuration (read-only display) ──────────────────────────────
  function renderConfig(cfg) {
    const el = document.getElementById('settings-config');
    el.textContent = formatConfigDisplay(cfg);
  }

  function formatConfigDisplay(cfg) {
    const lines = [];
    function walk(obj, indent) {
      for (const [k, v] of Object.entries(obj)) {
        if (v !== null && typeof v === 'object' && !Array.isArray(v)) {
          lines.push(`${indent}${k}:`);
          walk(v, indent + '  ');
        } else {
          lines.push(`${indent}${k}: ${JSON.stringify(v)}`);
        }
      }
    }
    walk(cfg, '');
    return lines.join('\n');
  }

  // ── Detection Rules ───────────────────────────────────────────────────────
  function renderRules(rules) {
    const el = document.getElementById('settings-rules');
    if (!rules.length) {
      el.innerHTML = '<div class="empty-state">No rules loaded. Check rules_path in config.</div>';
      return;
    }
    el.innerHTML = rules.map(r => `
      <div class="rule-item">
        <div class="rule-name">
          ${esc(r.name)}
          ${r.priority ? badge(r.priority) : ''}
        </div>
        ${r.description ? `<div class="rule-desc">${esc(r.description)}</div>` : ''}
        ${r.tags?.length ? `<div class="rule-tags">${r.tags.map(t => `<span class="rule-tag">${esc(t)}</span>`).join('')}</div>` : ''}
      </div>`).join('');
  }

  // ── Agent Information ─────────────────────────────────────────────────────
  function renderInfo(s) {
    const el = document.getElementById('settings-info');
    const items = [
      { key: 'Version',    val: s.version      || '—' },
      { key: 'Node',       val: s.node_name    || '—' },
      { key: 'Webhook',    val: s.webhook_addr || '—' },
      { key: 'Web UI',     val: s.web_addr     || '—' },
      { key: 'Started At', val: s.started_at ? new Date(s.started_at).toLocaleString() : '—' },
      { key: 'Uptime',     val: fmtUptime(s.uptime_seconds) },
      { key: 'Status',     val: s.status       || '—' },
    ];
    el.innerHTML = `<div class="info-grid">${items.map(i =>
      `<div class="info-item">
        <span class="info-key">${esc(i.key)}</span>
        <span class="info-val">${esc(String(i.val))}</span>
      </div>`).join('')}</div>`;
  }

  // ── Data Collection Settings (editable) ───────────────────────────────────
  function renderCollection(col) {
    const el = document.getElementById('settings-collection');
    if (!el) return;

    const toggles = [
      { key: 'collect_process',  label: 'Collect Process List',       val: col.collect_process  !== false },
      { key: 'collect_logs',     label: 'Collect Container Logs',     val: col.collect_logs     !== false },
      { key: 'collect_network',  label: 'Collect Network State',      val: col.collect_network  !== false },
      { key: 'collect_metadata', label: 'Collect Container Metadata', val: col.collect_metadata !== false },
      { key: 'capture_on_exit',  label: 'Capture on Container Exit',  val: col.capture_on_exit  !== false },
    ];

    el.innerHTML = `
      <div class="setting-form">
        ${toggles.map(t => `
          <div class="toggle-row">
            <span class="toggle-label">${esc(t.label)}</span>
            <label class="toggle-switch">
              <input type="checkbox" id="col-${t.key}" ${t.val ? 'checked' : ''}>
              <span class="toggle-slider"></span>
            </label>
          </div>`).join('')}

        <div class="setting-divider"></div>

        <div class="form-group">
          <label class="form-label" for="col-max-log-lines">Max Log Lines</label>
          <input type="number" id="col-max-log-lines" class="form-input"
                 value="${col.max_log_lines ?? 1000}" min="1" max="100000">
        </div>
        <div class="form-group">
          <label class="form-label" for="col-timeout">Collection Timeout (seconds)</label>
          <input type="number" id="col-timeout" class="form-input"
                 value="${col.timeout_seconds ?? 30}" min="5" max="300">
        </div>

        <div style="margin-top:16px">
          <button class="btn btn-primary" onclick="Settings.saveCollection()">Save Changes</button>
        </div>
      </div>`;
  }

  async function saveCollection() {
    const data = {
      collect_process:  document.getElementById('col-collect_process')?.checked,
      collect_logs:     document.getElementById('col-collect_logs')?.checked,
      collect_network:  document.getElementById('col-collect_network')?.checked,
      collect_metadata: document.getElementById('col-collect_metadata')?.checked,
      capture_on_exit:  document.getElementById('col-capture_on_exit')?.checked,
      max_log_lines:    parseInt(document.getElementById('col-max-log-lines')?.value, 10) || undefined,
      timeout_seconds:  parseInt(document.getElementById('col-timeout')?.value, 10)       || undefined,
    };
    // Strip undefined values
    Object.keys(data).forEach(k => { if (data[k] === undefined) delete data[k]; });

    try {
      await API.updateCollector(data);
      Toast.success('Data collection settings saved');
    } catch (e) {
      Toast.error('Failed to save: ' + e.message);
    }
  }

  // ── Capture Mode Settings (editable) ─────────────────────────────────────
  function renderCaptureMode(sched) {
    const el = document.getElementById('settings-capture-mode');
    if (!el) return;

    const isScheduled = sched.enabled === true;
    const interval    = sched.interval_seconds ?? 300;

    el.innerHTML = `
      <div class="setting-form">
        <p class="setting-description">
          <strong>Automated</strong> — evidence captured immediately when a Falco alert fires.<br>
          <strong>Scheduled</strong> — alerts are queued and captured in batches at a fixed interval,
          reducing system load during alert storms.
        </p>

        <div class="capture-mode-options">
          <label class="mode-option ${!isScheduled ? 'mode-option--active' : ''}">
            <input type="radio" name="capture-mode" value="automated"
                   ${!isScheduled ? 'checked' : ''}
                   onchange="Settings.onModeChange(this.value)">
            <div class="mode-option-body">
              <span class="mode-option-title">⚡ Automated</span>
              <span class="mode-option-desc">Capture immediately on each alert</span>
            </div>
          </label>

          <label class="mode-option ${isScheduled ? 'mode-option--active' : ''}">
            <input type="radio" name="capture-mode" value="scheduled"
                   ${isScheduled ? 'checked' : ''}
                   onchange="Settings.onModeChange(this.value)">
            <div class="mode-option-body">
              <span class="mode-option-title">🕐 Scheduled</span>
              <span class="mode-option-desc">Batch captures at a regular interval</span>
            </div>
          </label>
        </div>

        <div id="schedule-interval-wrap" style="${isScheduled ? '' : 'display:none'}">
          <div class="setting-divider"></div>
          <div class="form-group">
            <label class="form-label" for="sched-interval">Capture Interval (seconds)</label>
            <input type="number" id="sched-interval" class="form-input"
                   value="${interval}" min="10" max="86400">
            <span class="form-hint">Minimum 10 s · Default 300 s (5 min)</span>
          </div>
        </div>

        <div style="margin-top:16px">
          <button class="btn btn-primary" onclick="Settings.saveSchedule()">Save Changes</button>
        </div>
      </div>`;
  }

  function onModeChange(value) {
    const wrap = document.getElementById('schedule-interval-wrap');
    if (wrap) wrap.style.display = value === 'scheduled' ? '' : 'none';
    document.querySelectorAll('.mode-option').forEach(lbl => {
      const radio = lbl.querySelector('input[type=radio]');
      lbl.classList.toggle('mode-option--active', radio?.checked);
    });
  }

  async function saveSchedule() {
    const mode     = document.querySelector('input[name="capture-mode"]:checked')?.value;
    const interval = parseInt(document.getElementById('sched-interval')?.value, 10);

    const data = { enabled: mode === 'scheduled' };
    if (!isNaN(interval) && interval >= 10) data.interval_seconds = interval;

    try {
      await API.updateSchedule(data);
      Toast.success(`Capture mode set to ${mode === 'scheduled' ? 'Scheduled' : 'Automated'}`);
    } catch (e) {
      Toast.error('Failed to save: ' + e.message);
    }
  }

  // ── Uptime helper ─────────────────────────────────────────────────────────
  function fmtUptime(sec) {
    if (!sec && sec !== 0) return '—';
    const d = Math.floor(sec / 86400);
    const h = Math.floor((sec % 86400) / 3600);
    const m = Math.floor((sec % 3600) / 60);
    const s = sec % 60;
    if (d > 0) return `${d}d ${h}h ${m}m`;
    if (h > 0) return `${h}h ${m}m ${s}s`;
    if (m > 0) return `${m}m ${s}s`;
    return `${s}s`;
  }

  return { load, saveCollection, saveSchedule, onModeChange };
})();
