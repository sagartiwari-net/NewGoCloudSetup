package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func queryInt(r *http.Request, key string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(key))
	return n
}

func clampPage(page, size, total int) int {
	if total < 1 || page < 1 {
		return 1
	}
	pages := (total + size - 1) / size
	if page > pages {
		return pages
	}
	return page
}

func (s *server) writePage(w http.ResponseWriter, page, size, total int, items []any) {
	if items == nil {
		items = []any{}
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total, "page": page, "pageSize": size})
}

func istLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return time.FixedZone("IST", 5*3600+30*60)
	}
	return loc
}

func periodStartIST(resetDays int, now time.Time) time.Time {
	if resetDays < 1 {
		resetDays = 1
	}
	local := now.In(istLocation())
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	return day.AddDate(0, 0, -(resetDays - 1))
}

func parseExpireDate(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	loc := istLocation()
	if t, err := time.ParseInLocation("2006-01-02", raw, loc); err == nil {
		return t, true
	}
	if t, ok := parseTime(raw); ok {
		t = t.In(loc)
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc), true
	}
	return time.Time{}, false
}

func (s *server) refreshLimits() {
	now := time.Now()
	today := periodStartIST(1, now)
	rows, err := s.db.Query(`SELECT u.id, u.website_id, COALESCE(u.custom_limit_expire_at,''), COALESCE(w.default_limits_json,'{}')
		FROM panel_users u JOIN websites w ON w.id=u.website_id
		WHERE TRIM(COALESCE(u.custom_limit_expire_at,'')) != ''`)
	type expiredUser struct {
		id, website int
		expire      string
		defaults    map[string]int
	}
	expired := []expiredUser{}
	if err == nil {
		for rows.Next() {
			var item expiredUser
			var rawLimits string
			_ = rows.Scan(&item.id, &item.website, &item.expire, &rawLimits)
			when, ok := parseExpireDate(item.expire)
			if ok && today.After(when) {
				item.defaults = map[string]int{}
				_ = json.Unmarshal([]byte(rawLimits), &item.defaults)
				expired = append(expired, item)
			}
		}
		rows.Close()
	}
	for _, user := range expired {
		meters, mErr := s.db.Query(`SELECT key FROM user_meters WHERE user_id=? AND is_custom=1`, user.id)
		keys := []string{}
		if mErr == nil {
			for meters.Next() {
				var key string
				_ = meters.Scan(&key)
				keys = append(keys, key)
			}
			meters.Close()
		}
		fresh := time.Now().UTC().Format(time.RFC3339)
		for _, key := range keys {
			limit := user.defaults[key]
			_, _ = s.db.Exec(`UPDATE user_meters SET "limit"=?, is_custom=0, used=0, period_start=? WHERE user_id=? AND key=?`, limit, fresh, user.id, key)
		}
		_, _ = s.db.Exec(`UPDATE user_meters SET used=0, period_start=? WHERE user_id=? AND is_custom=0`, fresh, user.id)
		_, _ = s.db.Exec(`UPDATE panel_users SET custom_limit_expire_at=NULL WHERE id=?`, user.id)
	}

	meterRows, err := s.db.Query(`SELECT m.id, m.reset_days, COALESCE(m.period_start,''), u.username, u.website_id, m.key
		FROM user_meters m JOIN panel_users u ON u.id=m.user_id`)
	type meterPeriod struct {
		id, days, website    int
		start, username, key string
	}
	meters := []meterPeriod{}
	if err == nil {
		for meterRows.Next() {
			var item meterPeriod
			_ = meterRows.Scan(&item.id, &item.days, &item.start, &item.username, &item.website, &item.key)
			meters = append(meters, item)
		}
		meterRows.Close()
	}
	for _, meter := range meters {
		start := periodStartIST(meter.days, now).UTC().Format(time.RFC3339)
		if meter.start == start || meter.start > start {
			continue
		}
		var used int
		_ = s.db.QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM usage_events WHERE website_id=? AND username=? AND limit_key=? AND timestamp>=?`,
			meter.website, meter.username, meter.key, start).Scan(&used)
		_, _ = s.db.Exec(`UPDATE user_meters SET used=?, period_start=? WHERE id=?`, used, start, meter.id)
	}
}

func (s *server) purgeOldRows() {
	s.refreshLimits()
	now := time.Now().UTC()
	_, _ = s.db.Exec(`DELETE FROM live_sessions WHERE expires_at <= ?`, now.Format(time.RFC3339))
	rows, err := s.db.Query(`SELECT id, reset_days, timestamp FROM usage_events`)
	staleUsage := []int{}
	if err == nil {
		for rows.Next() {
			var id, days int
			var ts string
			_ = rows.Scan(&id, &days, &ts)
			if parsed, ok := parseTime(ts); ok && days > 0 && now.Sub(parsed) >= time.Duration(days)*24*time.Hour {
				staleUsage = append(staleUsage, id)
			}
		}
		rows.Close()
	}
	for _, id := range staleUsage {
		_, _ = s.db.Exec(`DELETE FROM usage_events WHERE id=?`, id)
	}
	cutoff := now.Add(-7 * 24 * time.Hour)
	extCutoff := now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	_, _ = s.db.Exec(`DELETE FROM extension_events WHERE created_at < ?`, extCutoff)
	for _, table := range []string{"violations", "security_events"} {
		scan, err := s.db.Query(`SELECT id, created_at FROM ` + table)
		if err != nil {
			continue
		}
		stale := []int{}
		for scan.Next() {
			var id int
			var ts string
			_ = scan.Scan(&id, &ts)
			if parsed, ok := parseTime(ts); ok && parsed.Before(cutoff) {
				stale = append(stale, id)
			}
		}
		scan.Close()
		for _, id := range stale {
			_, _ = s.db.Exec(`DELETE FROM `+table+` WHERE id=?`, id)
		}
	}
}

func parseTime(value string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func (s *server) userWhere(op operator, r *http.Request, paths bool) (string, []any) {
	where, args := s.scope(op, "u.website_id")
	if id := queryInt(r, "toolId"); id > 0 {
		where += " AND w.tool_id = ?"
		args = append(args, id)
	}
	if id := queryInt(r, "resellerId"); id > 0 {
		where += " AND u.website_id IN (SELECT website_id FROM operator_websites WHERE operator_id=?)"
		args = append(args, id)
	}
	if q := strings.TrimSpace(r.URL.Query().Get("query")); q != "" {
		like := "%" + q + "%"
		where += " AND (u.username LIKE ? OR w.name LIKE ? OR t.name LIKE ?"
		args = append(args, like, like, like)
		if paths {
			where += " OR EXISTS (SELECT 1 FROM usage_events e WHERE e.username=u.username AND e.website_id=u.website_id AND (e.target_path LIKE ? OR e.action LIKE ?))"
			args = append(args, like, like)
		}
		where += ")"
	}
	if r.URL.Query().Get("custom") == "true" {
		where += " AND (COALESCE(u.custom_limit_expire_at,'') <> '' OR EXISTS (SELECT 1 FROM user_meters m WHERE m.user_id=u.id AND m.is_custom=1))"
	}
	return where, args
}

func normalizeLimitVisibility(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "show", "hide":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func (s *server) userJSON(id int) map[string]any {
	var websiteID int
	var website, tool, username, status string
	var expire sql.NullString
	var visibility string
	err := s.db.QueryRow(`SELECT u.website_id, w.name, t.name, u.username, u.status, u.custom_limit_expire_at, COALESCE(u.limit_visibility, '')
		FROM panel_users u JOIN websites w ON w.id=u.website_id JOIN tools t ON t.id=w.tool_id WHERE u.id=?`, id).
		Scan(&websiteID, &website, &tool, &username, &status, &expire, &visibility)
	if err != nil {
		return nil
	}
	rows, err := s.db.Query(`SELECT key, label, reset_days, used, "limit", is_custom FROM user_meters WHERE user_id=? ORDER BY id`, id)
	meters := []any{}
	if err == nil {
		for rows.Next() {
			var reset, used, limit, custom int
			var key, label string
			_ = rows.Scan(&key, &label, &reset, &used, &limit, &custom)
			meters = append(meters, map[string]any{
				"key": key, "label": label, "reset_days": reset, "used": used, "limit": limit, "is_custom": custom == 1,
			})
		}
		rows.Close()
	}
	var expireVal any
	if expire.Valid && expire.String != "" {
		expireVal = expire.String
	}
	return map[string]any{
		"id": id, "website_id": websiteID, "website_name": website, "tool_name": tool,
		"username": username, "status": status, "custom_limit_expire_at": expireVal,
		"limit_visibility": normalizeLimitVisibility(visibility), "meters": meters,
	}
}

func (s *server) listUsers(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	s.purgeOldRows()
	page, size, _ := pageQuery(r)
	where, args := s.userWhere(op, r, false)
	// Limits page: only websites whose tool defines at least one limit.
	where += ` AND COALESCE(json_array_length(t.limits_json), 0) > 0`
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM panel_users u JOIN websites w ON w.id=u.website_id JOIN tools t ON t.id=w.tool_id WHERE `+where, args...).Scan(&total)
	page = clampPage(page, size, total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`SELECT u.id FROM panel_users u JOIN websites w ON w.id=u.website_id JOIN tools t ON t.id=w.tool_id WHERE `+where+` ORDER BY u.username LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ids := []int{}
	for rows.Next() {
		var id int
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	items := []any{}
	for _, id := range ids {
		if item := s.userJSON(id); item != nil {
			items = append(items, item)
		}
	}
	s.writePage(w, page, size, total, items)
}

func (s *server) listDirectoryUsers(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	page, size, q := pageQuery(r)
	scope, scopeArgs := s.scope(op, "u.website_id")
	toolID := queryInt(r, "toolId")
	resellerID := queryInt(r, "resellerId")

	where := scope
	args := append([]any{}, scopeArgs...)
	if toolID > 0 {
		where += " AND w.tool_id=?"
		args = append(args, toolID)
	}
	if resellerID > 0 {
		where += " AND u.website_id IN (SELECT website_id FROM operator_websites WHERE operator_id=?)"
		args = append(args, resellerID)
	}
	if q != "" {
		like := "%" + q + "%"
		where += " AND (u.username LIKE ? OR w.name LIKE ? OR t.name LIKE ? OR w.domain LIKE ?)"
		args = append(args, like, like, like, like)
	}

	from := ` FROM panel_users u
		JOIN websites w ON w.id=u.website_id
		JOIN tools t ON t.id=w.tool_id
		WHERE ` + where

	var total int
	_ = s.db.QueryRow(`SELECT COUNT(DISTINCT u.username)`+from, args...).Scan(&total)
	page = clampPage(page, size, total)

	listArgs := append(append([]any{}, args...), size, (page-1)*size)
	rows, err := s.db.Query(`SELECT u.username,
		COUNT(DISTINCT u.website_id),
		GROUP_CONCAT(DISTINCT w.id),
		GROUP_CONCAT(DISTINCT w.name),
		GROUP_CONCAT(DISTINCT t.name),
		GROUP_CONCAT(DISTINCT u.status),
		MIN(u.website_id)
		`+from+` GROUP BY u.username ORDER BY u.username LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()

	type dirRow struct {
		username   string
		siteCount  int
		siteIDs    string
		siteNames  string
		toolNames  string
		statuses   string
		primaryID  int
	}
	pending := []dirRow{}
	names := []string{}
	for rows.Next() {
		var row dirRow
		if rows.Scan(&row.username, &row.siteCount, &row.siteIDs, &row.siteNames, &row.toolNames, &row.statuses, &row.primaryID) != nil {
			continue
		}
		pending = append(pending, row)
		names = append(names, row.username)
	}

	loginStats := s.directoryLoginStats(op, names)
	items := []any{}
	for _, row := range pending {
		status := "active"
		parts := uniqueCSV(row.statuses)
		if len(parts) == 1 {
			status = parts[0]
		} else if len(parts) > 1 {
			status = "mixed"
		}
		stats := loginStats[row.username]
		items = append(items, map[string]any{
			"username":           row.username,
			"status":             status,
			"website_count":      row.siteCount,
			"website_ids":        splitIntCSV(row.siteIDs),
			"website_names":      uniqueCSV(row.siteNames),
			"tool_names":         uniqueCSV(row.toolNames),
			"primary_website_id": row.primaryID,
			"login_count":        stats.count,
			"distinct_ips":       stats.ips,
			"last_login_at":      stats.last,
		})
	}
	s.writePage(w, page, size, total, items)
}

type directoryLoginStat struct {
	count int
	ips   int
	last  string
}

func (s *server) directoryLoginStats(op operator, usernames []string) map[string]directoryLoginStat {
	out := map[string]directoryLoginStat{}
	if len(usernames) == 0 {
		return out
	}
	scope, scopeArgs := s.scope(op, "e.website_id")
	placeholders := make([]string, len(usernames))
	args := append([]any{}, scopeArgs...)
	for i, name := range usernames {
		placeholders[i] = "?"
		args = append(args, name)
	}
	rows, err := s.db.Query(`SELECT e.username, COUNT(*), COUNT(DISTINCT CASE WHEN TRIM(e.client_ip)<>'' THEN e.client_ip END), COALESCE(MAX(e.logged_in_at),'')
		FROM login_events e
		WHERE `+scope+` AND e.username IN (`+strings.Join(placeholders, ",")+`)
		GROUP BY e.username`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var name, last string
		var count, ips int
		if rows.Scan(&name, &count, &ips, &last) == nil {
			out[name] = directoryLoginStat{count: count, ips: ips, last: last}
		}
	}
	return out
}

func uniqueCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	return out
}

