package main

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
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

func branNormalizeExp(exp int64) int64 {
	// auth0-spa-js uses unix seconds; some exports store milliseconds.
	if exp > 1_000_000_000_000 {
		return exp / 1000
	}
	return exp
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
		// Auth0 SPA cache: expiresAt lives on the WRAPPER, tokens in body.
		var wrap struct {
			Body      branAuth0Cache `json:"body"`
			ExpiresAt int64          `json:"expiresAt"`
		}
		if err := json.Unmarshal([]byte(v), &wrap); err == nil && strings.TrimSpace(wrap.Body.AccessToken) != "" {
			cache := wrap.Body
			if cache.ExpiresAt <= 0 && wrap.ExpiresAt > 0 {
				cache.ExpiresAt = wrap.ExpiresAt
			}
			cache.ExpiresAt = branNormalizeExp(cache.ExpiresAt)
			return cache, true
		}
		var direct branAuth0Cache
		if err := json.Unmarshal([]byte(v), &direct); err == nil && strings.TrimSpace(direct.AccessToken) != "" {
			direct.ExpiresAt = branNormalizeExp(direct.ExpiresAt)
			return direct, true
		}
	}
	return branAuth0Cache{}, false
}

func jwtExpUnix(token string) (int64, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		// JWE access tokens (5 parts) have no readable exp — ignore.
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
	// JWE access + missing wrapper expiresAt: not enough signal to kill the paste.
	return false
}

// branAccountAuthAlive only validates the GoAuto Auth0 dump shape.
// Never call Auth0 from the VPS — userinfo/refresh from Hetzner false-marked
// fresh pastes as dead (JWE access tokens / datacenter IP) → Contact Admin.
// Real logout is detected in-browser (Auth0 /authorize) via branAuthWatchScript.
func branAccountAuthAlive(cfg Config, acc ToolAccount) (bool, string) {
	_ = cfg
	cache, ok := branAuth0CacheFromAccount(acc)
	if !ok || strings.TrimSpace(cache.AccessToken) == "" {
		return false, "missing Auth0 localStorage (GoAuto must include storage/@@auth0spajs@@)"
	}
	if strings.TrimSpace(cache.RefreshToken) != "" {
		return true, "ok (auth0 cache+refresh)"
	}
	if !branAuth0AccessExpired(cache) {
		return true, "ok (auth0 cache present)"
	}
	return false, "Auth0 access expired and no refresh_token"
}

