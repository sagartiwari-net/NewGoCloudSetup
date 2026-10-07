package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

type panelFOMetaKey struct{}

type panelFOMeta struct {
	Token    string
	Username string
	Account  ToolAccount
}

func withPanelFOMeta(r *http.Request, token, username string, acc ToolAccount) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), panelFOMetaKey{}, panelFOMeta{
		Token: token, Username: username, Account: acc,
	}))
}

func panelFOMetaFrom(ctx context.Context) (panelFOMeta, bool) {
	v, ok := ctx.Value(panelFOMetaKey{}).(panelFOMeta)
	return v, ok
}

func stoTruncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func stoCookieFP(cookie string) string {
	cookie = strings.TrimSpace(cookie)
	if cookie == "" {
		return ""
	}
	if tok := sessionToken(cookie); tok != "" {
		sum := sha256.Sum256([]byte(tok))
		return hex.EncodeToString(sum[:10])
	}
	sum := sha256.Sum256([]byte(cookie))
	return hex.EncodeToString(sum[:10])
}

func stoHome(cfg Config) string {
	home := cfg.HomePath
	if home == "" {
		home = "/"
	}
	if strings.Contains(home, "?") {
		return home + "&_tmr=1"
	}
	return home + "?_tmr=1"
}

func stoLooksLoggedOutPath(path string) bool {
	p := strings.ToLower(path)
	if idx := strings.Index(p, "?"); idx >= 0 {
		p = p[:idx]
	}
	markers := []string{
		"/login", "/signin", "/sign-in", "/signup", "/sign-up",
		"/auth/login", "/auth/signin", "/auth/sign-in",
		"/user/login", "/accounts/login",
	}
	for _, m := range markers {
		if p == m || strings.HasPrefix(p, m+"/") {
			return true
		}
	}
	return false
}

// stoLooksDeadAccount — login wall OR dead/no-subscription cookie (screenshot banner).
func stoLooksDeadAccount(path string, body []byte, location string) (bool, string) {
	if stoLooksLoggedOutPath(path) {
		return true, "url:" + stoTruncate(path, 80)
	}
	loc := strings.ToLower(strings.TrimSpace(location))
	if loc != "" && stoLooksLoggedOutPath(loc) {
		return true, "redirect:" + stoTruncate(location, 120)
	}
	if len(body) == 0 {
		return false, ""
	}
	low := strings.ToLower(string(body))
	if len(low) > 20000 {
		low = low[:20000]
	}
	subMarkers := []string{
		"do not have an active subscription",
		"you do not have an active subscription on your account",
		"oops, you do not have an active subscription",
	}
	for _, m := range subMarkers {
		if strings.Contains(low, m) {
			return true, "html:no_subscription"
		}
	}
	pairs := [][2]string{
		{"sign in", "password"},
		{"log in", "password"},
		{"login", "password"},
		{"email", "password"},
	}
	for _, pair := range pairs {
		if strings.Contains(low, pair[0]) && strings.Contains(low, pair[1]) {
			if strings.Contains(low, "forgot") || strings.Contains(low, "sign in to") ||
				strings.Contains(low, "log in to") || strings.Contains(low, "remember me") {
				return true, "html:" + pair[0]
			}
		}
	}
	return false, ""
}

func renderStoContactAdmin(w http.ResponseWriter, cfg Config, reason string) {
	writeLightCard(w, http.StatusServiceUnavailable, lightCard{
		Title:   "Account logged out",
		Heading: "Account logged out",
		Message: "SEO Tester Online account logged out (" + html.EscapeString(tmSanitizeReason(reason)) + "). Contact Admin/Provider to fix it ASAP.",
		Footer:  "Update the cookie in the panel, set status Active, then open a new access link",
	})
}

func renderStoSwitchPage(w http.ResponseWriter, cfg Config, accountName, returnPath string) {
	if returnPath == "" || !strings.HasPrefix(returnPath, "/") {
		returnPath = stoHome(cfg)
	}
	msg := "Account logged out. Switching to another account..."
	if accountName != "" {
		msg = "Account logged out. Switching to " + html.EscapeString(accountName) + "..."
	}
	writeLightCard(w, http.StatusOK, lightCard{
		Title: "Switching account", Heading: "Switching account", Message: msg,
		Badge: "Checking the next account", Footer: "This page refreshes automatically",
		Spin: true, Redirect: returnPath,
	})
}

