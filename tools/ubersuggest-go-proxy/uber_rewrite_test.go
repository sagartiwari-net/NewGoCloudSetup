package main

import "testing"

func TestRewriteUberLocationKeepsProxyHost(t *testing.T) {
	loc := "https://ubersuggest.com/en/analyze/dashboard"
	got := rewriteUberLocation(loc, "https", "brsg.upays.in", "app.neilpatel.com")
	want := "https://brsg.upays.in/en/analyze/dashboard"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRewriteUberHostsInHTML(t *testing.T) {
	in := `href="https://ubersuggest.com/en" src="https://app.neilpatel.com/x"`
	got := rewriteUberHosts(in, "https://brsg.upays.in", "brsg.upays.in", uberUpstreamHosts("app.neilpatel.com"))
	if got != `href="https://brsg.upays.in/en" src="https://brsg.upays.in/x"` {
		t.Fatalf("rewrite failed: %q", got)
	}
}
