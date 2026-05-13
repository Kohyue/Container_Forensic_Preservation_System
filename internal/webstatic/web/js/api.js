// API client — all fetch calls to /api/v1/*
const API = (() => {
  const BASE = '/api/v1';

  async function get(path) {
    const res = await fetch(BASE + path);
    if (!res.ok) throw new Error(`API error ${res.status}: ${path}`);
    return res.json();
  }

  async function post(path, data) {
    const res = await fetch(BASE + path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
    if (!res.ok) {
      const text = await res.text().catch(() => res.status);
      throw new Error(`API error ${res.status}: ${text}`);
    }
    return res.json();
  }

  return {
    status:       () => get('/status'),
    stats:        () => get('/stats'),
    alerts:       (params = {}) => get('/alerts?' + new URLSearchParams(params)),
    listEvidence: (params = {}) => get('/evidence?' + new URLSearchParams(params)),
    getEvidence:  (id) => get(`/evidence/${encodeURIComponent(id)}`),
    getFile:      (id, filename) =>
      fetch(`${BASE}/evidence/${encodeURIComponent(id)}/file/${encodeURIComponent(filename)}`)
        .then(r => { if (!r.ok) throw new Error('File not found'); return r.text(); }),
    config:  () => get('/config'),
    rules:   () => get('/rules'),
    // Runtime config updates
    updateCollector: (data) => post('/config/collector', data),
    updateSchedule:  (data) => post('/config/schedule',  data),
  };
})();
