package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func ensureAhrefsBarTables(db *sql.DB) {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS ahrefs_bar_reseller_settings (
  reseller_id      INTEGER NOT NULL PRIMARY KEY,
  hourly_limit     INTEGER NOT NULL DEFAULT 50,
  daily_limit      INTEGER NOT NULL DEFAULT 150,
  cache_ttl_hours  INTEGER NOT NULL DEFAULT 72,
  sticky_minutes   INTEGER NOT NULL DEFAULT 20,
  refund_on_5xx    INTEGER NOT NULL DEFAULT 1,
  updated_at       TEXT NOT NULL DEFAULT ''
)`,
		`CREATE TABLE IF NOT EXISTS ahrefs_bar_portals (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  reseller_id   INTEGER NOT NULL,
  name          TEXT NOT NULL DEFAULT '',
  site_url      TEXT NOT NULL,
  cookie_name   TEXT NOT NULL DEFAULT 'PHPSESSID',
  check_url     TEXT NOT NULL DEFAULT '',
  product_ids   TEXT NOT NULL DEFAULT '',
  shared_secret TEXT NOT NULL DEFAULT '',
  enabled       INTEGER NOT NULL DEFAULT 1,
  created_at    TEXT NOT NULL DEFAULT '',
  updated_at    TEXT NOT NULL DEFAULT ''
)`,
		`CREATE TABLE IF NOT EXISTS ahrefs_bar_accounts (
  id                 INTEGER PRIMARY KEY AUTOINCREMENT,
  reseller_id        INTEGER NOT NULL,
  name               TEXT NOT NULL,
  access_token_json  TEXT NOT NULL,
  status             TEXT NOT NULL DEFAULT 'active',
  last_error         TEXT NOT NULL DEFAULT '',
  cooldown_until     TEXT NULL,
  last_used_at       TEXT NULL,
  created_at         TEXT NOT NULL DEFAULT '',
  updated_at         TEXT NOT NULL DEFAULT ''
)`,
		`CREATE TABLE IF NOT EXISTS ahrefs_bar_user_limits (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  reseller_id    INTEGER NOT NULL,
  amember_login  TEXT NOT NULL,
  hourly_limit   INTEGER NOT NULL,
  daily_limit    INTEGER NOT NULL,
  is_custom      INTEGER NOT NULL DEFAULT 1,
  updated_at     TEXT NOT NULL DEFAULT ''
)`,
		`CREATE TABLE IF NOT EXISTS ahrefs_bar_usage_log (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  reseller_id    INTEGER NOT NULL,
  amember_login  TEXT NOT NULL DEFAULT '',
  account_id     INTEGER NULL,
  endpoint       TEXT NOT NULL DEFAULT '',
  target         TEXT NOT NULL DEFAULT '',
  credits        INTEGER NOT NULL DEFAULT 0,
  cache_hit      INTEGER NOT NULL DEFAULT 0,
  status_code    INTEGER NOT NULL DEFAULT 0,
  created_at     TEXT NOT NULL DEFAULT ''
)`,
		`CREATE TABLE IF NOT EXISTS ahrefs_bar_endpoint_costs (
  endpoint   TEXT NOT NULL PRIMARY KEY,
  credits    INTEGER NOT NULL DEFAULT 1,
  class      TEXT NOT NULL DEFAULT 'billable'
)`,
		`CREATE TABLE IF NOT EXISTS ahrefs_bar_usage_hourly (
  reseller_id    INTEGER NOT NULL,
  amember_login  TEXT NOT NULL,
  bucket_start   TEXT NOT NULL,
  used           INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (reseller_id, amember_login, bucket_start)
)`,
		`CREATE TABLE IF NOT EXISTS ahrefs_bar_usage_daily (
  reseller_id    INTEGER NOT NULL,
  amember_login  TEXT NOT NULL,
  day_ist        TEXT NOT NULL,
  used           INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (reseller_id, amember_login, day_ist)
)`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			alt := strings.ReplaceAll(q, "INTEGER PRIMARY KEY AUTOINCREMENT", "BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY")
			alt = strings.ReplaceAll(alt, "INTEGER NOT NULL PRIMARY KEY", "INT NOT NULL PRIMARY KEY")
			if _, err2 := db.Exec(alt); err2 != nil {
				msg := strings.ToLower(err.Error() + " " + err2.Error())
				if !strings.Contains(msg, "already") && !strings.Contains(msg, "exists") {
					log.Printf("[DB] ahrefs_bar ensure: %v / %v", err, err2)
				}
			}
		}
	}
	_, _ = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uq_ab_user_limits ON ahrefs_bar_user_limits (reseller_id, amember_login)`)
	ensureAhrefsBarAccountLinks(db)
	for _, row := range []struct {
		ep, class string
		credits   int
	}{
		{"tbGetUserInfoV3", "free", 0},
		{"tbGetStarterPlanPrice", "free", 0},
		{"tbGetHeaderV3", "cheap", 0},
		{"tbGetIconV3", "cheap", 0},
		{"tbGetKeywordStatsV3", "billable", 1},
	} {
		_, _ = db.Exec(`INSERT OR IGNORE INTO ahrefs_bar_endpoint_costs (endpoint, credits, class) VALUES (?,?,?)`, row.ep, row.credits, row.class)
		_, _ = db.Exec(`INSERT IGNORE INTO ahrefs_bar_endpoint_costs (endpoint, credits, class) VALUES (?,?,?)`, row.ep, row.credits, row.class)
	}
}

