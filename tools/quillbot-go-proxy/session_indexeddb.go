package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// GoAuto-style export with IndexedDB (Jasper / Firebase).
type goAutoIndexedDBExport struct {
	Referer         string          `json:"referer"`
	IncludedFormats []string        `json:"includedFormats"`
	IndexedDB       json.RawMessage `json:"indexedDB"`
	Cookies         json.RawMessage `json:"cookies"`
}

func isIndexedDBSession(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "{") {
		return false
	}
	var root map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &root) != nil {
		return false
	}
	if idb, ok := root["indexedDB"]; ok && len(idb) > 2 && string(idb) != "null" {
		return true
	}
	if fb, ok := root["firebaseLocalStorageDb"]; ok && len(fb) > 2 && string(fb) != "null" {
		return true
	}
	return false
}

// indexedDBObject accepts both saved shapes:
// the GoAuto export {"referer","indexedDB":{...}} and a bare dump
// {"firebaseLocalStorageDb":{...},"firebase-heartbeat-database":{...}}.
func indexedDBObject(root map[string]interface{}) map[string]interface{} {
	if idb, ok := root["indexedDB"].(map[string]interface{}); ok && len(idb) > 0 {
		return idb
	}
	if _, ok := root["firebaseLocalStorageDb"]; ok {
		return root
	}
	return nil
}

func findSTSTokenManager(root map[string]interface{}) (user map[string]interface{}, stm map[string]interface{}) {
	idb := indexedDBObject(root)
	if idb == nil {
		return nil, nil
	}
	fb, _ := idb["firebaseLocalStorageDb"].(map[string]interface{})
	if fb == nil {
		return nil, nil
	}
	stores, _ := fb["stores"].(map[string]interface{})
	if stores == nil {
		return nil, nil
	}
	rows, _ := stores["firebaseLocalStorage"].([]interface{})
	for _, row := range rows {
		m, _ := row.(map[string]interface{})
		if m == nil {
			continue
		}
		val, _ := m["value"].(map[string]interface{})
		if val == nil {
			continue
		}
		s, _ := val["stsTokenManager"].(map[string]interface{})
		if s != nil {
			return val, s
		}
	}
	return nil, nil
}

func extractFirebaseAccessToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var root map[string]interface{}
	if json.Unmarshal([]byte(raw), &root) != nil {
		return ""
	}
	_, stm := findSTSTokenManager(root)
	if stm == nil {
		return ""
	}
	if tok, ok := stm["accessToken"].(string); ok && tok != "" {
		return tok
	}
	return ""
}

func extractFirebaseExpirationMs(raw string) int64 {
	var root map[string]interface{}
	if json.Unmarshal([]byte(raw), &root) != nil {
		return 0
	}
	_, stm := findSTSTokenManager(root)
	if stm == nil {
		return 0
	}
	switch v := stm["expirationTime"].(type) {
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	case string:
		var n int64
		fmt.Sscanf(v, "%d", &n)
		return n
	}
	return 0
}

func extractFirebaseAPIKey(raw string, user map[string]interface{}) string {
	if user != nil {
		if k, ok := user["apiKey"].(string); ok && strings.HasPrefix(k, "AIza") {
			return k
		}
	}
	re := regexp.MustCompile(`AIza[0-9A-Za-z_-]{20,}`)
	if m := re.FindString(raw); m != "" {
		return m
	}
	return "AIzaSyAhX7hgWsGjY-Lo6eqwJmuRU2xxNRTY7kQ" // Quillbot web app default
}

var (
	firebaseRefreshMu   sync.Mutex
	firebaseLastRefresh time.Time
)

// ensureFreshFirebaseSession refreshes expired (or soon-expiring) Firebase
// access tokens via securetoken.googleapis.com and writes cookie.txt back.
// Without this, Jasper injects a stale accessToken → GraphQL 401 →
// "Something went wrong".
func ensureFreshFirebaseSession(path string) string {
	return ensureFreshFirebaseSessionRaw(loadSessionFileRaw(path), path)
}

