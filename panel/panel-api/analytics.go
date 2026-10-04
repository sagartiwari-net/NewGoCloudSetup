package main

import (
	"net/http"
	"strings"
	"time"
)

func analyticsDays(r *http.Request) int {
	days := queryInt(r, "days")
	if days != 30 {
		days = 7
	}
	return days
}

func analyticsSince(days int) string {
	if days < 1 {
		days = 7
	}
	return time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
}

func (s *server) analyticsScope(op operator, r *http.Request, column string) (string, []any) {
	where, args := s.scope(op, column)
	if id := queryInt(r, "websiteId"); id > 0 {
		where += " AND " + column + "=?"
		args = append(args, id)
	}
	return where, args
}

func (s *server) analyticsSummary(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	s.purgeOldRows()
	days := analyticsDays(r)
	since := analyticsSince(days)
	q := strings.TrimSpace(r.URL.Query().Get("query"))
	like := "%" + q + "%"

	loginWhere, loginArgs := s.analyticsScope(op, r, "e.website_id")
	loginWhere += " AND substr(e.logged_in_at,1,10) >= ?"
	loginArgs = append(loginArgs, since)
	if q != "" {
		loginWhere += " AND (e.username LIKE ? OR e.client_ip LIKE ?)"
		loginArgs = append(loginArgs, like, like)
	}

	extWhere, extArgs := s.analyticsScope(op, r, "e.website_id")
	extWhere += " AND substr(e.created_at,1,10) >= ?"
	extArgs = append(extArgs, since)
	if q != "" {
		extWhere += " AND (e.username LIKE ? OR e.action LIKE ? OR e.asin LIKE ? OR e.query_text LIKE ?)"
		extArgs = append(extArgs, like, like, like, like)
	}

	switchWhere, switchArgs := s.analyticsScope(op, r, "e.website_id")
	switchWhere += " AND substr(e.switched_at,1,10) >= ?"
	switchArgs = append(switchArgs, since)
	if q != "" {
		switchWhere += " AND (e.username LIKE ? OR e.reason LIKE ?)"
		switchArgs = append(switchArgs, like, like)
	}

	var loginCount, uniqueUsers, extensionCount, switchCount int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM login_events e WHERE `+loginWhere, loginArgs...).Scan(&loginCount)
	_ = s.db.QueryRow(`SELECT COUNT(DISTINCT e.username) FROM login_events e WHERE `+loginWhere, loginArgs...).Scan(&uniqueUsers)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM extension_events e WHERE `+extWhere, extArgs...).Scan(&extensionCount)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM switch_events e WHERE `+switchWhere, switchArgs...).Scan(&switchCount)

	series := []any{}
	for i := days - 1; i >= 0; i-- {
		day := time.Now().UTC().AddDate(0, 0, -i).Format("2006-01-02")
		dayLoginWhere := loginWhere + " AND substr(e.logged_in_at,1,10)=?"
		dayLoginArgs := append(append([]any{}, loginArgs...), day)
		dayExtWhere := extWhere + " AND substr(e.created_at,1,10)=?"
		dayExtArgs := append(append([]any{}, extArgs...), day)
		daySwitchWhere := switchWhere + " AND substr(e.switched_at,1,10)=?"
		daySwitchArgs := append(append([]any{}, switchArgs...), day)
		var logins, extension, switches int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM login_events e WHERE `+dayLoginWhere, dayLoginArgs...).Scan(&logins)
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM extension_events e WHERE `+dayExtWhere, dayExtArgs...).Scan(&extension)
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM switch_events e WHERE `+daySwitchWhere, daySwitchArgs...).Scan(&switches)
		series = append(series, map[string]any{"day": day, "logins": logins, "extension": extension, "switches": switches})
	}

	topUsers := s.analyticsTopUsers(loginWhere, loginArgs, extWhere, extArgs)
	topActions := s.analyticsNamedCounts(`SELECT e.action, COUNT(*) AS c FROM extension_events e WHERE `+extWhere+` GROUP BY e.action ORDER BY c DESC LIMIT 10`, extArgs)
	tools := s.analyticsTools(extWhere, extArgs)
	switchReasons := s.analyticsNamedCounts(`SELECT CASE WHEN TRIM(e.reason)='' THEN '(none)' ELSE e.reason END AS reason, COUNT(*) AS c FROM switch_events e WHERE `+switchWhere+` GROUP BY reason ORDER BY c DESC LIMIT 10`, switchArgs)

	writeJSON(w, 200, map[string]any{
		"days": days,
		"counts": map[string]any{
			"logins":       loginCount,
			"unique_users": uniqueUsers,
			"extension":    extensionCount,
			"switches":     switchCount,
		},
		"series":         series,
		"top_users":      topUsers,
		"top_actions":    topActions,
		"tools":          tools,
		"switch_reasons": switchReasons,
	})
}

