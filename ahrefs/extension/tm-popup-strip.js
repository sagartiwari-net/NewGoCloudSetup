/* Compact ToolsMandi auth + update strip on the official Ahrefs popup.
 * Auth is portal-cookie driven (aMember / panel_session) — no manual Sign out.
 * Also surfaces hourly/daily limit + pool cooldown with retry countdown.
 *
 * IMPORTANT: never wait on the service worker to paint status. Popup reads
 * chrome.storage.local directly so "checking…" cannot stick forever if SW hangs.
 */
(() => {
  const GENERIC_FETCH_ERR = 'Error fetching data. Please try again later.';
  const SUB_EXPIRED_MSG = 'Subscription expired. Contact to admin.';
  const USAGE_LIMIT_ERR = "You've reached your usage limit";

  const bar = document.createElement('div');
  bar.id = 'tm-auth-strip';
  bar.style.cssText =
    'display:flex;align-items:center;gap:8px;flex-wrap:wrap;padding:8px 12px;background:#0f172a;color:#e2e8f0;' +
    'font:12px/1.3 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;border-bottom:1px solid #1e293b;';
  bar.innerHTML =
    '<span id="tm-dot" style="width:8px;height:8px;border-radius:50%;background:#64748b;flex:0 0 auto"></span>' +
    '<strong style="font-weight:600">ToolsMandi</strong>' +
    '<span id="tm-status" style="flex:1;min-width:120px;color:#94a3b8">…</span>' +
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
  let lastAuth = { ok: false, error: '' };
  let tickTimer = null;
  const replaced = new WeakSet();
  const manVer = (chrome.runtime.getManifest() && chrome.runtime.getManifest().version) || '';

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

  /** Map tm_bar_error codes → clear UX (cookie / login / plan). */
  function humanizeAuthError(err) {
    const e = String(err || '').trim();
    if (!e) return 'Not signed in — log in on your membership site, then Refresh.';
    const low = e.toLowerCase();
    if (low === 'not_logged_in' || low === 'logged_out') {
      return 'Not logged in — open your aMember site, sign in, then click Refresh.';
    }
    if (low === 'no_portals') {
      return 'No portal bound — ask admin to add your aMember site in Ahrefs Bar → Portals.';
    }
    if (low.indexOf('no_cookie') >= 0 || low.indexOf('cookie') >= 0) {
      return 'Membership cookie missing — log in again on your aMember site, then Refresh.';
    }
    if (low.indexOf('mint_failed') >= 0) {
      return 'Session mint failed — click Refresh. If it keeps failing, contact admin.';
    }
    if (low.indexOf('subscription expired') >= 0) {
      return e;
    }
    if (low.indexOf('unauthorized') >= 0 || low.indexOf('deny') >= 0 || low.indexOf('plan') >= 0) {
      return 'No active plan on this membership — renew or contact admin. (' + e + ')';
    }
    if (low === 'check_timeout' || low === 'verify_timeout') {
      return 'Membership check timed out — click Refresh.';
    }
    if (low.indexOf('verify') >= 0) {
      return 'Membership check failed — confirm you are logged in on aMember. (' + e + ')';
    }
    return e;
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

  function diagnosticMessage() {
    const fromBlock = blockMessage(liveBlock);
    if (fromBlock) return fromBlock;
    if (liveBlock && liveBlock.kind === 'pool') return SUB_EXPIRED_MSG;
    if (!lastAuth.ok) return humanizeAuthError(lastAuth.error);
    return '';
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
    const msg = diagnosticMessage() || (lastAuth.ok ? SUB_EXPIRED_MSG : humanizeAuthError(lastAuth.error));
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
      if (t === GENERIC_FETCH_ERR || t === USAGE_LIMIT_ERR) {
        el.textContent = msg;
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
    lastAuth = { ok: ok, error: (r && r.tm_bar_error) || '' };
    dot.style.background = ok ? '#22c55e' : '#ef4444';
    if (ok) {
      const u = (r.tm_bar_user && (r.tm_bar_user.login || r.tm_bar_user.name)) || 'ok';
      status.textContent = u + (r.tm_bar_portal ? ' · ' + r.tm_bar_portal : '');
      status.style.color = '#86efac';
    } else {
      status.textContent = humanizeAuthError(lastAuth.error);
      status.style.color = '#fca5a5';
    }
    verEl.textContent = 'v' + ((r && r.version) || manVer || '');

    liveBlock = (r && r.tm_bar_block) || null;
    if (liveBlock && liveBlock.retry_at && liveBlock.retry_at <= Date.now()) liveBlock = null;
    // When metrics fail with a pool message, also mirror on the strip if authed.
    if (ok && liveBlock && blockMessage(liveBlock)) {
      status.textContent = blockMessage(liveBlock);
      status.style.color = '#fcd34d';
    }
    paintNotice();
    rewriteGenericErrors();
    if (liveBlock && liveBlock.retry_at) startTick();
  }

  /** Fast path — no service worker round-trip (cannot hang). */
  function paintFromStorage() {
    try {
      chrome.storage.local.get(
        ['tm_bar_authed', 'tm_bar_user', 'tm_bar_portal', 'tm_bar_error', 'tm_bar_block'],
        (st) => {
          if (chrome.runtime.lastError) {
            paint({ tm_bar_authed: false, tm_bar_error: 'storage error', version: manVer });
            return;
          }
          paint(Object.assign({ version: manVer }, st || {}));
        },
      );
    } catch (e) {
      paint({ tm_bar_authed: false, tm_bar_error: 'storage error', version: manVer });
    }
  }

  function askRefresh(onDone) {
    let finished = false;
    const finish = () => {
      if (finished) return;
      finished = true;
      try {
        onDone && onDone();
      } catch (_) {}
    };
    const t = setTimeout(finish, 6000);
    try {
      chrome.runtime.sendMessage({ type: 'tm:refresh' }, () => {
        clearTimeout(t);
        // Ignore lastError — still re-paint from storage.
        finish();
      });
    } catch (_) {
      clearTimeout(t);
      finish();
    }
  }

  function syncThenPaint() {
    // 1) Always show last known state immediately (never blank "checking…").
    paintFromStorage();
    status.style.color = '#eab308';
    // 2) Background re-check; re-paint when done or after 6s max.
    askRefresh(() => paintFromStorage());
  }

  btn.addEventListener('click', () => {
    status.textContent = 'refreshing…';
    status.style.color = '#eab308';
    btn.disabled = true;
    askRefresh(() => {
      btn.disabled = false;
      paintFromStorage();
    });
  });

  upd.addEventListener('click', () => {
    status.textContent = 'fetching update…';
    status.style.color = '#eab308';
    upd.disabled = true;
    let done = false;
    const finish = (r) => {
      if (done) return;
      done = true;
      upd.disabled = false;
      if (r && r.error) {
        status.textContent = r.error;
        status.style.color = '#fca5a5';
        return;
      }
      status.textContent = (r && r.opened) || 'update opened — unzip & Load unpacked';
      status.style.color = '#93c5fd';
      if (r && r.version) verEl.textContent = 'v' + r.version + (r.remote_version ? ' → ' + r.remote_version : '');
    };
    setTimeout(() => finish({ error: 'update timeout' }), 8000);
    try {
      chrome.runtime.sendMessage({ type: 'tm:update' }, (r) => finish(r || {}));
    } catch (e) {
      finish({ error: e.message || 'update failed' });
    }
  });

  chrome.storage.onChanged.addListener((changes, area) => {
    if (area !== 'local') return;
    if (changes.tm_bar_block || changes.tm_bar_authed || changes.tm_bar_error || changes.tm_bar_user) {
      paintFromStorage();
    }
  });

  const mo = new MutationObserver(() => rewriteGenericErrors());
  mo.observe(document.documentElement, { childList: true, subtree: true, characterData: true });

  syncThenPaint();
})();
