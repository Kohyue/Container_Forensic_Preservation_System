// Alerts section
const Alerts = (() => {
  let currentPage = 0;
  const PAGE_SIZE = 50;
  let totalAlerts = 0;
  let selectedRow = null;
  let currentAlerts = [];   // all alerts on current page

  // ── Read-state helpers (persisted in localStorage) ──────────────────────
  const READ_KEY = 'fp_read_alerts';

  function getReadSet() {
    try { return new Set(JSON.parse(localStorage.getItem(READ_KEY) || '[]')); }
    catch { return new Set(); }
  }

  function saveReadSet(set) {
    localStorage.setItem(READ_KEY, JSON.stringify([...set]));
  }

  // Use RFC3339 timestamp as the unique alert identifier.
  function alertKey(a) {
    return String(a.timestamp || '');
  }

  function markOneRead(key) {
    const s = getReadSet();
    s.add(key);
    saveReadSet(s);
    // Re-style the row without a full reload
    document.querySelectorAll('[data-alert-key]').forEach(tr => {
      if (tr.dataset.alertKey === key) tr.classList.add('row-read');
    });
    updateUnreadDot(key, true);
    updateNavBadges();
  }

  function markAllCurrentRead() {
    const s = getReadSet();
    currentAlerts.forEach(a => s.add(alertKey(a)));
    saveReadSet(s);
    renderRows(currentAlerts);   // re-render with read styles applied
    Toast.success('All visible alerts marked as read');
    // Sync dataset.total to the read set size so the badge calculation
    // resolves to zero even if the dashboard auto-refresh updated the total
    // in the background while the user was on this page.
    const ab = document.getElementById('nav-alerts-count');
    if (ab) ab.dataset.total = s.size;
    updateNavBadges();
  }

  function updateUnreadDot(key, isRead) {
    document.querySelectorAll(`[data-alert-key="${CSS.escape(key)}"] .unread-dot`).forEach(dot => {
      dot.style.display = isRead ? 'none' : '';
    });
  }

  // ── Format helpers ───────────────────────────────────────────────────────
  function fmtTs(ts) {
    if (!ts) return '—';
    return new Date(ts).toLocaleString();
  }

  // ── Load ─────────────────────────────────────────────────────────────────
  async function load(page = 0) {
    currentPage = page;
    const search   = document.getElementById('alert-search').value.trim();
    const priority = document.getElementById('alert-priority-filter').value;

    const params = { limit: PAGE_SIZE, offset: page * PAGE_SIZE };
    if (search)   params.search   = search;
    if (priority) params.priority = priority;

    const tbody = document.getElementById('alerts-tbody');
    tbody.innerHTML = '<tr><td colspan="6" class="empty-state"><span class="spinner"></span></td></tr>';

    try {
      const data = await API.alerts(params);
      totalAlerts   = data.total;
      currentAlerts = data.alerts || [];

      document.getElementById('alerts-count-label').textContent =
        `${data.total} alert${data.total !== 1 ? 's' : ''}`;

      renderRows(currentAlerts);
      renderPagination();
    } catch (e) {
      tbody.innerHTML = `<tr><td colspan="6" class="empty-state">Error: ${esc(e.message)}</td></tr>`;
      Toast.error('Failed to load alerts: ' + e.message);
    }
  }

  // ── Render ────────────────────────────────────────────────────────────────
  function renderRows(alerts) {
    const tbody = document.getElementById('alerts-tbody');
    if (!alerts.length) {
      tbody.innerHTML = '<tr><td colspan="6" class="empty-state">No alerts match your filters.</td></tr>';
      return;
    }
    const readSet = getReadSet();
    const offset  = currentPage * PAGE_SIZE;

    tbody.innerHTML = alerts.map((a, i) => {
      const key    = alertKey(a);
      const isRead = readSet.has(key);
      return `
        <tr data-idx="${i}" data-alert-key="${esc(key)}"
            class="${isRead ? 'row-read' : ''}"
            onclick="Alerts.openDetail(${i})">
          <td style="width:8px;padding-right:0">
            <span class="unread-dot" style="${isRead ? 'display:none' : ''}"></span>
          </td>
          <td class="mono" style="color:var(--text-3)">${offset + i + 1}</td>
          <td style="white-space:nowrap">${fmtTs(a.timestamp)}</td>
          <td class="mono truncate" title="${esc(a.container_id || '')}">${esc(short(a.container_id))}</td>
          <td>${esc(a.rule || '—')}</td>
          <td>${badge(a.priority)}</td>
        </tr>`;
    }).join('');

    tbody._data = alerts;
  }

  function renderPagination() {
    const el = document.getElementById('alerts-pagination');
    const totalPages = Math.ceil(totalAlerts / PAGE_SIZE);
    if (totalPages <= 1) { el.innerHTML = ''; return; }

    let html = '';
    if (currentPage > 0)
      html += `<button class="btn btn-secondary" onclick="Alerts.load(${currentPage - 1})">← Prev</button>`;

    const start = Math.max(0, currentPage - 2);
    const end   = Math.min(totalPages - 1, currentPage + 2);
    for (let p = start; p <= end; p++) {
      html += `<button class="btn ${p === currentPage ? 'active' : 'btn-secondary'}" onclick="Alerts.load(${p})">${p + 1}</button>`;
    }
    if (currentPage < totalPages - 1)
      html += `<button class="btn btn-secondary" onclick="Alerts.load(${currentPage + 1})">Next →</button>`;

    el.innerHTML = html;
  }

  // ── Detail panel ──────────────────────────────────────────────────────────
  function openDetail(idx) {
    const tbody  = document.getElementById('alerts-tbody');
    const alerts = tbody._data || [];
    const a      = alerts[idx];
    if (!a) return;

    // Highlight row
    if (selectedRow) selectedRow.classList.remove('selected');
    selectedRow = tbody.querySelector(`tr[data-idx="${idx}"]`);
    if (selectedRow) selectedRow.classList.add('selected');

    const key    = alertKey(a);
    const panel  = document.getElementById('alert-detail-panel');
    const body   = document.getElementById('alert-detail-body');
    const title  = document.getElementById('alert-detail-title');

    // Update the panel title to include a mark-as-read button
    const isRead = getReadSet().has(key);
    title.innerHTML = `
      Alert Detail
      <button class="btn btn-sm ${isRead ? 'btn-secondary' : 'btn-success'}"
              id="mark-read-btn"
              onclick="Alerts.markReadFromPanel('${esc(key)}')"
              style="margin-left:12px;font-size:11px;padding:3px 10px">
        ${isRead ? '✓ Read' : 'Mark as Read'}
      </button>`;

    const details = Object.entries(a.details || {}).map(([k, v]) =>
      `<div class="detail-field">
        <div class="detail-field-label">${esc(k)}</div>
        <div class="detail-field-value mono">${esc(v)}</div>
      </div>`).join('');

    body.innerHTML = `
      <div class="detail-field">
        <div class="detail-field-label">Timestamp</div>
        <div class="detail-field-value">${new Date(a.timestamp).toLocaleString()}</div>
      </div>
      <div class="detail-field">
        <div class="detail-field-label">Priority</div>
        <div class="detail-field-value">${badge(a.priority)}</div>
      </div>
      <div class="detail-field">
        <div class="detail-field-label">Rule</div>
        <div class="detail-field-value">${esc(a.rule || '—')}</div>
      </div>
      <div class="detail-field">
        <div class="detail-field-label">Container ID</div>
        <div class="detail-field-value mono">${esc(a.container_id || '—')}</div>
      </div>
      <hr class="detail-divider">
      ${details}
      ${a.error ? `<div class="detail-field">
        <div class="detail-field-label" style="color:var(--red)">Error</div>
        <div class="detail-field-value mono" style="color:var(--red)">${esc(a.error)}</div>
      </div>` : ''}`;

    panel.classList.remove('hidden');
  }

  function markReadFromPanel(key) {
    markOneRead(key);
    // Update the button in the panel
    const btn = document.getElementById('mark-read-btn');
    if (btn) {
      btn.textContent = '✓ Read';
      btn.className   = 'btn btn-sm btn-secondary';
      btn.style.fontSize   = '11px';
      btn.style.padding    = '3px 10px';
    }
  }

  function closeDetail() {
    document.getElementById('alert-detail-panel').classList.add('hidden');
    // Restore panel title to plain text
    const title = document.getElementById('alert-detail-title');
    if (title) title.textContent = 'Alert Detail';
    if (selectedRow) { selectedRow.classList.remove('selected'); selectedRow = null; }
  }

  // ── DOMContentLoaded wiring ───────────────────────────────────────────────
  document.addEventListener('DOMContentLoaded', () => {
    const si = document.getElementById('alert-search');
    if (si) si.addEventListener('keydown', e => { if (e.key === 'Enter') load(0); });

    const markAllBtn = document.getElementById('alerts-mark-all-read');
    if (markAllBtn) markAllBtn.addEventListener('click', markAllCurrentRead);
  });

  return { load, openDetail, markReadFromPanel, closeDetail };
})();