func (s *server) analyticsTopUsers(loginWhere string, loginArgs []any, extWhere string, extArgs []any) []any {
	type agg struct {
		logins    int
		extension int
	}
	byUser := map[string]*agg{}
	rows, err := s.db.Query(`SELECT e.username, COUNT(*) FROM login_events e WHERE `+loginWhere+` GROUP BY e.username`, loginArgs...)
	if err == nil {
		for rows.Next() {
			var username string
			var n int
			if rows.Scan(&username, &n) == nil {
				byUser[username] = &agg{logins: n}
			}
		}
		rows.Close()
	}
	rows, err = s.db.Query(`SELECT e.username, COUNT(*) FROM extension_events e WHERE `+extWhere+` GROUP BY e.username`, extArgs...)
	if err == nil {
		for rows.Next() {
			var username string
			var n int
			if rows.Scan(&username, &n) == nil {
				item := byUser[username]
				if item == nil {
					item = &agg{}
					byUser[username] = item
				}
				item.extension = n
			}
		}
		rows.Close()
	}
	type ranked struct {
		username  string
		logins    int
		extension int
		total     int
	}
	list := make([]ranked, 0, len(byUser))
	for username, item := range byUser {
		list = append(list, ranked{
			username:  username,
			logins:    item.logins,
			extension: item.extension,
			total:     item.logins + item.extension,
		})
	}
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j].total > list[i].total {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
	limit := 10
	if len(list) < limit {
		limit = len(list)
	}
	out := make([]any, 0, limit)
	for i := 0; i < limit; i++ {
		row := list[i]
		out = append(out, map[string]any{
			"username":  row.username,
			"logins":    row.logins,
			"extension": row.extension,
			"total":     row.total,
		})
	}
	return out
}

func (s *server) analyticsNamedCounts(query string, args []any) []any {
	rows, err := s.db.Query(query, args...)
	out := []any{}
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var count int
		if rows.Scan(&name, &count) == nil {
			out = append(out, map[string]any{"name": name, "count": count})
		}
	}
	return out
}

func (s *server) analyticsTools(extWhere string, extArgs []any) []any {
	rows, err := s.db.Query(`SELECT CASE WHEN TRIM(e.tool_key)='' THEN t.name ELSE e.tool_key END AS tool_key,
		t.name, COUNT(*) AS c
		FROM extension_events e
		JOIN websites w ON w.id=e.website_id
		JOIN tools t ON t.id=w.tool_id
		WHERE `+extWhere+`
		GROUP BY tool_key, t.name
		ORDER BY c DESC
		LIMIT 10`, extArgs...)
	out := []any{}
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var key, name string
		var count int
		if rows.Scan(&key, &name, &count) == nil {
			out = append(out, map[string]any{"tool_key": key, "tool_name": name, "count": count})
		}
	}
	return out
}

