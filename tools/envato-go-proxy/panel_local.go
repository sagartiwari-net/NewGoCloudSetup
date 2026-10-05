package main

import (
	"os"
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	envatoPanelDBFallback   = "/Users/sagartiwari/Desktop/oneclickgo/pending-tools/panel-api/data/panel.db"
	sessionCookieName       = "ct_session"
	legacySessionCookieName = "envato_session"
)

type envatoLocalSession struct {
	mu          sync.Mutex
	username    string
	expires     time.Time
	fp          string
	proof       string
	tracked     bool
	liveChecked time.Time
	liveOK      bool
	duration    time.Duration
}

var (
	envatoPanelOnce sync.Once
	envatoPanel     *sql.DB
	envatoPanelErr  error
	envatoSessions  sync.Map
	envatoPickMu    sync.Mutex // serializes claim/switch so concurrent Access opens round-robin
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

func panelDBPath() string {
	cfg := loadConfig()
	if p := strings.TrimSpace(cfg.PanelDB); p != "" {
		return p
	}
	if v := strings.TrimSpace(os.Getenv("PANEL_DB")); v != "" {
		return v
	}
	if _, err := os.Stat("/www/wwwroot/gt4rents.com/panel/data"); err == nil {
		return "/www/wwwroot/gt4rents.com/panel/data/panel.db"
	}
	return envatoPanelDBFallback
}

func openEnvatoPanel() (*sql.DB, error) {
	envatoPanelOnce.Do(func() {
		path := panelDBPath()
		envatoPanel, envatoPanelErr = sql.Open("sqlite", panelSQLiteDSN(path))
		if envatoPanelErr == nil {
			envatoPanel.SetMaxOpenConns(1)
			envatoPanelErr = envatoPanel.Ping()
		}
		if envatoPanelErr == nil {
			log.Printf("[PANEL] opened %s", path)
		}
	})
	return envatoPanel, envatoPanelErr
}

func readSessionCookie(r *http.Request) (*http.Cookie, error) {
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		return c, nil
	}
	return r.Cookie(legacySessionCookieName)
}

func setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	// Drop legacy cookie name so only ChatGPT-pattern ct_session remains.
	http.SetCookie(w, &http.Cookie{
		Name:     legacySessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func serveEnvatoPanelAccess(w http.ResponseWriter, r *http.Request, username, token string) {
	cfg := loadConfig()
	token = strings.TrimSpace(token)
	if token == "" {
		renderAccessDeniedPage(w)
		return
	}
	db, err := openEnvatoPanel()
	if err != nil {
		log.Printf("[PANEL] database open failed: %v", err)
		renderAccessDeniedPage(w)
		return
	}
	var dbUser, clientIP string
	err = db.QueryRow(`DELETE FROM access_tokens
		WHERE token = ? AND expires_at > ?
		AND website_id IN (SELECT id FROM websites WHERE domain IN (?, ?))
		RETURNING username, COALESCE(client_ip, '')`,
		token, time.Now().UTC().Format(time.RFC3339), "127.0.0.1:5261", cfg.PublicHost).
		Scan(&dbUser, &clientIP)
	username = strings.TrimSpace(dbUser)
	if err != nil || username == "" {
		log.Printf("[PANEL] token rejected err=%v", err)
		renderAccessDeniedPage(w)
		return
	}
	var userStatus string
	_ = db.QueryRow(`SELECT status FROM panel_users WHERE website_id IN (SELECT id FROM websites WHERE domain IN (?, ?)) AND username=?`,
		"127.0.0.1:5261", cfg.PublicHost, username).Scan(&userStatus)
	if userStatus == "suspended" {
		log.Printf("[PANEL] suspended user=%s", username)
		renderAccessDeniedPage(w)
		return
	}
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		renderAccessDeniedPage(w)
		return
	}
	sessionToken := hex.EncodeToString(buf)
	seenIP := panelClientIP(r, clientIP)
	websiteID, accountID, minutes := envatoSiteAccount(db, cfg.PublicHost)
	if minutes < 1 {
		minutes = 120
	}
	if websiteID == 0 || accountID == 0 {
		log.Printf("[PANEL] no active Envato account for access user=%s", username)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, renderWaitingForAccountPage())
		return
	}
	expires := time.Now().Add(time.Duration(minutes) * time.Minute)
	envatoSessions.Store(sessionToken, &envatoLocalSession{
		username: username,
		expires:  expires,
		tracked:  true,
		liveOK:   true,
		duration: time.Duration(minutes) * time.Minute,
	})
	recordEnvatoLogin(db, websiteID, accountID, username, sessionToken, seenIP, r.UserAgent(), expires)
	setSessionCookie(w, sessionToken, expires)
	log.Printf("[PANEL] access granted user=%s ip=%s account=%d", username, seenIP, accountID)
	page := lightMessageHTML(
		"Authenticating",
		"Authenticating...",
		`You are using <span class="brand">Envato</span>. Please wait a moment while we verify your secure access request.`,
		`<div class="pill"><span class="dot"></span>Verifying your request...</div><p class="foot">Secure session initialization in progress</p>`,
		"",
	)
	page = strings.Replace(page, "</body>", deviceBootScript(defaultLandingPath)+"</body>", 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func envatoPanelSessionOK(r *http.Request) bool {
	cookie, err := readSessionCookie(r)
	if err != nil || cookie.Value == "" {
		return false
	}
	raw, ok := envatoSessions.Load(cookie.Value)
	if !ok {
		if !restoreEnvatoSession(cookie.Value) {
			return false
		}
		raw, ok = envatoSessions.Load(cookie.Value)
		if !ok {
			return false
		}
	}
	sess := raw.(*envatoLocalSession)
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if time.Now().After(sess.expires) {
		envatoSessions.Delete(cookie.Value)
		return false
	}
	if sess.tracked {
		if time.Since(sess.liveChecked) > 3*time.Second {
			sess.liveChecked = time.Now()
			sess.liveOK = envatoLiveRowExists(cookie.Value)
		}
		if !sess.liveOK {
			envatoSessions.Delete(cookie.Value)
			return false
		}
	}
	return true
}

// envatoPanelProxyCandidates prefers the account proxy, then other active panel proxies.
func envatoPanelProxyCandidates(primary string) []*url.URL {
	seen := map[string]bool{}
	var out []*url.URL
	add := func(raw string) {
		px := parseProxyString(strings.TrimSpace(raw))
		if px == nil || seen[px.Host] {
			return
		}
		seen[px.Host] = true
		out = append(out, px)
	}
	add(primary)
	db, err := openEnvatoPanel()
	if err != nil {
		return out
	}
	rows, qErr := db.Query(`SELECT endpoint FROM proxies WHERE status='active' AND TRIM(endpoint) != '' ORDER BY id`)
	if qErr != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var endpoint string
		if rows.Scan(&endpoint) == nil {
			add(endpoint)
		}
	}
	return out
}

// envatoPanelAssets uses the account pinned on the live session (ChatGPT-style).
// It never "upgrades" to a stronger cookie/proxy — that broke round-robin.
func envatoPanelAssets(cfg Config, sessionToken string) (cookie, userAgent, proxy string) {
	userAgent = cfg.UserAgent
	db, err := openEnvatoPanel()
	if err != nil {
		return cookie, userAgent, proxy
	}
	envatoPickMu.Lock()
	defer envatoPickMu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	var accountID int
	if sessionToken != "" {
		_ = db.QueryRow(`SELECT assigned_account_id FROM live_sessions WHERE session_token=? AND expires_at>?`, sessionToken, now).Scan(&accountID)
	}
	panelCookie, panelUA, panelProxy, id, err := envatoAccountByID(db, cfg.PublicHost, accountID)
	if err != nil || strings.TrimSpace(panelCookie) == "" {
		panelCookie, panelUA, panelProxy, id, err = envatoClaimAccountLocked(db, cfg.PublicHost)
		if err != nil {
			log.Printf("[PANEL] account lookup: %v", err)
			return cookie, userAgent, proxy
		}
		if sessionToken != "" && id > 0 {
			_, _ = db.Exec(`UPDATE live_sessions SET assigned_account_id=? WHERE session_token=?`, id, sessionToken)
			log.Printf("[LB] auto-assigned account %d to envato session", id)
		}
	}
	if strings.TrimSpace(panelCookie) != "" {
		cookie = panelCookie
	}
	if strings.TrimSpace(panelUA) != "" {
		userAgent = panelUA
	}
	if strings.TrimSpace(panelProxy) != "" {
		proxy = panelProxy
	}
	return cookie, userAgent, proxy
}

func envatoAccountByID(db *sql.DB, publicHost string, accountID int) (cookie, userAgent, proxy string, id int, err error) {
	if accountID <= 0 {
		return "", "", "", 0, fmt.Errorf("no assigned account")
	}
	err = db.QueryRow(`SELECT a.id, a.cookie,
		CASE WHEN a.user_agent != '' THEN a.user_agent ELSE COALESCE(ua.user_agent, '') END,
		CASE WHEN TRIM(COALESCE(p.endpoint, '')) != '' THEN p.endpoint ELSE a.proxy END
		FROM accounts a
		JOIN websites w ON w.id = a.website_id
		LEFT JOIN user_agents ua ON ua.id = a.user_agent_id
		LEFT JOIN proxies p ON p.id = a.proxy_id
		WHERE a.id=? AND w.domain IN (?, ?) AND a.status='active' AND TRIM(a.cookie) != ''`,
		accountID, "127.0.0.1:5261", publicHost).Scan(&id, &cookie, &userAgent, &proxy)
	return cookie, userAgent, proxy, id, err
}

const envatoAccountSelect = `SELECT a.id, a.cookie,
		CASE WHEN a.user_agent != '' THEN a.user_agent ELSE COALESCE(ua.user_agent, '') END,
		CASE WHEN TRIM(COALESCE(p.endpoint, '')) != '' THEN p.endpoint ELSE a.proxy END
		FROM accounts a
		JOIN websites w ON w.id = a.website_id
		LEFT JOIN user_agents ua ON ua.id = a.user_agent_id
		LEFT JOIN proxies p ON p.id = a.proxy_id
		WHERE w.domain IN (?, ?) AND a.status='active' AND TRIM(a.cookie) != ''`

// ChatGPT-style round-robin: least recently used first; unused (empty last_used_at) first.
const envatoAccountOrder = ` ORDER BY CASE WHEN a.last_used_at = '' OR a.last_used_at IS NULL THEN 0 ELSE 1 END, a.last_used_at ASC, a.id ASC`

// envatoClaimAccountLocked picks the LRU active account and stamps last_used_at.
// Caller must hold envatoPickMu.
func envatoClaimAccountLocked(db *sql.DB, publicHost string) (cookie, userAgent, proxy string, id int, err error) {
	err = db.QueryRow(envatoAccountSelect+envatoAccountOrder+` LIMIT 1`, "127.0.0.1:5261", publicHost).
		Scan(&id, &cookie, &userAgent, &proxy)
	if err != nil {
		return "", "", "", 0, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, id)
	log.Printf("[LB] claimed account ID:%d for %s", id, publicHost)
	return cookie, userAgent, proxy, id, nil
}

// envatoPanelSwitch moves the session to the next LRU active account (instant logout failover).
func envatoPanelSwitch(sessionToken, reason string) (bool, string) {
	db, err := openEnvatoPanel()
	if err != nil || sessionToken == "" {
		return false, ""
	}
	envatoPickMu.Lock()
	defer envatoPickMu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	var websiteID, currentID int
	var username string
	err = db.QueryRow(`SELECT website_id, COALESCE(assigned_account_id, 0), username FROM live_sessions WHERE session_token=? AND expires_at>?`, sessionToken, now).Scan(&websiteID, &currentID, &username)
	if err != nil {
		return false, ""
	}
	if currentID > 0 {
		_, _ = db.Exec(`UPDATE accounts SET failure_count=failure_count+1 WHERE id=?`, currentID)
	}
	var nextID int
	var nextName string
	err = db.QueryRow(`SELECT a.id, a.name FROM accounts a
		WHERE a.website_id=? AND a.status='active' AND TRIM(a.cookie) != '' AND a.id != ?
		`+envatoAccountOrder+` LIMIT 1`, websiteID, currentID).Scan(&nextID, &nextName)
	if err != nil || nextID == 0 {
		log.Printf("[LB] no other envato account to switch to user=%s", username)
		return false, ""
	}
	if reason == "" {
		reason = "envato login wall"
	}
	_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, nextID)
	_, _ = db.Exec(`UPDATE live_sessions SET assigned_account_id=? WHERE session_token=?`, nextID, sessionToken)
	var fromName string
	_ = db.QueryRow(`SELECT name FROM accounts WHERE id=?`, currentID).Scan(&fromName)
	tmRecordSwitchLogout(db, websiteID, username, fromName, nextName, reason)
	cookieJarMu.Lock()
	for k := range cookieJar {
		delete(cookieJar, k)
	}
	cookieJarMu.Unlock()
	log.Printf("[LB] envato switched %s -> %s reason=%s", fromName, nextName, reason)
	return true, nextName
}

