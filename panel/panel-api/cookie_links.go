package main

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *server) ensureCookieLinks() {
	_, _ = s.db.Exec(`CREATE TABLE IF NOT EXISTS account_cookie_links (
		follower_account_id INTEGER PRIMARY KEY,
		source_account_id INTEGER NOT NULL,
		created_at TEXT NOT NULL
	)`)
}

func (s *server) shareCookieFrom(sourceID int) {
	if sourceID <= 0 || s.db == nil {
		return
	}
	s.ensureCookieLinks()
	var cookie, userAgent string
	var userAgentID sql.NullInt64
	err := s.db.QueryRow(`SELECT COALESCE(cookie, ''), user_agent_id, COALESCE(user_agent, '') FROM accounts WHERE id=?`, sourceID).
		Scan(&cookie, &userAgentID, &userAgent)
	cookie = strings.TrimSpace(cookie)
	if err != nil || cookie == "" {
		return
	}
	rows, err := s.db.Query(`SELECT follower_account_id FROM account_cookie_links WHERE source_account_id=?`, sourceID)
	if err != nil {
		return
	}
	defer rows.Close()
	var followers []int
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil && id > 0 && id != sourceID {
			followers = append(followers, id)
		}
	}
	if len(followers) == 0 {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	copyAgent := userAgentID.Valid || strings.TrimSpace(userAgent) != ""
	var agentID any
	if userAgentID.Valid {
		agentID = userAgentID.Int64
	}
	for _, followerID := range followers {
		_, _ = s.db.Exec(`UPDATE accounts SET cookie=?, cookie_updated_at=? WHERE id=?`, cookie, now, followerID)
		if copyAgent {
			_, _ = s.db.Exec(`UPDATE accounts SET user_agent_id=?, user_agent=? WHERE id=?`, agentID, userAgent, followerID)
		}
	}
}

func (s *server) accountShareLabel(accountID int) string {
	var accountName, siteName, domain string
	err := s.db.QueryRow(`SELECT a.name, w.name, w.domain FROM accounts a JOIN websites w ON w.id=a.website_id WHERE a.id=?`, accountID).
		Scan(&accountName, &siteName, &domain)
	if err != nil {
		return ""
	}
	return websiteChoiceLabel(siteName, domain) + " — " + strings.TrimSpace(accountName)
}

