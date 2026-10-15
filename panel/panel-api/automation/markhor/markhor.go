package markhor

// Recipe is the Markhor portal HTTP login. Azad and Toolzbuy will be sibling folders, not more files here.

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"panel-api/automation"
	"regexp"
	"strings"
	"sync"
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

type Recipe struct{}

func (Recipe) Check(ctx context.Context, cookies []automation.Cookie) (bool, []automation.Cookie, error) {
	if len(cookies) == 0 {
		return false, nil, nil
	}
	client, jar, record, err := newMarkhorClient(12)
	if err != nil {
		return false, nil, automation.ErrTimeout
	}
	seedMarkhorPortal(jar, cookies)
	loggedIn, _, err := markhorLoggedIn(ctx, client)
	if err != nil {
		return false, nil, err
	}
	if !loggedIn {
		return false, nil, nil
	}
	updated := automation.MergeCookieSets(storedMarkhorCookies(cookies), markhorExportCookies(jar, record, []string{
		markhorBaseURL + "/", markhorLoginURL, markhorMemberURL,
	}))
	if len(updated) == 0 {
		updated = storedMarkhorCookies(cookies)
	}
	return true, updated, nil
}

func (Recipe) PortalHost() string { return "markhorseotool.com" }

func (Recipe) Login(ctx context.Context, username, password string) ([]automation.Cookie, error) {
	client, jar, record, err := newMarkhorClient(12)
	if err != nil {
		return nil, automation.ErrTimeout
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
	if markhorRejected(html) {
		return nil, automation.At(automation.ErrRejected, landed)
	}
	loggedIn, memberURL, err := markhorLoggedIn(ctx, client)
	if err != nil {
		return nil, err
	}
	if !loggedIn {
		where := memberURL
		if where == "" {
			where = landed
		}
		return nil, automation.At(automation.ErrLoginPage, where)
	}
	cookies := markhorExportCookies(jar, record, []string{
		markhorBaseURL + "/", markhorLoginURL, markhorMemberURL,
	})
	log.Printf("[automation] markhor login page=%s cookies=%s", automation.Where(memberURL), automation.Names(cookies))
	if len(cookies) == 0 {
		return nil, automation.At(automation.ErrLoginPage, memberURL)
	}
	return cookies, nil
}

func (Recipe) Capture(ctx context.Context, portal []automation.Cookie, upstreamURL string) ([]automation.Cookie, error) {
	upstreamURL = strings.TrimSpace(upstreamURL)
	parsed, err := url.Parse(upstreamURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, automation.ErrUpstream
	}
	client, jar, record, err := newMarkhorClient(15)
	if err != nil {
		return nil, automation.ErrTimeout
	}
	seedMarkhorPortal(jar, portal)
	html, landed, err := markhorGet(ctx, client, upstreamURL, markhorMemberURL)
	if err != nil {
		if errors.Is(err, automation.ErrLoginPage) {
			return nil, err
		}
		return nil, markhorTransportErr(ctx, err)
	}
	if markhorShowsLogin(html, landed) {
		return nil, automation.At(automation.ErrLoginPage, landed)
	}
	if markhorShowsDenied(html) {
		return nil, automation.ErrDenied
	}
	host := landedHost(landed)
	if host == "" {
		host = strings.ToLower(parsed.Host)
	}
	live := markhorExportCookies(jar, record, []string{
		"https://" + host + "/",
		"https://markhorseotool.com/",
	})
	log.Printf("[automation] markhor capture page=%s cookies=%s new=%t", automation.Where(landed), automation.Names(live), markhorAddedCookie(portal, live))
	if !markhorAddedCookie(portal, live) {
		if len(portal) == 0 {
			return nil, automation.At(automation.ErrNoCookie, landed)
		}
		return storedMarkhorCookies(portal), nil
	}
	return live, nil
}

func newMarkhorClient(maxRedirects int) (*http.Client, *cookiejar.Jar, *markhorRecord, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, nil, nil, err
	}
	record := &markhorRecord{}
	client := &http.Client{
		Timeout: markhorHTTPWait,
		Jar:     jar,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			resp, err := http.DefaultTransport.RoundTrip(req)
			if err != nil {
				log.Printf("[automation] markhor hop %s failed before a response", automation.Where(requestURL(req)))
				return resp, err
			}
			if resp != nil {
				record.note(resp.Cookies())
				log.Printf("[automation] markhor hop %s status=%d set=%s", automation.Where(requestURL(req)), resp.StatusCode, cookieHeaderNames(resp.Cookies()))
			}
			return resp, err
		}),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			log.Printf("[automation] markhor redirect %s -> %s (%d)", automation.Where(requestURL(via[len(via)-1])), automation.Where(requestURL(req)), len(via))
			if len(via) >= maxRedirects {
				log.Printf("[automation] markhor stopped after %d redirects at %s", len(via), automation.Where(requestURL(req)))
				return automation.At(automation.ErrLoginPage, requestURL(req))
			}
			return nil
		},
	}
	return client, jar, record, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type markhorRecord struct {
	mu    sync.Mutex
	saved map[string]http.Cookie
}

