package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"recloud.dev/cdncache"
)

func TestRewriteKeepsBrowserOnPublicHost(t *testing.T) {
	in := []byte(`<a href="https://cg02.amztoolsconsultant.com/analytics/overview/">x</a><script src="https://static.amztoolsconsultant.com/app.js"></script><script src="https://de.semrush.com/app.js"></script>{"cookie_domain":".amztoolsconsultant.com","host":"cg02.amztoolsconsultant.com","bad":"cg.amzccg.gt4rents.com"}`)
	out := string(rewriteText(in, "cg02.amztoolsconsultant.com", "https://amzccg.gt4rents.com"))
	if strings.Contains(out, "amztoolsconsultant.com") || strings.Contains(out, "semrush.com") || strings.Contains(out, "cg.amzccg") || strings.Contains(out, "cg02.amzccg") {
		t.Fatalf("upstream host leaked: %s", out)
	}
	if !strings.Contains(out, "https://amzccg.gt4rents.com/analytics/overview/") {
		t.Fatal("public origin was not applied")
	}
	if !strings.Contains(out, ".amzccg.gt4rents.com") {
		t.Fatal("cookie domain was not rewritten")
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
	if _, ok := rewriteSetCookie("PHPSESSID=secret; Path=/; Secure", "cg02.amztoolsconsultant.com", "https://amzccg.gt4rents.com"); ok {
		t.Fatal("upstream session cookie must not reach the browser")
	}
	got, ok := rewriteSetCookie("snav_last_sidebar_tk=toolkit_seo; Domain=.semrush.com; Path=/", "cg02.amztoolsconsultant.com", "https://amzccg.gt4rents.com")
	if !ok || !strings.Contains(got, "Domain=amzccg.gt4rents.com") || strings.Contains(got, "semrush.com") {
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
		PublicHost:   "amzccg.gt4rents.com",
		PublicScheme: "http",
		ToolName:     "Amztoolsconsultant ChatGPT",
	}
	req := httptest.NewRequest(http.MethodGet, "http://amzccg.gt4rents.com/login?url=%2F&host=cg.amzccg.gt4rents.com&ssl=on", nil)
	req.Header.Set("Accept", "text/html")
	req.Host = "amzccg.gt4rents.com"
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

func TestOnlyLayoutNotificationIsHidden(t *testing.T) {
	in := []byte(`<html><head></head><body><ul class="experiment-sidebar-navigationList_f1guxiff"><a data-name="support-lnk">Support</a></ul><a class="docs-lnk">Docs</a></body></html>`)
	out := string(injectDeviceHTML(in, "sagar"))
	if !strings.Contains(out, `data-tm-viewer-name`) || !strings.Contains(out, `"sagar"`) {
		t.Fatal("viewer username script missing")
	}
	if !strings.Contains(out, `span.min-w-0.truncate.text-default > span.min-w-0.flex-1.truncate`) {
		t.Fatal("viewer name selector missing")
	}
	if !strings.Contains(out, `ul[class*="experiment-sidebar-navigationList"]{display:none!important}`) {
		t.Fatal("sidebar list hide style missing")
	}
	if !strings.Contains(out, `docs-lnk`) || strings.Contains(out, `docs-lnk{display:none`) {
		t.Fatal("docs link was hidden")
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
	req := httptest.NewRequest(http.MethodPost, "http://amzccg.gt4rents.com/gateway/experimentation/properties", nil)
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
	next, ok := sameSiteRequestURI("https://cg02.amztoolsconsultant.com/docs?x=1", "cg02.amztoolsconsultant.com", "https://amzccg.gt4rents.com")
	if !ok || next != "/docs?x=1" {
		t.Fatalf("same-site redirect %q %v", next, ok)
	}
	next, ok = sameSiteRequestURI("https://members.amztoolsconsultant.com/protect/new-rewrite?f=77&url=/", "cg02.amztoolsconsultant.com", "https://amzccg.gt4rents.com")
	if !ok || !strings.HasPrefix(next, "https://members.amztoolsconsultant.com/protect/new-rewrite") {
		t.Fatalf("members hop %q %v", next, ok)
	}
	if _, ok := sameSiteRequestURI("https://example.com/docs", "cg02.amztoolsconsultant.com", "https://amzccg.gt4rents.com"); ok {
		t.Fatal("off-site redirect must stay with the browser")
	}
	if _, ok := sameSiteRequestURI("/cdn-cgi/challenge-platform/h/g/scripts/precursor/main.js", "cg02.amztoolsconsultant.com", "https://amzccg.gt4rents.com"); ok {
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
	cfg := Config{TargetURL: upstream.URL, HomePath: "/", PublicHost: "amzccg.gt4rents.com", PublicScheme: "http", ToolName: "Amztoolsconsultant ChatGPT"}
	req := httptest.NewRequest(http.MethodGet, "http://amzccg.gt4rents.com/", nil)
	req.Header.Set("Accept", "text/html")
	req.Host = "amzccg.gt4rents.com"
	rr := httptest.NewRecorder()
	proxyUpstream(rr, req, cfg, ToolAccount{Cookie: "PHPSESSID=session"})
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "My docs") {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
}
