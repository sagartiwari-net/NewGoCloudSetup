package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
)

const configFile = "config.json"

// slimOversizedCookies keeps ct_session / tm_* when the browser jar is huge.
// Leftover cookies from other tools on this host were ~12KB; deleting the
// entire Cookie header wiped the new panel session so device-bind failed.
func slimOversizedCookies(r *http.Request) {
	raw := r.Header.Get("Cookie")
	if raw == "" || len(raw) <= 8192 {
		return
	}
	var kept []string
	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name := part
		if eq := strings.Index(part, "="); eq >= 0 {
			name = strings.TrimSpace(part[:eq])
		}
		nl := strings.ToLower(name)
		if nl == "ct_session" || strings.HasPrefix(nl, "tm_") {
			kept = append(kept, part)
		}
	}
	slim := strings.Join(kept, "; ")
	log.Printf("[COOKIE] slimmed oversized Cookie %d → %d bytes (path=%s)", len(raw), len(slim), r.URL.Path)
	if slim == "" {
		r.Header.Del("Cookie")
		return
	}
	r.Header.Set("Cookie", slim)
}

type Config struct {
	Port          string   `json:"port"`
	TargetURL     string   `json:"target_url"`
	PublicHost    string   `json:"public_host"`
	PublicScheme  string   `json:"public_scheme"`
	CookieFile    string   `json:"cookie_file"`
	LocalTestMode bool     `json:"local_test_mode"`
	BindLocalhost bool     `json:"bind_localhost"`
	DebugLog      bool     `json:"debug_log"`
	ToolName      string   `json:"tool_name"`
	ExtraDomains  []string `json:"extra_domains"`
	BlockedPaths      []string `json:"blocked_paths"`
	UserAgent         string   `json:"user_agent"`
	CloudflareBypass  bool     `json:"cloudflare_bypass"`
	PanelDB          string   `json:"panel_db"`
	HomePath         string   `json:"home_path"`
}

var (
	currentConfig Config
	configModTime time.Time
	sessionMu     sync.RWMutex
	sessionRaw    string
	sessionMTime  time.Time

	integrityRegex = regexp.MustCompile(`(?i)\s*integrity=(?:"[^"]*"|'[^']*')`)
)

func loadConfig() Config {
	path := os.Getenv("CONFIG_FILE")
	if path == "" {
		path = configFile
	}
	info, err := os.Stat(path)
	if err != nil {
		return currentConfig
	}
	if !info.ModTime().After(configModTime) {
		return currentConfig
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return currentConfig
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("[CONFIG] parse error: %v", err)
		return currentConfig
	}
	if cfg.Port == "" {
		cfg.Port = "4771"
	}
	if cfg.TargetURL == "" {
		cfg.TargetURL = "https://www.shortform.com"
	}
	if cfg.PublicHost == "" {
		cfg.PublicHost = "127.0.0.1:" + cfg.Port
	}
	if cfg.PublicScheme == "" {
		cfg.PublicScheme = "http"
	}
	if cfg.CookieFile == "" {
		cfg.CookieFile = "cookie.txt"
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	}
	if len(cfg.ExtraDomains) == 0 {
		cfg.ExtraDomains = defaultShortformExtraDomains()
	} else {
		cfg.ExtraDomains = mergeExtraDomains(cfg.ExtraDomains)
	}
	cfg.CloudflareBypass = cfg.CloudflareBypass || cfg.LocalTestMode
	currentConfig = cfg
	configModTime = info.ModTime()
	return cfg
}

func loadSession(cfg Config) string {
	path := cfg.CookieFile
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	sessionMu.RLock()
	if !info.ModTime().After(sessionMTime) && sessionRaw != "" {
		defer sessionMu.RUnlock()
		return sessionRaw
	}
	sessionMu.RUnlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(string(data))
	sessionMu.Lock()
	sessionRaw = raw
	sessionMTime = info.ModTime()
	sessionMu.Unlock()
	return raw
}

func proxyHost(cfg Config) string {
	host := cfg.PublicHost
	if h, _, err := net.SplitHostPort(cfg.PublicHost); err == nil {
		host = h
	}
	return host
}

func proxyOrigin(cfg Config) string {
	return cfg.PublicScheme + "://" + cfg.PublicHost
}

func targetHost(cfg Config) string {
	u, err := url.Parse(cfg.TargetURL)
	if err != nil || u.Host == "" {
		return "www.shortform.com"
	}
	return u.Host
}

func debugLog(cfg Config, format string, args ...interface{}) {
	if cfg.DebugLog {
		log.Printf("[DEBUG] "+format, args...)
	}
}

