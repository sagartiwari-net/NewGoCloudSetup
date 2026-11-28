/**
 * tm-serp-panel-fix.js — keep official Keywords ideas panel visible.
 *
 * Markhor leaves #ah-tb-serp-side-panel__wrapper where React mounted it.
 * We used to appendChild() it into a dock — that detaches React's node so
 * W6/tbGetKeywordStatsV3 updates never paint Volume/CPC/KD (list shows,
 * metrics blank / stuck). Fix: CSS-only positioning, never move the node.
 *
 * Soft nav: stay alive on google.com homepage so first /search?q= still works.
 */
(function tmSerpPanelFix() {
  if (!/google\./i.test(location.hostname)) return;

  const WRAP_ID = "ah-tb-serp-side-panel__wrapper";
  const STYLE_ID = "tm-serp-panel-fix-css";

  function isSearchPage() {
    try {
      if (/[?&]q=/.test(location.search)) return true;
      if (/\/search/i.test(location.pathname)) return true;
      return false;
    } catch (_) {
      return false;
    }
  }

  function ensureCss() {
    if (document.getElementById(STYLE_ID)) return;
    const s = document.createElement("style");
    s.id = STYLE_ID;
    // High z-index + fixed right rail — do NOT change parentage of the wrapper.
    s.textContent = [
      "#" + WRAP_ID + "{",
      "position:fixed!important;top:112px!important;right:12px!important;left:auto!important;",
      "width:min(380px,calc(100vw - 24px))!important;max-height:calc(100vh - 130px)!important;",
      "overflow:auto!important;z-index:2147483000!important;",
      "margin:0!important;transform:none!important;",
      "}",
      "#" + WRAP_ID + " .ah-tb-serp-side-panel,",
      "#" + WRAP_ID + " .ah-tb-serp-side-panel__r,",
      "#" + WRAP_ID + " .ah-tb-serp-side-panel__l{",
      "position:relative!important;left:0!important;right:auto!important;margin:0!important;",
      "width:100%!important;max-width:100%!important;",
      "}",
      /* Kill leftover dock from older builds if still in DOM */
      "#tm-serp-panel-dock{display:none!important;}",
      "#tm-kw-fallback-panel{display:none!important;}",
    ].join("");
    (document.head || document.documentElement).appendChild(s);
  }

  function tick() {
    if (!isSearchPage()) return;
    ensureCss();
    // Remove old dock wrapper if a prior build moved the panel — put it back
    // under body so React's next update still finds a connected tree, then
    // leave it alone (CSS positions it).
    const dock = document.getElementById("tm-serp-panel-dock");
    const wrap = document.getElementById(WRAP_ID);
    if (dock && wrap && wrap.parentElement === dock) {
      try {
        (document.body || document.documentElement).appendChild(wrap);
      } catch (_) {}
    }
    if (dock && !dock.querySelector("#" + WRAP_ID)) {
      try {
        dock.remove();
      } catch (_) {}
    }
    const fb = document.getElementById("tm-kw-fallback-panel");
    if (fb) {
      try {
        fb.remove();
      } catch (_) {}
    }
  }

  tick();
  setInterval(tick, 800);

  try {
    new MutationObserver(function () {
      tick();
    }).observe(document.documentElement || document, { childList: true, subtree: true });
  } catch (_) {}

  window.addEventListener("popstate", tick, true);
  try {
    const wrapHist = function (fn) {
      return function () {
        const ret = fn.apply(this, arguments);
        setTimeout(tick, 0);
        setTimeout(tick, 500);
        return ret;
      };
    };
    history.pushState = wrapHist(history.pushState);
    history.replaceState = wrapHist(history.replaceState);
  } catch (_) {}
})();
