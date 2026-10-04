package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type semrushGateSession struct {
	mu          sync.Mutex
	username    string
	expires     time.Time
	fp          string
	proof       string
	tracked     bool
	liveOK      bool
	liveChecked time.Time
}

type semrushAccount struct {
	ID        int
	Name      string
	Cookie    string
	UserAgent string
	Proxy     string
}

var semrushSessions sync.Map

func semrushOpenPath(path string) bool {
	switch path {
	case "/access", "/api/device-bind", "/tm-device-sw.js", "/__healthz", "/favicon.ico":
		return true
	}
	return false
}

func semrushSessionToken(r *http.Request) string {
	if r == nil {
		return ""
	}
	if v, ok := r.Context().Value(semrushSessionCtxKey{}).(string); ok && v != "" {
		return v
	}
	cookie, err := r.Cookie("sem_session")
	if err != nil || cookie.Value == "" {
		// Boot script fallback when browser drops Set-Cookie on plain HTTP.
		if h := strings.TrimSpace(r.Header.Get("X-Sem-Session")); h != "" {
			return h
		}
		return ""
	}
	return cookie.Value
}

func semrushSessionOK(r *http.Request) bool {
	token := semrushSessionToken(r)
	if token == "" {
		return false
	}
	raw, ok := semrushSessions.Load(token)
	if !ok {
		if !restoreSemrushSession(token) {
			return false
		}
		raw, ok = semrushSessions.Load(token)
		if !ok {
			return false
		}
	}
	sess := raw.(*semrushGateSession)
	sess.mu.Lock()
	if time.Now().After(sess.expires) {
		semrushSessions.Delete(token)
		sess.mu.Unlock()
		return false
	}
	tracked := sess.tracked
	needLive := tracked && time.Since(sess.liveChecked) > 3*time.Second
	liveOK := sess.liveOK
	sess.mu.Unlock()
	if needLive {
		liveOK = semrushLiveExists(token)
		sess.mu.Lock()
		sess.liveChecked = time.Now()
		sess.liveOK = liveOK
		sess.mu.Unlock()
	}
	if tracked && !liveOK {
		semrushSessions.Delete(token)
		return false
	}
	return true
}

func restoreSemrushSession(token string) bool {
	db, err := openSemrushPanel()
	if err != nil {
		return false
	}
	var username, expRaw, fp string
	err = db.QueryRow(`SELECT username, expires_at, COALESCE(fingerprint, '') FROM live_sessions WHERE session_token=?`, token).Scan(&username, &expRaw, &fp)
	if err != nil {
		return false
	}
	expires, err := time.Parse(time.RFC3339, expRaw)
	if err != nil {
		expires, err = time.Parse(time.RFC3339Nano, expRaw)
	}
	if err != nil || time.Now().After(expires) {
		return false
	}
	semrushSessions.Store(token, &semrushGateSession{username: username, expires: expires, fp: fp, tracked: true, liveOK: true})
	return true
}

func semrushLiveExists(token string) bool {
	db, err := openSemrushPanel()
	if err != nil {
		return true
	}
	var id int
	return db.QueryRow(`SELECT id FROM live_sessions WHERE session_token=?`, token).Scan(&id) == nil
}

// EventSource cannot set headers. The page appends the device proof as query
// params; copy them onto the headers the device check already understands,
// then drop them so they never reach Semrush or the request log.
func adoptSemrushDeviceQuery(r *http.Request) {
	q := r.URL.Query()
	fp := strings.TrimSpace(q.Get("tm_dfp"))
	proof := strings.TrimSpace(q.Get("tm_dproof"))
	if fp == "" && proof == "" {
		return
	}
	if fp != "" && strings.TrimSpace(r.Header.Get("X-Device-Fp")) == "" {
		r.Header.Set("X-Device-Fp", fp)
	}
	if proof != "" && strings.TrimSpace(r.Header.Get("X-Device-Proof")) == "" {
		r.Header.Set("X-Device-Proof", proof)
	}
	q.Del("tm_dfp")
	q.Del("tm_dproof")
	r.URL.RawQuery = q.Encode()
}

