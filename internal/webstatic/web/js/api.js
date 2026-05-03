// API client — all fetch calls to /api/v1/*
const API = (() => {
  const BASE = '/api/v1';

  async function get(path) {
    const res = await fetch(BASE + path);
    if (!res.ok) throw new Error(`API error ${res.status}: ${path}`);
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
    config: () => get('/config'),
    rules:  () => get('/rules'),
  };
})();
