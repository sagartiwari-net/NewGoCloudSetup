package main

import "testing"

func TestShouldFailover(t *testing.T) {
	if !shouldFailover(401, nil) || !shouldFailover(403, nil) {
		t.Fatal("401/403 should failover")
	}
	if shouldFailover(200, []byte(`["Ok",{}]`)) {
		t.Fatal("200 Ok should not failover")
	}
	if shouldFailover(500, nil) {
		t.Fatal("500 currently not auto-failover in Phase 4")
	}
}

func TestMockUpstreamFailBearer(t *testing.T) {
	st, body := mockUpstreamForBearer("MOCK:401", "tbGetIconV3", "", "GET", nil)
	if st != 401 {
		t.Fatalf("status=%d want 401 body=%s", st, body)
	}
	st, body = mockUpstreamForBearer("MOCK", "tbGetIconV3", "input=%7B%22target%22%3A%22x.com%22%7D", "GET", nil)
	if st != 200 || !looksLikeAhrefsOK(body) {
		t.Fatalf("MOCK ok failed status=%d body=%s", st, body)
	}
}