func (r *markhorRecord) note(cookies []*http.Cookie) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.saved == nil {
		r.saved = map[string]http.Cookie{}
	}
	for _, cookie := range cookies {
		if cookie == nil || cookie.Name == "" {
			continue
		}
		copy := *cookie
		if copy.Expires.IsZero() && copy.MaxAge > 0 {
			copy.Expires = time.Now().Add(time.Duration(copy.MaxAge) * time.Second)
		}
		r.saved[cookie.Name+"|"+cookie.Value] = copy
	}
}

func (r *markhorRecord) get(name, value string) (http.Cookie, bool) {
	if r == nil {
		return http.Cookie{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cookie, ok := r.saved[name+"|"+value]
	return cookie, ok
}

func markhorExportCookies(jar *cookiejar.Jar, record *markhorRecord, rawURLs []string) []automation.Cookie {
	var out []automation.Cookie
	for _, cookie := range automation.JarCookies(jar, rawURLs) {
		if saved, ok := record.get(cookie.Name, cookie.Value); ok {
			if strings.TrimSpace(saved.Domain) != "" {
				cookie.Domain = saved.Domain
			}
			if saved.Path != "" {
				cookie.Path = saved.Path
			}
			cookie.HTTPOnly = saved.HttpOnly
			cookie.Secure = saved.Secure
			cookie.SameSite = markhorSameSite(saved.SameSite)
			if !saved.Expires.IsZero() {
				value := float64(saved.Expires.Unix())
				cookie.ExpirationDate = &value
				cookie.Session = false
			}
		} else {
			cookie = markhorPortalDefaults(cookie)
		}
		out = append(out, cookie)
	}
	return filterMarkhorCookies(normalizeMarkhorCookies(out))
}

func markhorSameSite(mode http.SameSite) *string {
	var label string
	switch mode {
	case http.SameSiteStrictMode:
		label = "strict"
	case http.SameSiteLaxMode:
		label = "lax"
	case http.SameSiteNoneMode:
		label = "none"
	default:
		return nil
	}
	return &label
}

func storedMarkhorCookies(cookies []automation.Cookie) []automation.Cookie {
	out := make([]automation.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		out = append(out, markhorPortalDefaults(cookie))
	}
	return filterMarkhorCookies(normalizeMarkhorCookies(out))
}

func markhorPortalDefaults(cookie automation.Cookie) automation.Cookie {
	switch strings.ToLower(cookie.Name) {
	case "phpsessid", "mht_session":
	default:
		return cookie
	}
	cookie.Domain = ".markhorseotool.com"
	cookie.HostOnly = false
	cookie.Secure = true
	cookie.HTTPOnly = true
	if cookie.Path == "" {
		cookie.Path = "/"
	}
	if cookie.SameSite == nil {
		label := "lax"
		cookie.SameSite = &label
	}
	return cookie
}

func normalizeMarkhorCookies(cookies []automation.Cookie) []automation.Cookie {
	out := make([]automation.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		bare := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(cookie.Domain)), ".")
		if bare == "" || bare == "markhorseotool.com" || strings.HasSuffix(bare, ".markhorseotool.com") {
			cookie.Domain = ".markhorseotool.com"
			cookie.HostOnly = false
		}
		out = append(out, cookie)
	}
	return out
}

func markhorLoggedIn(ctx context.Context, client *http.Client) (bool, string, error) {
	html, landed, err := markhorGet(ctx, client, markhorMemberURL, "")
	if err != nil {
		if errors.Is(err, automation.ErrLoginPage) {
			return false, landed, nil
		}
		return false, "", markhorTransportErr(ctx, err)
	}
	if markhorRejected(html) || markhorShowsLogin(html, landed) {
		return false, landed, nil
	}
	return markhorShowsDashboard(html, landed), landed, nil
}

