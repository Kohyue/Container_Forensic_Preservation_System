// Real-time agent metrics panel (dashboard right column)
const Metrics = (() => {
  let _timer  = null;
  const INTERVAL = 3000;

  // ── Start / stop polling ───────────────────────────────────────────────────
  function start() {
    load();
    _timer = setInterval(load, INTERVAL);
  }

  function stop() {
    clearInterval(_timer);
    _timer = null;
  }

  // ── Fetch and render ───────────────────────────────────────────────────────
  async function load() {
    try {
      const m = await API.metrics();
      render(m);
    } catch (_) { /* fail silently — secondary UI */ }
  }

  function render(m) {
    // Memory bar
    const pct = m.sys_mb > 0 ? Math.min(100, (m.alloc_mb / m.sys_mb) * 100) : 0;
    setBar('metrics-mem-bar', pct);

    // Colour the bar: green < 50%, yellow < 80%, red >= 80%
    const bar = document.getElementById('metrics-mem-bar');
    if (bar) {
      bar.style.background =
        pct < 50 ? 'var(--green)' :
        pct < 80 ? 'var(--yellow)' : 'var(--red)';
    }

    setValue('metrics-alloc',      m.alloc_mb.toFixed(1)     + ' MB');
    setValue('metrics-sys',        m.sys_mb.toFixed(1)       + ' MB');
    setValue('metrics-heap-inuse', m.heap_inuse_mb.toFixed(1)+ ' MB');
    setValue('metrics-goroutines', m.goroutines);
    setValue('metrics-gc',         m.gc_runs);

    if (m.binary_size_kb > 0) {
      setValue('metrics-binary',
        m.binary_size_kb >= 1024
          ? (m.binary_size_kb / 1024).toFixed(1) + ' MB'
          : m.binary_size_kb + ' KB');
    }

    // Pulse the LIVE dot
    const dot = document.getElementById('metrics-live-dot');
    if (dot) {
      dot.classList.remove('metrics-pulse');
      void dot.offsetWidth;
      dot.classList.add('metrics-pulse');
    }

    // Timestamp
    const ts = document.getElementById('metrics-updated');
    if (ts) ts.textContent = 'Updated ' + new Date().toLocaleTimeString();
  }

  // ── DOM helpers ────────────────────────────────────────────────────────────
  function setValue(id, val) {
    const el = document.getElementById(id);
    if (!el) return;
    const s = String(val);
    if (el.textContent === s) return;
    el.textContent = s;
    flash(el);
  }

  function setBar(id, pct) {
    const el = document.getElementById(id);
    if (el) el.style.width = pct.toFixed(1) + '%';
  }

  function flash(el) {
    el.classList.remove('metrics-flash');
    void el.offsetWidth;
    el.classList.add('metrics-flash');
  }

  return { start, stop, load };
})();
