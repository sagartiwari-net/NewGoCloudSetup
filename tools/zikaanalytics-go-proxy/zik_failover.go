package main

import (
	"encoding/base64"
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
	zikSwitchLast       = map[string]time.Time{}
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

func zikJWTExpired(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return false
	}
	payload := parts[1]
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}
	raw, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		raw, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return false
		}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(raw, &claims) != nil || claims.Exp == 0 {
		return false
	}
	return time.Now().Unix() >= claims.Exp
}

func zikAccountHasSessionCookie(cookieRaw string) bool {
	cookieStr, _ := parseCookiesAndStorage(cookieRaw)
	lower := strings.ToLower(cookieStr)
	return strings.Contains(lower, "sessiontoken=")
}

func zikAuthDiag(cookieRaw string) (bearerOK bool, sessionOK bool, expired bool) {
	bearer := zikAuthBearerFromAccount(cookieRaw)
	sessionOK = zikAccountHasSessionCookie(cookieRaw)
	if bearer == "" {
		return false, sessionOK, false
	}
	return true, sessionOK, zikJWTExpired(bearer)
}

func serveZikCookieExpired(w http.ResponseWriter, cfg Config, currentUser, accountName, reason string) {
	log.Printf("[FAILOVER] STOP login-loop user=%s account=%s reason=%s — update hybrid cookie (cookies+localStorage.access)", currentUser, accountName, reason)
	writeLightCard(w, http.StatusOK, lightCard{
		Title:   "Session expired",
		Heading: "Zik cookie expired",
		Message: "Update <b>Zik Analytics 1</b> in the panel with a fresh export that includes <b>cookies + localStorage</b> (the <code>access</code> JWT). Then open a <b>new</b> access link.",
		Badge:   "No auto-refresh — fix cookie in panel",
		Footer:  "Do not keep reloading this page",
		Spin:    false,
	})
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

	bearerOK, sessionOK, expired := zikAuthDiag(activeAcc.Cookie)
	log.Printf("[ZIK_AUTH] user=%s account=%s bearer=%v sessionCookie=%v jwtExpired=%v reason=%s",
		currentUser, activeAcc.Name, bearerOK, sessionOK, expired, reason)

	heading := "Switching account"
	message := "This account signed out. Trying the next available account."
	redirect := home
	badge := "Checking the next account"
	spin := true

	if !zikSwitchCooldownOK(sessionToken) {
		if !bearerOK || expired || !sessionOK {
			serveZikCookieExpired(w, cfg, currentUser, activeAcc.Name, reason+":cooldown")
			return
		}
		heading = "Refreshing session"
		message = "Restoring your Zik session. Please wait."
		log.Printf("[FAILOVER] cooldown user=%s account=%s reason=%s", currentUser, activeAcc.Name, reason)
	} else {
		next, err := panelSwitchAccount(cfg, sessionToken, activeAcc.ID, activeAcc.Name, currentUser, reason)
		if err != nil {
			serveZikCookieExpired(w, cfg, currentUser, activeAcc.Name, reason)
			return
		}
		log.Printf("[FAILOVER] login wall user=%s %s (ID:%d) -> %s (ID:%d) reason=%s", currentUser, activeAcc.Name, activeAcc.ID, next.Name, next.ID, reason)
	}

	writeLightCard(w, http.StatusOK, lightCard{
		Title:    heading,
		Heading:  heading,
		Message:  html.EscapeString(message),
		Badge:    badge,
		Footer:   "This page refreshes automatically",
		Spin:     spin,
		Redirect: redirect,
	})
}

func zikSoftenAPIUnauthorized(path string, status int) bool {
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		return false
	}
	p := strings.ToLower(path)
	if !strings.HasPrefix(p, "/extra-cdn-") {
		return false
	}
	if strings.Contains(p, "/login") || strings.Contains(p, "/signin") || strings.Contains(p, "/token") {
		return false
	}
	// Only soften known non-critical dashboard widgets (not every API).
	return strings.Contains(p, "bestseller")
}

func zikSoftenEmptyBody(path string) string {
	_ = path
	return "[]"
}

