package main

import (
	"bytes"
	"net/http"
	"testing"
)

func TestResellerHostStaysOnRequestDomain(t *testing.T) {
	cfg := Config{
		TargetURL:    "https://members.erank.com",
		CDNURL:       "https://assets.erank.com",
		PublicHost:   "erank.gt4rents.com",
		PublicScheme: "https",
	}
	req, err := http.NewRequest(http.MethodGet, "https://ernk.amzpremiumsoftware.com/dashboard", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "ernk.amzpremiumsoftware.com"
	body := rewriteBody([]byte(`<script src="https://members.erank.com/assets/app.js"></script> https://erank.gt4rents.com/assets/font.woff2`), buildDomainReplacements(cfg, requestPublicBase(req, cfg)))
	if !bytes.Contains(body, []byte("https://ernk.amzpremiumsoftware.com/assets/app.js")) {
		t.Fatalf("script host: %s", body)
	}
	if !bytes.Contains(body, []byte("https://ernk.amzpremiumsoftware.com/assets/font.woff2")) {
		t.Fatalf("font host: %s", body)
	}
	if bytes.Contains(body, []byte("erank.gt4rents.com")) || bytes.Contains(body, []byte("members.erank.com")) {
		t.Fatalf("official host leaked: %s", body)
	}
}

func TestCDNCacheKeyIncludesHost(t *testing.T) {
	a, _ := http.NewRequest(http.MethodGet, "https://erank.gt4rents.com/assets/app.js", nil)
	a.Host = "erank.gt4rents.com"
	b, _ := http.NewRequest(http.MethodGet, "https://ernk.amzpremiumsoftware.com/assets/app.js", nil)
	b.Host = "ernk.amzpremiumsoftware.com"
	if cdnCacheKey(a) == "" || cdnCacheKey(a) == cdnCacheKey(b) {
		t.Fatalf("keys %q %q", cdnCacheKey(a), cdnCacheKey(b))
	}
}
