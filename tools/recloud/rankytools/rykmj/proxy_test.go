package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestRewriteMajesticHosts(t *testing.T) {
	in := []byte(`<a href="https://mj.rankytools.com/reports">x</a><script src="https://www.majestic.com/app.js"></script><script id="rt-cookie-guard">nuke()</script>`)
	out := string(rewriteText(in, "mj.rankytools.com", "http://127.0.0.1:6501"))
	if strings.Contains(out, "mj.rankytools.com") || strings.Contains(out, "www.majestic.com") {
		t.Fatalf("upstream host leaked: %s", out)
	}
	if strings.Contains(out, "rt-cookie-guard") || strings.Contains(out, "nuke()") {
		t.Fatal("rt-cookie-guard must be stripped")
	}
	if !strings.Contains(out, "http://127.0.0.1:6501/reports") {
		t.Fatal("public origin missing")
	}
}

func TestCookieJSONBecomesHeader(t *testing.T) {
	raw := `[{"name":"PHPSESSID","value":"abc"}]`
	got := parseCookieFromDB(raw)
	if !strings.Contains(got, "PHPSESSID=abc") {
		t.Fatal("cookie json did not become a header")
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
