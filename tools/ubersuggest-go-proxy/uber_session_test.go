package main

import (
	"strings"
	"testing"
)

func TestUberSessionCookieHeaderAcceptsUbersuggestDomain(t *testing.T) {
	raw := `{
	  "cookies": [
	    {"domain":".ubersuggest.com","name":"id","value":"eyJhbGciOiJIUzI1NiJ9.test"},
	    {"domain":".ubersuggest.com","name":"trnstl","value":"1.abc.def"},
	    {"domain":".ubersuggest.com","name":"_ga","value":"GA1.1.1"}
	  ]
	}`
	got := uberSessionCookieHeader(raw)
	if got == "" {
		t.Fatal("expected session cookies, got empty (ubersuggest.com domain was filtered)")
	}
	if !strings.Contains(got, "id=") || !strings.Contains(got, "trnstl=") {
		t.Fatalf("missing auth cookies in %q", got)
	}
	if strings.Contains(got, "_ga=") {
		t.Fatalf("tracking cookie leaked into session header: %q", got)
	}
}

func TestUberSessionCookieHeaderStillAcceptsNeilpatel(t *testing.T) {
	raw := `[{"domain":".neilpatel.com","name":"id","value":"tok"},{"domain":"app.neilpatel.com","name":"state","value":"st"}]`
	got := uberSessionCookieHeader(raw)
	if !strings.Contains(got, "id=") || !strings.Contains(got, "state=") {
		t.Fatalf("neilpatel session broken: %q", got)
	}
}
