package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
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
	markhorHost    = regexp.MustCompile(`(?i)(?:[a-z0-9-]+\.)?accesstool\.in`)
	semrushHost    = regexp.MustCompile(`(?i)https?://(?:[a-z0-9-]+\.)*semrush\.com`)
	semrushRelHost = regexp.MustCompile(`(?i)//(?:[a-z0-9-]+\.)*semrush\.com`)
	semrushHopURL  = regexp.MustCompile(`(?i)https?://[^/\s"']+/semrush-host/([a-z0-9,-]+)`)
)

const semrushHopPrefix = "/semrush-host/"

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
	if !strings.HasSuffix(strings.ToLower(upstreamHost), ".semrush.com") {
		s = strings.ReplaceAll(s, "https://"+upstreamHost, origin)
		s = strings.ReplaceAll(s, "http://"+upstreamHost, origin)
		s = strings.ReplaceAll(s, "//"+upstreamHost, "//"+hostOnly)
	}
	s = strings.ReplaceAll(s, "https://localhost", origin)
	s = strings.ReplaceAll(s, "http://localhost", origin)
	s = markhorHost.ReplaceAllString(s, hostOnly)
	s = semrushHost.ReplaceAllStringFunc(s, func(match string) string {
		return semrushHop(origin, match)
	})
	s = semrushRelHost.ReplaceAllStringFunc(s, func(match string) string {
		return semrushHop("//"+hostOnly, match)
	})
	s = strings.ReplaceAll(s, ".semrush.com", "."+cookieDomain)
	s = strings.ReplaceAll(s, "semnew."+cookieDomain, cookieDomain)
	return []byte(s)
}

func semrushHop(origin, match string) string {
	raw := match
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || !strings.HasSuffix(strings.ToLower(u.Hostname()), ".semrush.com") {
		return origin
	}
	return strings.TrimRight(origin, "/") + semrushHopPrefix + strings.ReplaceAll(u.Hostname(), ".", ",")
}

func semrushHopTarget(path string) (host, rest string, ok bool) {
	if !strings.HasPrefix(path, semrushHopPrefix) {
		return "", "", false
	}
	tail := strings.TrimPrefix(path, semrushHopPrefix)
	enc, pathRest := tail, "/"
	if i := strings.Index(tail, "/"); i >= 0 {
		enc = tail[:i]
		pathRest = tail[i:]
	}
	host = strings.ReplaceAll(enc, ",", ".")
	if enc == "" || strings.Contains(host, "..") || !strings.HasSuffix(strings.ToLower(host), ".semrush.com") {
		return "", "", false
	}
	if pathRest == "" {
		pathRest = "/"
	}
	return host, pathRest, true
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
	s := restoreSemrushHops(string(body))
	s = strings.ReplaceAll(s, public, upstreamOrigin)
	s = strings.ReplaceAll(s, host, upHost)
	return []byte(s)
}

func restoreSemrushHops(s string) string {
	return semrushHopURL.ReplaceAllStringFunc(s, func(match string) string {
		sub := semrushHopURL.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		host := strings.ReplaceAll(sub[1], ",", ".")
		if !strings.HasSuffix(strings.ToLower(host), ".semrush.com") {
			return match
		}
		scheme := "https"
		if strings.HasPrefix(strings.ToLower(match), "http://") {
			scheme = "http"
		}
		return scheme + "://" + host
	})
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
		var body io.Reader
		if bodyBytes != nil {
			body = bytes.NewReader(bodyBytes)
		}
		targetBase := base
		targetURI := requestURI
		if rel, err := url.Parse(requestURI); err == nil {
			if host, rest, ok := semrushHopTarget(rel.Path); ok {
				targetBase = &url.URL{Scheme: "https", Host: host}
				targetURI = rest
				if rel.RawQuery != "" {
					targetURI += "?" + rel.RawQuery
				}
			}
		}
		upReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL(targetBase, targetURI), body)
		if err != nil {
			http.Error(w, "Bad gateway", http.StatusBadGateway)
			return
		}
		copyRequestHeaders(upReq, r)
		upReq.Host = targetBase.Host
		upReq.Header.Set("Host", targetBase.Host)
		upReq.Header.Set("Origin", targetBase.Scheme+"://"+targetBase.Host)
		if targetBase.Host != base.Host {
			upReq.Header.Set("Referer", targetBase.Scheme+"://"+targetBase.Host+"/")
		} else if ref := r.Referer(); ref != "" {
			upReq.Header.Set("Referer", string(rewriteOutbound([]byte(ref), base.Scheme+"://"+base.Host, origin)))
		} else {
			upReq.Header.Set("Referer", base.Scheme+"://"+base.Host+"/")
		}
		if targetBase.Host != base.Host {
			upReq.Header.Del("Cookie")
		} else if cookie := cookiesForUpstream(acc.Cookie, r.Header.Get("Cookie")); cookie != "" {
			upReq.Header.Set("Cookie", cookie)
		} else {
			upReq.Header.Del("Cookie")
		}
		upReq.Header.Set("Accept-Encoding", "gzip, deflate, br")
		if upReq.Header.Get("User-Agent") == "" {
			upReq.Header.Set("User-Agent", "Mozilla/5.0")
		}

		resp, err := upstreamClient.Do(upReq)
		if err != nil {
			lastErr = err
			log.Printf("[UP] %s %s try %d/%d: %v", r.Method, requestURI, attempt, attempts, err)
			continue
		}
		payload, err := readUpstreamBody(resp)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			log.Printf("[UP] %s %s try %d/%d unreadable: %v", r.Method, requestURI, attempt, attempts, err)
			continue
		}
		ct := resp.Header.Get("Content-Type")
		statusCode := resp.StatusCode
		respHeader := resp.Header.Clone()
		if shouldRewriteBody(ct) && len(payload) > 0 && len(payload) <= 8<<20 {
			payload = rewriteText(payload, targetBase.Host, origin)
		}
		if isDocumentNavigation(r) && bodyIsDashboardGate(payload) {
			noteCookieExpired(cfg, r, acc, sessionTokenFromRequest(r))
			renderLoggedOutPage(w, cfg)
			return
		}
		if isDocumentNavigation(r) && !isLoginPath(r.URL.Path) && targetBase.Host == base.Host && semrushServerDown(payload) {
			if next, trial, trialHeader, ok := firstWorkingSemrushAccount(r.Context(), cfg, base, requestURI, acc.ID); ok {
				log.Printf("[LB] semrush server %s is up, leaving %s", next.Name, acc.Name)
				assignSessionAccount(cfg, sessionTokenFromRequest(r), next)
				acc = next
				payload = rewriteText(trial, base.Host, origin)
				ct = "text/html"
				statusCode = http.StatusOK
				respHeader = trialHeader
			}
		}
		if attempts > 1 && responseLooksLoggedOut(resp.StatusCode, resp.Header.Get("Location"), payload) {
			if attempt < attempts {
				log.Printf("[UP] session not in yet, try %d/%d", attempt, attempts)
				continue
			}
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
			payload = injectDeviceHTML(payload)
		}
		if cdnKey != "" && resp.StatusCode == http.StatusOK {
			cdncache.Store(cdnKey, resp.StatusCode, ct, payload)
			if cdncache.ServeDisk(w, r, cdnKey) {
				return
			}
		}
		h := w.Header()
		copyResponseHeaders(h, respHeader, base.Host, origin)
		h.Set("Cache-Control", "no-store")
		h.Set("Pragma", "no-cache")
		h.Del("Content-Encoding")
		h.Del("Content-Length")
		w.WriteHeader(statusCode)
		if r.Method != http.MethodHead {
			_, _ = w.Write(payload)
		}
		return
	}
	log.Printf("[UP] %s %s: %v", r.Method, r.URL.Path, lastErr)
	http.Error(w, "Upstream unavailable", http.StatusBadGateway)
}

