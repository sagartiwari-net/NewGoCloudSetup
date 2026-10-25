package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type panelGateSession struct {
	mu          sync.Mutex
	username    string
	expires     time.Time
	fp          string
	proof       string
	tracked     bool
	liveChecked time.Time
	liveOK      bool
}

type ToolAccount struct {
	ID        int
	Name      string
	Cookie    string
	ShowLimit bool
}

type browserCookieEntry struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	HostOnly bool   `json:"hostOnly"`
}

var (
	panelDBOnce sync.Once
	panelDB     *sql.DB
	panelDBErr  error
	panelSess   sync.Map
	panelPickMu sync.Mutex
)

func openPanelDB(cfg Config) (*sql.DB, error) {
	panelDBOnce.Do(func() {
		panelDB, panelDBErr = sql.Open("mysql", panelMySQLDSN(cfg))
		if panelDBErr != nil {
			return
		}
		panelDB.SetMaxOpenConns(4)
		panelDB.SetConnMaxLifetime(5 * time.Minute)
		panelDBErr = panelDB.Ping()
	})
	return panelDB, panelDBErr
}

func panelMySQLDSN(cfg Config) string {
	host := strings.TrimSpace(os.Getenv("GT4RENTS_MYSQL_HOST"))
	if host == "" {
		host = "127.0.0.1"
	}
	port := strings.TrimSpace(os.Getenv("GT4RENTS_MYSQL_PORT"))
	if port == "" {
		port = "3306"
	}
	user := strings.TrimSpace(os.Getenv("GT4RENTS_MYSQL_USER"))
	if user == "" {
		user = "gt4rents"
	}
	name := strings.TrimSpace(os.Getenv("GT4RENTS_MYSQL_DB"))
	if name == "" {
		name = "gt4rents"
	}
	pass := os.Getenv("GT4RENTS_MYSQL_PASSWORD")
	if pass == "" {
		pass = cfg.MySQLPassword
	}
	return url.UserPassword(user, pass).String() + "@tcp(" + host + ":" + port + ")/" + name + "?charset=utf8mb4&parseTime=false&sql_mode=ANSI_QUOTES"
}

func realClientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.TrimSpace(strings.Split(ip, ",")[0])
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return strings.TrimSpace(ip)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func requestHost(r *http.Request) string {
	host := strings.TrimSpace(r.Host)
	if i := strings.LastIndex(host, ":"); i > 0 && strings.Count(host, ":") == 1 {
		host = host[:i]
	}
	return host
}

func panelSessionUsername(r *http.Request) (string, error) {
	cookie, err := r.Cookie("ct_session")
	if err != nil || cookie.Value == "" {
		return "", fmt.Errorf("missing session")
	}
	raw, ok := panelSess.Load(cookie.Value)
	if !ok {
		restored, restoreErr := restorePanelSession(cookie.Value)
		if restoreErr != nil {
			return "", fmt.Errorf("session not found")
		}
		panelSess.Store(cookie.Value, restored)
		raw = restored
	}
	sess := raw.(*panelGateSession)
	sess.mu.Lock()
	expired := time.Now().After(sess.expires)
	username := sess.username
	tracked := sess.tracked
	fresh := sess.liveOK && time.Since(sess.liveChecked) < 3*time.Second
	sess.mu.Unlock()
	if expired {
		panelSess.Delete(cookie.Value)
		return "", fmt.Errorf("session expired")
	}
	if tracked && !fresh {
		okLive := panelLiveSessionExists(cookie.Value)
		sess.mu.Lock()
		sess.liveChecked = time.Now()
		sess.liveOK = okLive
		sess.mu.Unlock()
		if !okLive {
			panelSess.Delete(cookie.Value)
			log.Printf("[PANEL] session ended from panel user=%s", username)
			return "", fmt.Errorf("session ended")
		}
	}
	extendPanelSession(cookie.Value, sess)
	return username, nil
}

