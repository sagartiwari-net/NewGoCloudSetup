package main

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/andybalholm/brotli"

	"recloud.dev/cdncache"
)

var (
	upstreamClient = &http.Client{
		Timeout: 90 * time.Second,
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
			MaxIdleConns:        64,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 15 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	// RankyTools SimilarWeb gateway. Keep *.similarweb.com / similarcdn absolute
	// (Ranky interceptor remaps API hosts onto this origin in-browser).
	rankyHost     = regexp.MustCompile(`(?i)https?://(?:[a-z0-9-]+\.)*rankytools\.com`)
	rtCookieGuard = regexp.MustCompile(`(?is)<script[^>]*id=["']rt-cookie-guard["'][^>]*>.*?</script>`)
)

func publicOrigin(r *http.Request, cfg Config) string {
	scheme := strings.TrimSpace(cfg.PublicScheme)
	if scheme == "" {
		scheme = "http"
	}
	if fwd := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); fwd != "" {
		scheme = strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		host = cfg.PublicHost
	}
	return scheme + "://" + host
}

func hostIsIP(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	// Strip port for bracketless IPv4 / hostname checks.
	if i := strings.Index(host, ":"); i >= 0 {
		host = host[:i]
	}
	if host == "localhost" {
		return true
	}
	dots := 0
	for _, c := range host {
		if c == '.' {
			dots++
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return dots == 3
}

// rewriteText maps RankyTools absolute URLs onto the public origin.
// Local IP origins skip Domain= cookie rewrites (browsers reject .127.0.0.1).
func rewriteText(body []byte, upstreamHost, origin string) []byte {
	if len(body) == 0 || upstreamHost == "" || origin == "" {
		return body
	}
	hostOnly := origin
	if i := strings.Index(origin, "://"); i >= 0 {
		hostOnly = origin[i+3:]
	}
	cookieDomain := hostOnly
	if i := strings.Index(cookieDomain, ":"); i >= 0 {
		cookieDomain = cookieDomain[:i]
	}
	s := string(body)
	s = rtCookieGuard.ReplaceAllString(s, "")
	// Ranky /rt-token page brand chip — never show to end users.
	s = strings.ReplaceAll(s, `<div class="logo">RankyTools</div>`, "")
	s = strings.ReplaceAll(s, `<div class="logo">RankyOS</div>`, "")
	httpOrigin := strings.HasPrefix(strings.ToLower(origin), "http://")
	// Rewrite WS before bare "//host" so http origins get ws:// (not wss://).
	wsScheme := "wss"
	if httpOrigin {
		wsScheme = "ws"
	}
	s = strings.ReplaceAll(s, "wss://"+upstreamHost, wsScheme+"://"+hostOnly)
	s = strings.ReplaceAll(s, "ws://"+upstreamHost, wsScheme+"://"+hostOnly)
	s = strings.ReplaceAll(s, "https://"+upstreamHost, origin)
	s = strings.ReplaceAll(s, "http://"+upstreamHost, origin)
	s = strings.ReplaceAll(s, "//"+upstreamHost, "//"+hostOnly)
	// Ranky SimilarWeb inject hardcodes proxyHost="sw.rankytools.com" (no scheme).
	s = strings.ReplaceAll(s, `"`+upstreamHost+`"`, `"`+hostOnly+`"`)
	s = strings.ReplaceAll(s, `'`+upstreamHost+`'`, `'`+hostOnly+`'`)
	s = strings.ReplaceAll(s, "https://localhost", origin)
	s = strings.ReplaceAll(s, "http://localhost", origin)
	// Never rewrite the Ranky member portal onto the tool origin — that turns
	// logout 302s into http://127.0.0.1?return_url=... loops.
	s = strings.ReplaceAll(s, "member.rankytools.com", "__TM_MEMBER_HOST__")
	s = strings.ReplaceAll(s, "member.rankytools.co", "__TM_MEMBER_HOST_CO__")
	s = rankyHost.ReplaceAllString(s, origin)
	s = strings.ReplaceAll(s, "__TM_MEMBER_HOST__", "member.rankytools.com")
	s = strings.ReplaceAll(s, "__TM_MEMBER_HOST_CO__", "member.rankytools.co")
	// Cloudflare/gateway error pages often print the Ranky host bare
	// (e.g. "Host (sw.rankytools.com)"). Never leak it to end users.
	s = strings.ReplaceAll(s, upstreamHost, hostOnly)
	// Ranky HTML injects document.cookie=...SameSite=None;Secure — invalid on http://.
	if httpOrigin {
		s = strings.ReplaceAll(s, "SameSite=None;Secure", "SameSite=Lax")
		s = strings.ReplaceAll(s, "SameSite=None; Secure", "SameSite=Lax")
		s = strings.ReplaceAll(s, ";SameSite=None", ";SameSite=Lax")
		s = strings.ReplaceAll(s, "; SameSite=None", "; SameSite=Lax")
	}
	if hostIsIP(cookieDomain) {
		return []byte(s)
	}
	s = strings.ReplaceAll(s, "member.rankytools.com", "__TM_MEMBER_HOST__")
	s = strings.ReplaceAll(s, "member.rankytools.co", "__TM_MEMBER_HOST_CO__")
	s = strings.ReplaceAll(s, ".rankytools.com", "."+cookieDomain)
	s = strings.ReplaceAll(s, "__TM_MEMBER_HOST__", "member.rankytools.com")
	s = strings.ReplaceAll(s, "__TM_MEMBER_HOST_CO__", "member.rankytools.co")
	return []byte(s)
}

// mergeSessionCookies is kept for tests; runtime uses cookiesForUpstream + jar.
func mergeSessionCookies(accountCookie, browserCookie string) string {
	browser := browserCookiesForUpstream(&http.Request{
		Header: http.Header{"Cookie": []string{browserCookie}},
	})
	return mergeCookieHeaders(browser, accountCookie)
}

// injectRankyBrandHide removes RankyTools branding and stops AliExpress CDN
// from 403'ing product images (Referer ACL blocks http://127.0.0.1).
func injectRankyBrandHide(body []byte) []byte {
	if bytes.Contains(body, []byte("data-tm-rankybrand")) {
		return body
	}
	snippet := []byte(`<meta name="referrer" content="no-referrer" data-tm-rankybrand><script data-tm-rankybrand>
(function(){
  function scrubBrand(){
    var nodes=document.querySelectorAll(".logo, .ranky-brand, .ranky-logo");
    for(var i=0;i<nodes.length;i++){
      var el=nodes[i];
      var t=(el.textContent||"").replace(/\s+/g," ").trim();
      if(/^ranky(tools|os)$/i.test(t)){
        el.style.setProperty("display","none","important");
        el.setAttribute("hidden","hidden");
      }
    }
  }
  function scrubReferrer(root){
    var nodes=(root||document).querySelectorAll("img,source,video,iframe,a");
    for(var i=0;i<nodes.length;i++){
      try{nodes[i].setAttribute("referrerpolicy","no-referrer");}catch(e){}
    }
  }
  function scrub(){scrubBrand();scrubReferrer(document);}
  scrub();
  try{new MutationObserver(function(muts){
    scrubBrand();
    for(var i=0;i<muts.length;i++){
      var ns=muts[i].addedNodes;
      for(var j=0;j<ns.length;j++){
        var n=ns[j];
        if(n&&n.nodeType===1){
          if(/^(IMG|SOURCE|VIDEO|IFRAME|A)$/.test(n.tagName))try{n.setAttribute("referrerpolicy","no-referrer");}catch(e){}
          scrubReferrer(n);
        }
      }
    }
  }).observe(document.documentElement,{childList:true,subtree:true});}catch(e){}
})();
</script>`)
	lower := bytes.ToLower(body)
	if h := bytes.Index(lower, []byte("<head")); h >= 0 {
		if gt := bytes.IndexByte(lower[h:], '>'); gt >= 0 {
			at := h + gt + 1
			out := make([]byte, 0, len(body)+len(snippet))
			out = append(out, body[:at]...)
			out = append(out, snippet...)
			out = append(out, body[at:]...)
			return out
		}
	}
	if i := bytes.LastIndex(lower, []byte("</body>")); i >= 0 {
		out := make([]byte, 0, len(body)+len(snippet))
		out = append(out, body[:i]...)
		out = append(out, snippet...)
		out = append(out, body[i:]...)
		return out
	}
	return append(snippet, body...)
}

func rewriteOutbound(body []byte, upstreamOrigin, public string) []byte {
	if len(body) == 0 || upstreamOrigin == "" || public == "" {
		return body
	}
	host := public
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	upHost := upstreamOrigin
	if i := strings.Index(upHost, "://"); i >= 0 {
		upHost = upHost[i+3:]
	}
	s := strings.ReplaceAll(string(body), public, upstreamOrigin)
	s = strings.ReplaceAll(s, host, upHost)
	return []byte(s)
}

func rewriteLocation(loc, upstreamHost, origin string) string {
	return string(rewriteText([]byte(loc), upstreamHost, origin))
}

func shouldRewriteBody(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "text/") ||
		strings.Contains(ct, "javascript") ||
		strings.Contains(ct, "json") ||
		strings.Contains(ct, "xml") ||
		strings.Contains(ct, "svg")
}

func isCacheableStaticPath(path string) bool {
	p := path
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	p = strings.ToLower(p)
	if strings.HasPrefix(p, "/__static__/") || strings.HasPrefix(p, "/assets/") || strings.HasPrefix(p, "/static/") || strings.HasPrefix(p, "/css/") || strings.HasPrefix(p, "/js/") {
		return true
	}
	for _, ext := range []string{".js", ".css", ".woff2", ".woff", ".ttf", ".eot", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

func readUpstreamBody(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body
	switch strings.ToLower(resp.Header.Get("Content-Encoding")) {
	case "gzip":
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		reader = gr
	case "br":
		reader = brotli.NewReader(resp.Body)
	}
	return io.ReadAll(io.LimitReader(reader, 20<<20))
}

func proxyUpstream(w http.ResponseWriter, r *http.Request, cfg Config, acc ToolAccount) {
	base, err := url.Parse(liveUpstream(cfg))
	if err != nil || base.Host == "" {
		http.Error(w, "Upstream URL is not configured", http.StatusBadGateway)
		return
	}
	origin := publicOrigin(r, cfg)
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		proxyWebSocket(w, r, acc, base)
		return
	}
	target := *base
	target.Path = r.URL.Path
	target.RawPath = r.URL.RawPath
	target.RawQuery = r.URL.RawQuery
	cdnKey := cdncache.RequestKey(r, isCacheableStaticPath(r.URL.Path))
	if cdncache.Serve(w, r, cdnKey) {
		return
	}
	if cdnKey != "" {
		defer cdncache.Complete(cdnKey)
	}

	var body io.Reader
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		raw, err := io.ReadAll(io.LimitReader(r.Body, 20<<20))
		if err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		if shouldRewriteBody(r.Header.Get("Content-Type")) {
			raw = rewriteOutbound(raw, base.Scheme+"://"+base.Host, origin)
		}
		body = bytes.NewReader(raw)
	}
	upReq, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), body)
	if err != nil {
		http.Error(w, "Bad gateway", http.StatusBadGateway)
		return
	}
	copyRequestHeaders(upReq, r)
	upReq.Host = base.Host
	upReq.Header.Set("Host", base.Host)
	upReq.Header.Set("Origin", base.Scheme+"://"+base.Host)
	if ref := r.Referer(); ref != "" {
		upReq.Header.Set("Referer", string(rewriteOutbound([]byte(ref), base.Scheme+"://"+base.Host, origin)))
	} else {
		upReq.Header.Set("Referer", base.Scheme+"://"+base.Host+"/")
	}
	jar := cookiesForUpstream(acc, r)
	if jar != "" {
		upReq.Header.Set("Cookie", jar)
	} else {
		upReq.Header.Del("Cookie")
	}
	upReq.Header.Set("Accept-Encoding", "gzip, deflate, br")
	if upReq.Header.Get("User-Agent") == "" {
		upReq.Header.Set("User-Agent", "Mozilla/5.0")
	}

	resp, err := upstreamClient.Do(upReq)
	if err != nil {
		log.Printf("[UP] %s %s: %v", r.Method, r.URL.Path, err)
		http.Error(w, "Upstream unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	jar = absorbSetCookies(jar, resp.Header)
	rememberUpstreamCookies(acc, jar)

	payload, err := readUpstreamBody(resp)
	if err != nil {
		http.Error(w, "Upstream unreadable", http.StatusBadGateway)
		return
	}
	ct := resp.Header.Get("Content-Type")
	rawLoc := resp.Header.Get("Location")
	loc := rawLoc
	if loc != "" {
		loc = rewriteLocation(loc, base.Host, origin)
	}
	// Dead Ranky PHPSESSID → 302 member.rankytools.com?return_url=...
	// Rewriting that Location onto our origin causes ERR_TOO_MANY_REDIRECTS.
	if resp.StatusCode >= 300 && resp.StatusCode < 400 &&
		(isRankyMemberLogout(rawLoc) || isRankyReturnURLBounce(r, rawLoc, origin) ||
			isRankyReturnURLBounce(r, loc, origin) || homeRedirectLoop(r.URL.Path, loc, origin)) {
		token := sessionTokenFromRequest(r)
		_, _ = noteLogout(cfg, r, acc, token, "session_expired")
		renderLoggedOutPage(w, cfg)
		return
	}
	if shouldRewriteBody(ct) && len(payload) > 0 && len(payload) <= 8<<20 {
		payload = rewriteText(payload, base.Host, origin)
	}
	if bodyLooksLoggedOut(payload) {
		token := sessionTokenFromRequest(r)
		switched, _ := noteLogout(cfg, r, acc, token, "session_expired")
		homePathOnly := strings.SplitN(strings.TrimSpace(cfg.HomePath), "?", 2)[0]
		if switched && homePathOnly != "" && homePathOnly != "/" && !strings.EqualFold(homePathOnly, r.URL.Path) {
			http.Redirect(w, r, cfg.HomePath, http.StatusFound)
			return
		}
		renderLoggedOutPage(w, cfg)
		return
	}
	isHTMLDoc := isDocumentNavigation(r) && strings.Contains(strings.ToLower(ct), "text/html")
	if isHTMLDoc {
		// No Change Account (Semrush-only).
		payload = injectRankyBrandHide(payload)
		if usesPanelAccountMode(cfg) {
			payload = injectDeviceHTML(payload)
		}
	}

	if cdnKey != "" && resp.StatusCode == http.StatusOK {
		cdncache.Store(cdnKey, resp.StatusCode, ct, payload)
		if cdncache.ServeDisk(w, r, cdnKey) {
			return
		}
	}

	h := w.Header()
	copyResponseHeaders(h, resp.Header, base.Host, origin)
	// AliExpress CDN denies Referer=http://127.0.0.1 (ACL). Strip referrer on docs.
	if isHTMLDoc {
		h.Set("Referrer-Policy", "no-referrer")
	}
	// Static assets: Ahrefs-style long browser cache. HTML/API stay uncached.
	if cdnKey != "" {
		h.Set("Cache-Control", cdncache.CacheControl(cdnKey))
		h.Del("Pragma")
	} else {
		h.Set("Cache-Control", "no-store")
		h.Set("Pragma", "no-cache")
	}
	h.Del("Content-Encoding")
	h.Del("Content-Length")
	w.WriteHeader(resp.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = w.Write(payload)
	}
}

func copyRequestHeaders(dst *http.Request, src *http.Request) {
	for key, values := range src.Header {
		lk := strings.ToLower(key)
		switch lk {
		case "cookie", "host", "content-length", "accept-encoding", "connection", "upgrade", "proxy-connection", "x-device-fp", "x-device-proof":
			continue
		}
		for _, v := range values {
			dst.Header.Add(key, v)
		}
	}
}

func copyResponseHeaders(dst, src http.Header, upstreamHost, origin string) {
	httpOrigin := strings.HasPrefix(strings.ToLower(strings.TrimSpace(origin)), "http://")
	for key, values := range src {
		lk := strings.ToLower(key)
		switch lk {
		case "content-length", "content-encoding", "transfer-encoding", "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "trailer", "upgrade":
			continue
		case "content-security-policy", "content-security-policy-report-only":
			// Upstream sends upgrade-insecure-requests. On local http:// that forces
			// https://127.0.0.1 and the app never leaves the spinner.
			if httpOrigin {
				continue
			}
		case "access-control-allow-origin":
			dst.Set("Access-Control-Allow-Origin", origin)
			continue
		case "set-cookie":
			for _, v := range values {
				if rewritten, ok := rewriteSetCookie(v, upstreamHost, origin); ok {
					dst.Add("Set-Cookie", rewritten)
				}
			}
			continue
		case "location":
			for _, v := range values {
				dst.Add("Location", rewriteLocation(v, upstreamHost, origin))
			}
			continue
		}
		for _, v := range values {
			dst.Add(key, v)
		}
	}
}

func rewriteSetCookie(raw, upstreamHost, origin string) (string, bool) {
	parts := strings.Split(raw, ";")
	if len(parts) == 0 {
		return "", false
	}
	name := strings.TrimSpace(strings.SplitN(parts[0], "=", 2)[0])
	switch strings.ToLower(name) {
	case "proxy_token", "ct_session":
		return "", false
	}
	host := origin
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	if i := strings.Index(host, ":"); i >= 0 {
		host = host[:i]
	}
	out := []string{parts[0]}
	httpOrigin := strings.HasPrefix(strings.ToLower(strings.TrimSpace(origin)), "http://")
	for _, part := range parts[1:] {
		p := strings.TrimSpace(part)
		pl := strings.ToLower(p)
		if len(p) >= 7 && strings.EqualFold(p[:7], "domain=") {
			// Browsers reject Domain=<IP> and Domain=.grammarly.com on our host.
			if hostIsIP(host) {
				continue
			}
			out = append(out, "Domain="+host)
			continue
		}
		if httpOrigin && (pl == "secure" || strings.HasPrefix(pl, "secure=")) {
			continue
		}
		// SameSite=None requires Secure; after stripping Secure, force Lax so
		// Chrome accepts SimilarWeb/Ranky cookies on http://127.0.0.1.
		if httpOrigin && strings.HasPrefix(pl, "samesite=") {
			if strings.Contains(pl, "none") {
				out = append(out, "SameSite=Lax")
				continue
			}
		}
		out = append(out, part)
	}
	_ = upstreamHost
	return strings.Join(out, ";"), true
}
