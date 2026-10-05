package main

import (
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

func sscLooksLoggedOutPath(path string) bool {
	p := strings.ToLower(path)
	if idx := strings.Index(p, "?"); idx >= 0 {
		p = p[:idx]
	}
	markers := []string{
		"/auth/login",
		"/auth/sign-in",
		"/auth/signin",
		"/auth/signup",
		"/auth/sign-up",
		"/login",
		"/sign-in",
		"/signin",
		"/signup",
		"/sign-up",
	}
	for _, m := range markers {
		if p == m || strings.HasPrefix(p, m+"/") {
			return true
		}
	}
	return false
}

func sscLooksLoggedOut(path string, body []byte, status int, location string) (bool, string) {
	if sscLooksLoggedOutPath(path) {
		return true, "url:" + truncateForLog(path, 80)
	}
	loc := strings.ToLower(strings.TrimSpace(location))
	if loc != "" && sscLooksLoggedOutPath(loc) {
		return true, "redirect:" + truncateForLog(location, 120)
	}
	if status == 401 || status == 403 {
		return true, fmt.Sprintf("status_%d", status)
	}
	if len(body) == 0 {
		return false, ""
	}
	low := strings.ToLower(string(body))
	if len(low) > 14000 {
		low = low[:14000]
	}
	pairs := [][2]string{
		{"sign in", "password"},
		{"log in", "password"},
		{"sign in to seo", "email"},
		{"auth/login", "password"},
	}
	for _, pair := range pairs {
		if strings.Contains(low, pair[0]) && strings.Contains(low, pair[1]) {
			return true, "html:" + pair[0]
		}
	}
	return false, ""
}

func truncateForLog(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func clearSscIamCache(cfg Config) {
	cachedIamTokens.Store(IamTokens{})
	path := cfg.IamFile
	if path == "" {
		path = "iam.txt"
	}
	_ = os.Remove(path)
}

func renderSscContactAdmin(w http.ResponseWriter, cfg Config, reason string) {
	writeLightCard(w, http.StatusServiceUnavailable, lightCard{
		Title:   "Account logged out",
		Heading: "Account logged out",
		Message: "SEO Site Checkup account logged out (" + html.EscapeString(tmSanitizeReason(reason)) + "). Contact Admin/Provider to fix it ASAP.",
		Footer:  "Update the cookie in the panel, set status Active, then open a new access link",
	})
}

func renderSscSwitchPage(w http.ResponseWriter, cfg Config, accountName, returnPath string) {
	if returnPath == "" || !strings.HasPrefix(returnPath, "/") {
		returnPath = cfg.HomePath
	}
	if returnPath == "" {
		returnPath = "/dashboard"
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

// trySscPanelRevive: after DigitaVision/panel cookie paste, clear stale IAM and retry /dashboard once.
func trySscPanelRevive(w http.ResponseWriter, r *http.Request, cfg Config, sessionToken, currentUser string, activeAcc ToolAccount) bool {
	home := cfg.HomePath
	if home == "" {
		home = "/dashboard"
	}
	if sscReviveRecently(sessionToken) {
		return false
	}
	clearSscIamCache(cfg)
	reloaded, err := panelReloadAccount(cfg, sessionToken)
	if err != nil || reloaded.ID <= 0 {
		reloaded = activeAcc
	}
	tokens := loadIamFromRaw(reloaded.Cookie, "ssc.iam")
	if tokens.AccessToken == "" {
		return false
	}
	sscNoteRevive(sessionToken)
	log.Printf("[FAILOVER] revive from panel cookie user=%s account=%s iam_fp=%s → %s",
		currentUser, reloaded.Name, iamFingerprint(tokens), home)
	http.Redirect(w, r, home, http.StatusFound)
	return true
}

// serveSscCookieFailover: mark logged_out → switch if another active account → else contact-admin.
func serveSscCookieFailover(w http.ResponseWriter, r *http.Request, cfg Config, sessionToken, currentUser string, activeAcc ToolAccount, reason string) {
	log.Printf("[FAILOVER] start user=%s account=%s(%d) reason=%s", currentUser, activeAcc.Name, activeAcc.ID, reason)
	home := cfg.HomePath
	if home == "" {
		home = "/dashboard"
	}

	// Panel cookie just updated / status Active again → one revive before marking logged_out.
	if trySscPanelRevive(w, r, cfg, sessionToken, currentUser, activeAcc) {
		return
	}

	if reloaded, err := panelReloadAccount(cfg, sessionToken); err == nil && reloaded.ID > 0 {
		activeAcc = reloaded
	}

	if activeAcc.ID > 0 {
		panelMarkAccountLoggedOut(cfg, activeAcc.ID, reason)
	}
	clearSscIamCache(cfg)

	next, nextName, err := panelSwitchToOtherAccount(cfg, sessionToken, reason)
	if err == nil && next.ID != activeAcc.ID {
		log.Printf("[FAILOVER] switched user=%s %s -> %s reason=%s", currentUser, activeAcc.Name, nextName, reason)
		renderSscSwitchPage(w, cfg, nextName, home)
		return
	}

	log.Printf("[FAILOVER] contact-admin user=%s account=%s reason=%s err=%v", currentUser, activeAcc.Name, reason, err)
	if db, dbErr := openPanelDB(cfg); dbErr == nil {
		var websiteID int
		_ = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID)
		tmRecordSwitchLogout(db, websiteID, currentUser, activeAcc.Name, "(none)", "no_other_active:"+reason)
	}
	sscNoteContactAdmin(sessionToken)
	renderSscContactAdmin(w, cfg, reason)
}

func sscFailoverAPIHandler(w http.ResponseWriter, r *http.Request) {
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
		renderAccessDeniedPage(w, cfg)
		return
	}
	token := ""
	if c, err := r.Cookie("ct_session"); err == nil {
		token = c.Value
	}
	if token == "" {
		renderAccessDeniedPage(w, cfg)
		return
	}
	reason := strings.TrimSpace(r.URL.Query().Get("reason"))
	if reason == "" {
		reason = "client_auth_fail"
	}
	acc, err := loadPanelSessionAccount(cfg, token)
	if err != nil {
		log.Printf("[FAILOVER] api no account: %v", err)
		renderSscContactAdmin(w, cfg, "no_mapped_account")
		return
	}
	if sscFailoverRecently(token) {
		renderSscContactAdmin(w, cfg, reason)
		return
	}
	serveSscCookieFailover(w, r, cfg, token, username, acc, reason)
}

func sscFailoverWatchScript() string {
	return `<script data-tm-ssc-failover="1">
(function(){
  if (window.__tmSscFailoverWatch) return;
  window.__tmSscFailoverWatch = true;
  function go(reason){
    if (window.__tmSscFailing) return;
    window.__tmSscFailing = true;
    try { location.replace('/api/ssc-failover?reason=' + encodeURIComponent(reason || 'client')); }
    catch (e) {}
  }
  function looksLogout(){
    try {
      var h = location.pathname || '';
      if (/\/auth\/(login|sign-?in|sign-?up)/i.test(h)) return 'path';
      if (/^\/(login|sign-?in|sign-?up)(\/|$)/i.test(h)) return 'path';
      var t = (document.body && document.body.innerText || '').replace(/\s+/g,' ').trim().slice(0,900);
      if (/sign in/i.test(t) && /password/i.test(t) && t.length < 800) return 'wall';
    } catch (e) {}
    return '';
  }
  function tick(){
    var why = looksLogout();
    if (why) go('client_' + why);
  }
  setTimeout(tick, 800);
  setInterval(tick, 2000);
  try {
    var _push = history.pushState;
    var _replace = history.replaceState;
    history.pushState = function(){ var r = _push.apply(this, arguments); setTimeout(tick, 0); return r; };
    history.replaceState = function(){ var r = _replace.apply(this, arguments); setTimeout(tick, 0); return r; };
    window.addEventListener('popstate', function(){ setTimeout(tick, 0); });
  } catch (e) {}
})();
</script>`
}

type sscFOState struct {
	at           time.Time
	contactAdmin bool
	revived      bool
}

var (
	sscFailoverMu    sync.Mutex
	sscFailoverState = map[string]sscFOState{}
)

func sscFailoverRecently(sessionToken string) bool {
	sscFailoverMu.Lock()
	defer sscFailoverMu.Unlock()
	st, ok := sscFailoverState[sessionToken]
	wait := 5 * time.Second
	if ok && st.contactAdmin {
		wait = 40 * time.Second
	}
	if ok && time.Since(st.at) < wait {
		return true
	}
	sscFailoverState[sessionToken] = sscFOState{at: time.Now(), contactAdmin: false}
	return false
}

func sscNoteContactAdmin(sessionToken string) {
	sscFailoverMu.Lock()
	defer sscFailoverMu.Unlock()
	sscFailoverState[sessionToken] = sscFOState{at: time.Now(), contactAdmin: true}
}

func sscReviveRecently(sessionToken string) bool {
	sscFailoverMu.Lock()
	defer sscFailoverMu.Unlock()
	st, ok := sscFailoverState[sessionToken]
	if ok && st.revived && time.Since(st.at) < 45*time.Second {
		return true
	}
	return false
}

func sscNoteRevive(sessionToken string) {
	sscFailoverMu.Lock()
	defer sscFailoverMu.Unlock()
	sscFailoverState[sessionToken] = sscFOState{at: time.Now(), revived: true}
}
