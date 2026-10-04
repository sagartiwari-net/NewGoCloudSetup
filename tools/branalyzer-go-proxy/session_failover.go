package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const branAuth0Host = "dev-v3arhhay.us.auth0.com"

var (
	branAuthProbeMu sync.Mutex
	branAuthProbe   = map[string]struct {
		ok        bool
		why       string
		checkedAt time.Time
	}{}
)

type branAuth0Cache struct {
	ClientID     string `json:"client_id"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
	ExpiresAt    int64  `json:"expiresAt"`
	Audience     string `json:"audience"`
	DecodedToken struct {
		Claims map[string]interface{} `json:"claims"`
	} `json:"decodedToken"`
}

func branAppHome(cfg Config) string {
	if p := strings.TrimSpace(cfg.HomePath); p != "" && p != "/" {
		return p
	}
	return "/home"
}

func branDocumentNeedsSession(path string) bool {
	switch strings.TrimSuffix(path, "/") {
	case "", "/", "/index.html", "/home", "/dashboard":
		return true
	}
	return false
}

func branAuth0CacheFromAccount(acc ToolAccount) (branAuth0Cache, bool) {
	raw := localStorageJSONFromRaw([]byte(acc.Cookie))
	if raw == "" {
		return branAuth0Cache{}, false
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return branAuth0Cache{}, false
	}
	for k, v := range m {
		if !strings.Contains(strings.ToLower(k), "auth0spajs") {
			continue
		}
		var wrap struct {
			Body branAuth0Cache `json:"body"`
		}
		if err := json.Unmarshal([]byte(v), &wrap); err == nil && strings.TrimSpace(wrap.Body.AccessToken) != "" {
			return wrap.Body, true
		}
		var direct branAuth0Cache
		if err := json.Unmarshal([]byte(v), &direct); err == nil && strings.TrimSpace(direct.AccessToken) != "" {
			return direct, true
		}
	}
	return branAuth0Cache{}, false
}

func jwtExpUnix(token string) (int64, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return 0, false
	}
	payload := parts[1]
	if mod := len(payload) % 4; mod != 0 {
		payload += strings.Repeat("=", 4-mod)
	}
	b, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		b, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return 0, false
		}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(b, &claims); err != nil || claims.Exp <= 0 {
		return 0, false
	}
	return claims.Exp, true
}

// branAuth0AccessExpired uses SPA access expiry — NOT id_token.
// Auth0 id_tokens are short-lived (~1h) while access/refresh keep the app usable.
func branAuth0AccessExpired(cache branAuth0Cache) bool {
	now := time.Now().Unix()
	if cache.ExpiresAt > 0 {
		return cache.ExpiresAt <= now+30
	}
	if exp, ok := jwtExpUnix(cache.AccessToken); ok {
		return exp <= now+30
	}
	if cache.ExpiresIn > 0 {
		if exp, ok := jwtExpUnix(cache.IDToken); ok {
			// approx: id iat/exp window is useless; prefer presence of refresh below
			_ = exp
		}
	}
	return false
}

func branAccountAuthAlive(cfg Config, acc ToolAccount) (bool, string) {
	cache, ok := branAuth0CacheFromAccount(acc)
	if !ok || strings.TrimSpace(cache.AccessToken) == "" {
		return false, "missing Auth0 localStorage (GoAuto must include storage/@@auth0spajs@@)"
	}
	key := fmt.Sprintf("%d:%s:%s", acc.ID, cache.AccessToken, strings.TrimSpace(acc.Proxy))
	branAuthProbeMu.Lock()
	if prev, hit := branAuthProbe[key]; hit && time.Since(prev.checkedAt) < 45*time.Second {
		branAuthProbeMu.Unlock()
		return prev.ok, prev.why
	}
	branAuthProbeMu.Unlock()

	alive, why := branProbeAuth0(cfg, acc, cache)
	branAuthProbeMu.Lock()
	branAuthProbe[key] = struct {
		ok        bool
		why       string
		checkedAt time.Time
	}{ok: alive, why: why, checkedAt: time.Now()}
	branAuthProbeMu.Unlock()
	return alive, why
}

func branProbeAuth0(cfg Config, acc ToolAccount, cache branAuth0Cache) (bool, string) {
	// Fresh panel paste: if Auth0 SPA cache is present, serve the app.
	// Server-side Auth0 userinfo often fails on JWE access tokens / datacenter IP
	// even when the browser session is fine — that was false "Contact Admin".
	if !branAuth0AccessExpired(cache) {
		return true, "ok (auth0 cache present)"
	}
	if strings.TrimSpace(cache.RefreshToken) != "" && strings.TrimSpace(cache.ClientID) != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		form := url.Values{}
		form.Set("grant_type", "refresh_token")
		form.Set("client_id", cache.ClientID)
		form.Set("refresh_token", cache.RefreshToken)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+branAuth0Host+"/oauth/token", strings.NewReader(form.Encode()))
		if err != nil {
			// Ambiguous — let the browser try.
			return true, "ok (refresh probe skipped)"
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", firstNonEmpty(acc.UserAgent, cfg.UserAgent, "Mozilla/5.0"))
		resp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("[AUTH0] refresh probe network error (allowing): %v", err)
			return true, "ok (refresh unreachable)"
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK && bytes.Contains(body, []byte(`"access_token"`)) {
			return true, "ok (refreshed)"
		}
		low := strings.ToLower(string(body))
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden ||
			strings.Contains(low, "invalid_grant") || strings.Contains(low, "invalid_token") {
			return false, "Auth0 refresh rejected — cookie/session logged out"
		}
		// Non-auth errors (rate limit, 5xx): do not kill a fresh paste.
		log.Printf("[AUTH0] refresh probe status=%d (allowing)", resp.StatusCode)
		return true, fmt.Sprintf("ok (refresh status %d)", resp.StatusCode)
	}
	return false, "Auth0 access expired and no refresh_token"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func branInvalidateAuthProbe(acc ToolAccount) {
	branAuthProbeMu.Lock()
	defer branAuthProbeMu.Unlock()
	prefix := fmt.Sprintf("%d:", acc.ID)
	for k := range branAuthProbe {
		if strings.HasPrefix(k, prefix) {
			delete(branAuthProbe, k)
		}
	}
}

func renderBranContactAdminPage(w http.ResponseWriter, cfg Config) {
	_ = cfg
	writeLightCard(w, http.StatusServiceUnavailable, lightCard{
		Title:   "Account unavailable",
		Heading: "Account unavailable",
		Message: "Contact to Admin/Provider to fix it ASAP",
		Footer:  "Branalyzer session logged out — cookie update needed",
	})
}

// serveBranLogoutFailover: switch to another panel account, else contact-admin (no blank SPA).
func serveBranLogoutFailover(w http.ResponseWriter, r *http.Request, cfg Config, sessionToken, currentUser string, activeAcc ToolAccount, reason string) {
	returnPath := branAppHome(cfg)

	if reloaded, err := panelReloadAccount(cfg, sessionToken); err == nil {
		branInvalidateAuthProbe(reloaded)
		if ok, _ := branAccountAuthAlive(cfg, reloaded); ok {
			log.Printf("[FAILOVER] reloaded cookie OK user=%s account=%s → %s", currentUser, reloaded.Name, returnPath)
			http.Redirect(w, r, returnPath, http.StatusFound)
			return
		}
		activeAcc = reloaded
	}

	next, nextName, err := panelSwitchToOtherAccount(cfg, sessionToken, reason)
	if err == nil && next.ID != activeAcc.ID {
		branInvalidateAuthProbe(next)
		if ok, _ := branAccountAuthAlive(cfg, next); ok {
			log.Printf("[FAILOVER] switched user=%s %s -> %s reason=%s", currentUser, activeAcc.Name, nextName, reason)
			renderPanelAccountSwitchPage(w, cfg, nextName, returnPath)
			return
		}
		log.Printf("[FAILOVER] next account also dead user=%s next=%s reason=%s", currentUser, nextName, reason)
	} else {
		log.Printf("[FAILOVER] no other account user=%s account=%s reason=%s err=%v", currentUser, activeAcc.Name, reason, err)
	}
	renderBranContactAdminPage(w, cfg)
}

func branSessionCheckHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	if !usesPanelAccountMode(cfg) {
		http.Redirect(w, r, branAppHome(cfg), http.StatusFound)
		return
	}
	token, _, ok := sessionFromRequest(r)
	if !ok {
		renderAccessDeniedPage(w, cfg)
		return
	}
	name, err := panelSessionUsername(r)
	if err != nil {
		renderAccessDeniedPage(w, cfg)
		return
	}
	acc, err := loadPanelSessionAccount(cfg, token)
	if err != nil {
		renderBranContactAdminPage(w, cfg)
		return
	}
	if alive, why := branAccountAuthAlive(cfg, acc); alive {
		log.Printf("[FAILOVER] session-check OK user=%s account=%s", name, acc.Name)
		http.Redirect(w, r, branAppHome(cfg), http.StatusFound)
		return
	} else {
		log.Printf("[FAILOVER] session-check dead user=%s account=%s reason=%s", name, acc.Name, why)
		serveBranLogoutFailover(w, r, cfg, token, name, acc, "session_check")
	}
}

func branAuthWatchScript() string {
	return `<script data-tm-bran-auth="1">
(function(){
  if (window.__tmBranAuthWatch) return;
  window.__tmBranAuthWatch = true;
  var go = false;
  function bounce(){
    if (go) return;
    go = true;
    try { location.replace('/__tm_session_check'); } catch (e) {}
  }
  function badURL(u){
    var s = String(u || '');
    if (s.indexOf('/authorize') !== -1 && s.indexOf('auth0.com') !== -1) return true;
    if (s.indexOf('/oauth/token') !== -1 && s.indexOf('auth0.com') !== -1) return true;
    if (s.indexOf('branalyzerazure') !== -1) return true;
    if (s.indexOf('/extra-cdn-') !== -1) return true;
    return false;
  }
  var ofetch = window.fetch;
  if (ofetch) {
    window.fetch = function(){
      var args = arguments;
      return ofetch.apply(this, args).then(function(res){
        try {
          var u = '';
          if (typeof args[0] === 'string') u = args[0];
          else if (args[0] && args[0].url) u = args[0].url;
          if (badURL(u) && (res.status === 401 || res.status === 403)) bounce();
        } catch (e) {}
        return res;
      });
    };
  }
  // Only after inject had time to run — missing Auth0 dump means bad paste.
  setTimeout(function(){
    try {
      var has = false;
      for (var i = 0; i < localStorage.length; i++) {
        var k = localStorage.key(i) || '';
        if (k.indexOf('@@auth0spajs@@') === 0) { has = true; break; }
      }
      if (!has) bounce();
    } catch (e) {}
  }, 6000);
})();
</script>`
}
