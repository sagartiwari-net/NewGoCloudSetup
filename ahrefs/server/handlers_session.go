package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type verifyRequest struct {
	Token       string `json:"token"`
	Portal      string `json:"portal"`
	SiteURL     string `json:"site_url"`
	CookieName  string `json:"cookie_name"`
	Fingerprint string `json:"fingerprint"`
	ResellerID  int    `json:"reseller_id"`
}

func (s *server) handlePortals(w http.ResponseWriter, r *http.Request) {
	resellerID := 0
	if n, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("resellerId"))); err == nil {
		resellerID = n
	}
	if resellerID <= 0 {
		if n, err := strconv.Atoi(strings.TrimSpace(r.Header.Get("X-TM-Bar-Reseller"))); err == nil {
			resellerID = n
		}
	}
	list, err := s.listEnabledPortalsPublic(resellerID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"reseller_id": resellerID,
		"portals":     list,
	})
}

func (s *server) handleSessionVerify(w http.ResponseWriter, r *http.Request) {
	if !s.allowVerifyRate(r) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"success": false, "valid": false, "error": "rate_limited", "deny": "Unauthorized",
		})
		return
	}

	token := headerOr(r, "X-TM-Bar-Token", "X-Tb-Token")
	portal := headerOr(r, "X-TM-Bar-Portal", "X-Tb-Portal")
	cookieName := headerOr(r, "X-TM-Bar-Cookie", "X-Tb-Cookie")
	fp := headerOr(r, "X-TM-Bar-FP", "X-Tb-FP")
	resellerID := 0
	if n, err := strconv.Atoi(strings.TrimSpace(headerOr(r, "X-TM-Bar-Reseller"))); err == nil {
		resellerID = n
	}

	if r.Body != nil {
		defer r.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if len(body) > 0 {
			var req verifyRequest
			if json.Unmarshal(body, &req) == nil {
				if token == "" {
					token = strings.TrimSpace(req.Token)
				}
				if portal == "" {
					portal = strings.TrimSpace(firstNonEmpty(req.Portal, req.SiteURL))
				}
				if cookieName == "" {
					cookieName = strings.TrimSpace(req.CookieName)
				}
				if fp == "" {
					fp = strings.TrimSpace(req.Fingerprint)
				}
				if resellerID <= 0 && req.ResellerID > 0 {
					resellerID = req.ResellerID
				}
			}
		}
	}

	token = strings.TrimSpace(token)
	portal = strings.TrimSpace(portal)
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"success": false, "valid": false, "error": "not_logged_in", "deny": "login",
		})
		return
	}

	// Fast path: already minted tb.* session
	if isBarSessionToken(token) {
		sess, err := s.lookupSession(token)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "db_error")
			return
		}
		if sess == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"success": false, "valid": false, "error": "Unauthorized", "deny": "Unauthorized",
			})
			return
		}
		if portal != "" {
			want := normalizeSiteURL(portal)
			have := normalizeSiteURL(sess.PortalURL)
			if want != "" && have != "" && want != have {
				writeJSON(w, http.StatusUnauthorized, map[string]any{
					"success": false, "valid": false, "error": "Unauthorized", "deny": "Unauthorized",
				})
				return
			}
		}
		ok, fpCode := s.bindOrCheckFingerprint(sess, fp)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"success": false, "valid": false, "error": fpCode, "deny": "Unauthorized",
			})
			return
		}
		s.writeVerifyOK(w, sess, token)
		return
	}

	if portal == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"success": false, "valid": false, "error": "portal_required", "deny": "Unauthorized",
		})
		return
	}

	p, err := s.findPortalBySiteURLForReseller(portal, resellerID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error")
		return
	}
	if p == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"success": false, "valid": false, "error": "portal_not_allowed", "deny": "Unauthorized",
		})
		return
	}
	if resellerID > 0 && p.ResellerID != resellerID {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"success": false, "valid": false, "error": "reseller_mismatch", "deny": "Unauthorized",
		})
		return
	}

	result, err := s.callMembershipCheck(p, token, cookieName)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "check_failed")
		return
	}
	if result == nil || !result.OK || result.User == nil {
		errCode := "Unauthorized"
		if result != nil && result.Error != "" {
			errCode = result.Error
		}
		deny := "Unauthorized"
		if errCode == "not_logged_in" {
			deny = "login"
		} else if errCode == "no_product" || errCode == "no_subscription" || errCode == "no_access" {
			deny = "no_access"
		}
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"success": false, "valid": false, "error": errCode, "deny": deny,
		})
		return
	}
	if want := parseProductIDs(p.ProductIDs); len(want) > 0 {
		have := membershipActiveProductIDs(result)
		if !productIDsIntersect(have, want) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"success": false, "valid": false, "error": "no_product", "deny": "no_access",
				"want_products": want, "have_products": have,
			})
			return
		}
	}

	u := result.User
	if s.cfg.RequireFingerprint && strings.TrimSpace(fp) == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"success": false, "valid": false, "error": "fingerprint_required", "deny": "Unauthorized",
		})
		return
	}
	minted, err := s.mintSession(p.ResellerID, p.ID, p.SiteURL, u.Login, u.Email, u.Name, u.UID, fp)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "mint_failed")
		return
	}
	sess := &barSession{
		Token:        minted,
		ResellerID:   p.ResellerID,
		AmemberLogin: u.Login,
		Email:        u.Email,
		Name:         u.Name,
		UID:          u.UID,
		PortalURL:    p.SiteURL,
	}
	s.writeVerifyOK(w, sess, minted)
}

