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

func claudeIsLoginPath(path string) bool {
	p := strings.Split(path, "?")[0]
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		p = "/"
	}
	switch p {
	case "/login", "/signin", "/sign-in", "/log-in":
		return true
	}
	return strings.HasPrefix(p, "/login/") || strings.HasPrefix(p, "/signin/")
}

func claudeLoginDocument(r *http.Request, path string) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if !claudeIsLoginPath(path) {
		return false
	}
	accept := r.Header.Get("Accept")
	return accept == "" || strings.Contains(accept, "text/html") || strings.Contains(accept, "*/*")
}

func claudeHTMLLooksLikeLogin(body string) bool {
	if strings.Contains(body, "from=logout") || strings.Contains(body, "reauth=1") {
		// query rarely appears in body; keep text markers
	}
	lower := strings.ToLower(body)
	if strings.Contains(body, "Welcome back") && (strings.Contains(body, "Continue with Google") || strings.Contains(lower, "continue with email")) {
		return true
	}
	if strings.Contains(body, "Log in to Claude") || strings.Contains(body, "Log in to Anthropic") {
		return true
	}
	if strings.Contains(lower, "continue with google") && strings.Contains(lower, "continue with email") && len(body) < 500000 {
		return true
	}
	return false
}

func claudeQueryLooksLikeLogout(r *http.Request) bool {
	q := r.URL.Query()
	if strings.EqualFold(q.Get("from"), "logout") {
		return true
	}
	if q.Get("reauth") == "1" || strings.EqualFold(q.Get("reauth"), "true") {
		return true
	}
	return false
}

var (
	claudeSwitchCooldownMu sync.Mutex
	claudeSwitchLast       = map[string]time.Time{}
)

func claudeSwitchCooldownOK(sessionToken string) bool {
	claudeSwitchCooldownMu.Lock()
	defer claudeSwitchCooldownMu.Unlock()
	if last, ok := claudeSwitchLast[sessionToken]; ok && time.Since(last) < 20*time.Second {
		return false
	}
	claudeSwitchLast[sessionToken] = time.Now()
	return true
}

func claudeSafeReturn(raw, home string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return home
	}
	if dec, err := url.QueryUnescape(raw); err == nil {
		raw = dec
	}
	path := strings.Split(raw, "?")[0]
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || claudeIsLoginPath(path) {
		return home
	}
	return raw
}

func claudeHomePath(cfg Config) string {
	if strings.TrimSpace(cfg.HomePath) != "" {
		return cfg.HomePath
	}
	return "/new"
}

// serveClaudeAccountSwitch runs when Claude shows the login / logout wall.
func serveClaudeAccountSwitch(w http.ResponseWriter, r *http.Request, cfg Config, sessionToken, currentUser string, activeAcc ToolAccount, reason string) {
	home := claudeSafeReturn(r.URL.Query().Get("location"), claudeHomePath(cfg))
	if loc := strings.TrimSpace(r.URL.Query().Get("returnTo")); loc != "" && home == claudeHomePath(cfg) {
		home = claudeSafeReturn(loc, claudeHomePath(cfg))
	}
	if qReason := strings.TrimSpace(r.URL.Query().Get("reason")); qReason != "" {
		reason = qReason
	}
	// Claude login pages wipe localStorage. Reset server binding and mint a fresh
	// proof before returning home, otherwise /new looks like a copied cookie jar.
	clearPanelDeviceBinding(sessionToken)
	heading := "Switching account"
	message := "This account signed out. Trying the next available account."
	if claudeSwitchCooldownOK(sessionToken) {
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
		message = "Restoring your Claude session. Please wait."
		log.Printf("[FAILOVER] cooldown user=%s account=%s reason=%s", currentUser, activeAcc.Name, reason)
	}
	writeLightCard(w, http.StatusOK, lightCard{
		Title:   heading,
		Heading: heading,
		Message: html.EscapeString(message),
		Badge:   "Checking the next account",
		Footer:  "This page refreshes automatically",
		Spin:    true,
		ExtraScript: `<script>` + deviceSharedJS() + `
(function(){
  var home = ` + fmt.Sprintf("%q", home) + `;
  function go(){ window.location.replace(home); }
  tmEnsureProof().then(function(proof){
    return tmFingerprint().then(function(fp){
      try { sessionStorage.setItem("tm_device_fp", fp); localStorage.setItem("tm_device_fp", fp); } catch(e) {}
      return tmStore(fp, proof).then(function(){
        return fetch("/api/device-bind", {
          method: "POST",
          credentials: "same-origin",
          headers: { "X-Device-Fp": fp, "X-Device-Proof": proof }
        });
      });
    });
  }).then(function(res){
    if (res && res.ok) { setTimeout(go, 400); return; }
    setTimeout(go, 800);
  }).catch(function(){ setTimeout(go, 800); });
})();
</script>`,
	})
}

