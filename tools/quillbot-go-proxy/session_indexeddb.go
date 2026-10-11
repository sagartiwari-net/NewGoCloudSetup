package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
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

func firebaseDisplayName(raw string) string {
	var root map[string]interface{}
	if json.Unmarshal([]byte(raw), &root) != nil {
		return ""
	}
	user, _ := findSTSTokenManager(root)
	if user == nil {
		return ""
	}
	name, _ := user["displayName"].(string)
	return strings.TrimSpace(name)
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
	// Only Firebase databases belong in the page. A GoAuto export also
	// carries analytics stores and, sometimes, the wrapper keys themselves
	// (referer, includedFormats) which are not IndexedDB databases.
	filtered := make(map[string]interface{}, 2)
	for name, v := range idb {
		if strings.HasPrefix(name, "firebase") {
			filtered[name] = v
		}
	}
	if len(filtered) == 0 {
		return "{}"
	}
	b, err := json.Marshal(filtered)
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
      var timer = setTimeout(function(){ reject(new Error('idb timeout')); }, 5000);
      function done(err, db) {
        clearTimeout(timer);
        if (err) reject(err); else resolve(db);
      }
      function openAtV1() {
        var req = indexedDB.open(dbName, 1);
        req.onblocked = function(){ done(new Error('blocked')); };
        req.onerror = function(){ done(req.error || new Error('open failed')); };
        req.onupgradeneeded = function(e) {
          var d2 = e.target.result;
          if (!d2.objectStoreNames.contains(storeName)) {
            if (keyPath) d2.createObjectStore(storeName, { keyPath: keyPath });
            else d2.createObjectStore(storeName);
          }
        };
        req.onsuccess = function(){ done(null, req.result); };
      }
      // Firebase opens this database at version 1. A higher version makes
      // that open fail and Quillbot paints Log in / Sign up.
      if (dbName === 'firebaseLocalStorageDb') {
        var del = indexedDB.deleteDatabase(dbName);
        del.onblocked = function(){ openExisting(); };
        del.onerror = function(){ openAtV1(); };
        del.onsuccess = function(){ openAtV1(); };
        return;
      }
      function openExisting() {
        var req = indexedDB.open(dbName);
        req.onerror = function(){ done(req.error || new Error('open failed')); };
        req.onsuccess = function(){
          var db = req.result;
          if (db.objectStoreNames.contains(storeName)) { done(null, db); return; }
          db.close();
          openAtV1();
        };
      }
      openAtV1();
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
            }).catch(function(err){
              console.warn('[Quillbot] IndexedDB store failed', dbName, storeName, err);
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

var quillbotScriptRe = regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script>`)
var quillbotAttrRe = regexp.MustCompile(`(?i)([a-zA-Z_:][-a-zA-Z0-9_:.]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+))`)

func scriptAttr(attrs, key string) string {
	for _, m := range quillbotAttrRe.FindAllStringSubmatch(attrs, -1) {
		if !strings.EqualFold(m[1], key) {
			continue
		}
		if strings.Contains(m[0], `="`) {
			return m[2]
		}
		if strings.Contains(m[0], `='`) {
			return m[3]
		}
		return m[4]
	}
	return ""
}

func scriptHasAttr(attrs, key string) bool {
	if scriptAttr(attrs, key) != "" {
		return true
	}
	re := regexp.MustCompile(`(?i)(?:^|[\s/])` + regexp.QuoteMeta(key) + `(?:[\s=/>]|$)`)
	return re.MatchString(attrs)
}

