package main

import (
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

var logoutSeen sync.Map

func isLogoutPath(path string) bool {
	p := strings.ToLower(path)
	return strings.Contains(p, "logout") || strings.Contains(p, "sign-out") || strings.Contains(p, "signout")
}

func bodyLooksLoggedOut(body []byte) bool {
	text := strings.ToLower(string(body))
	// NoxTools gateway page: "Session expired, access again from Dashboard"
	// Do not treat Semrush SPA console "Session expired" scripts alone as logout.
	if strings.Contains(text, "access again from dashboard") {
		return true
	}
	if strings.Contains(text, "session expired") && strings.Contains(text, "dashboard") &&
		!strings.Contains(text, "console.log") {
		return true
	}
	// Logged-out marketing shell (proxy_token dead / no Semrush session).
	if strings.Contains(text, "your unfair advantage for growing brand visibility") {
		return true
	}
	if strings.Contains(text, "<title>semrush:</title>") || strings.Contains(text, "<title>semrush: your unfair") {
		return true
	}
	// Ranky member login wall when PHPSESSID is dead.
	if strings.Contains(text, "member.rankytools.com/login") ||
		strings.Contains(text, "member.rankytools.co/login") ||
		(strings.Contains(text, "return_url") && strings.Contains(text, "rankytools.com") && strings.Contains(text, "sign in")) {
		return true
	}
	return false
}


// isRankyMemberLogout is true when upstream bounced the session to the Ranky
// member portal (https://member.rankytools.com?return_url=...).
func isRankyMemberLogout(loc string) bool {
	low := strings.ToLower(strings.TrimSpace(loc))
	if low == "" {
		return false
	}
	return strings.Contains(low, "member.rankytools.com") ||
		strings.Contains(low, "member.rankytools.co")
}

// isRankyReturnURLBounce detects the ERR_TOO_MANY_REDIRECTS loop where member
// portal Location is rewritten onto our public origin with ?return_url=..., or
// the browser is already carrying a nested return_url from a prior bounce.
func isRankyReturnURLBounce(r *http.Request, loc, publicOrigin string) bool {
	locLow := strings.ToLower(strings.TrimSpace(loc))
	reqReturn := ""
	if r != nil {
		reqReturn = strings.TrimSpace(r.URL.Query().Get("return_url"))
	}
	hasLocReturn := strings.Contains(locLow, "return_url=")
	if reqReturn == "" && !hasLocReturn && !isRankyMemberLogout(loc) {
		return false
	}
	if isRankyMemberLogout(loc) {
		return true
	}
	// Nested return_url already in the browser URL = loop in progress.
	if reqReturn != "" && (strings.Contains(strings.ToLower(reqReturn), "return_url=") ||
		strings.Contains(strings.ToLower(reqReturn), "member.rankytools.")) {
		return true
	}
	if !hasLocReturn {
		return false
	}
	origin := strings.TrimRight(strings.ToLower(strings.TrimSpace(publicOrigin)), "/")
	if origin != "" && (strings.HasPrefix(locLow, origin) || locLow == origin+"/" || strings.HasPrefix(locLow, "/")) {
		return true
	}
	// Request already bouncing and upstream answered with another return_url.
	if reqReturn != "" && hasLocReturn {
		return true
	}
	return false
}

// homeRedirectLoop is an inner app path → / (or public origin root) after a dead session.
// Covers legacy /home/ and the analytics overview landing used as home_path.
func homeRedirectLoop(reqPath, location, publicOrigin string) bool {
	path := strings.ToLower(strings.TrimSpace(reqPath))
	switch path {
	case "/home", "/home/", "/analytics/overview", "/analytics/overview/":
		// ok
	default:
		return false
	}
	loc := strings.TrimSpace(location)
	if loc == "" {
		return false
	}
	origin := strings.TrimRight(strings.TrimSpace(publicOrigin), "/")
	switch strings.TrimRight(loc, "/") {
	case "", "/", origin:
		return true
	}
	if origin != "" && (loc == origin+"/" || strings.EqualFold(loc, origin)) {
		return true
	}
	return false
}

func noteLogout(cfg Config, r *http.Request, acc ToolAccount, sessionToken, reason string) (switched bool, nextName string) {
	if acc.ID > 0 {
		if prev, ok := logoutSeen.Load(acc.ID); ok {
			if time.Since(prev.(time.Time)) < 2*time.Minute {
				return false, ""
			}
		}
		logoutSeen.Store(acc.ID, time.Now())
	}
	db, err := openPanelDB(cfg)
	if err != nil {
		return false, ""
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var websiteID int
	var username, loginIP string
	if sessionToken != "" {
		_ = db.QueryRow(`SELECT website_id, username, COALESCE(client_ip, '') FROM live_sessions WHERE session_token=?`, sessionToken).Scan(&websiteID, &username, &loginIP)
	}
	if websiteID == 0 {
		_ = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID)
	}
	if websiteID > 0 {
		_, _ = db.Exec(`INSERT INTO automation_signals (website_id, created_at, state) VALUES (?,?,'pending')`, websiteID, now)
	}
	ip := ""
	if r != nil {
		ip = realClientIP(r)
	}
	if ip == "" || ip == "127.0.0.1" || ip == "::1" {
		ip = loginIP
	}
	if strings.TrimSpace(reason) == "" {
		reason = "logout"
	}
	next, err := scanPanelAccount(db.QueryRow(panelAccountSelect+`
		WHERE w.domain = ? AND a.status = 'active' AND TRIM(a.cookie) <> '' AND a.id <> ?
		`+panelAccountOrder+` LIMIT 1`, cfg.PublicHost, acc.ID))
	if err == nil {
		_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, next.ID)
		if sessionToken != "" {
			_, _ = db.Exec(`UPDATE live_sessions SET assigned_account_id=? WHERE session_token=?`, next.ID, sessionToken)
		}
		nextName = next.Name
		switched = true
		_, _ = db.Exec(`INSERT INTO switch_events (website_id, username, from_account_name, to_account_name, reason, switched_at) VALUES (?,?,?,?,?,?)`,
			websiteID, username, acc.Name, nextName, reason, now)
		log.Printf("[LB] switched %s -> %s user=%s", acc.Name, nextName, username)
	}
	_, _ = db.Exec(`INSERT INTO logout_events (website_id, username, account_name, next_account_name, reason, client_ip, created_at) VALUES (?,?,?,?,?,?,?)`,
		websiteID, username, acc.Name, nextName, reason, ip, now)
	log.Printf("[LB] logout recorded account=%s user=%s reason=%s", acc.Name, username, reason)
	// Do NOT mark the account logged_out here. That flips the tool to
	// Temporarily Unavailable (no retry). Keep status active so the browser
	// stays on "Signing you in" and reloads every 3s while automation
	// refreshes the cookie (and Shared-cookies followers).
	return switched, nextName
}
