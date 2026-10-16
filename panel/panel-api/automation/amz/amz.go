package amz

// Recipe is the Amz Tools Consultant HTTP login.
// The homepage job stores the member cookie and that same cookie is written to Helium10 Diamond.
// Keepa is the only tool job: it opens the Keepa page, follows /cloud/kp01, and saves the browseanywhere cookie.

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
	baseURL        = "https://members.amztoolsconsultant.com"
	loginURL       = "https://members.amztoolsconsultant.com/login"
	memberURL      = "https://members.amztoolsconsultant.com/member"
	keepaPageURL   = "https://members.amztoolsconsultant.com/page/b7m84YAgLseP+Keepa+New+Access"
	keepaLaunchURL = "https://members.amztoolsconsultant.com/cloud/kp01"
	httpWait       = 25 * time.Second
)

var (
	attemptRE = regexp.MustCompile(`(?i)name=["']login_attempt_id["'][^>]*value=["']([^"']+)["']|value=["']([^"']+)["'][^>]*name=["']login_attempt_id["']`)
	hrefRE    = regexp.MustCompile(`(?i)href\s*=\s*["']([^"']+)["']`)
	formRE    = regexp.MustCompile(`(?is)<form([^>]*)>(.*?)</form>`)
	inputRE   = regexp.MustCompile(`(?i)<input([^>]+)>`)
	attrRE    = regexp.MustCompile(`(?i)([a-z0-9_-]+)\s*=\s*["']([^"']*)["']`)
)

type Recipe struct{}

func (Recipe) PortalHost() string { return "amztoolsconsultant.com" }

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
	return true, HomepageCookies(updated), nil
}

func (Recipe) Login(ctx context.Context, username, password string) ([]automation.Cookie, error) {
	client, jar, _, err := newClient()
	if err != nil {
		return nil, automation.ErrTimeout
	}
	page, _, err := get(ctx, client, loginURL, baseURL)
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("amember_login", username)
	form.Set("amember_pass", password)
	if match := attemptRE.FindStringSubmatch(page); len(match) > 1 {
		token := match[1]
		if token == "" && len(match) > 2 {
			token = match[2]
		}
		if token != "" {
			form.Set("login_attempt_id", token)
		}
	}
	body, landed, err := postForm(ctx, client, loginURL, loginURL, form)
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
	cookies := HomepageCookies(portalCookies(jar))
	log.Printf("[automation] amz login page=%s cookies=%s", automation.Where(memberLanded), automation.Names(cookies))
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
	switch targetKind(parsed) {
	case "diamond":
		cookies := HomepageCookies(portal)
		log.Printf("[automation] amz diamond uses homepage cookies=%s", automation.Names(cookies))
		if len(cookies) == 0 {
			return nil, automation.ErrNoCookie
		}
		return cookies, nil
	case "keepa":
		return captureKeepa(ctx, portal, parsed)
	default:
		return nil, automation.ErrUpstream
	}
}

// HomepageCookies is the member login cookie, widened so Helium10 Diamond can send it.
func HomepageCookies(portal []automation.Cookie) []automation.Cookie {
	out := make([]automation.Cookie, 0, len(portal))
	seen := map[string]struct{}{}
	for _, cookie := range portal {
		name := strings.TrimSpace(cookie.Name)
		if name == "" || cookie.Value == "" || strings.Contains(strings.ToLower(name), "amember") {
			continue
		}
		key := name + "|" + cookie.Value
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		cookie.Name = name
		cookie.Domain = ".amztoolsconsultant.com"
		cookie.HostOnly = false
		if cookie.Path == "" {
			cookie.Path = "/"
		}
		out = append(out, cookie)
	}
	return out
}

func targetKind(parsed *url.URL) string {
	host := strings.ToLower(parsed.Hostname())
	path := strings.ToLower(parsed.EscapedPath())
	if strings.Contains(host, "browseanywhere.com") || strings.Contains(path, "kp01") || strings.Contains(host, "keepa") {
		return "keepa"
	}
	if host == "ht01.amztoolsconsultant.com" || strings.Contains(path, "black-box") || strings.Contains(path, "diamond") {
		return "diamond"
	}
	return ""
}

