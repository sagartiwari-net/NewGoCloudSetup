/*
 * tm-guard.js — ToolsMandi Ahrefs Bar
 * - Loads official Ahrefs toolbar SW (event.js)
 * - Portal cookie → /v1/session/verify → tb.* session (bound to device FP)
 * - Fetch rewrite: https://ahrefs.com/v4 → local/prod bar-api (Chrome blocks https→http DNR)
 * - DNR: inject X-TM-Bar-Token + X-TM-Bar-FP when proxy is https
 * - IndexedDB cache (tm-cache.js)
 */
importScripts('tm-config.js', 'tm-cache.js', 'tm-fp.js');

// Must wrap fetch BEFORE event.js captures it. Chrome also blocks DNR redirects
// from https://ahrefs.com → http://127.0.0.1, so local needs this rewrite.
(() => {
  const CFG0 = self.TM_BAR_CONFIG || {};
  const API0 = String(CFG0.apiBase || 'http://127.0.0.1:8088').replace(/\/$/, '');
  const AH_V4 = 'https://ahrefs.com/v4/';
  const origFetch = self.fetch.bind(self);
  self.__tmLiveSession = '';
  self.__tmLiveFp = '';
  self.fetch = async function tmFetch(input, init) {
    let url = '';
    try {
      url = typeof input === 'string' ? input : (input && input.url) || '';
    } catch (_) {}
    if (url && url.startsWith(AH_V4)) {
      const dest = API0 + url.slice('https://ahrefs.com'.length);
      let headers;
      try {
        if (init && init.headers) headers = new Headers(init.headers);
        else if (input && typeof input !== 'string' && input.headers) headers = new Headers(input.headers);
        else headers = new Headers();
      } catch (_) {
        headers = new Headers();
      }
      let tok = self.__tmLiveSession;
      let fp = self.__tmLiveFp;
      if (!tok) {
        try {
          const st = await new Promise((r) => chrome.storage.local.get(['tm_bar_session'], r));
          tok = st.tm_bar_session || '';
        } catch (_) {}
      }
      if (!fp && self.tmFp) {
        try { fp = await self.tmFp.getFingerprint(); } catch (_) {}
      }
      if (tok) headers.set('X-TM-Bar-Token', tok);
      if (fp) headers.set('X-TM-Bar-FP', fp);
      headers.delete('Authorization');
      const nextInit = Object.assign({}, init || {}, { headers });
      if ((!init || init.body === undefined) && input && typeof input !== 'string') {
        try {
          if (input.method) nextInit.method = input.method;
          if (input.body != null && nextInit.body === undefined) nextInit.body = input.body;
        } catch (_) {}
      }
      try {
        console.log('[tm-bar] v4 rewrite', url.slice(0, 100), '→', dest.slice(0, 100));
      } catch (_) {}
      const resp = await origFetch(dest, nextInit);
      try {
        console.log('[tm-bar] v4 status', resp.status, dest.split('/v4/')[1] || dest);
      } catch (_) {}
      try {
        const clone = resp.clone();
        clone.json().then((body) => {
          try { self.__tmNoteProxyBody && self.__tmNoteProxyBody(body, resp.headers); } catch (_) {}
        }).catch(() => {});
      } catch (_) {}
      return resp;
    }
    return origFetch(input, init);
  };
})();

importScripts('event.js');

