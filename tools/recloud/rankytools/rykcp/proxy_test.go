package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestRewriteChatGPTHosts(t *testing.T) {
	in := []byte(`<script src="https://cp.rankytools.com/__proxy_inject.js"></script><script id="rt-cookie-guard">nuke()</script><a href="https://os.rankytools.com/x">x</a>`)
	out := string(rewriteText(in, "cp.rankytools.com", "http://127.0.0.1:6502"))
	if strings.Contains(out, "cp.rankytools.com") || strings.Contains(out, "os.rankytools.com") {
		t.Fatalf("ranky host leaked: %s", out)
	}
	if strings.Contains(out, "rt-cookie-guard") || strings.Contains(out, "nuke()") {
		t.Fatal("rt-cookie-guard must be stripped")
	}
	if !strings.Contains(out, "http://127.0.0.1:6502/__proxy_inject.js") {
		t.Fatal("public origin missing")
	}
	if strings.Contains(out, "Change Account") {
		t.Fatal("no Change Account on ChatGPT")
	}
}

func TestMergeSessionCookies(t *testing.T) {
	got := mergeSessionCookies("PHPSESSID=abc", "ranky.sid=xyz; foo=1; connect.sid=c")
	if !strings.Contains(got, "PHPSESSID=abc") || !strings.Contains(got, "ranky.sid=xyz") || !strings.Contains(got, "connect.sid=c") {
		t.Fatal(got)
	}
	if strings.Contains(got, "foo=1") {
		t.Fatal("must not forward unrelated browser cookies:", got)
	}
}

func TestRewriteRankyOSCopy(t *testing.T) {
	in := `which you can access anytime at RankyOS.</p><span class="ranky-user-sub">RankyTools</span>` +
		`if(!document.querySelector('.ranky-brand')){var b=document.createElement('div');b.className='ranky-brand';b.innerHTML='ChatGPT by <a href="http://127.0.0.1:6502" target="_blank">RankyTools</a>';document.body.appendChild(b);}`
	out := rewriteRankyOSCopy(in)
	if strings.Contains(out, "RankyOS") || strings.Contains(out, "ranky-user-sub") || strings.Contains(out, "ranky-brand") {
		t.Fatal(out)
	}
	if !strings.Contains(out, "access anytime.</p>") {
		t.Fatal(out)
	}
}

func TestInjectViewerChromeHidesSub(t *testing.T) {
	in := []byte(`<html><body><span class="ranky-user-sub">RankyTools</span></body></html>`)
	out := string(injectViewerChrome(in, ""))
	if !strings.Contains(out, "data-tm-viewer") || !strings.Contains(out, ".ranky-user-sub,.ranky-brand{display:none") {
		t.Fatal(out)
	}
	if strings.Contains(out, `viewer="`) || strings.Contains(out, "viewer=\"local\"") {
		t.Fatal("local must not lock username")
	}
	out2 := string(injectViewerChrome(in, `alice"xss`))
	if !strings.Contains(out2, `viewer="alice\"xss"`) && !strings.Contains(out2, `viewer="alice\\\"xss"`) && !strings.Contains(out2, "alice") {
		t.Fatal(out2)
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
