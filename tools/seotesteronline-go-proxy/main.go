package main

import (
	"bytes"
	"compress/gzip"
	"context"
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

type proxyCtxKey int

const ctxBrowserOrigin proxyCtxKey = 1

type ReplacementPair struct {
	Search  string `json:"search"`
	Replace string `json:"replace"`
}

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
	// InjectCSS: injected into HTML <head> (e.g. hide workspace selector only).
	InjectCSS string `json:"inject_css"`
	// Replacements: DigitaVision→ToolsMandi etc. Applied outside JWTs; brand
	// tokens use word-boundary so emails like digitavision220926@… stay intact.
	Replacements []ReplacementPair `json:"replacements"`
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
		cfg.Port = "4801"
	}
	if cfg.TargetURL == "" {
		cfg.TargetURL = "https://suite.seotesteronline.com"
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
		cfg.ExtraDomains = defaultSEOTesterExtraDomains()
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
		return "suite.seotesteronline.com"
	}
	return u.Host
}

func debugLog(cfg Config, format string, args ...interface{}) {
	if cfg.DebugLog {
		log.Printf("[DEBUG] "+format, args...)
	}
}

func buildHeadInject(cfg Config, session string) string {
	var b strings.Builder
	if css := strings.TrimSpace(cfg.InjectCSS); css != "" {
		b.WriteString(`<style data-seotester-proxy-css="1">`)
		b.WriteString(css)
		b.WriteString("</style>\n")
	}
	b.WriteString(buildInjectScript(cfg, session))
	return b.String()
}

