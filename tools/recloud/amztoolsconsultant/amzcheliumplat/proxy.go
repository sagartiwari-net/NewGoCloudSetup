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
	consultantHost = regexp.MustCompile(`(?i)(?:[a-z0-9-]+\.)*amztoolsconsultant\.com`)
	semrushHost    = regexp.MustCompile(`(?i)https?://(?:[a-z0-9-]+\.)*semrush\.com`)
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
	s = strings.ReplaceAll(s, "https://"+upstreamHost, origin)
	s = strings.ReplaceAll(s, "http://"+upstreamHost, origin)
	s = strings.ReplaceAll(s, "//"+upstreamHost, "//"+hostOnly)
	s = strings.ReplaceAll(s, "https://localhost", origin)
	s = strings.ReplaceAll(s, "http://localhost", origin)
	s = consultantHost.ReplaceAllString(s, hostOnly)
	s = semrushHost.ReplaceAllString(s, origin)
	s = strings.ReplaceAll(s, ".semrush.com", "."+cookieDomain)
	return []byte(s)
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
	if strings.HasPrefix(p, "/__static__/") || strings.HasPrefix(p, "/static-semrush-com/") {
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

const loginAttempts = 3

var loginRetryWait = time.Second

func toolRetryPath(r *http.Request, cfg Config) string {
	if !isLoginPath(r.URL.Path) {
		return r.URL.RequestURI()
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("url")); raw != "" {
		if u, err := url.Parse(raw); err == nil && u.Host == "" && strings.HasPrefix(u.Path, "/") && u.Path != "/" && !isLoginPath(u.Path) {
			return u.RequestURI()
		}
	}
	if home := strings.TrimSpace(cfg.HomePath); home != "" {
		return home
	}
	return "/"
}

func locationIsLogin(loc string) bool {
	loc = strings.TrimSpace(loc)
	if loc == "" {
		return false
	}
	u, err := url.Parse(loc)
	if err != nil {
		return strings.Contains(strings.ToLower(loc), "/login")
	}
	return isLoginPath(u.Path)
}

func responseLooksLoggedOut(status int, location string, body []byte) bool {
	if status >= 300 && status < 400 && locationIsLogin(location) {
		return true
	}
	return bodyLooksLoggedOut(body)
}

func upstreamURL(base *url.URL, requestURI string) string {
	if abs, err := url.Parse(requestURI); err == nil && abs.Host != "" && (abs.Scheme == "http" || abs.Scheme == "https") {
		return abs.String()
	}
	u := *base
	rel, err := url.Parse(requestURI)
	if err != nil || rel.Host != "" || !strings.HasPrefix(rel.Path, "/") {
		u.Path = "/"
		u.RawQuery = ""
		return u.String()
	}
	u.Path = rel.Path
	u.RawQuery = rel.RawQuery
	return u.String()
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
	cdnKey := cdncache.RequestKey(r, isCacheableStaticPath(r.URL.Path))
	if cdncache.Serve(w, r, cdnKey) {
		return
	}
	if cdnKey != "" {
		defer cdncache.Complete(cdnKey)
	}

	var bodyBytes []byte
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		raw, err := io.ReadAll(io.LimitReader(r.Body, 20<<20))
		if err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		if shouldRewriteBody(r.Header.Get("Content-Type")) {
			raw = rewriteOutbound(raw, base.Scheme+"://"+base.Host, origin)
		}
		bodyBytes = raw
	}

	attempts := 1
	if r.Method == http.MethodGet && isDocumentNavigation(r) {
		attempts = loginAttempts
	}
	requestURI := r.URL.RequestURI()
	if isLoginPath(r.URL.Path) {
		requestURI = toolRetryPath(r, cfg)
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			time.Sleep(loginRetryWait)
			requestURI = toolRetryPath(r, cfg)
			if usesPanelAccountMode(cfg) {
				if fresh, err := loadPanelSessionAccount(cfg, sessionTokenFromRequest(r)); err == nil {
					acc = fresh
				}
			}
		}
		jar := cookiesForUpstream(acc, r)
		var resp *http.Response
		var payload []byte
		var ct string
		hops := 1
		if followSessionRedirects(r, cfg) {
			hops = maxCookieHops
		}
		bare := false
		originalURI := requestURI
		for hop := 0; hop < hops; hop++ {
			var body io.Reader
			if bodyBytes != nil && hop == 0 {
				body = bytes.NewReader(bodyBytes)
			}
			method := r.Method
			if hop > 0 {
				method = http.MethodGet
			}
			target := upstreamURL(base, requestURI)
			upReq, err := http.NewRequestWithContext(r.Context(), method, target, body)
			if err != nil {
				http.Error(w, "Bad gateway", http.StatusBadGateway)
				return
			}
			hopHost := base.Host
			hopScheme := base.Scheme
			if parsed, err := url.Parse(target); err == nil && parsed.Host != "" {
				hopHost = parsed.Host
				if parsed.Scheme != "" {
					hopScheme = parsed.Scheme
				}
			}
			if bare {
				setBareUpstreamHeaders(upReq, r)
			} else {
				copyRequestHeaders(upReq, r)
			}
			upReq.Host = hopHost
			upReq.Header.Set("Host", hopHost)
			if !bare {
				if r.Header.Get("Origin") != "" {
					upReq.Header.Set("Origin", hopScheme+"://"+hopHost)
				} else {
					upReq.Header.Del("Origin")
				}
				if ref := r.Referer(); ref != "" {
					upReq.Header.Set("Referer", string(rewriteOutbound([]byte(ref), base.Scheme+"://"+base.Host, origin)))
				} else {
					upReq.Header.Del("Referer")
				}
			}
			if jar != "" {
				upReq.Header.Set("Cookie", jar)
			} else {
				upReq.Header.Del("Cookie")
			}
			upReq.Header.Set("Accept-Encoding", "gzip, deflate, br")
			if upReq.Header.Get("User-Agent") == "" {
				upReq.Header.Set("User-Agent", "Mozilla/5.0")
			}
			resp, err = upstreamClient.Do(upReq)
			if err != nil {
				lastErr = err
				log.Printf("[UP] %s %s try %d/%d: %v", method, requestURI, attempt, attempts, err)
				resp = nil
				break
			}
			payload, err = readUpstreamBody(resp)
			resp.Body.Close()
			if err != nil {
				lastErr = err
				log.Printf("[UP] %s %s try %d/%d unreadable: %v", method, requestURI, attempt, attempts, err)
				resp = nil
				break
			}
			jar = absorbSetCookies(jar, resp.Header)
			if plainAccessDenied(resp.StatusCode, payload) && !bare && r.Method == http.MethodGet && isDocumentNavigation(r) {
				bare = true
				requestURI = originalURI
				jar = cookiesForUpstream(acc, r)
				hop = -1
				continue
			}
			rememberUpstreamCookies(acc, jar)
			next, ok := sameSiteRequestURI(resp.Header.Get("Location"), base.Host, origin)
			if ok && resp.StatusCode >= 300 && resp.StatusCode < 400 && hop+1 < hops {
				log.Printf("[UP] session cookie hop %d -> %s", hop+1, next)
				requestURI = next
				continue
			}
			break
		}
		if resp == nil {
			continue
		}
		ct = resp.Header.Get("Content-Type")
		if shouldRewriteBody(ct) && len(payload) > 0 && len(payload) <= 8<<20 {
			payload = rewriteText(payload, base.Host, origin)
			if strings.Contains(strings.ToLower(ct), "text/html") {
				payload = rewriteH10ChromeStoreURLs(payload, origin)
			}
		}
		loggedOut := attempts > 1 && responseLooksLoggedOut(resp.StatusCode, resp.Header.Get("Location"), payload)
		if loggedOut && attempt < attempts {
			log.Printf("[UP] session not in yet, try %d/%d", attempt, attempts)
			continue
		}
		if loggedOut {
			token := sessionTokenFromRequest(r)
			switched, _ := noteLogout(cfg, r, acc, token, "logout")
			if switched && cfg.HomePath != "" {
				http.Redirect(w, r, cfg.HomePath, http.StatusFound)
				return
			}
			renderLoggedOutPage(w, cfg)
			return
		}
		if isLoginPath(r.URL.Path) && resp.StatusCode < 400 {
			http.Redirect(w, r, requestURI, http.StatusFound)
			return
		}
		if isDocumentNavigation(r) && strings.Contains(strings.ToLower(ct), "text/html") {
			viewer := ""
			if name, err := panelSessionUsername(r); err == nil {
				viewer = name
			}
			payload = injectDeviceHTML(payload, viewer)
			payload = injectIntoHTMLHead(payload, h10ExtInstallGuardScript())
		}
		if cdnKey != "" && resp.StatusCode == http.StatusOK {
			cdncache.Store(cdnKey, resp.StatusCode, ct, payload)
			if cdncache.ServeDisk(w, r, cdnKey) {
				return
			}
		}
		h := w.Header()
		copyResponseHeaders(h, resp.Header, base.Host, origin)
		h.Set("Cache-Control", "no-store")
		h.Set("Pragma", "no-cache")
		h.Del("Content-Encoding")
		h.Del("Content-Length")
		w.WriteHeader(resp.StatusCode)
		if r.Method != http.MethodHead {
			_, _ = w.Write(payload)
		}
		return
	}
	log.Printf("[UP] %s %s: %v", r.Method, r.URL.Path, lastErr)
	http.Error(w, "Upstream unavailable", http.StatusBadGateway)
}

