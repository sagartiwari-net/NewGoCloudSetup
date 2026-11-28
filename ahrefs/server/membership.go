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
	OK    bool            `json:"ok"`
	Error string          `json:"error"`
	User  *membershipUser `json:"user"`
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
	q := u.Query()
	if ids := parseProductIDs(portal.ProductIDs); len(ids) > 0 {
		var ss []string
		for _, id := range ids {
			ss = append(ss, strconv.Itoa(id))
		}
		joined := strings.Join(ss, ",")
		// tm-check.php accepts both; keep products for older templates.
		q.Set("product_ids", joined)
		q.Set("products", joined)
	}
	u.RawQuery = q.Encode()

	preferred := strings.TrimSpace(cookieName)
	if preferred == "" {
		preferred = portal.CookieName
	}
	payload, _ := json.Marshal(map[string]any{
		"token":       sessionID,
		"cookie_name": preferred,
		"product_ids": q.Get("product_ids"),
		"products":    q.Get("products"),
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
