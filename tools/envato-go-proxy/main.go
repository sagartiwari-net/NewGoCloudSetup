package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/proxy"
)

const FALLBACK_PROXY_FILE = "proxy.txt"
const defaultLandingPath = "/photos"

// Config structures with MySQL support
type Config struct {
	Port          string `json:"port"`
	TargetURL     string `json:"target_url"`
	PublicHost    string `json:"public_host"`
	PublicScheme  string `json:"public_scheme"`
	UserAgent     string `json:"user_agent"`
	CookiePath    string `json:"cookie_path"`
	Proxy         string `json:"proxy"`
	PanelDB       string `json:"panel_db"`
	BypassAuth    bool   `json:"bypass_auth"`
	MySQLHost     string `json:"mysql_host"`
	MySQLPort     string `json:"mysql_port"`
	MySQLUser     string `json:"mysql_user"`
	MySQLPassword string `json:"mysql_password"`
	MySQLDB       string `json:"mysql_db"`
	WebsiteID     int    `json:"website_id"`
}

func configFilePath() string {
	if v := strings.TrimSpace(os.Getenv("CONFIG_FILE")); v != "" {
		return v
	}
	return "config.json"
}

var (
	currentConfig Config
	configModTime time.Time

	currentCookies string
	cookiesModTime time.Time

	// DB handle
	db               *sql.DB
	currentWebsiteID int = 6

	// Regexp for stripping Subresource Integrity (SRI)
	integrityRegex      = regexp.MustCompile(`(?i)\s*integrity=(?:"[^"]*"|'[^']*')`)
	webpackAttrRegex    = regexp.MustCompile(`(?i)(?:"integrity"|'integrity')\s*:`)
	webpackSetAttrRegex = regexp.MustCompile(`(?i)setAttribute\(\s*[\'"]integrity[\'"]`)

	// Live cookie jar: stores upstream Set-Cookie values (Cloudflare __cf_bm, _cfuvid etc.)
	cookieJarMu sync.RWMutex
	cookieJar   = make(map[string]string) // name → value

	// Cooldown map to prevent race conditions / duplicate swaps on concurrent requests
	sessionLastSwapMu sync.Mutex
	sessionLastSwap   = make(map[string]time.Time)

	// Deduplication cache for premium downloads
	downloadHitCache sync.Map

	// Fallback proxy hot-reloading
	currentFallbackProxy   *url.URL
	fallbackProxyModTime   time.Time
	fallbackProxyFileMutex sync.RWMutex
)

type contextKey string

const proxyContextKey contextKey = "proxy_candidates"
const directDialKey contextKey = "direct_dial"
const envatoDownloadUserKey contextKey = "envato_download_user"
const envatoChargeNameKey contextKey = "envato_charge_name"
const envatoUserNameKey contextKey = "envato_panel_user"

var reverseProxy *httputil.ReverseProxy

// loadConfig loads the configuration from CONFIG_FILE (default config.json) with hot-reloading
func loadConfig() Config {
	path := configFilePath()
	info, err := os.Stat(path)
	if err != nil {
		return currentConfig
	}
	if !info.ModTime().After(configModTime) {
		return currentConfig
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[CONFIG] Error reading config file: %v", err)
		return currentConfig
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("[CONFIG] Error parsing config JSON: %v", err)
		return currentConfig
	}

	if cfg.Port == "" {
		cfg.Port = "5261"
	}
	if cfg.TargetURL == "" {
		cfg.TargetURL = "https://app.envato.com"
	}
	if cfg.CookiePath == "" {
		cfg.CookiePath = "cookie.json"
	}
	if cfg.PublicHost == "" {
		cfg.PublicHost = "127.0.0.1:" + cfg.Port
	}
	if cfg.PublicScheme == "" {
		cfg.PublicScheme = "http"
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"
	}

	currentConfig = cfg
	configModTime = info.ModTime()

	log.Printf("[CONFIG] Config loaded from %s (Port: %s, Target: %s, Public: %s://%s, panel_db=%v) ✅",
		path, cfg.Port, cfg.TargetURL, cfg.PublicScheme, cfg.PublicHost, strings.TrimSpace(cfg.PanelDB) != "")
	return cfg
}

func usesPanelAccountMode(cfg Config) bool {
	return strings.TrimSpace(cfg.PanelDB) != ""
}

// loadCookies loads and parses premium cookies from path (JSON array or Netscape txt format) with hot-reloading
func loadCookies(path string) string {
	if path == "" {
		path = "cookie.json"
	}
	info, err := os.Stat(path)
	if err != nil {
		return currentCookies
	}
	if !info.ModTime().After(cookiesModTime) {
		return currentCookies
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[COOKIE] Error reading cookie file %s: %v", path, err)
		return currentCookies
	}

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		currentCookies = ""
		cookiesModTime = info.ModTime()
		return ""
	}

	var parsedCookies string

	// GoAuto {"referer"?: "...", "cookies":[...]} — referer is optional.
	if strings.HasPrefix(trimmed, "{") {
		var wrap struct {
			Cookies json.RawMessage `json:"cookies"`
		}
		if err := json.Unmarshal([]byte(trimmed), &wrap); err == nil {
			if inner := strings.TrimSpace(string(wrap.Cookies)); strings.HasPrefix(inner, "[") {
				trimmed = inner
			}
		}
	}

	// 1. Check if it's a JSON array
	if strings.HasPrefix(trimmed, "[") {
		var cookieList []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal([]byte(trimmed), &cookieList); err == nil {
			var cookieParts []string
			for _, c := range cookieList {
				if c.Name != "" && c.Value != "" {
					cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", c.Name, c.Value))
				}
			}
			parsedCookies = strings.Join(cookieParts, "; ")
		}
	}

	// 2. Netscape format or Raw lines fallback if JSON parsing wasn't done or failed
	if parsedCookies == "" {
		lines := strings.Split(trimmed, "\n")
		var cookieParts []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || (strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "#HttpOnly_")) || strings.HasPrefix(line, "//") {
				continue
			}
			if strings.HasPrefix(line, "#HttpOnly_") {
				line = strings.TrimPrefix(line, "#HttpOnly_")
			}

			columns := strings.Split(line, "\t")
			if len(columns) >= 7 {
				name := strings.TrimSpace(columns[5])
				value := strings.TrimSpace(columns[6])
				if name != "" {
					cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", name, value))
				}
				continue
			}

			fields := strings.Fields(line)
			if len(fields) >= 7 {
				name := strings.TrimSpace(fields[5])
				value := strings.TrimSpace(fields[6])
				if name != "" {
					cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", name, value))
				}
				continue
			}

			if strings.Contains(line, "=") && !strings.Contains(line, " ") {
				cookieParts = append(cookieParts, line)
			}
		}
		parsedCookies = strings.Join(cookieParts, "; ")
	}

	currentCookies = parsedCookies
	cookiesModTime = info.ModTime()
	log.Printf("[COOKIE] Premium cookies reloaded (Total cookies extracted) ✅")
	return currentCookies
}

// loadFallbackProxy loads outbound proxy from fallback proxy.txt
func loadFallbackProxy() *url.URL {
	fallbackProxyFileMutex.Lock()
	defer fallbackProxyFileMutex.Unlock()

	info, err := os.Stat(FALLBACK_PROXY_FILE)
	if err != nil {
		currentFallbackProxy = nil
		return nil
	}
	if !info.ModTime().After(fallbackProxyModTime) {
		return currentFallbackProxy
	}

	data, err := os.ReadFile(FALLBACK_PROXY_FILE)
	if err != nil {
		return currentFallbackProxy
	}

	var proxyStr string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		proxyStr = line
		break
	}

	fallbackProxyModTime = info.ModTime()
	if proxyStr == "" {
		currentFallbackProxy = nil
		return nil
	}

	parsed := parseProxyString(proxyStr)
	if parsed != nil {
		currentFallbackProxy = parsed
		log.Printf("[PROXY] Loaded fallback proxy from proxy.txt: %s://%s ✅", parsed.Scheme, parsed.Host)
	} else {
		currentFallbackProxy = nil
	}
	return currentFallbackProxy
}

// parseProxyString parses proxy URLs. Shorthand host:port:user:pass defaults to HTTP.
// Use explicit prefix for other types: http:host:port:user:pass or socks5:host:port:user:pass
func parseProxyString(s string) *url.URL {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	shorthandScheme := "http"
	if strings.HasPrefix(strings.ToLower(s), "http:") && !strings.Contains(s, "://") {
		shorthandScheme = "http"
		s = s[5:]
	} else if strings.HasPrefix(strings.ToLower(s), "socks5:") && !strings.Contains(s, "://") {
		shorthandScheme = "socks5"
		s = s[7:]
	}

	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return nil
		}
		scheme := strings.ToLower(u.Scheme)
		if scheme != "socks5" && scheme != "socks5h" && scheme != "http" && scheme != "https" {
			log.Printf("[PROXY] Unknown proxy scheme '%s' — skipping", scheme)
			return nil
		}
		return u
	}

	// Host:Port:User:Pass or Host:Port shorthand
	parts := strings.SplitN(s, ":", 4)
	switch len(parts) {
	case 2:
		raw := fmt.Sprintf("%s://%s:%s", shorthandScheme, parts[0], parts[1])
		u, _ := url.Parse(raw)
		return u
	case 4:
		raw := fmt.Sprintf("%s://%s:%s@%s:%s",
			shorthandScheme,
			url.QueryEscape(parts[2]),
			url.QueryEscape(parts[3]),
			parts[0], parts[1])
		u, _ := url.Parse(raw)
		return u
	}

	log.Printf("[PROXY] Unrecognized proxy format '%s' — skipping", s)
	return nil
}

// getActiveProxy returns the proxy from config or fallback proxy.txt
func getActiveProxy(cfg Config) *url.URL {
	if cfg.Proxy != "" {
		if parsed := parseProxyString(cfg.Proxy); parsed != nil {
			return parsed
		}
	}
	return loadFallbackProxy()
}

// dialThroughProxy connects to the remote host using SOCKS5 or HTTP CONNECT
func dialThroughProxy(ctx context.Context, targetAddr string, proxyURL *url.URL) (net.Conn, error) {
	switch strings.ToLower(proxyURL.Scheme) {
	case "socks5", "socks5h":
		var auth *proxy.Auth
		if proxyURL.User != nil {
			pass, _ := proxyURL.User.Password()
			auth = &proxy.Auth{
				User:     proxyURL.User.Username(),
				Password: pass,
			}
		}
		proxyHost := proxyURL.Host
		if !strings.Contains(proxyHost, ":") {
			proxyHost += ":1080"
		}
		baseDialer := &net.Dialer{Timeout: 20 * time.Second}
		socks5Dialer, err := proxy.SOCKS5("tcp4", proxyHost, auth, baseDialer)
		if err != nil {
			return nil, fmt.Errorf("socks5 dialer init: %w", err)
		}
		if cd, ok := socks5Dialer.(proxy.ContextDialer); ok {
			return cd.DialContext(ctx, "tcp4", targetAddr)
		}
		return socks5Dialer.Dial("tcp4", targetAddr)

	case "http", "https":
		proxyHost := proxyURL.Host
		if !strings.Contains(proxyHost, ":") {
			if proxyURL.Scheme == "https" {
				proxyHost += ":443"
			} else {
				proxyHost += ":80"
			}
		}
		conn, err := (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, "tcp4", proxyHost)
		if err != nil {
			return nil, fmt.Errorf("connect to HTTP proxy: %w", err)
		}
		connectLine := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", targetAddr, targetAddr)
		if proxyURL.User != nil {
			pass, _ := proxyURL.User.Password()
			creds := base64.StdEncoding.EncodeToString([]byte(proxyURL.User.Username() + ":" + pass))
			connectLine += "Proxy-Authorization: Basic " + creds + "\r\n"
		}
		connectLine += "\r\n"
		if _, err := conn.Write([]byte(connectLine)); err != nil {
			conn.Close()
			return nil, err
		}
		br := bufio.NewReader(conn)
		resp, err := http.ReadResponse(br, nil)
		if err != nil {
			conn.Close()
			return nil, err
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			conn.Close()
			return nil, fmt.Errorf("HTTP proxy CONNECT refused: %s", resp.Status)
		}
		return conn, nil

	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s", proxyURL.Scheme)
	}
}

