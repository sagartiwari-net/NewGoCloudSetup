package main

import (
	"database/sql"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Recloud websites use websites.kind = "recloud".
// A running proxy must read these on each request, not from config.json:
//
//	SELECT upstream_url FROM websites WHERE domain = ? AND kind = 'recloud'
//	SELECT cookie FROM accounts WHERE website_id = ? AND status = 'active' AND TRIM(cookie) <> '' ORDER BY id ASC LIMIT 1

func (s *server) recloudWebsite(websiteID int) bool {
	var kind string
	err := s.db.QueryRow(`SELECT COALESCE(kind, '') FROM websites WHERE id=?`, websiteID).Scan(&kind)
	return err == nil && kind == "recloud"
}

func (s *server) listRecloud(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	where, args := s.scope(op, "w.id")
	rows, err := s.db.Query(`SELECT w.id, t.name, w.domain, w.upstream_url,
		(SELECT a.id FROM accounts a WHERE a.website_id=w.id ORDER BY a.id ASC LIMIT 1),
		COALESCE((SELECT a.cookie_updated_at FROM accounts a WHERE a.website_id=w.id ORDER BY a.id ASC LIMIT 1), '')
		FROM websites w JOIN tools t ON t.id=w.tool_id
		WHERE w.kind='recloud' AND `+where+` ORDER BY t.name, w.domain`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id int
		var accountID sql.NullInt64
		var name, domain, upstream, updated string
		if rows.Scan(&id, &name, &domain, &upstream, &accountID, &updated) != nil {
			continue
		}
		var account any
		if accountID.Valid {
			account = accountID.Int64
		} else {
			account = 0
		}
		items = append(items, map[string]any{
			"website_id": id, "tool_name": name, "domain": domain,
			"upstream_url": upstream, "account_id": account, "cookie_updated_at": updated,
		})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *server) getRecloud(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	id, _ := strconv.Atoi(r.PathValue("id"))
	if id == 0 || !s.owns(op, id) {
		writeErr(w, 403, "You cannot manage this tool")
		return
	}
	item := s.recloudByID(id)
	if item == nil {
		writeErr(w, 404, "Recloud tool not found")
		return
	}
	writeJSON(w, 200, item)
}

func (s *server) recloudByID(id int) map[string]any {
	var accountID sql.NullInt64
	var name, domain, upstream, cookie, updated string
	err := s.db.QueryRow(`SELECT t.name, w.domain, w.upstream_url, a.id, COALESCE(a.cookie, ''), COALESCE(a.cookie_updated_at, '')
		FROM websites w JOIN tools t ON t.id=w.tool_id
		LEFT JOIN accounts a ON a.id=(SELECT id FROM accounts WHERE website_id=w.id ORDER BY id ASC LIMIT 1)
		WHERE w.id=? AND w.kind='recloud'`, id).Scan(&name, &domain, &upstream, &accountID, &cookie, &updated)
	if err != nil {
		return nil
	}
	account := 0
	if accountID.Valid {
		account = int(accountID.Int64)
	}
	return map[string]any{
		"website_id": id, "tool_name": name, "domain": domain,
		"upstream_url": upstream, "account_id": account, "cookie": cookie, "cookie_updated_at": updated,
	}
}

func (s *server) saveRecloud(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	var body struct {
		WebsiteID   int    `json:"website_id"`
		UpstreamURL string `json:"upstream_url"`
		Cookie      string `json:"cookie"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	if body.WebsiteID == 0 || !s.owns(op, body.WebsiteID) || !s.recloudWebsite(body.WebsiteID) {
		writeErr(w, 403, "You cannot manage this tool")
		return
	}
	upstream, err := cleanUpstream(body.UpstreamURL)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	cookie := strings.TrimSpace(body.Cookie)
	if cookie == "" {
		writeErr(w, 400, "Cookie is required")
		return
	}
	if _, err := s.db.Exec(`UPDATE websites SET upstream_url=? WHERE id=? AND kind='recloud'`, upstream, body.WebsiteID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	accountID, err := s.saveRecloudCookie(body.WebsiteID, cookie)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.shareCookieFrom(accountID)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func cleanUpstream(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return "", errUpstream
	}
	return raw, nil
}

var errUpstream = stringError("Enter an http or https upstream URL")

type stringError string

func (e stringError) Error() string { return string(e) }

func (s *server) saveRecloudCookie(websiteID int, cookie string) (int, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	var accountID int
	err := s.db.QueryRow(`SELECT id FROM accounts WHERE website_id=? ORDER BY id ASC LIMIT 1`, websiteID).Scan(&accountID)
	if err == sql.ErrNoRows {
		var name string
		_ = s.db.QueryRow(`SELECT t.name FROM websites w JOIN tools t ON t.id=w.tool_id WHERE w.id=?`, websiteID).Scan(&name)
		if strings.TrimSpace(name) == "" {
			name = "recloud"
		}
		res, err := s.db.Exec(`INSERT INTO accounts (website_id, name, cookie, user_agent_id, user_agent, proxy_id, proxy, status, show_limit, description, automation_task_uid, automation_ingest_key, cookie_updated_at, last_used_at)
			VALUES (?,?,?,NULL,'',NULL,'','active',1,'','','',?,'')`, websiteID, name, cookie, now)
		if err != nil {
			return 0, err
		}
		id, _ := res.LastInsertId()
		return int(id), nil
	}
	if err != nil {
		return 0, err
	}
	_, err = s.db.Exec(`UPDATE accounts SET cookie=?, status='active', cookie_updated_at=?, failure_count=0 WHERE id=?`, cookie, now, accountID)
	return accountID, err
}
