package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	accessTokenTTL       = 30 * time.Second
	accessHandoffSkew    = time.Minute
	accessRejectKeep     = 7 * 24 * time.Hour
	msgUnauthorisedSite  = "You are using from UnAuthorised website"
	msgDirectAccess      = "Direct access not allowed"
	msgTooManyAttempts   = "Too many attempts. Wait a minute."
	msgToolUnavailable   = "This tool is not available"
	msgToolNotOnSite     = "This website cannot open "
	msgUsernameSuspended = "This username is suspended"
	msgIPBlocked         = "This IP is blocked"
)

func ensureAccessHandoff(db *sql.DB) {
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS access_sites (
		id INTEGER PRIMARY KEY,
		domain TEXT NOT NULL UNIQUE,
		key_enc TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		note TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	)`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS access_nonces (
		nonce TEXT PRIMARY KEY,
		expires_at TEXT NOT NULL
	)`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS access_rejects (
		id INTEGER PRIMARY KEY,
		website_id INTEGER NOT NULL DEFAULT 0,
		username TEXT NOT NULL DEFAULT '',
		client_ip TEXT NOT NULL DEFAULT '',
		reason TEXT NOT NULL,
		message TEXT NOT NULL,
		site_domain TEXT NOT NULL DEFAULT '',
		tool_slug TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_access_rejects_created ON access_rejects (created_at)`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS access_site_tools (
		site_id INTEGER NOT NULL,
		website_id INTEGER NOT NULL,
		PRIMARY KEY (site_id, website_id)
	)`)
	_, _ = db.Exec(`ALTER TABLE websites ADD COLUMN access_slug TEXT NOT NULL DEFAULT ''`)
}

func normalizeAccessSlug(raw string) string {
	slug := strings.ToLower(strings.TrimSpace(raw))
	if slug == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func accessSlugFree(db *sql.DB, slug string, exceptID int) bool {
	var id int
	err := db.QueryRow(`SELECT id FROM websites WHERE lower(access_slug)=? AND id<>?`, slug, exceptID).Scan(&id)
	return err != nil
}

func normalizeSiteDomain(raw string) string {
	domain := strings.ToLower(strings.TrimSpace(raw))
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "www.")
	if i := strings.IndexAny(domain, "/?#"); i >= 0 {
		domain = domain[:i]
	}
	domain = strings.TrimSuffix(domain, ".")
	return domain
}

func accessCanonical(site, tool, username, ip string, ids []string, ts int64, nonce string) string {
	clean := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		clean = append(clean, id)
	}
	sort.Strings(clean)
	return strings.Join([]string{
		site, tool, username, strings.Join(clean, ","), ip, strconv.FormatInt(ts, 10), nonce,
	}, "\n")
}

func accessSign(key, canonical string) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

