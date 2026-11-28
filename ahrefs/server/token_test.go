package main

import "testing"

func TestExtractAccessToken(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`plain-token`, `plain-token`},
		{`{"accessToken":"abc"}`, `abc`},
		{`{"ahToken":{"accessToken":"nested"}}`, `nested`},
		{`{"activationData":{"ahToken":{"accessToken":"deep"}}}`, `deep`},
		{`{}`, ``},
	}
	for _, c := range cases {
		got := extractAccessToken(c.in)
		if got != c.want {
			t.Fatalf("extractAccessToken(%q)=%q want %q", c.in, got, c.want)
		}
	}
}
