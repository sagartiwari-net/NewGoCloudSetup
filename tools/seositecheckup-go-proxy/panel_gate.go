package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
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

var (
	panelDBOnce sync.Once
	panelDB     *sql.DB
	panelDBErr  error
	panelSess   sync.Map
)


// canonicalPanelDBPath is the Update Panel database — the only allowed cookie source on server.
const canonicalPanelDBPath = "/www/wwwroot/gt4rents.com/panel/data/panel.db"

// resolvePanelDBPath returns the Update Panel sqlite path.
// Never use a relative/local DB next to the tool binary (./panel.db, tool-dir data/).
func resolvePanelDBPath(cfg Config) string {
	path := strings.TrimSpace(cfg.PanelDB)
	if path != "" && !filepath.IsAbs(path) {
		log.Printf("[PANEL] refusing relative panel_db=%q — cookies must come from Update Panel DB", path)
		path = ""
	}
	// Guard: tool OUTDIR copies / local sqlite are not the Update Panel DB.
	if path != "" {
		base := strings.ToLower(filepath.Base(path))
		if base != "panel.db" {
			log.Printf("[PANEL] refusing non-panel.db path=%q — forcing Update Panel DB", path)
			path = ""
		}
		if strings.Contains(path, "/seositecheckup/") || strings.Contains(path, "/_repo/") {
			log.Printf("[PANEL] refusing tool-local path=%q — forcing Update Panel DB", path)
			path = ""
		}
	}
	if path == "" {
		if _, err := os.Stat(filepath.Dir(canonicalPanelDBPath)); err == nil {
			path = canonicalPanelDBPath
		} else {
			path = "/Users/sagartiwari/Desktop/oneclickgo/pending-tools/panel-api/data/panel.db"
		}
	}
	return path
}

func panelSQLiteDSN(path string) string {
	path = strings.TrimSpace(path)
	q := "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	if strings.Contains(path, "busy_timeout") {
		return path
	}
	if strings.HasPrefix(path, "/") {
		return "file://" + path + q
	}
	return "file:" + path + q
}