(() => {
  const CFG = self.TM_BAR_CONFIG || {};
  const API_BASE = String(CFG.apiBase || 'http://127.0.0.1:8088').replace(/\/$/, '');
  const ALARM = 'tm-bar-auth-sync';
  const AUTH_RULE_ID = 1000;
  const PROXY_FILTER = String(CFG.proxyHostFilter || '||127.0.0.1:8088/v4/');
  const PROXY_TYPES = ['xmlhttprequest', 'other', 'main_frame', 'sub_frame'];
  const DBG = true;
  const logs = [];
  let lastAuthKey = null;

  const getLocal = (keys) => new Promise((r) => chrome.storage.local.get(keys, r));
  const setLocal = (obj) => new Promise((r) => chrome.storage.local.set(obj, r));

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

  function limitMessage(windowName, sec) {
    const wait = formatWait(sec);
    if (windowName === 'daily') return "Today's limit reached. Try again in " + wait + '.';
    return 'Hourly limit reached. Try again in ' + wait + '.';
  }

  async function setBarBlock(block) {
    await setLocal({ tm_bar_block: block || null });
  }

  self.__tmNoteProxyBody = async function noteProxyBody(body, headers) {
    if (!Array.isArray(body)) return;
    if (body[0] === 'Ok') {
      const st = await getLocal(['tm_bar_block']);
      if (st.tm_bar_block) await setBarBlock(null);
      return;
    }
    if (body[0] !== 'Error') return;
    const code = body[1];
    const detail = body[2] && typeof body[2] === 'object' ? body[2] : null;
    const hdrRetry = parseInt((headers && headers.get && headers.get('X-TM-Bar-Retry-After')) || '0', 10) || 0;
    const hdrMsg = (headers && headers.get && headers.get('X-TM-Bar-Limit-Message')) || '';
    if (code === 'LimitReached') {
      const hdrWindow = (headers && headers.get && headers.get('X-TM-Bar-Limit')) ||
        (headers && headers.get && headers.get('X-TM-Bar-Limit-Window')) || '';
      const windowName = (detail && detail.window) || hdrWindow ||
        (typeof body[2] === 'string' ? body[2] : 'hourly') || 'hourly';
      const retry = (detail && detail.retry_after_sec) || hdrRetry || 3600;
      const message = (detail && detail.message) || hdrMsg || limitMessage(windowName, retry);
      await setBarBlock({
        kind: 'limit',
        window: windowName,
        retry_after_sec: retry,
        retry_at: Date.now() + retry * 1000,
        message: message,
      });
      return;
    }
    if (code === 'DataSourceUnavailable' || code === 'RateLimit') {
      const retry = (detail && detail.retry_after_sec) || hdrRetry || (code === 'RateLimit' ? 15 : 0);
      const message = (detail && detail.message) || hdrMsg ||
        (code === 'RateLimit'
          ? 'Ahrefs is busy right now. Try again in ' + formatWait(retry || 15) + '.'
          : retry > 0
            ? 'Ahrefs account cooling down. Try again in ' + formatWait(retry) + '.'
            : 'No Ahrefs account available right now. Please try again later.');
      await setBarBlock({
        kind: code === 'RateLimit' ? 'upstream' : 'pool',
        window: '',
        retry_after_sec: retry,
        retry_at: retry > 0 ? Date.now() + retry * 1000 : 0,
        message: message,
      });
    }
  };

  function log(...args) {
    const line = '[tm-bar ' + new Date().toISOString().slice(11, 23) + '] ' +
      args.map((a) => (typeof a === 'string' ? a : JSON.stringify(a))).join(' ');
    logs.push(line);
    if (logs.length > 200) logs.shift();
    if (DBG) console.log(line);
  }

  function mask(t) {
    if (!t) return '(none)';
    const s = String(t);
    return s.length <= 12 ? '(len ' + s.length + ')' : s.slice(0, 6) + '…' + s.slice(-4);
  }

  async function fingerprint() {
    if (!self.tmFp) return '';
    return self.tmFp.getFingerprint();
  }

  async function setAuthHeaderRules(token, fp) {
    const key = (token || '') + '|' + (fp || '');
    if (key === lastAuthKey) return;
    lastAuthKey = key;
    try {
      if (token) {
        const headers = [
          { header: 'X-TM-Bar-Token', operation: 'set', value: token },
        ];
        if (fp) {
          headers.push({ header: 'X-TM-Bar-FP', operation: 'set', value: fp });
        }
        await chrome.declarativeNetRequest.updateDynamicRules({
          removeRuleIds: [AUTH_RULE_ID],
          addRules: [{
            id: AUTH_RULE_ID,
            priority: 1,
            action: { type: 'modifyHeaders', requestHeaders: headers },
            condition: { urlFilter: PROXY_FILTER, resourceTypes: PROXY_TYPES },
          }],
        });
        log('auth headers SET', mask(token), mask(fp));
      } else {
        await chrome.declarativeNetRequest.updateDynamicRules({ removeRuleIds: [AUTH_RULE_ID] });
        log('auth headers REMOVED');
      }
    } catch (e) {
      log('setAuthHeaderRules error', e.message);
    }
  }

  // event.js switch after Fs(): 0/1 = fetch keyword+header stats; 2 = Starter (no API); 3 = exhausted.
  // Store plain 0 so TAG handlers fetch SERP domain bars + KeywordStats without waiting on userInfo.
  function paidUserData(email) {
    return {
      email: email || 'dev@local.dev',
      keywordsLimit: 0,
      siteExplorerLimit: 0,
      workspaceName: 'ToolsMandi',
    };
  }

  function needsPaidLimits(ud) {
    if (!ud || typeof ud !== 'object') return true;
    const k = ud.keywordsLimit;
    const s = ud.siteExplorerLimit;
    // Accept numeric 0/1 OR Melange "Unlimited" string from tbGetUserInfoV3.
    const ok = (v) => v === 0 || v === 1 || v === 'Unlimited';
    return !ok(k) || !ok(s);
  }

  function serpLayoutPaid() {
    return [
      { key: 'serp', label: 'Search results metrics', value: true },
      { key: 'sers', label: 'Google keyword metrics', value: true },
      { key: 'yt_sers', label: 'YouTube keyword metrics', value: true },
      { key: 'serp_count', label: 'SERP position numbers', value: true },
      { key: 'serp_highlight_domain', label: 'Highlight results', value: true },
      { key: 'g_trends', label: 'Show keyword metrics in Trends', value: true },
    ];
  }

  async function applyAuthed(sessionToken, user, portal) {
    const fp = await fingerprint();
    self.__tmLiveSession = sessionToken || '';
    self.__tmLiveFp = fp || '';
    await setAuthHeaderRules(sessionToken, fp);
    await setLocal({
      tm_bar_authed: true,
      tm_bar_session: sessionToken,
      tm_bar_user: user || null,
      tm_bar_portal: portal || '',
      tm_bar_error: '',
      // Official toolbar keys — keep re-asserting; event.js may clear them on boot.
      ahIsEnabled: true,
      ahIsAuth3: true,
      ahIsShowKeywords: true,
      ahIsShowSerpData: true,
      // Index [1] = "Google keyword metrics" — must be true or W6/tbGetKeywordStatsV3 never runs.
      ahSerpLayoutParams: serpLayoutPaid(),
      ahToken: { accessToken: sessionToken, expiresIn: 315360000 },
      ahUserData: paidUserData((user && user.email) || 'dev@local.dev'),
    });
  }

  async function applyParked(error) {
    self.__tmLiveSession = '';
    self.__tmLiveFp = '';
    await setAuthHeaderRules('', '');
    await setLocal({
      tm_bar_authed: false,
      tm_bar_session: '',
      tm_bar_user: null,
      tm_bar_portal: '',
      tm_bar_error: error || 'not_logged_in',
      ahIsAuth3: false,
      ahIsShowKeywords: false,
      ahIsShowSerpData: false,
      ahToken: { accessToken: undefined, expiresIn: undefined },
      // Starter defaults so official toolbar stops billable SERP/domain fetches.
      ahUserData: {
        email: '',
        keywordsLimit: 2,
        siteExplorerLimit: 2,
        workspaceName: '',
      },
    });
  }

  async function revokeServerSession(token) {
    if (!token) return;
    try {
      await fetch(API_BASE + '/v1/session/logout', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-TM-Bar-Token': token,
        },
        body: JSON.stringify({ token }),
      });
    } catch (e) {
      log('revoke session failed', e.message);
    }
  }

  function removeCookie(url, name) {
    return new Promise((res) => {
      try {
        chrome.cookies.remove({ url, name }, () => {
          if (chrome.runtime.lastError) return res(false);
          res(true);
        });
      } catch (_) {
        res(false);
      }
    });
  }

  async function fetchPortals() {
    try {
      const rid = Number(CFG.resellerId) || 0;
      if (!rid) {
        log('portals skipped — extension not stamped with resellerId');
        return [];
      }
      const r = await fetch(API_BASE + '/v1/portals?resellerId=' + encodeURIComponent(String(rid)), {
        cache: 'no-store',
        headers: { 'X-TM-Bar-Reseller': String(rid) },
      });
      const j = await r.json();
      if (j && j.success && Array.isArray(j.portals)) return j.portals;
    } catch (e) {
      log('portals fetch failed', e.message);
    }
    return [];
  }

  function getCookie(url, name) {
    return new Promise((res) => {
      chrome.cookies.get({ url, name }, (c) => {
        if (chrome.runtime.lastError) return res('');
        res(c && c.value ? c.value : '');
      });
    });
  }

  function setCookie(url, name, value) {
    return new Promise((res) => {
      try {
        // Host-only cookie (no `domain`) — Chrome often rejects domain cookies for
        // synthetic hosts like local.dev used in DEV_AUTH.
        chrome.cookies.set({
          url,
          name,
          value,
          path: '/',
          secure: String(url).startsWith('https:'),
          expirationDate: Math.floor(Date.now() / 1000) + 86400 * 7,
        }, (c) => {
          if (chrome.runtime.lastError) {
            log('cookie.set error', chrome.runtime.lastError.message);
            return res(false);
          }
          res(!!c);
        });
      } catch (e) {
        log('cookie.set threw', e.message);
        res(false);
      }
    });
  }

  // Read portal session cookie only — NEVER invent one.
  // aMember Pro often uses amember_nr (not PHPSESSID). Try configured name first, then common aliases.
  async function readPortalCookie(portal) {
    const site = portal.site_url;
    const preferred = portal.cookie_name || 'PHPSESSID';
    const names = [preferred, 'amember_nr', 'PHPSESSID', 'amember_ru', 'amember_r', 'amember_sess', 'amember', 'panel_session'];
    const seen = {};
    for (const name of names) {
      if (!name || seen[name]) continue;
      seen[name] = true;
      const val = await getCookie(site, name);
      if (val) return { value: val, name, site };
    }
    return { value: '', name: preferred, site };
  }

  async function verify(portal, cookieValue, cookieName) {
    const fp = await fingerprint();
    const rid = Number(CFG.resellerId) || 0;
    const r = await fetch(API_BASE + '/v1/session/verify', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-TM-Bar-FP': fp,
        ...(rid ? { 'X-TM-Bar-Reseller': String(rid) } : {}),
      },
      body: JSON.stringify({
        token: cookieValue,
        portal: portal.site_url,
        cookie_name: cookieName,
        fingerprint: fp,
        reseller_id: rid,
      }),
    });
    return r.json();
  }

  /**
   * Auth model (aMember-ready):
   *   live portal cookie → membership check (tm-check.php) → mint/refresh tb.*
   *   no cookie / check fail → revoke tb.* + park toolbar
   * tb.* alone is NOT enough after portal logout.
   */
  async function syncAuth(reason) {
    log('syncAuth', reason);
    const portals = await fetchPortals();
    if (!portals.length) {
      const cur = await getLocal(['tm_bar_session']);
      if (cur.tm_bar_session) await revokeServerSession(cur.tm_bar_session);
      await applyParked('no_portals');
      return { authed: false, error: 'no_portals' };
    }

    const prev = await getLocal(['tm_bar_session']);
    let sawCookie = false;

    for (const p of portals) {
      try {
        const c = await readPortalCookie(p);
        if (!c.value) {
          log('no portal cookie', p.site_url, c.name);
          continue;
        }
        sawCookie = true;
        const j = await verify(p, c.value, c.name);
        if (j && j.success && j.valid && j.session_token) {
          await applyAuthed(j.session_token, j.user, j.portal || p.site_url);
          log('AUTHED', p.site_url, mask(j.session_token), 'via', c.name);
          return { authed: true, portal: p.site_url };
        }
        log('verify denied', p.site_url, j && j.error);
      } catch (e) {
        log('verify error', p.site_url, e.message);
      }
    }

    if (prev.tm_bar_session) {
      await revokeServerSession(prev.tm_bar_session);
    }
    const err = sawCookie ? 'unauthorized' : 'not_logged_in';
    await applyParked(err);
    log('NOT AUTHED', err, 'reason=' + reason);
    return { authed: false, error: err };
  }

  // Console-only helper. Production / Markhor path never invents cookies —
  // panel or aMember login must set them.
  async function logoutAll(reason) {
    log('logoutAll', reason || 'manual');
    const st = await getLocal(['tm_bar_session']);
    if (st.tm_bar_session) await revokeServerSession(st.tm_bar_session);
    const portals = await fetchPortals();
    for (const p of portals) {
      const name = p.cookie_name || 'PHPSESSID';
      await removeCookie(p.site_url, name);
      await removeCookie(p.site_url, 'panel_session');
      await removeCookie(p.site_url, 'amember_nr');
      await removeCookie(p.site_url, 'amember_ru');
    }
    // Stale synthetic host from older builds
    await removeCookie('https://local.dev', 'PHPSESSID');
    await applyParked('logged_out');
    return { authed: false, error: 'logged_out' };
  }

  async function fetchMe() {
    const { tm_bar_session } = await getLocal(['tm_bar_session']);
    if (!tm_bar_session) return null;
    try {
      const fp = await fingerprint();
      const r = await fetch(API_BASE + '/v1/me', {
        headers: {
          'X-TM-Bar-Token': tm_bar_session,
          'X-TM-Bar-FP': fp,
        },
      });
      return r.json();
    } catch (e) {
      return { success: false, error: e.message };
    }
  }

  function parseIconOk(text) {
    try {
      const parsed = JSON.parse(text);
      if (Array.isArray(parsed) && parsed[0] === 'Ok' && parsed[1] && typeof parsed[1] === 'object') {
        return { ok: true, metrics: parsed[1], parsed };
      }
      return { ok: false, parsed };
    } catch (_) {
      return { ok: false, parsed: null };
    }
  }

  async function testProxy(target, opts) {
    target = target || 'ahrefs.com';
    const fresh = !!(opts && opts.fresh);
    const { tm_bar_session } = await getLocal(['tm_bar_session']);
    if (!tm_bar_session) {
      return { error: 'not_authed', hint: 'Refresh access first' };
    }

    if (!fresh && self.tmCache) {
      try {
        const hit = await self.tmCache.getDomainMetrics(target);
        if (hit && hit.data) {
          const body = JSON.stringify(['Ok', hit.data]);
          log('testProxy CACHE HIT', target, 'ageMs=' + hit.ageMs);
          return {
            mode: 'browser_cache',
            status: 200,
            url: '(indexeddb)',
            body: body.slice(0, 400),
            ok: true,
            cache: true,
            ageMs: hit.ageMs,
            source: hit.source,
            note: 'Served from extension IndexedDB (0 credit). Use Fresh to bypass.',
          };
        }
      } catch (e) {
        log('cache get error', e.message);
      }
    }

    const fp = await fingerprint();
    const q = 'input=' + encodeURIComponent(JSON.stringify({ target }));
    let directURL = API_BASE + '/v4/tbGetIconV3?' + q;
    if (fresh) directURL += '&fresh=1';
    log('testProxy direct', directURL, fresh ? 'FRESH' : '');
    try {
      const headers = {
        'X-TM-Bar-Token': tm_bar_session,
        'X-TM-Bar-FP': fp,
      };
      if (fresh) headers['X-TM-Bar-Fresh'] = '1';
      const resp = await fetch(directURL, { method: 'GET', headers });
      const text = await resp.text();
      const parsed = parseIconOk(text);
      if (parsed.ok && self.tmCache) {
        try {
          await self.tmCache.putDomainMetrics(target, parsed.metrics, fresh ? 'v4-fresh' : 'v4');
        } catch (e) {
          log('cache put error', e.message);
        }
      }
      const out = {
        mode: fresh ? 'direct_local_api_fresh' : 'direct_local_api',
        status: resp.status,
        url: directURL,
        body: text.slice(0, 400),
        ok: parsed.ok,
        cache: false,
        fresh: fresh,
        note: fresh
          ? 'Bypassed browser + server caches (X-TM-Bar-Fresh).'
          : 'Session bound to device fingerprint (X-TM-Bar-FP).',
      };
      log('testProxy RESULT', out.status, out.ok, out.mode);
      return out;
    } catch (e) {
      log('testProxy ERROR', e.message);
      return { error: e.message, hint: 'Is API running on ' + API_BASE + '?' };
    }
  }

  async function cacheStats() {
    if (!self.tmCache) return { error: 'tmCache missing' };
    return self.tmCache.stats();
  }

  async function cacheClear() {
    if (!self.tmCache) return { error: 'tmCache missing' };
    await self.tmCache.clearAll();
    log('cache cleared');
    return { ok: true };
  }

  async function cachePrewarm(msg) {
    if (!self.tmCache) return { error: 'tmCache missing' };
    const r = await self.tmCache.prewarmSerp(msg.engine || 'google', msg.query || '', msg.domains || []);
    log('serp prewarm', r);
    return { ok: true, ...r };
  }

  chrome.runtime.onInstalled.addListener(() => { syncAuth('installed'); });
  chrome.runtime.onStartup.addListener(() => { syncAuth('startup'); });
  chrome.alarms.create(ALARM, { periodInMinutes: 5 });
  chrome.alarms.onAlarm.addListener((a) => {
    if (a.name === ALARM) syncAuth('alarm');
  });

  chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
    if (!msg || typeof msg !== 'object') return;
    if (msg.type === 'tm:refresh') {
      syncAuth('manual').then(sendResponse);
      return true;
    }
    if (msg.type === 'tm:status') {
      getLocal(['tm_bar_authed', 'tm_bar_user', 'tm_bar_portal', 'tm_bar_error', 'tm_bar_session', 'tm_bar_block']).then(async (st) => {
        const me = st.tm_bar_authed ? await fetchMe() : null;
        let block = st.tm_bar_block || null;
        if (block && block.retry_at && block.retry_at <= Date.now()) {
          block = null;
          await setBarBlock(null);
        }
        let cache = null;
        try { cache = self.tmCache ? await self.tmCache.stats() : null; } catch (_) {}
        const fp = await fingerprint();
        const man = chrome.runtime.getManifest();
        sendResponse({
          ...st,
          tm_bar_block: block,
          me,
          cache,
          session_mask: mask(st.tm_bar_session),
          fp_mask: mask(fp),
          version: (CFG.version || man.version || ''),
          buildId: CFG.buildId || '',
        });
      });
      return true;
    }
    if (msg.type === 'tm:update') {
      (async () => {
        const man = chrome.runtime.getManifest();
        const localVer = CFG.version || man.version || '';
        let remote = null;
        try {
          const r = await fetch(API_BASE + '/v1/extension/update', { cache: 'no-store' });
          remote = await r.json();
        } catch (e) {
          sendResponse({ error: 'update check failed: ' + e.message });
          return;
        }
        const zipUrl = (remote && remote.download_url) || CFG.updateUrl || (API_BASE + '/v1/extension/download.zip');
        const panelUrl = (remote && remote.panel_url) || CFG.panelUrl || '';
        // Open zip download (same idea as old bar “get update”).
        try {
          await chrome.tabs.create({ url: zipUrl, active: true });
        } catch (e) {
          sendResponse({ error: 'could not open download: ' + e.message });
          return;
        }
        // Also open panel download page when configured (branded reseller zip).
        if (panelUrl && panelUrl !== zipUrl) {
          try { await chrome.tabs.create({ url: panelUrl, active: false }); } catch (_) {}
        }
        log('update opened', zipUrl, 'remote=', remote && remote.version);
        sendResponse({
          ok: true,
          opened: 'zip download started',
          version: localVer,
          remote_version: (remote && remote.version) || '',
          download_url: zipUrl,
          panel_url: panelUrl,
          notes: (remote && remote.notes) || '',
        });
      })();
      return true;
    }
    if (msg.type === 'tm:test-proxy') {
      testProxy(msg.target, { fresh: !!msg.fresh }).then(sendResponse);
      return true;
    }
    if (msg.type === 'tm:cache-stats') {
      cacheStats().then(sendResponse);
      return true;
    }
    if (msg.type === 'tm:cache-clear') {
      cacheClear().then(sendResponse);
      return true;
    }
    if (msg.type === 'tm:cache-prewarm') {
      cachePrewarm(msg).then(sendResponse);
      return true;
    }
    if (msg.type === 'tm:cache-get-domain') {
      if (!self.tmCache) { sendResponse({ error: 'tmCache missing' }); return true; }
      self.tmCache.getDomainMetrics(msg.target || '').then(sendResponse);
      return true;
    }
    if (msg.type === 'tm:logs') {
      sendResponse({ logs: logs.slice() });
      return true;
    }
    if (msg.type === 'tm:keyword-stats') {
      (async () => {
        const keyword = String(msg.keyword || '').trim().toLowerCase();
        const country = msg.country && msg.country !== 'us' ? String(msg.country) : '';
        if (!keyword) {
          sendResponse({ ok: false, error: 'empty_keyword' });
          return;
        }
        if (!self.__tmKwCache) self.__tmKwCache = new Map();
        if (!self.__tmKwInflight) self.__tmKwInflight = new Map();
        const cacheKey = keyword + '|' + (country || 'us');
        const hit = self.__tmKwCache.get(cacheKey);
        if (hit && hit.expires > Date.now()) {
          sendResponse({ ok: true, stats: hit.stats, cached: true });
          return;
        }
        if (self.__tmKwInflight.has(cacheKey)) {
          try {
            const stats = await self.__tmKwInflight.get(cacheKey);
            if (stats) sendResponse({ ok: true, stats: stats, cached: true });
            else sendResponse({ ok: false, error: 'inflight_fail' });
          } catch (e) {
            sendResponse({ ok: false, error: e.message });
          }
          return;
        }
        // First SERP paint often races boot syncAuth — wait briefly for session.
        let tok = self.__tmLiveSession || '';
        let fp = self.__tmLiveFp || '';
        if (!tok) {
          for (let i = 0; i < 8 && !tok; i++) {
            if (i > 0) await new Promise((r) => setTimeout(r, 250));
            const st = await getLocal(['tm_bar_session']);
            tok = self.__tmLiveSession || st.tm_bar_session || '';
          }
        }
        if (!fp && self.tmFp) {
          try { fp = await self.tmFp.getFingerprint(); } catch (_) {}
        }
        if (!tok) {
          sendResponse({ ok: false, error: 'not_authed' });
          return;
        }
        const inputObj = { keyword: keyword };
        if (country) inputObj.country = country;
        const input = encodeURIComponent(JSON.stringify(inputObj));
        const work = (async () => {
          const headers = { 'Content-Type': 'application/json;charset=utf-8' };
          headers['X-TM-Bar-Token'] = tok;
          if (fp) headers['X-TM-Bar-FP'] = fp;
          const r = await fetch(API_BASE + '/v4/tbGetKeywordStatsV3?input=' + input, {
            method: 'GET',
            headers,
            cache: 'no-store',
          });
          const body = await r.json();
          if (Array.isArray(body) && body[0] === 'Ok' && body[1] && body[1].stats) {
            self.__tmKwCache.set(cacheKey, {
              stats: body[1].stats,
              expires: Date.now() + 60 * 60 * 1000,
            });
            return body[1].stats;
          }
          const err = Array.isArray(body) ? body[1] : 'bad_response';
          const detail = Array.isArray(body) && body[2] && typeof body[2] === 'object' ? body[2] : null;
          const e = new Error(typeof err === 'string' ? err : 'bad_response');
          e.limit = err === 'LimitReached';
          e.raw = body;
          e.detail = detail;
          const hdrRetry = parseInt(r.headers.get('X-TM-Bar-Retry-After') || '0', 10) || 0;
          const hdrMsg = r.headers.get('X-TM-Bar-Limit-Message') || '';
          const hdrWindow = r.headers.get('X-TM-Bar-Limit') || r.headers.get('X-TM-Bar-Limit-Window') || '';
          e.window = (detail && detail.window) || hdrWindow || '';
          e.retryAfter = (detail && detail.retry_after_sec) || hdrRetry || 0;
          e.messageText = (detail && detail.message) || hdrMsg ||
            (e.limit ? limitMessage(e.window || 'hourly', e.retryAfter || 3600) : '');
          if (e.limit) {
            await setBarBlock({
              kind: 'limit',
              window: e.window || 'hourly',
              retry_after_sec: e.retryAfter || 3600,
              retry_at: Date.now() + (e.retryAfter || 3600) * 1000,
              message: e.messageText,
            });
          }
          throw e;
        })();
        self.__tmKwInflight.set(cacheKey, work.then(
          (s) => s,
          () => null
        ));
        try {
          const stats = await work;
          sendResponse({ ok: true, stats: stats });
        } catch (e) {
          sendResponse({
            ok: false,
            error: e.message || 'fail',
            limit: !!e.limit,
            message: e.messageText || '',
            retry_after_sec: e.retryAfter || 0,
            window: e.window || '',
          });
        } finally {
          self.__tmKwInflight.delete(cacheKey);
        }
      })();
      return true;
    }
  });

  self.tmBarStatus = async () => getLocal(null);
  self.tmBarLogs = () => logs.slice();
  self.tmBarSync = () => syncAuth('console');
  self.tmBarLogout = () => logoutAll('console');
  self.tmBarDiag = (t, fresh) => testProxy(t, { fresh: !!fresh });

  function needsKeywordSerpToggle(params) {
    if (!Array.isArray(params) || params.length < 2) return true;
    const sers = params[1];
    return !(sers && sers.value === true);
  }

  // If official SW clears ahIsAuth3 / resets Starter limits after our sync, re-assert.
  function reassertAuth(reason) {
    getLocal(['tm_bar_session', 'ahIsAuth3', 'ahUserData', 'ahSerpLayoutParams']).then(async (st) => {
      if (!st.tm_bar_session || !String(st.tm_bar_session).startsWith('tb.')) return;
      const badAuth = st.ahIsAuth3 !== true;
      const badLimits = needsPaidLimits(st.ahUserData);
      const badSerp = needsKeywordSerpToggle(st.ahSerpLayoutParams);
      if (!badAuth && !badLimits && !badSerp) return;
      log('reassert auth', reason, badAuth ? 'auth' : '', badLimits ? 'limits' : '', badSerp ? 'serp' : '');
      const u = await getLocal(['tm_bar_user', 'tm_bar_portal']);
      await applyAuthed(st.tm_bar_session, u.tm_bar_user, u.tm_bar_portal || '');
    });
  }

  // event.js Q_() falls back to {keywordsLimit:2} on userInfo decode miss → Keywords ideas
  // never calls tbGetKeywordStatsV3. Clamp any non-paid write while we are authed.
  let fixingUserData = false;
  chrome.storage.onChanged.addListener((changes, area) => {
    if (area !== 'local' || fixingUserData) return;
    const ch = changes.ahUserData;
    if (!ch || !needsPaidLimits(ch.newValue)) return;
    getLocal(['tm_bar_session', 'tm_bar_user']).then(async (st) => {
      if (!st.tm_bar_session || !String(st.tm_bar_session).startsWith('tb.')) return;
      fixingUserData = true;
      try {
        const email = (ch.newValue && ch.newValue.email) ||
          (st.tm_bar_user && st.tm_bar_user.email) ||
          'dev@local.dev';
        log('clamp ahUserData limits', ch.newValue && ch.newValue.keywordsLimit, '→ 0');
        await setLocal({ ahUserData: paidUserData(email), ahIsShowKeywords: true });
      } finally {
        fixingUserData = false;
      }
    });
  });

  // aMember / panel logout clears the portal cookie → park immediately.
  const PORTAL_COOKIE_NAMES = new Set([
    'PHPSESSID', 'panel_session', 'amember_nr', 'amember_ru', 'amember_r', 'amember_sess',
  ]);
  let cookieSyncTimer = 0;
  chrome.cookies.onChanged.addListener((info) => {
    try {
      const c = info.cookie;
      if (!c || !PORTAL_COOKIE_NAMES.has(c.name)) return;
      log('portal cookie changed', c.name, 'removed=' + info.removed, c.domain || '');
      // Debounce bursts (login sets multiple cookies).
      clearTimeout(cookieSyncTimer);
      cookieSyncTimer = setTimeout(() => syncAuth('cookie-change'), 250);
    } catch (_) {}
  });

  log('tm-guard loaded', CFG.brandName || 'ToolsMandi', CFG.buildId || 'dev', API_BASE);
  syncAuth('boot').then(() => {
    setTimeout(() => reassertAuth('t+1s'), 1000);
    setTimeout(() => reassertAuth('t+3s'), 3000);
    setTimeout(() => syncAuth('boot-retry'), 5000);
  });
})();
