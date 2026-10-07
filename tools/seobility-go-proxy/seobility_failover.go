package main

import (
	"html"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

func seobilityTruncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func seobilityLooksLoggedOutPath(path string) bool {
	p := strings.ToLower(path)
	if idx := strings.Index(p, "?"); idx >= 0 {
		p = p[:idx]
	}
	markers := []string{
		"/user/login",
		"/user/signin",
		"/user/sign-in",
		"/user/register",
		"/login",
		"/signin",
		"/sign-in",
		"/register",
	}
	for _, m := range markers {
		if p == m || strings.HasPrefix(p, m+"/") {
			return true
		}
	}
	return false
}

// seobilityLooksLoggedOut — login wall from screenshot (/user/login + Login form).
func seobilityLooksLoggedOut(path string, body []byte, location string) (bool, string) {
	if seobilityLooksLoggedOutPath(path) {
		return true, "url:" + seobilityTruncate(path, 80)
	}
	loc := strings.ToLower(strings.TrimSpace(location))
	if loc != "" && seobilityLooksLoggedOutPath(loc) {
		return true, "redirect:" + seobilityTruncate(location, 120)
	}
	if len(body) == 0 {
		return false, ""
	}
	low := strings.ToLower(string(body))
	if len(low) > 14000 {
		low = low[:14000]
	}
	pairs := [][2]string{
		{"login with the data you used for your registration", "password"},
		{"login with the data you used", "email"},
		{">login<", "password forgotten"},
		{"new here?", "register free"},
		{"/user/login", "password"},
	}
	for _, pair := range pairs {
		if strings.Contains(low, pair[0]) && strings.Contains(low, pair[1]) {
			return true, "html:" + pair[0]
		}
	}
	return false, ""
}

func renderSeobilityContactAdmin(w http.ResponseWriter, cfg Config, reason string) {
	writeLightCard(w, http.StatusServiceUnavailable, lightCard{
		Title:   "Account logged out",
		Heading: "Account logged out",
		Message: "Seobility account logged out (" + html.EscapeString(tmSanitizeReason(reason)) + "). Contact Admin/Provider to fix it ASAP.",
		Footer:  "Update the cookie in the panel, set status Active, then open a new access link",
	})
}

func renderSeobilitySwitchPage(w http.ResponseWriter, cfg Config, accountName, returnPath string) {
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

func serveSeobilityCookieFailover(w http.ResponseWriter, r *http.Request, cfg Config, sessionToken, currentUser string, activeAcc ToolAccount, reason string) {
	log.Printf("[FAILOVER] start user=%s account=%s(%d) reason=%s", currentUser, activeAcc.Name, activeAcc.ID, reason)
	seobilityNoteFailover(sessionToken)
	home := cfg.HomePath
	if home == "" {
		home = "/dashboard"
	}
	if reloaded, err := panelReloadAccount(cfg, sessionToken); err == nil && reloaded.ID > 0 {
		activeAcc = reloaded
	}

	next, nextName, err := panelSwitchToOtherAccount(cfg, sessionToken, reason)
	if err == nil && next.ID > 0 && next.ID != activeAcc.ID {
		if activeAcc.ID > 0 {
			panelMarkAccountLoggedOut(cfg, activeAcc.ID, reason)
		}
		log.Printf("[FAILOVER] switched user=%s %s -> %s reason=%s", currentUser, activeAcc.Name, nextName, reason)
		renderSeobilitySwitchPage(w, cfg, nextName, home)
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
	seobilityNoteContactAdmin(sessionToken)
	renderSeobilityContactAdmin(w, cfg, reason)
}

func seobilityFailoverAPIHandler(w http.ResponseWriter, r *http.Request) {
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
		renderSeobilityContactAdmin(w, cfg, "no_mapped_account")
		return
	}
	if seobilityFailoverRecently(token) {
		renderSeobilityContactAdmin(w, cfg, reason)
		return
	}
	serveSeobilityCookieFailover(w, r, cfg, token, username, acc, reason)
}

func seobilityFailoverWatchScript() string {
	return `<script data-tm-seobility-failover="1">
(function(){
  if (window.__tmSeobilityFailoverWatch) return;
  window.__tmSeobilityFailoverWatch = true;
  function go(reason){
    if (window.__tmSeobilityFailing) return;
    window.__tmSeobilityFailing = true;
    try { location.replace('/api/seobility-failover?reason=' + encodeURIComponent(reason || 'client')); }
    catch (e) {}
  }
  function onLoginPath(){
    try {
      var h = (location.pathname || '').toLowerCase();
      return h.indexOf('/user/login') === 0 || h === '/login' || h.indexOf('/login/') === 0;
    } catch (e) { return false; }
  }
  function looksWall(){
    try {
      var t = (document.body && document.body.innerText || '').replace(/\s+/g,' ').trim().slice(0,1400).toLowerCase();
      if (!t) return false;
      if (t.indexOf('password') < 0) return false;
      return t.indexOf('login with the data you used') !== -1 ||
        (t.indexOf('login') !== -1 && t.indexOf('register free') !== -1) ||
        (t.indexOf('password forgotten') !== -1 && t.indexOf('email') !== -1);
    } catch (e) { return false; }
  }
  function tick(){
    if (onLoginPath() && looksWall()) go('client_login_wall');
    else if (looksWall() && onLoginPath()) go('client_wall');
  }
  if (onLoginPath()) setTimeout(tick, 600);
  setTimeout(tick, 1500);
  setInterval(tick, 2500);
})();
</script>`
}

var (
	seobilityFOMu    sync.Mutex
	seobilityFOState = map[string]time.Time{}
)

func seobilityNoteContactAdmin(sessionToken string) {
	seobilityFOMu.Lock()
	defer seobilityFOMu.Unlock()
	seobilityFOState[sessionToken] = time.Now()
}

func seobilityFailoverRecently(sessionToken string) bool {
	seobilityFOMu.Lock()
	defer seobilityFOMu.Unlock()
	at, ok := seobilityFOState[sessionToken]
	if !ok {
		return false
	}
	if time.Since(at) < 25*time.Second {
		return true
	}
	delete(seobilityFOState, sessionToken)
	return false
}

func seobilityNoteFailover(sessionToken string) {
	seobilityFOMu.Lock()
	defer seobilityFOMu.Unlock()
	seobilityFOState[sessionToken] = time.Now()
}
