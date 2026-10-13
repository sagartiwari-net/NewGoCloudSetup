package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
)

func tmSanitizeReason(reason string) string {
	reason = strings.TrimSpace(reason)
	reason = strings.ReplaceAll(reason, "\n", " ")
	reason = strings.ReplaceAll(reason, "\r", " ")
	for strings.Contains(reason, "  ") {
		reason = strings.ReplaceAll(reason, "  ", " ")
	}
	if i := strings.Index(reason, "{"); i >= 0 {
		reason = strings.TrimSpace(reason[:i])
	}
	const max = 160
	if len(reason) > max {
		reason = reason[:max] + "…"
	}
	return reason
}

func tmEnsureLogoutEvents(db *sql.DB) {
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
}

func tmRecordSwitchLogout(db *sql.DB, websiteID int, username, fromName, nextName, reason string) {
	if db == nil || websiteID <= 0 {
		return
	}
	tmEnsureLogoutEvents(db)
	reason = tmSanitizeReason(reason)
	if strings.TrimSpace(nextName) == "" {
		nextName = "(none)"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO switch_events (website_id, username, from_account_name, to_account_name, reason, switched_at) VALUES (?,?,?,?,?,?)`,
		websiteID, username, fromName, nextName, reason, now); err != nil {
		log.Printf("[LB] switch_events insert failed: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO logout_events (website_id, username, account_name, next_account_name, reason, client_ip, created_at) VALUES (?,?,?,?,?,?,?)`,
		websiteID, username, fromName, nextName, reason, "", now); err != nil {
		log.Printf("[LB] logout_events insert failed: %v", err)
	}
}

func pickPanelAccount(db *sql.DB, websiteID int, excludeID int) (ToolAccount, error) {
	try := func(status string) (ToolAccount, error) {
		q := panelAccountSelect + `
			WHERE a.website_id = ? AND a.status = ? AND a.cookie != ''`
		args := []interface{}{websiteID, status}
		if excludeID > 0 {
			q += ` AND a.id != ?`
			args = append(args, excludeID)
		}
		q += panelAccountOrder + ` LIMIT 1`
		return scanPanelAccount(db.QueryRow(q, args...))
	}
	acc, err := try("active")
	if err == nil {
		return acc, nil
	}
	for _, st := range []string{"logged_out", "inactive"} {
		acc, err = try(st)
		if err == nil {
			_, _ = db.Exec(`UPDATE accounts SET status='active', failure_count=0 WHERE id=?`, acc.ID)
			log.Printf("[PANEL] revived account id=%d name=%q (was %s)", acc.ID, acc.Name, st)
			return acc, nil
		}
	}
	return ToolAccount{}, fmt.Errorf("no mapped account with cookie")
}

func panelReloadAccount(cfg Config, sessionToken string) (ToolAccount, error) {
	panelPickMu.Lock()
	defer panelPickMu.Unlock()
	db, err := openPanelDB(cfg)
	if err != nil {
		return ToolAccount{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var assigned, websiteID int
	if err := db.QueryRow(`SELECT COALESCE(assigned_account_id, 0), website_id FROM live_sessions WHERE session_token=? AND expires_at>?`, sessionToken, now).Scan(&assigned, &websiteID); err != nil {
		return ToolAccount{}, err
	}
	if !panelWebsiteSameTool(db, cfg.PublicHost, websiteID) {
		return ToolAccount{}, fmt.Errorf("session is for another website")
	}
	if assigned > 0 {
		acc, accErr := scanPanelAccount(db.QueryRow(panelAccountSelect+`
			WHERE a.id = ? AND a.website_id = ? AND a.cookie != '' AND a.status IN ('active','logged_out','inactive')`, assigned, websiteID))
		if accErr == nil {
			_, _ = db.Exec(`UPDATE accounts SET status='active', failure_count=0 WHERE id=? AND status='logged_out'`, acc.ID)
			return acc, nil
		}
	}
	return pickPanelAccount(db, websiteID, 0)
}

func panelMarkAccountLoggedOut(cfg Config, accountID int, reason string) {
	if accountID <= 0 {
		return
	}
	db, err := openPanelDB(cfg)
	if err != nil {
		return
	}
	_, _ = db.Exec(`UPDATE accounts SET failure_count=failure_count+1 WHERE id=?`, accountID)
	log.Printf("[LB] logout noted, status unchanged account=%d reason=%s", accountID, tmSanitizeReason(reason))
}

func panelSwitchToOtherAccount(cfg Config, sessionToken, reason string) (ToolAccount, string, error) {
	panelPickMu.Lock()
	defer panelPickMu.Unlock()
	db, err := openPanelDB(cfg)
	if err != nil {
		return ToolAccount{}, "", err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var websiteID, currentID int
	var username string
	err = db.QueryRow(`SELECT website_id, COALESCE(assigned_account_id, 0), username FROM live_sessions WHERE session_token=? AND expires_at>?`, sessionToken, now).
		Scan(&websiteID, &currentID, &username)
	if err != nil {
		return ToolAccount{}, "", err
	}
	if reason == "" {
		reason = "logout-failover"
	}
	var fromName string
	_ = db.QueryRow(`SELECT name FROM accounts WHERE id=?`, currentID).Scan(&fromName)
	acc, err := pickPanelAccount(db, websiteID, currentID)
	if err != nil {
		if websiteID > 0 {
			tmRecordSwitchLogout(db, websiteID, username, fromName, "", reason)
		}
		return ToolAccount{}, "", fmt.Errorf("no other active account")
	}
	_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, acc.ID)
	_, _ = db.Exec(`UPDATE live_sessions SET assigned_account_id=? WHERE session_token=?`, acc.ID, sessionToken)
	if websiteID > 0 {
		tmRecordSwitchLogout(db, websiteID, username, fromName, acc.Name, reason)
	}
	log.Printf("[LB] switched %s -> %s reason=%s", fromName, acc.Name, reason)
	return acc, acc.Name, nil
}