func buildLocalHostFix(cfg Config) string {
	if !cfg.LocalTestMode {
		return ""
	}
	fakeHost := targetHost(cfg)
	proxyName := proxyHost(cfg)
	fakeOrigin := upstreamOrigin(cfg)
	return fmt.Sprintf(`
try {
  var REAL_PROXY_ORIGIN = window.location.origin;
  var REAL_PROXY_HOST = window.location.host;
  var REAL_PROXY_PROTOCOL = window.location.protocol;
  var FAKE_NAME = %q;
  var FAKE_HOST = %q;
  var FAKE_ORIGIN = %q;
  var PROXY_NAME = %q;
  function isProxyHost(h) {
    if (!h) return false;
    return h === '127.0.0.1' || h === 'localhost' || h === PROXY_NAME || h.indexOf(PROXY_NAME) === 0;
  }
  var _realHostGet = null;
  var _hostDesc = Object.getOwnPropertyDescriptor(Location.prototype, 'hostname');
  if (_hostDesc && _hostDesc.get) {
    _realHostGet = _hostDesc.get;
    Object.defineProperty(Location.prototype, 'hostname', {
      get: function() {
        var h = _realHostGet.call(this);
        if (isProxyHost(h)) return FAKE_NAME;
        return h;
      },
      configurable: true
    });
  }
  var _hostFullDesc = Object.getOwnPropertyDescriptor(Location.prototype, 'host');
  if (_hostFullDesc && _hostFullDesc.get && _realHostGet) {
    var _realHostFullGet = _hostFullDesc.get;
    Object.defineProperty(Location.prototype, 'host', {
      get: function() {
        if (isProxyHost(_realHostGet.call(this))) return FAKE_HOST;
        return _realHostFullGet.call(this);
      },
      configurable: true
    });
  }
  var _originDesc = Object.getOwnPropertyDescriptor(Location.prototype, 'origin');
  if (_originDesc && _originDesc.get && _realHostGet) {
    var _realOriginGet = _originDesc.get;
    Object.defineProperty(Location.prototype, 'origin', {
      get: function() {
        if (isProxyHost(_realHostGet.call(this))) return FAKE_ORIGIN;
        return _realOriginGet.call(this);
      },
      configurable: true
    });
  }
  var _protocolDesc = Object.getOwnPropertyDescriptor(Location.prototype, 'protocol');
  if (_protocolDesc && _protocolDesc.get && _realHostGet) {
    var _realProtocolGet = _protocolDesc.get;
    Object.defineProperty(Location.prototype, 'protocol', {
      get: function() {
        if (isProxyHost(_realHostGet.call(this))) return 'https:';
        return _realProtocolGet.call(this);
      },
      configurable: true
    });
  }
  var _hrefDesc = Object.getOwnPropertyDescriptor(Location.prototype, 'href');
  if (_hrefDesc && _hrefDesc.get && _realHostGet) {
    var _realHrefGet = _hrefDesc.get;
    var _realHrefSet = _hrefDesc.set;
    var _pathDesc = Object.getOwnPropertyDescriptor(Location.prototype, 'pathname');
    var _searchDesc = Object.getOwnPropertyDescriptor(Location.prototype, 'search');
    var _hashDesc = Object.getOwnPropertyDescriptor(Location.prototype, 'hash');
    Object.defineProperty(Location.prototype, 'href', {
      get: function() {
        if (!isProxyHost(_realHostGet.call(this))) return _realHrefGet.call(this);
        var p = _pathDesc && _pathDesc.get ? _pathDesc.get.call(this) : '/';
        var q = _searchDesc && _searchDesc.get ? _searchDesc.get.call(this) : '';
        var h = _hashDesc && _hashDesc.get ? _hashDesc.get.call(this) : '';
        return FAKE_ORIGIN + p + q + h;
      },
      set: function(v) {
        // Keep native setter — redefine-without-set breaks location.replace/assign.
        if (_realHrefSet) {
          var s = String(v || "");
          if (s.indexOf(FAKE_ORIGIN) === 0) s = REAL_PROXY_ORIGIN + s.slice(FAKE_ORIGIN.length);
          _realHrefSet.call(this, s);
          return;
        }
      },
      configurable: true
    });
  }
} catch (e) {}
`, fakeHost, fakeHost, fakeOrigin, proxyName)
}