func captureKeepa(ctx context.Context, portal []automation.Cookie, upstream *url.URL) ([]automation.Cookie, error) {
	client, jar, record, err := newClient()
	if err != nil {
		return nil, automation.ErrTimeout
	}
	seedPortal(jar, portal)
	page, landed, err := get(ctx, client, keepaPageURL, memberURL)
	if err != nil {
		return nil, err
	}
	if showsLogin(page, landed) {
		return nil, automation.At(automation.ErrLoginPage, landed)
	}
	if denied(page) {
		return nil, automation.At(automation.ErrDenied, landed)
	}
	launch := keepaLaunch(page, landed)
	if launch == "" {
		launch = keepaLaunchURL
	}
	log.Printf("[automation] amz keepa open %s", automation.Where(launch))
	page, landed, err = get(ctx, client, launch, keepaPageURL)
	if err != nil {
		return nil, err
	}
	if showsLogin(page, landed) {
		return nil, automation.At(automation.ErrLoginPage, landed)
	}
	if action, values, ok := browseAnywhereForm(page, landed); ok {
		log.Printf("[automation] amz keepa form %s", automation.Where(action))
		page, landed, err = postForm(ctx, client, action, landed, values)
		if err != nil {
			return nil, err
		}
	}
	if isAuthLogin(landed) {
		if action, values, ok := browseAnywhereForm(page, landed); ok {
			page, landed, err = postForm(ctx, client, action, landed, values)
		} else {
			page, landed, err = postJSON(ctx, client, landed, "{}")
		}
		if err != nil {
			return nil, err
		}
	}
	if isAuthLogin(landed) || !strings.Contains(strings.ToLower(landed), "/amztoolsconsultant-kp01") {
		workspace := workspaceURL(upstream, landed)
		if workspace != "" && workspace != landed {
			log.Printf("[automation] amz keepa workspace %s", automation.Where(workspace))
			nextPage, nextLanded, nextErr := get(ctx, client, workspace, landed)
			if nextErr == nil {
				page, landed = nextPage, nextLanded
			}
		}
	}
	if isAuthLogin(landed) {
		return nil, automation.At(automation.ErrLoginPage, landed)
	}
	fresh := freshCookies(record.all(), portal)
	log.Printf("[automation] amz keepa page=%s cookies=%s", automation.Where(landed), automation.Names(fresh))
	if len(fresh) == 0 {
		return nil, automation.At(automation.ErrNoCookie, landed)
	}
	_ = page
	return fresh, nil
}

func workspaceURL(upstream *url.URL, landed string) string {
	host := ""
	scheme := "https"
	if upstream != nil && strings.Contains(strings.ToLower(upstream.Hostname()), "browseanywhere.com") {
		host = upstream.Hostname()
		if upstream.Scheme != "" {
			scheme = upstream.Scheme
		}
	}
	if landedHost := hostOf(landed); strings.Contains(strings.ToLower(landedHost), "browseanywhere.com") {
		host = landedHost
		if parsed, err := url.Parse(landed); err == nil && parsed.Scheme != "" {
			scheme = parsed.Scheme
		}
	}
	if host == "" {
		return ""
	}
	return scheme + "://" + host + "/amztoolsconsultant-kp01/"
}

func keepaLaunch(page, base string) string {
	parsed, _ := url.Parse(base)
	best := ""
	bestScore := 0
	for _, match := range hrefRE.FindAllStringSubmatch(page, -1) {
		if len(match) < 2 {
			continue
		}
		href := resolve(parsed, html.UnescapeString(match[1]))
		lower := strings.ToLower(href)
		score := 0
		if strings.Contains(lower, "/cloud/kp01") {
			score += 100
		}
		if strings.Contains(lower, "keepa") {
			score += 40
		}
		if strings.Contains(lower, "browseanywhere.com") && !strings.Contains(lower, "auth/login") {
			score += 80
		}
		if score > bestScore {
			bestScore = score
			best = href
		}
	}
	return best
}

