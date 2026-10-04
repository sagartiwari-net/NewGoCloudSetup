package main

import (
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

func h10IsLoginPath(path string) bool {
	p := strings.Split(path, "?")[0]
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		p = "/"
	}
	switch p {
	case "/user/signin", "/user/login", "/signin", "/login", "/sign-in", "/auth/login":
		return true
	}
	return strings.HasPrefix(p, "/user/signin/") ||
		strings.HasPrefix(p, "/user/login/") ||
		strings.HasPrefix(p, "/signin/") ||
		strings.HasPrefix(p, "/login/")
}

func h10LoginDocument(r *http.Request, path string) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if !h10IsLoginPath(path) {
		return false
	}
	accept := r.Header.Get("Accept")
	return accept == "" || strings.Contains(accept, "text/html") || strings.Contains(accept, "*/*")
}

func h10HTMLLooksLikeLogin(body []byte) bool {
	s := string(body)
	if strings.Contains(s, "Log In to Helium 10") {
		return true
	}
	if strings.Contains(s, "Log in with Google") && strings.Contains(s, "Forgot password") {
		return true
	}
	if strings.Contains(s, "Please enter your email") && strings.Contains(s, "Please enter your password") {
		return true
	}
	return false
}

var (
	h10SwitchCooldownMu sync.Mutex
	h10SwitchLast       = map[string]time.Time{}
)

func h10SwitchCooldownOK(sessionToken string) bool {
	h10SwitchCooldownMu.Lock()
	defer h10SwitchCooldownMu.Unlock()
	if last, ok := h10SwitchLast[sessionToken]; ok && time.Since(last) < 20*time.Second {
		return false
	}
	h10SwitchLast[sessionToken] = time.Now()
	return true
}

func h10SafeReturn(raw, home string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return home
	}
	if dec, err := url.QueryUnescape(raw); err == nil {
		raw = dec
	}
	path := strings.Split(raw, "?")[0]
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || h10IsLoginPath(path) {
		return home
	}
	return raw
}

// serveH10AccountSwitch runs when Helium 10 shows the login wall.
func serveH10AccountSwitch(w http.ResponseWriter, r *http.Request, cfg Config, sessionToken, currentUser string, activeAcc ToolAccount, reason string) {
	home := cfg.HomePath
	if home == "" {
		home = "/dashboard"
	}
	home = h10SafeReturn(r.URL.Query().Get("location"), home)
	if qReason := strings.TrimSpace(r.URL.Query().Get("reason")); qReason != "" {
		reason = qReason
	}
	heading := "Switching account"
	message := "This account signed out. Trying the next available account."
	if h10SwitchCooldownOK(sessionToken) {
		next, err := panelSwitchAccount(cfg, sessionToken, activeAcc.ID, activeAcc.Name, currentUser, reason)
		if err != nil {
			heading = "Waiting for an account"
			message = "No other account is ready yet. Checking again."
			log.Printf("[FAILOVER] login wall user=%s account=%s reason=%s no other account: %v", currentUser, activeAcc.Name, reason, err)
		} else {
			log.Printf("[FAILOVER] login wall user=%s %s (ID:%d) -> %s (ID:%d) reason=%s", currentUser, activeAcc.Name, activeAcc.ID, next.Name, next.ID, reason)
		}
	} else {
		heading = "Refreshing session"
		message = "Restoring your Helium 10 session. Please wait."
		log.Printf("[FAILOVER] cooldown user=%s account=%s reason=%s", currentUser, activeAcc.Name, reason)
	}
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

func h10LoginWatchScript(cfg Config) string {
	home := cfg.HomePath
	if home == "" {
		home = "/dashboard"
	}
	// NOTE: needle strings are split so body.textContent cannot match this script itself.
	// Device-lock CSS sets html{visibility:hidden}; innerText is then empty and a
	// textContent fallback would false-trigger failover ("h10_login_text") even when
	// the Helium cookie is valid.
	return fmt.Sprintf(`<script>
(function(){
  if (window.__h10LogoutWatch) return;
  window.__h10LogoutWatch = true;
  var HOME = %q;
  var switching = false;
  var NEEDLE_LOGIN = "Log In to Helium" + " 10";
  var NEEDLE_GOOGLE = "Log in with" + " Google";
  var NEEDLE_FORGOT = "Forgot" + " password";
  function pageVisible(){
    try {
      var st = getComputedStyle(document.documentElement);
      if (st && st.visibility === "hidden") return false;
    } catch (e) {}
    return !document.querySelector("style[data-tm-device]");
  }
  function wallText(){
    if (!pageVisible()) return false;
    var text = "";
    try {
      // innerText only — never textContent (includes our injected scripts).
      text = (document.body && document.body.innerText) || "";
    } catch (e) {}
    if (!text || text.length < 20) return false;
    if (text.indexOf(NEEDLE_LOGIN) !== -1) return true;
    if (text.indexOf(NEEDLE_GOOGLE) !== -1 && text.indexOf(NEEDLE_FORGOT) !== -1) return true;
    return false;
  }
  function wallPath(){
    var path = (location.pathname || "/").replace(/\/$/, "") || "/";
    return path === "/user/signin" || path === "/user/login" || path === "/signin" || path === "/login" ||
      path.indexOf("/user/signin/") === 0 || path.indexOf("/user/login/") === 0;
  }
  function switchAccount(reason){
    if (switching) return;
    switching = true;
    // Dedicated path — never /user/signin (that re-triggers wallPath forever).
    location.replace("/__h10_switch?location=" + encodeURIComponent(HOME) + "&reason=" + encodeURIComponent(reason || "h10_login_wall"));
  }
  setInterval(function(){
    if (wallPath() || wallText()) switchAccount(wallPath() ? "h10_login_path" : "h10_login_text");
  }, 1200);
})();
</script>`, home)
}