func buildInjectScript(cfg Config, session string) string {
	lsJSON := localStorageJSONForBrowser(session)
	jsData := strings.ReplaceAll(string(lsJSON), "</", "<\\/")
	blocked, _ := json.Marshal(cfg.BlockedPaths)
	// Do NOT use buildLocalHostFix here: Shortform's app.js does
	//   apiBase = fn.replace("localhost", location.hostname) when fn starts with http://localhost
	// Spoofing hostname to www.shortform.com would send API to the real site (CORS/mixed-content fail).
	localFix := `
  var REAL_PROXY_ORIGIN = window.location.origin;
  var REAL_PROXY_HOST = window.location.host;
  var REAL_PROXY_PROTOCOL = window.location.protocol;
  // Kill Shortform service worker — it breaks API auth on localhost proxies.
  try {
    if (navigator.serviceWorker) {
      navigator.serviceWorker.getRegistrations().then(function(regs) {
        regs.forEach(function(r) { r.unregister(); });
      });
    }
    if (window.caches && caches.keys) {
      caches.keys().then(function(keys) {
        keys.forEach(function(k) { caches.delete(k); });
      });
    }
  } catch (eSW) {}
  // localhost cookies are shared across ALL ports — clear so API requests stay under nginx limits.
  try {
    document.cookie.split(";").forEach(function(c) {
      var n = (c.split("=")[0] || "").trim();
      if (!n) return;
      document.cookie = n + "=;expires=Thu, 01 Jan 1970 00:00:00 GMT;path=/";
      document.cookie = n + "=;expires=Thu, 01 Jan 1970 00:00:00 GMT;path=/;domain=localhost";
      document.cookie = n + "=;expires=Thu, 01 Jan 1970 00:00:00 GMT;path=/;domain=.localhost";
    });
  } catch (eCk) {}
`
	extraDomains, _ := json.Marshal(cfg.ExtraDomains)
	tHost := targetHost(cfg)

	return `<script data-shortform-proxy="1">
(function(){
` + localFix + `
  var EXTRA_DOMAINS = ` + string(extraDomains) + `;
  var TARGET_HOST = ` + jsonString(tHost) + `;
  function proxyUrl(url) {
    if (typeof url !== "string") return url;
    var u = url.trim();
    // Bundle may still say http://this-host/... after an old http rewrite.
    // Keep API calls on the page origin so Secure cookies and auth stay intact.
    var httpSelf = "http://" + REAL_PROXY_HOST;
    var httpsSelf = "https://" + REAL_PROXY_HOST;
    if (u.indexOf(httpSelf) === 0) return REAL_PROXY_ORIGIN + u.slice(httpSelf.length);
    if (u.indexOf(httpsSelf) === 0) return REAL_PROXY_ORIGIN + u.slice(httpsSelf.length);
    var pairs = [
      ["https://" + TARGET_HOST, REAL_PROXY_ORIGIN],
      ["http://" + TARGET_HOST, REAL_PROXY_ORIGIN],
      ["wss://" + TARGET_HOST, REAL_PROXY_ORIGIN.replace(/^http/, "ws")],
      ["ws://" + TARGET_HOST, REAL_PROXY_ORIGIN.replace(/^http/, "ws")],
      ["//" + TARGET_HOST, REAL_PROXY_PROTOCOL + "//" + REAL_PROXY_HOST]
    ];
    for (var i = 0; i < EXTRA_DOMAINS.length; i++) {
      var d = EXTRA_DOMAINS[i];
      if (d === TARGET_HOST) continue;
      if (d.indexOf("firebaseio.com") !== -1 || d.indexOf("firebaseapp.com") !== -1) continue;
      var ext = REAL_PROXY_ORIGIN + "/ext-proxy/" + d;
      var wsext = ext.replace(/^http/, "ws");
      pairs.push(["https://" + d, ext]);
      pairs.push(["http://" + d, ext]);
      pairs.push(["wss://" + d, wsext]);
      pairs.push(["ws://" + d, wsext]);
      pairs.push(["//" + d, REAL_PROXY_PROTOCOL + "//" + REAL_PROXY_HOST + "/ext-proxy/" + d]);
    }
    for (var j = 0; j < pairs.length; j++) {
      if (u.indexOf(pairs[j][0]) === 0) return pairs[j][1] + u.slice(pairs[j][0].length);
    }
    try {
      var parsed = new URL(u, REAL_PROXY_ORIGIN);
      var h = parsed.hostname || "";
      if (h === TARGET_HOST || h === "www.shortform.com" || h === "shortform.com") {
        if (parsed.protocol === "ws:" || parsed.protocol === "wss:") {
          return REAL_PROXY_ORIGIN.replace(/^http/, "ws") + parsed.pathname + parsed.search + parsed.hash;
        }
        return REAL_PROXY_ORIGIN + parsed.pathname + parsed.search + parsed.hash;
      }
      if (h.indexOf("firebaseio.com") !== -1 || h.indexOf("firebaseapp.com") !== -1 || h.indexOf("googleapis.com") !== -1) {
        return url; // keep Firebase / Google APIs on official hosts
      }
      if (h.endsWith(".shortform.com") || h.indexOf("mixpanel") !== -1 || h.indexOf("mxpnl") !== -1 || h.indexOf("weglot") !== -1 || h.indexOf("sentry") !== -1 || h.indexOf("intercom") !== -1 || h.indexOf("stripe.com") !== -1) {
        var ext2 = REAL_PROXY_ORIGIN + "/ext-proxy/" + h;
        if (parsed.protocol === "ws:" || parsed.protocol === "wss:") {
          return ext2.replace(/^http/, "ws") + parsed.pathname + parsed.search + parsed.hash;
        }
        return ext2 + parsed.pathname + parsed.search + parsed.hash;
      }
    } catch (e) {}
    return url;
  }

  try {
    var storageData = ` + jsData + `;
    for (var key in storageData) {
      if (Object.prototype.hasOwnProperty.call(storageData, key) && storageData[key] != null) {
        localStorage.setItem(key, storageData[key]);
      }
    }
    if (Object.keys(storageData).length) {
      console.log("[Shortform Proxy] Restored localStorage (" + Object.keys(storageData).length + " keys)");
    }
  } catch (e) {
    console.warn("[Shortform Proxy] localStorage restore failed", e);
  }

  function sfLoggedOut(url) {
    var u = String(url || "");
    if (u.indexOf("/api/") === -1) return;
    if (location.pathname === "/__tm_logged_out") return;
    location.replace("/__tm_logged_out");
  }
  var __fetch = window.fetch;
  window.fetch = function(input, init) {
    if (typeof input === "string") input = proxyUrl(input);
    else if (input && input.url) {
      var proxied = proxyUrl(input.url);
      if (proxied !== input.url) input = new Request(proxied, input);
    }
    return __fetch.call(this, input, init).then(function(res) {
      try { if (res && res.status === 401) sfLoggedOut(typeof input === "string" ? input : (input && input.url)); } catch (e401) {}
      return res;
    });
  };

  var __xhrOpen = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function(method, url) {
    var args = arguments;
    if (typeof url === "string") args[1] = proxyUrl(url);
    else if (url && typeof url.toString === "function") args[1] = proxyUrl(url.toString());
    this.__sfURL = args[1];
    return __xhrOpen.apply(this, args);
  };
  var __xhrSend = XMLHttpRequest.prototype.send;
  XMLHttpRequest.prototype.send = function() {
    this.addEventListener("load", function() {
      try { if (this.status === 401) sfLoggedOut(this.__sfURL); } catch (e401) {}
    });
    return __xhrSend.apply(this, arguments);
  };

  var __beacon = navigator.sendBeacon && navigator.sendBeacon.bind(navigator);
  if (__beacon) {
    navigator.sendBeacon = function(url, data) {
      if (typeof url === "string") url = proxyUrl(url);
      return __beacon(url, data);
    };
  }

  // ─── Cloudflare Turnstile bypass ─────────────────────────────────────────
  // push.www.shortform.com WebSocket requires a valid Cloudflare Turnstile token.
  // We mock the turnstile API so Shortform JS thinks verification passed.
  var _cfFakeToken = "0." + Math.random().toString(36).slice(2) + Date.now();
  if (!window.turnstile) {
    window.turnstile = {
      render: function(el, opts) {
        // Immediately call the success callback with our fake token
        setTimeout(function() {
          _cfFakeToken = "0." + Math.random().toString(36).slice(2) + Date.now();
          if (opts && typeof opts.callback === "function") opts.callback(_cfFakeToken);
          // Also hide the container element so no UI shows
          try {
            var node = typeof el === "string" ? document.querySelector(el) : el;
            if (node) node.style.display = "none";
          } catch(e) {}
          // Close any visible anti-bot check dialog
          try {
            var dialogs = document.querySelectorAll('[class*="modal"], [class*="dialog"], [class*="overlay"]');
            for (var d = 0; d < dialogs.length; d++) {
              var txt = dialogs[d].innerText || "";
              if (txt.indexOf("Verify you are human") !== -1 || txt.indexOf("Anti-bot") !== -1) {
                dialogs[d].style.display = "none";
              }
            }
          } catch(e2) {}
        }, 100);
        return "proxy-widget-" + Math.random().toString(36).slice(2);
      },
      getResponse: function() { return _cfFakeToken; },
      reset: function() {},
      remove: function() {}
    };
    console.log("[Shortform Proxy] Cloudflare Turnstile bypassed");
  }

  // Intercept fetch calls to challenges.cloudflare.com — return fake success
  var __fetch = window.fetch;
  window.fetch = function(input, init) {
    var url = typeof input === "string" ? input : (input && input.url ? input.url : "");
    if (url && url.indexOf("challenges.cloudflare.com") !== -1) {
      console.log("[Shortform Proxy] Blocked CF challenge fetch:", url);
      return Promise.resolve(new Response(JSON.stringify({success:true,token:_cfFakeToken}), {
        status: 200,
        headers: {"Content-Type": "application/json"}
      }));
    }
    if (typeof input === "string") input = proxyUrl(input);
    else if (input && input.url) {
      var proxied = proxyUrl(input.url);
      if (proxied !== input.url) input = new Request(proxied, input);
    }
    return __fetch.call(this, input, init);
  };

  var __WS = window.WebSocket;
  window.WebSocket = function(url, protocols) {
    if (typeof url === "string") {
      url = proxyUrl(url);
      if (REAL_PROXY_PROTOCOL === "http:" && url.indexOf("wss://" + REAL_PROXY_HOST) === 0) {
        url = "ws://" + REAL_PROXY_HOST + url.slice(("wss://" + REAL_PROXY_HOST).length);
      }
    }
    if (protocols !== undefined) return new __WS(url, protocols);
    return new __WS(url);
  };
  window.WebSocket.prototype = __WS.prototype;
  window.WebSocket.CONNECTING = __WS.CONNECTING;
  window.WebSocket.OPEN = __WS.OPEN;
  window.WebSocket.CLOSING = __WS.CLOSING;
  window.WebSocket.CLOSED = __WS.CLOSED;

  var BLOCKED = ` + string(blocked) + `;
  // Vue path routes (not hash). Soft-block login/account/billing.
  var BLOCKED_PATHS = [
    "/app/login", "/app/signup", "/app/sign-up", "/app/register",
    "/app/pricing", "/app/plans", "/app/billing", "/app/subscription",
    "/app/account", "/app/settings", "/app/checkout", "/app/profile",
    "/app/onboarding", "/login", "/signup", "/pricing", "/billing",
    "/account", "/settings", "/logout"
  ];
  var HOME_PATH = "/app/discover";
  function pathBlocked(p) {
    p = String(p || "").toLowerCase().split("?")[0];
    if (!p) p = "/";
    for (var i = 0; i < BLOCKED_PATHS.length; i++) {
      var b = BLOCKED_PATHS[i];
      if (p === b || p.indexOf(b + "/") === 0) return true;
    }
    for (var j = 0; j < BLOCKED.length; j++) {
      var bb = String(BLOCKED[j] || "").toLowerCase();
      if (!bb) continue;
      if (p === bb || p.indexOf(bb + "/") === 0) return true;
    }
    return false;
  }
  function goHome() {
    try { location.replace(HOME_PATH); } catch (eR) { location.href = HOME_PATH; }
  }
  function guardPath() {
    try { if (pathBlocked(location.pathname)) goHome(); } catch (eG) {}
  }
  guardPath();
  window.addEventListener("popstate", guardPath, true);
  setInterval(guardPath, 800);

  try {
    var _push = history.pushState.bind(history);
    var _replace = history.replaceState.bind(history);
    history.pushState = function(state, title, url) {
      if (url != null) {
        try {
          var u = new URL(String(url), location.origin);
          if (pathBlocked(u.pathname)) url = HOME_PATH;
        } catch (eU) {}
      }
      return _push(state, title, url);
    };
    history.replaceState = function(state, title, url) {
      if (url != null) {
        try {
          var u2 = new URL(String(url), location.origin);
          if (pathBlocked(u2.pathname)) url = HOME_PATH;
        } catch (eU2) {}
      }
      return _replace(state, title, url);
    };
  } catch (eHist) {}

  document.addEventListener("click", function(e) {
    var a = e.target && e.target.closest ? e.target.closest("a,button") : null;
    if (!a) return;
    var href = (a.getAttribute("href") || "").toLowerCase();
    var txt = ((a.innerText || a.textContent || "") + "").replace(/\s+/g, " ").trim().toLowerCase();
    var hit = false;
    if (href) {
      try {
        var hp = href.indexOf("http") === 0 ? new URL(href).pathname : href.split("?")[0];
        if (pathBlocked(hp)) hit = true;
      } catch (eH) {}
    }
    if (txt === "logout" || txt === "log out" || txt === "sign out" || txt.indexOf("sign out") !== -1) hit = true;
    if (href.indexOf("logout") !== -1 || href.indexOf("sign-out") !== -1) hit = true;
    if (!hit) return;
    e.preventDefault();
    e.stopPropagation();
    e.stopImmediatePropagation();
    try {
      location.replace("/__tm_logged_out");
    } catch (eOut) { goHome(); }
  }, true);

  try {
    var st = document.createElement("style");
    st.setAttribute("data-shortform-hide", "1");
    st.textContent = '.user-menu,.header__user-menu,.user-menu__changelog-badge-container,.changelog-badge,a[href*="/app/account"],a[href*="/app/settings"],a[href*="/app/billing"],a[href*="/app/pricing"],a[href*="/app/login"],a[href*="logout"],a[href*="sign-out"]{display:none!important;visibility:hidden!important;pointer-events:none!important}';
    (document.head || document.documentElement).appendChild(st);
  } catch (eCss) {}

  // Logout clears auth_token/user — block remove so session sticks.
  try {
    var AUTH_KEYS = { auth_token: 1, user: 1 };
    var _rm = Storage.prototype.removeItem;
    Storage.prototype.removeItem = function(k) {
      if (this === window.localStorage && AUTH_KEYS[k]) {
        try { console.log("[Shortform Proxy] blocked localStorage.removeItem(" + k + ")"); } catch (e0) {}
        return;
      }
      return _rm.apply(this, arguments);
    };
    var _clear = Storage.prototype.clear;
    Storage.prototype.clear = function() {
      if (this === window.localStorage) {
        var keep = {};
        for (var k in AUTH_KEYS) {
          try { keep[k] = this.getItem(k); } catch (e1) {}
        }
        _clear.apply(this, arguments);
        for (var k2 in keep) {
          if (keep[k2] != null) this.setItem(k2, keep[k2]);
        }
        return;
      }
      return _clear.apply(this, arguments);
    };
  } catch (eAuth) {}
})();
</script>`
}

