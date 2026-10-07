package main

import (
	"bytes"
	"compress/gzip"
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
		cfg.Port = "4691"
	}
	if cfg.TargetURL == "" {
		cfg.TargetURL = "https://my.zonguru.com"
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
		cfg.ExtraDomains = defaultZonGuruExtraDomains()
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
		return "my.zonguru.com"
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
	localFix := buildLocalHostFix(cfg)
	extraDomains, _ := json.Marshal(cfg.ExtraDomains)
	tHost := targetHost(cfg)

	return `<script data-zonguru-proxy="1">
(function(){
` + localFix + `
  var EXTRA_DOMAINS = ` + string(extraDomains) + `;
  var TARGET_HOST = ` + jsonString(tHost) + `;
  function proxyUrl(url) {
    if (typeof url !== "string") return url;
    var u = url.trim();
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
      if (h === TARGET_HOST || h === "my.zonguru.com" || h === "www.zonguru.com") {
        if (parsed.protocol === "ws:" || parsed.protocol === "wss:") {
          return REAL_PROXY_ORIGIN.replace(/^http/, "ws") + parsed.pathname + parsed.search + parsed.hash;
        }
        return REAL_PROXY_ORIGIN + parsed.pathname + parsed.search + parsed.hash;
      }
      if (h.endsWith(".zonguru.com") || h.endsWith(".turbodash.co") || h.indexOf("intercom") !== -1 || h.indexOf("stripe.com") !== -1 || h.indexOf("profitwell.com") !== -1) {
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
      console.log("[ZonGuru Proxy] Restored localStorage (" + Object.keys(storageData).length + " keys)");
    }
  } catch (e) {
    console.warn("[ZonGuru Proxy] localStorage restore failed", e);
  }

  function stampDeviceHeaders(init, input) {
    try {
      var fp = localStorage.getItem("tm_device_fp") || sessionStorage.getItem("tm_device_fp") || "";
      var proof = localStorage.getItem("tm_device_proof") || "";
      if (!fp && !proof) return init;
      init = init || {};
      var headers = new Headers(init.headers || (input && input.headers) || undefined);
      if (fp && !headers.get("X-Device-Fp")) headers.set("X-Device-Fp", fp);
      if (proof && !headers.get("X-Device-Proof")) headers.set("X-Device-Proof", proof);
      init.headers = headers;
    } catch (e) {}
    return init;
  }

  var __fetch = window.fetch;
  window.fetch = function(input, init) {
    if (typeof input === "string") input = proxyUrl(input);
    else if (input && input.url) {
      var proxied = proxyUrl(input.url);
      if (proxied !== input.url) input = new Request(proxied, input);
    }
    init = stampDeviceHeaders(init, input);
    return __fetch.call(this, input, init);
  };

  var __xhrOpen = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function(method, url) {
    var args = Array.prototype.slice.call(arguments);
    if (typeof url === "string") args[1] = proxyUrl(url);
    else if (url && typeof url.toString === "function") args[1] = proxyUrl(url.toString());
    return __xhrOpen.apply(this, args);
  };
  // Angular hardcodes baseUrl https://my.zonguru.com — if a bundle skips rewrite,
  // absolute calls must still stay on this proxy (CORS to upstream fails otherwise).
  try {
    console.log("[ZonGuru Proxy] API base forced to", REAL_PROXY_ORIGIN);
  } catch (eBase) {}

  var __beacon = navigator.sendBeacon && navigator.sendBeacon.bind(navigator);
  if (__beacon) {
    navigator.sendBeacon = function(url, data) {
      if (typeof url === "string") url = proxyUrl(url);
      return __beacon(url, data);
    };
  }

  // ─── Cloudflare Turnstile bypass ─────────────────────────────────────────
  // push.my.zonguru.com WebSocket requires a valid Cloudflare Turnstile token.
  // We mock the turnstile API so ZonGuru JS thinks verification passed.
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
    console.log("[ZonGuru Proxy] Cloudflare Turnstile bypassed");
  }

  // Intercept fetch calls to challenges.cloudflare.com — return fake success
  var __fetch = window.fetch;
  window.fetch = function(input, init) {
    var url = typeof input === "string" ? input : (input && input.url ? input.url : "");
    if (url && url.indexOf("challenges.cloudflare.com") !== -1) {
      console.log("[ZonGuru Proxy] Blocked CF challenge fetch:", url);
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
  // Any #!/profile/* + auth pages. Angular ui-router states: app.profile.* / unlogged.*
  var BLOCKED_HASH = [
    "/profile",
    "/login",
    "/forgot-password",
    "/reset-password",
    "/promotion/",
    "/coupon/"
  ];
  var HOME_HASH = "#!/dashboard";
  var HOME_STATE = "app.dashboard";
  var _nativeHashDesc = Object.getOwnPropertyDescriptor(Location.prototype, "hash");
  function stripHash(h) {
    h = String(h || "").toLowerCase().split("?")[0];
    if (h.charAt(0) === "#") h = h.slice(1);
    if (h.charAt(0) === "!") h = h.slice(1);
    if (h.charAt(0) !== "/") h = "/" + h;
    return h;
  }
  function normHash() {
    try { return stripHash(window.location.hash); } catch (e0) { return "/"; }
  }
  function hashBlocked(h) {
    h = stripHash(h);
    for (var i = 0; i < BLOCKED_HASH.length; i++) {
      var b = BLOCKED_HASH[i];
      if (h === b || h.indexOf(b + "/") === 0 || (b.charAt(b.length - 1) === "/" && h.indexOf(b) === 0)) return true;
    }
    return false;
  }
  function blockedHash() { return hashBlocked(normHash()); }
  function stateBlocked(name) {
    name = String(name || "");
    return name.indexOf("app.profile") === 0 || name.indexOf("unlogged") === 0;
  }
  // Refuse profile hash writes (Angular otherwise overwrites our redirect)
  try {
    if (_nativeHashDesc && _nativeHashDesc.set && _nativeHashDesc.get) {
      Object.defineProperty(Location.prototype, "hash", {
        configurable: true,
        enumerable: true,
        get: function() { return _nativeHashDesc.get.call(this); },
        set: function(v) {
          if (hashBlocked(v)) v = HOME_HASH;
          return _nativeHashDesc.set.call(this, v);
        }
      });
    }
  } catch (eHashLock) {}
  function setHashNative(v) {
    try {
      if (_nativeHashDesc && _nativeHashDesc.set) _nativeHashDesc.set.call(window.location, v);
      else window.location.hash = v;
    } catch (eSet) {}
  }
  function goHome() {
    if (normHash().indexOf("/dashboard") === 0 && !blockedHash()) return;
    setHashNative(HOME_HASH);
    try { history.replaceState(null, "", "/" + HOME_HASH); } catch (eR) {}
    try {
      if (window.angular) {
        var root = document.querySelector("[ng-app],.ng-scope") || document.body;
        var inj = window.angular.element(root).injector();
        if (inj) {
          var $state = inj.get("$state");
          if ($state && typeof $state.go === "function") {
            $state.go(HOME_STATE, {}, { location: "replace", reload: false });
          }
        }
      }
    } catch (eAng) {}
  }
  function guardNav() {
    if (blockedHash()) goHome();
  }
  function hookAngular() {
    try {
      if (!window.angular || window.__zgAngularHooked) return !!window.__zgAngularHooked;
      var root = document.querySelector("[ng-app],.ng-scope") || document.body;
      var inj = window.angular.element(root).injector();
      if (!inj) return false;
      window.__zgAngularHooked = true;
      var $rootScope = inj.get("$rootScope");
      var $state = inj.get("$state");
      $rootScope.$on("$locationChangeStart", function(e, newUrl) {
        var h = "";
        try { h = (new URL(newUrl, REAL_PROXY_ORIGIN)).hash; } catch (x) { h = String(newUrl || ""); }
        if (hashBlocked(h)) {
          e.preventDefault();
          goHome();
        }
      });
      $rootScope.$on("$stateChangeStart", function(e, toState) {
        if (!stateBlocked(toState && toState.name)) return;
        e.preventDefault();
        try { $state.go(HOME_STATE, {}, { location: "replace" }); } catch (x) { goHome(); }
      });
      try {
        if (stateBlocked($state.current && $state.current.name)) {
          $state.go(HOME_STATE, {}, { location: "replace" });
        }
      } catch (x2) {}
      console.log("[ZonGuru Proxy] Angular profile/login states blocked");
      return true;
    } catch (eHook) { return false; }
  }
  guardNav();
  window.addEventListener("hashchange", guardNav, true);
  window.addEventListener("popstate", guardNav, true);
  setInterval(function() { guardNav(); hookAngular(); }, 200);

  try {
    var _push = history.pushState.bind(history);
    var _replace = history.replaceState.bind(history);
    function histHash(url) {
      if (url == null) return "";
      var s = String(url);
      var i = s.indexOf("#");
      return i >= 0 ? s.slice(i) : s;
    }
    history.pushState = function(state, title, url) {
      if (url != null && hashBlocked(histHash(url))) url = "/" + HOME_HASH;
      return _push(state, title, url);
    };
    history.replaceState = function(state, title, url) {
      if (url != null && hashBlocked(histHash(url))) url = "/" + HOME_HASH;
      return _replace(state, title, url);
    };
  } catch (eHist) {}

  document.addEventListener("click", function(e) {
    var a = e.target && e.target.closest ? e.target.closest("a,button,[ui-sref],md-menu-item,.md-button") : null;
    if (!a) return;
    var href = (a.getAttribute("href") || a.getAttribute("ui-sref") || "").toLowerCase();
    var cls = (a.className && a.className.toString ? a.className.toString() : "").toLowerCase();
    var txt = ((a.innerText || a.textContent || "") + "").replace(/\s+/g, " ").trim().toLowerCase();
    var hit = href.indexOf("profile") !== -1 || href.indexOf("assist-user") !== -1 || href.indexOf("unlogged") !== -1;
    if (cls.indexOf("bdd-profile") !== -1 || cls.indexOf("logout") !== -1) hit = true;
    if (txt === "logout" || txt === "log out") hit = true;
    if (!hit) return;
    e.preventDefault();
    e.stopPropagation();
    e.stopImmediatePropagation();
    goHome();
  }, true);

  // Hide only the header user/settings dropdown (.top-menu) — profile/logout options gone
  try {
    var st = document.createElement("style");
    st.setAttribute("data-zonguru-hide", "1");
    st.textContent = ".top-menu{display:none!important;visibility:hidden!important;pointer-events:none!important;}";
    (document.head || document.documentElement).appendChild(st);
  } catch (eCss) {}

  // Logout has no API — it only clears localStorage token/me then goes to #!/login.
  // Re-hydrate auth keys if cleared; keep user on dashboard.
  try {
    var AUTH_KEYS = { token: 1, me: 1 };
    var _rm = Storage.prototype.removeItem;
    Storage.prototype.removeItem = function(k) {
      if (this === window.localStorage && AUTH_KEYS[k]) {
        try { console.log("[ZonGuru Proxy] blocked localStorage.removeItem(" + k + ")"); } catch (e0) {}
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

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func proxyHostPort(cfg Config) string {
	host := strings.TrimSpace(cfg.PublicHost)
	if host == "" {
		return net.JoinHostPort("127.0.0.1", cfg.Port)
	}
	if strings.Contains(host, ":") {
		return host
	}
	// Production domains sit behind nginx — never rewrite to :4691 (browser can't reach it).
	if host == "127.0.0.1" || host == "localhost" || strings.HasSuffix(host, ".local") {
		return net.JoinHostPort(host, cfg.Port)
	}
	return host
}

func rewriteBody(body []byte, cfg Config, contentType string) []byte {
	s := string(body)
	origin := proxyOrigin(cfg)
	hostPort := proxyHostPort(cfg)
	tHost := targetHost(cfg)
	wsOrigin := strings.Replace(origin, "http", "ws", 1)
	ct := strings.ToLower(contentType)

	// JSON APIs must stay byte-stable — DigitaVision/email rewrites corrupted
	// /api/user/me (digitavision…@ → ToolsMandi…@) and broke dashboard tiles.
	isJSON := strings.Contains(ct, "json")
	isJS := strings.Contains(ct, "javascript") || strings.Contains(ct, "ecmascript")
	isHTMLDoc := isHTML(ct)

	if !isJSON {
		replacements := [][2]string{
			{"https://" + tHost, origin},
			{"http://" + tHost, origin},
			{"//" + tHost, "//" + hostPort},
			{"wss://" + tHost, wsOrigin},
			{"ws://" + tHost, wsOrigin},
			{"https://www.zonguru.com", origin},
			{"http://www.zonguru.com", origin},
			// Angular config hardcodes these — must become the proxy origin.
			{`baseUrl:"https://` + tHost + `"`, `baseUrl:"` + origin + `"`},
			{`baseUrl:"http://` + tHost + `"`, `baseUrl:"` + origin + `"`},
			{`cookieDomain:".zonguru.com"`, `cookieDomain:"` + cfg.PublicHost + `"`},
		}
		for _, d := range cfg.ExtraDomains {
			replacements = addSubdomainRewrites(replacements, cfg, d)
		}
		for _, pair := range replacements {
			s = strings.ReplaceAll(s, pair[0], pair[1])
		}
	}

	// Brand rename only in HTML chrome — never lowercase (emails are digitavision…).
	if isHTMLDoc {
		for _, pair := range [][2]string{
			{"DigitaVision", "ToolsMandi"},
			{"Digitavision", "ToolsMandi"},
		} {
			s = strings.ReplaceAll(s, pair[0], pair[1])
		}
		s = integrityRegex.ReplaceAllString(s, "")
		s = regexp.MustCompile(`(?i)<meta[^>]+http-equiv=["']Content-Security-Policy["'][^>]*>`).ReplaceAllString(s, "")
	}
	if isJS || isHTMLDoc {
		if cfg.PublicScheme == "http" {
			s = strings.ReplaceAll(s, "wss://"+hostPort, "ws://"+hostPort)
		}
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
		// Ensure FbaToken on API calls even if Angular race missed localStorage hydrate.
		if tok := fbaTokenFromSession(session); tok != "" && req.Header.Get("FbaToken") == "" {
			if zgIsAPIPath(req.URL.Path) || req.Header.Get("Sec-Fetch-Dest") == "empty" {
				req.Header.Set("FbaToken", tok)
			}
		}
		if cookieHdr, _ := parseSessionStorage(session); cookieHdr != "" {
			existing := req.Header.Get("Cookie")
			if existing != "" {
				req.Header.Set("Cookie", existing+"; "+cookieHdr)
			} else {
				req.Header.Set("Cookie", cookieHdr)
			}
		}
		zgOrigin := upstreamOrigin(cfg)
		if ref := req.Header.Get("Referer"); ref != "" {
			req.Header.Set("Referer", strings.ReplaceAll(ref, proxyOrigin(cfg), zgOrigin))
			req.Header.Set("Referer", strings.ReplaceAll(req.Header.Get("Referer"), "http://"+cfg.PublicHost, zgOrigin))
			req.Header.Set("Referer", strings.ReplaceAll(req.Header.Get("Referer"), "https://"+cfg.PublicHost, zgOrigin))
		} else if target.Host != targetHost(cfg) {
			req.Header.Set("Referer", zgOrigin+"/")
		}
		if origin := req.Header.Get("Origin"); origin != "" {
			req.Header.Set("Origin", zgOrigin)
		} else if target.Host != targetHost(cfg) {
			req.Header.Set("Origin", zgOrigin)
		}
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		if resp.StatusCode == http.StatusSwitchingProtocols {
			return nil
		}
		session := panelSessionFor(resp.Request, getSession)
		if resp.Request != nil && resp.StatusCode >= 400 {
			p := resp.Request.URL.Path
			if zgIsAPIPath(p) || resp.StatusCode == 401 || resp.StatusCode == 403 {
				log.Printf("[UPSTREAM] %s %s -> %d (FbaToken=%v Accept=%q)", resp.Request.Method, p, resp.StatusCode, resp.Request.Header.Get("FbaToken") != "", resp.Request.Header.Get("Accept"))
			}
		}
		ct := resp.Header.Get("Content-Type")
		enc := resp.Header.Get("Content-Encoding")
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		plain, err := decompressBody(body, enc)
		if err != nil {
			plain = body
		}
		if isRewritable(ct) {
			plain = rewriteBody(plain, cfg, ct)
			if cfg.CloudflareBypass {
				plain = stripCloudflareChallengeHTML(plain, cfg)
			}
			if isHTML(ct) && isCloudflareBlockPage(plain) {
				log.Printf("[CF] Upstream returned Cloudflare block page for %s", resp.Request.URL.Path)
			}
			if isHTML(ct) {
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
		if resp.Request != nil && strings.HasPrefix(strings.ToLower(resp.Request.URL.Path), "/api/dashboard/") {
			log.Printf("[DASH] %s %s -> %d bytes=%d", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode, len(plain))
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
		log.Printf("[SESSION] Loaded ZonGuru token (%d chars)", len(token))
	} else {
		log.Printf("[WARN] No token key found in cookie.txt localStorage export")
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

		acc, nextReq, handled := preparePanelRequest(w, r, cfg)
		if handled {
			return
		}
		if usesPanelAccountMode(cfg) {
			r = stampPanelAccount(nextReq, acc)
		}


		path := strings.ToLower(r.URL.Path)
		for _, blocked := range cfg.BlockedPaths {
			b := strings.ToLower(strings.TrimSpace(blocked))
			if b != "" && (path == b || strings.HasPrefix(path, b+"/")) {
				http.Redirect(w, r, "/#!/dashboard", http.StatusFound)
				return
			}
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

	authMode := "localStorage via cookie.txt"
	if usesPanelAccountMode(cfg) {
		authMode = "panel.db GoAuto localStorage (token+me)"
	}
	log.Printf("╔══════════════════════════════════════════════╗")
	log.Printf("║  ZonGuru Go Proxy                              ║")
	log.Printf("║  URL:    %s://%s", cfg.PublicScheme, cfg.PublicHost)
	log.Printf("║  Target: %s", cfg.TargetURL)
	log.Printf("║  Auth:   %s", authMode)
	log.Printf("╚══════════════════════════════════════════════╝")
	if usesPanelAccountMode(cfg) && session == "" {
		log.Printf("[INFO] cookie.txt empty is OK in panel mode — paste GoAuto localStorage into the panel account cookie")
	}

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("[FATAL] %v", err)
	}
}
