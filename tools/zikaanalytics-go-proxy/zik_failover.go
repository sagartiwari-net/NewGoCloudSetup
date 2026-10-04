package main

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

func zikIsLoginPath(path string) bool {
	p := strings.Split(path, "?")[0]
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		p = "/"
	}
	switch p {
	case "/login", "/signin", "/sign-in", "/auth/login", "/account/login":
		return true
	}
	return strings.HasPrefix(p, "/login/") || strings.HasPrefix(p, "/signin/")
}

func zikLoginDocument(r *http.Request, path string) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if !zikIsLoginPath(path) {
		return false
	}
	accept := r.Header.Get("Accept")
	return accept == "" || strings.Contains(accept, "text/html") || strings.Contains(accept, "*/*")
}

func zikHTMLLooksLikeLogin(body []byte) bool {
	s := string(body)
	if strings.Contains(s, "Continue to ZIK") {
		return true
	}
	if strings.Contains(s, "Enter your details below to access your account") {
		return true
	}
	if strings.Contains(s, "Welcome!") && strings.Contains(s, "Sign in with Google") {
		return true
	}
	return false
}

func zikAPILooksLoggedOut(path string, status int, body []byte) bool {
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		return false
	}
	p := strings.ToLower(strings.Split(path, "?")[0])
	if !(strings.Contains(p, "/api/") ||
		strings.Contains(p, "/user") ||
		strings.Contains(p, "/account") ||
		strings.Contains(p, "/auth") ||
		strings.Contains(p, "/subscriber") ||
		strings.Contains(p, "/session") ||
		strings.Contains(p, "/me")) {
		return false
	}
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, "unauthor") ||
		strings.Contains(lower, "not authenticated") ||
		strings.Contains(lower, "invalid token") ||
		strings.Contains(lower, "token expired") ||
		strings.Contains(lower, "login") ||
		len(body) < 8
}

var (
	zikSwitchCooldownMu sync.Mutex
	zikSwitchLast   = map[string]time.Time{}
)

func zikSwitchCooldownOK(sessionToken string) bool {
	zikSwitchCooldownMu.Lock()
	defer zikSwitchCooldownMu.Unlock()
	if last, ok := zikSwitchLast[sessionToken]; ok && time.Since(last) < 20*time.Second {
		return false
	}
	zikSwitchLast[sessionToken] = time.Now()
	return true
}

func serveZikAccountSwitch(w http.ResponseWriter, r *http.Request, cfg Config, sessionToken, currentUser string, activeAcc ToolAccount, reason string) {
	home := cfg.HomePath
	if home == "" {
		home = "/dashboard"
	}
	if loc := strings.TrimSpace(r.URL.Query().Get("location")); strings.HasPrefix(loc, "/") && !strings.HasPrefix(loc, "//") {
		p := strings.Split(loc, "?")[0]
		if p != "/login" && !strings.HasPrefix(p, "/login/") && p != "/signin" {
			home = loc
		}
	}
	if qReason := strings.TrimSpace(r.URL.Query().Get("reason")); qReason != "" {
		reason = qReason
	}
	heading := "Switching account"
	message := "This account signed out. Trying the next available account."
	if zikSwitchCooldownOK(sessionToken) {
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
		message = "Restoring your Zik session. Please wait."
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

func zikLoginWatchScript(cfg Config) string {
	home := cfg.HomePath
	if home == "" {
		home = "/dashboard"
	}
	return fmt.Sprintf(`<script>
(function(){
  if (window.__zikLogoutWatch) return;
  window.__zikLogoutWatch = true;
  var HOME = %q;
  var switching = false;
  var apiFail = 0;
  function wallText(){
    var text = "";
    try { text = (document.body && (document.body.innerText || document.body.textContent)) || ""; } catch (e) {}
    if (!text) return false;
    if (text.indexOf("Continue to ZIK") !== -1) return true;
    if (text.indexOf("Enter your details below to access your account") !== -1) return true;
    if (text.indexOf("Welcome!") !== -1 && text.indexOf("Sign in with Google") !== -1) return true;
    return false;
  }
  function wallPath(){
    var path = (location.pathname || "/").replace(/\/$/, "") || "/";
    return path === "/login" || path === "/signin" || path === "/sign-in" ||
      path.indexOf("/login/") === 0 || path.indexOf("/signin/") === 0;
  }
  function switchAccount(reason){
    if (switching || wallPath()) return;
    switching = true;
    location.replace("/login?location=" + encodeURIComponent(HOME) + "&reason=" + encodeURIComponent(reason || "zik_login_wall"));
  }
  setInterval(function(){
    if (wallPath() || wallText()) switchAccount("zik_login_text");
  }, 1500);
  var fo = window.fetch;
  if (typeof fo === "function") {
    window.fetch = function(input, init){
      return fo(input, init).then(function(res){
        try {
          var url = "";
          if (typeof input === "string") url = input;
          else if (input && input.url) url = input.url;
          var path = (url || "").split("?")[0].toLowerCase();
          var authPath = path.indexOf("/user/") !== -1 || path.indexOf("/dashboard/") !== -1 ||
            path.indexOf("/account") !== -1 || path.indexOf("/auth") !== -1 ||
            path.indexOf("/session") !== -1 || path.indexOf("/subscriber") !== -1;
          if ((res.status === 401 || res.status === 403) && authPath) {
            apiFail += 1;
            // Only rotate after repeated auth failures AND the login wall is visible.
            if (apiFail >= 3 && wallText()) switchAccount("zik_api_" + res.status);
          } else if (res.ok && authPath) {
            apiFail = 0;
          }
        } catch (e) {}
        return res;
      });
    };
  }
})();
</script>`, home)
}

func zikAuthBearerFromAccount(cookieRaw string) string {
	_, ls := parseCookiesAndStorage(cookieRaw)
	if ls == nil {
		return ""
	}
	if v, ok := ls["access"].(string); ok {
		return strings.TrimSpace(v)
	}
	// Some exports store nested JSON strings.
	if raw, ok := ls["access"]; ok {
		b, err := json.Marshal(raw)
		if err == nil {
			var s string
			if json.Unmarshal(b, &s) == nil {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}
