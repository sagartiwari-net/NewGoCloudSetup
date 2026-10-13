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
	semrushHost = regexp.MustCompile(`(?i)https?://(?:[a-z0-9-]+\.)*semrush\.com`)
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

func setCachePolicy(h http.Header, path, contentType string) {
	if isCacheableStaticPath(path) {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
		h.Del("Pragma")
		h.Del("Expires")
		h.Set("Vary", "Accept-Encoding")
		return
	}
	h.Set("Cache-Control", "no-store")
	h.Set("Pragma", "no-cache")
	_ = contentType
}

func compressStatic(r *http.Request, body []byte) ([]byte, string) {
	if len(body) < 256 || !isCacheableStaticPath(r.URL.Path) {
		return body, ""
	}
	ae := strings.ToLower(r.Header.Get("Accept-Encoding"))
	if strings.Contains(ae, "br") {
		var buf bytes.Buffer
		w := brotli.NewWriterLevel(&buf, 5)
		if _, err := w.Write(body); err == nil && w.Close() == nil && buf.Len() > 0 && buf.Len() < len(body) {
			return buf.Bytes(), "br"
		}
	}
	if strings.Contains(ae, "gzip") {
		var buf bytes.Buffer
		w, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
		if err == nil {
			_, werr := w.Write(body)
			cerr := w.Close()
			if werr == nil && cerr == nil && buf.Len() > 0 && buf.Len() < len(body) {
				return buf.Bytes(), "gzip"
			}
		}
	}
	return body, ""
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

	if cached := getStaticCached(r.URL.RequestURI()); cached != nil && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		writeCached(w, r, cached)
		return
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
	defer resp.Body.Close()

	payload, err := readUpstreamBody(resp)
	if err != nil {
		http.Error(w, "Upstream unreadable", http.StatusBadGateway)
		return
	}
	ct := resp.Header.Get("Content-Type")
	if shouldRewriteBody(ct) && len(payload) > 0 && len(payload) <= 8<<20 {
		payload = rewriteText(payload, base.Host, origin)
	}
	if isDocumentNavigation(r) && strings.Contains(strings.ToLower(ct), "text/html") {
		if bodyLooksLoggedOut(payload) {
			token := sessionTokenFromRequest(r)
			switched, _ := noteLogout(cfg, r, acc, token, "session_expired")
			if switched {
				http.Redirect(w, r, cfg.HomePath, http.StatusFound)
				return
			}
		}
		payload = injectDeviceHTML(payload)
	}

	if isCacheableStaticPath(r.URL.RequestURI()) && resp.StatusCode == http.StatusOK && r.Method == http.MethodGet {
		putStaticCached(r.URL.RequestURI(), resp.StatusCode, ct, payload)
	}

	h := w.Header()
	copyResponseHeaders(h, resp.Header, base.Host, origin)
	setCachePolicy(h, r.URL.Path, ct)
	out, encoding := compressStatic(r, payload)
	if encoding != "" {
		h.Set("Content-Encoding", encoding)
	} else {
		h.Del("Content-Encoding")
	}
	h.Del("Content-Length")
	w.WriteHeader(resp.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = w.Write(out)
	}
}

func writeCached(w http.ResponseWriter, r *http.Request, ent *staticCacheEntry) {
	h := w.Header()
	if ent.contentType != "" {
		h.Set("Content-Type", ent.contentType)
	}
	setCachePolicy(h, r.URL.Path, ent.contentType)
	out, encoding := compressStatic(r, ent.body)
	if encoding != "" {
		h.Set("Content-Encoding", encoding)
	}
	h.Set("X-OCG-Cache", "HIT")
	w.WriteHeader(ent.status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(out)
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
			out = append(out, "Domain="+host)
			continue
		}
		out = append(out, part)
	}
	_ = upstreamHost
	return strings.Join(out, ";"), true
}