func basicAuthToken(token string) string {
	// axios auth: { username: token, password: "" } → Basic base64(token + ":")
	return base64.StdEncoding.EncodeToString([]byte(token + ":"))
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func proxyHostPort(cfg Config) string {
	if strings.Contains(cfg.PublicHost, ":") {
		return cfg.PublicHost
	}
	return net.JoinHostPort(cfg.PublicHost, cfg.Port)
}

func rewriteBody(body []byte, cfg Config) []byte {
	s := string(body)
	origin := proxyOrigin(cfg)
	hostPort := proxyHostPort(cfg)
	tHost := targetHost(cfg)
	wsOrigin := strings.Replace(origin, "http", "ws", 1)

	replacements := [][2]string{
		{"https://" + tHost, origin},
		{"http://" + tHost, origin},
		{"//" + tHost, "//" + hostPort},
		{"wss://" + tHost, wsOrigin},
		{"ws://" + tHost, wsOrigin},
		{"https://www.shortform.com", origin},
		{"http://www.shortform.com", origin},
	}
	for _, d := range cfg.ExtraDomains {
		replacements = addSubdomainRewrites(replacements, cfg, d)
	}
	for _, pair := range replacements {
		s = strings.ReplaceAll(s, pair[0], pair[1])
	}
	// After rewriting frontend URL to localhost, force isLocalhost (Y_) false —
	// otherwise y=(()=>"http://localhost…".startsWith("http://localhost"))() becomes true.
	s = strings.ReplaceAll(s,
		`y=(()=>"`+origin+`".startsWith("http://localhost")||"`+origin+`".startsWith("http://backend"))()`,
		`y=(!1)`)
	for _, pair := range [][2]string{
		{"DigitaVision", "ToolsMandi"},
		{"Digitavision", "ToolsMandi"},
		{"digitavision", "ToolsMandi"},
	} {
		s = strings.ReplaceAll(s, pair[0], pair[1])
	}
	s = integrityRegex.ReplaceAllString(s, "")
	// Inline CSP meta blocks http://cdn.www.shortform.com — same-origin /cdn-proxy is allowed via 'self'
	s = regexp.MustCompile(`(?i)<meta[^>]+http-equiv=["']Content-Security-Policy["'][^>]*>`).ReplaceAllString(s, "")
	if cfg.PublicScheme == "http" {
		hostPort := proxyHostPort(cfg)
		s = strings.ReplaceAll(s, "wss://"+hostPort, "ws://"+hostPort)
	}
	return []byte(s)
}

func relaxResponseHeaders(resp *http.Response, cfg Config) {
	resp.Header.Del("Content-Security-Policy")
	resp.Header.Del("Content-Security-Policy-Report-Only")
	resp.Header.Del("X-Frame-Options")
	resp.Header.Set("Access-Control-Allow-Origin", proxyOrigin(cfg))
	resp.Header.Set("Access-Control-Allow-Credentials", "true")
}

func handleCORSPreflight(w http.ResponseWriter, r *http.Request, cfg Config) bool {
	if r.Method != http.MethodOptions {
		return false
	}
	w.Header().Set("Access-Control-Allow-Origin", proxyOrigin(cfg))
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS")
	if h := r.Header.Get("Access-Control-Request-Headers"); h != "" {
		w.Header().Set("Access-Control-Allow-Headers", h)
	} else {
		w.Header().Set("Access-Control-Allow-Headers", "*")
	}
	w.WriteHeader(http.StatusNoContent)
	return true
}

func decompressBody(body []byte, encoding string) ([]byte, error) {
	enc := strings.ToLower(strings.TrimSpace(encoding))
	switch {
	case strings.Contains(enc, "br"):
		r := brotli.NewReader(bytes.NewReader(body))
		return io.ReadAll(r)
	case strings.Contains(enc, "gzip"):
		r, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return body, err
		}
		defer r.Close()
		return io.ReadAll(r)
	default:
		return body, nil
	}
}

