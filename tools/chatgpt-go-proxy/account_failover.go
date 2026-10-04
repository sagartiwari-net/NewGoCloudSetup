package main

import (
	"context"
	"html"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const maxAccountFailoverAttempts = 15

// Strong signals only — broad strings like "Sign up for free" live in JS bundles on the main app.
var chatGPTStrongLogoutSniffs = []string{
	"Your session has expired",
	"Please log in again to continue",
	"Please log in again",
}

func isStrongLogoutSniff(sniff string) bool {
	s := strings.ToLower(strings.TrimSpace(sniff))
	return strings.Contains(s, "session has expired") ||
		strings.Contains(s, "please log in again")
}

func isStrongLogoutReason(reason string) bool {
	r := strings.ToLower(reason)
	return strings.HasPrefix(reason, "url_path:") ||
		strings.Contains(r, "session has expired") ||
		strings.Contains(r, "please log in again")
}

func chatGPTLoggedInHTML(body []byte) bool {
	if len(body) < 800 {
		return false
	}
	s := string(body)
	// Soft marketing strings ("What's on your mind", "Ask anything") ship in logged-out
	// JS bundles too — they false-positive and serve a shell with no working send button.
	for _, marker := range []string{
		"oai-client-auth-info",
		`"accessToken"`,
		`"authProvider"`,
		`"connectionType":"websocket"`,
	} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

func chatGPTSessionBodyOK(text string) bool {
	if strings.Contains(text, `"accessToken"`) {
		return true
	}
	if strings.Contains(text, `"user"`) &&
		!strings.Contains(text, `"user":null`) &&
		!strings.Contains(text, `"user": null`) {
		return true
	}
	return false
}

func chatGPTSessionBodyLoggedOut(text string) bool {
	t := strings.TrimSpace(text)
	return t == "" || t == "{}" ||
		strings.Contains(text, `"user":null`) ||
		strings.Contains(text, `"user": null`) ||
		strings.Contains(text, `"accessToken":null`) ||
		strings.Contains(text, `"accessToken": null`)
}

func looksLikeUpstreamChallenge(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "just a moment") ||
		strings.Contains(lower, "cf-browser-verification") ||
		strings.Contains(lower, "cdn-cgi/challenge") ||
		strings.Contains(lower, "cloudflare") && strings.Contains(lower, "<html")
}

// probeChatGPTSession checks whether the account cookie still has a live OpenAI session.
// Dead cookies still render the ChatGPT SPA shell, but the send control never enables.
// Important: bare HTTP 403 from Hetzner/WAF must NOT mark a cookie dead — only clear
// logged-out JSON ({}/user:null) should.
func probeChatGPTSession(cfg Config, acc ToolAccount) (bool, string) {
	cookie := strings.TrimSpace(parseCookieFromDB(acc.Cookie))
	if cookie == "" {
		return false, "empty_cookie"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if px := strings.TrimSpace(acc.Proxy); px != "" {
		ctx = context.WithValue(ctx, proxyContextKey, px)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chatgpt.com/api/auth/session", nil)
	if err != nil {
		return false, "build_request"
	}
	ua := strings.TrimSpace(acc.UserAgent)
	if ua == "" {
		ua = cfg.UserAgent
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cookie", cookie)
	resp, err := httpClient.Do(req)
	if err != nil {
		// Network blip — do not burn the account; let the page path decide.
		return true, "probe_dial_soft:" + err.Error()
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	text := string(body)
	ct := strings.ToLower(resp.Header.Get("Content-Type"))

	if chatGPTSessionBodyOK(text) {
		return true, "ok"
	}
	if looksLikeUpstreamChallenge(text) || (resp.StatusCode >= 400 && !strings.Contains(ct, "json") && strings.Contains(strings.ToLower(text), "<html")) {
		log.Printf("[LB] session probe WAF/challenge for '%s' (HTTP %d) — keeping account", acc.Name, resp.StatusCode)
		return true, "probe_waf_keep"
	}
	if chatGPTSessionBodyLoggedOut(text) {
		return false, "logged_out"
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return false, "logged_out_401"
	}
	if resp.StatusCode == http.StatusForbidden && strings.Contains(ct, "json") {
		// OpenAI JSON 403 with no accessToken → treat as dead session.
		return false, "logged_out_403"
	}
	if resp.StatusCode >= 400 {
		// Ambiguous (often datacenter WAF). Keep account; HTML failover still rotates on real login walls.
		log.Printf("[LB] session probe inconclusive HTTP %d for '%s' — keeping account", resp.StatusCode, acc.Name)
		return true, "probe_inconclusive"
	}
	return false, "no_access_token"
}

func chatGPTLogoutDetected(path string, body []byte, cfg Config) (bool, string) {
	loggedInShell := chatGPTLoggedInHTML(body)

	if detected, reason := detectLogout(path, body, cfg); detected {
		// Logged-in app shell embeds login strings in JS bundles — only trust URL redirects.
		if loggedInShell && !strings.HasPrefix(reason, "url_path:") {
			return false, ""
		}
		return true, reason
	}
	if loggedInShell || len(body) == 0 {
		return false, ""
	}
	bodyStr := string(body)
	textLower := strings.ToLower(bodyStr)
	if usesPanelAccountMode(cfg) && strings.Contains(textLower, "log in") &&
		(strings.Contains(textLower, "sign up") || strings.Contains(bodyStr, "auth.openai.com") || strings.Contains(textLower, "auth/login")) {
		return true, "panel_login_wall"
	}
	for _, sniff := range chatGPTStrongLogoutSniffs {
		if sniff == "" {
			continue
		}
		if strings.Contains(bodyStr, sniff) || strings.Contains(textLower, strings.ToLower(sniff)) {
			return true, "builtin_sniff:" + sniff
		}
	}
	return false, ""
}

func shouldRunHTMLAccountFailover(path string, cfg Config, r *http.Request) bool {
	if usesCookieFileMode(cfg) {
		return false
	}
	// Panel mode recovers with the built-in login signals. Other modes still follow config.
	if !usesPanelAccountMode(cfg) && !cfg.LogoutDetection.Enabled {
		return false
	}
	if !usesPanelAccountMode(cfg) && !dbConnected {
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if isProxyStaticPath(path) {
		return false
	}
	p := strings.Split(path, "?")[0]
	if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/access") {
		return false
	}
	return true
}

type htmlFailoverResult struct {
	bodyBytes    []byte
	upstreamResp *http.Response
	activeAcc    ToolAccount
	ran          bool
	waiting      bool
}

func runHTMLAccountFailover(
	r *http.Request,
	upstreamReq *http.Request,
	upstreamResp *http.Response,
	activeAcc ToolAccount,
	sessionToken, currentUser, path string,
	sendCookies bool,
	cfg Config,
) htmlFailoverResult {
	out := htmlFailoverResult{upstreamResp: upstreamResp, activeAcc: activeAcc}
	if !shouldRunHTMLAccountFailover(path, cfg, r) {
		return out
	}
	if upstreamResp == nil {
		return out
	}
	ct := upstreamResp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		return out
	}

	bodyBytes, err := decompressBody(upstreamResp)
	if err != nil {
		return out
	}
	out.ran = true
	out.bodyBytes = bodyBytes

	// Strong auth markers in HTML → keep serving.
	if chatGPTLoggedInHTML(bodyBytes) {
		return out
	}

	detected, reason := chatGPTLogoutDetected(path, bodyBytes, cfg)

	// Panel mode: logged-out SPA still looks "open" (composer, no send). Never serve it —
	// probe the account session and rotate when cookies are dead.
	if usesPanelAccountMode(cfg) && isDocumentNavigation(r) && sessionToken != "" && activeAcc.ID > 0 {
		ok, probeReason := probeChatGPTSession(cfg, activeAcc)
		if !ok {
			detected = true
			if reason == "" {
				reason = "session_probe:" + probeReason
			}
		} else if !detected {
			// Live session without strong HTML markers — allow shell.
			return out
		}
	}

	if !detected {
		out.bodyBytes = bodyBytes
		return out
	}
	log.Printf("[FAILOVER] attempt 1 | account=%s (ID:%d) user=%s reason=%s",
		activeAcc.Name, activeAcc.ID, currentUser, reason)

	nextAcc, swErr := switchToNextAccount(sessionToken, activeAcc.ID, activeAcc.Name, currentUser, "html_failover:"+reason)
	if swErr == nil && nextAcc.ID != activeAcc.ID {
		out.activeAcc = nextAcc
		log.Printf("[FAILOVER] switched → %s (ID:%d) user=%s — reload will try next account", nextAcc.Name, nextAcc.ID, currentUser)
	} else {
		log.Printf("[FAILOVER] no other account yet for user %s: %v", currentUser, swErr)
		out.activeAcc = activeAcc
	}
	// Never return the logged-out shell — always show switching / cookies-expired UI.
	out.waiting = true
	out.bodyBytes = nil
	return out
}

const accountSwitchRetryScript = `<script>
(function () {
  var n = 0;
  try { n = parseInt(sessionStorage.getItem("tm_acct_try") || "0", 10) || 0; } catch (e) {}
  n += 1;
  try { sessionStorage.setItem("tm_acct_try", String(n)); } catch (e) {}
  var msg = document.querySelector(".msg");
  var title = document.querySelector("h1");
  if (n >= 3) {
    if (title) title.textContent = "ChatGPT accounts logged out";
    if (msg) msg.textContent = "Every mapped ChatGPT cookie looks logged out. Open Panel → Accounts → paste fresh ChatGPT cookies, then open a new access link.";
    try { sessionStorage.removeItem("tm_acct_try"); } catch (e) {}
    return;
  }
  if (msg && n > 1) {
    var text = msg.textContent || "";
    var mark = "Switching to ";
    var at = text.indexOf(mark);
    if (at >= 0) msg.textContent = "Trying " + text.slice(at + mark.length);
    else msg.textContent = "Trying another account...";
  }
  setTimeout(function () { window.location.reload(); }, 4000);
})();
</script>`

func writeAccountSwitchPage(w http.ResponseWriter, accountName string) {
	message := "Account logged out. Switching to another account..."
	if accountName != "" {
		message = "Account logged out. Switching to " + html.EscapeString(accountName) + "..."
	}
	writeLightCard(w, http.StatusOK, lightCard{
		Title:       "Switching account",
		Heading:     "Switching account",
		Message:     message,
		Badge:       "Trying one account at a time",
		Footer:      "This keeps trying until an account is available",
		Spin:        true,
		ExtraScript: accountSwitchRetryScript,
	})
}

func isPanelAuthFailure(path string, status int) bool {
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		return false
	}
	p := strings.Split(path, "?")[0]
	return strings.HasPrefix(p, "/backend-api/") || strings.HasPrefix(p, "/api/auth/")
}

// recoverPanelAPIAccount moves the session to the other active account when ChatGPT rejects the cookie.
// GET and HEAD are retried immediately. Other methods keep the new account for the next request.
func recoverPanelAPIAccount(
	cfg Config,
	r *http.Request,
	upstreamReq *http.Request,
	upstreamResp *http.Response,
	activeAcc ToolAccount,
	sessionToken, currentUser, path string,
	sendCookies bool,
) (ToolAccount, *http.Response, bool) {
	if !usesPanelAccountMode(cfg) || upstreamResp == nil || !isPanelAuthFailure(path, upstreamResp.StatusCode) {
		return activeAcc, nil, false
	}
	nextAcc, err := switchToNextAccount(sessionToken, activeAcc.ID, activeAcc.Name, currentUser, "api_auth:"+http.StatusText(upstreamResp.StatusCode))
	if err != nil || nextAcc.ID == activeAcc.ID {
		log.Printf("[RECOVER] no other account for user %s: %v", currentUser, err)
		return activeAcc, nil, false
	}
	log.Printf("[RECOVER] %s (ID:%d) -> %s (ID:%d) user=%s path=%s", activeAcc.Name, activeAcc.ID, nextAcc.Name, nextAcc.ID, currentUser, path)
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return nextAcc, nil, false
	}
	accountCookieStr := parseCookieFromDB(nextAcc.Cookie)
	clientCookies := stripSensitiveCookies(r.Header.Get("Cookie"), cfg)
	if accountCookieStr != "" {
		if clientCookies != "" {
			clientCookies += "; "
		}
		clientCookies += accountCookieStr
	}
	if sendCookies {
		if clientCookies != "" {
			upstreamReq.Header.Set("Cookie", clientCookies)
		} else {
			upstreamReq.Header.Del("Cookie")
		}
	}
	ua := nextAcc.UserAgent
	if ua == "" {
		ua = cfg.UserAgent
	}
	if ua != "" {
		upstreamReq.Header.Set("User-Agent", ua)
	}
	newResp, doErr := httpClient.Do(upstreamReq)
	if doErr != nil {
		log.Printf("[RECOVER] retry failed: %v", doErr)
		return nextAcc, nil, false
	}
	return nextAcc, newResp, true
}

func mergeWatchdogTriggers(cfg Config) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, t := range cfg.WatchdogTriggers {
		add(t)
	}
	if cfg.LogoutDetection.Enabled {
		for _, t := range cfg.LogoutDetection.TextSniffs {
			add(t)
		}
		for _, s := range chatGPTStrongLogoutSniffs {
			add(s)
		}
	}
	return out
}
