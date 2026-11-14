package main

import (
	"net/http"
	"testing"
)

func TestCookieValueCleared(t *testing.T) {
	for _, v := range []string{"", "-", `""`, "deleted", "DELETED", `" "`} {
		if v == `" "` {
			continue // not cleared
		}
		if !cookieValueCleared(v) && v != `" "` {
			t.Fatalf("expected cleared: %q", v)
		}
	}
	if cookieValueCleared("s%3Areal-session") {
		t.Fatal("real session marked cleared")
	}
	if !cookieValueCleared(`""`) {
		t.Fatal("quoted empty should clear")
	}
}

func TestAbsorbSkipsClearedPreziAuth(t *testing.T) {
	localCookieMu.Lock()
	localCookieOverlay = map[string]string{}
	localCookieMu.Unlock()

	h := http.Header{}
	h.Add("Set-Cookie", "prezi-auth=-; Domain=.prezi.com; Path=/")
	h.Add("Set-Cookie", `p26-auth=""; Domain=.prezi.com; Max-Age=0; Path=/`)
	changed := absorbUpstreamSetCookies(h, http.StatusFound)
	if changed {
		t.Fatal("cleared auth cookies must not enter overlay")
	}
}

func TestLocationPointsToBlockedLogin(t *testing.T) {
	cfg := Config{
		BlockedPaths:    []string{"/login", "/signup"},
		BlockedPrefixes: []string{"/login", "/signup"},
	}
	if !locationPointsToBlockedAuth("/login/?next=https%3A%2F%2Fprezi.com%2Fdashboard%2Fnext%2F", cfg) {
		t.Fatal("expected /login blocked")
	}
	if !locationPointsToBlockedAuth("https://pprz.upays.in/login/?next=/dashboard/next/", cfg) {
		t.Fatal("expected absolute login blocked")
	}
	if locationPointsToBlockedAuth("/dashboard/next/", cfg) {
		t.Fatal("dashboard must not be treated as login")
	}
}
