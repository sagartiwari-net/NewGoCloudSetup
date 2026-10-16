package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"recloud.dev/cdncache"
)

func TestRewriteKeepsBrowserOnPublicHost(t *testing.T) {
	in := []byte(`<a href="https://ns01.amztoolsconsultant.com/analytics/overview/">x</a><script src="https://static.amztoolsconsultant.com/app.js"></script><script src="https://de.semrush.com/app.js"></script>{"cookie_domain":".amztoolsconsultant.com","host":"ns01.amztoolsconsultant.com","bad":"ns01.amzcnichs.gt4rents.com"}`)
	out := string(rewriteText(in, "ns01.amztoolsconsultant.com", "https://amzcnichs.gt4rents.com"))
	if strings.Contains(out, "amztoolsconsultant.com") || strings.Contains(out, "semrush.com") || strings.Contains(out, "ns01.amzcnichs") {
		t.Fatalf("upstream host leaked: %s", out)
	}
	if !strings.Contains(out, "https://amzcnichs.gt4rents.com/analytics/overview/") {
		t.Fatal("public origin was not applied")
	}
	if !strings.Contains(out, ".amzcnichs.gt4rents.com") {
		t.Fatal("cookie domain was not rewritten")
	}
}

func TestProtectLinkKeepsUpstreamSite(t *testing.T) {
	in := []byte(`<link rel="stylesheet" href="https://members.amztoolsconsultant.com/protect/new-rewrite?f=132&amp;url=/app.css&amp;host=ns01.amztoolsconsultant.com&amp;ssl=on">`)
	out := string(rewriteText(in, "ns01.amztoolsconsultant.com", "https://amzcnichs.gt4rents.com"))
	if !strings.Contains(out, "https://amzcnichs.gt4rents.com/protect/new-rewrite?") {
		t.Fatalf("protect link was not kept on this tool: %s", out)
	}
	if !strings.Contains(out, "host=ns01.amztoolsconsultant.com") || strings.Contains(out, "host=amzcnichs.gt4rents.com") {
		t.Fatalf("member site host was rewritten: %s", out)
	}
	base, err := url.Parse("https://ns01.amztoolsconsultant.com")
	if err != nil {
		t.Fatal(err)
	}
	got := membersProtectURL(base, "/protect/new-rewrite?f=132&url=/app.css&host=amzcnichs.gt4rents.com&ssl=on", "https://amzcnichs.gt4rents.com")
	if got != "https://members.amztoolsconsultant.com/protect/new-rewrite?f=132&url=/app.css&host=ns01.amztoolsconsultant.com&ssl=on" {
		t.Fatalf("protect target %s", got)
	}
}

func TestStylesheetRedirectIsFollowed(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app.css" {
			http.Redirect(w, r, "/static/app.css", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/css")
		_, _ = w.Write([]byte("body{color:#111}"))
	}))
	defer upstream.Close()
	cfg := Config{TargetURL: upstream.URL, HomePath: "/scraper/", PublicHost: "amzcnichs.gt4rents.com", PublicScheme: "http", ToolName: "Amztoolsconsultant Niche Scraper"}
	req := httptest.NewRequest(http.MethodGet, "http://amzcnichs.gt4rents.com/app.css", nil)
	req.Host = "amzcnichs.gt4rents.com"
	req.Header.Set("Accept", "text/css")
	req.Header.Set("Sec-Fetch-Dest", "style")
	rr := httptest.NewRecorder()
	proxyUpstream(rr, req, cfg, ToolAccount{Cookie: "PHPSESSID=session"})
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "color:#111") {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
}

func TestStaticCachePolicy(t *testing.T) {
	if !isCacheableStaticPath("/__static__/webpack/app.js") {
		t.Fatal("js should cache")
	}
	if isCacheableStaticPath("/analytics/overview/") {
		t.Fatal("html path should not cache")
	}
	req := httptest.NewRequest(http.MethodGet, "/analytics/overview/", nil)
	if cdncache.RequestKey(req, isCacheableStaticPath(req.URL.Path)) != "" {
		t.Fatal("html path must not be cached")
	}
}

func TestCookieJSONBecomesHeader(t *testing.T) {
	raw := `[{"name":"PHPSESSID","value":"abc","hostOnly":false},{"name":"snav_last_sidebar_tk","value":"toolkit_seo","hostOnly":true}]`
	got := parseCookieFromDB(raw)
	if !strings.Contains(got, "PHPSESSID=abc") || !strings.Contains(got, "snav_last_sidebar_tk=toolkit_seo") {
		t.Fatal("cookie json did not become a header")
	}
}

func TestSetCookieDropsUpstreamSession(t *testing.T) {
	if _, ok := rewriteSetCookie("PHPSESSID=secret; Path=/; Secure", "ns01.amztoolsconsultant.com", "https://amzcnichs.gt4rents.com"); ok {
		t.Fatal("upstream session cookie must not reach the browser")
	}
	got, ok := rewriteSetCookie("snav_last_sidebar_tk=toolkit_seo; Domain=.semrush.com; Path=/", "ns01.amztoolsconsultant.com", "https://amzcnichs.gt4rents.com")
	if !ok || !strings.Contains(got, "Domain=amzcnichs.gt4rents.com") || strings.Contains(got, "semrush.com") {
		t.Fatal("sidebar cookie domain was not rewritten")
	}
}

