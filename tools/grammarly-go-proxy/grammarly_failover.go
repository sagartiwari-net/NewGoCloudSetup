package main

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// grammarlyCookieSummary returns short diagnostics for app.log / __logs.
func grammarlyCookieSummary(cookieHeader string) string {
	cookieHeader = strings.TrimSpace(cookieHeader)
	if cookieHeader == "" {
		return "empty"
	}
	hasGrauth := cookieNamed(cookieHeader, "grauth") != ""
	hasCSRF := cookieNamed(cookieHeader, "csrf-token") != "" || cookieNamed(cookieHeader, "csrf_token") != ""
	return fmt.Sprintf("len=%d names=%d grauth=%v csrf=%v",
		len(cookieHeader), countCookieNames(cookieHeader), hasGrauth, hasCSRF)
}

func grammarlyLooksLoggedOutHTML(body []byte, status int, location string) (bool, string) {
	loc := strings.ToLower(strings.TrimSpace(location))
	if loc != "" {
		if strings.Contains(loc, "/login") || strings.Contains(loc, "/signin") ||
			strings.Contains(loc, "/sign-in") || strings.Contains(loc, "/signup") ||
			strings.Contains(loc, "auth.grammarly.com") {
			return true, "redirect:" + truncateForLog(location, 120)
		}
	}
	if status == 401 || status == 403 {
		return true, fmt.Sprintf("html_status_%d", status)
	}
	low := strings.ToLower(string(body))
	if len(low) > 12000 {
		low = low[:12000]
	}
	// App shell usually has editor markers; login walls shout these.
	markers := []string{
		`id="signup"`,
		`/signup`,
		`sign up for free`,
		`log in to grammarly`,
		`create an account`,
		`"isauthenticated":false`,
		`"authenticated":false`,
		`data-qa="login"`,
	}
	for _, m := range markers {
		if strings.Contains(low, m) {
			// Avoid false positive on logged-in pages that mention "signup" once in footer JS.
			if m == `/signup` && (strings.Contains(low, `grauth`) || strings.Contains(low, `editor`) || strings.Contains(low, `documents`)) {
				continue
			}
			return true, "html_marker:" + m
		}
	}
	return false, ""
}

func grammarlyAPIAuthFailPath(path string) bool {
	p := strings.ToLower(path)
	keys := []string{
		"/subscription", "/info", "/documents", "/user", "/identity",
		"/auth", "/session", "/me", "/account", "/properties",
	}
	for _, k := range keys {
		if strings.Contains(p, k) {
			return true
		}
	}
	return strings.Contains(p, "extra-cdn") || strings.Contains(p, "ext-host")
}