func ensureFreshFirebaseSessionRaw(raw, persistPath string) string {
	if !isIndexedDBSession(raw) {
		return raw
	}
	exp := extractFirebaseExpirationMs(raw)
	nowMs := time.Now().UnixMilli()
	// Refresh if missing, already expired, or within 10 minutes of expiry
	if exp > nowMs+10*60*1000 {
		return raw
	}

	firebaseRefreshMu.Lock()
	defer firebaseRefreshMu.Unlock()
	if persistPath != "" {
		raw = loadSessionFileRaw(persistPath)
		exp = extractFirebaseExpirationMs(raw)
		nowMs = time.Now().UnixMilli()
		if exp > nowMs+10*60*1000 {
			return raw
		}
	}
	if time.Since(firebaseLastRefresh) < 30*time.Second && exp > nowMs {
		return raw
	}

	var root map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		log.Printf("[QUILLBOT] refresh: parse cookie.txt failed: %v", err)
		return raw
	}
	user, stm := findSTSTokenManager(root)
	if stm == nil {
		log.Printf("[QUILLBOT] refresh: no stsTokenManager in cookie.txt")
		return raw
	}
	refreshTok, _ := stm["refreshToken"].(string)
	if refreshTok == "" {
		log.Printf("[QUILLBOT] refresh: empty refreshToken")
		return raw
	}
	apiKey := extractFirebaseAPIKey(raw, user)

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshTok)
	req, err := http.NewRequest("POST", "https://securetoken.googleapis.com/v1/token?key="+apiKey, strings.NewReader(form.Encode()))
	if err != nil {
		return raw
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "https://quillbot.com/")
	req.Header.Set("Origin", "https://quillbot.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[QUILLBOT] refresh: request failed: %v", err)
		return raw
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("[QUILLBOT] refresh: status %d body=%s", resp.StatusCode, snippetForLog(body, "application/json"))
		return raw
	}
	var tokResp struct {
		IDToken      string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    string `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokResp); err != nil || tokResp.IDToken == "" {
		log.Printf("[QUILLBOT] refresh: bad response: %v", err)
		return raw
	}
	expiresIn := 3600
	fmt.Sscanf(tokResp.ExpiresIn, "%d", &expiresIn)
	newExp := time.Now().UnixMilli() + int64(expiresIn)*1000

	stm["accessToken"] = tokResp.IDToken
	stm["expirationTime"] = float64(newExp)
	if tokResp.RefreshToken != "" {
		stm["refreshToken"] = tokResp.RefreshToken
	}
	if user != nil {
		if _, ok := user["idToken"]; ok {
			user["idToken"] = tokResp.IDToken
		}
	}

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return raw
	}
	if persistPath != "" {
		if err := os.WriteFile(persistPath, out, 0644); err != nil {
			log.Printf("[QUILLBOT] refresh: write cookie.txt failed: %v", err)
		}
	}
	firebaseLastRefresh = time.Now()
	log.Printf("[QUILLBOT] ✅ Firebase accessToken refreshed (expires in %ds)", expiresIn)
	return string(out)
}

func indexedDBJSONForInject(raw string) string {
	raw = strings.TrimSpace(raw)
	var root map[string]interface{}
	if json.Unmarshal([]byte(raw), &root) != nil {
		return "{}"
	}
	idb := indexedDBObject(root)
	if len(idb) == 0 {
		return "{}"
	}
	b, err := json.Marshal(idb)
	if err != nil {
		return "{}"
	}
	return strings.ReplaceAll(string(b), "</", "<\\/")
}

// buildIndexedDBInjectHTML restores Firebase auth into IndexedDB BEFORE app modules run.
func buildIndexedDBInjectHTML(sessionRaw string) string {
	if !isIndexedDBSession(sessionRaw) {
		return ""
	}
	idbJS := indexedDBJSONForInject(sessionRaw)
	if idbJS == "{}" {
		return ""
	}
	token := extractFirebaseAccessToken(sessionRaw)
	tokenJS, _ := json.Marshal(token)
	tokenTag := ""
	if len(token) > 16 {
		tokenTag = token[len(token)-16:]
	}
	tokenTagJS, _ := json.Marshal(tokenTag)

	return `<script data-tm-idb-restore="1">
(function(){
  var IDB_DUMP = ` + idbJS + `;
  var ACCESS_TOKEN = ` + string(tokenJS) + `;
  var TOKEN_TAG = ` + string(tokenTagJS) + `;
  function openStore(dbName, storeName, keyPath) {
    return new Promise(function(resolve, reject) {
      var timer = setTimeout(function(){ reject(new Error('idb timeout')); }, 2000);
      function done(err, db) {
        clearTimeout(timer);
        if (err) reject(err); else resolve(db);
      }
      var req = indexedDB.open(dbName);
      req.onblocked = function(){ done(new Error('blocked')); };
      req.onerror = function(){ done(req.error || new Error('open failed')); };
      req.onsuccess = function() {
        var db = req.result;
        if (db.objectStoreNames.contains(storeName)) {
          done(null, db);
          return;
        }
        var next = db.version + 1;
        db.close();
        var req2 = indexedDB.open(dbName, next);
        req2.onblocked = function(){ done(new Error('blocked')); };
        req2.onupgradeneeded = function(e2) {
          var d2 = e2.target.result;
          if (!d2.objectStoreNames.contains(storeName)) {
            if (keyPath) d2.createObjectStore(storeName, { keyPath: keyPath });
            else d2.createObjectStore(storeName, { autoIncrement: true });
          }
        };
        req2.onsuccess = function(){ done(null, req2.result); };
        req2.onerror = function(){ done(req2.error || new Error('upgrade failed')); };
      };
    });
  }
  function putAll(db, storeName, rows) {
    return new Promise(function(resolve, reject) {
      var tx = db.transaction(storeName, 'readwrite');
      var store = tx.objectStore(storeName);
      (rows || []).forEach(function(row){ try { store.put(row); } catch (e) {} });
      tx.oncomplete = function(){ resolve(); };
      tx.onerror = function(){ reject(tx.error); };
    });
  }
  window.__TM_IDB_READY = (async function(){
    try {
      if (ACCESS_TOKEN) {
        try { window.__TM_FIREBASE_ACCESS_TOKEN = ACCESS_TOKEN; } catch (e) {}
      }
      // Re-inject when access token rotates (stale sessionStorage caused "Something went wrong")
      try {
        if (TOKEN_TAG && sessionStorage.getItem('__tm_jasper_idb') === TOKEN_TAG) {
          console.log('[Quillbot] IndexedDB already warm');
          return;
        }
      } catch (e) {}
      var dbs = IDB_DUMP || {};
      var jobs = [];
      for (var dbName in dbs) {
        if (!Object.prototype.hasOwnProperty.call(dbs, dbName)) continue;
        var stores = (dbs[dbName] && dbs[dbName].stores) || {};
        for (var storeName in stores) {
          if (!Object.prototype.hasOwnProperty.call(stores, storeName)) continue;
          (function(dbName, storeName, rows){
            var keyPath = null;
            if (rows.length && rows[0] && rows[0].fbase_key != null) keyPath = 'fbase_key';
            jobs.push(openStore(dbName, storeName, keyPath).then(function(db){
              return putAll(db, storeName, rows).then(function(){ try { db.close(); } catch (e) {} });
            }));
          })(dbName, storeName, stores[storeName] || []);
        }
      }
      await Promise.all(jobs);
      try { if (TOKEN_TAG) sessionStorage.setItem('__tm_jasper_idb', TOKEN_TAG); } catch (e) {}
      console.log('[Quillbot] IndexedDB Firebase auth restored');
    } catch (e) {
      console.warn('[Quillbot] IndexedDB restore failed', e);
    }
  })();
})();
</script>`
}

var jasperEntryModuleRe = regexp.MustCompile(`(?i)<script([^>]*\btype=["']module["'][^>]*)\ssrc=["']([^"']*assets/index-[^"']+\.js)["']([^>]*)>\s*</script>`)

// gateJasperModules delays the Vite entry module until IndexedDB restore finishes.
func gateJasperModules(html []byte) []byte {
	if !jasperEntryModuleRe.Match(html) {
		return html
	}
	return jasperEntryModuleRe.ReplaceAllFunc(html, func(m []byte) []byte {
		sub := jasperEntryModuleRe.FindSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		src := string(sub[2])
		repl := fmt.Sprintf(`<script data-tm-module-gate="1">
(async function(){
  try {
    var ready = window.__TM_IDB_READY;
    if (ready && ready.then) {
      await Promise.race([ready, new Promise(function(r){ setTimeout(r, 2500); })]);
    }
  } catch (e) {}
  var s = document.createElement('script');
  s.type = 'module';
  s.src = %q;
  document.head.appendChild(s);
})();
</script>`, src)
		return []byte(repl)
	})
}

func loadSessionFileRaw(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return ""
	}
	return string(data)
}

func logIndexedDBSession(path string) {
	raw := ensureFreshFirebaseSession(path)
	if !isIndexedDBSession(raw) {
		return
	}
	tok := extractFirebaseAccessToken(raw)
	if tok == "" {
		log.Printf("[QUILLBOT] IndexedDB session loaded but no accessToken found")
		return
	}
	exp := extractFirebaseExpirationMs(raw)
	left := (exp - time.Now().UnixMilli()) / 1000
	log.Printf("[QUILLBOT] IndexedDB Firebase session ready (accessToken len=%d, expires_in=%ds)", len(tok), left)
}