type uTLSConn struct {
	*utls.UConn
}

func (c *uTLSConn) ConnectionState() tls.ConnectionState {
	cs := c.UConn.ConnectionState()
	return tls.ConnectionState{
		Version:            cs.Version,
		HandshakeComplete:  cs.HandshakeComplete,
		DidResume:          cs.DidResume,
		CipherSuite:        cs.CipherSuite,
		NegotiatedProtocol: cs.NegotiatedProtocol,
		ServerName:         cs.ServerName,
	}
}

func proxyCandidatesFromContext(ctx context.Context) []*url.URL {
	if list, ok := ctx.Value(proxyContextKey).([]*url.URL); ok && len(list) > 0 {
		return list
	}
	if px, ok := ctx.Value(proxyContextKey).(*url.URL); ok && px != nil {
		return []*url.URL{px}
	}
	return nil
}

func appendProxyCandidate(list []*url.URL, seen map[string]bool, px *url.URL) []*url.URL {
	if px == nil || px.Host == "" {
		return list
	}
	key := strings.ToLower(px.Scheme) + "://" + px.Host
	if seen[key] {
		return list
	}
	seen[key] = true
	return append(list, px)
}

// lookupPoolProxy returns the first active proxy from ctrl panel ahrefs_proxies table.
func lookupPoolProxy() *url.URL {
	if db == nil {
		return nil
	}
	var endpoint, proxyType string
	err := db.QueryRow(
		"SELECT endpoint, proxy_type FROM ahrefs_proxies WHERE status = 'active' ORDER BY id ASC LIMIT 1",
	).Scan(&endpoint, &proxyType)
	if err != nil {
		return nil
	}
	px := parseProxyEndpoint(endpoint, proxyType)
	if px != nil {
		log.Printf("[PROXY] Using ahrefs_proxies pool: %s (%s) ✅", endpoint, proxyType)
	}
	return px
}

// resolveOutboundProxyCandidates priority: account → website → ahrefs_proxies pool → config.json → proxy.txt
func resolveOutboundProxyCandidates(cfg Config, webID int, accountProxy string) []*url.URL {
	seen := make(map[string]bool)
	var candidates []*url.URL

	candidates = appendProxyCandidate(candidates, seen, lookupProxyByRef(accountProxy))
	candidates = appendProxyCandidate(candidates, seen, loadProxyFromDB(webID))
	candidates = appendProxyCandidate(candidates, seen, lookupPoolProxy())
	if cfg.Proxy != "" {
		candidates = appendProxyCandidate(candidates, seen, parseProxyString(cfg.Proxy))
	}
	candidates = appendProxyCandidate(candidates, seen, loadFallbackProxy())

	return candidates
}

// dialChrome dials via proxy (with fallback chain), wrapping the connection in uTLS
func dialChrome(ctx context.Context, addr string, candidates []*url.URL) (*uTLSConn, error) {
	host, _, _ := net.SplitHostPort(addr)
	if len(candidates) == 0 {
		log.Printf("[PROXY] no panel proxy — connecting directly")
		tcpConn, err := (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, err
		}
		return finishChromeConn(ctx, tcpConn, host, "direct")
	}

	var lastErr error
	for _, px := range candidates {
		tcpConn, err := dialThroughProxy(ctx, addr, px)
		if err != nil {
			lastErr = err
			log.Printf("[PROXY] %s failed (%v) — trying next", px.Host, err)
			continue
		}
		return finishChromeConn(ctx, tcpConn, host, px.Host)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no proxy")
	}
	return nil, fmt.Errorf("all outbound proxies failed: %w", lastErr)
}

func finishChromeConn(ctx context.Context, tcpConn net.Conn, host, via string) (*uTLSConn, error) {
	spec, err := utls.UTLSIdToSpec(utls.HelloChrome_Auto)
	if err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("failed to get uTLS spec: %w", err)
	}
	for _, ext := range spec.Extensions {
		if alpn, ok := ext.(*utls.ALPNExtension); ok {
			alpn.AlpnProtocols = []string{"http/1.1"}
		}
	}
	uConn := utls.UClient(tcpConn, &utls.Config{
		ServerName:         host,
		InsecureSkipVerify: true,
	}, utls.HelloCustom)
	if errPreset := uConn.ApplyPreset(&spec); errPreset != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("%s uTLS preset: %w", via, errPreset)
	}
	if errHandshake := uConn.HandshakeContext(ctx); errHandshake != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("%s TLS handshake: %w", via, errHandshake)
	}
	return &uTLSConn{uConn}, nil
}

// mergeCookies merges incoming client cookies with the premium cookies.
// Premium cookies override incoming client cookies if there are conflicts.
func mergeCookies(clientCookieStr, premiumCookieStr string) string {
	if premiumCookieStr == "" {
		return clientCookieStr
	}
	if clientCookieStr == "" {
		return premiumCookieStr
	}

	cookies := make(map[string]string)
	parts := strings.Split(clientCookieStr, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		subparts := strings.SplitN(part, "=", 2)
		if len(subparts) == 2 {
			cookies[subparts[0]] = subparts[1]
		}
	}

	premiumParts := strings.Split(premiumCookieStr, ";")
	for _, part := range premiumParts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		subparts := strings.SplitN(part, "=", 2)
		if len(subparts) == 2 {
			cookies[subparts[0]] = subparts[1]
		}
	}

	var merged []string
	for k, v := range cookies {
		merged = append(merged, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(merged, "; ")
}

// resolveTargetHost resolves which remote host and scheme to use based on the path
func resolveTargetHost(reqPath string) (string, string) {
	// Strip proxy path prefixes and forward to appropriate domains
	if strings.HasPrefix(reqPath, "/assets-proxy") {
		return "assets.envato.com", "https"
	}
	if strings.HasPrefix(reqPath, "/api-proxy") {
		return "api.envato.com", "https"
	}
	if strings.HasPrefix(reqPath, "/account-proxy") {
		return "account.envato.com", "https"
	}
	if strings.HasPrefix(reqPath, "/elements-proxy") {
		return "elements.envato.com", "https"
	}
	if strings.HasPrefix(reqPath, "/__resized__") {
		return "elements-resized.envatousercontent.com", "https"
	}
	if strings.HasPrefix(reqPath, "/__video__") {
		return "video-previews.elements.envatousercontent.com", "https"
	}
	if strings.HasPrefix(reqPath, "/__domain__/") {
		parts := strings.SplitN(strings.TrimPrefix(reqPath, "/__domain__/"), "/", 2)
		if len(parts) > 0 && parts[0] != "" {
			return parts[0], "https"
		}
	}

	// Default to primary app.envato.com
	return "app.envato.com", "https"
}

// cleanRequestPath returns the clean path expected by the target host after stripping prefixes
func cleanRequestPath(reqPath string) string {
	prefixes := []string{
		"/assets-proxy",
		"/api-proxy",
		"/account-proxy",
		"/elements-proxy",
		"/__resized__",
		"/__video__",
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(reqPath, prefix) {
			clean := strings.TrimPrefix(reqPath, prefix)
			if clean == "" {
				return "/"
			}
			return clean
		}
	}

	if strings.HasPrefix(reqPath, "/__domain__/") {
		parts := strings.SplitN(strings.TrimPrefix(reqPath, "/__domain__/"), "/", 2)
		if len(parts) > 1 {
			return "/" + parts[1]
		}
		return "/"
	}

	return reqPath
}

// CustomRoundTripper wraps a transport to delete tracking and header properties
type CustomRoundTripper struct {
	Transport http.RoundTripper
}

func (crt *CustomRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Del("X-Forwarded-For")
	req.Header.Del("X-Real-IP")

	// Strip browser client hints, then re-apply from outbound User-Agent (Director already set UA).
	ua := req.Header.Get("User-Agent")
	req.Header.Del("Sec-Ch-Ua")
	req.Header.Del("Sec-Ch-Ua-Mobile")
	req.Header.Del("Sec-Ch-Ua-Platform")
	req.Header.Del("Sec-Ch-Ua-Model")
	req.Header.Del("Sec-Ch-Ua-Arch")
	req.Header.Del("Sec-Ch-Ua-Full-Version")
	req.Header.Del("Sec-Ch-Ua-Full-Version-List")
	req.Header.Del("Sec-Ch-Ua-Platform-Version")
	req.Header.Del("Sec-Ch-Ua-Bitness")
	if ua != "" {
		setClientHintHeaders(req, ua)
	}

	resp, err := crt.Transport.RoundTrip(req)
	if err != nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		return resp, err
	}

	// Cloudflare may still set __cf_bm on 403 — merge into jar and retry once.
	setCookies := resp.Header["Set-Cookie"]
	if len(setCookies) == 0 {
		return resp, err
	}
	before := req.Header.Get("Cookie")
	updateCookieJar(setCookies)
	after := mergeCookiesWithJar(before)
	if after == before {
		return resp, err
	}

	resp.Body.Close()
	retryReq := req.Clone(req.Context())
	retryReq.Header.Set("Cookie", after)
	log.Printf("[CF-RETRY] Retrying %s%s with refreshed Cloudflare cookies", retryReq.Host, retryReq.URL.Path)
	return crt.Transport.RoundTrip(retryReq)
}

type Account struct {
	ID        int
	Name      string
	Cookie    string
	UserAgent string
	Proxy     string
}

func selectActiveAccount(websiteID int) (Account, error) {
	var acc Account
	if db == nil {
		return acc, fmt.Errorf("database offline")
	}
	query := "SELECT id, name, cookie, user_agent, COALESCE(proxy, '') FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' ORDER BY last_used_at ASC LIMIT 1"
	err := db.QueryRow(query, websiteID).Scan(&acc.ID, &acc.Name, &acc.Cookie, &acc.UserAgent, &acc.Proxy)
	if err != nil {
		return acc, err
	}

	// Update last_used_at to rotate account usage
	_, _ = db.Exec("UPDATE ahrefs_accounts SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?", acc.ID)
	return acc, nil
}

func swapActiveAccount(sessionToken string, currentAssignedID int64) (Account, error) {
	var nextAcc Account
	if db == nil {
		return nextAcc, fmt.Errorf("database offline")
	}

	// Strict Cooldown: check if this session already swapped in the last 8 seconds
	sessionLastSwapMu.Lock()
	lastSwapTime, exists := sessionLastSwap[sessionToken]
	if exists && time.Since(lastSwapTime) < 8*time.Second {
		sessionLastSwapMu.Unlock()
		log.Printf("[SWAP] 🛡️ Cooldown active for session %s (swapped %s ago). Skipping rotation trigger.", sessionToken, time.Since(lastSwapTime))
		return nextAcc, fmt.Errorf("swap cooldown active")
	}
	sessionLastSwap[sessionToken] = time.Now()
	sessionLastSwapMu.Unlock()

	var username string
	var sessionWebsiteID int
	err := db.QueryRow("SELECT username, website_id FROM ahrefs_sessions WHERE session_token = ?", sessionToken).Scan(&username, &sessionWebsiteID)
	if err != nil {
		return nextAcc, err
	}

	// Check how many active accounts exist for this website ID
	var activeCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM ahrefs_accounts WHERE website_id = ? AND status = 'active'", sessionWebsiteID).Scan(&activeCount)

	if activeCount <= 1 {
		// Only one active account exists in database!
		errSelectOnly := db.QueryRow("SELECT id, name, cookie, user_agent, COALESCE(proxy, '') FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' LIMIT 1", sessionWebsiteID).Scan(&nextAcc.ID, &nextAcc.Name, &nextAcc.Cookie, &nextAcc.UserAgent, &nextAcc.Proxy)
		if errSelectOnly != nil {
			return nextAcc, errSelectOnly
		}

		if nextAcc.ID == int(currentAssignedID) {
			log.Printf("[SWAP] 🛡️ Only 1 active account (%s) exists in database. Skipping rotation trigger.", nextAcc.Name)
			return nextAcc, fmt.Errorf("only 1 active account exists and is already active")
		}
	} else {
		// 2+ accounts exist. Fetch next active account that is NOT the current one (to rotate)
		var errSelect error
		if currentAssignedID != 0 {
			errSelect = db.QueryRow("SELECT id, name, cookie, user_agent, COALESCE(proxy, '') FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' AND id != ? ORDER BY last_used_at ASC LIMIT 1", sessionWebsiteID, currentAssignedID).Scan(&nextAcc.ID, &nextAcc.Name, &nextAcc.Cookie, &nextAcc.UserAgent, &nextAcc.Proxy)
		}
		if currentAssignedID == 0 || errSelect != nil {
			errSelect = db.QueryRow("SELECT id, name, cookie, user_agent, COALESCE(proxy, '') FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' ORDER BY last_used_at ASC LIMIT 1", sessionWebsiteID).Scan(&nextAcc.ID, &nextAcc.Name, &nextAcc.Cookie, &nextAcc.UserAgent, &nextAcc.Proxy)
		}

		if errSelect != nil {
			return nextAcc, errSelect
		}
	}

	// Update session with new assigned account
	_, _ = db.Exec("UPDATE ahrefs_sessions SET assigned_account_id = ? WHERE session_token = ? AND website_id = ?", nextAcc.ID, sessionToken, sessionWebsiteID)

	// Update last_used_at to rotate account usage
	if currentAssignedID != 0 {
		_, _ = db.Exec("UPDATE ahrefs_accounts SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?", currentAssignedID)
	}
	_, _ = db.Exec("UPDATE ahrefs_accounts SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?", nextAcc.ID)

	// Clear Cloudflare cookieJar on account switch
	cookieJarMu.Lock()
	for k := range cookieJar {
		delete(cookieJar, k)
	}
	cookieJarMu.Unlock()
	log.Printf("[SWAP] 🧹 Cleared cookieJar on account switch (old CF cookies removed for new account %s)", nextAcc.Name)

	return nextAcc, nil
}

