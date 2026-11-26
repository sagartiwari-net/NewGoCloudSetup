package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestRewriteSpamZillaHosts(t *testing.T) {
	in := []byte(`<div class="logo">RankyTools</div><link href="https://sz02.rankytools.com/css/app.css"><script id="rt-cookie-guard">nuke()</script>wss://sz02.rankytools.com/socket`)
	out := string(rewriteText(in, "sz02.rankytools.com", "http://127.0.0.1:6509"))
	if strings.Contains(out, "sz02.rankytools.com") {
		t.Fatalf("ranky host leaked: %s", out)
	}
	if strings.Contains(out, "rt-cookie-guard") || strings.Contains(out, "nuke()") {
		t.Fatal("rt-cookie-guard must be stripped")
	}
	if !strings.Contains(out, "http://127.0.0.1:6509/css/app.css") {
		t.Fatal("public origin missing")
	}
	if strings.Contains(out, "Change Account") {
		t.Fatal("no Change Account on SpamZilla")
	}
	if strings.Contains(out, `class="logo">RankyTools`) {
		t.Fatal("RankyTools logo must be stripped")
	}
	if !strings.Contains(out, "ws://127.0.0.1:6509/socket") {
		t.Fatalf("local http must rewrite wss→ws: %s", out)
	}
}

func TestMergeSessionCookies(t *testing.T) {
	got := mergeSessionCookies("PHPSESSID=abc", "csrf_token=tok; foo=1; _csrf=g")
	if !strings.Contains(got, "PHPSESSID=abc") || !strings.Contains(got, "csrf_token=tok") || !strings.Contains(got, "_csrf=g") {
		t.Fatal(got)
	}
	if strings.Contains(got, "foo=1") {
		t.Fatal(got)
	}
}

func TestRewriteSetCookieHTTPSameSite(t *testing.T) {
	got, ok := rewriteSetCookie("_csrf=xyz; Path=/; Domain=.rankytools.com; SameSite=None; Secure", "sz02.rankytools.com", "http://127.0.0.1:6509")
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
		"_csrf=fresh; Path=/; SameSite=None; Secure",
		"_identity=tok; Path=/",
	}})
	rememberUpstreamCookies(acc, jar)
	got := cookiesForUpstream(acc, nil)
	if !strings.Contains(got, "PHPSESSID=abc") || !strings.Contains(got, "_csrf=fresh") || !strings.Contains(got, "_identity=tok") {
		t.Fatal(got)
	}
}

func TestRankyMemberLogoutDetect(t *testing.T) {
	if !isRankyMemberLogout("https://member.rankytools.com?return_url=https%3A%2F%2Fsz02.rankytools.com%2F") {
		t.Fatal("expected member logout")
	}
}

func TestRankyReturnURLBounce(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:6509/?return_url=https%3A%2F%2Fsz02.rankytools.com%2F%3Freturn_url%3Dx", nil)
	if !isRankyReturnURLBounce(req, "", "http://127.0.0.1:6509") {
		t.Fatal("nested return_url request must bounce")
	}
}

func TestRewriteKeepsMemberPortal(t *testing.T) {
	in := []byte(`https://member.rankytools.com?return_url=https://sz02.rankytools.com/`)
	out := string(rewriteText(in, "sz02.rankytools.com", "http://127.0.0.1:6509"))
	if !strings.Contains(out, "member.rankytools.com") {
		t.Fatalf("member portal must stay: %s", out)
	}
	if strings.Contains(out, "http://127.0.0.1:6509?return_url") {
		t.Fatalf("member must not rewrite onto local: %s", out)
	}
}

func TestCookieJSONBecomesHeader(t *testing.T) {
	got := parseCookieFromDB(`[{"name":"PHPSESSID","value":"abc"}]`)
	if !strings.Contains(got, "PHPSESSID=abc") {
		t.Fatal(got)
	}
}
