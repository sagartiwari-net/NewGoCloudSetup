package main

import (
	"strings"
	"testing"
)

func TestIndexedDBGoAutoShapeExtractsAccessToken(t *testing.T) {
	raw := `{
		"referer":"https://app.jasper.ai/",
		"includedFormats":["indexedDB"],
		"indexedDB":{
			"firebase-heartbeat-database":{"stores":{"firebase-heartbeat-store":[{"heartbeats":[],"lastSentHeartbeatDate":"2026-11-14"}]}},
			"firebaseLocalStorageDb":{"stores":{"firebaseLocalStorage":[{
				"fbase_key":"firebase:authUser:AIzaSyCawBlyCbiRm8cyy_iSMwQCSi-eYQAlZ-E:[DEFAULT]",
				"value":{
					"apiKey":"AIzaSyCawBlyCbiRm8cyy_iSMwQCSi-eYQAlZ-E",
					"email":"user@example.com",
					"stsTokenManager":{
						"accessToken":"header.payload.sig",
						"expirationTime":1794642838738,
						"refreshToken":"refresh-token-value"
					},
					"uid":"abc"
				}
			}]}}
		}
	}`
	if !isIndexedDBSession(raw) {
		t.Fatal("expected indexedDB session")
	}
	tok := extractFirebaseAccessToken(raw)
	if tok != "header.payload.sig" {
		t.Fatalf("accessToken=%q", tok)
	}
	if extractFirebaseExpirationMs(raw) != 1794642838738 {
		t.Fatalf("expirationTime=%d", extractFirebaseExpirationMs(raw))
	}
	idb := indexedDBJSONForInject(raw)
	if !strings.Contains(idb, "firebaseLocalStorageDb") || !strings.Contains(idb, "stsTokenManager") {
		t.Fatalf("inject json missing firebase auth: %s", idb)
	}
	if buildIndexedDBInjectHTML(raw) == "" {
		t.Fatal("expected non-empty HTML inject")
	}
}
