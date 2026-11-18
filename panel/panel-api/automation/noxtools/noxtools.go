package noxtools

// Recipe keeps the noxtools.com aMember session in Storage, then captures tool
// cookies the same way Azad does: portal cookie → product page → Server N link →
// follow redirects onto *.semrush.com.in and save JSON cookies for the job target.
//
// Jobs stay separate (nxsm1 vs nxaud1). One credential / Storage row is shared.

import (
	"context"
	"html"
	"io"
	"log"
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

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"

var (
	baseURL     = "https://noxtools.com"
	loginURL    = "https://noxtools.com/login"
	memberURL   = "https://noxtools.com/secure/member"
	semrushPage = "https://noxtools.com/secure/page/semrush"
	httpWait    = 30 * time.Second
)

var (
	anchorRE = regexp.MustCompile(`(?i)<a\b([^>]*)>`)
)

type Recipe struct{}

type captureTarget struct {
	kind         string // semrush | siteaudit
	pageURL      string
	proxyHost    string // semrush.noxtools.com
	serverN      int
	cookieDomain string // .1.semrush.com.in
	finalHost    string // 1.semrush.com.in
	fallbackLaunch string
}

func (Recipe) PortalHost() string { return "noxtools.com" }

func targetFor(upstreamURL string) (captureTarget, error) {
	parsed, err := url.Parse(strings.TrimSpace(upstreamURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return captureTarget{}, automation.ErrUpstream
	}
	host := strings.ToLower(parsed.Hostname())
	if split, _, err := net.SplitHostPort(host); err == nil {
		host = split
	}

	if n, ok := semrushServerN(host); ok {
		return captureTarget{
			kind:           "semrush",
			pageURL:        semrushPage,
			proxyHost:      "semrush.noxtools.com",
			serverN:        n,
			cookieDomain:   "." + host,
			finalHost:      host,
			fallbackLaunch: "https://semrush.noxtools.com/server" + strconv.Itoa(n) + ".php",
		}, nil
	}
	if n, ok := siteauditServerN(host); ok {
		// Site Audit buttons live on the same Semrush product page:
		// <a class="nt-sem-btn nt-sem-btn-audit" href="https://semrush.noxtools.com/siteaudit1.php">
		return captureTarget{
			kind:           "siteaudit",
			pageURL:        semrushPage,
			proxyHost:      "semrush.noxtools.com",
			serverN:        n,
			cookieDomain:   "." + host,
			finalHost:      host,
			fallbackLaunch: "https://semrush.noxtools.com/siteaudit" + strconv.Itoa(n) + ".php",
		}, nil
	}
	return captureTarget{}, automation.ErrUpstream
}

func semrushServerN(host string) (int, bool) {
	if host == "semrush.com.in" {
		return 1, true
	}
	if !strings.HasSuffix(host, ".semrush.com.in") {
		return 0, false
	}
	prefix := strings.TrimSuffix(host, ".semrush.com.in")
	if strings.HasPrefix(prefix, "siteaudit") {
		return 0, false
	}
	n, err := strconv.Atoi(prefix)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

func siteauditServerN(host string) (int, bool) {
	if !strings.HasPrefix(host, "siteaudit") || !strings.HasSuffix(host, ".semrush.com.in") {
		return 0, false
	}
	mid := strings.TrimSuffix(strings.TrimPrefix(host, "siteaudit"), ".semrush.com.in")
	n, err := strconv.Atoi(mid)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
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
	target, err := targetFor(upstreamURL)
	if err != nil {
		return nil, err
	}
	client, jar, record, err := newClient()
	if err != nil {
		return nil, automation.ErrTimeout
	}
	seedPortal(jar, portal, target.proxyHost)

	page, landed, err := get(ctx, client, target.pageURL, memberURL)
	if err != nil {
		return nil, err
	}
	if showsLogin(page, landed) {
		return nil, automation.At(automation.ErrLoginPage, landed)
	}

	launch := extractServerLaunch(page, target)
	if launch == "" {
		launch = target.fallbackLaunch
	}
	log.Printf("[automation] noxtools %s open %s", target.kind, automation.Where(launch))
	page, landed, err = get(ctx, client, launch, target.pageURL)
	if err != nil {
		return nil, err
	}
	if showsLogin(page, landed) {
		return nil, automation.At(automation.ErrLoginPage, landed)
	}

	// Only hop onto the final tool home when still on a Nox gateway host.
	// Avoids re-opening the real upstream when serverN.php already redirected there.
	landedHost := strings.ToLower(hostOf(landed))
	if landedHost == strings.ToLower(target.proxyHost) || strings.Contains(landedHost, "noxtools.com") {
		home := "https://" + target.finalHost + "/"
		if target.kind == "semrush" {
			home = "https://" + target.finalHost + "/analytics/overview/?searchType=domain"
		} else {
			home = "https://" + target.finalHost + "/siteaudit/"
		}
		page, landed, err = get(ctx, client, home, landed)
		if err != nil {
			return nil, err
		}
		_ = page
	}

	tool := mapToolCookies(jar, record, landed, target)
	fresh := freshToolCookies(tool, portal)
	log.Printf("[automation] noxtools capture kind=%s page=%s tool=%s fresh=%s", target.kind, automation.Where(landed), automation.Names(tool), automation.Names(fresh))
	if len(fresh) == 0 {
		return nil, automation.At(automation.ErrNoCookie, landed)
	}
	return fresh, nil
}

func extractServerLaunch(page string, target captureTarget) string {
	wantPath := launchPath(target)
	var fallback string
	for _, tag := range anchorRE.FindAllStringSubmatch(page, -1) {
		if len(tag) < 2 {
			continue
		}
		attrs := tag[1]
		href := attr(attrs, "href")
		if href == "" {
			continue
		}
		abs := absolute(href)
		lower := strings.ToLower(abs)
		class := strings.ToLower(attr(attrs, "class"))
		hostOK := strings.Contains(lower, strings.ToLower(target.proxyHost)) ||
			strings.Contains(lower, strings.ToLower(target.finalHost))
		pathOK := strings.Contains(lower, wantPath)
		btnOK := strings.Contains(class, "nt-sem-btn")
		if target.kind == "siteaudit" {
			btnOK = btnOK && strings.Contains(class, "nt-sem-btn-audit")
		} else if strings.Contains(class, "nt-sem-btn-audit") {
			btnOK = false
		}
		if hostOK && pathOK {
			return abs
		}
		if (btnOK || hostOK) && pathOK && fallback == "" {
			fallback = abs
		}
	}
	return fallback
}

func launchPath(target captureTarget) string {
	n := strconv.Itoa(target.serverN)
	if target.kind == "siteaudit" {
		return "/siteaudit" + n + ".php"
	}
	return "/server" + n + ".php"
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

func portalPage(pageURL string) bool {
	host := strings.ToLower(hostOf(pageURL))
	if host == "" || host == "noxtools.com" || host == "www.noxtools.com" {
		return true
	}
	parsed, err := url.Parse(baseURL)
	return err == nil && strings.EqualFold(parsed.Hostname(), host)
}

func seedPortal(jar *cookiejar.Jar, cookies []automation.Cookie, extraHosts ...string) {
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
		domain := strings.ToLower(strings.TrimSpace(cookie.Domain))
		if domain != "" && !strings.Contains(domain, "noxtools.com") && baseHost != "" && !strings.EqualFold(strings.TrimPrefix(domain, "."), baseHost) {
			// Keep portal cookies; ignore unrelated tool cookies if someone pasted a mixed set.
			if !strings.Contains(domain, baseHost) {
				continue
			}
		}
		c := cookie
		c.Domain = ".noxtools.com"
		c.HostOnly = false
		if c.Path == "" {
			c.Path = "/"
		}
		widened = append(widened, c)
		if baseHost != "" && !strings.HasSuffix(strings.ToLower(baseHost), "noxtools.com") {
			loc := c
			loc.Domain = baseHost
			loc.HostOnly = true
			local = append(local, loc)
		}
	}
	automation.SeedCookieJar(jar, widened, "noxtools.com")
	automation.SeedCookieJar(jar, widened, "www.noxtools.com")
	if len(local) > 0 {
		automation.SeedCookieJar(jar, local, baseHost)
	}
	for _, host := range extraHosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		automation.SeedCookieJar(jar, widened, host)
	}
}

func portalCookies(jar *cookiejar.Jar) []automation.Cookie {
	all := automation.JarCookies(jar, []string{baseURL + "/", loginURL, memberURL})
	var out []automation.Cookie
	for _, c := range all {
		name := strings.ToLower(c.Name)
		if name != "phpsessid" && name != "amember_nr" {
			continue
		}
		c.Domain = ".noxtools.com"
		c.HostOnly = false
		c.Secure = true
		out = append(out, c)
	}
	return out
}

func mapToolCookies(jar *cookiejar.Jar, record *cookieRecord, landed string, target captureTarget) []automation.Cookie {
	hosts := []string{target.finalHost, target.proxyHost, "noxtools.com", "semrush.noxtools.com", "siteaudit.noxtools.com"}
	if parsed, err := url.Parse(landed); err == nil && parsed.Host != "" {
		hosts = append(hosts, parsed.Hostname())
	}
	seen := map[string]struct{}{}
	var out []automation.Cookie
	for _, host := range hosts {
		host = strings.TrimPrefix(strings.TrimSpace(host), ".")
		if host == "" {
			continue
		}
		raw := "https://" + host + "/"
		parsed, err := url.Parse(raw)
		if err != nil {
			continue
		}
		for _, cookie := range jar.Cookies(parsed) {
			if skipToolCookie(cookie.Name) {
				continue
			}
			key := cookie.Name + "|" + cookie.Value
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			mapped := automation.Cookie{
				Name:     cookie.Name,
				Value:    cookie.Value,
				Domain:   target.cookieDomain,
				Path:     first(cookie.Path, "/"),
				HostOnly: false,
				HTTPOnly: cookie.HttpOnly,
				Secure:   cookie.Secure,
				Session:  cookie.Expires.IsZero(),
			}
			if saved, ok := record.get(cookie.Name, cookie.Value); ok {
				mapped.HTTPOnly = saved.HttpOnly
				mapped.Secure = saved.Secure
				if saved.Path != "" {
					mapped.Path = saved.Path
				}
				mapped.SameSite = sameSiteLabel(saved.SameSite)
				if !saved.Expires.IsZero() {
					mapped.Session = false
					v := float64(saved.Expires.Unix())
					mapped.ExpirationDate = &v
				}
				// Keep host-only snav-style cookies on the bare tool host when Set-Cookie had no Domain.
				if strings.TrimSpace(saved.Domain) == "" && strings.EqualFold(cookie.Name, "snav_last_sidebar_tk") {
					mapped.Domain = target.finalHost
					mapped.HostOnly = true
					mapped.Secure = false
				}
			} else if !cookie.Expires.IsZero() {
				v := float64(cookie.Expires.Unix())
				mapped.ExpirationDate = &v
			}
			out = append(out, mapped)
		}
	}
	return out
}

func skipToolCookie(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return true
	}
	if strings.Contains(lower, "amember") || lower == "phpsessid" {
		return true
	}
	return false
}

func freshToolCookies(mapped, portal []automation.Cookie) []automation.Cookie {
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
				log.Printf("[automation] noxtools hop %s failed before a response", automation.Where(requestURL(req)))
				return resp, err
			}
			if resp != nil {
				record.note(resp.Cookies())
				log.Printf("[automation] noxtools hop %s status=%d set=%s", automation.Where(requestURL(req)), resp.StatusCode, namesOf(resp.Cookies()))
			}
			return resp, nil
		}),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			log.Printf("[automation] noxtools redirect %s -> %s (%d)", automation.Where(requestURL(via[len(via)-1])), automation.Where(requestURL(req)), len(via))
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
		return "", "", automation.ErrUpstream
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	log.Printf("[automation] noxtools GET %s", automation.Where(rawURL))
	resp, err := client.Do(req)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "timeout") || strings.Contains(strings.ToLower(err.Error()), "deadline") {
			return "", "", automation.ErrTimeout
		}
		return "", "", automation.ErrUpstream
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	landed := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		landed = resp.Request.URL.String()
	}
	return string(body), landed, nil
}

func post(ctx context.Context, client *http.Client, rawURL string, form url.Values) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", automation.ErrUpstream
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Origin", baseURL)
	req.Header.Set("Referer", loginURL)
	resp, err := client.Do(req)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "timeout") || strings.Contains(strings.ToLower(err.Error()), "deadline") {
			return "", "", automation.ErrTimeout
		}
		return "", "", automation.ErrUpstream
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	landed := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		landed = resp.Request.URL.String()
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

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func requestURL(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}
	return req.URL.String()
}

func namesOf(cookies []*http.Cookie) string {
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

func first(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