func claudeFailoverBounceHTML(cfg Config, reason string) string {
	home := claudeHomePath(cfg)
	return fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="UTF-8"><title>Switching account</title>
<style>body{min-height:100vh;display:flex;align-items:center;justify-content:center;margin:0;background:#eef3f8;color:#0f172a;font-family:system-ui,-apple-system,Segoe UI,sans-serif}
.card{width:min(440px,92vw);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px;text-align:center}
.ring{width:78px;height:78px;margin:0 auto 22px;border-radius:50%%;background:conic-gradient(#3b82f6 0 70deg,#e7eef8 70deg 360deg);animation:spin .9s linear infinite;display:grid;place-items:center}
.lock{width:64px;height:64px;border-radius:50%%;background:#fff;display:grid;place-items:center;font-size:26px}
h1{font-size:28px;font-weight:800;letter-spacing:-.03em;margin:0 0 12px}
p{color:#64748b;font-size:15px;line-height:1.55;margin:0}
@keyframes spin{to{transform:rotate(360deg)}}</style></head><body><div class="card"><div class="ring"><div class="lock">🔒</div></div>
<h1>Switching account</h1><p>This account signed out. Trying the next available account.</p></div>
<script>location.replace("/login?location="+encodeURIComponent(%q)+"&reason="+encodeURIComponent(%q));</script>
</body></html>`, home, reason)
}

func claudeLoginWatchScript(cfg Config) string {
	home := claudeHomePath(cfg)
	return fmt.Sprintf(`<script>
(function(){
  if (window.__claudeLogoutWatch) return;
  window.__claudeLogoutWatch = true;
  var HOME = %q;
  var switching = false;
  function wallText(){
    var text = "";
    try { text = (document.body && (document.body.innerText || document.body.textContent)) || ""; } catch (e) {}
    if (!text) return false;
    if (text.indexOf("Welcome back") !== -1 && (text.indexOf("Continue with Google") !== -1 || text.indexOf("Continue with email") !== -1)) return true;
    if (text.indexOf("Log in to Claude") !== -1 || text.indexOf("Log in to Anthropic") !== -1) return true;
    return false;
  }
  function wallPath(){
    var path = (location.pathname || "/").replace(/\/$/, "") || "/";
    return path === "/login" || path === "/signin" || path.indexOf("/login/") === 0;
  }
  function logoutQuery(){
    try {
      var q = new URLSearchParams(location.search || "");
      return q.get("from") === "logout" || q.get("reauth") === "1";
    } catch (e) { return false; }
  }
  function switchAccount(reason){
    if (switching) return;
    switching = true;
    location.replace("/login?location=" + encodeURIComponent(HOME) + "&reason=" + encodeURIComponent(reason || "claude_login_wall"));
  }
  setInterval(function(){
    if (wallPath() || logoutQuery() || wallText()) switchAccount(wallPath() || logoutQuery() ? "claude_login_path" : "claude_login_text");
  }, 1200);
})();
</script>`, home)
}
