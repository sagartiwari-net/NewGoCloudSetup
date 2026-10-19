package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	utls "github.com/refraction-networking/utls"
)

const configFile = "config.json"

type Config struct {
	Port          string `json:"port"`
	TargetURL     string `json:"target_url"`
	PublicHost    string `json:"public_host"`
	PublicScheme  string `json:"public_scheme"`
	UserAgent     string `json:"user_agent"`
	WebsiteID     int    `json:"website_id"`
	MySQLHost     string `json:"mysql_host"`
	MySQLPort     string `json:"mysql_port"`
	MySQLUser     string `json:"mysql_user"`
	MySQLPassword string `json:"mysql_password"`
	MySQLDB       string `json:"mysql_db"`
	PanelDB       string `json:"panel_db"`
	ToolName      string `json:"tool_name"`
	HomePath      string `json:"home_path"`
}

var (
	cfgMu          sync.RWMutex
	cfg            Config
	configModTime  time.Time
	jarMu          sync.Mutex
	cookieJar      = map[int]map[string]string{}
	upstreamOnce   sync.Once
	upstreamClient *http.Client
)

func main() {
	if err := reloadConfig(); err != nil {
		log.Fatalf("config: %v", err)
	}
	go func() {
		for {
			time.Sleep(2 * time.Second)
			_ = reloadConfig()
		}
	}()
	c := getConfig()
	log.Printf("Veo3 AI proxy → %s on :%s (home %s)", c.TargetURL, c.Port, homePath(c))
	if usesPanelAccountMode(c) {
		if _, err := openPanelDB(c); err != nil {
			log.Printf("WARNING: panel DB open failed: %v", err)
		} else {
			log.Printf("panel DB ready")
		}
	}
	addr := ":" + strings.TrimSpace(c.Port)
	if err := http.ListenAndServe(addr, http.HandlerFunc(proxyHandler)); err != nil {
		log.Fatal(err)
	}
}

func getConfig() Config {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return cfg
}

func loadConfig() Config {
	_ = reloadConfig()
	return getConfig()
}

func reloadConfig() error {
	st, err := os.Stat(configFile)
	if err != nil {
		return err
	}
	cfgMu.Lock()
	defer cfgMu.Unlock()
	if !configModTime.IsZero() && !st.ModTime().After(configModTime) {
		return nil
	}
	raw, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}
	var next Config
	if err := json.Unmarshal(raw, &next); err != nil {
		return err
	}
	if strings.TrimSpace(next.TargetURL) == "" {
		next.TargetURL = "https://flow.google.com"
	}
	if strings.TrimSpace(next.HomePath) == "" {
		next.HomePath = "/"
	}
	if strings.TrimSpace(next.ToolName) == "" {
		next.ToolName = "Veo3 AI"
	}
	if strings.TrimSpace(next.Port) == "" {
		next.Port = "5341"
	}
	if strings.TrimSpace(next.UserAgent) == "" {
		next.UserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"
	}
	cfg = next
	configModTime = st.ModTime()
	return nil
}