func (s *server) userReport(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	username := strings.TrimSpace(r.URL.Query().Get("username"))
	if username == "" {
		writeErr(w, 400, "username required")
		return
	}
	websiteID := queryInt(r, "websiteId")
	// Website list is always unfiltered (All websites dropdown needs full set).
	siteScope, siteArgs := s.scope(op, "w.id")
	userWhere := siteScope + " AND u.username=?"
	userArgs := append(append([]any{}, siteArgs...), username)
	if websiteID > 0 {
		userWhere += " AND u.website_id=?"
		userArgs = append(userArgs, websiteID)
	}
	var userID int
	_ = s.db.QueryRow(`SELECT u.id FROM panel_users u JOIN websites w ON w.id=u.website_id WHERE `+userWhere+` ORDER BY u.id LIMIT 1`, userArgs...).Scan(&userID)
	var user any
	if userID > 0 {
		user = s.userJSON(userID)
	}

	websites := []any{}
	siteRows, err := s.db.Query(`SELECT DISTINCT w.id, w.domain, w.name, t.name
		FROM websites w
		JOIN tools t ON t.id=w.tool_id
		WHERE `+siteScope+` AND (
			EXISTS (SELECT 1 FROM panel_users u WHERE u.website_id=w.id AND u.username=?)
			OR EXISTS (SELECT 1 FROM login_events e WHERE e.website_id=w.id AND e.username=?)
			OR EXISTS (SELECT 1 FROM live_sessions s WHERE s.website_id=w.id AND s.username=?)
		)
		ORDER BY w.name`, append(append([]any{}, siteArgs...), username, username, username)...)
	if err == nil {
		for siteRows.Next() {
			var id int
			var domain, name, tool string
			if siteRows.Scan(&id, &domain, &name, &tool) == nil {
				websites = append(websites, map[string]any{"id": id, "domain": domain, "name": name, "tool_name": tool})
			}
		}
		siteRows.Close()
	}

	eventWhere, eventArgs := s.userEventScope(op, websiteID, username)
	var loginCount, switchCount, extensionCount, usageCount int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM login_events e WHERE `+eventWhere, eventArgs...).Scan(&loginCount)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM switch_events e WHERE `+eventWhere, eventArgs...).Scan(&switchCount)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM extension_events e WHERE `+eventWhere, eventArgs...).Scan(&extensionCount)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM usage_events e WHERE `+eventWhere, eventArgs...).Scan(&usageCount)

	var lastLogin string
	_ = s.db.QueryRow(`SELECT COALESCE(MAX(e.logged_in_at),'') FROM login_events e WHERE `+eventWhere, eventArgs...).Scan(&lastLogin)

	now := time.Now().UTC().Format(time.RFC3339)
	sessionWhere, sessionArgs := s.scope(op, "s.website_id")
	sessionWhere += " AND s.username=? AND s.expires_at > ?"
	sessionArgs = append(sessionArgs, username, now)
	if websiteID > 0 {
		sessionWhere += " AND s.website_id=?"
		sessionArgs = append(sessionArgs, websiteID)
	}
	var activeSession any
	var sid, siteID, accountID int
	var siteName, token, clientIP, expires, created, account string
	err = s.db.QueryRow(`SELECT s.id, s.website_id, w.name, s.session_token, s.client_ip, s.expires_at, s.created_at, s.assigned_account_id, COALESCE(a.name,'Auto')
		FROM live_sessions s
		JOIN websites w ON w.id=s.website_id
		LEFT JOIN accounts a ON a.id=s.assigned_account_id
		WHERE `+sessionWhere+` ORDER BY s.id DESC LIMIT 1`, sessionArgs...).
		Scan(&sid, &siteID, &siteName, &token, &clientIP, &expires, &created, &accountID, &account)
	if err == nil {
		if accountID == 0 {
			account = "Auto"
		}
		activeSession = map[string]any{
			"id": sid, "website_id": siteID, "website_name": siteName, "session_token": token,
			"username": username, "client_ip": clientIP, "expires_at": expires, "created_at": created,
			"assigned_account_id": accountID, "assigned_account_name": account,
		}
	}

	meters := []any{}
	if u, ok := user.(map[string]any); ok {
		if m, ok := u["meters"].([]any); ok {
			meters = m
		}
	}

	ips := s.userReportIPs(op, username, websiteID)

	writeJSON(w, 200, map[string]any{
		"user":     user,
		"websites": websites,
		"meters":   meters,
		"ips":      ips,
		"summary": map[string]any{
			"login_count":     loginCount,
			"switch_count":    switchCount,
			"extension_count": extensionCount,
			"usage_hit_count": usageCount,
			"last_login_at":   lastLogin,
			"active_session":  activeSession,
			"distinct_ips":    len(ips),
		},
	})
}

