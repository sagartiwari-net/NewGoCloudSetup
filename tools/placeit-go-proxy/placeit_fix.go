package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// placeitEarlyCDNPatch rewrites Placeit commercial / img / placeitcode / S3 hosts
// before the editor boots, and beacons Network-style CLIENT-DIAG to app.log.
func placeitEarlyCDNPatch() string {
	return `<script data-tm-early-placeit="1">
(function(){
  if (window.__tmEarlyPlaceit) return;
  window.__tmEarlyPlaceit = true;
  var O = location.origin;
  var nf = window.fetch;
  var diagSent = 0;
  var DIAG_MAX = 120;
  var bindErrN = 0;

  function clip(s, n){ s = String(s == null ? '' : s); return s.length > n ? s.slice(0, n) + '…' : s; }
  function urlOf(input){
    try {
      if (typeof input === 'string') return input;
      if (input && input.url) return String(input.url);
    } catch (e) {}
    return '';
  }
  function methodOf(input, init){
    try {
      if (init && init.method) return String(init.method).toUpperCase();
      if (input && input.method) return String(input.method).toUpperCase();
    } catch (e) {}
    return 'GET';
  }
  function report(kind, info){
    if (diagSent >= DIAG_MAX) return;
    // debounce device-bind spam
    try {
      var u = (info && info.url) || '';
      if (/\/api\/device-bind/i.test(u)) {
        if (kind === 'fetch_error' || kind === 'xhr_error') {
          bindErrN++;
          if (bindErrN > 3) return;
        } else if (kind !== 'boot') {
          return; // skip successful device-bind noise
        }
      }
    } catch (e0) {}
    diagSent++;
    var payload = {kind: kind, t: Date.now(), path: location.pathname};
    try {
      if (info) for (var k in info) if (Object.prototype.hasOwnProperty.call(info, k)) payload[k] = info[k];
    } catch (e1) {}
    var body = '';
    try { body = JSON.stringify(payload); } catch (e2) { body = '{"kind":"bad_json"}'; }
    try {
      if (navigator.sendBeacon) {
        navigator.sendBeacon('/api/client-diag', new Blob([body], {type:'application/json'}));
        return;
      }
    } catch (e3) {}
    try {
      if (typeof nf === 'function') {
        nf.call(window, '/api/client-diag', {
          method:'POST', credentials:'same-origin', keepalive:true,
          headers:{'Content-Type':'application/json'}, body: body
        }).catch(function(){});
      }
    } catch (e4) {}
  }

  function forcePI(u){
    if (u == null) return u;
    if (typeof u !== 'string') {
      try { u = (u && u.url) ? String(u.url) : String(u); } catch (e) { return u; }
    }
    // User uploads + marketing buckets (s3 / s3-accelerate) — direct fetch = CORS fail
    u = u.replace(/https?:\/\/(placeit[a-z0-9.-]*\.amazonaws\.com)/gi, O + '/ext-host/$1');
    u = u.replace(/(^|[^:\/])\/\/(placeit[a-z0-9.-]*\.amazonaws\.com)/gi, '$1' + O + '/ext-host/$2');
    // Dynamic mockup image CDNs (placeit-img-1-p, placeit-img-2-p, …)
    u = u.replace(/https?:\/\/((?:[a-z0-9-]+\.)*cdn\.aws\.placeit\.net)/gi, O + '/ext-host/$1');
    u = u.replace(/https?:\/\/((?:[a-z0-9-]+\.)*placeitcode\.net)/gi, O + '/ext-host/$1');
    // Other placeit.net subdomains (not apex — apex is this proxy)
    u = u.replace(/https?:\/\/((?:[a-z0-9-]+\.)+)placeit\.net(?=\/|$)/gi, function(m, sub){
      var host = sub + 'placeit.net';
      if (host === 'www.placeit.net') return O;
      return O + '/ext-host/' + host;
    });
    u = u.replace(/(^|[^:\/])\/\/((?:[a-z0-9-]+\.)*cdn\.aws\.placeit\.net)/gi, '$1' + O + '/ext-host/$2');
    u = u.replace(/(^|[^:\/])\/\/((?:[a-z0-9-]+\.)*placeitcode\.net)/gi, '$1' + O + '/ext-host/$2');
    return u;
  }
  function isAdsNoise(u){
    return /doubleclick\.net|googlesyndication\.com|google-analytics\.com|googletagmanager\.com|facebook\.net\/tr|recurly\.com|kaptcha\.com|\/api\/client-diag/i.test(String(u||''));
  }
  function isStaticAsset(u){
    return /\.(js|css|png|jpe?g|gif|webp|svg|woff2?|ttf|map|ico|msh)(\?|$)/i.test(String(u||'')) ||
      /\/_next\/static\/|fonts\.googleapis|fonts\.gstatic/i.test(String(u||''));
  }
  function isInteresting(u){
    u = String(u || '');
    if (!u || isAdsNoise(u) || isStaticAsset(u)) return false;
    return /\/ext-host\/|\/extra-cdn-|amazonaws|placeit\.net|placeitcode\.net|nicev2|alloy\.|\/api\/|\/account|\/user|upload|library|\/session|\/auth|\/login|logged|\/me(?:\?|$)|palette|member|subscription/i.test(u);
  }
  function classify(u){
    u = String(u || '');
    if (/placeit[a-z0-9.-]*\.amazonaws\.com/i.test(u) && u.indexOf('/ext-host/') === -1 && u.indexOf('/extra-cdn-') === -1) return 's3_leak';
    if (/(?:^|\/\/)(?:[a-z0-9.-]+\.)?placeit\.net\//i.test(u) && u.indexOf(O) !== 0 && u.indexOf('/ext-host/') === -1) return 'placeit_leak';
    if (/\/ext-host\/placeit[^/]*amazonaws/i.test(u)) return 's3_proxy';
    if (/\/ext-host\/nicev2/i.test(u) || /nicev2\.placeit\.net/i.test(u)) return 'nicev2';
    if (/upload|user_image|library|user-uploads/i.test(u)) return 'upload';
    if (/\/account|\/users?\/|\/me\b|session|auth|login|logged/i.test(u)) return 'auth';
    if (/\/api\//i.test(u)) return 'api';
    return 'net';
  }
  function shouldLogStatus(status, u){
    if (status === 401 || status === 403 || status === 404 || status >= 500) return true;
    if (status >= 300 && status < 400 && isInteresting(u)) return true;
    if (isInteresting(u) && /auth|upload|nicev2|account|session|user|library|s3_proxy/i.test(classify(u))) return true;
    return false;
  }
  async function peekBody(res){
    try {
      var ct = (res.headers && res.headers.get('content-type')) || '';
      if (!/json|text|javascript|xml/i.test(ct)) return {ct: ct, body: ''};
      var clone = res.clone();
      var t = await clone.text();
      return {ct: ct, body: clip(t.replace(/\s+/g, ' '), 220)};
    } catch (e) {
      return {ct: '', body: ''};
    }
  }

  window.__tmForcePlaceit = forcePI;

  if (typeof nf === 'function') {
    window.fetch = function(input, init){
      var before = urlOf(input);
      try {
        if (typeof input === 'string') input = forcePI(input);
        else if (input && typeof Request !== 'undefined' && input instanceof Request)
          input = new Request(forcePI(input.url), input);
      } catch (e) {}
      var u = urlOf(input) || before;
      var method = methodOf(input, init);
      if (isAdsNoise(u) || isAdsNoise(before)) {
        return Promise.resolve(new Response('', {status:204, statusText:'No Content'}));
      }
      // Leak detection (rewrite missed)
      var kindHint = classify(before);
      if (kindHint === 's3_leak' || kindHint === 'placeit_leak') {
        report(kindHint, {url: clip(before, 240), after: clip(u, 240), method: method});
      }
      try {
        var abs = new URL(u, location.href);
        if (abs.origin === location.origin) {
          init = init ? Object.assign({}, init) : {};
          init.credentials = 'include';
        }
      } catch (e2) {}
      var p = nf.call(this, input, init);
      return Promise.resolve(p).then(function(res){
        if (res == null) {
          report('fetch_null', {url: clip(u, 240), method: method, type: classify(u)});
          return new Response('{}', {status: 502, headers:{'Content-Type':'application/json'}});
        }
        try {
          var st = res.status;
          if (shouldLogStatus(st, u) || shouldLogStatus(st, before)) {
            peekBody(res).then(function(peek){
              report('fetch_status', {
                url: clip(u, 240),
                method: method,
                status: st,
                type: classify(u),
                ct: clip(peek.ct, 80),
                body: peek.body,
                redirected: !!res.redirected,
                final: clip(res.url || '', 180)
              });
            }).catch(function(){
              report('fetch_status', {url: clip(u, 240), method: method, status: st, type: classify(u)});
            });
          }
        } catch (eLog) {}
        return res;
      }).catch(function(err){
        try {
          report('fetch_error', {
            url: clip(u, 240),
            before: clip(before, 180),
            method: method,
            type: classify(u),
            name: clip(err && err.name, 40),
            msg: clip(err && (err.message || err), 160)
          });
        } catch (e3) {}
        return new Response('{}', {status: 502, headers:{'Content-Type':'application/json'}});
      });
    };
  }

  var xo = XMLHttpRequest.prototype.open;
  var xs = XMLHttpRequest.prototype.send;
  XMLHttpRequest.prototype.open = function(m, u){
    try { u = forcePI(u); } catch (e) {}
    this.__tmNetURL = String(u || '');
    this.__tmNetMethod = String(m || 'GET').toUpperCase();
    var args = Array.prototype.slice.call(arguments);
    args[1] = u;
    return xo.apply(this, args);
  };
  XMLHttpRequest.prototype.send = function(){
    var xhr = this;
    var u = xhr.__tmNetURL || '';
    var method = xhr.__tmNetMethod || 'GET';
    if (/placeit[a-z0-9.-]*\.amazonaws\.com/i.test(u) && u.indexOf('/ext-host/') === -1) {
      report('s3_leak_xhr', {url: clip(u, 240), method: method});
    }
    xhr.addEventListener('loadend', function(){
      try {
        var st = xhr.status || 0;
        if (shouldLogStatus(st, u)) {
          var body = '';
          try {
            var ct = xhr.getResponseHeader('content-type') || '';
            if (/json|text/i.test(ct)) body = clip(String(xhr.responseText || '').replace(/\s+/g, ' '), 220);
            report('xhr_status', {url: clip(u, 240), method: method, status: st, type: classify(u), ct: clip(ct, 80), body: body});
          } catch (e) {
            report('xhr_status', {url: clip(u, 240), method: method, status: st, type: classify(u)});
          }
        }
      } catch (e2) {}
    });
    xhr.addEventListener('error', function(){
      report('xhr_error', {url: clip(u, 240), method: method, type: classify(u)});
    });
    return xs.apply(this, arguments);
  };

  // img/srcset — library thumbnails often set .src, not fetch
  try {
    function patchSrcProp(proto, prop){
      var desc = Object.getOwnPropertyDescriptor(proto, prop);
      if (!desc || !desc.set) return;
      Object.defineProperty(proto, prop, {
        configurable: true, enumerable: desc.enumerable, get: desc.get,
        set: function(v){ if (typeof v === 'string') v = forcePI(v); return desc.set.call(this, v); }
      });
    }
    if (typeof HTMLImageElement !== 'undefined') patchSrcProp(HTMLImageElement.prototype, 'src');
    if (typeof HTMLSourceElement !== 'undefined') patchSrcProp(HTMLSourceElement.prototype, 'src');
  } catch (eImg) {}

  // Boot + login-modal snapshot (what Network alone won't show)
  function bootSnap(tag){
    try {
      var txt = '';
      try { txt = (document.body && document.body.innerText) || ''; } catch (e) {}
      var loginModal = /Log in to see your recently uploaded|Ouhh, it's empty/i.test(txt);
      var hasLoginBtn = false;
      try {
        document.querySelectorAll('button,a').forEach(function(el){
          if (/^\s*Login\s*$/i.test(el.textContent || '')) hasLoginBtn = true;
        });
      } catch (e2) {}
      report(tag || 'boot', {
        href: clip(location.href, 180),
        earlyPatch: true,
        loginModal: loginModal,
        hasLoginBtn: hasLoginBtn,
        cookieLen: (document.cookie || '').length,
        hasSessionHint: /logged_session|userStatus|subscriptionType|_session_id/i.test(document.cookie || ''),
        diagLeft: DIAG_MAX - diagSent
      });
    } catch (e3) {}
  }
  setTimeout(function(){ bootSnap('boot'); }, 3500);
  setTimeout(function(){ bootSnap('boot2'); }, 9000);
})();
</script>`
}

