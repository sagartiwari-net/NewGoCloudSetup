package rankytools

// Recipe keeps the RankyTools aMember session in Storage (PHPSESSID + amember_nr),
// then writes the same portal cookie onto recloud tool accounts (e.g. Semrush ryksm1).
// Tool hosts are subdomains of rankytools.com and accept the portal PHPSESSID.

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
	baseURL    = "https://member.rankytools.com"
	loginURL   = "https://member.rankytools.com/login"
	memberURL  = "https://member.rankytools.com/member"
	userAgent  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	httpWait   = 30 * time.Second
	portalHost = "rankytools.com"
)

var attemptRE = regexp.MustCompile(`(?i)name=["']login_attempt_id["'][^>]*value=["']([^"']+)["']|value=["']([^"']+)["'][^>]*name=["']login_attempt_id["']`)

type Recipe struct{}

func (Recipe) PortalHost() string { return portalHost }

func (Recipe) Check(ctx context.Context, cookies []automation.Cookie) (bool, []automation.Cookie, error) {
	if len(cookies) == 0 {
		return false, nil, nil
	}
	client, jar, record, err := newClient(12)
	if err != nil {
		return false, nil, automation.ErrTimeout
	}
	seedPortal(jar, cookies)
	html, landed, err := get(ctx, client, memberURL, baseURL+"/")
	if err != nil {
		return false, nil, err
	}
	if showsLogin(html, landed) || !showsMember(html, landed) {
		return false, nil, nil
	}
	if !hasActivePlan(html) {
		return false, nil, automation.ErrDenied
	}
	updated := automation.MergeCookieSets(storedCookies(cookies), exportCookies(jar, record))
	if len(updated) == 0 {
		updated = storedCookies(cookies)
	}
	return true, updated, nil
}

func (Recipe) Login(ctx context.Context, username, password string) ([]automation.Cookie, error) {
	client, jar, record, err := newClient(12)
	if err != nil {
		return nil, automation.ErrTimeout
	}
	html, _, err := get(ctx, client, loginURL, baseURL+"/")
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("amember_login", username)
	form.Set("amember_pass", password)
	if match := attemptRE.FindStringSubmatch(html); len(match) > 0 {
		attemptID := match[1]
		if attemptID == "" && len(match) > 2 {
			attemptID = match[2]
		}
		if attemptID != "" {
			form.Set("login_attempt_id", attemptID)
		}
	}
	body, landed, err := post(ctx, client, loginURL, form)
	if err != nil {
		return nil, err
	}
	if rejected(body) {
		return nil, automation.At(automation.ErrRejected, landed)
	}
	member, memberLanded, err := get(ctx, client, memberURL, loginURL)
	if err != nil {
		return nil, err
	}
	if showsLogin(member, memberLanded) || !showsMember(member, memberLanded) {
		return nil, automation.At(automation.ErrLoginPage, memberLanded)
	}
	if !hasActivePlan(member) {
		return nil, automation.At(automation.ErrDenied, memberLanded)
	}
	cookies := exportCookies(jar, record)
	log.Printf("[automation] rankytools login page=%s cookies=%s", automation.Where(memberLanded), automation.Names(cookies))
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
	host := strings.ToLower(parsed.Hostname())
	if !strings.HasSuffix(host, "."+portalHost) && host != portalHost {
		return nil, automation.ErrUpstream
	}
	client, jar, record, err := newClient(15)
	if err != nil {
		return nil, automation.ErrTimeout
	}
	seedPortal(jar, portal)

	memberHTML, memberLanded, err := get(ctx, client, memberURL, baseURL+"/")
	if err != nil {
		return nil, err
	}
	if showsLogin(memberHTML, memberLanded) {
		return nil, automation.At(automation.ErrLoginPage, memberLanded)
	}
	if !hasActivePlan(memberHTML) {
		return nil, automation.At(automation.ErrDenied, memberLanded)
	}

	toolRoot := parsed.Scheme + "://" + parsed.Host + "/"
	html, landed, err := get(ctx, client, toolRoot, memberURL)
	if err != nil {
		return nil, err
	}
	if showsLogin(html, landed) || memberBounce(landed) {
		return nil, automation.At(automation.ErrLoginPage, landed)
	}
	if toolDenied(html) {
		return nil, automation.At(automation.ErrDenied, landed)
	}
	if strings.TrimSpace(parsed.Path) != "" && parsed.Path != "/" {
		if _, _, err := get(ctx, client, upstreamURL, memberURL); err != nil {
			return nil, err
		}
	}
	cookies := exportCookies(jar, record)
	log.Printf("[automation] rankytools capture page=%s cookies=%s", automation.Where(landed), automation.Names(cookies))
	if len(cookies) == 0 {
		if len(portal) == 0 {
			return nil, automation.At(automation.ErrNoCookie, landed)
		}
		return storedCookies(portal), nil
	}
	return cookies, nil
}

func newClient(maxRedirects int) (*http.Client, *cookiejar.Jar, *cookieRecord, error) {
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
				return resp, err
			}
			if resp != nil {
				record.note(resp.Cookies())
			}
			return resp, err
		}),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
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

type cookieRecord struct {
	mu    sync.Mutex
	saved map[string]http.Cookie
}

func (r *cookieRecord) note(cookies []*http.Cookie) {
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

func (r *cookieRecord) get(name, value string) (http.Cookie, bool) {
	if r == nil {
		return http.Cookie{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cookie, ok := r.saved[name+"|"+value]
	return cookie, ok
}

func get(ctx context.Context, client *http.Client, rawURL, referer string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", automation.ErrTimeout
	}
	setHeaders(req)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	return read(client, req, rawURL)
}

func post(ctx context.Context, client *http.Client, rawURL string, form url.Values) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", automation.ErrTimeout
	}
	setHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", baseURL)
	req.Header.Set("Referer", loginURL)
	return read(client, req, rawURL)
}

