package main

import (
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

var logoutSeen sync.Map

func isLogoutPath(path string) bool {
	p := strings.ToLower(path)
	return strings.Contains(p, "logout") || strings.Contains(p, "sign-out") || strings.Contains(p, "signout") || isLoginPath(p)
}

func isLoginPath(path string) bool {
	p := strings.ToLower(path)
	return p == "/login" || strings.HasPrefix(p, "/login/") || strings.HasPrefix(p, "/login?")
}

func bodyLooksLoggedOut(body []byte) bool {
	text := strings.ToLower(string(body))
	return strings.Contains(text, "session expired") ||
		strings.Contains(text, "access again from dashboard") ||
		strings.Contains(text, "this section is restricted")
}

func noteLogout(cfg Config, r *http.Request, acc ToolAccount, sessionToken, reason string) (switched bool, nextName string) {
	if acc.ID > 0 {
		if prev, ok := logoutSeen.Load(acc.ID); ok {
			if time.Since(prev.(time.Time)) < 2*time.Minute {
				return false, ""
			}
		}
		logoutSeen.Store(acc.ID, time.Now())
	}
	db, err := openPanelDB(cfg)
	if err != nil {
		return false, ""
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var websiteID int
	var username, loginIP string
	if sessionToken != "" {
		_ = db.QueryRow(`SELECT website_id, username, COALESCE(client_ip, '') FROM live_sessions WHERE session_token=?`, sessionToken).Scan(&websiteID, &username, &loginIP)
	}
	if websiteID == 0 {
		_ = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID)
	}
	ip := ""
	if r != nil {
		ip = realClientIP(r)
	}
	if ip == "" || ip == "127.0.0.1" || ip == "::1" {
		ip = loginIP
	}
	if strings.TrimSpace(reason) == "" {
		reason = "logout"
	}
	next, err := scanPanelAccount(db.QueryRow(panelAccountSelect+`
		WHERE w.domain = ? AND a.status = 'active' AND TRIM(a.cookie) <> '' AND a.id <> ?
		`+panelAccountOrder+` LIMIT 1`, cfg.PublicHost, acc.ID))
	if err == nil {
		_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, next.ID)
		if sessionToken != "" {
			_, _ = db.Exec(`UPDATE live_sessions SET assigned_account_id=? WHERE session_token=?`, next.ID, sessionToken)
		}
		nextName = next.Name
		switched = true
		_, _ = db.Exec(`INSERT INTO switch_events (website_id, username, from_account_name, to_account_name, reason, switched_at) VALUES (?,?,?,?,?,?)`,
			websiteID, username, acc.Name, nextName, reason, now)
		log.Printf("[LB] switched %s -> %s user=%s", acc.Name, nextName, username)
	}
	_, _ = db.Exec(`INSERT INTO logout_events (website_id, username, account_name, next_account_name, reason, client_ip, created_at) VALUES (?,?,?,?,?,?,?)`,
		websiteID, username, acc.Name, nextName, reason, ip, now)
	log.Printf("[LB] logout recorded account=%s user=%s reason=%s", acc.Name, username, reason)
	if reason == "session_expired" && !switched {
		_, _ = db.Exec(`UPDATE accounts SET status='logged_out' WHERE id=?`, acc.ID)
	}
	return switched, nextName
}
