// Settings section
const Settings = (() => {
  async function load() {
    try {
      const [cfg, rules, status] = await Promise.all([API.config(), API.rules(), API.status()]);
      renderConfig(cfg);
      renderRules(rules.rules || []);
      renderInfo(status);
    } catch (e) {
      Toast.error('Failed to load settings: ' + e.message);
    }
  }

  function renderConfig(cfg) {
    const el = document.getElementById('settings-config');
    // Pretty-print the config as YAML-like text from the JSON
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

  function renderInfo(s) {
    const el = document.getElementById('settings-info');
    const items = [
      { key: 'Version',      val: s.version       || '—' },
      { key: 'Node',         val: s.node_name      || '—' },
      { key: 'Webhook',      val: s.webhook_addr   || '—' },
      { key: 'Web UI',       val: s.web_addr       || '—' },
      { key: 'Started At',   val: s.started_at ? new Date(s.started_at).toLocaleString() : '—' },
      { key: 'Uptime',       val: fmtUptime(s.uptime_seconds) },
      { key: 'Status',       val: s.status         || '—' },
    ];
    el.innerHTML = `<div class="info-grid">${items.map(i =>
      `<div class="info-item">
        <span class="info-key">${esc(i.key)}</span>
        <span class="info-val">${esc(String(i.val))}</span>
      </div>`).join('')}</div>`;
  }

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

  return { load };
})();