func browseAnywhereForm(page, pageURL string) (string, url.Values, bool) {
	base, _ := url.Parse(pageURL)
	bestScore := 0
	bestAction := ""
	var best url.Values
	for _, match := range formRE.FindAllStringSubmatch(page, -1) {
		if len(match) < 3 {
			continue
		}
		attrs := parseAttrs(match[1])
		action := resolve(base, attrs["action"])
		if action == "" {
			action = pageURL
		}
		lower := strings.ToLower(action)
		score := 0
		if strings.Contains(lower, "browseanywhere.com") {
			score += 50
		}
		if strings.Contains(lower, "auth/login") || strings.Contains(lower, "/cloud/kp01") {
			score += 30
		}
		if strings.EqualFold(attrs["method"], "post") {
			score += 10
		}
		if score == 0 && strings.Contains(strings.ToLower(pageURL), "browseanywhere.com") {
			score = 5
		}
		if score < bestScore && bestScore > 0 {
			continue
		}
		if score == 0 {
			continue
		}
		values := url.Values{}
		for _, input := range inputRE.FindAllStringSubmatch(match[2], -1) {
			fields := parseAttrs(input[1])
			name := strings.TrimSpace(fields["name"])
			kind := strings.ToLower(fields["type"])
			if name == "" || kind == "submit" || kind == "button" {
				continue
			}
			values.Set(name, fields["value"])
		}
		bestScore = score
		bestAction = action
		best = values
	}
	if bestAction == "" {
		return "", nil, false
	}
	return bestAction, best, true
}

func parseAttrs(raw string) map[string]string {
	out := map[string]string{}
	for _, match := range attrRE.FindAllStringSubmatch(raw, -1) {
		out[strings.ToLower(match[1])] = html.UnescapeString(match[2])
	}
	return out
}

func resolve(base *url.URL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "#") || strings.HasPrefix(strings.ToLower(raw), "javascript:") {
		return ""
	}
	if base == nil {
		if strings.HasPrefix(raw, "http") {
			return raw
		}
		return ""
	}
	parsed, err := base.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.String()
}

func isAuthLogin(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.Contains(lower, "browseanywhere.com") && strings.Contains(lower, "auth/login")
}

func showsLogin(page, pageURL string) bool {
	if !portalPage(pageURL) {
		return false
	}
	lower := strings.ToLower(page)
	if strings.Contains(lower, "logout") || strings.Contains(lower, "/logout") {
		return false
	}
	if strings.Contains(strings.ToLower(pathOf(pageURL)), "/login") {
		return true
	}
	return strings.Contains(lower, "amember_login") && strings.Contains(lower, "amember_pass")
}

func showsDashboard(page, pageURL string) bool {
	if showsLogin(page, pageURL) {
		return false
	}
	lower := strings.ToLower(page)
	urlLower := strings.ToLower(pageURL)
	if !strings.Contains(urlLower, "/member") && !strings.Contains(lower, "logout") {
		return false
	}
	return strings.Contains(lower, "logout") || strings.Contains(lower, "/logout") ||
		strings.Contains(lower, "am-member-main") || strings.Contains(lower, "active subscriptions") ||
		strings.Contains(lower, "helpdesk") || strings.Contains(lower, "keepa")
}

func portalPage(pageURL string) bool {
	host := strings.ToLower(hostOf(pageURL))
	if host == "" || host == "members.amztoolsconsultant.com" || host == "amztoolsconsultant.com" || host == "www.amztoolsconsultant.com" {
		return true
	}
	parsed, err := url.Parse(baseURL)
	return err == nil && strings.EqualFold(parsed.Hostname(), host)
}

