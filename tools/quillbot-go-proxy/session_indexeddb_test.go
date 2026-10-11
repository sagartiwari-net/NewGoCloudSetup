package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestIndexedDBBothShapes(t *testing.T) {
	row := `"firebaseLocalStorage":[{"fbase_key":"k","value":{"apiKey":"AIzaTEST","stsTokenManager":{"accessToken":"tok","expirationTime":999,"refreshToken":"r"}}}]`
	goauto := `{"referer":"https://quillbot.com/","includedFormats":["indexedDB"],"indexedDB":{"referer":{"stores":{}},"firebaseLocalStorageDb":{"stores":{` + row + `}}}}`
	bare := `{"firebaseLocalStorageDb":{"stores":{` + row + `}},"firebase-heartbeat-database":{"stores":{"firebase-heartbeat-store":[]}}}`

	if !isIndexedDBSession(goauto) || !isIndexedDBSession(bare) {
		t.Fatal("both cookie shapes should be sessions")
	}
	if extractFirebaseAccessToken(goauto) != "tok" || extractFirebaseAccessToken(bare) != "tok" {
		t.Fatal("access token not found")
	}
	if firebaseDisplayName(goauto) != "" {
		t.Fatal("fixture has no display name")
	}
	named := strings.Replace(goauto, `"apiKey":"AIzaTEST"`, `"apiKey":"AIzaTEST","displayName":"Ada"`, 1)
	if firebaseDisplayName(named) != "Ada" {
		t.Fatal("display name")
	}
	injHTML := buildIndexedDBInjectHTML(goauto)
	if !strings.Contains(injHTML, "deleteDatabase") || !strings.Contains(injHTML, "indexedDB.open(dbName, 1)") {
		t.Fatal("firebase db must be recreated at version 1")
	}
	inj := indexedDBJSONForInject(goauto)
	if !strings.Contains(inj, "firebaseLocalStorageDb") {
		t.Fatal("inject missing firebase db")
	}
	if strings.Contains(inj, "referer") {
		t.Fatal("inject should not include wrapper keys")
	}
	if !strings.Contains(indexedDBJSONForInject(bare), "firebase-heartbeat-database") {
		t.Fatal("bare dump should keep the heartbeat db")
	}
}

func TestHostReplaceLeavesAuthDomain(t *testing.T) {
	in := []byte(`auth.quillbot.com https://quillbot.com "quillbot.com"`)
	out := string(rewriteBody(in, [][2]string{
		{"https://quillbot.com", "https://quillbot.gt4rents.com"},
		{"quillbot.com", "quillbot.gt4rents.com"},
	}))
	if !strings.Contains(out, "auth.quillbot.com") {
		t.Fatalf("auth domain rewritten: %s", out)
	}
	if !strings.Contains(out, "https://quillbot.gt4rents.com") || strings.Contains(out, `"quillbot.com"`) {
		t.Fatalf("public host not rewritten: %s", out)
	}
}

func TestGateQuillbotBootHoldsAppScripts(t *testing.T) {
	page := []byte(`<html><head>
<script data-tm-idb-restore="1">window.__TM_IDB_READY=Promise.resolve()</script>
<script data-tm-patcher="1">window.__patched=1</script>
<script src="/polyfills-v2.js"></script>
<script>window.__booted=1</script>
<script type="application/ld+json">{"@type":"WebSite"}</script>
</head><body></body></html>`)
	out := string(gateQuillbotBoot(page))
	if !strings.Contains(out, `data-tm-idb-restore="1"`) || !strings.Contains(out, `data-tm-patcher="1"`) {
		t.Fatal("our scripts were held")
	}
	if !strings.Contains(out, `data-tm-src="/polyfills-v2.js"`) || !strings.Contains(out, `type="text/tm-hold"`) {
		t.Fatalf("app script not held: %s", out)
	}
	if !strings.Contains(out, `window.__booted=1`) || !strings.Contains(out, `data-tm-idb-replay="1"`) {
		t.Fatalf("inline boot or replay missing: %s", out)
	}
	if !strings.Contains(out, `application/ld+json`) {
		t.Fatal("json-ld script should stay")
	}
}

func TestQuillbotChromeTargetsExactNodes(t *testing.T) {
	html := quillbotChromeHTML("sagar")
	for _, needle := range []string{
		`.MuiPopover-paper:has([data-testid="logout-option"])`,
		`p.MuiTypography-bodyMedium`,
		`div.MuiTypography-subtitleMedium.MuiTypography-noWrap`,
		`"sagar"`,
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("missing %s", needle)
		}
	}
}

func TestQuillbotSharedAssetsSkipDeviceGate(t *testing.T) {
	if !quillbotSharedAsset("/locales/prod/6ac89c65d6f02f60086a7e93/en/Global") {
		t.Fatal("locale json should be shared")
	}
	if !quillbotSharedAsset("/rebrand/manifest.json") {
		t.Fatal("rebrand manifest should be shared")
	}
	if quillbotSharedAsset("/api/auth/session") {
		t.Fatal("account api must stay gated")
	}
	if !isCacheableStaticPath("/locales/prod/x/en/Global") || !isCacheableStaticPath("/rebrand/manifest.json") {
		t.Fatal("shared assets should use the static path")
	}
	watch := string(injectQuillbotLogoutWatch([]byte("<html><body></body></html>"), "Sahana"))
	if !strings.Contains(watch, "Log in / Sign up") || !strings.Contains(watch, "Just a moment") {
		t.Fatal("logout watch should ignore a blank page and a cloudflare challenge")
	}
	if !strings.Contains(watch, "if (!uiOut) return;") {
		t.Fatal("logout watch should require the logged-out screen")
	}
}

func TestExtraCDNRootQueryUsesStreamHost(t *testing.T) {
	domains := []string{
		"https://assets.quillbot.com",
		"https://libs.quillbot.com",
		"https://media.quillbot.com",
		"https://auth.quillbot.com",
		"https://stream.quillbot.com",
	}
	idx, up, raw, ok := matchExtraCDN("/extra-cdn-4", "/extra-cdn-4", domains)
	if !ok || idx != 4 || up != "/" || raw != "" {
		t.Fatalf("root socket route = %d %q %q %v", idx, up, raw, ok)
	}
	idx, up, _, ok = matchExtraCDN("/extra-cdn-4/api/ws/fallback/event", "/extra-cdn-4/api/ws/fallback/event", domains)
	if !ok || idx != 4 || up != "/api/ws/fallback/event" {
		t.Fatalf("fallback route = %d %q %v", idx, up, ok)
	}
	if _, _, _, ok = matchExtraCDN("/api/ai-detector/limit", "/api/ai-detector/limit", domains); ok {
		t.Fatal("api path must stay on quillbot.com")
	}
	if !cloudflareChallenge(429, http.Header{}, []byte("<title>Just a moment...</title>")) {
		t.Fatal("cloudflare challenge should be detected")
	}
	if cloudflareChallenge(200, http.Header{}, []byte("<title>Just a moment...</title>")) {
		t.Fatal("a normal page must not be treated as a challenge")
	}
}
