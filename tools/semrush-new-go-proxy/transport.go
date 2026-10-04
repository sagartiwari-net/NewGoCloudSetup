package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

const proxyFileName = "proxy.txt"

type upstreamProxyCtxKeyType struct{}

var upstreamProxyCtxKey = upstreamProxyCtxKeyType{}

var (
	fileProxyMu  sync.Mutex
	fileProxyURL *url.URL
	fileProxyMod time.Time

	websiteProxyMu    sync.Mutex
	websiteProxyCache = map[int]struct {
		proxy   string
		expires time.Time
	}{}
)

func withUpstreamProxy(ctx context.Context, proxyStr string) context.Context {
	proxyStr = strings.TrimSpace(proxyStr)
	if proxyStr == "" {
		return ctx
	}
	return context.WithValue(ctx, upstreamProxyCtxKey, proxyStr)
}

func getWebsiteProxy(websiteID int) string {
	if websiteID <= 0 || db == nil {
		return ""
	}
	websiteProxyMu.Lock()
	defer websiteProxyMu.Unlock()
	if ent, ok := websiteProxyCache[websiteID]; ok && time.Now().Before(ent.expires) {
		return ent.proxy
	}
	var p string
	err := db.QueryRow("SELECT COALESCE(proxy,'') FROM ahrefs_websites WHERE id = ?", websiteID).Scan(&p)
	if err != nil {
		p = ""
	}
	p = strings.TrimSpace(p)
	websiteProxyCache[websiteID] = struct {
		proxy   string
		expires time.Time
	}{proxy: p, expires: time.Now().Add(60 * time.Second)}
	return p
}

func normalizeProxyEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "socks5://") || strings.HasPrefix(lower, "socks5h://") ||
		strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return raw
	}
	// host:port:user:pass (common panel paste format)
	parts := strings.Split(raw, ":")
	if len(parts) == 4 {
		return fmt.Sprintf("socks5://%s:%s@%s:%s", parts[2], parts[3], parts[0], parts[1])
	}
	// user:pass@host:port
	if strings.Contains(raw, "@") && !strings.Contains(raw, "://") {
		return "socks5://" + raw
	}
	// host:port
	if len(parts) == 2 {
		return "socks5://" + raw
	}
	return raw
}

