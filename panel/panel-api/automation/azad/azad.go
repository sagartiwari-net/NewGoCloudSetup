package azad

// Recipe is the Azad portal HTTP login from the old GoAuto client.
// Login stores the members.azadseo.com cookie. Semrush 2 opens semrush2.azadseo.com
// and writes .xemrush.site. Semrush 5 opens export1sem.azadseo.com and writes .6.semrush.com.in.
// Grammarly opens grammarly2.azadseo.com and writes .testingg.one.
// Jasper opens jasperai1.azadseo.com and keeps each cookie's own domain.
// Quillbot opens quillbot1.azadseo.com and writes .quillbot2.testingg.me.

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"panel-api/automation"
	"regexp"
	"strconv"
	"strings"
	"sync"
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
	azadSemrushHref = regexp.MustCompile(`(?i)href\s*=\s*["']([^"']*semrush2\.azadseo\.com[^"']*)["']`)
	azadAttemptRE   = regexp.MustCompile(`(?i)name=["']login_attempt_id["'][^>]*value=["']([^"']+)["']|value=["']([^"']+)["'][^>]*name=["']login_attempt_id["']`)
)

type semrushTarget struct {
	proxyHost     string
	cookieDomain  string
	otherDomain   string
	keepSetDomain bool
	readHosts     []string
}

func semrushTargetFor(upstreamHost string) semrushTarget {
	host := strings.ToLower(strings.TrimSpace(upstreamHost))
	if split, _, err := net.SplitHostPort(host); err == nil {
		host = split
	}
	if host == "6.semrush.com.in" || strings.HasSuffix(host, ".semrush.com.in") || host == "export1sem.azadseo.com" {
		return semrushTarget{proxyHost: "export1sem.azadseo.com", cookieDomain: ".6.semrush.com.in"}
	}
	if host == "testingg.one" || strings.HasSuffix(host, ".testingg.one") || host == "grammarly2.azadseo.com" {
		return semrushTarget{proxyHost: "grammarly2.azadseo.com", cookieDomain: ".testingg.one"}
	}
	if host == "jasperpro.premiium.click" || strings.HasSuffix(host, ".premiium.click") || host == "premiium.click" || host == "jasperai1.azadseo.com" {
		return semrushTarget{
			proxyHost:     "jasperai1.azadseo.com",
			cookieDomain:  ".jasperpro.premiium.click",
			otherDomain:   ".premiium.click",
			keepSetDomain: true,
			readHosts:     []string{"jasperpro.premiium.click", "premiium.click"},
		}
	}
	if host == "quillbot2.testingg.me" || strings.HasSuffix(host, ".testingg.me") || host == "testingg.me" || host == "quillbot1.azadseo.com" {
		return semrushTarget{
			proxyHost:    "quillbot1.azadseo.com",
			cookieDomain: ".quillbot2.testingg.me",
			readHosts:    []string{"quillbot2.testingg.me", "testingg.me"},
		}
	}
	if net.ParseIP(host) != nil || host == "localhost" {
		return semrushTarget{proxyHost: strings.TrimSpace(upstreamHost), cookieDomain: ".xemrush.site"}
	}
	return semrushTarget{proxyHost: "semrush2.azadseo.com", cookieDomain: ".xemrush.site"}
}

type Recipe struct{}

func (Recipe) PortalHost() string { return "azadseo.com" }

func (Recipe) Check(ctx context.Context, cookies []automation.Cookie) (bool, []automation.Cookie, error) {
	if len(cookies) == 0 {
		return false, nil, nil
	}
	client, jar, _, err := newAzadClient()
	if err != nil {
		return false, nil, automation.ErrTimeout
	}
	seedAzadPortal(jar, cookies, "")
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
	client, jar, _, err := newAzadClient()
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
	return captureSemrush(ctx, portal, semrushTargetFor(parsed.Host))
}

