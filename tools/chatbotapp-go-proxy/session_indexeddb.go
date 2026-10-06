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

// GoAuto-style export with IndexedDB (Cramly / Firebase).
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

func extractFirebaseUID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var root map[string]interface{}
	if json.Unmarshal([]byte(raw), &root) != nil {
		return ""
	}
	user, _ := findSTSTokenManager(root)
	if user == nil {
		return ""
	}
	if uid, ok := user["uid"].(string); ok && uid != "" {
		return uid
	}
	if uid, ok := user["localId"].(string); ok && uid != "" {
		return uid
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
	return ""
}

var (
	firebaseRefreshMu   sync.Mutex
	firebaseLastRefresh time.Time
)

// ensureFreshFirebaseSession refreshes expired (or soon-expiring) Firebase
// access tokens via securetoken.googleapis.com and writes cookie.txt back.
func ensureFreshFirebaseSession(path string) string {
	return ensureFreshFirebaseSessionRaw(loadSessionFileRaw(path), path)
}

// ensureFreshFirebaseSessionRaw refreshes from an in-memory GoAuto dump.
// When persistPath is non-empty, writes the refreshed JSON back to that file.
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
		log.Printf("[CHATBOT] refresh: parse session failed: %v", err)
		return raw
	}
	user, stm := findSTSTokenManager(root)
	if stm == nil {
		log.Printf("[CHATBOT] refresh: no stsTokenManager in session")
		return raw
	}
	refreshTok, _ := stm["refreshToken"].(string)
	if refreshTok == "" {
		log.Printf("[CHATBOT] refresh: empty refreshToken")
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
	req.Header.Set("Referer", "https://chat.chatbotapp.ai/")
	req.Header.Set("Origin", "https://chat.chatbotapp.ai")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[CHATBOT] refresh: request failed: %v", err)
		return raw
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("[CHATBOT] refresh: status %d body=%s", resp.StatusCode, snippetForLog(body, "application/json"))
		return raw
	}
	var tokResp struct {
		IDToken      string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    string `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokResp); err != nil || tokResp.IDToken == "" {
		log.Printf("[CHATBOT] refresh: bad response: %v", err)
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
	firebaseLastRefresh = time.Now()
	if persistPath != "" {
		if err := os.WriteFile(persistPath, out, 0644); err != nil {
			log.Printf("[CHATBOT] refresh: write cookie.txt failed: %v", err)
		}
	}
	log.Printf("[CHATBOT] ✅ Firebase accessToken refreshed (expires in %ds)", expiresIn)
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
	uid := extractFirebaseUID(sessionRaw)
	tokenJS, _ := json.Marshal(token)
	uidJS, _ := json.Marshal(uid)
	tokenTag := ""
	if len(token) > 16 {
		tokenTag = token[len(token)-16:]
	}
	tokenTagJS, _ := json.Marshal(tokenTag)

	return `<script data-tm-idb-restore="1">
(function(){
  var IDB_DUMP = ` + idbJS + `;
  var ACCESS_TOKEN = ` + string(tokenJS) + `;
  var USER_UID = ` + string(uidJS) + `;
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
  // Nginx drops underscore headers (x_token). Send hyphenated X-Cba-* so the
  // Go proxy can map them onto x_token for api.chatbotapp.ai.
  function attachCbaHeaders(headers) {
    var tok = window.__TM_FIREBASE_ACCESS_TOKEN || ACCESS_TOKEN || '';
    var uid = window.__TM_FIREBASE_UID || USER_UID || '';
    if (!tok) return headers;
    try {
      if (typeof Headers !== 'undefined' && headers instanceof Headers) {
        if (!headers.has('X-Cba-Token')) headers.set('X-Cba-Token', tok);
        if (uid && !headers.has('X-Cba-Uid')) headers.set('X-Cba-Uid', uid);
        return headers;
      }
    } catch (e0) {}
    headers = headers || {};
    if (!headers['X-Cba-Token'] && !headers['x-cba-token']) headers['X-Cba-Token'] = tok;
    if (uid && !headers['X-Cba-Uid'] && !headers['x-cba-uid']) headers['X-Cba-Uid'] = uid;
    return headers;
  }
  try {
    if (window.fetch && !window.fetch.__tmCba) {
      var _f = window.fetch.bind(window);
      window.fetch = function(input, init) {
        init = init || {};
        try {
          var u = typeof input === 'string' ? input : (input && input.url) || '';
          if (/extra-cdn-\d+|api\.chatbotapp\.ai/i.test(String(u))) {
            init.headers = attachCbaHeaders(init.headers);
          }
        } catch (e1) {}
        return _f(input, init);
      };
      window.fetch.__tmCba = true;
    }
    var xo = XMLHttpRequest.prototype.open;
    var xs = XMLHttpRequest.prototype.setRequestHeader;
    if (xo && !xo.__tmCba) {
      XMLHttpRequest.prototype.open = function(method, url) {
        this.__tmCbaURL = String(url || '');
        return xo.apply(this, arguments);
      };
      XMLHttpRequest.prototype.open.__tmCba = true;
      XMLHttpRequest.prototype.setRequestHeader = function(k, v) {
        return xs.apply(this, arguments);
      };
      var xsend = XMLHttpRequest.prototype.send;
      XMLHttpRequest.prototype.send = function() {
        try {
          if (/extra-cdn-\d+|api\.chatbotapp\.ai/i.test(this.__tmCbaURL || '')) {
            var tok = window.__TM_FIREBASE_ACCESS_TOKEN || ACCESS_TOKEN || '';
            var uid = window.__TM_FIREBASE_UID || USER_UID || '';
            if (tok) { try { xs.call(this, 'X-Cba-Token', tok); } catch (e2) {} }
            if (uid) { try { xs.call(this, 'X-Cba-Uid', uid); } catch (e3) {} }
          }
        } catch (e4) {}
        return xsend.apply(this, arguments);
      };
    }
  } catch (ePatch) {}
  window.__TM_IDB_READY = (async function(){
    try {
      if (ACCESS_TOKEN) {
        try { window.__TM_FIREBASE_ACCESS_TOKEN = ACCESS_TOKEN; } catch (e) {}
      }
      if (USER_UID) {
        try { window.__TM_FIREBASE_UID = USER_UID; } catch (e) {}
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
            }));
          })(dbName, storeName, stores[storeName] || []);
        }
      }
      await Promise.all(jobs);
      try { if (TOKEN_TAG) sessionStorage.setItem('__tm_chatbot_idb', TOKEN_TAG); } catch (e) {}
      console.log('[Chatbot] IndexedDB Firebase auth restored');
    } catch (e) {
      console.warn('[Chatbot] IndexedDB restore failed', e);
    }
  })();
})();
</script>`
}

var nextBootScriptRe = regexp.MustCompile(`(?i)<script\b[^>]*\bsrc=["'](/_next/static/[^"']+)["'][^>]*>\s*</script>`)

// gateChatbotNextScripts holds Next.js boot until Firebase auth is in IndexedDB.
// Those chunk tags are async, so they otherwise start before the restore write
// finishes and the app boots signed out (empty chat list, failed API calls).
func gateChatbotNextScripts(html []byte) []byte {
	if !nextBootScriptRe.Match(html) {
		return html
	}
	return nextBootScriptRe.ReplaceAllFunc(html, func(m []byte) []byte {
		sub := nextBootScriptRe.FindSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		src := string(sub[1])
		return []byte(fmt.Sprintf(`<link rel="preload" as="script" href="%s"><script data-tm-next-gate="1">
