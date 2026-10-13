package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"recloud.dev/cdncache"
)

func TestRewriteKeepsBrowserOnPublicHost(t *testing.T) {
	in := []byte(`<a href="https://gra.markhorseotool.com/analytics/overview/">x</a><script src="https://static.markhorseotool.com/app.js"></script><script src="https://de.semrush.com/app.js"></script>{"cookie_domain":".markhorseotool.com","host":"gra.markhorseotool.com","bad":"gra.markhgram.gt4rents.com"}`)
	out := string(rewriteText(in, "gra.markhorseotool.com", "https://markhgram.gt4rents.com"))
	if strings.Contains(out, "markhorseotool.com") || strings.Contains(out, "semrush.com") || strings.Contains(out, "gra.markhgram") {
		t.Fatalf("upstream host leaked: %s", out)
	}
	if !strings.Contains(out, "https://markhgram.gt4rents.com/analytics/overview/") {
		t.Fatal("public origin was not applied")
	}
	if !strings.Contains(out, ".markhgram.gt4rents.com") {
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
	raw := `[{"name":"proxy_token","value":"abc","hostOnly":false},{"name":"snav_last_sidebar_tk","value":"toolkit_seo","hostOnly":true}]`
	got := parseCookieFromDB(raw)
	if !strings.Contains(got, "proxy_token=abc") || !strings.Contains(got, "snav_last_sidebar_tk=toolkit_seo") {
		t.Fatal("cookie json did not become a header")
	}
}

func TestSetCookieDropsUpstreamSession(t *testing.T) {
	if _, ok := rewriteSetCookie("proxy_token=secret; Path=/; Secure", "gra.markhorseotool.com", "https://markhgram.gt4rents.com"); ok {
		t.Fatal("upstream session cookie must not reach the browser")
	}
	got, ok := rewriteSetCookie("snav_last_sidebar_tk=toolkit_seo; Domain=.semrush.com; Path=/", "gra.markhorseotool.com", "https://markhgram.gt4rents.com")
	if !ok || !strings.Contains(got, "Domain=markhgram.gt4rents.com") || strings.Contains(got, "semrush.com") {
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
		PublicHost:   "markhgram.gt4rents.com",
		PublicScheme: "http",
		ToolName:     "Markhor Grammarly",
	}
	req := httptest.NewRequest(http.MethodGet, "http://markhgram.gt4rents.com/login?url=%2F&host=gra.markhgram.gt4rents.com&ssl=on", nil)
	req.Header.Set("Accept", "text/html")
	req.Host = "markhgram.gt4rents.com"
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
	in := []byte(`<html><head></head><body><div class="srf-layout__notification"><p>out of date</p></div><a class="srf-login-btn">Log In</a></body></html>`)
	out := string(injectDeviceHTML(in))
	if !strings.Contains(out, `.srf-layout__notification{display:none!important}`) {
		t.Fatal("notification hide style missing")
	}
	if !strings.Contains(out, `srf-login-btn`) {
		t.Fatal("login button was removed")
	}
	if strings.Contains(out, `srf-login-btn{display:none`) {
		t.Fatal("login button was hidden")
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
		t.Fatal("markhor login page should count as logged out")
	}
	if bodyLooksLoggedOut([]byte(`<a class="srf-login-btn">Log In</a>`)) {
		t.Fatal("a login button alone is not logout")
	}
}