func captureSemrush(ctx context.Context, portal []automation.Cookie, target semrushTarget) ([]automation.Cookie, error) {
	client, jar, record, err := newAzadClient()
	if err != nil {
		return nil, automation.ErrTimeout
	}
	seedAzadPortal(jar, portal, target.proxyHost)
	html, landed, err := azadGet(ctx, client, azadMemberURL, azadBaseURL)
	if err != nil {
		return nil, err
	}
	if azadShowsLogin(html, landed) {
		return nil, automation.ErrLoginPage
	}
	launch := azadExtractSemrushURL(html, target.proxyHost)
	if launch == "" {
		launch = proxyOrigin(target.proxyHost) + "?tool_access_at=" + strconv.FormatInt(time.Now().Unix(), 10)
	}
	html, landed, err = azadGet(ctx, client, launch, azadMemberURL)
	if err != nil {
		return nil, err
	}
	if strings.Contains(strings.ToLower(landed), "protect/new-rewrite") {
		if azadShowsLogin(html, landed) {
			return nil, automation.ErrLoginPage
		}
		html, landed, err = azadGet(ctx, client, proxyOrigin(target.proxyHost), landed)
		if err != nil {
			return nil, err
		}
		_ = html
	}
	if azadShowsLogin(html, landed) {
		return nil, automation.ErrLoginPage
	}
	tool := mapSemrushCookies(jar, record, landed, target)
	if len(tool) == 0 {
		_, _, _ = azadGet(ctx, client, proxyOrigin(target.proxyHost), azadMemberURL)
		tool = mapSemrushCookies(jar, record, proxyOrigin(target.proxyHost), target)
	}
	fresh := semrushFreshCookies(tool, portal)
	if len(fresh) == 0 {
		return nil, automation.ErrLoginPage
	}
	return fresh, nil
}

func proxyOrigin(proxyHost string) string {
	scheme := "https"
	host := strings.TrimSpace(proxyHost)
	name := host
	if split, _, err := net.SplitHostPort(host); err == nil {
		name = split
	}
	if net.ParseIP(name) != nil || strings.HasPrefix(host, "127.") || strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "[::1]") {
		scheme = "http"
	}
	return scheme + "://" + host + "/"
}

func azadExtractSemrushURL(html, proxyHost string) string {
	pattern := regexp.MustCompile(`(?i)href\s*=\s*["']([^"']*` + regexp.QuoteMeta(proxyHost) + `[^"']*)["']`)
	match := pattern.FindStringSubmatch(html)
	if len(match) < 2 && proxyHost == "semrush2.azadseo.com" {
		match = azadSemrushHref.FindStringSubmatch(html)
	}
	if len(match) < 2 {
		return ""
	}
	href := strings.ReplaceAll(strings.TrimSpace(match[1]), "&amp;", "&")
	if strings.HasPrefix(href, "//") {
		href = "https:" + href
	}
	if strings.HasPrefix(href, "/") {
		href = strings.TrimRight(azadBaseURL, "/") + href
	}
	return href
}

func azadLaunchURL(html, upstreamHost string) string {
	target := semrushTargetFor(upstreamHost)
	if href := azadExtractSemrushURL(html, target.proxyHost); href != "" {
		return href
	}
	return azadLaunchURLFallback(html, upstreamHost)
}

func azadLaunchURLFallback(html, upstreamHost string) string {
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

func newAzadClient() (*http.Client, *cookiejar.Jar, *cookieRecord, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, nil, nil, err
	}
	record := &cookieRecord{}
	client := &http.Client{
		Timeout: azadHTTPWait,
		Jar:     jar,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			base := http.DefaultTransport
			resp, err := base.RoundTrip(req)
			if err == nil && resp != nil {
				record.note(resp.Cookies())
			}
			return resp, err
		}),
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

func seedAzadPortal(jar *cookiejar.Jar, cookies []automation.Cookie, proxyHost string) {
	automation.SeedCookieJar(jar, cookies, "azadseo.com")
	automation.SeedCookieJar(jar, cookies, "members.azadseo.com")
	widened := make([]automation.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		domain := strings.ToLower(strings.TrimSpace(cookie.Domain))
		if domain != "" && !strings.Contains(domain, "azadseo.com") {
			continue
		}
		cookie.Domain = ".azadseo.com"
		cookie.HostOnly = false
		widened = append(widened, cookie)
	}
	if len(widened) > 0 && strings.TrimSpace(proxyHost) != "" {
		automation.SeedCookieJar(jar, widened, proxyHost)
	}
}

