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

// tmRecordSwitchLogout writes Analytics Switches + Logouts with a short reason.
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
