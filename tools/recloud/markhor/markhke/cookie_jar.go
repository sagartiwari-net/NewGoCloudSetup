package main

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const maxCookieHops = 6

// Oversized values rewritten onto the public host make Chrome's Cookie header
// bigger than nginx large_client_header_buffers.
const maxBrowserCookieValueLen = 2048

// ke.markhorseotool.com is Node — default max-http-header-size is 16KiB. Over that
// it returns 431 "Error parsing headers: 'limit request headers fields size'".
const maxUpstreamCookieBytes = 12 * 1024

var derivedCookies sync.Map

func cookiesForUpstream(acc ToolAccount, r *http.Request) string {
	// Panel cookie wins. Derived/browser may add missing names only within budget.
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
	order, vals := parseCookieHeaderRaw(panel)
	order, vals = applyCookieHeaderKeepNew(order, vals, derived)
	order, vals = applyCookieHeaderKeepNew(order, vals, browser)
	return fitCookieBudget(order, vals, maxUpstreamCookieBytes)
}

func panelCookieOnly(acc ToolAccount) string {
	order, vals := parseCookieHeaderRaw(strings.TrimSpace(acc.Cookie))
	return fitCookieBudget(order, vals, maxUpstreamCookieBytes)
}

func clearDerivedCookies(acc ToolAccount) {
	if acc.ID == 0 {
		return
	}
	derivedCookies.Delete(acc.ID)
}

func browserCookiesForUpstream(r *http.Request) string {
	if r == nil {
		return ""
	}
	order, vals := parseCookieHeaderRaw(r.Header.Get("Cookie"))
	// Never forward our gate cookie; drop obvious analytics junk from the browser.
	outOrder := make([]string, 0, len(order))
	outVals := map[string]string{}
	for _, name := range order {
		if dropUpstreamCookie(name) || dropJunkCookie(name) {
			continue
		}
		value := vals[name]
		if value == "" || len(value) > maxBrowserCookieValueLen {
			continue
		}
		outOrder = append(outOrder, name)
		outVals[name] = value
	}
	return serializeCookies(outOrder, outVals)
}

func rememberUpstreamCookies(acc ToolAccount, merged string) {
	if acc.ID == 0 {
		return
	}
	order, vals := parseCookieHeaderRaw(merged)
	derivedCookies.Store(acc.ID, fitCookieBudget(order, vals, maxUpstreamCookieBytes))
}

func mergeCookieHeaders(base, overlay string) string {
	order, vals := parseCookieHeaderRaw(base)
	order, vals = applyCookieHeader(order, vals, overlay)
	return serializeCookies(order, vals)
}

func absorbSetCookies(existing string, header http.Header) string {
	if header == nil {
		return existing
	}
	order, vals := parseCookieHeaderRaw(existing)
	for _, raw := range header.Values("Set-Cookie") {
		order, vals = applySetCookie(order, vals, raw)
	}
	return fitCookieBudget(order, vals, maxUpstreamCookieBytes)
}

func parseCookieHeaderRaw(header string) ([]string, map[string]string) {
	order := []string{}
	vals := map[string]string{}
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		if part == "" || !strings.Contains(part, "=") {
			continue
		}
		name, value, _ := strings.Cut(part, "=")
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name == "" || dropUpstreamCookie(name) {
			continue
		}
		if _, ok := vals[name]; !ok {
			order = append(order, name)
		}
		vals[name] = value
	}
	return order, vals
}

func applyCookieHeader(order []string, vals map[string]string, header string) ([]string, map[string]string) {
	addOrder, addVals := parseCookieHeaderRaw(header)
	for _, name := range addOrder {
		if _, ok := vals[name]; !ok {
			order = append(order, name)
		}
		vals[name] = addVals[name]
	}
	return order, vals
}

// applyCookieHeaderKeepNew only fills names that are not already present (panel wins).
func applyCookieHeaderKeepNew(order []string, vals map[string]string, header string) ([]string, map[string]string) {
	addOrder, addVals := parseCookieHeaderRaw(header)
	for _, name := range addOrder {
		if _, ok := vals[name]; ok {
			continue
		}
		order = append(order, name)
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
	if name == "" || dropUpstreamCookie(name) || dropJunkCookie(name) {
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

func fitCookieBudget(order []string, vals map[string]string, maxBytes int) string {
	if maxBytes <= 0 {
		return serializeCookies(order, vals)
	}
	parts := make([]string, 0, len(order))
	n := 0
	for _, name := range order {
		value, ok := vals[name]
		if !ok || value == "" {
			continue
		}
		part := name + "=" + value
		add := len(part)
		if len(parts) > 0 {
			add += 2 // "; "
		}
		if n+add > maxBytes {
			continue
		}
		parts = append(parts, part)
		n += add
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

func dropJunkCookie(name string) bool {
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(lower, "_ga"), strings.HasPrefix(lower, "_gid"), strings.HasPrefix(lower, "_gat"):
		return true
	case strings.HasPrefix(lower, "_fbp"), strings.HasPrefix(lower, "_fbc"):
		return true
	case strings.HasPrefix(lower, "amp_"), strings.HasPrefix(lower, "ajs_"):
		return true
	case strings.HasPrefix(lower, "mp_"), strings.HasPrefix(lower, "intercom"):
		return true
	case strings.HasPrefix(lower, "hubspot"), strings.HasPrefix(lower, "__hstc"), strings.HasPrefix(lower, "hs"):
		return true
	case strings.HasPrefix(lower, "_hj"), strings.HasPrefix(lower, "ajs_"):
		return true
	}
	return false
}

func publicCookieHost(publicHost string) string {
	host := publicHost
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	if i := strings.Index(host, ":"); i >= 0 {
		host = host[:i]
	}
	return host
}

func expireCookie(dst http.Header, name, host string) {
	if name == "" || dst == nil {
		return
	}
	dst.Add("Set-Cookie", name+"=; Path=/; Max-Age=0")
	if host != "" {
		dst.Add("Set-Cookie", name+"=; Path=/; Max-Age=0; Domain="+host)
	}
}

// expireBloatedBrowserCookies drops oversized host cookies. When the Cookie
// header is already huge, expire every non-session cookie so the next request fits.
func expireBloatedBrowserCookies(dst http.Header, r *http.Request, publicHost string) {
	if r == nil || dst == nil {
		return
	}
	host := publicCookieHost(publicHost)
	raw := r.Header.Get("Cookie")
	nukeAll := len(raw) > 48*1024
	for _, c := range r.Cookies() {
		if c == nil || c.Name == "" {
			continue
		}
		lower := strings.ToLower(c.Name)
		if lower == "ct_session" {
			continue
		}
		if dropUpstreamCookie(c.Name) {
			expireCookie(dst, c.Name, host)
			continue
		}
		if nukeAll || len(c.Value) > maxBrowserCookieValueLen {
			expireCookie(dst, c.Name, host)
		}
	}
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
	if host != up && host != pub && !strings.HasSuffix(host, ".markhorseotool.com") && host != "markhorseotool.com" {
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
