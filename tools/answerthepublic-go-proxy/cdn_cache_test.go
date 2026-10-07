package main

import "testing"

func TestIsATPCDNPath(t *testing.T) {
	for _, p := range []string{
		"/assets/app.js",
		"/static/logo.png",
		"/wp-content/themes/x/style.css",
		"/dist/bundle.js",
		"/img/icon.svg",
		"/foo/bar.woff2",
	} {
		if !isATPCDNPath(p) {
			t.Fatalf("expected CDN path: %s", p)
		}
	}
	for _, p := range []string{
		"/api/v1/search",
		"/access",
		"/user/logout",
		"/en/dashboard/x/searches",
		"/billing/plan",
	} {
		if isATPCDNPath(p) {
			t.Fatalf("expected non-CDN path: %s", p)
		}
	}
}
