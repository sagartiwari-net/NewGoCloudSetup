package main

import (
	"net/http"
	"testing"
)

func TestCdnCacheKeyIncludesHost(t *testing.T) {
	upays, _ := http.NewRequest(http.MethodGet, "https://hhlm.upays.in/cdn-proxy/app.js", nil)
	upays.Host = "hhlm.upays.in"
	seogb, _ := http.NewRequest(http.MethodGet, "https://hhlm.seogbtools.com/cdn-proxy/app.js", nil)
	seogb.Host = "hhlm.seogbtools.com"
	if cdnCacheKey(upays) == "" || cdnCacheKey(upays) == cdnCacheKey(seogb) {
		t.Fatalf("host-specific cache keys required: %q vs %q", cdnCacheKey(upays), cdnCacheKey(seogb))
	}
}