func semrushRequireSession(w http.ResponseWriter, r *http.Request) bool {
	if semrushOpenPath(r.URL.Path) || semrushStaticGet(r) || semrushSessionOK(r) {
		return semrushRejectDevice(w, r)
	}
	if semrushDocument(r) || strings.Contains(r.Header.Get("Accept"), "text/html") {
		renderSemrushDenied(w)
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	fmt.Fprint(w, `{"error":"access_denied","message":"Open this tool from your access link."}`)
	return true
}

func semrushPanelAccount(publicHost, sessionToken string) (semrushAccount, bool) {
	db, err := openSemrushPanel()
	if err != nil {
		return semrushAccount{}, false
	}
	now := time.Now().UTC().Format(time.RFC3339)
	assigned := 0
	if sessionToken != "" {
		_ = db.QueryRow(`SELECT COALESCE(assigned_account_id, 0) FROM live_sessions WHERE session_token=? AND expires_at>?`, sessionToken, now).Scan(&assigned)
	}
	load := func(where string, args ...any) (semrushAccount, error) {
		var acc semrushAccount
		err := db.QueryRow(`SELECT a.id, a.name, a.cookie,
			CASE WHEN TRIM(COALESCE(a.user_agent,'')) != '' THEN a.user_agent ELSE COALESCE(ua.user_agent, '') END,
			CASE WHEN TRIM(COALESCE(p.endpoint,'')) != '' THEN p.endpoint ELSE COALESCE(a.proxy,'') END
			FROM accounts a
			JOIN websites w ON w.id = a.website_id
			LEFT JOIN user_agents ua ON ua.id = a.user_agent_id
			LEFT JOIN proxies p ON p.id = a.proxy_id
			WHERE `+where, args...).Scan(&acc.ID, &acc.Name, &acc.Cookie, &acc.UserAgent, &acc.Proxy)
		if err != nil || strings.TrimSpace(acc.Cookie) == "" {
			return semrushAccount{}, fmt.Errorf("account")
		}
		acc.Cookie = cookieHeaderFromStored(acc.Cookie)
		return acc, nil
	}
	if assigned > 0 {
		if acc, err := load(`a.id=? AND w.domain IN ('127.0.0.1:5141', ?) AND a.status='active' AND TRIM(a.cookie)!=''`, assigned, publicHost); err == nil {
			return acc, true
		}
	}
	acc, err := load(`w.domain IN ('127.0.0.1:5141', ?) AND a.status='active' AND TRIM(a.cookie)!=''
		ORDER BY CASE WHEN TRIM(COALESCE(a.last_used_at,'')) = '' THEN 0 ELSE 1 END, a.last_used_at ASC, a.id ASC LIMIT 1`, publicHost)
	if err != nil {
		return semrushAccount{}, false
	}
	if sessionToken != "" {
		_, _ = db.Exec(`UPDATE live_sessions SET assigned_account_id=? WHERE session_token=?`, acc.ID, sessionToken)
		_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, acc.ID)
	}
	return acc, true
}

var (
	semrushTriedMu sync.Mutex
	semrushTried   = map[string]map[int]bool{}
)

type semrushSessionCtxKey struct{}

func rememberSemrushSession(req *http.Request) {
	if req == nil {
		return
	}
	if c, err := req.Cookie("sem_session"); err == nil && c.Value != "" {
		*req = *req.WithContext(context.WithValue(req.Context(), semrushSessionCtxKey{}, c.Value))
	}
}

func semrushMarkTried(token string, id int) {
	if token == "" || id <= 0 {
		return
	}
	semrushTriedMu.Lock()
	defer semrushTriedMu.Unlock()
	if semrushTried[token] == nil {
		semrushTried[token] = map[int]bool{}
	}
	semrushTried[token][id] = true
}

func semrushAccountDeadTarget(raw string) bool {
	low := strings.ToLower(raw)
	return strings.Contains(low, "disable_hard") || strings.Contains(low, "/sso/logout") || strings.Contains(low, "/login/disable_hard")
}

func semrushSafeReturn(r *http.Request) string {
	if r == nil {
		return "/"
	}
	if path := semrushReturnPath(r.Referer(), r.Host); path != "" {
		return path
	}
	if !semrushSkipReturn(r.URL.Path, r.URL.RawQuery) {
		path := r.URL.Path
		if path == "" {
			path = "/"
		}
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}
		return path
	}
	return "/"
}

func semrushReturnPath(raw, host string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Host != "" && u.Host != host) {
		return ""
	}
	if semrushSkipReturn(u.Path, u.RawQuery) {
		return ""
	}
	path := u.Path
	if path == "" {
		path = "/"
	}
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	return path
}

