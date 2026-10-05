package main

import (
	"os"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"strconv"
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
	return username, nil
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
	username := strings.TrimSpace(r.URL.Query().Get("user"))
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if username == "" || token == "" {
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
		log.Printf("[PANEL] token rejected user=%s err=%v", username, err)
		renderAccessDeniedPage(w, cfg)
		return
	}
	var domain string
	err = db.QueryRow(`SELECT domain, COALESCE(session_duration, 30) FROM websites WHERE id = ?`, websiteID).Scan(&domain, &durationMin)
	if err != nil || domain != cfg.PublicHost || dbUser != username {
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

func listActivePanelAccounts(cfg Config, excludeID int) ([]ToolAccount, error) {
	panelPickMu.Lock()
	defer panelPickMu.Unlock()
	db, err := openPanelDB(cfg)
	if err != nil {
		return nil, err
	}
	var rows *sql.Rows
	if excludeID > 0 {
		rows, err = db.Query(panelAccountSelect+`
			WHERE w.domain = ? AND a.status = 'active' AND a.cookie != '' AND a.id != ?
			`+panelAccountOrder, cfg.PublicHost, excludeID)
	} else {
		rows, err = db.Query(panelAccountSelect+`
			WHERE w.domain = ? AND a.status = 'active' AND a.cookie != ''
			`+panelAccountOrder, cfg.PublicHost)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ToolAccount
	for rows.Next() {
		acc, scanErr := scanPanelAccount(rows)
		if scanErr != nil {
			continue
		}
		out = append(out, acc)
	}
	return out, rows.Err()
}

func pinPanelAccount(cfg Config, sessionToken string, acc ToolAccount, bumpFailureID int, record bool, currentName, username, reason string) {
	panelPickMu.Lock()
	defer panelPickMu.Unlock()
	db, err := openPanelDB(cfg)
	if err != nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if record && bumpFailureID > 0 {
		_, _ = db.Exec(`UPDATE accounts SET failure_count=failure_count+1 WHERE id=?`, bumpFailureID)
	}
	_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, acc.ID)
	if sessionToken != "" {
		_, _ = db.Exec(`UPDATE live_sessions SET assigned_account_id=? WHERE session_token=?`, acc.ID, sessionToken)
	}
	if !record {
		return
	}
	var websiteID int
	_ = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID)
	if websiteID > 0 && currentName != "" {
		tmRecordSwitchLogout(db, websiteID, username, currentName, acc.Name, reason)
		log.Printf("[LB] switched '%s' (ID:%d) -> '%s' (ID:%d) reason=%s", currentName, bumpFailureID, acc.Name, acc.ID, reason)
		notify := panelSwitchNote{websiteID: websiteID, username: username, from: currentName, to: acc.Name, reason: reason, sessionToken: sessionToken, at: now}
		go notifyPanelSwitch(cfg, notify)
	}
}

// claimPanelAccount picks the least recently used active account with a live ChatGPT session.
// Dead cookies still render the SPA shell (no send button) — probe /api/auth/session first.
func claimPanelAccount(cfg Config) (ToolAccount, error) {
	cands, err := listActivePanelAccounts(cfg, 0)
	if err != nil {
		return ToolAccount{}, err
	}
	if len(cands) == 0 {
		return ToolAccount{}, fmt.Errorf("no active account")
	}
	var lastErr error
	for _, acc := range cands {
		ok, reason := probeChatGPTSession(cfg, acc)
		if !ok {
			log.Printf("[LB] skip '%s' (ID:%d) session_probe=%s", acc.Name, acc.ID, reason)
			lastErr = fmt.Errorf("session_probe:%s", reason)
			continue
		}
		pinPanelAccount(cfg, "", acc, 0, false, "", "", "")
		log.Printf("[LB] claimed account '%s' (ID:%d) for %s (session ok)", acc.Name, acc.ID, cfg.PublicHost)
		return acc, nil
	}
	if lastErr != nil {
		return ToolAccount{}, fmt.Errorf("all ChatGPT cookies logged out (%d tried): %w", len(cands), lastErr)
	}
	return ToolAccount{}, fmt.Errorf("all ChatGPT cookies logged out (%d tried)", len(cands))
}

// loadPanelSessionAccount uses the account pinned on the live session.
// If the pinned cookie is logged out, rotate to the next live account (never keep a dead shell open).
func loadPanelSessionAccount(cfg Config, sessionToken string) (ToolAccount, error) {
	panelPickMu.Lock()
	db, err := openPanelDB(cfg)
	if err != nil {
		panelPickMu.Unlock()
		return ToolAccount{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var assigned int
	err = db.QueryRow(`SELECT assigned_account_id FROM live_sessions WHERE session_token=? AND expires_at > ?`, sessionToken, now).Scan(&assigned)
	var assignedAcc ToolAccount
	var assignedOK bool
	if err == nil && assigned > 0 {
		if acc, accErr := scanPanelAccount(db.QueryRow(panelAccountSelect+`
			WHERE a.id = ? AND w.domain = ? AND a.status = 'active' AND a.cookie != ''`, assigned, cfg.PublicHost)); accErr == nil {
			assignedAcc = acc
			assignedOK = true
		}
	}
	panelPickMu.Unlock()
	if err != nil {
		return ToolAccount{}, err
	}

	if assignedOK {
		ok, reason := probeChatGPTSession(cfg, assignedAcc)
		if ok {
			return assignedAcc, nil
		}
		log.Printf("[LB] assigned '%s' (ID:%d) logged out (%s) — rotating", assignedAcc.Name, assignedAcc.ID, reason)
		next, swErr := panelSwitchAccount(cfg, sessionToken, assignedAcc.ID, assignedAcc.Name, "", "html_failover:session_probe:"+reason)
		if swErr == nil {
			return next, nil
		}
		return ToolAccount{}, fmt.Errorf("assigned account logged out and no live alternate: %w", swErr)
	}

	acc, err := claimPanelAccount(cfg)
	if err != nil {
		return ToolAccount{}, err
	}
	pinPanelAccount(cfg, sessionToken, acc, 0, false, "", "", "")
	log.Printf("[LB] auto-assigned '%s' (ID:%d) to session", acc.Name, acc.ID)
	return acc, nil
}

// panelSwitchAccount moves this session to the next live account (session probe required).
// failure_count goes up. status stays active so the account can be used again later.
func panelSwitchAccount(cfg Config, sessionToken string, currentID int, currentName, username, reason string) (ToolAccount, error) {
	cands, err := listActivePanelAccounts(cfg, currentID)
	if err != nil {
		return ToolAccount{}, err
	}
	var lastErr error
	for _, acc := range cands {
		ok, probeReason := probeChatGPTSession(cfg, acc)
		if !ok {
			log.Printf("[LB] switch-skip '%s' (ID:%d) session_probe=%s", acc.Name, acc.ID, probeReason)
			lastErr = fmt.Errorf("session_probe:%s", probeReason)
			continue
		}
		record := shouldRecordFailover(currentID, reason)
		pinPanelAccount(cfg, sessionToken, acc, currentID, record, currentName, username, reason)
		return acc, nil
	}
	if lastErr != nil {
		return ToolAccount{}, fmt.Errorf("no other live account: %w", lastErr)
	}
	return ToolAccount{}, fmt.Errorf("no other active account")
}

var (
	panelFailoverLogMu  sync.Mutex
	panelFailoverLogged = map[int]time.Time{}
)

// Repeated login-wall retries must keep rotating the account, without a failure row every few seconds.
func shouldRecordFailover(accountID int, reason string) bool {
	if accountID <= 0 || !strings.Contains(reason, "html_failover:") {
		return true
	}
	panelFailoverLogMu.Lock()
	defer panelFailoverLogMu.Unlock()
	if last, ok := panelFailoverLogged[accountID]; ok && time.Since(last) < 45*time.Second {
		return false
	}
	panelFailoverLogged[accountID] = time.Now()
	return true
}

type panelSwitchNote struct {
	websiteID    int
	username     string
	from         string
	to           string
	reason       string
	sessionToken string
	at           string
}

func notifyPanelSwitch(cfg Config, note panelSwitchNote) {
	if note.websiteID == 0 {
		return
	}
	db, err := openPanelDB(cfg)
	if err != nil {
		return
	}
	var clientIP string
	_ = db.QueryRow(`SELECT client_ip FROM live_sessions WHERE session_token=?`, note.sessionToken).Scan(&clientIP)
	var tool, website string
	_ = db.QueryRow(`SELECT t.name, w.name FROM websites w JOIN tools t ON t.id=w.tool_id WHERE w.id=?`, note.websiteID).Scan(&tool, &website)
	if tool == "" {
		tool = toolDisplayName(cfg)
	}
	var routeID int
	var eventsRaw string
	if db.QueryRow(`SELECT id, events_json FROM telegram_routes WHERE website_id=?`, note.websiteID).Scan(&routeID, &eventsRaw) != nil || !strings.Contains(eventsRaw, "logout") {
		insertPanelDelivery(db, note.websiteID, note.username, "skipped", nil, note.at)
		return
	}
	rows, err := db.Query(`SELECT d.label, d.chat_id, o.username FROM telegram_route_chats c JOIN telegram_destinations d ON d.id=c.destination_id JOIN operators o ON o.id=d.reseller_id WHERE c.route_id=? AND d.enabled=1`, routeID)
	recipients := []map[string]string{}
	if err == nil {
		for rows.Next() {
			var label, chat, reseller string
			_ = rows.Scan(&label, &chat, &reseller)
			recipients = append(recipients, map[string]string{"label": label, "chat_id": chat, "reseller": reseller})
		}
		rows.Close()
	}
	minutes := 60
	var raw string
	_ = db.QueryRow(`SELECT value FROM settings WHERE key='repeat_minutes'`).Scan(&raw)
	if n, convErr := strconv.Atoi(raw); convErr == nil && n > 0 {
		minutes = n
	}
	status := "sent"
	windowStart := time.Now().UTC().Add(-time.Duration(minutes) * time.Minute).Format(time.RFC3339)
	var recent int
	_ = db.QueryRow(`SELECT COUNT(*) FROM switch_events WHERE website_id=? AND username=? AND from_account_name=? AND switched_at>=?`, note.websiteID, note.username, note.from, windowStart).Scan(&recent)
	if recent > 1 {
		status = "skipped"
	}
	var token, tmpl string
	_ = db.QueryRow(`SELECT value FROM settings WHERE key='bot_token'`).Scan(&token)
	_ = db.QueryRow(`SELECT value FROM settings WHERE key='logout_template'`).Scan(&tmpl)
	if strings.TrimSpace(tmpl) == "" || tmpl == "{tool} logged out for {username} on {website} at {time}." {
		tmpl = "{tool}: {from} logged out. Switched {username} to {to} on {website} at {time}. {reason}"
	}
	if !strings.Contains(tmpl, "{from}") {
		tmpl += " Switched {from} to {to}."
	}
	text := strings.NewReplacer(
		"{tool}", tool, "{username}", note.username, "{website}", website,
		"{time}", note.at, "{ip}", clientIP, "{reason}", note.reason, "{count}", "",
		"{from}", note.from, "{to}", note.to,
	).Replace(tmpl)
	if strings.TrimSpace(token) == "" {
		status = "skipped"
		log.Printf("[TG] bot token is empty, switch recorded without a message")
	}
	if status == "sent" {
		for _, rec := range recipients {
			postPanelTelegram(token, rec["chat_id"], text)
		}
	}
	insertPanelDelivery(db, note.websiteID, note.username, status, recipients, note.at)
}

func insertPanelDelivery(db *sql.DB, websiteID int, username, status string, recipients []map[string]string, at string) {
	raw, _ := json.Marshal(recipients)
	if recipients == nil {
		raw = []byte("[]")
	}
	if at == "" {
		at = time.Now().UTC().Format(time.RFC3339)
	}
	_, _ = db.Exec(`INSERT INTO telegram_deliveries (website_id, username, event, recipients_json, status, created_at) VALUES (?,?,?,?,?,?)`,
		websiteID, username, "logout", string(raw), status, at)
}

func postPanelTelegram(token, chatID, text string) {
	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", text)
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.PostForm("https://api.telegram.org/bot"+token+"/sendMessage", form)
	if err != nil {
		log.Printf("[TG] send failed: %v", err)
		return
	}
	resp.Body.Close()
}

type panelAccountScanner interface {
	Scan(dest ...any) error
}

func scanPanelAccount(row panelAccountScanner) (ToolAccount, error) {
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
