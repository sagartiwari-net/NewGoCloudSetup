package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"html"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type qbLogoutHit struct {
	at       time.Time
	next     string
	switched bool
}

var (
	qbLogoutMu   sync.Mutex
	qbLogoutSeen = map[int]qbLogoutHit{}
	qbSessionHop = map[string]time.Time{}
)

// noteQuillbotLogout records Analytics → Logouts and moves the session to
// another active account once. Account status is not changed. A second hit
// in the same session does not switch again.
func noteQuillbotLogout(cfg Config, r *http.Request, acc ToolAccount, sessionToken, reason string) (switched bool, next string) {
	if !usesPanelAccountMode(cfg) || acc.ID <= 0 {
		return false, ""
	}
	key := sessionToken
	if key == "" {
		key = "acc:" + acc.Name
	}
	qbLogoutMu.Lock()
	defer qbLogoutMu.Unlock()
	if prev, seen := qbSessionHop[key]; seen && time.Since(prev) < 2*time.Minute {
		last := qbLogoutSeen[acc.ID]
		if time.Since(prev) < 2*time.Second {
			return last.switched, last.next
		}
		return false, last.next
	}
	qbSessionHop[key] = time.Now()

	db, err := openPanelDB(cfg)
	if err != nil {
		log.Printf("[LB] logout db open failed: %v", err)
		delete(qbSessionHop, key)
		return false, ""
	}
	now := time.Now().UTC().Format(time.RFC3339)
	nextName := "(none)"
	skipIDs := []any{cfg.PublicHost, acc.ID}
	placeholders := "?"
	for id, hit := range qbLogoutSeen {
		if id == acc.ID || time.Since(hit.at) >= 10*time.Minute {
			continue
		}
		skipIDs = append(skipIDs, id)
		placeholders += ",?"
	}
	if other, err := scanPanelAccount(db.QueryRow(panelAccountSelect+`
		WHERE w.domain = ? AND a.status = 'active' AND a.cookie != '' AND a.id NOT IN (`+placeholders+`)
		`+panelAccountOrder+` LIMIT 1`, skipIDs...)); err == nil {
		_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, other.ID)
		if sessionToken != "" {
			_, _ = db.Exec(`UPDATE live_sessions SET assigned_account_id=? WHERE session_token=?`, other.ID, sessionToken)
		}
		rememberPanelAccount(sessionToken, other)
		nextName = other.Name
		switched = true
		log.Printf("[LB] switched %s -> %s (status unchanged)", acc.Name, other.Name)
	} else {
		forgetPanelAccount(sessionToken)
	}

	websiteID, username, loginIP := quillbotLogoutMeta(db, cfg, sessionToken)
	ip := ""
	if r != nil {
		ip = realClientIP(r)
	}
	if ip == "" || ip == "127.0.0.1" || ip == "::1" {
		if loginIP != "" && loginIP != "127.0.0.1" && loginIP != "::1" {
			ip = loginIP
		}
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "logged_out"
	}
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS logout_events (
		id INTEGER PRIMARY KEY,
		website_id INTEGER NOT NULL,
		username TEXT NOT NULL DEFAULT '',
		account_name TEXT NOT NULL DEFAULT '',
		next_account_name TEXT NOT NULL DEFAULT '',
		reason TEXT NOT NULL DEFAULT '',
		client_ip TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	)`)
	if _, err = db.Exec(`INSERT INTO logout_events (website_id, username, account_name, next_account_name, reason, client_ip, created_at) VALUES (?,?,?,?,?,?,?)`,
		websiteID, username, acc.Name, nextName, reason, ip, now); err != nil {
		log.Printf("[LB] logout_events insert failed: %v", err)
	} else {
		log.Printf("[LB] logout recorded account=%s next=%s user=%s ip=%s reason=%s", acc.Name, nextName, username, ip, reason)
	}
	qbLogoutSeen[acc.ID] = qbLogoutHit{at: time.Now(), next: nextName, switched: switched}
	return switched, nextName
}

func quillbotLogoutMeta(db *sql.DB, cfg Config, sessionToken string) (websiteID int, username, loginIP string) {
	if sessionToken != "" {
		_ = db.QueryRow(`SELECT website_id, username, COALESCE(client_ip, '') FROM live_sessions WHERE session_token=?`, sessionToken).Scan(&websiteID, &username, &loginIP)
	}
	if websiteID <= 0 {
		_ = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID)
	}
	return websiteID, username, loginIP
}

func quillbotLogoutHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmtErrorJSON(w, "method")
		return
	}
	if !usesPanelAccountMode(cfg) {
		w.WriteHeader(http.StatusNotFound)
		fmtErrorJSON(w, "unavailable")
		return
	}
	cookie, err := r.Cookie("ct_session")
	if err != nil || cookie.Value == "" {
		w.WriteHeader(http.StatusUnauthorized)
		fmtErrorJSON(w, "unauthorized")
		return
	}
	if _, err = panelSessionUsername(r); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		fmtErrorJSON(w, "unauthorized")
		return
	}
	acc, err := loadPanelSessionAccount(cfg, cookie.Value)
	if err != nil {
		writeQuillbotContactAdmin(w, cfg)
		return
	}
	switched, _ := noteQuillbotLogout(cfg, r, acc, cookie.Value, "logged_out")
	_ = json.NewEncoder(w).Encode(map[string]bool{"switched": switched})
}

func fmtErrorJSON(w http.ResponseWriter, msg string) {
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeQuillbotContactAdmin(w http.ResponseWriter, cfg Config) {
	name := html.EscapeString(toolDisplayName(cfg))
	writeLightCard(w, http.StatusOK, lightCard{
		Title:   "Temporarily Unavailable",
		Heading: "Temporarily Unavailable",
		Message: "<span class=\"brand\">" + name + "</span> is temporarily unavailable. Please try again in a few minutes.",
		Footer:  "Open this tool again from your access link",
	})
}

func serveQuillbotContactAdmin(w http.ResponseWriter, r *http.Request) {
	writeQuillbotContactAdmin(w, loadConfig())
}

func injectQuillbotLogoutWatch(body []byte, displayName string) []byte {
	if bytes.Contains(body, []byte("data-tm-qb-watch")) {
		return body
	}
	nameJS, _ := json.Marshal(displayName)
	script := `<script data-tm-qb-watch="1">
