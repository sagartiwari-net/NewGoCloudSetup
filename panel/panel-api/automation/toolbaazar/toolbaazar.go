package toolbaazar

// Recipe logs in at app.toolbaazar.com and stores that portal cookie.
// Semrush opens /content/p/id/128/ and then the Access link (s2new.toolbaazar.com).
// Site Audit opens /content/p/id/132/ and then the Access link on that page.
// Tool jobs reuse the stored login. They do not submit the password again.

import (
	"context"
	"errors"
	"html"
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

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

var (
	baseURL   = "https://app.toolbaazar.com"
	loginURL  = "https://app.toolbaazar.com/login"
	memberURL = "https://app.toolbaazar.com/member"
	httpWait  = 25 * time.Second
)

const (
	semrushContent = "128"
	auditContent   = "132"
	semrushAccess  = "https://s2new.toolbaazar.com/?backup=1"
)

var anchorRE = regexp.MustCompile(`(?i)<a\b([^>]*)>`)

type Recipe struct{}

type toolTarget struct {
	contentID string
	fallback  string
}

func (Recipe) PortalHost() string { return "toolbaazar.com" }

func targetFor(upstream string) toolTarget {
	parsed, _ := url.Parse(strings.TrimSpace(upstream))
	host := strings.ToLower(parsed.Hostname())
	path := strings.ToLower(parsed.EscapedPath())
	if host == "site.verygoodtool.com" || strings.Contains(path, "siteaudit") || strings.Contains(host, "audit") {
		return toolTarget{contentID: auditContent}
	}
	return toolTarget{contentID: semrushContent, fallback: semrushAccess}
}

func (Recipe) Check(ctx context.Context, cookies []automation.Cookie) (bool, []automation.Cookie, error) {
	if len(cookies) == 0 {
		return false, nil, nil
	}
	client, jar, _, err := newClient()
	if err != nil {
		return false, nil, automation.ErrTimeout
	}
	seedPortal(jar, cookies)
	page, landed, err := get(ctx, client, memberURL, baseURL)
	if err != nil {
		return false, nil, err
	}
	if showsLogin(page, landed) || !showsDashboard(page, landed) {
		return false, nil, nil
	}
	updated := automation.MergeCookieSets(cookies, portalCookies(jar))
	if len(updated) == 0 {
		updated = cookies
	}
	return true, updated, nil
}

func (Recipe) Login(ctx context.Context, username, password string) ([]automation.Cookie, error) {
	client, jar, _, err := newClient()
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
	if rejected(member) {
		return nil, automation.At(automation.ErrRejected, memberLanded)
	}
	if showsLogin(member, memberLanded) || !showsDashboard(member, memberLanded) {
		return nil, automation.At(automation.ErrLoginPage, memberLanded)
	}
	cookies := portalCookies(jar)
	log.Printf("[automation] toolbaazar login page=%s cookies=%s", automation.Where(memberLanded), automation.Names(cookies))
	if len(cookies) == 0 {
		return nil, automation.At(automation.ErrLoginPage, memberLanded)
	}
	return cookies, nil
}

func (Recipe) Capture(ctx context.Context, portal []automation.Cookie, upstreamURL string) ([]automation.Cookie, error) {
	upstreamURL = strings.TrimSpace(upstreamURL)
	parsed, err := url.Parse(upstreamURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, automation.ErrUpstream
	}
	target := targetFor(upstreamURL)
	client, jar, record, err := newClient()
	if err != nil {
		return nil, automation.ErrTimeout
	}
	seedPortal(jar, portal)
	contentURL := strings.TrimRight(baseURL, "/") + "/content/p/id/" + target.contentID + "/"
	page, landed, err := get(ctx, client, contentURL, memberURL)
	if err != nil {
		return nil, err
	}
	if showsLogin(page, landed) {
		return nil, automation.At(automation.ErrLoginPage, landed)
	}
	launch := extractAccess(page, target)
	if launch == "" {
		return nil, automation.At(automation.ErrNoCookie, landed)
	}
	log.Printf("[automation] toolbaazar open %s", automation.Where(launch))
	page, landed, err = get(ctx, client, launch, contentURL)
	if err != nil {
		return nil, err
	}
	if showsLogin(page, landed) {
		return nil, automation.At(automation.ErrLoginPage, landed)
	}
	fresh := freshCookies(record.all(), portal)
	log.Printf("[automation] toolbaazar capture page=%s cookies=%s", automation.Where(landed), automation.Names(fresh))
	if len(fresh) == 0 {
		return nil, automation.At(automation.ErrNoCookie, landed)
	}
	return fresh, nil
}

func extractAccess(page string, target toolTarget) string {
	var links []string
	for _, tag := range anchorRE.FindAllStringSubmatch(page, -1) {
		if len(tag) < 2 || !strings.Contains(strings.ToLower(tag[1]), "btn-primary") {
			continue
		}
		href := attr(tag[1], "href")
		if href == "" || strings.Contains(strings.ToLower(href), "/content/p/id/") {
			continue
		}
		links = append(links, absolute(href))
	}
	if target.contentID == semrushContent {
		for _, href := range links {
			if strings.Contains(strings.ToLower(href), "s2new.toolbaazar.com") {
				return href
			}
		}
	}
	if len(links) > 0 {
		return links[0]
	}
	return target.fallback
}

func attr(tag, name string) string {
	pattern := regexp.MustCompile(`(?i)` + name + `\s*=\s*["']([^"']*)["']`)
	match := pattern.FindStringSubmatch(tag)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(html.UnescapeString(match[1]))
}

func absolute(href string) string {
	href = strings.TrimSpace(href)
	if strings.HasPrefix(href, "//") {
		return "https:" + href
	}
	if strings.HasPrefix(href, "/") {
		return strings.TrimRight(baseURL, "/") + href
	}
	return href
}

func showsLogin(page, pageURL string) bool {
	if !portalPage(pageURL) {
		return false
	}
	lower := strings.ToLower(page)
	if strings.Contains(lower, "logout") || strings.Contains(lower, "/logout") {
		return false
	}
	path := strings.ToLower(pathOf(pageURL))
	if strings.Contains(path, "/login") {
		return true
	}
	return strings.Contains(lower, "amember_login") && strings.Contains(lower, "amember_pass")
}

func showsDashboard(page, pageURL string) bool {
	if showsLogin(page, pageURL) {
		return false
	}
	lower := strings.ToLower(page)
	return strings.Contains(lower, "logout") || strings.Contains(lower, "/logout") || strings.Contains(lower, "/content/p/id/")
}

func rejected(page string) bool {
	lower := strings.ToLower(page)
	return strings.Contains(lower, "user name or password is incorrect") || strings.Contains(lower, "invalid username or password")
}

func portalPage(pageURL string) bool {
	host := strings.ToLower(hostOf(pageURL))
	if host == "" || host == "app.toolbaazar.com" || host == "toolbaazar.com" || host == "www.toolbaazar.com" {
		return true
	}
	parsed, err := url.Parse(baseURL)
	return err == nil && strings.EqualFold(parsed.Hostname(), host)
}

func seedPortal(jar *cookiejar.Jar, cookies []automation.Cookie) {
	widened := make([]automation.Cookie, 0, len(cookies))
	local := make([]automation.Cookie, 0, len(cookies))
	baseHost := ""
	if parsed, err := url.Parse(baseURL); err == nil {
		baseHost = parsed.Hostname()
	}
	for _, cookie := range cookies {
		if strings.TrimSpace(cookie.Name) == "" || cookie.Value == "" {
			continue
		}
		if cookie.Path == "" {
			cookie.Path = "/"
		}
		cookie.Domain = ".toolbaazar.com"
		cookie.HostOnly = false
		widened = append(widened, cookie)
		if baseHost != "" && !strings.HasSuffix(strings.ToLower(baseHost), "toolbaazar.com") {
			cookie.Domain = baseHost
			cookie.HostOnly = true
			local = append(local, cookie)
		}
	}
	automation.SeedCookieJar(jar, widened, "app.toolbaazar.com")
	automation.SeedCookieJar(jar, widened, "s2new.toolbaazar.com")
	if len(local) > 0 {
		automation.SeedCookieJar(jar, local, baseHost)
	}
}

func portalCookies(jar *cookiejar.Jar) []automation.Cookie {
	return automation.JarCookies(jar, []string{baseURL + "/", loginURL, memberURL})
}

func freshCookies(set []setCookie, portal []automation.Cookie) []automation.Cookie {
	known := map[string]struct{}{}
	for _, cookie := range portal {
		if cookie.Name == "" || cookie.Value == "" {
			continue
		}
		known[cookie.Name+"|"+cookie.Value] = struct{}{}
	}
	seen := map[string]struct{}{}
	var out []automation.Cookie
	for _, cookie := range set {
		name := strings.TrimSpace(cookie.Name)
		if name == "" || cookie.Value == "" || strings.Contains(strings.ToLower(name), "amember") {
			continue
		}
		if _, ok := known[name+"|"+cookie.Value]; ok {
			continue
		}
		if _, ok := seen[name+"|"+cookie.Value]; ok {
			continue
		}
		seen[name+"|"+cookie.Value] = struct{}{}
		domain := strings.TrimSpace(cookie.Domain)
		hostOnly := domain == ""
		if hostOnly {
			domain = cookie.Host
		}
		if domain != "" && !hostOnly && !strings.HasPrefix(domain, ".") {
			domain = "." + domain
		}
		path := cookie.Path
		if path == "" {
			path = "/"
		}
		expires := cookie.Expires
		session := expires.IsZero()
		var exp *float64
		if !session {
			value := float64(expires.Unix())
			exp = &value
		}
		out = append(out, automation.Cookie{
			Name:           name,
			Value:          cookie.Value,
			Domain:         domain,
			Path:           path,
			HostOnly:       hostOnly,
			HTTPOnly:       cookie.HTTPOnly,
			Secure:         cookie.Secure,
			SameSite:       sameSiteLabel(cookie.SameSite),
			Session:        session,
			ExpirationDate: exp,
		})
	}
	return out
}

func sameSiteLabel(mode http.SameSite) *string {
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

func newClient() (*http.Client, *cookiejar.Jar, *cookieRecord, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, nil, nil, err
	}
	record := &cookieRecord{}
	client := &http.Client{
		Timeout: httpWait,
		Jar:     jar,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			resp, err := http.DefaultTransport.RoundTrip(req)
			if err != nil {
				log.Printf("[automation] toolbaazar hop %s failed before a response", automation.Where(requestURL(req)))
				return resp, err
			}
			if resp != nil {
				record.note(req, resp.Cookies())
				log.Printf("[automation] toolbaazar hop %s status=%d set=%s", automation.Where(requestURL(req)), resp.StatusCode, namesOf(resp.Cookies()))
			}
			return resp, nil
		}),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			log.Printf("[automation] toolbaazar redirect %s -> %s (%d)", automation.Where(requestURL(via[len(via)-1])), automation.Where(requestURL(req)), len(via))
			if len(via) >= 15 {
				return automation.At(automation.ErrLoginPage, requestURL(req))
			}
			return nil
		},
	}
	return client, jar, record, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