func splitIntCSV(raw string) []int {
	out := []int{}
	for _, part := range uniqueCSV(raw) {
		n, err := strconv.Atoi(part)
		if err == nil {
			out = append(out, n)
		}
	}
	return out
}

func parseLimitDefs(raw []byte) []limitDef {
	var defs []limitDef
	_ = json.Unmarshal(raw, &defs)
	return defs
}

func (s *server) syncToolLimits(toolID int, previous, next []limitDef) {
	old := map[string]limitDef{}
	for _, def := range previous {
		old[def.Key] = def
	}
	rows, err := s.db.Query(`SELECT id, default_limits_json FROM websites WHERE tool_id=?`, toolID)
	if err != nil {
		return
	}
	type siteLimits struct {
		id       int
		defaults map[string]int
	}
	sites := []siteLimits{}
	for rows.Next() {
		var id int
		var raw string
		_ = rows.Scan(&id, &raw)
		defaults := map[string]int{}
		_ = json.Unmarshal([]byte(raw), &defaults)
		sites = append(sites, siteLimits{id: id, defaults: defaults})
	}
	rows.Close()
	for _, def := range next {
		if _, existed := old[def.Key]; !existed {
			for i := range sites {
				if _, present := sites[i].defaults[def.Key]; !present {
					sites[i].defaults[def.Key] = -1
				}
				encoded, _ := json.Marshal(sites[i].defaults)
				_, _ = s.db.Exec(`UPDATE websites SET default_limits_json=? WHERE id=?`, string(encoded), sites[i].id)
				s.ensureDefaultMeter(sites[i].id, def, sites[i].defaults[def.Key])
			}
			continue
		}
		_, _ = s.db.Exec(`UPDATE user_meters SET label=?, reset_days=? WHERE key=? AND user_id IN (SELECT u.id FROM panel_users u JOIN websites w ON w.id=u.website_id WHERE w.tool_id=?)`,
			def.Label, def.ResetDays, def.Key, toolID)
	}
}

