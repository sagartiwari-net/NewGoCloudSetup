package main

import (
	"net/http"
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
	rewriteGoogleAuth(req, "SAPISID=test-sid; __Secure-1PAPISID=test-1p")
	got := req.Header.Get("Authorization")
	if strings.Contains(got, "test-sid") || strings.Contains(got, "test-1p") || strings.Contains(got, "old") {
		t.Fatalf("leaked %s", got)
	}
	if !strings.Contains(got, "SAPISIDHASH ") || !strings.Contains(got, "SAPISID1PHASH ") {
		t.Fatalf("got %s", got)
	}
}
