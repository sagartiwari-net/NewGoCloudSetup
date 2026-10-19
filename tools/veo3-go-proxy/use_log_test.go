package main

import (
	"strings"
	"testing"
)

func TestVeoUsageSummarySkipsPrompt(t *testing.T) {
	body := []byte(`{"prompt":"a secret story","model":"veo-3.1","duration":5,"credits":12,"balance":88,"token":"hide-me"}`)
	got := veoUsageSummary(body)
	if strings.Contains(got, "secret") || strings.Contains(got, "hide-me") {
		t.Fatalf("leaked %s", got)
	}
	for _, part := range []string{"model=veo-3.1", "duration=5", "credits=12", "balance=88"} {
		if !strings.Contains(got, part) {
			t.Fatalf("missing %s in %s", part, got)
		}
	}
}

func TestShouldLogVeoUse(t *testing.T) {
	if !shouldLogVeoUse("POST", "/api/generate", "") {
		t.Fatal("generate post should log")
	}
	if shouldLogVeoUse("GET", "/_next/static/app.js", "credits=1") {
		t.Fatal("static should stay quiet")
	}
	if !shouldLogVeoUse("GET", "/api/user", "credits=4") {
		t.Fatal("credit numbers on a read should log")
	}
	if shouldLogVeoUse("GET", "/image-to-video", "") {
		t.Fatal("plain page should stay quiet")
	}
}
