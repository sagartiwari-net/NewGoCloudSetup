package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const ahrefsPanelDB = "/Users/sagartiwari/Desktop/oneclickgo/pending-tools/panel-api/data/panel.db"

var (
	ahrefsPanelOnce sync.Once
	ahrefsPanel     *sql.DB
	ahrefsPanelErr  error
)

func openAhrefsPanel() (*sql.DB, error) {
	ahrefsPanelOnce.Do(func() {
		ahrefsPanel, ahrefsPanelErr = sql.Open("sqlite", "file:"+ahrefsPanelDB+"?_pragma=busy_timeout(5000)")
		if ahrefsPanelErr == nil {
			ahrefsPanel.SetMaxOpenConns(1)
			ahrefsPanelErr = ahrefsPanel.Ping()
		}
	})
	return ahrefsPanel, ahrefsPanelErr
}

func ahrefsPanelAccount(publicHost, sessionToken string) (AhrefsAccount, bool) {
	db, err := openAhrefsPanel()
	if err != nil {
		log.Printf("[PANEL] database open failed: %v", err)
		return AhrefsAccount{}, false
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	assignedID := 0
	if sessionToken != "" {
		_ = db.QueryRow(`SELECT COALESCE(assigned_account_id, 0) FROM live_sessions WHERE session_token=? AND expires_at>?`, sessionToken, now).Scan(&assignedID)
	}
	if assignedID > 0 {
		if acc, err := ahrefsLoadAccount(db, publicHost, assignedID); err == nil {
			log.Printf("[LB] assigned account %s id=%d", acc.Name, acc.ID)
			return acc, true
		}
	}
	acc, err := ahrefsLeastUsedAccount(db, publicHost, 0)
	if err != nil {
		log.Printf("[PANEL] account lookup: %v", err)
		return AhrefsAccount{}, false
	}
	_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, acc.ID)
	if sessionToken != "" {
		_, _ = db.Exec(`UPDATE live_sessions SET assigned_account_id=? WHERE session_token=?`, acc.ID, sessionToken)
	}
	log.Printf("[LB] load balanced to account %s id=%d", acc.Name, acc.ID)
	return acc, true
}

func ahrefsLoadAccount(db *sql.DB, publicHost string, id int) (AhrefsAccount, error) {
	var acc AhrefsAccount
	err := db.QueryRow(`SELECT a.id, a.name, a.cookie,
		CASE WHEN TRIM(a.user_agent) != '' THEN a.user_agent ELSE COALESCE(ua.user_agent, '') END,
		CASE WHEN TRIM(COALESCE(p.endpoint, '')) != '' THEN p.endpoint ELSE a.proxy END
		FROM accounts a
		JOIN websites w ON w.id = a.website_id
		LEFT JOIN user_agents ua ON ua.id = a.user_agent_id
		LEFT JOIN proxies p ON p.id = a.proxy_id
		WHERE w.domain IN ('127.0.0.1:5291', ?) AND a.id=? AND a.status='active' AND TRIM(a.cookie) != ''`,
		publicHost, id).Scan(&acc.ID, &acc.Name, &acc.Cookie, &acc.UserAgent, &acc.Proxy)
	if err != nil {
		return acc, err
	}
	if strings.TrimSpace(acc.UserAgent) == "" {
		return acc, fmt.Errorf("missing user agent")
	}
	acc.ShowLimit = true
	return acc, nil
}

func ahrefsLeastUsedAccount(db *sql.DB, publicHost string, excludeID int) (AhrefsAccount, error) {
	var acc AhrefsAccount
	err := db.QueryRow(`SELECT a.id, a.name, a.cookie,
		CASE WHEN TRIM(a.user_agent) != '' THEN a.user_agent ELSE COALESCE(ua.user_agent, '') END,
		CASE WHEN TRIM(COALESCE(p.endpoint, '')) != '' THEN p.endpoint ELSE a.proxy END
		FROM accounts a
		JOIN websites w ON w.id = a.website_id
		LEFT JOIN user_agents ua ON ua.id = a.user_agent_id
		LEFT JOIN proxies p ON p.id = a.proxy_id
		WHERE w.domain IN ('127.0.0.1:5291', ?) AND a.status='active' AND TRIM(a.cookie) != '' AND a.id != ?
		ORDER BY CASE WHEN TRIM(COALESCE(a.last_used_at,'')) = '' THEN 0 ELSE 1 END, a.last_used_at ASC, a.id ASC
		LIMIT 1`, publicHost, excludeID).Scan(&acc.ID, &acc.Name, &acc.Cookie, &acc.UserAgent, &acc.Proxy)
	if err != nil {
		return acc, err
	}
	if strings.TrimSpace(acc.UserAgent) == "" {
		return acc, fmt.Errorf("missing user agent")
	}
	acc.ShowLimit = true
	return acc, nil
}

func ahrefsSessionUser(token string) string {
	if token == "" {
		return ""
	}
	db, err := openAhrefsPanel()
	if err != nil {
		return ""
	}
	var username string
	_ = db.QueryRow(`SELECT username FROM live_sessions WHERE session_token=?`, token).Scan(&username)
	return username
}

func switchAhrefsPanelAccount(publicHost, sessionToken string, current AhrefsAccount, username, reason string) (AhrefsAccount, error) {
	db, err := openAhrefsPanel()
	if err != nil {
		return AhrefsAccount{}, err
	}
	next, err := ahrefsLeastUsedAccount(db, publicHost, current.ID)
	if err != nil {
		return AhrefsAccount{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, next.ID)
	if sessionToken != "" {
		_, _ = db.Exec(`UPDATE live_sessions SET assigned_account_id=? WHERE session_token=?`, next.ID, sessionToken)
	}
	var websiteID int
	_ = db.QueryRow(`SELECT id FROM websites WHERE domain IN ('127.0.0.1:5291', ?) LIMIT 1`, publicHost).Scan(&websiteID)
	if websiteID != 0 {
		_, _ = db.Exec(`INSERT INTO switch_events (website_id, username, from_account_name, to_account_name, reason, switched_at) VALUES (?,?,?,?,?,?)`,
			websiteID, username, current.Name, next.Name, reason, now)
	}
	log.Printf("[LB] switched %s -> %s reason=%s", current.Name, next.Name, reason)
	return next, nil
}
