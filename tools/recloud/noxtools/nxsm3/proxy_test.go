package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"recloud.dev/cdncache"
)

func TestRewriteKeepsBrowserOnPublicHost(t *testing.T) {
	// Same shape as azsm5: *.semrush.com.in gateway + cookie_domain rewrite.
	in := []byte(`<a href="https://3.semrush.com.in/analytics/overview/">x</a><script src="https://static.semrush.com.in/app.js"></script><script src="https://de.semrush.com/app.js"></script>{"cookie_domain":".semrush.com.in"}`)
	out := string(rewriteText(in, "3.semrush.com.in", "https://nxsm3.upays.in"))
	if strings.Contains(out, "3.semrush.com.in") || strings.Contains(out, "de.semrush.com") || strings.Contains(out, "static.semrush.com.in") {
		t.Fatalf("upstream host leaked: %s", out)
	}
	if !strings.Contains(out, "https://nxsm3.upays.in/analytics/overview/") {
		t.Fatal("public origin was not applied")
	}
	if !strings.Contains(out, ".nxsm3.upays.in") {
		t.Fatal("cookie domain was not rewritten")
	}
}

func TestHTTPOriginDropsUpgradeInsecureCSP(t *testing.T) {
	src := make(http.Header)
	src.Set("Content-Security-Policy", "upgrade-insecure-requests")
	src.Set("X-Test", "1")
	dst := make(http.Header)
	copyResponseHeaders(dst, src, "3.semrush.com.in", "http://127.0.0.1:6312")
	if dst.Get("Content-Security-Policy") != "" {
		t.Fatal("http origin must drop upgrade-insecure-requests CSP")
	}
	if dst.Get("X-Test") != "1" {
		t.Fatal("other headers must still copy")
	}
	dst2 := make(http.Header)
	copyResponseHeaders(dst2, src, "3.semrush.com.in", "https://nxsm3.upays.in")
	if dst2.Get("Content-Security-Policy") != "upgrade-insecure-requests" {
		t.Fatal("https origin should keep CSP")
	}
}

func TestRewriteIPOriginKeepsUpstreamCookieDomain(t *testing.T) {
	in := []byte(`{"cookie_domain": ".semrush.com"} <a href="https://3.semrush.com.in/x">`)
	out := string(rewriteText(in, "3.semrush.com.in", "http://127.0.0.1:6312"))
	if strings.Contains(out, `"cookie_domain": ".127.0.0.1"`) || strings.Contains(out, `"cookie_domain":".127.0.0.1"`) {
		t.Fatal("must not rewrite cookie_domain onto an IP")
	}
	if !strings.Contains(out, `"cookie_domain": ".semrush.com"`) {
		t.Fatalf("expected upstream cookie_domain kept, got %s", out)
	}
	if strings.Contains(out, "3.semrush.com.in") {
		t.Fatal("absolute gateway URL was not rewritten")
	}
	if strings.Contains(out, "1.127.0.0.1.in") {
		t.Fatal("gateway hostname was corrupted")
	}
}

func TestStaticCachePolicy(t *testing.T) {
	if !isCacheableStaticPath("/__static__/webpack/app.js") {
		t.Fatal("js should cache")
	}
	if isCacheableStaticPath("/home/") {
		t.Fatal("html path should not cache")
	}
	req := httptest.NewRequest(http.MethodGet, "/home/", nil)
	if cdncache.RequestKey(req, isCacheableStaticPath(req.URL.Path)) != "" {
		t.Fatal("html path must not be cached")
	}
}

func TestCookieJSONBecomesHeader(t *testing.T) {
	raw := `[{"name":"proxy_token","value":"abc","hostOnly":false}]`
	got := parseCookieFromDB(raw)
	if !strings.Contains(got, "proxy_token=abc") {
		t.Fatal("cookie json did not become a header")
	}
}

func TestSetCookieDropsUpstreamSession(t *testing.T) {
	if _, ok := rewriteSetCookie("proxy_token=secret; Path=/; Secure", "3.semrush.com.in", "https://nxsm3.upays.in"); ok {
		t.Fatal("upstream session cookie must not reach the browser")
	}
	got, ok := rewriteSetCookie("snav_last_sidebar_tk=toolkit_seo; Domain=.semrush.com; Path=/", "3.semrush.com.in", "https://nxsm3.upays.in")
	if !ok || !strings.Contains(got, "Domain=nxsm3.upays.in") || strings.Contains(got, "semrush.com") {
		t.Fatal("sidebar cookie domain was not rewritten")
	}
}

func TestLogoutPath(t *testing.T) {
	if !isLogoutPath("/sso/logout") || isLogoutPath("/home/") {
		t.Fatal("logout path detection")
	}
}

func TestBodyLooksLoggedOutNoxGateway(t *testing.T) {
	if !bodyLooksLoggedOut([]byte(`<h1>Session expired, access again from Dashboard</h1>`)) {
		t.Fatal("nox gateway message should count as logout")
	}
	if bodyLooksLoggedOut([]byte(`<title>Semrush Projects</title><script>console.log('Session expired, redirecting...');</script>`)) {
		t.Fatal("semrush spa timer script alone must not count as logout")
	}
	if bodyLooksLoggedOut([]byte(`<title>Semrush Projects: Take control of your data</title>`)) {
		t.Fatal("normal projects page must not count as logout")
	}
	if !bodyLooksLoggedOut([]byte(`<title>Semrush: Your Unfair Advantage for Growing Brand Visibility</title>`)) {
		t.Fatal("marketing homepage should count as logged out")
	}
}

func TestHomeRedirectLoop(t *testing.T) {
	if !homeRedirectLoop("/home/", "http://127.0.0.1:6312/", "http://127.0.0.1:6312") {
		t.Fatal("expected /home/ → / loop")
	}
	if !homeRedirectLoop("/analytics/overview/", "http://127.0.0.1:6312/", "http://127.0.0.1:6312") {
		t.Fatal("expected analytics overview → / loop")
	}
	if homeRedirectLoop("/analytics/", "http://127.0.0.1:6312/", "http://127.0.0.1:6312") {
		t.Fatal("non-home path should not count")
	}
}