(async function(){
  try { if (window.__TM_IDB_READY) await window.__TM_IDB_READY; } catch (e) {}
  var s = document.createElement('script');
  s.async = true;
  s.src = %q;
  (document.head || document.documentElement).appendChild(s);
})();
</script>`, src, src))
	})
}

var cramlyBootScriptRe = regexp.MustCompile(`(?i)<script\b([^>]*\bsrc=["']([^"']+\.js)["'][^>]*)>\s*</script>`)

// gateCramlyModules delays Angular boot bundles until IndexedDB restore finishes.
func gateCramlyModules(html []byte) []byte {
	if !cramlyBootScriptRe.Match(html) {
		return html
	}
	return cramlyBootScriptRe.ReplaceAllFunc(html, func(m []byte) []byte {
		sub := cramlyBootScriptRe.FindSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		attrs := strings.ToLower(string(sub[1]))
		src := string(sub[2])
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "//") {
			return m
		}
		base := src
		if i := strings.LastIndex(src, "/"); i >= 0 {
			base = src[i+1:]
		}
		isBoot := strings.HasPrefix(base, "runtime.") ||
			strings.HasPrefix(base, "polyfills.") ||
			strings.HasPrefix(base, "main.") ||
			strings.HasPrefix(base, "scripts.") ||
			strings.HasPrefix(base, "index.") ||
			strings.Contains(src, "/assets/")
		if !isBoot {
			return m
		}
		isModule := strings.Contains(attrs, `type="module"`) || strings.Contains(attrs, `type='module'`)
		isDefer := strings.Contains(attrs, "defer")
		typeLine := ""
		if isModule {
			typeLine = "  s.type = 'module';\n"
		}
		deferLine := ""
		if isDefer {
			deferLine = "  s.defer = true;\n"
		}
		repl := fmt.Sprintf(`<script data-tm-module-gate="1">
