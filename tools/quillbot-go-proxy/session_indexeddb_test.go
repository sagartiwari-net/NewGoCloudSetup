package main

import (
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