func read(client *http.Client, req *http.Request, rawURL string) (string, string, error) {
	log.Printf("[automation] rankytools %s %s", req.Method, automation.Where(rawURL))
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", "", automation.ErrTimeout
		}
		var page *automation.PageError
		if errors.As(err, &page) {
			return "", "", err
		}
		return "", "", automation.ErrTimeout
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", "", automation.ErrTimeout
	}
	landed := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		landed = resp.Request.URL.String()
	}
	return string(body), landed, nil
}

func setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
}

func requestURL(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}
	return req.URL.String()
}

func seedPortal(jar *cookiejar.Jar, cookies []automation.Cookie) {
	widened := make([]automation.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		if strings.TrimSpace(cookie.Name) == "" || cookie.Value == "" {
			continue
		}
		c := portalDefaults(cookie)
		widened = append(widened, c)
	}
	automation.SeedCookieJar(jar, widened, portalHost)
	automation.SeedCookieJar(jar, widened, "member.rankytools.com")
	automation.SeedCookieJar(jar, widened, "www.rankytools.com")
}

func exportCookies(jar *cookiejar.Jar, record *cookieRecord) []automation.Cookie {
	var out []automation.Cookie
	for _, cookie := range automation.JarCookies(jar, []string{baseURL + "/", loginURL, memberURL, "https://sr.rankytools.com/"}) {
		name := strings.ToLower(cookie.Name)
		if name != "phpsessid" && name != "amember_nr" {
			continue
		}
		if saved, ok := record.get(cookie.Name, cookie.Value); ok {
			if strings.TrimSpace(saved.Domain) != "" {
				cookie.Domain = saved.Domain
			}
			if saved.Path != "" {
				cookie.Path = saved.Path
			}
			cookie.HTTPOnly = saved.HttpOnly
			cookie.Secure = saved.Secure
			if !saved.Expires.IsZero() {
				v := float64(saved.Expires.Unix())
				cookie.ExpirationDate = &v
				cookie.Session = false
			}
		}
		out = append(out, portalDefaults(cookie))
	}
	return filterPortal(out)
}

func storedCookies(cookies []automation.Cookie) []automation.Cookie {
	out := make([]automation.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		out = append(out, portalDefaults(cookie))
	}
	return filterPortal(out)
}

func portalDefaults(cookie automation.Cookie) automation.Cookie {
	switch strings.ToLower(cookie.Name) {
	case "phpsessid", "amember_nr":
	default:
		return cookie
	}
	cookie.Domain = "." + portalHost
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

func filterPortal(cookies []automation.Cookie) []automation.Cookie {
	var out []automation.Cookie
	for _, cookie := range cookies {
		if cookie.Name == "" || cookie.Value == "" {
			continue
		}
		name := strings.ToLower(cookie.Name)
		if name != "phpsessid" && name != "amember_nr" {
			continue
		}
		out = append(out, cookie)
	}
	return out
}

func showsLogin(html, pageURL string) bool {
	path := strings.ToLower(pathOf(pageURL))
	if strings.Contains(path, "/login") {
		return true
	}
	lower := strings.ToLower(html)
	if strings.Contains(lower, `name="amember_login"`) || strings.Contains(lower, `id="amember-login"`) {
		return true
	}
	return strings.Contains(lower, "welcome back") && strings.Contains(lower, "sign in to continue")
}

func showsMember(html, pageURL string) bool {
	path := strings.ToLower(pathOf(pageURL))
	urlLower := strings.ToLower(pageURL)
	if strings.Contains(path, "/login") {
		return false
	}
	lower := strings.ToLower(html)
	if strings.Contains(lower, "your membership information") {
		return true
	}
	if strings.Contains(urlLower, "/member") && (strings.Contains(lower, "logout") || strings.Contains(lower, "subscription")) {
		return true
	}
	return strings.Contains(lower, "member-main-subscriptions") && !showsLogin(html, pageURL)
}

func hasActivePlan(html string) bool {
	lower := strings.ToLower(html)
	if strings.Contains(lower, "no active subscription") || strings.Contains(lower, "you have no active") {
		return false
	}
	// aMember widget: plan title + next bill / expires inside subscriptions list.
	if strings.Contains(lower, "member-main-subscriptions") || strings.Contains(lower, "am-list-subscriptions") {
		if strings.Contains(lower, "next bill") || strings.Contains(lower, "expires") || strings.Contains(lower, "active") {
			return true
		}
		// Plan name present without empty-state copy.
		if strings.Contains(lower, "plan") && !strings.Contains(lower, "no subscription") {
			return true
		}
	}
	return strings.Contains(lower, "seo premium plan") || strings.Contains(lower, "seo mega plan")
}

func rejected(html string) bool {
	lower := strings.ToLower(html)
	return strings.Contains(lower, "invalid username or password") ||
		strings.Contains(lower, "user name or password is incorrect") ||
		strings.Contains(lower, "authentication failed") ||
		strings.Contains(lower, "login failed")
}

func memberBounce(pageURL string) bool {
	path := strings.ToLower(pathOf(pageURL))
	return strings.Contains(path, "/login")
}

func toolDenied(html string) bool {
	lower := strings.ToLower(html)
	return strings.Contains(lower, "access denied") && strings.Contains(lower, "subscription")
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
