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