type setCookie struct {
	Name     string
	Value    string
	Domain   string
	Host     string
	Path     string
	Expires  time.Time
	HTTPOnly bool
	Secure   bool
	SameSite http.SameSite
}

type cookieRecord struct {
	mu    sync.Mutex
	saved []setCookie
}

func (r *cookieRecord) note(req *http.Request, cookies []*http.Cookie) {
	if r == nil {
		return
	}
	host := ""
	if req != nil && req.URL != nil {
		host = req.URL.Hostname()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, cookie := range cookies {
		if cookie == nil || cookie.Name == "" {
			continue
		}
		expires := cookie.Expires
		if expires.IsZero() && cookie.MaxAge > 0 {
			expires = time.Now().Add(time.Duration(cookie.MaxAge) * time.Second)
		}
		r.saved = append(r.saved, setCookie{
			Name: cookie.Name, Value: cookie.Value, Domain: cookie.Domain, Host: host,
			Path: cookie.Path, Expires: expires, HTTPOnly: cookie.HttpOnly, Secure: cookie.Secure, SameSite: cookie.SameSite,
		})
	}
}

func (r *cookieRecord) all() []setCookie {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]setCookie, len(r.saved))
	copy(out, r.saved)
	return out
}

func get(ctx context.Context, client *http.Client, rawURL, referer string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", automation.ErrUpstream
	}
	headers(req, referer)
	return do(ctx, client, req)
}

func post(ctx context.Context, client *http.Client, rawURL string, form url.Values) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", automation.ErrUpstream
	}
	headers(req, loginURL)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", baseURL)
	return do(ctx, client, req)
}

func headers(req *http.Request, referer string) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
}

func do(ctx context.Context, client *http.Client, req *http.Request) (string, string, error) {
	log.Printf("[automation] toolbaazar %s %s", req.Method, automation.Where(req.URL.String()))
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
	log.Printf("[automation] toolbaazar %s landed %s status=%d bytes=%d login=%t", req.Method, automation.Where(final), resp.StatusCode, len(body), showsLogin(string(body), final))
	return string(body), final, nil
}

func requestURL(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}
	return req.URL.String()
}

func hostOf(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func pathOf(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.EscapedPath()
}

func namesOf(cookies []*http.Cookie) string {
	names := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie != nil && cookie.Name != "" {
			names = append(names, cookie.Name)
		}
	}
	return strings.Join(names, ",")
}