func envatoSignInRequest(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	path := strings.ToLower(r.URL.Path)
	if strings.Contains(path, "sign_out") || strings.Contains(path, "logout") {
		return false
	}
	return strings.Contains(path, "sign_in") || strings.Contains(path, "/login")
}

func detectEnvatoLogoutOrLimit(body string) bool {
	if body == "" {
		return false
	}
	low := strings.ToLower(body)
	// Logged-out shell: nav Sign in button (data-cy / analytics), SSO forms, or login copy.
	return strings.Contains(body, `id="sso-forms__submit"`) ||
		strings.Contains(body, `data-testid="submitButton"`) ||
		strings.Contains(body, `data-cy="sign-in"`) ||
		strings.Contains(body, `data-analytics-target="sign-in"`) ||
		strings.Contains(body, "Sign in to Envato App") ||
		strings.Contains(body, "Sign in to Envato Elements") ||
		strings.Contains(body, "Sign in to your Envato account") ||
		strings.Contains(body, "Unexpected Server Error") ||
		(strings.Contains(body, ">Sign in<") && (strings.Contains(low, "sign_in") || strings.Contains(body, "Choose your plan"))) ||
		(strings.Contains(body, "account.envato.com") && strings.Contains(low, "sign_in") && strings.Contains(low, "sign in"))
}

func renderWaitingForAccountPage() string {
	// Never reload /account-proxy/sign_in — that re-triggers switch forever.
	script := fmt.Sprintf(`setTimeout(function(){ window.location.replace(%q); }, 5000);`, defaultLandingPath)
	return lightMessageHTML(
		"Waiting for an account",
		"Waiting for an account",
		`No active <span class="brand">Envato</span> account is available right now. Retrying automatically...`,
		`<div class="pill"><span class="dot"></span>Checking accounts...</div><p class="foot">Add or fix a mapped account cookie in the panel</p>`,
		script,
	)
}

func renderAccountSwitchPage(accountName string) string {
	msg := "Account logged out. Switching to another account..."
	if accountName != "" {
		msg = "Account logged out. Switching to " + html.EscapeString(accountName) + "..."
	}
	// Go to the app with the new account — do NOT reload sign_in (causes Acc1↔Acc2 loop).
	script := fmt.Sprintf(`(function(){
  try { sessionStorage.setItem("tm_envato_switched_at", String(Date.now())); } catch (e) {}
  try { sessionStorage.removeItem("tm_envato_signin_switch"); } catch (e) {}
  setTimeout(function(){ window.location.replace(%q); }, 1200);
})();`, defaultLandingPath)
	return lightMessageHTML("Switching account", "Switching account", msg,
		`<div class="pill"><span class="dot"></span>Opening with the next account</div><p class="foot">You will be redirected to Envato in a moment</p>`,
		script)
}