func followSessionRedirects(r *http.Request, cfg Config) bool {
	if r.Method != http.MethodGet || !isDocumentNavigation(r) {
		return false
	}
	path := r.URL.Path
	if path == "/" || path == "" || isLoginPath(path) {
		return true
	}
	home := cfg.HomePath
	if i := strings.Index(home, "?"); i >= 0 {
		home = home[:i]
	}
	return home != "" && home != "/" && path == home
}

func plainAccessDenied(status int, body []byte) bool {
	return status == http.StatusForbidden && strings.TrimSpace(string(body)) == "Access Denied"
}

func setBareUpstreamHeaders(dst *http.Request, src *http.Request) {
	if accept := src.Header.Get("Accept"); accept != "" {
		dst.Header.Set("Accept", accept)
	}
	if lang := src.Header.Get("Accept-Language"); lang != "" {
		dst.Header.Set("Accept-Language", lang)
	}
	if ua := src.Header.Get("User-Agent"); ua != "" {
		dst.Header.Set("User-Agent", ua)
	}
}

func copyRequestHeaders(dst *http.Request, src *http.Request) {
	for key, values := range src.Header {
		lk := strings.ToLower(key)
		switch lk {
		case "cookie", "host", "content-length", "accept-encoding", "connection", "upgrade", "proxy-connection", "x-device-fp", "x-device-proof",
			"cf-connecting-ip", "cf-ray", "cf-ipcountry", "cf-visitor", "cf-worker", "cdn-loop", "true-client-ip", "x-forwarded-host", "x-forwarded-server", "forwarded":
			continue
		}
		for _, v := range values {
			dst.Header.Add(key, v)
		}
	}
}

func copyResponseHeaders(dst, src http.Header, upstreamHost, origin string) {
	for key, values := range src {
		lk := strings.ToLower(key)
		switch lk {
		case "content-length", "content-encoding", "transfer-encoding", "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "trailer", "upgrade":
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
	case "phpsessid", "ct_session":
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
			out = append(out, "Domain="+host)
			continue
		}
		out = append(out, part)
	}
	_ = upstreamHost
	return strings.Join(out, ";"), true
}
