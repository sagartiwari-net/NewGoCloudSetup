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

// ahrefsPublicHost keeps a second Ahrefs website on port 5291 on its own domain.
func ahrefsPublicHost(r *http.Request, cfg Config) string {
	fallback := strings.TrimSpace(cfg.PublicHost)
	if r == nil || fallback == "" {
		return fallback
	}
	candidate := panelRequestHost(r, cfg.PublicHost)
	if candidate == "" || candidate == panelHostOnly(fallback) || panelHostIsLocal(candidate) {
		return fallback
	}
	db, err := openAhrefsPanel()
	if err != nil {
		return fallback
	}
	id := ahrefsWebsiteID(db, candidate)
	if id <= 0 {
		return fallback
	}
	var n int
	err = db.QueryRow(`SELECT COUNT(*) FROM websites w
		JOIN websites owner ON owner.tool_id = w.tool_id
		WHERE w.id = ?
		  AND (lower(trim(owner.domain)) = lower(trim(?)) OR lower(trim(owner.domain)) = lower(trim(?)))`,
		id, fallback, panelHostOnly(fallback)).Scan(&n)
	if err != nil || n == 0 {
		return fallback
	}
	return candidate
}
