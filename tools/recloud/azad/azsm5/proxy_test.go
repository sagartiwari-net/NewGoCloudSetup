package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"recloud.dev/cdncache"
)

func TestRewriteKeepsBrowserOnPublicHost(t *testing.T) {
	in := []byte(`<a href="https://6.semrush.com.in/analytics/overview/">x</a><script src="https://static.semrush.com.in/app.js"></script><script src="https://de.semrush.com/app.js"></script>{"cookie_domain":".semrush.com.in"}`)
	out := string(rewriteText(in, "6.semrush.com.in", "https://azsm5.gt4rents.com"))
	if strings.Contains(out, "semrush.com") || strings.Contains(out, "gt4rents.com.in") {
		t.Fatalf("upstream host leaked: %s", out)
	}
	if !strings.Contains(out, "https://azsm5.gt4rents.com/analytics/overview/") {
		t.Fatal("public origin was not applied")
	}
	if !strings.Contains(out, ".azsm5.gt4rents.com") {
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
	if _, ok := rewriteSetCookie("proxy_token=secret; Path=/; Secure", "6.semrush.com.in", "https://azsm5.gt4rents.com"); ok {
		t.Fatal("upstream session cookie must not reach the browser")
	}
	got, ok := rewriteSetCookie("snav_last_sidebar_tk=toolkit_seo; Domain=.semrush.com; Path=/", "6.semrush.com.in", "https://azsm5.gt4rents.com")
	if !ok || !strings.Contains(got, "Domain=azsm5.gt4rents.com") || strings.Contains(got, "semrush.com") {
		t.Fatal("sidebar cookie domain was not rewritten")
	}
}

func TestLogoutPath(t *testing.T) {
	if !isLogoutPath("/sso/logout") || isLogoutPath("/analytics/overview/") {
		t.Fatal("logout path detection")
	}
}
