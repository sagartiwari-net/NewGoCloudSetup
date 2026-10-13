package main

import (
	"database/sql"
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

// ahrefsRecordSwitchOnce writes the logout even when no other account is free.
// The same website, user, account, and reason is kept to one row per two minutes
// so a reload loop does not fill Analytics.
func ahrefsRecordSwitchOnce(db *sql.DB, websiteID int, username, fromName, nextName, reason string) {
	if db == nil || websiteID <= 0 {
		return
	}
	reason = tmSanitizeReason(reason)
	username = strings.TrimSpace(username)
	cutoff := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	var id int
	err := db.QueryRow(`SELECT id FROM switch_events WHERE website_id=? AND username=? AND from_account_name=? AND reason=? AND switched_at>? LIMIT 1`,
		websiteID, username, fromName, reason, cutoff).Scan(&id)
	if err == nil {
		return
	}
	tmRecordSwitchLogout(db, websiteID, username, fromName, nextName, reason)
}

// tmRecordSwitchLogout writes Analytics Switches + Logouts with a short reason.
func tmRecordSwitchLogout(db *sql.DB, websiteID int, username, fromName, nextName, reason string) {
	if db == nil || websiteID <= 0 {
		return
	}
	tmEnsureLogoutEvents(db)
	reason = tmSanitizeReason(reason)
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
