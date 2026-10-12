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

func logUberAuthCookies(path, header string) {
	if path != "/" && path != "/dashboard" && !strings.Contains(path, "/analyze") && !strings.Contains(path, "/en/") {
		return
	}
	pairs := uberCookiePairs(header)
	_, hasID := pairs["id"]
	_, hasState := pairs["state"]
	log.Printf("[UBER] upstream %s cookies=%d id=%v state=%v json=%v", path, len(pairs), hasID, hasState, strings.HasPrefix(strings.TrimSpace(header), "{"))
}

// rewriteUpstreamSetCookies keeps session cookies on the proxy host.
// Leaving Domain=app.neilpatel.com makes the browser drop them.
func rewriteUpstreamSetCookies(resp *http.Response) {
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		return
	}
	resp.Header.Del("Set-Cookie")
	for _, c := range cookies {
		if c == nil || c.Name == "" {
			continue
		}
		c.Domain = ""
		if c.Path == "" {
			c.Path = "/"
		}
		if c.SameSite == http.SameSiteDefaultMode {
			c.SameSite = http.SameSiteLaxMode
		}
		resp.Header.Add("Set-Cookie", c.String())
	}
}

// seedUberAuthCookies puts id and state on the browser. The app reads them
// from document.cookie; sending them only upstream still paints Sign in.
func seedUberAuthCookies(resp *http.Response, header string) {
	pairs := uberCookiePairs(header)
	for _, name := range []string{"id", "state"} {
		value := pairs[name]
		if value == "" {
			continue
		}
		c := &http.Cookie{
			Name:     name,
			Value:    value,
			Path:     "/",
			MaxAge:   7 * 24 * 3600,
			SameSite: http.SameSiteLaxMode,
			HttpOnly: false,
		}
		resp.Header.Add("Set-Cookie", c.String())
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