func panelSessionDuration() time.Duration {
	cfg := loadConfig()
	db, err := openPanelDB(cfg)
	if err != nil {
		return 30 * time.Minute
	}
	var mins int
	_ = db.QueryRow(`SELECT session_duration FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&mins)
	if mins <= 0 {
		mins = 30
	}
	return time.Duration(mins) * time.Minute
}

func extendPanelSession(token string, sess *panelGateSession) {
	window := panelSessionDuration()
	sess.mu.Lock()
	if time.Until(sess.expires) > window/3 {
		sess.mu.Unlock()
		return
	}
	sess.expires = time.Now().Add(window)
	exp := sess.expires.UTC().Format(time.RFC3339)
	sess.mu.Unlock()
	db, err := openPanelDB(loadConfig())
	if err != nil {
		return
	}
	_, _ = db.Exec(`UPDATE live_sessions SET expires_at=? WHERE session_token=?`, exp, token)
}

func restorePanelSession(token string) (*panelGateSession, error) {
	cfg := loadConfig()
	db, err := openPanelDB(cfg)
	if err != nil {
		return nil, err
	}
	var username, expires string
	err = db.QueryRow(`SELECT s.username, s.expires_at FROM live_sessions s
		JOIN websites w ON w.id = s.website_id
		WHERE s.session_token = ? AND w.domain = ?`, token, cfg.PublicHost).Scan(&username, &expires)
	if err != nil {
		return nil, err
	}
	exp, err := time.Parse(time.RFC3339, expires)
	if err != nil || time.Now().After(exp) {
		return nil, fmt.Errorf("session expired")
	}
	return &panelGateSession{
		username:    username,
		expires:     exp,
		tracked:     true,
		liveOK:      true,
		liveChecked: time.Now(),
	}, nil
}

func panelLiveSessionExists(sessionToken string) bool {
	cfg := loadConfig()
	db, err := openPanelDB(cfg)
	if err != nil {
		return false
	}
	var n int
	err = db.QueryRow(`SELECT COUNT(*) FROM live_sessions s
		JOIN websites w ON w.id = s.website_id
		WHERE s.session_token = ? AND w.domain = ?`, sessionToken, cfg.PublicHost).Scan(&n)
	return err == nil && n > 0
}

func dropPanelSessionsFor(username string) {
	panelSess.Range(func(key, value any) bool {
		sess := value.(*panelGateSession)
		sess.mu.Lock()
		name := sess.username
		sess.mu.Unlock()
		if name == username {
			panelSess.Delete(key)
		}
		return true
	})
}

func issuePanelSession(username string, duration time.Duration) (string, time.Time, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", time.Time{}, err
	}
	token := hex.EncodeToString(buf)
	expires := time.Now().Add(duration)
	dropPanelSessionsFor(username)
	panelSess.Store(token, &panelGateSession{username: username, expires: expires})
	return token, expires, nil
}

func servePanelAccess(w http.ResponseWriter, r *http.Request, cfg Config) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		renderAccessDeniedPage(w, cfg)
		return
	}
	db, err := openPanelDB(cfg)
	if err != nil {
		log.Printf("[PANEL] database open failed: %v", err)
		renderAccessDeniedPage(w, cfg)
		return
	}
	reqHost := requestHost(r)
	var dbUser, productID, clientIP string
	var websiteID, durationMin int
	err = db.QueryRow(`SELECT username, COALESCE(product_id, ''), COALESCE(client_ip, ''), website_id FROM access_tokens
		WHERE token = ? AND expires_at > ?
		AND website_id IN (
			SELECT id FROM websites WHERE lower(trim(domain)) IN (lower(trim(?)), lower(trim(?)))
		)`,
		token, time.Now().UTC().Format(time.RFC3339), cfg.PublicHost, reqHost).
		Scan(&dbUser, &productID, &clientIP, &websiteID)
	if err != nil {
		log.Printf("[PANEL] token rejected host=%s err=%v", reqHost, err)
		renderAccessDeniedPage(w, cfg)
		return
	}
	username := strings.TrimSpace(dbUser)
	if username == "" {
		renderAccessDeniedPage(w, cfg)
		return
	}
	var domain string
	err = db.QueryRow(`SELECT domain, session_duration FROM websites WHERE id = ?`, websiteID).Scan(&domain, &durationMin)
	if err != nil {
		renderAccessDeniedPage(w, cfg)
		return
	}
	_, _ = db.Exec(`DELETE FROM access_tokens WHERE token=?`, token)
	acc, accErr := claimPanelAccount(cfg)
	if accErr != nil {
		log.Printf("[PANEL] no mapped account: %v", accErr)
		renderNoActiveAccountsPage(w, cfg)
		return
	}
	if durationMin <= 0 {
		durationMin = cfg.SessionDurationMinutes
	}
	if durationMin <= 0 {
		durationMin = 30
	}
	sessionToken, sessionExpiry, err := issuePanelSession(username, time.Duration(durationMin)*time.Minute)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	seen := realClientIP(r)
	if clientIP != "" && (seen == "" || seen == "127.0.0.1" || seen == "::1") {
		seen = clientIP
	}
	recordPanelLogin(db, domain, username, sessionToken, seen, r.UserAgent(), sessionExpiry, acc.ID)
	http.SetCookie(w, &http.Cookie{
		Name:     "ct_session",
		Value:    sessionToken,
		Path:     "/",
		Expires:  sessionExpiry,
		HttpOnly: true,
		Secure:   cookieSecure(r, cfg),
		SameSite: http.SameSiteLaxMode,
	})
	log.Printf("[PANEL] access granted user=%s", username)
	renderPanelLoadingPage(w, cfg)
}

const panelAccountSelect = `SELECT a.id, a.name, a.cookie, a.show_limit
	FROM accounts a
	JOIN websites w ON w.id = a.website_id`

const panelAccountOrder = `ORDER BY CASE WHEN a.last_used_at = '' THEN 0 ELSE 1 END, a.last_used_at ASC, a.id ASC`

func claimPanelAccount(cfg Config) (ToolAccount, error) {
	panelPickMu.Lock()
	defer panelPickMu.Unlock()
	db, err := openPanelDB(cfg)
	if err != nil {
		return ToolAccount{}, err
	}
	acc, err := scanPanelAccount(db.QueryRow(panelAccountSelect+`
		WHERE w.domain = ? AND a.status = 'active' AND TRIM(a.cookie) <> ''
		`+panelAccountOrder+` LIMIT 1`, cfg.PublicHost))
	if err != nil {
		return ToolAccount{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, acc.ID)
	log.Printf("[LB] claimed account %s id=%d", acc.Name, acc.ID)
	return acc, nil
}

func loadPanelSessionAccount(cfg Config, sessionToken string) (ToolAccount, error) {
	panelPickMu.Lock()
	defer panelPickMu.Unlock()
	db, err := openPanelDB(cfg)
	if err != nil {
		return ToolAccount{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var assigned int
	err = db.QueryRow(`SELECT assigned_account_id FROM live_sessions WHERE session_token=? AND expires_at > ?`, sessionToken, now).Scan(&assigned)
	if err != nil {
		return ToolAccount{}, err
	}
	if assigned > 0 {
		acc, accErr := scanPanelAccount(db.QueryRow(panelAccountSelect+`
			WHERE a.id = ? AND w.domain = ? AND a.status = 'active' AND TRIM(a.cookie) <> ''`, assigned, cfg.PublicHost))
		if accErr == nil {
			return acc, nil
		}
	}
	acc, err := scanPanelAccount(db.QueryRow(panelAccountSelect+`
		WHERE w.domain = ? AND a.status = 'active' AND TRIM(a.cookie) <> ''
		`+panelAccountOrder+` LIMIT 1`, cfg.PublicHost))
	if err != nil {
		return ToolAccount{}, err
	}
	_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, acc.ID)
	_, _ = db.Exec(`UPDATE live_sessions SET assigned_account_id=? WHERE session_token=?`, acc.ID, sessionToken)
	return acc, nil
}

func scanPanelAccount(row *sql.Row) (ToolAccount, error) {
	var acc ToolAccount
	var showLimit int
	if err := row.Scan(&acc.ID, &acc.Name, &acc.Cookie, &showLimit); err != nil {
		return ToolAccount{}, err
	}
	acc.ShowLimit = showLimit == 1
	acc.Cookie = parseCookieFromDB(acc.Cookie)
	if strings.TrimSpace(acc.Cookie) == "" {
		return ToolAccount{}, fmt.Errorf("mapped account cookie is empty")
	}
	return acc, nil
}

func recordPanelLogin(db *sql.DB, domain, username, sessionToken, clientIP, userAgent string, expires time.Time, accountID int) {
	var websiteID int
	if err := db.QueryRow(`SELECT id FROM websites WHERE domain=?`, domain).Scan(&websiteID); err != nil {
		log.Printf("[PANEL] login not recorded: %v", err)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = db.Exec(`DELETE FROM live_sessions WHERE username=? AND website_id=?`, username, websiteID)
	_, err := db.Exec(`INSERT INTO live_sessions (website_id, session_token, username, client_ip, fingerprint, expires_at, created_at, assigned_account_id) VALUES (?,?,?,?,?,?,?,?)`,
		websiteID, sessionToken, username, clientIP, "", expires.UTC().Format(time.RFC3339), now, accountID)
	if err != nil {
		log.Printf("[PANEL] live session insert failed: %v", err)
		return
	}
	if raw, ok := panelSess.Load(sessionToken); ok {
		sess := raw.(*panelGateSession)
		sess.mu.Lock()
		sess.tracked = true
		sess.mu.Unlock()
	}
	if _, err = db.Exec(`INSERT INTO login_events (website_id, username, client_ip, user_agent, logged_in_at) VALUES (?,?,?,?,?)`,
		websiteID, username, clientIP, userAgent, now); err != nil {
		log.Printf("[PANEL] login event insert failed: %v", err)
	}
	noteAccessPattern(db, websiteID, username, clientIP, userAgent)
}

func parseCookieFromDB(raw string) string {
	raw = strings.TrimSpace(strings.NewReplacer("\r", "", "\n", "", "\x00", "").Replace(raw))
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "{") {
		var wrap struct {
			Cookies []browserCookieEntry `json:"cookies"`
		}
		if json.Unmarshal([]byte(raw), &wrap) == nil && len(wrap.Cookies) > 0 {
			return cookieEntriesToHeader(wrap.Cookies)
		}
		return raw
	}
	if !strings.HasPrefix(raw, "[") {
		return raw
	}
	var cookies []browserCookieEntry
	if json.Unmarshal([]byte(raw), &cookies) != nil {
		return raw
	}
	return cookieEntriesToHeader(cookies)
}

func cookieEntriesToHeader(cookies []browserCookieEntry) string {
	type scored struct {
		value string
		score int
	}
	best := map[string]scored{}
	var order []string
	for _, c := range cookies {
		name := strings.TrimSpace(c.Name)
		value := strings.TrimSpace(c.Value)
		if name == "" || value == "" {
			continue
		}
		score := 1
		if c.HostOnly {
			score += 2
		}
		prev, ok := best[name]
		if !ok {
			order = append(order, name)
		}
		if !ok || score >= prev.score {
			best[name] = scored{value: value, score: score}
		}
	}
	parts := make([]string, 0, len(order))
	for _, name := range order {
		parts = append(parts, name+"="+best[name].value)
	}
	return strings.Join(parts, "; ")
}

func cookieSecure(r *http.Request, cfg Config) bool {
	if strings.EqualFold(strings.TrimSpace(cfg.PublicScheme), "http") {
		return false
	}
	if r.TLS != nil {
		return true
	}
	if strings.Contains(strings.ToLower(r.Header.Get("X-Forwarded-Proto")), "https") {
		return true
	}
	return strings.EqualFold(cfg.PublicScheme, "https")
}

func sessionTokenFromRequest(r *http.Request) string {
	c, err := r.Cookie("ct_session")
	if err != nil {
		return ""
	}
	return c.Value
}

// ensureRecloudSite creates the panel website row so the Recloud page lists this tool.
// An existing upstream_url is left alone. A cookie.json file is imported only when the account cookie is empty.
func ensureRecloudSite(cfg Config) {
	db, err := openPanelDB(cfg)
	if err != nil {
		log.Printf("[PANEL] recloud site not ensured: %v", err)
		return
	}
	name := toolDisplayName(cfg)
	upstream := strings.TrimSpace(cfg.TargetURL)
	var websiteID, linkedTool int
	var currentURL string
	err = db.QueryRow(`SELECT id, tool_id, COALESCE(upstream_url, '') FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID, &linkedTool, &currentURL)
	if err != nil {
		toolID := ensureRecloudTool(db, name)
		if toolID == 0 {
			return
		}
		buf := make([]byte, 16)
		_, _ = rand.Read(buf)
		secret := hex.EncodeToString(buf)
		res, insErr := db.Exec(`INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url)
			VALUES (?,?,?,?,30,'{}',1,'recloud',?)`, toolID, name, cfg.PublicHost, secret, upstream)
		if insErr != nil {
			log.Printf("[PANEL] website insert failed: %v", insErr)
			return
		}
		id, _ := res.LastInsertId()
		websiteID = int(id)
		log.Printf("[PANEL] recloud website created id=%d host=%s name=%s", websiteID, cfg.PublicHost, name)
		importCookieFile(db, websiteID)
		return
	}
	toolID := recloudToolID(db, name, linkedTool)
	if toolID == 0 {
		return
	}
	nextURL := strings.TrimSpace(currentURL)
	if nextURL == "" {
		nextURL = upstream
	}
	_, _ = db.Exec(`UPDATE websites SET tool_id=?, name=?, kind='recloud', upstream_url=? WHERE id=?`, toolID, name, nextURL, websiteID)
	_, _ = db.Exec(`UPDATE accounts SET name=? WHERE website_id=?`, name, websiteID)
	// A dead session used to mark the only account logged out, which replaces the tool with Temporarily Unavailable.
	_, _ = db.Exec(`UPDATE accounts SET status='active' WHERE website_id=? AND status='logged_out' AND TRIM(cookie) <> ''`, websiteID)
	importCookieFile(db, websiteID)
}

