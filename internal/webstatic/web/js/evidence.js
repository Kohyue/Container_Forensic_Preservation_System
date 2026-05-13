// Evidence section
const Evidence = (() => {
  let currentPage   = 0;
  const PAGE_SIZE   = 24;
  let totalPackages = 0;
  let allPackages   = [];
  let openPkgId     = null;   // ID of the currently open detail panel

  // ── Read-state helpers (persisted in localStorage) ───────────────────────
  const READ_KEY = 'fp_read_evidence';

  function getReadSet() {
    try { return new Set(JSON.parse(localStorage.getItem(READ_KEY) || '[]')); }
    catch { return new Set(); }
  }

  function saveReadSet(set) {
    localStorage.setItem(READ_KEY, JSON.stringify([...set]));
  }

  function markOneRead(id) {
    const s = getReadSet();
    s.add(id);
    saveReadSet(s);
    // Update card appearance without full re-render
    const card = document.querySelector(`.evidence-card[data-pkg-id="${CSS.escape(id)}"]`);
    if (card) {
      card.classList.add('ev-card--read');
      const dot = card.querySelector('.unread-dot');
      if (dot) dot.style.display = 'none';
    }
    // Update panel button
    const btn = document.getElementById('ev-mark-read-btn');
    if (btn) {
      btn.textContent = '✓ Read';
      btn.className   = 'btn btn-sm btn-secondary';
      btn.style.fontSize = '11px';
      btn.style.padding  = '3px 10px';
    }
    updateNavBadges();
  }

  function markAllCurrentRead() {
    const s = getReadSet();
    allPackages.forEach(p => s.add(p.id));
    saveReadSet(s);
    renderGrid(allPackages);
    Toast.success('All visible evidence packages marked as read');
    updateNavBadges();
  }

  // ── Duration formatter ────────────────────────────────────────────────────
  function fmtDuration(ms) {
    if (!ms && ms !== 0) return null;
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(1)}s`;
  }

  function fmtTs(ts) {
    if (!ts) return '—';
    return new Date(ts).toLocaleString();
  }

  // ── Load ──────────────────────────────────────────────────────────────────
  async function load(page = 0) {
    currentPage = page;
    const grid = document.getElementById('evidence-grid');
    grid.innerHTML = '<div class="empty-state"><span class="spinner"></span></div>';

    try {
      const data = await API.listEvidence({ limit: PAGE_SIZE, offset: page * PAGE_SIZE });
      totalPackages = data.total;
      allPackages   = data.packages || [];

      document.getElementById('evidence-count-label').textContent =
        `${data.total} package${data.total !== 1 ? 's' : ''}`;

      renderGrid(allPackages);
      renderPagination();
    } catch (e) {
      grid.innerHTML = `<div class="empty-state">Error: ${esc(e.message)}</div>`;
      Toast.error('Failed to load evidence: ' + e.message);
    }
  }

  // ── Render grid ───────────────────────────────────────────────────────────
  function renderGrid(pkgs) {
    const grid    = document.getElementById('evidence-grid');
    const readSet = getReadSet();

    if (!pkgs.length) {
      grid.innerHTML = '<div class="empty-state" style="grid-column:1/-1">No evidence packages captured yet.</div>';
      return;
    }

    const q = document.getElementById('evidence-search')?.value.toLowerCase() || '';
    const filtered = q
      ? pkgs.filter(p =>
          (p.container_id   || '').toLowerCase().includes(q) ||
          (p.trigger_rule   || '').toLowerCase().includes(q) ||
          (p.container_name || '').toLowerCase().includes(q))
      : pkgs;

    if (!filtered.length) {
      grid.innerHTML = '<div class="empty-state" style="grid-column:1/-1">No packages match your search.</div>';
      return;
    }

    grid.innerHTML = filtered.map(p => {
      const isRead = readSet.has(p.id);
      const dur    = fmtDuration(p.capture_duration_ms);
      return `
        <div class="evidence-card ${isRead ? 'ev-card--read' : ''}"
             data-pkg-id="${esc(p.id)}"
             onclick="Evidence.openDetail('${esc(p.id)}')">
          <div class="ev-header">
            <div style="display:flex;align-items:center;gap:6px">
              <span class="unread-dot" style="${isRead ? 'display:none' : ''}"></span>
              <span class="ev-id">${esc(short(p.container_id))}</span>
            </div>
            ${badge(p.trigger_priority)}
          </div>
          <div class="ev-body">
            <div class="ev-row">
              <span class="ev-key">Time</span>
              <span class="ev-val">${fmtTs(p.captured_at)}</span>
            </div>
            ${p.container_name ? `<div class="ev-row">
              <span class="ev-key">Name</span>
              <span class="ev-val">${esc(p.container_name)}</span>
            </div>` : ''}
            <div class="ev-row">
              <span class="ev-key">Rule</span>
              <span class="ev-val ev-rule">${esc(p.trigger_rule || '—')}</span>
            </div>
          </div>
          <div class="ev-footer">
            <span class="ev-files">${p.artifact_count ?? 0} artifact${p.artifact_count !== 1 ? 's' : ''}</span>
            ${dur ? `<span class="duration-badge">${esc(dur)}</span>` : ''}
          </div>
        </div>`;
    }).join('');
  }

  // ── Pagination ────────────────────────────────────────────────────────────
  function renderPagination() {
    const el = document.getElementById('evidence-pagination');
    const totalPages = Math.ceil(totalPackages / PAGE_SIZE);
    if (totalPages <= 1) { el.innerHTML = ''; return; }

    let html = '';
    if (currentPage > 0)
      html += `<button class="btn btn-secondary" onclick="Evidence.load(${currentPage - 1})">← Prev</button>`;
    for (let p = Math.max(0, currentPage - 2); p <= Math.min(totalPages - 1, currentPage + 2); p++) {
      html += `<button class="btn ${p === currentPage ? 'active' : 'btn-secondary'}" onclick="Evidence.load(${p})">${p + 1}</button>`;
    }
    if (currentPage < totalPages - 1)
      html += `<button class="btn btn-secondary" onclick="Evidence.load(${currentPage + 1})">Next →</button>`;
    el.innerHTML = html;
  }

  // ── Detail panel ──────────────────────────────────────────────────────────
  async function openDetail(id) {
    openPkgId = id;
    const panel = document.getElementById('evidence-detail-panel');
    const body  = document.getElementById('evidence-detail-body');
    const title = document.getElementById('evidence-detail-title');

    title.textContent = 'Loading…';
    body.innerHTML    = '<div class="empty-state"><span class="spinner"></span></div>';
    panel.classList.remove('hidden');

    try {
      const pkg    = await API.getEvidence(id);
      const isRead = getReadSet().has(id);

      title.innerHTML = `
        ${esc(short(pkg.container_id) || id)}
        <button class="btn btn-sm ${isRead ? 'btn-secondary' : 'btn-success'}"
                id="ev-mark-read-btn"
                onclick="Evidence.markReadFromPanel('${esc(id)}')"
                style="margin-left:12px;font-size:11px;padding:3px 10px">
          ${isRead ? '✓ Read' : 'Mark as Read'}
        </button>`;

      renderDetail(pkg);
    } catch (e) {
      body.innerHTML = `<div class="empty-state">Error: ${esc(e.message)}</div>`;
    }
  }

  function markReadFromPanel(id) {
    markOneRead(id);
  }

  function renderDetail(pkg) {
    const body = document.getElementById('evidence-detail-body');
    const m    = pkg.manifest || {};
    const dur  = fmtDuration(pkg.capture_duration_ms);

    const fileList = (pkg.files || []).map(f => `
      <button class="file-btn" onclick="Evidence.viewFile('${esc(pkg.id)}', '${esc(f)}', this)">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
          <polyline points="14,2 14,8 20,8"/>
        </svg>
        ${esc(f)}
      </button>`).join('');

    const artifacts = (m.artifacts || []).map(a => `
      <div style="margin-bottom:10px;padding:10px;background:var(--bg-0);border-radius:var(--radius);border:1px solid var(--border-sub)">
        <div style="font-size:13px;font-weight:600;margin-bottom:4px">${esc(a.filename)}</div>
        <div style="font-family:var(--font-mono);font-size:11px;color:var(--text-2);word-break:break-all">SHA-256: ${esc(a.sha256)}</div>
        <div style="font-size:11px;color:var(--text-3);margin-top:2px">${a.size_bytes ?? 0} bytes · ${new Date(a.captured_at).toLocaleString()}</div>
      </div>`).join('');

    body.innerHTML = `
      <div class="detail-field">
        <div class="detail-field-label">Container ID</div>
        <div class="detail-field-value mono">${esc(pkg.container_id || '—')}</div>
      </div>
      ${pkg.container_name ? `<div class="detail-field">
        <div class="detail-field-label">Container Name</div>
        <div class="detail-field-value">${esc(pkg.container_name)}</div>
      </div>` : ''}
      <div class="detail-field">
        <div class="detail-field-label">Captured At</div>
        <div class="detail-field-value">${new Date(pkg.captured_at).toLocaleString()}</div>
      </div>
      <div class="detail-field">
        <div class="detail-field-label">Trigger Rule</div>
        <div class="detail-field-value">${esc(pkg.trigger_rule || '—')}</div>
      </div>
      <div class="detail-field">
        <div class="detail-field-label">Priority</div>
        <div class="detail-field-value">${badge(pkg.trigger_priority)}</div>
      </div>
      ${dur ? `<div class="detail-field">
        <div class="detail-field-label">Capture Duration</div>
        <div class="detail-field-value" style="color:var(--blue);font-weight:600">${esc(dur)}</div>
      </div>` : ''}
      ${m.manifest_sha256 ? `<div class="detail-field">
        <div class="detail-field-label">Manifest SHA-256</div>
        <div class="detail-field-value mono" style="font-size:11px;color:var(--text-2)">${esc(m.manifest_sha256)}</div>
      </div>` : ''}
      <hr class="detail-divider">
      <div style="font-size:12px;font-weight:600;color:var(--text-2);margin-bottom:10px;text-transform:uppercase;letter-spacing:.4px">Artifact Hashes</div>
      ${artifacts || '<div class="empty-state">No manifest data.</div>'}
      <hr class="detail-divider">
      <div style="font-size:12px;font-weight:600;color:var(--text-2);margin-bottom:10px;text-transform:uppercase;letter-spacing:.4px">View Files</div>
      <div class="file-list">${fileList}</div>
      <div id="file-viewer-wrap"></div>`;
  }

  // ── File viewer ───────────────────────────────────────────────────────────
  async function viewFile(pkgId, filename, btn) {
    document.querySelectorAll('.file-btn').forEach(b => b.classList.remove('active'));
    btn.classList.add('active');

    const wrap = document.getElementById('file-viewer-wrap');
    wrap.innerHTML = '<div class="empty-state"><span class="spinner"></span></div>';

    try {
      const content = await API.getFile(pkgId, filename);
      const isJSON  = filename.endsWith('.json');
      let display   = content;
      if (isJSON) {
        try { display = JSON.stringify(JSON.parse(content), null, 2); } catch (_) {}
      }
      wrap.innerHTML = `
        <div class="file-content-wrap">
          <div class="file-content-header">
            <span>${esc(filename)}</span>
            <span style="color:var(--text-3)">${content.length.toLocaleString()} bytes</span>
          </div>
          <pre class="file-content">${esc(display)}</pre>
        </div>`;
    } catch (e) {
      wrap.innerHTML = `<div class="empty-state">Error loading file: ${esc(e.message)}</div>`;
    }
  }

  function closeDetail() {
    document.getElementById('evidence-detail-panel').classList.add('hidden');
    openPkgId = null;
    const title = document.getElementById('evidence-detail-title');
    if (title) title.textContent = 'Evidence Package';
  }

  // ── DOMContentLoaded wiring ───────────────────────────────────────────────
  document.addEventListener('DOMContentLoaded', () => {
    const si = document.getElementById('evidence-search');
    if (si) si.addEventListener('keydown', e => {
      if (e.key === 'Enter') renderGrid(allPackages);
    });

    const markAllBtn = document.getElementById('evidence-mark-all-read');
    if (markAllBtn) markAllBtn.addEventListener('click', markAllCurrentRead);
  });

  return { load, openDetail, viewFile, closeDetail, markReadFromPanel };
})();