func semrushSkipReturn(path, rawQuery string) bool {
	low := strings.ToLower(path)
	if low == "" || low == "/" || low == "/access" || strings.HasPrefix(low, "/access/") {
		return true
	}
	return semrushAccountDeadTarget(path) || semrushAccountDeadTarget(rawQuery)
}

// semrushRotatePanelAccount skips the account Semrush just logged out and assigns
// the next active one. It does not change accounts.status.
func semrushRotatePanelAccount(publicHost, sessionToken string) (semrushAccount, bool) {
	db, err := openSemrushPanel()
	if err != nil || sessionToken == "" {
		log.Printf("[SWAP] cannot rotate session=%t db=%v", sessionToken != "", err)
		return semrushAccount{}, false
	}
	current := 0
	_ = db.QueryRow(`SELECT COALESCE(assigned_account_id, 0) FROM live_sessions WHERE session_token=?`, sessionToken).Scan(&current)
	if current > 0 {
		semrushMarkTried(sessionToken, current)
		_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339), current)
	}
	semrushTriedMu.Lock()
	tried := semrushTried[sessionToken]
	semrushTriedMu.Unlock()
	rows, err := db.Query(`SELECT a.id, a.name FROM accounts a
		JOIN websites w ON w.id = a.website_id
		WHERE w.domain IN ('127.0.0.1:5141', ?) AND a.status='active' AND TRIM(a.cookie) != ''
		ORDER BY CASE WHEN TRIM(COALESCE(a.last_used_at,'')) = '' THEN 0 ELSE 1 END, a.last_used_at ASC, a.id ASC`, publicHost)
	if err != nil {
		return semrushAccount{}, false
	}
	defer rows.Close()
	nextID := 0
	nextName := ""
	for rows.Next() {
		var id int
		var name string
		if rows.Scan(&id, &name) != nil {
			continue
		}
		if tried[id] {
			continue
		}
		nextID = id
		nextName = name
		break
	}
	rows.Close()
	if nextID == 0 && current > 0 {
		again, err := db.Query(`SELECT a.id, a.name FROM accounts a
			JOIN websites w ON w.id = a.website_id
			WHERE w.domain IN ('127.0.0.1:5141', ?) AND a.status='active' AND TRIM(a.cookie) != '' AND a.id != ?
			ORDER BY a.id ASC LIMIT 1`, publicHost, current)
		if err == nil {
			if again.Next() {
				_ = again.Scan(&nextID, &nextName)
			}
			again.Close()
		}
	}
	if nextID == 0 {
		log.Printf("[SWAP] no further active account after id=%d", current)
		return semrushAccount{}, false
	}
	_, _ = db.Exec(`UPDATE live_sessions SET assigned_account_id=? WHERE session_token=?`, nextID, sessionToken)
	invalidateSessionAccountCache(sessionToken)
	log.Printf("[SWAP] account id=%d logged out, next id=%d name=%s", current, nextID, nextName)
	return semrushPanelAccount(publicHost, sessionToken)
}

func semrushAccountSwapPage(r *http.Request) (string, bool) {
	cfg := loadConfig()
	token := semrushSessionToken(r)
	if acc, ok := semrushRotatePanelAccount(cfg.PublicHost, token); ok {
		dest := semrushSafeReturn(r)
		name := strings.TrimSpace(acc.Name)
		message := "Account logged out. Switching to another account..."
		if name != "" {
			message = "Account logged out. Switching to " + html.EscapeString(name) + "..."
		}
		script := fmt.Sprintf(`setTimeout(function(){location.replace(%q);},2500);`, dest)
		return semrushLightPage("Switching account", "Switching account",
			message,
			`<div class="pill"><span class="dot"></span>Trying one account at a time</div><p class="foot">This keeps trying until an account is available</p>`,
			true, script), true
	}
	return semrushLightPage("Account logged out", "Account logged out",
		"Account logged out. Contact the admin.",
		"", false, ""), false
}