func (s *server) ahrefsBarScope(op operator, r *http.Request) (where string, args []any, resellerFilter int) {
	where, args = "1=1", []any{}
	resellerFilter = 0
	if op.Role != "master" {
		where += " AND reseller_id=?"
		args = append(args, op.ID)
		resellerFilter = op.ID
		return
	}
	if rid := strings.TrimSpace(r.URL.Query().Get("resellerId")); rid != "" && rid != "0" && rid != "all" {
		if n, err := strconv.Atoi(rid); err == nil && n > 0 {
			where += " AND reseller_id=?"
			args = append(args, n)
			resellerFilter = n
		}
	}
	return
}

func (s *server) ownsAhrefsBarRow(op operator, table string, id int64) bool {
	if op.Role == "master" || id == 0 {
		return true
	}
	var rid int
	if s.db.QueryRow(`SELECT reseller_id FROM `+table+` WHERE id=?`, id).Scan(&rid) != nil {
		return false
	}
	return rid == op.ID
}

func (s *server) resolveAhrefsReseller(op operator, requested int) int {
	if op.Role != "master" {
		return op.ID
	}
	if requested > 0 {
		return requested
	}
	return op.ID
}

// resolveAhrefsResellerRequired forces master to pick a reseller (or self) so tokens never land on the wrong pool.
func (s *server) resolveAhrefsResellerRequired(op operator, requested int) (int, string) {
	if op.Role != "master" {
		return op.ID, ""
	}
	if requested <= 0 {
		return 0, "Choose a reseller"
	}
	var role, status string
	err := s.db.QueryRow(`SELECT role, status FROM operators WHERE id=?`, requested).Scan(&role, &status)
	if err != nil {
		return 0, "Reseller not found"
	}
	if role != "master" && role != "reseller" {
		return 0, "Invalid reseller"
	}
	if status == "suspended" {
		return 0, "Reseller is suspended"
	}
	return requested, ""
}

func (s *server) ahrefsBarResellerLabel(resellerID int64) string {
	if resellerID <= 0 {
		return ""
	}
	var username string
	if s.db.QueryRow(`SELECT username FROM operators WHERE id=?`, resellerID).Scan(&username) != nil {
		return ""
	}
	return strings.TrimSpace(username)
}

// ensureAhrefsBarOperatorFlags adds per-reseller sell/access toggle.
func ensureAhrefsBarOperatorFlags(db *sql.DB) {
	if db == nil {
		return
	}
	// Try common dialects; ignore "duplicate column" errors.
	for _, q := range []string{
		`ALTER TABLE operators ADD COLUMN ahrefs_bar_enabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE operators ADD COLUMN ahrefs_bar_enabled TINYINT(1) NOT NULL DEFAULT 0`,
		`ALTER TABLE operators ADD COLUMN ahrefs_bar_enabled INT NOT NULL DEFAULT 0`,
	} {
		if _, err := db.Exec(q); err == nil {
			break
		}
	}
	_, _ = db.Exec(`UPDATE operators SET ahrefs_bar_enabled=1 WHERE role='master'`)
}

func (s *server) operatorAhrefsBarEnabled(op operator) bool {
	if op.Role == "master" {
		return true
	}
	var n int
	_ = s.db.QueryRow(`SELECT COALESCE(ahrefs_bar_enabled,0) FROM operators WHERE id=?`, op.ID).Scan(&n)
	return n == 1
}

// authAhrefsBar requires login + Ahrefs Bar access (master always; reseller only if enabled).
func (s *server) authAhrefsBar(w http.ResponseWriter, r *http.Request) (operator, bool) {
	op, ok := s.auth(w, r)
	if !ok {
		return op, false
	}
	if !s.operatorAhrefsBarEnabled(op) {
		writeErr(w, 403, "Ahrefs Bar is not enabled for this account")
		return op, false
	}
	return op, true
}

func maskAhrefsToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Prefer bearer-looking token from JSON blob.
	tok := raw
	if strings.HasPrefix(raw, "{") {
		var obj map[string]any
		if json.Unmarshal([]byte(raw), &obj) == nil {
			for _, k := range []string{"accessToken", "access_token", "token", "bearer"} {
				if v, ok := obj[k].(string); ok && strings.TrimSpace(v) != "" {
					tok = v
					break
				}
			}
		}
	}
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return "(set)"
	}
	if len(tok) <= 12 {
		return "(len " + strconv.Itoa(len(tok)) + ")"
	}
	return tok[:6] + "…" + tok[len(tok)-4:]
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// --- Overview ---