func buildInjectScript(cfg Config, session string) string {
	lsJSON := localStorageJSONForBrowser(session)
	jsData := strings.ReplaceAll(string(lsJSON), "</", "<\\/")
	blocked, _ := json.Marshal(cfg.BlockedPaths)
	// Do NOT spoof location.hostname/origin/protocol to suite.seotesteronline.com.
	// Angular html5Mode + $location reconcile against the real browser URL; spoofing
	// https://suite… while the page is http://localhost:4801 leaves the SPA on the
	// loading splash forever. (Same lesson as Shortform — skip buildLocalHostFix.)
	// Keeping real localhost also lets AppController skip socket init on localhost.
	localFix := `
  var REAL_PROXY_ORIGIN = window.location.origin;
  var REAL_PROXY_HOST = window.location.host;
  var REAL_PROXY_PROTOCOL = window.location.protocol;
`
	extraForJS := make([]string, 0, len(cfg.ExtraDomains))
	for _, d := range cfg.ExtraDomains {
		if skipURLRewrite(d) {
			continue
		}
		extraForJS = append(extraForJS, d)
	}
	extraDomains, _ := json.Marshal(extraForJS)
	tHost := targetHost(cfg)

	return `<script data-seotester-proxy="1">
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
      if (h === TARGET_HOST || h === "suite.seotesteronline.com" || h === "www.seotesteronline.com" || h === "seotesteronline.com") {
        // Relative /socket.io on suite host must go to socket subdomain
        if (parsed.pathname.indexOf("/socket.io") === 0) {
          var sock = REAL_PROXY_ORIGIN + "/ext-proxy/socket.seotesteronline.com" + parsed.pathname + parsed.search + parsed.hash;
          if (parsed.protocol === "ws:" || parsed.protocol === "wss:") {
            return sock.replace(/^http/, "ws");
          }
          return sock;
        }
        if (parsed.protocol === "ws:" || parsed.protocol === "wss:") {
          return REAL_PROXY_ORIGIN.replace(/^http/, "ws") + parsed.pathname + parsed.search + parsed.hash;
        }
        return REAL_PROXY_ORIGIN + parsed.pathname + parsed.search + parsed.hash;
      }
      if (h.endsWith(".firebaseio.com") || h.endsWith(".firebasedatabase.app") || h.endsWith(".firebaseapp.com")) {
        return url; // Firebase RTDB crashes if URL is rewritten through /ext-proxy/
      }
      if (h.endsWith(".seotesteronline.com") || h.indexOf("stripe.com") !== -1 || h.indexOf("zdassets") !== -1 || h.indexOf("calendly") !== -1 || h.indexOf("amazonaws.com") !== -1) {
        var ext2 = REAL_PROXY_ORIGIN + "/ext-proxy/" + h;
        if (parsed.protocol === "ws:" || parsed.protocol === "wss:") {
          return ext2.replace(/^http/, "ws") + parsed.pathname + parsed.search + parsed.hash;
        }
        return ext2 + parsed.pathname + parsed.search + parsed.hash;
      }
    } catch (e) {}
    return url;
  }

  // Hydrate BEFORE Angular/ngStorage boot (defer bundles). Re-apply briefly so
  // ngStorage cannot win a wipe race and leave isAuthenticated() false forever.
  var storageData = ` + jsData + `;
  function hydrateLocalStorage(reason) {
    try {
      for (var key in storageData) {
        if (Object.prototype.hasOwnProperty.call(storageData, key) && storageData[key] != null) {
          localStorage.setItem(key, storageData[key]);
        }
      }
      if (reason === "init" && Object.keys(storageData).length) {
        console.log("[SEOTester Proxy] Restored localStorage (" + Object.keys(storageData).length + " keys)");
      }
    } catch (e) {
      if (reason === "init") console.warn("[SEOTester Proxy] localStorage restore failed", e);
    }
  }
  hydrateLocalStorage("init");
  try {
    document.addEventListener("DOMContentLoaded", function() { hydrateLocalStorage("dom"); });
    var _hN = 0;
    var _hI = setInterval(function() {
      hydrateLocalStorage("tick");
      if (++_hN >= 25) clearInterval(_hI);
    }, 100);
  } catch (eHydrate) {}

  // Keep __env API/socket URLs on the real page origin (localhost vs 127.0.0.1).
  // Server-side rewrite hardcodes public_host; mismatch → cross-origin CORS fail.
  function fixEnvOrigins() {
    try {
      if (!window.__env) return;
      window.__env.frontEndUrl = REAL_PROXY_ORIGIN;
      window.__env.apiGateway = REAL_PROXY_ORIGIN + "/ext-proxy/api.seotesteronline.com";
      window.__env.socketUrl = REAL_PROXY_ORIGIN + "/ext-proxy/socket.seotesteronline.com";
      window.__env.proxyIframe = REAL_PROXY_ORIGIN + "/ext-proxy/proxy.seotesteronline.com";
    } catch (eEnv) {}
  }
  fixEnvOrigins();
  try {
    document.addEventListener("DOMContentLoaded", fixEnvOrigins);
    var _eN = 0;
    var _eI = setInterval(function() {
      fixEnvOrigins();
      if (++_eN >= 40) clearInterval(_eI);
    }, 50);
  } catch (eEnv2) {}

  // Keyword Explorer awaits Google JSONP seeds before request-by-search.
  // Direct clients1.google.com often hangs/blocked → search never fires.
  // Route suggest JSONP through same-origin /ext-proxy/.
  function rewriteGoogleSuggest(url) {
    if (typeof url !== "string") return url;
    if (url.indexOf("clients1.google.com/complete/search") !== -1 ||
        url.indexOf("suggestqueries.google.com/complete/search") !== -1) {
      return proxyUrl(url);
    }
    return url;
  }
  try {
    var _setAttr = Element.prototype.setAttribute;
    Element.prototype.setAttribute = function(name, value) {
      if (String(name).toLowerCase() === "src" && typeof value === "string") {
        value = rewriteGoogleSuggest(value);
      }
      return _setAttr.call(this, name, value);
    };
    var scriptProto = window.HTMLScriptElement && HTMLScriptElement.prototype;
    if (scriptProto) {
      var srcDesc = Object.getOwnPropertyDescriptor(scriptProto, "src");
      if (srcDesc && srcDesc.set) {
        Object.defineProperty(scriptProto, "src", {
          configurable: true,
          enumerable: srcDesc.enumerable,
          get: srcDesc.get,
          set: function(v) { srcDesc.set.call(this, rewriteGoogleSuggest(v)); }
        });
      }
    }
  } catch (eScript) {}

  // Keyword Explorer: API.post(..., sanitize=true) JSON.stringify→replace→JSON.parse
  // on multi-MB request-by-search payloads freezes the main thread while
  // st-table-fluid stays on gray loading skeletons. Skip that roundtrip.
  // Also cap lists-queries follow-up (many UUIDs → huge POST + localStorage quota).
  function applyKeywordExplorerAPIPatch(API, $q, KLS) {
    try {
      if (!API || !API.post || API.__stoPatched) return !!API && !!API.__stoPatched;
      var origPost = API.post.bind(API);
      function skipSanitizeUrl(u) {
        u = String(u || "");
        return u.indexOf("keyword-explorer") !== -1 ||
          u.indexOf("lists-queries") !== -1 ||
          u.indexOf("keyword-list") !== -1;
      }
      API.post = function(url, data, sanitize, abort, cache) {
        if (sanitize && skipSanitizeUrl(url)) {
          return origPost(url, data, false, abort, cache).then(function(resp) {
            if (resp && resp.data !== undefined && (resp.status !== undefined || resp.headers)) {
              return resp.data;
            }
            return resp;
          });
        }
        return origPost(url, data, sanitize, abort, cache);
      };
      API.__stoPatched = true;
      try { window.__stoKEPatched = true; } catch (eF) {}
      if (KLS && KLS.getKeywordListByQuery && !KLS.__stoPatched && $q) {
        var origList = KLS.getKeywordListByQuery.bind(KLS);
        KLS.getKeywordListByQuery = function(t) {
          if (t && t.keywordUuids && t.keywordUuids.length > 250) {
            return $q.resolve({ data: [] });
          }
          return origList(t);
        };
        KLS.__stoPatched = true;
      }
      console.log("[SEOTester Proxy] Keyword Explorer API sanitize bypass active");
      return true;
    } catch (ePatch) {
      return false;
    }
  }
  function queueKeywordExplorerRunBlock(mod) {
    if (!mod || mod.__stoKEPatchQueued) return;
    mod.__stoKEPatchQueued = true;
    try {
      mod.run(["API", "$q", "$injector", function(API, $q, $injector) {
        var KLS = null;
        try { KLS = $injector.get("KeywordListService"); } catch (eK) {}
        applyKeywordExplorerAPIPatch(API, $q, KLS);
      }]);
    } catch (eRun) {}
  }
  // Hook angular.module so we attach run{} when "app" is created (before bootstrap).
  // setInterval alone is unreliable (background/headless timer throttling).
  function hookAngularModule() {
    try {
      if (!window.angular || !angular.module || angular.module.__stoHooked) {
        return !!(window.angular && angular.module && angular.module.__stoHooked);
      }
      var origModule = angular.module.bind(angular);
      angular.module = function(name, requires, configFn) {
        var mod = origModule.apply(angular, arguments);
        if (name === "app") queueKeywordExplorerRunBlock(mod);
        return mod;
      };
      angular.module.__stoHooked = true;
      // If app already exists, queue immediately
      try { queueKeywordExplorerRunBlock(origModule("app")); } catch (eExist) {}
      return true;
    } catch (eHook) {
      return false;
    }
  }
  function patchKeywordExplorerBinding() {
    try {
      hookAngularModule();
      if (!window.angular) return false;
      try { queueKeywordExplorerRunBlock(angular.module("app")); } catch (eMod) {}
      var roots = [document.body, document.documentElement];
      try {
        var ng = document.querySelector("[ng-app], [data-ng-app], .ng-scope");
        if (ng) roots.unshift(ng);
      } catch (eRoot) {}
      for (var ri = 0; ri < roots.length; ri++) {
        if (!roots[ri]) continue;
        var inj = angular.element(roots[ri]).injector();
        if (!inj) continue;
        try {
          var API = inj.get("API");
          var $q = inj.get("$q");
          var KLS = null;
          try { KLS = inj.get("KeywordListService"); } catch (eK2) {}
          if (applyKeywordExplorerAPIPatch(API, $q, KLS)) return true;
        } catch (eGet) {}
      }
      return !!(window.__stoKEPatched);
    } catch (ePatch) {
      return false;
    }
  }
  try {
    // Catch angular.js assignment so we wrap module() before "app" is created.
    var _stoAng = window.angular;
    Object.defineProperty(window, "angular", {
      configurable: true,
      enumerable: true,
      get: function() { return _stoAng; },
      set: function(v) {
        _stoAng = v;
        try { hookAngularModule(); } catch (eSet) {}
      }
    });
    if (_stoAng) hookAngularModule();
  } catch (eAngHook) {}
  try {
    hookAngularModule();
    var _pN = 0;
    var _pI = setInterval(function() {
      if (patchKeywordExplorerBinding() || ++_pN >= 300) clearInterval(_pI);
    }, 100);
    document.addEventListener("DOMContentLoaded", function() { patchKeywordExplorerBinding(); });
  } catch (ePatchBoot) {}

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
      var t = localStorage.getItem("accessToken") || "";
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
      // Static assets / marketing / Google — never attach JWT
      if (s.indexOf("/bundle.") !== -1 || s.indexOf("/assets/") !== -1 || s.indexOf(".css") !== -1) {
        if (s.indexOf("/api/") === -1) return false;
      }
      var m = s.match(/\/ext-proxy\/([^/?#]+)/);
      var h = m ? m[1].toLowerCase() : "";
      if (!h) {
        try { h = new URL(s, REAL_PROXY_ORIGIN).hostname.toLowerCase(); } catch (e0) { return false; }
      }
      if (!h) return false;
      if (h.indexOf("stripe") !== -1 || h.indexOf("zdassets") !== -1 || h.indexOf("calendly") !== -1 || h.indexOf("firebase") !== -1 || h.indexOf("googleapis.com") !== -1 || h.indexOf("gstatic.com") !== -1 || h.indexOf("google.com") !== -1 || h.indexOf("googletagmanager") !== -1 || h.indexOf("paypal") !== -1) return false;
      if (h === "api.seotesteronline.com" || h === "socket.seotesteronline.com" || h === "proxy.seotesteronline.com") return true;
      if (h.endsWith(".seotesteronline.com") && h.indexOf("suite.") === -1 && h.indexOf("www.") === -1 && h !== "seotesteronline.com") return true;
      // Same-origin /api/ via suite (rare) or ext-proxy already covered
      if (s.indexOf("/api/") !== -1 && (h === "suite.seotesteronline.com" || h === "localhost" || h === "127.0.0.1" || !m)) return true;
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
    console.log("[SEOTester Proxy] Cloudflare Turnstile bypassed");
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
      console.log("[SEOTester Proxy] Blocked CF challenge fetch:", url);
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
      return __fetch.call(this, input, withBearer(init, finalUrl)).then(function(resp) {
        if (resp && resp.ok) {
          var ct = (resp.headers && resp.headers.get("content-type")) || "";
          if (ct.indexOf("application/json") !== -1) return resp;
        }
        console.log("[SEOTester Proxy] token refresh stubbed (upstream not JSON)");
        return refreshStubResponse();
      }).catch(function() {
        return refreshStubResponse();
      });
    }
    return __fetch.call(this, input, withBearer(init, finalUrl));
  };

  var __xhrOpen = XMLHttpRequest.prototype.open;
  var __xhrSend = XMLHttpRequest.prototype.send;
  var __xhrSetHeader = XMLHttpRequest.prototype.setRequestHeader;
  XMLHttpRequest.prototype.open = function(method, url) {
    var args = arguments;
    if (typeof url === "string") args[1] = proxyUrl(url);
    else if (url && typeof url.toString === "function") args[1] = proxyUrl(url.toString());
    this.__stoNeedAuth = urlNeedsBearer(typeof args[1] === "string" ? args[1] : "");
    this.__stoHasAuth = false;
    this.__stoUrl = typeof args[1] === "string" ? args[1] : "";
    return __xhrOpen.apply(this, args);
  };
  XMLHttpRequest.prototype.setRequestHeader = function(name, value) {
    if (String(name || "").toLowerCase() === "authorization") {
      this.__stoHasAuth = true;
      if (!this.__stoNeedAuth) {
        // Drop auth on static assets — CDN returns 401
        return;
      }
      value = normalizeAuthHeader(value);
    }
    return __xhrSetHeader.call(this, name, value);
  };
  XMLHttpRequest.prototype.send = function() {
    try {
      if (this.__stoNeedAuth && authToken() && !this.__stoHasAuth) {
        __xhrSetHeader.call(this, "Authorization", bearerValue());
        this.__stoHasAuth = true;
      }
    } catch (eAuthHdr) {}
    // Axios uses XHR — stub refresh if upstream returns non-JSON
    if (isTokenRefreshUrl(this.__stoUrl) && authToken()) {
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
          console.log("[SEOTester Proxy] XHR token refresh stubbed");
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
  var HOME_PATH = "/";
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
      // Soft-nav to suite home without hard reload loops.
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
  // Poll as a safety net for blocked paths (Angular ui-router).
  setInterval(guardNav, 2000);

  // Soft-block signin/signup without reload loops: swallow history updates,
  // keep URL on /, and nudge ui-router back to welcome when available.
  function nudgeWelcome() {
    try {
      var inj = window.angular && window.angular.element(document.body).injector();
      if (!inj) return;
      var $state = inj.get("$state");
      if ($state && $state.current && String($state.current.name || "").indexOf("access.") === 0) {
        $state.go("app.welcome", {}, { location: "replace", notify: true });
      }
    } catch (eN) {}
  }

  try {
    var _push = history.pushState.bind(history);
    var _replace = history.replaceState.bind(history);
    history.pushState = function(state, title, url) {
      if (url != null && pathBlocked(pathOf(url))) {
        // Swallow login/billing navigations — do not rewrite Next state onto /home
        try { console.log("[SEOTester Proxy] blocked pushState", url); } catch (e0) {}
        softHome();
        setTimeout(nudgeWelcome, 0);
        return;
      }
      return _push(state, title, url);
    };
    history.replaceState = function(state, title, url) {
      if (url != null && pathBlocked(pathOf(url))) {
        try { console.log("[SEOTester Proxy] blocked replaceState", url); } catch (e0) {}
        softHome();
        setTimeout(nudgeWelcome, 0);
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

  // Dampen rapid reload loops
  try {
    var _reload = window.location.reload.bind(window.location);
    var _lastReload = 0;
    window.location.reload = function() {
      var now = Date.now();
      if (now - _lastReload < 8000) {
        try { console.log("[SEOTester Proxy] blocked rapid location.reload"); } catch (e0) {}
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

  // Block logout clearing of accessToken/session/userInfo
  try {
    var AUTH_KEYS = { accessToken: 1, session: 1, userInfo: 1, "ngStorage-currentUser": 1 };
    var _rm = Storage.prototype.removeItem;
    Storage.prototype.removeItem = function(k) {
      if (this === window.localStorage && AUTH_KEYS[k]) {
        try { console.log("[SEOTester Proxy] blocked localStorage.removeItem(" + k + ")"); } catch (e0) {}
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
	}
	for _, d := range cfg.ExtraDomains {
		replacements = addSubdomainRewrites(replacements, cfg, d)
	}
	for _, pair := range replacements {
		s = strings.ReplaceAll(s, pair[0], pair[1])
	}
	// Undo accidental Firebase RTDB rewrites (dots in /ext-proxy/ path crash the SDK)
	s = undoFirebaseProxyRewrites(s, origin)
	// DigitaVision → ToolsMandi + config replacements; never touch JWT payloads (eyJ…)
	s = replaceBrandOutsideJWTs(s)
	s = applyConfigReplacementsOutsideJWTs(s, cfg)
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
var jwtLikeRe = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*`)

