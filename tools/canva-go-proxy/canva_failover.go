package main

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

var canvaStripScriptsRe = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>|<style[^>]*>.*?</style>`)

var (
	canvaFailoverMu   sync.Mutex
	canvaFailoverLast = map[string]time.Time{}
)

func canvaIsLoginPath(path string) bool {
	p := strings.Split(path, "?")[0]
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		p = "/"
	}
	switch p {
	case "/login", "/_login", "/signin", "/sign-in", "/en/login":
		return true
	}
	return strings.HasPrefix(p, "/login/") || strings.HasPrefix(p, "/_login/")
}

func canvaLoginDocument(r *http.Request, path string) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if !canvaIsLoginPath(path) {
		return false
	}
	accept := r.Header.Get("Accept")
	return accept == "" || strings.Contains(accept, "text/html") || strings.Contains(accept, "*/*")
}

func canvaSafeReturn(raw string, cfg Config) string {
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
	for strings.Contains(raw, "//") {
		raw = strings.ReplaceAll(raw, "//", "/")
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return home
	}
	path := strings.Split(raw, "?")[0]
	if canvaIsLoginPath(path) {
		return home
	}
	return raw
}

func canvaBodyLooksLikeLogin(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	s := string(body)
	if len(s) > 120000 {
		s = s[:120000]
	}
	s = canvaStripScriptsRe.ReplaceAllString(s, " ")
	lower := strings.ToLower(s)
	if strings.Contains(lower, "log in to canva") || strings.Contains(lower, "log into canva") {
		return true
	}
	hasLogIn := strings.Contains(s, ">Log in<") || strings.Contains(s, ">Log In<") ||
		strings.Contains(lower, ">log in</") || strings.Contains(s, "\"Log in\"")
	hasSignUp := strings.Contains(lower, "sign up") || strings.Contains(lower, "signup")
	hasContinue := strings.Contains(lower, "continue with google") ||
		strings.Contains(lower, "continue with email") ||
		strings.Contains(lower, "continue with facebook")
	return hasLogIn && (hasContinue || (hasSignUp && strings.Contains(lower, "canva")))
}

func canvaWaitingRetryScript(home string, delayMS int) string {
	if delayMS <= 0 {
		delayMS = 2500
	}
	return fmt.Sprintf(`<script>setTimeout(function(){ window.location.replace(%s); }, %d);</script>`, mustJSONString(home), delayMS)
}

func mustJSONString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `"/"`
	}
	return string(b)
}

func canvaShouldThrottleFailover(sessionToken string) bool {
	canvaFailoverMu.Lock()
	defer canvaFailoverMu.Unlock()
	last, ok := canvaFailoverLast[sessionToken]
	if ok && time.Since(last) < 4*time.Second {
		return true
	}
	canvaFailoverLast[sessionToken] = time.Now()
	return false
}

// serveCanvaAccountSwitch runs when Canva shows the logged-out / Log in wall.
func serveCanvaAccountSwitch(w http.ResponseWriter, r *http.Request, cfg Config, sessionToken, currentUser string, activeAcc ToolAccount) {
	locParam := strings.TrimSpace(r.URL.Query().Get("location"))
	home := canvaSafeReturn(locParam, cfg)
	if locParam == "" && !canvaIsLoginPath(r.URL.Path) {
		cand := r.URL.Path
		if r.URL.RawQuery != "" {
			cand += "?" + r.URL.RawQuery
		}
		home = canvaSafeReturn(cand, cfg)
	}
	heading := "Switching account"
	message := "This account signed out. Trying the next available account."
	next, nextName, err := panelSwitchAccount(cfg, sessionToken, "canva_login_wall")
	if err != nil {
		log.Printf("[FAILOVER] login wall user=%s account=%s no account: %v", currentUser, activeAcc.Name, err)
		writeLightCard(w, http.StatusOK, lightCard{
			Title:       "Waiting for an account",
			Heading:     "Waiting for an account",
			Message:     "No other account is ready yet. Checking again.",
			Badge:       "Trying one account at a time",
			Footer:      "This keeps trying until an account is available",
			Spin:        true,
			ExtraScript: canvaWaitingRetryScript(home, 4000),
		})
		return
	}
	clearLocalCookieOverlay()

	if next.ID == activeAcc.ID {
		log.Printf("[FAILOVER] login wall user=%s reloaded cookie for %s (ID:%d) → %s", currentUser, next.Name, next.ID, home)
		if canvaShouldThrottleFailover(sessionToken) {
			writeLightCard(w, http.StatusOK, lightCard{
				Title:       "Switching account",
				Heading:     "Switching account",
				Message:     "Reloading the account cookie. Opening the tool again.",
				Badge:       "Checking the account cookie",
				Footer:      "This page refreshes automatically",
				Spin:        true,
				ExtraScript: canvaWaitingRetryScript(home, 2500),
			})
			return
		}
		writeLightCard(w, http.StatusOK, lightCard{
			Title:    "Switching account",
			Heading:  "Switching account",
			Message:  "Reloading the account cookie. Opening the tool again.",
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

func canvaLoginWatchScript(cfg Config) string {
	home := strings.TrimSpace(cfg.HomePath)
	if home == "" {
		home = "/"
	}
	homeJSON := mustJSONString(home)
	return fmt.Sprintf(`<script>
