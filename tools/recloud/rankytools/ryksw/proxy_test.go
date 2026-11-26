package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestRewriteSimilarWebHosts(t *testing.T) {
	in := []byte(`<div class="logo">RankyTools</div><script>var proxyHost="sw.rankytools.com",proxyOrigin="https://sw.rankytools.com";</script><a href="https://static-us-west-2.similarcdn.com/x.js">cdn</a><script id="rt-cookie-guard">nuke()</script>`)
	out := string(rewriteText(in, "sw.rankytools.com", "http://127.0.0.1:6510"))
	if strings.Contains(out, "sw.rankytools.com") {
		t.Fatalf("ranky host leaked: %s", out)
	}
	if !strings.Contains(out, "similarcdn.com") {
		t.Fatal("external similarcdn host must stay")
	}
	if strings.Contains(out, "rt-cookie-guard") || strings.Contains(out, "nuke()") {
		t.Fatal("rt-cookie-guard must be stripped")
	}
	if !strings.Contains(out, "http://127.0.0.1:6510") {
		t.Fatal("public origin missing")
	}
	if strings.Contains(out, `class="logo">RankyTools`) {
		t.Fatal("RankyTools logo must be stripped")
	}
}

func TestMergeSessionCookies(t *testing.T) {
	got := mergeSessionCookies("PHPSESSID=abc", "csrf_token=tok; foo=1; sw_session=g")
	if !strings.Contains(got, "PHPSESSID=abc") || !strings.Contains(got, "csrf_token=tok") || !strings.Contains(got, "sw_session=g") {
		t.Fatal(got)
	}
	if strings.Contains(got, "foo=1") {
		t.Fatal(got)
	}
}

func TestRewriteSetCookieHTTPSameSite(t *testing.T) {
	got, ok := rewriteSetCookie("sw_session=xyz; Path=/; Domain=.rankytools.com; SameSite=None; Secure", "sw.rankytools.com", "http://127.0.0.1:6510")
	if !ok {
		t.Fatal("expected rewrite")
	}
	if strings.Contains(strings.ToLower(got), "samesite=none") {
		t.Fatalf("SameSite=None must become Lax on http: %s", got)
	}
	if !strings.Contains(got, "SameSite=Lax") {
		t.Fatalf("want SameSite=Lax: %s", got)
	}
}

func TestAbsorbCookiesForUpstream(t *testing.T) {
	acc := ToolAccount{ID: 99, Cookie: "PHPSESSID=abc"}
	jar := cookiesForUpstream(acc, nil)
	jar = absorbSetCookies(jar, http.Header{"Set-Cookie": []string{
		"sw_session=fresh; Path=/; SameSite=None; Secure",
		"csrf_token=tok; Path=/",
	}})
	rememberUpstreamCookies(acc, jar)
	got := cookiesForUpstream(acc, nil)
	if !strings.Contains(got, "PHPSESSID=abc") || !strings.Contains(got, "sw_session=fresh") || !strings.Contains(got, "csrf_token=tok") {
		t.Fatal(got)
	}
}

func TestRankyMemberLogoutDetect(t *testing.T) {
	if !isRankyMemberLogout("https://member.rankytools.com?return_url=https%3A%2F%2Fsw.rankytools.com%2F") {
		t.Fatal("expected member logout")
	}
}

func TestRankyReturnURLBounce(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:6510/?return_url=https%3A%2F%2Fsw.rankytools.com%2F%3Freturn_url%3Dx", nil)
	if !isRankyReturnURLBounce(req, "", "http://127.0.0.1:6510") {
		t.Fatal("nested return_url request must bounce")
	}
}

func TestRewriteKeepsMemberPortal(t *testing.T) {
	in := []byte(`https://member.rankytools.com?return_url=https://sw.rankytools.com/`)
	out := string(rewriteText(in, "sw.rankytools.com", "http://127.0.0.1:6510"))
	if !strings.Contains(out, "member.rankytools.com") {
		t.Fatalf("member portal must stay: %s", out)
	}
	if strings.Contains(out, "http://127.0.0.1:6510?return_url") {
		t.Fatalf("member must not rewrite onto local: %s", out)
	}
}

func TestCookieJSONBecomesHeader(t *testing.T) {
	got := parseCookieFromDB(`[{"name":"PHPSESSID","value":"abc"}]`)
	if !strings.Contains(got, "PHPSESSID=abc") {
		t.Fatal(got)
	}
}

func TestRewriteCloudflareHostLeak(t *testing.T) {
	in := []byte(`Bad gateway Host (sw.rankytools.com) Error code 502`)
	out := string(rewriteText(in, "sw.rankytools.com", "http://127.0.0.1:6510"))
	if strings.Contains(out, "sw.rankytools.com") {
		t.Fatalf("ranky host leaked on CF page: %s", out)
	}
	if !strings.Contains(out, "127.0.0.1:6510") {
		t.Fatalf("public host missing: %s", out)
	}
}