// replaceBrandOutsideJWTs rewrites standalone DigitaVision brand tokens without
// touching JWT payloads or email local-parts (digitavision220926@…).
func replaceBrandOutsideJWTs(s string) string {
	parts := jwtLikeRe.Split(s, -1)
	matches := jwtLikeRe.FindAllString(s, -1)
	var b strings.Builder
	b.Grow(len(s))
	for i, part := range parts {
		b.WriteString(brandReplaceSafe(part))
		if i < len(matches) {
			b.WriteString(matches[i])
		}
	}
	return b.String()
}

// applyConfigReplacementsOutsideJWTs runs config.json replacements on non-JWT
// text. DigitaVision-family searches use word boundaries (never emails/JWT).
func applyConfigReplacementsOutsideJWTs(s string, cfg Config) string {
	if len(cfg.Replacements) == 0 || s == "" {
		return s
	}
	parts := jwtLikeRe.Split(s, -1)
	matches := jwtLikeRe.FindAllString(s, -1)
	var b strings.Builder
	b.Grow(len(s))
	for i, part := range parts {
		b.WriteString(applyConfigReplacementsToPart(part, cfg))
		if i < len(matches) {
			b.WriteString(matches[i])
		}
	}
	return b.String()
}

func applyConfigReplacementsToPart(s string, cfg Config) string {
	for _, repl := range cfg.Replacements {
		if repl.Search == "" {
			continue
		}
		if strings.EqualFold(repl.Search, "DigitaVision") ||
			strings.EqualFold(repl.Search, "Digitavision") ||
			strings.EqualFold(repl.Search, "digitavision") {
			re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(repl.Search) + `\b`)
			s = re.ReplaceAllString(s, repl.Replace)
			continue
		}
		s = strings.ReplaceAll(s, repl.Search, repl.Replace)
	}
	return s
}

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