func serveSemrushAccountSwap(w http.ResponseWriter, r *http.Request) {
	page, _ := semrushAccountSwapPage(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func serveSemrushAccess(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.URL.Query().Get("user"))
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if username == "" || token == "" {
		renderSemrushDenied(w)
		return
	}
	db, err := openSemrushPanel()
	if err != nil {
		renderSemrushDenied(w)
		return
	}
	cfg := loadConfig()
	var dbUser, productID, clientIP, expiresRaw string
	var websiteID, minutes int
	err = db.QueryRow(`DELETE FROM access_tokens WHERE token=? AND expires_at>? RETURNING username, product_id, COALESCE(client_ip, ''), expires_at, website_id`,
		token, time.Now().UTC().Format(time.RFC3339)).Scan(&dbUser, &productID, &clientIP, &expiresRaw, &websiteID)
	if err != nil || dbUser != username {
		log.Printf("[PANEL] token rejected user=%s err=%v", username, err)
		renderSemrushDenied(w)
		return
	}
	var domain string
	err = db.QueryRow(`SELECT domain, COALESCE(session_duration, 30) FROM websites WHERE id=?`, websiteID).Scan(&domain, &minutes)
	if err != nil || (domain != cfg.PublicHost && domain != "127.0.0.1:5141") {
		renderSemrushDenied(w)
		return
	}
	if minutes < 1 {
		minutes = 30
	}
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	sessionToken := hex.EncodeToString(buf)
	expires := time.Now().Add(time.Duration(minutes) * time.Minute)
	semrushSessions.Store(sessionToken, &semrushGateSession{username: username, expires: expires, tracked: true, liveOK: true})
	seen := realClientIP(r)
	if clientIP != "" && (seen == "" || seen == "127.0.0.1" || seen == "::1") {
		seen = clientIP
	}
	acc, ok := semrushPanelAccount(cfg.PublicHost, "")
	accountID := 0
	if ok {
		accountID = acc.ID
	}
	recordSemrushLogin(db, websiteID, accountID, username, sessionToken, seen, r.UserAgent(), expires)
	if accountID > 0 {
		_, _ = db.Exec(`UPDATE live_sessions SET assigned_account_id=? WHERE session_token=?`, accountID, sessionToken)
	}
	maxAge := int(time.Until(expires).Seconds())
	if maxAge < 60 {
		maxAge = 60
	}
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") ||
		strings.EqualFold(cfg.PublicScheme, "https")
	http.SetCookie(w, &http.Cookie{
		Name: "sem_session", Value: sessionToken, Path: "/",
		Expires: expires, MaxAge: maxAge,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
	log.Printf("[PANEL] access granted user=%s ip=%s", username, seen)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	// Embed token so device-bind works even if browser drops Set-Cookie on HTTP.
	boot := strings.Replace(semrushBootScript(), "__SEM_SESSION__", sessionToken, 1)
	page := semrushLightPage("Authenticating", "Authenticating...",
		`You are using <span class="brand">Semrush</span>. Please wait a moment while we verify your secure access request.`,
		`<div class="pill"><span class="dot"></span>Verifying your request...</div><p class="foot">Secure session initialization in progress</p>`,
		true, boot)
	_, _ = w.Write([]byte(page))
}

func recordSemrushLogin(db *sql.DB, websiteID, accountID int, username, sessionToken, clientIP, userAgent string, expires time.Time) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = db.Exec(`DELETE FROM live_sessions WHERE username=? AND website_id=?`, username, websiteID)
	_, err := db.Exec(`INSERT INTO live_sessions (website_id, session_token, username, client_ip, fingerprint, expires_at, created_at, assigned_account_id)
		VALUES (?,?,?,?,?,?,?,?)`, websiteID, sessionToken, username, clientIP, "", expires.UTC().Format(time.RFC3339), now, accountID)
	if err != nil {
		log.Printf("[PANEL] live session insert failed: %v", err)
		return
	}
	_, _ = db.Exec(`INSERT INTO login_events (website_id, username, client_ip, user_agent, logged_in_at) VALUES (?,?,?,?,?)`,
		websiteID, username, clientIP, userAgent, now)
	noteSemrushAccess(db, websiteID, username, clientIP, userAgent)
}

func noteSemrushAccess(db *sql.DB, websiteID int, username, clientIP, userAgent string) {
	sameCount := semrushSettingInt(db, "access_same_count", 4)
	sameMinutes := semrushSettingInt(db, "access_same_minutes", 30)
	multiCount := semrushSettingInt(db, "access_multi_count", 3)
	multiMinutes := semrushSettingInt(db, "access_multi_minutes", 3)
	repeatMinutes := semrushSettingInt(db, "access_repeat_minutes", 30)
	repeatHours := semrushSettingInt(db, "access_repeat_hours", 3)
	repeatOpens := semrushSettingInt(db, "access_repeat_opens", 4)
	now := time.Now().UTC()
	var reasons []string
	var same int
	_ = db.QueryRow(`SELECT COUNT(*) FROM login_events WHERE username=? AND website_id=? AND logged_in_at>=?`,
		username, websiteID, now.Add(-time.Duration(sameMinutes)*time.Minute).Format(time.RFC3339)).Scan(&same)
	if same >= sameCount {
		reasons = append(reasons, "Semrush opened "+strconv.Itoa(same)+" times in "+strconv.Itoa(sameMinutes)+" minutes")
	}
	var tools int
	_ = db.QueryRow(`SELECT COUNT(DISTINCT website_id) FROM login_events WHERE username=? AND logged_in_at>=?`,
		username, now.Add(-time.Duration(multiMinutes)*time.Minute).Format(time.RFC3339)).Scan(&tools)
	if tools >= multiCount {
		reasons = append(reasons, strconv.Itoa(tools)+" different tools opened within "+strconv.Itoa(multiMinutes)+" minutes")
	}
	if reason := semrushRepeatReason(db, username, repeatMinutes, repeatHours, repeatOpens); reason != "" {
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
			VALUES (?,?,?,?,?,?,?,?)`, websiteID, username, clientIP, "access_pattern", "", reason, userAgent, now.Format(time.RFC3339))
	}
}

func semrushRepeatReason(db *sql.DB, username string, maxGapMinutes, hours, minOpens int) string {
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

func semrushSettingInt(db *sql.DB, key string, fallback int) int {
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

func handleSemrushDeviceBind(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	token := semrushSessionToken(r)
	raw, ok := semrushSessions.Load(token)
	if token == "" || !ok {
		if token == "" || !restoreSemrushSession(token) {
			log.Printf("[DEVICE] bind rejected: no session cookie (cookie_header_empty=%v)", r.Header.Get("Cookie") == "")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"ok":false,"error":"no_session"}`)
			return
		}
		raw, ok = semrushSessions.Load(token)
		if !ok {
			log.Printf("[DEVICE] bind rejected: restore failed")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"ok":false,"error":"unknown_session"}`)
			return
		}
	}
	fp := strings.TrimSpace(r.Header.Get("X-Device-Fp"))
	proof := strings.TrimSpace(r.Header.Get("X-Device-Proof"))
	sess := raw.(*semrushGateSession)
	sess.mu.Lock()
	mismatch := false
	if fp == "missing" || proof == "missing" {
		mismatch = true
	} else if fp == "" || proof == "" {
		// Empty headers on first bind: allow without binding (document navigation OK).
		mismatch = false
	} else if sess.proof == "" {
		sess.fp = fp
		sess.proof = proof
	} else if sess.fp != fp || sess.proof != proof {
		mismatch = true
	}
	boundFp, boundProof := sess.fp, sess.proof
	sess.mu.Unlock()
	if mismatch {
		log.Printf("[DEVICE] bind rejected: device mismatch")
		semrushSessions.Delete(token)
		recordSemrushCookieShare(r, token)
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"ok":false,"error":"device_mismatch"}`)
		return
	}
	if boundFp != "" {
		if db, err := openSemrushPanel(); err == nil {
			_, _ = db.Exec(`UPDATE live_sessions SET fingerprint=? WHERE session_token=?`, boundFp, token)
		}
	}
	log.Printf("[DEVICE] bind ok proof=%v", boundProof != "")
	fmt.Fprint(w, `{"status":"ok"}`)
}