func handleEnvatoDeviceBind(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	cookie, err := readSessionCookie(r)
	if err != nil || cookie.Value == "" {
		renderAccessDeniedPage(w)
		return
	}
	raw, ok := envatoSessions.Load(cookie.Value)
	if !ok {
		renderAccessDeniedPage(w)
		return
	}
	sess := raw.(*envatoLocalSession)
	fp := strings.TrimSpace(r.Header.Get("X-Device-Fp"))
	proof := strings.TrimSpace(r.Header.Get("X-Device-Proof"))
	if err := bindEnvatoDevice(cookie.Value, sess, fp, proof); err != nil {
		if strings.Contains(err.Error(), "device mismatch") {
			recordEnvatoCookieShare(r, cookie.Value)
		}
		renderAccessDeniedPage(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"ok"}`)
}

func injectEnvatoDeviceScript(body, username string) string {
	script := devicePageScript() + envatoLimitWidgetScript() + envatoProfileScript(username) + envatoSignInSwitchScript()
	lower := strings.ToLower(body)
	if h := strings.Index(lower, "<head"); h >= 0 {
		if gt := strings.IndexByte(lower[h:], '>'); gt >= 0 {
			at := h + gt + 1
			return body[:at] + script + body[at:]
		}
	}
	return script + body
}

// envatoSignInSwitchScript watches for the logged-out "Sign in" nav and forces
// one account switch (with cooldown so Acc1↔Acc2 does not ping-pong).
func envatoSignInSwitchScript() string {
	return `<script data-tm-signin-switch="1">
(function(){
  var key = "tm_envato_signin_switch";
  var busy = false;
  function hasSignIn(){
    try {
      return !!(document.querySelector('[data-cy="sign-in"],[data-analytics-target="sign-in"],a[href*="sign_in"]'));
    } catch (e) { return false; }
  }
  function run(){
    if (busy || !hasSignIn()) return;
    var last = 0;
    try { last = parseInt(sessionStorage.getItem("tm_envato_switched_at") || "0", 10) || 0; } catch (e) {}
    if (last && (Date.now() - last) < 10000) return;
    var n = 0;
    try { n = parseInt(sessionStorage.getItem(key) || "0", 10) || 0; } catch (e) {}
    if (n >= 3) return;
    busy = true;
    try { sessionStorage.setItem(key, String(n + 1)); } catch (e) {}
    location.replace("/account-proxy/sign_in?to=envatoapp&tm_switch=1");
  }
  function arm(){
    setTimeout(run, 1200);
    setTimeout(run, 3000);
    try {
      var mo = new MutationObserver(function(){ run(); });
      mo.observe(document.documentElement, { childList: true, subtree: true });
      setTimeout(function(){ try { mo.disconnect(); } catch (e) {} }, 15000);
    } catch (e) {}
  }
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", arm);
  else arm();
})();
</script>`
}

func envatoProfileScript(username string) string {
	name := strconv.Quote(strings.TrimSpace(username))
	return `<script data-tm-profile="1">
(function(){
  var name = ` + name + `;
  function block(e){
    var t = e.target;
    if (!t || !t.closest) return;
    if (t.closest('[data-analytics-target="profile-menu"]')) {
      e.preventDefault();
      e.stopPropagation();
      e.stopImmediatePropagation();
    }
  }
  ['pointerdown','mousedown','click','keydown'].forEach(function(type){
    document.addEventListener(type, block, true);
  });
  function closeMenu(){
    document.querySelectorAll('[role="dialog"]').forEach(function(dlg){
      if (dlg.querySelector('[data-analytics-target="sign-out"], [data-analytics-target="my-downloads"]')) dlg.remove();
    });
  }
  function rename(){
    if (!name) return;
    var btn = document.querySelector('[data-analytics-target="profile-menu"]');
    if (!btn) return;
    btn.querySelectorAll('[title]').forEach(function(el){
      var title = el.getAttribute('title') || '';
      if (title && title !== name) el.setAttribute('title', name);
    });
    btn.querySelectorAll('title').forEach(function(el){
      if ((el.textContent || '').trim() && el.textContent.trim() !== name) el.textContent = name;
    });
    btn.querySelectorAll('span').forEach(function(el){
      if (el.children.length) return;
      var text = (el.textContent || '').trim();
      if (text && text !== name) el.textContent = name;
    });
  }
  function hideCredits(){
    document.querySelectorAll('button[data-cy="ai-generations-meter"]').forEach(function(btn){
      btn.style.setProperty('display', 'none', 'important');
      btn.setAttribute('aria-hidden', 'true');
    });
  }
  function tick(){ closeMenu(); rename(); hideCredits(); }
  var style = document.createElement('style');
  style.setAttribute('data-tm-hide-credits', '1');
  style.textContent = 'button[data-cy="ai-generations-meter"]{display:none !important;}';
  document.documentElement.appendChild(style);
  tick();
  setInterval(tick, 400);
})();
</script>`
}

func panelClientIP(r *http.Request, tokenIP string) string {
	seen := realClientIP(r)
	if seen == "" || seen == "127.0.0.1" || seen == "::1" {
		if strings.TrimSpace(tokenIP) != "" {
			return tokenIP
		}
	}
	return seen
}

func envatoSiteAccount(db *sql.DB, publicHost string) (websiteID, accountID, minutes int) {
	err := db.QueryRow(`SELECT w.id, COALESCE(w.session_duration, 120)
		FROM websites w
		WHERE w.domain IN ('127.0.0.1:5261', ?)
		LIMIT 1`, publicHost).Scan(&websiteID, &minutes)
	if err != nil {
		log.Printf("[PANEL] site lookup: %v", err)
		return websiteID, accountID, minutes
	}
	envatoPickMu.Lock()
	_, _, _, accountID, err = envatoClaimAccountLocked(db, publicHost)
	envatoPickMu.Unlock()
	if err != nil {
		log.Printf("[PANEL] account claim: %v", err)
		accountID = 0
	}
	return websiteID, accountID, minutes
}

func recordEnvatoLogin(db *sql.DB, websiteID, accountID int, username, sessionToken, clientIP, userAgent string, expires time.Time) {
	if websiteID == 0 {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = db.Exec(`DELETE FROM live_sessions WHERE username=? AND website_id=?`, username, websiteID)
	_, err := db.Exec(`INSERT INTO live_sessions (website_id, session_token, username, client_ip, fingerprint, expires_at, created_at, assigned_account_id)
		VALUES (?,?,?,?,?,?,?,?)`,
		websiteID, sessionToken, username, clientIP, "", expires.UTC().Format(time.RFC3339), now, accountID)
	if err != nil {
		log.Printf("[PANEL] live session insert failed: %v", err)
		return
	}
	if _, err = db.Exec(`INSERT INTO login_events (website_id, username, client_ip, user_agent, logged_in_at) VALUES (?,?,?,?,?)`,
		websiteID, username, clientIP, userAgent, now); err != nil {
		log.Printf("[PANEL] login event insert failed: %v", err)
	}
	noteAccessPattern(db, websiteID, username, clientIP, userAgent)
}

func envatoLiveRowExists(token string) bool {
	db, err := openEnvatoPanel()
	if err != nil {
		return true
	}
	var id int
	err = db.QueryRow(`SELECT id FROM live_sessions WHERE session_token=?`, token).Scan(&id)
	return err == nil
}

func restoreEnvatoSession(token string) bool {
	db, err := openEnvatoPanel()
	if err != nil {
		return false
	}
	var username, expiresRaw, fp string
	err = db.QueryRow(`SELECT username, expires_at, COALESCE(fingerprint, '') FROM live_sessions WHERE session_token=?`, token).Scan(&username, &expiresRaw, &fp)
	if err != nil {
		return false
	}
	expires, err := time.Parse(time.RFC3339, expiresRaw)
	if err != nil || time.Now().After(expires) {
		return false
	}
	minutes := 30
	_ = db.QueryRow(`SELECT COALESCE(w.session_duration, 30) FROM live_sessions s JOIN websites w ON w.id=s.website_id WHERE s.session_token=?`, token).Scan(&minutes)
	if minutes < 1 {
		minutes = 30
	}
	envatoSessions.Store(token, &envatoLocalSession{
		username: username,
		expires:  expires,
		fp:       fp,
		tracked:  true,
		liveOK:   true,
		duration: time.Duration(minutes) * time.Minute,
	})
	return true
}

func refreshEnvatoSessionCookie(w http.ResponseWriter, r *http.Request) {
	cookie, err := readSessionCookie(r)
	if err != nil || cookie.Value == "" {
		return
	}
	raw, ok := envatoSessions.Load(cookie.Value)
	if !ok {
		return
	}
	sess := raw.(*envatoLocalSession)
	sess.mu.Lock()
	expires := sess.expires
	sess.mu.Unlock()
	setSessionCookie(w, cookie.Value, expires)
}

func bindEnvatoDevice(token string, sess *envatoLocalSession, fp, proof string) error {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if time.Now().After(sess.expires) {
		envatoSessions.Delete(token)
		return fmt.Errorf("session expired")
	}
	if fp == "" || proof == "" {
		return fmt.Errorf("missing proof")
	}
	if fp == "missing" || proof == "missing" {
		envatoSessions.Delete(token)
		return fmt.Errorf("device mismatch")
	}
	if sess.proof == "" {
		if sess.fp != "" && sess.fp != fp {
			envatoSessions.Delete(token)
			return fmt.Errorf("device mismatch")
		}
		sess.fp = fp
		sess.proof = proof
	} else if sess.proof != proof || sess.fp != fp {
		envatoSessions.Delete(token)
		return fmt.Errorf("device mismatch")
	}
	db, err := openEnvatoPanel()
	if err == nil {
		_, _ = db.Exec(`UPDATE live_sessions SET fingerprint=? WHERE session_token=?`, fp, token)
	}
	return nil
}

func recordEnvatoCookieShare(r *http.Request, token string) {
	db, err := openEnvatoPanel()
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
	path, ua := "", ""
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

func envatoRejectDevice(w http.ResponseWriter, r *http.Request) bool {
	cookie, err := readSessionCookie(r)
	if err != nil || cookie.Value == "" {
		return false
	}
	raw, ok := envatoSessions.Load(cookie.Value)
	if !ok {
		return false
	}
	sess := raw.(*envatoLocalSession)
	fp := strings.TrimSpace(r.Header.Get("X-Device-Fp"))
	proof := strings.TrimSpace(r.Header.Get("X-Device-Proof"))
	sess.mu.Lock()
	bound := sess.proof != ""
	storedFp, storedProof := sess.fp, sess.proof
	username := sess.username
	sess.mu.Unlock()
	if !bound {
		return false
	}
	if fp == storedFp && proof == storedProof {
		return false
	}
	if browserSubresource(r) {
		return false
	}
	if fp == "" && proof == "" {
		if isDocumentNavigation(r) || strings.HasSuffix(r.URL.Path, ".data") || strings.HasPrefix(r.URL.Path, "/.well-known/") || isEnvatoAssetPath(r.URL.Path) {
			return false
		}
		log.Printf("[DEVICE] required path=%s user=%s", r.URL.Path, username)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"device_required","message":"Open this tool again from your access link."}`)
		return true
	}
	envatoSessions.Delete(cookie.Value)
	recordEnvatoCookieShare(r, cookie.Value)
	log.Printf("[DEVICE] session killed user=%s missing=%v", username, proof == "")
	if isDocumentNavigation(r) || strings.Contains(r.Header.Get("Accept"), "text/html") {
		renderAccessDeniedPage(w)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"device_mismatch","message":"Open this tool again from your access link."}`)
	}
	return true
}

func isEnvatoAssetPath(path string) bool {
	switch {
	case strings.HasPrefix(path, "/__domain__/"),
		strings.HasPrefix(path, "/__resized__/"),
		strings.HasPrefix(path, "/__video__/"),
		strings.HasPrefix(path, "/assets-proxy/"):
		return true
	default:
		return false
	}
}

func browserSubresource(r *http.Request) bool {
	switch strings.ToLower(r.Header.Get("Sec-Fetch-Dest")) {
	case "image", "style", "font", "audio", "video", "script", "empty":
		return true
	}
	path := strings.ToLower(r.URL.Path)
	if strings.HasPrefix(path, "/images/") ||
		strings.HasPrefix(path, "/__domain__/") ||
		strings.HasPrefix(path, "/cdn-cgi/") ||
		path == "/favicon.ico" ||
		strings.HasSuffix(path, ".avif") ||
		strings.HasSuffix(path, ".js") ||
		strings.HasSuffix(path, ".css") ||
		strings.HasSuffix(path, ".woff2") ||
		strings.HasSuffix(path, ".png") ||
		strings.HasSuffix(path, ".jpg") ||
		strings.HasSuffix(path, ".webp") ||
		strings.HasSuffix(path, ".svg") ||
		strings.HasSuffix(path, ".ico") {
		return true
	}
	return false
}

func isDocumentNavigation(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	mode := r.Header.Get("Sec-Fetch-Mode")
	dest := r.Header.Get("Sec-Fetch-Dest")
	if mode == "navigate" || dest == "document" || dest == "iframe" {
		return true
	}
	return mode == "" && dest == "" && strings.Contains(r.Header.Get("Accept"), "text/html")
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

type envatoLimitView struct {
	show  bool
	limit int
	used  int
	label string
}

func creditLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return time.FixedZone("IST", 5*3600+30*60)
	}
	return loc
}

func creditPeriodStart(resetDays int) time.Time {
	if resetDays < 1 {
		resetDays = 1
	}
	now := time.Now().In(creditLocation())
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return day.AddDate(0, 0, -(resetDays - 1))
}

func creditExpirePassed(raw string) bool {
	raw = strings.TrimSpace(raw)
	loc := creditLocation()
	var day time.Time
	if t, err := time.ParseInLocation("2006-01-02", raw, loc); err == nil {
		day = t
	} else if t, err := time.Parse(time.RFC3339, raw); err == nil {
		t = t.In(loc)
		day = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	} else {
		return false
	}
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	return today.After(day)
}

func envatoSessionUsername(r *http.Request) string {
	cookie, err := readSessionCookie(r)
	if err != nil || cookie.Value == "" {
		return ""
	}
	raw, ok := envatoSessions.Load(cookie.Value)
	if !ok {
		if !restoreEnvatoSession(cookie.Value) {
			return ""
		}
		raw, ok = envatoSessions.Load(cookie.Value)
		if !ok {
			return ""
		}
	}
	sess := raw.(*envatoLocalSession)
	return sess.username
}

func envatoPanelLimitView(username string) envatoLimitView {
	view := envatoLimitView{show: true, limit: 10, label: "Downloads"}
	cfg := loadConfig()
	db, err := openEnvatoPanel()
	if err != nil {
		return view
	}
	var websiteID, showLimit, accountID int
	var rawLimits string
	err = db.QueryRow(`SELECT id, COALESCE(default_limits_json, '{}') FROM websites WHERE domain IN ('127.0.0.1:5261', ?) LIMIT 1`, cfg.PublicHost).Scan(&websiteID, &rawLimits)
	if err != nil {
		return view
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_ = db.QueryRow(`SELECT assigned_account_id FROM live_sessions WHERE website_id=? AND username=? AND expires_at>? ORDER BY id DESC LIMIT 1`, websiteID, username, now).Scan(&accountID)
	if accountID > 0 {
		_ = db.QueryRow(`SELECT COALESCE(show_limit, 0) FROM accounts WHERE id=?`, accountID).Scan(&showLimit)
	}
	view.show = accountID > 0 && showLimit != 0
	var mode string
	_ = db.QueryRow(`SELECT COALESCE(limit_visibility, '') FROM panel_users WHERE website_id=? AND username=?`, websiteID, username).Scan(&mode)
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "show":
		view.show = true
	case "hide":
		view.show = false
	}
	limits := map[string]int{}
	_ = json.Unmarshal([]byte(rawLimits), &limits)
	key := "download"
	if n, ok := limits["download"]; ok {
		view.limit = n
	} else if n, ok := limits["credits"]; ok {
		view.limit = n
		key = "credits"
		view.label = "Credits"
	}
	resetDays := 1
	var userID, meterID, meterLimit, meterUsed, days, custom int
	var expire string
	err = db.QueryRow(`SELECT u.id, COALESCE(u.custom_limit_expire_at, ''),
		COALESCE(m.id, 0), COALESCE(m."limit", 0), COALESCE(m.used, 0), COALESCE(m.reset_days, 1), COALESCE(m.is_custom, 0)
		FROM panel_users u
		LEFT JOIN user_meters m ON m.user_id = u.id AND m.key = ?
		WHERE u.website_id = ? AND u.username = ?`, key, websiteID, username).Scan(&userID, &expire, &meterID, &meterLimit, &meterUsed, &days, &custom)
	if err == nil {
		view.used = meterUsed
		if days > 0 {
			resetDays = days
		}
		if meterID > 0 {
			view.limit = meterLimit
		}
	}
	_ = resetDays
	return view
}

var (
	envatoDownloadMu  sync.Mutex
	envatoRecentFiles = map[string]time.Time{}
	envatoRecentItems = map[string]time.Time{}
)

func isEnvatoLicensePath(path string) bool {
	lower := strings.ToLower(strings.Split(path, "?")[0])
	return lower == "/download.data" || strings.HasSuffix(lower, "/download.data")
}

func envatoItemLabel(r *http.Request) string {
	if r == nil {
		return ""
	}
	if id := downloadIdentityKey(r); strings.HasPrefix(id, "item:") {
		return id
	}
	ref := r.Referer()
	parsed, err := url.Parse(ref)
	if err != nil {
		return "item:download"
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) >= 2 {
		kind := parts[len(parts)-2]
		id := strings.TrimSuffix(parts[len(parts)-1], ".data")
		if len(id) >= 8 && kind != "" && kind != "app.envato.com" {
			return "item:" + kind + "/" + strings.ToLower(id)
		}
	}
	return "item:download"
}

func envatoPreviewPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.Contains(lower, "motion_shortened_preview") ||
		strings.Contains(lower, "preview_original") ||
		strings.Contains(lower, "first_frame") ||
		strings.Contains(lower, "cover-image") ||
		strings.Contains(lower, "elements-cover") ||
		strings.Contains(lower, "lut-reference") ||
		strings.HasSuffix(lower, ".m4a") ||
		strings.HasSuffix(lower, ".json")
}

func envatoPreviewHost(host string) bool {
	h := strings.ToLower(host)
	return strings.Contains(h, "elements-resized") ||
		strings.Contains(h, "elements-cover") ||
		strings.Contains(h, "cover-images") ||
		strings.Contains(h, "resized") ||
		(strings.Contains(h, "preview") && !strings.Contains(h, "download"))
}

// envatoIncomingDownload detects a real Elements file CDN hit.
// Thumbnails on elements-resized / cover hosts must never count as downloads.
func envatoIncomingDownload(path string) (host, name string, ok bool) {
	raw := path
	if i := strings.Index(raw, "?"); i >= 0 {
		raw = raw[:i]
	}
	if envatoPreviewPath(raw) {
		return "", "", false
	}
	host = "app.envato.com"
	filePath := raw
	const prefix = "/__domain__/"
	if strings.HasPrefix(raw, prefix) {
		rest := strings.TrimPrefix(raw, prefix)
		slash := strings.Index(rest, "/")
		if slash <= 0 {
			return "", "", false
		}
		host = rest[:slash]
		filePath = rest[slash:]
	}
	lowerHost := strings.ToLower(host)
	lowerPath := strings.ToLower(filePath)
	if envatoPreviewHost(lowerHost) || envatoPreviewPath(lowerPath) {
		return "", "", false
	}
	// Only real download CDNs / license file routes — not gallery thumbnails.
	tracked := strings.Contains(lowerHost, "dam-assets") ||
		(strings.Contains(lowerHost, "download") && !strings.Contains(lowerHost, "resized")) ||
		strings.Contains(lowerPath, "downloaded-file") ||
		strings.Contains(lowerPath, "/license")
	if !tracked {
		return "", "", false
	}
	name = filePath
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if decoded, err := url.PathUnescape(name); err == nil {
		name = decoded
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", false
	}
	return host, name, true
}

func downloadFileName(path, disposition string) string {
	if name := filenameFromDisposition(disposition); name != "" {
		return name
	}
	name := path
	if i := strings.Index(name, "?"); i >= 0 {
		name = name[:i]
	}
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if decoded, err := url.PathUnescape(name); err == nil {
		name = decoded
	}
	name = strings.TrimSpace(name)
	if strings.EqualFold(name, "download.data") || strings.EqualFold(name, "download") {
		return ""
	}
	return name
}

func filenameFromDisposition(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	if i := strings.Index(lower, "filename*="); i >= 0 {
		part := value[i+len("filename*="):]
		if j := strings.Index(part, ";"); j >= 0 {
			part = part[:j]
		}
		part = strings.Trim(part, `"`)
		if k := strings.Index(part, "''"); k >= 0 {
			part = part[k+2:]
		}
		if decoded, err := url.PathUnescape(strings.TrimSpace(part)); err == nil && decoded != "" {
			return decoded
		}
	}
	if i := strings.Index(lower, "filename="); i >= 0 {
		part := value[i+len("filename="):]
		if j := strings.Index(part, ";"); j >= 0 {
			part = part[:j]
		}
		part = strings.Trim(part, ` "`)
		if part != "" {
			return part
		}
	}
	return ""
}

