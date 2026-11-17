package noxtools

// Recipe keeps the noxtools.com aMember PHPSESSID warm.
// Session jobs only need Check (+ optional Login). Capture returns the portal cookie
// until a dedicated recloud tool website exists.

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"panel-api/automation"
	"strings"
	"time"
)

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"

var (
	baseURL   = "https://noxtools.com"
	loginURL  = "https://noxtools.com/login"
	memberURL = "https://noxtools.com/secure/member"
	httpWait  = 30 * time.Second
)

type Recipe struct{}

func (Recipe) PortalHost() string { return "noxtools.com" }

func (Recipe) Check(ctx context.Context, cookies []automation.Cookie) (bool, []automation.Cookie, error) {
	if len(cookies) == 0 {
		return false, nil, nil
	}
	client, jar, err := newClient()
	if err != nil {
		return false, nil, automation.ErrTimeout
	}
	seedPortal(jar, cookies)
	page, landed, err := get(ctx, client, memberURL, baseURL)
	if err != nil {
		return false, nil, err
	}
	if showsLogin(page, landed) || !showsMember(page, landed) {
		return false, nil, nil
	}
	updated := automation.MergeCookieSets(cookies, portalCookies(jar))
	if len(updated) == 0 {
		updated = cookies
	}
	return true, updated, nil
}

func (Recipe) Login(ctx context.Context, username, password string) ([]automation.Cookie, error) {
	client, jar, err := newClient()
	if err != nil {
		return nil, automation.ErrTimeout
	}
	if _, _, err = get(ctx, client, loginURL, baseURL); err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("amember_login", username)
	form.Set("amember_pass", password)
	body, landed, err := post(ctx, client, loginURL, form)
	if err != nil {
		return nil, err
	}
	if rejected(body) {
		return nil, automation.At(automation.ErrRejected, landed)
	}
	member, memberLanded, err := get(ctx, client, memberURL, baseURL)
	if err != nil {
		return nil, err
	}
	if showsLogin(member, memberLanded) || !showsMember(member, memberLanded) {
		return nil, automation.At(automation.ErrLoginPage, memberLanded)
	}
	cookies := portalCookies(jar)
	log.Printf("[automation] noxtools login page=%s cookies=%s", automation.Where(memberLanded), automation.Names(cookies))
	if len(cookies) == 0 {
		return nil, automation.At(automation.ErrLoginPage, memberLanded)
	}
	return cookies, nil
}

func (Recipe) Capture(ctx context.Context, portal []automation.Cookie, upstreamURL string) ([]automation.Cookie, error) {
	// No dedicated tool host yet — homepage/session cookie is what we keep alive.
	_ = upstreamURL
	ok, updated, err := (Recipe{}).Check(ctx, portal)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, automation.ErrLoginPage
	}
	if len(updated) == 0 {
		updated = portal
	}
	return append([]automation.Cookie(nil), updated...), nil
}

func showsLogin(page, pageURL string) bool {
	lower := strings.ToLower(page)
	path := strings.ToLower(pathOf(pageURL))
	if strings.Contains(path, "/login") {
		return true
	}
	if strings.Contains(lower, "name=\"amember_login\"") || strings.Contains(lower, "id=\"amember_login\"") {
		return true
	}
	return strings.Contains(lower, "amember_login") && strings.Contains(lower, "amember_pass")
}

func showsMember(page, pageURL string) bool {
	if showsLogin(page, pageURL) {
		return false
	}
	path := strings.ToLower(pathOf(pageURL))
	if !strings.Contains(path, "/secure/member") && !strings.Contains(path, "/member") {
		// Some setups land on / with a huge marketing page when logged out.
		if !strings.Contains(strings.ToLower(page), "member-main-subscriptions") {
			return false
		}
	}
	lower := strings.ToLower(page)
	if strings.Contains(lower, "id=\"member-main-subscriptions\"") || strings.Contains(lower, "member-main-subscriptions") {
		return true
	}
	if strings.Contains(lower, "/secure/member/payment-history") {
		return true
	}
	if strings.Contains(lower, "/logout") || strings.Contains(lower, ">logout<") {
		return true
	}
	return false
}

func rejected(page string) bool {
	lower := strings.ToLower(page)
	return strings.Contains(lower, "user name or password is incorrect") ||
		strings.Contains(lower, "invalid username or password") ||
		strings.Contains(lower, "authentication failed")
}

func newClient() (*http.Client, *cookiejar.Jar, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, nil, err
	}
	return &http.Client{
		Jar:     jar,
		Timeout: httpWait,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 8 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}, jar, nil
}

func seedPortal(jar *cookiejar.Jar, cookies []automation.Cookie) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return
	}
	var list []*http.Cookie
	for _, c := range cookies {
		if strings.TrimSpace(c.Name) == "" || c.Value == "" {
			continue
		}
		list = append(list, &http.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Path:     first(c.Path, "/"),
			Domain:   "noxtools.com",
			Secure:   true,
			HttpOnly: c.HTTPOnly,
		})
	}
	jar.SetCookies(u, list)
}

func portalCookies(jar *cookiejar.Jar) []automation.Cookie {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}
	var out []automation.Cookie
	for _, c := range jar.Cookies(u) {
		if c.Name != "PHPSESSID" && c.Name != "amember_nr" {
			continue
		}
		out = append(out, automation.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   ".noxtools.com",
			Path:     first(c.Path, "/"),
			Secure:   true,
			HTTPOnly: true,
			Session:  true,
		})
	}
	return out
}

func get(ctx context.Context, client *http.Client, rawURL, referer string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", automation.ErrTimeout
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", automation.ErrTimeout
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	landed := resp.Request.URL.String()
	if landed == "" {
		landed = rawURL
	}
	return string(body), landed, nil
}

func post(ctx context.Context, client *http.Client, rawURL string, form url.Values) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", automation.ErrTimeout
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Origin", baseURL)
	req.Header.Set("Referer", loginURL)
	resp, err := client.Do(req)
	if err != nil {
		return "", "", automation.ErrTimeout
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	landed := resp.Request.URL.String()
	if landed == "" {
		landed = rawURL
	}
	return string(body), landed, nil
}

func pathOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.EscapedPath()
}

func first(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
