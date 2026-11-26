package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"recloud.dev/cdncache"
)

func TestRewriteKeepsBrowserOnPublicHost(t *testing.T) {
	in := []byte(`<a href="https://sr.rankytools.com/home/">x</a><script src="https://static.semrush.com/app.js"></script><a href="https://member.rankytools.com/protect">p</a>{"cookie_domain":".semrush.com"}`)
	out := string(rewriteText(in, "sr.rankytools.com", "https://ryksm1.upays.in"))
	if strings.Contains(out, "sr.rankytools.com") || strings.Contains(out, "static.semrush.com") {
		t.Fatalf("upstream host leaked: %s", out)
	}
	// member portal must stay absolute — rewriting it onto the tool host
	// causes logout return_url redirect loops.
	if !strings.Contains(out, "member.rankytools.com") {
		t.Fatal("member portal must not be rewritten onto tool origin")
	}
	if !strings.Contains(out, "https://ryksm1.upays.in/home/") {
		t.Fatal("public origin was not applied")
	}
	if !strings.Contains(out, ".ryksm1.upays.in") {
		t.Fatal("cookie domain was not rewritten")
	}
}

func TestHTTPOriginDropsUpgradeInsecureCSP(t *testing.T) {
	src := make(http.Header)
	src.Set("Content-Security-Policy", "upgrade-insecure-requests")
	src.Set("X-Test", "1")
	dst := make(http.Header)
	copyResponseHeaders(dst, src, "sr.rankytools.com", "http://127.0.0.1:6500")
	if dst.Get("Content-Security-Policy") != "" {
		t.Fatal("http origin must drop upgrade-insecure-requests CSP")
	}
	if dst.Get("X-Test") != "1" {
		t.Fatal("other headers must still copy")
	}
}

func TestRewriteIPOriginKeepsUpstreamCookieDomain(t *testing.T) {
	in := []byte(`{"cookie_domain": ".semrush.com"} <a href="https://sr.rankytools.com/x">`)
	out := string(rewriteText(in, "sr.rankytools.com", "http://127.0.0.1:6500"))
	if strings.Contains(out, `"cookie_domain": ".127.0.0.1"`) || strings.Contains(out, `"cookie_domain":".127.0.0.1"`) {
		t.Fatal("must not rewrite cookie_domain onto an IP")
	}
	if !strings.Contains(out, `"cookie_domain": ".semrush.com"`) {
		t.Fatalf("expected upstream cookie_domain kept, got %s", out)
	}
	if strings.Contains(out, "sr.rankytools.com") {
		t.Fatal("absolute gateway URL was not rewritten")
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
	raw := `[{"name":"PHPSESSID","value":"abc","hostOnly":false}]`
	got := parseCookieFromDB(raw)
	if !strings.Contains(got, "PHPSESSID=abc") {
		t.Fatal("cookie json did not become a header")
	}
}

func TestSetCookieDropsDomainOnIP(t *testing.T) {
	got, ok := rewriteSetCookie("PHPSESSID=x; Domain=.rankytools.com; Path=/", "sr.rankytools.com", "http://127.0.0.1:6500")
	if !ok {
		t.Fatal("cookie should rewrite")
	}
	if strings.Contains(strings.ToLower(got), "domain=") {
		t.Fatalf("IP origin must drop Domain=: %s", got)
	}
}

func TestRankyBrandBecomesChangeAccount(t *testing.T) {
	in := []byte(`<a href="/" class="rankytools-brand" aria-label="Semrush by RankyTools"><span class="rt-logo-text">Semrush<small>by RankyTools</small></span><span class="rt-lb-badge">LB 2</span></a>`)
	out := string(rewriteText(in, "sr.rankytools.com", "http://127.0.0.1:6500"))
	if strings.Contains(out, "by RankyTools") || strings.Contains(out, "Semrush by RankyTools") {
		t.Fatalf("RankyTools brand leaked: %s", out)
	}
	if !strings.Contains(out, "Change Account") || !strings.Contains(out, "👉🏻") {
		t.Fatalf("Change Account brand missing: %s", out)
	}
	injected := string(injectBrandHTML(in))
	if !strings.Contains(injected, "data-tm-brand") {
		t.Fatal("brand inject missing")
	}
}

func TestRankyMemberLogoutDetect(t *testing.T) {
	if !isRankyMemberLogout("https://member.rankytools.com?return_url=https%3A%2F%2Fexample%2F") {
		t.Fatal("expected member logout")
	}
	if isRankyMemberLogout("https://example.rankytools.com/app") {
		t.Fatal("tool URL is not member logout")
	}
}

func TestRankyReturnURLBounce(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:9/?return_url=https%3A%2F%2Fx%2F%3Freturn_url%3Dy", nil)
	if !isRankyReturnURLBounce(req, "", "http://127.0.0.1:9") {
		t.Fatal("nested return_url request must bounce")
	}
	if !isRankyReturnURLBounce(nil, "http://127.0.0.1:9?return_url=https%3A%2F%2Fx%2F", "http://127.0.0.1:9") {
		t.Fatal("rewritten member location must bounce")
	}
}

func TestRewriteKeepsMemberPortal(t *testing.T) {
	// Use each tool's upstream host from a typical rewrite call site in other tests if present.
	in := []byte(`https://member.rankytools.com?return_url=https://tool.rankytools.com/`)
	out := string(rewriteText(in, "tool.rankytools.com", "http://127.0.0.1:9"))
	if !strings.Contains(out, "member.rankytools.com") {
		t.Fatalf("member portal must stay: %s", out)
	}
	if strings.Contains(out, "http://127.0.0.1:9?return_url") {
		t.Fatalf("member must not rewrite onto local: %s", out)
	}
}