func homePath(c Config) string {
	p := strings.TrimSpace(c.HomePath)
	if p == "" {
		return "/image-to-video"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	c := getConfig()
	if veoBlocked(r.URL.Path) {
		writeLightCard(w, http.StatusForbidden, lightCard{
			Title:   "Not available",
			Heading: "Not available",
			Message: "This page is not available here.",
		})
		return
	}
	acc, req, handled := preparePanelRequest(w, r, c)
	if handled {
		return
	}
	r = req
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		home := homePath(c)
		if r.URL.Path == "/" && home != "/" {
			http.Redirect(w, r, home, http.StatusFound)
			return
		}
	}
	target, err := url.Parse(c.TargetURL)
	if err != nil || target.Host == "" {
		renderProxyProblem(w, r)
		return
	}
	reqSnap := snapVeoBody(r)
	upstream, err := http.NewRequestWithContext(r.Context(), r.Method, target.Scheme+"://"+target.Host+r.URL.RequestURI(), r.Body)
	if err != nil {
		renderProxyProblem(w, r)
		return
	}
	copyUpstreamHeaders(upstream, r)
	// Auth.js trusts forwarded host. Leave the official host so the saved session stays logged in.
	for _, h := range []string{"X-Forwarded-Host", "X-Forwarded-Proto", "X-Forwarded-For", "X-Real-Ip", "Forwarded", "Via", "Cf-Connecting-Ip", "Cf-Ray", "Cf-Visitor", "Cf-Ipcountry", "True-Client-Ip"} {
		upstream.Header.Del(h)
	}
	publicHost := panelRequestHost(r, c.PublicHost)
	rewriteUpstreamOrigin(upstream, publicHost, target.Host)
	ua := strings.TrimSpace(acc.UserAgent)
	if ua == "" {
		ua = c.UserAgent
	}
	upstream.Header.Set("User-Agent", ua)
	upstream.Header.Set("Accept-Encoding", "gzip")
	cookie := mergedAccountCookie(acc)
	if cookie != "" {
		upstream.Header.Set("Cookie", cookie)
	} else {
		upstream.Header.Del("Cookie")
	}
	rewriteGoogleAuth(upstream, cookie)
	upstream.Host = target.Host

	resp, err := chromeClient().Do(upstream)
	if err != nil {
		log.Printf("[PROXY] upstream: %v", err)
		renderProxyProblem(w, r)
		return
	}
	defer resp.Body.Close()
	rememberUpstreamCookies(acc.ID, resp.Header.Values("Set-Cookie"))

	publicBase := publicBaseURL(r, c, publicHost)
	if loc := resp.Header.Get("Location"); loc != "" {
		resp.Header.Set("Location", rewriteText(loc, publicBase, publicHost))
	}
	stripAuthSetCookies(resp)

	body, encoding, err := readRewriteBody(resp)
	if err != nil {
		log.Printf("[PROXY] body: %v", err)
		renderProxyProblem(w, r)
		return
	}
	logVeoUse(r.Method, r.URL.Path, resp.StatusCode, veoSessionUser(r), reqSnap, body)
	if body != nil {
		rewritten := []byte(rewriteText(string(body), publicBase, publicHost))
		ct := strings.ToLower(resp.Header.Get("Content-Type"))
		if strings.Contains(ct, "text/html") {
			rewritten = injectDeviceHTML(rewritten)
			resp.Header.Set("Cache-Control", "no-store")
		}
		if encoding == "gzip" {
			var buf bytes.Buffer
			zw := gzip.NewWriter(&buf)
			_, _ = zw.Write(rewritten)
			_ = zw.Close()
			rewritten = buf.Bytes()
			resp.Header.Set("Content-Encoding", "gzip")
		} else {
			resp.Header.Del("Content-Encoding")
		}
		resp.Header.Del("Content-Length")
		copyResponseHeaders(w, resp)
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(rewritten)
		return
	}
	copyResponseHeaders(w, resp)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func copyUpstreamHeaders(dst, src *http.Request) {
	hop := map[string]bool{
		"connection": true, "keep-alive": true, "proxy-authenticate": true,
		"proxy-authorization": true, "te": true, "trailer": true,
		"transfer-encoding": true, "upgrade": true, "cookie": true,
		"host": true, "content-length": true,
	}
	for k, vals := range src.Header {
		if hop[strings.ToLower(k)] {
			continue
		}
		if strings.HasPrefix(strings.ToLower(k), "x-panel-") || strings.HasPrefix(strings.ToLower(k), "x-device-") {
			continue
		}
		for _, v := range vals {
			dst.Header.Add(k, v)
		}
	}
}

func rewriteUpstreamOrigin(req *http.Request, publicHost, targetHost string) {
	for _, key := range []string{"Referer", "Origin"} {
		val := req.Header.Get(key)
		if val == "" {
			continue
		}
		val = strings.ReplaceAll(val, "https://"+publicHost, "https://"+targetHost)
		val = strings.ReplaceAll(val, "http://"+publicHost, "https://"+targetHost)
		req.Header.Set(key, val)
	}
}

func publicBaseURL(r *http.Request, c Config, publicHost string) string {
	scheme := "http"
	if cookieSecure(r, c) {
		scheme = "https"
	} else if s := strings.TrimSpace(c.PublicScheme); s == "http" || s == "https" {
		scheme = s
	}
	return scheme + "://" + publicHost
}

func mergedAccountCookie(acc ToolAccount) string {
	base := panelCookieForAccount(acc.Cookie)
	jarMu.Lock()
	extra := cookieJar[acc.ID]
	jarMu.Unlock()
	if len(extra) == 0 {
		return base
	}
	seen := map[string]bool{}
	parts := make([]string, 0, len(extra)+8)
	for _, part := range strings.Split(base, ";") {
		part = strings.TrimSpace(part)
		if part == "" || strings.HasPrefix(part, "ct_session=") {
			continue
		}
		name, _, _ := strings.Cut(part, "=")
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		parts = append(parts, part)
	}
	for name, value := range extra {
		if seen[name] {
			for i, part := range parts {
				if strings.HasPrefix(part, name+"=") {
					parts[i] = name + "=" + value
				}
			}
			continue
		}
		parts = append(parts, name+"="+value)
	}
	return strings.Join(parts, "; ")
}

func rememberUpstreamCookies(accountID int, cookies []string) {
	if accountID == 0 || len(cookies) == 0 {
		return
	}
	jarMu.Lock()
	defer jarMu.Unlock()
	jar := cookieJar[accountID]
	if jar == nil {
		jar = map[string]string{}
		cookieJar[accountID] = jar
	}
	for _, raw := range cookies {
		name, value, ok := splitSetCookie(raw)
		if !ok || name == "" || value == "" || value == "deleted" {
			continue
		}
		jar[name] = value
	}
}

func splitSetCookie(raw string) (string, string, bool) {
	part := strings.TrimSpace(strings.Split(raw, ";")[0])
	name, value, ok := strings.Cut(part, "=")
	if !ok {
		return "", "", false
	}
	return strings.TrimSpace(name), value, true
}

func stripAuthSetCookies(resp *http.Response) {
	kept := resp.Header.Values("Set-Cookie")
	resp.Header.Del("Set-Cookie")
	for _, raw := range kept {
		name, _, ok := splitSetCookie(raw)
		if !ok || authCookie(name) {
			continue
		}
		raw = strings.ReplaceAll(raw, "domain=.google.com", "")
		raw = strings.ReplaceAll(raw, "domain=google.com", "")
		raw = strings.ReplaceAll(raw, "Domain=.google.com", "")
		raw = strings.ReplaceAll(raw, "Domain=flow.google.com", "")
		raw = strings.ReplaceAll(raw, "domain=.flow.google.com", "")
		resp.Header.Add("Set-Cookie", raw)
	}
}

func authCookie(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "sid", "hsid", "ssid", "apisid", "sapisid", "osid", "nid", "sidcc", "aec":
		return true
	}
	if strings.Contains(n, "psid") || strings.Contains(n, "papisid") || strings.Contains(n, "osid") || strings.Contains(n, "authjs") || strings.Contains(n, "session-token") {
		return true
	}
	return false
}

