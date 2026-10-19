package main

import "testing"

func TestRewriteVeoHost(t *testing.T) {
	got := rewriteText("https://www.veo3ai.io/image-to-video", "https://veo3.gt4rents.com", "veo3.gt4rents.com")
	if got != "https://veo3.gt4rents.com/image-to-video" {
		t.Fatalf("got %s", got)
	}
	escaped := rewriteText(`https:\/\/www.veo3ai.io\/api\/auth\/session`, "https://veo3.gt4rents.com", "veo3.gt4rents.com")
	if escaped != `https:\/\/veo3.gt4rents.com\/api\/auth\/session` {
		t.Fatalf("escaped got %s", escaped)
	}
}

func TestVeoBlocked(t *testing.T) {
	if !veoBlocked("/pricing") || !veoBlocked("/api/auth/signin") || !veoBlocked("/api/auth/signout") {
		t.Fatal("expected pricing and sign-in blocked")
	}
	if veoBlocked("/image-to-video") || veoBlocked("/api/auth/session") || veoBlocked("/api/auth/csrf") {
		t.Fatal("studio and session check must stay open")
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