func (s *server) ensureDefaultMeter(websiteID int, def limitDef, limit int) {
	rows, err := s.db.Query(`SELECT id FROM panel_users WHERE website_id=?`, websiteID)
	if err != nil {
		return
	}
	ids := []int{}
	for rows.Next() {
		var id int
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		var n int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM user_meters WHERE user_id=? AND key=?`, id, def.Key).Scan(&n)
		if n > 0 {
			continue
		}
		_, _ = s.db.Exec(`INSERT INTO user_meters (user_id, key, label, reset_days, used, "limit", is_custom) VALUES (?,?,?,?,0,?,0)`,
			id, def.Key, def.Label, def.ResetDays, limit)
	}
}

func (s *server) applySessionDuration(websiteID, minutes int) {
	if minutes < 1 {
		minutes = 30
	}
	rows, err := s.db.Query(`SELECT id, created_at FROM live_sessions WHERE website_id=?`, websiteID)
	if err != nil {
		return
	}
	type stamped struct {
		id      int
		created string
	}
	list := []stamped{}
	for rows.Next() {
		var item stamped
		_ = rows.Scan(&item.id, &item.created)
		list = append(list, item)
	}
	rows.Close()
	for _, item := range list {
		created, ok := parseTime(item.created)
		if !ok {
			continue
		}
		expires := created.Add(time.Duration(minutes) * time.Minute).UTC().Format(time.RFC3339)
		_, _ = s.db.Exec(`UPDATE live_sessions SET expires_at=? WHERE id=?`, expires, item.id)
	}
}

func (s *server) syncWebsiteLimits(websiteID int, defaults map[string]int) {
	var raw string
	_ = s.db.QueryRow(`SELECT t.limits_json FROM websites w JOIN tools t ON t.id=w.tool_id WHERE w.id=?`, websiteID).Scan(&raw)
	defs := parseLimitDefs([]byte(raw))
	known := map[string]limitDef{}
	for _, def := range defs {
		known[def.Key] = def
	}
	for key, limit := range defaults {
		if limit < -1 {
			continue
		}
		_, _ = s.db.Exec(`UPDATE user_meters SET "limit"=? WHERE key=? AND is_custom=0 AND user_id IN (SELECT id FROM panel_users WHERE website_id=?)`,
			limit, key, websiteID)
		if def, ok := known[key]; ok {
			s.ensureDefaultMeter(websiteID, def, limit)
		}
	}
}

func (s *server) saveUser(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	var body struct {
		ID         int    `json:"id"`
		Website    int    `json:"website_id"`
		Username   string `json:"username"`
		Status     string `json:"status"`
		Expire     string `json:"custom_limit_expire_at"`
		Visibility string `json:"limit_visibility"`
		Meters     []struct {
			Key   string `json:"key"`
			Limit int    `json:"limit"`
			Used  int    `json:"used"`
		} `json:"meters"`
	}
	if err := readBody(r, &body); err != nil || strings.TrimSpace(body.Username) == "" || body.Website == 0 {
		writeErr(w, 400, "Invalid body")
		return
	}
	if op.Role != "master" && !s.owns(op, body.Website) {
		writeErr(w, 403, "You cannot manage this website")
		return
	}
	defaults := map[string]int{}
	var raw string
	_ = s.db.QueryRow(`SELECT default_limits_json FROM websites WHERE id=?`, body.Website).Scan(&raw)
	_ = json.Unmarshal([]byte(raw), &defaults)
	var expire any
	if strings.TrimSpace(body.Expire) != "" {
		expire = strings.TrimSpace(body.Expire)
	}
	visibility := normalizeLimitVisibility(body.Visibility)
	tx, err := s.db.Begin()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	userID := body.ID
	if userID == 0 {
		res, err := tx.Exec(`INSERT INTO panel_users (website_id, username, status, custom_limit_expire_at, limit_visibility) VALUES (?,?,?,?,?)`,
			body.Website, strings.TrimSpace(body.Username), body.Status, expire, visibility)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		id, _ := res.LastInsertId()
		userID = int(id)
	} else {
		_, err = tx.Exec(`UPDATE panel_users SET website_id=?, username=?, status=?, custom_limit_expire_at=?, limit_visibility=? WHERE id=?`,
			body.Website, strings.TrimSpace(body.Username), body.Status, expire, visibility, userID)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		_, _ = tx.Exec(`DELETE FROM user_meters WHERE user_id=?`, userID)
	}
	labels := map[string]struct {
		Label string
		Days  int
	}{}
	var limitsRaw string
	_ = tx.QueryRow(`SELECT t.limits_json FROM websites w JOIN tools t ON t.id=w.tool_id WHERE w.id=?`, body.Website).Scan(&limitsRaw)
	var defs []limitDef
	_ = json.Unmarshal([]byte(limitsRaw), &defs)
	for _, def := range defs {
		labels[def.Key] = struct {
			Label string
			Days  int
		}{def.Label, def.ResetDays}
	}
	for _, meter := range body.Meters {
		meta := labels[meter.Key]
		if meta.Label == "" {
			meta.Label = meter.Key
			meta.Days = 1
		}
		custom := 0
		if meter.Limit != defaults[meter.Key] || expire != nil {
			custom = 1
		}
		_, _ = tx.Exec(`INSERT INTO user_meters (user_id, key, label, reset_days, used, "limit", is_custom) VALUES (?,?,?,?,?,?,?)`,
			userID, meter.Key, meta.Label, meta.Days, meter.Used, meter.Limit, custom)
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) owns(op operator, websiteID int) bool {
	if op.Role == "master" {
		return true
	}
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM operator_websites WHERE operator_id=? AND website_id=?`, op.ID, websiteID).Scan(&n)
	return n > 0
}

func (s *server) deleteUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.auth(w, r); !ok {
		return
	}
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	_, _ = s.db.Exec(`DELETE FROM user_meters WHERE user_id=?`, id)
	_, _ = s.db.Exec(`DELETE FROM panel_users WHERE id=?`, id)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) resetUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.auth(w, r); !ok {
		return
	}
	id, _ := strconv.Atoi(r.PathValue("id"))
	_, _ = s.db.Exec(`UPDATE user_meters SET used=0 WHERE user_id=?`, id)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) listSessions(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	page, size, q := pageQuery(r)
	where, args := s.scope(op, "s.website_id")
	if id := queryInt(r, "toolId"); id > 0 {
		where += " AND w.tool_id=?"
		args = append(args, id)
	}
	if id := queryInt(r, "resellerId"); id > 0 {
		where += " AND s.website_id IN (SELECT website_id FROM operator_websites WHERE operator_id=?)"
		args = append(args, id)
	}
	if q != "" {
		where += " AND (s.username LIKE ? OR s.client_ip LIKE ? OR w.name LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	where += " AND s.expires_at > ?"
	args = append(args, time.Now().UTC().Format(time.RFC3339))
	s.purgeOldRows()
	from := ` FROM live_sessions s JOIN websites w ON w.id=s.website_id LEFT JOIN accounts a ON a.id=s.assigned_account_id WHERE ` + where
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*)`+from, args...).Scan(&total)
	page = clampPage(page, size, total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`SELECT s.id, s.website_id, w.name, s.session_token, s.username, s.client_ip, s.expires_at, s.created_at, s.assigned_account_id, COALESCE(a.name,'Auto')`+from+` ORDER BY s.id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, websiteID, accountID int
		var website, token, username, ip, expires, created, account string
		_ = rows.Scan(&id, &websiteID, &website, &token, &username, &ip, &expires, &created, &accountID, &account)
		if accountID == 0 {
			account = "Auto"
		}
		items = append(items, map[string]any{
			"id": id, "website_id": websiteID, "website_name": website, "session_token": token,
			"username": username, "client_ip": ip, "expires_at": expires, "created_at": created,
			"assigned_account_id": accountID, "assigned_account_name": account,
		})
	}
	s.writePage(w, page, size, total, items)
}

