package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"recloud.dev/cdncache"
)

func TestRewriteKeepsBrowserOnPublicHost(t *testing.T) {
	in := []byte(`<a href="https://siteaudit1.semrush.com.in/siteaudit/">x</a><script src="https://static.semrush.com.in/app.js"></script><script src="https://de.semrush.com/app.js"></script>{"cookie_domain":".semrush.com.in"}`)
	out := string(rewriteText(in, "siteaudit1.semrush.com.in", "https://nxaud1.upays.in"))
	if strings.Contains(out, "siteaudit1.semrush.com.in") || strings.Contains(out, "de.semrush.com") || strings.Contains(out, "static.semrush.com.in") {
		t.Fatalf("upstream host leaked: %s", out)
	}
	if !strings.Contains(out, "https://nxaud1.upays.in/siteaudit/") {
		t.Fatal("public origin was not applied")
	}
	if !strings.Contains(out, ".nxaud1.upays.in") && !strings.Contains(out, ".nxaud1.upays.in") {
		t.Fatal("cookie domain was not rewritten")
	}
}

func TestHTTPOriginDropsUpgradeInsecureCSP(t *testing.T) {
	src := make(http.Header)
	src.Set("Content-Security-Policy", "upgrade-insecure-requests")
	src.Set("X-Test", "1")
	dst := make(http.Header)
	copyResponseHeaders(dst, src, "siteaudit1.semrush.com.in", "http://127.0.0.1:6315")
	if dst.Get("Content-Security-Policy") != "" {
		t.Fatal("http origin must drop upgrade-insecure-requests CSP")
	}
	if dst.Get("X-Test") != "1" {
		t.Fatal("other headers must still copy")
	}
	dst2 := make(http.Header)
	copyResponseHeaders(dst2, src, "siteaudit1.semrush.com.in", "https://nxaud1.upays.in")
	if dst2.Get("Content-Security-Policy") != "upgrade-insecure-requests" {
		t.Fatal("https origin should keep CSP")
	}
}

func TestRewriteIPOriginKeepsUpstreamCookieDomain(t *testing.T) {
	in := []byte(`{"cookie_domain": ".semrush.com"} <a href="https://siteaudit1.semrush.com.in/x">`)
	out := string(rewriteText(in, "siteaudit1.semrush.com.in", "http://127.0.0.1:6315"))
	if strings.Contains(out, `"cookie_domain": ".127.0.0.1"`) || strings.Contains(out, `"cookie_domain":".127.0.0.1"`) {
		t.Fatal("must not rewrite cookie_domain onto an IP")
	}
	if !strings.Contains(out, `"cookie_domain": ".semrush.com"`) {
		t.Fatalf("expected upstream cookie_domain kept, got %s", out)
	}
	if strings.Contains(out, "siteaudit1.semrush.com.in") {
		t.Fatal("absolute gateway URL was not rewritten")
	}
}

func TestStaticCachePolicy(t *testing.T) {
	if !isCacheableStaticPath("/__static__/webpack/app.js") {
		t.Fatal("js should cache")
	}
	if isCacheableStaticPath("/siteaudit/") {
		t.Fatal("html path should not cache")
	}
	req := httptest.NewRequest(http.MethodGet, "/siteaudit/", nil)
	if cdncache.RequestKey(req, isCacheableStaticPath(req.URL.Path)) != "" {
		t.Fatal("html path must not be cached")
	}
}

func TestCookieJSONBecomesHeader(t *testing.T) {
	raw := `[{"name":"proxy_token","value":"abc","hostOnly":false},{"name":"uxx","value":"xyz","hostOnly":false}]`
	got := parseCookieFromDB(raw)
	if !strings.Contains(got, "proxy_token=abc") || !strings.Contains(got, "uxx=xyz") {
		t.Fatal("cookie json did not become a header")
	}
}

func TestSetCookieDropsUpstreamSession(t *testing.T) {
	if _, ok := rewriteSetCookie("proxy_token=secret; Path=/; Secure", "siteaudit1.semrush.com.in", "https://nxaud1.upays.in"); ok {
		t.Fatal("upstream session cookie must not reach the browser")
	}
	got, ok := rewriteSetCookie("snav_last_sidebar_tk=toolkit_seo; Domain=.semrush.com; Path=/", "siteaudit1.semrush.com.in", "https://nxaud1.upays.in")
	if !ok || !strings.Contains(got, "Domain=nxaud1.upays.in") || strings.Contains(got, "semrush.com") {
		t.Fatalf("sidebar cookie domain was not rewritten: %q", got)
	}
}

func TestLogoutPaths(t *testing.T) {
	if !isLogoutPath("/sso/logout") || isLogoutPath("/siteaudit/") {
		t.Fatal("logout path detection mismatch")
	}
}

func TestBodyLooksLoggedOut(t *testing.T) {
	if !bodyLooksLoggedOut([]byte(`Session expired, access again from Dashboard`)) {
		t.Fatal("expected session expired text")
	}
	if bodyLooksLoggedOut([]byte(`<title>Semrush Projects: Take control of your data</title>`)) {
		t.Fatal("normal projects page must not count as logout")
	}
	if !bodyLooksLoggedOut([]byte(`<title>Semrush: Your Unfair Advantage for Growing Brand Visibility</title>`)) {
		t.Fatal("marketing homepage should count as logged out")
	}
}

func TestHomeRedirectLoop(t *testing.T) {
	if !homeRedirectLoop("/siteaudit/", "http://127.0.0.1:6315/", "http://127.0.0.1:6315") {
		t.Fatal("expected /siteaudit/ → / loop")
	}
	if homeRedirectLoop("/analytics/", "http://127.0.0.1:6315/", "http://127.0.0.1:6315") {
		t.Fatal("non-home path should not count")
	}
}
