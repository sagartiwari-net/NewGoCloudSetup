package main

import (
	"database/sql"
	"net/http"
	"strings"
	"time"
)

func panelWebsiteIDForDomain(db *sql.DB, host string) int {
	if db == nil {
		return 0
	}
	host = strings.TrimSpace(host)
	bare := panelHostOnly(host)
	var id int
	_ = db.QueryRow(`SELECT id FROM websites
		WHERE lower(trim(domain)) IN (lower(trim(?)), lower(trim(?)))
		ORDER BY id ASC LIMIT 1`, host, bare).Scan(&id)
	return id
}

func panelWebsiteSameTool(db *sql.DB, publicHost string, websiteID int) bool {
	if db == nil || websiteID <= 0 {
		return false
	}
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM websites w
		JOIN websites owner ON owner.tool_id = w.tool_id
		WHERE w.id = ?
		  AND (lower(trim(owner.domain)) = lower(trim(?)) OR lower(trim(owner.domain)) = lower(trim(?)))`,
		websiteID, publicHost, panelHostOnly(publicHost)).Scan(&n)
	return err == nil && n > 0
}

func panelHostAllowed(publicHost, reqHost string) bool {
	reqHost = panelHostOnly(reqHost)
	if reqHost == "" || panelHostIsLocal(reqHost) {
		return false
	}
	cfg := loadConfig()
	db, err := openPanelDB(cfg)
	if err != nil {
		return false
	}
	var n int
	err = db.QueryRow(`SELECT COUNT(*) FROM websites w
		JOIN websites owner ON owner.tool_id = w.tool_id
		WHERE (lower(trim(w.domain)) = lower(trim(?)) OR lower(trim(w.domain)) = lower(trim(?)))
		  AND (lower(trim(owner.domain)) = lower(trim(?)) OR lower(trim(owner.domain)) = lower(trim(?)))`,
		reqHost, reqHost, publicHost, panelHostOnly(publicHost)).Scan(&n)
	return err == nil && n > 0
}

func panelToolDomains(publicHost string) []string {
	cfg := loadConfig()
	db, err := openPanelDB(cfg)
	if err != nil {
		return nil
	}
	rows, err := db.Query(`SELECT w.domain FROM websites w
		JOIN websites owner ON owner.tool_id = w.tool_id
		WHERE lower(trim(owner.domain)) = lower(trim(?)) OR lower(trim(owner.domain)) = lower(trim(?))`,
		publicHost, panelHostOnly(publicHost))
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	seen := map[string]bool{}
	for rows.Next() {
		var domain string
		if rows.Scan(&domain) != nil {
			continue
		}
		domain = panelHostOnly(domain)
		if domain == "" || seen[domain] {
			continue
		}
		seen[domain] = true
		out = append(out, domain)
	}
	return out
}

func panelWebsiteIDForUser(db *sql.DB, publicHost, username string) int {
	if db == nil {
		return 0
	}
	username = strings.TrimSpace(username)
	if username != "" {
		var id int
		now := time.Now().UTC().Format(time.RFC3339)
		err := db.QueryRow(`SELECT s.website_id FROM live_sessions s
			JOIN websites w ON w.id = s.website_id
			JOIN websites owner ON owner.tool_id = w.tool_id
			WHERE s.username = ? AND s.expires_at > ?
			  AND (lower(trim(owner.domain)) = lower(trim(?)) OR lower(trim(owner.domain)) = lower(trim(?)))
			ORDER BY s.id DESC LIMIT 1`,
			username, now, publicHost, panelHostOnly(publicHost)).Scan(&id)
		if err == nil && id > 0 {
			return id
		}
	}
	return panelWebsiteIDForDomain(db, publicHost)
}

func panelWebsiteIDForRequest(r *http.Request) int {
	if r == nil {
		return 0
	}
	cfg := loadConfig()
	db, err := openPanelDB(cfg)
	if err != nil {
		return 0
	}
	host := panelRequestHost(r, cfg.PublicHost)
	id := panelWebsiteIDForDomain(db, host)
	if id <= 0 || !panelWebsiteSameTool(db, cfg.PublicHost, id) {
		return 0
	}
	return id
}