func semrushRejectDevice(w http.ResponseWriter, r *http.Request) bool {
	token := semrushSessionToken(r)
	if token == "" {
		return false
	}
	raw, ok := semrushSessions.Load(token)
	if !ok {
		return false
	}
	sess := raw.(*semrushGateSession)
	fp := strings.TrimSpace(r.Header.Get("X-Device-Fp"))
	proof := strings.TrimSpace(r.Header.Get("X-Device-Proof"))
	sess.mu.Lock()
	bound := sess.proof != ""
	storedFp, storedProof, username := sess.fp, sess.proof, sess.username
	sess.mu.Unlock()
	if !bound || (fp == storedFp && proof == storedProof) || semrushSubresource(r) || semrushStaticGet(r) {
		return false
	}
	// Missing device headers: allow (Semrush SPA boots many APIs before/without our fetch
	// patch). Only a real fingerprint/proof mismatch means cookie share → kill session.
	if fp == "" && proof == "" {
		return false
	}
	semrushSessions.Delete(token)
	recordSemrushCookieShare(r, token)
	log.Printf("[DEVICE] session killed user=%s", username)
	if semrushDocument(r) || strings.Contains(r.Header.Get("Accept"), "text/html") {
		renderSemrushDenied(w)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"device_mismatch","message":"Open this tool again from your access link."}`)
	}
	return true
}

func semrushStaticGet(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	path := strings.ToLower(strings.Split(r.URL.Path, "?")[0])
	if strings.HasPrefix(path, "/__static__/") || strings.HasPrefix(path, "/static-proxy/") || strings.HasPrefix(path, "/secure-proxy/") || strings.HasPrefix(path, "/cdn-proxy/") || strings.HasPrefix(path, "/advertising/") {
		return true
	}
	for _, ext := range []string{".js", ".css", ".json", ".map", ".woff", ".woff2", ".svg", ".png", ".jpg", ".jpeg", ".webp", ".gif", ".ico"} {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

func semrushSubresource(r *http.Request) bool {
	switch strings.ToLower(r.Header.Get("Sec-Fetch-Dest")) {
	case "image", "style", "font", "audio", "video", "script":
		return true
	}
	return false
}

func semrushDocument(r *http.Request) bool {
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

func recordSemrushCookieShare(r *http.Request, token string) {
	db, err := openSemrushPanel()
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
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = db.Exec(`INSERT INTO security_events (website_id, username, client_ip, event_type, attempted_url, details, user_agent, created_at)
		VALUES (?,?,?,?,?,?,?,?)`, websiteID, username, ip, "cookie_share", r.URL.RequestURI(),
		"Copied the session cookie into another browser. Both browsers were signed out.", r.UserAgent(), now)
	log.Printf("[PANEL] cookie share recorded user=%s", username)
}

func serveSemrushDeviceSW(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("self.addEventListener('fetch', function () { return; });\n"))
}

func renderSemrushDenied(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(semrushLightPage("Access Denied", "Access Denied",
		`You cannot open <span class="brand">Semrush</span> directly. Open it from your access link.`,
		`<p class="foot">A direct visit is not allowed</p>`, false, "")))
}

func renderSemrushProxyProblem(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusBadGateway)
	_, _ = w.Write([]byte(semrushLightPage("Proxy problem", "Proxy problem",
		"Contact to Admin/Provider to fix it ASAP", "", false, "")))
}

func semrushLightPage(title, heading, message, extra string, spin bool, script string) string {
	ring := "ring"
	if spin {
		ring = "ring spin"
	}
	scriptTag := ""
	if script != "" {
		scriptTag = "<script>" + script + "</script>"
	}
	return `<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1.0"><title>` + html.EscapeString(title) + `</title>
<style>*{box-sizing:border-box;margin:0;padding:0}body{min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;background:#eef3f8;color:#0f172a;font-family:system-ui,-apple-system,Segoe UI,sans-serif}.card{width:min(440px,100%);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px 36px;text-align:center}.ring{width:78px;height:78px;margin:0 auto 22px;border-radius:50%;background:conic-gradient(#3b82f6 0 70deg,#e7eef8 70deg 360deg);display:grid;place-items:center}.ring.spin{animation:spin .9s linear infinite}.lock{width:64px;height:64px;border-radius:50%;background:#fff;display:grid;place-items:center;font-size:26px}.ring.spin .lock{animation:spin .9s linear infinite reverse}h1{font-size:28px;line-height:1.2;font-weight:800;letter-spacing:-.03em;margin-bottom:12px}.msg{color:#64748b;font-size:15px;line-height:1.55}.brand{color:#2563eb;font-weight:700}.pill{margin:22px auto 0;display:inline-flex;align-items:center;gap:8px;padding:8px 14px;border:1px solid #e6ebf2;border-radius:999px;color:#334155;font-size:14px;background:#fff}.dot{width:14px;height:14px;border-radius:50%;border:2px solid #dbe4f0;border-top-color:#3b82f6;animation:spin .8s linear infinite}.foot{margin-top:18px;color:#94a3b8;font-size:13px}@keyframes spin{to{transform:rotate(360deg)}}</style></head>
<body><div class="card"><div class="` + ring + `"><div class="lock">&#128274;</div></div><h1>` + html.EscapeString(heading) + `</h1><p class="msg">` + message + `</p>` + extra + `</div>` + scriptTag + `</body></html>`
}

func semrushBootScript() string {
	// crypto.subtle only on HTTPS; HTTP needs weakHash.
	// Placeholder __SEM_SESSION__ is replaced with the real token in serveSemrushAccess.
	return `(function(){
var SEM="__SEM_SESSION__";
function proof(){var k="tm_device_proof";var e="";try{e=localStorage.getItem(k)||"";}catch(x){}if(e)return Promise.resolve(e);var e2="";try{var b=new Uint8Array(32);(crypto.getRandomValues||function(){for(var i=0;i<32;i++)b[i]=Math.floor(Math.random()*256);})(b);e2=Array.from(b).map(function(n){return n.toString(16).padStart(2,"0");}).join("");}catch(x){e2=String(Date.now())+Math.random().toString(16).slice(2);}try{localStorage.setItem(k,e2);}catch(x){}return Promise.resolve(e2);}
function weakHash(s){var h=0;for(var i=0;i<s.length;i++){h=((h<<5)-h)+s.charCodeAt(i);h|=0;}var out="";for(var j=0;j<8;j++){out+=((h>>> (j*4)) & 15).toString(16);h=(h*1664525+1013904223)|0;}while(out.length<64)out+=out;return out.slice(0,64);}
function fp(){var c=document.createElement("canvas");c.width=220;c.height=30;var g=c.getContext("2d");var sample="";if(g){g.textBaseline="top";g.font="14px Arial";g.fillStyle="#f60";g.fillRect(0,0,220,30);g.fillStyle="#069";g.fillText("tm-fp",2,2);try{sample=c.toDataURL().slice(-48);}catch(e){sample="x";}}var zone="";try{zone=Intl.DateTimeFormat().resolvedOptions().timeZone||"";}catch(e){}var raw=[navigator.userAgent||"",navigator.platform||"",navigator.language||"",String(navigator.hardwareConcurrency||0),String(screen.width)+"x"+String(screen.height),zone,sample].join("|");if(window.crypto&&crypto.subtle&&window.isSecureContext){return crypto.subtle.digest("SHA-256", new TextEncoder().encode(raw)).then(function(buf){return Array.from(new Uint8Array(buf)).map(function(b){return b.toString(16).padStart(2,"0");}).join("");});}return Promise.resolve(weakHash(raw));}
function fail(err){var h=document.querySelector("h1");var m=document.querySelector(".msg");if(h)h.textContent="Access Denied";if(m)m.textContent="Open this tool again from your access link.";(window.console&&console.warn&&console.warn("semrush boot",err));}
proof().then(function(p){return fp().then(function(f){try{localStorage.setItem("tm_device_fp",f);}catch(e){}var hdr={"X-Device-Fp":f,"X-Device-Proof":p};if(SEM&&SEM.indexOf("__")!==0)hdr["X-Sem-Session"]=SEM;return fetch("/api/device-bind",{method:"POST",credentials:"same-origin",headers:hdr});});}).then(function(res){if(!res.ok)throw new Error("bind "+res.status);location.replace("/");}).catch(fail);
})();`
}

func injectSemrushDeviceScript(body string) string {
	if strings.Contains(body, "data-tm-device") {
		return body
	}
	script := `<style data-tm-device>html{visibility:hidden !important}</style><script data-tm-device>
function tmDeny(){document.documentElement.style.cssText="visibility:visible;background:#eef3f8;margin:0";while(document.documentElement.firstChild)document.documentElement.removeChild(document.documentElement.firstChild);var b=document.createElement("body");b.style.cssText="min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;margin:0;background:#eef3f8;font-family:system-ui,sans-serif";b.innerHTML='<div style="width:min(440px,100%);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px;text-align:center"><div style="font-size:28px">&#128274;</div><h1 style="font-size:28px;margin:12px 0">Access Denied</h1><p style="color:#64748b">You cannot open <span style="color:#2563eb;font-weight:700">Semrush</span> directly. Open it from your access link.</p><p style="margin-top:18px;color:#94a3b8;font-size:13px">A direct visit is not allowed</p></div>';document.documentElement.appendChild(b);}
function tmReveal(){document.documentElement.style.visibility="visible";var s=document.querySelector("style[data-tm-device]");if(s)s.remove();}
function tmPatch(fp,proof){if(window.__tmDevicePatched)return;window.__tmDevicePatched=true;function sameOrigin(u){try{return new URL(String(u),location.href).origin===location.origin;}catch(e){return false;}}var orig=window.fetch;window.fetch=function(input,init){var url=typeof input==="string"?input:(input&&input.url)||"";if(sameOrigin(url)){init=init||{};var h=new Headers(init.headers||(input&&input.headers)||undefined);if(!h.get("X-Device-Fp"))h.set("X-Device-Fp",fp);if(!h.get("X-Device-Proof"))h.set("X-Device-Proof",proof);init.headers=h;}return orig.call(this,input,init);};var xo=XMLHttpRequest.prototype.open;var xs=XMLHttpRequest.prototype.send;XMLHttpRequest.prototype.open=function(m,u){this.__tmSame=sameOrigin(u);return xo.apply(this,arguments);};XMLHttpRequest.prototype.send=function(){if(this.__tmSame){try{this.setRequestHeader("X-Device-Fp",fp);this.setRequestHeader("X-Device-Proof",proof);}catch(e){}}return xs.apply(this,arguments);};if(window.EventSource){var OES=window.EventSource;window.EventSource=function(url,opts){try{var u=new URL(String(url),location.href);if(u.origin===location.origin){u.searchParams.set("tm_dfp",fp);u.searchParams.set("tm_dproof",proof);url=u.pathname+u.search;}}catch(e){}return new OES(url,opts);};window.EventSource.prototype=OES.prototype;}}
(function(){var proof="";try{proof=localStorage.getItem("tm_device_proof")||"";}catch(e){}if(!proof){fetch("/api/device-bind",{method:"POST",credentials:"same-origin",headers:{"X-Device-Fp":"missing","X-Device-Proof":"missing"}}).finally(tmDeny);return;}tmReveal();function weakHash(s){var h=0;for(var i=0;i<s.length;i++){h=((h<<5)-h)+s.charCodeAt(i);h|=0;}var out="";for(var j=0;j<8;j++){out+=((h>>> (j*4)) & 15).toString(16);h=(h*1664525+1013904223)|0;}while(out.length<64)out+=out;return out.slice(0,64);}function ensureFp(){var fp="";try{fp=localStorage.getItem("tm_device_fp")||"";}catch(e){}if(fp)return Promise.resolve(fp);var c=document.createElement("canvas");c.width=220;c.height=30;var g=c.getContext("2d");var sample="";if(g){g.textBaseline="top";g.font="14px Arial";g.fillStyle="#f60";g.fillRect(0,0,220,30);g.fillStyle="#069";g.fillText("tm-fp",2,2);try{sample=c.toDataURL().slice(-48);}catch(e){sample="x";}}var zone="";try{zone=Intl.DateTimeFormat().resolvedOptions().timeZone||"";}catch(e){}var raw=[navigator.userAgent||"",navigator.platform||"",navigator.language||"",String(navigator.hardwareConcurrency||0),String(screen.width)+"x"+String(screen.height),zone,sample].join("|");var done=function(f){try{localStorage.setItem("tm_device_fp",f);}catch(e){}return f;};if(window.crypto&&crypto.subtle&&window.isSecureContext){return crypto.subtle.digest("SHA-256",new TextEncoder().encode(raw)).then(function(buf){return done(Array.from(new Uint8Array(buf)).map(function(b){return b.toString(16).padStart(2,"0");}).join(""));});}return Promise.resolve(done(weakHash(raw)));}ensureFp().then(function(fp){tmPatch(fp,proof);setInterval(function(){fetch("/api/device-bind",{method:"POST",credentials:"same-origin",headers:{"X-Device-Fp":fp,"X-Device-Proof":proof}}).then(function(r){if(!r.ok)tmDeny();});},2000);});})();
</script>`
	lower := strings.ToLower(body)
	if h := strings.Index(lower, "<head"); h >= 0 {
		if gt := strings.IndexByte(lower[h:], '>'); gt >= 0 {
			at := h + gt + 1
			return body[:at] + script + body[at:]
		}
	}
	return script + body
}
