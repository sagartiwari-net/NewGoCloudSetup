// Lock Helium Amazon/Walmart account switcher + show panel username.
(function () {
  "use strict";
  if (window.__tmH10AccountLock) return;
  window.__tmH10AccountLock = true;

  var PROXY_ORIGIN = "__H10_PROXY_ORIGIN__";
  var panelUser = "";
  var ACCOUNT_ID_RE = /^\d{8,12}$/;

  function fetchUsername() {
    if (!PROXY_ORIGIN || PROXY_ORIGIN.indexOf("__H10_") === 0) {
      return Promise.resolve("");
    }
    return new Promise(function (resolve) {
      try {
        chrome.runtime.sendMessage({ type: "tm_panel_username" }, function (res) {
          if (chrome.runtime.lastError) {
            resolve("");
            return;
          }
          resolve(res && res.username ? String(res.username).trim() : "");
        });
      } catch (e) {
        resolve("");
      }
    });
  }

  function walkRoots(root, visit) {
    if (!root) return;
    visit(root);
    var all = [];
    try {
      all = root.querySelectorAll("*");
    } catch (e) {
      return;
    }
    for (var i = 0; i < all.length; i++) {
      if (all[i].shadowRoot) walkRoots(all[i].shadowRoot, visit);
    }
  }

  function textOf(el) {
    try {
      return (el.innerText || el.textContent || "").replace(/\s+/g, " ").trim();
    } catch (e) {
      return "";
    }
  }

  function looksLikeAccountSwitcher(el) {
    if (!el || el.nodeType !== 1) return false;
    var t = textOf(el);
    if (!t || t.length > 80) return false;
    // e.g. "ALS" + "1543036265" or dropdown rows "extra42" / id
    var parts = t.split(" ").filter(Boolean);
    var hasId = parts.some(function (p) {
      return ACCOUNT_ID_RE.test(p);
    });
    if (hasId && parts.length <= 4) return true;
    // Compact trigger: short label + chevron, near Helium root
    if (t.length <= 24 && /[∨▾▼˅]|chevron/i.test(el.innerHTML || "")) {
      var idNear = false;
      try {
        var sib = el.parentElement;
        if (sib && ACCOUNT_ID_RE.test(textOf(sib).split(/\s+/).pop() || "")) idNear = true;
      } catch (e) {}
      if (idNear) return true;
    }
    return false;
  }

  function lockEl(el) {
    if (!el || el.dataset.tmH10Locked === "1") return;
    el.dataset.tmH10Locked = "1";
    el.style.setProperty("pointer-events", "none", "important");
    el.style.setProperty("cursor", "default", "important");
    el.setAttribute("aria-disabled", "true");
    el.addEventListener(
      "click",
      function (e) {
        e.preventDefault();
        e.stopPropagation();
        if (e.stopImmediatePropagation) e.stopImmediatePropagation();
      },
      true
    );
  }

  function replaceLabel(el) {
    if (!panelUser || !el) return;
    // Prefer leaf text nodes / small labels, not whole menus
    var nodes = [];
    try {
      nodes = el.querySelectorAll("span,div,button,p,label");
    } catch (e) {}
    var touched = false;
    for (var i = 0; i < nodes.length; i++) {
      var n = nodes[i];
      if (n.children && n.children.length > 0) continue;
      var t = textOf(n);
      if (!t || ACCOUNT_ID_RE.test(t)) continue;
      if (t === panelUser) {
        touched = true;
        continue;
      }
      // Short account nicknames like ALS / extra42
      if (t.length <= 32 && !/account|seller|amazon|walmart|login|sign/i.test(t)) {
        n.textContent = panelUser;
        touched = true;
      }
    }
    if (!touched && textOf(el).length <= 40) {
      try {
        el.childNodes.forEach(function (c) {
          if (c.nodeType === 3) {
            var v = String(c.nodeValue || "").trim();
            if (v && !ACCOUNT_ID_RE.test(v) && v.length <= 32) {
              c.nodeValue = panelUser;
            }
          }
        });
      } catch (e) {}
    }
  }

  function scan() {
    walkRoots(document, function (root) {
      var candidates = [];
      try {
        candidates = root.querySelectorAll("div,button,span,li,[role='button'],[role='listbox'],[role='option']");
      } catch (e) {
        return;
      }
      for (var i = 0; i < candidates.length; i++) {
        var el = candidates[i];
        if (!looksLikeAccountSwitcher(el)) continue;
        lockEl(el);
        replaceLabel(el);
        // Also lock parent clickable chip (1–2 levels)
        var p = el.parentElement;
        for (var d = 0; d < 2 && p; d++) {
          if (looksLikeAccountSwitcher(p) || textOf(p).length <= 48) {
            lockEl(p);
            replaceLabel(p);
          }
          p = p.parentElement;
        }
      }
    });
  }

  // Capture-phase: never open account menus
  document.addEventListener(
    "click",
    function (e) {
      var t = e.target;
      if (!t || !t.closest) return;
      var hit = t.closest("div,button,span,li");
      if (!hit) return;
      // Walk composed path for shadow DOM
      var path = typeof e.composedPath === "function" ? e.composedPath() : [];
      for (var i = 0; i < path.length; i++) {
        var node = path[i];
        if (node && node.dataset && node.dataset.tmH10Locked === "1") {
          e.preventDefault();
          e.stopPropagation();
          if (e.stopImmediatePropagation) e.stopImmediatePropagation();
          return;
        }
        if (node && looksLikeAccountSwitcher(node)) {
          e.preventDefault();
          e.stopPropagation();
          if (e.stopImmediatePropagation) e.stopImmediatePropagation();
          lockEl(node);
          replaceLabel(node);
          return;
        }
      }
    },
    true
  );

  fetchUsername().then(function (u) {
    panelUser = u || "";
    scan();
    setInterval(scan, 1500);
    try {
      new MutationObserver(function () {
        scan();
      }).observe(document.documentElement, { childList: true, subtree: true });
    } catch (e) {}
  });
})();
