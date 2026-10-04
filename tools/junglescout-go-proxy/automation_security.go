package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	proxyBuildTag       = "jungle-v12-outbound-proxy"
	automationStateFile = "automation_state.json"
)

func refreshWebsiteSecurityFromDB() {
	if !dbConnected {
		currentWebsiteSecurityEnabled = true
		return
	}
	var enabled int
	err := db.QueryRow(
		"SELECT COALESCE(session_security_enabled, 1) FROM ahrefs_websites WHERE id = ?",
		currentWebsiteID,
	).Scan(&enabled)
	if err != nil {
		currentWebsiteSecurityEnabled = true
		return
	}
	currentWebsiteSecurityEnabled = enabled == 1
}

func killSession(sessionToken, reason string) {
	if sessionToken == "" || !dbConnected {
		return
	}
	_, _ = db.Exec("DELETE FROM ahrefs_sessions WHERE session_token = ? AND website_id = ?", sessionToken, currentWebsiteID)
	ipTracker.Delete(sessionToken)
	preview := sessionToken
	if len(preview) > 8 {
		preview = preview[:8] + "..."
	}
	log.Printf("[SECURITY] Session killed | reason=%s token=%s", reason, preview)
}

func cookieSecure(r *http.Request, cfg Config) bool {
	// Pre-SSL: overlays use public_scheme=http. Never mark cookies Secure or CF X-Forwarded-Proto=https drops them on http:// pages.
	if strings.EqualFold(strings.TrimSpace(cfg.PublicScheme), "http") {
		return false
	}
	if r.TLS != nil {
		return true
	}
	if strings.Contains(strings.ToLower(r.Header.Get("X-Forwarded-Proto")), "https") {
		return true
	}
	if r.Header.Get("X-Forwarded-Ssl") == "on" {
		return true
	}
	return strings.EqualFold(cfg.PublicScheme, "https")
}

func requestScheme(r *http.Request, cfg Config) string {
	if cookieSecure(r, cfg) {
		return "https"
	}
	if cfg.PublicScheme != "" {
		return cfg.PublicScheme
	}
	return "http"
}

func isLocalDev(cfg Config) bool {
	host := strings.ToLower(cfg.PublicHost)
	return strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1")
}

func usesCookieFileMode(cfg Config) bool {
	if usesPanelAccountMode(cfg) {
		return false
	}
	return cfg.BypassAuth || !cfg.UseDatabase || !dbConnected || isLocalDev(cfg) || cfg.LocalTestMode
}

func detectLogout(path string, body []byte, cfg Config) (bool, string) {
	ld := cfg.LogoutDetection
	if !ld.Enabled {
		return false, ""
	}

	pathClean := strings.Split(path, "?")[0]
	pathClean = strings.TrimSuffix(pathClean, "/")
	if pathClean == "" {
		pathClean = "/"
	}
	for _, p := range ld.URLPaths {
		pClean := strings.TrimSuffix(strings.TrimSpace(p), "/")
		if pClean == "" {
			continue
		}
		if pathClean == pClean || strings.HasPrefix(pathClean, pClean+"/") {
			return true, "url_path:" + p
		}
	}

	if len(body) == 0 {
		return false, ""
	}
	bodyStr := string(body)
	// SPA shells ship large JS bundles — "Sign in" etc. appear in minified code, not on screen.
	if len(bodyStr) > 20000 {
		for _, h := range ld.HTMLSniffs {
			if h != "" && strings.Contains(bodyStr, h) {
				return true, "html_sniff:" + h[:min(60, len(h))]
			}
		}
		return false, ""
	}
	for _, t := range ld.TextSniffs {
		if t != "" && (strings.Contains(bodyStr, t) || strings.Contains(strings.ToLower(bodyStr), strings.ToLower(t))) {
			return true, "text_sniff:" + t
		}
	}
	for _, h := range ld.HTMLSniffs {
		if h != "" && strings.Contains(bodyStr, h) {
			return true, "html_sniff:" + h[:min(60, len(h))]
		}
	}
	return false, ""
}

func isLoginRedirectLocation(loc string, cfg Config) bool {
	if !cfg.LogoutDetection.Enabled && !cfg.Automation.Enabled {
		return false
	}
	locLower := strings.ToLower(strings.TrimSpace(loc))
	if locLower == "" {
		return false
	}
	if strings.Contains(locLower, "login.junglescout.com") {
		return true
	}
	u, err := url.Parse(loc)
	path := loc
	if err == nil {
		if u.Path != "" {
			path = u.Path
		} else if u.Host != "" {
			path = "/"
		}
	}
	detected, _ := detectLogout(path, nil, cfg)
	return detected
}

