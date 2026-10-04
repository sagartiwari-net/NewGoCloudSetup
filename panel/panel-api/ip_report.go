package main

import (
	"net"
	"net/http"
	"strings"
	"time"
)

func (s *server) ipReport(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	ip := strings.TrimSpace(r.URL.Query().Get("ip"))
	if ip == "" {
		ip = strings.TrimSpace(r.URL.Query().Get("client_ip"))
	}
	ip = publicIP(ip)
	if ip == "" || net.ParseIP(ip) == nil {
		writeErr(w, 400, "valid ip required")
		return
	}

	scope, scopeArgs := s.scope(op, "website_id")

	// Latest known classification from host_reports (scoped).
	ipType, org, location := "", "", ""
	var lastClassified string
	_ = s.db.QueryRow(`SELECT ip_type, org, location, created_at
		FROM host_reports
		WHERE client_ip=? AND `+scope+`
		ORDER BY created_at DESC LIMIT 1`, append([]any{ip}, scopeArgs...)...).
		Scan(&ipType, &org, &location, &lastClassified)

	// Live refresh when missing/unknown (or older than 7 days).
	needLookup := ipType == "" || ipType == "unknown"
	if !needLookup && lastClassified != "" {
		if t, ok := parseTime(lastClassified); ok && time.Since(t) > 7*24*time.Hour {
			needLookup = true
		}
	}
	lookup := map[string]any{}
	if needLookup {
		kind, lookupOrg, lookupLoc, extra := classifyIPDetailed(ip)
		if kind != "" {
			ipType = kind
		}
		if lookupOrg != "" {
			org = lookupOrg
		}
		if lookupLoc != "" {
			location = lookupLoc
		}
		lookup = extra
		lastClassified = time.Now().UTC().Format(time.RFC3339)
	} else {
		_, _, _, lookup = classifyIPDetailed(ip)
	}
	if ipType == "" {
		ipType = "unknown"
	}

	blocked := 0
	blockScope, blockArgs := s.scope(op, "b.website_id")
	_ = s.db.QueryRow(`SELECT EXISTS(
		SELECT 1 FROM blocked_ips b
		WHERE b.client_ip=? AND (b.website_id=0 OR (`+blockScope+`))
	)`, append([]any{ip}, blockArgs...)...).Scan(&blocked)

	eventScope, eventArgs := s.scope(op, "e.website_id")
	eventScope += " AND e.client_ip=?"
	eventArgs = append(eventArgs, ip)

	var loginCount, distinctUsers int
	var firstLogin, lastLogin string
	_ = s.db.QueryRow(`SELECT COUNT(*), COUNT(DISTINCT e.username), COALESCE(MIN(e.logged_in_at),''), COALESCE(MAX(e.logged_in_at),'')
		FROM login_events e WHERE `+eventScope, eventArgs...).
		Scan(&loginCount, &distinctUsers, &firstLogin, &lastLogin)

	userRows, err := s.db.Query(`SELECT e.username, COUNT(*), COALESCE(MAX(e.logged_in_at),''),
		GROUP_CONCAT(DISTINCT w.name), GROUP_CONCAT(DISTINCT w.id)
		FROM login_events e
		JOIN websites w ON w.id=e.website_id
		WHERE `+eventScope+`
		GROUP BY e.username
		ORDER BY COUNT(*) DESC, e.username
		LIMIT 100`, eventArgs...)
	users := []any{}
	if err == nil {
		for userRows.Next() {
			var username, last, siteNames, siteIDs string
			var count int
			if userRows.Scan(&username, &count, &last, &siteNames, &siteIDs) == nil {
				users = append(users, map[string]any{
					"username":      username,
					"login_count":   count,
					"last_login_at": last,
					"website_names": uniqueCSV(siteNames),
					"website_ids":   splitIntCSV(siteIDs),
				})
			}
		}
		userRows.Close()
	}

	// Also users who appear only in host_reports / live_sessions for this IP.
	seenUser := map[string]bool{}
	for _, item := range users {
		if m, ok := item.(map[string]any); ok {
			if name, _ := m["username"].(string); name != "" {
				seenUser[name] = true
			}
		}
	}
	hostScope, hostArgs := s.scope(op, "h.website_id")
	hostScope += " AND h.client_ip=?"
	hostArgs = append(hostArgs, ip)
	hostUserRows, err := s.db.Query(`SELECT h.username, COUNT(*), COALESCE(MAX(h.created_at),''),
		GROUP_CONCAT(DISTINCT w.name)
		FROM host_reports h
		JOIN websites w ON w.id=h.website_id
		WHERE `+hostScope+`
		GROUP BY h.username
		ORDER BY COUNT(*) DESC
		LIMIT 100`, hostArgs...)
	if err == nil {
		for hostUserRows.Next() {
			var username, last, siteNames string
			var count int
			if hostUserRows.Scan(&username, &count, &last, &siteNames) != nil {
				continue
			}
			if seenUser[username] {
				continue
			}
			seenUser[username] = true
			users = append(users, map[string]any{
				"username":      username,
				"login_count":   0,
				"last_login_at": last,
				"website_names": uniqueCSV(siteNames),
				"website_ids":   []int{},
				"source":        "host_report",
			})
		}
		hostUserRows.Close()
	}

	loginRows, err := s.db.Query(`SELECT e.id, e.website_id, w.domain, w.name, e.username, e.user_agent, e.logged_in_at
		FROM login_events e
		JOIN websites w ON w.id=e.website_id
		WHERE `+eventScope+`
		ORDER BY e.logged_in_at DESC
		LIMIT 50`, eventArgs...)
	recentLogins := []any{}
	if err == nil {
		for loginRows.Next() {
			var id, websiteID int
			var domain, siteName, username, ua, at string
			if loginRows.Scan(&id, &websiteID, &domain, &siteName, &username, &ua, &at) == nil {
				recentLogins = append(recentLogins, map[string]any{
					"id": id, "website_id": websiteID, "domain": domain, "website_name": siteName,
					"username": username, "user_agent": ua, "logged_in_at": at,
				})
			}
		}
		loginRows.Close()
	}

	reportRows, err := s.db.Query(`SELECT h.id, h.website_id, w.name, h.username, h.ip_type, h.org, h.location, h.created_at
		FROM host_reports h
		JOIN websites w ON w.id=h.website_id
		WHERE `+hostScope+`
		ORDER BY h.created_at DESC
		LIMIT 50`, hostArgs...)
	reports := []any{}
	if err == nil {
		for reportRows.Next() {
			var id, websiteID int
			var siteName, username, kind, reportOrg, reportLoc, at string
			if reportRows.Scan(&id, &websiteID, &siteName, &username, &kind, &reportOrg, &reportLoc, &at) == nil {
				reports = append(reports, map[string]any{
					"id": id, "website_id": websiteID, "website_name": siteName, "username": username,
					"ip_type": kind, "org": reportOrg, "location": reportLoc, "created_at": at,
				})
			}
		}
		reportRows.Close()
	}

	writeJSON(w, 200, map[string]any{
		"ip": ip,
		"classification": map[string]any{
			"ip_type":         ipType,
			"type_label":      ipTypeLabel(ipType),
			"org":             org,
			"location":        location,
			"classified_at":   lastClassified,
			"lookup":          lookup,
			"blocked":         blocked == 1,
		},
		"summary": map[string]any{
			"login_count":    loginCount,
			"distinct_users": distinctUsers,
			"user_count":     len(users),
			"first_seen_at":  firstLogin,
			"last_seen_at":   lastLogin,
		},
		"users":         users,
		"recent_logins": recentLogins,
		"host_reports":  reports,
	})
}

func ipTypeLabel(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "vpn":
		return "VPN / anonymizer"
	case "proxy":
		return "Proxy"
	case "hosting":
		return "Hosting / VPS / RDP-like"
	case "residential":
		return "Residential / home ISP"
	default:
		return "Unknown"
	}
}
