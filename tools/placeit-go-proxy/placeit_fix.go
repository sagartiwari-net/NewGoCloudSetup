package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// placeitUnrewriteRequestBody maps browser-facing proxy URLs back to real Placeit/S3
// hosts before POSTing upstream. Without this, nicev2 jobs see
// placeit.gt4rents.com/extra-cdn-N/... and return "Failed to get output image".
func placeitUnrewriteRequestBody(body []byte, publicScheme, publicHost string, cfg Config) []byte {
	if len(body) == 0 || publicHost == "" {
		return body
	}
	if !bytes.Contains(body, []byte(publicHost)) {
		return body
	}
	if publicScheme == "" {
		publicScheme = "https"
	}
	publicBase := publicScheme + "://" + publicHost

	// Longer proxy prefixes first (extra-cdn / cdn-proxy / ext-host) before bare origin.
	for i, extra := range cfg.ExtraCDNDomains {
		host := strings.TrimPrefix(strings.TrimPrefix(extra, "https://"), "http://")
		host = strings.Split(host, "/")[0]
		if host == "" {
			continue
		}
		from := fmt.Sprintf("%s/extra-cdn-%d", publicBase, i)
		to := "https://" + host
		body = bytes.ReplaceAll(body, []byte(from), []byte(to))
		body = bytes.ReplaceAll(body,
			[]byte(strings.ReplaceAll(from, "/", `\/`)),
			[]byte(strings.ReplaceAll(to, "/", `\/`)))
		// http:// public variants
		fromHTTP := fmt.Sprintf("http://%s/extra-cdn-%d", publicHost, i)
		body = bytes.ReplaceAll(body, []byte(fromHTTP), []byte(to))
		body = bytes.ReplaceAll(body,
			[]byte(strings.ReplaceAll(fromHTTP, "/", `\/`)),
			[]byte(strings.ReplaceAll(to, "/", `\/`)))
	}

	if cfg.CDNURL != "" {
		if cdn, err := url.Parse(cfg.CDNURL); err == nil && cdn.Host != "" {
			from := publicBase + "/cdn-proxy"
			to := "https://" + cdn.Host
			body = bytes.ReplaceAll(body, []byte(from), []byte(to))
			body = bytes.ReplaceAll(body,
				[]byte(strings.ReplaceAll(from, "/", `\/`)),
				[]byte(strings.ReplaceAll(to, "/", `\/`)))
		}
	}

	reExt := regexp.MustCompile(`(?i)https?://` + regexp.QuoteMeta(publicHost) + `/ext-host/([a-z0-9.-]+)`)
	body = reExt.ReplaceAll(body, []byte("https://$1"))
	reExtEsc := regexp.MustCompile(`(?i)https?:\\/\\/` + regexp.QuoteMeta(publicHost) + `\\/ext-host\\/([a-z0-9.-]+)`)
	body = reExtEsc.ReplaceAll(body, []byte(`https:\/\/$1`))

	targetHost := ""
	if u, err := url.Parse(cfg.TargetURL); err == nil {
		targetHost = u.Host
	}
	if targetHost != "" {
		to := "https://" + targetHost
		body = bytes.ReplaceAll(body, []byte(publicBase), []byte(to))
		body = bytes.ReplaceAll(body,
			[]byte(strings.ReplaceAll(publicBase, "/", `\/`)),
			[]byte(strings.ReplaceAll(to, "/", `\/`)))
		body = bytes.ReplaceAll(body, []byte("http://"+publicHost), []byte(to))
		body = bytes.ReplaceAll(body,
			[]byte(strings.ReplaceAll("http://"+publicHost, "/", `\/`)),
			[]byte(strings.ReplaceAll(to, "/", `\/`)))
	}
	return body
}

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
  function isHeavyDiagURL(u){
    // related_templates_jobs returns multi‑MB base64 — never clone/read it
    return /related_templates_jobs|\/api\/v4\/jobs\/?/i.test(String(u||''));
  }
  function shouldLogStatus(status, u){
    if (isHeavyDiagURL(u)) return status === 401 || status === 403 || status >= 500;
    if (status === 401 || status === 403 || status === 404 || status >= 500) return true;
    if (status >= 300 && status < 400 && isInteresting(u)) return true;
    if (isInteresting(u) && /auth|upload|account|session|user|library|s3_proxy/i.test(classify(u))) return true;
    // nicev2 OK noise — skip (was flooding + slowing UI)
    if (/nicev2/i.test(classify(u)) && status === 200) return false;
    return false;
  }
  async function peekBody(res){
    try {
      var ct = (res.headers && res.headers.get('content-type')) || '';
      if (!/json|text|javascript|xml/i.test(ct)) return {ct: ct, body: ''};
      var cl = parseInt(res.headers.get('content-length') || '0', 10);
      if (cl > 8000) return {ct: ct, body: '(skipped large body ' + cl + 'b)'};
      var clone = res.clone();
      var t = await clone.text();
      if (t && t.length > 12000) return {ct: ct, body: '(skipped large body ' + t.length + 'b)'};
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
            if (isHeavyDiagURL(u) || isHeavyDiagURL(before)) {
              report('fetch_status', {url: clip(u, 240), method: method, status: st, type: classify(u), body: '(heavy skipped)'});
            } else {
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
          if (isHeavyDiagURL(u)) {
            report('xhr_status', {url: clip(u, 240), method: method, status: st, type: classify(u), body: '(heavy skipped)'});
            return;
          }
          var body = '';
          try {
            var ct = xhr.getResponseHeader('content-type') || '';
            var cl = parseInt(xhr.getResponseHeader('content-length') || '0', 10);
            if (cl > 8000) body = '(skipped large body ' + cl + 'b)';
            else if (/json|text/i.test(ct)) {
              var rt = String(xhr.responseText || '');
              if (rt.length > 12000) body = '(skipped large body ' + rt.length + 'b)';
              else body = clip(rt.replace(/\s+/g, ' '), 220);
            }
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

  // Classic editor upload pane shows "Log in to see…" when client cookies look logged-out
  // (upstream auth still works via proxy). Hide that gate so library / select works.
  function fixUploadLibraryGate(){
    try {
      var nodes = document.querySelectorAll('div,section,aside');
      for (var i = 0; i < nodes.length && i < 900; i++) {
        var el = nodes[i];
        var kids = el.childNodes ? el.childNodes.length : 99;
        if (kids > 14) continue;
        var t = el.textContent || '';
        if (t.length < 20 || t.length > 420) continue;
        if (/Log in to see your recently uploaded images/i.test(t) ||
            (/Ouhh, it's empty in here/i.test(t) && /Login/i.test(t))) {
          el.style.setProperty('display', 'none', 'important');
          el.setAttribute('data-tm-upload-gate', '1');
        }
      }
      document.querySelectorAll('button').forEach(function(btn){
        if (!/^\s*Login\s*$/i.test(btn.textContent || '')) return;
        var ctx = (btn.parentElement && btn.parentElement.textContent) || '';
        if (/recently uploaded|Ouhh, it's empty/i.test(ctx)) {
          btn.style.setProperty('display', 'none', 'important');
          try { btn.disabled = true; } catch (e) {}
        }
      });
    } catch (e) {}
  }
  try {
    new MutationObserver(fixUploadLibraryGate).observe(document.documentElement, {childList:true, subtree:true});
    setInterval(fixUploadLibraryGate, 800);
    setTimeout(fixUploadLibraryGate, 500);
  } catch (eGate) {}
})();
</script>`
}

func placeitCookieValue(header, name string) string {
	name = strings.TrimSpace(name)
	if header == "" || name == "" {
		return ""
	}
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		eq := strings.Index(part, "=")
		if eq <= 0 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(part[:eq]), name) {
			return strings.TrimSpace(part[eq+1:])
		}
	}
	return ""
}

// placeitMirrorBrowserAuthCookies exposes Placeit auth cookies to the browser so
// classic editor client-side checks treat the session as logged-in (upload library).
func placeitMirrorBrowserAuthCookies(w http.ResponseWriter, r *http.Request, accountCookie string) {
	if accountCookie == "" {
		return
	}
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	for _, name := range []string{"userStatus", "subscriptionType", "logged_session", "_session_id"} {
		val := placeitCookieValue(accountCookie, name)
		if val == "" {
			continue
		}
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    val,
			Path:     "/",
			MaxAge:   7 * 24 * 3600,
			Secure:   secure,
			HttpOnly: false,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

// placeitAuthCookieSeedScript sets auth cookies before Placeit SPA boots.
func placeitAuthCookieSeedScript(accountCookie string) string {
	if accountCookie == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<script data-tm-placeit-auth="1">(function(){try{`)
	for _, name := range []string{"userStatus", "subscriptionType", "logged_session", "_session_id"} {
		val := placeitCookieValue(accountCookie, name)
		if val == "" {
			continue
		}
		line := name + "=" + val + "; path=/; SameSite=Lax; max-age=604800"
		raw, err := json.Marshal(line)
		if err != nil {
			continue
		}
		b.WriteString(`document.cookie=`)
		b.Write(raw)
		b.WriteString(`;`)
	}
	b.WriteString(`}catch(e){}})();</script>`)
	return b.String()
}

func placeitSkipHeavyJSONRewrite(path string, contentLength int64) bool {
	p := strings.ToLower(path)
	if strings.Contains(p, "related_templates_jobs") || strings.Contains(p, "/api/v4/jobs") {
		return true
	}
	if contentLength > 350000 {
		return true
	}
	return false
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
