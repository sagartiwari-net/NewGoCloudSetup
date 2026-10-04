package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

var atpStripScriptsRe = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>|<style[^>]*>.*?</style>`)

// Rate-limit sole-account login-wall retries so a dead cookie does not spin every second,
// while still retrying home after a panel cookie update.
var (
	atpFailoverMu   sync.Mutex
	atpFailoverLast = map[string]time.Time{}

	atpHealthMu sync.Mutex
	atpHealth   = map[string]atpHealthEntry{}
)

type atpHealthEntry struct {
	ok        bool
	checkedAt time.Time
	cookieFP  string
}

func atpIsLoginPath(path string) bool {
	p := strings.Split(path, "?")[0]
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		p = "/"
	}
	if p == "/login" || p == "/sign-in" || p == "/en/login" {
		return true
	}
	return strings.Contains(p, "/users/sign_in") || strings.HasSuffix(p, "/sign_in")
}

func atpLoginDocument(r *http.Request, path string) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if !atpIsLoginPath(path) {
		return false
	}
	accept := r.Header.Get("Accept")
	return accept == "" || strings.Contains(accept, "text/html") || strings.Contains(accept, "*/*")
}

// atpBodyLooksLikeLogin detects the Welcome-back wall even when the URL is still /dashboard/...
// Scripts/styles are stripped first so our injected watch needles cannot false-trigger.
func atpBodyLooksLikeLogin(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	s := string(body)
	if len(s) > 120000 {
		s = s[:120000]
	}
	s = atpStripScriptsRe.ReplaceAllString(s, " ")
	welcome := strings.Contains(s, "Welcome back")
	signIn := strings.Contains(s, "Sign in with email") ||
		strings.Contains(s, "Continue with Google") ||
		strings.Contains(s, "Sign in to your existing account")
	return welcome && signIn
}

func atpWaitingRetryScript(home string, delayMS int) string {
	if delayMS <= 0 {
		delayMS = 2500
	}
	// Retry the real app path so a freshly updated panel cookie is used upstream.
	return fmt.Sprintf(`<script>setTimeout(function(){ window.location.replace(%s); }, %d);</script>`, mustJSON(home), delayMS)
}

func atpSafeReturn(raw string, cfg Config) string {
	raw = strings.TrimSpace(raw)
	if dec, err := url.QueryUnescape(raw); err == nil {
		raw = dec
	}
	home := strings.TrimSpace(cfg.HomePath)
	if home == "" {
		home = "/"
	}
	if raw == "" {
		return home
	}
	// Collapse accidental // in paths (e.g. /en/dashboard//hw3o2g/searches)
	for strings.Contains(raw, "//") {
		raw = strings.ReplaceAll(raw, "//", "/")
	}
	if !strings.HasPrefix(raw, "/") {
		return home
	}
	path := strings.Split(raw, "?")[0]
	if atpIsLoginPath(path) {
		return home
	}
	return raw
}

func clearLocalCookieOverlay() {
	localCookieMu.Lock()
	localCookieOverlay = map[string]string{}
	localCookieMu.Unlock()
}

func atpShouldThrottleFailover(sessionToken string) bool {
	atpFailoverMu.Lock()
	defer atpFailoverMu.Unlock()
	now := time.Now()
	if last, ok := atpFailoverLast[sessionToken]; ok && now.Sub(last) < 6*time.Second {
		return true
	}
	atpFailoverLast[sessionToken] = now
	return false
}

func atpCookieFingerprint(cookieHeader string) string {
	// Fingerprint auth cookies only — full header order is non-deterministic (map iteration).
	sess := cookieNamed(cookieHeader, "_answerthepublic_session")
	cf := cookieNamed(cookieHeader, "cf_clearance")
	sum := sha256.Sum256([]byte(sess + "|" + cf))
	return hex.EncodeToString(sum[:8])
}

// atpCookieAlive checks whether the panel cookie is accepted by ATP's API.
// Cached ~25s per account+cookie fingerprint so page loads stay fast.
func atpCookieAlive(cfg Config, acc ToolAccount) bool {
	cookie := applyLocalCookieOverlay(parseCookieFromDB(acc.Cookie))
	if strings.TrimSpace(cookie) == "" {
		return false
	}
	fp := atpCookieFingerprint(cookie)
	key := fmt.Sprintf("%d:%s", acc.ID, fp)

	atpHealthMu.Lock()
	if ent, ok := atpHealth[key]; ok && time.Since(ent.checkedAt) < 25*time.Second {
		atpHealthMu.Unlock()
		return ent.ok
	}
	atpHealthMu.Unlock()

	ok := atpProbeUsersMe(cfg, acc, cookie)
	atpHealthMu.Lock()
	atpHealth[key] = atpHealthEntry{ok: ok, checkedAt: time.Now(), cookieFP: fp}
	atpHealthMu.Unlock()
	if !ok {
		log.Printf("[ATP-AUTH] cookie dead for account=%s (ID:%d) fp=%s — /api/v1/users/me not authorized", acc.Name, acc.ID, fp)
	} else {
		log.Printf("[ATP-AUTH] cookie OK for account=%s (ID:%d)", acc.Name, acc.ID)
	}
	return ok
}

func atpProbeUsersMe(cfg Config, acc ToolAccount, cookieHeader string) bool {
	req, err := http.NewRequest(http.MethodGet, "https://api.answerthepublic.com/api/v1/users/me", nil)
	if err != nil {
		return false
	}
	ua := strings.TrimSpace(acc.UserAgent)
	if ua == "" {
		ua = strings.TrimSpace(cfg.UserAgent)
	}
	if ua == "" {
		ua = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", "https://answerthepublic.com")
	req.Header.Set("Referer", "https://answerthepublic.com/en/dashboard/hw3o2g/searches")
	req.Header.Set("Cookie", cookieHeader)

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[ATP-AUTH] probe error: %v", err)
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8192))
	return resp.StatusCode == http.StatusOK
}

func serveATPInvalidCookie(w http.ResponseWriter, home, accountName string) {
	msg := "This account cookie is not logged in (API returns Unauthorized). Export a fresh cookie from a logged-in AnswerThePublic browser tab and paste it in the panel."
	if accountName != "" {
		msg = "Account <span class=\"brand\">" + html.EscapeString(accountName) + "</span> cookie is not logged in (API 401). Export a fresh cookie from a logged-in AnswerThePublic tab and update it in the panel."
	}
	writeLightCard(w, http.StatusOK, lightCard{
		Title:       "Cookie invalid",
		Heading:     "Cookie invalid",
		Message:     msg,
		Badge:       "Waiting for a working cookie",
		Footer:      "After saving a new cookie in the panel, this page retries automatically",
		Spin:        true,
		ExtraScript: atpWaitingRetryScript(home, 15000),
	})
}

// serveATPAccountSwitch runs when AnswerThePublic shows the login wall.
// Pins the next panel account (or reloads the only one) and returns a light card.
func serveATPAccountSwitch(w http.ResponseWriter, r *http.Request, cfg Config, sessionToken, currentUser string, activeAcc ToolAccount) {
	locParam := strings.TrimSpace(r.URL.Query().Get("location"))
	home := atpSafeReturn(locParam, cfg)
	// When login HTML is served on /dashboard/..., return there after a successful switch.
	if locParam == "" && !atpIsLoginPath(r.URL.Path) {
		cand := r.URL.Path
		if r.URL.RawQuery != "" {
			cand += "?" + r.URL.RawQuery
		}
		home = atpSafeReturn(cand, cfg)
	}
	heading := "Switching account"
	message := "This account signed out. Trying the next available account."
	next, nextName, err := panelSwitchAccount(cfg, sessionToken, "atp_login_wall")
	if err != nil {
		log.Printf("[FAILOVER] login wall user=%s account=%s no account: %v", currentUser, activeAcc.Name, err)
		writeLightCard(w, http.StatusOK, lightCard{
			Title:       "Waiting for an account",
			Heading:     "Waiting for an account",
			Message:     "No other account is ready yet. Checking again.",
			Badge:       "Trying one account at a time",
			Footer:      "This keeps trying until an account is available",
			Spin:        true,
			ExtraScript: atpWaitingRetryScript(home, 4000),
		})
		return
	}
	// Drop stale absorbed CF/session overlay so the freshly loaded panel cookie wins.
	clearLocalCookieOverlay()

	// Probe the assigned cookie before sending the user into a guest Sign-in shell.
	if !atpCookieAlive(cfg, next) {
		log.Printf("[FAILOVER] login wall user=%s account=%s cookie still unauthorized after reload", currentUser, next.Name)
		serveATPInvalidCookie(w, home, next.Name)
		return
	}

	// Same account with a working cookie — open the app.
	if next.ID == activeAcc.ID {
		log.Printf("[FAILOVER] login wall user=%s reloaded live cookie for %s (ID:%d) → %s", currentUser, next.Name, next.ID, home)
		if atpShouldThrottleFailover(sessionToken) {
			writeLightCard(w, http.StatusOK, lightCard{
				Title:       "Switching account",
				Heading:     "Switching account",
				Message:     "Cookie looks good. Opening the tool again.",
				Badge:       "Checking the account cookie",
				Footer:      "This page refreshes automatically",
				Spin:        true,
				ExtraScript: atpWaitingRetryScript(home, 1500),
			})
			return
		}
		writeLightCard(w, http.StatusOK, lightCard{
			Title:    "Switching account",
			Heading:  "Switching account",
			Message:  "Cookie looks good. Opening the tool again.",
			Badge:    "Checking the account cookie",
			Footer:   "This page refreshes automatically",
			Spin:     true,
			Redirect: home,
		})
		return
	}
	if nextName != "" {
		message = "Account logged out. Switching to " + nextName + "..."
	}
	log.Printf("[FAILOVER] login wall user=%s %s (ID:%d) -> %s (ID:%d) return=%s",
		currentUser, activeAcc.Name, activeAcc.ID, next.Name, next.ID, home)
	writeLightCard(w, http.StatusOK, lightCard{
		Title:    heading,
		Heading:  heading,
		Message:  html.EscapeString(message),
		Badge:    "Checking the next account",
		Footer:   "This page refreshes automatically",
		Spin:     true,
		Redirect: home,
	})
}

func mustJSON(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `"/"`
	}
	return string(b)
}

func atpLoginWatchScript(cfg Config) string {
	home := strings.TrimSpace(cfg.HomePath)
	if home == "" {
		home = "/"
	}
	homeJSON := mustJSON(home)
	// ATP hydrates auth in ~2–3s. Wait 5s before treating Sign-in/guest UI as a real logout,
	// then switch account only if still logged out.
	return fmt.Sprintf(`<script>
