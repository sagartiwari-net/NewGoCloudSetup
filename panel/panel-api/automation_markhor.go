package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	markhorBaseURL   = "https://markhorseotool.com"
	markhorLoginURL  = "https://markhorseotool.com/login"
	markhorMemberURL = "https://markhorseotool.com/member/"
	markhorUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	markhorHTTPWait  = 25 * time.Second
)

var markhorCSRF = regexp.MustCompile(`(?i)name=["']csrf_token["'][^>]*value=["']([^"']+)["']|value=["']([^"']+)["'][^>]*name=["']csrf_token["']`)

type markhorRecipe struct{}

func (markhorRecipe) Check(ctx context.Context, cookies []cookieJSON) (bool, []cookieJSON, error) {
	if len(cookies) == 0 {
		return false, nil, nil
	}
	client, jar, err := newMarkhorClient(12)
	if err != nil {
		return false, nil, errAutomationTimeout
	}
	seedCookieJar(jar, cookies, "markhorseotool.com")
	loggedIn, err := markhorLoggedIn(ctx, client)
	if err != nil {
		return false, nil, err
	}
	if !loggedIn {
		return false, nil, nil
	}
	updated := mergeCookieSets(cookies, filterMarkhorCookies(jarCookies(jar, []string{
		markhorBaseURL + "/", markhorLoginURL, markhorMemberURL,
	})))
	if len(updated) == 0 {
		updated = cookies
	}
	return true, updated, nil
}

func (markhorRecipe) Login(ctx context.Context, username, password string) ([]cookieJSON, error) {
	client, jar, err := newMarkhorClient(12)
	if err != nil {
		return nil, errAutomationTimeout
	}
	token, err := markhorFetchCSRF(ctx, client)
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("csrf_token", token)
	form.Set("username", username)
	form.Set("password", password)
	html, landed, err := markhorPostForm(ctx, client, markhorLoginURL, form)
	if err != nil {
		return nil, err
	}
	if markhorShowsLogin(html, landed) {
		return nil, errAutomationLoginPage
	}
	loggedIn, err := markhorLoggedIn(ctx, client)
	if err != nil {
		return nil, err
	}
	if !loggedIn {
		return nil, errAutomationLoginPage
	}
	cookies := filterMarkhorCookies(jarCookies(jar, []string{
		markhorBaseURL + "/", markhorLoginURL, markhorMemberURL,
	}))
	if len(cookies) == 0 {
		return nil, errAutomationLoginPage
	}
	return cookies, nil
}

func (markhorRecipe) Capture(ctx context.Context, portal []cookieJSON, upstreamURL string) ([]cookieJSON, error) {
	upstreamURL = strings.TrimSpace(upstreamURL)
	parsed, err := url.Parse(upstreamURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errAutomationUpstream
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, errAutomationTimeout
	}
	seedCookieJar(jar, portal, "markhorseotool.com")
	client := &http.Client{
		Timeout: markhorHTTPWait,
		Jar:     jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 15 {
				return errAutomationLoginPage
			}
			return nil
		},
	}
	html, landed, err := markhorGet(ctx, client, upstreamURL, markhorMemberURL)
	if err != nil {
		if errors.Is(err, errAutomationLoginPage) {
			return nil, err
		}
		return nil, markhorTransportErr(ctx, err)
	}
	if markhorShowsLogin(html, landed) {
		return nil, errAutomationLoginPage
	}
	if markhorShowsDenied(html) {
		return nil, errAutomationDenied
	}
	host := landedHost(landed)
	if host == "" {
		host = strings.ToLower(parsed.Host)
	}
	live := markhorCookiesForHost(jar, host)
	if !markhorAddedCookie(portal, live) {
		if len(portal) == 0 {
			return nil, errAutomationLoginPage
		}
		return portal, nil
	}
	return live, nil
}

func newMarkhorClient(maxRedirects int) (*http.Client, *cookiejar.Jar, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, nil, err
	}
	client := &http.Client{
		Timeout: markhorHTTPWait,
		Jar:     jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errAutomationLoginPage
			}
			return nil
		},
	}
	return client, jar, nil
}

func markhorLoggedIn(ctx context.Context, client *http.Client) (bool, error) {
	html, landed, err := markhorGet(ctx, client, markhorMemberURL, "")
	if err != nil {
		if errors.Is(err, errAutomationLoginPage) {
			return false, nil
		}
		return false, markhorTransportErr(ctx, err)
	}
	if markhorShowsLogin(html, landed) {
		return false, nil
	}
	return markhorShowsDashboard(html, landed), nil
}

