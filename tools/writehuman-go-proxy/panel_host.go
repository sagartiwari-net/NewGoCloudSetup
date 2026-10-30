package main

import (
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

// requestPublicBase is the browser origin for this hit. A second panel website
// on the same port keeps its own domain. Anything that is not this tool stays
// on the configured public host.
func requestPublicBase(r *http.Request, c Config) string {
	host := panelHostOnly(c.PublicHost)
	scheme := strings.ToLower(strings.TrimSpace(c.PublicScheme))
	if scheme != "http" && scheme != "https" {
		scheme = "http"
	}
	if r != nil {
		candidate := panelRequestHost(r, c.PublicHost)
		if candidate == host || panelHostAllowed(c.PublicHost, candidate) {
			host = candidate
		}
		if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
			proto = strings.ToLower(strings.TrimSpace(strings.Split(proto, ",")[0]))
			if proto == "http" || proto == "https" {
				scheme = proto
			}
		} else if r.TLS != nil {
			scheme = "https"
		}
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return scheme + "://" + host
}

// tenantPublicHost keeps a second website on this same port on its own domain.
// A host that is not this tool stays on the configured public host.
func tenantPublicHost(r *http.Request, cfg Config) string {
	fallback := strings.TrimSpace(cfg.PublicHost)
	if r == nil || fallback == "" {
		return fallback
	}
	candidate := panelRequestHost(r, cfg.PublicHost)
	if candidate == "" || candidate == panelHostOnly(fallback) || panelHostIsLocal(candidate) {
		return fallback
	}
	if panelHostAllowed(cfg.PublicHost, candidate) {
		return candidate
	}
	return fallback
}