func isHTML(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml")
}

func isRewritable(ct string) bool {
	ct = strings.ToLower(ct)
	return isHTML(ct) || strings.Contains(ct, "javascript") || strings.Contains(ct, "json") || strings.Contains(ct, "text/css")
}

func newReverseProxy(target *url.URL, cfg Config, getSession func() string) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = buildChromeTransport()
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if marked, px := panelUpstreamProxy(r.Context()); marked && px != "" {
			log.Printf("[PROXY] dial failed %s %s: %v", r.Method, r.URL.Path, err)
			renderProxyProblem(w, r)
			return
		}
		log.Printf("[PROXY] upstream error %s %s: %v", r.Method, r.URL.Path, err)
		http.Error(w, "Upstream error", http.StatusBadGateway)
	}
	origDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDirector(req)
		session := panelSessionFor(req, getSession)
		req.Host = target.Host
		applyBrowserHeaders(req, cfg)
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			req.Header.Set("Sec-Fetch-Dest", "empty")
			req.Header.Set("Sec-Fetch-Mode", "cors")
			req.Header.Set("Sec-Fetch-Site", "same-origin")
		}
		// Shortform API: axios uses HTTP Basic (username=auth_token, password="").
		// Always set from the panel account so discover works even if the browser token races.
		if strings.HasPrefix(req.URL.Path, "/api/") {
			tok := sessionToken(session)
			if tok == "" {
				log.Printf("[API] no auth_token in panel account path=%s — refresh Shortform GoAuto localStorage", req.URL.Path)
			} else {
				req.Header.Set("Authorization", "Basic "+basicAuthToken(tok))
			}
			if req.Header.Get("X-Sf-Client") == "" {
				req.Header.Set("X-Sf-Client", "11.8.0")
			}
			if req.Header.Get("Accept") == "" || strings.Contains(req.Header.Get("Accept"), "text/html") {
				req.Header.Set("Accept", "application/json, text/plain, */*")
			}
			// API auth is the Basic token. Account cookies on /api make CloudFront 401.
			req.Header.Del("Cookie")
		} else {
			// NEVER merge the browser Cookie header: leftover jars bloat past nginx limits.
			req.Header.Del("Cookie")
			if cookieHdr, _ := parseSessionStorage(session); cookieHdr != "" {
				req.Header.Set("Cookie", cookieHdr)
			}
		}
		zgOrigin := upstreamOrigin(cfg)
		if ref := req.Header.Get("Referer"); ref != "" {
			req.Header.Set("Referer", strings.ReplaceAll(ref, proxyOrigin(cfg), zgOrigin))
		} else {
			req.Header.Set("Referer", zgOrigin+"/app/discover")
		}
		if origin := req.Header.Get("Origin"); origin != "" {
			req.Header.Set("Origin", zgOrigin)
		} else if req.Method != http.MethodGet && req.Method != http.MethodHead {
			req.Header.Set("Origin", zgOrigin)
		}
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		if resp.StatusCode == http.StatusSwitchingProtocols {
			return nil
		}
		// Prevent upstream/session cookies from piling onto shared localhost jar
		resp.Header.Del("Set-Cookie")
		session := panelSessionFor(resp.Request, getSession)
		ct := resp.Header.Get("Content-Type")
		enc := resp.Header.Get("Content-Encoding")
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		if resp.Request != nil && strings.HasPrefix(resp.Request.URL.Path, "/api/") && resp.StatusCode >= 400 {
			snip := body
			if len(snip) > 180 {
				snip = snip[:180]
			}
			log.Printf("[API] %s %s → %d %s", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode, bytes.TrimSpace(snip))
			if resp.StatusCode == http.StatusUnauthorized && bytes.Contains(bytes.ToLower(body), []byte("unauthorized")) {
				noteShortformLogout(cfg, resp.Request, "api_unauthorized")
			}
		}
		plain, err := decompressBody(body, enc)
		if err != nil {
			plain = body
		}
		if isRewritable(ct) {
			plain = rewriteBody(plain, cfg)
			if cfg.CloudflareBypass {
				plain = stripCloudflareChallengeHTML(plain, cfg)
			}
			if isHTML(ct) && isCloudflareBlockPage(plain) {
				log.Printf("[CF] Upstream returned Cloudflare block page for %s", resp.Request.URL.Path)
			}
			// Never inject SPA bootstrap into API HTML error pages
			apiReq := resp.Request != nil && strings.HasPrefix(resp.Request.URL.Path, "/api/")
			if isHTML(ct) && !apiReq {
				if usesPanelAccountMode(cfg) {
					plain = injectDeviceHTML(plain)
				}
				inject := buildInjectScript(cfg, session)
				lower := strings.ToLower(string(plain))
				if idx := strings.Index(lower, "<head>"); idx >= 0 {
					pos := idx + len("<head>")
					plain = append([]byte(string(plain[:pos])+"\n"+inject), plain[pos:]...)
				} else if idx := strings.Index(lower, "<head "); idx >= 0 {
					if end := strings.Index(string(plain[idx:]), ">"); end >= 0 {
						pos := idx + end + 1
						plain = append([]byte(string(plain[:pos])+"\n"+inject), plain[pos:]...)
					}
				}
			}
		}
		relaxResponseHeaders(resp, cfg)
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
		resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(plain)))
		resp.Body = io.NopCloser(bytes.NewReader(plain))
		return nil
	}
	return proxy
}