func (s *server) ahrefsBarOverview(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	where, args, _ := s.ahrefsBarScope(op, r)

	var accountsTotal, accountsActive, accountsBad, portalsTotal int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM ahrefs_bar_accounts WHERE `+where, args...).Scan(&accountsTotal)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM ahrefs_bar_accounts WHERE `+where+` AND status='active'`, args...).Scan(&accountsActive)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM ahrefs_bar_accounts WHERE `+where+` AND status IN ('bad','cooldown')`, args...).Scan(&accountsBad)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM ahrefs_bar_portals WHERE `+where, args...).Scan(&portalsTotal)

	since := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	usageWhere := where + " AND created_at >= ?"
	usageArgs := append(append([]any{}, args...), since)
	var credits24, requests24, errors24, cacheHits24 int
	_ = s.db.QueryRow(`SELECT COALESCE(SUM(credits),0), COUNT(*), COALESCE(SUM(CASE WHEN status_code>=400 OR status_code=429 THEN 1 ELSE 0 END),0), COALESCE(SUM(cache_hit),0) FROM ahrefs_bar_usage_log WHERE `+usageWhere, usageArgs...).Scan(&credits24, &requests24, &errors24, &cacheHits24)

	writeJSON(w, 200, map[string]any{
		"accounts_total":  accountsTotal,
		"accounts_active": accountsActive,
		"accounts_bad":    accountsBad,
		"portals_total":   portalsTotal,
		"credits_24h":     credits24,
		"requests_24h":    requests24,
		"errors_24h":      errors24,
		"cache_hits_24h":  cacheHits24,
	})
}

// --- Accounts ---

