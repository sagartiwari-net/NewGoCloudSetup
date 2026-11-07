package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsSellerAmpAuthLocation(t *testing.T) {
	cases := map[string]bool{
		"https://sas.selleramp.com/site/login?returnUrl=%2Fsas%2Flookup": true,
		"/site/logout": true,
		"https://sas.selleramp.com/r/api/auth/session/sign-out?redirectTo=/sas/lookup": true,
		"https://sas.selleramp.com/sas/lookup?SasLookup%5Bsearch_term%5D=x":           false,
		"/sas/lookup": false,
	}
	for loc, want := range cases {
		if got := isSellerAmpAuthLocation(loc); got != want {
			t.Fatalf("isSellerAmpAuthLocation(%q)=%v want %v", loc, got, want)
		}
	}
}

func TestBreakSellerAmpAuthLocationNeverReturnsLookup(t *testing.T) {
	cfg := Config{HomePath: "/"}
	loc := "https://sas.selleramp.com/site/login?returnUrl=%2Fsas%2Flookup%3FSasLookup%5Bsearch_term%5D%3Dtempered%2Bglass"
	got := breakSellerAmpAuthLocation(loc, "https", "seller.amzpremiumsoftware.com", cfg)
	want := "https://seller.amzpremiumsoftware.com/"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSellerAmpSameResourceRedirect(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/sas/lookup?SasLookup%5Bsearch_term%5D=tempered+glass", nil)
	r.Host = "seller.amzpremiumsoftware.com"
	loc := "https://seller.amzpremiumsoftware.com/sas/lookup?SasLookup%5Bsearch_term%5D=tempered+glass"
	if !sellerAmpSameResourceRedirect(r, loc, "seller.amzpremiumsoftware.com") {
		t.Fatal("expected same-resource redirect")
	}
	if sellerAmpSameResourceRedirect(r, "https://seller.amzpremiumsoftware.com/", "seller.amzpremiumsoftware.com") {
		t.Fatal("home should not match lookup")
	}
}
