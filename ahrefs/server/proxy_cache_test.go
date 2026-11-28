package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStripFreshParam(t *testing.T) {
	got := stripFreshParam("input=%7B%22target%22%3A%22x.com%22%7D&fresh=1")
	want := "input=%7B%22target%22%3A%22x.com%22%7D"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if stripFreshParam("a=1&b=2") != "a=1&b=2" {
		t.Fatal("untouched query changed")
	}
}

func TestWantsFresh(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v4/tbGetIconV3?fresh=1", nil)
	if !wantsFresh(r) {
		t.Fatal("expected fresh from query")
	}
	r2 := httptest.NewRequest(http.MethodGet, "/v4/tbGetIconV3", nil)
	r2.Header.Set("X-TM-Bar-Fresh", "1")
	if !wantsFresh(r2) {
		t.Fatal("expected fresh from header")
	}
	r3 := httptest.NewRequest(http.MethodGet, "/v4/tbGetIconV3", nil)
	if wantsFresh(r3) {
		t.Fatal("expected not fresh")
	}
}

func TestV4CacheRoundTrip(t *testing.T) {
	key := v4CacheKey("tbGetIconV3", "input=x")
	v4CacheDelete(key)
	if _, ok := v4CacheGet(key); ok {
		t.Fatal("expected miss")
	}
	v4CacheSet(key, 200, []byte(`["Ok",{}]`), 1<<30)
	hit, ok := v4CacheGet(key)
	if !ok || hit.status != 200 {
		t.Fatal("expected hit")
	}
	v4CacheDelete(key)
	if _, ok := v4CacheGet(key); ok {
		t.Fatal("expected deleted")
	}
}
