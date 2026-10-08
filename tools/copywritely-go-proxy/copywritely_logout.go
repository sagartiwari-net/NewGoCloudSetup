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

type cwLogoutHit struct {
	at       time.Time
	next     string
	switched bool
}

var (
	cwLogoutMu   sync.Mutex
	cwLogoutSeen = map[int]cwLogoutHit{}
	cwSessionHop = map[string]time.Time{}
)

func copywritelyLogoutRequest(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	q := r.URL.Query()
	if q.Get("login_popup") == "1" || strings.TrimSpace(q.Get("loggedout")) != "" {
		return true
	}
	p := strings.ToLower(r.URL.Path)
	if strings.Contains(p, "logout") {
		return true
	}
	return strings.Contains(p, "wp-login.php") && strings.EqualFold(q.Get("action"), "logout")
}

func copywritelyLogoutReason(r *http.Request) string {
	if r != nil && strings.Contains(strings.ToLower(r.URL.Path), "logout") {
		return "user_logout"
	}
	return "login_popup"
}

func copywritelyUpstreamLogout(resp *http.Response) bool {
	if resp == nil || resp.StatusCode < 300 || resp.StatusCode > 399 {
		return false
	}
	loc := strings.ToLower(resp.Header.Get("Location"))
	return strings.Contains(loc, "login_popup=") || strings.Contains(loc, "loggedout=") || strings.Contains(loc, "action=logout")
}

// noteCopywritelyLogout writes Analytics → Logouts and moves the live session
// to another active account when one exists. Account status is left unchanged.
func noteCopywritelyLogout(cfg Config, r *http.Request, acc ToolAccount, sessionToken, reason string) (switched bool, next string) {
	if !usesPanelAccountMode(cfg) || acc.ID <= 0 {
		return false, ""
	}
	cwLogoutMu.Lock()
	if last, seen := cwLogoutSeen[acc.ID]; seen && time.Since(last.at) < 2*time.Minute {
		cwLogoutMu.Unlock()
		// Already handled this account. Redirecting again loops on /tools/.
		return false, last.next
	}
	cwLogoutMu.Unlock()

	db, err := openPanelDB(cfg)
	if err != nil {
		log.Printf("[LB] logout db open failed: %v", err)
		return false, ""
	}
	now := time.Now().UTC().Format(time.RFC3339)
	nextName := "(none)"
	skipIDs := []any{cfg.PublicHost, acc.ID}
	placeholders := "?"
	cwLogoutMu.Lock()
	for id, hit := range cwLogoutSeen {
		if id == acc.ID || time.Since(hit.at) >= 10*time.Minute {
			continue
		}
		skipIDs = append(skipIDs, id)
		placeholders += ",?"
	}
	cwLogoutMu.Unlock()
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

	websiteID, username, loginIP := copywritelyLogoutMeta(db, cfg, sessionToken)
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
		reason = "login_popup"
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
	cwLogoutMu.Lock()
	cwLogoutSeen[acc.ID] = cwLogoutHit{at: time.Now(), next: nextName, switched: switched}
	cwLogoutMu.Unlock()
	return switched, nextName
}

func copywritelyLogoutMeta(db *sql.DB, cfg Config, sessionToken string) (websiteID int, username, loginIP string) {
	if sessionToken != "" {
		_ = db.QueryRow(`SELECT website_id, username, COALESCE(client_ip, '') FROM live_sessions WHERE session_token=?`, sessionToken).Scan(&websiteID, &username, &loginIP)
	}
	if websiteID <= 0 {
		_ = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID)
	}
	return websiteID, username, loginIP
}

func serveCopywritelyLogout(w http.ResponseWriter, r *http.Request, cfg Config, acc ToolAccount, sessionToken, reason string) {
	switched, _ := noteCopywritelyLogout(cfg, r, acc, sessionToken, reason)
	if switched && copywritelyCanHop(sessionToken) {
		home := cfg.HomePath
		if home == "" || home == "/" {
			home = "/tools/"
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

// copywritelyCanHop allows one switch redirect per session. A second logout
// in the same burst must render contact-admin, not 302 back to /tools/.
func copywritelyCanHop(sessionToken string) bool {
	if sessionToken == "" {
		return false
	}
	cwLogoutMu.Lock()
	defer cwLogoutMu.Unlock()
	if last, ok := cwSessionHop[sessionToken]; ok && time.Since(last) < 2*time.Minute {
		return false
	}
	cwSessionHop[sessionToken] = time.Now()
	return true
}
