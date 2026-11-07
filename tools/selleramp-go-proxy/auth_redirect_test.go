package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestIsSellerAmpAuthLocation(t *testing.T) {
	cases := map[string]bool{
		"https://sas.selleramp.com/site/login?returnUrl=%2Fsas%2Flookup": true,
		"/site/logout": true,
		"https://sas.selleramp.com/r/api/auth/session/sign-out?redirectTo=/sas/lookup": true,
		"https://sas.selleramp.com/r/api/auth/session":                                 false,
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

func TestSellerAmpSelfPathRedirectIgnoresQuery(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/sas/lookup?SasLookup%5Bsearch_term%5D=tempered+glass", nil)
	r.Host = "seller.amzpremiumsoftware.com"
	loc := "https://seller.amzpremiumsoftware.com/sas/lookup?SasLookup%5Bsearch_term%5D=other"
	if !sellerAmpSelfPathRedirect(r, loc, "seller.amzpremiumsoftware.com") {
		t.Fatal("expected self-path redirect when only query differs")
	}
	post := httptest.NewRequest(http.MethodPost, "/sas/lookup", nil)
	post.Host = "seller.amzpremiumsoftware.com"
	if sellerAmpSelfPathRedirect(post, loc, "seller.amzpremiumsoftware.com") {
		t.Fatal("POST PRG self-path should not be intercepted")
	}
}

func TestMergeSetCookieHeader(t *testing.T) {
	got := mergeSetCookieHeader("a=1; b=2", []string{
		"b=9; Path=/; HttpOnly",
		"c=3; Path=/",
		"a=; Max-Age=0; Path=/",
	})
	if !strings.Contains(got, "b=9") || !strings.Contains(got, "c=3") || strings.Contains(got, "a=") {
		t.Fatalf("unexpected merge result: %q", got)
	}
}

func TestResolveSellerAmpFollowURLMapsPublicHost(t *testing.T) {
	target, err := url.Parse("https://sas.selleramp.com")
	if err != nil {
		t.Fatal(err)
	}
	cur := *target
	cur.Path = "/sas/lookup"
	cur.RawQuery = "SasLookup%5Bsearch_term%5D=x"
	next, err := resolveSellerAmpFollowURL(cur, "https://seller.amzpremiumsoftware.com/sas/lookup?SasLookup%5Bsearch_term%5D=x", "seller.amzpremiumsoftware.com", target)
	if err != nil {
		t.Fatal(err)
	}
	if next.Host != "sas.selleramp.com" {
		t.Fatalf("host=%q", next.Host)
	}
	if next.Path != "/sas/lookup" {
		t.Fatalf("path=%q", next.Path)
	}
}

func TestResolveSellerAmpFollowURLKeepsNextSASPath(t *testing.T) {
	target, err := url.Parse("https://sas.selleramp.com")
	if err != nil {
		t.Fatal(err)
	}
	cur := *target
	cur.Path = "/sas/lookup"
	next, err := resolveSellerAmpFollowURL(cur, "https://sas.selleramp.com/r/sas/lookup?SasLookup%5Bsearch_term%5D=tempered", "sllr.upays.in", target)
	if err != nil {
		t.Fatal(err)
	}
	if next.Path != "/r/sas/lookup" {
		t.Fatalf("path=%q want /r/sas/lookup (must not normalize back to /sas)", next.Path)
	}
}

func TestSellerAmpUpstreamPathPrefersNextLookup(t *testing.T) {
	cases := map[string]string{
		"/sas/lookup":           "/r/sas/lookup",
		"/sas/history":          "/r/sas/history",
		"/r/sas/lookup":         "/r/sas/lookup",
		"/site/login":           "/site/login",
		"/sas":                  "/r/sas/lookup",
	}
	for in, want := range cases {
		if got := sellerAmpUpstreamPath(in); got != want {
			t.Fatalf("sellerAmpUpstreamPath(%q)=%q want %q", in, got, want)
		}
	}
}
