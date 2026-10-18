package main

import "testing"

func TestRewriteVeoHost(t *testing.T) {
	got := rewriteText("https://www.veo3ai.io/image-to-video", "https://veo3.gt4rents.com", "veo3.gt4rents.com")
	if got != "https://veo3.gt4rents.com/image-to-video" {
		t.Fatalf("got %s", got)
	}
}

func TestVeoBlocked(t *testing.T) {
	if !veoBlocked("/pricing") || !veoBlocked("/api/auth/signin") {
		t.Fatal("expected pricing and auth blocked")
	}
	if veoBlocked("/image-to-video") {
		t.Fatal("image-to-video must stay open")
	}
}

func TestAuthCookieHidden(t *testing.T) {
	if !authCookie("__Secure-authjs.session-token") {
		t.Fatal("session cookie must stay on the proxy")
	}
	if authCookie("NEXT_LOCALE") {
		t.Fatal("locale cookie can reach the browser")
	}
}
