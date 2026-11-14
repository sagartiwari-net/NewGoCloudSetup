package main

import (
	"net/http"
	"testing"
)

func TestAbsorbSkipsAcceptedWAF(t *testing.T) {
	localCookieMu.Lock()
	localCookieOverlay = map[string]string{}
	localCookieMu.Unlock()

	h := http.Header{}
	h.Add("Set-Cookie", "connect.sid=bad-from-waf; Path=/")
	if absorbUpstreamSetCookies(h, http.StatusAccepted) {
		t.Fatal("202 WAF must not absorb cookies")
	}
	localCookieMu.Lock()
	defer localCookieMu.Unlock()
	if localCookieOverlay["connect.sid"] != "" {
		t.Fatalf("connect.sid leaked into overlay: %q", localCookieOverlay["connect.sid"])
	}
}

func TestPreferBrowserOnlyWhenMissingWAF(t *testing.T) {
	account := "connect.sid=good; aws-waf-token=panel-token; userSession=u"
	browser := "aws-waf-token=browser-proxy-token; other=1"
	// Mimic production guard: only prefer when missing.
	out := account
	if !containsCookieName(out, "aws-waf-token") {
		out = preferBrowserCookie(out, browser, "aws-waf-token")
	}
	if !containsCookieValue(out, "panel-token") {
		t.Fatalf("panel waf token replaced: %s", out)
	}
	missing := "connect.sid=good; userSession=u"
	filled := preferBrowserCookie(missing, browser, "aws-waf-token")
	if !containsCookieValue(filled, "browser-proxy-token") {
		t.Fatalf("expected browser fill: %s", filled)
	}
}

func containsCookieName(header, name string) bool {
	return cookieHeaderToMap(header)[name] != ""
}

func containsCookieValue(header, want string) bool {
	for _, v := range cookieHeaderToMap(header) {
		if v == want {
			return true
		}
	}
	return false
}
