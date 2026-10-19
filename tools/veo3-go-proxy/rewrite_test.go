package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestRewriteFlowHost(t *testing.T) {
	got := rewriteText("https://flow.google.com/fx/tools/flow", "https://veo3.gt4rents.com", "veo3.gt4rents.com")
	if got != "https://veo3.gt4rents.com/fx/tools/flow" {
		t.Fatalf("got %s", got)
	}
	escaped := rewriteText(`https:\/\/flow.google.com\/`, "https://veo3.gt4rents.com", "veo3.gt4rents.com")
	if escaped != `https:\/\/veo3.gt4rents.com\/` {
		t.Fatalf("escaped got %s", escaped)
	}
	gstatic := rewriteText("https://www.gstatic.com/og/js.js", "https://veo3.gt4rents.com", "veo3.gt4rents.com")
	if gstatic != "https://veo3.gt4rents.com/__/host/www.gstatic.com/og/js.js" {
		t.Fatalf("gstatic got %s", gstatic)
	}
	rpc := rewriteText("https://ogads-pa.clients6.google.com/$rpc/GetAsyncData", "https://veo3.gt4rents.com", "veo3.gt4rents.com")
	if rpc != "https://veo3.gt4rents.com/__/host/ogads-pa.clients6.google.com/$rpc/GetAsyncData" {
		t.Fatalf("rpc got %s", rpc)
	}
	studio := rewriteText("https://labs.google/fx/tools/flow", "https://veo3.gt4rents.com", "veo3.gt4rents.com")
	if studio != "https://veo3.gt4rents.com/__/host/labs.google/fx/tools/flow" {
		t.Fatalf("studio got %s", studio)
	}
}

func TestRewriteLocationBreaksRootLoop(t *testing.T) {
	got := rewriteLocation("https://flow.google.com/", "https://veo3.gt4rents.com", "veo3.gt4rents.com")
	if got != "https://veo3.gt4rents.com/about" {
		t.Fatalf("got %s", got)
	}
}

func TestRestoreProxyURLs(t *testing.T) {
	got := restoreProxyURLs("https://veo3.gt4rents.com/__/host/labs.google/fx/tools/flow", "veo3.gt4rents.com")
	if got != "https://labs.google/fx/tools/flow" {
		t.Fatalf("got %s", got)
	}
	q := restoreProxyURLs("continue="+url.QueryEscape("https://veo3.gt4rents.com/about"), "veo3.gt4rents.com")
	if !strings.Contains(q, url.QueryEscape("https://flow.google.com/about")) {
		t.Fatalf("continue got %s", q)
	}
}

func TestFlowTarget(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://veo3.gt4rents.com/__/host/www.gstatic.com/og/js.js", nil)
	if err != nil {
		t.Fatal(err)
	}
	main, _ := url.Parse("https://flow.google.com")
	scheme, host, uri := flowTarget(req, main)
	if scheme != "https" || host != "www.gstatic.com" || uri != "/og/js.js" {
		t.Fatalf("%s %s %s", scheme, host, uri)
	}
	req.URL.Path = "/__/host/evil.example/$rpc"
	if scheme, _, _ := flowTarget(req, main); scheme != "" {
		t.Fatal("foreign host must not be proxied")
	}
}

func TestFlowQuietPath(t *testing.T) {
	if !flowQuietPath("/menu_item.css.map") || !flowQuietPath("/__/host/www.gstatic.com/a.js") {
		t.Fatal("static files must skip the device gate")
	}
	if flowQuietPath("/about") || flowQuietPath("/__/host/labs.google/fx/tools/flow") {
		t.Fatal("pages stay on the device gate")
	}
}

func TestVeoBlocked(t *testing.T) {
	if !veoBlocked("/pricing") || !veoBlocked("/settings") {
		t.Fatal("expected account pages blocked")
	}
	if veoBlocked("/") || veoBlocked("/fx/tools/flow") {
		t.Fatal("flow studio must stay open")
	}
}

func TestAuthCookieHidden(t *testing.T) {
	for _, name := range []string{"SID", "SAPISID", "__Secure-1PSID", "__Secure-OSID", "OSID"} {
		if !authCookie(name) {
			t.Fatalf("%s must stay on the proxy", name)
		}
	}
	if authCookie("_ga") || authCookie("NEXT_LOCALE") {
		t.Fatal("analytics cookie can reach the browser")
	}
}

func TestGoogleSAPISIDHash(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://flow.google.com/api", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "SAPISIDHASH 1_old")
	rewriteGoogleAuth(req, "SAPISID=test-sid; __Secure-1PAPISID=test-1p", "https://labs.google")
	got := req.Header.Get("Authorization")
	if strings.Contains(got, "test-sid") || strings.Contains(got, "test-1p") || strings.Contains(got, "old") {
		t.Fatalf("leaked %s", got)
	}
	if !strings.Contains(got, "SAPISIDHASH ") || !strings.Contains(got, "SAPISID1PHASH ") {
		t.Fatalf("got %s", got)
	}
	if req.Header.Get("X-Goog-AuthUser") != "0" {
		t.Fatalf("auth user %q", req.Header.Get("X-Goog-AuthUser"))
	}
}

func TestGoogleAuthInjectedWithoutBrowserHeader(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://flow.google.com/_/AiSandboxAngularFrontend/data/batchexecute", nil)
	if err != nil {
		t.Fatal(err)
	}
	rewriteGoogleAuth(req, "SAPISID=test-sid", "https://flow.google.com")
	got := req.Header.Get("Authorization")
	if !strings.HasPrefix(got, "SAPISIDHASH ") || strings.Contains(got, "test-sid") {
		t.Fatalf("got %s", got)
	}
}

func TestPanelCookieFlattensGoogleExport(t *testing.T) {
	raw := `{"cookies":[{"name":"SAPISID","value":"abc"},{"name":"SID","value":"def"}],"localStorage":{"x":"1"}}`
	got := panelCookieForAccount(raw)
	if !strings.Contains(got, "SAPISID=abc") || !strings.Contains(got, "SID=def") || strings.Contains(got, "localStorage") {
		t.Fatalf("got %s", got)
	}
}
