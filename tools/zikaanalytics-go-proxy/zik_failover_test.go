package main

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestStripNamedCookies(t *testing.T) {
	in := "ct_session=abc; SessionToken=stale; foo=bar; access_token=x"
	out := stripNamedCookies(in, "SessionToken", "access_token", "ct_session")
	if out != "foo=bar" {
		t.Fatalf("got %q", out)
	}
}

func TestZikJWTExpired(t *testing.T) {
	mk := func(exp int64) string {
		header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
		payload, _ := json.Marshal(map[string]int64{"exp": exp})
		body := base64.RawURLEncoding.EncodeToString(payload)
		return header + "." + body + ".sig"
	}
	if !zikJWTExpired(mk(time.Now().Add(-time.Hour).Unix())) {
		t.Fatal("expected expired")
	}
	if zikJWTExpired(mk(time.Now().Add(time.Hour).Unix())) {
		t.Fatal("expected not expired")
	}
}

func TestZikRewriteLoginLocation(t *testing.T) {
	cfg := Config{HomePath: "/dashboard"}
	if got := zikRewriteLoginLocation("https://app.zikanalytics.com/login", cfg); got != "/dashboard" {
		t.Fatalf("got %q", got)
	}
	if got := zikRewriteLoginLocation("/login?x=1", cfg); got != "/dashboard" {
		t.Fatalf("got %q", got)
	}
	if got := zikRewriteLoginLocation("/dashboard", cfg); got != "/dashboard" {
		t.Fatalf("got %q", got)
	}
}