func isEnvatoItemIDLabel(name string) bool {
	return !isChargeableEnvatoFileName(name)
}

// isChargeableEnvatoFileName is true only for a real downloaded asset title
// (must include a media/file extension). Category slugs like "stock-video" and
// endpoints like download.data / item:<uuid> must never consume a credit.
func isChargeableEnvatoFileName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "item:") || lower == "download.data" || lower == "download" || lower == "license" {
		return false
	}
	base := lower
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	dot := strings.LastIndex(base, ".")
	if dot <= 0 || dot == len(base)-1 {
		return false
	}
	ext := base[dot+1:]
	switch ext {
	case "data", "html", "htm", "php", "json", "js", "css", "map":
		return false
	}
	switch ext {
	case "mov", "mp4", "webm", "mkv", "avi", "m4v",
		"jpg", "jpeg", "png", "gif", "webp", "svg", "tif", "tiff", "bmp",
		"zip", "rar", "7z", "tar", "gz",
		"wav", "mp3", "aac", "flac", "ogg", "m4a",
		"pdf", "psd", "ai", "eps", "svgz",
		"cube", "3dl", "lut",
		"obj", "fbx", "blend", "gltf", "glb",
		"fig", "sketch", "xd",
		"doc", "docx", "ppt", "pptx", "xls", "xlsx":
		return true
	}
	// Unknown but plausible short file extension (e.g. .aep, .prproj)
	if len(ext) >= 2 && len(ext) <= 8 {
		for _, r := range ext {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
				return false
			}
		}
		return true
	}
	return false
}

