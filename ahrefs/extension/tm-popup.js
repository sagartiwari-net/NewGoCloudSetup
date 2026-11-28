(() => {
  const dot = document.getElementById('dot');
  const status = document.getElementById('status');
  const userEl = document.getElementById('user');
  const portalEl = document.getElementById('portal');
  const limitsEl = document.getElementById('limits');
  const cacheEl = document.getElementById('cache');
  const diag = document.getElementById('diag');
  const btn = document.getElementById('refresh');
  const testBtn = document.getElementById('test');
  const freshBtn = document.getElementById('testFresh');
  const clearBtn = document.getElementById('clearCache');

  function renderCache(c) {
    if (!c || c.error) {
      cacheEl.textContent = 'Cache: —';
      return;
    }
    const kinds = c.byKind || {};
    cacheEl.textContent =
      'Cache: ' + (c.total || 0) + ' entities · TTL ' + (c.ttlHours || 72) + 'h' +
      (kinds.domain ? ' · domains ' + kinds.domain : '') +
      (kinds.serp ? ' · serp ' + kinds.serp : '');
  }

  function render(payload) {
    const authed = !!(payload && payload.tm_bar_authed);
    dot.className = 'dot ' + (authed ? 'ok' : 'bad');
    status.textContent = authed ? 'access active' : (payload && payload.tm_bar_error) || 'not signed in';
    status.style.color = authed ? '#22c55e' : '#ef4444';

    const u = (payload && payload.tm_bar_user) || {};
    userEl.textContent = authed ? ('User: ' + (u.login || u.name || '—')) : 'User: —';
    portalEl.textContent = 'Portal: ' + ((payload && payload.tm_bar_portal) || '—');

    const me = payload && payload.me;
    if (me && me.success && me.limits) {
      const L = me.limits;
      limitsEl.textContent =
        'Credits: hourly ' + L.hourly_remaining + '/' + L.hourly_limit +
        ' · daily ' + L.daily_remaining + '/' + L.daily_limit;
    } else {
      limitsEl.textContent = 'Credits: —';
    }
    renderCache(payload && payload.cache);
  }

  function refreshStatus() {
    chrome.runtime.sendMessage({ type: 'tm:status' }, (r) => render(r || {}));
  }

  function runTest(fresh) {
    diag.textContent = fresh ? 'Fresh fetch (bypass caches)…' : 'Testing (cache-first)…';
    testBtn.disabled = true;
    freshBtn.disabled = true;
    chrome.runtime.sendMessage({ type: 'tm:test-proxy', target: 'ahrefs.com', fresh: !!fresh }, (r) => {
      testBtn.disabled = false;
      freshBtn.disabled = false;
      diag.textContent = JSON.stringify(r || { error: 'no response' }, null, 2);
      refreshStatus();
    });
  }

  btn.addEventListener('click', () => {
    status.textContent = 'refreshing…';
    status.style.color = '#eab308';
    btn.disabled = true;
    chrome.runtime.sendMessage({ type: 'tm:refresh' }, () => {
      btn.disabled = false;
      refreshStatus();
    });
  });

  testBtn.addEventListener('click', () => runTest(false));
  freshBtn.addEventListener('click', () => runTest(true));

  clearBtn.addEventListener('click', () => {
    clearBtn.disabled = true;
    chrome.runtime.sendMessage({ type: 'tm:cache-clear' }, (r) => {
      clearBtn.disabled = false;
      diag.textContent = JSON.stringify(r || { ok: true }, null, 2);
      refreshStatus();
    });
  });

  refreshStatus();
})();
