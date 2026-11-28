package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *server) handleDevAccounts(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.DevAuth {
		writeErr(w, http.StatusNotFound, "dev_disabled")
		return
	}
	list, err := s.listAccountsPublic(1)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "accounts": list})
}

type devLimitsReq struct {
	Token  string `json:"token"`
	Login  string `json:"login"`
	Hourly int    `json:"hourly"`
	Daily  int    `json:"daily"`
}

func (s *server) handleDevSetLimits(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.DevAuth {
		writeErr(w, http.StatusNotFound, "dev_disabled")
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var req devLimitsReq
	_ = json.Unmarshal(body, &req)
	login := strings.TrimSpace(req.Login)
	resellerID := 1
	if tok := strings.TrimSpace(req.Token); tok != "" {
		sess, err := s.lookupSession(tok)
		if err != nil || sess == nil {
			writeErr(w, http.StatusUnauthorized, "Unauthorized")
			return
		}
		login = sess.AmemberLogin
		resellerID = sess.ResellerID
	}
	if login == "" {
		writeErr(w, http.StatusBadRequest, "login_or_token_required")
		return
	}
	if req.Hourly <= 0 {
		req.Hourly = 5
	}
	if req.Daily <= 0 {
		req.Daily = 150
	}
	if err := s.setUserCustomLimits(resellerID, login, req.Hourly, req.Daily); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	snap, _ := s.meterSnapshot(resellerID, login)
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"login":   login,
		"limits": map[string]any{
			"hourly_limit":     snap.HourlyLimit,
			"daily_limit":      snap.DailyLimit,
			"hourly_used":      snap.HourlyUsed,
			"daily_used":       snap.DailyUsed,
			"hourly_remaining": snap.HourlyRemaining,
			"daily_remaining":  snap.DailyRemaining,
		},
	})
}

func (s *server) handleDevResetMeters(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.DevAuth {
		writeErr(w, http.StatusNotFound, "dev_disabled")
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var req devLimitsReq
	_ = json.Unmarshal(body, &req)
	login := strings.TrimSpace(req.Login)
	resellerID := 1
	if tok := strings.TrimSpace(firstNonEmpty(req.Token, headerOr(r, "X-TM-Bar-Token"))); tok != "" {
		sess, err := s.lookupSession(tok)
		if err != nil || sess == nil {
			writeErr(w, http.StatusUnauthorized, "Unauthorized")
			return
		}
		login = sess.AmemberLogin
		resellerID = sess.ResellerID
	}
	if login == "" {
		writeErr(w, http.StatusBadRequest, "login_or_token_required")
		return
	}
	if err := s.resetUserMeters(resellerID, login); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	snap, _ := s.meterSnapshot(resellerID, login)
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"login":   login,
		"limits": map[string]any{
			"hourly_limit":     snap.HourlyLimit,
			"daily_limit":      snap.DailyLimit,
			"hourly_used":      snap.HourlyUsed,
			"daily_used":       snap.DailyUsed,
			"hourly_remaining": snap.HourlyRemaining,
			"daily_remaining":  snap.DailyRemaining,
		},
	})
}

// Local tm-check (AHREFS_BAR_DEV_AUTH=1) — mirrors aMember tm-check.php.
// Validates panel_session against admin_sessions in the shared panel DB.
// When aMember is wired, the real portal uses tm-check.php instead of this.
func (s *server) handleDevTMCheck(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.DevAuth {
		writeErr(w, http.StatusNotFound, "dev_disabled")
		return
	}
	key := r.Header.Get("X-TM-Check-Key")
	if key == "" {
		key = r.URL.Query().Get("key")
	}
	if !secureEqual(key, s.cfg.DevCheckSecret) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "forbidden"})
		return
	}

	cookie := r.Header.Get("Cookie")
	sessionVal := extractCookieValue(cookie, "panel_session")
	if sessionVal == "" {
		sessionVal = extractCookieValue(cookie, "PHPSESSID")
	}
	if sessionVal == "" {
		sessionVal = firstCookieValue(cookie)
	}
	if sessionVal == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "not_logged_in"})
		return
	}

	var uid int64
	var login, role, status, expires string
	err := s.db.QueryRow(`
SELECT o.id, o.username, o.role, o.status, s.expires_at
FROM admin_sessions s
JOIN operators o ON o.id = s.operator_id
WHERE s.token = ?
LIMIT 1`, sessionVal).Scan(&uid, &login, &role, &status, &expires)
	if err != nil || status != "active" || panelSessionExpired(expires) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "not_logged_in"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"user": map[string]any{
			"uid":   uid,
			"login": login,
			"email": login + "@panel.local",
			"name":  login,
			"role":  role,
		},
	})
}

func panelSessionExpired(raw string) bool {
	raw = strings.TrimSpace(raw)
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		exp, err := time.Parse(layout, raw)
		if err == nil {
			return !exp.After(time.Now())
		}
	}
	return true
}

func secureEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

func extractCookieValue(header, name string) string {
	parts := strings.Split(header, ";")
	prefix := name + "="
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(p, prefix))
		}
	}
	return ""
}

func firstCookieValue(header string) string {
	parts := strings.Split(header, ";")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if i := strings.IndexByte(p, '='); i > 0 {
			return strings.TrimSpace(p[i+1:])
		}
	}
	return ""
}

func short(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
