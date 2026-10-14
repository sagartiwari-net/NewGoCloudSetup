package azad

// Recipe is the Azad portal HTTP login from the old GoAuto client.
// Login stores the members.azadseo.com cookie. Capture opens the Semrush
// launch link on that session and keeps the tool cookies.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"panel-api/automation"
	"regexp"
	"strings"
	"time"
)

const azadUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

var (
	azadBaseURL   = "https://members.azadseo.com"
	azadLoginURL  = "https://members.azadseo.com/login"
	azadMemberURL = "https://members.azadseo.com/member"
	azadHTTPWait  = 25 * time.Second
)

var (
	azadSemrushHref = regexp.MustCompile(`(?i)href\s*=\s*["']([^"']*semrush[^"']*)["']`)
	azadAttemptRE   = regexp.MustCompile(`(?i)name=["']login_attempt_id["'][^>]*value=["']([^"']+)["']|value=["']([^"']+)["'][^>]*name=["']login_attempt_id["']`)
)

type Recipe struct{}

func (Recipe) PortalHost() string { return "azadseo.com" }

func (Recipe) Check(ctx context.Context, cookies []automation.Cookie) (bool, []automation.Cookie, error) {
	if len(cookies) == 0 {
		return false, nil, nil
	}
	client, jar, err := newAzadClient()
	if err != nil {
		return false, nil, automation.ErrTimeout
	}
	seedAzadPortal(jar, cookies)
	html, landed, err := azadGet(ctx, client, azadMemberURL, azadBaseURL)
	if err != nil {
		return false, nil, err
	}
	if azadShowsLogin(html, landed) || !azadShowsDashboard(html, landed) {
		return false, nil, nil
	}
	updated := automation.MergeCookieSets(cookies, portalCookies(jar))
	if len(updated) == 0 {
		updated = cookies
	}
	return true, updated, nil
}

func (Recipe) Login(ctx context.Context, username, password string) ([]automation.Cookie, error) {
	client, jar, err := newAzadClient()
	if err != nil {
		return nil, automation.ErrTimeout
	}
	html, _, err := azadGet(ctx, client, azadLoginURL, azadBaseURL)
	if err != nil {
		return nil, err
	}
	attemptID := ""
	if match := azadAttemptRE.FindStringSubmatch(html); len(match) > 0 {
		attemptID = match[1]
		if attemptID == "" && len(match) > 2 {
			attemptID = match[2]
		}
	}
	form := url.Values{}
	form.Set("amember_login", username)
	form.Set("amember_pass", password)
	form.Set("remember_login", "1")
	if attemptID != "" {
		form.Set("login_attempt_id", attemptID)
	}
	body, landed, err := azadPost(ctx, client, azadLoginURL, form)
	if err != nil {
		return nil, err
	}
	if azadShowsLogin(body, landed) {
		return nil, automation.ErrLoginPage
	}
	member, memberURL, err := azadGet(ctx, client, azadMemberURL, azadBaseURL)
	if err != nil {
		return nil, err
	}
	if azadShowsLogin(member, memberURL) || !azadShowsDashboard(member, memberURL) {
		return nil, automation.ErrLoginPage
	}
	cookies := portalCookies(jar)
	if len(cookies) == 0 {
		return nil, automation.ErrLoginPage
	}
	return cookies, nil
}

func (Recipe) Capture(ctx context.Context, portal []automation.Cookie, upstreamURL string) ([]automation.Cookie, error) {
	upstreamURL = strings.TrimSpace(upstreamURL)
	parsed, err := url.Parse(upstreamURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, automation.ErrUpstream
	}
	client, jar, err := newAzadClient()
	if err != nil {
		return nil, automation.ErrTimeout
	}
	seedAzadPortal(jar, portal)
	html, landed, err := azadGet(ctx, client, azadMemberURL, azadBaseURL)
	if err != nil {
		return nil, err
	}
	if azadShowsLogin(html, landed) {
		return nil, automation.ErrLoginPage
	}
	launch := azadLaunchURL(html, parsed.Host)
	if launch == "" {
		launch = upstreamURL
	}
	html, landed, err = azadGet(ctx, client, launch, azadMemberURL)
	if err != nil {
		return nil, err
	}
	if strings.Contains(strings.ToLower(landed), "protect/new-rewrite") {
		if azadShowsLogin(html, landed) {
			return nil, automation.ErrLoginPage
		}
		origin := parsed.Scheme + "://" + parsed.Host + "/"
		html, landed, err = azadGet(ctx, client, origin, landed)
		if err != nil {
			return nil, err
		}
		_ = html
	}
	if azadShowsLogin(html, landed) {
		return nil, automation.ErrLoginPage
	}
	tool := toolCookies(jar, parsed.Host, landed)
	if len(tool) == 0 {
		if len(portal) == 0 {
			return nil, automation.ErrLoginPage
		}
		return portal, nil
	}
	return tool, nil
}