func (s *server) userReportIPs(op operator, username string, websiteID int) []any {
	where, args := s.userEventScope(op, websiteID, username)
	where += ` AND TRIM(e.client_ip) <> ''`
	rows, err := s.db.Query(`SELECT e.client_ip, COUNT(*), MAX(e.logged_in_at),
		GROUP_CONCAT(DISTINCT w.domain)
		FROM login_events e
		JOIN websites w ON w.id=e.website_id
		WHERE `+where+`
		GROUP BY e.client_ip
		ORDER BY MAX(e.logged_in_at) DESC
		LIMIT 100`, args...)
	if err != nil {
		return []any{}
	}
	type ipAgg struct {
		ip, last, domains string
		count             int
	}
	aggs := []ipAgg{}
	for rows.Next() {
		var row ipAgg
		if rows.Scan(&row.ip, &row.count, &row.last, &row.domains) == nil {
			aggs = append(aggs, row)
		}
	}
	rows.Close()

	hostByIP := map[string][3]string{}
	hostWhere, hostArgs := s.scope(op, "h.website_id")
	hostWhere += " AND h.username=?"
	hostArgs = append(hostArgs, username)
	if websiteID > 0 {
		hostWhere += " AND h.website_id=?"
		hostArgs = append(hostArgs, websiteID)
	}
	hostRows, err := s.db.Query(`SELECT h.client_ip, h.ip_type, h.org, h.location, h.created_at, w.domain,
		EXISTS(SELECT 1 FROM blocked_ips b WHERE b.client_ip=h.client_ip AND (b.website_id=h.website_id OR b.website_id=0))
		FROM host_reports h JOIN websites w ON w.id=h.website_id
		WHERE `+hostWhere+` ORDER BY h.created_at DESC LIMIT 200`, hostArgs...)
	type hostExtra struct {
		ip, kind, org, location, at, domain string
		blocked                             bool
	}
	extras := []hostExtra{}
	if err == nil {
		for hostRows.Next() {
			var row hostExtra
			var blocked int
			if hostRows.Scan(&row.ip, &row.kind, &row.org, &row.location, &row.at, &row.domain, &blocked) != nil {
				continue
			}
			row.blocked = blocked == 1
			if _, ok := hostByIP[row.ip]; !ok {
				hostByIP[row.ip] = [3]string{row.kind, row.org, row.location}
			}
			extras = append(extras, row)
		}
		hostRows.Close()
	}

	blockedSet := map[string]bool{}
	if len(aggs) > 0 {
		blockRows, berr := s.db.Query(`SELECT DISTINCT client_ip FROM blocked_ips`)
		if berr == nil {
			for blockRows.Next() {
				var ip string
				if blockRows.Scan(&ip) == nil {
					blockedSet[ip] = true
				}
			}
			blockRows.Close()
		}
	}

	items := []any{}
	seen := map[string]bool{}
	for _, row := range aggs {
		meta := hostByIP[row.ip]
		ipType := meta[0]
		if ipType == "" {
			ipType = "unknown"
		}
		items = append(items, map[string]any{
			"client_ip": row.ip, "ip_type": ipType, "org": meta[1], "location": meta[2],
			"login_count": row.count, "last_seen_at": row.last, "domains": row.domains, "blocked": blockedSet[row.ip],
		})
		seen[row.ip] = true
	}
	for _, row := range extras {
		if seen[row.ip] {
			continue
		}
		seen[row.ip] = true
		kind := row.kind
		if kind == "" {
			kind = "unknown"
		}
		items = append(items, map[string]any{
			"client_ip": row.ip, "ip_type": kind, "org": row.org, "location": row.location,
			"login_count": 0, "last_seen_at": row.at, "domains": row.domain, "blocked": row.blocked,
		})
	}
	return items
}

