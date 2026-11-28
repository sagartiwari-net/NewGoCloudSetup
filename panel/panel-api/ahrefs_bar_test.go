package main

import "testing"

func TestMaskAhrefsToken(t *testing.T) {
	if maskAhrefsToken("") != "" {
		t.Fatal("empty")
	}
	got := maskAhrefsToken("abcdefghijklmnopqrstuvwxyz")
	if got != "abcdef…wxyz" {
		t.Fatalf("got %q", got)
	}
	got = maskAhrefsToken(`{"accessToken":"tok_ABCDEFGHIJKLMNOP"}`)
	if got != "tok_AB…MNOP" {
		t.Fatalf("json got %q", got)
	}
}
