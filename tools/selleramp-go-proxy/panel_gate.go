package main

import (
	"os"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"html"
	"log"
	"net/http"
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


func panelSQLiteDSN(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		if _, err := os.Stat("/www/wwwroot/gt4rents.com/panel/data"); err == nil {
			path = "/www/wwwroot/gt4rents.com/panel/data/panel.db"
		} else {
			path = "/Users/sagartiwari/Desktop/oneclickgo/pending-tools/panel-api/data/panel.db"
		}
	}
	q := "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	if strings.Contains(path, "busy_timeout") {
		// already a full DSN
		return path
	}
	if strings.HasPrefix(path, "/") {
		return "file://" + path + q
	}
	return "file:" + path + q
}

func usesPanelAccountMode(cfg Config) bool {
	return strings.TrimSpace(cfg.PanelDB) != ""
}

func openPanelDB(cfg Config) (*sql.DB, error) {
	panelDBOnce.Do(func() {
		panelDB, panelDBErr = sql.Open("sqlite", panelSQLiteDSN(cfg.PanelDB))
		if panelDBErr != nil {
			return
		}
		panelDB.SetMaxOpenConns(1)
		panelDBErr = panelDB.Ping()
	})
	return panelDB, panelDBErr
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
	// Prefer domain match, but fall back to token-only — domain string drift
	// (trim/case/old host) was leaving valid cookies unrestorable.
	err = db.QueryRow(`SELECT s.username, s.expires_at FROM live_sessions s
		JOIN websites w ON w.id = s.website_id
		WHERE s.session_token = ? AND lower(trim(w.domain)) = lower(trim(?))`, token, cfg.PublicHost).Scan(&username, &expires)
	if err != nil {
		err = db.QueryRow(`SELECT username, expires_at FROM live_sessions WHERE session_token = ?`, token).Scan(&username, &expires)
		if err != nil {
			return nil, err
		}
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
		WHERE s.session_token = ? AND lower(trim(w.domain)) = lower(trim(?))`, sessionToken, cfg.PublicHost).Scan(&n)
	if err != nil || n == 0 {
		err = db.QueryRow(`SELECT COUNT(*) FROM live_sessions WHERE session_token = ?`, sessionToken).Scan(&n)
		if err != nil {
			log.Printf("[PANEL] live session lookup failed: %v", err)
			return false
		}
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
	reqHost := strings.TrimSpace(r.Host)
	if i := strings.LastIndex(reqHost, ":"); i > 0 && strings.Count(reqHost, ":") == 1 {
		reqHost = reqHost[:i]
	}
	var dbUser, productID, clientIP, expiresRaw string
	var websiteID, durationMin int
	// Consume OTT only when website domain matches public_host or request Host.
	err = db.QueryRow(`DELETE FROM access_tokens
		WHERE token = ? AND expires_at > ?
		AND website_id IN (
			SELECT id FROM websites WHERE lower(trim(domain)) IN (lower(trim(?)), lower(trim(?)))
		)
		RETURNING username, product_id, COALESCE(client_ip, ''), expires_at, website_id`,
		token, time.Now().UTC().Format(time.RFC3339), cfg.PublicHost, reqHost).
		Scan(&dbUser, &productID, &clientIP, &expiresRaw, &websiteID)
	if err != nil {
		log.Printf("[PANEL] token rejected host=%s public_host=%s err=%v", reqHost, cfg.PublicHost, err)
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
	if err != nil {
		log.Printf("[PANEL] session_duration lookup failed user=%s wid=%d err=%v", username, websiteID, err)
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
	recordPanelLogin(db, websiteID, username, sessionToken, seen, r.UserAgent(), sessionExpiry, accID)
	// Bust any stale ct_session before issuing the new one (orphan cookies caused
	// bind/home to look up a dead token while the fresh session sat unused).
	http.SetCookie(w, &http.Cookie{
		Name:     "ct_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   cookieSecure(r, cfg),
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "ct_session",
		Value:    sessionToken,
		Path:     "/",
		Expires:  sessionExpiry,
		MaxAge:   int(time.Until(sessionExpiry).Seconds()),
		HttpOnly: true,
		Secure:   cookieSecure(r, cfg),
		SameSite: http.SameSiteLaxMode,
	})
	home := cfg.HomePath
	if home == "" {
		home = "/"
	}
	log.Printf("[PANEL] access granted user=%s product=%s domain=%s wid=%d → %s", username, productID, domain, websiteID, home)
	// 302 + Set-Cookie is more reliable than the Authenticating HTML boot page
	// (device bind races / wrong cookie). Device proof attaches on the first HTML page.
	http.Redirect(w, r, home, http.StatusFound)
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

// claimPanelAccount picks the least recently used active account and stamps last_used_at.
// Status is left unchanged. The next login then lands on a different account.
func claimPanelAccount(cfg Config) (ToolAccount, error) {
	panelPickMu.Lock()
	defer panelPickMu.Unlock()
	db, err := openPanelDB(cfg)
	if err != nil {
		return ToolAccount{}, err
	}
	acc, err := scanPanelAccount(db.QueryRow(panelAccountSelect+`
		WHERE w.domain = ? AND a.status = 'active' AND a.cookie != ''
		`+panelAccountOrder+` LIMIT 1`, cfg.PublicHost))
	if err != nil {
		return ToolAccount{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, acc.ID)
	log.Printf("[LB] claimed account '%s' (ID:%d) for %s", acc.Name, acc.ID, cfg.PublicHost)
	return acc, nil
}

// loadPanelSessionAccount uses the account pinned on the live session.
// assigned_account_id 0 means auto: claim the least recently used account and pin it.
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
			WHERE a.id = ? AND w.domain = ? AND a.status = 'active' AND a.cookie != ''`, assigned, cfg.PublicHost))
		if accErr == nil {
			return acc, nil
		}
	}
	acc, err := scanPanelAccount(db.QueryRow(panelAccountSelect+`
		WHERE w.domain = ? AND a.status = 'active' AND a.cookie != ''
		`+panelAccountOrder+` LIMIT 1`, cfg.PublicHost))
	if err != nil {
		return ToolAccount{}, err
	}
	_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, acc.ID)
	_, _ = db.Exec(`UPDATE live_sessions SET assigned_account_id=? WHERE session_token=?`, acc.ID, sessionToken)
	log.Printf("[LB] auto-assigned '%s' (ID:%d) to session", acc.Name, acc.ID)
	return acc, nil
}

// panelSwitchAccount moves this session to the next active account.
// failure_count goes up. status stays active so the account can be used again later.
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

func recordPanelLogin(db *sql.DB, websiteID int, username, sessionToken, clientIP, userAgent string, expires time.Time, accountID int) {
	if websiteID <= 0 {
		log.Printf("[PANEL] login not recorded: invalid website_id")
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
		sess.liveOK = true
		sess.liveChecked = time.Now()
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


func cookieSecure(r *http.Request, cfg Config) bool {
	// Pre-SSL: overlays use public_scheme=http. Never mark cookies Secure or CF X-Forwarded-Proto=https drops them on http:// pages.
	if strings.EqualFold(strings.TrimSpace(cfg.PublicScheme), "http") {
		return false
	}
	if r.TLS != nil {
		return true
	}
	if strings.Contains(strings.ToLower(r.Header.Get("X-Forwarded-Proto")), "https") {
		return true
	}
	if r.Header.Get("X-Forwarded-Ssl") == "on" {
		return true
	}
	return strings.EqualFold(cfg.PublicScheme, "https")
}
