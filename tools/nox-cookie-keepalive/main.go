// Standalone NoxTools cookie keepalive — NOT a proxy tool.
// Hits /secure/member on an interval so PHPSESSID stays warm, then saves cookies.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultURL      = "https://noxtools.com/secure/member"
	defaultInterval = 15 * time.Minute
	defaultUA       = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"
)

type browserCookie struct {
	Domain   string `json:"domain"`
	HostOnly bool   `json:"hostOnly"`
	HTTPOnly bool   `json:"httpOnly"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Secure   bool   `json:"secure"`
	Session  bool   `json:"session"`
	Value    string `json:"value"`
}

func main() {
	cookieFile := envOr("NOX_COOKIE_FILE", "cookie.json")
	warmURL := envOr("NOX_WARM_URL", defaultURL)
	ua := envOr("NOX_UA", defaultUA)
	interval := defaultInterval
	if v := strings.TrimSpace(os.Getenv("NOX_INTERVAL")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d >= time.Minute {
			interval = d
		}
	}
	once := strings.EqualFold(os.Getenv("NOX_ONCE"), "1") || (len(os.Args) > 1 && os.Args[1] == "once")

	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[nox-keep] ")
	log.Printf("cookie_file=%s url=%s interval=%s", cookieFile, warmURL, interval)

	if err := runOnce(cookieFile, warmURL, ua); err != nil {
		log.Printf("first run: %v", err)
	} else {
		log.Printf("first run ok")
	}
	if once {
		return
	}
	for {
		time.Sleep(interval)
		if err := runOnce(cookieFile, warmURL, ua); err != nil {
			log.Printf("warm failed: %v", err)
			continue
		}
		log.Printf("warm ok")
	}
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func runOnce(cookieFile, warmURL, ua string) error {
	cookies, err := loadCookies(cookieFile)
	if err != nil {
		return err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return err
	}
	u, err := url.Parse(warmURL)
	if err != nil {
		return err
	}
	jar.SetCookies(u, toHTTPCookies(cookies, u))

	client := &http.Client{
		Jar:     jar,
		Timeout: 45 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	req, err := http.NewRequest(http.MethodGet, warmURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", "https://noxtools.com/")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()

	merged := mergeCookies(cookies, jar.Cookies(u), resp.Cookies())
	landed := ""
	if resp.Request != nil && resp.Request.URL != nil {
		landed = resp.Request.URL.String()
	}
	if err := assertNoxSessionAlive(resp.StatusCode, landed, body); err != nil {
		// Do not overwrite a good cookie file with a dead/login response.
		return err
	}
	if err := saveCookies(cookieFile, merged); err != nil {
		return err
	}
	log.Printf("session_ok status=%d bytes=%d landed=%s php=%s", resp.StatusCode, len(body), landed, short(merged, "PHPSESSID"))
	return nil
}

func assertNoxSessionAlive(status int, landed string, body []byte) error {
	low := strings.ToLower(string(body))
	loc := strings.ToLower(landed)
	if status >= 400 {
		return fmt.Errorf("HTTP %d", status)
	}
	if strings.Contains(loc, "/login") || strings.Contains(loc, "signin") {
		return fmt.Errorf("LOGGED_OUT landed_on_login status=%d loc=%s", status, landed)
	}
	if strings.Contains(low, "name=\"amember_login\"") || strings.Contains(low, "id=\"amember_login\"") ||
		(strings.Contains(low, "amember_login") && strings.Contains(low, "amember_pass")) {
		return fmt.Errorf("LOGGED_OUT login_form status=%d bytes=%d", status, len(body))
	}
	// Require a positive member-area signal. Plain 200 is not enough (homepage also 200).
	ok := strings.Contains(low, "member-main-subscriptions") ||
		strings.Contains(low, "/secure/member/payment-history") ||
		(strings.Contains(loc, "/secure/member") && (strings.Contains(low, "/logout") || strings.Contains(low, ">logout<")))
	if !ok {
		return fmt.Errorf("LOGGED_OUT missing_member_markers status=%d bytes=%d loc=%s", status, len(body), landed)
	}
	return nil
}

func short(cookies []browserCookie, name string) string {
	for _, c := range cookies {
		if c.Name == name {
			v := c.Value
			if len(v) > 10 {
				return v[:6] + "…" + v[len(v)-4:]
			}
			return v
		}
	}
	return "-"
}

func loadCookies(path string) ([]browserCookie, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(data))
	var list []browserCookie
	if err := json.Unmarshal([]byte(trimmed), &list); err == nil && len(list) > 0 {
		return list, nil
	}
	var wrap struct {
		Cookies []browserCookie `json:"cookies"`
	}
	if err := json.Unmarshal([]byte(trimmed), &wrap); err == nil && len(wrap.Cookies) > 0 {
		return wrap.Cookies, nil
	}
	return nil, fmt.Errorf("unsupported cookie format in %s", path)
}

func saveCookies(path string, cookies []browserCookie) error {
	keep := make([]browserCookie, 0, 2)
	seen := map[string]browserCookie{}
	for _, c := range cookies {
		if c.Name == "PHPSESSID" || c.Name == "amember_nr" {
			seen[c.Name] = c
		}
	}
	for _, name := range []string{"amember_nr", "PHPSESSID"} {
		if c, ok := seen[name]; ok && c.Value != "" {
			if c.Domain == "" {
				c.Domain = ".noxtools.com"
			}
			if c.Path == "" {
				c.Path = "/"
			}
			c.Secure = true
			c.HTTPOnly = true
			c.Session = true
			keep = append(keep, c)
		}
	}
	b, err := json.MarshalIndent(keep, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func toHTTPCookies(in []browserCookie, u *url.URL) []*http.Cookie {
	out := make([]*http.Cookie, 0, len(in))
	for _, c := range in {
		out = append(out, &http.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Path:     firstNonEmpty(c.Path, "/"),
			Domain:   strings.TrimPrefix(c.Domain, "."),
			Secure:   c.Secure,
			HttpOnly: c.HTTPOnly,
		})
	}
	_ = u
	return out
}

func mergeCookies(base []browserCookie, jarCookies, setCookies []*http.Cookie) []browserCookie {
	m := map[string]browserCookie{}
	for _, c := range base {
		m[c.Name] = c
	}
	apply := func(list []*http.Cookie) {
		for _, c := range list {
			if c == nil || c.Value == "" || strings.EqualFold(c.Value, "deleted") {
				continue
			}
			if c.Name != "PHPSESSID" && c.Name != "amember_nr" {
				continue
			}
			prev := m[c.Name]
			prev.Name = c.Name
			prev.Value = c.Value
			if c.Path != "" {
				prev.Path = c.Path
			}
			if c.Domain != "" {
				prev.Domain = c.Domain
				if !strings.HasPrefix(prev.Domain, ".") {
					prev.Domain = "." + prev.Domain
				}
			}
			if prev.Domain == "" {
				prev.Domain = ".noxtools.com"
			}
			if prev.Path == "" {
				prev.Path = "/"
			}
			prev.Secure = true
			prev.HTTPOnly = true
			prev.Session = true
			m[c.Name] = prev
		}
	}
	apply(jarCookies)
	apply(setCookies)
	out := make([]browserCookie, 0, len(m))
	for _, name := range []string{"amember_nr", "PHPSESSID"} {
		if c, ok := m[name]; ok {
			out = append(out, c)
		}
	}
	return out
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
