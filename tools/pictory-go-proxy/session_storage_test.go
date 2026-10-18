package main

import (
	"strings"
	"testing"
)

func TestLocalStorageAcceptsFlatDump(t *testing.T) {
	raw := `{"lastLoggedInUser":"user-1","CognitoIdentityServiceProvider.client.user-1.idToken":"id","referer":"https://app.pictory.ai/"}`
	got := localStorageJSONFromRaw(raw)
	if !strings.Contains(got, `"lastLoggedInUser":"user-1"`) {
		t.Fatalf("flat dump missing login key: %s", got)
	}
	if strings.Contains(got, "app.pictory.ai") {
		t.Fatalf("referer field was stored as localStorage: %s", got)
	}
	if parseCookieFromDB(raw) != "" {
		t.Fatal("flat localStorage dump was forwarded as a cookie header")
	}
}

func TestLocalStorageAcceptsRefererWrap(t *testing.T) {
	raw := `{"referer":"https://app.pictory.ai/","includedFormats":["localStorage"],"storage":{"localStorage":{"lastAuthMethod":"EMAIL","IdTokenJwtToken":"id"}}}`
	got := localStorageJSONFromRaw(raw)
	if !strings.Contains(got, `"lastAuthMethod":"EMAIL"`) || !strings.Contains(got, `"IdTokenJwtToken":"id"`) {
		t.Fatalf("referer wrap not read: %s", got)
	}
	if parseCookieFromDB(raw) != "" {
		t.Fatal("referer localStorage wrap was forwarded as a cookie header")
	}
}

func TestLocalStorageAcceptsTopLevelKey(t *testing.T) {
	raw := `{"referer":"https://app.pictory.ai/","localStorage":{"amplify-signin-with-hostedUI":"false"}}`
	got := localStorageJSONFromRaw(raw)
	if !strings.Contains(got, "amplify-signin-with-hostedUI") {
		t.Fatalf("top-level localStorage not read: %s", got)
	}
}

func TestCookieArrayStillParses(t *testing.T) {
	raw := `[{"name":"session","value":"abc","domain":"app.pictory.ai","hostOnly":true}]`
	if localStorageJSONFromRaw(raw) != "" {
		t.Fatal("cookie array was treated as localStorage")
	}
	header := parseCookieFromDB(raw)
	if !strings.Contains(header, "session=abc") {
		t.Fatalf("cookie array not parsed: %q", header)
	}
}

func TestRefererCookieWrapKeepsCookies(t *testing.T) {
	raw := `{"referer":"https://app.pictory.ai/","includedFormats":["cookies"],"cookies":[{"name":"session","value":"abc","domain":"app.pictory.ai"}]}`
	if localStorageJSONFromRaw(raw) != "" {
		t.Fatal("cookie wrap was treated as localStorage")
	}
	header := parseCookieFromDB(raw)
	if !strings.Contains(header, "session=abc") {
		t.Fatalf("referer cookie wrap not parsed: %q", header)
	}
}