func branInvalidateAuthProbe(acc ToolAccount) {
	_ = acc
	// Kept for call-site compatibility; server Auth0 probe cache removed.
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
		if ok, why := branAccountAuthAlive(cfg, reloaded); ok {
			log.Printf("[FAILOVER] reloaded cookie OK user=%s account=%s (%s) → %s", currentUser, reloaded.Name, why, returnPath)
			http.Redirect(w, r, returnPath, http.StatusFound)
			return
		}
		activeAcc = reloaded
	}

	next, nextName, err := panelSwitchToOtherAccount(cfg, sessionToken, reason)
	if err == nil && next.ID != activeAcc.ID {
		branInvalidateAuthProbe(next)
		if ok, why := branAccountAuthAlive(cfg, next); ok {
			log.Printf("[FAILOVER] switched user=%s %s -> %s reason=%s (%s)", currentUser, activeAcc.Name, nextName, reason, why)
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
	alive, why := branAccountAuthAlive(cfg, acc)
	if alive {
		log.Printf("[FAILOVER] session-check OK user=%s account=%s (%s)", name, acc.Name, why)
		http.Redirect(w, r, branAppHome(cfg), http.StatusFound)
		return
	}
	log.Printf("[FAILOVER] session-check dead user=%s account=%s reason=%s", name, acc.Name, why)
	serveBranLogoutFailover(w, r, cfg, token, name, acc, "session_check")
}

func branAuth0ExtraCDNIndex(cfg Config) int {
	for i, extra := range cfg.ExtraCDNDomains {
		host := strings.ToLower(strings.TrimSpace(extra))
		host = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
		host = strings.Split(host, "/")[0]
		if strings.Contains(host, "auth0.com") {
			return i
		}
	}
	return -1
}

func branAuthWatchScript(cfg Config) string {
	authIdx := branAuth0ExtraCDNIndex(cfg)
	authProxy := ""
	if authIdx >= 0 {
		authProxy = "/extra-cdn-" + strconv.Itoa(authIdx) + "/"
	}
	return `<script data-tm-bran-auth="1">
(function(){
  if (window.__tmBranAuthWatch) return;
  window.__tmBranAuthWatch = true;
  var go = false;
  var AUTH_PROXY = ` + strconv.Quote(authProxy) + `;
  function bounce(){
    if (go) return;
    go = true;
    try { location.replace('/__tm_session_check'); } catch (e) {}
  }
  function auth0DeadURL(u){
    var s = String(u || '');
    // ONLY Auth0 login/token failures mean mapped session is gone.
    // Azure /extra-cdn 401s during boot must NOT trigger Contact Admin.
    var onAuth0 = s.indexOf('auth0.com') !== -1 || (AUTH_PROXY && s.indexOf(AUTH_PROXY) !== -1);
    if (!onAuth0) return false;
    if (s.indexOf('/authorize') !== -1) return true;
    if (s.indexOf('/oauth/token') !== -1) return true;
    if (s.indexOf('/u/login') !== -1) return true;
    if (s.indexOf('/usernamepassword/login') !== -1) return true;
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
          if (auth0DeadURL(u) && (res.status === 401 || res.status === 403 || res.status === 400)) bounce();
        } catch (e) {}
        return res;
      });
    };
  }
  // SPA often uses location.assign to Auth0 /authorize when refresh fails.
  try {
    var _assign = Location.prototype.assign;
    Location.prototype.assign = function(url){
      try { if (auth0DeadURL(url)) { bounce(); return; } } catch (e) {}
      return _assign.apply(this, arguments);
    };
    var _replace = Location.prototype.replace;
    Location.prototype.replace = function(url){
      try { if (auth0DeadURL(url)) { bounce(); return; } } catch (e) {}
      return _replace.apply(this, arguments);
    };
  } catch (e) {}
  // Missing Auth0 dump after inject window → bad paste (cookies-only).
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

// branUnlockSearchScript — Auth0 nickname empty disables Be Curious; Angular
// searchText can stay "" while the input shows a domain → ValidURL snack.
// Capture-phase click: stop Angular, seed domain from DOM, go /summary.
func branUnlockSearchScript() string {
	return `<script data-tm-bran-search="1">
(function(){
  if (window.__tmBranSearchUnlock) return;
  window.__tmBranSearchUnlock = true;
  function claims(){
    var out = {nickname:'', email:''};
    try {
      for (var i = 0; i < localStorage.length; i++) {
        var k = localStorage.key(i) || '';
        if (k.indexOf('auth0spajs') === -1) continue;
        var o = JSON.parse(localStorage.getItem(k) || '{}');
        var body = o.body || o;
        var c = (body.decodedToken && body.decodedToken.claims) || {};
        out.nickname = out.nickname || c.nickname || c.name || '';
        out.email = out.email || c.email || '';
        if ((!out.nickname || !out.email) && body.id_token) {
          try {
            var part = body.id_token.split('.')[1].replace(/-/g,'+').replace(/_/g,'/');
            while (part.length % 4) part += '=';
            var p = JSON.parse(atob(part));
            out.nickname = out.nickname || p.nickname || p.name || p.email || '';
            out.email = out.email || p.email || '';
          } catch (e1) {}
        }
      }
    } catch (e) {}
    if (!out.nickname) out.nickname = 'member';
    if (!out.email) out.email = out.nickname.indexOf('@') >= 0 ? out.nickname : (out.nickname + '@branalyzer.local');
    return out;
  }
  function searchInput(){
    var nodes = document.querySelectorAll('app-home-search input, form.search-form input, mat-form-field input');
    for (var i = 0; i < nodes.length; i++) {
      var el = nodes[i];
      if (!el || el.type === 'hidden') continue;
      if (String(el.value || '').trim()) return el;
    }
    return nodes[0] || null;
  }
  function normalizeDomain(v){
    v = String(v || '').trim();
    if (!v) return null;
    var url = v.toLowerCase().indexOf('http') === 0 ? v.toLowerCase() : ('https://' + v.toLowerCase());
    var name = url.replace(/^https?:\/\//,'').split('/')[0];
    if (!name || name.indexOf('.') < 0) return null;
    return {url:url, name:name, raw:v};
  }
  function seedState(dom){
    var c = claims();
    var N = window.__tmBranN;
    if (N && N.domain) {
      N.domain.url = dom.url;
      N.domain.name = dom.name;
      if (N.currentUser) {
        if (!N.currentUser.nickname) N.currentUser.nickname = c.nickname;
        if (!N.currentUser.email) N.currentUser.email = c.email;
        if (!N.currentUser.userStatus) N.currentUser.userStatus = 'Logged';
        N.currentUser.allowed = true;
      }
    }
    try { sessionStorage.setItem('tm_bran_force_domain', JSON.stringify(dom)); } catch (e) {}
  }
  function unlockBtns(){
    try {
      document.querySelectorAll('button').forEach(function(btn){
        var t = (btn.textContent || '').replace(/\s+/g,' ').trim();
        if (!/^Be Curious$/i.test(t)) return;
        btn.disabled = false;
        btn.removeAttribute('disabled');
        btn.style.pointerEvents = 'auto';
        btn.style.opacity = '1';
      });
    } catch (e) {}
  }
  function forceSummary(dom){
    seedState(dom);
    var dest = '/summary?tm_url=' + encodeURIComponent(dom.name);
    try { location.assign(dest); } catch (e) { location.href = dest; }
  }
  document.addEventListener('click', function(ev){
    try {
      var btn = ev.target && ev.target.closest ? ev.target.closest('button') : null;
      if (!btn) return;
      var t = (btn.textContent || '').replace(/\s+/g,' ').trim();
      if (!/^Be Curious$/i.test(t)) return;
      var inp = searchInput();
      var dom = normalizeDomain(inp && inp.value);
      // Stop Angular getBrandInfo("") → "Introduce any valid URL" snack.
      ev.preventDefault();
      ev.stopPropagation();
      if (ev.stopImmediatePropagation) ev.stopImmediatePropagation();
      if (!dom) return;
      forceSummary(dom);
    } catch (e) {}
  }, true);
  document.addEventListener('click', function(ev){
    try {
      var a = ev.target && ev.target.closest ? ev.target.closest('a[href*="summary"], [routerlink="summary"], a[href="/summary"]') : null;
      if (!a) return;
      var inp = searchInput();
      var dom = normalizeDomain(inp && inp.value);
      if (dom) seedState(dom);
    } catch (e) {}
  }, true);
  unlockBtns();
  setInterval(unlockBtns, 800);
})();
</script>`
}
