package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// placeitEarlyCDNPatch rewrites Placeit commercial / img / placeitcode hosts before
// the editor boots. Missing rewrite → CORS → fetch undefined → "reading 'ok'".
func placeitEarlyCDNPatch() string {
	return `<script data-tm-early-placeit="1">
(function(){
  if (window.__tmEarlyPlaceit) return;
  window.__tmEarlyPlaceit = true;
  var O = location.origin;
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
    return /doubleclick\.net|googlesyndication\.com|google-analytics\.com|googletagmanager\.com|facebook\.net\/tr/i.test(String(u||''));
  }
  window.__tmForcePlaceit = forcePI;
  var nf = window.fetch;
  if (typeof nf === 'function') {
    window.fetch = function(input, init){
      try {
        if (typeof input === 'string') input = forcePI(input);
        else if (input && typeof Request !== 'undefined' && input instanceof Request)
          input = new Request(forcePI(input.url), input);
      } catch (e) {}
      try {
        var u0 = typeof input === 'string' ? input : (input && input.url) || '';
        if (isAdsNoise(u0)) {
          return Promise.resolve(new Response('', {status:204, statusText:'No Content'}));
        }
        var abs = new URL(u0, location.href);
        if (abs.origin === location.origin) {
          init = init ? Object.assign({}, init) : {};
          init.credentials = 'include';
        }
      } catch (e2) {}
      var p = nf.call(this, input, init);
      // Never resolve to undefined — Placeit editor crashes on response.ok
      return Promise.resolve(p).then(function(res){
        if (res == null) {
          return new Response('{}', {status: 502, headers:{'Content-Type':'application/json'}});
        }
        return res;
      }).catch(function(err){
        try {
          var body = JSON.stringify({kind:'fetch_error', url: String(typeof input==='string'?input:(input&&input.url)||'').slice(0,220), msg: String(err&&err.message||err).slice(0,160)});
          if (navigator.sendBeacon) navigator.sendBeacon('/api/client-diag', new Blob([body],{type:'application/json'}));
        } catch (e3) {}
        return new Response('{}', {status: 502, headers:{'Content-Type':'application/json'}});
      });
    };
  }
  var xo = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function(m, u){
    try { u = forcePI(u); } catch (e) {}
    var args = Array.prototype.slice.call(arguments);
    args[1] = u;
    return xo.apply(this, args);
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
	body, _ := io.ReadAll(io.LimitReader(r.Body, 8<<10))
	detail := strings.TrimSpace(string(body))
	if len(detail) > 500 {
		detail = detail[:500] + "…"
	}
	log.Printf("[CLIENT-DIAG] %s", detail)
	pushProxyLog(ProxyLogEntry{
		Source: "CLIENT",
		Level:  "warn",
		Method: "POST",
		Path:   "/api/client-diag",
		Status: 200,
		Detail: detail,
	})
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"ok"}`)
}
