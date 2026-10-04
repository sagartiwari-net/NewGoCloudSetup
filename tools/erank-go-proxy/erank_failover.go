package main

import (
	"html"
	"log"
	"net/http"
	"net/url"
	"strings"
)

func erankIsLoginPath(path string) bool {
	p := strings.Split(path, "?")[0]
	return p == "/login" || strings.HasPrefix(p, "/login/")
}

func erankLoginDocument(r *http.Request, path string) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if !erankIsLoginPath(path) {
		return false
	}
	accept := r.Header.Get("Accept")
	return accept == "" || strings.Contains(accept, "text/html") || strings.Contains(accept, "*/*")
}

func erankSafeReturn(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "/"
	}
	if dec, err := url.QueryUnescape(raw); err == nil {
		raw = dec
	}
	path := strings.Split(raw, "?")[0]
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || path == "/login" || strings.HasPrefix(path, "/login/") {
		return "/"
	}
	return raw
}

// serveErankAccountSwitch runs when eRank shows the login wall.
// It pins the next active panel account and sends the browser back into the tool.
func serveErankAccountSwitch(w http.ResponseWriter, r *http.Request, cfg Config, sessionToken, currentUser string, activeAcc ToolAccount) {
	home := erankSafeReturn(r.URL.Query().Get("location"))
	if home == "/" && cfg.HomePath != "" {
		home = cfg.HomePath
	}
	heading := "Switching account"
	message := "This account signed out. Trying the next available account."
	next, err := panelSwitchAccount(cfg, sessionToken, activeAcc.ID, activeAcc.Name, currentUser, "erank_login_wall")
	if err != nil {
		heading = "Waiting for an account"
		message = "No other account is ready yet. Checking again."
		log.Printf("[FAILOVER] login wall user=%s account=%s no other account: %v", currentUser, activeAcc.Name, err)
	} else {
		log.Printf("[FAILOVER] login wall user=%s %s (ID:%d) -> %s (ID:%d)", currentUser, activeAcc.Name, activeAcc.ID, next.Name, next.ID)
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

func erankLoginWatchScript() string {
	return `<script>
(function(){
  if (window.__erankLogoutWatch) return;
  window.__erankLogoutWatch = true;
  function wall(){
    var path = location.pathname || "/";
    if (path === "/login" || path.indexOf("/login/") === 0) return true;
    var text = "";
    try { text = (document.body && (document.body.innerText || document.body.textContent)) || ""; } catch (e) {}
    return text.indexOf("Welcome back") !== -1 && text.indexOf("Sign in with Google") !== -1;
  }
  setInterval(function(){
    if (!wall() || window.__erankReloading) return;
    window.__erankReloading = true;
    var back = location.pathname + location.search;
    if (!back || back === "/login" || back.indexOf("/login?") === 0 || back.indexOf("/login/") === 0) back = "/dashboard";
    location.replace("/login?location=" + encodeURIComponent(back));
  }, 1200);
})();
</script>`
}