func rewriteGoogleAuth(req *http.Request, cookie string) {
	if req == nil {
		return
	}
	auth := req.Header.Get("Authorization")
	if !strings.Contains(auth, "SAPISID") {
		return
	}
	origin := "https://flow.google.com"
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	var parts []string
	if sid := cookieValue(cookie, "SAPISID"); sid != "" {
		parts = append(parts, "SAPISIDHASH "+ts+"_"+googleSAPISIDHash(ts, sid, origin))
	}
	if sid := cookieValue(cookie, "__Secure-1PAPISID"); sid != "" {
		parts = append(parts, "SAPISID1PHASH "+ts+"_"+googleSAPISIDHash(ts, sid, origin))
	}
	if sid := cookieValue(cookie, "__Secure-3PAPISID"); sid != "" {
		parts = append(parts, "SAPISID3PHASH "+ts+"_"+googleSAPISIDHash(ts, sid, origin))
	}
	if len(parts) > 0 {
		req.Header.Set("Authorization", strings.Join(parts, " "))
	}
}

func googleSAPISIDHash(ts, sid, origin string) string {
	sum := sha1.Sum([]byte(ts + " " + sid + " " + origin))
	return hex.EncodeToString(sum[:])
}

func cookieValue(cookie, name string) string {
	prefix := name + "="
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, prefix) {
			return part[len(prefix):]
		}
	}
	return ""
}

