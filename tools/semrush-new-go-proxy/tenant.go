package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"strings"
)

type tenantContextKey struct{}

func normalizeHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.TrimSuffix(host, ".")
}

func lookupWebsiteIDByHost(host string) int {
	host = normalizeHost(host)
	if host == "" || db == nil {
		return 0
	}
	var wid int
	err := db.QueryRow("SELECT id FROM ahrefs_websites WHERE LOWER(domain) = ?", host).Scan(&wid)
	if err == nil && wid > 0 {
		return wid
	}
	// Tolerate accidental www. prefix in DB or Host header
	alt := host
	if strings.HasPrefix(host, "www.") {
		alt = strings.TrimPrefix(host, "www.")
	} else {
		alt = "www." + host
	}
	err = db.QueryRow("SELECT id FROM ahrefs_websites WHERE LOWER(domain) = ?", alt).Scan(&wid)
	if err != nil {
		return 0
	}
	return wid
}

func resolveHandshakeHost(r *http.Request, cfg Config) string {
	candidates := []string{
		r.Header.Get("X-Forwarded-Host"),
		r.Header.Get("X-Original-Host"),
		r.Host,
		cfg.PublicHost,
	}
	for _, raw := range candidates {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		// X-Forwarded-Host can be a comma-separated list
		if i := strings.Index(raw, ","); i >= 0 {
			raw = raw[:i]
		}
		if h := normalizeHost(raw); h != "" {
			return h
		}
	}
	return ""
}

func resolveWebsiteIDForHost(host string, cfg Config) int {
	if wid := lookupWebsiteIDByHost(host); wid > 0 {
		return wid
	}
	// Fallback when nginx Host is wrong but public_host is this reseller domain
	if host != "" && normalizeHost(cfg.PublicHost) == host && cfg.WebsiteID > 0 {
		log.Printf("[DB] Using config website_id=%d for host %q (DB domain row missing or Host mismatch)", cfg.WebsiteID, host)
		return cfg.WebsiteID
	}
	return 0
}

func websiteIDForRequest(r *http.Request) int {
	if id := tenantWebsiteID(r.Context()); id > 0 {
		return id
	}
	if wid := lookupWebsiteIDByHost(r.Host); wid > 0 {
		return wid
	}
	cfg := loadConfig()
	if cfg.WebsiteID > 0 {
		return cfg.WebsiteID
	}
	return 1
}

func withTenantWebsiteID(ctx context.Context, websiteID int) context.Context {
	if websiteID <= 0 {
		return ctx
	}
	return context.WithValue(ctx, tenantContextKey{}, websiteID)
}

func tenantWebsiteID(ctx context.Context) int {
	if ctx == nil {
		return 0
	}
	if v, ok := ctx.Value(tenantContextKey{}).(int); ok && v > 0 {
		return v
	}
	return 0
}

func validateRequestHostMatchesWebsite(r *http.Request, websiteID int) bool {
	if websiteID <= 0 {
		return true
	}
	hostID := lookupWebsiteIDByHost(r.Host)
	if hostID <= 0 {
		hostID = lookupWebsiteIDByHost(r.Header.Get("X-Forwarded-Host"))
	}
	if hostID <= 0 {
		log.Printf("[SECURITY] Unknown host %q for website_id=%d", r.Host, websiteID)
		return false
	}
	return hostID == websiteID
}

func ottIPAllowed(storedIP, requestIP string) bool {
	storedIP = strings.TrimSpace(storedIP)
	requestIP = strings.TrimSpace(requestIP)
	if storedIP == "" || requestIP == "" {
		return true
	}
	if storedIP == requestIP {
		return true
	}
	// Panel handshake often stores member IP; allow common proxy / IPv4-mapped differences.
	if strings.HasPrefix(requestIP, storedIP+":") || strings.HasPrefix(storedIP, requestIP+":") {
		return true
	}
	log.Printf("[ACCESS] ⚠️ OTT IP soft-mismatch (stored=%s req=%s) — allowing panel redirect", storedIP, requestIP)
	return true
}