// corsAllowOrigin echoes the browser Origin when it targets this proxy
// (localhost or 127.0.0.1 on our port). Hardcoding public_host breaks CORS
// when the page is opened on the other loopback host while env.js points at
// localhost (cross-origin + credentials → Provisional headers / empty search).
func corsAllowOrigin(r *http.Request, cfg Config) string {
	if r == nil {
		return proxyOrigin(cfg)
	}
	origin := ""
	if v, ok := r.Context().Value(ctxBrowserOrigin).(string); ok {
		origin = strings.TrimSpace(v)
	}
	if origin == "" {
		origin = strings.TrimSpace(r.Header.Get("Origin"))
	}
	if origin == "" {
		return proxyOrigin(cfg)
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return proxyOrigin(cfg)
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	if (host == "localhost" || host == "127.0.0.1") && port == cfg.Port {
		return u.Scheme + "://" + u.Host
	}
	if u.Host == cfg.PublicHost || origin == proxyOrigin(cfg) {
		return origin
	}
	return proxyOrigin(cfg)
}

func relaxResponseHeaders(resp *http.Response, cfg Config) {
	resp.Header.Del("Content-Security-Policy")
	resp.Header.Del("Content-Security-Policy-Report-Only")
	resp.Header.Del("X-Frame-Options")
	origin := proxyOrigin(cfg)
	if resp.Request != nil {
		origin = corsAllowOrigin(resp.Request, cfg)
	}
	resp.Header.Set("Access-Control-Allow-Origin", origin)
	resp.Header.Set("Access-Control-Allow-Credentials", "true")
	resp.Header.Set("Access-Control-Expose-Headers", "Content-Type, Content-Length, ETag")
	// Ensure caches don't mix ACAO across localhost vs 127.0.0.1
	resp.Header.Add("Vary", "Origin")
}

func handleCORSPreflight(w http.ResponseWriter, r *http.Request, cfg Config) bool {
	if r.Method != http.MethodOptions {
		return false
	}
	w.Header().Set("Access-Control-Allow-Origin", corsAllowOrigin(r, cfg))
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS")
	if h := r.Header.Get("Access-Control-Request-Headers"); h != "" {
		w.Header().Set("Access-Control-Allow-Headers", h)
	} else {
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Workspace, X-Xsrf-Token, Accept, Origin, X-Requested-With")
	}
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.Header().Add("Vary", "Origin")
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

func isJSOrCSS(ct, path string) bool {
	ct = strings.ToLower(ct)
	if strings.Contains(ct, "javascript") || strings.Contains(ct, "ecmascript") || strings.Contains(ct, "text/css") {
		return true
	}
	p := strings.ToLower(path)
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	return strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".css") || strings.HasSuffix(p, ".mjs")
}

func isBinaryStaticCT(ct string) bool {
	ct = strings.ToLower(ct)
	if ct == "" {
		return false
	}
	if isRewritable(ct) {
		return false
	}
	prefixes := []string{
		"image/", "font/", "audio/", "video/", "application/font",
		"application/octet-stream", "application/wasm", "application/pdf",
		"application/zip", "application/gzip",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(ct, p) || strings.Contains(ct, p) {
			return true
		}
	}
	return strings.Contains(ct, "woff") || strings.Contains(ct, "truetype") || strings.Contains(ct, "opentype")
}

// In-memory static cache — SPA boots ~200 assets; re-proxying each via uTLS
// (decompress + rewriteBody on multi-MB bundles) made finish ~18s.
type staticCacheEntry struct {
	status      int
	contentType string
	encoding    string
	body        []byte
	expires     time.Time
}

var staticAssetCache sync.Map // key -> *staticCacheEntry

func staticCacheKey(host, method, path string) string {
	return strings.ToLower(host) + " " + method + " " + path
}

func isCacheableStaticPath(path string) bool {
	p := path
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	p = strings.ToLower(p)
	if strings.HasPrefix(p, "/assets/") || strings.HasPrefix(p, "/static/") ||
		strings.HasPrefix(p, "/dist/") || strings.HasPrefix(p, "/fonts/") ||
		strings.HasPrefix(p, "/img/") || strings.HasPrefix(p, "/images/") {
		return true
	}
	// Hashed webpack bundles at suite root: bundle.main.<hash>.js
	base := p
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if strings.HasPrefix(base, "bundle.") {
		return true
	}
	if base == "env.js" {
		return true
	}
	for _, ext := range []string{".js", ".css", ".mjs", ".woff2", ".woff", ".ttf", ".eot", ".otf", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp", ".map", ".wasm"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

func getStaticCached(host, method, path string) *staticCacheEntry {
	if method != http.MethodGet && method != http.MethodHead {
		return nil
	}
	if !isCacheableStaticPath(path) {
		return nil
	}
	v, ok := staticAssetCache.Load(staticCacheKey(host, method, path))
	if !ok {
		if method == http.MethodHead {
			v, ok = staticAssetCache.Load(staticCacheKey(host, http.MethodGet, path))
		}
		if !ok {
			return nil
		}
	}
	ent := v.(*staticCacheEntry)
	if time.Now().After(ent.expires) {
		staticAssetCache.Delete(staticCacheKey(host, http.MethodGet, path))
		return nil
	}
	return ent
}

func putStaticCached(host, method, path string, status int, contentType, encoding string, body []byte) {
	if method != http.MethodGet || status != 200 || !isCacheableStaticPath(path) {
		return
	}
	if len(body) == 0 || len(body) > 12<<20 { // skip empty / >12MB
		return
	}
	cp := make([]byte, len(body))
	copy(cp, body)
	staticAssetCache.Store(staticCacheKey(host, method, path), &staticCacheEntry{
		status:      status,
		contentType: contentType,
		encoding:    encoding,
		body:        cp,
		expires:     time.Now().Add(6 * time.Hour),
	})
}

func serveStaticCached(w http.ResponseWriter, r *http.Request, ent *staticCacheEntry, cfg Config) {
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	w.Header().Del("Pragma")
	w.Header().Del("Expires")
	if ent.contentType != "" {
		w.Header().Set("Content-Type", ent.contentType)
	}
	if ent.encoding != "" {
		w.Header().Set("Content-Encoding", ent.encoding)
	} else {
		w.Header().Del("Content-Encoding")
	}
	w.Header().Set("X-OCG-Cache", "HIT")
	w.Header().Set("Access-Control-Allow-Origin", corsAllowOrigin(r, cfg))
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Add("Vary", "Origin")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(ent.body)))
	w.WriteHeader(ent.status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(ent.body)
	}
}

func setStaticCacheControl(resp *http.Response, path string) {
	if isCacheableStaticPath(path) {
		resp.Header.Set("Cache-Control", "public, max-age=86400, immutable")
		resp.Header.Del("Pragma")
		resp.Header.Del("Expires")
		resp.Header.Set("X-OCG-Cache", "MISS")
	}
}

// bodyNeedsRewrite: cheap needle scan before full rewriteBody on multi-MB bundles.
func bodyNeedsRewrite(plain []byte, cfg Config) bool {
	if len(plain) == 0 {
		return false
	}
	lower := bytes.ToLower(plain)
	if bytes.Contains(lower, []byte("seotesteronline.com")) {
		return true
	}
	if bytes.Contains(lower, []byte("digitavision")) {
		return true
	}
	if bytes.Contains(lower, []byte("integrity=")) {
		return true
	}
	if bytes.Contains(lower, []byte("content-security-policy")) {
		return true
	}
	for _, d := range cfg.ExtraDomains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" || strings.Contains(d, "seotesteronline.com") {
			continue
		}
		if skipURLRewrite(d) {
			continue
		}
		if bytes.Contains(lower, []byte(d)) {
			return true
		}
	}
	for _, repl := range cfg.Replacements {
		if repl.Search == "" {
			continue
		}
		if bytes.Contains(lower, bytes.ToLower([]byte(repl.Search))) {
			return true
		}
	}
	return false
}

