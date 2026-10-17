package ahrefs

// Recipe refreshes a saved Ahrefs cookie. It does not log in with a password.
// The cookie is pasted in Storage. Each run opens the dashboard and keeps BSSESSID.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"panel-api/automation"
	"strings"
	"time"
)

var (
	dashboardURL = "https://app.ahrefs.com/dashboard"
	refreshURL   = "https://app.ahrefs.com/v4/loginCheckCompletion"
	httpWait     = 25 * time.Second
)

var errLoginRedirect = errors.New("ahrefs login redirect")

type Recipe struct{}

func (Recipe) PortalHost() string { return "ahrefs.com" }

func (Recipe) Check(ctx context.Context, cookies []automation.Cookie) (bool, []automation.Cookie, error) {
	if !hasName(cookies, "BSSESSID") {
		return false, nil, nil
	}
	if automation.UserAgent(ctx) == "" {
		return false, nil, automation.ErrUserAgent
	}
	updated, err := refresh(ctx, cookies)
	if err != nil || !hasName(updated, "BSSESSID") {
		if err == nil || errors.Is(err, errLoginRedirect) {
			return false, nil, automation.ErrCookieRejected
		}
		return false, nil, err
	}
	return true, updated, nil
}

func (Recipe) Login(context.Context, string, string) ([]automation.Cookie, error) {
	return nil, automation.ErrStoredCookie
}

func (Recipe) Capture(ctx context.Context, portal []automation.Cookie, _ string) ([]automation.Cookie, error) {
	if hasName(portal, "BSSESSID") {
		return portal, nil
	}
	updated, err := refresh(ctx, portal)
	if err != nil || !hasName(updated, "BSSESSID") {
		if err == nil || errors.Is(err, errLoginRedirect) {
			err = automation.ErrCookieRejected
		}
		return nil, err
	}
	return updated, nil
}

func refresh(ctx context.Context, cookies []automation.Cookie) ([]automation.Cookie, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	automation.SeedCookieJar(jar, cookies, "app.ahrefs.com")
	client := &http.Client{Timeout: httpWait, Jar: jar, CheckRedirect: stopLogin}
	if _, err := get(ctx, client, dashboardURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, refreshURL, strings.NewReader("null"))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", automation.UserAgent(ctx))
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://app.ahrefs.com")
	req.Header.Set("Referer", dashboardURL)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, errLoginRedirect
	}
	if _, err := get(ctx, client, dashboardURL); err != nil {
		return nil, err
	}
	live := automation.JarCookies(jar, []string{dashboardURL, "https://app.ahrefs.com/", "https://ahrefs.com/"})
	merged := widen(automation.MergeCookieSets(cookies, live))
	if !hasName(merged, "BSSESSID") {
		return nil, errLoginRedirect
	}
	return merged, nil
}

func get(ctx context.Context, client *http.Client, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", automation.UserAgent(ctx))
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Referer", dashboardURL)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp, nil
}

func stopLogin(req *http.Request, via []*http.Request) error {
	if strings.Contains(strings.ToLower(req.URL.Path), "/user/login") || strings.Contains(strings.ToLower(req.URL.Path), "/login") {
		return errLoginRedirect
	}
	if len(via) >= 5 {
		return errors.New("too many redirects")
	}
	return nil
}

func widen(cookies []automation.Cookie) []automation.Cookie {
	out := make([]automation.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		host := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(cookie.Domain)), ".")
		if host == "" || host == "app.ahrefs.com" || strings.HasSuffix(host, ".ahrefs.com") || host == "ahrefs.com" {
			cookie.Domain = ".ahrefs.com"
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
