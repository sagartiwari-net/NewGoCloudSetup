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

type syntxLogoutHit struct {
	at       time.Time
	next     string
	switched bool
}

var (
	syntxLogoutMu   sync.Mutex
	syntxLogoutSeen = map[int]syntxLogoutHit{}
	syntxSessionHop = map[string]time.Time{}
)

func syntxLogoutPath(path string) bool {
	p := strings.ToLower(path)
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	p = strings.TrimSuffix(p, "/")
	// API routes such as /api/v1/auth/google/login are the login button, not a logged-out page.
	if strings.Contains(p, "/api/") || strings.HasPrefix(p, "/extra-cdn-") || strings.HasPrefix(p, "/ext-host/") {
		return false
	}
	switch p {
	case "/__tm/logout", "/login", "/signin", "/sign-in", "/signup", "/sign-up", "/logout", "/register", "/forgot", "/forgot-password":
		return true
	}
	return false
}

func syntxAuthLocation(loc string) bool {
	l := strings.ToLower(strings.TrimSpace(loc))
	if l == "" {
		return false
	}
	if strings.Contains(l, "syntx.ai/login") || strings.Contains(l, "syntx.ai/sign-in") || strings.Contains(l, "syntx.ai/signin") || strings.Contains(l, "/users/sign_in") {
		return true
	}
	path := l
	if i := strings.Index(path, "://"); i >= 0 {
		if slash := strings.Index(path[i+3:], "/"); slash >= 0 {
			path = path[i+3+slash:]
		} else {
			return false
		}
	}
	return syntxLogoutPath(path)
}

func syntxLoggedOutHTML(body []byte) bool {
	s := strings.ToLower(string(body))
	return strings.Contains(s, "sign in and try the most popular ai models for free") && strings.Contains(s, "login with")
}

func syntxUpstreamLogout(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	return syntxAuthLocation(resp.Header.Get("Location")) || syntxAuthLocation(resp.Header.Get("X-Inertia-Location"))
}

func noteSyntxLogout(cfg Config, r *http.Request, acc ToolAccount, sessionToken, reason string) (switched bool, next string) {
	if !usesPanelAccountMode(cfg) || acc.ID <= 0 {
		return false, ""
	}
	key := sessionToken
	if key == "" {
		key = "acc:" + acc.Name
	}
	syntxLogoutMu.Lock()
	defer syntxLogoutMu.Unlock()
	if prev, seen := syntxSessionHop[key]; seen && time.Since(prev) < 2*time.Minute {
		last := syntxLogoutSeen[acc.ID]
		if time.Since(prev) < 2*time.Second {
			return last.switched, last.next
		}
		return false, last.next
	}
	syntxSessionHop[key] = time.Now()

	db, err := openPanelDB(cfg)
	if err != nil {
		log.Printf("[LB] logout db open failed: %v", err)
		delete(syntxSessionHop, key)
		return false, ""
	}
	now := time.Now().UTC().Format(time.RFC3339)
	nextName := "(none)"
	skipIDs := []any{cfg.PublicHost, acc.ID}
	placeholders := "?"
	for id, hit := range syntxLogoutSeen {
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

	websiteID, username, loginIP := syntxLogoutMeta(db, cfg, sessionToken)
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
	syntxLogoutSeen[acc.ID] = syntxLogoutHit{at: time.Now(), next: nextName, switched: switched}
	return switched, nextName
}

func syntxLogoutMeta(db *sql.DB, cfg Config, sessionToken string) (websiteID int, username, loginIP string) {
	if sessionToken != "" {
		_ = db.QueryRow(`SELECT website_id, username, COALESCE(client_ip, '') FROM live_sessions WHERE session_token=?`, sessionToken).Scan(&websiteID, &username, &loginIP)
	}
	if websiteID <= 0 {
		_ = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID)
	}
	return websiteID, username, loginIP
}

func syntxHome(cfg Config) string {
	if cfg.HomePath != "" {
		return cfg.HomePath
	}
	return "/video/runway"
}

func serveSyntxLogout(w http.ResponseWriter, r *http.Request, cfg Config, acc ToolAccount, sessionToken, reason string) {
	for k := range w.Header() {
		w.Header().Del(k)
	}
	switched, _ := noteSyntxLogout(cfg, r, acc, sessionToken, reason)
	log.Printf("[LB] expired session path=%s account=%s switched=%v", r.URL.Path, acc.Name, switched)
	if switched {
		http.Redirect(w, r, syntxHome(cfg), http.StatusFound)
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

func finishSyntxUpstreamLogout(w http.ResponseWriter, r *http.Request, resp *http.Response, cfg Config, acc ToolAccount, sessionToken string) {
	if resp != nil && resp.Body != nil {
		io.Copy(io.Discard, resp.Body)
	}
	serveSyntxLogout(w, r, cfg, acc, sessionToken, "session_expired")
}

func syntxLoginWatchHTML() string {
	// /login is swallowed by the page patcher (isAuthWall). Hop to a path it allows.
	return `<script data-tm-syntx-login="1">
(function(){
  function loggedOut(){
    var t = "";
    try { t = (document.body.innerText || "").replace(/\s+/g, " ").toLowerCase(); } catch (e) {}
    return t.indexOf("sign in and try the most popular ai models for free") !== -1 && t.indexOf("login with") !== -1;
  }
  function hop(){
    location.replace("/__tm/logout?_tm=" + Date.now());
  }
  function go(){
    try {
      if (!loggedOut()) return;
      var last = 0;
      try { last = parseInt(sessionStorage.getItem("tm_syntx_hop") || "0", 10) || 0; } catch (e) {}
      if (Date.now() - last < 8000) return;
      try { sessionStorage.setItem("tm_syntx_hop", String(Date.now())); } catch (e) {}
      hop();
    } catch (e) {}
  }
  go();
  setInterval(go, 1000);
})();
</script>`
}
