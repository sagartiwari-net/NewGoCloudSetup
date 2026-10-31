package main

import (
	"net/http"
	"testing"
)

func TestTenantPublicHostUsesRequestHost(t *testing.T) {
	cfg := Config{PublicHost: "ggrk.upays.in"}
	req := httptestRequest("ggrk.seogbtools.com")
	got := tenantPublicHost(req, cfg)
	if got != "ggrk.seogbtools.com" {
		t.Fatalf("got %q", got)
	}
}

func TestTenantPublicHostKeepsOwnerHost(t *testing.T) {
	cfg := Config{PublicHost: "ggrk.upays.in"}
	req := httptestRequest("ggrk.upays.in")
	got := tenantPublicHost(req, cfg)
	if got != "ggrk.upays.in" {
		t.Fatalf("got %q", got)
	}
}

func httptestRequest(host string) *http.Request {
	req, _ := http.NewRequest(http.MethodGet, "https://"+host+"/", nil)
	req.Host = host
	req.Header.Set("X-Forwarded-Host", host)
	return req
}