func (s *server) userEventScope(op operator, websiteID int, username string) (string, []any) {
	where, args := s.scope(op, "e.website_id")
	where += " AND e.username=?"
	args = append(args, username)
	if websiteID > 0 {
		where += " AND e.website_id=?"
		args = append(args, websiteID)
	}
	return where, args
}

func (s *server) userReportEvents(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	username := strings.TrimSpace(r.URL.Query().Get("username"))
	if username == "" {
		writeErr(w, 400, "username required")
		return
	}
	section := strings.TrimSpace(r.URL.Query().Get("section"))
	websiteID := queryInt(r, "websiteId")
	page, size, _ := pageQuery(r)
	where, args := s.userEventScope(op, websiteID, username)

	switch section {
	case "logins":
		s.pageUserLogins(w, where, args, page, size)
	case "switches":
		s.pageUserSwitches(w, where, args, page, size)
	case "extension":
		s.pageUserExtension(w, where, args, page, size)
	case "usage":
		s.pageUserUsage(w, where, args, page, size)
	case "security":
		s.pageUserSecurity(w, op, username, websiteID, page, size)
	default:
		writeErr(w, 400, "invalid section")
	}
}

func (s *server) pageUserLogins(w http.ResponseWriter, where string, args []any, page, size int) {
	from := ` FROM login_events e JOIN websites w ON w.id=e.website_id WHERE ` + where
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*)`+from, args...).Scan(&total)
	page = clampPage(page, size, total)
	qArgs := append(append([]any{}, args...), size, (page-1)*size)
	rows, err := s.db.Query(`SELECT e.id, e.website_id, w.domain, e.username, e.client_ip, e.user_agent, e.logged_in_at`+from+` ORDER BY e.logged_in_at DESC LIMIT ? OFFSET ?`, qArgs...)
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

func (s *server) pageUserSwitches(w http.ResponseWriter, where string, args []any, page, size int) {
	from := ` FROM switch_events e JOIN websites w ON w.id=e.website_id WHERE ` + where
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*)`+from, args...).Scan(&total)
	page = clampPage(page, size, total)
	qArgs := append(append([]any{}, args...), size, (page-1)*size)
	rows, err := s.db.Query(`SELECT e.id, e.website_id, w.domain, e.username, e.from_account_name, e.to_account_name, e.reason, e.switched_at`+from+` ORDER BY e.switched_at DESC LIMIT ? OFFSET ?`, qArgs...)
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