(function(){
  var NAME = ` + string(nameJS) + `;
  var sent = false;
  function send() {
    if (sent) return;
    sent = true;
    fetch('/api/qb-logout', {method:'POST', credentials:'same-origin', headers:{'Content-Type':'application/json'}, body:'{}'})
      .then(function(r){ return r.json(); })
      .then(function(j){
        if (j && j.switched) { location.reload(); return; }
        location.replace('/__tm_contact_admin');
      })
      .catch(function(){});
  }
  setTimeout(function(){
    var text = '';
    try { text = document.body ? (document.body.innerText || '') : ''; } catch (e) {}
    if (!text || text.length < 80) return;
    if (text.indexOf('Just a moment') !== -1 || text.indexOf('Performing security verification') !== -1) return;
    if (NAME && text.indexOf(NAME) !== -1) return;
    var uiOut = text.indexOf('Log in / Sign up') !== -1;
    if (!uiOut) return;
    var req;
    try { req = indexedDB.open('firebaseLocalStorageDb'); } catch (e) { send(); return; }
    var timer = setTimeout(function(){}, 2000);
    req.onerror = function(){ clearTimeout(timer); send(); };
    req.onsuccess = function(){
      clearTimeout(timer);
      var db = req.result;
      if (!db.objectStoreNames.contains('firebaseLocalStorage')) {
        try { db.close(); } catch (e) {}
        send();
        return;
      }
      var tx = db.transaction('firebaseLocalStorage', 'readonly');
      var g = tx.objectStore('firebaseLocalStorage').getAll();
      g.onerror = function(){ try { db.close(); } catch (e) {} send(); };
      g.onsuccess = function(){
        var ok = false;
        var rows = g.result || [];
        for (var i = 0; i < rows.length; i++) {
          var v = rows[i] && rows[i].value;
          if (v && v.stsTokenManager && v.stsTokenManager.accessToken) ok = true;
        }
        try { db.close(); } catch (e) {}
        if (ok) return;
        var now = '';
        try { now = document.body ? (document.body.innerText || '') : ''; } catch (e2) {}
        if (NAME && now.indexOf(NAME) !== -1) return;
        if (now.indexOf('Log in / Sign up') !== -1) send();
      };
    };
  }, 10000);
})();
</script>`
	raw := []byte(script)
	if i := bytes.LastIndex(bytes.ToLower(body), []byte("</body>")); i >= 0 {
		out := make([]byte, 0, len(body)+len(raw))
		out = append(out, body[:i]...)
		out = append(out, raw...)
		out = append(out, body[i:]...)
		return out
	}
	return append(body, raw...)
}
