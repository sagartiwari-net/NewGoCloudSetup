package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

func atpExtraCDNPath(cfg Config, hostNeedle string) string {
	hostNeedle = strings.ToLower(strings.TrimSpace(hostNeedle))
	for i, d := range cfg.ExtraCDNDomains {
		clean := strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://"))
		clean = strings.Split(clean, "/")[0]
		if clean == hostNeedle || strings.HasSuffix(clean, "."+hostNeedle) || strings.Contains(clean, hostNeedle) {
			return fmt.Sprintf("/extra-cdn-%d", i)
		}
	}
	return ""
}

func atpAPIProxyPath(cfg Config) string {
	if p := atpExtraCDNPath(cfg, "api.answerthepublic.com"); p != "" {
		return p
	}
	return "/extra-cdn-1"
}

func atpComposeoProxyPath(cfg Config) string {
	return atpExtraCDNPath(cfg, "composeoapi.neilpatelapi.com")
}

// atpEarlyAPIPatch runs first in <head> so api.answerthepublic.com never leaves
// this origin (Network CORS on /me + /plan_limits).
func atpEarlyAPIPatch(cfg Config) string {
	apiPath := atpAPIProxyPath(cfg)
	composeo := atpComposeoProxyPath(cfg)
	return fmt.Sprintf(`<script data-tm-early-atp="1">
(function(){
  if (window.__tmEarlyATP) return;
  window.__tmEarlyATP = true;
  var O = location.origin;
  var API = %q;
  var COMPOSEO = %q;
  function forceAPI(u){
    if (u == null) return u;
    if (typeof u !== 'string') {
      try { u = (u && u.url) ? String(u.url) : String(u); } catch (e) { return u; }
    }
    u = u.replace(/https?:\/\/api\.answerthepublic\.com/gi, O + API);
    u = u.replace(/(^|[^:\/])\/\/api\.answerthepublic\.com/gi, '$1' + O + API);
    if (COMPOSEO) {
      u = u.replace(/https?:\/\/composeoapi\.neilpatelapi\.com/gi, O + COMPOSEO);
      u = u.replace(/(^|[^:\/])\/\/composeoapi\.neilpatelapi\.com/gi, '$1' + O + COMPOSEO);
    }
    return u;
  }
  window.__tmForceATP = forceAPI;
  var nf = window.fetch;
  if (typeof nf === 'function') {
    window.fetch = function(input, init){
      try {
        if (typeof input === 'string') input = forceAPI(input);
        else if (input && typeof Request !== 'undefined' && input instanceof Request)
          input = new Request(forceAPI(input.url), input);
        else if (input && typeof input.url === 'string')
          input = new Request(forceAPI(input.url), input);
      } catch (e) {}
      try {
        var u = typeof input === 'string' ? input : (input && input.url) || '';
        var abs = new URL(u, location.href);
        if (abs.origin === location.origin || String(u).indexOf(API) === 0 || String(u).indexOf(O + API) === 0) {
          init = init ? Object.assign({}, init) : {};
          init.credentials = 'include';
        }
      } catch (e2) {}
      return nf.call(this, input, init);
    };
  }
  var xo = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function(m, u){
    try { u = forceAPI(u); } catch (e) {}
    var args = Array.prototype.slice.call(arguments);
    args[1] = u;
    try {
      if (String(u).indexOf(API) !== -1 || String(u).indexOf(location.origin + API) === 0) {
        this.withCredentials = true;
      }
    } catch (e3) {}
    return xo.apply(this, args);
  };
})();
</script>`, apiPath, composeo)
}

