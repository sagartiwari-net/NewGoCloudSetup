package main

import (
	"database/sql"
	"log"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

const semrushPanelDB = "/Users/sagartiwari/Desktop/oneclickgo/pending-tools/panel-api/data/panel.db"

var (
	semrushPanelOnce   sync.Once
	semrushPanel       *sql.DB
	semrushPanelErr    error
	semrushPanelLogged int
	semrushPanelLogMu  sync.Mutex
)

func openSemrushPanel() (*sql.DB, error) {
	semrushPanelOnce.Do(func() {
		semrushPanel, semrushPanelErr = sql.Open("sqlite", "file:"+semrushPanelDB+"?_pragma=busy_timeout(20000)&_pragma=journal_mode(WAL)")
		if semrushPanelErr == nil {
			semrushPanel.SetMaxOpenConns(8)
			semrushPanelErr = semrushPanel.Ping()
		}
	})
	return semrushPanel, semrushPanelErr
}

func panelSemrushCookie(publicHost string) string {
	db, err := openSemrushPanel()
	if err != nil {
		log.Printf("[PANEL] database open failed: %v", err)
		return ""
	}
	var id int
	var name, raw string
	err = db.QueryRow(`SELECT a.id, a.name, a.cookie
		FROM accounts a
		JOIN websites w ON w.id = a.website_id
		WHERE w.domain IN ('127.0.0.1:5141', ?) AND a.status='active' AND TRIM(a.cookie) != ''
		ORDER BY CASE WHEN TRIM(COALESCE(a.last_used_at,'')) = '' THEN 0 ELSE 1 END, a.last_used_at ASC, a.id ASC
		LIMIT 1`, publicHost).Scan(&id, &name, &raw)
	if err != nil {
		log.Printf("[PANEL] no Semrush account cookie: %v", err)
		return ""
	}
	header := cookieHeaderFromStored(raw)
	if strings.TrimSpace(header) == "" {
		log.Printf("[PANEL] account %s id=%d cookie did not parse", name, id)
		return ""
	}
	semrushPanelLogMu.Lock()
	if semrushPanelLogged != id {
		log.Printf("[PANEL] cookie from account %s id=%d (%d cookies)", name, id, strings.Count(header, ";")+1)
		semrushPanelLogged = id
	}
	semrushPanelLogMu.Unlock()
	return header
}
