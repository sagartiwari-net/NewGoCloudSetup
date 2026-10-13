package main

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const maxCookieHops = 6

var derivedCookies sync.Map

func cookiesForUpstream(acc ToolAccount) string {
	base := strings.TrimSpace(acc.Cookie)
	if acc.ID == 0 {
		return base
	}
	extra, ok := derivedCookies.Load(acc.ID)
	if !ok {
		return base
	}
	return mergeCookieHeaders(extra.(string), base)
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
	switch strings.ToLower(name) {
	case "ct_session", "proxy_token":
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
		if !strings.HasPrefix(u.Path, "/") {
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
	if host != up && host != pub && !strings.HasSuffix(host, ".markhorseotool.com") && host != "markhorseotool.com" {
		return "", false
	}
	if u.Path == "" {
		u.Path = "/"
	}
	return u.RequestURI(), true
}