(function(){
  if (window.__atpLogoutWatch) return;
  window.__atpLogoutWatch = true;
  var HOME = %s;
  var READY_AT = Date.now() + 5000;
  var failHits = 0;
  function goFailover(back){
    if (window.__atpReloading) return;
    window.__atpReloading = true;
    if (!back || back.indexOf("/users/sign_in") !== -1 || back === "/login" || back.indexOf("/login?") === 0) {
      back = HOME;
    }
    location.replace("/en/users/sign_in?location=" + encodeURIComponent(back));
  }
  function bodyText(){
    try { return (document.body && (document.body.innerText || document.body.textContent)) || ""; } catch (e) { return ""; }
  }
  function loggedIn(){
    try {
      var stored = localStorage.getItem("auth-storage");
      if (stored && stored.indexOf('"user"') !== -1 && stored.indexOf('"id"') !== -1) return true;
    } catch (e) {}
    try {
      if (document.querySelector('[data-testid="nav-avatar-button"],[data-slot="dropdown-menu-trigger"]')) return true;
    } catch (e) {}
    var t = bodyText();
    if (t.indexOf("Sign in") === -1 && t.indexOf("Register for FREE") === -1 && t.length > 200) return true;
    return false;
  }
  function wall(){
    var path = location.pathname || "/";
    if (path.indexOf("/users/sign_in") !== -1 || path === "/login" || path === "/sign-in" || path === "/en/login") return true;
    var text = bodyText();
    if (text.length > 12000) text = text.slice(0, 12000);
    var welcome = text.indexOf("Wel" + "come back") !== -1;
    var signIn = text.indexOf("Sign in with " + "email") !== -1 || text.indexOf("Continue with " + "Google") !== -1 || text.indexOf("Sign in to your existing " + "account") !== -1;
    return welcome && signIn;
  }
  // Guest shell: top-right Sign in + register promo (common while ATP is still hydrating)
  function guestShell(){
    try {
      var text = bodyText();
      if (text.indexOf("Register for FREE") !== -1 && text.indexOf("Sign in") !== -1) return true;
    } catch (e) {}
    return false;
  }
  setInterval(function(){
    if (window.__atpReloading) return;
    if (Date.now() < READY_AT) return;
    if (loggedIn()) { failHits = 0; return; }
    if (wall() || guestShell()) {
      failHits++;
      // After the 5s grace, require one more tick (~1.5s) so a late hydrate can still win.
      if (failHits >= 2) goFailover(location.pathname + location.search);
      return;
    }
    failHits = 0;
  }, 1500);
})();
</script>`, homeJSON)
}