func portalCookies(jar *cookiejar.Jar) []automation.Cookie {
	return automation.JarCookies(jar, []string{
		azadBaseURL + "/",
		azadLoginURL,
		azadMemberURL,
		"https://azadseo.com/",
	})
}

func mapSemrushCookies(jar *cookiejar.Jar, record *cookieRecord, landed string, target semrushTarget) []automation.Cookie {
	hosts := []string{target.proxyHost, "azadseo.com", "members.azadseo.com"}
	hosts = append(hosts, target.readHosts...)
	if parsed, err := url.Parse(landed); err == nil && parsed.Host != "" {
		hosts = append(hosts, parsed.Host)
	}
	seen := map[string]struct{}{}
	var out []automation.Cookie
	for _, host := range hosts {
		host = strings.TrimPrefix(strings.TrimSpace(host), ".")
		if host == "" {
			continue
		}
		raw := "https://" + host + "/"
		if strings.HasPrefix(host, "127.") || strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "[::1]") {
			raw = "http://" + host + "/"
		}
		parsed, err := url.Parse(raw)
		if err != nil {
			continue
		}
		for _, cookie := range jar.Cookies(parsed) {
			if cookie.Name == "" || strings.Contains(strings.ToLower(cookie.Name), "amember") {
				continue
			}
			key := cookie.Name + "|" + cookie.Value
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			path := cookie.Path
			httpOnly := cookie.HttpOnly
			secure := cookie.Secure
			expires := cookie.Expires
			domain := target.cookieDomain
			var sameSite *string
			if saved, ok := record.get(cookie.Name, cookie.Value); ok {
				if saved.Path != "" {
					path = saved.Path
				}
				httpOnly = saved.HttpOnly
				secure = saved.Secure
				if !saved.Expires.IsZero() {
					expires = saved.Expires
				}
				sameSite = sameSiteLabel(saved.SameSite)
				if target.keepSetDomain {
					domain = domainFromSetCookie(saved, cookie.Name, target.cookieDomain, target.otherDomain)
				}
			} else if target.keepSetDomain {
				domain = domainFromSetCookie(http.Cookie{}, cookie.Name, target.cookieDomain, target.otherDomain)
			}
			if path == "" {
				path = "/"
			}
			session := expires.IsZero()
			var exp *float64
			if !session {
				value := float64(expires.Unix())
				exp = &value
			}
			out = append(out, automation.Cookie{
				Name:           cookie.Name,
				Value:          cookie.Value,
				Domain:         domain,
				Path:           path,
				HostOnly:       false,
				HTTPOnly:       httpOnly,
				Secure:         secure,
				SameSite:       sameSite,
				Session:        session,
				ExpirationDate: exp,
			})
		}
	}
	return out
}

func domainFromSetCookie(saved http.Cookie, name, proxyFallback, otherFallback string) string {
	domain := strings.TrimSpace(saved.Domain)
	if domain != "" {
		domain = strings.TrimPrefix(domain, ".")
		return "." + domain
	}
	if strings.EqualFold(name, "proxy_token") && proxyFallback != "" {
		return proxyFallback
	}
	if otherFallback != "" {
		return otherFallback
	}
	return proxyFallback
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

func semrushFreshCookies(mapped, portal []automation.Cookie) []automation.Cookie {
	known := map[string]struct{}{}
	for _, cookie := range portal {
		if cookie.Name == "" || cookie.Value == "" {
			continue
		}
		known[cookie.Name+"|"+cookie.Value] = struct{}{}
	}
	out := make([]automation.Cookie, 0, len(mapped))
	for _, cookie := range mapped {
		if cookie.Name == "" || cookie.Value == "" {
			continue
		}
		if _, ok := known[cookie.Name+"|"+cookie.Value]; ok {
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