func azadLaunchURL(html, upstreamHost string) string {
	upstreamHost = strings.ToLower(strings.TrimSpace(upstreamHost))
	matches := azadSemrushHref.FindAllStringSubmatch(html, -1)
	fallback := ""
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		href := strings.ReplaceAll(strings.TrimSpace(match[1]), "&amp;", "&")
		if strings.HasPrefix(href, "//") {
			href = "https:" + href
		}
		if strings.HasPrefix(href, "/") {
			href = azadBaseURL + href
		}
		parsed, err := url.Parse(href)
		if err != nil || parsed.Host == "" {
			continue
		}
		if fallback == "" {
			fallback = href
		}
		if upstreamHost != "" && strings.Contains(strings.ToLower(parsed.Host), upstreamHost) {
			return href
		}
	}
	if strings.Contains(upstreamHost, "semrush") && fallback != "" {
		return fallback
	}
	return ""
}

func azadShowsLogin(html, pageURL string) bool {
	lower := strings.ToLower(html)
	urlLower := strings.ToLower(pageURL)
	if strings.Contains(urlLower, "/login") && strings.Contains(lower, "amember_login") {
		return true
	}
	if strings.Contains(lower, "name=\"amember_login\"") || strings.Contains(lower, "name='amember_login'") {
		return true
	}
	return strings.Contains(lower, "login to your account")
}

func azadShowsDashboard(html, pageURL string) bool {
	if azadShowsLogin(html, pageURL) {
		return false
	}
	lower := strings.ToLower(html)
	urlLower := strings.ToLower(pageURL)
	if strings.Contains(urlLower, "/login") {
		return false
	}
	return strings.Contains(lower, "logout") ||
		strings.Contains(lower, "/logout") ||
		strings.Contains(lower, "semrush") ||
		strings.Contains(lower, "am-member") ||
		(strings.Contains(urlLower, "/member") && !strings.Contains(lower, "amember_login"))
}

func newAzadClient() (*http.Client, *cookiejar.Jar, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, nil, err
	}
	client := &http.Client{
		Timeout: azadHTTPWait,
		Jar:     jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 15 {
				return automation.ErrLoginPage
			}
			if req.URL != nil && strings.Contains(strings.ToLower(req.URL.String()), "protect/new-rewrite") {
				for _, previous := range via {
					if previous.URL != nil && strings.Contains(strings.ToLower(previous.URL.String()), "protect/new-rewrite") {
						return http.ErrUseLastResponse
					}
				}
			}
			return nil
		},
	}
	return client, jar, nil
}

func seedAzadPortal(jar *cookiejar.Jar, cookies []automation.Cookie) {
	automation.SeedCookieJar(jar, cookies, "azadseo.com")
	automation.SeedCookieJar(jar, cookies, "members.azadseo.com")
}

func portalCookies(jar *cookiejar.Jar) []automation.Cookie {
	return automation.JarCookies(jar, []string{
		azadBaseURL + "/",
		azadLoginURL,
		azadMemberURL,
		"https://azadseo.com/",
	})
}

func toolCookies(jar *cookiejar.Jar, host, landed string) []automation.Cookie {
	urls := []string{"https://" + host + "/"}
	if parsed, err := url.Parse(landed); err == nil && parsed.Host != "" {
		urls = append(urls, parsed.Scheme+"://"+parsed.Host+"/")
	}
	var out []automation.Cookie
	for _, cookie := range automation.JarCookies(jar, urls) {
		if strings.Contains(strings.ToLower(cookie.Name), "amember") {
			continue
		}
		out = append(out, cookie)
	}
	return out
}

func azadGet(ctx context.Context, client *http.Client, rawURL, referer string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", automation.ErrUpstream
	}
	azadHeaders(req, referer)
	return azadDo(ctx, client, req)
}

func azadPost(ctx context.Context, client *http.Client, rawURL string, form url.Values) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", automation.ErrUpstream
	}
	azadHeaders(req, azadLoginURL)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", azadBaseURL)
	return azadDo(ctx, client, req)
}

func azadHeaders(req *http.Request, referer string) {
	req.Header.Set("User-Agent", azadUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
}

func azadDo(ctx context.Context, client *http.Client, req *http.Request) (string, string, error) {
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, automation.ErrLoginPage) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return "", "", automation.ErrTimeout
			}
			return "", "", err
		}
		if strings.Contains(strings.ToLower(err.Error()), "timeout") || strings.Contains(strings.ToLower(err.Error()), "deadline") {
			return "", "", automation.ErrTimeout
		}
		return "", "", automation.ErrUpstream
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", "", automation.ErrUpstream
	}
	final := req.URL.String()
	if resp.Request != nil && resp.Request.URL != nil {
		final = resp.Request.URL.String()
	}
	return string(body), final, nil
}