func truncateForLog(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func renderGrammarlyContactAdminPage(w http.ResponseWriter, cfg Config, reason string) {
	_ = cfg
	msg := "Contact to Admin/Provider to fix it ASAP"
	if reason != "" {
		msg = "Grammarly cookie/session failed (" + html.EscapeString(reason) + "). Contact to Admin/Provider to fix it ASAP"
	}
	writeLightCard(w, http.StatusServiceUnavailable, lightCard{
		Title:   "Account unavailable",
		Heading: "Account unavailable",
		Message: msg,
		Footer:  "Update the mapped Grammarly cookie in the panel, then open a new access link",
	})
}

func renderPanelAccountSwitchPage(w http.ResponseWriter, cfg Config, accountName, returnPath string) {
	if returnPath == "" || !strings.HasPrefix(returnPath, "/") {
		returnPath = "/"
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

// serveGrammarlyCookieFailover: reload same cookie → switch other account → contact admin.
func serveGrammarlyCookieFailover(w http.ResponseWriter, r *http.Request, cfg Config, sessionToken, currentUser string, activeAcc ToolAccount, reason string) {
	log.Printf("[FAILOVER] start user=%s account=%s(%d) reason=%s cookie=%s",
		currentUser, activeAcc.Name, activeAcc.ID, reason, grammarlyCookieSummary(activeAcc.Cookie))
	pushProxyLog(ProxyLogEntry{
		Source:  "FAILOVER",
		Level:   "error",
		Method:  r.Method,
		Path:    r.URL.Path,
		Status:  503,
		User:    currentUser,
		Account: activeAcc.Name,
		Detail:  "cookie_dead reason=" + reason + " " + grammarlyCookieSummary(activeAcc.Cookie),
		CookieN: countCookieNames(activeAcc.Cookie),
	})

	if reloaded, err := panelReloadAccount(cfg, sessionToken); err == nil {
		if cookieNamed(reloaded.Cookie, "grauth") != "" && reloaded.ID == activeAcc.ID {
			// Same account, fresher paste — bounce home once.
			if reason == "missing_grauth" && cookieNamed(activeAcc.Cookie, "grauth") == "" && cookieNamed(reloaded.Cookie, "grauth") != "" {
				log.Printf("[FAILOVER] reloaded grauth OK user=%s account=%s", currentUser, reloaded.Name)
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
		}
		activeAcc = reloaded
	}

	next, nextName, err := panelSwitchToOtherAccount(cfg, sessionToken, reason)
	if err == nil && next.ID != activeAcc.ID {
		log.Printf("[FAILOVER] switched user=%s %s -> %s reason=%s cookie=%s",
			currentUser, activeAcc.Name, nextName, reason, grammarlyCookieSummary(next.Cookie))
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
		renderPanelAccountSwitchPage(w, cfg, nextName, "/")
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
	renderGrammarlyContactAdminPage(w, cfg, reason)
}

func grammarlyFailoverAPIHandler(w http.ResponseWriter, r *http.Request) {
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
	acc, err := loadPanelSessionAccount(cfg, token)
	if err != nil {
		log.Printf("[FAILOVER] api no account: %v", err)
		renderGrammarlyContactAdminPage(w, cfg, "no_mapped_account")
		return
	}
	serveGrammarlyCookieFailover(w, r, cfg, token, username, acc, reason)
}

func grammarlyClientDiagHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 8<<10))
	user := ""
	account := ""
	if u, err := panelSessionUsername(r); err == nil {
		user = u
		if tok, _, ok := sessionFromRequest(r); ok {
			if acc, aerr := loadPanelSessionAccount(loadConfig(), tok); aerr == nil {
				account = acc.Name
			}
		}
	}
	detail := truncateForLog(string(body), 500)
	log.Printf("[CLIENT-DIAG] user=%s account=%s %s", user, account, detail)
	pushProxyLog(ProxyLogEntry{
		Source:  "CLIENT",
		Level:   "warn",
		Method:  "POST",
		Path:    "/api/client-diag",
		Status:  200,
		User:    user,
		Account: account,
		Detail:  detail,
	})
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"ok"}`)
}

// grammarlyEarlyGatewayPatch runs FIRST in <head> so gateway.grammarly.com never
// leaves this origin (CLIENT-DIAG showed experimentation/* CORS blanking the SPA).
func grammarlyEarlyGatewayPatch() string {
	return `<script data-tm-early-gw="1">
(function(){
  if (window.__tmEarlyGW) return;
  window.__tmEarlyGW = true;
  var O = location.origin;
  function forceGW(u){
    if (u == null) return u;
    if (typeof u !== 'string') {
      try { u = (u && u.url) ? String(u.url) : String(u); } catch (e) { return u; }
    }
    u = u.replace(/https?:\/\/gateway\.grammarly\.com/gi, O + '/ext-host/gateway.grammarly.com');
    u = u.replace(/https?:\/\/treatment\.grammarly\.com/gi, O + '/ext-host/treatment.grammarly.com');
    u = u.replace(/https?:\/\/gates\.grammarly\.com/gi, O + '/ext-host/gates.grammarly.com');
    u = u.replace(/https?:\/\/institution\.grammarly\.com/gi, O + '/ext-host/institution.grammarly.com');
    u = u.replace(/(^|[^:])\/\/gateway\.grammarly\.com/gi, '$1' + O + '/ext-host/gateway.grammarly.com');
    // Any other *.grammarly.com still absolute → ext-host (except this host)
    u = u.replace(/https?:\/\/((?:[a-z0-9-]+\.)+grammarly\.com)(?=\/|$)/gi, function(m, host){
      if (host === 'app.grammarly.com') return O;
      return O + '/ext-host/' + host;
    });
    return u;
  }
  window.__tmForceGW = forceGW;
  var nf = window.fetch;
  if (typeof nf === 'function') {
    window.fetch = function(input, init){
      try {
        if (typeof input === 'string') input = forceGW(input);
        else if (input && typeof Request !== 'undefined' && input instanceof Request)
          input = new Request(forceGW(input.url), input);
        else if (input && typeof input.url === 'string')
          input = new Request(forceGW(input.url), input);
      } catch (e) {}
      var u = typeof input === 'string' ? input : (input && input.url) || '';
      try {
        var abs = new URL(u, location.href);
        if (abs.origin !== location.origin && /grammarly\.com$/i.test(abs.hostname) &&
            /experimentation|\/properties|\/treatment|\/gates\//i.test(abs.pathname)) {
          return Promise.resolve(new Response('{}', {status:200, headers:{'Content-Type':'application/json'}}));
        }
      } catch (e) {}
      return nf.call(this, input, init);
    };
  }
  var xo = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function(m, u){
    try { u = forceGW(u); } catch (e) {}
    var args = Array.prototype.slice.call(arguments);
    args[1] = u;
    return xo.apply(this, args);
  };
})();
</script>`
}

func injectHeadStart(body []byte, script string) []byte {
	if script == "" || bytes.Contains(body, []byte("data-tm-early-gw")) {
		return body
	}
	inj := []byte(script)
	lower := bytes.ToLower(body)
	if h := bytes.Index(lower, []byte("<head")); h >= 0 {
		if gt := bytes.IndexByte(lower[h:], '>'); gt >= 0 {
			at := h + gt + 1
			out := make([]byte, 0, len(body)+len(inj))
			out = append(out, body[:at]...)
			out = append(out, inj...)
			out = append(out, body[at:]...)
			return out
		}
	}
	return append(inj, body...)
}

// Injected into Grammarly HTML: report blank/CORS + bounce to failover on auth 401.
func grammarlyDiagAndFailoverScript() string {
	return `<script data-tm-grammarly-diag="1">
(function(){
  if (window.__tmGrammarlyDiag) return;
  window.__tmGrammarlyDiag = true;
  function postDiag(payload){
    try {
      // Use raw path only — avoid recursion through patched fetch noise
      var body = JSON.stringify(payload);
      if (navigator.sendBeacon) {
        try { navigator.sendBeacon('/api/client-diag', new Blob([body], {type:'application/json'})); return; } catch (e) {}
      }
      fetch('/api/client-diag', {
        method: 'POST',
        credentials: 'same-origin',
        headers: {'Content-Type':'application/json'},
        body: body
      }).catch(function(){});
    } catch (e) {}
  }
  function goFailover(reason){
    if (window.__tmGrammarlyFailing) return;
    window.__tmGrammarlyFailing = true;
    postDiag({event:'failover', reason: reason, href: location.href});
    try { location.replace('/api/grammarly-failover?reason=' + encodeURIComponent(reason || 'client')); }
    catch (e) {}
  }
  function interesting(u){
    try {
      var s = String(u || '');
      return /subscription|\/info|documents|\/auth|identity|\/me|account|properties|extra-cdn|ext-host|gateway|treatment|gates/i.test(s);
    } catch (e) { return false; }
  }
  var ofetch = window.fetch;
  if (typeof ofetch === 'function') {
    window.fetch = function(input, init){
      try {
        if (typeof window.__tmForceGW === 'function') {
          if (typeof input === 'string') input = window.__tmForceGW(input);
          else if (input && input.url) input = new Request(window.__tmForceGW(input.url), input);
        } else if (typeof window.__tmPatchURL === 'function') {
          if (typeof input === 'string') input = window.__tmPatchURL(input);
          else if (input && input.url) input = new Request(window.__tmPatchURL(input.url), input);
        }
      } catch (e) {}
      var url = typeof input === 'string' ? input : (input && input.url) || '';
      return ofetch.call(this, input, init).then(function(res){
        try {
          if (res && (res.status === 401 || res.status === 403) && interesting(url) && url.indexOf('/ext-host/') === -1 && url.indexOf('/extra-cdn') === -1 && url.indexOf(location.origin) === 0) {
            // same-origin API 401 after rewrite — real cookie problem
            if (/subscription|documents|\/info|passport|identity/i.test(url)) {
              postDiag({event:'auth_status', status: res.status, url: String(url).slice(0,180)});
              goFailover('api_' + res.status);
            }
          }
        } catch (e) {}
        return res;
      }).catch(function(err){
        try {
          if (interesting(url) || /properties/i.test(String(url))) {
            postDiag({event:'fetch_error', url: String(url).slice(0,180), err: String(err && err.message || err)});
          }
        } catch (e) {}
        // Never blank the SPA on experimentation CORS
        if (/gateway\.grammarly\.com|experimentation|\/properties/i.test(String(url))) {
          return new Response('{}', {status:200, headers:{'Content-Type':'application/json'}});
        }
        throw err;
      });
    };
  }
  // After boot: if page still blank / hidden, report once.
  setTimeout(function(){
    try {
      var html = document.documentElement;
      var vis = html ? getComputedStyle(html).visibility : '?';
      var bodyText = (document.body && document.body.innerText || '').replace(/\s+/g,' ').trim().slice(0,120);
      var kids = document.body ? document.body.children.length : 0;
      postDiag({
        event: 'boot_check',
        visibility: vis,
        title: document.title || '',
        bodyKids: kids,
        bodyText: bodyText,
        href: location.href
      });
      if (vis === 'hidden' || (kids < 2 && bodyText.length < 8)) {
        postDiag({event:'blank_suspect', visibility: vis, bodyKids: kids});
      }
    } catch (e) {}
  }, 3500);
})();
</script>`
}

// Rate-limit document failover so we don't loop switch pages.
var (
	grammarlyFailoverMu   sync.Mutex
	grammarlyFailoverLast = map[string]time.Time{}
)

func grammarlyFailoverRecently(sessionToken string) bool {
	grammarlyFailoverMu.Lock()
	defer grammarlyFailoverMu.Unlock()
	t, ok := grammarlyFailoverLast[sessionToken]
	if ok && time.Since(t) < 45*time.Second {
		return true
	}
	grammarlyFailoverLast[sessionToken] = time.Now()
	return false
}