// shouldAttachBearer: JWT on static assets → 401. Only API / socket / proxy hosts.
func shouldAttachBearer(host, path string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	p := strings.ToLower(path)
	if h == "" {
		return strings.HasPrefix(p, "/api/")
	}
	deny := []string{
		"stripe.com", "zdassets", "calendly", "googleapis.com", "gstatic.com",
		"cloudflare", "firebaseio.com", "firebasedatabase.app", "firebaseapp.com",
		"googletagmanager", "google-analytics", "sentry.io", "js.stripe",
		"paypal.com", "google.com", "typeform.com",
	}
	for _, d := range deny {
		if strings.Contains(h, d) {
			return false
		}
	}
	// Suite SPA static (bundles, assets, env.js) — no Bearer
	if h == "suite.seotesteronline.com" || h == "www.seotesteronline.com" || h == "seotesteronline.com" {
		return strings.HasPrefix(p, "/api/")
	}
	allow := []string{
		"api.seotesteronline.com",
		"socket.seotesteronline.com",
		"proxy.seotesteronline.com",
	}
	for _, a := range allow {
		if h == a {
			return true
		}
	}
	if strings.HasSuffix(h, ".seotesteronline.com") {
		if strings.HasPrefix(h, "suite.") || strings.HasPrefix(h, "www.") {
			return strings.HasPrefix(p, "/api/")
		}
		// marketing locales etc. — no Bearer
		if strings.HasPrefix(h, "it.") || strings.HasPrefix(h, "es.") || strings.HasPrefix(h, "pl.") ||
			strings.HasPrefix(h, "go.") || strings.HasPrefix(h, "feedback.") || strings.HasPrefix(h, "partner.") {
			return false
		}
		return true
	}
	if strings.HasPrefix(p, "/api/") {
		return true
	}
	return false
}

func isSEOTesterAPIHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if i := strings.Index(h, ":"); i >= 0 {
		h = h[:i]
	}
	return h == "api.seotesteronline.com"
}

func isGoogleSuggestHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if i := strings.Index(h, ":"); i >= 0 {
		h = h[:i]
	}
	switch h {
	case "clients1.google.com", "suggestqueries.google.com", "www.google.com", "google.com":
		return true
	default:
		return false
	}
}

// fixKeywordExplorerBody maps invalid country codes the SPA may send.
// GBL (Global) is not a keywords DB — upstream 500s; US is the production fallback
// (same as SERP: "GBL"===selectedCountry?"US":…).
func fixKeywordExplorerBody(body []byte, path string) []byte {
	if !strings.Contains(path, "keyword-explorer") || len(body) == 0 {
		return body
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}
	changed := false
	filters, _ := payload["filters"].(map[string]interface{})
	if filters == nil {
		filters = map[string]interface{}{}
		payload["filters"] = filters
		changed = true
	}
	cc, _ := filters["countryCode"].(string)
	cc = strings.TrimSpace(cc)
	if cc == "" || strings.EqualFold(cc, "GBL") || strings.EqualFold(cc, "null") {
		filters["countryCode"] = "US"
		changed = true
	} else if cc != strings.ToUpper(cc) && len(cc) <= 3 {
		// SPA/storage sometimes lowercases (us) → Unknown database keywords_country:us
		filters["countryCode"] = strings.ToUpper(cc)
		changed = true
	}
	if !changed {
		return body
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return out
}

var proxyByHost sync.Map // host → *httputil.ReverseProxy (shared transport / keep-alive)

func getReverseProxy(target *url.URL, getSession func() string) *httputil.ReverseProxy {
	host := target.Host
	if v, ok := proxyByHost.Load(host); ok {
		return v.(*httputil.ReverseProxy)
	}
	p := newReverseProxy(target, getSession)
	actual, _ := proxyByHost.LoadOrStore(host, p)
	return actual.(*httputil.ReverseProxy)
}

func newReverseProxy(target *url.URL, getSession func() string) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = getSharedTransport()
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
		cfg := loadConfig()
		// Preserve browser Accept / content negotiation BEFORE chrome document defaults.
		// api.seotesteronline.com Vary: Accept — forcing text/html returns SPA HTML, not JSON.
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
		if isGoogleSuggestHost(target.Host) {
			// JSONP / suggest — do not force document Accept
			if origAccept != "" {
				req.Header.Set("Accept", origAccept)
			} else {
				req.Header.Set("Accept", "*/*")
			}
			req.Header.Set("Sec-Fetch-Dest", "script")
			req.Header.Set("Sec-Fetch-Mode", "no-cors")
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			req.Header.Del("Sec-Fetch-User")
			req.Header.Del("Upgrade-Insecure-Requests")
			req.Header.Del("Authorization")
		} else if apiLike {
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
		// Preserve browser Origin for CORS ACAO (director overwrites Origin for upstream)
		if browserOrigin := req.Header.Get("Origin"); browserOrigin != "" {
			*req = *req.WithContext(context.WithValue(req.Context(), ctxBrowserOrigin, browserOrigin))
		}
		if ref := req.Header.Get("Referer"); ref != "" {
			req.Header.Set("Referer", strings.ReplaceAll(ref, proxyOrigin(cfg), zgOrigin))
		} else {
			req.Header.Set("Referer", zgOrigin+"/")
		}
		if origin := req.Header.Get("Origin"); origin != "" {
			req.Header.Set("Origin", zgOrigin)
		} else {
			req.Header.Set("Origin", zgOrigin)
		}
		// Keyword explorer: normalize countryCode (GBL/us → US) before upstream
		if req.Body != nil && req.Method == http.MethodPost &&
			strings.Contains(req.URL.Path, "/keyword-explorer/") {
			body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
			req.Body.Close()
			if err == nil {
				fixed := fixKeywordExplorerBody(body, req.URL.Path)
				req.Body = io.NopCloser(bytes.NewReader(fixed))
				req.ContentLength = int64(len(fixed))
				req.Header.Set("Content-Length", fmt.Sprintf("%d", len(fixed)))
			} else {
				req.Body = io.NopCloser(bytes.NewReader(nil))
			}
		}
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		cfg := loadConfig()
		if resp.StatusCode == http.StatusSwitchingProtocols {
			return nil
		}
		// Prevent upstream/session cookies from piling onto shared localhost jar
		resp.Header.Del("Set-Cookie")
		session := panelSessionFor(resp.Request, getSession)
		ct := resp.Header.Get("Content-Type")
		enc := resp.Header.Get("Content-Encoding")
		reqHost, reqPath, reqMethod := "", "", ""
		if resp.Request != nil {
			reqHost = resp.Request.URL.Host
			reqPath = resp.Request.URL.Path
			if resp.Request.URL.RawQuery != "" && isCacheableStaticPath(reqPath) {
				// Keep query in cache key only when path alone is not enough (rare)
				reqPath = resp.Request.URL.Path
			}
			reqMethod = resp.Request.Method
		}
		apiHost := isSEOTesterAPIHost(reqHost)
		apiPath := strings.Contains(reqPath, "/api/")
		googleSuggest := isGoogleSuggestHost(reqHost)
		ctLower := strings.ToLower(ct)

		// Binary images/fonts/wasm: no decompress, no rewrite — optional cache.
		if isBinaryStaticCT(ct) || (!isRewritable(ct) && isCacheableStaticPath(reqPath)) {
			relaxResponseHeaders(resp, cfg)
			if resp.StatusCode == 200 && reqMethod == http.MethodGet && isCacheableStaticPath(reqPath) {
				body, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil {
					return err
				}
				putStaticCached(reqHost, reqMethod, reqPath, resp.StatusCode, ct, enc, body)
				setStaticCacheControl(resp, reqPath)
				resp.Header.Del("Content-Length")
				resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
				resp.Body = io.NopCloser(bytes.NewReader(body))
				return nil
			}
			return nil // stream upstream body as-is
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}

		// Peek uncompressed for JSON / rewrite decisions without always rewriting.
		plain, decErr := decompressBody(body, enc)
		decompressedOK := decErr == nil
		if decErr != nil {
			plain = body
		}
		looksJSON := len(plain) > 0 && (plain[0] == '{' || plain[0] == '[')
		isJSON := strings.Contains(ctLower, "json") || looksJSON
		// Never rewrite API / Google suggest bodies — DigitaVision/URL replace
		// corrupts JWT/JSON and stalls multi-MB keyword explorer results.
		skipRewrite := googleSuggest || apiHost || (apiPath && (isJSON || looksJSON || ct == "" || strings.Contains(ctLower, "text/plain")))

		if skipRewrite || !isRewritable(ct) {
			relaxResponseHeaders(resp, cfg)
			// Pass original bytes + encoding (no recompress tax on API JSON).
			if (apiHost || apiPath) && looksJSON &&
				(ct == "" || strings.Contains(ctLower, "text/plain")) {
				resp.Header.Set("Content-Type", "application/json; charset=utf-8")
			}
			resp.Header.Del("Content-Length")
			resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
			resp.Body = io.NopCloser(bytes.NewReader(body))
			return nil
		}

		didRewrite := false
		staticAsset := isCacheableStaticPath(reqPath) && isJSOrCSS(ct, reqPath)

		// Large hashed JS/CSS: skip rewriteBody when no URL/brand needles present.
		if staticAsset && !bodyNeedsRewrite(plain, cfg) {
			relaxResponseHeaders(resp, cfg)
			putStaticCached(reqHost, reqMethod, reqPath, resp.StatusCode, ct, enc, body)
			setStaticCacheControl(resp, reqPath)
			resp.Header.Del("Content-Length")
			resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
			resp.Body = io.NopCloser(bytes.NewReader(body))
			return nil
		}

		plain = rewriteBody(plain, cfg)
		didRewrite = true
		if cfg.CloudflareBypass {
			plain = stripCloudflareChallengeHTML(plain, cfg)
		}
		if isHTML(ct) && isCloudflareBlockPage(plain) {
			log.Printf("[CF] Upstream returned Cloudflare block page for %s", reqPath)
		}
		// Inject into suite HTML (not API hosts /api JSON error pages)
		if isHTML(ct) && !apiHost && !apiPath {
			if usesPanelAccountMode(cfg) {
				plain = injectDeviceHTML(plain)
			}
			inject := buildHeadInject(cfg, session)
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

		relaxResponseHeaders(resp, cfg)
		out := plain
		outEnc := ""
		if didRewrite || decompressedOK || enc == "" {
			resp.Header.Del("Content-Encoding")
		} else {
			out = body
			outEnc = enc
		}
		if staticAsset && resp.StatusCode == 200 {
			putStaticCached(reqHost, reqMethod, reqPath, resp.StatusCode, ct, outEnc, out)
			setStaticCacheControl(resp, reqPath)
		}
		resp.Header.Del("Content-Length")
		resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(out)))
		resp.Body = io.NopCloser(bytes.NewReader(out))
		return nil
	}
	return proxy
}

