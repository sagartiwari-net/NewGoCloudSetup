package main

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func recordCookieShare(cfg Config, r *http.Request, token string) {
	panelSess.Delete(token)
	db, err := openPanelDB(cfg)
	if err != nil {
		return
	}
	var websiteID int
	var username, ip string
	err = db.QueryRow(`SELECT website_id, username, COALESCE(client_ip, '') FROM live_sessions WHERE session_token=?`, token).Scan(&websiteID, &username, &ip)
	_, _ = db.Exec(`DELETE FROM live_sessions WHERE session_token=?`, token)
	if err != nil {
		return
	}
	path := ""
	ua := ""
	if r != nil {
		path = r.URL.RequestURI()
		ua = r.UserAgent()
		if ip == "" {
			ip = realClientIP(r)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = db.Exec(`INSERT INTO security_events (website_id, username, client_ip, event_type, attempted_url, details, user_agent, created_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		websiteID, username, ip, "cookie_share", path,
		"Copied the session cookie into another browser. Both browsers were signed out.", ua, now)
	log.Printf("[PANEL] cookie share recorded user=%s ip=%s", username, ip)
}

func noteAccessPattern(db *sql.DB, websiteID int, username, clientIP, userAgent string) {
	sameCount := panelSettingInt(db, "access_same_count", 4)
	sameMinutes := panelSettingInt(db, "access_same_minutes", 30)
	multiCount := panelSettingInt(db, "access_multi_count", 3)
	multiMinutes := panelSettingInt(db, "access_multi_minutes", 3)
	repeatMinutes := panelSettingInt(db, "access_repeat_minutes", 30)
	repeatHours := panelSettingInt(db, "access_repeat_hours", 3)
	repeatOpens := panelSettingInt(db, "access_repeat_opens", 4)
	now := time.Now().UTC()
	var tool string
	_ = db.QueryRow(`SELECT name FROM websites WHERE id=?`, websiteID).Scan(&tool)
	if tool == "" {
		tool = "this tool"
	}
	var reasons []string
	var same int
	_ = db.QueryRow(`SELECT COUNT(*) FROM login_events WHERE username=? AND website_id=? AND logged_in_at>=?`,
		username, websiteID, now.Add(-time.Duration(sameMinutes)*time.Minute).Format(time.RFC3339)).Scan(&same)
	if same >= sameCount {
		reasons = append(reasons, tool+" opened "+strconv.Itoa(same)+" times in "+strconv.Itoa(sameMinutes)+" minutes")
	}
	var tools int
	_ = db.QueryRow(`SELECT COUNT(DISTINCT website_id) FROM login_events WHERE username=? AND logged_in_at>=?`,
		username, now.Add(-time.Duration(multiMinutes)*time.Minute).Format(time.RFC3339)).Scan(&tools)
	if tools >= multiCount {
		reasons = append(reasons, strconv.Itoa(tools)+" different tools opened within "+strconv.Itoa(multiMinutes)+" minutes")
	}
	if reason := repeatAccessReason(db, username, repeatMinutes, repeatHours, repeatOpens); reason != "" {
		reasons = append(reasons, reason)
	}
	for _, reason := range reasons {
		var recent int
		_ = db.QueryRow(`SELECT COUNT(*) FROM security_events WHERE username=? AND event_type='access_pattern' AND details=? AND created_at>=?`,
			username, reason, now.Add(-30*time.Minute).Format(time.RFC3339)).Scan(&recent)
		if recent > 0 {
			continue
		}
		_, _ = db.Exec(`INSERT INTO security_events (website_id, username, client_ip, event_type, attempted_url, details, user_agent, created_at)
			VALUES (?,?,?,?,?,?,?,?)`,
			websiteID, username, clientIP, "access_pattern", "", reason, userAgent, now.Format(time.RFC3339))
		log.Printf("[PANEL] access pattern user=%s %s", username, reason)
	}
}

func repeatAccessReason(db *sql.DB, username string, maxGapMinutes, hours, minOpens int) string {
	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
	rows, err := db.Query(`SELECT logged_in_at FROM login_events WHERE username=? AND logged_in_at>=? ORDER BY logged_in_at`, username, since.Format(time.RFC3339))
	if err != nil {
		return ""
	}
	defer rows.Close()
	var times []time.Time
	for rows.Next() {
		var raw string
		if rows.Scan(&raw) != nil {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			continue
		}
		times = append(times, parsed)
	}
	if len(times) < minOpens {
		return ""
	}
	maxGap := time.Duration(maxGapMinutes) * time.Minute
	for i := 1; i < len(times); i++ {
		if times[i].Sub(times[i-1]) > maxGap {
			return ""
		}
	}
	if times[len(times)-1].Sub(times[0]) < time.Duration(hours)*time.Hour {
		return ""
	}
	return "Opened tools every " + strconv.Itoa(maxGapMinutes) + " minutes or less for " + strconv.Itoa(hours) + " hours (" + strconv.Itoa(len(times)) + " opens)"
}

func panelSettingInt(db *sql.DB, key string, fallback int) int {
	var raw string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&raw); err != nil {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return fallback
	}
	return n
}
