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
		PublicHost:      "zknl.upays.in",
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
		if p[0] == "https://zknl.upays.in" {
			gotOwner = true
			if p[1] != resellerBase {
				t.Fatalf("owner rewrite = %q", p[1])
			}
		}
		if strings.Contains(p[1], "zknl.upays.in") {
			t.Fatalf("rewrite still points at owner host: %#v", p)
		}
	}
	if !gotAPI || !gotOwner {
		t.Fatalf("missing pairs api=%v owner=%v pairs=%v", gotAPI, gotOwner, pairs)
	}
}

func TestRequestPublicBaseUsesForwardedHost(t *testing.T) {
	cfg := Config{PublicHost: "zknl.upays.in", PublicScheme: "http"}
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
	cfg := Config{PublicHost: "zknl.upays.in", PublicScheme: "http"}
	r := httptest.NewRequest("GET", "/", nil)
	r.Host = "zknl.upays.in"
	got := requestPublicBase(r, cfg)
	if got != "https://zknl.upays.in" {
		t.Fatalf("public base = %q (http config must not leak onto live hosts)", got)
	}
}

func TestRequestPublicBaseIgnoresHTTPForwardedProtoOnLiveHost(t *testing.T) {
	cfg := Config{PublicHost: "zknl.upays.in", PublicScheme: "http"}
	r := httptest.NewRequest("GET", "/", nil)
	r.Host = "zik.amzpremiumsoftware.com"
	r.Header.Set("X-Forwarded-Proto", "http")
	got := requestPublicBase(r, cfg)
	if got != "https://zik.amzpremiumsoftware.com" {
		t.Fatalf("public base = %q (X-Forwarded-Proto=http must not create mixed-content rewrites)", got)
	}
}

func TestPatcherRewritesOwnerAndAPI(t *testing.T) {
	cfg := Config{
		TargetURL:       "https://app.zikanalytics.com",
		CDNURL:          "https://app.zikanalytics.com",
		PublicHost:      "zknl.upays.in",
		ExtraCDNDomains: []string{"api.zikanalytics.com"},
		HomePath:        "/dashboard",
	}
	js := patcherScript(cfg)
	for _, want := range []string{
		`OWNER = "zknl.upays.in"`,
		`api.zikanalytics.com`,
		`/extra-cdn-0`,
		`mapProxyUrl`,
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("patcher missing %q", want)
		}
	}
}