// warmCriticalAssets prefetches homepage + hashed bundles into memory cache.
func warmCriticalAssets(cfg Config, listenAddr string) {
	port := cfg.Port
	if strings.HasPrefix(listenAddr, ":") {
		port = strings.TrimPrefix(listenAddr, ":")
	} else if _, p, err := net.SplitHostPort(listenAddr); err == nil {
		port = p
	}
	dialAddr := "127.0.0.1:" + port
	for i := 0; i < 50; i++ {
		c, err := net.DialTimeout("tcp", dialAddr, 150*time.Millisecond)
		if err == nil {
			c.Close()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	base := "http://" + dialAddr
	client := &http.Client{Timeout: 90 * time.Second, Transport: &http.Transport{DisableCompression: true, MaxIdleConnsPerHost: 16}}
	fetch := func(path string) {
		req, err := http.NewRequest(http.MethodGet, base+path, nil)
		if err != nil {
			return
		}
		req.Header.Set("User-Agent", cfg.UserAgent)
		req.Header.Set("Accept", "*/*")
		resp, err := client.Do(req)
		if err != nil {
			return
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	log.Printf("[CACHE] Warming critical assets via %s …", base)
	fetch("/")
	// Discover hashed bundles from homepage HTML.
	req, err := http.NewRequest(http.MethodGet, base+"/", nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", cfg.UserAgent)
	req.Header.Set("Accept", "text/html")
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	htmlBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	resp.Body.Close()
	re := regexp.MustCompile(`(?:src|href)=["']\.?/?([^"']+\.(?:js|css))["']`)
	seen := map[string]bool{"/env.js": true}
	paths := []string{"/env.js"}
	for _, m := range re.FindAllSubmatch(htmlBody, -1) {
		p := string(m[1])
		if strings.HasPrefix(p, "http") || strings.HasPrefix(p, "//") {
			continue
		}
		if strings.Contains(p, "ext-proxy/") || strings.Contains(p, "cdn-cgi/") {
			continue
		}
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
	}
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, p := range paths {
		if !isCacheableStaticPath(p) {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(path string) {
			defer wg.Done()
			defer func() { <-sem }()
			fetch(path)
		}(p)
	}
	wg.Wait()
	log.Printf("[CACHE] Warm-up finished (%d asset paths)", len(paths))
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
// Leftover ChatGPT (and other tool) cookies on this host were ~10KB; the old
// code deleted the entire Cookie header and wiped the new panel session.
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
		log.Printf("[SESSION] Loaded SEO Tester accessToken (%d chars)", len(token))
	} else {
		log.Printf("[WARN] No accessToken key found in cookie.txt localStorage export")
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

		// Suite home is "/" (Angular app.welcome) — no redirect

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
				// Rewrite to / in-place (no 302) — 302 /signin→/ causes reload loops
				// when the SPA hard-navigates to /signin after a failed auth check.
				r2 := r.Clone(r.Context())
				r2.URL.Path = "/"
				r2.URL.RawPath = ""
				r2.URL.RawQuery = ""
				debugLog(cfg, "blocked %s → serve /", r.URL.Path)
				if isWebSocket(r2) {
					handleWebSocket(w, r2, cfg, targetHost(cfg), r2.URL.Path, getSession)
					return
				}
				getReverseProxy(targetURL, getSession).ServeHTTP(w, r2)
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
			if ent := getStaticCached(extHost, r.Method, extPath); ent != nil {
				serveStaticCached(w, r, ent, cfg)
				return
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
			debugLog(cfg, "%s /ext-proxy/%s%s", r.Method, extHost, extPath)
			getReverseProxy(extTarget, getSession).ServeHTTP(w, r2)
			return
		}

		// Suite mistakenly hits /socket.io on the SPA host (env socketUrl bypass /
		// relative connect). Forward to socket.seotesteronline.com — HTML from
		// suite breaks Engine.IO polling forever.
		if strings.HasPrefix(r.URL.Path, "/socket.io/") {
			sockHost := "socket.seotesteronline.com"
			sockTarget, err := url.Parse("https://" + sockHost)
			if err == nil {
				debugLog(cfg, "%s %s → %s", r.Method, r.URL.RequestURI(), sockHost)
				if isWebSocket(r) {
					handleWebSocket(w, r, cfg, sockHost, r.URL.Path, getSession)
					return
				}
				r2 := r.Clone(r.Context())
				r2.Host = sockHost
				getReverseProxy(sockTarget, getSession).ServeHTTP(w, r2)
				return
			}
		}

		// Relative /api/* → api.seotesteronline.com (Angular apiGateway)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			apiHost := "api.seotesteronline.com"
			apiTarget, err := url.Parse("https://" + apiHost)
			if err == nil {
				debugLog(cfg, "%s %s → %s", r.Method, r.URL.RequestURI(), apiHost)
				r2 := r.Clone(r.Context())
				r2.Host = apiHost
				getReverseProxy(apiTarget, getSession).ServeHTTP(w, r2)
				return
			}
		}

		debugLog(cfg, "%s %s", r.Method, r.URL.RequestURI())
		if isWebSocket(r) {
			handleWebSocket(w, r, cfg, targetHost(cfg), r.URL.Path, getSession)
			return
		}
		if ent := getStaticCached(targetHost(cfg), r.Method, r.URL.Path); ent != nil {
			serveStaticCached(w, r, ent, cfg)
			return
		}
		getReverseProxy(targetURL, getSession).ServeHTTP(w, r)
	})

	addr := ":" + cfg.Port
	if cfg.BindLocalhost {
		addr = "127.0.0.1:" + cfg.Port
	}

	log.Printf("╔══════════════════════════════════════════════╗")
	log.Printf("║  SEO Tester Online Go Proxy — LOCAL                      ║")
	log.Printf("║  URL:    %s://%s", cfg.PublicScheme, cfg.PublicHost)
	log.Printf("║  Target: %s", cfg.TargetURL)
	log.Printf("║  Auth:   localStorage via cookie.txt         ║")
	log.Printf("╚══════════════════════════════════════════════╝")

	// Wrap: slim huge Cookie jars — NEVER delete the whole header.
	// Dropping Cookie wiped ct_session → /api/device-bind failed → Access Denied.
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
	go warmCriticalAssets(cfg, addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("[FATAL] %v", err)
	}
}
