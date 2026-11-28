package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type membershipUser struct {
	UID    int64  `json:"uid"`
	Login  string `json:"login"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Portal string `json:"portal"`
	Label  string `json:"label"`
}

type membershipCheckResult struct {
	OK       bool            `json:"ok"`
	Error    string          `json:"error"`
	User     *membershipUser `json:"user"`
	Products []int           `json:"products"`
	PlanIDs  []int           `json:"plan_ids"`
}

func membershipActiveProductIDs(r *membershipCheckResult) []int {
	if r == nil {
		return nil
	}
	if len(r.Products) > 0 {
		return r.Products
	}
	return r.PlanIDs
}

func productIDsIntersect(have []int, want []int) bool {
	if len(want) == 0 {
		return true // portal allows any active member
	}
	if len(have) == 0 {
		return false
	}
	set := map[int]bool{}
	for _, id := range have {
		set[id] = true
	}
	for _, id := range want {
		if set[id] {
			return true
		}
	}
	return false
}

func parseProductIDs(raw string) []int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var ids []int
	for _, p := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	}) {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err == nil {
			ids = append(ids, n)
		}
	}
	return ids
}

func buildCookieHeader(sessionID, preferred string) string {
	names := []string{
		"PHPSESSID", "panel_session", "amember", "amember_nr", "amember_ru", "amember_r", "amember_sess",
		"semrushtoolz-session", "laravel_session",
	}
	preferred = strings.TrimSpace(preferred)
	if preferred != "" {
		names = append([]string{preferred}, names...)
	}
	seen := map[string]bool{}
	var parts []string
	for _, n := range names {
		if seen[n] {
			continue
		}
		seen[n] = true
		parts = append(parts, n+"="+sessionID)
	}
	return strings.Join(parts, "; ")
}

func (s *server) callMembershipCheck(portal *portalRow, sessionID, cookieName string) (*membershipCheckResult, error) {
	checkURL := strings.TrimSpace(portal.CheckURL)
	if checkURL == "" || sessionID == "" {
		return &membershipCheckResult{OK: false, Error: "bad_request"}, nil
	}

	u, err := url.Parse(checkURL)
	if err != nil {
		return nil, err
	}
	// Do NOT send product_ids to tm-check — let it auth the member and return
	// active plan_ids. Portal product filter is applied here after the response
	// (avoids false no_product when portal still lists an old/expired product id).

	preferred := strings.TrimSpace(cookieName)
	if preferred == "" {
		preferred = portal.CookieName
	}
	payload, _ := json.Marshal(map[string]any{
		"token":       sessionID,
		"cookie_name": preferred,
	})
	req, err := http.NewRequest(http.MethodPost, u.String(), strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", buildCookieHeader(sessionID, preferred))
	req.Header.Set("Accept", "application/json")
	if secret := strings.TrimSpace(portal.SharedSecret); secret != "" {
		req.Header.Set("X-TM-Check-Key", secret)
	}

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return &membershipCheckResult{OK: false, Error: "check_unreachable"}, nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var parsed membershipCheckResult
	if err := json.Unmarshal(body, &parsed); err != nil {
		if resp.StatusCode != 200 {
			return &membershipCheckResult{OK: false, Error: fmt.Sprintf("check_http_%d", resp.StatusCode)}, nil
		}
		return &membershipCheckResult{OK: false, Error: "check_bad_json"}, nil
	}
	if resp.StatusCode != 200 && !parsed.OK {
		if parsed.Error == "" {
			parsed.Error = fmt.Sprintf("check_http_%d", resp.StatusCode)
		}
		return &parsed, nil
	}
	if parsed.OK && parsed.User != nil {
		if parsed.User.Portal == "" {
			parsed.User.Portal = portal.SiteURL
		}
		if parsed.User.Label == "" {
			parsed.User.Label = portal.Name
		}
		if parsed.User.Name == "" {
			parsed.User.Name = parsed.User.Login
		}
	}
	return &parsed, nil
}
