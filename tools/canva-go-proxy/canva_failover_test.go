package main

import "testing"

func TestCanvaBodyLooksLikeBlock(t *testing.T) {
	page := []byte(`<html><body><h1>We'll have you designing again soon</h1><p>RayID: abc123</p></body></html>`)
	if !canvaBodyLooksLikeBlock(page) {
		t.Fatal("canva block page was not detected")
	}
	if canvaBodyLooksLikeBlock([]byte(`<html><body>Open a design in Canva</body></html>`)) {
		t.Fatal("normal canva html was treated as a block page")
	}
	if canvaBodyLooksLikeLogin(page) {
		t.Fatal("block page was treated as a login wall")
	}
}
