package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ahrefsPanelMu   sync.Mutex
	ahrefsPanel     *sql.DB
	ahrefsPanelErr  error
	ahrefsPanelPath string
	// Per-session panel account switch budget (stops Ahrefs 1↔2 reload loops).
	ahrefsPanelSwitchCounts sync.Map // sessionToken → int
)

const ahrefsPanelSwitchMax = 3

func ahrefsPanelSwitchBudgetExceeded(sessionToken string) bool {
	if sessionToken == "" {
		return false
	}
	raw, ok := ahrefsPanelSwitchCounts.Load(sessionToken)
	if !ok {
		return false
	}
	return raw.(int) >= ahrefsPanelSwitchMax
}

func ahrefsPanelSwitchBump(sessionToken string) {
	if sessionToken == "" {
		return
	}
	for {
		raw, _ := ahrefsPanelSwitchCounts.LoadOrStore(sessionToken, 0)
		n := raw.(int)
		if ahrefsPanelSwitchCounts.CompareAndSwap(sessionToken, n, n+1) {
			return
		}
	}
}

func ahrefsPanelDBPath() string {
	cfg := loadConfig()
	if p := strings.TrimSpace(cfg.PanelDB); p != "" {
		return p
	}
	if v := strings.TrimSpace(os.Getenv("PANEL_DB")); v != "" {
		return v
	}
	// Server default for gt4rents deploy
	if _, err := os.Stat("/www/wwwroot/gt4rents.com/panel/data"); err == nil {
		return "/www/wwwroot/gt4rents.com/panel/data/panel.db"
	}
	// Local Mac fallback
	return "/Users/sagartiwari/Desktop/oneclickgo/pending-tools/panel-api/data/panel.db"
}

func openAhrefsPanel() (*sql.DB, error) {
	path := ahrefsPanelDBPath()
	ahrefsPanelMu.Lock()
	defer ahrefsPanelMu.Unlock()
	if ahrefsPanel != nil && ahrefsPanelErr == nil && ahrefsPanelPath == path {
		return ahrefsPanel, nil
	}
	if ahrefsPanel != nil {
		_ = ahrefsPanel.Close()
		ahrefsPanel = nil
	}
	ahrefsPanelPath = path
	// Absolute paths need file:///… (3 slashes). file:/www/... is mis-parsed and fails with CANTOPEN.
	dsn := path
	if strings.HasPrefix(path, "/") {
		dsn = "file://" + path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	} else {
		dsn = "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	}
	ahrefsPanel, ahrefsPanelErr = sql.Open("sqlite", dsn)
	if ahrefsPanelErr == nil {
		ahrefsPanel.SetMaxOpenConns(1)
		ahrefsPanelErr = ahrefsPanel.Ping()
	}
	if ahrefsPanelErr != nil {
		log.Printf("[PANEL] open failed path=%s dsn=%s err=%v", path, dsn, ahrefsPanelErr)
	} else {
		log.Printf("[PANEL] opened %s", path)
	}
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
	q := `SELECT a.id, a.name, a.cookie,
		CASE WHEN TRIM(a.user_agent) != '' THEN a.user_agent ELSE COALESCE(ua.user_agent, '') END,
		CASE WHEN TRIM(COALESCE(p.endpoint, '')) != '' THEN p.endpoint ELSE a.proxy END
		FROM accounts a
		JOIN websites w ON w.id = a.website_id
		LEFT JOIN user_agents ua ON ua.id = a.user_agent_id
		LEFT JOIN proxies p ON p.id = a.proxy_id
		WHERE w.domain IN ('127.0.0.1:5291', ?) AND a.status='active' AND TRIM(a.cookie) != ''`
	args := []any{publicHost}
	if excludeID > 0 {
		q += ` AND a.id != ?`
		args = append(args, excludeID)
	}
	q += ` ORDER BY CASE WHEN a.last_used_at IS NULL OR TRIM(a.last_used_at)='' THEN 0 ELSE 1 END, a.last_used_at ASC, a.id ASC LIMIT 1`
	err := db.QueryRow(q, args...).Scan(&acc.ID, &acc.Name, &acc.Cookie, &acc.UserAgent, &acc.Proxy)
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
		tmRecordSwitchLogout(db, websiteID, username, current.Name, next.Name, reason)
	}
	log.Printf("[LB] switched %s -> %s reason=%s", current.Name, next.Name, reason)
	return next, nil
}
