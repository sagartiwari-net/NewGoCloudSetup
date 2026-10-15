package main

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const maxCookieHops = 6

var derivedCookies sync.Map

func cookiesForUpstream(acc ToolAccount, r *http.Request) string {
	// The panel cookie is the session the user saved. A later Set-Cookie on this
	// host must not replace those names, or the next menu navigation is sent
	// with a guest session and the upstream answers 403 Access Denied.
	panel := strings.TrimSpace(acc.Cookie)
	derived := ""
	if acc.ID != 0 {
		if extra, ok := derivedCookies.Load(acc.ID); ok {
			derived = extra.(string)
		}
	}
	browser := ""
	if r != nil {
		browser = browserCookiesForUpstream(r)
	}
	merged := mergeCookieHeaders(browser, derived)
	return mergeCookieHeaders(merged, panel)
}

func browserCookiesForUpstream(r *http.Request) string {
	if r == nil {
		return ""
	}
	order, vals := parseCookieHeader(r.Header.Get("Cookie"))
	return serializeCookies(order, vals)
}

func rememberUpstreamCookies(acc ToolAccount, merged string) {
	if acc.ID == 0 {
		return
	}
	derivedCookies.Store(acc.ID, strings.TrimSpace(merged))
}

func mergeCookieHeaders(base, overlay string) string {
	order, vals := parseCookieHeader(base)
	order, vals = applyCookieHeader(order, vals, overlay)
	return serializeCookies(order, vals)
}

func absorbSetCookies(existing string, header http.Header) string {
	if header == nil {
		return existing
	}
	order, vals := parseCookieHeader(existing)
	for _, raw := range header.Values("Set-Cookie") {
		order, vals = applySetCookie(order, vals, raw)
	}
	return serializeCookies(order, vals)
}

func parseCookieHeader(header string) ([]string, map[string]string) {
	order := []string{}
	vals := map[string]string{}
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		if part == "" || !strings.Contains(part, "=") {
			continue
		}
		name, value, _ := strings.Cut(part, "=")
		name = strings.TrimSpace(name)
		if name == "" || dropUpstreamCookie(name) {
			continue
		}
		if _, ok := vals[name]; !ok {
			order = append(order, name)
		}
		vals[name] = strings.TrimSpace(value)
	}
	return order, vals
}

func applyCookieHeader(order []string, vals map[string]string, header string) ([]string, map[string]string) {
	addOrder, addVals := parseCookieHeader(header)
	for _, name := range addOrder {
		if _, ok := vals[name]; !ok {
			order = append(order, name)
		}
		vals[name] = addVals[name]
	}
	return order, vals
}

func applySetCookie(order []string, vals map[string]string, raw string) ([]string, map[string]string) {
	parts := strings.Split(raw, ";")
	if len(parts) == 0 || !strings.Contains(parts[0], "=") {
		return order, vals
	}
	name, value, _ := strings.Cut(parts[0], "=")
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	if name == "" || dropUpstreamCookie(name) {
		return order, vals
	}
	if setCookieExpired(parts[1:]) {
		delete(vals, name)
		next := order[:0]
		for _, item := range order {
			if item != name {
				next = append(next, item)
			}
		}
		return next, vals
	}
	if _, ok := vals[name]; !ok {
		order = append(order, name)
	}
	vals[name] = value
	return order, vals
}

func setCookieExpired(attrs []string) bool {
	for _, attr := range attrs {
		attr = strings.TrimSpace(attr)
		if len(attr) >= 8 && strings.EqualFold(attr[:8], "max-age=") {
			age := strings.TrimSpace(attr[8:])
			return age == "0" || strings.HasPrefix(age, "-")
		}
	}
	return false
}

func serializeCookies(order []string, vals map[string]string) string {
	parts := make([]string, 0, len(order))
	for _, name := range order {
		value, ok := vals[name]
		if !ok || value == "" {
			continue
		}
		parts = append(parts, name+"="+value)
	}
	return strings.Join(parts, "; ")
}

func dropUpstreamCookie(name string) bool {
	// auth_session is the browseanywhere login cookie saved on the Recloud account.
	// Only the panel access cookie stays off the upstream request.
	switch strings.ToLower(name) {
	case "ct_session":
		return true
	}
	return false
}

func sameSiteRequestURI(loc, upstreamHost, publicHost string) (string, bool) {
	loc = strings.TrimSpace(loc)
	if loc == "" {
		return "", false
	}
	u, err := url.Parse(loc)
	if err != nil {
		return "", false
	}
	if u.Host == "" {
		if !strings.HasPrefix(u.Path, "/") || strings.Contains(strings.ToLower(u.Path), "/cdn-cgi/") {
			return "", false
		}
		return u.RequestURI(), true
	}
	host := strings.ToLower(u.Hostname())
	up := strings.ToLower(upstreamHost)
	if i := strings.Index(up, ":"); i >= 0 {
		up = up[:i]
	}
	pub := strings.ToLower(publicHost)
	if i := strings.Index(pub, ":"); i >= 0 {
		pub = pub[:i]
	}
	if host != up && host != pub && !strings.HasSuffix(host, ".browseanywhere.com") && host != "browseanywhere.com" {
		return "", false
	}
	if u.Path == "" {
		u.Path = "/"
	}
	if strings.Contains(strings.ToLower(u.Path), "/cdn-cgi/") {
		return "", false
	}
	return u.RequestURI(), true
}