func (s *server) pageUserExtension(w http.ResponseWriter, where string, args []any, page, size int) {
	from := ` FROM extension_events e JOIN websites w ON w.id=e.website_id JOIN tools t ON t.id=w.tool_id WHERE ` + where
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*)`+from, args...).Scan(&total)
	page = clampPage(page, size, total)
	qArgs := append(append([]any{}, args...), size, (page-1)*size)
	rows, err := s.db.Query(`SELECT e.id, e.website_id, w.domain, t.name, e.username, e.tool_key, e.source, e.action, e.target_path, e.page_url, e.asin, e.marketplace, e.query_text, e.status_code, e.client_ip, e.user_agent, e.created_at`+from+` ORDER BY e.created_at DESC LIMIT ? OFFSET ?`, qArgs...)
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

func (s *server) pageUserUsage(w http.ResponseWriter, where string, args []any, page, size int) {
	from := ` FROM usage_events e JOIN websites w ON w.id=e.website_id JOIN tools t ON t.id=w.tool_id WHERE ` + where
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*)`+from, args...).Scan(&total)
	page = clampPage(page, size, total)
	qArgs := append(append([]any{}, args...), size, (page-1)*size)
	rows, err := s.db.Query(`SELECT e.id, e.website_id, w.name, t.name, e.username, e.limit_key, e.limit_label, e.reset_days, e.action, e.target_path, e.amount, e.timestamp`+from+` ORDER BY e.timestamp DESC LIMIT ? OFFSET ?`, qArgs...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, website, reset, amount int
		var site, tool, user, key, label, action, path, ts string
		_ = rows.Scan(&id, &website, &site, &tool, &user, &key, &label, &reset, &action, &path, &amount, &ts)
		items = append(items, map[string]any{
			"id": id, "website_id": website, "website_name": site, "tool_name": tool, "username": user,
			"limit_key": key, "limit_label": label, "reset_days": reset, "action": action, "target_path": path, "amount": amount, "timestamp": ts,
		})
	}
	s.writePage(w, page, size, total, items)
}

func (s *server) pageUserSecurity(w http.ResponseWriter, op operator, username string, websiteID, page, size int) {
	type row struct {
		at   string
		item map[string]any
	}
	all := []row{}

	vWhere, vArgs := s.scope(op, "v.website_id")
	vWhere += " AND v.username=?"
	vArgs = append(vArgs, username)
	if websiteID > 0 {
		vWhere += " AND v.website_id=?"
		vArgs = append(vArgs, websiteID)
	}
	if rows, err := s.db.Query(`SELECT v.id, v.website_id, w.name, v.username, v.client_ip, v.reason, v.created_at
		FROM violations v JOIN websites w ON w.id=v.website_id WHERE `+vWhere+` ORDER BY v.created_at DESC`, vArgs...); err == nil {
		for rows.Next() {
			var id, website int
			var name, user, ip, reason, at string
			if rows.Scan(&id, &website, &name, &user, &ip, &reason, &at) == nil {
				all = append(all, row{at: at, item: map[string]any{
					"kind": "violation", "id": id, "website_id": website, "website_name": name,
					"username": user, "client_ip": ip, "reason": reason, "event_type": "violation",
					"attempted_url": "", "details": reason, "user_agent": "", "created_at": at,
				}})
			}
		}
		rows.Close()
	}

	sWhere, sArgs := s.scope(op, "e.website_id")
	sWhere += " AND e.username=?"
	sArgs = append(sArgs, username)
	if websiteID > 0 {
		sWhere += " AND e.website_id=?"
		sArgs = append(sArgs, websiteID)
	}
	if rows, err := s.db.Query(`SELECT e.id, e.website_id, w.name, e.username, e.client_ip, e.event_type, e.attempted_url, e.details, e.user_agent, e.created_at
		FROM security_events e JOIN websites w ON w.id=e.website_id WHERE `+sWhere+` ORDER BY e.created_at DESC`, sArgs...); err == nil {
		for rows.Next() {
			var id, website int
			var name, user, ip, eventType, url, details, ua, at string
			if rows.Scan(&id, &website, &name, &user, &ip, &eventType, &url, &details, &ua, &at) == nil {
				all = append(all, row{at: at, item: map[string]any{
					"kind": "security", "id": id, "website_id": website, "website_name": name,
					"username": user, "client_ip": ip, "reason": details, "event_type": eventType,
					"attempted_url": url, "details": details, "user_agent": ua, "created_at": at,
				}})
			}
		}
		rows.Close()
	}

	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].at > all[i].at {
				all[i], all[j] = all[j], all[i]
			}
		}
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
	items := []any{}
	for _, row := range all[start:end] {
		items = append(items, row.item)
	}
	s.writePage(w, page, size, total, items)
}
