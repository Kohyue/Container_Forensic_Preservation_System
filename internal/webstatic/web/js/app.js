// Main app — router, navigation, auto-refresh
const App = (() => {
  const SECTIONS = ['dashboard', 'alerts', 'evidence', 'settings'];
  const TITLES = {
    dashboard: 'Dashboard',
    alerts:    'Alerts',
    evidence:  'Evidence',
    settings:  'Settings',
  };
  const LOADERS = {
    dashboard: () => Dashboard.load(),
    alerts:    () => Alerts.load(0),
    evidence:  () => Evidence.load(0),
    settings:  () => Settings.load(),
  };

  let currentSection = 'dashboard';
  let refreshTimer   = null;
  const REFRESH_MS   = 30_000; // auto-refresh every 30 s

  function navigate(section) {
    if (!SECTIONS.includes(section)) section = 'dashboard';
    currentSection = section;

    // Update hash without triggering hashchange again
    history.replaceState(null, '', '#' + section);

    // Show/hide sections
    SECTIONS.forEach(s => {
      document.getElementById('section-' + s)?.classList.toggle('hidden', s !== section);
    });

    // Update nav active state
    document.querySelectorAll('.nav-item').forEach(el => {
      el.classList.toggle('active', el.dataset.section === section);
    });

    // Update page title
    document.getElementById('page-title').textContent = TITLES[section] || section;

    // Close any open detail panels
    document.querySelectorAll('.detail-panel').forEach(p => p.classList.add('hidden'));

    // Start/stop real-time metrics polling
    if (section === 'dashboard') {
      Metrics.start();
    } else {
      Metrics.stop();
    }

    // Load section data
    LOADERS[section]?.();

    // Restart auto-refresh
    scheduleRefresh();
  }

  function refresh() {
    const btn = document.getElementById('refresh-btn');
    btn?.classList.add('spinning');
    Promise.resolve(LOADERS[currentSection]?.())
      .finally(() => {
        btn?.classList.remove('spinning');
        updateLastUpdated();
      });
  }

  function scheduleRefresh() {
    clearTimeout(refreshTimer);
    if (currentSection === 'dashboard') {
      refreshTimer = setTimeout(() => { Dashboard.load(); scheduleRefresh(); }, REFRESH_MS);
    }
  }

  function updateLastUpdated() {
    const el = document.getElementById('last-updated');
    if (el) el.textContent = 'Updated ' + new Date().toLocaleTimeString();
  }

  async function init() {
    // Sidebar toggle for mobile
    document.getElementById('sidebar-toggle')?.addEventListener('click', () => {
      document.getElementById('sidebar')?.classList.toggle('open');
    });

    // Close sidebar when clicking main on mobile
    document.querySelector('.layout')?.addEventListener('click', () => {
      if (window.innerWidth < 768) {
        document.getElementById('sidebar')?.classList.remove('open');
      }
    });

    // Close detail panels on Escape
    document.addEventListener('keydown', e => {
      if (e.key === 'Escape') {
        document.querySelectorAll('.detail-panel').forEach(p => p.classList.add('hidden'));
      }
    });

    // Hash-based routing
    window.addEventListener('hashchange', () => {
      const hash = location.hash.replace('#', '') || 'dashboard';
      navigate(hash);
    });

    // Probe the API to set status indicator
    try {
      await API.status();
      document.getElementById('status-dot').className  = 'status-dot active';
      document.getElementById('status-label').textContent = 'Active';
    } catch (_) {
      document.getElementById('status-dot').className  = 'status-dot error';
      document.getElementById('status-label').textContent = 'Offline';
    }

    // Initial navigation
    const initial = location.hash.replace('#', '') || 'dashboard';
    navigate(initial);
    updateLastUpdated();
  }

  // Add spinning animation for refresh button
  const style = document.createElement('style');
  style.textContent = `.spinning svg { animation: spin .6s linear infinite; }`;
  document.head.appendChild(style);

  document.addEventListener('DOMContentLoaded', init);

  return { navigate, refresh };
})();
