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

type liLogoutHit struct {
	at       time.Time
	next     string
	switched bool
}

var (
	liLogoutMu   sync.Mutex
	liLogoutSeen = map[int]liLogoutHit{}
	liSessionHop = map[string]time.Time{}
)

func linkedinLogoutPath(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	p := strings.ToLower(r.URL.Path)
	if strings.HasPrefix(p, "/learning-login") {
		return true
	}
	if p == "/login" || strings.HasPrefix(p, "/login/") || p == "/logout" || strings.HasPrefix(p, "/logout/") {
		return true
	}
	return strings.Contains(p, "/uas/login") || strings.Contains(p, "/uas/logout") || strings.Contains(p, "/m/logout")
}

// linkedinGuestHTML is the logged-out Learning homepage: Start free trial + Sign in.
func linkedinGuestHTML(body []byte) bool {
	s := strings.ToLower(string(body))
	if strings.Contains(s, "data-test-live-nav-sub-upsell") {
		return true
	}
	if strings.Contains(s, "homepage-learning_nav-header-signin") {
		return true
	}
	if strings.Contains(s, "d_learning_home_guest") {
		return true
	}
	return strings.Contains(s, "start free trial") && strings.Contains(s, "/learning-login")
}

func linkedinUpstreamLogout(resp *http.Response) bool {
	if resp == nil || resp.StatusCode < 300 || resp.StatusCode > 399 {
		return false
	}
	loc := strings.ToLower(resp.Header.Get("Location"))
	return strings.Contains(loc, "learning-login") ||
		strings.Contains(loc, "/uas/login") ||
		strings.Contains(loc, "/uas/logout") ||
		strings.Contains(loc, "/checkpoint/lg/login")
}

// noteLinkedinLogout writes Analytics → Logouts and moves the live session
// to another active account when one exists. Account status is left unchanged.
func noteLinkedinLogout(cfg Config, r *http.Request, acc ToolAccount, sessionToken, reason string) (switched bool, next string) {
	if !usesPanelAccountMode(cfg) || acc.ID <= 0 {
		return false, ""
	}
	liLogoutMu.Lock()
	if last, seen := liLogoutSeen[acc.ID]; seen && time.Since(last.at) < 2*time.Minute {
		liLogoutMu.Unlock()
		return false, last.next
	}
	liLogoutMu.Unlock()

	db, err := openPanelDB(cfg)
	if err != nil {
		log.Printf("[LB] logout db open failed: %v", err)
		return false, ""
	}
	now := time.Now().UTC().Format(time.RFC3339)
	nextName := "(none)"
	skipIDs := []any{cfg.PublicHost, acc.ID}
	placeholders := "?"
	liLogoutMu.Lock()
	for id, hit := range liLogoutSeen {
		if id == acc.ID || time.Since(hit.at) >= 10*time.Minute {
			continue
		}
		skipIDs = append(skipIDs, id)
		placeholders += ",?"
	}
	liLogoutMu.Unlock()
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

	websiteID, username, loginIP := linkedinLogoutMeta(db, cfg, sessionToken)
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
	liLogoutMu.Lock()
	liLogoutSeen[acc.ID] = liLogoutHit{at: time.Now(), next: nextName, switched: switched}
	liLogoutMu.Unlock()
	return switched, nextName
}

func linkedinLogoutMeta(db *sql.DB, cfg Config, sessionToken string) (websiteID int, username, loginIP string) {
	if sessionToken != "" {
		_ = db.QueryRow(`SELECT website_id, username, COALESCE(client_ip, '') FROM live_sessions WHERE session_token=?`, sessionToken).Scan(&websiteID, &username, &loginIP)
	}
	if websiteID <= 0 {
		_ = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID)
	}
	return websiteID, username, loginIP
}

func clearProxyHeaders(w http.ResponseWriter) {
	h := w.Header()
	for k := range h {
		h.Del(k)
	}
}

func serveLinkedinLogout(w http.ResponseWriter, r *http.Request, cfg Config, acc ToolAccount, sessionToken, reason string) {
	clearProxyHeaders(w)
	switched, _ := noteLinkedinLogout(cfg, r, acc, sessionToken, reason)
	if switched && linkedinCanHop(sessionToken) {
		home := cfg.HomePath
		if home == "" {
			home = "/learning/"
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

// linkedinCanHop allows one switch redirect per session so a dead replacement
// account cannot bounce the browser forever.
func linkedinCanHop(sessionToken string) bool {
	if sessionToken == "" {
		return false
	}
	liLogoutMu.Lock()
	defer liLogoutMu.Unlock()
	if last, ok := liSessionHop[sessionToken]; ok && time.Since(last) < 2*time.Minute {
		return false
	}
	liSessionHop[sessionToken] = time.Now()
	return true
}
