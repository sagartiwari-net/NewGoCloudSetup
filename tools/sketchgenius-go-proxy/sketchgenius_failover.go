package main

import (
	"html"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

func sgTruncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func sgLooksLoggedOutPath(path string) bool {
	p := strings.ToLower(path)
	if idx := strings.Index(p, "?"); idx >= 0 {
		p = p[:idx]
	}
	markers := []string{
		"/login",
		"/signin",
		"/sign-in",
		"/register",
		"/signup",
		"/sign-up",
		"/auth/login",
		"/auth/signin",
	}
	for _, m := range markers {
		if p == m || strings.HasPrefix(p, m+"/") {
			return true
		}
	}
	return false
}

// sgLooksLoggedOut — login wall / Unauthenticated (SPA may stay on /dashboard).
func sgLooksLoggedOut(path string, body []byte, location string) (bool, string) {
	if sgLooksLoggedOutPath(path) {
		return true, "url:" + sgTruncate(path, 80)
	}
	loc := strings.ToLower(strings.TrimSpace(location))
	if loc != "" && sgLooksLoggedOutPath(loc) {
		return true, "redirect:" + sgTruncate(location, 120)
	}
	if len(body) == 0 {
		return false, ""
	}
	low := strings.ToLower(string(body))
	if len(low) > 16000 {
		low = low[:16000]
	}
	if strings.Contains(low, "unauthenticated") &&
		(strings.Contains(low, "password") || strings.Contains(low, "log in") || strings.Contains(low, "email")) {
		return true, "html:unauthenticated"
	}
	pairs := [][2]string{
		{"email address", "password"},
		{"log in", "email address"},
		{"remember me", "password"},
		{"log in", "password"},
		{">log in<", "password"},
	}
	for _, pair := range pairs {
		if strings.Contains(low, pair[0]) && strings.Contains(low, pair[1]) {
			return true, "html:" + pair[0]
		}
	}
	return false, ""
}

func renderSgContactAdmin(w http.ResponseWriter, cfg Config, reason string) {
	writeLightCard(w, http.StatusServiceUnavailable, lightCard{
		Title:   "Account logged out",
		Heading: "Account logged out",
		Message: "SketchGenius account logged out (" + html.EscapeString(tmSanitizeReason(reason)) + "). Contact Admin/Provider to fix it ASAP.",
		Footer:  "Update the cookie in the panel, set status Active, then open a new access link",
	})
}

func renderSgSwitchPage(w http.ResponseWriter, cfg Config, accountName, returnPath string) {
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

func serveSgCookieFailover(w http.ResponseWriter, r *http.Request, cfg Config, sessionToken, currentUser string, activeAcc ToolAccount, reason string) {
	log.Printf("[FAILOVER] start user=%s account=%s(%d) reason=%s", currentUser, activeAcc.Name, activeAcc.ID, reason)
	sgNoteFailover(sessionToken)
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
		renderSgSwitchPage(w, cfg, nextName, home)
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
	sgNoteContactAdmin(sessionToken)
	renderSgContactAdmin(w, cfg, reason)
}

func sgFailoverAPIHandler(w http.ResponseWriter, r *http.Request) {
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
		// Already marked logged_out — still show contact-admin.
		if reloaded, rErr := panelReloadAccount(cfg, token); rErr == nil {
			acc = reloaded
		} else {
			renderSgContactAdmin(w, cfg, "no_mapped_account")
			return
		}
	}
	if sgFailoverRecently(token) {
		renderSgContactAdmin(w, cfg, reason)
		return
	}
	serveSgCookieFailover(w, r, cfg, token, username, acc, reason)
}

func sgFailoverWatchScript() string {
	return `<script data-tm-sg-failover="1">
(function(){
  if (window.__tmSgFailoverWatch) return;
  window.__tmSgFailoverWatch = true;
  function go(reason){
    if (window.__tmSgFailing) return;
    window.__tmSgFailing = true;
    var url = '/api/sg-failover?reason=' + encodeURIComponent(reason || 'client');
    try { location.replace(url); } catch (e) {
      try { location.href = url; } catch (e2) {}
    }
  }
  function onLoginPath(){
    try {
      var h = (location.pathname || '').toLowerCase();
      return h === '/login' || h.indexOf('/login/') === 0 ||
        h === '/signin' || h === '/sign-in' ||
        h.indexOf('/auth/login') === 0;
    } catch (e) { return false; }
  }
  function looksWall(){
    try {
      // SPA login form (toasts can be empty) — password + email inputs are enough.
      var pass = document.querySelector('input[type="password"]');
      var email = document.querySelector('input[type="email"], input[name*="email" i], input[id*="email" i], input[placeholder*="mail" i]');
      if (pass && email) return true;
      var logoLogin = document.querySelector('form input[type="password"]') &&
        /log\s*in/i.test((document.body && document.body.innerText) || '');
      if (logoLogin) return true;
      var t = (document.body && document.body.innerText || '').replace(/\s+/g,' ').trim().slice(0,1800).toLowerCase();
      if (!t) return false;
      if (t.indexOf('unauthenticated') !== -1) return true;
      var hasPass = t.indexOf('password') !== -1;
      var hasEmail = t.indexOf('email') !== -1;
      var hasLogin = t.indexOf('log in') !== -1 || t.indexOf('login') !== -1;
      return hasPass && hasEmail && hasLogin;
    } catch (e) { return false; }
  }
  function tick(){
    if (looksWall()) go(onLoginPath() ? 'client_login_wall' : 'client_unauthenticated_wall');
  }
  // Hook API 401 / Unauthenticated so we don't wait only on DOM text.
  try {
    var ofetch = window.fetch;
    if (typeof ofetch === 'function') {
      window.fetch = function(){
        return ofetch.apply(this, arguments).then(function(resp){
          try {
            if (resp && (resp.status === 401 || resp.status === 419)) go('client_fetch_' + resp.status);
            else {
              var ct = (resp && resp.headers && resp.headers.get('content-type')) || '';
              if (ct.indexOf('json') !== -1) {
                resp.clone().text().then(function(txt){
                  if (/unauthenticated/i.test(txt || '')) go('client_fetch_unauthenticated');
                }).catch(function(){});
              }
            }
          } catch (e) {}
          return resp;
        });
      };
    }
  } catch (e3) {}
  setTimeout(tick, 400);
  setTimeout(tick, 1200);
  setTimeout(tick, 2500);
  setInterval(tick, 2000);
})();
</script>`
}

var (
	sgFOMu    sync.Mutex
	sgFOState = map[string]time.Time{}
)

func sgNoteContactAdmin(sessionToken string) {
	sgFOMu.Lock()
	defer sgFOMu.Unlock()
	sgFOState[sessionToken] = time.Now()
}

func sgFailoverRecently(sessionToken string) bool {
	sgFOMu.Lock()
	defer sgFOMu.Unlock()
	at, ok := sgFOState[sessionToken]
	if !ok {
		return false
	}
	if time.Since(at) < 25*time.Second {
		return true
	}
	delete(sgFOState, sessionToken)
	return false
}

func sgNoteFailover(sessionToken string) {
	sgFOMu.Lock()
	defer sgFOMu.Unlock()
	sgFOState[sessionToken] = time.Now()
}