func lightMessageHTML(title, heading, message, extra, script string) string {
	scriptTag := ""
	if script != "" {
		scriptTag = "<script>" + script + "</script>"
	}
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1.0">
<title>` + html.EscapeString(title) + `</title>
<style>
* { box-sizing:border-box;margin:0;padding:0; }
body { min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;background:#eef3f8;color:#0f172a;font-family:system-ui,-apple-system,Segoe UI,sans-serif; }
.card { width:min(440px,100%);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px 36px;text-align:center; }
.ring { width:78px;height:78px;margin:0 auto 22px;border-radius:50%;background:conic-gradient(#3b82f6 0 70deg,#e7eef8 70deg 360deg);display:grid;place-items:center; }
.ring.spin { animation:spin .9s linear infinite; }
.lock { width:64px;height:64px;border-radius:50%;background:#fff;display:grid;place-items:center;font-size:26px; }
.ring.spin .lock { animation:spin .9s linear infinite reverse; }
h1 { font-size:28px;line-height:1.2;font-weight:800;letter-spacing:-.03em;margin-bottom:12px; }
.msg { color:#64748b;font-size:15px;line-height:1.55; }
.brand { color:#2563eb;font-weight:700; }
.pill { margin:22px auto 0;display:inline-flex;align-items:center;gap:8px;padding:8px 14px;border:1px solid #e6ebf2;border-radius:999px;color:#334155;font-size:14px;background:#fff; }
.dot { width:14px;height:14px;border-radius:50%;border:2px solid #dbe4f0;border-top-color:#3b82f6;animation:spin .8s linear infinite; }
.foot { margin-top:18px;color:#94a3b8;font-size:13px; }
.extra { margin-top:22px;color:#334155;font-size:14px; }
.stats { display:flex;justify-content:center;gap:12px;margin-top:18px; }
.stat { background:#f8fafc;border:1px solid #e6ebf2;border-radius:16px;padding:12px 16px;min-width:120px; }
.stat b { display:block;font-size:20px;color:#0f172a; }
.retry { display:inline-block;margin-top:18px;color:#2563eb;font-weight:700;text-decoration:none; }
@keyframes spin { to { transform:rotate(360deg); } }
</style>
</head>
<body>
<div class="card">
<div class="ring spin"><div class="lock">🔒</div></div>
<h1>` + html.EscapeString(heading) + `</h1>
<p class="msg">` + message + `</p>
` + extra + `
</div>
` + scriptTag + `
</body>
</html>`
}

func renderSwappingPage(redirectTo string) string {
	script := "setTimeout(function(){ window.location.reload(); }, 1800);"
	if redirectTo != "" {
		script = fmt.Sprintf("setTimeout(function(){ window.location.href = %q; }, 1800);", redirectTo)
	}
	return lightMessageHTML(
		"Rotating Session",
		"Rotating Session",
		"Switching to a fresh premium account. Please wait a moment.",
		"",
		script,
	)
}

func renderAccessDeniedPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	fmt.Fprint(w, lightMessageHTML(
		"Access Denied",
		"Access Denied",
		"You cannot open <span class=\"brand\">Envato</span> directly. Open it again from your access link.",
		`<p class="foot">Your session ended or this browser is not authorized</p>`,
		"",
	))
}

func renderLimitReachedPage(w http.ResponseWriter, username string, used, limit int) {
	_ = username
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	extra := fmt.Sprintf(`<div class="stats"><div class="stat"><span>Downloads used</span><b>%d</b></div><div class="stat"><span>Daily limit</span><b>%d</b></div></div><p class="extra">Your limit resets at midnight (12:00 AM IST).</p>`, used, limit)
	fmt.Fprint(w, lightMessageHTML(
		"Daily Download Limit Reached",
		"Daily Limit Reached",
		"You have used your download limit for today.",
		extra,
		"",
	))
}

func isStaticPath(path string) bool {
	lowerPath := strings.ToLower(path)
	if strings.HasPrefix(lowerPath, "/assets-proxy/") ||
		strings.HasPrefix(lowerPath, "/__resized__/") ||
		strings.HasPrefix(lowerPath, "/__video__/") {
		if strings.Contains(lowerPath, "/files/") ||
			strings.Contains(lowerPath, "download") ||
			strings.Contains(lowerPath, "license") ||
			strings.Contains(lowerPath, ".zip") ||
			strings.Contains(lowerPath, ".wav") ||
			strings.Contains(lowerPath, ".mp3") {
			return false
		}
		return true
	}
	exts := []string{".js", ".css", ".woff", ".woff2", ".ttf", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico"}
	for _, ext := range exts {
		if strings.HasSuffix(lowerPath, ext) || strings.Contains(lowerPath, ext+"?") {
			return true
		}
	}
	return false
}

func isLoginOrLogoutPath(path string) bool {
	lowerPath := strings.ToLower(path)
	return strings.Contains(lowerPath, "logout") ||
		strings.Contains(lowerPath, "sign_out")
}

func isDownloadRequest(path string) bool {
	lowerPath := strings.ToLower(path)
	if strings.Contains(lowerPath, "preview") ||
		strings.Contains(lowerPath, "my-downloads") ||
		strings.Contains(lowerPath, "my_downloads") {
		return false
	}
	return strings.Contains(lowerPath, "download") ||
		strings.Contains(lowerPath, "license") ||
		strings.Contains(lowerPath, ".zip") ||
		strings.Contains(lowerPath, ".wav") ||
		strings.Contains(lowerPath, ".mp3")
}

func downloadIdentityKey(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	// Prefer stable item identifiers used by Envato API/download URLs.
	for _, key := range []string{"itemUuid", "item_uuid", "item_id", "itemId", "uuid"} {
		if v := strings.TrimSpace(getQueryParamCaseInsensitive(r.URL, key)); v != "" {
			return "item:" + strings.ToLower(v)
		}
	}
	// Signed URLs often include filename in response-content-disposition.
	if fn := strings.TrimSpace(extractFilenameFromQuery(r.URL)); fn != "" {
		return "file:" + strings.ToLower(fn)
	}
	// Fallback for non-standard download routes.
	return "path:" + strings.ToLower(r.URL.Path)
}

func getQueryParamCaseInsensitive(u *url.URL, key string) string {
	target := strings.ToLower(key)
	for k, vals := range u.Query() {
		if strings.ToLower(k) == target && len(vals) > 0 {
			return vals[0]
		}
	}
	return ""
}

func extractFilenameFromQuery(u *url.URL) string {
	if fn := getQueryParamCaseInsensitive(u, "filename"); fn != "" {
		return fn
	}
	if rcd := getQueryParamCaseInsensitive(u, "response-content-disposition"); rcd != "" {
		if idx := strings.Index(strings.ToLower(rcd), "filename="); idx != -1 {
			filename := rcd[idx+9:]
			filename = strings.Trim(filename, `"' `)
			if idx2 := strings.Index(filename, `"`); idx2 != -1 {
				filename = filename[:idx2]
			} else if idx2 := strings.Index(filename, `'`); idx2 != -1 {
				filename = filename[:idx2]
			}
			if filename != "" {
				return filename
			}
		}
	}
	return ""
}

func extractItemName(path string, u *url.URL, referer string) string {
	if fn := extractFilenameFromQuery(u); fn != "" {
		return fn
	}
	lowerPath := strings.ToLower(path)
	if idx := strings.Index(lowerPath, "/items/"); idx != -1 {
		subPath := path[idx+7:]
		parts := strings.Split(subPath, "/")
		if len(parts) > 0 && parts[0] != "" {
			return parts[0]
		}
	}

	if (strings.Contains(lowerPath, "download") || lowerPath == "" || lowerPath == "/") && referer != "" {
		if refURL, err := url.Parse(referer); err == nil {
			refPath := refURL.Path
			refLower := strings.ToLower(refPath)
			if !strings.HasPrefix(refPath, "/api") && !strings.Contains(refLower, "download") {
				idxLast := strings.LastIndex(refPath, "/")
				if idxLast != -1 && len(refPath) > idxLast+1 {
					slug := refPath[idxLast+1:]
					if slug != "" {
						return fmt.Sprintf("%s (via page: %s)", slug, refPath)
					}
				}
			}
		}
	}

	idx := strings.LastIndex(path, "/")
	if idx != -1 && len(path) > idx+1 {
		return path[idx+1:]
	}
	return path
}

func realClientIP(r *http.Request) string {
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Original-Forwarded-For"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func generateSessionToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func sanitizeCookieHeader(s string) string {
	replacer := strings.NewReplacer("\r", "", "\n", "", "\x00", "", "\t", " ")
	return strings.TrimSpace(replacer.Replace(s))
}

func parseCookieFromDB(raw string) string {
	raw = sanitizeCookieHeader(raw)
	if raw == "" {
		return ""
	}

	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "[") {
		return raw
	}

	var cookies []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(trimmed), &cookies); err != nil {
		log.Printf("[COOKIE] ⚠️ Could not parse JSON cookie array: %v — using raw value", err)
		return raw
	}

	parts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		if c.Name != "" {
			parts = append(parts, c.Name+"="+c.Value)
		}
	}
	return strings.Join(parts, "; ")
}

func stripCloudflareCookies(cookieStr string) string {
	parts := strings.Split(cookieStr, "; ")
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		idx := strings.IndexByte(part, '=')
		if idx > 0 {
			name := strings.TrimSpace(part[:idx])
			lowerName := strings.ToLower(name)
			if lowerName == "__cf_bm" || lowerName == "_cfuvid" || lowerName == "cf_clearance" || lowerName == "__cfruid" || lowerName == "__cf_address" {
				continue
			}
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, "; ")
}

func mergeCookiesWithJar(base string) string {
	cookieJarMu.RLock()
	defer cookieJarMu.RUnlock()
	if len(cookieJar) == 0 {
		return base
	}
	cookies := make(map[string]string)
	parts := strings.Split(base, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		subparts := strings.SplitN(part, "=", 2)
		if len(subparts) == 2 {
			cookies[subparts[0]] = subparts[1]
		}
	}
	for name, value := range cookieJar {
		cookies[name] = value
	}
	var merged []string
	for k, v := range cookies {
		merged = append(merged, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(merged, "; ")
}

// updateCookieJar stores fresh short-lived Cloudflare cookies from upstream Set-Cookie headers.
func updateCookieJar(setCookieHeaders []string) {
	if len(setCookieHeaders) == 0 {
		return
	}
	cookieJarMu.Lock()
	defer cookieJarMu.Unlock()
	for _, header := range setCookieHeaders {
		parts := strings.SplitN(header, ";", 2)
		nameVal := strings.TrimSpace(parts[0])
		idx := strings.IndexByte(nameVal, '=')
		if idx <= 0 {
			continue
		}
		name := strings.TrimSpace(nameVal[:idx])
		val := nameVal[idx+1:]
		if name == "__cf_bm" || name == "_cfuvid" || name == "cf_clearance" {
			cookieJar[name] = val
			log.Printf("[COOKIEJAR] Updated live cookie: %s", name)
		}
	}
}

func loadProxyFromDB(webID int) *url.URL {
	if db == nil {
		return nil
	}
	var proxyStr string
	err := db.QueryRow("SELECT COALESCE(proxy, '') FROM ahrefs_websites WHERE id = ?", webID).Scan(&proxyStr)
	if err != nil || strings.TrimSpace(proxyStr) == "" {
		return nil
	}
	return lookupProxyByRef(proxyStr)
}

// parseProxyEndpoint builds a proxy URL using ahrefs_proxies.proxy_type (HTTP/SOCKS5/HTTPS).
func parseProxyEndpoint(endpoint, proxyType string) *url.URL {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil
	}
	if strings.Contains(endpoint, "://") {
		return parseProxyString(endpoint)
	}
	prefix := "http:"
	switch strings.ToUpper(strings.TrimSpace(proxyType)) {
	case "SOCKS5":
		prefix = "socks5:"
	case "HTTPS":
		prefix = "https:"
	}
	return parseProxyString(prefix + endpoint)
}

// lookupProxyByRef resolves ctrl-panel proxy by account/website reference (name or endpoint).
// DB lookup runs first so proxy_type (HTTP/SOCKS5/HTTPS) from ahrefs_proxies is respected.
func lookupProxyByRef(ref string) *url.URL {
	if db == nil {
		return nil
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}

	var endpoint, proxyType string
	err := db.QueryRow(
		"SELECT endpoint, proxy_type FROM ahrefs_proxies WHERE status = 'active' AND endpoint = ? LIMIT 1",
		ref,
	).Scan(&endpoint, &proxyType)
	if err == nil {
		return parseProxyEndpoint(endpoint, proxyType)
	}
	err = db.QueryRow(
		"SELECT endpoint, proxy_type FROM ahrefs_proxies WHERE status = 'active' AND name = ? LIMIT 1",
		ref,
	).Scan(&endpoint, &proxyType)
	if err == nil {
		return parseProxyEndpoint(endpoint, proxyType)
	}

	// Explicit URL or http:/socks5: prefix only — do not guess type for bare host:port:user:pass
	if strings.Contains(ref, "://") ||
		strings.HasPrefix(strings.ToLower(ref), "http:") ||
		strings.HasPrefix(strings.ToLower(ref), "socks5:") {
		return parseProxyString(ref)
	}
	return nil
}

func resolveOutboundProxy(cfg Config, webID int, accountProxy string) *url.URL {
	candidates := resolveOutboundProxyCandidates(cfg, webID, accountProxy)
	if len(candidates) == 0 {
		return nil
	}
	return candidates[0]
}

func getProxy(cfg Config, webID int) *url.URL {
	return resolveOutboundProxy(cfg, webID, "")
}

func setClientHintHeaders(req *http.Request, ua string) {
	req.Header.Del("Sec-Ch-Ua")
	req.Header.Del("Sec-Ch-Ua-Mobile")
	req.Header.Del("Sec-Ch-Ua-Platform")
	req.Header.Del("Sec-Ch-Ua-Model")
	req.Header.Del("Sec-Ch-Ua-Arch")
	req.Header.Del("Sec-Ch-Ua-Full-Version")
	req.Header.Del("Sec-Ch-Ua-Full-Version-List")
	req.Header.Del("Sec-Ch-Ua-Platform-Version")
	req.Header.Del("Sec-Ch-Ua-Bitness")

	if ua == "" {
		return
	}

	version := "133"
	platform := `"Windows"`
	if strings.Contains(ua, "Macintosh") {
		platform = `"macOS"`
	} else if strings.Contains(ua, "Android") {
		platform = `"Android"`
	} else if strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad") {
		platform = `"iOS"`
	} else if strings.Contains(ua, "Linux") {
		platform = `"Linux"`
	}

	req.Header.Set("Sec-Ch-Ua", fmt.Sprintf(`"Not(A:Brand";v="99", "Google Chrome";v="%s", "Chromium";v="%s"`, version, version))
	if platform == `"Android"` || platform == `"iOS"` {
		req.Header.Set("Sec-Ch-Ua-Mobile", "?1")
	} else {
		req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	}
	req.Header.Set("Sec-Ch-Ua-Platform", platform)
}

func handleAuthHandshake(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Username      string        `json:"username"`
		ProductIDsRaw []interface{} `json:"product_ids"`
		ClientIP      string        `json:"client_ip"`
		Timestamp     int64         `json:"timestamp"`
		Signature     string        `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Printf("[HANDSHAKE] Bad JSON payload: %v", err)
		http.Error(w, "Bad Request: Invalid JSON", http.StatusBadRequest)
		return
	}

	if payload.Username == "" || payload.ClientIP == "" || payload.Signature == "" {
		http.Error(w, "Bad Request: Missing required fields", http.StatusBadRequest)
		return
	}

	cfg := loadConfig()
	currentHost := r.Host
	currentScheme := "http"
	if r.TLS != nil {
		currentScheme = "https"
	} else if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		currentScheme = proto
	} else if cfg.PublicScheme != "" {
		currentScheme = cfg.PublicScheme
	}
	if currentHost == "" {
		currentHost = cfg.PublicHost
	}

	// Resolve website ID dynamically from Host header to support multi-domain
	requestWebsiteID := currentWebsiteID
	if db != nil {
		errW := db.QueryRow("SELECT id FROM ahrefs_websites WHERE domain = ?", currentHost).Scan(&requestWebsiteID)
		if errW != nil {
			log.Printf("[HANDSHAKE] ⚠️ Could not resolve website ID for host '%s': %v. Using fallback ID: %d", currentHost, errW, currentWebsiteID)
		}
	}

	// Validate signature using secret key from database (or config fallback)
	var dbSecretKey string
	if db != nil {
		err := db.QueryRow("SELECT secret_key FROM ahrefs_websites WHERE id = ?", requestWebsiteID).Scan(&dbSecretKey)
		if err != nil {
			dbSecretKey = "toolsmandi_envato_secret_xyz123"
		}
	} else {
		dbSecretKey = "toolsmandi_envato_secret_xyz123"
	}

	// HMAC-SHA256 signature check
	mac := hmac.New(sha256.New, []byte(dbSecretKey))
	mac.Write([]byte(fmt.Sprintf("%s:%d", payload.Username, payload.Timestamp)))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(payload.Signature), []byte(expectedSig)) {
		log.Printf("[HANDSHAKE] ❌ HMAC verification failed for user: %s", payload.Username)
		http.Error(w, "Forbidden: Invalid signature", http.StatusForbidden)
		return
	}

	// Reject requests older than 5 minutes
	if time.Now().Unix()-payload.Timestamp > 300 {
		log.Printf("[HANDSHAKE] ❌ Stale request rejected: %s", payload.Username)
		http.Error(w, "Forbidden: Request expired", http.StatusForbidden)
		return
	}

	if db == nil {
		http.Error(w, "Service Unavailable: DB Offline", http.StatusServiceUnavailable)
		return
	}

	// Generate One-Time Token (OTT)
	ottBytes := make([]byte, 32)
	if _, err := rand.Read(ottBytes); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	ott := hex.EncodeToString(ottBytes)
	expiresAt := time.Now().Add(2 * time.Minute)

	// Insert OTT into local database under the resolved requestWebsiteID
	_, err := db.Exec(
		"INSERT INTO ahrefs_tokens (website_id, token, username, client_ip, expires_at) VALUES (?, ?, ?, ?, ?)",
		requestWebsiteID, ott, payload.Username, payload.ClientIP, expiresAt,
	)
	if err != nil {
		log.Printf("[HANDSHAKE] ❌ DB Error inserting OTT: %v", err)
		http.Error(w, "Internal Database Error", http.StatusInternalServerError)
		return
	}

	// Return secure handshake redirect URL
	redirectURL := fmt.Sprintf("%s://%s/access?token=%s",
		currentScheme, currentHost, ott)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"redirect_url": redirectURL,
	})
}

func handleAccess(w http.ResponseWriter, r *http.Request) {
	ott := strings.TrimSpace(r.URL.Query().Get("token"))
	if ott == "" {
		renderAccessDeniedPage(w)
		return
	}

	cfgAccess := loadConfig()
	if db == nil || usesPanelAccountMode(cfgAccess) {
		serveEnvatoPanelAccess(w, r, "", ott)
		return
	}
	username := strings.TrimSpace(r.URL.Query().Get("user"))

	var storedUsername, storedIP string
	var expiresAt time.Time
	var tokenID, tokenWebsiteID int
	err := db.QueryRow(
		"SELECT id, website_id, username, client_ip, expires_at FROM ahrefs_tokens WHERE token = ?",
		ott,
	).Scan(&tokenID, &tokenWebsiteID, &storedUsername, &storedIP, &expiresAt)

	if err == sql.ErrNoRows {
		log.Printf("[ACCESS] ❌ OTT not found for token: %s", ott)
		renderAccessDeniedPage(w)
		return
	} else if err != nil {
		log.Printf("[ACCESS] DB error on OTT lookup: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Dynamically synchronize the website_id from the authorized token
	currentWebsiteID = tokenWebsiteID

	// Delete OTT
	_, _ = db.Exec("DELETE FROM ahrefs_tokens WHERE id = ?", tokenID)

	if time.Now().After(expiresAt) {
		log.Printf("[ACCESS] ❌ Expired OTT for user: %s (expired at %v)", username, expiresAt)
		renderAccessDeniedPage(w)
		return
	}

	if username != "" && storedUsername != username {
		log.Printf("[ACCESS] ❌ Username mismatch: token was for '%s', got '%s'", storedUsername, username)
		renderAccessDeniedPage(w)
		return
	}
	username = storedUsername

	var dbStatus string
	err = db.QueryRow("SELECT status FROM ahrefs_users WHERE username = ? AND website_id = ?", username, currentWebsiteID).Scan(&dbStatus)
	if err == sql.ErrNoRows {
		// Fetch website's configured default limits
		var defCreditLimit int
		if dbErr := db.QueryRow("SELECT COALESCE(default_credit_limit, 50) FROM ahrefs_websites WHERE id = ?", currentWebsiteID).Scan(&defCreditLimit); dbErr != nil {
			defCreditLimit = 50
		}
		_, _ = db.Exec("INSERT INTO ahrefs_users (username, website_id, credit_limit) VALUES (?, ?, ?)", username, currentWebsiteID, defCreditLimit)
	} else if dbStatus == "suspended" {
		http.Error(w, "Account Suspended. Contact support.", http.StatusForbidden)
		return
	}

	sessionToken := generateSessionToken()
	sessionDuration := 30
	var dbSessionDuration int
	if errDb := db.QueryRow("SELECT session_duration FROM ahrefs_websites WHERE id = ?", currentWebsiteID).Scan(&dbSessionDuration); errDb == nil && dbSessionDuration > 0 {
		sessionDuration = dbSessionDuration
	}
	sessionExpires := time.Now().Add(time.Duration(sessionDuration) * time.Minute)

	// Single session enforcement
	_, _ = db.Exec("DELETE FROM ahrefs_sessions WHERE username = ? AND website_id = ?", username, currentWebsiteID)

	var assignedAccountID sql.NullInt64
	if initialAcc, errAcc := selectActiveAccount(currentWebsiteID); errAcc == nil {
		assignedAccountID.Int64 = int64(initialAcc.ID)
		assignedAccountID.Valid = true
	}

	_, err = db.Exec(
		"INSERT INTO ahrefs_sessions (session_token, username, client_ip, expires_at, website_id, assigned_account_id) VALUES (?, ?, ?, ?, ?, ?)",
		sessionToken, username, realClientIP(r), sessionExpires, currentWebsiteID, assignedAccountID,
	)
	if err != nil {
		log.Printf("[ACCESS] ❌ DB Error inserting session: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	userAgentStr := r.Header.Get("User-Agent")
	_, _ = db.Exec("INSERT INTO ahrefs_login_logs (website_id, username, client_ip, user_agent) VALUES (?, ?, ?, ?)", currentWebsiteID, username, realClientIP(r), userAgentStr)

	cfg := loadConfig()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionToken,
		Path:     "/",
		Expires:  sessionExpires,
		HttpOnly: true,
		Secure:   cfg.PublicScheme == "https",
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleUserLimits(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Cookie")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	cookie, errC := readSessionCookie(r)
	if errC != nil || cookie.Value == "" {
		log.Printf("[LIMITS-API] ⚠️ Request unauthorized (no session cookie): %v", errC)
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "unauthorized", "message": "no active session"})
		return
	}

	if db == nil {
		username := envatoSessionUsername(r)
		if username == "" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "unauthorized", "message": "session invalid"})
			return
		}
		view := envatoPanelLimitView(username)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"show_limit":   view.show,
			"username":     username,
			"credit_limit": view.limit,
			"credit_used":  view.used,
			"credit_label": view.label,
		})
		return
	}

	var currentUser string
	var sessionWebsiteID int
	var assignedAccountID sql.NullInt64
	err := db.QueryRow("SELECT username, website_id, assigned_account_id FROM ahrefs_sessions WHERE session_token = ?", cookie.Value).Scan(&currentUser, &sessionWebsiteID, &assignedAccountID)
	if err != nil {
		log.Printf("[LIMITS-API] ⚠️ Invalid session token '%s': %v", cookie.Value, err)
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "unauthorized", "message": "session invalid"})
		return
	}

	var creditLimit int
	err = db.QueryRow("SELECT credit_limit FROM ahrefs_users WHERE username = ? AND website_id = ?", currentUser, sessionWebsiteID).Scan(&creditLimit)
	if err == sql.ErrNoRows {
		err = db.QueryRow("SELECT COALESCE(default_credit_limit, 20) FROM ahrefs_websites WHERE id = ?", sessionWebsiteID).Scan(&creditLimit)
		if err != nil {
			creditLimit = 20
		}
	}

	var creditUsed int
	_ = db.QueryRow("SELECT COUNT(*) FROM ahrefs_credit_logs WHERE username = ? AND website_id = ? AND DATE(timestamp) = CURDATE()", currentUser, sessionWebsiteID).Scan(&creditUsed)

	showLimit := true
	if assignedAccountID.Valid {
		var showLimitVal int
		err = db.QueryRow("SELECT show_limit FROM ahrefs_accounts WHERE id = ? AND website_id = ?", assignedAccountID.Int64, sessionWebsiteID).Scan(&showLimitVal)
		if err == nil {
			showLimit = (showLimitVal == 1)
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"show_limit":   showLimit,
		"username":     currentUser,
		"credit_limit": creditLimit,
		"credit_used":  creditUsed,
	})
}

func handleTriggerSwap(w http.ResponseWriter, r *http.Request) {
	cookie, errC := readSessionCookie(r)
	if errC != nil || cookie.Value == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if db == nil {
		ok, name := envatoPanelSwitch(cookie.Value, "manual swap")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if ok {
			fmt.Fprint(w, renderAccountSwitchPage(name))
		} else {
			fmt.Fprint(w, renderWaitingForAccountPage())
		}
		return
	}

	var username string
	var expiresAt time.Time
	var sessionWebsiteID int
	errS := db.QueryRow("SELECT username, expires_at, website_id FROM ahrefs_sessions WHERE session_token = ?", cookie.Value).Scan(&username, &expiresAt, &sessionWebsiteID)
	if errS != nil || time.Now().After(expiresAt) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Retrieve current swap count from cookie or request header
	swapCount := 0
	if swapCookie, errS := r.Cookie("envato_swap_count"); errS == nil {
		fmt.Sscanf(swapCookie.Value, "%d", &swapCount)
	}

	if swapCount >= 3 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Loop protection active",
		})
		return
	}

	var currentAssignedID sql.NullInt64
	_ = db.QueryRow("SELECT assigned_account_id FROM ahrefs_sessions WHERE session_token = ?", cookie.Value).Scan(&currentAssignedID)

	nextAcc, errSwap := swapActiveAccount(cookie.Value, currentAssignedID.Int64)
	if errSwap != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   errSwap.Error(),
		})
		return
	}

	log.Printf("[API-SWAP] 🔄 Swapped account to %d (%s) for session %s via client-side trigger", nextAcc.ID, nextAcc.Name, cookie.Value)

	newSwapCountCookie := &http.Cookie{
		Name:     "envato_swap_count",
		Value:    fmt.Sprintf("%d", swapCount+1),
		Path:     "/",
		MaxAge:   120,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, newSwapCountCookie)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"account": nextAcc.Name,
	})
}

func handleProxyRequest(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()

	// Disk CDN: /assets-proxy static + /__resized__ thumbs (≤30MB thumb budget). Never video.
	cdnKey := cdnCacheKey(r)
	if serveCachedCDN(w, r) {
		return
	}
	if cdnKey != "" {
		defer completeCDNFlight(cdnKey)
	}

	// 1. Resolve website ID from Host header
	websiteID := currentWebsiteID
	if db != nil {
		_ = db.QueryRow("SELECT id FROM ahrefs_websites WHERE domain = ?", r.Host).Scan(&websiteID)
	}

	var resolvedCookie, resolvedUserAgent, resolvedProxy string
	var accID int
	var staticCookie string
	var currentUser string
	var sessionCookie *http.Cookie
	var errC error
	isStaticMode := false

	if db == nil {
		if !cfg.BypassAuth && !isStaticPath(r.URL.Path) && !envatoPanelSessionOK(r) {
			renderAccessDeniedPage(w)
			return
		}
		if !cfg.BypassAuth && !isStaticPath(r.URL.Path) && envatoRejectDevice(w, r) {
			return
		}
		if isDocumentNavigation(r) && envatoSignInRequest(r) {
			token := ""
			if c, err := readSessionCookie(r); err == nil {
				token = c.Value
			}
			swapCount := 0
			if c, err := r.Cookie("envato_swap_count"); err == nil {
				fmt.Sscanf(c.Value, "%d", &swapCount)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			if token != "" && swapCount < 3 {
				if ok, name := envatoPanelSwitch(token, "sign_in path"); ok {
					http.SetCookie(w, &http.Cookie{
						Name: "envato_swap_count", Value: fmt.Sprintf("%d", swapCount+1), Path: "/", MaxAge: 120, HttpOnly: true, SameSite: http.SameSiteLaxMode,
					})
					fmt.Fprint(w, renderAccountSwitchPage(name))
					return
				}
			}
			fmt.Fprint(w, renderWaitingForAccountPage())
			return
		}
		if isDocumentNavigation(r) {
			refreshEnvatoSessionCookie(w, r)
		}
		resolvedCookie, resolvedUserAgent, resolvedProxy = envatoPanelAssets(cfg, func() string {
			if c, err := readSessionCookie(r); err == nil {
				sessionCookie = c
				return c.Value
			}
			return ""
		}())
		isStaticMode = true
	} else {
		if cookieData, err := os.ReadFile("cookie.txt"); err == nil {
			staticCookie = string(cookieData)
		}

		// Static mode is active ONLY if cookie.txt exists, or if database is offline.
		isStaticMode = staticCookie != "" || db == nil

		if isStaticMode {
			if staticCookie != "" {
				resolvedCookie = staticCookie
			} else if cfg.CookiePath != "" {
				resolvedCookie = loadCookies(cfg.CookiePath)
			}
			resolvedUserAgent = cfg.UserAgent
			resolvedProxy = cfg.Proxy
		} else if db != nil {
			sessionCookie, errC = readSessionCookie(r)
			if errC == nil && sessionCookie != nil && sessionCookie.Value != "" {
				var sessionWebsiteID int
				var assignedAccountID sql.NullInt64
				errSession := db.QueryRow("SELECT website_id, assigned_account_id FROM ahrefs_sessions WHERE session_token = ?", sessionCookie.Value).Scan(&sessionWebsiteID, &assignedAccountID)

				if errSession == nil {
					websiteID = sessionWebsiteID

					if assignedAccountID.Valid {
						_ = db.QueryRow("SELECT id, cookie, user_agent, COALESCE(proxy, '') FROM ahrefs_accounts WHERE id = ? AND website_id = ? AND status = 'active'", assignedAccountID.Int64, websiteID).Scan(&accID, &resolvedCookie, &resolvedUserAgent, &resolvedProxy)
					}

					if accID == 0 {
						acc, errAcc := selectActiveAccount(websiteID)
						if errAcc == nil {
							accID = acc.ID
							resolvedCookie = acc.Cookie
							resolvedUserAgent = acc.UserAgent
							resolvedProxy = acc.Proxy
							_, _ = db.Exec("UPDATE ahrefs_sessions SET assigned_account_id = ? WHERE session_token = ? AND website_id = ?", accID, sessionCookie.Value, websiteID)
						}
					}
				}
			}

			if resolvedCookie == "" {
				acc, errAcc := selectActiveAccount(websiteID)
				if errAcc == nil {
					accID = acc.ID
					resolvedCookie = acc.Cookie
					resolvedUserAgent = acc.UserAgent
					resolvedProxy = acc.Proxy
				}
			}
		}
	}

	// Normalize Cookie & Merge with live Cloudflare cookies in cookieJar
	if resolvedCookie != "" {
		resolvedCookie = parseCookieFromDB(resolvedCookie)
		resolvedCookie = stripCloudflareCookies(resolvedCookie)
		resolvedCookie = mergeCookiesWithJar(resolvedCookie)
	}

	// Normalize User-Agent
	resolvedUserAgent = strings.TrimSpace(resolvedUserAgent)
	if resolvedUserAgent == "" {
		resolvedUserAgent = cfg.UserAgent
	}

	// 2. Session verification & active sliding window (skip for static assets or if in static testing mode)
	isStatic := isStaticPath(r.URL.Path)
	if !isStatic && !isStaticMode && db != nil {
		var isAuthed bool
		if errC == nil && sessionCookie != nil && sessionCookie.Value != "" {
			var expiresAt time.Time
			var sessionWebsiteID int
			errS := db.QueryRow("SELECT username, expires_at, website_id FROM ahrefs_sessions WHERE session_token = ?", sessionCookie.Value).Scan(&currentUser, &expiresAt, &sessionWebsiteID)
			if errS == nil && time.Now().Before(expiresAt) {
				isAuthed = true
				websiteID = sessionWebsiteID
			}
		}

		// Security block: require authentication
		if !isAuthed {
			renderAccessDeniedPage(w)
			return
		}

		// Intercept logout/sign_out request to swap accounts
		if isAuthed && isLoginOrLogoutPath(r.URL.Path) {
			if sessionCookie != nil && sessionCookie.Value != "" {
				swapCount := 0
				if swapCookie, errS := r.Cookie("envato_swap_count"); errS == nil {
					fmt.Sscanf(swapCookie.Value, "%d", &swapCount)
				}

				if swapCount < 3 {
					var currentAssignedID sql.NullInt64
					_ = db.QueryRow("SELECT assigned_account_id FROM ahrefs_sessions WHERE session_token = ?", sessionCookie.Value).Scan(&currentAssignedID)

					_, errSwap := swapActiveAccount(sessionCookie.Value, currentAssignedID.Int64)
					if errSwap == nil {
						newSwapCountCookie := &http.Cookie{
							Name:     "envato_swap_count",
							Value:    fmt.Sprintf("%d", swapCount+1),
							Path:     "/",
							MaxAge:   10,
							HttpOnly: true,
						}
						http.SetCookie(w, newSwapCountCookie)

						htmlContent := renderSwappingPage(defaultLandingPath)
						w.Header().Set("Content-Type", "text/html; charset=utf-8")
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(htmlContent))
						return
					}
				}
			}
		}

		// Limit check for premium downloads
		if isDownloadRequest(r.URL.Path) && currentUser != "" {
			// Expire custom limits that passed their expiration date
			_, _ = db.Exec(`
				UPDATE ahrefs_users u
				JOIN ahrefs_websites w ON u.website_id = w.id
				SET u.credit_limit = COALESCE(w.default_credit_limit, 20),
					u.custom_limit_expire_at = NULL
				WHERE u.custom_limit_expire_at IS NOT NULL AND u.custom_limit_expire_at < NOW()
			`)

			// Fetch user status and limit
			var downloadLimit int
			var userStatus string
			err := db.QueryRow("SELECT credit_limit, status FROM ahrefs_users WHERE username = ? AND website_id = ?", currentUser, websiteID).Scan(&downloadLimit, &userStatus)
			if err == sql.ErrNoRows {
				err = db.QueryRow("SELECT COALESCE(default_credit_limit, 20) FROM ahrefs_websites WHERE id = ?", websiteID).Scan(&downloadLimit)
				if err != nil {
					downloadLimit = 20
				}
				userStatus = "active"
			} else if err != nil {
				log.Printf("[LIMITS] Error querying user limits for '%s': %v", currentUser, err)
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}

			if userStatus == "suspended" {
				http.Error(w, "Account Suspended. Contact support.", http.StatusForbidden)
				return
			}

			// Count today's download logs
			var usedDownloadsToday int
			err = db.QueryRow("SELECT COUNT(*) FROM ahrefs_credit_logs WHERE username = ? AND website_id = ? AND DATE(timestamp) = CURDATE()", currentUser, websiteID).Scan(&usedDownloadsToday)

			if usedDownloadsToday >= downloadLimit {
				log.Printf("[LIMITS] 🚫 Blocked download for user '%s' — limit reached (%d/%d)", currentUser, usedDownloadsToday, downloadLimit)

				isHTMLPage := strings.Contains(r.Header.Get("Accept"), "text/html")
				if !isHTMLPage {
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprintf(w, `{"error":"daily_download_limit_reached","message":"Your daily premium download limit has been reached. Please try again tomorrow.","code":403}`)
				} else {
					renderLimitReachedPage(w, currentUser, usedDownloadsToday, downloadLimit)
				}
				return
			}

			// Dedup hits: one logical item download should be counted once even if
			// Envato triggers both /download.data and a signed __domain__ download URL.
			dedupID := downloadIdentityKey(r)
			if dedupID == "" {
				dedupID = "path:" + strings.ToLower(r.URL.Path)
			}
			dedupKey := fmt.Sprintf("%s:%d:%s", currentUser, websiteID, dedupID)
			shouldCharge := true
			if lastHit, ok := downloadHitCache.Load(dedupKey); ok {
				if time.Since(lastHit.(time.Time)) < 2*time.Minute {
					shouldCharge = false
				}
			}

			if shouldCharge {
				filenameLogged := extractItemName(r.URL.Path, r.URL, r.Referer())
				_, err = db.Exec("INSERT INTO ahrefs_credit_logs (username, website_id, endpoint) VALUES (?, ?, ?)", currentUser, websiteID, filenameLogged)
				if err != nil {
					log.Printf("[LIMITS] Failed logging credit hit for '%s' (Item: %s): %v", currentUser, filenameLogged, err)
				} else {
					downloadHitCache.Store(dedupKey, time.Now())
				}
			}
		}
	}

	// 3. Outbound proxy from ctrl panel DB (account → website → pool → config fallback)
	var proxyCandidates []*url.URL
	directDial := false
	if db == nil {
		proxyCandidates = envatoPanelProxyCandidates(resolvedProxy)
		directDial = len(proxyCandidates) == 0
	} else {
		proxyCandidates = resolveOutboundProxyCandidates(cfg, websiteID, resolvedProxy)
	}
	if len(proxyCandidates) == 0 {
		log.Printf("[PROXY] ⚠️ No outbound proxy — add in ctrl panel → Proxies, or config.json \"proxy\"")
	} else {
		px := proxyCandidates[0]
		log.Printf("[PROXY] Using %s://%s (+%d fallback(s))", px.Scheme, px.Host, len(proxyCandidates)-1)
	}

	ctx := context.WithValue(r.Context(), proxyContextKey, proxyCandidates)
	if directDial {
		ctx = context.WithValue(ctx, directDialKey, true)
	}
	if db == nil {
		user := envatoSessionUsername(r)
		if user != "" {
			ctx = context.WithValue(ctx, envatoUserNameKey, user)
		}
		view := envatoPanelLimitView(user)
		overLimit := user != "" && view.limit >= 0 && view.used >= view.limit
		if overLimit && (r.URL.Path == "/download.data" || strings.HasSuffix(r.URL.Path, "/download.data")) {
			renderLimitReachedPage(w, user, view.used, view.limit)
			return
		}
		chargeName := ""
		if isEnvatoLicensePath(r.URL.Path) {
			chargeName = envatoItemLabel(r)
		}
		_, _, fileDownload := envatoIncomingDownload(r.URL.Path)
		if chargeName != "" || fileDownload {
			if overLimit {
				renderLimitReachedPage(w, user, view.used, view.limit)
				return
			}
			ctx = context.WithValue(ctx, envatoDownloadUserKey, user)
			if chargeName != "" {
				ctx = context.WithValue(ctx, envatoChargeNameKey, chargeName)
			}
		}
	}
	r = r.WithContext(ctx)

	// Set header fields to pass state to Director and ModifyResponse callback
	r.Header.Set("X-Resolved-Cookie", resolvedCookie)
	r.Header.Set("X-Resolved-UA", resolvedUserAgent)
	if sessionCookie != nil {
		r.Header.Set("X-Client-Session-Token", sessionCookie.Value)
	}
	swapCount := 0
	if swapCookie, errSw := r.Cookie("envato_swap_count"); errSw == nil {
		fmt.Sscanf(swapCookie.Value, "%d", &swapCount)
	}
	r.Header.Set("X-Client-Swap-Count", fmt.Sprintf("%d", swapCount))

	reverseProxy.ServeHTTP(w, r)
}

func main() {
	cfg := loadConfig()

	if usesPanelAccountMode(cfg) {
		if _, err := openEnvatoPanel(); err != nil {
			log.Fatalf("[PANEL] panel_db set but open failed: %v", err)
		}
		log.Printf("[DB] Panel mode (ChatGPT-pattern). MySQL skipped. panel_db=%s", cfg.PanelDB)
		db = nil
	} else if _, err := openEnvatoPanel(); err == nil {
		log.Printf("[DB] Using panel database. MySQL is not used.")
		db = nil
	} else if cfg.MySQLDB != "" {
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=Asia%%2FKolkata",
			cfg.MySQLUser, cfg.MySQLPassword, cfg.MySQLHost, cfg.MySQLPort, cfg.MySQLDB)
		var err error
		db, err = sql.Open("mysql", dsn)
		if err != nil {
			log.Printf("[DB] ⚠️ MySQL Connection failed to initialize: %v", err)
		} else {
			db.SetMaxOpenConns(25)
			db.SetMaxIdleConns(5)
			db.SetConnMaxLifetime(5 * time.Minute)
			if err := db.Ping(); err != nil {
				log.Printf("[DB] ⚠️ Could not connect to MySQL: %v. Database mode is offline.", err)
				db.Close()
				db = nil
			} else {
				log.Printf("[DB] Connected to MySQL successfully! Database: %s ✅", cfg.MySQLDB)
			}
		}
	} else {
		log.Printf("[DB] MySQL configuration not provided. Running in Standalone Mode.")
	}

	if cfg.WebsiteID != 0 {
		currentWebsiteID = cfg.WebsiteID
	}

	candidates := resolveOutboundProxyCandidates(cfg, currentWebsiteID, "")
	if len(candidates) == 0 {
		log.Printf("[PROXY] ⚠️  NO outbound proxy — add in config.json \"proxy\" or proxy.txt")
	} else {
		px := candidates[0]
		log.Printf("[PROXY] Outbound proxy active: %s://%s ✅ (required for Cloudflare bypass)", px.Scheme, px.Host)
	}

	targetUrl, err := url.Parse(cfg.TargetURL)
	if err != nil {
		log.Fatalf("[FATAL] Invalid target URL: %v", err)
	}

	// Create reverse proxy
	reverseProxy = httputil.NewSingleHostReverseProxy(targetUrl)

	// Custom HTTP Client transport with uTLS and outbound proxy
	reverseProxy.Transport = &CustomRoundTripper{
		Transport: &http.Transport{
			ForceAttemptHTTP2: false,
			DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				if ctx.Value(directDialKey) == true {
					return dialChrome(ctx, addr, nil)
				}
				candidates := proxyCandidatesFromContext(ctx)
				if len(candidates) == 0 {
					cfgLocal := loadConfig()
					candidates = resolveOutboundProxyCandidates(cfgLocal, currentWebsiteID, "")
				}
				return dialChrome(ctx, addr, candidates)
			},
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   20 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	// Proxy Director: Rewrites outgoing requests
	reverseProxy.Director = func(req *http.Request) {
		cfgLocal := loadConfig()

		targetHost, targetScheme := resolveTargetHost(req.URL.Path)
		req.URL.Path = cleanRequestPath(req.URL.Path)

		resolvedCookie := req.Header.Get("X-Resolved-Cookie")
		resolvedUA := req.Header.Get("X-Resolved-UA")

		req.Header.Del("X-Resolved-Cookie")
		req.Header.Del("X-Resolved-UA")
		req.Header.Del("X-Device-Fp")
		req.Header.Del("X-Device-Proof")
		req.Header.Del("X-Client-Session-Token")
		req.Header.Del("X-Client-Swap-Count")

		// Detect public host & scheme dynamically
		publicHost := req.Host
		if publicHost == "" {
			publicHost = cfgLocal.PublicHost
		}
		publicScheme := cfgLocal.PublicScheme
		if strings.Contains(publicHost, "localhost") || strings.Contains(publicHost, "127.0.0.1") {
			publicScheme = "http"
		} else if req.TLS != nil {
			publicScheme = "https"
		} else if proto := req.Header.Get("X-Forwarded-Proto"); proto != "" {
			publicScheme = proto
		}

		// Store proxy parameters in headers for response mod later
		req.Header.Set("X-Proxy-Host", publicHost)
		req.Header.Set("X-Proxy-Scheme", publicScheme)
		req.Header.Set("X-Resolved-Target", targetHost)

		req.Header.Set("Host", targetHost)
		req.Host = targetHost
		req.URL.Host = targetHost
		req.URL.Scheme = targetScheme

		// Spoof User-Agent
		uaToUse := resolvedUA
		if uaToUse == "" {
			uaToUse = cfgLocal.UserAgent
		}

		if uaToUse != "" {
			req.Header.Set("User-Agent", uaToUse)
			setClientHintHeaders(req, uaToUse)
		}
		req.Header.Set("Accept-Encoding", "gzip")
		if !strings.HasSuffix(req.URL.Path, ".data") {
			req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
		}
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Upgrade-Insecure-Requests", "1")
		req.Header.Set("Cache-Control", "no-cache")
		req.Header.Set("Pragma", "no-cache")
		req.Header.Set("Origin", targetScheme+"://"+targetHost)

		// Remove tracing / source IP headers
		req.Header.Del("X-Forwarded-For")
		req.Header.Del("X-Real-IP")
		req.Header.Del("X-Forwarded-Host")
		req.Header.Del("X-Forwarded-Proto")

		// Envato subdomains check for cookie injection
		isEnvatoDomain := targetHost == "app.envato.com" ||
			targetHost == "elements.envato.com" ||
			targetHost == "api.envato.com" ||
			targetHost == "account.envato.com" ||
			strings.HasSuffix(targetHost, ".envato.com")

		if isEnvatoDomain && resolvedCookie != "" {
			req.Header.Set("Cookie", resolvedCookie)
		} else {
			req.Header.Del("Cookie")
		}

		// Rewrite Referer and Origin headers
		if ref := req.Header.Get("Referer"); ref != "" {
			ref = strings.ReplaceAll(ref, publicHost+"/elements-proxy", "https://elements.envato.com")
			ref = strings.ReplaceAll(ref, publicHost+"/api-proxy", "https://api.envato.com")
			ref = strings.ReplaceAll(ref, publicHost+"/account-proxy", "https://account.envato.com")
			ref = strings.ReplaceAll(ref, publicHost+"/assets-proxy", "https://assets.envato.com")
			ref = strings.ReplaceAll(ref, publicHost, targetHost)
			ref = strings.ReplaceAll(ref, publicScheme+"://", targetScheme+"://")
			req.Header.Set("Referer", ref)
		}
		if origin := req.Header.Get("Origin"); origin != "" {
			origin = strings.ReplaceAll(origin, publicHost, targetHost)
			origin = strings.ReplaceAll(origin, publicScheme+"://", targetScheme+"://")
			req.Header.Set("Origin", origin)
		}

		// Replace query parameters containing proxy URL
		rawQuery := req.URL.RawQuery
		if rawQuery != "" {
			encodedProxy := url.QueryEscape(publicScheme + "://" + publicHost)
			encodedTarget := url.QueryEscape(targetScheme + "://" + targetHost)
			rawQuery = strings.ReplaceAll(rawQuery, encodedProxy, encodedTarget)
			rawQuery = strings.ReplaceAll(rawQuery, publicHost, targetHost)
			req.URL.RawQuery = rawQuery
		}

		pxHost := ""
		pxScheme := ""
		if candidates := proxyCandidatesFromContext(req.Context()); len(candidates) > 0 {
			pxScheme = candidates[0].Scheme
			pxHost = candidates[0].Host
		}
		cookieLen := len(req.Header.Get("Cookie"))
		log.Printf("[REQ] %s %s%s (Target: %s) (Proxy: %s://%s) (CookieLen: %d)", req.Method, req.Host, req.URL.Path, targetHost, pxScheme, pxHost, cookieLen)
	}

	// ModifyResponse: Rewrites response headers and body content
	reverseProxy.ModifyResponse = func(resp *http.Response) error {
		cfgLocal := loadConfig()

		reqPath := ""
		reqHost := ""
		if resp.Request != nil {
			reqPath = resp.Request.URL.Path
			reqHost = resp.Request.URL.Host
		}

		proxyHost := ""
		proxyScheme := ""
		if resp.Request != nil {
			proxyHost = resp.Request.Header.Get("X-Proxy-Host")
			proxyScheme = resp.Request.Header.Get("X-Proxy-Scheme")
		}
		if proxyHost == "" {
			proxyHost = cfgLocal.PublicHost
		}
		if proxyScheme == "" {
			proxyScheme = cfgLocal.PublicScheme
		}
		proxySchemeHost := proxyScheme + "://" + proxyHost

		log.Printf("[RESP] %s%s | Status: %d | Location: %s | CD=%q", reqHost, reqPath, resp.StatusCode, resp.Header.Get("Location"), resp.Header.Get("Content-Disposition"))
		if resp.StatusCode == 200 || resp.StatusCode == 206 {
			if resp.Request != nil {
				if user, ok := resp.Request.Context().Value(envatoDownloadUserKey).(string); ok && user != "" {
					chargeName, _ := resp.Request.Context().Value(envatoChargeNameKey).(string)
					referer := resp.Request.Referer()
					ct := strings.ToLower(resp.Header.Get("Content-Type"))
					isJSON := strings.Contains(ct, "json") || strings.Contains(ct, "javascript")
					isHTML := strings.Contains(ct, "text/html")
					licenseHit := isEnvatoLicensePath(reqPath)

					name := bestEnvatoFileName(reqPath, resp.Request.URL, referer, resp.Header.Get("Content-Disposition"), chargeName)
					if jsonName := peekEnvatoDownloadJSONFilename(resp); jsonName != "" {
						if isChargeableEnvatoFileName(jsonName) || name == "" {
							name = jsonName
						}
					}
					dedupe := chargeName
					if dedupe == "" {
						dedupe = downloadIdentityKey(resp.Request)
					}
					if dedupe == "" {
						dedupe = name
					}

					// Bill on successful /download.data (license JSON) OR real CDN file bytes.
					// Photo/CDN URLs often leave the proxy (absolute elements CDN) so waiting
					// only for file bytes left the widget at 0/10 after a completed download.
					previewCDN := envatoPreviewHost(reqHost) || envatoPreviewPath(reqPath)
					if licenseHit {
						display := name
						if display == "" {
							display = chargeName
						}
						if display == "" {
							display = dedupe
						}
						if display != "" {
							recordEnvatoDownload(user, display, dedupe)
						} else {
							log.Printf("[DOWNLOAD] licensed but empty label user=%s path=%s", user, reqPath)
						}
					} else if !isHTML && !isJSON && !previewCDN && isChargeableEnvatoFileName(name) {
						display := name
						if display != "" {
							recordEnvatoDownload(user, display, dedupe)
						} else {
							log.Printf("[DOWNLOAD] no display name user=%s path=%s ct=%s", user, reqPath, ct)
						}
					} else if !licenseHit {
						log.Printf("[DOWNLOAD] wait file user=%s name=%q ct=%s path=%s", user, name, ct, reqPath)
					}
					if !isJSON && !isHTML && isChargeableEnvatoFileName(name) {
						cur := filenameFromDisposition(resp.Header.Get("Content-Disposition"))
						if !isChargeableEnvatoFileName(cur) {
							safe := sanitizeContentDispositionFilename(name)
							resp.Header.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safe))
							log.Printf("[DOWNLOAD] set Content-Disposition filename=%s path=%s", safe, reqPath)
						}
					}
				}
			}
		}
		if resp.StatusCode == 403 {
			log.Printf("[CF-BLOCK] Cloudflare block on %s%s — check config.json proxy or ctrl account proxy", reqHost, reqPath)
		}

		// Remove Framing Restrictions
		resp.Header.Del("X-Frame-Options")
		resp.Header.Del("Content-Security-Policy")
		resp.Header.Del("Content-Security-Policy-Report-Only")
		resp.Header.Del("X-Content-Security-Policy")
		resp.Header.Del("Strict-Transport-Security")

		// Strip Secure & Domain flags from Set-Cookie so the local browser accepts them
		if setCookies := resp.Header["Set-Cookie"]; len(setCookies) > 0 {
			// Capture fresh Cloudflare cookies for next upstream request (including 403 block pages)
			if resp.StatusCode == 200 || resp.StatusCode == 403 || (resp.StatusCode >= 300 && resp.StatusCode < 400) {
				updateCookieJar(setCookies)
			}
			var updatedCookies []string
			for _, sc := range setCookies {
				parts := strings.Split(sc, ";")
				var newParts []string
				for _, part := range parts {
					trimmed := strings.TrimSpace(part)
					lower := strings.ToLower(trimmed)
					if strings.HasPrefix(lower, "domain=") {
						continue
					}
					if lower == "secure" {
						continue
					}
					newParts = append(newParts, part)
				}
				updatedCookies = append(updatedCookies, strings.Join(newParts, "; "))
			}
			resp.Header["Set-Cookie"] = updatedCookies
		}

		// Rewrite redirect headers
		if loc := resp.Header.Get("Location"); loc != "" {
			// Check if Location points to Envato's login / SSO forms
			isSignInRedirect := strings.Contains(strings.ToLower(loc), "sign_in") ||
				strings.Contains(strings.ToLower(loc), "login") ||
				strings.Contains(strings.ToLower(loc), "auth")

			if isSignInRedirect && resp.Request != nil {
				sessionToken := resp.Request.Header.Get("X-Client-Session-Token")
				if sessionToken != "" {
					swapCount := 0
					if swapCountStr := resp.Request.Header.Get("X-Client-Swap-Count"); swapCountStr != "" {
						fmt.Sscanf(swapCountStr, "%d", &swapCount)
					}
					if swapCount < 3 {
						switched := false
						nextName := ""
						if db == nil {
							switched, nextName = envatoPanelSwitch(sessionToken, "login redirect")
						} else {
							var currentAssignedID sql.NullInt64
							_ = db.QueryRow("SELECT assigned_account_id FROM ahrefs_sessions WHERE session_token = ?", sessionToken).Scan(&currentAssignedID)
							if nextAcc, errSwap := swapActiveAccount(sessionToken, currentAssignedID.Int64); errSwap == nil {
								log.Printf("[SWAP] Redirect to login detected. Swapping to account: %d (%s)", nextAcc.ID, nextAcc.Name)
								switched = true
								nextName = nextAcc.Name
							}
						}
						htmlContent := renderWaitingForAccountPage()
						if switched {
							resp.Header.Add("Set-Cookie", (&http.Cookie{
								Name: "envato_swap_count", Value: fmt.Sprintf("%d", swapCount+1), Path: "/", MaxAge: 120, HttpOnly: true, SameSite: http.SameSiteLaxMode,
							}).String())
							htmlContent = renderAccountSwitchPage(nextName)
						}
						resp.StatusCode = http.StatusOK
						resp.Header.Set("Content-Type", "text/html; charset=utf-8")
						resp.Body = io.NopCloser(strings.NewReader(htmlContent))
						resp.ContentLength = int64(len(htmlContent))
						resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(htmlContent)))
						resp.Header.Del("Content-Encoding")
						resp.Header.Del("Location")
						return nil
					}
					htmlContent := renderWaitingForAccountPage()
					resp.StatusCode = http.StatusOK
					resp.Header.Set("Content-Type", "text/html; charset=utf-8")
					resp.Body = io.NopCloser(strings.NewReader(htmlContent))
					resp.ContentLength = int64(len(htmlContent))
					resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(htmlContent)))
					resp.Header.Del("Content-Encoding")
					resp.Header.Del("Location")
					return nil
				}
			}

			loc = strings.ReplaceAll(loc, "https://elements.envato.com", proxySchemeHost+"/elements-proxy")
			loc = strings.ReplaceAll(loc, "https://app.envato.com", proxySchemeHost)
			loc = strings.ReplaceAll(loc, "https://api.envato.com", proxySchemeHost+"/api-proxy")
			loc = strings.ReplaceAll(loc, "https://account.envato.com", proxySchemeHost+"/account-proxy")
			loc = strings.ReplaceAll(loc, "https://assets.envato.com", proxySchemeHost+"/assets-proxy")
			loc = strings.ReplaceAll(loc, "https://assets.elements.envato.com", proxySchemeHost+"/assets-proxy")

			resp.Header.Set("Location", loc)
		}

		// Process response body for text responses
		contentType := resp.Header.Get("Content-Type")
		isText := strings.Contains(contentType, "text/html") ||
			strings.Contains(contentType, "text/css") ||
			strings.Contains(contentType, "text/x-script") ||
			strings.Contains(contentType, "javascript") ||
			strings.Contains(contentType, "json") ||
			strings.Contains(contentType, "xml")

		if isText {
			encoding := resp.Header.Get("Content-Encoding")
			isGzip := strings.EqualFold(encoding, "gzip")

			var reader io.Reader
			if isGzip {
				gzipReader, err := gzip.NewReader(resp.Body)
				if err != nil {
					return err
				}
				defer gzipReader.Close()
				reader = gzipReader
			} else {
				reader = resp.Body
			}

			bodyBytes, err := io.ReadAll(reader)
			if err != nil {
				return err
			}
			resp.Body.Close()

			bodyStr := string(bodyBytes)

			// Detect limits, sign in prompts, and unexpected errors in HTML response
			isHTML := strings.Contains(contentType, "text/html")
			if isHTML && detectEnvatoLogoutOrLimit(bodyStr) && resp.Request != nil {
				sessionToken := resp.Request.Header.Get("X-Client-Session-Token")
				swapCount := 0
				if swapCountStr := resp.Request.Header.Get("X-Client-Swap-Count"); swapCountStr != "" {
					fmt.Sscanf(swapCountStr, "%d", &swapCount)
				}
				if db == nil && sessionToken != "" {
					htmlContent := renderWaitingForAccountPage()
					if swapCount < 3 {
						if ok, name := envatoPanelSwitch(sessionToken, "html login wall"); ok {
							resp.Header.Add("Set-Cookie", (&http.Cookie{
								Name: "envato_swap_count", Value: fmt.Sprintf("%d", swapCount+1), Path: "/", MaxAge: 120, HttpOnly: true, SameSite: http.SameSiteLaxMode,
							}).String())
							htmlContent = renderAccountSwitchPage(name)
						}
					}
					resp.StatusCode = http.StatusOK
					resp.Header.Set("Content-Type", "text/html; charset=utf-8")
					resp.Body = io.NopCloser(strings.NewReader(htmlContent))
					resp.ContentLength = int64(len(htmlContent))
					resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(htmlContent)))
					resp.Header.Del("Content-Encoding")
					return nil
				}
			}
			if isHTML && db != nil && detectEnvatoLogoutOrLimit(bodyStr) {
				if resp.Request != nil {
					sessionToken := resp.Request.Header.Get("X-Client-Session-Token")
					if sessionToken != "" {
						swapCount := 0
						if swapCountStr := resp.Request.Header.Get("X-Client-Swap-Count"); swapCountStr != "" {
							fmt.Sscanf(swapCountStr, "%d", &swapCount)
						}

						if swapCount < 3 {
							var currentAssignedID sql.NullInt64
							_ = db.QueryRow("SELECT assigned_account_id FROM ahrefs_sessions WHERE session_token = ?", sessionToken).Scan(&currentAssignedID)

							nextAcc, errSwap := swapActiveAccount(sessionToken, currentAssignedID.Int64)
							if errSwap == nil {
								log.Printf("[SWAP] 🔄 Login signatures in body. Swapping to account: %d (%s)", nextAcc.ID, nextAcc.Name)
								newSwapCountCookie := &http.Cookie{
									Name:     "envato_swap_count",
									Value:    fmt.Sprintf("%d", swapCount+1),
									Path:     "/",
									MaxAge:   10,
									HttpOnly: true,
								}
								resp.Header.Add("Set-Cookie", newSwapCountCookie.String())

								// Return swapping page
								htmlContent := renderSwappingPage(defaultLandingPath)
								resp.StatusCode = http.StatusOK
								resp.Header.Set("Content-Type", "text/html; charset=utf-8")
								resp.Body = io.NopCloser(strings.NewReader(htmlContent))
								resp.ContentLength = int64(len(htmlContent))
								resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(htmlContent)))
								resp.Header.Del("Content-Encoding")
								return nil
							}
						}
					}
				}
			}

			// Domain Rewriting to route links via local Proxy
			bodyStr = strings.ReplaceAll(bodyStr, "https://assets.elements.envato.com", proxySchemeHost+"/assets-proxy")
			bodyStr = strings.ReplaceAll(bodyStr, "https://elements.envato.com", proxySchemeHost+"/elements-proxy")
			bodyStr = strings.ReplaceAll(bodyStr, "https://app.envato.com", proxySchemeHost)
			bodyStr = strings.ReplaceAll(bodyStr, "https://api.envato.com", proxySchemeHost+"/api-proxy")
			bodyStr = strings.ReplaceAll(bodyStr, "https://account.envato.com", proxySchemeHost+"/account-proxy")
			bodyStr = strings.ReplaceAll(bodyStr, "https://assets.envato.com", proxySchemeHost+"/assets-proxy")
			bodyStr = strings.ReplaceAll(bodyStr, "https://public-assets.content-platform.envatousercontent.com", proxySchemeHost+"/__domain__/public-assets.content-platform.envatousercontent.com")

			bodyStr = strings.ReplaceAll(bodyStr, "//assets.elements.envato.com", "//"+proxyHost+"/assets-proxy")
			bodyStr = strings.ReplaceAll(bodyStr, "//elements.envato.com", "//"+proxyHost+"/elements-proxy")
			bodyStr = strings.ReplaceAll(bodyStr, "//app.envato.com", "//"+proxyHost)
			bodyStr = strings.ReplaceAll(bodyStr, "//api.envato.com", "//"+proxyHost+"/api-proxy")
			bodyStr = strings.ReplaceAll(bodyStr, "//account.envato.com", "//"+proxyHost+"/account-proxy")
			bodyStr = strings.ReplaceAll(bodyStr, "//assets.envato.com", "//"+proxyHost+"/assets-proxy")
			bodyStr = strings.ReplaceAll(bodyStr, "//public-assets.content-platform.envatousercontent.com", "//"+proxyHost+"/__domain__/public-assets.content-platform.envatousercontent.com")
			if isHTML && db == nil && !strings.Contains(bodyStr, "data-tm-device") {
				userName := ""
				if resp.Request != nil {
					userName, _ = resp.Request.Context().Value(envatoUserNameKey).(string)
				}
				bodyStr = injectEnvatoDeviceScript(bodyStr, userName)
			}

			// Neutralize local domain checks that redirect to app.envato.com
			bodyStr = strings.ReplaceAll(bodyStr, `if(["app.envato.com"].indexOf(h)===-1)`, `if(false)`)
			bodyStr = strings.ReplaceAll(bodyStr, `if (["app.envato.com"].indexOf(h) === -1)`, `if(false)`)

			// Strip Subresource Integrity (SRI)
			bodyStr = integrityRegex.ReplaceAllString(bodyStr, "")
			if strings.Contains(contentType, "javascript") {
				bodyStr = strings.ReplaceAll(bodyStr, ".integrity=", "._ntegrity=")
				bodyStr = webpackAttrRegex.ReplaceAllString(bodyStr, `"_ntegrity":`)
				bodyStr = webpackSetAttrRegex.ReplaceAllString(bodyStr, `setAttribute("_ntegrity"`)
			}

			// Inject proxy-fixer.js
			if strings.Contains(contentType, "text/html") {
				if fixerBytes, errFixer := os.ReadFile("proxy-fixer.js"); errFixer == nil {
					fixerScriptContent := strings.ReplaceAll(string(fixerBytes), "__DYNAMIC_USER_AGENT__", cfgLocal.UserAgent)
					fixerScriptContent = strings.ReplaceAll(fixerScriptContent, "__PRIMARY_HOST__", "app.envato.com")
					fixerScript := "\n<script>\n" + fixerScriptContent + "\n</script>\n"

					headIdx := strings.Index(strings.ToLower(bodyStr), "<head>")
					if headIdx != -1 {
						insertPos := headIdx + len("<head>")
						bodyStr = bodyStr[:insertPos] + fixerScript + bodyStr[insertPos:]
					} else {
						bodyIdx := strings.Index(strings.ToLower(bodyStr), "<body>")
						if bodyIdx != -1 {
							insertPos := bodyIdx + len("<body>")
							bodyStr = bodyStr[:insertPos] + fixerScript + bodyStr[insertPos:]
						} else {
							bodyStr = bodyStr + fixerScript
						}
					}
				}
			}

			modifiedBytes := []byte(bodyStr)
			resp.Header.Del("Transfer-Encoding")

			if isGzip {
				var buf bytes.Buffer
				gzipWriter := gzip.NewWriter(&buf)
				if _, err := gzipWriter.Write(modifiedBytes); err != nil {
					return err
				}
				gzipWriter.Close()
				resp.Body = io.NopCloser(&buf)
				resp.ContentLength = int64(buf.Len())
				resp.Header.Set("Content-Length", fmt.Sprintf("%d", buf.Len()))
				resp.Header.Set("Content-Encoding", "gzip")
			} else {
				resp.Header.Del("Content-Encoding")
				resp.Body = io.NopCloser(bytes.NewBuffer(modifiedBytes))
				resp.ContentLength = int64(len(modifiedBytes))
				resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(modifiedBytes)))
			}

			ctLower := strings.ToLower(contentType)
			if isHTML || strings.Contains(ctLower, "json") {
				resp.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
				resp.Header.Set("Pragma", "no-cache")
				resp.Header.Set("Expires", "0")
			} else if resp.StatusCode == http.StatusOK && resp.Request != nil &&
				cdnCacheKeyFromUpstream(resp.Request) != "" &&
				!strings.Contains(strings.ToLower(resp.Header.Get("Content-Disposition")), "attachment") {
				// Public CDN: allow cache even when CF sets __cf_bm Set-Cookie.
				resp.Header.Set("Cache-Control", "public, max-age=86400")
				resp.Header.Del("Pragma")
				resp.Header.Del("Expires")
				storeCDNCache(resp.Request, resp.StatusCode, contentType, modifiedBytes)
			}
		} else if resp.StatusCode == http.StatusOK && resp.Request != nil &&
			cdnCacheKeyFromUpstream(resp.Request) != "" &&
			!strings.Contains(strings.ToLower(resp.Header.Get("Content-Disposition")), "attachment") {
			bodyBytes, err := io.ReadAll(resp.Body)
			if err == nil {
				resp.Body.Close()
				if len(bodyBytes) > 0 {
					resp.Header.Set("Cache-Control", "public, max-age=86400")
					resp.Header.Del("Pragma")
					resp.Header.Del("Expires")
					storeCDNCache(resp.Request, resp.StatusCode, contentType, bodyBytes)
				}
				resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				resp.ContentLength = int64(len(bodyBytes))
				resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(bodyBytes)))
				resp.Header.Del("Content-Encoding")
			}
		}

		return nil
	}

	reverseProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if err != nil && (errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "context canceled")) {
			return
		}
		log.Printf("[PROXY_ERROR] ❌ Error for %s: %v", r.URL.Path, err)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)

		errHtml := lightMessageHTML(
			"Proxy problem",
			"Proxy problem",
			"Contact to Admin/Provider to fix it ASAP",
			`<a class="retry" href="javascript:location.reload()">Try again</a>`,
			"",
		)
		w.Write([]byte(errHtml))
	}

	startCDNCacheSweep()

	addr := ":" + cfg.Port
	log.Printf("╔══════════════════════════════════════════════════╗")
	log.Printf("║  🚀 Envato Database-Driven Proxy Online          ║")
	log.Printf("║  Local URL  : http://localhost%s                 ║", addr)
	log.Printf("║  Target URL : %s                      ║", cfg.TargetURL)
	log.Printf("╚══════════════════════════════════════════════════╝")

	http.HandleFunc("/api/auth-handshake", handleAuthHandshake)
	http.HandleFunc("/api/device-bind", handleEnvatoDeviceBind)
	http.HandleFunc("/tm-device-sw.js", serveDeviceSW)
	http.HandleFunc("/access", handleAccess)
	http.HandleFunc("/api/user-limits", handleUserLimits)
	http.HandleFunc("/api/trigger-swap", handleTriggerSwap)
	http.HandleFunc("/", handleProxyRequest)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("[FATAL] Server crashed: %v", err)
	}
}
