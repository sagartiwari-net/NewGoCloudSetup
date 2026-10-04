(function() {
  var PROXY_HOST = window.location.host;
  var PROXY_SCHEME = window.location.protocol;
  var PROXY_ORIGIN = window.location.origin;
  var TARGET_USER_AGENT = "__DYNAMIC_USER_AGENT__";

  // Global user limit cache to block downloads pre-emptively on client side
  window.tm_user_limits = null;

  // After login, warm session on / first then navigate to Photos (avoids Cloudflare block on cold /photos).
  function redirectRootToPhotos() {
    var path = window.location.pathname;
    if (path !== '/' && path !== '') return;
    if (document.body && (document.body.innerText || '').indexOf('Sorry, you have been blocked') !== -1) return;

    setTimeout(function() {
      if (window.location.pathname === '/' || window.location.pathname === '') {
        window.location.replace('/photos');
      }
    }, 50);
  }

  redirectRootToPhotos();

  function jsIsDownloadRequest(url) {
      if (!url || typeof url !== 'string') return false;
      var lower = url.toLowerCase();
      if (lower.indexOf('preview') !== -1 || lower.indexOf('my-downloads') !== -1 || lower.indexOf('my_downloads') !== -1) {
          return false;
      }
      return lower.indexOf('download') !== -1 ||
             lower.indexOf('license') !== -1 ||
             lower.indexOf('.zip') !== -1 ||
             lower.indexOf('.wav') !== -1 ||
             lower.indexOf('.mp3') !== -1;
  }

  // Override navigator.userAgent to match our proxy's User-Agent so Cloudflare/Envato JS checks don't fail
  try {
    if (TARGET_USER_AGENT && TARGET_USER_AGENT !== "__DYNAMIC_USER_AGENT__") {
      Object.defineProperty(navigator, 'userAgent', {
        get: function() { return TARGET_USER_AGENT; }
      });
      Object.defineProperty(navigator, 'appVersion', {
        get: function() { return TARGET_USER_AGENT.replace(/^Mozilla\//, ''); }
      });

      // Parse Chrome version for userAgentData mock
      var chromeMatch = TARGET_USER_AGENT.match(/Chrome\/(\d+)/);
      var chromeVer = chromeMatch ? chromeMatch[1] : "124";
      var platform = TARGET_USER_AGENT.indexOf("Mac OS X") !== -1 ? "macOS" : "Windows";

      if (navigator.userAgentData) {
        Object.defineProperty(navigator, 'userAgentData', {
          get: function() {
            return {
              brands: [
                { brand: "Chromium", version: chromeVer },
                { brand: "Google Chrome", version: chromeVer },
                { brand: "Not-A.Brand", version: "99" }
              ],
              mobile: false,
              platform: platform
            };
          }
        });
      }

      // Mock window.chrome if missing (e.g., when user browses via Firefox but proxy sets Chrome User-Agent)
      if (!window.chrome && TARGET_USER_AGENT.indexOf("Chrome") !== -1) {
        window.chrome = {
          app: { isInstalled: false },
          runtime: {},
          loadTimes: function() {}
        };
      }
    }
  } catch (e) {
    console.error("Failed to override navigator.userAgent", e);
  }

  // Identify primary host (default to app.envato.com or elements.envato.com based on configuration)
  var isAppProxyPath = window.location.pathname.startsWith('/app-proxy');
  var isElementsProxyPath = window.location.pathname.startsWith('/elements-proxy');
  
  // Injected dynamically by the proxy server from config.json target_url
  var primaryHost = '__PRIMARY_HOST__';

  function mapUrl(u) {
    if (!u || typeof u !== 'string') return u;
    try {
      // Resolve relative URL against current document location
      var url = new URL(u, window.location.href);
      var h = url.hostname.toLowerCase();

      // If it's already using our proxy host, ensure the scheme matches the current page (HTTP vs HTTPS)
      if (h === PROXY_HOST.split(':')[0]) {
        if (url.protocol !== PROXY_SCHEME) {
          return PROXY_ORIGIN + url.pathname + url.search + url.hash;
        }
        return u;
      }

      // If it's not an Envato domain, do not touch it
      if (!h.endsWith('envato.com') && !h.endsWith('envatousercontent.com')) {
        return u;
      }

      var pathWithSearch = url.pathname + url.search + url.hash;

      // Map subdomains to their proxy prefixes
      if (h === 'app.envato.com') {
        if (primaryHost === 'app.envato.com') {
          return PROXY_ORIGIN + pathWithSearch;
        } else {
          return PROXY_ORIGIN + '/app-proxy' + pathWithSearch;
        }
      }
      if (h === 'elements.envato.com') {
        if (primaryHost === 'elements.envato.com') {
          return PROXY_ORIGIN + pathWithSearch;
        } else {
          return PROXY_ORIGIN + '/elements-proxy' + pathWithSearch;
        }
      }
      if (h === 'api.envato.com') {
        return PROXY_ORIGIN + '/api-proxy' + pathWithSearch;
      }
      if (h === 'account.envato.com') {
        return PROXY_ORIGIN + '/account-proxy' + pathWithSearch;
      }
      if (h === 'assets.envato.com' || h === 'assets.elements.envato.com') {
        return PROXY_ORIGIN + '/assets-proxy' + pathWithSearch;
      }
      if (h === 'elements-resized.envatousercontent.com') {
        return PROXY_ORIGIN + '/__resized__' + pathWithSearch;
      }
      if (h === 'video-previews.elements.envatousercontent.com') {
        return PROXY_ORIGIN + '/__video__' + pathWithSearch;
      }

      // Fallback for any other envato domains (like downloads)
      return PROXY_ORIGIN + '/__domain__/' + h + pathWithSearch;
    } catch (e) {
      return u;
    }
  }

  try {
    var mediaSrc = Object.getOwnPropertyDescriptor(HTMLMediaElement.prototype, "src");
    if (mediaSrc && mediaSrc.set) {
      Object.defineProperty(HTMLMediaElement.prototype, "src", {
        configurable: true,
        get: mediaSrc.get,
        set: function(value) {
          var next = typeof value === "string" ? mapUrl(value) : value;
          return mediaSrc.set.call(this, next);
        }
      });
    }
    var OrigAudio = window.Audio;
    window.Audio = function(src) {
      if (typeof src === "string") src = mapUrl(src);
      return new OrigAudio(src);
    };
    window.Audio.prototype = OrigAudio.prototype;
  } catch (e) {}

  // Intercept window.fetch safely
  try {
    var _fetch = window.fetch;
    window.fetch = function(input, init) {
      var fetchUrl = typeof input === 'string' ? input : (input && input.url ? input.url : '');

      // MOCK refresh_id_token to prevent the SPA from reloading the page when it fails!
      // Return a fake successful response so the frontend proceeds with the download/action.
      if (fetchUrl.indexOf('refresh_id_token') !== -1 || fetchUrl.indexOf('session/refresh') !== -1) {
         console.warn("Mocked refresh_id_token request to prevent reload loop and unblock downloads!");
         return Promise.resolve(new Response(JSON.stringify({ success: true, valid: true }), {
           status: 200,
           headers: { 'Content-Type': 'application/json' }
         }));
      }

      // Pre-emptive Client-side Quota Check for download requests
      if (jsIsDownloadRequest(fetchUrl)) {
        if (window.tm_user_limits && window.tm_user_limits.credit_used >= window.tm_user_limits.credit_limit) {
          console.warn("[TM-WIDGET] Client-side download blocked because daily limit is reached!");
          showLimitReachedPopup();
          return Promise.resolve(new Response(JSON.stringify({ success: false, error: "limit_reached" }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' }
          }));
        }
      }

      if (typeof input === 'string') {
        input = mapUrl(input);
      } else if (input && input.url) {
        try {
          var newUrl = mapUrl(input.url);
          input = new Request(newUrl, input);
        } catch (e) {}
      }

      try {
        var deviceFp = localStorage.getItem("tm_device_fp") || sessionStorage.getItem("tm_device_fp") || "";
        var deviceProof = localStorage.getItem("tm_device_proof") || "";
        var sameOrigin = new URL(typeof input === "string" ? input : input.url, window.location.href).origin === window.location.origin;
          if (deviceFp && deviceProof && sameOrigin) {
            init = init || {};
            var deviceHeaders = new Headers(init.headers || (typeof input !== "string" && input.headers) || undefined);
            if (!deviceHeaders.get("X-Device-Fp")) deviceHeaders.set("X-Device-Fp", deviceFp);
            if (!deviceHeaders.get("X-Device-Proof")) deviceHeaders.set("X-Device-Proof", deviceProof);
            init.headers = deviceHeaders;
          }
      } catch (e) {}

      return _fetch(input, init).then(function(response) {
        // Sync limits after successful download
        if (response.status === 200 && jsIsDownloadRequest(fetchUrl)) {
          if (window.tm_refresh_quota_data) {
            setTimeout(window.tm_refresh_quota_data, 1000);
          }
        }

        if (response.status === 403) {
          return response.clone().text().then(function(text) {
            if (text.indexOf('daily_download_limit_reached') !== -1 || text.indexOf('Download Limit Reached') !== -1) {
              showLimitReachedPopup();
              // Return a fake successful empty response to keep the React SPA happy and prevent the brown alert box!
              return new Response(JSON.stringify({ success: false, error: "limit_reached" }), {
                status: 200,
                headers: { 'Content-Type': 'application/json' }
              });
            }
            return response;
          }).catch(function() {
            return response;
          });
        }
        return response;
      });
    };
  } catch (e) {
    console.error("Failed to override fetch:", e);
  }

  // Intercept window.open safely (often used for downloads)
  try {
    var _windowOpen = window.open;
    window.open = function(url, name, specs) {
      if (typeof url === 'string') {
        url = mapUrl(url);
      }
      return _windowOpen.call(window, url, name, specs);
    };
  } catch (e) {
    console.error("Failed to override window.open:", e);
  }

  // Intercept XMLHttpRequest safely
  try {
    var _open = XMLHttpRequest.prototype.open;
    var _send = XMLHttpRequest.prototype.send;
    XMLHttpRequest.prototype.open = function(method, url, async, user, pass) {
      this._interceptedUrl = url;
      if (typeof url === 'string') {
        url = mapUrl(url);
      }
      return _open.call(this, method, url, async !== false, user, pass);
    };
    XMLHttpRequest.prototype.send = function() {
      var self = this;
      if (this._interceptedUrl && (this._interceptedUrl.indexOf('refresh_id_token') !== -1 || this._interceptedUrl.indexOf('session/refresh') !== -1)) {
         console.warn("Blocked XHR refresh_id_token request to prevent reload loop!");
         return; // Do nothing, keep request pending forever
      }

      // Pre-emptive Client-side Quota Check for XHR download requests
      if (this._interceptedUrl && jsIsDownloadRequest(this._interceptedUrl)) {
        if (window.tm_user_limits && window.tm_user_limits.credit_used >= window.tm_user_limits.credit_limit) {
          console.warn("[TM-WIDGET] Client-side XHR download blocked because daily limit is reached!");
          showLimitReachedPopup();
          // Mock XHR status to 200 success response locally without sending to server
          setTimeout(function() {
            try {
              Object.defineProperty(self, 'readyState', { get: function() { return 4; } });
              Object.defineProperty(self, 'status', { get: function() { return 200; } });
              Object.defineProperty(self, 'responseText', { get: function() { return JSON.stringify({ success: false, error: "limit_reached" }); } });
              if (self.onreadystatechange) {
                self.onreadystatechange();
              }
            } catch(e) {}
          }, 50);
          return;
        }
      }

      var originalOnReadyStateChange = this.onreadystatechange;
      this.onreadystatechange = function() {
        // Sync limits after successful download
        if (self.readyState === 4 && self.status === 200 && self._interceptedUrl && jsIsDownloadRequest(self._interceptedUrl)) {
          if (window.tm_refresh_quota_data) {
            setTimeout(window.tm_refresh_quota_data, 1000);
          }
        }

        if (self.readyState === 4 && self.status === 403) {
          var text = self.responseText || "";
          if (text.indexOf('daily_download_limit_reached') !== -1 || text.indexOf('Download Limit Reached') !== -1) {
            showLimitReachedPopup();
            // Mock XHR status to 200 to prevent React from showing the brown alert!
            try {
              Object.defineProperty(self, 'status', { get: function() { return 200; } });
              Object.defineProperty(self, 'responseText', { get: function() { return JSON.stringify({ success: false, error: "limit_reached" }); } });
            } catch (e) {}
          }
        }
        if (originalOnReadyStateChange) {
          return originalOnReadyStateChange.apply(this, arguments);
        }
      };

      return _send.apply(this, arguments);
    }
  } catch (e) {
    console.error("Failed to override XMLHttpRequest.open:", e);
  }

  // Intercept Location replace and assign safely using try-catch to prevent TypeError crashes in Chrome
  try {
    var _locationReplace = window.location.replace;
    var _locationAssign = window.location.assign;
    var _locationReload = window.location.reload;

    function handleLocationChange(url, originalFunc) {
      console.warn("Proxy intercepted location change to: ", url);
      if (!url) return;

      var mapped = url;
      if (typeof url === 'string' && (url.indexOf('envato.com') !== -1 || url.startsWith('/'))) {
        mapped = mapUrl(url);
      }

      var targetHref;
      try {
        targetHref = new URL(mapped, window.location.href).href;
      } catch (e) {
        targetHref = mapped;
      }

      // Prevent rapid refresh loops to the exact same page
      if (targetHref === window.location.href || targetHref === window.location.href + '/' || window.location.href === targetHref + '/') {
        console.error("BLOCKED INFINITE RELOAD LOOP TO: ", url);
        return;
      }

      return originalFunc.call(window.location, mapped);
    }

    if (_locationReplace) {
      window.location.replace = function(url) {
        return handleLocationChange(url, _locationReplace);
      };
    }
    if (_locationAssign) {
      window.location.assign = function(url) {
        return handleLocationChange(url, _locationAssign);
      };
    }
    if (_locationReload) {
      window.location.reload = function() {
        console.error("BLOCKED WINDOW.LOCATION.RELOAD() to prevent refresh loops.");
        return;
      };
    }
  } catch (e) {
    console.warn("Chrome blocked window.location overrides, skipping safely...");
  }

  // Intercept window.location.href setter via Location prototype (covers SPA redirects)
  try {
    var _locHrefDesc = Object.getOwnPropertyDescriptor(Location.prototype, 'href');
    if (_locHrefDesc && _locHrefDesc.set) {
      var _origHrefSet = _locHrefDesc.set;
      Object.defineProperty(Location.prototype, 'href', {
        get: _locHrefDesc.get,
        set: function(url) {
          console.warn('[PROXY FIXER] Intercepted Location.href setter:', url);
          var mapped = (typeof url === 'string') ? mapUrl(url) : url;
          return _origHrefSet.call(this, mapped);
        },
        configurable: true
      });
    }
  } catch (e) {
    console.warn('[PROXY FIXER] Could not intercept Location.prototype.href:', e);
  }

  // Intercept History API — SPAs (like Envato Elements React) use pushState/replaceState for routing
  try {
    var _pushState = window.history.pushState;
    var _replaceState = window.history.replaceState;
    if (_pushState) {
      window.history.pushState = function(state, title, url) {
        if (url && typeof url === 'string') url = mapUrl(url);
        return _pushState.call(this, state, title, url);
      };
    }
    if (_replaceState) {
      window.history.replaceState = function(state, title, url) {
        if (url && typeof url === 'string') url = mapUrl(url);
        return _replaceState.call(this, state, title, url);
      };
    }
  } catch (e) {
    console.error('[PROXY FIXER] Failed to override history API:', e);
  }

  // Intercept dynamically added elements via MutationObserver
  function rewriteNode(el) {
    try {
      if (!el || !el.getAttribute) return;
      if (el.tagName === 'IMG' || el.tagName === 'SOURCE') {
        if (el.hasAttribute('crossorigin')) {
          el.removeAttribute('crossorigin');
        }
      }
      ['src', 'href', 'poster'].forEach(function(k) {
        var v = el.getAttribute(k);
        if (v) {
          var nv = mapUrl(v);
          if (nv !== v) el.setAttribute(k, nv);
        }
      });
      if (el.getAttribute('srcset')) {
        var s = el.getAttribute('srcset');
        var nv = s.split(',').map(function(part) {
          var t = part.trim();
          var sp = t.split(/\s+/, 2);
          if (sp[0]) sp[0] = mapUrl(sp[0]);
          return sp.join(' ');
        }).join(', ');
        if (nv !== s) el.setAttribute('srcset', nv);
      }
    } catch (e) {}
  }

  function initialRewrite() {
    document.querySelectorAll('[src],[href],[poster],[srcset]').forEach(rewriteNode);
  }

  function applyDisplayUsername() {
    try {
      var username = window.tm_user_limits && window.tm_user_limits.username;
      if (!username) return;

      var btn = document.querySelector('button[data-cy="profile-menu"]');
      if (!btn) return;

      // Show proxy username but keep Envato profile menu closed/non-interactive.
      if (!btn.dataset.tmProfileLocked) {
        btn.dataset.tmProfileLocked = '1';
        btn.setAttribute('aria-disabled', 'true');
        btn.setAttribute('aria-expanded', 'false');
        btn.style.pointerEvents = 'none';
        btn.style.cursor = 'default';
        btn.addEventListener('click', function(e) {
          e.preventDefault();
          e.stopPropagation();
          e.stopImmediatePropagation();
          return false;
        }, true);
      }

      // Update only the visible label span once (avoid parent+child double text).
      var labelHost = btn.querySelector('div[title]');
      if (labelHost) {
        labelHost.setAttribute('title', username);
        var labelSpan = labelHost.querySelector('span');
        if (labelSpan) {
          var current = (labelSpan.textContent || '').trim();
          if (current !== username) {
            labelSpan.textContent = username;
          }
        }
      }

      // Keep SVG title in sync for accessibility without creating duplicate visible text.
      btn.querySelectorAll('svg title').forEach(function(node) {
        if ((node.textContent || '').trim() !== username) {
          node.textContent = username;
        }
      });
    } catch (e) {
      console.warn('[PROXY FIXER] Failed to replace profile username:', e);
    }
  }

  try {
    var mo = new MutationObserver(function(muts) {
      muts.forEach(function(m) {
        if (m.type === 'attributes' && m.target) {
          rewriteNode(m.target);
        }
        (m.addedNodes || []).forEach(function(n) {
          if (n.nodeType === 1) {
            rewriteNode(n);
            if (n.querySelectorAll) {
              n.querySelectorAll('[src],[href],[poster],[srcset]').forEach(rewriteNode);
            }
          }
        });
      });
      applyDisplayUsername();
    });

    mo.observe(document.documentElement, {
      subtree: true,
      childList: true,
      attributes: true,
      attributeFilter: ['src', 'href', 'poster', 'srcset', 'crossorigin']
    });
  } catch (e) {
    console.error("MutationObserver init failed:", e);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initialRewrite);
  } else {
    initialRewrite();
  }

  // Client-side detection of login/sign-in pages (SPA-aware)
  function checkAndTriggerSwap() {
    // STRICT: Only detect login by actual DOM presence of the login form button.
    // Do NOT trigger on URL path (/sign_in etc.) because that URL persists after
    // reload, causing an infinite loop. Only trigger when the actual login form is visible.
    var hasLoginForm = false;

    // Check for specific login button elements in React DOM
    var loginBtn = document.getElementById('sso-forms__submit') ||
                   document.querySelector('[data-testid="submitButton"]');
    if (loginBtn && loginBtn.innerText && loginBtn.innerText.toLowerCase().indexOf("sign in") !== -1) {
      hasLoginForm = true;
    }

    // Secondary check: "Sign in to Envato App" heading rendered by React
    if (!hasLoginForm && document.body) {
      var headings = document.querySelectorAll('h1, h2, h3');
      for (var i = 0; i < headings.length; i++) {
        var ht = headings[i].innerText || '';
        if (ht.toLowerCase().indexOf('sign in to envato') !== -1) {
          hasLoginForm = true;
          break;
        }
      }
    }

    if (hasLoginForm) {
      // Prevent rapid duplicate trigger loop by checking sessionStorage
      var now = Date.now();
      var lastTrigger = sessionStorage.getItem('mandi_last_swap_trigger');
      if (lastTrigger && (now - parseInt(lastTrigger, 10) < 15000)) {
        return; // already triggered swap within last 15 seconds
      }
      sessionStorage.setItem('mandi_last_swap_trigger', now.toString());

      console.warn("[PROXY FIXER] 🔄 Login screen detected on client-side! Triggering dynamic session swap...");
      
      // Draw premium loader overlay instantly
      showPremiumOverlay();

      // Send POST request to trigger-swap endpoint
      fetch('/api/trigger-swap', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        }
      })
      .then(function(response) {
        return response.json();
      })
      .then(function(data) {
        if (data && data.success) {
          console.log("[PROXY FIXER] ✅ Session swapped to: " + data.account + "! Reloading page...");
          setTimeout(function() {
            // Use replace() so sign_in URL is removed from browser history
            // and can never be re-visited via back button
            window.location.replace('/photos');
          }, 1500);
        } else {
          console.error("[PROXY FIXER] ❌ Swap failed:", data ? data.error : 'unknown');
          // Hide overlay if it failed so user is not blocked forever
          var overlay = document.getElementById('mandi-swap-overlay');
          if (overlay) overlay.parentNode.removeChild(overlay);
        }
      })
      .catch(function(err) {
        console.error("[PROXY FIXER] ❌ Swap fetch error:", err);
        var overlay = document.getElementById('mandi-swap-overlay');
        if (overlay) overlay.parentNode.removeChild(overlay);
      });
    }
  }

  function showPremiumOverlay() {
    if (document.getElementById('mandi-swap-overlay')) return;
    var overlay = document.createElement('div');
    overlay.id = 'mandi-swap-overlay';
    overlay.style.position = 'fixed';
    overlay.style.top = '0';
    overlay.style.left = '0';
    overlay.style.width = '100vw';
    overlay.style.height = '100vh';
    overlay.style.zIndex = '9999999';
    overlay.style.background = 'radial-gradient(circle at center, #0f172a 0%, #020617 100%)';
    overlay.style.color = '#ffffff';
    overlay.style.display = 'flex';
    overlay.style.flexDirection = 'column';
    overlay.style.alignItems = 'center';
    overlay.style.justifyContent = 'center';
    overlay.style.fontFamily = "'Outfit', sans-serif";

    var style = document.createElement('style');
    style.innerHTML = `
      @import url('https://fonts.googleapis.com/css2?family=Outfit:wght@300;400;600;800&display=swap');
      @keyframes mandi-spin {
        0% { transform: rotate(0deg); }
        100% { transform: rotate(360deg); }
      }
      @keyframes mandi-spin-reverse {
        0% { transform: rotate(360deg); }
        100% { transform: rotate(0deg); }
      }
      @keyframes mandi-pulse {
        0%, 100% { transform: translate(-50%, -50%) scale(0.9); opacity: 0.5; }
        50% { transform: translate(-50%, -50%) scale(1.1); opacity: 1; }
      }
      .mandi-loader-wrapper {
        position: relative;
        width: 80px;
        height: 80px;
        margin-bottom: 30px;
      }
      .mandi-loader {
        width: 100%;
        height: 100%;
        border: 4px solid rgba(99, 102, 241, 0.1);
        border-top-color: #6366f1;
        border-radius: 50%;
        animation: mandi-spin 1s cubic-bezier(0.5, 0, 0.5, 1) infinite;
      }
      .mandi-loader-inner {
        position: absolute;
        top: 10px;
        left: 10px;
        right: 10px;
        bottom: 10px;
        border: 4px solid rgba(236, 72, 153, 0.1);
        border-top-color: #ec4899;
        border-radius: 50%;
        animation: mandi-spin-reverse 1.5s cubic-bezier(0.5, 0, 0.5, 1) infinite;
      }
      .mandi-glow {
        position: absolute;
        top: 50%;
        left: 50%;
        transform: translate(-50%, -50%);
        width: 120px;
        height: 120px;
        background: radial-gradient(circle, rgba(99, 102, 241, 0.15) 0%, rgba(99, 102, 241, 0) 70%);
        animation: mandi-pulse 2s ease-in-out infinite;
      }
      .mandi-title {
        font-size: 26px;
        font-weight: 800;
        margin-bottom: 12px;
        background: linear-gradient(135deg, #6366f1 0%, #ec4899 100%);
        -webkit-background-clip: text;
        -webkit-text-fill-color: transparent;
        letter-spacing: -0.5px;
      }
      .mandi-desc {
        font-size: 15px;
        color: #94a3b8;
        line-height: 1.6;
        font-weight: 400;
        text-align: center;
        max-width: 400px;
      }
    `;
    document.head.appendChild(style);

    overlay.innerHTML = `
      <div class="mandi-loader-wrapper">
        <div class="mandi-glow"></div>
        <div class="mandi-loader"></div>
        <div class="mandi-loader-inner"></div>
      </div>
      <h1 class="mandi-title">Rotating Session</h1>
      <p class="mandi-desc">Switching to a fresh premium account for uninterrupted access. Please wait...</p>
    `;
    document.body.appendChild(overlay);
  }

  // Run ONCE on page load only (200ms after DOM ready).
  // DO NOT use setInterval — it causes false positives on working dashboard pages
  // because React re-renders may briefly show auth elements during hydration.
  // The server-side ModifyResponse redirect detection is the reliable swap mechanism.
  // Client-side only catches the rare case where React renders login WITHOUT a 3xx redirect.
  setTimeout(checkAndTriggerSwap, 1500);

  // ── 7.5 Premium Quota/Limit Reached Popup Modal ──
  function showLimitReachedPopup() {
      if (document.getElementById('tm-limit-popup-overlay') || document.getElementById('tm-limit-screen')) {
          return;
      }
      var overlay = document.createElement('div');
      overlay.id = 'tm-limit-popup-overlay';
      overlay.style.cssText = 'position:fixed;inset:0;z-index:2147483647;background:#eef3f8;display:flex;align-items:center;justify-content:center;padding:24px;font-family:system-ui,-apple-system,Segoe UI,sans-serif;';
      overlay.innerHTML = '<div style="width:min(440px,100%);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px 36px;text-align:center;color:#0f172a;">'
          + '<div style="width:78px;height:78px;margin:0 auto 22px;border-radius:50%;background:conic-gradient(#3b82f6 0 70deg,#e7eef8 70deg 360deg);display:grid;place-items:center;">'
          + '<div style="width:64px;height:64px;border-radius:50%;background:#fff;display:grid;place-items:center;font-size:26px;">&#128274;</div></div>'
          + '<h1 style="font-size:28px;line-height:1.2;font-weight:800;letter-spacing:-.03em;margin:0 0 12px;">Daily Limit Reached</h1>'
          + '<p style="color:#64748b;font-size:15px;line-height:1.55;margin:0;">You have used your download limit for today. The limit resets at midnight (12:00 AM IST).</p>'
          + '<p style="margin-top:18px;color:#94a3b8;font-size:13px;">This limit applies to the current access</p>'
          + '<button id="tm-limit-close" type="button" style="margin-top:22px;background:none;border:0;color:#2563eb;font-weight:700;font-size:15px;cursor:pointer;font-family:inherit;">Close</button>'
          + '</div>';
      document.body.appendChild(overlay);
      var close = document.getElementById('tm-limit-close');
      if (close) close.onclick = function() {
          if (overlay.parentNode) overlay.parentNode.removeChild(overlay);
      };
  }

  // ── 8. Quotas/Limits Check Slider Widget Drawer (Admin Panel Switchable) ──
  function initQuotasWidget() {
      if (!document.body) {
          return;
      }
      
      // Avoid duplicate widgets in DOM
      if (document.getElementById('tm-quota-trigger')) {
          return;
      }

      console.log("[TM-WIDGET] Initializing quotas limits widget...");

      // Create sidebar slide-out container
      var side = document.createElement('div');
      side.id = 'tm-quota-sidebar';
      side.style.cssText = 'position:fixed;top:0;right:-320px;width:300px;height:100vh;background:rgba(15,23,42,0.95);backdrop-filter:blur(12px);border-left:1px solid rgba(255,255,255,0.08);z-index:10000000;box-shadow:-10px 0 30px rgba(0,0,0,0.5);transition:right 0.3s cubic-bezier(0.16,1,0.3,1);padding:24px;font-family:sans-serif;color:#ffffff;display:flex;flex-direction:column;gap:20px;';

      // Create floating toggle trigger button (lightning button)
      var btn = document.createElement('div');
      btn.id = 'tm-quota-trigger';
      btn.innerHTML = '⚡';
      btn.style.cssText = 'position:fixed;bottom:85px;right:20px;width:50px;height:50px;border-radius:25px;background:linear-gradient(135deg,#4f46e5 0%,#818cf8 100%);color:#ffffff;display:none;align-items:center;justify-content:center;font-size:22px;cursor:pointer;box-shadow:0 4px 15px rgba(79,70,229,0.5);z-index:9999999;transition:all 0.2s ease;font-family:sans-serif;font-weight:bold;animation: pulseGlow 2s infinite alternate;';
      btn.onmouseover = function() { btn.style.transform = 'scale(1.08)'; };
      btn.onmouseout = function() { btn.style.transform = 'scale(1)'; };

      // Inject custom styling keyframes for premium pulse animation
      if (!document.getElementById('tm-quota-styles')) {
          var styles = document.createElement('style');
          styles.id = 'tm-quota-styles';
          styles.innerHTML = '@keyframes pulseGlow { 0% { box-shadow: 0 4px 15px rgba(79,70,229,0.4); } 100% { box-shadow: 0 4px 25px rgba(129,140,248,0.8); } }';
          document.head.appendChild(styles);
      }

      var closeBtn = null;

      function refreshData() {
          console.log("[TM-WIDGET] Fetching user limits data from API...");
          fetch('/api/user-limits')
          .then(function(r) { return r.json(); })
          .then(function(d) {
              console.log("[TM-WIDGET] Limits response loaded successfully:", d);
              if (!d) return;

              // Cache limits globally for preemptive client-side blocking
              window.tm_user_limits = {
                  credit_used: d.credit_used,
                  credit_limit: d.credit_limit,
                  show_limit: d.show_limit,
                  username: d.username
              };
              applyDisplayUsername();

              if (!d.show_limit) {
                  console.log("[TM-WIDGET] show_limit is false from backend. Hiding widgets.");
                  btn.style.display = 'none';
                  side.style.display = 'none';
                  return;
              }
              console.log("[TM-WIDGET] show_limit is true from backend. Displaying widgets.");
              btn.style.display = 'flex';
              side.style.display = 'flex';

              var credsPercent = Math.min(100, Math.round((d.credit_used / d.credit_limit) * 100)) || 0;
              var credsRem = Math.max(0, d.credit_limit - d.credit_used);

              side.innerHTML = '<div style="display:flex;justify-content:space-between;align-items:center;border-bottom:1px solid rgba(255,255,255,0.1);padding-bottom:12px;margin-bottom:10px;font-family:sans-serif;">' +
                      '<div style="display:flex;flex-direction:column;">' +
                          '<span style="font-size:12px;color:#94a3b8;font-weight:600;text-transform:uppercase;letter-spacing:0.5px;">Active Quotas</span>' +
                          '<span style="font-size:18px;font-weight:700;color:#f8fafc;margin-top:2px;">👤 ' + d.username + '</span>' +
                      '</div>' +
                      '<span id="tm-quota-close" style="font-size:24px;color:#94a3b8;cursor:pointer;transition:color 0.2s;">&times;</span>' +
                  '</div>' +
                  '<div style="background:rgba(255,255,255,0.03);border:1px solid rgba(255,255,255,0.06);border-radius:16px;padding:16px;display:flex;flex-direction:column;gap:8px;font-family:sans-serif;">' +
                      '<div style="display:flex;justify-content:space-between;font-size:14px;font-weight:600;">' +
                          '<span style="color:#e2e8f0;">Daily Downloads</span>' +
                          '<span style="color:#10b981;">' + d.credit_used + ' / ' + d.credit_limit + '</span>' +
                      '</div>' +
                      '<div style="width:100%;height:8px;background:rgba(255,255,255,0.1);border-radius:4px;overflow:hidden;">' +
                          '<div style="width:' + credsPercent + '%;height:100%;background:linear-gradient(90deg,#10b981 0%,#34d399 100%);border-radius:4px;"></div>' +
                      '</div>' +
                      '<div style="display:flex;justify-content:space-between;font-size:12px;color:#94a3b8;margin-top:2px;">' +
                          '<span>Remaining</span>' +
                          '<strong>' + credsRem + ' Downloads</strong>' +
                      '</div>' +
                  '</div>' +
                  '<div style="margin-top:auto;font-size:11px;color:#64748b;line-height:1.5;text-align:center;border-top:1px solid rgba(255,255,255,0.05);padding-top:16px;font-family:sans-serif;">' +
                      'Daily downloads usage resets every night at <strong>12:00 AM IST (Midnight)</strong>.' +
                  '</div>';

              closeBtn = side.querySelector('#tm-quota-close');
              if (closeBtn) {
                  closeBtn.onclick = function() {
                      side.style.right = '-320px';
                  };
                  closeBtn.onmouseover = function() { closeBtn.style.color = '#ef4444'; };
                  closeBtn.onmouseout = function() { closeBtn.style.color = '#94a3b8'; };
              }
          })
          .catch(function(e) {
              console.warn('[TM-WIDGET] Limits drawer update failed', e);
          });
      }

      // Register the refreshData function globally for post-download sync
      window.tm_refresh_quota_data = refreshData;

      btn.onclick = function() {
          refreshData();
          side.style.right = '0px';
      };

      // Initial check and load
      refreshData();

      document.body.appendChild(btn);
      document.body.appendChild(side);
  }

  setInterval(function() {
      if (document.body) applyDisplayUsername();
  }, 2000);

  // Global image error handler to recover failed images by stripping crossorigin
  window.addEventListener('error', function(e) {
    if (e.target && e.target.tagName === 'IMG') {
      var img = e.target;
      if (img.src && (img.src.indexOf('/__resized__') !== -1 || img.src.indexOf('/__domain__/') !== -1)) {
        if (img.getAttribute('crossorigin') !== null) {
          console.warn("[PROXY FIXER] Image failed to load with CORS. Retrying without crossorigin:", img.src);
          img.removeAttribute('crossorigin');
          var src = img.src;
          img.src = '';
          img.src = src;
        }
      }
    }
  }, true);

  console.log("🚀 [PROXY FIXER] Runtime rewriter successfully active!");
})();
