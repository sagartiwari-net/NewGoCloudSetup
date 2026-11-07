package main

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicOriginFromRequestPrefersTenantHost(t *testing.T) {
	cfg := Config{PublicHost: "sllr.upays.in", PublicScheme: "https"}
	r := httptest.NewRequest(http.MethodGet, "/extension.zip", nil)
	r.Host = "seller.amzpremiumsoftware.com"
	r.Header.Set("X-Forwarded-Proto", "https")
	origin, host := publicOriginFromRequest(r, cfg)
	if host != "seller.amzpremiumsoftware.com" {
		t.Fatalf("host=%q want seller.amzpremiumsoftware.com", host)
	}
	if origin != "https://seller.amzpremiumsoftware.com" {
		t.Fatalf("origin=%q", origin)
	}
}

func TestBakeExtensionBytesUsesProxyHost(t *testing.T) {
	zipBytes, err := bakeExtensionBytes("https://seller.amzpremiumsoftware.com", "seller.amzpremiumsoftware.com")
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatal(err)
	}
	var manifest, parser string
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		switch f.Name {
		case "manifest.json":
			manifest = string(b)
		case "js/background/utility/sasUrlParser.js":
			parser = string(b)
		}
	}
	if strings.Contains(manifest, "__SAS_PROXY_") {
		t.Fatal("manifest still has placeholders")
	}
	if !strings.Contains(manifest, "*://seller.amzpremiumsoftware.com/*") {
		t.Fatal("manifest missing exact tenant match")
	}
	if !strings.Contains(manifest, "*://*.amzpremiumsoftware.com/*") {
		t.Fatal("manifest missing amzpremium wildcard")
	}
	if !strings.Contains(parser, "seller\\.amzpremiumsoftware\\.com") {
		t.Fatalf("sasUrlParser missing escaped host: %s", parser)
	}
}