func TestLoginPageRetriesThenOpensHome(t *testing.T) {
	oldWait := loginRetryWait
	loginRetryWait = 0
	t.Cleanup(func() { loginRetryWait = oldWait })

	var hits int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if hits < loginAttempts {
			_, _ = w.Write([]byte(`<h1>Access Denied</h1><p>This section is restricted.</p>`))
			return
		}
		if r.URL.Path != "/analytics/overview/" {
			t.Errorf("retry path %s", r.URL.RequestURI())
		}
		_, _ = w.Write([]byte(`<html><body>overview ready</body></html>`))
	}))
	defer upstream.Close()

	cfg := Config{
		TargetURL:    upstream.URL,
		HomePath:     "/analytics/overview/?searchType=domain",
		PublicHost:   "amzcnichs.gt4rents.com",
		PublicScheme: "http",
		ToolName:     "Amztoolsconsultant Niche Scraper",
	}
	req := httptest.NewRequest(http.MethodGet, "http://amzcnichs.gt4rents.com/login?url=%2F&host=cg.amzcnichs.gt4rents.com&ssl=on", nil)
	req.Header.Set("Accept", "text/html")
	req.Host = "amzcnichs.gt4rents.com"
	rr := httptest.NewRecorder()
	proxyUpstream(rr, req, cfg, ToolAccount{Cookie: "PHPSESSID=fresh"})
	if hits != loginAttempts {
		t.Fatalf("upstream hits=%d", hits)
	}
	if rr.Code != http.StatusFound {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Location") != cfg.HomePath {
		t.Fatalf("location %s", rr.Header().Get("Location"))
	}
	if strings.Contains(rr.Body.String(), "restricted") {
		t.Fatal("upstream restricted page was shown")
	}
}

func TestOnlyThreeNavItemsAreHidden(t *testing.T) {
	in := []byte(`<html><head></head><body><li class="dropdown"><a href="/getting-started.php">Tutorials</a></li><a href="/scraper/">Open</a><div class="container-fluid"><p>Results</p></div></body></html>`)
	out := string(injectDeviceHTML(in, "sagar"))
	for _, sel := range []string{
		`li.dropdown:has(a[href="/change-password.php"]){display:none!important}`,
		`div.container-fluid:has(> .navbar-header):has(#bs-example-navbar-collapse-1){display:none!important}`,
		`li:has(> a[href="/affiliate.php"]){display:none!important}`,
	} {
		if !strings.Contains(out, sel) {
			t.Fatalf("missing hide %s", sel)
		}
	}
	if strings.Contains(out, `.dropdown{display:none`) || strings.Contains(out, `li{display:none`) || strings.Contains(out, `.container-fluid{display:none`) {
		t.Fatal("nav hide is broader than the three pasted items")
	}
	if !strings.Contains(out, `href="/getting-started.php"`) || !strings.Contains(out, `>Results</p>`) {
		t.Fatal("tutorials or page content was removed")
	}
}

func TestScraperPageIsNotRestyled(t *testing.T) {
	in := []byte(`<html><head></head><body><a href="/scraper/">Open</a><span class="user"><img class="user__avatar" src="/a.png"><div class="user__dropdown">Menu</div></span></body></html>`)
	out := string(injectDeviceHTML(in, "sagar"))
	if strings.Contains(out, `span.user:has`) || strings.Contains(out, `user__dropdown){display:none`) {
		t.Fatal("niche scraper must not hide the account menu")
	}
	if !strings.Contains(out, `class="user__dropdown"`) {
		t.Fatal("account menu was removed")
	}
}