// bestEnvatoFileName picks a real download filename only (with extension).
// Never returns page slugs like stock-video or download.data.
func bestEnvatoFileName(reqPath string, u *url.URL, referer, disposition, chargeName string) string {
	_ = referer // category/referer slugs are not file names — do not use them for billing
	if fn := filenameFromDisposition(disposition); isChargeableEnvatoFileName(fn) {
		return fn
	}
	if u != nil {
		if fn := extractFilenameFromQuery(u); isChargeableEnvatoFileName(fn) {
			return fn
		}
	}
	if base := downloadFileName(reqPath, ""); isChargeableEnvatoFileName(base) {
		return base
	}
	if isChargeableEnvatoFileName(chargeName) {
		return chargeName
	}
	return ""
}

func sanitizeContentDispositionFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, `"`, "")
	name = strings.ReplaceAll(name, "\n", "")
	name = strings.ReplaceAll(name, "\r", "")
	if name == "" {
		return "download"
	}
	return name
}

// peekEnvatoDownloadJSONFilename reads a small JSON download.data body for a file title
// and restores resp.Body so the client still gets the payload.
func peekEnvatoDownloadJSONFilename(resp *http.Response) string {
	if resp == nil || resp.Body == nil || resp.Request == nil {
		return ""
	}
	path := strings.ToLower(resp.Request.URL.Path)
	if !strings.Contains(path, "download.data") && !strings.Contains(path, "/download") {
		return ""
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if !strings.Contains(ct, "json") && !strings.Contains(ct, "text/plain") && !strings.Contains(ct, "javascript") {
		return ""
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	if err != nil || len(raw) == 0 {
		return ""
	}
	var payload interface{}
	if json.Unmarshal(raw, &payload) != nil {
		return ""
	}
	keys := []string{
		"filename", "file_name", "fileName", "original_filename", "originalFilename",
		"download_filename", "downloadFilename", "name", "title",
	}
	if name := findJSONStringByKeys(payload, keys); name != "" && !isEnvatoItemIDLabel(name) {
		return name
	}
	// Sometimes only a CDN URL is present — take basename if it looks like a file.
	urlKeys := []string{"downloadUrl", "download_url", "url", "href", "signedUrl", "signed_url"}
	if u := findJSONStringByKeys(payload, urlKeys); u != "" {
		if parsed, err := url.Parse(u); err == nil {
			base := downloadFileName(parsed.Path, "")
			if q := extractFilenameFromQuery(parsed); q != "" {
				base = q
			}
			if base != "" && !isEnvatoItemIDLabel(base) {
				return base
			}
		}
	}
	return ""
}

func findJSONStringByKeys(v interface{}, keys []string) string {
	want := map[string]bool{}
	for _, k := range keys {
		want[strings.ToLower(k)] = true
	}
	var walk func(interface{}) string
	walk = func(node interface{}) string {
		switch n := node.(type) {
		case map[string]interface{}:
			for k, child := range n {
				if want[strings.ToLower(k)] {
					if s, ok := child.(string); ok {
						s = strings.TrimSpace(s)
						if s != "" {
							return s
						}
					}
				}
			}
			for _, child := range n {
				if s := walk(child); s != "" {
					return s
				}
			}
		case []interface{}:
			for _, child := range n {
				if s := walk(child); s != "" {
					return s
				}
			}
		}
		return ""
	}
	return walk(v)
}

func isJunkDownloadLabel(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch lower {
	case "", "download.data", "download", "stock-video", "stock-photos", "photos", "videos",
		"music", "graphics", "fonts", "web", "item:download":
		return true
	}
	return false
}

// recordEnvatoDownload bills one successful Elements download.
// displayName is shown in the panel; dedupeKey (item:uuid or file name) prevents double-count
// between /download.data and the CDN file follow-up.
func recordEnvatoDownload(username, displayName, dedupeKey string) {
	username = strings.TrimSpace(username)
	displayName = strings.TrimSpace(displayName)
	dedupeKey = strings.TrimSpace(dedupeKey)
	if username == "" {
		return
	}
	if isJunkDownloadLabel(displayName) {
		displayName = ""
	}
	if dedupeKey == "" {
		dedupeKey = displayName
	}
	if isJunkDownloadLabel(dedupeKey) && !strings.HasPrefix(strings.ToLower(dedupeKey), "item:") {
		dedupeKey = ""
	}
	if displayName == "" {
		displayName = dedupeKey
	}
	if displayName == "" {
		log.Printf("[DOWNLOAD] skip empty label user=%s", username)
		return
	}
	// Still reject bare category slugs with no item:/extension.
	if isJunkDownloadLabel(displayName) {
		return
	}
	if !isChargeableEnvatoFileName(displayName) && !strings.HasPrefix(strings.ToLower(displayName), "item:") {
		log.Printf("[DOWNLOAD] skip non-file label user=%s name=%q", username, displayName)
		return
	}

	envatoDownloadMu.Lock()
	defer envatoDownloadMu.Unlock()

	db, err := openEnvatoPanel()
	if err != nil {
		return
	}
	var websiteID int
	_ = db.QueryRow(`SELECT id FROM websites WHERE domain IN ('127.0.0.1:5261', ?)`, loadConfig().PublicHost).Scan(&websiteID)
	if websiteID == 0 {
		return
	}
	now := time.Now().UTC()
	fileKey := username + "\n" + strings.ToLower(displayName)
	dedupeMem := username + "\n" + strings.ToLower(dedupeKey)

	// Same item/file within 60s → one credit (covers license JSON + CDN file).
	if last, ok := envatoRecentFiles[dedupeMem]; ok && time.Since(last) < 60*time.Second {
		// Upgrade placeholder item: row to a real filename when it arrives.
		if isChargeableEnvatoFileName(displayName) && strings.HasPrefix(strings.ToLower(dedupeKey), "item:") {
			var rowID int
			_ = db.QueryRow(`SELECT id FROM usage_events WHERE website_id=? AND username=? AND limit_key='download'
				AND target_path=? ORDER BY timestamp DESC LIMIT 1`, websiteID, username, dedupeKey).Scan(&rowID)
			if rowID > 0 {
				_, _ = db.Exec(`UPDATE usage_events SET target_path=? WHERE id=?`, displayName, rowID)
				log.Printf("[DOWNLOAD] renamed item→file user=%s file=%s", username, displayName)
			}
		}
		envatoRecentFiles[fileKey] = now
		return
	}
	if last, ok := envatoRecentFiles[fileKey]; ok && time.Since(last) < 60*time.Second {
		return
	}
	if last, ok := envatoRecentItems[username]; ok && time.Since(last) < 10*time.Second {
		var recentSame int
		_ = db.QueryRow(`SELECT COUNT(*) FROM usage_events WHERE website_id=? AND username=? AND limit_key='download' AND timestamp>=?`,
			websiteID, username, now.Add(-10*time.Second).Format(time.RFC3339)).Scan(&recentSame)
		if recentSame > 0 {
			envatoRecentFiles[fileKey] = now
			envatoRecentFiles[dedupeMem] = now
			return
		}
	}

	view := envatoPanelLimitView(username)
	if view.limit >= 0 && view.used >= view.limit {
		return
	}
	var recent int
	_ = db.QueryRow(`SELECT COUNT(*) FROM usage_events WHERE website_id=? AND username=? AND limit_key='download' AND target_path=? AND timestamp>=?`,
		websiteID, username, displayName, now.Add(-60*time.Second).Format(time.RFC3339)).Scan(&recent)
	if recent > 0 {
		envatoRecentFiles[fileKey] = now
		envatoRecentFiles[dedupeMem] = now
		return
	}
	_, err = db.Exec(`INSERT INTO usage_events (website_id, username, limit_key, limit_label, reset_days, action, target_path, amount, timestamp)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		websiteID, username, "download", "Downloads", 1, "Download", displayName, 1, now.Format(time.RFC3339))
	if err != nil {
		log.Printf("[DOWNLOAD] log failed user=%s file=%s: %v", username, displayName, err)
		return
	}
	envatoRecentFiles[fileKey] = now
	envatoRecentFiles[dedupeMem] = now
	envatoRecentItems[username] = now
	used := bumpEnvatoMeter(username, view)
	log.Printf("[DOWNLOAD] saved user=%s file=%s dedupe=%s used=%d limit=%d", username, displayName, dedupeKey, used, view.limit)
}

func bumpEnvatoMeter(username string, view envatoLimitView) int {
	db, err := openEnvatoPanel()
	if err != nil {
		return view.used + 1
	}
	var websiteID, userID int
	if err = db.QueryRow(`SELECT id FROM websites WHERE domain IN ('127.0.0.1:5261', ?) LIMIT 1`, loadConfig().PublicHost).Scan(&websiteID); err != nil || websiteID == 0 {
		return view.used + 1
	}
	err = db.QueryRow(`SELECT id FROM panel_users WHERE website_id=? AND username=?`, websiteID, username).Scan(&userID)
	if err != nil {
		res, insertErr := db.Exec(`INSERT INTO panel_users (website_id, username, status, custom_limit_expire_at) VALUES (?,?,?,NULL)`, websiteID, username, "active")
		if insertErr != nil {
			return view.used + 1
		}
		id, _ := res.LastInsertId()
		userID = int(id)
	}
	res, err := db.Exec(`UPDATE user_meters SET used = used + 1 WHERE user_id=? AND key='download'`, userID)
	changed := int64(0)
	if err == nil && res != nil {
		changed, _ = res.RowsAffected()
	}
	if err != nil || changed == 0 {
		_, _ = db.Exec(`INSERT INTO user_meters (user_id, key, label, reset_days, used, "limit", is_custom) VALUES (?,?,?,?,?,?,0)`,
			userID, "download", view.label, 1, view.used+1, view.limit)
		return view.used + 1
	}
	var used int
	_ = db.QueryRow(`SELECT used FROM user_meters WHERE user_id=? AND key='download'`, userID).Scan(&used)
	return used
}

func syncEnvatoPanelUser(username string, view envatoLimitView) {
	if username == "" {
		return
	}
	db, err := openEnvatoPanel()
	if err != nil {
		return
	}
	var websiteID int
	if err = db.QueryRow(`SELECT id FROM websites WHERE domain IN ('127.0.0.1:5261', ?) LIMIT 1`, loadConfig().PublicHost).Scan(&websiteID); err != nil || websiteID == 0 {
		return
	}
	var userID int
	err = db.QueryRow(`SELECT id FROM panel_users WHERE website_id=? AND username=?`, websiteID, username).Scan(&userID)
	if err != nil {
		res, insertErr := db.Exec(`INSERT INTO panel_users (website_id, username, status, custom_limit_expire_at) VALUES (?,?,?,NULL)`, websiteID, username, "active")
		if insertErr != nil {
			return
		}
		id, _ := res.LastInsertId()
		userID = int(id)
	}
	limit := view.limit
	if limit < 0 {
		limit = 10
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM user_meters WHERE user_id=? AND key='download'`, userID).Scan(&n)
	if n == 0 {
		_, _ = db.Exec(`INSERT INTO user_meters (user_id, key, label, reset_days, used, "limit", is_custom) VALUES (?,?,?,?,?,?,0)`,
			userID, "download", "Download", 1, view.used, limit)
		return
	}
	_, _ = db.Exec(`UPDATE user_meters SET used=?, "limit"=?, label='Download' WHERE user_id=? AND key='download' AND is_custom=0`, view.used, limit, userID)
}

func envatoLimitWidgetScript() string {
	return `<script data-tm-credits="1">
(function() {
    var CREDIT_LABEL = 'Downloads';
    var open = false;
    var latest = null;
    function removeDock(){
        var el = document.getElementById('tm-limit-dock');
        if (el) el.remove();
    }
    function host() {
        if (latest && !latest.show_limit) { removeDock(); return null; }
        if (window.__tmDenied) return null;
        var el = document.getElementById('tm-limit-dock');
        if (el) return el;
        el = document.createElement('div');
        el.id = 'tm-limit-dock';
        var root = el.attachShadow({ mode: 'open' });
        root.innerHTML = ''
            + '<style>'
            + ':host{all:initial}'
            + '.tab{position:fixed;left:16px;bottom:24px;z-index:2147483646;width:48px;height:48px;border:0;border-radius:999px;background:#22c55e;color:#fff;display:grid;place-items:center;box-shadow:0 10px 24px rgba(22,163,74,.35);cursor:pointer}'
            + '.tab svg{width:22px;height:22px;display:block}'
            + '.tab.warn{background:#f59e0b}.tab.low{background:#ef4444}.tab.hide{opacity:0;pointer-events:none}'
            + '.back{position:fixed;inset:0;z-index:2147483646;background:rgba(15,23,42,.18);opacity:0;pointer-events:none;transition:opacity .2s}'
            + '.back.show{opacity:1;pointer-events:auto}'
            + '.card{position:fixed;left:16px;bottom:24px;z-index:2147483647;width:232px;background:#f3fbf6;color:#14532d;border-radius:28px;box-shadow:0 22px 50px rgba(15,23,42,.18);padding:16px 16px 14px;box-sizing:border-box;font:500 14px/1.3 system-ui,sans-serif;transform:translateY(10px) scale(.96);opacity:0;pointer-events:none;transition:transform .22s ease,opacity .22s ease}'
            + '.card.show{transform:none;opacity:1;pointer-events:auto}'
            + '.head{display:flex;align-items:center;justify-content:space-between;margin-bottom:6px}'
            + '.title{font-weight:750;font-size:15px}'
            + '.x{border:0;background:#e7f6ec;color:#166534;width:28px;height:28px;border-radius:999px;cursor:pointer;font:700 16px/1 system-ui,sans-serif}'
            + '.ringwrap{position:relative;width:168px;height:168px;margin:4px auto 8px}'
            + '.ring{width:168px;height:168px;transform:rotate(-90deg)}'
            + '.track{fill:none;stroke:#d9f3e3;stroke-width:10}'
            + '.fill{fill:none;stroke:#22c55e;stroke-width:10;stroke-linecap:round;stroke-dasharray:289;stroke-dashoffset:0;transition:stroke-dashoffset .35s ease,stroke .2s}'
            + '.fill.warn{stroke:#f59e0b}.fill.low{stroke:#ef4444}'
            + '.center{position:absolute;inset:0;display:flex;flex-direction:column;align-items:center;justify-content:center}'
            + '.num{font-size:40px;font-weight:800;letter-spacing:-.04em;color:#14532d}'
            + '.sub{margin-top:2px;color:#4d7c5e;font-size:13px}'
            + '.meter{background:#fff;border-radius:16px;padding:10px 12px}'
            + '.meter-top{display:flex;justify-content:space-between;align-items:center;margin-bottom:8px;color:#166534;font-size:13px}'
            + '.count{font-weight:750}'
            + '.bar{height:6px;border-radius:999px;background:#e7f6ec;overflow:hidden}'
            + '.barfill{height:100%;width:0;border-radius:999px;background:#22c55e;transition:width .35s ease}'
            + '.barfill.warn{background:#f59e0b}.barfill.low{background:#ef4444}'
            + '<' + '/style>'
            + '<button class="tab" id="tm-tab" type="button" aria-label="Downloads"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="8" fill="none" stroke="currentColor" stroke-width="2" opacity=".35"><' + '/circle><path d="M12 4a8 8 0 0 1 8 8" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round"><' + '/path><' + '/svg><' + '/button>'
            + '<div class="back" id="tm-back"><' + '/div>'
            + '<aside class="card" id="tm-drawer">'
            + '<div class="head"><div class="title" id="tm-title">Downloads<' + '/div><button class="x" id="tm-close" type="button" aria-label="Close">×<' + '/button><' + '/div>'
            + '<div class="ringwrap"><svg class="ring" viewBox="0 0 120 120"><circle class="track" cx="60" cy="60" r="46"><' + '/circle><circle class="fill" id="tm-ring" cx="60" cy="60" r="46"><' + '/circle><' + '/svg>'
            + '<div class="center"><div class="num" id="tm-left">0<' + '/div><div class="sub" id="tm-sub">left<' + '/div><' + '/div><' + '/div>'
            + '<div class="meter"><div class="meter-top"><span id="tm-meter-label">Downloads<' + '/span><span class="count" id="tm-count">0 / 0<' + '/span><' + '/div><div class="bar"><div class="barfill" id="tm-bar"><' + '/div><' + '/div><' + '/div>'
            + '<' + '/aside>';
        (document.body || document.documentElement).appendChild(el);
        root.getElementById('tm-tab').addEventListener('click', function(ev){ ev.stopPropagation(); setOpen(true); });
        root.getElementById('tm-close').addEventListener('click', function(ev){ ev.stopPropagation(); setOpen(false); });
        root.getElementById('tm-back').addEventListener('click', function(){ setOpen(false); });
        root.getElementById('tm-drawer').addEventListener('click', function(ev){ ev.stopPropagation(); });
        return el;
    }
    function setOpen(next) {
        open = next;
        var el = host();
        if (!el) return;
        var shadow = el.shadowRoot;
        shadow.getElementById('tm-drawer').classList.toggle('show', open);
        shadow.getElementById('tm-back').classList.toggle('show', open);
        shadow.getElementById('tm-tab').classList.toggle('hide', open);
    }
    function paint(d) {
        if (!d || !d.show_limit) { removeDock(); return; }
        var el = host();
        if (!el) return;
        var shadow = el.shadowRoot;
        var title = (d && d.credit_label) || CREDIT_LABEL;
        shadow.getElementById('tm-title').textContent = title;
        shadow.getElementById('tm-meter-label').textContent = title;
        var tab = shadow.getElementById('tm-tab');
        var ring = shadow.getElementById('tm-ring');
        var bar = shadow.getElementById('tm-bar');
        var used = Number(d.credit_used) || 0;
        var limit = Number(d.credit_limit);
        var unlimited = !(limit >= 0);
        var left = unlimited ? used : Math.max(0, limit - used);
        var ratio = unlimited ? 1 : (limit === 0 ? 0 : left / limit);
        shadow.getElementById('tm-left').textContent = unlimited ? '∞' : String(left);
        shadow.getElementById('tm-sub').textContent = unlimited ? 'unlimited' : 'left';
        shadow.getElementById('tm-count').textContent = unlimited ? (String(used) + ' used') : (String(used) + ' / ' + String(limit));
        ring.style.strokeDasharray = '289';
        ring.style.strokeDashoffset = String(289 * (1 - ratio));
        bar.style.width = Math.round(ratio * 100) + '%';
        var low = !unlimited && left < 3;
        var warn = !unlimited && left >= 3 && left < 10;
        tab.classList.toggle('low', low);
        tab.classList.toggle('warn', warn);
        ring.classList.toggle('low', low);
        ring.classList.toggle('warn', warn);
        bar.classList.toggle('low', low);
        bar.classList.toggle('warn', warn);
    }
    function noDownloadsLeft() {
        if (!latest || !latest.show_limit) return false;
        var used = Number(latest.credit_used) || 0;
        var limit = Number(latest.credit_limit) || 0;
        return limit >= 0 && used >= limit;
    }
    function isCountedDownload(url) {
        var lower = String(url || "").toLowerCase();
        if (lower.indexOf("preview") !== -1 || lower.indexOf("first_frame") !== -1 || lower.indexOf("motion_shortened") !== -1) return false;
        return lower.indexOf("/download.data") !== -1 || lower.indexOf("downloaded-file") !== -1 || lower.indexOf("downloads.") !== -1 || lower.indexOf(".cube") !== -1 || lower.indexOf(".3dl") !== -1;
    }
    function showLimitCard() {
        if (document.getElementById("tm-limit-screen")) return;
        var el = document.createElement("div");
        el.id = "tm-limit-screen";
        el.style.cssText = "position:fixed;inset:0;z-index:2147483647;background:#eef3f8;color:#0f172a;display:flex;align-items:center;justify-content:center;padding:24px;font-family:system-ui,-apple-system,Segoe UI,sans-serif;";
        el.innerHTML = '<div style="width:min(440px,92vw);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px 36px;text-align:center;">'
            + '<div style="width:78px;height:78px;margin:0 auto 22px;border-radius:50%;background:#fff;display:grid;place-items:center;font-size:28px;border:1px solid #e6ebf2;">&#128274;<' + '/div>'
            + '<h1 style="font-size:28px;line-height:1.2;font-weight:800;letter-spacing:-.03em;margin:0 0 12px;">Daily Limit Reached<' + '/h1>'
            + '<p style="color:#64748b;font-size:15px;line-height:1.55;margin:0;">You have used your download limit for today. The limit resets at midnight (12:00 AM IST).<' + '/p>'
            + '<p style="margin-top:18px;color:#94a3b8;font-size:13px;">This limit applies to the current access<' + '/p><' + '/div>';
        (document.body || document.documentElement).appendChild(el);
    }
    function updateBadge() {
        window.fetch('/api/user-limits', { credentials: 'same-origin', cache: 'no-store' })
            .then(function(r){ return r.json(); })
            .then(function(d){ latest = d; paint(d); })
            .catch(function(){});
    }
    function boot() { if (window.__tmDenied) return; updateBadge(); }
    var trackedFetch = window.fetch;
    window.fetch = function(input, init) {
        var url = typeof input === "string" ? input : (input && input.url) || "";
        if (isCountedDownload(url) && noDownloadsLeft()) {
            showLimitCard();
            return Promise.resolve(new Response('{"error":"daily_download_limit_reached"}', { status: 403, headers: { "Content-Type": "application/json" } }));
        }
        return trackedFetch.apply(this, arguments).then(function(res) {
            if (res && res.status === 403 && isCountedDownload(url)) showLimitCard();
            return res;
        });
    };
    if (document.body) boot();
    else document.addEventListener('DOMContentLoaded', boot);
    setInterval(function(){ if (window.__tmDenied) return; updateBadge(); }, 4000);
})();
</script>`
}