func (s *server) writeVerifyOK(w http.ResponseWriter, sess *barSession, sessionToken string) {
	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"valid":         true,
		"session_token": sessionToken,
		"portal":        sess.PortalURL,
		"user": map[string]any{
			"uid":   sess.UID,
			"login": sess.AmemberLogin,
			"email": sess.Email,
			"name":  firstNonEmpty(sess.Name, sess.AmemberLogin),
		},
	})
}

// handleSessionLogout deletes the tb.* session so a portal logout cannot keep
// the bar authed forever. Extension also clears local storage / Ahrefs keys.
func (s *server) handleSessionLogout(w http.ResponseWriter, r *http.Request) {
	token := headerOr(r, "X-TM-Bar-Token", "Authorization")
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimSpace(token)
	if r.Body != nil {
		defer r.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		if len(body) > 0 {
			var req struct {
				Token string `json:"token"`
			}
			if json.Unmarshal(body, &req) == nil && token == "" {
				token = strings.TrimSpace(req.Token)
			}
		}
	}
	if token != "" {
		_ = s.revokeSession(token)
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *server) handleMe(w http.ResponseWriter, r *http.Request) {
	token := headerOr(r, "X-TM-Bar-Token", "Authorization")
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimSpace(token)
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"success": false, "error": "not_logged_in"})
		return
	}
	sess, code, err := s.lookupSessionChecked(r, token)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error")
		return
	}
	if sess == nil {
		errCode := code
		if errCode == "" {
			errCode = "Unauthorized"
		}
		writeJSON(w, http.StatusUnauthorized, map[string]any{"success": false, "error": errCode})
		return
	}
	snap, err := s.meterSnapshot(sess.ResellerID, sess.AmemberLogin)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "limits_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"user": map[string]any{
			"uid":   sess.UID,
			"login": sess.AmemberLogin,
			"email": sess.Email,
			"name":  firstNonEmpty(sess.Name, sess.AmemberLogin),
		},
		"portal":      sess.PortalURL,
		"reseller_id": sess.ResellerID,
		"limits": map[string]any{
			"hourly_limit":     snap.HourlyLimit,
			"daily_limit":      snap.DailyLimit,
			"hourly_used":      snap.HourlyUsed,
			"daily_used":       snap.DailyUsed,
			"hourly_remaining": snap.HourlyRemaining,
			"daily_remaining":  snap.DailyRemaining,
		},
		"expires_at": sess.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z"),
	})
}

func headerOr(r *http.Request, names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(r.Header.Get(n)); v != "" {
			return v
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
