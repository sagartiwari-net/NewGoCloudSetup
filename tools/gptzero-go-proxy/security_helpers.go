package main

import (
	"fmt"
	"html"
	"net"
	"net/http"
	"strings"
)

// ToolAccount is a mapped panel website account (cookie from panel DB).
type ToolAccount struct {
	ID        int
	Name      string
	Cookie    string
	UserAgent string
	Proxy     string
	ShowLimit bool
}

func loadConfig() Config {
	return getConfig()
}

func realClientIP(r *http.Request) string {
	for _, h := range []string{"CF-Connecting-IP", "True-Client-IP", "X-Real-IP", "X-Forwarded-For"} {
		v := strings.TrimSpace(r.Header.Get(h))
		if v == "" {
			continue
		}
		if h == "X-Forwarded-For" {
			v = strings.TrimSpace(strings.Split(v, ",")[0])
		}
		if host, _, err := net.SplitHostPort(v); err == nil {
			v = host
		}
		if v != "" {
			return v
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func renderAccessDeniedPage(w http.ResponseWriter, cfg Config) {
	name := html.EscapeString(toolDisplayName(cfg))
	writeLightCard(w, http.StatusForbidden, lightCard{
		Title:   "Access Denied",
		Heading: "Access Denied",
		Message: "You cannot open <span class=\"brand\">" + name + "</span> directly. Open it again from your access link.",
		Footer:  "Your session ended or this browser is not authorized",
	})
}

func renderNoActiveAccountsPage(w http.ResponseWriter, cfg Config) {
	name := html.EscapeString(toolDisplayName(cfg))
	writeLightCard(w, http.StatusServiceUnavailable, lightCard{
		Title:   "Temporarily Unavailable",
		Heading: "Temporarily Unavailable",
		Message: "All mapped <span class=\"brand\">" + name + "</span> accounts are currently undergoing maintenance. Please try again in a few minutes.",
		Footer:  "No active account is available right now",
	})
}

func ctSessionToken(r *http.Request) string {
	c, err := r.Cookie("ct_session")
	if err != nil || c == nil {
		return ""
	}
	return strings.TrimSpace(c.Value)
}

// wantsLightDenied shows the #eef3f8 Access Denied card for page navigations.
// JSON APIs (/v2, /v3, /api-proxy, /extra-cdn) keep a JSON 401.
func wantsLightDenied(r *http.Request, path string) bool {
	if isDocumentNavigation(r) || strings.Contains(r.Header.Get("Accept"), "text/html") {
		return true
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	p := strings.ToLower(path)
	if strings.HasPrefix(p, "/v2/") || strings.HasPrefix(p, "/v3/") ||
		strings.HasPrefix(p, "/api-proxy/") || strings.HasPrefix(p, "/extra-cdn-") ||
		strings.HasPrefix(p, "/api/") {
		return false
	}
	return true
}

func sessionRawForRequest(r *http.Request, cfg Config) (string, error) {
	if !usesPanelAccountMode(cfg) || cfg.BypassAuth {
		return loadSession(), nil
	}
	token := ctSessionToken(r)
	if token == "" {
		return "", fmt.Errorf("missing session")
	}
	acc, err := loadPanelSessionAccount(cfg, token)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(acc.UserAgent) != "" {
		// Prefer account UA for this request's outbound calls via config override is handled in proxy.
		_ = acc.UserAgent
	}
	return acc.Cookie, nil
}
