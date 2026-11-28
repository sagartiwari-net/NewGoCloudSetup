/* Compact ToolsMandi auth + update strip on the official Ahrefs popup.
 * Auth is portal-cookie driven (aMember / panel_session) — no manual Sign out.
 * Also surfaces hourly/daily limit + pool cooldown with retry countdown.
 */
(() => {
  const GENERIC_FETCH_ERR = 'Error fetching data. Please try again later.';
  const SUB_EXPIRED_MSG = 'Subscription expired. Contact to admin.';

  const bar = document.createElement('div');
  bar.id = 'tm-auth-strip';
  bar.style.cssText =
    'display:flex;align-items:center;gap:8px;flex-wrap:wrap;padding:8px 12px;background:#0f172a;color:#e2e8f0;' +
    'font:12px/1.3 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;border-bottom:1px solid #1e293b;';
  bar.innerHTML =
    '<span id="tm-dot" style="width:8px;height:8px;border-radius:50%;background:#64748b;flex:0 0 auto"></span>' +
    '<strong style="font-weight:600">ToolsMandi</strong>' +
    '<span id="tm-status" style="flex:1;min-width:120px;color:#94a3b8">checking…</span>' +
    '<span id="tm-ver" style="color:#64748b;font-size:11px"></span>' +
    '<button id="tm-refresh" type="button" style="background:#22c55e;color:#04120a;border:0;border-radius:6px;padding:4px 10px;font-weight:600;cursor:pointer">Refresh</button>' +
    '<button id="tm-update" type="button" style="background:#334155;color:#e2e8f0;border:0;border-radius:6px;padding:4px 10px;font-weight:600;cursor:pointer">Update</button>';

  const notice = document.createElement('div');
  notice.id = 'tm-limit-notice';
  notice.style.cssText =
    'display:none;padding:8px 12px;background:#7c2d12;color:#ffedd5;font:12px/1.4 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;' +
    'border-bottom:1px solid #9a3412;';

  document.body.insertBefore(notice, document.body.firstChild);
  document.body.insertBefore(bar, notice);

  const dot = document.getElementById('tm-dot');
  const status = document.getElementById('tm-status');
  const verEl = document.getElementById('tm-ver');
  const btn = document.getElementById('tm-refresh');
  const upd = document.getElementById('tm-update');

  let liveBlock = null;
  let tickTimer = null;
  const replaced = new WeakSet();

  function formatWait(sec) {
    sec = Math.max(0, Math.floor(Number(sec) || 0));
    if (sec < 60) return sec + 's';
    const min = Math.floor(sec / 60);
    if (min < 60) {
      const rem = sec % 60;
      return rem ? min + 'm ' + rem + 's' : min + 'm';
    }
    const h = Math.floor(min / 60);
    const m = min % 60;
    return m ? h + 'h ' + m + 'm' : h + 'h';
  }

  function blockMessage(block) {
    if (!block) return '';
    if (block.message && /subscription expired/i.test(block.message)) {
      return block.message;
    }
    if (block.retry_at && block.retry_at > Date.now()) {
      const left = Math.ceil((block.retry_at - Date.now()) / 1000);
      if (block.kind === 'limit' && block.window === 'daily') {
        return "Today's limit reached. Try again in " + formatWait(left) + '.';
      }
      if (block.kind === 'limit') {
        return 'Hourly limit reached. Try again in ' + formatWait(left) + '.';
      }
      if (block.kind === 'pool' && left > 0) {
        return block.message || ('Ahrefs account cooling down. Try again in ' + formatWait(left) + '.');
      }
    }
    return block.message || '';
  }

  function paintNotice() {
    const msg = blockMessage(liveBlock);
    if (!msg) {
      notice.style.display = 'none';
      notice.textContent = '';
      return;
    }
    notice.style.display = 'block';
    notice.style.background = liveBlock && liveBlock.kind === 'limit' ? '#7c2d12' : '#1e3a5f';
    notice.style.borderBottomColor = liveBlock && liveBlock.kind === 'limit' ? '#9a3412' : '#1e40af';
    notice.style.color = liveBlock && liveBlock.kind === 'limit' ? '#ffedd5' : '#dbeafe';
    notice.textContent = msg;
  }

  function rewriteGenericErrors() {
    const msg = blockMessage(liveBlock);
    if (!msg) return;
    const nodes = document.querySelectorAll('div, span, p, h1, h2, h3, h4');
    for (let i = 0; i < nodes.length; i++) {
      const el = nodes[i];
      if (replaced.has(el)) {
        el.textContent = msg;
        continue;
      }
      if (el.childElementCount > 0) continue;
      const t = (el.textContent || '').trim();
      if (t === GENERIC_FETCH_ERR || t === "You've reached your usage limit") {
        el.textContent = msg || SUB_EXPIRED_MSG;
        replaced.add(el);
      }
    }
  }

  function startTick() {
    if (tickTimer) return;
    tickTimer = setInterval(() => {
      if (liveBlock && liveBlock.retry_at && liveBlock.retry_at <= Date.now()) {
        liveBlock = null;
        paintNotice();
        clearInterval(tickTimer);
        tickTimer = null;
        return;
      }
      paintNotice();
      rewriteGenericErrors();
    }, 1000);
  }

  function paint(r) {
    const ok = !!(r && r.tm_bar_authed);
    dot.style.background = ok ? '#22c55e' : '#ef4444';
    if (ok) {
      const u = (r.tm_bar_user && (r.tm_bar_user.login || r.tm_bar_user.name)) || 'ok';
      status.textContent = u + (r.tm_bar_portal ? ' · ' + r.tm_bar_portal : '');
      status.style.color = '#86efac';
    } else {
      status.textContent = (r && r.tm_bar_error) || 'not signed in — log in to panel / aMember';
      status.style.color = '#fca5a5';
    }
    if (r && r.version) verEl.textContent = 'v' + r.version;

    liveBlock = (r && r.tm_bar_block) || null;
    if (liveBlock && liveBlock.retry_at && liveBlock.retry_at <= Date.now()) liveBlock = null;
    paintNotice();
    rewriteGenericErrors();
    if (liveBlock && liveBlock.retry_at) startTick();
  }

  function loadStatus() {
    chrome.runtime.sendMessage({ type: 'tm:status' }, (r) => paint(r || {}));
  }

  function withTimeout(fn, ms, fallback) {
    let done = false;
    const t = setTimeout(() => {
      if (done) return;
      done = true;
      fallback();
    }, ms);
    fn(() => {
      if (done) return;
      done = true;
      clearTimeout(t);
    });
  }

  // Always re-check live portal cookie when popup opens (never hang on "checking…").
  function syncThenPaint() {
    status.textContent = 'checking…';
    status.style.color = '#eab308';
    withTimeout(
      (done) => {
        chrome.runtime.sendMessage({ type: 'tm:refresh' }, () => {
          loadStatus();
          done();
        });
      },
      12000,
      () => {
        status.textContent = 'check timeout — tap Refresh';
        status.style.color = '#fca5a5';
        loadStatus();
      },
    );
  }

  btn.addEventListener('click', () => {
    status.textContent = 'refreshing…';
    status.style.color = '#eab308';
    btn.disabled = true;
    withTimeout(
      (done) => {
        chrome.runtime.sendMessage({ type: 'tm:refresh' }, () => {
          btn.disabled = false;
          loadStatus();
          done();
        });
      },
      12000,
      () => {
        btn.disabled = false;
        status.textContent = 'check timeout — tap Refresh';
        status.style.color = '#fca5a5';
        loadStatus();
      },
    );
  });

  upd.addEventListener('click', () => {
    status.textContent = 'fetching update…';
    status.style.color = '#eab308';
    upd.disabled = true;
    chrome.runtime.sendMessage({ type: 'tm:update' }, (r) => {
      upd.disabled = false;
      if (r && r.error) {
        status.textContent = r.error;
        status.style.color = '#fca5a5';
        return;
      }
      status.textContent = (r && r.opened) || 'update opened — unzip & Load unpacked';
      status.style.color = '#93c5fd';
      if (r && r.version) verEl.textContent = 'v' + r.version + (r.remote_version ? ' → ' + r.remote_version : '');
    });
  });

  chrome.storage.onChanged.addListener((changes, area) => {
    if (area !== 'local' || !changes.tm_bar_block) return;
    liveBlock = changes.tm_bar_block.newValue || null;
    paintNotice();
    rewriteGenericErrors();
    if (liveBlock && liveBlock.retry_at) startTick();
  });

  const mo = new MutationObserver(() => rewriteGenericErrors());
  mo.observe(document.documentElement, { childList: true, subtree: true, characterData: true });

  syncThenPaint();
})();