func readRewriteBody(resp *http.Response) ([]byte, string, error) {
	if !rewriteContent(resp.Header.Get("Content-Type")) {
		return nil, "", nil
	}
	const max = 12 << 20
	raw, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, "", err
	}
	if len(raw) > max {
		return nil, "", fmt.Errorf("response too large")
	}
	enc := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	if enc == "gzip" {
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return raw, "", nil
		}
		defer zr.Close()
		plain, err := io.ReadAll(io.LimitReader(zr, max+1))
		if err != nil || len(plain) > max {
			return raw, "", nil
		}
		return plain, "gzip", nil
	}
	return raw, "", nil
}

func rewriteContent(contentType string) bool {
	ct := strings.ToLower(contentType)
	if ct == "" {
		return false
	}
	for _, part := range []string{"text/html", "javascript", "json", "text/css", "text/plain", "xml"} {
		if strings.Contains(ct, part) {
			return true
		}
	}
	return false
}

func rewriteText(s, publicBase, publicHost string) string {
	escapedBase := strings.ReplaceAll(publicBase, "/", `\/`)
	escapedHost := strings.ReplaceAll("//"+publicHost, "/", `\/`)
	replacer := strings.NewReplacer(
		`https:\/\/flow.google.com`, escapedBase,
		`http:\/\/flow.google.com`, escapedBase,
		`\/\/flow.google.com`, escapedHost,
		"https://flow.google.com", publicBase,
		"http://flow.google.com", publicBase,
		"//flow.google.com", "//"+publicHost,
	)
	return replacer.Replace(s)
}

func copyResponseHeaders(w http.ResponseWriter, resp *http.Response) {
	hop := map[string]bool{
		"connection": true, "keep-alive": true, "proxy-authenticate": true,
		"proxy-authorization": true, "trailer": true, "transfer-encoding": true,
		"upgrade": true, "content-length": true,
	}
	for k, vals := range resp.Header {
		if hop[strings.ToLower(k)] {
			continue
		}
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
}

func veoBlocked(path string) bool {
	p := strings.ToLower(path)
	switch p {
	case "/api/auth/session", "/api/auth/csrf", "/api/auth/providers":
		return false
	}
	if p == "/pricing" || strings.HasPrefix(p, "/pricing/") || p == "/api/auth" {
		return true
	}
	for _, prefix := range []string{"/login", "/sign-in", "/signin", "/signup", "/sign-up", "/register", "/account", "/billing", "/settings", "/api/auth/signin", "/api/auth/signout", "/api/auth/callback", "/api/auth/error"} {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	return false
}

func chromeClient() *http.Client {
	upstreamOnce.Do(func() {
		upstreamClient = &http.Client{
			Timeout: 90 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Transport: &http.Transport{
				DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return dialChrome(ctx, addr)
				},
				ForceAttemptHTTP2:   false,
				TLSHandshakeTimeout: 20 * time.Second,
				DisableKeepAlives:   true,
			},
		}
	})
	return upstreamClient
}

func dialChrome(ctx context.Context, addr string) (net.Conn, error) {
	tcp, err := panelDialTCP(ctx, addr)
	if err != nil {
		return nil, err
	}
	host, _, _ := net.SplitHostPort(addr)
	spec, err := utls.UTLSIdToSpec(utls.HelloChrome_Auto)
	if err != nil {
		tcp.Close()
		return nil, err
	}
	for _, ext := range spec.Extensions {
		if alpn, ok := ext.(*utls.ALPNExtension); ok {
			alpn.AlpnProtocols = []string{"http/1.1"}
		}
	}
	uConn := utls.UClient(tcp, &utls.Config{
		ServerName:         host,
		InsecureSkipVerify: true,
	}, utls.HelloCustom)
	if err := uConn.ApplyPreset(&spec); err != nil {
		tcp.Close()
		return nil, err
	}
	if err := uConn.HandshakeContext(ctx); err != nil {
		tcp.Close()
		return nil, err
	}
	return uConn, nil
}
