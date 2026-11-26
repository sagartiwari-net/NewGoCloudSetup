package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestRewriteSpyFuHosts(t *testing.T) {
	in := []byte(`<div class="logo">RankyTools</div><script src="https://sf.rankytools.com/next/static/x.js"></script><a href="https://cdn1.spyfu.com/x">cdn</a><script id="rt-cookie-guard">nuke()</script>wss://sf.rankytools.com/socket`)
	out := string(rewriteText(in, "sf.rankytools.com", "http://127.0.0.1:6507"))
	if strings.Contains(out, "sf.rankytools.com") {
		t.Fatalf("ranky host leaked: %s", out)
	}
	if !strings.Contains(out, "cdn1.spyfu.com") {
		t.Fatal("external spyfu CDN host must stay")
	}
	if strings.Contains(out, "rt-cookie-guard") || strings.Contains(out, "nuke()") {
		t.Fatal("rt-cookie-guard must be stripped")
	}
	if !strings.Contains(out, "http://127.0.0.1:6507/next/static/x.js") {
		t.Fatal("public origin missing")
	}
	if strings.Contains(out, "Change Account") {
		t.Fatal("no Change Account on SpyFu")
	}
	if strings.Contains(out, `class="logo">RankyTools`) {
		t.Fatal("RankyTools logo must be stripped")
	}
	if !strings.Contains(out, "ws://127.0.0.1:6507/socket") {
		t.Fatalf("local http must rewrite wss→ws: %s", out)
	}
	if strings.Contains(out, "wss://127.0.0.1") {
		t.Fatal("must not leave wss on http origin")
	}
}

func TestMergeSessionCookies(t *testing.T) {
	got := mergeSessionCookies("PHPSESSID=abc", "csrf_token=tok; foo=1; spyfu_session=g")
	if !strings.Contains(got, "PHPSESSID=abc") || !strings.Contains(got, "csrf_token=tok") || !strings.Contains(got, "spyfu_session=g") {
		t.Fatal(got)
	}
	if strings.Contains(got, "foo=1") {
		t.Fatal(got)
	}
}

func TestRewriteSetCookieHTTPSameSite(t *testing.T) {
	got, ok := rewriteSetCookie("spyfu_session=xyz; Path=/; Domain=.rankytools.com; SameSite=None; Secure", "sf.rankytools.com", "http://127.0.0.1:6507")
	if !ok {
		t.Fatal("expected rewrite")
	}
	if strings.Contains(strings.ToLower(got), "samesite=none") {
		t.Fatalf("SameSite=None must become Lax on http: %s", got)
	}
	if strings.Contains(strings.ToLower(got), "secure") {
		t.Fatalf("Secure must be stripped on http: %s", got)
	}
	if !strings.Contains(got, "SameSite=Lax") {
		t.Fatalf("want SameSite=Lax: %s", got)
	}
}

func TestAbsorbCookiesForUpstream(t *testing.T) {
	acc := ToolAccount{ID: 99, Cookie: "PHPSESSID=abc"}
	jar := cookiesForUpstream(acc, nil)
	jar = absorbSetCookies(jar, http.Header{"Set-Cookie": []string{
		"spyfu_session=fresh; Path=/; SameSite=None; Secure",
		"csrf_token=tok; Path=/",
	}})
	rememberUpstreamCookies(acc, jar)
	got := cookiesForUpstream(acc, nil)
	if !strings.Contains(got, "PHPSESSID=abc") || !strings.Contains(got, "spyfu_session=fresh") || !strings.Contains(got, "csrf_token=tok") {
		t.Fatal(got)
	}
}

func TestInjectRankyBrandHide(t *testing.T) {
	out := string(injectRankyBrandHide([]byte(`<html><head></head><body><div class="logo">RankyTools</div></body></html>`)))
	if !strings.Contains(out, "data-tm-rankybrand") {
		t.Fatal("inject missing")
	}
}

func TestCookieJSONBecomesHeader(t *testing.T) {
	got := parseCookieFromDB(`[{"name":"PHPSESSID","value":"abc"}]`)
	if !strings.Contains(got, "PHPSESSID=abc") {
		t.Fatal(got)
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
	in := []byte(`https://member.rankytools.com?return_url=https://tool.rankytools.com/`)
	out := string(rewriteText(in, "tool.rankytools.com", "http://127.0.0.1:9"))
	if !strings.Contains(out, "member.rankytools.com") {
		t.Fatalf("member portal must stay: %s", out)
	}
	if strings.Contains(out, "http://127.0.0.1:9?return_url") {
		t.Fatalf("member must not rewrite onto local: %s", out)
	}
}
