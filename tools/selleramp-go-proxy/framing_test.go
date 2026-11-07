package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAllowExtensionFraming(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	allowExtensionFraming(w)
	if w.Header().Get("X-Frame-Options") != "" {
		t.Fatalf("X-Frame-Options still set: %q", w.Header().Get("X-Frame-Options"))
	}
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-ancestors") || !strings.Contains(csp, "chrome-extension:") {
		t.Fatalf("CSP missing frame-ancestors chrome-extension: %q", csp)
	}
}