func markhorFetchCSRF(ctx context.Context, client *http.Client) (string, error) {
	html, _, err := markhorGet(ctx, client, markhorLoginURL, "")
	if err != nil {
		if errors.Is(err, errAutomationLoginPage) {
			return "", errAutomationCSRF
		}
		return "", markhorTransportErr(ctx, err)
	}
	match := markhorCSRF.FindStringSubmatch(html)
	if len(match) == 0 {
		return "", errAutomationCSRF
	}
	token := match[1]
	if token == "" && len(match) > 2 {
		token = match[2]
	}
	if strings.TrimSpace(token) == "" {
		return "", errAutomationCSRF
	}
	return token, nil
}

func markhorPostForm(ctx context.Context, client *http.Client, rawURL string, form url.Values) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", errAutomationTimeout
	}
	setMarkhorHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", markhorBaseURL)
	req.Header.Set("Referer", markhorLoginURL)
	return markhorRead(client, req, rawURL)
}

func markhorGet(ctx context.Context, client *http.Client, rawURL, referer string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", errAutomationTimeout
	}
	setMarkhorHeaders(req)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	return markhorRead(client, req, rawURL)
}

func markhorRead(client *http.Client, req *http.Request, rawURL string) (string, string, error) {
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", "", err
	}
	landed := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		landed = resp.Request.URL.String()
	}
	return string(body), landed, nil
}

func setMarkhorHeaders(req *http.Request) {
	req.Header.Set("User-Agent", markhorUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
}

func markhorTransportErr(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errAutomationTimeout
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline") || strings.Contains(msg, "timed out") {
		return errAutomationTimeout
	}
	return errAutomationTimeout
}

func markhorShowsLogin(html, pageURL string) bool {
	lower := strings.ToLower(html)
	urlLower := strings.ToLower(pageURL)
	if strings.Contains(urlLower, "/login") {
		return true
	}
	if strings.Contains(lower, `name="username"`) && strings.Contains(lower, `name="password"`) {
		return true
	}
	return strings.Contains(lower, "member login") && strings.Contains(lower, "sign in")
}

func markhorShowsDashboard(html, pageURL string) bool {
	lower := strings.ToLower(html)
	urlLower := strings.ToLower(pageURL)
	if strings.Contains(urlLower, "/login") || !strings.Contains(urlLower, "/member") {
		return false
	}
	if strings.Contains(lower, "logout") || strings.Contains(lower, "dashboard") || strings.Contains(lower, "my tools") || strings.Contains(lower, "welcome") {
		return true
	}
	return !strings.Contains(lower, `name="password"`)
}

func markhorShowsDenied(html string) bool {
	lower := strings.ToLower(html)
	if !strings.Contains(lower, "access denied") {
		return false
	}
	return strings.Contains(lower, "active subscription") || strings.Contains(lower, "do not have an active")
}

func filterMarkhorCookies(cookies []cookieJSON) []cookieJSON {
	var out []cookieJSON
	for _, cookie := range cookies {
		if cookie.Name == "" || cookie.Value == "" {
			continue
		}
		if strings.Contains(strings.ToLower(cookie.Domain), "markhorseotool.com") {
			out = append(out, cookie)
		}
	}
	return out
}

func markhorCookiesForHost(jar http.CookieJar, host string) []cookieJSON {
	host = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" {
		return nil
	}
	hosts := []string{host}
	if strings.HasSuffix(host, ".markhorseotool.com") {
		hosts = append(hosts, "markhorseotool.com")
	}
	var urls []string
	for _, item := range hosts {
		urls = append(urls, "https://"+item+"/", "http://"+item+"/")
	}
	return jarCookies(jar, urls)
}

func markhorAddedCookie(portal, live []cookieJSON) bool {
	seen := map[string]string{}
	for _, cookie := range portal {
		seen[cookie.Name+"|"+strings.ToLower(strings.TrimPrefix(cookie.Domain, "."))] = cookie.Value
	}
	for _, cookie := range live {
		key := cookie.Name + "|" + strings.ToLower(strings.TrimPrefix(cookie.Domain, "."))
		prev, ok := seen[key]
		if !ok || prev != cookie.Value {
			return true
		}
	}
	return false
}

func landedHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Host)
}
