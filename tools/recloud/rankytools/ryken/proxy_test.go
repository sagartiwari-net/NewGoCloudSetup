package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestRewriteEnvatoHosts(t *testing.T) {
	in := []byte(`<div class="logo">RankyTools</div><script src="https://en.rankytools.com/assets/root.js"></script><a href="https://elements.envato.com/x">cdn</a><script id="rt-cookie-guard">nuke()</script>wss://en.rankytools.com/socket`)
	out := string(rewriteText(in, "en.rankytools.com", "http://127.0.0.1:6505"))
	if strings.Contains(out, "en.rankytools.com") {
		t.Fatalf("ranky host leaked: %s", out)
	}
	if !strings.Contains(out, "elements.envato.com") {
		t.Fatal("external envato CDN host must stay")
	}
	if strings.Contains(out, "rt-cookie-guard") || strings.Contains(out, "nuke()") {
		t.Fatal("rt-cookie-guard must be stripped")
	}
	if !strings.Contains(out, "http://127.0.0.1:6505/assets/root.js") {
		t.Fatal("public origin missing")
	}
	if strings.Contains(out, "Change Account") {
		t.Fatal("no Change Account on Envato")
	}
	if strings.Contains(out, `class="logo">RankyTools`) {
		t.Fatal("RankyTools logo must be stripped")
	}
	if !strings.Contains(out, "ws://127.0.0.1:6505/socket") {
		t.Fatalf("local http must rewrite wss→ws: %s", out)
	}
	if strings.Contains(out, "wss://127.0.0.1") {
		t.Fatal("must not leave wss on http origin")
	}
}

func TestInjectRankyBrandHide(t *testing.T) {
	out := string(injectRankyBrandHide([]byte(`<html><head></head><body><div class="logo">RankyTools</div></body></html>`)))
	if !strings.Contains(out, "data-tm-rankybrand") {
		t.Fatal("inject missing")
	}
}

func TestMergeSessionCookies(t *testing.T) {
	got := mergeSessionCookies("PHPSESSID=abc", "csrf_token=tok; foo=1; envato_session=g")
	if !strings.Contains(got, "PHPSESSID=abc") || !strings.Contains(got, "csrf_token=tok") || !strings.Contains(got, "envato_session=g") {
		t.Fatal(got)
	}
	if strings.Contains(got, "foo=1") {
		t.Fatal(got)
	}
}

func TestRewriteSetCookieHTTPSameSite(t *testing.T) {
	got, ok := rewriteSetCookie("envato_session=xyz; Path=/; Domain=.rankytools.com; SameSite=None; Secure", "en.rankytools.com", "http://127.0.0.1:6505")
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
		"envato_session=fresh; Path=/; SameSite=None; Secure",
		"csrf_token=tok; Path=/",
	}})
	rememberUpstreamCookies(acc, jar)
	got := cookiesForUpstream(acc, nil)
	if !strings.Contains(got, "PHPSESSID=abc") || !strings.Contains(got, "envato_session=fresh") || !strings.Contains(got, "csrf_token=tok") {
		t.Fatal(got)
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
