package main

import (
	"strconv"
	"strings"
	"testing"
)

func TestChromeAutoUserAgentMatchesHello(t *testing.T) {
	ua := chromeAutoUserAgent()
	want := "Chrome/" + strconv.Itoa(chromeAutoMajor()) + "."
	if !strings.Contains(ua, want) {
		t.Fatalf("ua %q missing %q", ua, want)
	}
}

func TestCanvaUpstreamUAReplacesStaleChrome(t *testing.T) {
	stale := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	got := canvaUpstreamUA(stale, stale)
	if chromeMajorFromUA(got) != chromeAutoMajor() {
		t.Fatalf("stale ua kept: %q", got)
	}
	fresh := chromeAutoUserAgent()
	if canvaUpstreamUA(fresh, stale) != fresh {
		t.Fatal("current account ua should be kept")
	}
}