func newSiteKey() string {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

type handoffLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func (h *handoffLimiter) allow(ip string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.hits == nil {
		h.hits = map[string][]time.Time{}
	}
	now := time.Now()
	cutoff := now.Add(-time.Minute)
	kept := h.hits[ip][:0]
	for _, at := range h.hits[ip] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	if len(kept) >= 300 {
		h.hits[ip] = kept
		return false
	}
	h.hits[ip] = append(kept, now)
	return true
}

var accessHandoffLimit = &handoffLimiter{}

func (s *server) listAccessSites(w http.ResponseWriter, r *http.Request) {
	s.pruneAccessRejects()
	rows, err := s.db.Query(`SELECT id, domain, enabled, note, created_at FROM access_sites ORDER BY domain`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	type siteRow struct {
		id, enabled           int
		domain, note, created string
	}
	pending := []siteRow{}
	for rows.Next() {
		var row siteRow
		if rows.Scan(&row.id, &row.domain, &row.enabled, &row.note, &row.created) != nil {
			continue
		}
		pending = append(pending, row)
	}
	rows.Close()
	items := []any{}
	for _, row := range pending {
		ids, tools := s.accessSiteTools(row.id)
		items = append(items, map[string]any{
			"id": row.id, "domain": row.domain, "enabled": row.enabled == 1, "note": row.note, "created_at": row.created,
			"website_ids": ids, "tools": tools,
		})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *server) accessSiteTools(siteID int) ([]int, []any) {
	ids := []int{}
	tools := []any{}
	rows, err := s.db.Query(`SELECT t.website_id, COALESCE(w.name, ''), COALESCE(w.access_slug, '')
		FROM access_site_tools t LEFT JOIN websites w ON w.id=t.website_id
		WHERE t.site_id=? ORDER BY w.name`, siteID)
	if err != nil {
		return ids, tools
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var name, slug string
		if rows.Scan(&id, &name, &slug) != nil {
			continue
		}
		ids = append(ids, id)
		tools = append(tools, map[string]any{"id": id, "name": name, "access_slug": slug})
	}
	return ids, tools
}

func (s *server) replaceAccessSiteTools(siteID int, raw []int) {
	_, _ = s.db.Exec(`DELETE FROM access_site_tools WHERE site_id=?`, siteID)
	seen := map[int]bool{}
	for _, id := range raw {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		var exists int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM websites WHERE id=?`, id).Scan(&exists); err != nil || exists == 0 {
			continue
		}
		_, _ = s.db.Exec(`INSERT INTO access_site_tools (site_id, website_id) VALUES (?,?)`, siteID, id)
		if len(seen) >= 200 {
			break
		}
	}
}

func (s *server) saveAccessSite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID         int    `json:"id"`
		Domain     string `json:"domain"`
		Note       string `json:"note"`
		Enabled    *bool  `json:"enabled"`
		WebsiteIDs *[]int `json:"website_ids"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	note := strings.TrimSpace(body.Note)
	if len(note) > 200 {
		note = note[:200]
	}
	if body.ID == 0 {
		domain := normalizeSiteDomain(body.Domain)
		if domain == "" || strings.Contains(domain, " ") {
			writeErr(w, 400, "Enter the aMember domain")
			return
		}
		key := newSiteKey()
		enc, err := encryptAutomation(key)
		if err != nil {
			writeErr(w, 500, "Could not store the site key")
			return
		}
		res, err := s.db.Exec(`INSERT INTO access_sites (domain, key_enc, enabled, note, created_at) VALUES (?,?,1,?,?)`,
			domain, enc, note, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			writeErr(w, 400, "That domain is already on the list")
			return
		}
		id, _ := res.LastInsertId()
		chosen := []int{}
		if body.WebsiteIDs != nil {
			chosen = *body.WebsiteIDs
		}
		s.replaceAccessSiteTools(int(id), chosen)
		ids, tools := s.accessSiteTools(int(id))
		writeJSON(w, 200, map[string]any{
			"id": id, "domain": domain, "enabled": true, "note": note, "key": key,
			"website_ids": ids, "tools": tools,
		})
		return
	}
	enabled := 1
	if body.Enabled != nil && !*body.Enabled {
		enabled = 0
	}
	domain := normalizeSiteDomain(body.Domain)
	if domain == "" {
		_, err := s.db.Exec(`UPDATE access_sites SET enabled=?, note=? WHERE id=?`, enabled, note, body.ID)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	} else {
		_, err := s.db.Exec(`UPDATE access_sites SET domain=?, enabled=?, note=? WHERE id=?`, domain, enabled, note, body.ID)
		if err != nil {
			writeErr(w, 400, "That domain is already on the list")
			return
		}
	}
	if body.WebsiteIDs != nil {
		s.replaceAccessSiteTools(body.ID, *body.WebsiteIDs)
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) deleteAccessSite(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if id == 0 {
		writeErr(w, 400, "Site is required")
		return
	}
	_, _ = s.db.Exec(`DELETE FROM access_site_tools WHERE site_id=?`, id)
	_, _ = s.db.Exec(`DELETE FROM access_sites WHERE id=?`, id)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) accessHandoff(w http.ResponseWriter, r *http.Request) {
	if !accessHandoffLimit.allow(clientIP(r)) {
		writeJSON(w, 429, map[string]string{"message": msgTooManyAttempts})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var body struct {
		Site       string `json:"site"`
		Tool       string `json:"tool"`
		Username   string `json:"username"`
		ProductIDs []any  `json:"product_ids"`
		ClientIP   string `json:"client_ip"`
		Timestamp  int64  `json:"timestamp"`
		Nonce      string `json:"nonce"`
		Signature  string `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.finishHandoff(w, 0, "", "", "", "", "bad_signature", msgDirectAccess)
		return
	}
	site := normalizeSiteDomain(body.Site)
	tool := normalizeAccessSlug(body.Tool)
	username := strings.TrimSpace(body.Username)
	if len(username) > 80 {
		username = username[:80]
	}
	customerIP := strings.TrimSpace(body.ClientIP)
	nonce := strings.TrimSpace(body.Nonce)
	ids := productIDList(body.ProductIDs)
	if site == "" || tool == "" || username == "" || nonce == "" || body.Signature == "" {
		s.finishHandoff(w, 0, site, tool, username, customerIP, "bad_signature", msgDirectAccess)
		return
	}
	var siteID, enabled int
	var keyEnc string
	err := s.db.QueryRow(`SELECT id, key_enc, enabled FROM access_sites WHERE domain=?`, site).Scan(&siteID, &keyEnc, &enabled)
	if err != nil || enabled != 1 {
		s.finishHandoff(w, 0, site, tool, username, customerIP, "not_whitelisted", msgUnauthorisedSite)
		return
	}
	key, err := decryptAutomation(keyEnc)
	if err != nil {
		s.finishHandoff(w, 0, site, tool, username, customerIP, "bad_signature", msgDirectAccess)
		return
	}
	canonical := accessCanonical(site, tool, username, customerIP, ids, body.Timestamp, nonce)
	expected := accessSign(key, canonical)
	skew := time.Now().Unix() - body.Timestamp
	if skew < 0 {
		skew = -skew
	}
	if !hmac.Equal([]byte(expected), []byte(strings.ToLower(strings.TrimSpace(body.Signature)))) || skew > int64(accessHandoffSkew/time.Second) {
		s.finishHandoff(w, 0, site, tool, username, customerIP, "bad_signature", msgDirectAccess)
		return
	}
	nonceUntil := time.Now().Add(2 * time.Minute).UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`INSERT INTO access_nonces (nonce, expires_at) VALUES (?,?)`, nonce, nonceUntil); err != nil {
		s.finishHandoff(w, 0, site, tool, username, customerIP, "replay", msgDirectAccess)
		return
	}
	var websiteID int
	var name, domain string
	err = s.db.QueryRow(`SELECT id, name, domain FROM websites WHERE lower(access_slug)=?`, tool).Scan(&websiteID, &name, &domain)
	if err != nil || websiteID == 0 || strings.TrimSpace(domain) == "" {
		s.finishHandoff(w, 0, site, tool, username, customerIP, "tool_not_found", msgToolUnavailable)
		return
	}
	var allowed int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM access_site_tools WHERE site_id=? AND website_id=?`, siteID, websiteID).Scan(&allowed)
	if allowed == 0 {
		s.finishHandoff(w, websiteID, site, tool, username, customerIP, "tool_not_allowed", msgToolNotOnSite+name)
		return
	}
	matched := s.matchedProduct(websiteID, ids)
	if matched == "" {
		s.finishHandoff(w, websiteID, site, tool, username, customerIP, "no_plan", "You do not have access of "+name)
		return
	}
	if s.userBlock(websiteID, username) != "" {
		s.finishHandoff(w, websiteID, site, tool, username, customerIP, "suspended", msgUsernameSuspended)
		return
	}
	ip := publicIP(customerIP)
	if ip == "" {
		ip = customerIP
	}
	if publicIP(customerIP) != "" {
		var blocked int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM blocked_ips WHERE client_ip=? AND (website_id=? OR website_id=0)`, publicIP(customerIP), websiteID).Scan(&blocked)
		if blocked > 0 {
			s.finishHandoff(w, websiteID, site, tool, username, customerIP, "blocked_ip", msgIPBlocked)
			return
		}
	}
	openURL, err := s.issueAccessToken(websiteID, domain, username, matched, ip)
	if err != nil {
		writeErr(w, 500, "Could not open the tool")
		return
	}
	writeJSON(w, 200, map[string]string{"open_url": openURL})
}

func (s *server) finishHandoff(w http.ResponseWriter, websiteID int, site, tool, username, ip, reason, message string) {
	s.recordAccessReject(websiteID, site, tool, username, ip, reason, message)
	writeJSON(w, 200, map[string]string{"message": message})
}

func (s *server) recordAccessReject(websiteID int, site, tool, username, ip, reason, message string) {
	s.pruneAccessRejects()
	_, _ = s.db.Exec(`INSERT INTO access_rejects (website_id, username, client_ip, reason, message, site_domain, tool_slug, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		websiteID, username, ip, reason, message, site, tool, time.Now().UTC().Format(time.RFC3339))
}

func (s *server) pruneAccessRejects() {
	cutoff := time.Now().Add(-accessRejectKeep).UTC().Format(time.RFC3339)
	_, _ = s.db.Exec(`DELETE FROM access_rejects WHERE created_at < ?`, cutoff)
	_, _ = s.db.Exec(`DELETE FROM access_nonces WHERE expires_at < ?`, time.Now().UTC().Format(time.RFC3339))
}

func (s *server) matchedProduct(websiteID int, ids []string) string {
	for _, id := range ids {
		if s.productMapped(websiteID, id) {
			return id
		}
	}
	return ""
}

func productIDList(raw []any) []string {
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, item := range raw {
		var id string
		switch v := item.(type) {
		case string:
			id = strings.TrimSpace(v)
		case float64:
			id = strconv.FormatInt(int64(v), 10)
		default:
			continue
		}
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Strings(out)
	if len(out) > 50 {
		out = out[:50]
	}
	return out
}

func (s *server) issueAccessToken(websiteID int, domain, username, productID, ip string) (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf)
	now := time.Now().UTC()
	expires := now.Add(accessTokenTTL).Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT INTO access_tokens (token, website_id, username, product_id, client_ip, expires_at, created_at) VALUES (?,?,?,?,?,?,?)`,
		token, websiteID, username, productID, ip, expires, now.Format(time.RFC3339))
	if err != nil {
		return "", err
	}
	scheme := strings.TrimSpace(os.Getenv("TOOL_PUBLIC_SCHEME"))
	if scheme != "https" {
		scheme = "http"
	}
	return scheme + "://" + domain + "/access?token=" + url.QueryEscape(token), nil
}

func (s *server) listAccessRejects(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	s.pruneAccessRejects()
	page, size, q := pageQuery(r)
	where, args := "1=1", []any{}
	if op.Role != "master" {
		where, args = s.scope(op, "r.website_id")
		where += " AND r.website_id<>0"
	}
	if q != "" {
		where += " AND (r.username LIKE ? OR r.message LIKE ? OR r.site_domain LIKE ? OR r.tool_slug LIKE ? OR COALESCE(w.name,'') LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like, like, like, like)
	}
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM access_rejects r LEFT JOIN websites w ON w.id=r.website_id WHERE `+where, args...).Scan(&total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`SELECT r.id, r.website_id, COALESCE(w.name,''), r.username, r.client_ip, r.reason, r.message, r.site_domain, r.tool_slug, r.created_at
		FROM access_rejects r LEFT JOIN websites w ON w.id=r.website_id
		WHERE `+where+` ORDER BY r.id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, websiteID int
		var website, username, ip, reason, message, site, tool, created string
		if rows.Scan(&id, &websiteID, &website, &username, &ip, &reason, &message, &site, &tool, &created) != nil {
			continue
		}
		items = append(items, map[string]any{
			"id": id, "website_id": websiteID, "website_name": website, "username": username,
			"client_ip": ip, "reason": reason, "message": message, "site_domain": site,
			"tool_slug": tool, "created_at": created,
		})
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total, "page": page, "pageSize": size})
}

func (s *server) clearAccessRejects(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	if op.Role == "master" {
		_, _ = s.db.Exec(`DELETE FROM access_rejects`)
		writeJSON(w, 200, map[string]string{"status": "ok"})
		return
	}
	where, args := s.scope(op, "website_id")
	_, _ = s.db.Exec(`DELETE FROM access_rejects WHERE website_id<>0 AND `+where, args...)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
