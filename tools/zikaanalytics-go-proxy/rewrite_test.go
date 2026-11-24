package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildDomainReplacementsUsesRequestHost(t *testing.T) {
	cfg := Config{
		TargetURL:       "https://app.zikanalytics.com",
		CDNURL:          "https://app.zikanalytics.com",
		PublicHost:      "ziknl.upays.in",
		PublicScheme:    "https",
		ExtraCDNDomains: []string{"api.zikanalytics.com"},
	}
	resellerBase := "https://zik.amzpremiumsoftware.com"
	pairs := buildDomainReplacements(cfg, resellerBase)

	gotAPI := false
	gotOwner := false
	for _, p := range pairs {
		if p[0] == "https://api.zikanalytics.com" {
			gotAPI = true
			if p[1] != resellerBase+"/extra-cdn-0" {
				t.Fatalf("api rewrite = %q", p[1])
			}
		}
		if p[0] == "https://ziknl.upays.in" {
			gotOwner = true
			if p[1] != resellerBase {
				t.Fatalf("owner rewrite = %q", p[1])
			}
		}
		if strings.Contains(p[1], "ziknl.upays.in") {
			t.Fatalf("rewrite still points at owner host: %#v", p)
		}
	}
	if !gotAPI || !gotOwner {
		t.Fatalf("missing pairs api=%v owner=%v pairs=%v", gotAPI, gotOwner, pairs)
	}
}

func TestRequestPublicBaseUsesForwardedHost(t *testing.T) {
	cfg := Config{PublicHost: "ziknl.upays.in", PublicScheme: "http"}
	r := httptest.NewRequest("GET", "https://ignored/", nil)
	r.Host = "ignored.example"
	r.Header.Set("X-Forwarded-Host", "zik.amzpremiumsoftware.com")
	r.Header.Set("X-Forwarded-Proto", "https")
	got := requestPublicBase(r, cfg)
	if got != "https://zik.amzpremiumsoftware.com" {
		t.Fatalf("public base = %q", got)
	}
}

func TestRequestPublicBaseForcesHTTPSWithoutForwardedProto(t *testing.T) {
	cfg := Config{PublicHost: "ziknl.upays.in", PublicScheme: "http"}
	r := httptest.NewRequest("GET", "/", nil)
	r.Host = "ziknl.upays.in"
	got := requestPublicBase(r, cfg)
	if got != "https://ziknl.upays.in" {
		t.Fatalf("public base = %q (http config must not leak onto live hosts)", got)
	}
}

func TestPatcherRewritesOwnerAndAPI(t *testing.T) {
	cfg := Config{
		TargetURL:       "https://app.zikanalytics.com",
		CDNURL:          "https://app.zikanalytics.com",
		PublicHost:      "ziknl.upays.in",
		ExtraCDNDomains: []string{"api.zikanalytics.com"},
		HomePath:        "/dashboard",
	}
	js := patcherScript(cfg)
	for _, want := range []string{
		`OWNER = "ziknl.upays.in"`,
		`api.zikanalytics.com`,
		`/extra-cdn-0`,
		`mapProxyUrl`,
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("patcher missing %q", want)
		}
	}
}
