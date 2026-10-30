package main

import (
	"net/http"
	"testing"
)

func TestTenantPublicHostUsesRequestHost(t *testing.T) {
	cfg := Config{PublicHost: "lnkd.upays.in"}
	req := httptestRequest("lnkd.seogbtools.com")
	got := tenantPublicHost(req, cfg)
	if got != "lnkd.seogbtools.com" {
		t.Fatalf("got %q", got)
	}
}

func TestTenantPublicHostKeepsOwnerHost(t *testing.T) {
	cfg := Config{PublicHost: "lnkd.upays.in"}
	req := httptestRequest("lnkd.upays.in")
	got := tenantPublicHost(req, cfg)
	if got != "lnkd.upays.in" {
		t.Fatalf("got %q", got)
	}
}

func httptestRequest(host string) *http.Request {
	req, _ := http.NewRequest(http.MethodGet, "https://"+host+"/learning/", nil)
	req.Host = host
	req.Header.Set("X-Forwarded-Host", host)
	return req
}
