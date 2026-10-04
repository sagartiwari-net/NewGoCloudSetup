// Light account-switcher lock — avoid heavy full-DOM scans that freeze Amazon tabs.
(function () {
  "use strict";
  if (window.__tmH10AccountLock) return;
  window.__tmH10AccountLock = true;

  var PROXY_ORIGIN = "__H10_PROXY_ORIGIN__";
  var panelUser = "";
  var ACCOUNT_ID_RE = /^\d{8,12}$/;
  var scanning = false;

  function fetchUsername() {
    return new Promise(function (resolve) {
      var done = false;
      function finish(v) {
        if (done) return;
        done = true;
        resolve(v || "");
      }
      setTimeout(function () {
        finish("");
      }, 2500);
      if (!PROXY_ORIGIN || PROXY_ORIGIN.indexOf("__H10_") === 0) {
        finish("");
        return;
      }
      try {
        chrome.runtime.sendMessage({ type: "tm_panel_username" }, function (res) {
          if (chrome.runtime.lastError) {
            finish("");
            return;
          }
          finish(res && res.username ? String(res.username).trim() : "");
        });
      } catch (e) {
        finish("");
      }
    });
  }

  function textOf(el) {
    try {
      return (el.innerText || el.textContent || "").replace(/\s+/g, " ").trim();
    } catch (e) {
      return "";
    }
  }

  function lockAndLabel(el) {
    if (!el || el.dataset.tmH10Locked === "1") {
      if (el && panelUser) label(el);
      return;
    }
    el.dataset.tmH10Locked = "1";
    el.style.setProperty("pointer-events", "none", "important");
    el.style.setProperty("cursor", "default", "important");
    label(el);
  }

  function label(el) {
    if (!panelUser || !el) return;
    var nodes;
    try {
      nodes = el.querySelectorAll("span,div,button,p");
    } catch (e) {
      return;
    }
    for (var i = 0; i < nodes.length; i++) {
      var n = nodes[i];
      if (n.children && n.children.length) continue;
      var t = textOf(n);
      if (!t || ACCOUNT_ID_RE.test(t) || t === panelUser) continue;
      if (t.length <= 32) n.textContent = panelUser;
    }
  }

  function scanRoot(root) {
    if (!root || !root.querySelectorAll) return;
    var list;
    try {
      // Helium mounts under these hosts / ids — keep query narrow.
      list = root.querySelectorAll(
        "#h10-style-container, [id^='h10-'], [class*='h10'], [data-h10]"
      );
    } catch (e) {
      return;
    }
    for (var i = 0; i < list.length; i++) {
      var host = list[i];
      var scope = host.shadowRoot || host;
      var candidates;
      try {
        candidates = scope.querySelectorAll("button,div,span,li,[role='button']");
      } catch (e) {
        continue;
      }
      // Cap work per host
      var max = Math.min(candidates.length, 80);
      for (var j = 0; j < max; j++) {
        var el = candidates[j];
        var t = textOf(el);
        if (!t || t.length > 64) continue;
        var parts = t.split(" ");
        var hasId = false;
        for (var k = 0; k < parts.length; k++) {
          if (ACCOUNT_ID_RE.test(parts[k])) {
            hasId = true;
            break;
          }
        }
        if (hasId) lockAndLabel(el.parentElement || el);
      }
      if (host.shadowRoot) {
        // one level only
      }
    }
  }

  function scan() {
    if (scanning) return;
    scanning = true;
    try {
      scanRoot(document);
      var all = document.querySelectorAll("*");
      var limit = Math.min(all.length, 400);
      for (var i = 0; i < limit; i++) {
        if (all[i].shadowRoot) scanRoot(all[i].shadowRoot);
      }
    } catch (e) {}
    scanning = false;
  }

  document.addEventListener(
    "click",
    function (e) {
      var path = typeof e.composedPath === "function" ? e.composedPath() : [];
      for (var i = 0; i < path.length; i++) {
        var node = path[i];
        if (!node || !node.dataset) continue;
        if (node.dataset.tmH10Locked === "1") {
          e.preventDefault();
          e.stopPropagation();
          return;
        }
      }
    },
    true
  );

  fetchUsername().then(function (u) {
    panelUser = u;
    scan();
    setInterval(scan, 4000);
  });
})();