func (s *server) assignSession(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.auth(w, r); !ok {
		return
	}
	var body struct {
		SessionID int `json:"session_id"`
		AccountID int `json:"account_id"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	if body.AccountID == 0 {
		_, _ = s.db.Exec(`UPDATE live_sessions SET assigned_account_id=0 WHERE id=?`, body.SessionID)
	} else {
		_, _ = s.db.Exec(`UPDATE live_sessions SET assigned_account_id=? WHERE id=? AND website_id=(SELECT website_id FROM accounts WHERE id=?)`, body.AccountID, body.SessionID, body.AccountID)
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) endSession(w http.ResponseWriter, r *http.Request) {
	s.deleteID("live_sessions")(w, r)
}

func (s *server) openSession(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.auth(w, r); !ok {
		return
	}
	var body struct {
		WebsiteID   int    `json:"website_id"`
		Username    string `json:"username"`
		ClientIP    string `json:"client_ip"`
		Fingerprint string `json:"fingerprint"`
		Token       string `json:"session_token"`
		Expires     string `json:"expires_at"`
	}
	if err := readBody(r, &body); err != nil || body.WebsiteID == 0 || strings.TrimSpace(body.Username) == "" {
		writeErr(w, 400, "Invalid body")
		return
	}
	_, _ = s.db.Exec(`DELETE FROM live_sessions WHERE username=? AND website_id=?`, strings.TrimSpace(body.Username), body.WebsiteID)
	if body.Expires == "" {
		body.Expires = time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	}
	if body.Token == "" {
		body.Token = "token_live_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT INTO live_sessions (website_id, session_token, username, client_ip, fingerprint, expires_at, created_at, assigned_account_id) VALUES (?,?,?,?,?,?,?,0)`,
		body.WebsiteID, body.Token, strings.TrimSpace(body.Username), body.ClientIP, body.Fingerprint, body.Expires, now)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.noteIP(body.WebsiteID, body.Username, body.ClientIP, "login_ip", body.Fingerprint)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) checkSession(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.auth(w, r); !ok {
		return
	}
	var body struct {
		SessionID   int    `json:"session_id"`
		Fingerprint string `json:"fingerprint"`
		ClientIP    string `json:"client_ip"`
	}
	if err := readBody(r, &body); err != nil || body.SessionID == 0 {
		writeErr(w, 400, "Invalid body")
		return
	}
	var token, fingerprint, username, ip string
	var websiteID int
	err := s.db.QueryRow(`SELECT session_token, fingerprint, username, client_ip, website_id FROM live_sessions WHERE id=?`, body.SessionID).
		Scan(&token, &fingerprint, &username, &ip, &websiteID)
	if err != nil {
		writeJSON(w, 200, map[string]any{"kept": false})
		return
	}
	if fingerprint != "" && fingerprint != body.Fingerprint {
		_, _ = s.db.Exec(`DELETE FROM live_sessions WHERE id=? OR (session_token<>'' AND session_token=?)`, body.SessionID, token)
		writeJSON(w, 200, map[string]any{"kept": false})
		return
	}
	if body.ClientIP != "" {
		_, _ = s.db.Exec(`UPDATE live_sessions SET client_ip=?, fingerprint=CASE WHEN fingerprint='' THEN ? ELSE fingerprint END WHERE id=?`, body.ClientIP, body.Fingerprint, body.SessionID)
		if body.ClientIP != ip {
			s.noteIP(websiteID, username, body.ClientIP, "ip_seen", "fingerprint matched")
		}
	}
	writeJSON(w, 200, map[string]any{"kept": true})
}

