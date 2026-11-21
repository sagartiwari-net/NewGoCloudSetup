package main

import (
	"net/http"
	"testing"
)

func TestEnvatoRefererSlug(t *testing.T) {
	got := envatoRefererSlug("https://elements.envato.com/man-plays-guitar-in-low-light-setting-ABC123")
	if got != "man-plays-guitar-in-low-light-setting-ABC123" {
		t.Fatalf("got %q", got)
	}
	if envatoRefererSlug("https://elements.envato.com/stock-video") != "" {
		t.Fatal("category should be junk")
	}
}

func TestEnvatoIsBillableLabel(t *testing.T) {
	if !envatoIsBillableLabel("clip.mp4", "item:abc") {
		t.Fatal("file should bill")
	}
	if !envatoIsBillableLabel("item:abc-def", "item:abc-def") {
		t.Fatal("item id should bill")
	}
	if !envatoIsBillableLabel("man-plays-guitar-in-low-light", "item:abc") {
		t.Fatal("slug with item dedupe should bill")
	}
	if envatoIsBillableLabel("stock-video", "") {
		t.Fatal("junk should not bill")
	}
}

func TestEnvatoAliasMeterKeysIncludesCanonical(t *testing.T) {
	keys := envatoAliasMeterKeys("download_limit")
	if keys[0] != "download_limit" {
		t.Fatalf("canonical first: %v", keys)
	}
	found := false
	for _, k := range keys {
		if k == "download" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected download alias in %v", keys)
	}
}

func TestDownloadIdentityKey(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://app.envato.com/download.data?itemUuid=F9F2203D-784B-4E2E-843C-C225521A56BA", nil)
	got := downloadIdentityKey(req)
	if got != "item:f9f2203d-784b-4e2e-843c-c225521a56ba" {
		t.Fatalf("got %q", got)
	}
}
