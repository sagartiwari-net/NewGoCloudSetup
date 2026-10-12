package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

func uberCookiePairs(header string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		eq := strings.Index(part, "=")
		if eq <= 0 {
			continue
		}
		name := strings.TrimSpace(part[:eq])
		if name == "" {
			continue
		}
		out[name] = part[eq+1:]
	}
	return out
}

func uberSessionName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "id", "state", "ubs-accounts", "ubs-account-count":
		return true
	default:
		return false
	}
}

func uberNeilDomain(domain string) bool {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return true
	}
	return strings.Contains(domain, "neilpatel")
}

// uberSessionCookieHeader keeps the Neil Patel session cookies.
// A full browser export also contains tracking cookies, and a second cookie
// with the same name was replacing id/state before they reached the API.
func uberSessionCookieHeader(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	entries := panelCookieEntries(raw)
	if len(entries) == 0 && !strings.HasPrefix(raw, "{") && !strings.HasPrefix(raw, "[") {
		for name, value := range uberCookiePairs(raw) {
			entries = append(entries, panelCookieEntry{name: name, value: value})
		}
	}
	best := map[string]panelCookieEntry{}
	var order []string
	for _, e := range entries {
		if e.name == "" || e.value == "" || !uberSessionName(e.name) || !uberNeilDomain(e.domain) {
			continue
		}
		prev, ok := best[e.name]
		if ok && uberNeilDomain(prev.domain) && strings.Contains(strings.ToLower(prev.domain), "neilpatel") && !strings.Contains(strings.ToLower(e.domain), "neilpatel") {
			continue
		}
		if !ok {
			order = append(order, e.name)
		}
		best[e.name] = e
	}
	// id and state have to be first. The API rejects the session without them.
	prefer := []string{"id", "state", "ubs-accounts", "ubs-account-count"}
	var parts []string
	seen := map[string]bool{}
	for _, name := range prefer {
		if e, ok := best[name]; ok {
			parts = append(parts, e.name+"="+e.value)
			seen[name] = true
		}
	}
	for _, name := range order {
		if seen[name] {
			continue
		}
		parts = append(parts, best[name].name+"="+best[name].value)
	}
	return strings.Join(parts, "; ")
}

func logUberAuthCookies(path, header string) {
	if path != "/" && path != "/dashboard" && !strings.Contains(path, "/analyze") && !strings.Contains(path, "/en/") && !strings.Contains(path, "/api/") {
		return
	}
	pairs := uberCookiePairs(header)
	_, hasID := pairs["id"]
	_, hasState := pairs["state"]
	_, hasAccounts := pairs["ubs-accounts"]
	log.Printf("[UBER] upstream %s id=%v(%d) state=%v(%d) ubs-accounts=%v(%d)", path, hasID, len(pairs["id"]), hasState, len(pairs["state"]), hasAccounts, len(pairs["ubs-accounts"]))
}

// rewriteUpstreamSetCookies keeps session cookies on the proxy host.
// Leaving Domain=app.neilpatel.com makes the browser drop them.
// The raw line is kept so a long state value is not stripped by the cookie parser.
func rewriteUpstreamSetCookies(resp *http.Response) {
	raw := resp.Header.Values("Set-Cookie")
	if len(raw) == 0 {
		return
	}
	resp.Header.Del("Set-Cookie")
	for _, line := range raw {
		parts := strings.Split(line, ";")
		kept := []string{parts[0]}
		for _, p := range parts[1:] {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(p)), "domain=") {
				continue
			}
			kept = append(kept, p)
		}
		resp.Header.Add("Set-Cookie", strings.Join(kept, ";"))
	}
}

func uberSetCookieLine(name, value, path string) string {
	if path == "" {
		path = "/"
	}
	return name + "=" + value + "; Path=" + path + "; Max-Age=604800; SameSite=Lax"
}

// seedUberAuthCookies puts the session on the browser. /api/get_token stays
// logged out unless id and state are on the request.
func seedUberAuthCookies(resp *http.Response, header string) {
	pairs := uberCookiePairs(header)
	for _, name := range []string{"id", "state", "ubs-accounts", "ubs-account-count"} {
		value := pairs[name]
		if value == "" || strings.ContainsAny(value, ";\r\n") {
			continue
		}
		resp.Header.Add("Set-Cookie", uberSetCookieLine(name, value, "/"))
		if name == "ubs-accounts" {
			resp.Header.Add("Set-Cookie", uberSetCookieLine(name, value, "/api/next/session"))
		}
	}
}

func uberAuthCookieScript(header string) string {
	pairs := uberCookiePairs(header)
	var sets []string
	for _, name := range []string{"id", "state"} {
		value := pairs[name]
		if value == "" {
			continue
		}
		nameJS, err1 := json.Marshal(name)
		valueJS, err2 := json.Marshal(value)
		if err1 != nil || err2 != nil {
			continue
		}
		sets = append(sets, "document.cookie="+string(nameJS)+"+'='+"+string(valueJS)+"+'; path=/; max-age=604800; SameSite=Lax';")
	}
	if len(sets) == 0 {
		return ""
	}
	return "<script>(function(){" + strings.Join(sets, "") + "})();</script>"
}
