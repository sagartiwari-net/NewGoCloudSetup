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
	// RankyTools gateway host. Ubersuggest/neilpatel absolute hosts stay as-is
	// (Ranky's PROXY_INTERCEPTOR maps the ones it needs via PROXY_BASE).
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

// rewriteText maps RankyTools + Ubersuggest absolute URLs onto the public origin.
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
	// Ranky injects a guard that deletes every non-amember_* cookie in the
	// browser — that would wipe our local session jar. Strip it always.
	s = rtCookieGuard.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "https://"+upstreamHost, origin)
	s = strings.ReplaceAll(s, "http://"+upstreamHost, origin)
	s = strings.ReplaceAll(s, "//"+upstreamHost, "//"+hostOnly)
	s = strings.ReplaceAll(s, `"`+upstreamHost+`"`, `"`+hostOnly+`"`)
	s = strings.ReplaceAll(s, `'`+upstreamHost+`'`, `'`+hostOnly+`'`)
	s = strings.ReplaceAll(s, "https://localhost", origin)
	s = strings.ReplaceAll(s, "http://localhost", origin)
	s = strings.ReplaceAll(s, "member.rankytools.com", "__TM_MEMBER_HOST__")
	s = strings.ReplaceAll(s, "member.rankytools.co", "__TM_MEMBER_HOST_CO__")
	s = rankyHost.ReplaceAllString(s, origin)
	s = strings.ReplaceAll(s, "__TM_MEMBER_HOST__", "member.rankytools.com")
	s = strings.ReplaceAll(s, "__TM_MEMBER_HOST_CO__", "member.rankytools.co")
	// Cloudflare/gateway error pages often print the Ranky host bare
	// (e.g. "Host (sw.rankytools.com)"). Never leak it to end users.
	s = strings.ReplaceAll(s, upstreamHost, hostOnly)
	// Keep ubersuggest.com / neilpatelapi.com absolute — rewriting them onto
	// 127.0.0.1 breaks Site Audit / hydration (React Oops page).
	if hostIsIP(cookieDomain) {
		return []byte(rewriteRankyBrand(s))
	}
	s = strings.ReplaceAll(s, "member.rankytools.com", "__TM_MEMBER_HOST__")
	s = strings.ReplaceAll(s, "member.rankytools.co", "__TM_MEMBER_HOST_CO__")
	s = strings.ReplaceAll(s, ".rankytools.com", "."+cookieDomain)
	s = strings.ReplaceAll(s, "__TM_MEMBER_HOST__", "member.rankytools.com")
	s = strings.ReplaceAll(s, "__TM_MEMBER_HOST_CO__", "member.rankytools.co")
	return []byte(rewriteRankyBrand(s))
}

// rewriteRankyBrand is intentionally a no-op on Ubersuggest — Change Account
// is Semrush-only until the user asks for it elsewhere.
func rewriteRankyBrand(s string) string { return s }

// injectNavFixHTML appends a hard-navigation helper before </body>.
// Must not inject into <head> / React root — that triggers hydration Oops pages.
func injectNavFixHTML(body []byte) []byte {
	if bytes.Contains(body, []byte("data-tm-navfix")) {
		return body
	}
	script := []byte(navFixPageScript())
	lower := bytes.ToLower(body)
	if i := bytes.LastIndex(lower, []byte("</body>")); i >= 0 {
		out := make([]byte, 0, len(body)+len(script))
		out = append(out, body[:i]...)
		out = append(out, script...)
		out = append(out, body[i:]...)
		return out
	}
	return append(body, script...)
}

func navFixPageScript() string {
	return `<script data-tm-navfix>
(function(){
  if(window.__tmNavFix)return;window.__tmNavFix=1;
  function go(e){
    if(e.defaultPrevented||e.metaKey||e.ctrlKey||e.shiftKey||e.altKey)return;
    if(e.type==="click"&&e.button!==0)return;
    var t=e.target; if(!t||!t.closest)return;
    var a=t.closest("a[href]"); if(!a)return;
    if(a.target&&a.target!=="_self"&&a.target!=="")return;
    if(a.hasAttribute("download"))return;
    var href=a.getAttribute("href"); if(!href||href.charAt(0)==="#")return;
    var u; try{u=new URL(href,location.href);}catch(err){return;}
    if(u.origin!==location.origin)return;
    if(u.pathname===location.pathname&&u.search===location.search)return;
    e.preventDefault(); e.stopImmediatePropagation();
    location.href=u.pathname+u.search+u.hash;
  }
  document.addEventListener("click",go,true);
  document.addEventListener("pointerup",go,true);
})();
</script>`
}

