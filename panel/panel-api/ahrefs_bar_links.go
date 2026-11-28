package main

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
)

func ensureAhrefsBarAccountLinks(db *sql.DB) {
	if db == nil {
		return
	}
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS ahrefs_bar_account_links (
  follower_account_id INTEGER PRIMARY KEY,
  source_account_id   INTEGER NOT NULL,
  created_at          TEXT NOT NULL DEFAULT ''
)`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS ahrefs_bar_account_links (
  follower_account_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  source_account_id   BIGINT UNSIGNED NOT NULL,
  created_at          VARCHAR(64) NOT NULL DEFAULT ''
)`)
}

func (s *server) ensureAhrefsBarAccountLinks() {
	ensureAhrefsBarAccountLinks(s.db)
}

func (s *server) ahrefsBarAccountLabel(id int64) string {
	var name string
	var rid int64
	if s.db.QueryRow(`SELECT name, reseller_id FROM ahrefs_bar_accounts WHERE id=?`, id).Scan(&name, &rid) != nil {
		return ""
	}
	label := strings.TrimSpace(name)
	if u := s.ahrefsBarResellerLabel(rid); u != "" {
		label = u + " — " + label
	}
	return label
}

func (s *server) ahrefsBarLinkInfo(followerID int64) (sourceID int64, sourceLabel string) {
	s.ensureAhrefsBarAccountLinks()
	_ = s.db.QueryRow(`SELECT source_account_id FROM ahrefs_bar_account_links WHERE follower_account_id=?`, followerID).Scan(&sourceID)
	if sourceID > 0 {
		sourceLabel = s.ahrefsBarAccountLabel(sourceID)
	}
	return
}

func (s *server) shareAhrefsBarTokenFrom(sourceID int64) {
	if sourceID <= 0 || s.db == nil {
		return
	}
	s.ensureAhrefsBarAccountLinks()
	var tok string
	if s.db.QueryRow(`SELECT access_token_json FROM ahrefs_bar_accounts WHERE id=?`, sourceID).Scan(&tok) != nil {
		return
	}
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return
	}
	rows, err := s.db.Query(`SELECT follower_account_id FROM ahrefs_bar_account_links WHERE source_account_id=?`, sourceID)
	if err != nil {
		return
	}
	defer rows.Close()
	now := nowRFC3339()
	for rows.Next() {
		var fid int64
		if rows.Scan(&fid) != nil || fid <= 0 || fid == sourceID {
			continue
		}
		_, _ = s.db.Exec(`UPDATE ahrefs_bar_accounts SET access_token_json=?, last_error='', cooldown_until=NULL, updated_at=? WHERE id=?`,
			tok, now, fid)
	}
}