func stoRedirectHome(w http.ResponseWriter, r *http.Request, cfg Config) {
	http.Redirect(w, r, stoHome(cfg), http.StatusFound)
}

// tryStoSoftRevive reloads panel cookie and sends user home once per cookie fingerprint
// before marking logged_out. Lets admin cookie updates take effect.
func tryStoSoftRevive(w http.ResponseWriter, r *http.Request, cfg Config, sessionToken, currentUser string, acc ToolAccount) bool {
	home := stoHome(cfg)
	reloaded, err := panelReloadAccount(cfg, sessionToken)
	if err == nil && reloaded.ID > 0 {
		acc = reloaded
	}
	fp := stoCookieFP(acc.Cookie)
	if fp == "" {
		return false
	}
	if stoAlreadySoftRevived(sessionToken, fp) {
		return false
	}
	stoNoteSoftRevive(sessionToken, fp)
	log.Printf("[FAILOVER] soft-revive user=%s account=%s fp=%s → %s", currentUser, acc.Name, fp, home)
	stoRedirectHome(w, r, cfg)
	return true
}

func serveStoCookieFailover(w http.ResponseWriter, r *http.Request, cfg Config, sessionToken, currentUser string, activeAcc ToolAccount, reason string) {
	log.Printf("[FAILOVER] start user=%s account=%s(%d) reason=%s", currentUser, activeAcc.Name, activeAcc.ID, reason)
	home := stoHome(cfg)

	if reloaded, err := panelReloadAccount(cfg, sessionToken); err == nil && reloaded.ID > 0 {
		activeAcc = reloaded
	}
	fp := stoCookieFP(activeAcc.Cookie)

	// Cookie changed in panel after a prior fail → home, do not mark again.
	if fp != "" && stoCookieChangedSinceFail(sessionToken, fp) {
		stoNoteSoftRevive(sessionToken, fp)
		log.Printf("[FAILOVER] cookie updated — retry home user=%s account=%s fp=%s", currentUser, activeAcc.Name, fp)
		stoRedirectHome(w, r, cfg)
		return
	}

	// First detection for this cookie: bounce home once (no logged_out yet).
	if tryStoSoftRevive(w, r, cfg, sessionToken, currentUser, activeAcc) {
		return
	}

	stoNoteFailover(sessionToken, fp)

	next, nextName, err := panelSwitchToOtherAccount(cfg, sessionToken, reason)
	if err == nil && next.ID > 0 && next.ID != activeAcc.ID {
		if activeAcc.ID > 0 {
			panelMarkAccountLoggedOut(cfg, activeAcc.ID, reason)
		}
		log.Printf("[FAILOVER] switched user=%s %s -> %s reason=%s", currentUser, activeAcc.Name, nextName, reason)
		renderStoSwitchPage(w, cfg, nextName, home)
		return
	}

	if activeAcc.ID > 0 {
		panelMarkAccountLoggedOut(cfg, activeAcc.ID, reason)
	}
	log.Printf("[FAILOVER] contact-admin user=%s account=%s reason=%s err=%v", currentUser, activeAcc.Name, reason, err)
	if db, dbErr := openPanelDB(cfg); dbErr == nil {
		var websiteID int
		_ = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID)
		tmRecordSwitchLogout(db, websiteID, currentUser, activeAcc.Name, "(none)", "no_other_active:"+reason)
	}
	stoNoteContactAdmin(sessionToken, fp)
	renderStoContactAdmin(w, cfg, reason)
}

func stoFailoverAPIHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if !usesPanelAccountMode(cfg) {
		http.Error(w, "not in panel mode", http.StatusBadRequest)
		return
	}
	username, sessErr := panelSessionUsername(r)
	if sessErr != nil {
		panelAccessDenied(w, cfg)
		return
	}
	token := ""
	if c, err := r.Cookie("ct_session"); err == nil {
		token = c.Value
	}
	if token == "" {
		panelAccessDenied(w, cfg)
		return
	}
	reason := strings.TrimSpace(r.URL.Query().Get("reason"))
	if reason == "" {
		reason = "client_auth_fail"
	}

	acc, err := loadPanelSessionAccount(cfg, token)
	if err != nil {
		// Status logged_out after prior fail — reload/revive panel cookie and retry home.
		reloaded, rerr := panelReloadAccount(cfg, token)
		if rerr == nil && reloaded.ID > 0 && stoCookieFP(reloaded.Cookie) != "" {
			fp := stoCookieFP(reloaded.Cookie)
			stoClearFailoverGate(token)
			stoNoteSoftRevive(token, fp)
			log.Printf("[FAILOVER] revive logged_out account user=%s account=%s fp=%s", username, reloaded.Name, fp)
			stoRedirectHome(w, r, cfg)
			return
		}
		renderStoContactAdmin(w, cfg, "no_mapped_account")
		return
	}

	// Stuck on contact-admin URL after cookie paste: escape if fingerprint changed.
	if stoContactAdminSticky(token) {
		if reloaded, rerr := panelReloadAccount(cfg, token); rerr == nil && reloaded.ID > 0 {
			fp := stoCookieFP(reloaded.Cookie)
			if fp != "" && stoCookieChangedSinceFail(token, fp) {
				stoClearFailoverGate(token)
				stoNoteSoftRevive(token, fp)
				log.Printf("[FAILOVER] escape contact-admin after cookie update user=%s fp=%s", username, fp)
				stoRedirectHome(w, r, cfg)
				return
			}
		}
		renderStoContactAdmin(w, cfg, reason)
		return
	}

	if stoFailoverGateRecently(token) {
		renderStoContactAdmin(w, cfg, reason)
		return
	}
	serveStoCookieFailover(w, r, cfg, token, username, acc, reason)
}

// applyStoFailoverToResponse runs failover and replaces the upstream HTML body.
func applyStoFailoverToResponse(resp *http.Response, cfg Config, meta panelFOMeta, reason string) {
	rec := httptest.NewRecorder()
	serveStoCookieFailover(rec, resp.Request, cfg, meta.Token, meta.Username, meta.Account, reason)
	body := rec.Body.Bytes()
	resp.StatusCode = rec.Code
	resp.Header.Del("Content-Encoding")
	for k, vv := range rec.Header() {
		resp.Header.Del(k)
		for _, v := range vv {
			resp.Header.Add(k, v)
		}
	}
	if resp.Header.Get("Content-Type") == "" {
		resp.Header.Set("Content-Type", "text/html; charset=utf-8")
	}
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
	resp.ContentLength = int64(len(body))
	resp.Body = io.NopCloser(bytes.NewReader(body))
}

func stoFailoverWatchScript() string {
	return `<script data-tm-sto-failover="1">
(function(){
  if (window.__tmStoFailoverWatch) return;
  window.__tmStoFailoverWatch = true;
  try {
    var q = new URLSearchParams(location.search || '');
    if (q.get('_tmr') === '1') {
      sessionStorage.setItem('tm_sto_fo_cool', String(Date.now() + 90000));
      q.delete('_tmr');
      var next = location.pathname + (q.toString() ? ('?' + q.toString()) : '') + (location.hash || '');
      history.replaceState(null, '', next || '/');
    }
  } catch (e) {}
  var hits = 0;
  function cooling(){
    try {
      var until = parseInt(sessionStorage.getItem('tm_sto_fo_cool') || '0', 10);
      return until && Date.now() < until;
    } catch (e) { return false; }
  }
  function go(reason){
    if (window.__tmStoFailing || cooling()) return;
    window.__tmStoFailing = true;
    try { sessionStorage.setItem('tm_sto_fo_cool', String(Date.now() + 90000)); } catch (e) {}
    try { location.replace('/api/sto-failover?reason=' + encodeURIComponent(reason || 'client')); }
    catch (e) {}
  }
  function onLoginPath(){
    try {
      var h = (location.pathname || '').toLowerCase();
      return h === '/login' || h.indexOf('/login/') === 0 ||
        h === '/signin' || h.indexOf('/signin/') === 0 ||
        h === '/sign-in' || h.indexOf('/sign-in/') === 0;
    } catch (e) { return false; }
  }
  function looksSubDead(){
    try {
      var t = (document.body && document.body.innerText || '').replace(/\s+/g,' ').trim().slice(0,2400).toLowerCase();
      if (!t) return false;
      return t.indexOf('do not have an active subscription') !== -1 ||
        t.indexOf('oops, you do not have an active subscription') !== -1;
    } catch (e) { return false; }
  }
  function looksLoginWall(){
    try {
      var t = (document.body && document.body.innerText || '').replace(/\s+/g,' ').trim().slice(0,1400).toLowerCase();
      if (!t || t.indexOf('password') < 0) return false;
      return t.indexOf('sign in') !== -1 || t.indexOf('log in') !== -1 ||
        (t.indexOf('login') !== -1 && (t.indexOf('forgot') !== -1 || t.indexOf('remember me') !== -1));
    } catch (e) { return false; }
  }
  function tick(){
    if (cooling()) { hits = 0; return; }
    if (looksSubDead() || (onLoginPath() && looksLoginWall())) {
      hits++;
      if (hits >= 2) go(looksSubDead() ? 'client_no_subscription' : 'client_login_wall');
    } else {
      hits = 0;
    }
  }
  setTimeout(tick, 1500);
  setTimeout(tick, 3500);
  setInterval(tick, 3000);
})();
</script>`
}

