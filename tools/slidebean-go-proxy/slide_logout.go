package main

import (
	"database/sql"
	"html"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type slideLogoutHit struct {
	at       time.Time
	next     string
	switched bool
}

var (
	slideLogoutMu   sync.Mutex
	slideLogoutSeen = map[int]slideLogoutHit{}
	slideSessionHop = map[string]time.Time{}
)

func slideLogoutPath(path string) bool {
	p := strings.ToLower(path)
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	p = strings.TrimSuffix(p, "/")
	switch {
	case strings.Contains(p, "/signin"),
		strings.Contains(p, "/sign-in"),
		strings.Contains(p, "/login"),
		strings.Contains(p, "/signup"),
		strings.Contains(p, "/sign-up"),
		strings.Contains(p, "/logout"),
		strings.Contains(p, "/register"),
		strings.Contains(p, "/forgot"):
		return true
	}
	return false
}

func slideLoggedOutHTML(body []byte) bool {
	s := strings.ToLower(string(body))
	return strings.Contains(s, "welcome back") && strings.Contains(s, "let me in") && strings.Contains(s, "sign in with google")
}

func slideUpstreamLogout(resp *http.Response) bool {
	if resp == nil || resp.StatusCode < 300 || resp.StatusCode > 399 {
		return false
	}
	loc := strings.ToLower(resp.Header.Get("Location"))
	return strings.Contains(loc, "/signin") || strings.Contains(loc, "/sign-in") || strings.Contains(loc, "/login") || strings.Contains(loc, "/signup") || strings.Contains(loc, "/sign-up")
}

func noteSlideLogout(cfg Config, r *http.Request, acc ToolAccount, sessionToken, reason string) (switched bool, next string) {
	if !usesPanelAccountMode(cfg) || acc.ID <= 0 {
		return false, ""
	}
	key := sessionToken
	if key == "" {
		key = "acc:" + acc.Name
	}
	slideLogoutMu.Lock()
	defer slideLogoutMu.Unlock()
	if prev, seen := slideSessionHop[key]; seen && time.Since(prev) < 2*time.Minute {
		last := slideLogoutSeen[acc.ID]
		if time.Since(prev) < 2*time.Second {
			return last.switched, last.next
		}
		return false, last.next
	}
	slideSessionHop[key] = time.Now()

	db, err := openPanelDB(cfg)
	if err != nil {
		log.Printf("[LB] logout db open failed: %v", err)
		delete(slideSessionHop, key)
		return false, ""
	}
	now := time.Now().UTC().Format(time.RFC3339)
	nextName := "(none)"
	skipIDs := []any{cfg.PublicHost, acc.ID}
	placeholders := "?"
	for id, hit := range slideLogoutSeen {
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
		nextName = other.Name
		switched = true
		log.Printf("[LB] switched %s -> %s (status unchanged)", acc.Name, other.Name)
	}

	websiteID, username, loginIP := slideLogoutMeta(db, cfg, sessionToken)
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
		reason = "session_expired"
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
	slideLogoutSeen[acc.ID] = slideLogoutHit{at: time.Now(), next: nextName, switched: switched}
	return switched, nextName
}

func slideLogoutMeta(db *sql.DB, cfg Config, sessionToken string) (websiteID int, username, loginIP string) {
	if sessionToken != "" {
		_ = db.QueryRow(`SELECT website_id, username, COALESCE(client_ip, '') FROM live_sessions WHERE session_token=?`, sessionToken).Scan(&websiteID, &username, &loginIP)
	}
	if websiteID <= 0 {
		_ = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID)
	}
	return websiteID, username, loginIP
}

func slideHome(cfg Config) string {
	if cfg.HomePath != "" {
		return cfg.HomePath
	}
	return "/templates"
}

func serveSlideLogout(w http.ResponseWriter, r *http.Request, cfg Config, acc ToolAccount, sessionToken, reason string) {
	for k := range w.Header() {
		w.Header().Del(k)
	}
	switched, _ := noteSlideLogout(cfg, r, acc, sessionToken, reason)
	log.Printf("[LB] expired session path=%s account=%s switched=%v", r.URL.Path, acc.Name, switched)
	if switched {
		http.Redirect(w, r, slideHome(cfg), http.StatusFound)
		return
	}
	name := html.EscapeString(toolDisplayName(cfg))
	writeLightCard(w, http.StatusOK, lightCard{
		Title:   "Contact admin",
		Heading: "Contact admin",
		Message: "The <span class=\"brand\">" + name + "</span> session ended and no other account is available. Contact Admin/Provider.",
		Footer:  "Saved in panel Analytics → Logouts. Account status was not changed.",
	})
}

func finishSlideUpstreamLogout(w http.ResponseWriter, r *http.Request, resp *http.Response, cfg Config, acc ToolAccount, sessionToken string) {
	if resp != nil && resp.Body != nil {
		io.Copy(io.Discard, resp.Body)
	}
	serveSlideLogout(w, r, cfg, acc, sessionToken, "session_expired")
}

func slideLoginWatchHTML() string {
	return `<script data-tm-slide-login="1">
(function(){
  function loggedOut(){
    var t = "";
    try { t = (document.body.innerText || "").replace(/\s+/g, " ").toLowerCase(); } catch (e) {}
    return t.indexOf("welcome back") !== -1 && t.indexOf("let me in") !== -1 && t.indexOf("sign in with google") !== -1;
  }
  function go(){
    try {
      var p = location.pathname || "";
      if (window.__tmLogoutGo || !loggedOut()) return;
      if (p.indexOf("/signin") === 0 || p.indexOf("/login") === 0) return;
      window.__tmLogoutGo = true;
      location.replace("/signin?_tm=" + Date.now());
    } catch (e) {}
  }
  go();
  setInterval(go, 1000);
})();
</script>`
}