func markhorFetchCSRF(ctx context.Context, client *http.Client) (string, error) {
	html, _, err := markhorGet(ctx, client, markhorLoginURL, "")
	if err != nil {
		if errors.Is(err, automation.ErrLoginPage) {
			return "", automation.ErrCSRF
		}
		return "", markhorTransportErr(ctx, err)
	}
	match := markhorCSRF.FindStringSubmatch(html)
	if len(match) == 0 {
		return "", automation.ErrCSRF
	}
	token := match[1]
	if token == "" && len(match) > 2 {
		token = match[2]
	}
	if strings.TrimSpace(token) == "" {
		return "", automation.ErrCSRF
	}
	return token, nil
}

func markhorPostForm(ctx context.Context, client *http.Client, rawURL string, form url.Values) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", automation.ErrTimeout
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
		return "", "", automation.ErrTimeout
	}
	setMarkhorHeaders(req)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	return markhorRead(client, req, rawURL)
}

func markhorRead(client *http.Client, req *http.Request, rawURL string) (string, string, error) {
	log.Printf("[automation] markhor %s %s", req.Method, automation.Where(rawURL))
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[automation] markhor %s %s stopped: %s", req.Method, automation.Where(rawURL), safeMarkhorErr(err))
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
	log.Printf("[automation] markhor %s landed %s status=%d bytes=%d login=%t dashboard=%t rejected=%t", req.Method, automation.Where(landed), resp.StatusCode, len(body), markhorShowsLogin(string(body), landed), markhorShowsDashboard(string(body), landed), markhorRejected(string(body)))
	return string(body), landed, nil
}

func safeMarkhorErr(err error) string {
	if err == nil {
		return ""
	}
	var page *automation.PageError
	if errors.As(err, &page) {
		return page.Public()
	}
	if errors.Is(err, automation.ErrLoginPage) || errors.Is(err, automation.ErrTimeout) || errors.Is(err, automation.ErrDenied) || errors.Is(err, automation.ErrCSRF) {
		return err.Error()
	}
	return "request failed"
}

func requestURL(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}
	return req.URL.String()
}

func cookieHeaderNames(cookies []*http.Cookie) string {
	if len(cookies) == 0 {
		return "none"
	}
	names := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie != nil && cookie.Name != "" {
			names = append(names, cookie.Name)
		}
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ",")
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
		return automation.ErrTimeout
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline") || strings.Contains(msg, "timed out") {
		return automation.ErrTimeout
	}
	return automation.ErrTimeout
}

func markhorRejected(html string) bool {
	lower := strings.ToLower(html)
	return strings.Contains(lower, "invalid username or password") ||
		strings.Contains(lower, "user name or password is incorrect") ||
		strings.Contains(lower, "username or password is incorrect")
}

func markhorShowsLogin(html, pageURL string) bool {
	parsed, _ := url.Parse(pageURL)
	host := ""
	path := ""
	if parsed != nil {
		host = strings.ToLower(parsed.Hostname())
		path = strings.ToLower(parsed.EscapedPath())
	}
	// sm01 and the other tools serve their own app. Their HTML can mention a login field.
	if host != "" && host != "markhorseotool.com" && host != "www.markhorseotool.com" {
		return false
	}
	lower := strings.ToLower(html)
	if strings.Contains(lower, "logout") || strings.Contains(lower, "my tools") {
		return false
	}
	if strings.Contains(path, "/login") {
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

func seedMarkhorPortal(jar *cookiejar.Jar, cookies []automation.Cookie) {
	widened := make([]automation.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		domain := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(cookie.Domain)), ".")
		if domain == "" || domain == "markhorseotool.com" || strings.HasSuffix(domain, ".markhorseotool.com") {
			cookie.Domain = ".markhorseotool.com"
			cookie.HostOnly = false
		}
		widened = append(widened, cookie)
	}
	automation.SeedCookieJar(jar, widened, "markhorseotool.com")
}

func filterMarkhorCookies(cookies []automation.Cookie) []automation.Cookie {
	var out []automation.Cookie
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

func markhorAddedCookie(portal, live []automation.Cookie) bool {
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
