package main

import (
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

func wordtuneCookieSummary(cookieHeader string) string {
	cookieHeader = strings.TrimSpace(cookieHeader)
	if cookieHeader == "" {
		return "empty"
	}
	hasSess := cookieValueFromHeader(cookieHeader, "__session") != ""
	hasGAESA := cookieValueFromHeader(cookieHeader, "GAESA") != ""
	return fmt.Sprintf("len=%d names=%d session=%v gaesa=%v",
		len(cookieHeader), countCookieNames(cookieHeader), hasSess, hasGAESA)
}

func truncateForLog(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func wordtuneLooksLoggedOutPath(path string) bool {
	p := strings.ToLower(path)
	if idx := strings.Index(p, "?"); idx >= 0 {
		p = p[:idx]
	}
	markers := []string{
		"/auth/signup",
		"/auth/sign-up",
		"/auth/signin",
		"/auth/sign-in",
		"/auth/login",
		"/editor/auth/signup",
		"/editor/auth/signin",
		"/editor/auth/login",
	}
	for _, m := range markers {
		if strings.Contains(p, m) {
			return true
		}
	}
	return false
}

func wordtuneLooksLoggedOutHTML(path string, body []byte, status int, location string) (bool, string) {
	if wordtuneLooksLoggedOutPath(path) {
		return true, "url_path:" + truncateForLog(path, 80)
	}
	loc := strings.ToLower(strings.TrimSpace(location))
	if loc != "" {
		if wordtuneLooksLoggedOutPath(loc) ||
			strings.Contains(loc, "/signup") || strings.Contains(loc, "/sign-up") ||
			strings.Contains(loc, "/signin") || strings.Contains(loc, "/sign-in") ||
			strings.Contains(loc, "/login") {
			return true, "redirect:" + truncateForLog(location, 120)
		}
	}
	if status == 401 || status == 403 {
		return true, fmt.Sprintf("html_status_%d", status)
	}
	if len(body) == 0 {
		return false, ""
	}
	low := strings.ToLower(string(body))
	if len(low) > 16000 {
		low = low[:16000]
	}
	// Strong Wordtune signup wall (screenshot): Welcome + Continue with Google.
	strongPairs := [][2]string{
		{"welcome to wordtune", "continue with google"},
		{"welcome to wordtune", "sign in or create an account"},
		{"continue with google", "continue with email"},
		{"continue with google", "continue with sso"},
	}
	for _, pair := range strongPairs {
		if strings.Contains(low, pair[0]) && strings.Contains(low, pair[1]) {
			return true, "html_wall:" + pair[0]
		}
	}
	markers := []string{
		`/editor/auth/signup`,
		`/auth/signup`,
		`"isauthenticated":false`,
		`"authenticated":false`,
	}
	for _, m := range markers {
		if strings.Contains(low, m) {
			return true, "html_marker:" + m
		}
	}
	return false, ""
}

func renderWordtuneContactAdminPage(w http.ResponseWriter, cfg Config, reason string) {
	name := html.EscapeString(toolDisplayName(cfg))
	msg := "Wordtune account logged out. Contact Admin/Provider to fix it ASAP."
	if reason != "" {
		msg = "Wordtune account logged out (" + html.EscapeString(reason) + "). Contact Admin/Provider to fix it ASAP."
	}
	_ = name
	writeLightCard(w, http.StatusServiceUnavailable, lightCard{
		Title:   "Account logged out",
		Heading: "Account logged out",
		Message: msg,
		Footer:  "Update the Wordtune cookie in the panel, set status Active, then open a new access link",
	})
}

func renderWordtuneAccountSwitchPage(w http.ResponseWriter, cfg Config, accountName, returnPath string) {
	if returnPath == "" || !strings.HasPrefix(returnPath, "/") {
		returnPath = cfg.HomePath
	}
	if returnPath == "" {
		returnPath = "/editor/"
	}
	msg := "This account signed out. Trying the next available account."
	if accountName != "" {
		msg = "Account logged out. Switching to " + html.EscapeString(accountName) + "..."
	}
	_ = cfg
	writeLightCard(w, http.StatusOK, lightCard{
		Title:    "Switching account",
		Heading:  "Switching account",
		Message:  msg,
		Badge:    "Checking the next account",
		Footer:   "This page refreshes automatically",
		Spin:     true,
		Redirect: returnPath,
	})
}

// serveWordtuneCookieFailover: mark dead cookie logged_out → switch if another
// active account exists → otherwise contact-admin page.
func serveWordtuneCookieFailover(w http.ResponseWriter, r *http.Request, cfg Config, sessionToken, currentUser string, activeAcc ToolAccount, reason string) {
	log.Printf("[FAILOVER] start user=%s account=%s(%d) reason=%s cookie=%s",
		currentUser, activeAcc.Name, activeAcc.ID, reason, wordtuneCookieSummary(activeAcc.Cookie))
	pushProxyLog(ProxyLogEntry{
		Source:  "FAILOVER",
		Level:   "error",
		Method:  r.Method,
		Path:    r.URL.Path,
		Status:  503,
		User:    currentUser,
		Account: activeAcc.Name,
		Detail:  "cookie_dead reason=" + reason + " " + wordtuneCookieSummary(activeAcc.Cookie),
		CookieN: countCookieNames(activeAcc.Cookie),
	})

	// Fresh paste into the same panel row can revive without switching.
	if reloaded, err := panelReloadAccount(cfg, sessionToken); err == nil && reloaded.ID == activeAcc.ID {
		oldFP := cookieSessionFingerprint(activeAcc.Cookie)
		newFP := cookieSessionFingerprint(reloaded.Cookie)
		if newFP != "" && newFP != oldFP {
			clearLocalCookieOverlay()
			home := cfg.HomePath
			if home == "" {
				home = "/editor/"
			}
			log.Printf("[FAILOVER] reloaded fresher cookie user=%s account=%s", currentUser, reloaded.Name)
			http.Redirect(w, r, home, http.StatusFound)
			return
		}
		activeAcc = reloaded
	}

	// Always mark the dead account logged_out in panel.db so DigitaVision / admin sees it.
	if activeAcc.ID > 0 {
		panelMarkAccountLoggedOut(cfg, activeAcc.ID, reason)
	}
	clearLocalCookieOverlay()

	next, nextName, err := panelSwitchToOtherAccount(cfg, sessionToken, reason)
	if err == nil && next.ID != activeAcc.ID {
		log.Printf("[FAILOVER] switched user=%s %s -> %s reason=%s cookie=%s",
			currentUser, activeAcc.Name, nextName, reason, wordtuneCookieSummary(next.Cookie))
		pushProxyLog(ProxyLogEntry{
			Source:  "FAILOVER",
			Level:   "info",
			Method:  r.Method,
			Path:    r.URL.Path,
			Status:  200,
			User:    currentUser,
			Account: nextName,
			Detail:  "switched_from=" + activeAcc.Name + " reason=" + reason,
			CookieN: countCookieNames(next.Cookie),
		})
		home := cfg.HomePath
		if home == "" {
			home = "/editor/"
		}
		renderWordtuneAccountSwitchPage(w, cfg, nextName, home)
		return
	}

	log.Printf("[FAILOVER] contact-admin user=%s account=%s reason=%s err=%v", currentUser, activeAcc.Name, reason, err)
	pushProxyLog(ProxyLogEntry{
		Source:  "FAILOVER",
		Level:   "error",
		Method:  r.Method,
		Path:    r.URL.Path,
		Status:  503,
		User:    currentUser,
		Account: activeAcc.Name,
		Detail:  "contact_admin reason=" + reason + " err=" + fmt.Sprint(err),
		CookieN: countCookieNames(activeAcc.Cookie),
	})
	wordtuneNoteContactAdmin(sessionToken)
	renderWordtuneContactAdminPage(w, cfg, reason)
}

func wordtuneFailoverAPIHandler(w http.ResponseWriter, r *http.Request) {
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
	token, _, ok := sessionFromRequest(r)
	if !ok || token == "" {
		renderAccessDeniedPage(w, cfg)
		return
	}
	reason := strings.TrimSpace(r.URL.Query().Get("reason"))
	if reason == "" {
		reason = "client_auth_fail"
	}
	// Prefer currently assigned row even if just marked logged_out — failover marks + switches.
	acc, err := loadPanelSessionAccount(cfg, token)
	if err != nil {
		log.Printf("[FAILOVER] api no account: %v", err)
		renderWordtuneContactAdminPage(w, cfg, "no_mapped_account")
		return
	}
	if wordtuneFailoverRecently(token) {
		// Still in cooldown after a switch/contact-admin — don't loop.
		renderWordtuneContactAdminPage(w, cfg, reason)
		return
	}
	serveWordtuneCookieFailover(w, r, cfg, token, username, acc, reason)
}

// Client watchdog: SPA can land on signup without a full document round-trip.
func wordtuneFailoverWatchScript() string {
	return `<script data-tm-wordtune-failover="1">
(function(){
  if (window.__tmWordtuneFailoverWatch) return;
  window.__tmWordtuneFailoverWatch = true;
  function go(reason){
    if (window.__tmWordtuneFailing) return;
    window.__tmWordtuneFailing = true;
    try { location.replace('/api/wordtune-failover?reason=' + encodeURIComponent(reason || 'client')); }
    catch (e) {}
  }
  function looksLogout(){
    try {
      var h = location.pathname || '';
      if (/\/auth\/(sign-?up|sign-?in|login)/i.test(h)) return 'path';
      var t = (document.body && document.body.innerText || '').replace(/\s+/g,' ').trim().slice(0,900);
      if (/Welcome to Wordtune/i.test(t) && /Continue with Google/i.test(t)) return 'wall';
      if (/Sign in or create an account/i.test(t) && /Continue with Email/i.test(t)) return 'wall2';
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

type wordtuneFOState struct {
	at           time.Time
	contactAdmin bool
}

var (
	wordtuneFailoverMu    sync.Mutex
	wordtuneFailoverState = map[string]wordtuneFOState{}
)

// wordtuneFailoverRecently rate-limits failover loops.
// After a successful switch, only ~5s cooldown so the next dead cookie can rotate.
// After contact-admin, longer cooldown so the message is stable.
func wordtuneFailoverRecently(sessionToken string) bool {
	wordtuneFailoverMu.Lock()
	defer wordtuneFailoverMu.Unlock()
	st, ok := wordtuneFailoverState[sessionToken]
	wait := 5 * time.Second
	if ok && st.contactAdmin {
		wait = 40 * time.Second
	}
	if ok && time.Since(st.at) < wait {
		return true
	}
	wordtuneFailoverState[sessionToken] = wordtuneFOState{at: time.Now(), contactAdmin: false}
	return false
}

func wordtuneNoteContactAdmin(sessionToken string) {
	wordtuneFailoverMu.Lock()
	defer wordtuneFailoverMu.Unlock()
	wordtuneFailoverState[sessionToken] = wordtuneFOState{at: time.Now(), contactAdmin: true}
}