(function(){
  if (window.__canvaLogoutWatch) return;
  window.__canvaLogoutWatch = true;
  var HOME = %s;
  var READY_AT = Date.now() + 5000;
  var failHits = 0;
  function goFailover(back){
    if (window.__canvaReloading) return;
    window.__canvaReloading = true;
    if (!back || back.indexOf("/_login") === 0 || back.indexOf("/login") === 0 || back.indexOf("/signin") === 0) {
      back = HOME;
    }
    location.replace("/_login?location=" + encodeURIComponent(back));
  }
  function norm(s){ return String(s||'').replace(/\s+/g,' ').trim().toLowerCase(); }
  function visible(el){
    if (!el) return false;
    try {
      var st = window.getComputedStyle(el);
      if (!st || st.display === 'none' || st.visibility === 'hidden' || st.opacity === '0') return false;
      var r = el.getBoundingClientRect();
      return r.width > 0 && r.height > 0;
    } catch (e) { return false; }
  }
  function loggedIn(){
    try {
      if (document.querySelector('[data-tm-user-set], [data-tm-panel-user], .dIH_FQ[data-tm-user-set]')) return true;
    } catch (e) {}
    try {
      var names = document.querySelectorAll('.dIH_FQ, [class*="dIH_FQ"]');
      for (var i = 0; i < names.length; i++) {
        var t = norm(names[i].textContent);
        if (t && t !== 'log in' && t.indexOf('log in') === -1 && visible(names[i])) return true;
      }
    } catch (e) {}
    return false;
  }
  function wall(){
    var path = location.pathname || "/";
    if (path === "/login" || path.indexOf("/login/") === 0 || path === "/_login" || path.indexOf("/_login/") === 0 || path === "/signin" || path === "/sign-in") return true;
    try {
      var nodes = document.querySelectorAll('a,button,span,[role="button"]');
      for (var i = 0; i < nodes.length; i++) {
        var el = nodes[i];
        if (norm(el.textContent) !== 'log in') continue;
        if (!visible(el)) continue;
        // Prefer header/nav Log in CTAs, not buried footer links.
        if (el.closest('header,nav,[role="banner"],[class*="Nav"],[class*="Header"]') || el.getBoundingClientRect().top < 120) {
          return true;
        }
      }
    } catch (e) {}
    return false;
  }
  setInterval(function(){
    if (window.__canvaReloading) return;
    if (Date.now() < READY_AT) return;
    if (loggedIn()) { failHits = 0; return; }
    if (wall()) {
      failHits++;
      if (failHits >= 2) goFailover(location.pathname + location.search);
      return;
    }
    failHits = 0;
  }, 1500);
})();
</script>`, homeJSON)
}
