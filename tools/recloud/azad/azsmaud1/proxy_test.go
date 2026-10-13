package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"recloud.dev/cdncache"
)

func TestRewriteKeepsBrowserOnPublicHost(t *testing.T) {
	in := []byte(`<a href="https://siteaudit1.xemrush.site/analytics/overview/">x</a><script src="https://de.semrush.com/app.js"></script>{"cookie_domain":".semrush.com"}`)
	out := string(rewriteText(in, "siteaudit1.xemrush.site", "https://azsmaud1.gt4rents.com"))
	if strings.Contains(out, "siteaudit1.xemrush.site") || strings.Contains(out, "semrush.com") {
		t.Fatalf("upstream host leaked: %s", out)
	}
	if !strings.Contains(out, "https://azsmaud1.gt4rents.com/analytics/overview/") {
		t.Fatal("public origin was not applied")
	}
	if !strings.Contains(out, ".azsmaud1.gt4rents.com") {
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
	if _, ok := rewriteSetCookie("proxy_token=secret; Path=/; Secure", "siteaudit1.xemrush.site", "https://azsmaud1.gt4rents.com"); ok {
		t.Fatal("upstream session cookie must not reach the browser")
	}
	got, ok := rewriteSetCookie("snav_last_sidebar_tk=toolkit_seo; Domain=.semrush.com; Path=/", "siteaudit1.xemrush.site", "https://azsmaud1.gt4rents.com")
	if !ok || !strings.Contains(got, "Domain=azsmaud1.gt4rents.com") || strings.Contains(got, "semrush.com") {
		t.Fatal("sidebar cookie domain was not rewritten")
	}
}

func TestPublicHeaderIsLoggedOut(t *testing.T) {
	body := []byte(`<div class="srf-header__buttons"><a href="/login/?src=header" class="srf-header__link-button srf-login-btn">Log In</a><a href="/signup/?src=header" class="srf-header__link-button srf-register-btn">Start for free</a></div>`)
	if !bodyLooksLoggedOut(body) {
		t.Fatal("public login header should count as logged out")
	}
	if bodyLooksLoggedOut([]byte(`<a href="/siteaudit/">open</a>`)) {
		t.Fatal("the app page is not a logout")
	}
}

func TestLogoutPath(t *testing.T) {
	if !isLogoutPath("/sso/logout") || isLogoutPath("/analytics/overview/") {
		t.Fatal("logout path detection")
	}
}