// gateQuillbotBoot keeps Quillbot's own scripts from running until the
// Firebase session is in IndexedDB. The site boots from inline scripts, so
// the Vite-only gate never waits and the app paints logged out.
func gateQuillbotBoot(page []byte) []byte {
	if !bytes.Contains(page, []byte(`data-tm-idb-restore`)) || bytes.Contains(page, []byte(`data-tm-idb-replay`)) {
		return page
	}
	page = quillbotScriptRe.ReplaceAllFunc(page, func(m []byte) []byte {
		sub := quillbotScriptRe.FindSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		attrs := string(sub[1])
		if strings.Contains(attrs, "data-tm-") {
			return m
		}
		typ := strings.ToLower(strings.TrimSpace(scriptAttr(attrs, "type")))
		switch typ {
		case "application/ld+json", "application/json", "text/template", "text/x-template", "importmap":
			return m
		}
		var b strings.Builder
		b.WriteString(`<script type="text/tm-hold" data-tm-hold="1"`)
		if typ != "" {
			b.WriteString(` data-tm-type="`)
			b.WriteString(html.EscapeString(typ))
			b.WriteString(`"`)
		}
		src := scriptAttr(attrs, "src")
		if src != "" {
			b.WriteString(` data-tm-src="`)
			b.WriteString(html.EscapeString(src))
			b.WriteString(`"`)
		}
		if scriptHasAttr(attrs, "async") {
			b.WriteString(` data-tm-async="1"`)
		}
		if scriptHasAttr(attrs, "defer") {
			b.WriteString(` data-tm-defer="1"`)
		}
		if scriptHasAttr(attrs, "nomodule") {
			b.WriteString(` data-tm-nomodule="1"`)
		}
		if cross := scriptAttr(attrs, "crossorigin"); cross != "" {
			b.WriteString(` data-tm-crossorigin="`)
			b.WriteString(html.EscapeString(cross))
			b.WriteString(`"`)
		}
		b.WriteString(`>`)
		if src == "" {
			b.Write(sub[2])
		}
		b.WriteString(`</script>`)
		return []byte(b.String())
	})
	replay := []byte(quillbotReplayScript)
	if i := bytes.LastIndex(bytes.ToLower(page), []byte("</body>")); i >= 0 {
		out := make([]byte, 0, len(page)+len(replay))
		out = append(out, page[:i]...)
		out = append(out, replay...)
		out = append(out, page[i:]...)
		return out
	}
	return append(page, replay...)
}

const quillbotReplayScript = `<script data-tm-idb-replay="1">
(async function(){
  try {
    var ready = window.__TM_IDB_READY;
    if (ready && ready.then) {
      await Promise.race([ready, new Promise(function(r){ setTimeout(r, 8000); })]);
    }
  } catch (e) {}
  var list = Array.prototype.slice.call(document.querySelectorAll('script[data-tm-hold="1"]'));
  for (var i = 0; i < list.length; i++) {
    var old = list[i];
    if (!old.parentNode) continue;
    var s = document.createElement('script');
    var typ = old.getAttribute('data-tm-type');
    if (typ) s.type = typ;
    if (old.getAttribute('data-tm-nomodule') === '1') s.noModule = true;
    var cross = old.getAttribute('data-tm-crossorigin');
    if (cross) s.crossOrigin = cross;
    var src = old.getAttribute('data-tm-src');
    if (src) {
      if (old.getAttribute('data-tm-async') === '1') s.async = true;
      s.src = src;
      old.parentNode.insertBefore(s, old.nextSibling);
      if (old.getAttribute('data-tm-async') !== '1') {
        await new Promise(function(res){ s.onload = s.onerror = function(){ res(); }; });
      }
    } else {
      s.text = old.text || old.textContent || '';
      old.parentNode.insertBefore(s, old.nextSibling);
    }
  }
})();
</script>`

func loadSessionFileRaw(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return ""
	}
	return string(data)
}

// Quillbot sets cookies on the parent of location.hostname. On
// quillbot.gt4rents.com that is .gt4rents.com, so the panel request
// carries them and Chrome shows "This page couldn't load".
func injectQuillbotCookieScope(body []byte) []byte {
	const script = `<script data-tm-cookie-scope>(function(){var proto=Document.prototype;var desc=Object.getOwnPropertyDescriptor(proto,"cookie");if(!desc||!desc.set||!desc.get)return;Object.defineProperty(proto,"cookie",{configurable:true,get:function(){return desc.get.call(this);},set:function(v){v=String(v).replace(/;\s*domain=\.?gt4rents\.com/ig,"");return desc.set.call(this,v);}});})();</script>`
	if bytes.Contains(body, []byte("data-tm-cookie-scope")) {
		return body
	}
	reHead := regexp.MustCompile(`(?i)<head[^>]*>`)
	if loc := reHead.FindIndex(body); loc != nil {
		out := make([]byte, 0, len(body)+len(script))
		out = append(out, body[:loc[1]]...)
		out = append(out, script...)
		out = append(out, body[loc[1]:]...)
		return out
	}
	return append([]byte(script), body...)
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