// atpClientNetDiagScript beacons failed/leaked network calls to /api/client-diag
// so app.log + /__logs show what DevTools Network shows.
func atpClientNetDiagScript() string {
	return `<script data-tm-netdiag="1">
(function(){
  if (window.__tmNetDiag) return;
  window.__tmNetDiag = true;
  var sent = 0;
  var MAX = 80;
  function clip(s, n){ s = String(s || ''); return s.length > n ? s.slice(0, n) + '…' : s; }
  function report(kind, info){
    if (sent >= MAX) return;
    sent++;
    var payload = {kind: kind, t: Date.now(), href: location.href};
    try {
      if (info) for (var k in info) if (Object.prototype.hasOwnProperty.call(info, k)) payload[k] = info[k];
    } catch (e) {}
    var body = '';
    try { body = JSON.stringify(payload); } catch (e2) { body = '{"kind":"bad_json"}'; }
    try {
      if (navigator.sendBeacon) {
        navigator.sendBeacon('/api/client-diag', new Blob([body], {type:'application/json'}));
        return;
      }
    } catch (e3) {}
    try {
      fetch('/api/client-diag', {method:'POST', credentials:'same-origin',
        headers:{'Content-Type':'application/json'}, body: body, keepalive: true}).catch(function(){});
    } catch (e4) {}
  }
  function urlOf(input){
    try {
      if (typeof input === 'string') return input;
      if (input && input.url) return String(input.url);
    } catch (e) {}
    return '';
  }
  function classify(u){
    u = String(u || '');
    if (/api\.answerthepublic\.com/i.test(u)) return 'api_leak';
    if (/composeoapi\.neilpatelapi\.com/i.test(u)) return 'composeo_leak';
    if (/\/pricing(?:\?|$)/i.test(u)) return 'pricing_nav';
    if (/\/users\/sign_in/i.test(u)) return 'sign_in_nav';
    if (/plan_limits|\/users\/me(?:\?|$)|\/api\/v1\//i.test(u)) return 'api_call';
    return 'fetch';
  }
  var nf = window.fetch;
  if (typeof nf === 'function') {
    window.fetch = function(input, init){
      var u = urlOf(input);
      var kindHint = classify(u);
      if (kindHint === 'api_leak' || kindHint === 'composeo_leak') {
        report(kindHint, {url: clip(u, 220), note: 'cross-origin before/without rewrite'});
      }
      return nf.call(this, input, init).then(function(res){
        try {
          if (res && (res.status === 401 || res.status === 403 || res.status >= 500)) {
            report('fetch_status', {url: clip(u || urlOf(res.url), 220), status: res.status, type: kindHint});
          }
        } catch (e) {}
        return res;
      }).catch(function(err){
        report('fetch_error', {
          url: clip(u, 220),
          type: kindHint,
          msg: clip(err && (err.message || err.name || err), 160)
        });
        throw err;
      });
    };
  }
  try {
    var xo = XMLHttpRequest.prototype.open;
    var xs = XMLHttpRequest.prototype.send;
    XMLHttpRequest.prototype.open = function(m, u){
      this.__tmNetURL = String(u || '');
      this.__tmNetMethod = m;
      return xo.apply(this, arguments);
    };
    XMLHttpRequest.prototype.send = function(){
      var xhr = this;
      var u = xhr.__tmNetURL || '';
      if (/api\.answerthepublic\.com/i.test(u)) {
        report('api_leak_xhr', {url: clip(u, 220), method: xhr.__tmNetMethod || ''});
      }
      xhr.addEventListener('loadend', function(){
        try {
          var st = xhr.status;
          if (st === 401 || st === 403 || st >= 500) {
            report('xhr_status', {url: clip(u, 220), status: st, method: xhr.__tmNetMethod || ''});
          }
        } catch (e) {}
      });
      xhr.addEventListener('error', function(){
        report('xhr_error', {url: clip(u, 220), method: xhr.__tmNetMethod || ''});
      });
      return xs.apply(xhr, arguments);
    };
  } catch (e5) {}
  // One boot summary after hydrate window
  setTimeout(function(){
    try {
      var auth = '';
      try { auth = localStorage.getItem('auth-storage') || ''; } catch (e) {}
      var hasUser = auth.indexOf('"user"') !== -1 && auth.indexOf('"id"') !== -1;
      report('boot', {
        path: location.pathname + location.search,
        hasAuthStorage: hasUser,
        authLen: auth.length,
        earlyPatch: !!window.__tmEarlyATP
      });
    } catch (e6) {}
  }, 4000);
})();
</script>`
}

func injectHeadStart(body []byte, script string) []byte {
	if script == "" || len(body) == 0 {
		return body
	}
	if bytes.Contains(body, []byte("data-tm-early-atp")) && strings.Contains(script, "data-tm-early-atp") {
		return body
	}
	inj := []byte(script)
	lower := bytes.ToLower(body)
	if h := bytes.Index(lower, []byte("<head")); h >= 0 {
		if gt := bytes.IndexByte(lower[h:], '>'); gt >= 0 {
			at := h + gt + 1
			out := make([]byte, 0, len(body)+len(inj))
			out = append(out, body[:at]...)
			out = append(out, inj...)
			out = append(out, body[at:]...)
			return out
		}
	}
	// Fallback: prepend
	out := make([]byte, 0, len(body)+len(inj))
	out = append(out, inj...)
	out = append(out, body...)
	return out
}

func atpClientDiagHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodOptions {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 8<<10))
	user := ""
	account := ""
	if u, err := getAuthenticatedUser(r, loadConfig()); err == nil {
		user = u
		if tok, _, ok := sessionFromRequest(r); ok {
			if acc, aerr := loadPanelSessionAccount(loadConfig(), tok); aerr == nil {
				account = acc.Name
			}
		}
	}
	detail := truncateForLog(string(body), 500)
	log.Printf("[CLIENT-DIAG] user=%s account=%s %s", user, account, detail)
	pushProxyLog(ProxyLogEntry{
		Source:  "CLIENT",
		Level:   "warn",
		Method:  "POST",
		Path:    "/api/client-diag",
		Status:  200,
		User:    user,
		Account: account,
		Detail:  detail,
	})
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"ok"}`)
}

func truncateForLog(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func shouldLogATPNet(path string, isExtraCDN bool, status int) bool {
	if isExtraCDN {
		return true
	}
	p := strings.ToLower(path)
	if strings.Contains(p, "plan_limits") || strings.Contains(p, "/users/me") ||
		strings.HasPrefix(p, "/api/") || strings.Contains(p, "/extra-cdn-") {
		return true
	}
	if status == 401 || status == 403 || status >= 500 {
		return true
	}
	return false
}

func logATPNet(method, path, user, account, upstreamHost string, status int, origin, acao string, cookieN int, detail string) {
	log.Printf("[NET] %s %s → host=%s status=%d origin=%q acao=%q cookies=%d user=%s account=%s %s",
		method, path, upstreamHost, status, origin, acao, cookieN, user, account, detail)
	pushProxyLog(ProxyLogEntry{
		Source:  "NET",
		Level:   "info",
		Method:  method,
		Path:    path,
		Status:  status,
		User:    user,
		Account: account,
		Detail:  fmt.Sprintf("host=%s origin=%s acao=%s %s", upstreamHost, origin, acao, detail),
		CookieN: cookieN,
	})
}
