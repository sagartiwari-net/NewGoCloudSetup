package main

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

func panelHostOnly(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	} else if i := strings.Index(host, ":"); i > 0 && strings.Count(host, ":") == 1 {
		host = host[:i]
	}
	return strings.TrimSuffix(host, ".")
}

func panelHostIsLocal(host string) bool {
	host = panelHostOnly(host)
	return host == "" || host == "localhost" || host == "::1" || strings.HasPrefix(host, "127.")
}

// panelRequestHost is the website domain for this request.
// Localhost and empty hosts fall back to the tool's configured public host.
func panelRequestHost(r *http.Request, publicHost string) string {
	raw := ""
	if r != nil {
		if xh := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); xh != "" {
			raw = strings.TrimSpace(strings.Split(xh, ",")[0])
		} else if xh := strings.TrimSpace(r.Header.Get("X-Original-Host")); xh != "" {
			raw = strings.TrimSpace(strings.Split(xh, ",")[0])
		} else {
			raw = r.Host
		}
	}
	host := panelHostOnly(raw)
	if panelHostIsLocal(host) {
		if ph := panelHostOnly(publicHost); ph != "" {
			return ph
		}
	}
	if host == "" {
		return panelHostOnly(publicHost)
	}
	return host
}

// tenantPublicHost keeps a second website on this same port on its own domain.
// A host that is not this tool stays on the configured public host.
func tenantPublicHost(r *http.Request, cfg Config) string {
	fallback := strings.TrimSpace(cfg.PublicHost)
	candidate := panelRequestHost(r, cfg.PublicHost)
	if candidate != "" && !panelHostIsLocal(candidate) {
		return candidate
	}
	return fallback
}

// requestPublicScheme prefers the edge proto so reseller HTTPS pages do not get http:// asset rewrites.
func requestPublicScheme(r *http.Request, cfg Config) string {
	if r != nil {
		if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
			proto = strings.ToLower(strings.TrimSpace(strings.Split(proto, ",")[0]))
			if proto == "http" || proto == "https" {
				return proto
			}
		}
		if r.TLS != nil {
			return "https"
		}
	}
	scheme := strings.ToLower(strings.TrimSpace(cfg.PublicScheme))
	if scheme == "http" || scheme == "https" {
		return scheme
	}
	return "https"
}

// requestPublicBase is https://<this-request-host> for HTML/JS rewrites.
// Using cfg.PublicHost alone rewrote reseller pages to ziknl.upays.in → CORS on api calls.
func requestPublicBase(r *http.Request, cfg Config) string {
	host := tenantPublicHost(r, cfg)
	if host == "" {
		host = strings.TrimSpace(cfg.PublicHost)
	}
	return fmt.Sprintf("%s://%s", requestPublicScheme(r, cfg), host)
}
