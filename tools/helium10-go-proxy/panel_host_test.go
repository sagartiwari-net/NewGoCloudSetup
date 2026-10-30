package main

import (
	"net/http"
	"testing"
)

func TestTenantPublicHostUsesRequestHost(t *testing.T) {
	cfg := Config{PublicHost: "hhlm.upays.in"}
	req := httptestRequest("hhlm.seogbtools.com")
	got := tenantPublicHost(req, cfg)
	if got != "hhlm.seogbtools.com" {
		t.Fatalf("got %q", got)
	}
}

func TestTenantPublicHostKeepsOwnerHost(t *testing.T) {
	cfg := Config{PublicHost: "hhlm.upays.in"}
	req := httptestRequest("hhlm.upays.in")
	got := tenantPublicHost(req, cfg)
	if got != "hhlm.upays.in" {
		t.Fatalf("got %q", got)
	}
}

func httptestRequest(host string) *http.Request {
	req, _ := http.NewRequest(http.MethodGet, "https://"+host+"/dashboard", nil)
	req.Host = host
	req.Header.Set("X-Forwarded-Host", host)
	return req
}