func serveLoginReconnectPage(w http.ResponseWriter, cfg Config) {
	ld := cfg.LogoutDetection
	refreshSec := ld.RefreshSeconds
	if refreshSec <= 0 {
		refreshSec = 5
	}
	homePath := cfg.HomePath
	if homePath == "" {
		homePath = "/"
	}
	title := ld.OverlayTitle
	if title == "" {
		title = "Session Update"
	}
	msg := ld.OverlayMessage
	if msg == "" {
		msg = "Account logout detected. We are updating your session, please wait..."
	}
	toolName := cfg.ToolName
	if toolName == "" {
		toolName = "Jungle Scout"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	applySecurityHeaders(w, cfg)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1.0">
<meta http-equiv="refresh" content="%d;url=%s">
<title>%s — %s</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;background:#0f172a;color:#fff;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:20px}
.c{text-align:center;max-width:420px;padding:36px 28px;background:rgba(255,255,255,0.04);border:1px solid rgba(255,255,255,0.08);border-radius:20px}
.spin{width:48px;height:48px;border:4px solid rgba(255,255,255,0.15);border-top-color:#6366f1;border-radius:50%%;margin:0 auto 20px;animation:spin .8s linear infinite}
h1{font-size:22px;margin-bottom:10px}
p{color:#94a3b8;line-height:1.6;font-size:15px}
@keyframes spin{to{transform:rotate(360deg)}}
</style>
</head>
<body>
<div class="c">
<div class="spin"></div>
<h1>%s</h1>
<p>%s</p>
<p style="margin-top:12px;font-size:13px">Redirecting in %d seconds...</p>
</div>
</body>
</html>`,
		refreshSec, html.EscapeString(homePath),
		html.EscapeString(title), html.EscapeString(toolName),
		html.EscapeString(title), html.EscapeString(msg), refreshSec)
}

func handleLogoutDetected(cfg Config, reason string, acc ToolAccount, username, sessionToken string) bool {
	if !cfg.LogoutDetection.Enabled && !cfg.Automation.Enabled {
		return false
	}
	log.Printf("[LOGOUT] Detected | user=%s account=%s (ID:%d) reason=%s", username, acc.Name, acc.ID, reason)

	triggered := false
	if cfg.Automation.Enabled {
		triggered = tryTriggerAutomationForAccount(cfg, reason, acc, username)
	}

	if dbConnected && sessionToken != "" {
		if nextAcc, err := switchToNextAccount(sessionToken, acc.ID, acc.Name, username, "logout: "+reason); err == nil {
			log.Printf("[LOGOUT] Next account for session: '%s' (ID:%d)", nextAcc.Name, nextAcc.ID)
		}
	}

	return triggered
}

func getAccountTaskUID(acc ToolAccount, cfg Config) string {
	if acc.AutomationTaskUID != "" {
		return acc.AutomationTaskUID
	}
	if uid, ok := cfg.Automation.Payload["task_uid"].(string); ok {
		return uid
	}
	return ""
}

type persistedAutomationState struct {
	Accounts map[string]string `json:"accounts"`
}

func readAutomationStateFile() persistedAutomationState {
	automationStateMu.Lock()
	defer automationStateMu.Unlock()
	data, err := os.ReadFile(automationStateFile)
	if err != nil {
		return persistedAutomationState{Accounts: map[string]string{}}
	}
	var s persistedAutomationState
	if json.Unmarshal(data, &s) != nil || s.Accounts == nil {
		return persistedAutomationState{Accounts: map[string]string{}}
	}
	return s
}

func writeAutomationStateFile(s persistedAutomationState) {
	automationStateMu.Lock()
	defer automationStateMu.Unlock()
	if s.Accounts == nil {
		s.Accounts = map[string]string{}
	}
	out, _ := json.MarshalIndent(s, "", "  ")
	_ = os.WriteFile(automationStateFile, out, 0644)
}

func canTriggerAutomationForAccount(accountID, cooldownSeconds int) bool {
	if cooldownSeconds <= 0 {
		cooldownSeconds = 300
	}
	s := readAutomationStateFile()
	lastStr, ok := s.Accounts[strconv.Itoa(accountID)]
	if !ok || lastStr == "" {
		return true
	}
	last, err := time.Parse(time.RFC3339, lastStr)
	if err != nil {
		return true
	}
	return time.Since(last) >= time.Duration(cooldownSeconds)*time.Second
}

func markAutomationTriggeredForAccount(accountID int) {
	s := readAutomationStateFile()
	if s.Accounts == nil {
		s.Accounts = map[string]string{}
	}
	s.Accounts[strconv.Itoa(accountID)] = time.Now().UTC().Format(time.RFC3339)
	writeAutomationStateFile(s)
}

func logoutOverlayImmediateScript(cfg Config) string {
	ld := cfg.LogoutDetection
	if !ld.Enabled || !ld.ShowOverlay {
		return ""
	}
	title := ld.OverlayTitle
	if title == "" {
		title = "Session Update"
	}
	msg := ld.OverlayMessage
	if msg == "" {
		msg = "Account logout detected. We are updating your session, please wait..."
	}
	refreshSec := ld.RefreshSeconds
	if refreshSec <= 0 {
		refreshSec = 5
	}
	return fmt.Sprintf(`<script>window.__tm_show_logout_overlay=true;window.__tm_overlay_title=%q;window.__tm_overlay_msg=%q;window.__tm_schedule_refresh=true;window.__tm_refresh_seconds=%d;</script>`,
		title, msg, refreshSec)
}

func tryTriggerAutomationForAccount(cfg Config, reason string, acc ToolAccount, username string) bool {
	auto := cfg.Automation
	if !auto.Enabled || auto.URL == "" {
		return false
	}
	cooldown := auto.CooldownSeconds
	if cooldown <= 0 {
		cooldown = 300
	}
	if !canTriggerAutomationForAccount(acc.ID, cooldown) {
		log.Printf("[AUTOMATION] Cooldown active for account '%s' (ID:%d) — max 1 trigger per %ds",
			acc.Name, acc.ID, cooldown)
		return false
	}

	taskUID := getAccountTaskUID(acc, cfg)
	if taskUID == "" {
		log.Printf("[AUTOMATION] No task_uid for account '%s' (ID:%d)", acc.Name, acc.ID)
		return false
	}

	autoURL, err := normalizeAutomationURL(auto.URL)
	if err != nil {
		log.Printf("[AUTOMATION] Invalid automation.url: %v", err)
		return false
	}

	markAutomationTriggeredForAccount(acc.ID)
	log.Printf("[AUTOMATION] Triggering | account=%s (ID:%d) task_uid=%s reason=%s",
		acc.Name, acc.ID, taskUID, reason)

	go func() {
		method := strings.ToUpper(auto.Method)
		if method == "" {
			method = http.MethodPost
		}
		payload := make(map[string]interface{})
		for k, v := range auto.Payload {
			payload[k] = v
		}
		payload["task_uid"] = taskUID
		payload["reason"] = reason
		payload["account_id"] = acc.ID
		payload["account_name"] = acc.Name
		payload["username"] = username
		payload["tool"] = cfg.ToolName
		payload["website_id"] = currentWebsiteID
		payload["detected_at"] = time.Now().Format(time.RFC3339)

		bodyBytes, err := json.Marshal(payload)
		if err != nil {
			log.Printf("[AUTOMATION] Payload marshal error: %v", err)
			return
		}
		req, err := http.NewRequest(method, autoURL, bytes.NewReader(bodyBytes))
		if err != nil {
			log.Printf("[AUTOMATION] Request build error: %v", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range auto.Headers {
			req.Header.Set(k, v)
		}
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[AUTOMATION] Call failed for account '%s': %v", acc.Name, err)
			return
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		respStr := string(respBody[:min(len(respBody), 500)])
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			log.Printf("[AUTOMATION] Success | account=%s task_uid=%s status=%d response=%s",
				acc.Name, taskUID, resp.StatusCode, respStr)
			return
		}
		log.Printf("[AUTOMATION] Failed | account=%s status=%d response=%s", acc.Name, resp.StatusCode, respStr)
	}()
	return true
}

func normalizeAutomationURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("must be absolute URL with scheme and host")
	}
	path := u.Path
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	u.Path = path
	return u.String(), nil
}

func securityPingHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if !securityEnabled(cfg) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true,"security":"off"}`)
		return
	}
	reqHost := requestHost(r)
	if cfg.PublicHost != "" && !isLocalDev(cfg) && !isInternalRequestHost(reqHost) {
		ss := normalizeSessionSecurity(cfg)
		if !hostMatchesPublic(reqHost, cfg.PublicHost, ss.DomainCheck.AllowedHosts) {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":"host_mismatch"}`)
			return
		}
	}
	if _, err := getAuthenticatedUser(r, cfg); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"unauthorized"}`)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"ok":true}`)
}

func triggerAutomationHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Reason == "" {
		payload.Reason = "client_side_detection"
	}

	acc := ToolAccount{ID: 1, Name: "Local Standalone Account"}
	username := "local_dev"
	sessionToken := ""

	if c, err := r.Cookie("ct_session"); err == nil {
		sessionToken = c.Value
	}
	if !usesCookieFileMode(cfg) && sessionToken != "" {
		_ = db.QueryRow("SELECT username FROM ahrefs_sessions WHERE session_token = ? AND website_id = ?", sessionToken, currentWebsiteID).Scan(&username)
		if a, ok := getSessionAssignedAccount(sessionToken); ok {
			acc = a
		}
	}

	log.Printf("[LOGOUT] Client-side detection | user=%s account=%s (ID:%d) reason=%s", username, acc.Name, acc.ID, payload.Reason)
	triggered := handleLogoutDetected(cfg, payload.Reason, acc, username, sessionToken)

	refreshSec := cfg.LogoutDetection.RefreshSeconds
	if refreshSec <= 0 {
		refreshSec = 5
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "ok",
		"triggered":    triggered,
		"refresh_in":   refreshSec,
		"account_id":   acc.ID,
		"account_name": acc.Name,
	})
}

func buildLogoutAutomationJS(cfg Config) string {
	ld := cfg.LogoutDetection
	if !ld.Enabled {
		return ""
	}
	title := ld.OverlayTitle
	if title == "" {
		title = "Session Update"
	}
	msg := ld.OverlayMessage
	if msg == "" {
		msg = "Account logout detected. We are updating your session, please wait..."
	}
	refreshSec := ld.RefreshSeconds
	if refreshSec <= 0 {
		refreshSec = 5
	}
	urlPathsJSON, _ := json.Marshal(ld.URLPaths)
	textSniffsJSON, _ := json.Marshal(ld.TextSniffs)
	htmlSniffsJSON, _ := json.Marshal(ld.HTMLSniffs)
	return fmt.Sprintf(`
    // Logout detection → overlay → refresh loop
    var AUTO_TRIGGER_ENABLED = %v;
    var SHOW_LOGOUT_OVERLAY = %v;
    var REFRESH_MS = %d;
    var OVERLAY_TITLE = %q;
    var OVERLAY_MSG = %q;
    var refreshTimer = null;
    var countdownTimer = null;
    var apiCalledThisPage = false;

    function showTmLogoutOverlay() {
        if (!SHOW_LOGOUT_OVERLAY || document.getElementById('tm-logout-overlay')) return;
        var o = document.createElement('div');
        o.id = 'tm-logout-overlay';
        o.style.cssText = 'position:fixed;inset:0;background:rgba(15,23,42,0.93);backdrop-filter:blur(10px);z-index:2147483647;display:flex;align-items:center;justify-content:center;font-family:-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;padding:20px;';
        o.innerHTML = '<div style="background:#fff;border-radius:20px;padding:36px 32px;max-width:420px;width:100%%;text-align:center;box-shadow:0 25px 60px rgba(0,0,0,0.35);">' +
            '<div style="width:52px;height:52px;border:4px solid #e2e8f0;border-top-color:#4f46e5;border-radius:50%%;margin:0 auto 20px;animation:tmSpin 0.8s linear infinite;"></div>' +
            '<h2 style="font-size:20px;font-weight:700;color:#0f172a;margin:0 0 10px;">' + OVERLAY_TITLE + '</h2>' +
            '<p style="font-size:15px;color:#64748b;line-height:1.6;margin:0 0 8px;">' + OVERLAY_MSG + '</p>' +
            '<p id="tm-logout-countdown" style="font-size:13px;color:#94a3b8;margin:0;">Refreshing in ' + (REFRESH_MS/1000) + ' seconds...</p></div>';
        if (!document.getElementById('tm-logout-spin-style')) {
            var s = document.createElement('style');
            s.id = 'tm-logout-spin-style';
            s.textContent = '@keyframes tmSpin{to{transform:rotate(360deg)}}';
            document.head.appendChild(s);
        }
        (document.body || document.documentElement).appendChild(o);
    }

    function startCountdown() {
        var el = document.getElementById('tm-logout-countdown');
        if (!el) return;
        var sec = Math.ceil(REFRESH_MS / 1000);
        el.textContent = 'Refreshing in ' + sec + ' seconds...';
        if (countdownTimer) clearInterval(countdownTimer);
        countdownTimer = setInterval(function() {
            sec--;
            if (sec <= 0) {
                clearInterval(countdownTimer);
                countdownTimer = null;
                el.textContent = 'Refreshing now...';
                return;
            }
            el.textContent = 'Refreshing in ' + sec + ' second' + (sec === 1 ? '' : 's') + '...';
        }, 1000);
    }

    function hideTmLogoutOverlay() {
        var o = document.getElementById('tm-logout-overlay');
        if (o) o.remove();
        if (refreshTimer) { clearTimeout(refreshTimer); refreshTimer = null; }
        if (countdownTimer) { clearInterval(countdownTimer); countdownTimer = null; }
        apiCalledThisPage = false;
    }

    function scheduleRefresh() {
        if (refreshTimer) return;
        startCountdown();
        refreshTimer = setTimeout(function() {
            refreshTimer = null;
            window.location.reload();
        }, REFRESH_MS);
    }

    function detectLogoutReason() {
        var path = (window.location.pathname || '/').replace(/\/$/, '') || '/';
        var html = document.documentElement ? document.documentElement.innerHTML : '';
        var text = document.body ? (document.body.innerText || document.body.textContent || '').trim() : '';
        var URL_PATHS = %s;
        var TEXT_SNIFFS = %s;
        var HTML_SNIFFS = %s;
        for (var i = 0; i < URL_PATHS.length; i++) {
            var p = (URL_PATHS[i] || '').replace(/\/$/, '');
            if (p && (path === p || path.indexOf(p + '/') === 0)) return 'url_path:' + URL_PATHS[i];
        }
        // Text sniffs: visible page text only (not bundled JS in innerHTML)
        if (text.length > 0 && text.length < 6000) {
            for (var j = 0; j < TEXT_SNIFFS.length; j++) {
                if (TEXT_SNIFFS[j] && text.indexOf(TEXT_SNIFFS[j]) !== -1)
                    return 'text_sniff:' + TEXT_SNIFFS[j];
            }
        }
        if (html.length < 20000) {
            for (var k = 0; k < HTML_SNIFFS.length; k++) {
                if (HTML_SNIFFS[k] && html.indexOf(HTML_SNIFFS[k]) !== -1) return 'html_sniff:' + HTML_SNIFFS[k].substring(0, 40);
            }
        }
        return '';
    }

    function onLogoutDetected(reason) {
        showTmLogoutOverlay();
        scheduleRefresh();
        if (AUTO_TRIGGER_ENABLED && !apiCalledThisPage) {
            apiCalledThisPage = true;
            fetch('/api/trigger-automation', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({ reason: reason })
            }).catch(function(e){ console.error('[ToolsMandi] automation call failed', e); });
        }
    }

    function checkLogoutAutomation() {
        var reason = detectLogoutReason();
        if (reason) {
            onLogoutDetected(reason);
        } else {
            hideTmLogoutOverlay();
        }
    }

    window.addEventListener('popstate', function() { setTimeout(checkLogoutAutomation, 300); });
    if (window.__tm_show_logout_overlay) {
        if (window.__tm_overlay_title) OVERLAY_TITLE = window.__tm_overlay_title;
        if (window.__tm_overlay_msg) OVERLAY_MSG = window.__tm_overlay_msg;
        if (window.__tm_refresh_seconds) REFRESH_MS = window.__tm_refresh_seconds * 1000;
        apiCalledThisPage = true;
        showTmLogoutOverlay();
        scheduleRefresh();
    }
    setTimeout(checkLogoutAutomation, 800);
    setInterval(checkLogoutAutomation, 3000);
`, cfg.Automation.Enabled, ld.ShowOverlay, refreshSec*1000, title, msg,
		string(urlPathsJSON), string(textSniffsJSON), string(htmlSniffsJSON))
}
