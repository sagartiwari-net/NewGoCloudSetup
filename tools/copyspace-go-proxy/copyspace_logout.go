package main

import (
	"database/sql"
	"html"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type csLogoutHit struct {
	at       time.Time
	next     string
	switched bool
}

var (
	csLogoutMu   sync.Mutex
	csLogoutSeen = map[int]csLogoutHit{}
	csSessionHop = map[string]time.Time{}
)

func copyspaceLogoutPath(path string) bool {
	p := strings.ToLower(path)
	if i := strings.Index(p, "/extra-cdn-"); i >= 0 {
		rest := p[i+len("/extra-cdn-"):]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			p = rest[slash:]
		}
	}
	p = strings.TrimSuffix(p, "/")
	switch p {
	case "/signin", "/sign-in", "/login", "/signup", "/register", "/logout", "/forgot-password":
		return true
	}
	return strings.HasPrefix(p, "/signin/") || strings.HasPrefix(p, "/login/") || strings.HasPrefix(p, "/sign-in/")
}

func copyspaceLoggedOutHTML(body []byte) bool {
	s := strings.ToLower(string(body))
	if strings.Contains(s, "sign in to your account") && strings.Contains(s, "welcome back") {
		return true
	}
	return strings.Contains(s, "forgot password") && strings.Contains(s, "don't have an account")
}

func copyspaceUpstreamLogout(resp *http.Response) bool {
	if resp == nil || resp.StatusCode < 300 || resp.StatusCode > 399 {
		return false
	}
	loc := strings.ToLower(resp.Header.Get("Location"))
	return strings.Contains(loc, "/signin") || strings.Contains(loc, "/sign-in") ||
		strings.Contains(loc, "/login") || strings.Contains(loc, "/logout")
}

func clearCopyspaceSessionOverlay() {
	localCookieMu.Lock()
	delete(localCookieOverlay, "copyspaceai_session")
	delete(localCookieOverlay, "XSRF-TOKEN")
	localCookieMu.Unlock()
}

func noteCopyspaceLogout(cfg Config, r *http.Request, acc ToolAccount, sessionToken, reason string) (switched bool, next string) {
	if !usesPanelAccountMode(cfg) || acc.ID <= 0 {
		return false, ""
	}
	csLogoutMu.Lock()
	if last, seen := csLogoutSeen[acc.ID]; seen && time.Since(last.at) < 2*time.Minute {
		csLogoutMu.Unlock()
		return false, last.next
	}
	csLogoutMu.Unlock()

	db, err := openPanelDB(cfg)
	if err != nil {
		log.Printf("[LB] logout db open failed: %v", err)
		return false, ""
	}
	now := time.Now().UTC().Format(time.RFC3339)
	nextName := "(none)"
	skipIDs := []any{cfg.PublicHost, acc.ID}
	placeholders := "?"
	csLogoutMu.Lock()
	for id, hit := range csLogoutSeen {
		if id == acc.ID || time.Since(hit.at) >= 10*time.Minute {
			continue
		}
		skipIDs = append(skipIDs, id)
		placeholders += ",?"
	}
	csLogoutMu.Unlock()
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

	websiteID, username, loginIP := copyspaceLogoutMeta(db, cfg, sessionToken)
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
	csLogoutMu.Lock()
	csLogoutSeen[acc.ID] = csLogoutHit{at: time.Now(), next: nextName, switched: switched}
	csLogoutMu.Unlock()
	return switched, nextName
}

func copyspaceLogoutMeta(db *sql.DB, cfg Config, sessionToken string) (websiteID int, username, loginIP string) {
	if sessionToken != "" {
		_ = db.QueryRow(`SELECT website_id, username, COALESCE(client_ip, '') FROM live_sessions WHERE session_token=?`, sessionToken).Scan(&websiteID, &username, &loginIP)
	}
	if websiteID <= 0 {
		_ = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID)
	}
	return websiteID, username, loginIP
}

func serveCopyspaceLogout(w http.ResponseWriter, r *http.Request, cfg Config, acc ToolAccount, sessionToken, reason string) {
	clearCopyspaceSessionOverlay()
	for k := range w.Header() {
		w.Header().Del(k)
	}
	switched, _ := noteCopyspaceLogout(cfg, r, acc, sessionToken, reason)
	if switched && copyspaceCanHop(sessionToken) {
		home := cfg.HomePath
		if home == "" {
			home = "/"
		}
		http.Redirect(w, r, home, http.StatusFound)
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

func copyspaceCanHop(sessionToken string) bool {
	if sessionToken == "" {
		return false
	}
	csLogoutMu.Lock()
	defer csLogoutMu.Unlock()
	if last, ok := csSessionHop[sessionToken]; ok && time.Since(last) < 2*time.Minute {
		return false
	}
	csSessionHop[sessionToken] = time.Now()
	return true
}
