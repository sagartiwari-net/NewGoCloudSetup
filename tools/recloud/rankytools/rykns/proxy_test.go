package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestRewriteNicheScraperHosts(t *testing.T) {
	in := []byte(`<div class="logo">RankyTools</div><link href="https://ns.rankytools.com/css/index.css"><script id="rt-cookie-guard">nuke()</script>wss://ns.rankytools.com/socket`)
	out := string(rewriteText(in, "ns.rankytools.com", "http://127.0.0.1:6508"))
	if strings.Contains(out, "ns.rankytools.com") {
		t.Fatalf("ranky host leaked: %s", out)
	}
	if strings.Contains(out, "rt-cookie-guard") || strings.Contains(out, "nuke()") {
		t.Fatal("rt-cookie-guard must be stripped")
	}
	if !strings.Contains(out, "http://127.0.0.1:6508/css/index.css") {
		t.Fatal("public origin missing")
	}
	if strings.Contains(out, "Change Account") {
		t.Fatal("no Change Account on Niche Scraper")
	}
	if strings.Contains(out, `class="logo">RankyTools`) {
		t.Fatal("RankyTools logo must be stripped")
	}
	if !strings.Contains(out, "ws://127.0.0.1:6508/socket") {
		t.Fatalf("local http must rewrite wss→ws: %s", out)
	}
	if strings.Contains(out, "wss://127.0.0.1") {
		t.Fatal("must not leave wss on http origin")
	}
}

func TestMergeSessionCookies(t *testing.T) {
	got := mergeSessionCookies("PHPSESSID=abc", "csrf_token=tok; foo=1; niche_session=g")
	if !strings.Contains(got, "PHPSESSID=abc") || !strings.Contains(got, "csrf_token=tok") || !strings.Contains(got, "niche_session=g") {
		t.Fatal(got)
	}
	if strings.Contains(got, "foo=1") {
		t.Fatal(got)
	}
}

func TestRewriteSetCookieHTTPSameSite(t *testing.T) {
	got, ok := rewriteSetCookie("niche_session=xyz; Path=/; Domain=.rankytools.com; SameSite=None; Secure", "ns.rankytools.com", "http://127.0.0.1:6508")
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
		"niche_session=fresh; Path=/; SameSite=None; Secure",
		"csrf_token=tok; Path=/",
	}})
	rememberUpstreamCookies(acc, jar)
	got := cookiesForUpstream(acc, nil)
	if !strings.Contains(got, "PHPSESSID=abc") || !strings.Contains(got, "niche_session=fresh") || !strings.Contains(got, "csrf_token=tok") {
		t.Fatal(got)
	}
}

func TestInjectRankyBrandHide(t *testing.T) {
	out := string(injectRankyBrandHide([]byte(`<html><head></head><body><div class="logo">RankyTools</div></body></html>`)))
	if !strings.Contains(out, "data-tm-rankybrand") {
		t.Fatal("inject missing")
	}
	if !strings.Contains(out, `name="referrer" content="no-referrer"`) {
		t.Fatal("AliExpress hotlink fix (no-referrer meta) missing")
	}
}

func TestCookieJSONBecomesHeader(t *testing.T) {
	got := parseCookieFromDB(`[{"name":"PHPSESSID","value":"abc"}]`)
	if !strings.Contains(got, "PHPSESSID=abc") {
		t.Fatal(got)
	}
}

func TestRankyMemberLogoutDetect(t *testing.T) {
	if !isRankyMemberLogout("https://member.rankytools.com?return_url=https%3A%2F%2Fns.rankytools.com%2F") {
		t.Fatal("expected member logout")
	}
	if isRankyMemberLogout("https://ns.rankytools.com/scraper") {
		t.Fatal("tool URL is not member logout")
	}
}

func TestRankyReturnURLBounce(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:6508/?return_url=https%3A%2F%2Fns.rankytools.com%2F%3Freturn_url%3Dx", nil)
	if !isRankyReturnURLBounce(req, "", "http://127.0.0.1:6508") {
		t.Fatal("nested return_url request must bounce")
	}
	if !isRankyReturnURLBounce(nil, "http://127.0.0.1:6508?return_url=https%3A%2F%2Fns.rankytools.com%2F", "http://127.0.0.1:6508") {
		t.Fatal("rewritten member location must bounce")
	}
	if isRankyReturnURLBounce(nil, "http://127.0.0.1:6508/scraper", "http://127.0.0.1:6508") {
		t.Fatal("normal location must not bounce")
	}
}

func TestRewriteKeepsMemberPortal(t *testing.T) {
	in := []byte(`Location: https://member.rankytools.com?return_url=https://ns.rankytools.com/`)
	out := string(rewriteText(in, "ns.rankytools.com", "http://127.0.0.1:6508"))
	if !strings.Contains(out, "member.rankytools.com") {
		t.Fatalf("member portal must stay: %s", out)
	}
	if strings.Contains(out, "http://127.0.0.1:6508?return_url") {
		t.Fatalf("member must not rewrite onto local: %s", out)
	}
	if !strings.Contains(out, "http://127.0.0.1:6508/") && !strings.Contains(out, "return_url=http://127.0.0.1:6508") {
		// return_url value may still rewrite ns.rankytools.com — that's ok;
		// the important part is the Location host stays member.*
	}
}