func ensureRecloudTool(db *sql.DB, name string) int {
	var toolID int
	if db.QueryRow(`SELECT id FROM tools WHERE name=?`, name).Scan(&toolID) == nil {
		return toolID
	}
	res, err := db.Exec(`INSERT INTO tools (name, category, limits_json) VALUES (?,?,?)`, name, "recloud", "[]")
	if err != nil {
		log.Printf("[PANEL] tool insert failed: %v", err)
		return 0
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// recloudToolID keeps one tool row for this website and renames it to the group label.
func recloudToolID(db *sql.DB, name string, linkedTool int) int {
	var linkedName, linkedCategory string
	if db.QueryRow(`SELECT name, COALESCE(category, '') FROM tools WHERE id=?`, linkedTool).Scan(&linkedName, &linkedCategory) == nil && linkedCategory == "recloud" {
		if linkedName == name {
			return linkedTool
		}
		var taken int
		if db.QueryRow(`SELECT id FROM tools WHERE name=? AND id<>?`, name, linkedTool).Scan(&taken) == nil {
			return taken
		}
		if _, err := db.Exec(`UPDATE tools SET name=? WHERE id=?`, name, linkedTool); err != nil {
			log.Printf("[PANEL] tool rename failed: %v", err)
			return linkedTool
		}
		return linkedTool
	}
	return ensureRecloudTool(db, name)
}

func importCookieFile(db *sql.DB, websiteID int) {
	body, err := os.ReadFile("cookie.json")
	if err != nil || len(strings.TrimSpace(string(body))) == 0 {
		return
	}
	header := parseCookieFromDB(string(body))
	if header == "" {
		return
	}
	var accountID int
	var existing string
	err = db.QueryRow(`SELECT id, cookie FROM accounts WHERE website_id=? ORDER BY id ASC LIMIT 1`, websiteID).Scan(&accountID, &existing)
	if err == nil {
		if strings.TrimSpace(existing) != "" {
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		_, _ = db.Exec(`UPDATE accounts SET cookie=?, status='active', cookie_updated_at=? WHERE id=?`, string(body), now, accountID)
		log.Printf("[PANEL] imported cookie file into empty account id=%d", accountID)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = db.Exec(`INSERT INTO accounts (website_id, name, cookie, user_agent_id, user_agent, proxy_id, proxy, status, show_limit, description, automation_task_uid, automation_ingest_key, cookie_updated_at, last_used_at)
		VALUES (?,?,?,NULL,'',NULL,'','active',1,'','','',?,'')`, websiteID, toolDisplayName(loadConfig()), string(body), now)
	log.Printf("[PANEL] imported cookie file into a new account")
}

func liveUpstream(cfg Config) string {
	fallback := strings.TrimRight(strings.TrimSpace(cfg.TargetURL), "/")
	if fallback == "" {
		fallback = "https://testingg.one"
	}
	db, err := openPanelDB(cfg)
	if err != nil {
		return fallback
	}
	var raw string
	err = db.QueryRow(`SELECT COALESCE(upstream_url, '') FROM websites WHERE domain=? AND kind='recloud'`, cfg.PublicHost).Scan(&raw)
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if err != nil || raw == "" {
		return fallback
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fallback
	}
	return raw
}

func renderLoggedOutPage(w http.ResponseWriter, cfg Config) {
	writeLightCard(w, http.StatusForbidden, lightCard{
		Title:       "Logged out",
		Heading:     "Signing you in",
		Message:     "Tools signed out. Signing you in again.",
		Footer:      "This page checks again in a few seconds",
		ExtraScript: loggedOutRefreshScript(),
	})
}

func loggedOutRefreshScript() string {
	return `<script>(function(){var k="oneclickgo-relogin",n=0;try{n=Number(sessionStorage.getItem(k)||"0")}catch(e){}if(n>=8)return;try{sessionStorage.setItem(k,String(n+1))}catch(e){}setTimeout(function(){location.reload()},n===0?3000:4000)})();</script>`
}

func renderAccessDeniedPage(w http.ResponseWriter, cfg Config) {
	writeLightCard(w, http.StatusForbidden, lightCard{
		Title:   "Access Denied",
		Heading: "Access Denied",
		Message: "You cannot open Tools directly. Open it again from your access link.",
		Footer:  "Your session ended or this browser is not authorized",
	})
}

func renderNoActiveAccountsPage(w http.ResponseWriter, cfg Config) {
	writeLightCard(w, http.StatusServiceUnavailable, lightCard{
		Title:   "Temporarily Unavailable",
		Heading: "Temporarily Unavailable",
		Message: "Tools is temporarily unavailable. Please try again in a few minutes.",
		Footer:  "Open this tool again from your access link",
	})
}

func renderPanelLoadingPage(w http.ResponseWriter, cfg Config) {
	home := cfg.HomePath
	if home == "" {
		home = "/"
	}
	writeLightCard(w, http.StatusOK, lightCard{
		Title:       "Authenticating",
		Heading:     "Authenticating...",
		Message:     "You are using Tools. Please wait a moment while we verify your secure access request.",
		Badge:       "Verifying your request...",
		Footer:      "Secure session initialization in progress",
		Spin:        true,
		ExtraScript: deviceBootScript(home),
	})
}

type lightCard struct {
	Title       string
	Heading     string
	Message     string
	Badge       string
	Footer      string
	Spin        bool
	ExtraScript string
}

func writeLightCard(w http.ResponseWriter, status int, card lightCard) {
	spinClass := "ring"
	if card.Spin {
		spinClass = "ring spin"
	}
	badge := ""
	if card.Badge != "" {
		badge = `<div class="pill"><span class="dot"></span>` + html.EscapeString(card.Badge) + `</div>`
	}
	footer := ""
	if card.Footer != "" {
		footer = `<p class="foot">` + html.EscapeString(card.Footer) + `</p>`
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1.0">
<title>%s</title>
<style>
* { box-sizing:border-box;margin:0;padding:0; }
body { min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;background:#eef3f8;color:#0f172a;font-family:system-ui,-apple-system,Segoe UI,sans-serif; }
.card { width:min(440px,100%%);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px 36px;text-align:center; }
.ring { width:78px;height:78px;margin:0 auto 22px;border-radius:50%%;background:conic-gradient(#3b82f6 0 70deg,#e7eef8 70deg 360deg);display:grid;place-items:center; }
.ring.spin { animation:spin .9s linear infinite; }
.lock { width:64px;height:64px;border-radius:50%%;background:#fff;display:grid;place-items:center;font-size:26px; }
.ring.spin .lock { animation:spin .9s linear infinite reverse; }
h1 { font-size:28px;line-height:1.2;font-weight:800;letter-spacing:-.03em;margin-bottom:12px; }
.msg { color:#64748b;font-size:15px;line-height:1.55; }
.brand { color:#2563eb;font-weight:700; }
.pill { margin:22px auto 0;display:inline-flex;align-items:center;gap:8px;padding:8px 14px;border:1px solid #e6ebf2;border-radius:999px;color:#334155;font-size:14px;background:#fff; }
.dot { width:14px;height:14px;border-radius:50%%;border:2px solid #dbe4f0;border-top-color:#3b82f6;animation:spin .8s linear infinite; }
.foot { margin-top:18px;color:#94a3b8;font-size:13px; }
@keyframes spin { to { transform:rotate(360deg); } }
</style>
</head>
<body>
<div class="card">
<div class="%s"><div class="lock">🔒</div></div>
<h1>%s</h1>
<p class="msg">%s</p>
%s
%s
</div>
%s
</body>
</html>`, html.EscapeString(card.Title), spinClass, html.EscapeString(card.Heading), card.Message, badge, footer, card.ExtraScript)
}

func noteAccessPattern(db *sql.DB, websiteID int, username, clientIP, userAgent string) {
	sameCount := panelSettingInt(db, "access_same_count", 4)
	sameMinutes := panelSettingInt(db, "access_same_minutes", 30)
	now := time.Now().UTC()
	var tool string
	_ = db.QueryRow(`SELECT name FROM websites WHERE id=?`, websiteID).Scan(&tool)
	if tool == "" {
		tool = "this tool"
	}
	var same int
	_ = db.QueryRow(`SELECT COUNT(*) FROM login_events WHERE username=? AND website_id=? AND logged_in_at>=?`,
		username, websiteID, now.Add(-time.Duration(sameMinutes)*time.Minute).Format(time.RFC3339)).Scan(&same)
	if same < sameCount {
		return
	}
	reason := tool + " opened " + strconv.Itoa(same) + " times in " + strconv.Itoa(sameMinutes) + " minutes"
	var recent int
	_ = db.QueryRow(`SELECT COUNT(*) FROM security_events WHERE username=? AND website_id=? AND event_type='access_pattern' AND details=? AND created_at>=?`,
		username, websiteID, reason, now.Add(-30*time.Minute).Format(time.RFC3339)).Scan(&recent)
	if recent > 0 {
		return
	}
	_, _ = db.Exec(`INSERT INTO security_events (website_id, username, client_ip, event_type, attempted_url, details, user_agent, created_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		websiteID, username, clientIP, "access_pattern", "", reason, userAgent, now.Format(time.RFC3339))
}

func panelSettingInt(db *sql.DB, key string, fallback int) int {
	var raw string
	if err := db.QueryRow(`SELECT value FROM settings WHERE "key"=?`, key).Scan(&raw); err != nil {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return fallback
	}
	return n
}
