// Dashboard section
const Dashboard = (() => {
  const PRIO_ORDER = ['emergency','alert','critical','error','warning','notice','informational','debug'];
  const PRIO_COLORS = {
    emergency:'var(--red)', alert:'var(--red)', critical:'var(--orange)',
    error:'var(--orange)', warning:'var(--yellow)', notice:'var(--blue)',
    informational:'var(--text-2)', debug:'var(--text-3)',
  };

  function fmtUptime(seconds) {
    const d = Math.floor(seconds / 86400);
    const h = Math.floor((seconds % 86400) / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    const s = seconds % 60;
    if (d > 0) return `${d}d ${h}h ${m}m`;
    if (h > 0) return `${h}h ${m}m ${s}s`;
    if (m > 0) return `${m}m ${s}s`;
    return `${s}s`;
  }

  function fmtTs(ts) {
    if (!ts) return '—';
    const d = new Date(ts);
    return d.toLocaleString();
  }

  async function load() {
    try {
      const [status, stats] = await Promise.all([API.status(), API.stats()]);
      renderStatus(status);
      renderStats(stats);
      renderRecentAlerts(stats.recent_alerts || []);
      renderPriorityChart(stats.alerts_by_priority || {});
      renderRecentEvidence(stats.recent_evidence || []);
    } catch (e) {
      Toast.error('Failed to load dashboard: ' + e.message);
    }
  }

  function renderStatus(s) {
    const dot   = document.getElementById('status-dot');
    const label = document.getElementById('status-label');
    const node  = document.getElementById('stat-node');
    const uptEl = document.getElementById('uptime-text');
    const statEl= document.getElementById('stat-status');

    dot.className   = 'status-dot active';
    label.textContent = 'Active';
    node.textContent  = s.node_name || '—';
    uptEl.textContent = fmtUptime(s.uptime_seconds);
    statEl.textContent = 'ACTIVE';
    statEl.style.color = 'var(--green)';

    document.getElementById('stat-uptime').textContent = fmtUptime(s.uptime_seconds);
  }

  function renderStats(s) {
    document.getElementById('stat-alerts').textContent   = s.total_alerts  ?? '—';
    document.getElementById('stat-evidence').textContent = s.total_evidence ?? '—';

    const ab = document.getElementById('nav-alerts-count');
    const eb = document.getElementById('nav-evidence-count');
    if (s.total_alerts > 0) { ab.textContent = s.total_alerts; ab.classList.add('visible'); }
    if (s.total_evidence > 0) { eb.textContent = s.total_evidence; eb.classList.add('visible'); }
  }

  function renderRecentAlerts(alerts) {
    const el = document.getElementById('dash-recent-alerts');
    if (!alerts.length) { el.innerHTML = '<div class="empty-state">No alerts recorded yet.</div>'; return; }
    const rows = alerts.map((a, i) => `
      <tr>
        <td>${fmtTs(a.timestamp)}</td>
        <td class="mono truncate">${esc(a.container_id || '—')}</td>
        <td>${esc(a.rule || '—')}</td>
        <td>${badge(a.priority)}</td>
      </tr>`).join('');
    el.innerHTML = `
      <table class="data-table">
        <thead><tr><th>Time</th><th>Container</th><th>Rule</th><th>Priority</th></tr></thead>
        <tbody>${rows}</tbody>
      </table>`;
  }

  function renderPriorityChart(byPriority) {
    const el = document.getElementById('dash-priority-chart');
    const entries = PRIO_ORDER
      .map(p => [p, byPriority[p] || 0])
      .filter(([, v]) => v > 0);

    if (!entries.length) { el.innerHTML = '<div class="empty-state">No alerts yet.</div>'; return; }

    const max = Math.max(...entries.map(([, v]) => v));
    const rows = entries.map(([p, v]) => `
      <div class="prio-row">
        <span class="prio-label">${p}</span>
        <div class="prio-bar-wrap">
          <div class="prio-bar" style="width:${Math.round(v/max*100)}%;background:${PRIO_COLORS[p]||'var(--text-2)'}"></div>
        </div>
        <span class="prio-count">${v}</span>
      </div>`).join('');
    el.innerHTML = `<div class="prio-chart">${rows}</div>`;
  }

  function renderRecentEvidence(pkgs) {
    const el = document.getElementById('dash-recent-evidence');
    if (!pkgs.length) { el.innerHTML = '<div class="empty-state">No evidence packages captured yet.</div>'; return; }
    const rows = pkgs.map(p => `
      <tr onclick="App.navigate('evidence')">
        <td class="mono truncate">${esc(short(p.container_id))}</td>
        <td>${fmtTs(p.captured_at)}</td>
        <td>${esc(p.trigger_rule || '—')}</td>
        <td>${badge(p.trigger_priority)}</td>
        <td>${p.artifact_count ?? '—'} files</td>
      </tr>`).join('');
    el.innerHTML = `
      <table class="data-table">
        <thead><tr><th>Container</th><th>Captured</th><th>Trigger Rule</th><th>Priority</th><th>Files</th></tr></thead>
        <tbody>${rows}</tbody>
      </table>`;
  }

  return { load };
})();

// ── Shared helpers (used by multiple modules) ───────────────────────────────
function esc(s) {
  return String(s ?? '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}

function short(id) {
  return id ? id.slice(0, 12) : '—';
}

function badge(priority) {
  const p = (priority || 'unknown').toLowerCase();
  return `<span class="badge badge-${p}">${p}</span>`;
}

const Toast = {
  show(msg, type = '') {
    const c = document.getElementById('toast-container');
    const t = document.createElement('div');
    t.className = `toast toast-${type}`;
    t.textContent = msg;
    c.appendChild(t);
    setTimeout(() => t.remove(), 4000);
  },
  error:   (msg) => Toast.show(msg, 'error'),
  success: (msg) => Toast.show(msg, 'success'),
};