func (s *server) listCookieLinks(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.master(w, r); !ok {
		return
	}
	s.ensureCookieLinks()
	rows, err := s.db.Query(`SELECT l.follower_account_id, l.source_account_id, l.created_at,
		fa.name, fw.name, fw.domain, sa.name, sw.name, sw.domain
		FROM account_cookie_links l
		JOIN accounts fa ON fa.id=l.follower_account_id
		JOIN websites fw ON fw.id=fa.website_id
		JOIN accounts sa ON sa.id=l.source_account_id
		JOIN websites sw ON sw.id=sa.website_id
		ORDER BY fw.name, fa.name`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var followerID, sourceID int
		var created, followerName, followerSite, followerDomain, sourceName, sourceSite, sourceDomain string
		if rows.Scan(&followerID, &sourceID, &created, &followerName, &followerSite, &followerDomain, &sourceName, &sourceSite, &sourceDomain) != nil {
			continue
		}
		items = append(items, map[string]any{
			"follower_account_id": followerID,
			"source_account_id":   sourceID,
			"created_at":          created,
			"follower_label":      websiteChoiceLabel(followerSite, followerDomain) + " — " + strings.TrimSpace(followerName),
			"source_label":        websiteChoiceLabel(sourceSite, sourceDomain) + " — " + strings.TrimSpace(sourceName),
		})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *server) cookieLinkSources(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.master(w, r); !ok {
		return
	}
	s.ensureCookieLinks()
	accountID, _ := strconv.Atoi(r.URL.Query().Get("accountId"))
	if accountID <= 0 {
		writeErr(w, 400, "Choose an account")
		return
	}
	var exists int
	if err := s.db.QueryRow(`SELECT id FROM accounts WHERE id=?`, accountID).Scan(&exists); err != nil {
		writeErr(w, 404, "Account not found")
		return
	}
	var usedBy int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM account_cookie_links WHERE source_account_id=?`, accountID).Scan(&usedBy)
	var sourceID int
	var sourceLabel string
	err := s.db.QueryRow(`SELECT source_account_id FROM account_cookie_links WHERE follower_account_id=?`, accountID).Scan(&sourceID)
	if err == nil {
		sourceLabel = s.accountShareLabel(sourceID)
	}
	rows, err := s.db.Query(`SELECT a.id, a.name, w.name, w.domain
		FROM accounts a
		JOIN websites w ON w.id=a.website_id
		WHERE a.id<>?
		AND a.id NOT IN (SELECT follower_account_id FROM account_cookie_links)
		ORDER BY w.name, a.name`, accountID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	sources := []map[string]any{}
	for rows.Next() {
		var id int
		var name, site, domain string
		if rows.Scan(&id, &name, &site, &domain) != nil {
			continue
		}
		sources = append(sources, map[string]any{
			"id":    id,
			"label": websiteChoiceLabel(site, domain) + " — " + strings.TrimSpace(name),
		})
	}
	var current any
	if sourceID > 0 {
		current = map[string]any{"source_account_id": sourceID, "source_label": sourceLabel}
	}
	writeJSON(w, 200, map[string]any{
		"is_source": usedBy > 0,
		"current":   current,
		"sources":   sources,
	})
}

func (s *server) saveCookieLink(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.master(w, r); !ok {
		return
	}
	s.ensureCookieLinks()
	var body struct {
		FollowerAccountID int `json:"follower_account_id"`
		SourceAccountID   int `json:"source_account_id"`
	}
	if err := readBody(r, &body); err != nil || body.FollowerAccountID <= 0 || body.SourceAccountID <= 0 {
		writeErr(w, 400, "Choose an account")
		return
	}
	if body.FollowerAccountID == body.SourceAccountID {
		writeErr(w, 400, "Choose a different account")
		return
	}
	var followerExists, sourceExists int
	if err := s.db.QueryRow(`SELECT id FROM accounts WHERE id=?`, body.FollowerAccountID).Scan(&followerExists); err != nil {
		writeErr(w, 404, "Account not found")
		return
	}
	if err := s.db.QueryRow(`SELECT id FROM accounts WHERE id=?`, body.SourceAccountID).Scan(&sourceExists); err != nil {
		writeErr(w, 404, "Account not found")
		return
	}
	var follows int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM account_cookie_links WHERE follower_account_id=?`, body.SourceAccountID).Scan(&follows)
	if follows > 0 {
		writeErr(w, 400, "That account already uses another cookie")
		return
	}
	var usedBy int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM account_cookie_links WHERE source_account_id=?`, body.FollowerAccountID).Scan(&usedBy)
	if usedBy > 0 {
		writeErr(w, 400, "Other accounts already use this cookie")
		return
	}
	var cookie, userAgent string
	var userAgentID sql.NullInt64
	if err := s.db.QueryRow(`SELECT COALESCE(cookie, ''), user_agent_id, COALESCE(user_agent, '') FROM accounts WHERE id=?`, body.SourceAccountID).
		Scan(&cookie, &userAgentID, &userAgent); err != nil {
		writeErr(w, 404, "Account not found")
		return
	}
	if strings.TrimSpace(cookie) == "" {
		writeErr(w, 400, "That account has no cookie yet")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.db.Exec(`DELETE FROM account_cookie_links WHERE follower_account_id=?`, body.FollowerAccountID)
	if _, err := s.db.Exec(`INSERT INTO account_cookie_links (follower_account_id, source_account_id, created_at) VALUES (?,?,?)`,
		body.FollowerAccountID, body.SourceAccountID, now); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.shareCookieFrom(body.SourceAccountID)
	var copiedCookie, copiedAgent string
	var copiedAgentID sql.NullInt64
	_ = s.db.QueryRow(`SELECT COALESCE(cookie, ''), user_agent_id, COALESCE(user_agent, '') FROM accounts WHERE id=?`, body.FollowerAccountID).
		Scan(&copiedCookie, &copiedAgentID, &copiedAgent)
	var agentOut any
	if copiedAgentID.Valid {
		agentOut = copiedAgentID.Int64
	}
	writeJSON(w, 200, map[string]any{
		"status":            "ok",
		"cookie":            copiedCookie,
		"user_agent":        copiedAgent,
		"user_agent_id":     agentOut,
		"copied_user_agent": userAgentID.Valid || strings.TrimSpace(userAgent) != "",
		"source_label":      s.accountShareLabel(body.SourceAccountID),
		"source_account_id": body.SourceAccountID,
	})
}

func (s *server) deleteCookieLink(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.master(w, r); !ok {
		return
	}
	s.ensureCookieLinks()
	followerID, _ := strconv.Atoi(r.URL.Query().Get("follower"))
	if followerID <= 0 {
		writeErr(w, 400, "Choose an account")
		return
	}
	if _, err := s.db.Exec(`DELETE FROM account_cookie_links WHERE follower_account_id=?`, followerID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) master(w http.ResponseWriter, r *http.Request) (operator, bool) {
	op, ok := s.auth(w, r)
	if !ok {
		return op, false
	}
	if op.Role != "master" {
		writeErr(w, 403, "Master only")
		return op, false
	}
	return op, true
}
