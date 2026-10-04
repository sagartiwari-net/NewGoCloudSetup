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
	var probe struct {
		IndexedDB json.RawMessage `json:"indexedDB"`
	}
	if json.Unmarshal([]byte(raw), &probe) != nil {
		return false
	}
	return len(probe.IndexedDB) > 2
}

func findSTSTokenManager(root map[string]interface{}) (user map[string]interface{}, stm map[string]interface{}) {
	idb, _ := root["indexedDB"].(map[string]interface{})
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
	return "AIzaSyCawBlyCbiRm8cyy_iSMwQCSi-eYQAlZ-E" // Jasper web app default
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
	raw := loadSessionFileRaw(path)
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
	// Re-read under lock (another goroutine may have refreshed)
	raw = loadSessionFileRaw(path)
	exp = extractFirebaseExpirationMs(raw)
	nowMs = time.Now().UnixMilli()
	if exp > nowMs+10*60*1000 {
		return raw
	}
	if time.Since(firebaseLastRefresh) < 30*time.Second && exp > nowMs {
		return raw
	}

	var root map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		log.Printf("[JASPER] refresh: parse cookie.txt failed: %v", err)
		return raw
	}
	user, stm := findSTSTokenManager(root)
	if stm == nil {
		log.Printf("[JASPER] refresh: no stsTokenManager in cookie.txt")
		return raw
	}
	refreshTok, _ := stm["refreshToken"].(string)
	if refreshTok == "" {
		log.Printf("[JASPER] refresh: empty refreshToken")
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
	req.Header.Set("Referer", "https://app.jasper.ai/")
	req.Header.Set("Origin", "https://app.jasper.ai")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[JASPER] refresh: request failed: %v", err)
		return raw
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("[JASPER] refresh: status %d body=%s", resp.StatusCode, snippetForLog(body, "application/json"))
		return raw
	}
	var tokResp struct {
		IDToken      string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    string `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokResp); err != nil || tokResp.IDToken == "" {
		log.Printf("[JASPER] refresh: bad response: %v", err)
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
	if err := os.WriteFile(path, out, 0644); err != nil {
		log.Printf("[JASPER] refresh: write cookie.txt failed: %v", err)
		return string(out) // still use in-memory fresh copy
	}
	firebaseLastRefresh = time.Now()
	log.Printf("[JASPER] ✅ Firebase accessToken refreshed (expires in %ds)", expiresIn)
	return string(out)
}

func indexedDBJSONForInject(raw string) string {
	raw = strings.TrimSpace(raw)
	var exp goAutoIndexedDBExport
	if json.Unmarshal([]byte(raw), &exp) != nil || len(exp.IndexedDB) < 3 {
		return "{}"
	}
	var compact json.RawMessage
	if json.Unmarshal(exp.IndexedDB, &compact) != nil {
		return "{}"
	}
	b, err := json.Marshal(compact)
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
      var req = indexedDB.open(dbName, 1);
      req.onupgradeneeded = function(e) {
        var db = e.target.result;
        if (!db.objectStoreNames.contains(storeName)) {
          if (keyPath) db.createObjectStore(storeName, { keyPath: keyPath });
          else db.createObjectStore(storeName, { autoIncrement: true });
        }
      };
      req.onsuccess = function() {
        var db = req.result;
        if (!db.objectStoreNames.contains(storeName)) {
          db.close();
          var v = db.version + 1;
          var req2 = indexedDB.open(dbName, v);
          req2.onupgradeneeded = function(e2) {
            var d2 = e2.target.result;
            if (!d2.objectStoreNames.contains(storeName)) {
              if (keyPath) d2.createObjectStore(storeName, { keyPath: keyPath });
              else d2.createObjectStore(storeName, { autoIncrement: true });
            }
          };
          req2.onsuccess = function(){ resolve(req2.result); };
          req2.onerror = function(){ reject(req2.error); };
          return;
        }
        resolve(db);
      };
      req.onerror = function(){ reject(req.error); };
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
          console.log('[Jasper] IndexedDB already warm');
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
      console.log('[Jasper] IndexedDB Firebase auth restored');
    } catch (e) {
      console.warn('[Jasper] IndexedDB restore failed', e);
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
  try { if (window.__TM_IDB_READY) await window.__TM_IDB_READY; } catch (e) {}
  var s = document.createElement('script');
  s.type = 'module';
  s.crossOrigin = '';
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
		log.Printf("[JASPER] IndexedDB session loaded but no accessToken found")
		return
	}
	exp := extractFirebaseExpirationMs(raw)
	left := (exp - time.Now().UnixMilli()) / 1000
	log.Printf("[JASPER] IndexedDB Firebase session ready (accessToken len=%d, expires_in=%ds)", len(tok), left)
}
