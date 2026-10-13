package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"recloud.dev/cdncache"
)

func TestRewriteKeepsBrowserOnPublicHost(t *testing.T) {
	in := []byte(`<a href="https://kwfinder.premiium.click/">x</a><script src="https://static.premiium.click/app.js"></script>{"cookie_domain":".premiium.click"}`)
	out := string(rewriteText(in, "kwfinder.premiium.click", "https://azkwfind.gt4rents.com"))
	if strings.Contains(out, "premiium.click") {
		t.Fatalf("upstream host leaked: %s", out)
	}
	if !strings.Contains(out, "https://azkwfind.gt4rents.com/") || !strings.Contains(out, "https://azkwfind.gt4rents.com/app.js") {
		t.Fatal("public origin was not applied")
	}
	if !strings.Contains(out, ".azkwfind.gt4rents.com") {
		t.Fatal("cookie domain was not rewritten")
	}
}

func TestMangoolsStaysOnRecloudHost(t *testing.T) {
	in := []byte(`<a href="https://mangools.com/users/sign_in">x</a><script src="https://app.mangools.com/kwfinder/app.js"></script>{"cookie_domain":".mangools.com"}`)
	out := string(rewriteText(in, "kwfinder.premiium.click", "https://azkwfind.gt4rents.com"))
	if strings.Contains(out, "mangools.com") {
		t.Fatalf("official host leaked: %s", out)
	}
	if !strings.Contains(out, "https://azkwfind.gt4rents.com/users/sign_in") || !strings.Contains(out, "https://azkwfind.gt4rents.com/kwfinder/app.js") {
		t.Fatalf("official urls were not kept on the recloud host: %s", out)
	}
	if !strings.Contains(out, ".azkwfind.gt4rents.com") {
		t.Fatal("mangools cookie domain was not rewritten")
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
	if _, ok := rewriteSetCookie("proxy_token=secret; Path=/; Secure", "kwfinder.premiium.click", "https://azkwfind.gt4rents.com"); ok {
		t.Fatal("upstream session cookie must not reach the browser")
	}
	got, ok := rewriteSetCookie("snav_last_sidebar_tk=toolkit_seo; Domain=.semrush.com; Path=/", "kwfinder.premiium.click", "https://azkwfind.gt4rents.com")
	if !ok || !strings.Contains(got, "Domain=azkwfind.gt4rents.com") || strings.Contains(got, "semrush.com") {
		t.Fatal("sidebar cookie domain was not rewritten")
	}
}

func TestDashboardExpiryIsLogout(t *testing.T) {
	if !bodyLooksLoggedOut([]byte("Session expired, access again from Dashboard")) {
		t.Fatal("dashboard expiry was not detected")
	}
}

func TestLogoutPath(t *testing.T) {
	if !isLogoutPath("/sso/logout") || isLogoutPath("/analytics/overview/") {
		t.Fatal("logout path detection")
	}
}