func rejected(page string) bool {
	lower := strings.ToLower(page)
	return strings.Contains(lower, "user name or password is incorrect") || strings.Contains(lower, "invalid username or password")
}

func denied(page string) bool {
	lower := strings.ToLower(page)
	return strings.Contains(lower, "access denied") || strings.Contains(lower, "do not have access") ||
		strings.Contains(lower, "no active subscription")
}

func seedPortal(jar *cookiejar.Jar, cookies []automation.Cookie) {
	widened := HomepageCookies(cookies)
	if len(widened) == 0 {
		return
	}
	automation.SeedCookieJar(jar, widened, "members.amztoolsconsultant.com")
	automation.SeedCookieJar(jar, widened, "amztoolsconsultant.com")
	if parsed, err := url.Parse(baseURL); err == nil {
		host := parsed.Hostname()
		if host != "" && !strings.HasSuffix(strings.ToLower(host), "amztoolsconsultant.com") {
			local := append([]automation.Cookie(nil), widened...)
			for i := range local {
				local[i].Domain = host
				local[i].HostOnly = true
			}
			automation.SeedCookieJar(jar, local, host)
		}
	}
}

func portalCookies(jar *cookiejar.Jar) []automation.Cookie {
	return automation.JarCookies(jar, []string{baseURL + "/", loginURL, memberURL})
}

func freshCookies(set []setCookie, portal []automation.Cookie) []automation.Cookie {
	known := map[string]struct{}{}
	for _, cookie := range portal {
		if cookie.Name != "" && cookie.Value != "" {
			known[cookie.Name+"|"+cookie.Value] = struct{}{}
		}
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
		} else if !strings.HasPrefix(domain, ".") {
			domain = "." + domain
		}
		path := cookie.Path
		if path == "" {
			path = "/"
		}
		session := cookie.Expires.IsZero()
		var exp *float64
		if !session {
			value := float64(cookie.Expires.Unix())
			exp = &value
		}
		out = append(out, automation.Cookie{
			Name: name, Value: cookie.Value, Domain: domain, Path: path, HostOnly: hostOnly,
			HTTPOnly: cookie.HTTPOnly, Secure: cookie.Secure, SameSite: sameSiteLabel(cookie.SameSite),
			Session: session, ExpirationDate: exp,
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
				log.Printf("[automation] amz hop %s failed before a response", automation.Where(requestURL(req)))
				return resp, err
			}
			if resp != nil {
				record.note(req, resp.Cookies())
				log.Printf("[automation] amz hop %s status=%d set=%s", automation.Where(requestURL(req)), resp.StatusCode, namesOf(resp.Cookies()))
			}
			return resp, nil
		}),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			log.Printf("[automation] amz redirect %s -> %s (%d)", automation.Where(requestURL(via[len(via)-1])), automation.Where(requestURL(req)), len(via))
			if len(via) >= 12 {
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
	return do(client, req.WithContext(ctx))
}

func postForm(ctx context.Context, client *http.Client, rawURL, referer string, form url.Values) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", automation.ErrUpstream
	}
	headers(req, referer)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if parsed, err := url.Parse(rawURL); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		req.Header.Set("Origin", parsed.Scheme+"://"+parsed.Host)
	}
	return do(client, req.WithContext(ctx))
}

func postJSON(ctx context.Context, client *http.Client, rawURL, body string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(body))
	if err != nil {
		return "", "", automation.ErrUpstream
	}
	headers(req, rawURL)
	req.Header.Set("Content-Type", "application/json")
	return do(client, req.WithContext(ctx))
}

func headers(req *http.Request, referer string) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
}

func do(client *http.Client, req *http.Request) (string, string, error) {
	log.Printf("[automation] amz %s %s", req.Method, automation.Where(req.URL.String()))
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, automation.ErrLoginPage) {
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
	log.Printf("[automation] amz %s landed %s status=%d bytes=%d login=%t", req.Method, automation.Where(final), resp.StatusCode, len(body), showsLogin(string(body), final))
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