type stoFOEntry struct {
	at           time.Time
	contactAdmin bool
	softReviveAt time.Time
	cookieFP     string
	failFP       string
}

var (
	stoFOMu   sync.Mutex
	stoFOMap  = map[string]stoFOEntry{}
)

func stoAlreadySoftRevived(sessionToken, fp string) bool {
	stoFOMu.Lock()
	defer stoFOMu.Unlock()
	st, ok := stoFOMap[sessionToken]
	if !ok || fp == "" {
		return false
	}
	// Same cookie already got a soft revive — next hit may mark logged_out.
	return st.cookieFP == fp && !st.softReviveAt.IsZero() && time.Since(st.softReviveAt) < 15*time.Minute
}

func stoNoteSoftRevive(sessionToken, fp string) {
	stoFOMu.Lock()
	defer stoFOMu.Unlock()
	stoFOMap[sessionToken] = stoFOEntry{
		at:           time.Now(),
		softReviveAt: time.Now(),
		cookieFP:     fp,
		failFP:       fp,
		contactAdmin: false,
	}
}

func stoCookieChangedSinceFail(sessionToken, fp string) bool {
	stoFOMu.Lock()
	defer stoFOMu.Unlock()
	st, ok := stoFOMap[sessionToken]
	if !ok || fp == "" {
		return false
	}
	if st.failFP == "" && st.cookieFP == "" {
		return false
	}
	prev := st.failFP
	if prev == "" {
		prev = st.cookieFP
	}
	return prev != fp
}

func stoNoteFailover(sessionToken, fp string) {
	stoFOMu.Lock()
	defer stoFOMu.Unlock()
	st := stoFOMap[sessionToken]
	st.at = time.Now()
	st.failFP = fp
	st.cookieFP = fp
	st.contactAdmin = false
	stoFOMap[sessionToken] = st
}

func stoNoteContactAdmin(sessionToken, fp string) {
	stoFOMu.Lock()
	defer stoFOMu.Unlock()
	prev := stoFOMap[sessionToken]
	stoFOMap[sessionToken] = stoFOEntry{
		at:           time.Now(),
		contactAdmin: true,
		softReviveAt: prev.softReviveAt,
		cookieFP:     fp,
		failFP:       fp,
	}
}

func stoContactAdminSticky(sessionToken string) bool {
	stoFOMu.Lock()
	defer stoFOMu.Unlock()
	st, ok := stoFOMap[sessionToken]
	if !ok || !st.contactAdmin {
		return false
	}
	if time.Since(st.at) < 10*time.Minute {
		return true
	}
	delete(stoFOMap, sessionToken)
	return false
}

func stoFailoverGateRecently(sessionToken string) bool {
	stoFOMu.Lock()
	defer stoFOMu.Unlock()
	st, ok := stoFOMap[sessionToken]
	if !ok || st.contactAdmin {
		return false
	}
	// Soft-revive sets at == softReviveAt — never gate that.
	if !st.softReviveAt.IsZero() && !st.at.After(st.softReviveAt) {
		return false
	}
	if time.Since(st.at) < 8*time.Second && st.failFP != "" {
		return true
	}
	return false
}

func stoClearFailoverGate(sessionToken string) {
	stoFOMu.Lock()
	defer stoFOMu.Unlock()
	delete(stoFOMap, sessionToken)
}

// stoFailoverRecently — used by ModifyResponse to avoid loops.
func stoFailoverRecently(sessionToken string) bool {
	stoFOMu.Lock()
	defer stoFOMu.Unlock()
	st, ok := stoFOMap[sessionToken]
	if !ok {
		return false
	}
	if st.contactAdmin && time.Since(st.at) < 30*time.Second {
		return true
	}
	// Soft-revive just happened — don't immediately re-enter from HTML modify.
	if !st.softReviveAt.IsZero() && time.Since(st.softReviveAt) < 20*time.Second {
		return true
	}
	return false
}
