package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRewriteUbersuggestHosts(t *testing.T) {
	in := []byte(`<a href="https://us.rankytools.com/en/dashboard">x</a><script src="https://ubersuggest.com/app.js"></script><link href="https://growthbook-proxy.neilpatelapi.com/x"><script id="rt-cookie-guard">nuke()</script>`)
	out := string(rewriteText(in, "us.rankytools.com", "http://127.0.0.1:6503"))
	if strings.Contains(out, "us.rankytools.com") {
		t.Fatalf("ranky host leaked: %s", out)
	}
	if !strings.Contains(out, "ubersuggest.com") || !strings.Contains(out, "neilpatelapi.com") {
		t.Fatal("third-party hosts must stay absolute for Ubersuggest")
	}
	if strings.Contains(out, "rt-cookie-guard") || strings.Contains(out, "nuke()") {
		t.Fatal("rt-cookie-guard must be stripped")
	}
	if !strings.Contains(out, "http://127.0.0.1:6503/en/dashboard") {
		t.Fatal("public origin missing")
	}
	if strings.Contains(out, "Change Account") {
		t.Fatal("Change Account must not be injected on Ubersuggest")
	}
}

func TestStripNextRSC(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:6503/en/analyze/site-audit?_rsc=abc&x=1", nil)
	req.Header.Set("RSC", "1")
	req.Header.Set("Next-Url", "/en/dashboard")
	stripNextRSC(req)
	if req.Header.Get("RSC") != "" || req.Header.Get("Next-Url") != "" {
		t.Fatal("RSC headers remain")
	}
	if req.URL.Query().Get("_rsc") != "" {
		t.Fatal("_rsc query remains")
	}
	if req.URL.Query().Get("x") != "1" {
		t.Fatal("unrelated query dropped")
	}
}

func TestIsUbersuggestBounce(t *testing.T) {
	if !isUbersuggestBounce("https://ubersuggest.com/en/analyze/site-audit?_rsc=1") {
		t.Fatal("expected bounce")
	}
	if isUbersuggestBounce("https://us.rankytools.com/en/dashboard") {
		t.Fatal("ranky host is not bounce")
	}
}

func TestNavFixBeforeBody(t *testing.T) {
	in := []byte(`<html><body><div id="root">x</div></body></html>`)
	out := string(injectNavFixHTML(in))
	if strings.Contains(out, "Change Account") {
		t.Fatal("no Change Account on Ubersuggest")
	}
	if !strings.Contains(out, "data-tm-navfix") {
		t.Fatal("navfix missing")
	}
	if i := strings.Index(out, "data-tm-navfix"); i < 0 || i > strings.Index(out, "</body>") {
		t.Fatal("navfix must be before </body>")
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
	in := []byte(`https://member.rankytools.com?return_url=https://tool.rankytools.com/`)
	out := string(rewriteText(in, "tool.rankytools.com", "http://127.0.0.1:9"))
	if !strings.Contains(out, "member.rankytools.com") {
		t.Fatalf("member portal must stay: %s", out)
	}
	if strings.Contains(out, "http://127.0.0.1:9?return_url") {
		t.Fatalf("member must not rewrite onto local: %s", out)
	}
}