func (s *server) ahrefsBarAccountLinkSources(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	if op.Role != "master" {
		writeErr(w, 403, "Master only")
		return
	}
	s.ensureAhrefsBarAccountLinks()
	accountID, _ := strconv.ParseInt(r.URL.Query().Get("accountId"), 10, 64)
	if accountID <= 0 {
		writeErr(w, 400, "Choose an account")
		return
	}
	var exists int64
	if s.db.QueryRow(`SELECT id FROM ahrefs_bar_accounts WHERE id=?`, accountID).Scan(&exists) != nil {
		writeErr(w, 404, "Account not found")
		return
	}
	var usedBy int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM ahrefs_bar_account_links WHERE source_account_id=?`, accountID).Scan(&usedBy)
	srcID, srcLabel := s.ahrefsBarLinkInfo(accountID)

	rows, err := s.db.Query(`
SELECT a.id, a.name, a.reseller_id
FROM ahrefs_bar_accounts a
WHERE a.id<>?
  AND a.id NOT IN (SELECT follower_account_id FROM ahrefs_bar_account_links)
ORDER BY a.reseller_id, a.name`, accountID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	sources := []map[string]any{}
	for rows.Next() {
		var id, rid int64
		var name string
		if rows.Scan(&id, &name, &rid) != nil {
			continue
		}
		label := strings.TrimSpace(name)
		if u := s.ahrefsBarResellerLabel(rid); u != "" {
			label = u + " — " + label
		}
		sources = append(sources, map[string]any{"id": id, "label": label})
	}
	var current any
	if srcID > 0 {
		current = map[string]any{"source_account_id": srcID, "source_label": srcLabel}
	}
	writeJSON(w, 200, map[string]any{
		"is_source": usedBy > 0,
		"current":   current,
		"sources":   sources,
	})
}

func (s *server) saveAhrefsBarAccountLink(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	if op.Role != "master" {
		writeErr(w, 403, "Master only")
		return
	}
	s.ensureAhrefsBarAccountLinks()
	var body struct {
		FollowerAccountID int64 `json:"follower_account_id"`
		SourceAccountID   int64 `json:"source_account_id"`
	}
	if err := readBody(r, &body); err != nil || body.FollowerAccountID <= 0 || body.SourceAccountID <= 0 {
		writeErr(w, 400, "Choose an account")
		return
	}
	if body.FollowerAccountID == body.SourceAccountID {
		writeErr(w, 400, "Choose a different account")
		return
	}
	var followerExists, sourceExists int64
	if s.db.QueryRow(`SELECT id FROM ahrefs_bar_accounts WHERE id=?`, body.FollowerAccountID).Scan(&followerExists) != nil {
		writeErr(w, 404, "Account not found")
		return
	}
	if s.db.QueryRow(`SELECT id FROM ahrefs_bar_accounts WHERE id=?`, body.SourceAccountID).Scan(&sourceExists) != nil {
		writeErr(w, 404, "Account not found")
		return
	}
	var follows int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM ahrefs_bar_account_links WHERE follower_account_id=?`, body.SourceAccountID).Scan(&follows)
	if follows > 0 {
		writeErr(w, 400, "That account already uses another token")
		return
	}
	var usedBy int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM ahrefs_bar_account_links WHERE source_account_id=?`, body.FollowerAccountID).Scan(&usedBy)
	if usedBy > 0 {
		writeErr(w, 400, "Other accounts already use this token")
		return
	}
	var tok string
	if s.db.QueryRow(`SELECT access_token_json FROM ahrefs_bar_accounts WHERE id=?`, body.SourceAccountID).Scan(&tok) != nil {
		writeErr(w, 404, "Account not found")
		return
	}
	if strings.TrimSpace(tok) == "" {
		writeErr(w, 400, "That account has no token yet")
		return
	}
	now := nowRFC3339()
	_, _ = s.db.Exec(`DELETE FROM ahrefs_bar_account_links WHERE follower_account_id=?`, body.FollowerAccountID)
	_, err := s.db.Exec(`INSERT INTO ahrefs_bar_account_links (follower_account_id, source_account_id, created_at) VALUES (?,?,?)`,
		body.FollowerAccountID, body.SourceAccountID, now)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_, _ = s.db.Exec(`UPDATE ahrefs_bar_accounts SET access_token_json=?, last_error='', cooldown_until=NULL, updated_at=? WHERE id=?`,
		tok, now, body.FollowerAccountID)
	writeJSON(w, 200, map[string]any{
		"status":              "ok",
		"follower_account_id": body.FollowerAccountID,
		"source_account_id":   body.SourceAccountID,
		"source_label":        s.ahrefsBarAccountLabel(body.SourceAccountID),
		"token_mask":          maskAhrefsToken(tok),
	})
}

func (s *server) deleteAhrefsBarAccountLink(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	if op.Role != "master" {
		writeErr(w, 403, "Master only")
		return
	}
	s.ensureAhrefsBarAccountLinks()
	followerID, _ := strconv.ParseInt(r.URL.Query().Get("follower"), 10, 64)
	if followerID <= 0 {
		writeErr(w, 400, "Choose an account")
		return
	}
	_, _ = s.db.Exec(`DELETE FROM ahrefs_bar_account_links WHERE follower_account_id=?`, followerID)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