func placeitInjectHeadStart(body []byte, script string) []byte {
	if script == "" || len(body) == 0 {
		return body
	}
	if bytes.Contains(body, []byte("data-tm-early-placeit")) {
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
	return append(inj, body...)
}

func placeitClientDiagHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodOptions {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 16<<10))
	detail := strings.TrimSpace(string(body))
	if len(detail) > 1200 {
		detail = detail[:1200] + "…"
	}
	user := ""
	if u, err := getAuthenticatedUser(r, loadConfig()); err == nil {
		user = u
	}
	log.Printf("[CLIENT-DIAG] user=%s %s", user, detail)
	pushProxyLog(ProxyLogEntry{
		Source: "CLIENT",
		Level:  "warn",
		Method: "POST",
		Path:   "/api/client-diag",
		Status: 200,
		User:   user,
		Detail: detail,
	})
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"ok"}`)
}

// placeitShouldNetLog marks proxy paths worth printing as [NET] in app.log.
func placeitShouldNetLog(path string, status int) bool {
	p := strings.ToLower(path)
	if strings.HasPrefix(p, "/api/client-diag") || strings.HasPrefix(p, "/api/device-bind") {
		return status >= 400
	}
	// Skip static/CDN noise unless error
	if strings.Contains(p, ".map") || strings.Contains(p, "/_next/static/") ||
		strings.Contains(p, "fonts.googleapis") || strings.Contains(p, "/css") && strings.Contains(p, "extra-cdn") {
		return status >= 400
	}
	if strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".css") ||
		strings.HasSuffix(p, ".woff") || strings.HasSuffix(p, ".woff2") ||
		strings.HasSuffix(p, ".msh") || strings.Contains(p, "/fonts/") {
		return status >= 400
	}
	interesting := strings.Contains(p, "/ext-host/") ||
		strings.Contains(p, "/extra-cdn-") ||
		strings.Contains(p, "/api/") ||
		strings.Contains(p, "/account") ||
		strings.Contains(p, "upload") ||
		strings.Contains(p, "session") ||
		strings.Contains(p, "user") ||
		strings.Contains(p, "library") ||
		strings.Contains(p, "palette") ||
		strings.Contains(p, "placeitcode") ||
		strings.Contains(p, "alloy.")
	if !interesting {
		return status == 401 || status == 403 || status >= 500
	}
	// Healthy static-ish CDN GETs under extra-cdn/ext-host images: only log APIs / uploads / errors
	if status == 200 && (strings.Contains(p, "/extra-cdn-") || strings.Contains(p, "/ext-host/")) {
		if strings.Contains(p, "/api/") || strings.Contains(p, "upload") ||
			strings.Contains(p, "token") || strings.Contains(p, "user_image") ||
			strings.Contains(p, "data.json") || strings.Contains(p, "palette") ||
			strings.Contains(p, "related_templates") || strings.Contains(p, "stages/") {
			return true
		}
		return false
	}
	return true
}

func placeitNetLog(method, path, host string, status int, contentType string, cookieN int, user, account, detail string) {
	if !placeitShouldNetLog(path, status) {
		return
	}
	ct := contentType
	if idx := strings.Index(ct, ";"); idx >= 0 {
		ct = strings.TrimSpace(ct[:idx])
	}
	if len(ct) > 60 {
		ct = ct[:60]
	}
	d := strings.TrimSpace(detail)
	if len(d) > 180 {
		d = d[:180] + "…"
	}
	if d != "" {
		log.Printf("[NET] %s %s → %d host=%s ct=%s cookies=%d user=%s acc=%s detail=%s",
			method, path, status, host, ct, cookieN, user, account, d)
	} else {
		log.Printf("[NET] %s %s → %d host=%s ct=%s cookies=%d user=%s acc=%s",
			method, path, status, host, ct, cookieN, user, account)
	}
}