func (s *server) noteIP(websiteID int, username, ip, eventType, details string) {
	if ip == "" {
		return
	}
	_, _ = s.db.Exec(`INSERT INTO security_events (website_id, username, client_ip, event_type, attempted_url, details, user_agent, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		websiteID, username, ip, eventType, "", details, "", time.Now().UTC().Format(time.RFC3339))
}

func (s *server) listQuota(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	s.purgeOldRows()
	page, size, _ := pageQuery(r)
	where, args := s.userWhere(op, r, true)
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM panel_users u JOIN websites w ON w.id=u.website_id JOIN tools t ON t.id=w.tool_id WHERE `+where, args...).Scan(&total)
	page = clampPage(page, size, total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`SELECT u.id FROM panel_users u JOIN websites w ON w.id=u.website_id JOIN tools t ON t.id=w.tool_id WHERE `+where+` ORDER BY u.username LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ids := []int{}
	for rows.Next() {
		var id int
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	items := []any{}
	for _, id := range ids {
		user := s.userJSON(id)
		if user == nil {
			continue
		}
		items = append(items, map[string]any{"user": user, "events": s.usageEvents(user["username"].(string), user["website_id"].(int))})
	}
	s.writePage(w, page, size, total, items)
}

func (s *server) usageEvents(username string, websiteID int) []any {
	rows, err := s.db.Query(`SELECT e.id, e.website_id, w.name, t.name, e.username, e.limit_key, e.limit_label, e.reset_days, e.action, e.target_path, e.amount, e.timestamp
		FROM usage_events e JOIN websites w ON w.id=e.website_id JOIN tools t ON t.id=w.tool_id
		WHERE e.username=? AND e.website_id=? ORDER BY e.timestamp DESC`, username, websiteID)
	events := []any{}
	if err != nil {
		return events
	}
	defer rows.Close()
	for rows.Next() {
		var id, website, reset, amount int
		var site, tool, user, key, label, action, path, ts string
		_ = rows.Scan(&id, &website, &site, &tool, &user, &key, &label, &reset, &action, &path, &amount, &ts)
		events = append(events, map[string]any{
			"id": id, "website_id": website, "website_name": site, "tool_name": tool, "username": user,
			"limit_key": key, "limit_label": label, "reset_days": reset, "action": action, "target_path": path, "amount": amount, "timestamp": ts,
		})
	}
	return events
}

func (s *server) listLogins(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	page, size, q := pageQuery(r)
	where, args := s.scope(op, "e.website_id")
	if id := queryInt(r, "websiteId"); id > 0 {
		where += " AND e.website_id=?"
		args = append(args, id)
	}
	if q != "" {
		where += " AND (e.username LIKE ? OR e.client_ip LIKE ? OR w.domain LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	from := ` FROM login_events e JOIN websites w ON w.id=e.website_id WHERE ` + where
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*)`+from, args...).Scan(&total)
	page = clampPage(page, size, total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`SELECT e.id, e.website_id, w.domain, e.username, e.client_ip, e.user_agent, e.logged_in_at`+from+` ORDER BY e.logged_in_at DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, website int
		var domain, username, ip, ua, at string
		_ = rows.Scan(&id, &website, &domain, &username, &ip, &ua, &at)
		items = append(items, map[string]any{"id": id, "website_id": website, "domain": domain, "username": username, "client_ip": ip, "user_agent": ua, "logged_in_at": at})
	}
	s.writePage(w, page, size, total, items)
}

func (s *server) clearLogins(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	if op.Role == "master" {
		_, _ = s.db.Exec(`DELETE FROM login_events`)
	} else {
		_, _ = s.db.Exec(`DELETE FROM login_events WHERE website_id IN (SELECT website_id FROM operator_websites WHERE operator_id=?)`, op.ID)
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) listSwitches(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	page, size, q := pageQuery(r)
	where, args := s.scope(op, "e.website_id")
	if id := queryInt(r, "websiteId"); id > 0 {
		where += " AND e.website_id=?"
		args = append(args, id)
	}
	if q != "" {
		where += " AND (e.username LIKE ? OR w.domain LIKE ? OR e.reason LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	from := ` FROM switch_events e JOIN websites w ON w.id=e.website_id WHERE ` + where
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*)`+from, args...).Scan(&total)
	page = clampPage(page, size, total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`SELECT e.id, e.website_id, w.domain, e.username, e.from_account_name, e.to_account_name, e.reason, e.switched_at`+from+` ORDER BY e.switched_at DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, website int
		var domain, username, from, to, reason, at string
		_ = rows.Scan(&id, &website, &domain, &username, &from, &to, &reason, &at)
		items = append(items, map[string]any{
			"id": id, "website_id": website, "domain": domain, "username": username,
			"from_account_name": from, "to_account_name": to, "reason": reason, "switched_at": at,
		})
	}
	s.writePage(w, page, size, total, items)
}

func (s *server) clearSwitches(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	if op.Role == "master" {
		_, _ = s.db.Exec(`DELETE FROM switch_events`)
	} else {
		_, _ = s.db.Exec(`DELETE FROM switch_events WHERE website_id IN (SELECT website_id FROM operator_websites WHERE operator_id=?)`, op.ID)
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) listLogouts(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	page, size, q := pageQuery(r)
	where, args := s.scope(op, "e.website_id")
	if id := queryInt(r, "websiteId"); id > 0 {
		where += " AND e.website_id=?"
		args = append(args, id)
	}
	if q != "" {
		where += " AND (e.username LIKE ? OR w.domain LIKE ? OR e.account_name LIKE ? OR e.reason LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like, like, like)
	}
	from := ` FROM logout_events e JOIN websites w ON w.id=e.website_id WHERE ` + where
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*)`+from, args...).Scan(&total)
	page = clampPage(page, size, total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`SELECT e.id, e.website_id, w.domain, e.username, e.account_name, e.next_account_name, e.reason, e.client_ip, e.created_at`+from+` ORDER BY e.created_at DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, website int
		var domain, username, account, next, reason, ip, at string
		_ = rows.Scan(&id, &website, &domain, &username, &account, &next, &reason, &ip, &at)
		items = append(items, map[string]any{
			"id": id, "website_id": website, "domain": domain, "username": username,
			"account_name": account, "next_account_name": next, "reason": reason,
			"client_ip": ip, "created_at": at,
		})
	}
	s.writePage(w, page, size, total, items)
}

func (s *server) clearLogouts(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	if op.Role == "master" {
		_, _ = s.db.Exec(`DELETE FROM logout_events`)
	} else {
		_, _ = s.db.Exec(`DELETE FROM logout_events WHERE website_id IN (SELECT website_id FROM operator_websites WHERE operator_id=?)`, op.ID)
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) insertLogoutEvent(websiteID int, username, accountName, nextAccount, reason, clientIP string) {
	if websiteID <= 0 {
		return
	}
	_, err := s.db.Exec(`INSERT INTO logout_events (website_id, username, account_name, next_account_name, reason, client_ip, created_at) VALUES (?,?,?,?,?,?,?)`,
		websiteID, username, accountName, nextAccount, reason, clientIP, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		log.Printf("[logout] insert failed: %v", err)
	}
}

func (s *server) listExtensionEvents(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	s.purgeOldRows()
	page, size, q := pageQuery(r)
	where, args := s.scope(op, "e.website_id")
	if id := queryInt(r, "websiteId"); id > 0 {
		where += " AND e.website_id=?"
		args = append(args, id)
	}
	if tool := strings.TrimSpace(r.URL.Query().Get("toolKey")); tool != "" {
		where += " AND e.tool_key=?"
		args = append(args, tool)
	}
	if source := strings.TrimSpace(r.URL.Query().Get("source")); source != "" {
		where += " AND e.source=?"
		args = append(args, source)
	}
	if q != "" {
		where += " AND (e.username LIKE ? OR e.action LIKE ? OR e.target_path LIKE ? OR e.asin LIKE ? OR e.query_text LIKE ? OR w.domain LIKE ? OR t.name LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like, like, like, like, like, like)
	}
	from := ` FROM extension_events e JOIN websites w ON w.id=e.website_id JOIN tools t ON t.id=w.tool_id WHERE ` + where
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*)`+from, args...).Scan(&total)
	page = clampPage(page, size, total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`SELECT e.id, e.website_id, w.domain, t.name, e.username, e.tool_key, e.source, e.action, e.target_path, e.page_url, e.asin, e.marketplace, e.query_text, e.status_code, e.client_ip, e.user_agent, e.created_at`+from+` ORDER BY e.created_at DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, website, status int
		var domain, toolName, username, toolKey, source, action, path, pageURL, asin, marketplace, queryText, ip, ua, at string
		_ = rows.Scan(&id, &website, &domain, &toolName, &username, &toolKey, &source, &action, &path, &pageURL, &asin, &marketplace, &queryText, &status, &ip, &ua, &at)
		items = append(items, map[string]any{
			"id": id, "website_id": website, "domain": domain, "tool_name": toolName, "username": username,
			"tool_key": toolKey, "source": source, "action": action, "target_path": path, "page_url": pageURL,
			"asin": asin, "marketplace": marketplace, "query_text": queryText, "status_code": status,
			"client_ip": ip, "user_agent": ua, "created_at": at,
		})
	}
	s.writePage(w, page, size, total, items)
}

func (s *server) clearExtensionEvents(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	if op.Role == "master" {
		_, _ = s.db.Exec(`DELETE FROM extension_events`)
	} else {
		_, _ = s.db.Exec(`DELETE FROM extension_events WHERE website_id IN (SELECT website_id FROM operator_websites WHERE operator_id=?)`, op.ID)
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) listViolations(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	s.purgeOldRows()
	page, size, q := pageQuery(r)
	where, args := s.scope(op, "v.website_id")
	if id := queryInt(r, "websiteId"); id > 0 {
		where += " AND v.website_id=?"
		args = append(args, id)
	}
	if id := queryInt(r, "resellerId"); id > 0 {
		where += " AND v.website_id IN (SELECT website_id FROM operator_websites WHERE operator_id=?)"
		args = append(args, id)
	}
	if q != "" {
		where += " AND (v.username LIKE ? OR v.reason LIKE ? OR v.client_ip LIKE ? OR w.name LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like, like, like)
	}
	from := ` FROM violations v JOIN websites w ON w.id=v.website_id WHERE ` + where
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*)`+from, args...).Scan(&total)
	page = clampPage(page, size, total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`SELECT v.id, v.website_id, w.name, v.username, v.client_ip, v.reason, v.created_at`+from+` ORDER BY v.created_at DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, website int
		var name, username, ip, reason, at string
		_ = rows.Scan(&id, &website, &name, &username, &ip, &reason, &at)
		items = append(items, map[string]any{"id": id, "website_id": website, "website_name": name, "username": username, "client_ip": ip, "reason": reason, "created_at": at})
	}
	s.writePage(w, page, size, total, items)
}

func (s *server) listSecurity(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	s.purgeOldRows()
	page, size, q := pageQuery(r)
	where, args := s.scope(op, "e.website_id")
	if id := queryInt(r, "websiteId"); id > 0 {
		where += " AND e.website_id=?"
		args = append(args, id)
	}
	if kind := r.URL.Query().Get("eventType"); kind != "" && kind != "all" {
		where += " AND e.event_type=?"
		args = append(args, kind)
	}
	if q != "" {
		where += " AND (e.username LIKE ? OR e.details LIKE ? OR e.attempted_url LIKE ? OR e.event_type LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like, like, like)
	}
	from := ` FROM security_events e JOIN websites w ON w.id=e.website_id WHERE ` + where
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*)`+from, args...).Scan(&total)
	page = clampPage(page, size, total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`SELECT e.id, e.website_id, w.name, e.username, e.client_ip, e.event_type, e.attempted_url, e.details, e.user_agent, e.created_at`+from+` ORDER BY e.created_at DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, website int
		var name, username, ip, kind, url, details, ua, at string
		_ = rows.Scan(&id, &website, &name, &username, &ip, &kind, &url, &details, &ua, &at)
		items = append(items, map[string]any{
			"id": id, "website_id": website, "website_name": name, "username": username, "client_ip": ip,
			"event_type": kind, "attempted_url": url, "details": details, "user_agent": ua, "created_at": at,
		})
	}
	s.writePage(w, page, size, total, items)
}

func (s *server) listBlocked(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	page, size, q := pageQuery(r)
	where, args := "1=1", []any{}
	if op.Role != "master" {
		where = "(b.website_id=0 OR b.website_id IN (SELECT website_id FROM operator_websites WHERE operator_id=?))"
		args = append(args, op.ID)
	}
	if id := queryInt(r, "websiteId"); id > 0 {
		where += " AND b.website_id=?"
		args = append(args, id)
	}
	if q != "" {
		where += " AND (b.client_ip LIKE ? OR b.reason LIKE ? OR COALESCE(w.name,'All websites') LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	from := ` FROM blocked_ips b LEFT JOIN websites w ON w.id=b.website_id WHERE ` + where
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*)`+from, args...).Scan(&total)
	page = clampPage(page, size, total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`SELECT b.id, b.website_id, COALESCE(w.name, CASE WHEN b.website_id=0 THEN 'All websites' ELSE '' END), b.client_ip, b.reason, b.created_at`+from+` ORDER BY b.id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, website int
		var name, ip, reason, at string
		_ = rows.Scan(&id, &website, &name, &ip, &reason, &at)
		items = append(items, map[string]any{"id": id, "website_id": website, "website_name": name, "client_ip": ip, "reason": reason, "created_at": at})
	}
	s.writePage(w, page, size, total, items)
}

func (s *server) listResellers(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok || op.Role != "master" {
		if ok {
			writeErr(w, 403, "Master only")
		}
		return
	}
	page, size, q := pageQuery(r)
	where, args := "role='reseller'", []any{}
	if status := r.URL.Query().Get("status"); status != "" && status != "all" {
		where += " AND status=?"
		args = append(args, status)
	}
	if q != "" {
		where += " AND username LIKE ?"
		args = append(args, "%"+q+"%")
	}
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM operators WHERE `+where, args...).Scan(&total)
	page = clampPage(page, size, total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`SELECT id, username, status FROM operators WHERE `+where+` ORDER BY username LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	type resellerRow struct {
		id               int
		username, status string
	}
	found := []resellerRow{}
	for rows.Next() {
		var item resellerRow
		_ = rows.Scan(&item.id, &item.username, &item.status)
		found = append(found, item)
	}
	rows.Close()
	items := []any{}
	for _, item := range found {
		items = append(items, map[string]any{"id": item.id, "username": item.username, "role": "reseller", "status": item.status, "website_ids": s.websiteIDs(item.id)})
	}
	s.writePage(w, page, size, total, items)
}

func (s *server) websiteIDs(operatorID int) []int {
	ids := []int{}
	rows, err := s.db.Query(`SELECT website_id FROM operator_websites WHERE operator_id=? ORDER BY website_id`, operatorID)
	if err != nil {
		return ids
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids
}

func (s *server) saveReseller(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok || op.Role != "master" {
		if ok {
			writeErr(w, 403, "Master only")
		}
		return
	}
	var body struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Status   string `json:"status"`
		Sites    []int  `json:"website_ids"`
	}
	if err := readBody(r, &body); err != nil || strings.TrimSpace(body.Username) == "" {
		writeErr(w, 400, "Username is required")
		return
	}
	if body.Status == "" {
		body.Status = "active"
	}
	tx, err := s.db.Begin()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	id := body.ID
	if id == 0 {
		res, err := tx.Exec(`INSERT INTO operators (username, password_hash, role, status) VALUES (?,?,?,?)`,
			strings.TrimSpace(body.Username), hashPassword("toolsmandi"), "reseller", body.Status)
		if err != nil {
			writeErr(w, 400, "That username is already used")
			return
		}
		raw, _ := res.LastInsertId()
		id = int(raw)
	} else {
		_, err = tx.Exec(`UPDATE operators SET username=?, status=? WHERE id=? AND role='reseller'`, strings.TrimSpace(body.Username), body.Status, id)
		if err != nil {
			writeErr(w, 400, "That username is already used")
			return
		}
		_, _ = tx.Exec(`DELETE FROM operator_websites WHERE operator_id=?`, id)
	}
	for _, websiteID := range body.Sites {
		_, _ = tx.Exec(`INSERT OR IGNORE INTO operator_websites (operator_id, website_id) VALUES (?,?)`, id, websiteID)
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) deleteReseller(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok || op.Role != "master" {
		if ok {
			writeErr(w, 403, "Master only")
		}
		return
	}
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	var role, username string
	if s.db.QueryRow(`SELECT role, username FROM operators WHERE id=?`, id).Scan(&role, &username) != nil || role != "reseller" {
		writeErr(w, 404, "Reseller not found")
		return
	}
	if username == "master" {
		writeErr(w, 400, "That operator cannot be deleted")
		return
	}
	_, _ = s.db.Exec(`DELETE FROM operator_websites WHERE operator_id=?`, id)
	_, _ = s.db.Exec(`DELETE FROM operators WHERE id=?`, id)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) listDestinations(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	page, size, q := pageQuery(r)
	where, args := "1=1", []any{}
	if r.URL.Query().Get("allChats") != "true" && op.Role != "master" {
		where += " AND d.reseller_id=?"
		args = append(args, op.ID)
	}
	if id := queryInt(r, "resellerId"); id > 0 {
		where += " AND d.reseller_id=?"
		args = append(args, id)
	}
	if q != "" {
		where += " AND (d.label LIKE ? OR d.chat_id LIKE ? OR o.username LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	from := ` FROM telegram_destinations d JOIN operators o ON o.id=d.reseller_id WHERE ` + where
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*)`+from, args...).Scan(&total)
	page = clampPage(page, size, total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`SELECT d.id, d.reseller_id, o.username, d.label, d.chat_id, d.enabled`+from+` ORDER BY o.username, d.label LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, reseller, enabled int
		var name, label, chat string
		_ = rows.Scan(&id, &reseller, &name, &label, &chat, &enabled)
		items = append(items, map[string]any{"id": id, "reseller_id": reseller, "reseller_name": name, "label": label, "chat_id": chat, "enabled": enabled == 1})
	}
	s.writePage(w, page, size, total, items)
}

func (s *server) saveDestination(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	var body struct {
		ID       int    `json:"id"`
		Reseller int    `json:"reseller_id"`
		Label    string `json:"label"`
		ChatID   string `json:"chat_id"`
		Enabled  bool   `json:"enabled"`
	}
	if err := readBody(r, &body); err != nil || strings.TrimSpace(body.Label) == "" || strings.TrimSpace(body.ChatID) == "" {
		writeErr(w, 400, "Label and chat id are required")
		return
	}
	reseller := body.Reseller
	if op.Role != "master" {
		reseller = op.ID
	}
	enabled := 0
	if body.Enabled {
		enabled = 1
	}
	if body.ID == 0 {
		_, err := s.db.Exec(`INSERT INTO telegram_destinations (reseller_id, label, chat_id, enabled) VALUES (?,?,?,?)`, reseller, strings.TrimSpace(body.Label), strings.TrimSpace(body.ChatID), enabled)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	} else {
		_, err := s.db.Exec(`UPDATE telegram_destinations SET reseller_id=?, label=?, chat_id=?, enabled=? WHERE id=?`, reseller, strings.TrimSpace(body.Label), strings.TrimSpace(body.ChatID), enabled, body.ID)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) deleteDestination(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.auth(w, r); !ok {
		return
	}
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	_, _ = s.db.Exec(`DELETE FROM telegram_route_chats WHERE destination_id=?`, id)
	_, _ = s.db.Exec(`DELETE FROM telegram_destinations WHERE id=?`, id)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

type routeRow struct {
	id      int
	website int
	name    string
	domain  string
	events  []string
	dests   []int
	labels  []string
	owners  []int
}

func (s *server) listRoutes(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	page, size, q := pageQuery(r)
	where, args := s.scope(op, "r.website_id")
	rows, err := s.db.Query(`SELECT r.id, r.website_id, w.name, w.domain, r.events_json FROM telegram_routes r JOIN websites w ON w.id=r.website_id WHERE `+where+` ORDER BY w.domain`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	all := []routeRow{}
	for rows.Next() {
		var item routeRow
		var raw string
		_ = rows.Scan(&item.id, &item.website, &item.name, &item.domain, &raw)
		_ = json.Unmarshal([]byte(raw), &item.events)
		all = append(all, item)
	}
	rows.Close()
	for i := range all {
		all[i].dests, all[i].labels, all[i].owners = s.routeChats(all[i].id)
	}
	event := r.URL.Query().Get("event")
	reseller := queryInt(r, "resellerId")
	needle := strings.ToLower(q)
	filtered := []routeRow{}
	for _, item := range all {
		if event != "" && event != "all" && !contains(item.events, event) {
			continue
		}
		if reseller > 0 && !containsInt(item.owners, reseller) {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(item.name+" "+item.domain), needle) {
			continue
		}
		filtered = append(filtered, item)
	}
	total := len(filtered)
	page = clampPage(page, size, total)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	items := []any{}
	for _, item := range filtered[start:end] {
		items = append(items, map[string]any{
			"id": item.id, "website_id": item.website, "website_name": item.name, "website_domain": item.domain,
			"events": item.events, "destination_ids": orInts(item.dests), "chat_labels": orStrings(item.labels),
		})
	}
	s.writePage(w, page, size, total, items)
}

func (s *server) routeChats(routeID int) ([]int, []string, []int) {
	ids, labels, owners := []int{}, []string{}, []int{}
	rows, err := s.db.Query(`SELECT d.id, d.label, d.reseller_id FROM telegram_route_chats c JOIN telegram_destinations d ON d.id=c.destination_id WHERE c.route_id=? ORDER BY d.label`, routeID)
	if err != nil {
		return ids, labels, owners
	}
	defer rows.Close()
	for rows.Next() {
		var id, owner int
		var label string
		_ = rows.Scan(&id, &label, &owner)
		ids = append(ids, id)
		labels = append(labels, label)
		owners = append(owners, owner)
	}
	return ids, labels, owners
}

func (s *server) saveRoute(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	var body struct {
		Website int      `json:"website_id"`
		Events  []string `json:"events"`
		Dests   []int    `json:"destination_ids"`
	}
	if err := readBody(r, &body); err != nil || body.Website == 0 {
		writeErr(w, 400, "Invalid body")
		return
	}
	if op.Role != "master" && !s.owns(op, body.Website) {
		writeErr(w, 403, "You cannot manage this website")
		return
	}
	events := []string{}
	seen := map[string]bool{}
	for _, event := range body.Events {
		if (event == "logout" || event == "spam") && !seen[event] {
			events = append(events, event)
			seen[event] = true
		}
	}
	if len(events) == 0 {
		writeErr(w, 400, "Choose at least one event")
		return
	}
	if len(body.Dests) == 0 {
		writeErr(w, 400, "Choose at least one chat")
		return
	}
	raw, _ := json.Marshal(events)
	tx, err := s.db.Begin()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	var routeID int64
	err = tx.QueryRow(`SELECT id FROM telegram_routes WHERE website_id=?`, body.Website).Scan(&routeID)
	if err != nil {
		res, insErr := tx.Exec(`INSERT INTO telegram_routes (website_id, events_json) VALUES (?,?)`, body.Website, string(raw))
		if insErr != nil {
			writeErr(w, 500, insErr.Error())
			return
		}
		routeID, _ = res.LastInsertId()
	} else {
		_, _ = tx.Exec(`UPDATE telegram_routes SET events_json=? WHERE id=?`, string(raw), routeID)
		_, _ = tx.Exec(`DELETE FROM telegram_route_chats WHERE route_id=?`, routeID)
	}
	for _, dest := range body.Dests {
		if _, err := tx.Exec(`INSERT INTO telegram_route_chats (route_id, destination_id) VALUES (?,?)`, routeID, dest); err != nil {
			writeErr(w, 400, "Chat not found")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) listDeliveries(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	page, size, q := pageQuery(r)
	where, args := s.scope(op, "d.website_id")
	if event := r.URL.Query().Get("event"); event != "" && event != "all" {
		where += " AND d.event=?"
		args = append(args, event)
	}
	if status := r.URL.Query().Get("status"); status != "" && status != "all" {
		where += " AND d.status=?"
		args = append(args, status)
	}
	if q != "" {
		where += " AND (d.username LIKE ? OR w.name LIKE ? OR w.domain LIKE ? OR d.recipients_json LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like, like, like)
	}
	from := ` FROM telegram_deliveries d JOIN websites w ON w.id=d.website_id WHERE ` + where
	rows, err := s.db.Query(`SELECT d.id, d.website_id, w.name, w.domain, d.username, d.event, d.recipients_json, d.status, d.created_at`+from+` ORDER BY d.created_at DESC`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	type delivery struct {
		item map[string]any
	}
	all := []map[string]any{}
	resellerName := ""
	if id := queryInt(r, "resellerId"); id > 0 {
		_ = s.db.QueryRow(`SELECT username FROM operators WHERE id=?`, id).Scan(&resellerName)
	}
	for rows.Next() {
		var id, website int
		var name, domain, username, event, raw, status, at string
		_ = rows.Scan(&id, &website, &name, &domain, &username, &event, &raw, &status, &at)
		recipients := []map[string]any{}
		_ = json.Unmarshal([]byte(raw), &recipients)
		names := []string{}
		seen := map[string]bool{}
		for _, rec := range recipients {
			if reseller, _ := rec["reseller"].(string); reseller != "" && !seen[reseller] {
				names = append(names, reseller)
				seen[reseller] = true
			}
		}
		if resellerName != "" && !seen[resellerName] {
			continue
		}
		all = append(all, map[string]any{
			"id": id, "website_id": website, "reseller_names": names, "website_name": name, "website_domain": domain,
			"username": username, "event": event, "recipients": cleanRecipients(recipients), "status": status, "created_at": at,
		})
	}
	total := len(all)
	page = clampPage(page, size, total)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	s.writePage(w, page, size, total, anySlice(all[start:end]))
}

func cleanRecipients(rows []map[string]any) []any {
	out := []any{}
	for _, row := range rows {
		out = append(out, map[string]any{"label": row["label"], "chat_id": row["chat_id"]})
	}
	return out
}

func anySlice(rows []map[string]any) []any {
	out := []any{}
	for _, row := range rows {
		out = append(out, row)
	}
	return out
}

func (s *server) clearDeliveries(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	if op.Role == "master" {
		_, _ = s.db.Exec(`DELETE FROM telegram_deliveries`)
	} else {
		_, _ = s.db.Exec(`DELETE FROM telegram_deliveries WHERE website_id IN (SELECT website_id FROM operator_websites WHERE operator_id=?)`, op.ID)
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) saveSpamRule(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok || op.Role != "master" {
		if ok {
			writeErr(w, 403, "Master only")
		}
		return
	}
	var body struct {
		Window int `json:"window_minutes"`
		Opens  int `json:"max_opens"`
		IPs    int `json:"max_distinct_ips"`
	}
	if err := readBody(r, &body); err != nil || body.Window < 1 || body.Opens < 1 || body.IPs < 1 {
		writeErr(w, 400, "Use numbers of at least 1")
		return
	}
	_, _ = s.db.Exec(`INSERT INTO settings (key, value) VALUES ('spam_window', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, strconv.Itoa(body.Window))
	_, _ = s.db.Exec(`INSERT INTO settings (key, value) VALUES ('spam_opens', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, strconv.Itoa(body.Opens))
	_, _ = s.db.Exec(`INSERT INTO settings (key, value) VALUES ('spam_ips', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, strconv.Itoa(body.IPs))
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) listSpamReports(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	where, args := s.scope(op, "d.website_id")
	where += " AND d.event='spam'"
	rows, err := s.db.Query(`SELECT d.id, d.website_id, t.name, d.username, d.created_at, d.recipients_json FROM telegram_deliveries d JOIN websites w ON w.id=d.website_id JOIN tools t ON t.id=w.tool_id WHERE `+where+` ORDER BY d.created_at DESC`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, website int
		var tool, username, at, raw string
		_ = rows.Scan(&id, &website, &tool, &username, &at, &raw)
		recipients := []map[string]any{}
		_ = json.Unmarshal([]byte(raw), &recipients)
		labels := []string{}
		for _, rec := range recipients {
			if label, _ := rec["label"].(string); label != "" {
				labels = append(labels, label)
			}
		}
		items = append(items, map[string]any{"id": id, "website_id": website, "tool_name": tool, "username": username, "reason": "spam report", "created_at": at, "chat_labels": labels})
	}
	writeJSON(w, 200, items)
}

func (s *server) useAccount(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.auth(w, r); !ok {
		return
	}
	websiteID, _ := strconv.Atoi(r.PathValue("id"))
	var body struct {
		Username  string `json:"username"`
		Reason    string `json:"reason"`
		FailedIDs []int  `json:"failed_ids"`
		ClientIP  string `json:"client_ip"`
	}
	_ = readBody(r, &body)
	rows, err := s.db.Query(`SELECT id FROM accounts WHERE website_id=? AND status='active' ORDER BY CASE WHEN last_used_at='' THEN 0 ELSE 1 END, last_used_at ASC, id ASC`, websiteID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	chosen := 0
	for rows.Next() {
		var id int
		_ = rows.Scan(&id)
		if containsInt(body.FailedIDs, id) {
			continue
		}
		chosen = id
		break
	}
	rows.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	if strings.TrimSpace(body.Reason) != "" && len(body.FailedIDs) > 0 {
		prev := body.FailedIDs[len(body.FailedIDs)-1]
		_, _ = s.db.Exec(`UPDATE accounts SET failure_count=failure_count+1 WHERE id=?`, prev)
		fromName := ""
		_ = s.db.QueryRow(`SELECT name FROM accounts WHERE id=?`, prev).Scan(&fromName)
		toName := ""
		if chosen != 0 {
			_ = s.db.QueryRow(`SELECT name FROM accounts WHERE id=?`, chosen).Scan(&toName)
			_, _ = s.db.Exec(`INSERT INTO switch_events (website_id, username, from_account_name, to_account_name, reason, switched_at) VALUES (?,?,?,?,?,?)`,
				websiteID, body.Username, fromName, toName, body.Reason, now)
		} else {
			toName = "(none)"
		}
		s.insertLogoutEvent(websiteID, body.Username, fromName, toName, body.Reason, body.ClientIP)
		s.recordLogout(websiteID, body.Username, body.ClientIP, body.Reason)
	}
	if chosen == 0 {
		writeJSON(w, 200, map[string]any{"account_id": 0})
		return
	}
	_, _ = s.db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, chosen)
	writeJSON(w, 200, map[string]any{"account_id": chosen})
}

func (s *server) recordLogout(websiteID int, username, ip, reason string) {
	var tool, website string
	_ = s.db.QueryRow(`SELECT t.name, w.name FROM websites w JOIN tools t ON t.id=w.tool_id WHERE w.id=?`, websiteID).Scan(&tool, &website)
	var routeID int
	var eventsRaw string
	if s.db.QueryRow(`SELECT id, events_json FROM telegram_routes WHERE website_id=?`, websiteID).Scan(&routeID, &eventsRaw) != nil || !strings.Contains(eventsRaw, "logout") {
		// Still keep a telegram_deliveries breadcrumb; Analytics Logouts come from logout_events.
		s.insertDelivery(websiteID, username, "logout", "sent", nil)
		return
	}
	rows, err := s.db.Query(`SELECT d.label, d.chat_id, o.username FROM telegram_route_chats c JOIN telegram_destinations d ON d.id=c.destination_id JOIN operators o ON o.id=d.reseller_id WHERE c.route_id=? AND d.enabled=1`, routeID)
	recipients := []map[string]string{}
	if err == nil {
		for rows.Next() {
			var label, chat, reseller string
			_ = rows.Scan(&label, &chat, &reseller)
			recipients = append(recipients, map[string]string{"label": label, "chat_id": chat, "reseller": reseller})
		}
		rows.Close()
	}
	status := "sent"
	minutes := 60
	var raw string
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE key='repeat_minutes'`).Scan(&raw)
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		minutes = n
	}
	var last string
	err = s.db.QueryRow(`SELECT created_at FROM telegram_deliveries WHERE website_id=? AND username=? AND event='logout' AND status='sent' ORDER BY created_at DESC LIMIT 1`, websiteID, username).Scan(&last)
	if err == nil {
		if parsed, ok := parseTime(last); ok && time.Since(parsed) < time.Duration(minutes)*time.Minute {
			status = "skipped"
		}
	}
	var token, tmpl string
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE key='bot_token'`).Scan(&token)
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE key='logout_template'`).Scan(&tmpl)
	if tmpl == "" {
		tmpl = "{tool} logged out for {username} on {website} at {time}."
	}
	text := strings.NewReplacer("{tool}", tool, "{username}", username, "{website}", website, "{time}", time.Now().UTC().Format(time.RFC3339), "{ip}", ip, "{reason}", reason, "{count}", "", "{from}", "", "{to}", "").Replace(tmpl)
	if strings.TrimSpace(token) != "" && status == "sent" {
		for _, rec := range recipients {
			postTelegram(token, rec["chat_id"], text)
		}
	}
	s.insertDelivery(websiteID, username, "logout", status, recipients)
}

func (s *server) insertDelivery(websiteID int, username, event, status string, recipients []map[string]string) {
	raw, _ := json.Marshal(recipients)
	if recipients == nil {
		raw = []byte("[]")
	}
	_, _ = s.db.Exec(`INSERT INTO telegram_deliveries (website_id, username, event, recipients_json, status, created_at) VALUES (?,?,?,?,?,?)`,
		websiteID, username, event, string(raw), status, time.Now().UTC().Format(time.RFC3339))
}

func postTelegram(token, chatID, text string) {
	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", text)
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.PostForm("https://api.telegram.org/bot"+token+"/sendMessage", form)
	if err == nil {
		resp.Body.Close()
	}
}

func (s *server) listHostReports(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	page, size, q := pageQuery(r)
	where, args := s.scope(op, "h.website_id")
	if id := queryInt(r, "websiteId"); id > 0 {
		where += " AND h.website_id=?"
		args = append(args, id)
	}
	if kind := r.URL.Query().Get("type"); kind != "" && kind != "all" {
		where += " AND h.ip_type=?"
		args = append(args, kind)
	}
	if q != "" {
		where += " AND (h.username LIKE ? OR h.client_ip LIKE ? OR h.org LIKE ? OR h.location LIKE ? OR w.name LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like, like, like, like)
	}
	from := ` FROM host_reports h JOIN websites w ON w.id=h.website_id WHERE ` + where
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*)`+from, args...).Scan(&total)
	page = clampPage(page, size, total)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(`SELECT h.id, h.website_id, w.name, h.username, h.client_ip, h.ip_type, h.org, h.location, h.created_at,
		EXISTS(SELECT 1 FROM blocked_ips b WHERE b.client_ip=h.client_ip AND (b.website_id=h.website_id OR b.website_id=0))`+from+` ORDER BY h.created_at DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, website, blocked int
		var name, username, ip, kind, org, location, at string
		_ = rows.Scan(&id, &website, &name, &username, &ip, &kind, &org, &location, &at, &blocked)
		items = append(items, map[string]any{
			"id": id, "website_id": website, "website_name": name, "username": username,
			"client_ip": ip, "ip_type": kind, "org": org, "location": location, "created_at": at,
			"blocked": blocked == 1,
		})
	}
	s.writePage(w, page, size, total, items)
}

func (s *server) hostReport(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.auth(w, r); !ok {
		return
	}
	var body struct {
		Website  int    `json:"website_id"`
		Username string `json:"username"`
		IP       string `json:"ip"`
		ClientIP string `json:"client_ip"`
		Type     string `json:"type"`
		Org      string `json:"org"`
		Location string `json:"location"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	if body.IP == "" {
		body.IP = body.ClientIP
	}
	switch body.Type {
	case "residential", "hosting", "proxy", "vpn":
	default:
		writeErr(w, 400, "Unknown IP type")
		return
	}
	_, err := s.db.Exec(`INSERT INTO host_reports (website_id, username, client_ip, ip_type, org, location, created_at) VALUES (?,?,?,?,?,?,?)`,
		body.Website, body.Username, body.IP, body.Type, body.Org, body.Location, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) accessCheck(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.auth(w, r); !ok {
		return
	}
	website, _ := strconv.Atoi(r.URL.Query().Get("website_id"))
	ip := r.URL.Query().Get("ip")
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM blocked_ips WHERE client_ip=? AND (website_id=? OR website_id=0)`, ip, website).Scan(&n)
	writeJSON(w, 200, map[string]any{"allowed": n == 0})
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func containsInt(values []int, needle int) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func orInts(values []int) []int {
	if values == nil {
		return []int{}
	}
	return values
}

func orStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
