package zik

// Recipe logs in at api.zikanalytics.com and stores SessionToken plus the access token.
// The proxy reads the access token from localStorage inside the saved cookie JSON.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"panel-api/automation"
	"strings"
	"time"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

var (
	loginURL = "https://api.zikanalytics.com/auth/login"
	httpWait = 25 * time.Second
)

type Recipe struct{}

func (Recipe) PortalHost() string { return "zikanalytics.com" }

func (Recipe) Check(_ context.Context, cookies []automation.Cookie) (bool, []automation.Cookie, error) {
	token := valueOf(cookies, "access")
	session := valueOf(cookies, "SessionToken")
	if token == "" || session == "" || jwtExpired(token) {
		return false, nil, nil
	}
	return true, cookies, nil
}

func (Recipe) Login(ctx context.Context, username, password string) ([]automation.Cookie, error) {
	body, err := json.Marshal(map[string]string{"email": username, "password": password})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://app.zikanalytics.com")
	req.Header.Set("Referer", "https://app.zikanalytics.com/")
	client := &http.Client{Timeout: httpWait}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, automation.ErrRejected
	}
	if resp.StatusCode != http.StatusOK {
		return nil, automation.ErrLoginPage
	}
	var parsed struct {
		Success bool `json:"success"`
		Result  struct {
			AccessToken string `json:"accessToken"`
		} `json:"result"`
	}
	if json.Unmarshal(raw, &parsed) != nil || !parsed.Success || strings.TrimSpace(parsed.Result.AccessToken) == "" {
		return nil, automation.ErrRejected
	}
	session := ""
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "SessionToken" && cookie.Value != "" {
			session = cookie.Value
			break
		}
	}
	if session == "" {
		return nil, automation.ErrNoCookie
	}
	exp := float64(time.Now().Add(365 * 24 * time.Hour).Unix())
	lax := "lax"
	return []automation.Cookie{
		{
			Name: "access", Value: parsed.Result.AccessToken, Domain: "app.zikanalytics.com",
			HostOnly: true, Path: "/", Session: true,
		},
		{
			Name: "SessionToken", Value: session, Domain: ".zikanalytics.com", Path: "/",
			HTTPOnly: true, Secure: true, ExpirationDate: &exp, SameSite: &lax,
		},
	}, nil
}

func (Recipe) Capture(_ context.Context, portal []automation.Cookie, _ string) ([]automation.Cookie, error) {
	if valueOf(portal, "access") == "" || valueOf(portal, "SessionToken") == "" {
		return nil, automation.ErrNoCookie
	}
	return portal, nil
}

func (Recipe) Document(cookies []automation.Cookie) (string, error) {
	access := valueOf(cookies, "access")
	kept := make([]automation.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie.Name == "access" || cookie.Value == "" {
			continue
		}
		kept = append(kept, cookie)
	}
	if access == "" || !hasName(kept, "SessionToken") {
		return "", automation.ErrNoCookie
	}
	payload := map[string]any{
		"referer": "https://app.zikanalytics.com/dashboard",
		"cookies": kept,
		"storage": map[string]any{
			"localStorage": map[string]string{
				"access":             access,
				"crDefaultTab":       "ai suggestions",
				"defaultMarketplace": "",
				"stripeError":        "false",
				"zk_geo_iso":         "US",
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func jwtExpired(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return true
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		raw, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return true
		}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(raw, &claims) != nil || claims.Exp == 0 {
		return true
	}
	return time.Now().Add(2*time.Minute).Unix() >= claims.Exp
}

func valueOf(cookies []automation.Cookie, name string) string {
	for _, cookie := range cookies {
		if cookie.Name == name && strings.TrimSpace(cookie.Value) != "" {
			return cookie.Value
		}
	}
	return ""
}

func hasName(cookies []automation.Cookie, name string) bool {
	return valueOf(cookies, name) != ""
}
