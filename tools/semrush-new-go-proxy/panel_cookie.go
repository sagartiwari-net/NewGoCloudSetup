package main

import (
	"database/sql"
	"log"
	"os"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	semrushPanelOnce   sync.Once
	semrushPanel       *sql.DB
	semrushPanelErr    error
	semrushPanelLogged int
	semrushPanelLogMu  sync.Mutex
	semrushPanelPath   string
)

func panelSQLiteDSN(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		if _, err := os.Stat("/www/wwwroot/gt4rents.com/panel/data"); err == nil {
			path = "/www/wwwroot/gt4rents.com/panel/data/panel.db"
		} else {
			path = "/Users/sagartiwari/Desktop/oneclickgo/pending-tools/panel-api/data/panel.db"
		}
	}
	q := "?_pragma=busy_timeout(20000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	if strings.HasPrefix(path, "/") {
		return "file://" + path + q
	}
	return "file:" + path + q
}

func semrushPanelDBPath() string {
	cfg := loadConfig()
	if p := strings.TrimSpace(cfg.PanelDB); p != "" {
		return p
	}
	if v := strings.TrimSpace(os.Getenv("PANEL_DB")); v != "" {
		return v
	}
	if _, err := os.Stat("/www/wwwroot/gt4rents.com/panel/data"); err == nil {
		return "/www/wwwroot/gt4rents.com/panel/data/panel.db"
	}
	return "/Users/sagartiwari/Desktop/oneclickgo/pending-tools/panel-api/data/panel.db"
}

func openSemrushPanel() (*sql.DB, error) {
	semrushPanelOnce.Do(func() {
		semrushPanelPath = semrushPanelDBPath()
		dsn := panelSQLiteDSN(semrushPanelPath)
		semrushPanel, semrushPanelErr = sql.Open("sqlite", dsn)
		if semrushPanelErr == nil {
			semrushPanel.SetMaxOpenConns(8)
			semrushPanelErr = semrushPanel.Ping()
		}
		if semrushPanelErr != nil {
			log.Printf("[PANEL] open failed path=%s err=%v", semrushPanelPath, semrushPanelErr)
		} else {
			log.Printf("[PANEL] opened %s", semrushPanelPath)
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