func (s *server) listAhrefsBarAccounts(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	page, size, q := pageQuery(r)
	where, args, _ := s.ahrefsBarScope(op, r)
	if status := r.URL.Query().Get("status"); status != "" && status != "all" {
		where += " AND status=?"
		args = append(args, status)
	}
	if q != "" {
		where += " AND name LIKE ?"
		args = append(args, "%"+q+"%")
	}
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM ahrefs_bar_accounts WHERE `+where, args...).Scan(&total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`
SELECT id, reseller_id, name, access_token_json, status, last_error, COALESCE(cooldown_until,''), COALESCE(last_used_at,''), created_at, updated_at
FROM ahrefs_bar_accounts WHERE `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, rid int64
		var name, tok, status, lastErr, cool, used, created, updated string
		_ = rows.Scan(&id, &rid, &name, &tok, &status, &lastErr, &cool, &used, &created, &updated)
		srcID, srcLabel := s.ahrefsBarLinkInfo(id)
		items = append(items, map[string]any{
			"id": id, "reseller_id": rid, "reseller_username": s.ahrefsBarResellerLabel(rid), "name": name,
			"token_mask": maskAhrefsToken(tok), "has_token": strings.TrimSpace(tok) != "",
			"status": status, "last_error": lastErr,
			"cooldown_until": cool, "last_used_at": used,
			"created_at": created, "updated_at": updated,
			"source_account_id": srcID, "source_label": srcLabel,
		})
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total, "page": page, "pageSize": size})
}

func (s *server) getAhrefsBarAccount(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if id == 0 || !s.ownsAhrefsBarRow(op, "ahrefs_bar_accounts", id) {
		writeErr(w, 404, "Not found")
		return
	}
	var rid int64
	var name, tok, status, lastErr, cool, used, created, updated string
	err := s.db.QueryRow(`
SELECT reseller_id, name, access_token_json, status, last_error, COALESCE(cooldown_until,''), COALESCE(last_used_at,''), created_at, updated_at
FROM ahrefs_bar_accounts WHERE id=?`, id).Scan(&rid, &name, &tok, &status, &lastErr, &cool, &used, &created, &updated)
	if err != nil {
		writeErr(w, 404, "Not found")
		return
	}
	srcID, srcLabel := s.ahrefsBarLinkInfo(id)
	writeJSON(w, 200, map[string]any{
		"id": id, "reseller_id": rid, "reseller_username": s.ahrefsBarResellerLabel(rid), "name": name,
		"access_token_json": tok, "token_mask": maskAhrefsToken(tok),
		"status": status, "last_error": lastErr,
		"cooldown_until": cool, "last_used_at": used,
		"created_at": created, "updated_at": updated,
		"source_account_id": srcID, "source_label": srcLabel,
	})
}

func (s *server) saveAhrefsBarAccount(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	var body struct {
		ID              int64  `json:"id"`
		ResellerID      int    `json:"reseller_id"`
		Name            string `json:"name"`
		AccessTokenJSON string `json:"access_token_json"`
		Status          string `json:"status"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeErr(w, 400, "Enter a name")
		return
	}
	status := strings.TrimSpace(body.Status)
	if status == "" {
		status = "active"
	}
	now := nowRFC3339()

	if body.ID == 0 {
		reseller, errMsg := s.resolveAhrefsResellerRequired(op, body.ResellerID)
		if errMsg != "" {
			writeErr(w, 400, errMsg)
			return
		}
		tok := strings.TrimSpace(body.AccessTokenJSON)
		if tok == "" {
			writeErr(w, 400, "Paste Ahrefs access token / JSON")
			return
		}
		_, err := s.db.Exec(`INSERT INTO ahrefs_bar_accounts (reseller_id, name, access_token_json, status, last_error, created_at, updated_at)
VALUES (?,?,?,?,?,?,?)`, reseller, name, tok, status, "", now, now)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	} else {
		if !s.ownsAhrefsBarRow(op, "ahrefs_bar_accounts", body.ID) {
			writeErr(w, 403, "You cannot manage this item")
			return
		}
		tok := strings.TrimSpace(body.AccessTokenJSON)
		if tok == "" {
			_, err := s.db.Exec(`UPDATE ahrefs_bar_accounts SET name=?, status=?, updated_at=? WHERE id=?`, name, status, now, body.ID)
			if err != nil {
				writeErr(w, 500, err.Error())
				return
			}
		} else {
			_, err := s.db.Exec(`UPDATE ahrefs_bar_accounts SET name=?, access_token_json=?, status=?, last_error='', cooldown_until=NULL, updated_at=? WHERE id=?`,
				name, tok, status, now, body.ID)
			if err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			s.shareAhrefsBarTokenFrom(body.ID)
		}
		if op.Role == "master" && body.ResellerID > 0 {
			reseller, errMsg := s.resolveAhrefsResellerRequired(op, body.ResellerID)
			if errMsg != "" {
				writeErr(w, 400, errMsg)
				return
			}
			_, _ = s.db.Exec(`UPDATE ahrefs_bar_accounts SET reseller_id=? WHERE id=?`, reseller, body.ID)
		}
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) deleteAhrefsBarAccount(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if id == 0 || !s.ownsAhrefsBarRow(op, "ahrefs_bar_accounts", id) {
		writeErr(w, 403, "You cannot manage this item")
		return
	}
	s.ensureAhrefsBarAccountLinks()
	_, _ = s.db.Exec(`DELETE FROM ahrefs_bar_account_links WHERE follower_account_id=? OR source_account_id=?`, id, id)
	_, _ = s.db.Exec(`DELETE FROM ahrefs_bar_accounts WHERE id=?`, id)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// --- Portals ---

func (s *server) listAhrefsBarPortals(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	page, size, q := pageQuery(r)
	where, args, _ := s.ahrefsBarScope(op, r)
	if q != "" {
		where += " AND (name LIKE ? OR site_url LIKE ?)"
		args = append(args, "%"+q+"%", "%"+q+"%")
	}
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM ahrefs_bar_portals WHERE `+where, args...).Scan(&total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`
SELECT id, reseller_id, name, site_url, cookie_name, check_url, product_ids, shared_secret, enabled, created_at, updated_at
FROM ahrefs_bar_portals WHERE `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, rid int64
		var name, site, cookie, check, products, secret, created, updated string
		var enabled int
		_ = rows.Scan(&id, &rid, &name, &site, &cookie, &check, &products, &secret, &enabled, &created, &updated)
		secretMask := ""
		if strings.TrimSpace(secret) != "" {
			secretMask = maskAhrefsToken(secret)
		}
		items = append(items, map[string]any{
			"id": id, "reseller_id": rid, "name": name, "site_url": site,
			"cookie_name": cookie, "check_url": check, "product_ids": products,
			"has_secret": strings.TrimSpace(secret) != "", "secret_mask": secretMask,
			"enabled": enabled == 1, "created_at": created, "updated_at": updated,
		})
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total, "page": page, "pageSize": size})
}

func (s *server) getAhrefsBarPortal(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if id == 0 || !s.ownsAhrefsBarRow(op, "ahrefs_bar_portals", id) {
		writeErr(w, 404, "Not found")
		return
	}
	var rid int64
	var name, site, cookie, check, products, secret, created, updated string
	var enabled int
	err := s.db.QueryRow(`
SELECT reseller_id, name, site_url, cookie_name, check_url, product_ids, shared_secret, enabled, created_at, updated_at
FROM ahrefs_bar_portals WHERE id=?`, id).Scan(&rid, &name, &site, &cookie, &check, &products, &secret, &enabled, &created, &updated)
	if err != nil {
		writeErr(w, 404, "Not found")
		return
	}
	writeJSON(w, 200, map[string]any{
		"id": id, "reseller_id": rid, "name": name, "site_url": site,
		"cookie_name": cookie, "check_url": check, "product_ids": products,
		"shared_secret": secret, "enabled": enabled == 1,
		"created_at": created, "updated_at": updated,
	})
}

func (s *server) saveAhrefsBarPortal(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	var body struct {
		ID           int64  `json:"id"`
		ResellerID   int    `json:"reseller_id"`
		Name         string `json:"name"`
		SiteURL      string `json:"site_url"`
		CookieName   string `json:"cookie_name"`
		CheckURL     string `json:"check_url"`
		ProductIDs   string `json:"product_ids"`
		SharedSecret string `json:"shared_secret"`
		Enabled      *bool  `json:"enabled"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	site := strings.TrimSpace(body.SiteURL)
	if site == "" {
		writeErr(w, 400, "Enter site URL")
		return
	}
	cookie := strings.TrimSpace(body.CookieName)
	if cookie == "" {
		cookie = "PHPSESSID"
	}
	enabled := 1
	if body.Enabled != nil && !*body.Enabled {
		enabled = 0
	}
	reseller := s.resolveAhrefsReseller(op, body.ResellerID)
	now := nowRFC3339()
	name := strings.TrimSpace(body.Name)

	if body.ID == 0 {
		_, err := s.db.Exec(`INSERT INTO ahrefs_bar_portals
(reseller_id, name, site_url, cookie_name, check_url, product_ids, shared_secret, enabled, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?)`,
			reseller, name, site, cookie, strings.TrimSpace(body.CheckURL), strings.TrimSpace(body.ProductIDs),
			strings.TrimSpace(body.SharedSecret), enabled, now, now)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	} else {
		if !s.ownsAhrefsBarRow(op, "ahrefs_bar_portals", body.ID) {
			writeErr(w, 403, "You cannot manage this item")
			return
		}
		secret := strings.TrimSpace(body.SharedSecret)
		if secret == "" {
			_, err := s.db.Exec(`UPDATE ahrefs_bar_portals SET name=?, site_url=?, cookie_name=?, check_url=?, product_ids=?, enabled=?, updated_at=? WHERE id=?`,
				name, site, cookie, strings.TrimSpace(body.CheckURL), strings.TrimSpace(body.ProductIDs), enabled, now, body.ID)
			if err != nil {
				writeErr(w, 500, err.Error())
				return
			}
		} else {
			_, err := s.db.Exec(`UPDATE ahrefs_bar_portals SET name=?, site_url=?, cookie_name=?, check_url=?, product_ids=?, shared_secret=?, enabled=?, updated_at=? WHERE id=?`,
				name, site, cookie, strings.TrimSpace(body.CheckURL), strings.TrimSpace(body.ProductIDs), secret, enabled, now, body.ID)
			if err != nil {
				writeErr(w, 500, err.Error())
				return
			}
		}
		if op.Role == "master" && body.ResellerID > 0 {
			_, _ = s.db.Exec(`UPDATE ahrefs_bar_portals SET reseller_id=? WHERE id=?`, reseller, body.ID)
		}
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) deleteAhrefsBarPortal(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if id == 0 || !s.ownsAhrefsBarRow(op, "ahrefs_bar_portals", id) {
		writeErr(w, 403, "You cannot manage this item")
		return
	}
	_, _ = s.db.Exec(`DELETE FROM ahrefs_bar_portals WHERE id=?`, id)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// --- Defaults / settings ---

func (s *server) getAhrefsBarSettings(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	rid := op.ID
	if op.Role == "master" {
		if n, err := strconv.Atoi(r.URL.Query().Get("resellerId")); err == nil && n > 0 {
			rid = n
		}
	}
	var hourly, daily, ttl, sticky, refund int
	var updated string
	err := s.db.QueryRow(`
SELECT hourly_limit, daily_limit, cache_ttl_hours, sticky_minutes, refund_on_5xx, COALESCE(updated_at,'')
FROM ahrefs_bar_reseller_settings WHERE reseller_id=?`, rid).
		Scan(&hourly, &daily, &ttl, &sticky, &refund, &updated)
	if err == sql.ErrNoRows {
		writeJSON(w, 200, map[string]any{
			"reseller_id": rid, "hourly_limit": 50, "daily_limit": 150,
			"cache_ttl_hours": 72, "sticky_minutes": 20, "refund_on_5xx": true,
			"updated_at": "", "exists": false,
		})
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"reseller_id": rid, "hourly_limit": hourly, "daily_limit": daily,
		"cache_ttl_hours": ttl, "sticky_minutes": sticky, "refund_on_5xx": refund == 1,
		"updated_at": updated, "exists": true,
	})
}

func (s *server) saveAhrefsBarSettings(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	var body struct {
		ResellerID    int   `json:"reseller_id"`
		HourlyLimit   int   `json:"hourly_limit"`
		DailyLimit    int   `json:"daily_limit"`
		CacheTTLHours int   `json:"cache_ttl_hours"`
		StickyMinutes int   `json:"sticky_minutes"`
		RefundOn5xx   *bool `json:"refund_on_5xx"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	rid := s.resolveAhrefsReseller(op, body.ResellerID)
	if body.HourlyLimit < 0 {
		body.HourlyLimit = 0
	}
	if body.DailyLimit < 0 {
		body.DailyLimit = 0
	}
	if body.CacheTTLHours <= 0 {
		body.CacheTTLHours = 72
	}
	if body.StickyMinutes <= 0 {
		body.StickyMinutes = 20
	}
	refund := 1
	if body.RefundOn5xx != nil && !*body.RefundOn5xx {
		refund = 0
	}
	now := nowRFC3339()
	_, err := s.db.Exec(`INSERT INTO ahrefs_bar_reseller_settings
(reseller_id, hourly_limit, daily_limit, cache_ttl_hours, sticky_minutes, refund_on_5xx, updated_at)
VALUES (?,?,?,?,?,?,?)
ON CONFLICT(reseller_id) DO UPDATE SET
 hourly_limit=excluded.hourly_limit, daily_limit=excluded.daily_limit,
 cache_ttl_hours=excluded.cache_ttl_hours, sticky_minutes=excluded.sticky_minutes,
 refund_on_5xx=excluded.refund_on_5xx, updated_at=excluded.updated_at`,
		rid, body.HourlyLimit, body.DailyLimit, body.CacheTTLHours, body.StickyMinutes, refund, now)
	if err != nil {
		// MySQL upsert
		_, err = s.db.Exec(`INSERT INTO ahrefs_bar_reseller_settings
(reseller_id, hourly_limit, daily_limit, cache_ttl_hours, sticky_minutes, refund_on_5xx, updated_at)
VALUES (?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
 hourly_limit=VALUES(hourly_limit), daily_limit=VALUES(daily_limit),
 cache_ttl_hours=VALUES(cache_ttl_hours), sticky_minutes=VALUES(sticky_minutes),
 refund_on_5xx=VALUES(refund_on_5xx), updated_at=VALUES(updated_at)`,
			rid, body.HourlyLimit, body.DailyLimit, body.CacheTTLHours, body.StickyMinutes, refund, now)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) listAhrefsBarCosts(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authAhrefsBar(w, r); !ok {
		return
	}
	rows, err := s.db.Query(`SELECT endpoint, credits, class FROM ahrefs_bar_endpoint_costs ORDER BY class, endpoint`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var ep, class string
		var credits int
		_ = rows.Scan(&ep, &credits, &class)
		items = append(items, map[string]any{"endpoint": ep, "credits": credits, "class": class})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *server) saveAhrefsBarCost(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	if op.Role != "master" {
		writeErr(w, 403, "Master only")
		return
	}
	var body struct {
		Endpoint string `json:"endpoint"`
		Credits  int    `json:"credits"`
		Class    string `json:"class"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	ep := strings.TrimSpace(body.Endpoint)
	if ep == "" {
		writeErr(w, 400, "endpoint required")
		return
	}
	class := strings.TrimSpace(body.Class)
	if class == "" {
		class = "billable"
	}
	_, err := s.db.Exec(`INSERT INTO ahrefs_bar_endpoint_costs (endpoint, credits, class) VALUES (?,?,?)
ON CONFLICT(endpoint) DO UPDATE SET credits=excluded.credits, class=excluded.class`, ep, body.Credits, class)
	if err != nil {
		_, err = s.db.Exec(`INSERT INTO ahrefs_bar_endpoint_costs (endpoint, credits, class) VALUES (?,?,?)
ON DUPLICATE KEY UPDATE credits=VALUES(credits), class=VALUES(class)`, ep, body.Credits, class)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// --- User custom limits ---

func (s *server) listAhrefsBarUserLimits(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	page, size, q := pageQuery(r)
	where, args, _ := s.ahrefsBarScope(op, r)
	if q != "" {
		where += " AND amember_login LIKE ?"
		args = append(args, "%"+q+"%")
	}
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM ahrefs_bar_user_limits WHERE `+where, args...).Scan(&total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`
SELECT id, reseller_id, amember_login, hourly_limit, daily_limit, is_custom, updated_at
FROM ahrefs_bar_user_limits WHERE `+where+` ORDER BY updated_at DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, rid int64
		var login, updated string
		var hourly, daily, custom int
		_ = rows.Scan(&id, &rid, &login, &hourly, &daily, &custom, &updated)
		items = append(items, map[string]any{
			"id": id, "reseller_id": rid, "amember_login": login,
			"hourly_limit": hourly, "daily_limit": daily, "is_custom": custom == 1,
			"updated_at": updated,
		})
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total, "page": page, "pageSize": size})
}

func (s *server) saveAhrefsBarUserLimit(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	var body struct {
		ID           int64  `json:"id"`
		ResellerID   int    `json:"reseller_id"`
		AmemberLogin string `json:"amember_login"`
		HourlyLimit  int    `json:"hourly_limit"`
		DailyLimit   int    `json:"daily_limit"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	login := strings.TrimSpace(body.AmemberLogin)
	if login == "" {
		writeErr(w, 400, "Enter aMember login")
		return
	}
	reseller := s.resolveAhrefsReseller(op, body.ResellerID)
	now := nowRFC3339()
	if body.ID > 0 {
		if !s.ownsAhrefsBarRow(op, "ahrefs_bar_user_limits", body.ID) {
			writeErr(w, 403, "You cannot manage this item")
			return
		}
		_, err := s.db.Exec(`UPDATE ahrefs_bar_user_limits SET amember_login=?, hourly_limit=?, daily_limit=?, is_custom=1, updated_at=? WHERE id=?`,
			login, body.HourlyLimit, body.DailyLimit, now, body.ID)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	} else {
		_, err := s.db.Exec(`INSERT INTO ahrefs_bar_user_limits (reseller_id, amember_login, hourly_limit, daily_limit, is_custom, updated_at)
VALUES (?,?,?,?,1,?)`, reseller, login, body.HourlyLimit, body.DailyLimit, now)
		if err != nil {
			// upsert by unique key
			_, err = s.db.Exec(`UPDATE ahrefs_bar_user_limits SET hourly_limit=?, daily_limit=?, is_custom=1, updated_at=?
WHERE reseller_id=? AND amember_login=?`, body.HourlyLimit, body.DailyLimit, now, reseller, login)
			if err != nil {
				writeErr(w, 500, err.Error())
				return
			}
		}
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) deleteAhrefsBarUserLimit(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if id == 0 || !s.ownsAhrefsBarRow(op, "ahrefs_bar_user_limits", id) {
		writeErr(w, 403, "You cannot manage this item")
		return
	}
	_, _ = s.db.Exec(`DELETE FROM ahrefs_bar_user_limits WHERE id=?`, id)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// --- Usage log ---

func (s *server) listAhrefsBarUsage(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	page, size, q := pageQuery(r)
	view := strings.TrimSpace(r.URL.Query().Get("view"))
	login := strings.TrimSpace(r.URL.Query().Get("login"))
	where, args, _ := s.ahrefsBarScope(op, r)
	if login != "" {
		where += " AND amember_login = ?"
		args = append(args, login)
	}
	if q != "" {
		where += " AND (amember_login LIKE ? OR endpoint LIKE ? OR target LIKE ?)"
		args = append(args, "%"+q+"%", "%"+q+"%", "%"+q+"%")
	}

	// Per-user summary (one row per login) for the usage homepage.
	if view == "users" {
		var total int
		_ = s.db.QueryRow(`
SELECT COUNT(*) FROM (
  SELECT amember_login, reseller_id FROM ahrefs_bar_usage_log WHERE `+where+` GROUP BY amember_login, reseller_id
)`, args...).Scan(&total)
		qArgs := append(append([]any{}, args...), size, (page-1)*size)
		rows, err := s.db.Query(`
SELECT amember_login,
       reseller_id,
       COUNT(*) AS requests,
       COALESCE(SUM(credits),0) AS credits,
       COALESCE(SUM(CASE WHEN cache_hit=1 THEN 1 ELSE 0 END),0) AS cache_hits,
       MAX(created_at) AS last_at
FROM ahrefs_bar_usage_log
WHERE `+where+`
GROUP BY amember_login, reseller_id
ORDER BY last_at DESC
LIMIT ? OFFSET ?`, qArgs...)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		type userAgg struct {
			login, lastAt               string
			rid, requests, credits, hits int
		}
		aggs := make([]userAgg, 0, size)
		for rows.Next() {
			var u userAgg
			_ = rows.Scan(&u.login, &u.rid, &u.requests, &u.credits, &u.hits, &u.lastAt)
			aggs = append(aggs, u)
		}
		rows.Close()
		// Meter lookups must run after rows.Close — SQLite allows one active query per conn.
		items := make([]any, 0, len(aggs))
		for _, u := range aggs {
			hLim, dLim, hUsed, dUsed := s.ahrefsBarMeterFor(u.rid, u.login)
			items = append(items, map[string]any{
				"amember_login": u.login,
				"reseller_id":   u.rid,
				"requests":      u.requests,
				"credits":       u.credits,
				"cache_hits":    u.hits,
				"last_at":       u.lastAt,
				"hourly_used":   hUsed,
				"hourly_limit":  hLim,
				"daily_used":    dUsed,
				"daily_limit":   dLim,
			})
		}
		writeJSON(w, 200, map[string]any{"items": items, "total": total, "page": page, "pageSize": size, "view": "users"})
		return
	}

	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM ahrefs_bar_usage_log WHERE `+where, args...).Scan(&total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`
SELECT id, reseller_id, amember_login, COALESCE(account_id,0), endpoint, target, credits, cache_hit, status_code, created_at
FROM ahrefs_bar_usage_log WHERE `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, rid, accID int64
		var loginName, ep, target, created string
		var credits, cacheHit, status int
		_ = rows.Scan(&id, &rid, &loginName, &accID, &ep, &target, &credits, &cacheHit, &status, &created)
		items = append(items, map[string]any{
			"id": id, "reseller_id": rid, "amember_login": loginName, "account_id": accID,
			"endpoint": ep, "target": shortUsageTarget(target), "credits": credits,
			"cache_hit": cacheHit == 1, "status_code": status, "created_at": created,
		})
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total, "page": page, "pageSize": size})
}

func shortUsageTarget(target string) string {
	t := strings.TrimSpace(target)
	if t == "" {
		return ""
	}
	if len(t) <= 120 {
		return t
	}
	return t[:117] + "..."
}

func ahrefsBarISTDay() string {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		loc = time.FixedZone("IST", 5*3600+30*60)
	}
	return time.Now().In(loc).Format("2006-01-02")
}

// ahrefsBarMeterFor returns limit + used for rolling hour + IST day.
func (s *server) ahrefsBarMeterFor(resellerID int, login string) (hLim, dLim, hUsed, dUsed int) {
	hLim, dLim = 50, 150
	_ = s.db.QueryRow(`SELECT hourly_limit, daily_limit FROM ahrefs_bar_reseller_settings WHERE reseller_id=?`, resellerID).
		Scan(&hLim, &dLim)
	var ch, cd, custom int
	if s.db.QueryRow(`
SELECT hourly_limit, daily_limit, is_custom FROM ahrefs_bar_user_limits
WHERE reseller_id=? AND amember_login=?`, resellerID, login).Scan(&ch, &cd, &custom) == nil && custom == 1 {
		hLim, dLim = ch, cd
	}
	since := time.Now().UTC().Add(-60 * time.Minute).Format(time.RFC3339)
	_ = s.db.QueryRow(`
SELECT COALESCE(SUM(used),0) FROM ahrefs_bar_usage_hourly
WHERE reseller_id=? AND amember_login=? AND bucket_start >= ?`, resellerID, login, since).Scan(&hUsed)
	_ = s.db.QueryRow(`
SELECT COALESCE(used,0) FROM ahrefs_bar_usage_daily
WHERE reseller_id=? AND amember_login=? AND day_ist=?`, resellerID, login, ahrefsBarISTDay()).Scan(&dUsed)
	return
}

func (s *server) resetAhrefsBarMeters(resellerID int, login string) error {
	_, err := s.db.Exec(`DELETE FROM ahrefs_bar_usage_hourly WHERE reseller_id=? AND amember_login=?`, resellerID, login)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM ahrefs_bar_usage_daily WHERE reseller_id=? AND amember_login=?`, resellerID, login)
	return err
}

func (s *server) getAhrefsBarMeters(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	login := strings.TrimSpace(r.URL.Query().Get("login"))
	if login == "" {
		writeErr(w, 400, "login required")
		return
	}
	rid := op.ID
	if op.Role == "master" {
		if n, err := strconv.Atoi(r.URL.Query().Get("resellerId")); err == nil && n > 0 {
			rid = n
		} else {
			_ = s.db.QueryRow(`
SELECT reseller_id FROM ahrefs_bar_usage_log WHERE amember_login=? ORDER BY id DESC LIMIT 1`, login).Scan(&rid)
			if rid == 0 {
				rid = 1
			}
		}
	}
	hLim, dLim, hUsed, dUsed := s.ahrefsBarMeterFor(rid, login)
	writeJSON(w, 200, map[string]any{
		"amember_login":    login,
		"reseller_id":      rid,
		"hourly_used":      hUsed,
		"hourly_limit":     hLim,
		"daily_used":       dUsed,
		"daily_limit":      dLim,
		"hourly_remaining": max(0, hLim-hUsed),
		"daily_remaining":  max(0, dLim-dUsed),
	})
}

func (s *server) resetAhrefsBarMetersHandler(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	var body struct {
		Login      string `json:"login"`
		ResellerID int    `json:"reseller_id"`
	}
	if err := readBody(r, &body); err != nil || strings.TrimSpace(body.Login) == "" {
		writeErr(w, 400, "login required")
		return
	}
	login := strings.TrimSpace(body.Login)
	rid := s.resolveAhrefsReseller(op, body.ResellerID)
	if op.Role == "master" && body.ResellerID <= 0 {
		_ = s.db.QueryRow(`
SELECT reseller_id FROM ahrefs_bar_usage_log WHERE amember_login=? ORDER BY id DESC LIMIT 1`, login).Scan(&rid)
		if rid == 0 {
			rid = 1
		}
	}
	if err := s.resetAhrefsBarMeters(rid, login); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	hLim, dLim, hUsed, dUsed := s.ahrefsBarMeterFor(rid, login)
	writeJSON(w, 200, map[string]any{
		"status":        "ok",
		"amember_login": login,
		"reseller_id":   rid,
		"hourly_used":   hUsed,
		"hourly_limit":  hLim,
		"daily_used":    dUsed,
		"daily_limit":   dLim,
	})
}
