package main

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	_ "embed"
	"encoding/json"
	"fmt"
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

//go:embed proxy_inject_fallback.js
var proxyInjectFallback []byte

var (
	upstreamClient = &http.Client{
		Timeout: 45 * time.Second,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
			MaxIdleConns:          128,
			MaxIdleConnsPerHost:   32,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   12 * time.Second,
			ResponseHeaderTimeout: 25 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	// Short client for Ranky's secondary injector — must never hang the ChatGPT shell.
	injectClient = &http.Client{
		Timeout: 8 * time.Second,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
			MaxIdleConns:          16,
			MaxIdleConnsPerHost:   8,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   6 * time.Second,
			ResponseHeaderTimeout: 6 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	// RankyTools ChatGPT gateway (+ sibling os host used in chrome).
	rankyHost      = regexp.MustCompile(`(?i)https?://(?:[a-z0-9-]+\.)*rankytools\.com`)
	rtCookieGuard  = regexp.MustCompile(`(?is)<script[^>]*id=["']rt-cookie-guard["'][^>]*>.*?</script>`)
	rankyBrandBoot = regexp.MustCompile(`(?is)if\(!document\.querySelector\(['"]\.ranky-brand['"]\)\)\{.*?document\.body\.appendChild\(b\);\}`)
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
	// Ranky cookie-guard deletes non-amember cookies (incl. PHPSESSID / ranky.sid).
	s = rtCookieGuard.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "https://"+upstreamHost, origin)
	s = strings.ReplaceAll(s, "http://"+upstreamHost, origin)
	s = strings.ReplaceAll(s, "//"+upstreamHost, "//"+hostOnly)
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
	s = rewriteRankyOSCopy(s)
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

// rewriteRankyOSCopy strips Ranky branding crumbs from gateway chrome/copy.
func rewriteRankyOSCopy(s string) string {
	replacements := [][2]string{
		{` at RankyOS`, ``},
		{` at RankyOS.`, `.`},
		// Sidebar footer website label under the display name.
		{`<span class="ranky-user-sub">RankyTools</span>`, ``},
		{`<span class='ranky-user-sub'>RankyTools</span>`, ``},
	}
	for _, pair := range replacements {
		s = strings.ReplaceAll(s, pair[0], pair[1])
	}
	// Corner "ChatGPT by RankyTools" badge from __proxy_inject.js — not needed.
	s = rankyBrandBoot.ReplaceAllString(s, "")
	return s
}

// injectViewerChrome hides the RankyTools subtitle and, in panel mode, locks
// #ranky-user-name to the live panel username (same pattern as other recloud tools).
// Local mode leaves the upstream display name alone for now.
func injectViewerChrome(body []byte, panelUser string) []byte {
	if bytes.Contains(body, []byte("data-tm-viewer")) {
		return body
	}
	userJS := "null"
	if u := strings.TrimSpace(panelUser); u != "" {
		b, _ := json.Marshal(u)
		userJS = string(b)
	}
	script := []byte(`<style data-tm-viewer>.ranky-user-sub,.ranky-brand{display:none!important}</style><script data-tm-viewer>
(function(){
  var viewer=` + userJS + `;
  if(!viewer)return;
  function paint(){
    var el=document.getElementById("ranky-user-name");
    if(el&&el.textContent!==viewer)el.textContent=viewer;
    var av=document.getElementById("ranky-user-avatar");
    if(av)av.textContent=(viewer.charAt(0)||"U").toUpperCase();
    try{
      window.__RANKY_USER=window.__RANKY_USER||{};
      window.__RANKY_USER.name=viewer;
      window.__RANKY_USER.username=viewer;
    }catch(e){}
  }
  paint();
  // No setInterval — ChatGPT SPA + full-tree observer already paints on DOM changes.
  try{
    var root=document.getElementById("ranky-user-name")||document.body||document.documentElement;
    new MutationObserver(paint).observe(root,{childList:true,subtree:true,characterData:true});
  }catch(e){}
})();
</script>`)
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

// mergeSessionCookies keeps PHPSESSID from the account jar and overlays
// Ranky ChatGPT gateway cookies the browser received (ranky.sid / connect.sid).
func mergeSessionCookies(accountCookie, browserCookie string) string {
	type pair struct{ name, value string }
	order := make([]string, 0, 8)
	vals := map[string]string{}
	add := func(raw string, allow func(string) bool) {
		for _, part := range strings.Split(raw, ";") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			name, value, ok := strings.Cut(part, "=")
			name = strings.TrimSpace(name)
			if !ok || name == "" {
				continue
			}
			if allow != nil && !allow(name) {
				continue
			}
			if _, exists := vals[name]; !exists {
				order = append(order, name)
			}
			vals[name] = value
		}
	}
	add(accountCookie, nil)
	add(browserCookie, func(n string) bool {
		ln := strings.ToLower(n)
		return ln == "ranky.sid" || ln == "connect.sid"
	})
	out := make([]string, 0, len(order))
	for _, n := range order {
		out = append(out, n+"="+vals[n])
	}
	return strings.Join(out, "; ")
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
	if strings.HasPrefix(p, "/__static__/") || strings.HasPrefix(p, "/cdn/") || strings.HasPrefix(p, "/__sandbox/") {
		return true
	}
	for _, ext := range []string{".js", ".css", ".woff2", ".woff", ".ttf", ".eot", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

func isProxyInjectPath(path string) bool {
	p := path
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	return strings.EqualFold(p, "/__proxy_inject.js")
}

// patchProxyInject rewrites Ranky's secondary injector onto our public origin.
// ChatGPT shell hangs if this file never arrives or still points at cp.rankytools.com.
func patchProxyInject(body []byte, upstreamHost, origin string) []byte {
	if len(body) == 0 {
		return body
	}
	out := rewriteText(body, upstreamHost, origin)
	s := string(out)
	s = strings.ReplaceAll(s, "__TM_ORIGIN__", origin)
	// Defense: force base= onto the live origin even if Ranky changes host wording.
	if i := strings.Index(s, "var base = "); i >= 0 {
		if j := strings.Index(s[i:], ";"); j > 0 {
			s = s[:i] + `var base = "` + origin + `"` + s[i+j:]
		}
	}
	return []byte(s)
}

func fallbackProxyInject(origin string) []byte {
	s := string(proxyInjectFallback)
	s = strings.ReplaceAll(s, "__TM_ORIGIN__", origin)
	return []byte(s)
}

// serveProxyInjectJS never blocks the ChatGPT shell on a slow Ranky asset.
// Cache key ignores ?v= so every Open does not re-fetch upstream.
func serveProxyInjectJS(w http.ResponseWriter, r *http.Request, acc ToolAccount, base *url.URL, origin string, started time.Time) {
	const cdnKey = "/__proxy_inject.js"
	path := r.URL.RequestURI()
	if cdncache.Serve(w, r, cdnKey) {
		log.Printf("[REQ] ← %s %s 200 cache inject", r.Method, path)
		pushProxyLog(proxyLogEntry{
			Source: "cdn", Method: r.Method, Path: path, Status: 200,
			Ms: time.Since(started).Milliseconds(), Detail: "inject cache",
		})
		return
	}
	defer cdncache.Complete(cdnKey)

	payload := fallbackProxyInject(origin)
	src := "fallback"
	upURL := strings.TrimRight(base.String(), "/") + "/__proxy_inject.js"
	upReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, upURL, nil)
	if err == nil {
		upReq.Host = base.Host
		upReq.Header.Set("Host", base.Host)
		upReq.Header.Set("User-Agent", "Mozilla/5.0")
		upReq.Header.Set("Accept", "*/*")
		upReq.Header.Set("Accept-Encoding", "gzip, deflate, br")
		upReq.Header.Set("Referer", base.Scheme+"://"+base.Host+"/")
		if cookieHdr := mergeSessionCookies(acc.Cookie, r.Header.Get("Cookie")); cookieHdr != "" {
			upReq.Header.Set("Cookie", cookieHdr)
		}
		resp, err := injectClient.Do(upReq)
		if err == nil {
			body, readErr := readUpstreamBody(resp)
			_ = resp.Body.Close()
			if readErr == nil && resp.StatusCode == http.StatusOK && len(body) > 200 &&
				bytes.Contains(body, []byte("var base")) {
				payload = patchProxyInject(body, base.Host, origin)
				src = "upstream"
			} else {
				log.Printf("[inject] upstream unusable status=%d err=%v bytes=%d — using fallback",
					resp.StatusCode, readErr, len(body))
			}
		} else {
			log.Printf("[inject] upstream fetch failed: %v — using fallback", err)
		}
	}

	cdncache.Store(cdnKey, http.StatusOK, "application/javascript; charset=utf-8", payload)
	if cdncache.ServeDisk(w, r, cdnKey) {
		ms := time.Since(started).Milliseconds()
		log.Printf("[REQ] ← %s %s 200 %dms bytes=%d inject=%s", r.Method, path, ms, len(payload), src)
		pushProxyLog(proxyLogEntry{
			Source: "inject", Method: r.Method, Path: path, Status: 200,
			Ms: ms, Detail: fmt.Sprintf("%s bytes=%d", src, len(payload)),
		})
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/javascript; charset=utf-8")
	h.Set("Cache-Control", "public, max-age=86400")
	h.Set("X-TM-Inject", src)
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(payload)
	}
	ms := time.Since(started).Milliseconds()
	log.Printf("[REQ] ← %s %s 200 %dms bytes=%d inject=%s", r.Method, path, ms, len(payload), src)
	pushProxyLog(proxyLogEntry{
		Source: "inject", Method: r.Method, Path: path, Status: 200,
		Ms: ms, Detail: fmt.Sprintf("%s bytes=%d", src, len(payload)),
	})
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
	started := time.Now()
	path := r.URL.RequestURI()
	// Always print immediately so `tail -f app.log` shows the hang point.
	log.Printf("[REQ] → %s %s", r.Method, path)
	base, err := url.Parse(liveUpstream(cfg))
	if err != nil || base.Host == "" {
		http.Error(w, "Upstream URL is not configured", http.StatusBadGateway)
		return
	}
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		log.Printf("[WS] → upgrade %s", path)
		proxyWebSocket(w, r, acc, base)
		pushProxyLog(proxyLogEntry{
			Source: "ws", Method: r.Method, Path: path, Status: 101,
			Ms: time.Since(started).Milliseconds(), Detail: "websocket closed",
		})
		return
	}
	origin := publicOrigin(r, cfg)
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		if isProxyInjectPath(r.URL.Path) {
			serveProxyInjectJS(w, r, acc, base, origin, started)
			return
		}
	}
	target := *base
	target.Path = r.URL.Path
	target.RawPath = r.URL.RawPath
	target.RawQuery = r.URL.RawQuery
	cdnKey := cdncache.RequestKey(r, isCacheableStaticPath(r.URL.Path))
	if cdncache.Serve(w, r, cdnKey) {
		log.Printf("[REQ] ← %s %s 200 cache", r.Method, path)
		pushProxyLog(proxyLogEntry{
			Source: "cdn", Method: r.Method, Path: path, Status: 200,
			Ms: time.Since(started).Milliseconds(), Detail: "cdn cache",
		})
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
	cookieHdr := mergeSessionCookies(acc.Cookie, r.Header.Get("Cookie"))
	if cookieHdr != "" {
		upReq.Header.Set("Cookie", cookieHdr)
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
		pushProxyLog(proxyLogEntry{
			Source: "up", Method: r.Method, Path: r.URL.RequestURI(), Status: 502,
			Ms: time.Since(started).Milliseconds(), Detail: err.Error(),
		})
		http.Error(w, "Upstream unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	payload, err := readUpstreamBody(resp)
	if err != nil {
		pushProxyLog(proxyLogEntry{
			Source: "up", Method: r.Method, Path: r.URL.RequestURI(), Status: 502,
			Ms: time.Since(started).Milliseconds(), Detail: "body read failed",
		})
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
	if isDocumentNavigation(r) && strings.Contains(strings.ToLower(ct), "text/html") {
		viewer := ""
		if usesPanelAccountMode(cfg) {
			if name, err := panelSessionUsername(r); err == nil {
				viewer = name
			}
			payload = injectDeviceHTML(payload)
		}
		// Always hide RankyTools subtitle; panel mode also locks username.
		payload = injectViewerChrome(payload, viewer)
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
	ms := time.Since(started).Milliseconds()
	detail := ct
	if len(payload) > 0 {
		detail = fmt.Sprintf("%s bytes=%d", ct, len(payload))
	}
	log.Printf("[REQ] ← %s %s %d %dms bytes=%d", r.Method, path, resp.StatusCode, ms, len(payload))
	// Keep ring buffer full for /__logs, but terminal is the live source of truth.
	pushProxyLog(proxyLogEntry{
		Source: "up", Method: r.Method, Path: path, Status: resp.StatusCode,
		Ms: ms, Detail: detail,
	})
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
