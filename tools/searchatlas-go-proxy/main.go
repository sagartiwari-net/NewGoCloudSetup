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
	Port             string   `json:"port"`
	TargetURL        string   `json:"target_url"`
	PublicHost       string   `json:"public_host"`
	PublicScheme     string   `json:"public_scheme"`
	CookieFile       string   `json:"cookie_file"`
	LocalTestMode    bool     `json:"local_test_mode"`
	BindLocalhost    bool     `json:"bind_localhost"`
	DebugLog         bool     `json:"debug_log"`
	ToolName         string   `json:"tool_name"`
	ExtraDomains     []string `json:"extra_domains"`
	BlockedPaths     []string `json:"blocked_paths"`
	UserAgent        string   `json:"user_agent"`
	CloudflareBypass bool     `json:"cloudflare_bypass"`
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
		cfg.Port = "4711"
	}
	if cfg.TargetURL == "" {
		cfg.TargetURL = "https://dashboard.searchatlas.com"
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
		cfg.ExtraDomains = defaultSearchAtlasExtraDomains()
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
		return "dashboard.searchatlas.com"
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
              try {
                var path = s;
                if (s.indexOf("://") !== -1) path = new URL(s, REAL_PROXY_ORIGIN).pathname;
                else if (s.charAt(0) === "/") path = s.split("?")[0].split("#")[0];
                var blocked = ["/login","/register","/signup","/logout","/pricing","/billing","/checkout","/subscription","/auth/login"];
                var pl = String(path || "").toLowerCase();
                for (var bi = 0; bi < blocked.length; bi++) {
                  var bp = blocked[bi];
                  if (pl === bp || pl.indexOf(bp + "/") === 0) {
                    try { console.log("[SearchAtlas Proxy] blocked location.href", path); } catch (eB0) {}
                    return;
                  }
                }
              } catch (ePath) {}
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
	extraForJS := make([]string, 0, len(cfg.ExtraDomains))
	for _, d := range cfg.ExtraDomains {
		if skipURLRewrite(d) {
			continue
		}
		extraForJS = append(extraForJS, d)
	}
	extraDomains, _ := json.Marshal(extraForJS)
	tHost := targetHost(cfg)

	return `<script data-searchatlas-proxy="1">
(function(){
  // Production leaves local_test_mode off, so the host-spoof block is empty.
  // proxyUrl still reads these; an undefined origin throws and the dashboard
  // fetch never leaves the browser (the star loader never finishes).
  var REAL_PROXY_ORIGIN = window.location.origin;
  var REAL_PROXY_HOST = window.location.host;
  var REAL_PROXY_PROTOCOL = window.location.protocol;
` + localFix + `
  var EXTRA_DOMAINS = ` + string(extraDomains) + `;
  var TARGET_HOST = ` + jsonString(tHost) + `;
  function proxyUrl(url) {
    if (typeof url !== "string") return url;
    try { return proxyUrlInner(url); } catch (e) { return url; }
  }
  function proxyUrlInner(url) {
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
      if (/\.firebaseio\.com$/i.test(d) || /\.firebasedatabase\.app$/i.test(d) || /\.firebaseapp\.com$/i.test(d)) continue;
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
      if (h === TARGET_HOST || h === "dashboard.searchatlas.com" || h === "www.searchatlas.com" || h === "searchatlas.com") {
        if (parsed.protocol === "ws:" || parsed.protocol === "wss:") {
          return REAL_PROXY_ORIGIN.replace(/^http/, "ws") + parsed.pathname + parsed.search + parsed.hash;
        }
        return REAL_PROXY_ORIGIN + parsed.pathname + parsed.search + parsed.hash;
      }
      if (h.endsWith(".firebaseio.com") || h.endsWith(".firebasedatabase.app") || h.endsWith(".firebaseapp.com")) {
        return url; // Firebase RTDB crashes if URL is rewritten through /ext-proxy/
      }
      if (h.endsWith(".searchatlas.com") || h.indexOf("intercom") !== -1 || h.indexOf("stripe.com") !== -1 || h.indexOf("posthog.com") !== -1 || h.indexOf("userpilot") !== -1 || h.indexOf("flagcdn.com") !== -1) {
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
      console.log("[SearchAtlas Proxy] Restored localStorage (" + Object.keys(storageData).length + " keys)");
    }
  } catch (e) {
    console.warn("[SearchAtlas Proxy] localStorage restore failed", e);
  }

  // Firebase RTDB: keep host values as bare *.firebaseio.com (never /ext-proxy/...)
  try {
    for (var fi = localStorage.length - 1; fi >= 0; fi--) {
      var fk = localStorage.key(fi);
      if (!fk || fk.indexOf("firebase:host:") !== 0) continue;
      var fv = String(localStorage.getItem(fk) || "");
      if (fv.indexOf("/ext-proxy/") !== -1) {
        fv = fv.split("/ext-proxy/").pop() || fv;
      }
      fv = fv.replace(/^https?:\/\//i, "").split("/")[0];
      if (fv) localStorage.setItem(fk, fv);
    }
  } catch (eFbHost) {}

  function authToken() {
    try {
      var t = localStorage.getItem("token") || "";
      t = String(t).replace(/^Bearer\s+/i, "").trim();
      return t;
    } catch (e) { return ""; }
  }
  function bearerValue() {
    var tok = authToken();
    return tok ? ("Bearer " + tok) : "";
  }
  function normalizeAuthHeader(v) {
    var s = String(v || "").trim();
    if (!s) return bearerValue();
    // Strip repeated "Bearer" prefixes / comma-joined duplicates from XHR merges
    s = s.replace(/Bearer\s+/gi, "").replace(/,/g, " ").trim();
    var parts = s.split(/\s+/).filter(Boolean);
    var tok = parts.length ? parts[parts.length - 1] : "";
    if (!tok) tok = authToken();
    return tok ? ("Bearer " + tok) : "";
  }
  function urlNeedsBearer(u) {
    try {
      var s = String(u || "");
      if (!s) return false;
      if (s.indexOf("/_next/static") !== -1 || s.indexOf("static-assets") !== -1) return false;
      if (s.indexOf("/api/") !== -1) return true;
      var m = s.match(/\/ext-proxy\/([^/?#]+)/);
      var h = m ? m[1].toLowerCase() : "";
      if (!h) {
        try { h = new URL(s, REAL_PROXY_ORIGIN).hostname.toLowerCase(); } catch (e0) { return false; }
      }
      if (!h) return false;
      if (h.indexOf("static-assets") !== -1 || h.indexOf("static.") === 0 || h.indexOf("stripe") !== -1 || h.indexOf("posthog") !== -1 || h.indexOf("userpilot") !== -1 || h.indexOf("intercom") !== -1 || h.indexOf("firebase") !== -1) return false;
      if (h === "api.searchatlas.com" || h === "keyword.searchatlas.com" || h === "sa.searchatlas.com" || h.indexOf("llmvis") !== -1 || h.indexOf("backlink") !== -1 || h.indexOf("gsc.") === 0 || h.indexOf("agent.searchatlas") !== -1) return true;
      if (h.endsWith(".searchatlas.com") && h.indexOf("dashboard") === -1 && h.indexOf("assets") === -1 && h.indexOf("cdn") === -1) return true;
      return false;
    } catch (e) { return false; }
  }
  function withBearer(init, url) {
    if (!urlNeedsBearer(url)) return init;
    var tok = authToken();
    if (!tok) return init;
    init = init ? Object.assign({}, init) : {};
    var headers = init.headers;
    if (headers && typeof Headers !== "undefined" && headers instanceof Headers) {
      headers = new Headers(headers);
      var cur = headers.get("Authorization") || headers.get("authorization") || "";
      headers.set("Authorization", normalizeAuthHeader(cur || ("Bearer " + tok)));
      init.headers = headers;
      return init;
    }
    var h = {};
    if (headers && typeof headers === "object") {
      for (var k in headers) {
        if (Object.prototype.hasOwnProperty.call(headers, k)) h[k] = headers[k];
      }
    }
    var cur2 = "";
    for (var ak in h) {
      if (String(ak).toLowerCase() === "authorization") { cur2 = h[ak]; delete h[ak]; break; }
    }
    h["Authorization"] = normalizeAuthHeader(cur2 || ("Bearer " + tok));
    init.headers = h;
    return init;
  }
  function readDevice() {
    var fp = "", proof = "";
    try {
      fp = localStorage.getItem("tm_device_fp") || sessionStorage.getItem("tm_device_fp") || "";
      proof = localStorage.getItem("tm_device_proof") || "";
    } catch (e) {}
    return { fp: fp, proof: proof };
  }
  function sameOriginUrl(u) {
    try { return new URL(String(u || ""), location.href).origin === location.origin; } catch (e) { return false; }
  }
  // Device patch only stamps headers when the URL is already same-origin.
  // API calls start as https://api.searchatlas.com and become /ext-proxy/ here,
  // after that check, so they must be stamped again or the gate returns 401.
  function withDevice(init, url) {
    if (!sameOriginUrl(url)) return init;
    var d = readDevice();
    if (!d.fp || !d.proof) return init;
    init = init ? Object.assign({}, init) : {};
    var headers = init.headers;
    if (headers && typeof Headers !== "undefined" && headers instanceof Headers) {
      headers = new Headers(headers);
      if (!headers.get("X-Device-Fp")) headers.set("X-Device-Fp", d.fp);
      if (!headers.get("X-Device-Proof")) headers.set("X-Device-Proof", d.proof);
      init.headers = headers;
      return init;
    }
    var h = {};
    if (headers && typeof headers === "object") {
      for (var k in headers) {
        if (Object.prototype.hasOwnProperty.call(headers, k)) h[k] = headers[k];
      }
    }
    var hasFp = false, hasProof = false;
    for (var ak in h) {
      var lk = String(ak).toLowerCase();
      if (lk === "x-device-fp") hasFp = true;
      if (lk === "x-device-proof") hasProof = true;
    }
    if (!hasFp) h["X-Device-Fp"] = d.fp;
    if (!hasProof) h["X-Device-Proof"] = d.proof;
    init.headers = h;
    return init;
  }

  var __beacon = navigator.sendBeacon && navigator.sendBeacon.bind(navigator);
  if (__beacon) {
    navigator.sendBeacon = function(url, data) {
      if (typeof url === "string") url = proxyUrl(url);
      return __beacon(url, data);
    };
  }

  // Cloudflare Turnstile bypass (shared pattern)
  var _cfFakeToken = "0." + Math.random().toString(36).slice(2) + Date.now();
  if (!window.turnstile) {
    window.turnstile = {
      render: function(el, opts) {
        setTimeout(function() {
          _cfFakeToken = "0." + Math.random().toString(36).slice(2) + Date.now();
          if (opts && typeof opts.callback === "function") opts.callback(_cfFakeToken);
          try {
            var node = typeof el === "string" ? document.querySelector(el) : el;
            if (node) node.style.display = "none";
          } catch(e) {}
        }, 100);
        return "proxy-widget-" + Math.random().toString(36).slice(2);
      },
      getResponse: function() { return _cfFakeToken; },
      reset: function() {},
      remove: function() {}
    };
    console.log("[SearchAtlas Proxy] Cloudflare Turnstile bypassed");
  }

  function isTokenRefreshUrl(u) {
    try {
      var p = String(u || "");
      return p.indexOf("/api/token/refresh") !== -1;
    } catch (e) { return false; }
  }
  function refreshStubResponse() {
    var tok = authToken();
    return new Response(JSON.stringify({ token: tok }), {
      status: 200,
      headers: { "Content-Type": "application/json" }
    });
  }

  var __fetch = window.fetch;
  window.fetch = function(input, init) {
    var url = typeof input === "string" ? input : (input && input.url ? input.url : "");
    if (url && url.indexOf("challenges.cloudflare.com") !== -1) {
      console.log("[SearchAtlas Proxy] Blocked CF challenge fetch:", url);
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
    var finalUrl = typeof input === "string" ? input : (input && input.url ? input.url : url);
    // Keep session alive even if upstream refresh HTML/404s
    if (isTokenRefreshUrl(finalUrl) && authToken()) {
      return __fetch.call(this, input, withDevice(withBearer(init, finalUrl), finalUrl)).then(function(resp) {
        if (resp && resp.ok) {
          var ct = (resp.headers && resp.headers.get("content-type")) || "";
          if (ct.indexOf("application/json") !== -1) return resp;
        }
        console.log("[SearchAtlas Proxy] token refresh stubbed (upstream not JSON)");
        return refreshStubResponse();
      }).catch(function() {
        return refreshStubResponse();
      });
    }
    return __fetch.call(this, input, withDevice(withBearer(init, finalUrl), finalUrl));
  };

  var __xhrOpen = XMLHttpRequest.prototype.open;
  var __xhrSend = XMLHttpRequest.prototype.send;
  var __xhrSetHeader = XMLHttpRequest.prototype.setRequestHeader;
  XMLHttpRequest.prototype.open = function(method, url) {
    var args = arguments;
    if (typeof url === "string") args[1] = proxyUrl(url);
    else if (url && typeof url.toString === "function") args[1] = proxyUrl(url.toString());
    this.__saNeedAuth = urlNeedsBearer(typeof args[1] === "string" ? args[1] : "");
    this.__saHasAuth = false;
    this.__saUrl = typeof args[1] === "string" ? args[1] : "";
    return __xhrOpen.apply(this, args);
  };
  XMLHttpRequest.prototype.setRequestHeader = function(name, value) {
    var lk = String(name || "").toLowerCase();
    if (lk === "authorization") {
      this.__saHasAuth = true;
      if (!this.__saNeedAuth) {
        // Drop auth on static assets — CDN returns 401
        return;
      }
      value = normalizeAuthHeader(value);
    }
    // setRequestHeader appends. A second device stamp becomes "proof, proof"
    // and the gate treats it as a copied session.
    if (lk === "x-device-fp" || lk === "x-device-proof") {
      this.__saDevHdrs = this.__saDevHdrs || {};
      if (this.__saDevHdrs[lk]) return;
      this.__saDevHdrs[lk] = 1;
    }
    return __xhrSetHeader.call(this, name, value);
  };
  XMLHttpRequest.prototype.send = function() {
    try {
      if (this.__saNeedAuth && authToken() && !this.__saHasAuth) {
        __xhrSetHeader.call(this, "Authorization", bearerValue());
        this.__saHasAuth = true;
      }
      var dev = readDevice();
      if (dev.fp && dev.proof && sameOriginUrl(this.__saUrl)) {
        this.__saDevHdrs = this.__saDevHdrs || {};
        if (!this.__saDevHdrs["x-device-fp"]) __xhrSetHeader.call(this, "X-Device-Fp", dev.fp);
        if (!this.__saDevHdrs["x-device-proof"]) __xhrSetHeader.call(this, "X-Device-Proof", dev.proof);
      }
    } catch (eAuthHdr) {}
    // Axios uses XHR — stub refresh if upstream returns non-JSON
    if (isTokenRefreshUrl(this.__saUrl) && authToken()) {
      var xhr = this;
      var _done = function() {
        try {
          if (xhr.readyState !== 4) return;
          var ct = "";
          try { ct = xhr.getResponseHeader("content-type") || ""; } catch (e1) {}
          if (xhr.status >= 200 && xhr.status < 300 && ct.indexOf("application/json") !== -1) return;
          var tok = authToken();
          var body = JSON.stringify({ token: tok });
          try {
            Object.defineProperty(xhr, "status", { value: 200 });
            Object.defineProperty(xhr, "responseText", { value: body });
            Object.defineProperty(xhr, "response", { value: body });
          } catch (e2) {}
          console.log("[SearchAtlas Proxy] XHR token refresh stubbed");
        } catch (e3) {}
      };
      this.addEventListener("readystatechange", _done);
    }
    return __xhrSend.apply(this, arguments);
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
  var HOME_PATH = "/home";
  function pathOf(u) {
    try {
      if (!u) return window.location.pathname || "/";
      var s = String(u);
      if (s.charAt(0) === "/") return s.split("?")[0].split("#")[0];
      return new URL(s, REAL_PROXY_ORIGIN).pathname;
    } catch (e) { return "/"; }
  }
  function pathBlocked(p) {
    p = String(p || "").toLowerCase();
    for (var i = 0; i < BLOCKED.length; i++) {
      var b = String(BLOCKED[i] || "").toLowerCase();
      if (!b) continue;
      if (p === b || p.indexOf(b + "/") === 0) return true;
    }
    return false;
  }
  function softHome() {
    try {
      // Never reuse Next.js history state and never fire popstate —
      // foreign/empty state makes Next set __NA → window.location.reload() loop.
      if ((window.location.pathname || "") === HOME_PATH) return;
      history.replaceState(null, "", HOME_PATH);
    } catch (e) {}
  }
  function goHome() {
    try {
      if ((window.location.pathname || "") === HOME_PATH) return;
      softHome();
    } catch (e) {}
  }
  function guardNav() {
    if (pathBlocked(window.location.pathname)) goHome();
  }
  guardNav();
  // No popstate listener — it fights Next App Router and triggers hard reloads.
  // Poll only as a safety net for blocked paths.
  setInterval(guardNav, 2000);

  try {
    var _push = history.pushState.bind(history);
    var _replace = history.replaceState.bind(history);
    history.pushState = function(state, title, url) {
      if (url != null && pathBlocked(pathOf(url))) {
        // Swallow login/billing navigations — do not rewrite Next state onto /home
        try { console.log("[SearchAtlas Proxy] blocked pushState", url); } catch (e0) {}
        return;
      }
      return _push(state, title, url);
    };
    history.replaceState = function(state, title, url) {
      if (url != null && pathBlocked(pathOf(url))) {
        try { console.log("[SearchAtlas Proxy] blocked replaceState", url); } catch (e0) {}
        return;
      }
      return _replace(state, title, url);
    };
  } catch (eHist) {}

  // Block hard navigations to /login etc. (location.assign/replace/href)
  try {
    var _assign = window.location.assign.bind(window.location);
    var _replaceLoc = window.location.replace.bind(window.location);
    window.location.assign = function(u) {
      if (pathBlocked(pathOf(u))) { softHome(); return; }
      return _assign(u);
    };
    window.location.replace = function(u) {
      if (pathBlocked(pathOf(u))) { softHome(); return; }
      return _replaceLoc(u);
    };
  } catch (eLoc) {}

  // Kill Next.js __NA hard-reload loops
  try {
    var _reload = window.location.reload.bind(window.location);
    var _lastReload = 0;
    window.location.reload = function() {
      var now = Date.now();
      if (now - _lastReload < 8000) {
        try { console.log("[SearchAtlas Proxy] blocked rapid location.reload"); } catch (e0) {}
        return;
      }
      _lastReload = now;
      return _reload();
    };
  } catch (eRel) {}

  document.addEventListener("click", function(e) {
    var a = e.target && e.target.closest ? e.target.closest("a,button") : null;
    if (!a) return;
    var href = (a.getAttribute("href") || "").toLowerCase();
    var txt = ((a.innerText || a.textContent || "") + "").replace(/\s+/g, " ").trim().toLowerCase();
    var hit = false;
    if (href) {
      try { hit = pathBlocked(pathOf(href)); } catch (e1) {}
      if (href.indexOf("logout") !== -1 || href.indexOf("login") !== -1 || href.indexOf("signup") !== -1 || href.indexOf("billing") !== -1 || href.indexOf("pricing") !== -1 || href.indexOf("checkout") !== -1 || href.indexOf("subscription") !== -1) hit = true;
    }
    if (txt === "logout" || txt === "log out" || txt === "sign out") hit = true;
    if (!hit) return;
    e.preventDefault();
    e.stopPropagation();
    e.stopImmediatePropagation();
    goHome();
  }, true);

  // Block logout clearing of JWT token
  try {
    var AUTH_KEYS = { token: 1 };
    var _rm = Storage.prototype.removeItem;
    Storage.prototype.removeItem = function(k) {
      if (this === window.localStorage && AUTH_KEYS[k]) {
        try { console.log("[SearchAtlas Proxy] blocked localStorage.removeItem(" + k + ")"); } catch (e0) {}
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

// searchAtlasUserMenuScript keeps the header user button, but the account
// menu (Settings, Billing, Logout, …) does not open. The visible name is the
// panel user, not the Search Atlas account display name.
func searchAtlasUserMenuScript(panelUsername string) string {
	userJS, _ := json.Marshal(strings.TrimSpace(panelUsername))
	return `<style data-tm-sa-user>
.user-menu-overlay,[data-radix-popper-content-wrapper]:has(.user-menu-overlay){display:none!important;visibility:hidden!important;pointer-events:none!important}
</style>
<script data-tm-sa-user="1">
(function(){
  if (window.__tmSaUserMenu) return;
  window.__tmSaUserMenu = true;
  var TM_USER = ` + string(userJS) + `;
  function menuBtn(t) {
    return !!(t && t.closest && t.closest('button[aria-label="Open user menu"]'));
  }
  function block(e) {
    if (!menuBtn(e.target)) return;
    e.preventDefault();
    e.stopPropagation();
    if (e.stopImmediatePropagation) e.stopImmediatePropagation();
  }
  ["pointerdown","mousedown","mouseup","click","auxclick","keydown","keyup","touchstart","touchend"].forEach(function(type){
    document.addEventListener(type, block, true);
  });
  function hideMenus(root) {
    if (!root || !root.querySelectorAll) return;
    var menus = root.querySelectorAll(".user-menu-overlay");
    for (var i = 0; i < menus.length; i++) {
      var wrap = menus[i].closest("[data-radix-popper-content-wrapper]") || menus[i];
      wrap.style.setProperty("display", "none", "important");
    }
    var btns = root.querySelectorAll('button[aria-label="Open user menu"][aria-expanded="true"]');
    for (var b = 0; b < btns.length; b++) {
      try { btns[b].setAttribute("aria-expanded", "false"); } catch (e) {}
    }
  }
  function setPanelName(root) {
    if (!TM_USER || !root || !root.querySelectorAll) return;
    var spans = root.querySelectorAll('button[aria-label="Open user menu"] span.block');
    for (var i = 0; i < spans.length; i++) {
      if (spans[i].textContent !== TM_USER) spans[i].textContent = TM_USER;
    }
  }
  function run() {
    try { hideMenus(document); setPanelName(document); } catch (e) {}
  }
  run();
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", run);
  try {
    new MutationObserver(function(){ run(); }).observe(document.documentElement, {childList:true, subtree:true, characterData:true});
  } catch (e) {}
  setInterval(run, 500);
})();
</script>`
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
		{"https://www.searchatlas.com", origin},
		{"http://www.searchatlas.com", origin},
	}
	for _, d := range cfg.ExtraDomains {
		replacements = addSubdomainRewrites(replacements, cfg, d)
	}
	for _, pair := range replacements {
		s = strings.ReplaceAll(s, pair[0], pair[1])
	}
	// Undo accidental Firebase RTDB rewrites (dots in /ext-proxy/ path crash the SDK)
	s = undoFirebaseProxyRewrites(s, origin)
	for _, pair := range [][2]string{
		{"DigitaVision", "ToolsMandi"},
		{"Digitavision", "ToolsMandi"},
		{"digitavision", "ToolsMandi"},
	} {
		s = strings.ReplaceAll(s, pair[0], pair[1])
	}
	s = integrityRegex.ReplaceAllString(s, "")
	// Inline CSP meta blocks http://cdn.my.zonguru.com — same-origin /cdn-proxy is allowed via 'self'
	s = regexp.MustCompile(`(?i)<meta[^>]+http-equiv=["']Content-Security-Policy["'][^>]*>`).ReplaceAllString(s, "")
	if cfg.PublicScheme == "http" {
		hostPort := proxyHostPort(cfg)
		s = strings.ReplaceAll(s, "wss://"+hostPort, "ws://"+hostPort)
	}
	return []byte(s)
}

var firebaseProxyRe = regexp.MustCompile(`(?i)https?://(?:localhost|127\.0\.0\.1)(?::\d+)?/ext-proxy/([a-z0-9.-]+\.firebaseio\.com)`)
var firebaseProxyEscRe = regexp.MustCompile(`(?i)https?:\\/\\/(?:localhost|127\\.0\\.0\\.1)(?::\\d+)?\\/ext-proxy\\/([a-z0-9.-]+\\.firebaseio\\.com)`)
var firebaseDBProxyRe = regexp.MustCompile(`(?i)https?://(?:localhost|127\.0\.0\.1)(?::\d+)?/ext-proxy/([a-z0-9.-]+\.firebasedatabase\.app)`)

func undoFirebaseProxyRewrites(s, origin string) string {
	s = firebaseProxyRe.ReplaceAllString(s, "https://$1")
	s = firebaseProxyEscRe.ReplaceAllString(s, `https:\/\/$1`)
	s = firebaseDBProxyRe.ReplaceAllString(s, "https://$1")
	if origin != "" {
		s = strings.ReplaceAll(s, origin+"/ext-proxy/", "\x00EXT\x00")
		// Only restore firebase hosts; put other ext-proxy back
		reFire := regexp.MustCompile(`\x00EXT\x00([a-z0-9.-]+\.firebaseio\.com)`)
		s = reFire.ReplaceAllString(s, "https://$1")
		reFire2 := regexp.MustCompile(`\x00EXT\x00([a-z0-9.-]+\.firebasedatabase\.app)`)
		s = reFire2.ReplaceAllString(s, "https://$1")
		s = strings.ReplaceAll(s, "\x00EXT\x00", origin+"/ext-proxy/")
	}
	return s
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
	return isHTML(ct) || strings.Contains(ct, "javascript") || strings.Contains(ct, "json") || strings.Contains(ct, "text/css") || strings.Contains(ct, "text/x-component")
}

// shouldAttachBearer: JWT on static CDN chunks → 401 (blank pages). Only API hosts.
func shouldAttachBearer(host, path string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	p := strings.ToLower(path)
	if h == "" {
		return strings.HasPrefix(p, "/api/")
	}
	deny := []string{
		"static-assets", "static.searchatlas", "cdn.searchatlas", "assets.searchatlas",
		"stripe.com", "posthog.com", "userpilot", "intercom", "googleapis.com",
		"gstatic.com", "cloudflare", "flagcdn.com", "firebaseio.com", "firebasedatabase.app",
		"googletagmanager", "google-analytics", "sentry.io", "js.stripe",
	}
	for _, d := range deny {
		if strings.Contains(h, d) {
			return false
		}
	}
	if strings.HasPrefix(p, "/_next/static") || strings.HasPrefix(p, "/_next/image") {
		return false
	}
	if strings.HasPrefix(p, "/api/") {
		return true
	}
	allow := []string{
		"api.searchatlas.com", "keyword.searchatlas.com", "sa.searchatlas.com",
		"llmvis.searchatlas.com", "gsc.searchatlas.com", "backlink.searchatlas.com",
		"ca.searchatlas.com", "agent.searchatlas.com", "status.searchatlas.com",
		"ucmsi.searchatlas.com", "otto-ppc.searchatlas.com", "api.builder.searchatlas.com",
		"messaging-gateway.searchatlas.com", "connector-gateway.searchatlas.com",
		"pdf-generator.searchatlas.com", "adstudio.searchatlas.com",
	}
	for _, a := range allow {
		if h == a || strings.HasSuffix(h, "."+a) {
			return true
		}
	}
	// other *.searchatlas.com API-ish hosts (not dashboard/www/static)
	if strings.HasSuffix(h, ".searchatlas.com") {
		if strings.HasPrefix(h, "dashboard.") || h == "www.searchatlas.com" || h == "searchatlas.com" {
			return strings.HasPrefix(p, "/api/")
		}
		if strings.Contains(h, "static") || strings.Contains(h, "assets") || strings.Contains(h, "cdn") {
			return false
		}
		return true
	}
	return false
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
		// Preserve browser Accept / content negotiation BEFORE chrome document defaults.
		// api.searchatlas.com Vary: Accept — forcing text/html returns SPA HTML, not JSON.
		origAccept := req.Header.Get("Accept")
		origDest := req.Header.Get("Sec-Fetch-Dest")
		origMode := req.Header.Get("Sec-Fetch-Mode")
		origSite := req.Header.Get("Sec-Fetch-Site")
		origDirector(req)
		session := panelSessionFor(req, getSession)
		req.Host = target.Host
		applyBrowserHeaders(req, cfg)
		apiLike := strings.Contains(strings.ToLower(origAccept), "application/json") ||
			origDest == "empty" || origMode == "cors" ||
			strings.HasPrefix(req.URL.Path, "/api/") ||
			req.Method == http.MethodPost || req.Method == http.MethodPut ||
			req.Method == http.MethodPatch || req.Method == http.MethodDelete
		if apiLike {
			if origAccept != "" {
				req.Header.Set("Accept", origAccept)
			} else {
				req.Header.Set("Accept", "application/json, text/plain, */*")
			}
			req.Header.Set("Sec-Fetch-Dest", "empty")
			req.Header.Set("Sec-Fetch-Mode", "cors")
			if origSite != "" {
				req.Header.Set("Sec-Fetch-Site", origSite)
			} else {
				req.Header.Set("Sec-Fetch-Site", "same-site")
			}
			req.Header.Del("Sec-Fetch-User")
			req.Header.Del("Upgrade-Insecure-Requests")
		} else if req.Method != http.MethodGet && req.Method != http.MethodHead {
			req.Header.Set("Sec-Fetch-Dest", "empty")
			req.Header.Set("Sec-Fetch-Mode", "cors")
			req.Header.Set("Sec-Fetch-Site", "same-origin")
		}
		// Auth is JWT Bearer — drop browser Cookie (localhost shares cookies
		// across all ports; other tools' huge cookies → upstream/Go HTTP 431).
		req.Header.Del("Cookie")
		if cookieHdr, _ := parseSessionStorage(session); cookieHdr != "" && len(cookieHdr) < 4096 {
			req.Header.Set("Cookie", cookieHdr)
		}
		// Normalize Authorization only for API hosts.
		// Sending Bearer JWT to static-assets CDN → 401 on turbopack chunks → blank page.
		auth := req.Header.Get("Authorization")
		if !shouldAttachBearer(target.Host, req.URL.Path) {
			req.Header.Del("Authorization")
		} else {
			tok := sessionToken(session)
			if auth != "" {
				var last string
				for _, p := range strings.Fields(strings.ReplaceAll(auth, ",", " ")) {
					if strings.EqualFold(p, "Bearer") {
						continue
					}
					last = p
				}
				if last != "" {
					tok = last
				}
			}
			tok = strings.TrimSpace(tok)
			if strings.HasPrefix(strings.ToLower(tok), "bearer ") {
				tok = strings.TrimSpace(tok[7:])
			}
			if tok != "" {
				req.Header.Set("Authorization", "Bearer "+tok)
			} else {
				req.Header.Del("Authorization")
			}
		}
		zgOrigin := upstreamOrigin(cfg)
		if ref := req.Header.Get("Referer"); ref != "" {
			req.Header.Set("Referer", strings.ReplaceAll(ref, proxyOrigin(cfg), zgOrigin))
		} else {
			req.Header.Set("Referer", zgOrigin+"/home")
		}
		if origin := req.Header.Get("Origin"); origin != "" {
			req.Header.Set("Origin", zgOrigin)
		} else {
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
		plain, err := decompressBody(body, enc)
		if err != nil {
			plain = body
		}
		if resp.StatusCode >= 400 && resp.Request != nil && strings.Contains(resp.Request.URL.Path, "/api/agent/") {
			msg := string(plain)
			if len(msg) > 300 {
				msg = msg[:300]
			}
			log.Printf("[AGENT] %s %s -> %d %s", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode, msg)
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
			apiReq := resp.Request != nil && strings.Contains(resp.Request.URL.Path, "/api/")
			if isHTML(ct) && !apiReq {
				if usesPanelAccountMode(cfg) {
					plain = injectDeviceHTML(plain)
				}
				inject := buildInjectScript(cfg, session)
				if usesPanelAccountMode(cfg) && resp.Request != nil {
					if panelUser, err := panelSessionUsername(resp.Request); err == nil {
						inject += searchAtlasUserMenuScript(panelUser)
					}
				}
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
		if key := staticCacheKeyFrom(resp.Request); key != "" && resp.StatusCode == http.StatusOK {
			resp.Header.Set("Cache-Control", "public, max-age=86400")
			resp.Header.Del("Pragma")
			resp.Header.Del("Expires")
			putStaticCached(key, ct, resp.StatusCode, plain)
		}
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
		resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(plain)))
		resp.Body = io.NopCloser(bytes.NewReader(plain))
		return nil
	}
	return proxy
}

func dashboardLocalAPI(path string) bool {
	p := path
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	if p == "/api/countries" || strings.HasPrefix(p, "/api/countries/") {
		return true
	}
	for _, pre := range []string{"/api/chatwoot/", "/api/mcp-connect/", "/api/project-creation/"} {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	return false
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

// slimOversizedCookies keeps ct_session / tm_* when the browser jar is huge.
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

func main() {
	cfg := loadConfig()
	session := loadSession(cfg)
	if session == "" {
		log.Printf("[WARN] cookie.txt is empty — paste GoAuto localStorage export before testing")
	} else if token := sessionToken(session); token != "" {
		log.Printf("[SESSION] Loaded SearchAtlas token (%d chars)", len(token))
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

		// Home redirect: / → /home
		if r.URL.Path == "/" || r.URL.Path == "" {
			http.Redirect(w, r, "/home", http.StatusFound)
			return
		}

		// Log SPA runtime errors (helps debug client loops)
		if strings.Contains(r.URL.Path, "runtime-errors") && r.Method == http.MethodPost {
			body, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
			r.Body = io.NopCloser(bytes.NewReader(body))
			if len(body) > 0 {
				msg := string(body)
				if len(msg) > 800 {
					msg = msg[:800]
				}
				log.Printf("[RUNTIME-ERROR] %s", msg)
			}
		}

		path := strings.ToLower(r.URL.Path)
		for _, blocked := range cfg.BlockedPaths {
			b := strings.ToLower(strings.TrimSpace(blocked))
			if b != "" && (path == b || strings.HasPrefix(path, b+"/")) {
				// Rewrite to /home in-place (no 302) — 302 /login→/home causes reload loops
				// when the SPA hard-navigates to /login after a failed auth check.
				r2 := r.Clone(r.Context())
				r2.URL.Path = "/home"
				r2.URL.RawPath = ""
				r2.URL.RawQuery = ""
				debugLog(cfg, "blocked %s → serve /home", r.URL.Path)
				if isWebSocket(r2) {
					handleWebSocket(w, r2, cfg, targetHost(cfg), r2.URL.Path, getSession)
					return
				}
				newReverseProxy(targetURL, cfg, getSession).ServeHTTP(w, r2)
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
			if serveCachedStatic(w, r) {
				return
			}
			r2 := r.Clone(r.Context())
			r2.URL.Path = extPath
			r2.URL.RawPath = ""
			r2.Host = extHost
			r2 = tagStaticCache(r2, r)
			debugLog(cfg, "%s /ext-proxy/%s%s", r.Method, extHost, extPath)
			newReverseProxy(extTarget, cfg, getSession).ServeHTTP(w, r2)
			return
		}

		// Dashboard-relative /api/* → api.searchatlas.com (Next rewrites + axios fallbacks).
		// A few routes are Next handlers on the dashboard itself. Sending those to
		// api.searchatlas.com returns the HTML "Not Found" page (countries, etc.).
		if strings.HasPrefix(r.URL.Path, "/api/") && !dashboardLocalAPI(r.URL.Path) {
			apiHost := "api.searchatlas.com"
			apiTarget, err := url.Parse("https://" + apiHost)
			if err == nil {
				debugLog(cfg, "%s %s → %s", r.Method, r.URL.RequestURI(), apiHost)
				r2 := r.Clone(r.Context())
				r2.Host = apiHost
				newReverseProxy(apiTarget, cfg, getSession).ServeHTTP(w, r2)
				return
			}
		}

		debugLog(cfg, "%s %s", r.Method, r.URL.RequestURI())
		if isWebSocket(r) {
			handleWebSocket(w, r, cfg, targetHost(cfg), r.URL.Path, getSession)
			return
		}
		if serveCachedStatic(w, r) {
			return
		}
		newReverseProxy(targetURL, cfg, getSession).ServeHTTP(w, tagStaticCache(r, r))
	})

	addr := ":" + cfg.Port
	if cfg.BindLocalhost {
		addr = "127.0.0.1:" + cfg.Port
	}

	log.Printf("╔══════════════════════════════════════════════╗")
	log.Printf("║  SearchAtlas Go Proxy — LOCAL                      ║")
	log.Printf("║  URL:    %s://%s", cfg.PublicScheme, cfg.PublicHost)
	log.Printf("║  Target: %s", cfg.TargetURL)
	log.Printf("║  Auth:   localStorage via cookie.txt         ║")
	log.Printf("╚══════════════════════════════════════════════╝")
	startCDNCacheSweep()

	// Wrap: slim huge Cookie jars — NEVER delete the whole header.
	// Dropping Cookie wiped ct_session → /api/device-bind failed → Access Denied
	// on every fresh access link once the jar passed 8KB.
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
