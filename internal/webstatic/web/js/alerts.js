// Alerts section
const Alerts = (() => {
  let currentPage = 0;
  const PAGE_SIZE = 50;
  let totalAlerts = 0;
  let selectedRow = null;

  function fmtTs(ts) {
    if (!ts) return '—';
    return new Date(ts).toLocaleString();
  }

  async function load(page = 0) {
    currentPage = page;
    const search   = document.getElementById('alert-search').value.trim();
    const priority = document.getElementById('alert-priority-filter').value;

    const params = { limit: PAGE_SIZE, offset: page * PAGE_SIZE };
    if (search)   params.search   = search;
    if (priority) params.priority = priority;

    const tbody = document.getElementById('alerts-tbody');
    tbody.innerHTML = '<tr><td colspan="5" class="empty-state"><span class="spinner"></span></td></tr>';

    try {
      const data = await API.alerts(params);
      totalAlerts = data.total;
      document.getElementById('alerts-count-label').textContent =
        `${data.total} alert${data.total !== 1 ? 's' : ''}`;
      renderRows(data.alerts || []);
      renderPagination();
    } catch (e) {
      tbody.innerHTML = `<tr><td colspan="5" class="empty-state">Error: ${esc(e.message)}</td></tr>`;
      Toast.error('Failed to load alerts: ' + e.message);
    }
  }

  function renderRows(alerts) {
    const tbody = document.getElementById('alerts-tbody');
    if (!alerts.length) {
      tbody.innerHTML = '<tr><td colspan="5" class="empty-state">No alerts match your filters.</td></tr>';
      return;
    }
    const offset = currentPage * PAGE_SIZE;
    tbody.innerHTML = alerts.map((a, i) => `
      <tr data-idx="${i}" onclick="Alerts.openDetail(${i})">
        <td class="mono" style="color:var(--text-3)">${offset + i + 1}</td>
        <td style="white-space:nowrap">${fmtTs(a.timestamp)}</td>
        <td class="mono truncate" title="${esc(a.container_id || '')}">${esc(short(a.container_id))}</td>
        <td>${esc(a.rule || '—')}</td>
        <td>${badge(a.priority)}</td>
      </tr>`).join('');

    // Store data for detail panel
    tbody._data = alerts;
  }

  function renderPagination() {
    const el = document.getElementById('alerts-pagination');
    const totalPages = Math.ceil(totalAlerts / PAGE_SIZE);
    if (totalPages <= 1) { el.innerHTML = ''; return; }

    let html = '';
    if (currentPage > 0) {
      html += `<button class="btn btn-secondary" onclick="Alerts.load(${currentPage - 1})">← Prev</button>`;
    }
    const start = Math.max(0, currentPage - 2);
    const end   = Math.min(totalPages - 1, currentPage + 2);
    for (let p = start; p <= end; p++) {
      html += `<button class="btn ${p === currentPage ? 'active' : 'btn-secondary'}" onclick="Alerts.load(${p})">${p + 1}</button>`;
    }
    if (currentPage < totalPages - 1) {
      html += `<button class="btn btn-secondary" onclick="Alerts.load(${currentPage + 1})">Next →</button>`;
    }
    el.innerHTML = html;
  }

  function openDetail(idx) {
    const tbody = document.getElementById('alerts-tbody');
    const alerts = tbody._data || [];
    const a = alerts[idx];
    if (!a) return;

    // Highlight row
    if (selectedRow) selectedRow.classList.remove('selected');
    selectedRow = tbody.querySelector(`tr[data-idx="${idx}"]`);
    if (selectedRow) selectedRow.classList.add('selected');

    const panel = document.getElementById('alert-detail-panel');
    const body  = document.getElementById('alert-detail-body');

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

  function closeDetail() {
    document.getElementById('alert-detail-panel').classList.add('hidden');
    if (selectedRow) { selectedRow.classList.remove('selected'); selectedRow = null; }
  }

  // Enter key triggers filter
  document.addEventListener('DOMContentLoaded', () => {
    const si = document.getElementById('alert-search');
    if (si) si.addEventListener('keydown', e => { if (e.key === 'Enter') load(0); });
  });

  return { load, openDetail, closeDetail };
})();
