package automation

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Cookie is the Chrome-export shape the recloud proxies already accept.
type Cookie struct {
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

func CookiesToJSON(cookies []Cookie) (string, error) {
	cleaned := make([]Cookie, 0, len(cookies))
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
		return "", ErrLoginPage
	}
	raw, err := json.Marshal(cleaned)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func ParseStoredCookies(raw string) []Cookie {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var cookies []Cookie
		if json.Unmarshal([]byte(raw), &cookies) == nil {
			return dropEmptyCookies(cookies)
		}
	}
	if strings.HasPrefix(raw, "{") {
		var wrapped struct {
			Cookies []Cookie `json:"cookies"`
		}
		if json.Unmarshal([]byte(raw), &wrapped) == nil && len(wrapped.Cookies) > 0 {
			return dropEmptyCookies(wrapped.Cookies)
		}
	}
	return headerCookies(raw)
}

func dropEmptyCookies(cookies []Cookie) []Cookie {
	out := make([]Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		if strings.TrimSpace(cookie.Name) == "" || cookie.Value == "" {
			continue
		}
		out = append(out, cookie)
	}
	return out
}

func headerCookies(raw string) []Cookie {
	var out []Cookie
	for _, part := range strings.Split(raw, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" || value == "" {
			continue
		}
		out = append(out, Cookie{Name: name, Value: value, Path: "/", HostOnly: true})
	}
	return out
}

func MergeCookieSets(stored, live []Cookie) []Cookie {
	seen := map[string]struct{}{}
	out := make([]Cookie, 0, len(stored)+len(live))
	add := func(batch []Cookie) {
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

func SeedCookieJar(jar http.CookieJar, cookies []Cookie, fallbackDomain string) {
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

func httpCookieToJSON(cookie *http.Cookie, fallbackHost string) Cookie {
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
	return Cookie{
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

func JarCookies(jar http.CookieJar, rawURLs []string) []Cookie {
	seen := map[string]struct{}{}
	var out []Cookie
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

func CookieValues(cookies []Cookie) []string {
	values := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie.Value != "" {
			values = append(values, cookie.Value)
		}
	}
	return values
}
