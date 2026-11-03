package main

import (
	"net/http"
	"testing"
)

func TestPanelRequestHostUsesSeoGroupBuyHost(t *testing.T) {
	req := httptestRequest("envt.seogbtools.com")
	got := panelRequestHost(req, "envt.upays.in")
	if got != "envt.seogbtools.com" {
		t.Fatalf("got %q want envt.seogbtools.com", got)
	}
}

func TestPanelRequestHostKeepsOwnerHost(t *testing.T) {
	req := httptestRequest("envt.upays.in")
	got := panelRequestHost(req, "envt.upays.in")
	if got != "envt.upays.in" {
		t.Fatalf("got %q", got)
	}
}

func httptestRequest(host string) *http.Request {
	req, _ := http.NewRequest(http.MethodGet, "https://"+host+"/photos", nil)
	req.Host = host
	req.Header.Set("X-Forwarded-Host", host)
	return req
}