(async function(){
  try { if (window.__TM_IDB_READY) await window.__TM_IDB_READY; } catch (e) {}
  var s = document.createElement('script');
%s%s  s.src = %q;
  (document.body || document.head || document.documentElement).appendChild(s);
})();
</script>`, typeLine, deferLine, src)
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

// ensureChatbotappAPIAuth forces official Origin/Referer and the Firebase
// headers Chatbot App's Express API expects: x_token + x_user_id (+ platform).
// Authorization Bearer alone is NOT enough — upstream returns
// error_code 4002 "x_token header is required".
//
// Auth source = GoAuto IndexedDB firebaseLocalStorage (stsTokenManager.accessToken).
// Analytics cookies (_ga etc.) are NOT used for /api/v2/*. Nginx may drop
// underscore headers from the browser, so we inject from panel IndexedDB and
// also accept hyphenated X-Cba-Token (client patch) as a fallback.
func ensureChatbotappAPIAuth(upstreamReq *http.Request, cfg Config, activeAcc *ToolAccount, upstreamHost string) {
	if upstreamReq == nil || !strings.Contains(strings.ToLower(cfg.TargetURL), "chatbotapp.ai") {
		return
	}
	h := strings.ToLower(strings.TrimSpace(upstreamHost))
	if h == "" {
		h = strings.ToLower(upstreamReq.URL.Host)
	}
	if h == "" {
		h = strings.ToLower(upstreamReq.Host)
	}
	path := upstreamReq.URL.Path
	isChatbotAPI := strings.Contains(h, "api.chatbotapp.ai") ||
		strings.Contains(h, "payment-api.chatbotapp.ai") ||
		strings.Contains(h, "data.chatbotapp.ai") ||
		strings.Contains(h, "event.chatbotapp.ai") ||
		((strings.HasPrefix(path, "/api/v2/") || strings.HasPrefix(path, "/api/v1/")) && strings.Contains(h, "chatbotapp.ai"))
	isFirebase := strings.Contains(h, "googleapis.com") ||
		strings.Contains(h, "firebaseio.com") ||
		strings.Contains(h, "firebaseapp.com") ||
		strings.Contains(h, "firestore.googleapis.com")
	if !isChatbotAPI && !isFirebase && !strings.Contains(h, "chatbotapp.ai") && !strings.Contains(h, "chatbot.app") {
		return
	}
	upstreamReq.Header.Set("Origin", "https://chat.chatbotapp.ai")
	upstreamReq.Header.Set("Referer", "https://chat.chatbotapp.ai/")
	if !isChatbotAPI {
		return
	}
	if activeAcc == nil {
		log.Printf("[CHATBOT] api auth SKIP path=%s host=%s reason=no_account", path, h)
		return
	}
	raw := strings.TrimSpace(activeAcc.Cookie)
	if !isIndexedDBSession(raw) {
		log.Printf("[CHATBOT] api auth SKIP path=%s account=%s(%d) reason=no_indexeddb (paste GoAuto IndexedDB dump)", path, activeAcc.Name, activeAcc.ID)
		// Still try browser-forwarded hyphen fallback
		if tok := strings.TrimSpace(upstreamReq.Header.Get("X-Cba-Token")); tok != "" {
			uid := strings.TrimSpace(upstreamReq.Header.Get("X-Cba-Uid"))
			applyChatbotappTokenHeaders(upstreamReq, tok, uid)
			log.Printf("[CHATBOT] api auth FALLBACK X-Cba-Token path=%s tok_len=%d", path, len(tok))
		}
		return
	}
	fresh := ensureFreshFirebaseSessionRaw(raw, "")
	if fresh != "" && fresh != raw {
		activeAcc.Cookie = fresh
		persistPanelAccountCookie(cfg, activeAcc.ID, fresh)
		raw = fresh
		log.Printf("[CHATBOT] Firebase accessToken refreshed for account=%s(%d)", activeAcc.Name, activeAcc.ID)
	}
	tok := extractFirebaseAccessToken(raw)
	uid := extractFirebaseUID(raw)
	if tok == "" {
		tok = strings.TrimSpace(upstreamReq.Header.Get("X-Cba-Token"))
	}
	if uid == "" {
		uid = strings.TrimSpace(upstreamReq.Header.Get("X-Cba-Uid"))
	}
	if tok == "" {
		log.Printf("[CHATBOT] api auth SKIP path=%s account=%s(%d) reason=empty_accessToken", path, activeAcc.Name, activeAcc.ID)
		return
	}
	applyChatbotappTokenHeaders(upstreamReq, tok, uid)
	log.Printf("[CHATBOT] api auth OK path=%s account=%s tok_len=%d uid=%s", path, activeAcc.Name, len(tok), uid)
}

func applyChatbotappTokenHeaders(req *http.Request, tok, uid string) {
	if req == nil || tok == "" {
		return
	}
	// Underscore names are what Express checks (x_token ≠ x-token).
	setUnderscoreHeader(req, "x_token", tok)
	setUnderscoreHeader(req, "X_Token", tok)
	setUnderscoreHeader(req, "x_platform", "web")
	setUnderscoreHeader(req, "X_Platform", "web")
	if uid != "" {
		setUnderscoreHeader(req, "x_user_id", uid)
		setUnderscoreHeader(req, "X_User_Id", uid)
		req.Header.Set("X-User-Id", uid)
	}
	req.Header.Set("X-Platform", "web")
	req.Header.Set("Authorization", "Bearer "+tok)
}

func setUnderscoreHeader(req *http.Request, key, value string) {
	if req == nil || key == "" || value == "" {
		return
	}
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header[key] = []string{value}
}

func logIndexedDBSession(path string) {
	raw := ensureFreshFirebaseSession(path)
	if !isIndexedDBSession(raw) {
		return
	}
	tok := extractFirebaseAccessToken(raw)
	if tok == "" {
		log.Printf("[CHATBOT] IndexedDB session loaded but no accessToken found")
		return
	}
	exp := extractFirebaseExpirationMs(raw)
	left := (exp - time.Now().UnixMilli()) / 1000
	log.Printf("[CHATBOT] IndexedDB Firebase session ready (accessToken len=%d, expires_in=%ds)", len(tok), left)
}
