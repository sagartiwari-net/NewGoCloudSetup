package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

// placeitEarlyCDNPatch rewrites Placeit CDN/S3 hosts before the editor boots.
// (No CLIENT-DIAG / network logging — keep this lean.)
func placeitEarlyCDNPatch() string {
	return `<script data-tm-early-placeit="1">
(function(){
  if (window.__tmEarlyPlaceit) return;
  window.__tmEarlyPlaceit = true;
  var O = location.origin;
  var nf = window.fetch;
  var RELATED_STUB = '{"_internalRemoveWatermark":true,"previewImage":{"type":"base64","value":"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="},"highResolution":false,"migrated":true}';

  function forcePI(u){
    if (u == null) return u;
    if (typeof u !== 'string') {
      try { u = (u && u.url) ? String(u.url) : String(u); } catch (e) { return u; }
    }
    u = u.replace(/https?:\/\/(placeit[a-z0-9.-]*\.amazonaws\.com)/gi, O + '/ext-host/$1');
    u = u.replace(/(^|[^:\/])\/\/(placeit[a-z0-9.-]*\.amazonaws\.com)/gi, '$1' + O + '/ext-host/$2');
    u = u.replace(/https?:\/\/((?:[a-z0-9-]+\.)*cdn\.aws\.placeit\.net)/gi, O + '/ext-host/$1');
    u = u.replace(/https?:\/\/((?:[a-z0-9-]+\.)*placeitcode\.net)/gi, O + '/ext-host/$1');
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
    return /doubleclick\.net|googlesyndication\.com|google-analytics\.com|googletagmanager\.com|facebook\.net\/tr|recurly\.com|kaptcha\.com/i.test(String(u||''));
  }
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

  window.__tmForcePlaceit = forcePI;

  if (typeof nf === 'function') {
    window.fetch = function(input, init){
      try {
        if (typeof input === 'string') input = forcePI(input);
        else if (input && typeof Request !== 'undefined' && input instanceof Request)
          input = new Request(forcePI(input.url), input);
      } catch (e) {}
      var u = urlOf(input);
      var method = methodOf(input, init);
      if (isAdsNoise(u)) {
        return Promise.resolve(new Response('', {status:204, statusText:'No Content'}));
      }
      if (/related_templates_jobs/i.test(u) && method === 'POST') {
        return Promise.resolve(new Response(RELATED_STUB, {status:200, headers:{'Content-Type':'application/json'}}));
      }
      try {
        var abs = new URL(u, location.href);
        if (abs.origin === location.origin) {
          init = init ? Object.assign({}, init) : {};
          init.credentials = 'include';
        }
      } catch (e2) {}
      return Promise.resolve(nf.call(this, input, init)).then(function(res){
        if (res == null) {
          return new Response('{}', {status: 502, headers:{'Content-Type':'application/json'}});
        }
        return res;
      }).catch(function(){
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
    if (/related_templates_jobs/i.test(u) && method === 'POST') {
      try {
        Object.defineProperty(xhr, 'readyState', {configurable:true, get:function(){ return 4; }});
        Object.defineProperty(xhr, 'status', {configurable:true, get:function(){ return 200; }});
        Object.defineProperty(xhr, 'responseText', {configurable:true, get:function(){ return RELATED_STUB; }});
        Object.defineProperty(xhr, 'response', {configurable:true, get:function(){ return RELATED_STUB; }});
        xhr.getResponseHeader = function(h){ if (String(h).toLowerCase() === 'content-type') return 'application/json'; return null; };
        xhr.getAllResponseHeaders = function(){ return 'content-type: application/json\r\n'; };
      } catch (eStub) {}
      setTimeout(function(){
        try { if (typeof xhr.onreadystatechange === 'function') xhr.onreadystatechange(); } catch (e1) {}
        try { if (typeof xhr.onload === 'function') xhr.onload(); } catch (e2) {}
        try { xhr.dispatchEvent && xhr.dispatchEvent(new Event('load')); } catch (e3) {}
        try { xhr.dispatchEvent && xhr.dispatchEvent(new Event('loadend')); } catch (e4) {}
      }, 0);
      return;
    }
    return xs.apply(this, arguments);
  };

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

// placeitChromeScript hides ONLY pricing / My Account / Log out nav items,
// and replaces .username-container .username with the panel member username.
func placeitChromeScript(panelUsername string) string {
	userJS, _ := json.Marshal(strings.TrimSpace(panelUsername))
	return fmt.Sprintf(`<script data-tm-placeit-chrome="1">
(function(){
  if (window.__tmPlaceitChrome) return;
  window.__tmPlaceitChrome = true;
  var TM_USER = %s;
  function hide(el){
    if (!el || (el.dataset && el.dataset.tmPiHide === '1')) return;
    el.style.setProperty('display','none','important');
    el.style.setProperty('visibility','hidden','important');
    el.setAttribute('aria-hidden','true');
    if (el.dataset) el.dataset.tmPiHide = '1';
  }
  function run(){
    try {
      // 1) "Go Annual!" pricing button only
      document.querySelectorAll('#buy-a-subscription-mobile, a.main-pricing-button').forEach(function(a){
        hide(a.closest('div.item.pricing') || a.closest('.item.pricing') || a);
      });
      document.querySelectorAll('div.item.pricing.square-button.has-subscription').forEach(hide);

      // 2) My Account → /account (exact href only — do not touch other links)
      document.querySelectorAll('a[href="/account"], a[href="/account/"]').forEach(function(a){
        hide(a.closest('li') || a);
      });

      // 3) Log out
      document.querySelectorAll('a.logout-button, a#logout-button, #logout-button, a[href="/logout"], a[href="/logout/"]').forEach(function(a){
        hide(a.closest('li') || a);
      });

      // 4) Show panel username instead of Placeit account name
      if (TM_USER) {
        document.querySelectorAll('.username-container span.username, .username-container .username').forEach(function(span){
          if (span.textContent !== TM_USER) span.textContent = TM_USER;
        });
      }
    } catch (e) {}
  }
  run();
  try {
    new MutationObserver(run).observe(document.documentElement, {childList:true, subtree:true});
  } catch (e2) {}
  setInterval(run, 1000);
})();
</script>`, userJS)
}

// placeitShouldUnrewriteBody — only JSON/text APIs. Multipart uploads must stream.
func placeitShouldUnrewriteBody(method, path, contentType string) bool {
	m := strings.ToUpper(method)
	if m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions {
		return false
	}
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "multipart/") || strings.Contains(ct, "octet-stream") ||
		strings.Contains(ct, "image/") || strings.Contains(ct, "video/") || strings.Contains(ct, "audio/") {
		return false
	}
	p := strings.ToLower(path)
	if strings.Contains(p, "/cropper/upload") || strings.Contains(p, "/upload") && strings.Contains(ct, "multipart") {
		return false
	}
	if strings.Contains(ct, "json") || strings.Contains(ct, "text/") ||
		strings.Contains(ct, "javascript") || strings.Contains(ct, "urlencoded") {
		return true
	}
	// JSON APIs sometimes omit Content-Type
	if strings.Contains(p, "related_templates") || strings.Contains(p, "/api/v4/jobs") ||
		strings.Contains(p, "/api/") {
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

// placeitClientDiagHandler is a silent no-op (diag logging removed).
func placeitClientDiagHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Body != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 4<<10))
		_ = r.Body.Close()
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"ok"}`)
}