func TestLibraryRequestHidesPublicHost(t *testing.T) {
	var host, forwarded string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host = r.Header.Get("X-Forwarded-Host")
		forwarded = r.Header.Get("Forwarded")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body>library</body></html>`))
	}))
	defer upstream.Close()
	cfg := Config{TargetURL: upstream.URL, HomePath: "/scraper/", PublicHost: "amzcnichs.gt4rents.com", PublicScheme: "http", ToolName: "Amztoolsconsultant Niche Scraper"}
	req := httptest.NewRequest(http.MethodGet, "http://amzcnichs.gt4rents.com/scraper/", nil)
	req.Host = "amzcnichs.gt4rents.com"
	req.Header.Set("Accept", "text/html")
	req.Header.Set("X-Forwarded-Host", "amzcnichs.gt4rents.com")
	req.Header.Set("Forwarded", "host=amzcnichs.gt4rents.com")
	rr := httptest.NewRecorder()
	proxyUpstream(rr, req, cfg, ToolAccount{Cookie: "PHPSESSID=session"})
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "library") {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	if host != "" || forwarded != "" {
		t.Fatalf("public host leaked upstream host=%q forwarded=%q", host, forwarded)
	}
}

func TestLogoutPath(t *testing.T) {
	if !isLogoutPath("/sso/logout") || !isLogoutPath("/login") || isLogoutPath("/analytics/overview/") {
		t.Fatal("logout path detection")
	}
	if !bodyLooksLoggedOut([]byte(`<h1>Access Denied</h1><p>This section is restricted.</p>`)) {
		t.Fatal("restricted page should count as logged out")
	}
	if !bodyLooksLoggedOut([]byte(`<h1>Member Login</h1><a>Forgot password?</a>`)) {
		t.Fatal("consultant login page should count as logged out")
	}
	if !bodyLooksLoggedOut([]byte(`<h1>Members Area</h1><p>Login to your Account</p><a>Forgot password?</a>`)) {
		t.Fatal("account login page should count as logged out")
	}
	if bodyLooksLoggedOut([]byte(`<p>Your session expired while the reply was still streaming.</p>`)) {
		t.Fatal("a chat message that mentions session expired is not logout")
	}
	if bodyLooksLoggedOut([]byte(`<a class="srf-login-btn">Log In</a>`)) {
		t.Fatal("a login button alone is not logout")
	}
}

func TestBrowserSessionCookiesAreForwarded(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://amzcnichs.gt4rents.com/gateway/experimentation/properties", nil)
	req.Header.Set("Cookie", "ct_session=panel-session; grauth=from-browser; csrf_token=tok")
	got := cookiesForUpstream(ToolAccount{Cookie: "PHPSESSID=session"}, req)
	if !strings.Contains(got, "PHPSESSID=session") || !strings.Contains(got, "grauth=from-browser") || !strings.Contains(got, "csrf_token=tok") {
		t.Fatalf("browser session cookies were dropped: %s", got)
	}
	if strings.Contains(got, "ct_session") || strings.Contains(got, "panel-session") {
		t.Fatalf("panel session leaked upstream: %s", got)
	}
}

func TestSessionCookieFollowsUpstream(t *testing.T) {
	got := absorbSetCookies("PHPSESSID=session", http.Header{"Set-Cookie": []string{"grauth=fresh; Path=/; Secure", "csrf_token=tok; Path=/"}})
	if !strings.Contains(got, "PHPSESSID=session") || !strings.Contains(got, "grauth=fresh") || !strings.Contains(got, "csrf_token=tok") {
		t.Fatalf("cookies were not merged: %s", got)
	}
	next, ok := sameSiteRequestURI("https://ns01.amztoolsconsultant.com/docs?x=1", "ns01.amztoolsconsultant.com", "https://amzcnichs.gt4rents.com")
	if !ok || next != "/docs?x=1" {
		t.Fatalf("same-site redirect %q %v", next, ok)
	}
	next, ok = sameSiteRequestURI("https://members.amztoolsconsultant.com/protect/new-rewrite?f=77&url=/", "ns01.amztoolsconsultant.com", "https://amzcnichs.gt4rents.com")
	if !ok || !strings.HasPrefix(next, "https://members.amztoolsconsultant.com/protect/new-rewrite") {
		t.Fatalf("members hop %q %v", next, ok)
	}
	if _, ok := sameSiteRequestURI("https://example.com/docs", "ns01.amztoolsconsultant.com", "https://amzcnichs.gt4rents.com"); ok {
		t.Fatal("off-site redirect must stay with the browser")
	}
	if _, ok := sameSiteRequestURI("/cdn-cgi/challenge-platform/h/g/scripts/precursor/main.js", "ns01.amztoolsconsultant.com", "https://amzcnichs.gt4rents.com"); ok {
		t.Fatal("cloudflare challenge must not be followed")
	}

	oldWait := loginRetryWait
	loginRetryWait = 0
	t.Cleanup(func() { loginRetryWait = oldWait })
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.SetCookie(w, &http.Cookie{Name: "grauth", Value: "fresh", Path: "/"})
			http.Redirect(w, r, "/docs", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=session") || !strings.Contains(r.Header.Get("Cookie"), "grauth=fresh") {
			_, _ = w.Write([]byte(`<h1>Member Login</h1><a>Forgot password?</a>`))
			return
		}
		_, _ = w.Write([]byte(`<html><body>My docs</body></html>`))
	}))
	defer upstream.Close()
	cfg := Config{TargetURL: upstream.URL, HomePath: "/", PublicHost: "amzcnichs.gt4rents.com", PublicScheme: "http", ToolName: "Amztoolsconsultant Niche Scraper"}
	req := httptest.NewRequest(http.MethodGet, "http://amzcnichs.gt4rents.com/", nil)
	req.Header.Set("Accept", "text/html")
	req.Host = "amzcnichs.gt4rents.com"
	rr := httptest.NewRecorder()
	proxyUpstream(rr, req, cfg, ToolAccount{Cookie: "PHPSESSID=session"})
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "My docs") {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
}
