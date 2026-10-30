package main

import (
	"net/http"
	"testing"
)

func TestStaticCacheKeyIncludesHost(t *testing.T) {
	upays := staticCacheKey(http.MethodGet, "/learning/app.css", "lnkd.upays.in")
	seogb := staticCacheKey(http.MethodGet, "/learning/app.css", "lnkd.seogbtools.com")
	if upays == "" || upays == seogb {
		t.Fatalf("host-specific cache keys required: %q vs %q", upays, seogb)
	}
}
