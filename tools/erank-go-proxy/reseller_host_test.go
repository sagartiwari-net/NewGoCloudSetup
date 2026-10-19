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
	raw := []byte(`<script src="https://members.erank.com/assets/app.js"></script> https://erank.gt4rents.com/cdn-proxy/production/app.js "https:\/\/assets.erank.com\/build\/x.js" return"https://assets.erank.com/production/fde0431/build/"+e`)
	body := rewriteBody(raw, buildDomainReplacements(cfg, requestPublicBase(req, cfg)))
	for _, want := range []string{`src="/assets/app.js"`, ` /e-assets/production/app.js `, `"\/e-assets\/build\/x.js"`, `return"/e-assets/production/fde0431/build/"+e`} {
		if !bytes.Contains(body, []byte(want)) {
			t.Fatalf("missing %s in %s", want, body)
		}
	}
	if bytes.Contains(body, []byte("erank.gt4rents.com")) || bytes.Contains(body, []byte("members.erank.com")) || bytes.Contains(body, []byte("assets.erank.com")) {
		t.Fatalf("absolute host leaked: %s", body)
	}
}

func TestPatcherKeepsAssetsOnPageOrigin(t *testing.T) {
	cfg := Config{
		TargetURL:    "https://members.erank.com",
		CDNURL:       "https://assets.erank.com",
		PublicHost:   "erank.gt4rents.com",
		PublicScheme: "https",
		HomePath:     "/",
	}
	script := patcherScript(cfg)
	if bytes.Contains([]byte(script), []byte("%!s")) {
		t.Fatal(script)
	}
	for _, want := range []string{"erank.gt4rents.com", "O+'/e-assets'", "/cdn-proxy"} {
		if !bytes.Contains([]byte(script), []byte(want)) {
			t.Fatalf("missing %s", want)
		}
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