// zikPublicAssetPath — no ct_session required (SPA still needs these after hard refresh).
func zikPublicAssetPath(path string) bool {
	p := strings.ToLower(strings.Split(path, "?")[0])
	if p == "/manifest.json" || p == "/favicon.ico" || p == "/robots.txt" {
		return true
	}
	if strings.HasPrefix(p, "/static/") {
		return true
	}
	for _, ext := range []string{".js", ".css", ".map", ".woff", ".woff2", ".ttf", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".ico"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

// zikEssentialStorage keeps only keys the SPA needs for auth — dumping full
// Chrome localStorage (Intercom/Beamer/etc.) has broken page parse before.
func zikEssentialStorage(all map[string]interface{}) map[string]interface{} {
	if len(all) == 0 {
		return nil
	}
	out := make(map[string]interface{})
	for _, k := range []string{"access", "accessToken", "token", "defaultMarketplace", "sidebarExpand", "zk_geo_iso", "crDefaultTab"} {
		if v, ok := all[k]; ok && v != nil {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// zikLoginWatchScript — lightweight. Same-origin auth headers only.
// No history/Location monkeypatches (those blanked React). No watchdog overlay.
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
  function sameOrigin(url){
    try { return new URL(url, location.href).origin === location.origin; } catch (e) { return false; }
  }
  function wallText(){
    var text = "";
    try { text = (document.body && (document.body.innerText || document.body.textContent)) || ""; } catch (e) {}
    if (!text) return false;
    if (text.indexOf("Continue to ZIK") !== -1) return true;
    if (text.indexOf("Enter your details below to access your account") !== -1) return true;
    if (text.indexOf("Welcome!") !== -1 && text.indexOf("Sign in with Google") !== -1) return true;
    return false;
  }
  function switchAccount(reason){
    if (switching || !wallText()) return;
    switching = true;
    location.replace("/login?location=" + encodeURIComponent(HOME) + "&reason=" + encodeURIComponent(reason || "zik_login_wall"));
  }
  function patchInit(url, init){
    if (!sameOrigin(url)) return init;
    init = init ? Object.assign({}, init) : {};
    var headers = init.headers ? new Headers(init.headers) : new Headers();
    try {
      var token = localStorage.getItem("access") || "";
      if (token) headers.set("Authorization", "Bearer " + token);
      var proof = localStorage.getItem("tm_device_proof") || "";
      var fp = localStorage.getItem("tm_device_fp") || sessionStorage.getItem("tm_device_fp") || "";
      if (proof) headers.set("X-Device-Proof", proof);
      if (fp) headers.set("X-Device-Fp", fp);
    } catch (e) {}
    init.headers = headers;
    return init;
  }
  setInterval(function(){ if (wallText()) switchAccount("zik_login_text"); }, 5000);
  var fo = window.fetch;
  if (typeof fo === "function") {
    window.fetch = function(input, init){
      var url = typeof input === "string" ? input : (input && input.url) || "";
      try { init = patchInit(url, init); } catch (e) {}
      return fo(input, init);
    };
  }
  try {
    var xo = XMLHttpRequest.prototype.open;
    var xs = XMLHttpRequest.prototype.send;
    XMLHttpRequest.prototype.open = function(m, u){ this.__zikURL = u; return xo.apply(this, arguments); };
    XMLHttpRequest.prototype.send = function(body){
      try {
        if (sameOrigin(this.__zikURL || "")) {
          var token = localStorage.getItem("access") || "";
          if (token) this.setRequestHeader("Authorization", "Bearer " + token);
          var proof = localStorage.getItem("tm_device_proof") || "";
          var fp = localStorage.getItem("tm_device_fp") || sessionStorage.getItem("tm_device_fp") || "";
          if (proof) this.setRequestHeader("X-Device-Proof", proof);
          if (fp) this.setRequestHeader("X-Device-Fp", fp);
        }
      } catch (e) {}
      return xs.apply(this, arguments);
    };
  } catch (e) {}
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

func stripNamedCookies(cookieHeader string, names ...string) string {
	if cookieHeader == "" || len(names) == 0 {
		return cookieHeader
	}
	drop := make(map[string]bool, len(names))
	for _, n := range names {
		drop[strings.ToLower(strings.TrimSpace(n))] = true
	}
	parts := strings.Split(cookieHeader, ";")
	var kept []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		name := trimmed
		if eq := strings.Index(trimmed, "="); eq >= 0 {
			name = strings.TrimSpace(trimmed[:eq])
		}
		if drop[strings.ToLower(name)] {
			continue
		}
		kept = append(kept, trimmed)
	}
	return strings.Join(kept, "; ")
}

func zikRewriteLoginLocation(loc string, cfg Config) string {
	home := cfg.HomePath
	if home == "" {
		home = "/dashboard"
	}
	u := strings.TrimSpace(loc)
	if u == "" {
		return loc
	}
	lower := strings.ToLower(u)
	if strings.Contains(lower, "zikanalytics.com") && (strings.Contains(lower, "/login") || strings.Contains(lower, "/signin")) {
		return home
	}
	if strings.HasPrefix(u, "/") {
		pathOnly := strings.Split(u, "?")[0]
		if zikIsLoginPath(pathOnly) {
			return home
		}
	}
	return loc
}