func usesPanelAccountMode(cfg Config) bool {
	return strings.TrimSpace(cfg.PanelDB) != "" || fileExists(canonicalPanelDBPath)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func openPanelDB(cfg Config) (*sql.DB, error) {
	panelDBOnce.Do(func() {
		path := resolvePanelDBPath(cfg)
		log.Printf("[PANEL] opening Update Panel DB for cookies path=%s", path)
		panelDB, panelDBErr = sql.Open("sqlite", panelSQLiteDSN(path))
		if panelDBErr != nil {
			return
		}
		panelDB.SetMaxOpenConns(1)
		panelDBErr = panelDB.Ping()
		if panelDBErr != nil {
			log.Printf("[PANEL] ping failed path=%s err=%v", path, panelDBErr)
		}
	})
	return panelDB, panelDBErr
}

// pickPanelAccount loads cookie from Update Panel accounts table only.
// Prefers status=active; if none, revives a logged_out row that still has a cookie
// (admin re-pasted cookie in Update Panel but status may still say logged_out).
func pickPanelAccount(db *sql.DB, domain string, excludeID int) (ToolAccount, error) {
	try := func(status string) (ToolAccount, error) {
		q := panelAccountSelect + `
			WHERE w.domain = ? AND a.status = ? AND a.cookie != ''`
		args := []interface{}{domain, status}
		if excludeID > 0 {
			q += ` AND a.id != ?`
			args = append(args, excludeID)
		}
		q += panelAccountOrder + ` LIMIT 1`
		return scanPanelAccount(db.QueryRow(q, args...))
	}
	acc, err := try("active")
	if err == nil {
		return acc, nil
	}
	acc, err = try("logged_out")
	if err != nil {
		return ToolAccount{}, err
	}
	_, _ = db.Exec(`UPDATE accounts SET status='active', failure_count=0 WHERE id=?`, acc.ID)
	log.Printf("[PANEL] revived account id=%d name=%q from panel.db (cookie present, was logged_out)", acc.ID, acc.Name)
	return acc, nil
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
		log.Printf("[PANEL] session restored user=%s", restored.username)
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
		log.Printf("[PANEL] live session check failed: %v", err)
		return false
	}
	var n int
	err = db.QueryRow(`SELECT COUNT(*) FROM live_sessions s
		JOIN websites w ON w.id = s.website_id
		WHERE s.session_token = ? AND w.domain = ?`, sessionToken, cfg.PublicHost).Scan(&n)
	if err != nil {
		log.Printf("[PANEL] live session lookup failed: %v", err)
		return false
	}
	return n > 0
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
	var dbUser, productID, clientIP, expiresRaw string
	var websiteID, durationMin int
	err = db.QueryRow(`DELETE FROM access_tokens
		WHERE token = ? AND expires_at > ?
		RETURNING username, product_id, COALESCE(client_ip, ''), expires_at, website_id`,
		token, time.Now().UTC().Format(time.RFC3339)).
		Scan(&dbUser, &productID, &clientIP, &expiresRaw, &websiteID)
	if err != nil {
		log.Printf("[PANEL] token rejected err=%v", err)
		renderAccessDeniedPage(w, cfg)
		return
	}
	username := strings.TrimSpace(dbUser)
	if username == "" {
		log.Printf("[PANEL] token has empty username")
		renderAccessDeniedPage(w, cfg)
		return
	}
	var domain string
	err = db.QueryRow(`SELECT domain, COALESCE(session_duration, 120) FROM websites WHERE id = ?`, websiteID).Scan(&domain, &durationMin)
	if err != nil || domain != cfg.PublicHost {
		log.Printf("[PANEL] token mismatch user=%s domain=%s", username, domain)
		renderAccessDeniedPage(w, cfg)
		return
	}
	if _, err = time.Parse(time.RFC3339, expiresRaw); err != nil {
		renderAccessDeniedPage(w, cfg)
		return
	}
	acc, accErr := claimPanelAccount(cfg)
	if accErr != nil {
		log.Printf("[PANEL] no mapped account: %v", accErr)
		renderNoActiveAccountsPage(w, cfg)
		return
	}
	accID := acc.ID
	if durationMin <= 0 {
		durationMin = cfg.SessionDurationMinutes
	}
	if durationMin <= 0 {
		durationMin = 120
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
	recordPanelLogin(db, domain, username, sessionToken, seen, r.UserAgent(), sessionExpiry, accID)
	http.SetCookie(w, &http.Cookie{
		Name:     "ct_session",
		Value:    sessionToken,
		Path:     "/",
		Expires:  sessionExpiry,
		HttpOnly: true,
		Secure:   cookieSecure(r, cfg),
		SameSite: http.SameSiteLaxMode,
	})
	log.Printf("[PANEL] access granted user=%s product=%s", username, productID)
	renderPanelLoadingPage(w, cfg)
}

const panelAccountSelect = `SELECT a.id, a.name, a.cookie,
	CASE WHEN a.user_agent != '' THEN a.user_agent ELSE COALESCE(ua.user_agent, '') END,
	CASE WHEN TRIM(COALESCE(p.endpoint, '')) != '' THEN p.endpoint ELSE a.proxy END,
	a.show_limit
	FROM accounts a
	JOIN websites w ON w.id = a.website_id
	LEFT JOIN user_agents ua ON ua.id = a.user_agent_id
	LEFT JOIN proxies p ON p.id = a.proxy_id`

// Least recently used first. Unused accounts (empty last_used_at) go first.
const panelAccountOrder = `ORDER BY CASE WHEN a.last_used_at = '' THEN 0 ELSE 1 END, a.last_used_at ASC, a.id ASC`

var panelPickMu sync.Mutex

// claimPanelAccount picks the least recently used account from Update Panel DB and stamps last_used_at.
func claimPanelAccount(cfg Config) (ToolAccount, error) {
	panelPickMu.Lock()
	defer panelPickMu.Unlock()
	db, err := openPanelDB(cfg)
	if err != nil {
		return ToolAccount{}, err
	}
	acc, err := pickPanelAccount(db, cfg.PublicHost, 0)
	if err != nil {
		var nActive, nAny int
		_ = db.QueryRow(`SELECT COUNT(*) FROM accounts a JOIN websites w ON w.id=a.website_id WHERE w.domain=? AND a.status='active' AND a.cookie!=''`, cfg.PublicHost).Scan(&nActive)
		_ = db.QueryRow(`SELECT COUNT(*) FROM accounts a JOIN websites w ON w.id=a.website_id WHERE w.domain=? AND a.cookie!=''`, cfg.PublicHost).Scan(&nAny)
		log.Printf("[PANEL] no usable account domain=%s active_with_cookie=%d any_with_cookie=%d db=%s err=%v",
			cfg.PublicHost, nActive, nAny, resolvePanelDBPath(cfg), err)
		return ToolAccount{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, acc.ID)
	log.Printf("[LB] claimed account '%s' (ID:%d) for %s from panel.db", acc.Name, acc.ID, cfg.PublicHost)
	return acc, nil
}

// loadPanelSessionAccount uses the account pinned on the live session (cookie from panel.db only).
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
			WHERE a.id = ? AND w.domain = ? AND a.cookie != '' AND a.status IN ('active','logged_out')`, assigned, cfg.PublicHost))
		if accErr == nil {
			_, _ = db.Exec(`UPDATE accounts SET status='active', failure_count=0 WHERE id=? AND status='logged_out'`, acc.ID)
			return acc, nil
		}
	}
	acc, err := pickPanelAccount(db, cfg.PublicHost, 0)
	if err != nil {
		return ToolAccount{}, err
	}
	_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, acc.ID)
	_, _ = db.Exec(`UPDATE live_sessions SET assigned_account_id=? WHERE session_token=?`, acc.ID, sessionToken)
	log.Printf("[LB] auto-assigned '%s' (ID:%d) to session from panel.db", acc.Name, acc.ID)
	return acc, nil
}

// panelReloadAccount refreshes the mapped account cookie from Update Panel DB (same ID).
func panelReloadAccount(cfg Config, sessionToken string) (ToolAccount, error) {
	panelPickMu.Lock()
	defer panelPickMu.Unlock()
	db, err := openPanelDB(cfg)
	if err != nil {
		return ToolAccount{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var assigned int
	if err := db.QueryRow(`SELECT COALESCE(assigned_account_id, 0) FROM live_sessions WHERE session_token=? AND expires_at>?`, sessionToken, now).Scan(&assigned); err != nil {
		return ToolAccount{}, err
	}
	if assigned > 0 {
		acc, accErr := scanPanelAccount(db.QueryRow(panelAccountSelect+`
			WHERE a.id = ? AND w.domain = ? AND a.cookie != '' AND a.status IN ('active','logged_out')`, assigned, cfg.PublicHost))
		if accErr == nil {
			_, _ = db.Exec(`UPDATE accounts SET status='active', failure_count=0 WHERE id=? AND status='logged_out'`, acc.ID)
			return acc, nil
		}
	}
	return pickPanelAccount(db, cfg.PublicHost, 0)
}

func panelMarkAccountLoggedOut(cfg Config, accountID int, reason string) {
	if accountID <= 0 {
		return
	}
	db, err := openPanelDB(cfg)
	if err != nil {
		return
	}
	_, _ = db.Exec(`UPDATE accounts SET status='logged_out', failure_count=failure_count+1 WHERE id=?`, accountID)
	log.Printf("[LB] marked logged_out account=%d reason=%s", accountID, reason)
}

// panelSwitchToOtherAccount pins a different active account and records Analytics Switches + Logouts.
func panelSwitchToOtherAccount(cfg Config, sessionToken, reason string) (ToolAccount, string, error) {
	panelPickMu.Lock()
	defer panelPickMu.Unlock()
	db, err := openPanelDB(cfg)
	if err != nil {
		return ToolAccount{}, "", err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var websiteID, currentID int
	var username string
	err = db.QueryRow(`SELECT website_id, COALESCE(assigned_account_id, 0), username FROM live_sessions WHERE session_token=? AND expires_at>?`, sessionToken, now).
		Scan(&websiteID, &currentID, &username)
	if err != nil {
		return ToolAccount{}, "", err
	}
	if reason == "" {
		reason = "ssc-logout-failover"
	}
	var fromName string
	_ = db.QueryRow(`SELECT name FROM accounts WHERE id=?`, currentID).Scan(&fromName)
	acc, err := pickPanelAccount(db, cfg.PublicHost, currentID)
	if err != nil {
		return ToolAccount{}, "", fmt.Errorf("no other active account")
	}
	_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, acc.ID)
	_, _ = db.Exec(`UPDATE live_sessions SET assigned_account_id=? WHERE session_token=?`, acc.ID, sessionToken)
	if websiteID > 0 {
		tmRecordSwitchLogout(db, websiteID, username, fromName, acc.Name, reason)
	}
	log.Printf("[LB] switched %s -> %s reason=%s", fromName, acc.Name, reason)
	return acc, acc.Name, nil
}

func scanPanelAccount(row *sql.Row) (ToolAccount, error) {
	var acc ToolAccount
	var showLimit int
	if err := row.Scan(&acc.ID, &acc.Name, &acc.Cookie, &acc.UserAgent, &acc.Proxy, &showLimit); err != nil {
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

func toolDisplayName(cfg Config) string {
	name := strings.TrimSpace(cfg.ToolName)
	if name == "" {
		return "Tool"
	}
	return name
}


func renderProxyProblem(w http.ResponseWriter, r *http.Request) {
	if isDocumentNavigation(r) || strings.Contains(r.Header.Get("Accept"), "text/html") {
		writeLightCard(w, http.StatusBadGateway, lightCard{
			Title:   "Proxy problem",
			Heading: "Proxy problem",
			Message: "Contact to Admin/Provider to fix it ASAP",
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	fmt.Fprintf(w, `{"error":"proxy_unavailable","message":"Contact to Admin/Provider to fix it ASAP"}`)
}

func renderPanelLoadingPage(w http.ResponseWriter, cfg Config) {
	home := cfg.HomePath
	if home == "" {
		home = "/"
	}
	name := html.EscapeString(toolDisplayName(cfg))
	writeLightCard(w, http.StatusOK, lightCard{
		Title:       "Authenticating",
		Heading:     "Authenticating...",
		Message:     "You are using <span class=\"brand\">" + name + "</span>. Please wait a moment while we verify your secure access request.",
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
	Redirect    string
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
	redirect := ""
	if card.Redirect != "" {
		redirect = `<script>setTimeout(function(){ window.location.replace(` + fmt.Sprintf("%q", card.Redirect) + `); }, 1200);</script>`
	}
	redirect += card.ExtraScript
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
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
</html>`, html.EscapeString(card.Title), spinClass, html.EscapeString(card.Heading), card.Message, badge, footer, redirect)
}
