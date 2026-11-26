package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestRewriteGrammarlyHosts(t *testing.T) {
	in := []byte(`<script src="https://gm.rankytools.com/denali-static/js/x.js"></script><a href="https://support.grammarly.com/hc">help</a><script id="rt-cookie-guard">nuke()</script>wss://gm.rankytools.com/dox case"slowNetwork":return{content:"Your Internet connection is unstable."}`)
	out := string(rewriteText(in, "gm.rankytools.com", "http://127.0.0.1:6504"))
	if strings.Contains(out, "gm.rankytools.com") {
		t.Fatalf("ranky host leaked: %s", out)
	}
	if !strings.Contains(out, "support.grammarly.com") {
		t.Fatal("external grammarly support host must stay")
	}
	if strings.Contains(out, "rt-cookie-guard") || strings.Contains(out, "nuke()") {
		t.Fatal("rt-cookie-guard must be stripped")
	}
	if !strings.Contains(out, "http://127.0.0.1:6504/denali-static/js/x.js") {
		t.Fatal("public origin missing")
	}
	if strings.Contains(out, "Change Account") {
		t.Fatal("no Change Account on Grammarly")
	}
	if strings.Contains(out, `case"slowNetwork":`) {
		t.Fatal("slowNetwork case must be renamed")
	}
	if !strings.Contains(out, `case"__tm_slowNetwork_hidden":`) {
		t.Fatal("slowNetwork case rename missing")
	}
	if !strings.Contains(out, "ws://127.0.0.1:6504/dox") {
		t.Fatalf("local http must rewrite wss→ws: %s", out)
	}
	if strings.Contains(out, "wss://127.0.0.1") {
		t.Fatal("must not leave wss on http origin")
	}
}

func TestMergeSessionCookies(t *testing.T) {
	got := mergeSessionCookies("PHPSESSID=abc", "csrf_token=tok; foo=1; grauth=g")
	if !strings.Contains(got, "PHPSESSID=abc") || !strings.Contains(got, "csrf_token=tok") || !strings.Contains(got, "grauth=g") {
		t.Fatal(got)
	}
	if strings.Contains(got, "foo=1") {
		t.Fatal(got)
	}
}

func TestRewriteSetCookieHTTPSameSite(t *testing.T) {
	got, ok := rewriteSetCookie("grauth=xyz; Path=/; Domain=.rankytools.com; SameSite=None; Secure", "gm.rankytools.com", "http://127.0.0.1:6504")
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

func TestAbsorbGrauthForUpstream(t *testing.T) {
	acc := ToolAccount{ID: 99, Cookie: "PHPSESSID=abc"}
	jar := cookiesForUpstream(acc, nil)
	jar = absorbSetCookies(jar, http.Header{"Set-Cookie": []string{
		"grauth=fresh; Path=/; SameSite=None; Secure",
		"csrf_token=tok; Path=/",
	}})
	rememberUpstreamCookies(acc, jar)
	got := cookiesForUpstream(acc, nil)
	if !strings.Contains(got, "PHPSESSID=abc") || !strings.Contains(got, "grauth=fresh") || !strings.Contains(got, "csrf_token=tok") {
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