func parseProxyURL(raw string) (*url.URL, error) {
	raw = normalizeProxyEndpoint(raw)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	// Common mistake: leaving the example placeholders
	upper := strings.ToUpper(raw)
	if strings.Contains(upper, "USER:PASS@HOST") || strings.Contains(upper, "@HOST:PORT") {
		return nil, fmt.Errorf("placeholder URL — replace USER/PASS/HOST/PORT with real proxy credentials")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Host == "" {
		return nil, fmt.Errorf("missing host")
	}
	switch strings.ToLower(u.Scheme) {
	case "socks5", "socks5h", "http", "https":
		return u, nil
	default:
		return nil, fmt.Errorf("unsupported scheme %q (use socks5/http)", u.Scheme)
	}
}

func loadProxyFromFile() *url.URL {
	fileProxyMu.Lock()
	defer fileProxyMu.Unlock()

	info, err := os.Stat(proxyFileName)
	if err != nil {
		fileProxyURL = nil
		fileProxyMod = time.Time{}
		return nil
	}
	if !info.ModTime().After(fileProxyMod) {
		return fileProxyURL
	}
	data, err := os.ReadFile(proxyFileName)
	if err != nil {
		return fileProxyURL
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
	fileProxyMod = info.ModTime()
	if proxyStr == "" {
		fileProxyURL = nil
		log.Printf("[PROXY] %s is empty — Semrush uses VPS IP directly", proxyFileName)
		return nil
	}
	u, err := parseProxyURL(proxyStr)
	if err != nil {
		fileProxyURL = nil
		log.Printf("[PROXY] ❌ Invalid %s entry: %v", proxyFileName, err)
		return nil
	}
	fileProxyURL = u
	if fileProxyURL != nil {
		log.Printf("[PROXY] Loaded upstream from %s → %s://%s", proxyFileName, fileProxyURL.Scheme, fileProxyURL.Host)
	}
	return fileProxyURL
}

// resolveUpstreamProxy order: request/account override → proxy.txt → config → website_id default.
func resolveUpstreamProxy(cfg Config, requestOverride string) *url.URL {
	try := func(raw, src string) *url.URL {
		u, err := parseProxyURL(raw)
		if err != nil {
			log.Printf("[PROXY] ❌ Invalid %s: %v", src, err)
			return nil
		}
		return u
	}
	if u := try(requestOverride, "account/request proxy"); u != nil {
		return u
	}
	if u := loadProxyFromFile(); u != nil {
		return u
	}
	if u := try(cfg.UpstreamProxy, "config.upstream_proxy"); u != nil {
		return u
	}
	if cfg.WebsiteID > 0 {
		if u := try(getWebsiteProxy(cfg.WebsiteID), "ahrefs_websites.proxy"); u != nil {
			return u
		}
	}
	return nil
}

func dialThroughProxy(ctx context.Context, targetAddr string, proxyURL *url.URL) (net.Conn, error) {
	switch strings.ToLower(proxyURL.Scheme) {
	case "socks5", "socks5h":
		return dialThroughSocks5(ctx, targetAddr, proxyURL)
	case "http", "https":
		return dialThroughHTTPProxy(ctx, targetAddr, proxyURL)
	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s", proxyURL.Scheme)
	}
}

func dialThroughSocks5(ctx context.Context, targetAddr string, proxyURL *url.URL) (net.Conn, error) {
	var auth *proxy.Auth
	if proxyURL.User != nil {
		pass, _ := proxyURL.User.Password()
		auth = &proxy.Auth{User: proxyURL.User.Username(), Password: pass}
	}
	proxyHost := proxyURL.Host
	if !strings.Contains(proxyHost, ":") {
		proxyHost += ":1080"
	}
	baseDialer := &net.Dialer{Timeout: 20 * time.Second}
	socks5Dialer, err := proxy.SOCKS5("tcp", proxyHost, auth, baseDialer)
	if err != nil {
		return nil, fmt.Errorf("socks5 dialer init: %w", err)
	}
	if cd, ok := socks5Dialer.(proxy.ContextDialer); ok {
		return cd.DialContext(ctx, "tcp", targetAddr)
	}
	return socks5Dialer.Dial("tcp", targetAddr)
}

func dialThroughHTTPProxy(ctx context.Context, targetAddr string, proxyURL *url.URL) (net.Conn, error) {
	proxyHost := proxyURL.Host
	if !strings.Contains(proxyHost, ":") {
		if proxyURL.Scheme == "https" {
			proxyHost += ":443"
		} else {
			proxyHost += ":80"
		}
	}
	conn, err := (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, "tcp", proxyHost)
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
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
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
}

func buildUpstreamTransport() http.RoundTripper {
	base := &net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Transport{
		Proxy: nil, // dial manually so socks5 works
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			cfg := loadConfig()
			reqProxy := ""
			if v, ok := ctx.Value(upstreamProxyCtxKey).(string); ok {
				reqProxy = v
			}
			if px := resolveUpstreamProxy(cfg, reqProxy); px != nil {
				conn, err := dialThroughProxy(ctx, addr, px)
				if err != nil {
					return nil, fmt.Errorf("upstream proxy %s://%s: %w", px.Scheme, px.Host, err)
				}
				return conn, nil
			}
			return base.DialContext(ctx, network, addr)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   64,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 180 * time.Second,
	}
}

func isCloudflareChallengeBody(body []byte) bool {
	lower := strings.ToLower(string(body))
	if strings.Contains(lower, "cf-browser-verification") || strings.Contains(lower, "cf-challenge") {
		return true
	}
	if strings.Contains(lower, "challenge-platform") && strings.Contains(lower, "cloudflare") {
		return true
	}
	if strings.Contains(lower, "just a moment") && strings.Contains(lower, "cloudflare") {
		return true
	}
	if strings.Contains(lower, "g-recaptcha") && strings.Contains(lower, "checking your browser") {
		return true
	}
	return false
}

func logUpstreamProxyStatus(cfg Config) {
	if px := resolveUpstreamProxy(cfg, ""); px != nil {
		log.Printf("[PROXY] Upstream Semrush traffic via %s://%s", px.Scheme, px.Host)
	} else {
		log.Printf("[PROXY] ⚠️ No proxy configured (account / website / proxy.txt / upstream_proxy) — VPS IP hits Semrush directly (Cloudflare can 403)")
	}
}
