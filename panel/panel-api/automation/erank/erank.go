package erank

// Recipe logs in at members.erank.com with Laravel Sanctum and stores sid_er.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"panel-api/automation"
	"strings"
	"time"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

var (
	origin   = "https://members.erank.com"
	httpWait = 25 * time.Second
)

type Recipe struct{}

func (Recipe) PortalHost() string { return "erank.com" }

func (Recipe) Check(ctx context.Context, cookies []automation.Cookie) (bool, []automation.Cookie, error) {
	if !hasName(cookies, "sid_er") {
		return false, nil, nil
	}
	client, err := clientWith(cookies)
	if err != nil {
		return false, nil, err
	}
	ok, err := loggedIn(ctx, client)
	if err != nil || !ok {
		return false, nil, err
	}
	return true, collect(client.Jar, cookies), nil
}

func (Recipe) Login(ctx context.Context, username, password string) ([]automation.Cookie, error) {
	client, err := clientWith(nil)
	if err != nil {
		return nil, err
	}
	if err := get(ctx, client, origin+"/login"); err != nil {
		return nil, err
	}
	if err := get(ctx, client, origin+"/sanctum/csrf-cookie"); err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]string{"email": username, "password": password})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, origin+"/api/login", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	setAPI(req, xsrf(client.Jar))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", origin+"/login")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusNotFound, http.StatusUnprocessableEntity:
		return nil, erankStatus(origin+"/api/login", raw, automation.ErrRejected)
	case http.StatusForbidden:
		return nil, erankStatus(origin+"/api/login", raw, errors.New("eRank refused this login"))
	case http.StatusOK, http.StatusCreated:
	case 419:
		return nil, automation.ErrCSRF
	default:
		return nil, automation.ErrLoginPage
	}
	ok, err := loggedIn(ctx, client)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, automation.ErrRejected
	}
	_ = get(ctx, client, origin+"/dashboard")
	cookies := collect(client.Jar, nil)
	if !hasName(cookies, "sid_er") {
		return nil, automation.ErrNoCookie
	}
	return cookies, nil
}

func (Recipe) Capture(_ context.Context, portal []automation.Cookie, _ string) ([]automation.Cookie, error) {
	if !hasName(portal, "sid_er") {
		return nil, automation.ErrNoCookie
	}
	return portal, nil
}

func clientWith(cookies []automation.Cookie) (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	automation.SeedCookieJar(jar, cookies, "members.erank.com")
	return &http.Client{Timeout: httpWait, Jar: jar}, nil
}

func loggedIn(ctx context.Context, client *http.Client) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/api/auth/status", nil)
	if err != nil {
		return false, err
	}
	setAPI(req, xsrf(client.Jar))
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return false, nil
	}
	lower := strings.ToLower(string(raw))
	return strings.Contains(lower, `"logged_in":true`) || strings.Contains(lower, `"logged_in": true`), nil
}

func get(ctx context.Context, client *http.Client, rawURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 400 {
		return automation.ErrLoginPage
	}
	return nil
}

func setAPI(req *http.Request, token string) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("X-User-Agent", "erank-app/3.0")
	req.Header.Set("Origin", origin)
	req.Header.Set("Referer", origin+"/login")
	if token != "" {
		req.Header.Set("X-XSRF-TOKEN", token)
	}
}

func erankStatus(rawURL string, body []byte, fallback error) error {
	msg := erankMessage(body)
	if msg == "" {
		return automation.At(fallback, rawURL)
	}
	return automation.At(errors.New(msg), rawURL)
}

func erankMessage(raw []byte) string {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return ""
	}
	var asString string
	if json.Unmarshal(raw, &asString) == nil && strings.TrimSpace(asString) != "" {
		text = asString
	} else {
		var obj struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(raw, &obj) == nil && strings.TrimSpace(obj.Message) != "" {
			text = obj.Message
		}
	}
	text = strings.Join(strings.Fields(text), " ")
	lower := strings.ToLower(text)
	if strings.Contains(lower, "<html") || strings.Contains(lower, "<!doctype") {
		return ""
	}
	if len(text) > 160 {
		text = text[:160]
	}
	return text
}

func xsrf(jar http.CookieJar) string {
	if jar == nil {
		return ""
	}
	parsed, err := url.Parse(origin + "/")
	if err != nil {
		return ""
	}
	for _, cookie := range jar.Cookies(parsed) {
		if cookie.Name == "XSRF-TOKEN" && cookie.Value != "" {
			decoded, err := url.QueryUnescape(cookie.Value)
			if err != nil {
				return cookie.Value
			}
			return decoded
		}
	}
	return ""
}

func collect(jar http.CookieJar, stored []automation.Cookie) []automation.Cookie {
	live := automation.JarCookies(jar, []string{origin + "/", origin + "/dashboard", origin + "/api/auth/status"})
	merged := automation.MergeCookieSets(stored, live)
	out := make([]automation.Cookie, 0, len(merged))
	for _, cookie := range merged {
		host := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(cookie.Domain)), ".")
		if host == "" || host == "members.erank.com" || strings.HasSuffix(host, ".erank.com") || host == "erank.com" {
			cookie.Domain = ".erank.com"
			cookie.HostOnly = false
			if cookie.Path == "" {
				cookie.Path = "/"
			}
		}
		out = append(out, cookie)
	}
	return out
}

func hasName(cookies []automation.Cookie, name string) bool {
	for _, cookie := range cookies {
		if strings.EqualFold(cookie.Name, name) && cookie.Value != "" {
			return true
		}
	}
	return false
}
