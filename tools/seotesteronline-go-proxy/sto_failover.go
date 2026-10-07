package main

import (
	"bytes"
	"context"
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
		returnPath = cfg.HomePath
	}
	if returnPath == "" {
		returnPath = "/"
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

func serveStoCookieFailover(w http.ResponseWriter, r *http.Request, cfg Config, sessionToken, currentUser string, activeAcc ToolAccount, reason string) {
	log.Printf("[FAILOVER] start user=%s account=%s(%d) reason=%s", currentUser, activeAcc.Name, activeAcc.ID, reason)
	stoNoteFailover(sessionToken)
	home := cfg.HomePath
	if home == "" {
		home = "/"
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
	stoNoteContactAdmin(sessionToken)
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
		// Already logged_out / no active cookie — do not revive here.
		renderStoContactAdmin(w, cfg, "no_mapped_account")
		return
	}
	if stoFailoverRecently(token) {
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
	resp.Header.Del("Location")
	resp.Header.Set("Content-Type", "text/html; charset=utf-8")
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
	resp.ContentLength = int64(len(body))
	resp.Body = io.NopCloser(bytes.NewReader(body))
}

func stoFailoverWatchScript() string {
	return `<script data-tm-sto-failover="1">
(function(){
  if (window.__tmStoFailoverWatch) return;
  window.__tmStoFailoverWatch = true;
  function go(reason){
    if (window.__tmStoFailing) return;
    window.__tmStoFailing = true;
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
        t.indexOf('oops, you do not have an active subscription') !== -1 ||
        (t.indexOf('active subscription') !== -1 && t.indexOf('choose the best plan') !== -1);
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
    if (looksSubDead()) { go('client_no_subscription'); return; }
    if (onLoginPath() && looksLoginWall()) go('client_login_wall');
  }
  setTimeout(tick, 800);
  setTimeout(tick, 2000);
  setInterval(tick, 2500);
  try {
    var obs = new MutationObserver(function(){ tick(); });
    if (document.documentElement) obs.observe(document.documentElement, { childList: true, subtree: true, characterData: true });
  } catch (e) {}
})();
</script>`
}

var (
	stoFOMu    sync.Mutex
	stoFOState = map[string]time.Time{}
)

func stoNoteContactAdmin(sessionToken string) {
	stoFOMu.Lock()
	defer stoFOMu.Unlock()
	stoFOState[sessionToken] = time.Now()
}

func stoFailoverRecently(sessionToken string) bool {
	stoFOMu.Lock()
	defer stoFOMu.Unlock()
	at, ok := stoFOState[sessionToken]
	if !ok {
		return false
	}
	if time.Since(at) < 25*time.Second {
		return true
	}
	delete(stoFOState, sessionToken)
	return false
}

func stoNoteFailover(sessionToken string) {
	stoFOMu.Lock()
	defer stoFOMu.Unlock()
	stoFOState[sessionToken] = time.Now()
}