func stripPrefix(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return path
	}
	p := strings.TrimPrefix(path, prefix)
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

func main() {
	cfg := loadConfig()
	session := loadSession(cfg)
	if session == "" {
		log.Printf("[WARN] cookie.txt is empty — paste GoAuto localStorage export before testing")
	} else if token := sessionToken(session); token != "" {
		log.Printf("[SESSION] Loaded Shortform token (%d chars)", len(token))
	} else {
		log.Printf("[WARN] No auth_token key found in cookie.txt localStorage export")
	}

	targetURL, err := url.Parse(cfg.TargetURL)
	if err != nil {
		log.Fatalf("[FATAL] invalid target_url: %v", err)
	}
	getSession := func() string {
		cfg = loadConfig()
		return loadSession(cfg)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg = loadConfig()
		if handleCORSPreflight(w, r, cfg) {
			return
		}

		if cfg.CloudflareBypass {
			if strings.HasPrefix(r.URL.Path, "/cdn-cgi/") || r.URL.Path == "/cf-turnstile-bypass.js" {
				serveCloudflareBypass(w, r.URL.Path)
				return
			}
		}

		if r.TLS != nil || strings.Contains(strings.ToLower(r.Header.Get("X-Forwarded-Proto")), "https") {
			cfg.PublicScheme = "https"
		}
		acc, nextReq, handled := preparePanelRequest(w, r, cfg)
		if handled {
			return
		}
		if usesPanelAccountMode(cfg) {
			r = stampPanelAccount(nextReq, acc)
			r = withShortformAccount(r, acc)
		}
		if r.URL.Path == "/__tm_logout" || r.URL.Path == "/__tm_logged_out" {
			reason := "api_unauthorized"
			if r.URL.Path == "/__tm_logout" {
				reason = "user_logout"
			}
			serveShortformLogout(w, r, cfg, reason)
			return
		}


		path := strings.ToLower(r.URL.Path)
		for _, blocked := range cfg.BlockedPaths {
			b := strings.ToLower(strings.TrimSpace(blocked))
			if b != "" && (path == b || strings.HasPrefix(path, b+"/")) {
				http.Redirect(w, r, "/app/discover", http.StatusFound)
				return
			}
		}

		if r.URL.Path == "/" || r.URL.Path == "" {
			http.Redirect(w, r, "/app/discover", http.StatusFound)
			return
		}

		// Disable PWA service worker on proxy (causes API 400 / stale cache on localhost)
		if r.URL.Path == "/service-worker.js" || r.URL.Path == "/firebase-messaging-sw.js" {
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("self.addEventListener('install',function(e){self.skipWaiting()});self.addEventListener('activate',function(e){e.waitUntil(self.registration.unregister())});\n"))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/ext-proxy/") {
			rest := strings.TrimPrefix(r.URL.Path, "/ext-proxy/")
			parts := strings.SplitN(rest, "/", 2)
			if len(parts) == 0 || parts[0] == "" {
				http.NotFound(w, r)
				return
			}
			extHost := parts[0]
			extPath := "/"
			if len(parts) == 2 && parts[1] != "" {
				extPath = "/" + parts[1]
			}
			extTarget, err := url.Parse("https://" + extHost)
			if err != nil {
				http.Error(w, "bad ext host", http.StatusBadRequest)
				return
			}
			if isWebSocket(r) {
				handleWebSocket(w, r, cfg, extHost, extPath, getSession)
				return
			}
			r2 := r.Clone(r.Context())
			r2.URL.Path = extPath
			r2.URL.RawPath = ""
			r2.Host = extHost
			newReverseProxy(extTarget, cfg, getSession).ServeHTTP(w, r2)
			return
		}

		debugLog(cfg, "%s %s", r.Method, r.URL.RequestURI())
		if isWebSocket(r) {
			handleWebSocket(w, r, cfg, targetHost(cfg), r.URL.Path, getSession)
			return
		}
		newReverseProxy(targetURL, cfg, getSession).ServeHTTP(w, r)
	})

	addr := ":" + cfg.Port
	if cfg.BindLocalhost {
		addr = "127.0.0.1:" + cfg.Port
	}

	log.Printf("╔══════════════════════════════════════════════╗")
	log.Printf("║  Shortform Go Proxy — LOCAL                      ║")
	log.Printf("║  URL:    %s://%s", cfg.PublicScheme, cfg.PublicHost)
	log.Printf("║  Target: %s", cfg.TargetURL)
	log.Printf("║  Auth:   localStorage via cookie.txt         ║")
	log.Printf("╚══════════════════════════════════════════════╝")

	// Wrap: slim huge Cookie jars but keep ct_session. Deleting the whole
	// header wiped the panel session and /api/device-bind returned Access Denied.
	safe := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slimOversizedCookies(r)
		handler.ServeHTTP(w, r)
	})

	server := &http.Server{
		Addr:           addr,
		Handler:        safe,
		ReadTimeout:    120 * time.Second,
		WriteTimeout:   300 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 4 << 20, // 4MB — avoid HTTP 431 from shared localhost cookies
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("[FATAL] %v", err)
	}
}
