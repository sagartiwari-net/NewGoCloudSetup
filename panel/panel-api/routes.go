package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (s *server) listTools(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.auth(w, r); !ok {
		return
	}
	rows, err := s.db.Query(`SELECT id, name, category, limits_json FROM tools ORDER BY name`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []any{}
	for rows.Next() {
		var id int
		var name, category, limits string
		_ = rows.Scan(&id, &name, &category, &limits)
		list = append(list, map[string]any{"id": id, "name": name, "category": category, "limits": jsonRaw(limits, []any{})})
	}
	writeJSON(w, 200, list)
}

func (s *server) saveTool(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok || op.Role != "master" {
		if ok {
			writeErr(w, 403, "Master only")
		}
		return
	}
	var body struct {
		ID       int    `json:"id"`
		Name     string `json:"name"`
		Category string `json:"category"`
		Limits   any    `json:"limits"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	raw, _ := json.Marshal(body.Limits)
	nextDefs := parseLimitDefs(raw)
	previous := []limitDef{}
	if body.ID != 0 {
		var oldRaw string
		_ = s.db.QueryRow(`SELECT limits_json FROM tools WHERE id=?`, body.ID).Scan(&oldRaw)
		previous = parseLimitDefs([]byte(oldRaw))
	}
	if body.ID == 0 {
		res, err := s.db.Exec(`INSERT INTO tools (name, category, limits_json) VALUES (?,?,?)`, body.Name, body.Category, string(raw))
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		id, _ := res.LastInsertId()
		body.ID = int(id)
	} else {
		_, _ = s.db.Exec(`UPDATE tools SET name=?, category=?, limits_json=? WHERE id=?`, body.Name, body.Category, string(raw), body.ID)
	}
	s.syncToolLimits(body.ID, previous, nextDefs)
	writeJSON(w, 200, map[string]any{"id": body.ID, "name": body.Name, "category": body.Category, "limits": jsonRaw(string(raw), []any{})})
}

func (s *server) listWebsiteOptions(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	where, args := s.scope(op, "w.id")
	if r.URL.Query().Get("exclude") == "recloud" {
		where += ` AND COALESCE(w.kind, '') <> 'recloud'`
	}
	rows, err := s.db.Query(`SELECT w.id, w.tool_id, w.name, w.domain, w.default_limits_json, COALESCE(w.access_slug, '')
		FROM websites w WHERE `+where+` ORDER BY w.name`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, toolID int
		var name, domain, limits, slug string
		if rows.Scan(&id, &toolID, &name, &domain, &limits, &slug) != nil {
			continue
		}
		items = append(items, map[string]any{
			"id": id, "tool_id": toolID, "name": name, "domain": domain,
			"default_limits": jsonRaw(limits, map[string]int{}), "access_slug": slug,
		})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *server) listWebsites(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	page, size, q := pageQuery(r)
	where, args := s.scope(op, "w.id")
	if q != "" {
		where += " AND (w.name LIKE ? OR w.domain LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like)
	}
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM websites w WHERE `+where, args...).Scan(&total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`SELECT w.id, w.tool_id, w.name, w.domain, w.secret_key, w.session_duration, w.default_limits_json, w.session_security_enabled, COALESCE(w.access_slug, '')
		FROM websites w WHERE `+where+` ORDER BY w.name LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		item := scanWebsite(rows)
		item["secret_key"] = ""
		items = append(items, item)
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total, "page": page, "pageSize": size})
}

func (s *server) getWebsite(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	id, _ := strconv.Atoi(r.PathValue("id"))
	if id == 0 || !s.owns(op, id) {
		writeErr(w, 403, "You cannot manage this website")
		return
	}
	var toolID, duration, security int
	var name, domain, secret, limits string
	var slug string
	err := s.db.QueryRow(`SELECT tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, COALESCE(access_slug, '') FROM websites WHERE id=?`, id).
		Scan(&toolID, &name, &domain, &secret, &duration, &limits, &security, &slug)
	if err != nil {
		writeErr(w, 404, "Website not found")
		return
	}
	if op.Role != "master" {
		secret = ""
	}
	writeJSON(w, 200, map[string]any{
		"id": id, "tool_id": toolID, "name": name, "domain": domain, "secret_key": secret,
		"session_duration": duration, "default_limits": jsonRaw(limits, map[string]int{}),
		"session_security_enabled": security == 1, "access_slug": slug,
	})
}

func scanWebsite(rows *sql.Rows) map[string]any {
	var id, toolID, duration, security int
	var name, domain, secret, limits, slug string
	_ = rows.Scan(&id, &toolID, &name, &domain, &secret, &duration, &limits, &security, &slug)
	return map[string]any{
		"id": id, "tool_id": toolID, "name": name, "domain": domain, "secret_key": secret,
		"session_duration": duration, "default_limits": jsonRaw(limits, map[string]int{}),
		"session_security_enabled": security == 1, "access_slug": slug,
	}
}

func jsonRaw(raw string, empty any) any {
	if raw == "" {
		return empty
	}
	var v any
	if json.Unmarshal([]byte(raw), &v) != nil || v == nil {
		return empty
	}
	// tools.limits_json must be a JSON array; coerce {} / scalars so the panel never crashes on .map
	if _, wantArr := empty.([]any); wantArr {
		if _, ok := v.([]any); !ok {
			return empty
		}
	}
	return v
}

func (s *server) scope(op operator, column string) (string, []any) {
	if op.Role == "master" {
		return "1=1", []any{}
	}
	return column + " IN (SELECT website_id FROM operator_websites WHERE operator_id = ?)", []any{op.ID}
}

func (s *server) saveWebsite(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok || op.Role != "master" {
		if ok {
			writeErr(w, 403, "Master only")
		}
		return
	}
	var body struct {
		ID                     int            `json:"id"`
		ToolID                 int            `json:"tool_id"`
		Name                   string         `json:"name"`
		Domain                 string         `json:"domain"`
		SecretKey              string         `json:"secret_key"`
		AccessSlug             string         `json:"access_slug"`
		SessionDuration        int            `json:"session_duration"`
		DefaultLimits          map[string]int `json:"default_limits"`
		SessionSecurityEnabled bool           `json:"session_security_enabled"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	raw, _ := json.Marshal(body.DefaultLimits)
	sec := 0
	if body.SessionSecurityEnabled {
		sec = 1
	}
	slug := normalizeAccessSlug(body.AccessSlug)
	if slug != "" && !accessSlugFree(s.db, slug, body.ID) {
		writeErr(w, 400, "This button name is already used")
		return
	}
	if body.ID == 0 {
		body.SessionDuration = 30
		res, err := s.db.Exec(`INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, access_slug) VALUES (?,?,?,?,?,?,?,?)`,
			body.ToolID, body.Name, body.Domain, body.SecretKey, body.SessionDuration, string(raw), sec, slug)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		id, _ := res.LastInsertId()
		body.ID = int(id)
	} else {
		_, _ = s.db.Exec(`UPDATE websites SET tool_id=?, name=?, domain=?, secret_key=?, session_duration=?, default_limits_json=?, session_security_enabled=?, access_slug=? WHERE id=?`,
			body.ToolID, body.Name, body.Domain, body.SecretKey, body.SessionDuration, string(raw), sec, slug, body.ID)
	}
	s.syncWebsiteLimits(body.ID, body.DefaultLimits)
	s.applySessionDuration(body.ID, body.SessionDuration)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) saveWebsiteSettings(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	var body struct {
		ID              int            `json:"id"`
		SessionDuration int            `json:"session_duration"`
		DefaultLimits   map[string]int `json:"default_limits"`
	}
	if err := readBody(r, &body); err != nil || body.ID == 0 {
		writeErr(w, 400, "Website is required")
		return
	}
	if op.Role != "master" && !s.owns(op, body.ID) {
		writeErr(w, 403, "You cannot manage this website")
		return
	}
	if body.SessionDuration < 1 {
		writeErr(w, 400, "Use at least 1 minute")
		return
	}
	var limitsRaw, currentRaw string
	err := s.db.QueryRow(`SELECT t.limits_json, w.default_limits_json FROM websites w JOIN tools t ON t.id=w.tool_id WHERE w.id=?`, body.ID).Scan(&limitsRaw, &currentRaw)
	if err != nil {
		writeErr(w, 404, "Website not found")
		return
	}
	allowed := map[string]bool{}
	for _, def := range parseLimitDefs([]byte(limitsRaw)) {
		allowed[def.Key] = true
	}
	current := map[string]int{}
	_ = json.Unmarshal([]byte(currentRaw), &current)
	if current == nil {
		current = map[string]int{}
	}
	for key, limit := range body.DefaultLimits {
		if !allowed[key] || limit < -1 {
			continue
		}
		current[key] = limit
	}
	raw, _ := json.Marshal(current)
	if _, err := s.db.Exec(`UPDATE websites SET session_duration=?, default_limits_json=? WHERE id=?`, body.SessionDuration, string(raw), body.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.syncWebsiteLimits(body.ID, current)
	s.applySessionDuration(body.ID, body.SessionDuration)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) saveWebsiteSessions(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	var body struct {
		IDs             []int `json:"website_ids"`
		SessionDuration int   `json:"session_duration"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	if body.SessionDuration < 1 {
		writeErr(w, 400, "Use at least 1 minute")
		return
	}
	if len(body.IDs) == 0 {
		writeErr(w, 400, "Select at least one website")
		return
	}
	updated := 0
	for _, id := range body.IDs {
		if id <= 0 || (op.Role != "master" && !s.owns(op, id)) {
			continue
		}
		var exists int
		if s.db.QueryRow(`SELECT id FROM websites WHERE id=?`, id).Scan(&exists) != nil {
			continue
		}
		if _, err := s.db.Exec(`UPDATE websites SET session_duration=? WHERE id=?`, body.SessionDuration, id); err != nil {
			continue
		}
		s.applySessionDuration(id, body.SessionDuration)
		updated++
	}
	if updated == 0 {
		writeErr(w, 400, "No websites were updated")
		return
	}
	writeJSON(w, 200, map[string]any{"status": "ok", "updated": updated})
}

func (s *server) deleteWebsite(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok || op.Role != "master" {
		if ok {
			writeErr(w, 403, "Master only")
		}
		return
	}
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	_, _ = s.db.Exec(`DELETE FROM websites WHERE id=?`, id)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) listAccounts(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	page, size, q := pageQuery(r)
	where, args := s.scope(op, "a.website_id")
	if id, _ := strconv.Atoi(r.URL.Query().Get("websiteId")); id > 0 {
		where += " AND a.website_id = ?"
		args = append(args, id)
	}
	if q != "" {
		where += " AND (a.name LIKE ? OR w.name LIKE ? OR a.description LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	where += ` AND COALESCE(w.kind, '') <> 'recloud'`
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM accounts a JOIN websites w ON w.id=a.website_id WHERE `+where, args...).Scan(&total)
	args = append(args, size, (page-1)*size)
	// Cookie text stays out of the list. Edit loads one account through GET /api/accounts/{id}.
	rows, err := s.db.Query(`SELECT a.id, a.website_id, w.name, a.name, a.user_agent_id,
		CASE WHEN a.user_agent != '' THEN a.user_agent ELSE COALESCE(ua.user_agent, '') END,
		a.proxy_id,
		CASE WHEN TRIM(COALESCE(p.endpoint, '')) != '' THEN p.endpoint ELSE a.proxy END,
		a.status, a.last_used_at, a.failure_count, a.show_limit, a.description,
		a.automation_task_uid, a.automation_ingest_key, a.cookie_updated_at,
		COALESCE((SELECT status FROM ingest_logs WHERE account_id=a.id ORDER BY id DESC LIMIT 1), '')
		FROM accounts a
		JOIN websites w ON w.id=a.website_id
		LEFT JOIN user_agents ua ON ua.id=a.user_agent_id
		LEFT JOIN proxies p ON p.id=a.proxy_id
		WHERE `+where+` ORDER BY w.name, a.name LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, websiteID, failure, show int
		var uaID, proxyID sql.NullInt64
		var website, name, ua, proxy, status, lastUsed, description, task, ingest, cookieAt, latest string
		if err := rows.Scan(&id, &websiteID, &website, &name, &uaID, &ua, &proxyID, &proxy, &status, &lastUsed, &failure, &show, &description, &task, &ingest, &cookieAt, &latest); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if op.Role != "master" {
			if uaID.Valid && !s.ownsCatalog(op, "user_agents", int(uaID.Int64)) {
				ua = ""
				uaID.Valid = false
			}
			if proxyID.Valid && !s.ownsCatalog(op, "proxies", int(proxyID.Int64)) {
				proxy = ""
				proxyID.Valid = false
			}
		}
		proxy = ""
		if r.URL.Path != "/api/automate-tasks" || op.Role != "master" {
			ingest = ""
		}
		var uaOut any
		var proxyOut any
		if uaID.Valid {
			uaOut = uaID.Int64
		}
		if proxyID.Valid {
			proxyOut = proxyID.Int64
		}
		items = append(items, map[string]any{
			"id": id, "website_id": websiteID, "website_name": website, "name": name, "cookie": "",
			"user_agent_id": uaOut, "user_agent": ua, "proxy_id": proxyOut, "proxy": proxy, "status": status,
			"last_used_at": lastUsed, "failure_count": failure, "show_limit": show == 1, "description": description,
			"automation_task_uid": task, "automation_ingest_key": ingest, "cookie_updated_at": cookieAt,
			"latest_ingest_status": latest,
			"last_status":          latest, "last_time": cookieAt, "last_bytes": 0, "last_error": "",
		})
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total, "page": page, "pageSize": size})
}

func (s *server) accountByID(id int) map[string]any {
	var websiteID, failure, show int
	var uaID, proxyID sql.NullInt64
	var name, cookie, ua, proxy, status, lastUsed, description, task, ingest, cookieAt, website string
	err := s.db.QueryRow(`SELECT a.website_id, w.name, a.name, a.cookie, a.user_agent_id, a.user_agent, a.proxy_id, a.proxy, a.status, a.last_used_at, a.failure_count, a.show_limit, a.description, a.automation_task_uid, a.automation_ingest_key, a.cookie_updated_at
		FROM accounts a JOIN websites w ON w.id=a.website_id WHERE a.id=?`, id).
		Scan(&websiteID, &website, &name, &cookie, &uaID, &ua, &proxyID, &proxy, &status, &lastUsed, &failure, &show, &description, &task, &ingest, &cookieAt)
	if err != nil {
		return nil
	}
	if uaID.Valid {
		_ = s.db.QueryRow(`SELECT user_agent FROM user_agents WHERE id=?`, uaID.Int64).Scan(&ua)
	}
	if proxyID.Valid {
		_ = s.db.QueryRow(`SELECT endpoint FROM proxies WHERE id=?`, proxyID.Int64).Scan(&proxy)
	}
	var latest string
	_ = s.db.QueryRow(`SELECT status FROM ingest_logs WHERE account_id=? ORDER BY id DESC LIMIT 1`, id).Scan(&latest)
	var uaOut any
	var proxyOut any
	if uaID.Valid {
		uaOut = uaID.Int64
	}
	if proxyID.Valid {
		proxyOut = proxyID.Int64
	}
	return map[string]any{
		"id": id, "website_id": websiteID, "website_name": website, "name": name, "cookie": cookie,
		"user_agent_id": uaOut, "user_agent": ua, "proxy_id": proxyOut, "proxy": proxy, "status": status,
		"last_used_at": lastUsed, "failure_count": failure, "show_limit": show == 1, "description": description,
		"automation_task_uid": task, "automation_ingest_key": ingest, "cookie_updated_at": cookieAt,
		"latest_ingest_status": latest,
		"last_status":          latest, "last_time": cookieAt, "last_bytes": 0, "last_error": "",
	}
}

func (s *server) getAccount(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	id, _ := strconv.Atoi(r.PathValue("id"))
	item := s.accountByID(id)
	if item == nil {
		writeJSON(w, 200, nil)
		return
	}
	websiteID, _ := item["website_id"].(int)
	if !s.owns(op, websiteID) {
		writeErr(w, 403, "You cannot manage this account")
		return
	}
	if op.Role != "master" {
		item["automation_ingest_key"] = ""
		item["automation_task_uid"] = ""
		if id, ok := item["proxy_id"].(int64); ok && !s.ownsCatalog(op, "proxies", int(id)) {
			item["proxy_id"] = nil
			item["proxy"] = ""
		}
		if id, ok := item["user_agent_id"].(int64); ok && !s.ownsCatalog(op, "user_agents", int(id)) {
			item["user_agent_id"] = nil
			item["user_agent"] = ""
		}
	}
	writeJSON(w, 200, item)
}

func (s *server) saveAccount(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	var body struct {
		ID          int    `json:"id"`
		WebsiteID   int    `json:"website_id"`
		Name        string `json:"name"`
		Cookie      string `json:"cookie"`
		UserAgentID *int   `json:"user_agent_id"`
		UserAgent   string `json:"user_agent"`
		ProxyID     *int   `json:"proxy_id"`
		Proxy       string `json:"proxy"`
		Status      string `json:"status"`
		ShowLimit   bool   `json:"show_limit"`
		Description string `json:"description"`
		Task        string `json:"automation_task_uid"`
		Ingest      string `json:"automation_ingest_key"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	if strings.TrimSpace(body.Cookie) == "" {
		writeErr(w, 400, "Cookie is required")
		return
	}
	if !s.owns(op, body.WebsiteID) {
		writeErr(w, 403, "You cannot manage this website")
		return
	}
	if s.recloudWebsite(body.WebsiteID) {
		writeErr(w, 400, "Recloud tools are updated on the Recloud page")
		return
	}
	if body.ID != 0 {
		var currentWebsite int
		if err := s.db.QueryRow(`SELECT website_id FROM accounts WHERE id=?`, body.ID).Scan(&currentWebsite); err != nil || !s.owns(op, currentWebsite) {
			writeErr(w, 403, "You cannot manage this account")
			return
		}
	}
	if op.Role != "master" {
		if body.ID != 0 {
			var keptTask, keptIngest string
			_ = s.db.QueryRow(`SELECT automation_task_uid, automation_ingest_key FROM accounts WHERE id=?`, body.ID).Scan(&keptTask, &keptIngest)
			body.Task = keptTask
			body.Ingest = keptIngest
			var curProxy, curUA sql.NullInt64
			var curProxyText, curUAText string
			_ = s.db.QueryRow(`SELECT proxy_id, proxy, user_agent_id, user_agent FROM accounts WHERE id=?`, body.ID).Scan(&curProxy, &curProxyText, &curUA, &curUAText)
			if (body.ProxyID == nil || *body.ProxyID == 0) && curProxy.Valid && !s.ownsCatalog(op, "proxies", int(curProxy.Int64)) {
				kept := int(curProxy.Int64)
				body.ProxyID = &kept
				body.Proxy = curProxyText
			}
			if (body.UserAgentID == nil || *body.UserAgentID == 0) && curUA.Valid && !s.ownsCatalog(op, "user_agents", int(curUA.Int64)) {
				kept := int(curUA.Int64)
				body.UserAgentID = &kept
				body.UserAgent = curUAText
			}
		}
		if body.ProxyID != nil && *body.ProxyID != 0 && !s.ownsCatalog(op, "proxies", *body.ProxyID) {
			writeErr(w, 403, "You cannot use this proxy")
			return
		}
		if body.UserAgentID != nil && *body.UserAgentID != 0 && !s.ownsCatalog(op, "user_agents", *body.UserAgentID) {
			writeErr(w, 403, "You cannot use this user agent")
			return
		}
	}
	show := 0
	if body.ShowLimit {
		show = 1
	}
	// Tools mark accounts logged_out on failover. UI historically used inactive.
	// Cookie paste always revives unless admin explicitly sets inactive/blocked.
	status := strings.ToLower(strings.TrimSpace(body.Status))
	if status != "inactive" && status != "blocked" {
		status = "active"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if body.ID == 0 {
		_, err := s.db.Exec(`INSERT INTO accounts (website_id, name, cookie, user_agent_id, user_agent, proxy_id, proxy, status, show_limit, description, automation_task_uid, automation_ingest_key, cookie_updated_at, last_used_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,'')`,
			body.WebsiteID, body.Name, body.Cookie, body.UserAgentID, body.UserAgent, body.ProxyID, body.Proxy, status, show, body.Description, body.Task, strings.ToLower(body.Ingest), now)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	} else {
		_, err := s.db.Exec(`UPDATE accounts SET website_id=?, name=?, cookie=?, user_agent_id=?, user_agent=?, proxy_id=?, proxy=?, status=?, show_limit=?, description=?, automation_task_uid=?, automation_ingest_key=?, cookie_updated_at=?, failure_count=CASE WHEN ?='active' THEN 0 ELSE failure_count END WHERE id=?`,
			body.WebsiteID, body.Name, body.Cookie, body.UserAgentID, body.UserAgent, body.ProxyID, body.Proxy, status, show, body.Description, body.Task, strings.ToLower(body.Ingest), now, status, body.ID)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		s.shareCookieFrom(body.ID)
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	s.deleteID("accounts")(w, r)
}

func (s *server) bulkDeleteAccounts(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	var body struct {
		IDs []int `json:"ids"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	if len(body.IDs) == 0 {
		writeErr(w, 400, "Select at least one account")
		return
	}
	if len(body.IDs) > 200 {
		writeErr(w, 400, "Select at most 200 accounts")
		return
	}
	seen := map[int]bool{}
	ids := make([]int, 0, len(body.IDs))
	for _, id := range body.IDs {
		if id <= 0 || seen[id] {
			continue
		}
		websiteID, found := s.websiteOf("accounts", id)
		if !found || !s.owns(op, websiteID) {
			writeErr(w, 403, "You cannot manage this website")
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		writeErr(w, 400, "Select at least one account")
		return
	}
	s.ensureCookieLinks()
	deleted := 0
	for _, id := range ids {
		if _, err := s.db.Exec(`DELETE FROM accounts WHERE id=?`, id); err != nil {
			continue
		}
		_, _ = s.db.Exec(`DELETE FROM account_cookie_links WHERE follower_account_id=? OR source_account_id=?`, id, id)
		deleted++
	}
	if deleted == 0 {
		writeErr(w, 400, "No accounts were deleted")
		return
	}
	writeJSON(w, 200, map[string]any{"status": "ok", "deleted": deleted})
}

func (s *server) assignProxy(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	var body struct {
		IDs     []int `json:"ids"`
		ProxyID int   `json:"proxy_id"`
	}
	_ = readBody(r, &body)
	if !s.ownsCatalog(op, "proxies", body.ProxyID) {
		writeErr(w, 403, "You cannot use this proxy")
		return
	}
	for _, id := range body.IDs {
		websiteID, found := s.websiteOf("accounts", id)
		if !found || !s.owns(op, websiteID) {
			writeErr(w, 403, "You cannot manage this website")
			return
		}
		_, _ = s.db.Exec(`UPDATE accounts SET proxy_id=?, proxy='' WHERE id=?`, body.ProxyID, id)
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) assignUserAgent(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	var body struct {
		IDs []int `json:"ids"`
		ID  int   `json:"user_agent_id"`
	}
	_ = readBody(r, &body)
	if !s.ownsCatalog(op, "user_agents", body.ID) {
		writeErr(w, 403, "You cannot use this user agent")
		return
	}
	for _, id := range body.IDs {
		websiteID, found := s.websiteOf("accounts", id)
		if !found || !s.owns(op, websiteID) {
			writeErr(w, 403, "You cannot manage this website")
			return
		}
		_, _ = s.db.Exec(`UPDATE accounts SET user_agent_id=?, user_agent='' WHERE id=?`, body.ID, id)
		s.shareCookieFrom(id)
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) nextAccount(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	websiteID, _ := strconv.Atoi(r.PathValue("id"))
	if !s.owns(op, websiteID) {
		writeErr(w, 403, "You cannot manage this website")
		return
	}
	rows, err := s.db.Query(`SELECT id FROM accounts WHERE website_id=? AND status='active' ORDER BY CASE WHEN last_used_at='' THEN 0 ELSE 1 END, last_used_at ASC, id ASC`, websiteID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	writeJSON(w, 200, map[string]any{
		"website_id": websiteID,
		"order":      ids,
		"note":       "Active accounts only, least recently used first. A failed try does not set status to inactive.",
	})
}

func (s *server) deleteID(table string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		op, ok := s.auth(w, r)
		if !ok {
			return
		}
		id, _ := strconv.Atoi(r.URL.Query().Get("id"))
		if table == "proxies" || table == "user_agents" {
			if !s.ownsCatalog(op, table, id) {
				writeErr(w, 403, "You cannot manage this item")
				return
			}
		}
		if table == "accounts" || table == "products" || table == "live_sessions" {
			websiteID, found := s.websiteOf(table, id)
			if !found || !s.owns(op, websiteID) {
				writeErr(w, 403, "You cannot manage this website")
				return
			}
		}
		if table == "blocked_ips" && op.Role != "master" {
			var websiteID int
			err := s.db.QueryRow(`SELECT website_id FROM blocked_ips WHERE id=?`, id).Scan(&websiteID)
			if err != nil || websiteID == 0 || !s.owns(op, websiteID) {
				writeErr(w, 403, "You cannot manage this website")
				return
			}
		}
		_, _ = s.db.Exec(`DELETE FROM `+table+` WHERE id=?`, id)
		if table == "accounts" {
			s.ensureCookieLinks()
			_, _ = s.db.Exec(`DELETE FROM account_cookie_links WHERE follower_account_id=? OR source_account_id=?`, id, id)
		}
		writeJSON(w, 200, map[string]string{"status": "ok"})
	}
}

func (s *server) listAutomate(w http.ResponseWriter, r *http.Request) { s.listAccounts(w, r) }

func websiteChoiceLabel(name, domain string) string {
	name = strings.TrimSpace(name)
	domain = strings.TrimSpace(domain)
	if domain == "" || strings.EqualFold(domain, name) {
		return name
	}
	return name + " · " + domain
}

func (s *server) productFilter(op operator, websiteID, resellerID int, q string) (string, []any, int, string) {
	where, args := s.scope(op, "p.website_id")
	if resellerID > 0 {
		if op.Role != "master" && resellerID != op.ID {
			return "", nil, 403, "You cannot manage this reseller"
		}
		where += " AND p.website_id IN (SELECT website_id FROM operator_websites WHERE operator_id=?)"
		args = append(args, resellerID)
	}
	if websiteID > 0 {
		if op.Role != "master" && !s.owns(op, websiteID) {
			return "", nil, 403, "You cannot manage this website"
		}
		where += " AND p.website_id=?"
		args = append(args, websiteID)
	}
	if q != "" {
		where += " AND (p.product_name LIKE ? OR p.product_ids_json LIKE ? OR w.name LIKE ? OR w.domain LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like, like, like)
	}
	return where, args, 0, ""
}

func (s *server) listProducts(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	page, size, q := pageQuery(r)
	where, args, status, message := s.productFilter(op, queryInt(r, "websiteId"), queryInt(r, "resellerId"), q)
	if status != 0 {
		writeErr(w, status, message)
		return
	}
	from := ` FROM products p JOIN websites w ON w.id=p.website_id WHERE ` + where
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*)`+from, args...).Scan(&total)
	page = clampPage(page, size, total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`SELECT p.id, p.website_id, w.name, w.domain, p.product_name, p.product_ids_json`+from+` ORDER BY w.name, p.product_name LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, website int
		var name, domain, product, raw string
		_ = rows.Scan(&id, &website, &name, &domain, &product, &raw)
		ids := []string{}
		_ = json.Unmarshal([]byte(raw), &ids)
		items = append(items, map[string]any{
			"id": id, "website_id": website, "website_name": websiteChoiceLabel(name, domain), "product_name": product, "product_ids": ids,
		})
	}
	s.writePage(w, page, size, total, items)
}

func (s *server) saveProduct(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	var body struct {
		ID      int      `json:"id"`
		Website int      `json:"website_id"`
		Name    string   `json:"product_name"`
		IDs     []string `json:"product_ids"`
	}
	if err := readBody(r, &body); err != nil || body.Website == 0 || strings.TrimSpace(body.Name) == "" {
		writeErr(w, 400, "Website and product name are required")
		return
	}
	if op.Role != "master" && !s.owns(op, body.Website) {
		writeErr(w, 403, "You cannot manage this website")
		return
	}
	if body.ID != 0 {
		currentWebsite, found := s.websiteOf("products", body.ID)
		if !found || !s.owns(op, currentWebsite) {
			writeErr(w, 403, "You cannot manage this website")
			return
		}
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, id := range body.IDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		writeErr(w, 400, "Add at least one product id")
		return
	}
	name := strings.TrimSpace(body.Name)
	var match int
	err := s.db.QueryRow(`SELECT id FROM products WHERE website_id=? AND product_name=? AND id<>?`, body.Website, name, body.ID).Scan(&match)
	if err == nil {
		body.ID = match
	}
	if existing := s.productIDTaken(body.Website, ids, body.ID); existing != "" {
		writeErr(w, 400, "Product id "+existing+" is already mapped on this tool")
		return
	}
	raw, _ := json.Marshal(ids)
	if body.ID == 0 {
		_, err = s.db.Exec(`INSERT INTO products (website_id, product_name, product_ids_json) VALUES (?,?,?)`, body.Website, name, string(raw))
	} else {
		_, err = s.db.Exec(`UPDATE products SET website_id=?, product_name=?, product_ids_json=? WHERE id=?`, body.Website, name, string(raw), body.ID)
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) bulkProductIDs(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	var body struct {
		Action     string   `json:"action"`
		MappingIDs []int    `json:"mapping_ids"`
		IDs        []string `json:"product_ids"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	if body.Action != "add" && body.Action != "remove" {
		writeErr(w, 400, "Choose add or remove")
		return
	}
	if len(body.MappingIDs) == 0 {
		writeErr(w, 400, "Select at least one mapping")
		return
	}
	if len(body.MappingIDs) > 200 {
		writeErr(w, 400, "Select at most 200 mappings")
		return
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, id := range body.IDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		writeErr(w, 400, "Enter at least one product id")
		return
	}
	where, args := s.scope(op, "p.website_id")
	marks := make([]string, 0, len(body.MappingIDs))
	for _, id := range body.MappingIDs {
		marks = append(marks, "?")
		args = append(args, id)
	}
	where += " AND p.id IN (" + strings.Join(marks, ",") + ")"
	rows, err := s.db.Query(`SELECT p.id, p.website_id, w.name, w.domain, p.product_ids_json FROM products p JOIN websites w ON w.id=p.website_id WHERE `+where, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	type productRow struct {
		id, website int
		label, raw  string
	}
	found := []productRow{}
	for rows.Next() {
		var item productRow
		var name, domain string
		_ = rows.Scan(&item.id, &item.website, &name, &domain, &item.raw)
		item.label = websiteChoiceLabel(name, domain)
		found = append(found, item)
	}
	updated := 0
	skipped := []string{}
	for _, item := range found {
		current := []string{}
		_ = json.Unmarshal([]byte(item.raw), &current)
		next := append([]string{}, current...)
		if body.Action == "add" {
			for _, id := range ids {
				if containsString(next, id) {
					continue
				}
				if taken := s.productIDTaken(item.website, []string{id}, item.id); taken != "" {
					skipped = append(skipped, item.label+": product id "+id+" is already mapped on this tool")
					continue
				}
				next = append(next, id)
			}
		} else {
			drop := map[string]bool{}
			for _, id := range ids {
				drop[id] = true
			}
			kept := []string{}
			for _, id := range current {
				if !drop[id] {
					kept = append(kept, id)
				}
			}
			if len(kept) == 0 {
				skipped = append(skipped, item.label+": a mapping needs at least one product id")
				continue
			}
			next = kept
		}
		if sameStrings(current, next) {
			continue
		}
		raw, _ := json.Marshal(next)
		if _, err := s.db.Exec(`UPDATE products SET product_ids_json=? WHERE id=?`, string(raw), item.id); err != nil {
			skipped = append(skipped, item.label+": "+err.Error())
			continue
		}
		updated++
	}
	if len(skipped) > 12 {
		extra := len(skipped) - 12
		skipped = append(skipped[:12], fmt.Sprintf("%d more", extra))
	}
	writeJSON(w, 200, map[string]any{"status": "ok", "updated": updated, "skipped": skipped})
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (s *server) productIDTaken(websiteID int, ids []string, except int) string {
	rows, err := s.db.Query(`SELECT id, product_ids_json FROM products WHERE website_id=? AND id<>?`, websiteID, except)
	if err != nil {
		return ""
	}
	defer rows.Close()
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	for rows.Next() {
		var id int
		var raw string
		_ = rows.Scan(&id, &raw)
		current := []string{}
		_ = json.Unmarshal([]byte(raw), &current)
		for _, value := range current {
			if want[value] {
				return value
			}
		}
	}
	return ""
}
func (s *server) blockIP(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	var body struct {
		WebsiteID int    `json:"website_id"`
		IP        string `json:"client_ip"`
		Reason    string `json:"reason"`
	}
	_ = readBody(r, &body)
	if op.Role != "master" {
		if body.WebsiteID == 0 || !s.owns(op, body.WebsiteID) {
			writeErr(w, 403, "You cannot manage this website")
			return
		}
	}
	_, _ = s.db.Exec(`INSERT INTO blocked_ips (website_id, client_ip, reason, created_at) VALUES (?,?,?,?)`, body.WebsiteID, body.IP, body.Reason, time.Now().UTC().Format(time.RFC3339))
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
func (s *server) listProxies(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	page, size, q := pageQuery(r)
	where, args := "1=1", []any{}
	if op.Role != "master" {
		where += " AND operator_id=?"
		args = append(args, op.ID)
	}
	if status := r.URL.Query().Get("status"); status != "" && status != "all" {
		where += " AND status = ?"
		args = append(args, status)
	}
	if q != "" {
		where += " AND name LIKE ?"
		args = append(args, "%"+q+"%")
	}
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM proxies WHERE `+where, args...).Scan(&total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`SELECT id, name, proxy_type, endpoint, status, created_at FROM proxies WHERE `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id int
		var name, kind, endpoint, status, created string
		_ = rows.Scan(&id, &name, &kind, &endpoint, &status, &created)
		items = append(items, map[string]any{"id": id, "name": name, "proxy_type": kind, "endpoint": endpoint, "status": status, "created_at": created})
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total, "page": page, "pageSize": size})
}

func (s *server) saveProxy(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	var body struct {
		ID       int    `json:"id"`
		Name     string `json:"name"`
		Type     string `json:"proxy_type"`
		Endpoint string `json:"endpoint"`
		Status   string `json:"status"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	endpoint, err := normalizeProxyEndpoint(body.Type, body.Endpoint)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ownerID := op.ID
	if op.Role == "master" {
		ownerID = 0
	}
	if body.ID == 0 {
		_, _ = s.db.Exec(`INSERT INTO proxies (name, proxy_type, endpoint, status, created_at, operator_id) VALUES (?,?,?,?,?,?)`, body.Name, body.Type, endpoint, body.Status, time.Now().UTC().Format(time.RFC3339), ownerID)
	} else if !s.ownsCatalog(op, "proxies", body.ID) {
		writeErr(w, 403, "You cannot manage this item")
		return
	} else {
		_, _ = s.db.Exec(`UPDATE proxies SET name=?, proxy_type=?, endpoint=?, status=? WHERE id=?`, body.Name, body.Type, endpoint, body.Status, body.ID)
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func normalizeProxyEndpoint(kind, raw string) (string, error) {
	scheme := ""
	switch strings.ToUpper(strings.TrimSpace(kind)) {
	case "SOCKS5":
		scheme = "socks5"
	case "HTTP":
		scheme = "http"
	case "HTTPS":
		scheme = "https"
	default:
		return "", fmt.Errorf("Choose SOCKS5, HTTP, or HTTPS")
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("Enter an endpoint")
	}
	var host, port, user, pass string
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Hostname() == "" || parsed.Port() == "" {
			return "", fmt.Errorf("Use host:port:user:pass, or host:port")
		}
		host, port = parsed.Hostname(), parsed.Port()
		if parsed.User != nil {
			user = parsed.User.Username()
			pass, _ = parsed.User.Password()
		}
	} else {
		parts := strings.SplitN(raw, ":", 4)
		if len(parts) != 2 && len(parts) != 4 {
			return "", fmt.Errorf("Use host:port:user:pass, or host:port")
		}
		host, port = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if len(parts) == 4 {
			user, pass = parts[2], parts[3]
		}
	}
	n, err := strconv.Atoi(port)
	if host == "" || err != nil || n < 1 || n > 65535 || (user == "") != (pass == "") {
		return "", fmt.Errorf("Use host:port:user:pass, or host:port")
	}
	if user == "" {
		return scheme + "://" + host + ":" + port, nil
	}
	built := url.URL{
		Scheme: scheme,
		User:   url.UserPassword(user, pass),
		Host:   host + ":" + port,
	}
	return built.String(), nil
}

func (s *server) listAgents(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	page, size, q := pageQuery(r)
	where, args := "1=1", []any{}
	if op.Role != "master" {
		where += " AND operator_id=?"
		args = append(args, op.ID)
	}
	if q != "" {
		where += " AND (name LIKE ? OR user_agent LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like)
	}
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM user_agents WHERE `+where, args...).Scan(&total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`SELECT id, name, user_agent, created_at FROM user_agents WHERE `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id int
		var name, ua, created string
		_ = rows.Scan(&id, &name, &ua, &created)
		items = append(items, map[string]any{"id": id, "name": name, "user_agent": ua, "created_at": created})
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total, "page": page, "pageSize": size})
}

func (s *server) saveAgent(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	var body struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
		UA   string `json:"user_agent"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	ownerID := op.ID
	if op.Role == "master" {
		ownerID = 0
	}
	if body.ID == 0 {
		_, _ = s.db.Exec(`INSERT INTO user_agents (name, user_agent, created_at, operator_id) VALUES (?,?,?,?)`, body.Name, body.UA, time.Now().UTC().Format(time.RFC3339), ownerID)
	} else if !s.ownsCatalog(op, "user_agents", body.ID) {
		writeErr(w, 403, "You cannot manage this item")
		return
	} else {
		_, _ = s.db.Exec(`UPDATE user_agents SET name=?, user_agent=? WHERE id=?`, body.Name, body.UA, body.ID)
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
func (s *server) telegramSettings(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	minutes := 60
	var raw string
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE "key"='repeat_minutes'`).Scan(&raw)
	if n, err := strconv.Atoi(raw); err == nil {
		minutes = n
	}
	var token any
	if op.Role == "master" {
		var value string
		_ = s.db.QueryRow(`SELECT value FROM settings WHERE "key"='bot_token'`).Scan(&value)
		if value != "" {
			token = value
		}
	}
	writeJSON(w, 200, map[string]any{"botToken": token, "repeatMinutes": minutes})
}

func (s *server) saveSetting(key string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		op, ok := s.auth(w, r)
		if !ok || op.Role != "master" {
			if ok {
				writeErr(w, 403, "Master only")
			}
			return
		}
		var body struct {
			Token string `json:"token"`
		}
		_ = readBody(r, &body)
		_, _ = s.db.Exec(`INSERT INTO settings ("key", value) VALUES (?,?) ON DUPLICATE KEY UPDATE value=VALUES(value)`, key, body.Token)
		writeJSON(w, 200, map[string]string{"status": "ok"})
	}
}

func (s *server) saveRepeat(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok || op.Role != "master" {
		if ok {
			writeErr(w, 403, "Master only")
		}
		return
	}
	var body struct {
		Minutes int `json:"minutes"`
	}
	_ = readBody(r, &body)
	_, _ = s.db.Exec(`UPDATE settings SET value=? WHERE "key"='repeat_minutes'`, strconv.Itoa(body.Minutes))
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) telegramTemplates(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.auth(w, r); !ok {
		return
	}
	var logout, spam, switched, network string
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE "key"='logout_template'`).Scan(&logout)
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE "key"='spam_template'`).Scan(&spam)
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE "key"='switch_template'`).Scan(&switched)
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE "key"='network_template'`).Scan(&network)
	if switched == "" {
		switched = "{tool} {account} {problem}"
	}
	if network == "" {
		network = "{tool} {username} {type} detected"
	}
	writeJSON(w, 200, map[string]string{"logout": logout, "spam": spam, "switch": switched, "network": network})
}

func (s *server) saveTemplates(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok || op.Role != "master" {
		if ok {
			writeErr(w, 403, "Master only")
		}
		return
	}
	var body struct {
		Logout  string `json:"logout"`
		Spam    string `json:"spam"`
		Switch  string `json:"switch"`
		Network string `json:"network"`
	}
	_ = readBody(r, &body)
	_, _ = s.db.Exec(`INSERT INTO settings ("key", value) VALUES ('logout_template', ?) ON DUPLICATE KEY UPDATE value=VALUES(value)`, body.Logout)
	_, _ = s.db.Exec(`INSERT INTO settings ("key", value) VALUES ('spam_template', ?) ON DUPLICATE KEY UPDATE value=VALUES(value)`, body.Spam)
	_, _ = s.db.Exec(`INSERT INTO settings ("key", value) VALUES ('switch_template', ?) ON DUPLICATE KEY UPDATE value=VALUES(value)`, body.Switch)
	_, _ = s.db.Exec(`INSERT INTO settings ("key", value) VALUES ('network_template', ?) ON DUPLICATE KEY UPDATE value=VALUES(value)`, body.Network)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
