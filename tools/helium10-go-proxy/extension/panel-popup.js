// Lightweight panel gate — static HTML in popup.html does the first paint.
(function () {
  "use strict";

  var PROXY_ORIGIN = "__H10_PROXY_ORIGIN__";
  var MSG =
    "To use the Helium 10 extension, please access Helium 10 from the panel first.";

  function $(id) {
    return document.getElementById(id);
  }

  function setSignedIn(user) {
    document.body.classList.add("tm-ready");
    var bar = $("tm-h10-userbar");
    if (bar) bar.textContent = "Signed in as " + user;
  }

  function stayOnGate() {
    document.body.classList.remove("tm-ready");
    var gate = $("tm-h10-panel-gate");
    if (gate) {
      var p = gate.querySelector(".card p");
      if (p) p.textContent = MSG;
    }
  }

  function scrubLoginChrome() {
    try {
      var root = $("popup-root");
      if (!root) return;
      root.querySelectorAll("a,button").forEach(function (el) {
        var t = (el.textContent || "").replace(/\s+/g, " ").trim();
        if (
          /^Log into My Account$/i.test(t) ||
          /^Sign up for Free$/i.test(t) ||
          /^My Dashboard$/i.test(t)
        ) {
          el.style.setProperty("display", "none", "important");
        }
      });
    } catch (e) {}
  }

  function fetchUsername() {
    return new Promise(function (resolve) {
      var done = false;
      function finish(v) {
        if (done) return;
        done = true;
        resolve(v || "");
      }
      // Never hang the popup if the service worker is asleep/broken.
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

  function boot() {
    stayOnGate();
    fetchUsername().then(function (user) {
      if (user) {
        setSignedIn(user);
        scrubLoginChrome();
        var t = setInterval(scrubLoginChrome, 1000);
        setTimeout(function () {
          clearInterval(t);
        }, 15000);
        return;
      }
      stayOnGate();
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", boot);
  } else {
    boot();
  }
})();
