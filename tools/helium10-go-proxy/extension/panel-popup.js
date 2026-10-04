// Panel access gate for the Helium 10 popup.
// Shows a clear message instead of blank → login flicker when ct_session is missing.
(function () {
  "use strict";

  var PROXY_ORIGIN = "__H10_PROXY_ORIGIN__";
  var MSG =
    "To use the Helium 10 extension, please access Helium 10 from the panel first.";
  var overlay = null;
  var usernameEl = null;

  function ensureOverlay() {
    if (overlay && overlay.isConnected) return overlay;
    overlay = document.createElement("div");
    overlay.id = "tm-h10-panel-gate";
    overlay.setAttribute("data-tm-h10-gate", "1");
    overlay.style.cssText =
      "position:fixed;inset:0;z-index:2147483646;display:flex;align-items:center;justify-content:center;" +
      "padding:24px;background:#eef3f8;font-family:system-ui,-apple-system,Segoe UI,sans-serif;";
    overlay.innerHTML =
      '<div style="width:min(340px,100%);background:#fff;border-radius:22px;box-shadow:0 18px 50px rgba(15,23,42,.1);padding:32px 24px;text-align:center;">' +
      '<div style="width:64px;height:64px;margin:0 auto 16px;border-radius:50%;background:#e0e7ff;display:grid;place-items:center;font-size:26px;">🔒</div>' +
      '<h1 style="margin:0 0 10px;font-size:18px;font-weight:800;color:#0f172a;letter-spacing:-.02em;">Panel access required</h1>' +
      '<p style="margin:0;color:#64748b;font-size:14px;line-height:1.55;" data-tm-msg></p>' +
      '<p style="margin:16px 0 0;color:#94a3b8;font-size:12px;line-height:1.45;">Open Helium 10 from your member panel, then reopen this extension.</p>' +
      "</div>";
    var msg = overlay.querySelector("[data-tm-msg]");
    if (msg) msg.textContent = MSG;
    document.documentElement.appendChild(overlay);
    return overlay;
  }

  function showGate() {
    ensureOverlay().style.display = "flex";
    try {
      var root = document.getElementById("popup-root");
      if (root) root.style.visibility = "hidden";
    } catch (e) {}
  }

  function hideGate() {
    if (overlay) overlay.style.display = "none";
    try {
      var root = document.getElementById("popup-root");
      if (root) root.style.visibility = "visible";
    } catch (e) {}
  }

  function ensureUsernameBar(name) {
    if (!name) return;
    if (!usernameEl || !usernameEl.isConnected) {
      usernameEl = document.createElement("div");
      usernameEl.id = "tm-h10-userbar";
      usernameEl.style.cssText =
        "position:fixed;left:0;right:0;top:0;z-index:2147483645;background:#0f172a;color:#fff;" +
        "font:600 12px/1.2 system-ui,-apple-system,Segoe UI,sans-serif;padding:8px 12px;text-align:center;";
      document.documentElement.appendChild(usernameEl);
      try {
        document.body.style.paddingTop = "32px";
      } catch (e) {}
    }
    usernameEl.textContent = "Signed in as " + name;
  }

  function scrubLoginChrome() {
    try {
      var root = document.getElementById("popup-root") || document.body;
      if (!root) return;
      root.querySelectorAll("a,button").forEach(function (el) {
        var t = (el.textContent || "").replace(/\s+/g, " ").trim();
        if (
          /^Log into My Account$/i.test(t) ||
          /^Sign up for Free$/i.test(t) ||
          /^My Dashboard$/i.test(t) ||
          /^Learn how to use/i.test(t)
        ) {
          el.style.setProperty("display", "none", "important");
          el.style.setProperty("pointer-events", "none", "important");
        }
      });
      root.querySelectorAll("p,h1,h2,div,span").forEach(function (el) {
        if (el.children && el.children.length > 2) return;
        var t = (el.textContent || "").replace(/\s+/g, " ").trim();
        if (/Please log in to launch the extension tools/i.test(t)) {
          el.textContent = MSG;
        }
        if (/Welcome to the Helium 10 Chrome Extension/i.test(t)) {
          el.textContent = "Panel access required";
        }
      });
    } catch (e) {}
  }

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

  // Paint gate immediately so users never stare at a blank white popup.
  showGate();

  function boot() {
    fetchUsername().then(function (user) {
      if (user) {
        hideGate();
        ensureUsernameBar(user);
        scrubLoginChrome();
        try {
          new MutationObserver(function () {
            scrubLoginChrome();
            ensureUsernameBar(user);
          }).observe(document.documentElement, { childList: true, subtree: true });
        } catch (e) {}
        return;
      }
      showGate();
      scrubLoginChrome();
      try {
        new MutationObserver(function () {
          showGate();
          scrubLoginChrome();
        }).observe(document.documentElement, { childList: true, subtree: true });
      } catch (e) {}
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", boot);
  } else {
    boot();
  }
})();