func firstWorkingSemrushAccount(ctx context.Context, cfg Config, base *url.URL, requestURI string, except int) (ToolAccount, []byte, http.Header, bool) {
	for _, other := range storedSemrushAccounts(cfg, except) {
		status, hdr, body, err := probeSemrushAccount(ctx, base, requestURI, other)
		if err != nil || bodyIsDashboardGate(body) || semrushServerDown(body) || status >= 400 {
			continue
		}
		if hdr == nil {
			hdr = make(http.Header)
		}
		return other, body, hdr, true
	}
	return ToolAccount{}, nil, nil, false
}

func probeSemrushAccount(ctx context.Context, base *url.URL, requestURI string, acc ToolAccount) (int, http.Header, []byte, error) {
	gate := upstreamSessionCookie(acc.Cookie)
	if gate == "" || base == nil {
		return 0, nil, nil, fmt.Errorf("no gate cookie")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstreamURL(base, requestURI), nil)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Host = base.Host
	req.Header.Set("Host", base.Host)
	req.Header.Set("Origin", base.Scheme+"://"+base.Host)
	req.Header.Set("Referer", base.Scheme+"://"+base.Host+"/")
	req.Header.Set("Cookie", gate)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	resp, err := upstreamClient.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	body, err := readUpstreamBody(resp)
	if err != nil {
		return resp.StatusCode, nil, nil, err
	}
	return resp.StatusCode, resp.Header.Clone(), body, nil
}

func copyRequestHeaders(dst *http.Request, src *http.Request) {
	for key, values := range src.Header {
		lk := strings.ToLower(key)
		switch lk {
		case "cookie", "host", "content-length", "accept-encoding", "connection", "upgrade", "proxy-connection", "x-device-fp", "x-device-proof",
			"cf-connecting-ip", "cf-ray", "cf-ipcountry", "cf-visitor", "cf-worker", "cdn-loop", "true-client-ip",
			"x-forwarded-host", "x-forwarded-server", "x-forwarded-for", "x-real-ip", "forwarded":
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
		case "content-security-policy", "content-security-policy-report-only":
			for _, v := range values {
				dst.Add(key, string(rewriteText([]byte(v), upstreamHost, origin)))
			}
			continue
		}
		for _, v := range values {
			dst.Add(key, v)
		}
	}
}

func cookiesForUpstream(panelHeader, browserHeader string) string {
	var parts []string
	seen := map[string]bool{}
	if gate := upstreamSessionCookie(panelHeader); gate != "" {
		parts = append(parts, gate)
		seen["sr_gate"] = true
	}
	for _, part := range strings.Split(browserHeader, ";") {
		part = strings.TrimSpace(part)
		name, value, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		name = strings.TrimSpace(name)
		switch strings.ToLower(name) {
		case "ct_session", "sr_gate", "proxy_token":
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		parts = append(parts, name+"="+value)
	}
	return strings.Join(parts, "; ")
}

func upstreamSessionCookie(header string) string {
	var kept []string
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		name, value, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(name), "sr_gate") {
			kept = append(kept, strings.TrimSpace(name)+"="+value)
		}
	}
	return strings.Join(kept, "; ")
}

func rewriteSetCookie(raw, upstreamHost, origin string) (string, bool) {
	parts := strings.Split(raw, ";")
	if len(parts) == 0 {
		return "", false
	}
	name := strings.TrimSpace(strings.SplitN(parts[0], "=", 2)[0])
	switch strings.ToLower(name) {
	case "proxy_token", "sr_gate", "ct_session":
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
