package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// cookieJSON is the Chrome-export shape the recloud proxies already accept.
type cookieJSON struct {
	Domain         string   `json:"domain"`
	ExpirationDate *float64 `json:"expirationDate,omitempty"`
	HostOnly       bool     `json:"hostOnly"`
	HTTPOnly       bool     `json:"httpOnly"`
	Name           string   `json:"name"`
	Path           string   `json:"path"`
	SameSite       *string  `json:"sameSite"`
	Secure         bool     `json:"secure"`
	Session        bool     `json:"session"`
	StoreID        *string  `json:"storeId"`
	Value          string   `json:"value"`
}

func cookiesToJSON(cookies []cookieJSON) (string, error) {
	cleaned := make([]cookieJSON, 0, len(cookies))
	for _, cookie := range cookies {
		cookie.Name = strings.TrimSpace(cookie.Name)
		if cookie.Name == "" || cookie.Value == "" {
			continue
		}
		if strings.TrimSpace(cookie.Path) == "" {
			cookie.Path = "/"
		}
		cleaned = append(cleaned, cookie)
	}
	if len(cleaned) == 0 {
		return "", errAutomationLoginPage
	}
	raw, err := json.Marshal(cleaned)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func parseStoredCookies(raw string) []cookieJSON {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var cookies []cookieJSON
		if json.Unmarshal([]byte(raw), &cookies) == nil {
			return dropEmptyCookies(cookies)
		}
	}
	if strings.HasPrefix(raw, "{") {
		var wrapped struct {
			Cookies []cookieJSON `json:"cookies"`
		}
		if json.Unmarshal([]byte(raw), &wrapped) == nil && len(wrapped.Cookies) > 0 {
			return dropEmptyCookies(wrapped.Cookies)
		}
	}
	return headerCookies(raw)
}

func dropEmptyCookies(cookies []cookieJSON) []cookieJSON {
	out := make([]cookieJSON, 0, len(cookies))
	for _, cookie := range cookies {
		if strings.TrimSpace(cookie.Name) == "" || cookie.Value == "" {
			continue
		}
		out = append(out, cookie)
	}
	return out
}

func headerCookies(raw string) []cookieJSON {
	var out []cookieJSON
	for _, part := range strings.Split(raw, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" || value == "" {
			continue
		}
		out = append(out, cookieJSON{Name: name, Value: value, Path: "/", HostOnly: true})
	}
	return out
}

func mergeCookieSets(stored, live []cookieJSON) []cookieJSON {
	seen := map[string]struct{}{}
	out := make([]cookieJSON, 0, len(stored)+len(live))
	add := func(batch []cookieJSON) {
		for _, cookie := range batch {
			if strings.TrimSpace(cookie.Name) == "" || cookie.Value == "" {
				continue
			}
			key := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(cookie.Domain), ".")) + "|" + cookie.Name + "|" + cookie.Path
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, cookie)
		}
	}
	add(live)
	add(stored)
	return out
}

func seedCookieJar(jar http.CookieJar, cookies []cookieJSON, fallbackDomain string) {
	byURL := map[string][]*http.Cookie{}
	for _, cookie := range cookies {
		if strings.TrimSpace(cookie.Name) == "" || cookie.Value == "" {
			continue
		}
		domain := strings.TrimSpace(cookie.Domain)
		if domain == "" {
			domain = fallbackDomain
		}
		host := strings.TrimPrefix(domain, ".")
		path := cookie.Path
		if path == "" {
			path = "/"
		}
		hc := &http.Cookie{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Domain:   domain,
			Path:     path,
			HttpOnly: cookie.HTTPOnly,
			Secure:   cookie.Secure,
		}
		if cookie.ExpirationDate != nil && *cookie.ExpirationDate > 0 {
			hc.Expires = time.Unix(int64(*cookie.ExpirationDate), 0)
		}
		raw := "https://" + host + path
		byURL[raw] = append(byURL[raw], hc)
	}
	for raw, list := range byURL {
		parsed, err := url.Parse(raw)
		if err != nil {
			continue
		}
		jar.SetCookies(parsed, list)
	}
}

func httpCookieToJSON(cookie *http.Cookie, fallbackHost string) cookieJSON {
	domain := cookie.Domain
	if domain == "" {
		domain = fallbackHost
	}
	hostOnly := !strings.HasPrefix(domain, ".")
	session := cookie.Expires.IsZero()
	var exp *float64
	if !session {
		value := float64(cookie.Expires.Unix())
		exp = &value
	}
	path := cookie.Path
	if path == "" {
		path = "/"
	}
	return cookieJSON{
		Domain:         domain,
		ExpirationDate: exp,
		HostOnly:       hostOnly,
		HTTPOnly:       cookie.HttpOnly,
		Name:           cookie.Name,
		Path:           path,
		Secure:         cookie.Secure,
		Session:        session,
		Value:          cookie.Value,
	}
}

func jarCookies(jar http.CookieJar, rawURLs []string) []cookieJSON {
	seen := map[string]struct{}{}
	var out []cookieJSON
	for _, raw := range rawURLs {
		parsed, err := url.Parse(raw)
		if err != nil {
			continue
		}
		for _, cookie := range jar.Cookies(parsed) {
			if cookie.Name == "" {
				continue
			}
			key := cookie.Name + "|" + cookie.Domain + "|" + cookie.Path
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, httpCookieToJSON(cookie, parsed.Host))
		}
	}
	return out
}

func cookieValues(cookies []cookieJSON) []string {
	values := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie.Value != "" {
			values = append(values, cookie.Value)
		}
	}
	return values
}
