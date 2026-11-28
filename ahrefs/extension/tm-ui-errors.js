/**
 * tm-ui-errors.js — replace Melange's generic "Error fetching data…"
 * on the in-page metrics bar / panels with ToolsMandi diagnostics
 * (login, cookie, plan, subscription / pool).
 */
(function tmUiErrors() {
  const GENERIC = 'Error fetching data. Please try again later.';
  const USAGE = "You've reached your usage limit";
  const SUB = 'Subscription expired. Contact to admin.';
  const replaced = new WeakSet();
  let live = { ok: false, error: '', block: null };

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

  function humanizeAuthError(err) {
    const e = String(err || '').trim();
    if (!e) return 'Not signed in — log in on your membership site, then Refresh.';
    const low = e.toLowerCase();
    if (low === 'not_logged_in' || low === 'logged_out') {
      return 'Not logged in — open your aMember site, sign in, then Refresh.';
    }
    if (low === 'no_portals') {
      return 'No portal bound — ask admin to add your aMember site.';
    }
    if (low.indexOf('cookie') >= 0) {
      return 'Membership cookie missing — log in again on aMember, then Refresh.';
    }
    if (low.indexOf('mint_failed') >= 0) {
      return 'Session mint failed — Refresh, or contact admin.';
    }
    if (low.indexOf('subscription expired') >= 0) return e;
    if (low.indexOf('unauthorized') >= 0 || low.indexOf('deny') >= 0 || low.indexOf('plan') >= 0) {
      return 'No active plan — renew membership or contact admin.';
    }
    if (low === 'check_timeout' || low === 'verify_timeout') {
      return 'Membership check timed out — click Refresh.';
    }
    return e;
  }

  function blockMessage(block) {
    if (!block) return '';
    if (block.message) return String(block.message);
    if (block.retry_at && block.retry_at > Date.now()) {
      const left = Math.ceil((block.retry_at - Date.now()) / 1000);
      if (block.kind === 'limit' && block.window === 'daily') {
        return "Today's limit reached. Try again in " + formatWait(left) + '.';
      }
      if (block.kind === 'limit') {
        return 'Hourly limit reached. Try again in ' + formatWait(left) + '.';
      }
      if (block.kind === 'pool') {
        return 'Ahrefs account cooling down. Try again in ' + formatWait(left) + '.';
      }
    }
    if (block.kind === 'pool') return SUB;
    return '';
  }

  function message() {
    const b = blockMessage(live.block);
    if (b) return b;
    if (!live.ok) return humanizeAuthError(live.error);
    return SUB;
  }

  function rewrite() {
    const msg = message();
    if (!msg) return;
    const nodes = document.querySelectorAll('div, span, p');
    for (let i = 0; i < nodes.length; i++) {
      const el = nodes[i];
      if (replaced.has(el)) {
        if (el.textContent !== msg) el.textContent = msg;
        continue;
      }
      if (el.childElementCount > 0) continue;
      const t = (el.textContent || '').trim();
      if (t === GENERIC || t === USAGE) {
        el.textContent = msg;
        replaced.add(el);
      }
    }
  }

  function load() {
    try {
      chrome.storage.local.get(
        ['tm_bar_authed', 'tm_bar_error', 'tm_bar_block'],
        (st) => {
          live = {
            ok: !!(st && st.tm_bar_authed),
            error: (st && st.tm_bar_error) || '',
            block: (st && st.tm_bar_block) || null,
          };
          if (live.block && live.block.retry_at && live.block.retry_at <= Date.now()) {
            live.block = null;
          }
          rewrite();
        },
      );
    } catch (_) {}
  }

  chrome.storage.onChanged.addListener((changes, area) => {
    if (area !== 'local') return;
    if (changes.tm_bar_block || changes.tm_bar_authed || changes.tm_bar_error) load();
  });

  const mo = new MutationObserver(() => rewrite());
  mo.observe(document.documentElement, { childList: true, subtree: true, characterData: true });
  load();
  setInterval(rewrite, 1500);
})();