// stripNextRSC drops App Router flight headers/query so upstream returns HTML
// on us.rankytools.com instead of 301 → ubersuggest.com.
func stripNextRSC(req *http.Request) {
	if req == nil {
		return
	}
	for _, h := range []string{
		"RSC",
		"Next-Router-State-Tree",
		"Next-Router-Prefetch",
		"Next-Router-Segment-Prefetch",
		"Next-Url",
		"Next-Action",
	} {
		req.Header.Del(h)
	}
	if req.URL == nil {
		return
	}
	q := req.URL.Query()
	if q.Has("_rsc") {
		q.Del("_rsc")
		req.URL.RawQuery = q.Encode()
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
}

func locationHost(loc string) string {
	u, err := url.Parse(strings.TrimSpace(loc))
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func isUbersuggestBounce(loc string) bool {
	h := locationHost(loc)
	return h == "ubersuggest.com" || strings.HasSuffix(h, ".ubersuggest.com")
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
	if strings.HasPrefix(p, "/__static__/") || strings.HasPrefix(p, "/cdn-neilpatel/") {
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
	target := *base
	target.Path = r.URL.Path
	target.RawPath = r.URL.RawPath
	target.RawQuery = r.URL.RawQuery
	origin := publicOrigin(r, cfg)
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
	stripNextRSC(upReq)
	upReq.Host = base.Host
	upReq.Header.Set("Host", base.Host)
	upReq.Header.Set("Origin", base.Scheme+"://"+base.Host)
	if ref := r.Referer(); ref != "" {
		upReq.Header.Set("Referer", string(rewriteOutbound([]byte(ref), base.Scheme+"://"+base.Host, origin)))
	} else {
		upReq.Header.Set("Referer", base.Scheme+"://"+base.Host+"/")
	}
	if acc.Cookie != "" {
		upReq.Header.Set("Cookie", acc.Cookie)
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
	// Canonical bounce us → ubersuggest.com (same path): refetch as HTML on
	// the Ranky host so the browser never enters a rewritten self-301 loop.
	if resp.StatusCode >= 300 && resp.StatusCode < 400 && isUbersuggestBounce(resp.Header.Get("Location")) &&
		(r.Method == http.MethodGet || r.Method == http.MethodHead) {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		retryURL := *base
		retryURL.Path = r.URL.Path
		retryURL.RawPath = r.URL.RawPath
		retryURL.RawQuery = ""
		retry, err := http.NewRequestWithContext(r.Context(), http.MethodGet, retryURL.String(), nil)
		if err != nil {
			http.Error(w, "Bad gateway", http.StatusBadGateway)
			return
		}
		copyRequestHeaders(retry, r)
		stripNextRSC(retry)
		retry.Host = base.Host
		retry.Header.Set("Host", base.Host)
		retry.Header.Set("Origin", base.Scheme+"://"+base.Host)
		retry.Header.Set("Referer", base.Scheme+"://"+base.Host+"/")
		retry.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		retry.Header.Set("Accept-Encoding", "gzip, deflate, br")
		if acc.Cookie != "" {
			retry.Header.Set("Cookie", acc.Cookie)
		}
		if retry.Header.Get("User-Agent") == "" {
			retry.Header.Set("User-Agent", "Mozilla/5.0")
		}
		resp, err = upstreamClient.Do(retry)
		if err != nil {
			log.Printf("[UP] bounce-retry %s: %v", r.URL.Path, err)
			http.Error(w, "Upstream unavailable", http.StatusBadGateway)
			return
		}
	}
	defer resp.Body.Close()

	payload, err := readUpstreamBody(resp)
	if err != nil {
		http.Error(w, "Upstream unreadable", http.StatusBadGateway)
		return
	}
	ct := resp.Header.Get("Content-Type")
	rawLoc := resp.Header.Get("Location")
	loc := rawLoc
	if loc != "" {
		// Never rewrite ubersuggest.com → public origin for Location: that
		// recreates the exact URL the client already requested (301 loop).
		if isUbersuggestBounce(loc) {
			loc = ""
		} else {
			loc = rewriteLocation(loc, base.Host, origin)
		}
	}
	// Dead Ranky PHPSESSID → 302 member.rankytools.com?return_url=...
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
	isHTML := strings.Contains(strings.ToLower(ct), "text/html")
	// Never run logout heuristics on JS/CSS — Ubersuggest bundles mention
	// "sign in" / "session" and would false-positive into Updating tools.
	if isHTML && bodyLooksLoggedOut(payload) {
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
	if isDocumentNavigation(r) && isHTML {
		// No Change Account chip (Semrush-only). Nav helper goes before </body>
		// only — head/root injection breaks Next hydration (React #412 Oops).
		payload = injectNavFixHTML(payload)
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
			// https://127.0.0.1 and Domain Overview never leaves the spinner.
			if httpOrigin {
				continue
			}
		case "access-control-allow-origin":
			// Ranky returns ACAO=https://us.rankytools.com; browser on 127.0.0.1
			// would block XHR/fetch without matching origin.
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
				if isUbersuggestBounce(v) {
					// Drop — rewritten self-301 loops the browser on menu RSC fetches.
					continue
				}
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
	for _, part := range parts[1:] {
		p := strings.TrimSpace(part)
		if len(p) >= 7 && strings.EqualFold(p[:7], "domain=") {
			// Browsers reject Domain=<IP>; drop Domain so the cookie is host-only.
			if hostIsIP(host) {
				continue
			}
			out = append(out, "Domain="+host)
			continue
		}
		out = append(out, part)
	}
	_ = upstreamHost
	return strings.Join(out, ";"), true
}
