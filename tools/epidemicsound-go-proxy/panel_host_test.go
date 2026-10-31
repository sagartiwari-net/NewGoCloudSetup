package main

import (
	"net/http"
	"testing"
)

func TestTenantPublicHostUsesRequestHost(t *testing.T) {
	cfg := Config{PublicHost: "pdmc.upays.in"}
	req := httptestRequest("pdmc.seogbtools.com")
	got := tenantPublicHost(req, cfg)
	if got != "pdmc.seogbtools.com" {
		t.Fatalf("got %q", got)
	}
}

func TestTenantPublicHostKeepsOwnerHost(t *testing.T) {
	cfg := Config{PublicHost: "pdmc.upays.in"}
	req := httptestRequest("pdmc.upays.in")
	got := tenantPublicHost(req, cfg)
	if got != "pdmc.upays.in" {
		t.Fatalf("got %q", got)
	}
}

func httptestRequest(host string) *http.Request {
	req, _ := http.NewRequest(http.MethodGet, "https://"+host+"/music/featured/", nil)
	req.Host = host
	req.Header.Set("X-Forwarded-Host", host)
	return req
}
