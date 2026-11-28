package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAllowVerifyRate(t *testing.T) {
	s := &server{cfg: config{VerifyRatePerMin: 2}}
	r := httptest.NewRequest(http.MethodPost, "/v1/session/verify", nil)
	r.RemoteAddr = "203.0.113.50:9999"
	if !s.allowVerifyRate(r) {
		t.Fatal("first should allow")
	}
	if !s.allowVerifyRate(r) {
		t.Fatal("second should allow")
	}
	if s.allowVerifyRate(r) {
		t.Fatal("third should block")
	}
}

func TestAllowVerifyRateDisabled(t *testing.T) {
	s := &server{cfg: config{VerifyRatePerMin: 0}}
	r := httptest.NewRequest(http.MethodPost, "/v1/session/verify", nil)
	r.RemoteAddr = "203.0.113.51:1"
	for i := 0; i < 100; i++ {
		if !s.allowVerifyRate(r) {
			t.Fatal("disabled limiter blocked")
		}
	}
}
