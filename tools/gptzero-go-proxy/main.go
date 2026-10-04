package main

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
)

// GPTZero local proxy — panel access + fingerprint device lock + DB cookie.

type Config struct {
	Port                   string   `json:"port"`
	TargetURL              string   `json:"target_url"`
	PublicHost             string   `json:"public_host"`
	PublicScheme           string   `json:"public_scheme"`
	CookieFile             string   `json:"cookie_file"`
	ToolName               string   `json:"tool_name"`
	UserAgent              string   `json:"user_agent"`
	ExtraDomains           []string `json:"extra_domains"`
	SupabaseURL            string   `json:"supabase_url"`
	SupabaseKey            string   `json:"supabase_key"`
	PanelDB                string   `json:"panel_db"`
	BypassAuth             bool     `json:"bypass_auth"`
	HomePath               string   `json:"home_path"`
	SessionDurationMinutes int      `json:"session_duration_minutes"`
	MemberAreaURL          string   `json:"member_area_url"`
	BlockedPaths           []string `json:"blocked_paths"`
}

type goAutoExport struct {
	Cookies json.RawMessage `json:"cookies"`
	Storage struct {
		LocalStorage map[string]string `json:"localStorage"`
		Cookies      json.RawMessage   `json:"cookies"`
	} `json:"storage"`
}

var (
	cfgMu   sync.RWMutex
	cfg     Config
	sessMu  sync.RWMutex
	sessRaw string
	sessMT  time.Time
	refreshMu sync.Mutex
)

func main() {
	path := os.Getenv("GPTZERO_CONFIG")
	if path == "" {
		if _, err := os.Stat("config.local.json"); err == nil {
			path = "config.local.json"
		} else {
			path = "config.json"
		}
	}
	if err := reloadConfig(path); err != nil {
		log.Fatalf("config: %v", err)
	}
	go func() {
		for {
			time.Sleep(2 * time.Second)
			_ = reloadConfig(path)
		}
	}()

	c := getConfig()
	mode := "panel+fingerprint"
	if c.BypassAuth || !usesPanelAccountMode(c) {
		mode = "local cookie.txt (bypass)"
	}
	log.Printf("GPTZero local proxy → %s on :%s (%s)", c.TargetURL, c.Port, mode)
	if usesPanelAccountMode(c) {
		if _, err := openPanelDB(c); err != nil {
			log.Printf("WARNING: panel DB open failed: %v", err)
		} else {
			log.Printf("panel DB ready (%s)", c.PanelDB)
		}
	}
	if raw := loadSession(); raw == "" {
		log.Printf("WARNING: cookie.txt empty — used only when bypass_auth=true")
	} else {
		log.Printf("cookie.txt present (%d bytes)", len(raw))
		if _, err := ensureFreshSessionRaw(raw, 0, ""); err != nil {
			log.Printf("WARNING: supabase refresh failed: %v", err)
		} else {
			log.Printf("session token refreshed OK")
		}
	}
	go func() {
		for {
			time.Sleep(10 * time.Minute)
			if _, err := ensureFreshSessionRaw(loadSession(), 0, ""); err != nil {
				log.Printf("[AUTH] background refresh: %v", err)
			}
		}
	}()

	_ = os.MkdirAll("cdn-cache", 0o755)
	startCDNCacheSweep()

	mux := http.NewServeMux()
	mux.HandleFunc("/access", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if usesPanelAccountMode(cfg) {
			servePanelAccess(w, r, cfg)
			return
		}
		renderAccessDeniedPage(w, cfg)
	})
	mux.HandleFunc("/api/device-bind", deviceBindHandler)
	mux.HandleFunc("/api/user-limits", userLimitsAPIHandler)
	mux.HandleFunc("/tm-device-sw.js", serveDeviceSW)
	mux.HandleFunc("/__tm_access_denied", serveAccessDeniedHTML)
	mux.HandleFunc("/", proxyHandler)
	addr := ":" + c.Port
	log.Fatal(http.ListenAndServe(addr, mux))
}

func reloadConfig(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var next Config
	if err := json.Unmarshal(b, &next); err != nil {
		return err
	}
	if next.Port == "" {
		next.Port = "5301"
	}
	if next.TargetURL == "" {
		next.TargetURL = "https://app.gptzero.me"
	}
	if next.PublicHost == "" {
		next.PublicHost = "127.0.0.1:" + next.Port
	}
	if next.PublicScheme == "" {
		next.PublicScheme = "http"
	}
	if next.CookieFile == "" {
		next.CookieFile = "cookie.txt"
	}
	if next.UserAgent == "" {
		next.UserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	}
	if len(next.ExtraDomains) == 0 {
		next.ExtraDomains = []string{
			"https://lydqhgdzhvsqlcobdfxi.supabase.co",
			"https://api.gptzero.me",
			"https://cdn.gptzero.me",
			"https://auth.gptzero.me",
			"https://gptzero.me",
		}
	}
	if next.SupabaseURL == "" {
		next.SupabaseURL = "https://lydqhgdzhvsqlcobdfxi.supabase.co"
	}
	if next.SupabaseKey == "" {
		// GPTZero dashboard publishable key (also embedded in app JS).
		next.SupabaseKey = "sb_publishable_-TRlvcmoZ3y9LvkQys7Vcg_TImPL6et"
	}
	if next.HomePath == "" {
		next.HomePath = "/"
	}
	if next.SessionDurationMinutes <= 0 {
		next.SessionDurationMinutes = 120
	}
	if next.PanelDB == "" {
		if _, err := os.Stat("../pending-tools/panel-api/data/panel.db"); err == nil {
			next.PanelDB = "../pending-tools/panel-api/data/panel.db"
		}
	}
	cfgMu.Lock()
	cfg = next
	cfgMu.Unlock()
	return nil
}

func getConfig() Config {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return cfg
}

func loadSession() string {
	c := getConfig()
	info, err := os.Stat(c.CookieFile)
	if err != nil {
		return ""
	}
	sessMu.RLock()
	if !info.ModTime().After(sessMT) && sessRaw != "" {
		raw := sessRaw
		sessMu.RUnlock()
		return raw
	}
	sessMu.RUnlock()
	b, err := os.ReadFile(c.CookieFile)
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(string(b))
	sessMu.Lock()
	sessRaw = raw
	sessMT = info.ModTime()
	sessMu.Unlock()
	return raw
}

func parseSession(raw string) (cookieHeader string, ls map[string]string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	var g goAutoExport
	if json.Unmarshal([]byte(raw), &g) != nil {
		return "", nil
	}
	ls = g.Storage.LocalStorage
	cookieHeader = cookiesToHeader(g.Cookies)
	if cookieHeader == "" {
		cookieHeader = cookiesToHeader(g.Storage.Cookies)
	}
	return cookieHeader, ls
}

func cookiesToHeader(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var list []map[string]interface{}
	if json.Unmarshal(raw, &list) != nil {
		return ""
	}
	now := time.Now().Unix()
	var parts []string
	for _, c := range list {
		name, _ := c["name"].(string)
		val, _ := c["value"].(string)
		if name == "" || val == "" {
			continue
		}
		if exp, ok := c["expirationDate"].(float64); ok && exp > 0 && int64(exp) < now {
			continue
		}
		parts = append(parts, name+"="+val)
	}
	return strings.Join(parts, "; ")
}

func accessTokenFromLS(ls map[string]string) string {
	for k, v := range ls {
		if !strings.HasPrefix(k, "sb-") || !strings.HasSuffix(k, "-auth-token") {
			continue
		}
		var sess map[string]interface{}
		if json.Unmarshal([]byte(v), &sess) != nil {
			continue
		}
		if t, _ := sess["access_token"].(string); t != "" {
			return t
		}
	}
	return ""
}

func refreshTokenFromLS(ls map[string]string) string {
	for k, v := range ls {
		if !strings.HasPrefix(k, "sb-") || !strings.HasSuffix(k, "-auth-token") {
			continue
		}
		var sess map[string]interface{}
		if json.Unmarshal([]byte(v), &sess) != nil {
			continue
		}
		if t, _ := sess["refresh_token"].(string); t != "" {
			return t
		}
	}
	return ""
}

func jwtExpUnix(accessToken string) int64 {
	parts := strings.Split(accessToken, ".")
	if len(parts) < 2 {
		return 0
	}
	payload := parts[1]
	for len(payload)%4 != 0 {
		payload += "="
	}
	raw, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		raw, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return 0
		}
	}
	var claims map[string]interface{}
	if json.Unmarshal(raw, &claims) != nil {
		return 0
	}
	if v, ok := claims["exp"].(float64); ok {
		return int64(v)
	}
	return 0
}

// ensureFreshSessionRaw refreshes the Supabase JWT when expired/near expiry.
// accountID > 0 writes the refreshed GoAuto blob back into panel accounts.
// proxyStr, when set, dials Supabase through the panel account proxy.
func ensureFreshSessionRaw(raw string, accountID int, proxyStr string) (string, error) {
	refreshMu.Lock()
	defer refreshMu.Unlock()

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty session cookie")
	}
	if isDeadRefresh(accountID) {
		return raw, fmt.Errorf("refresh_token_already_used (cached)")
	}
	_, ls := parseSession(raw)
	token := accessTokenFromLS(ls)
	rt := refreshTokenFromLS(ls)
	if rt == "" {
		return raw, fmt.Errorf("no refresh_token in session")
	}
	exp := jwtExpUnix(token)
	if token != "" && exp > time.Now().Unix()+300 {
		return raw, nil
	}

	c := getConfig()
	body, _ := json.Marshal(map[string]string{"refresh_token": rt})
	endpoint := strings.TrimRight(c.SupabaseURL, "/") + "/auth/v1/token?grant_type=refresh_token"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return raw, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "+c.SupabaseKey)
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := upstreamHTTPClient(proxyStr).Do(req)
	if err != nil {
		return raw, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		bodyStr := string(respBody)
		if resp.StatusCode == 400 || strings.Contains(strings.ToLower(bodyStr), "refresh_token_already_used") ||
			strings.Contains(strings.ToLower(bodyStr), "invalid refresh token") {
			markDeadRefresh(accountID)
		}
		return raw, fmt.Errorf("supabase refresh HTTP %d: %s", resp.StatusCode, truncate(bodyStr, 180))
	}
	var out map[string]interface{}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return raw, err
	}
	newAT, _ := out["access_token"].(string)
	if newAT == "" {
		return raw, fmt.Errorf("refresh response missing access_token")
	}
	newRT, _ := out["refresh_token"].(string)
	if newRT == "" {
		newRT = rt
	}

	var g map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		return raw, err
	}
	storage, _ := g["storage"].(map[string]interface{})
	if storage == nil {
		storage = map[string]interface{}{}
		g["storage"] = storage
	}
	lsMap, _ := storage["localStorage"].(map[string]interface{})
	if lsMap == nil {
		lsMap = map[string]interface{}{}
		storage["localStorage"] = lsMap
	}
	authKey := "sb-lydqhgdzhvsqlcobdfxi-auth-token"
	var sess map[string]interface{}
	if prev, _ := lsMap[authKey].(string); prev != "" {
		_ = json.Unmarshal([]byte(prev), &sess)
	}
	if sess == nil {
		sess = map[string]interface{}{}
	}
	sess["access_token"] = newAT
	sess["refresh_token"] = newRT
	sess["token_type"] = "bearer"
	if v, ok := out["expires_at"]; ok {
		sess["expires_at"] = v
	}
	if v, ok := out["expires_in"]; ok {
		sess["expires_in"] = v
	}
	if v, ok := out["user"]; ok {
		sess["user"] = v
	}
	sessBytes, _ := json.Marshal(sess)
	lsMap[authKey] = string(sessBytes)

	if cookies, ok := g["cookies"].([]interface{}); ok {
		for _, item := range cookies {
			m, _ := item.(map[string]interface{})
			if m != nil && m["name"] == "accessToken4" {
				m["value"] = newAT
			}
		}
	}

	updated, err := json.Marshal(g)
	if err != nil {
		return raw, err
	}
	_ = os.WriteFile(c.CookieFile, updated, 0644)
	if accountID > 0 && usesPanelAccountMode(c) {
		if db, dbErr := openPanelDB(c); dbErr == nil {
			now := time.Now().UTC().Format(time.RFC3339)
			_, _ = db.Exec(`UPDATE accounts SET cookie=?, cookie_updated_at=? WHERE id=?`, string(updated), now, accountID)
		}
	}
	sessMu.Lock()
	sessRaw = string(updated)
	if info, err := os.Stat(c.CookieFile); err == nil {
		sessMT = info.ModTime()
	}
	sessMu.Unlock()
	log.Printf("[AUTH] refreshed access token (exp in ~%dm)", (jwtExpUnix(newAT)-time.Now().Unix())/60)
	return string(updated), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func planFromSession(cookieHeader string, ls map[string]string) string {
	p := ""
	for _, part := range strings.Split(cookieHeader, "; ") {
		if strings.HasPrefix(part, "plan=") {
			p = strings.TrimPrefix(part, "plan=")
			break
		}
	}
	if p == "" {
		p = strings.TrimSpace(ls["plan"])
	}
	return normalizeGPTZeroPlan(p)
}

// normalizeGPTZeroPlan keeps plagiarism unlocked — SPA gates it when PLAN is Free/empty.
func normalizeGPTZeroPlan(p string) string {
	p = strings.TrimSpace(p)
	low := strings.ToLower(p)
	if p == "" || low == "free" || strings.HasPrefix(low, "essential") ||
		low == "null" || low == "undefined" {
		return "Premium (Annual)"
	}
	if low == "premium" {
		return "Premium (Annual)"
	}
	return p
}

// forceGPTZeroPremiumPlan rewrites profile/subscription JSON so plagiarism is not upgrade-walled.
func forceGPTZeroPremiumPlan(path string, body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	trim := bytes.TrimSpace(body)
	if len(trim) == 0 || (trim[0] != '{' && trim[0] != '[') {
		return body
	}
	pathL := strings.ToLower(path)
	// Only touch profile / plan payloads — never scan/document JSON (breaks side panels).
	interesting := strings.Contains(pathL, "profile") || strings.Contains(pathL, "subscription") ||
		strings.Contains(pathL, "/plans") || strings.Contains(pathL, "rest/v1/profiles") ||
		strings.Contains(pathL, "rest/v1/plans") ||
		(bytes.Contains(trim, []byte(`"full_plan"`)) && bytes.Contains(trim, []byte(`"email"`)))
	if !interesting {
		return body
	}
	var v interface{}
	if err := json.Unmarshal(trim, &v); err != nil {
		return body
	}
	if !forcePremiumPlanValue(v) {
		return body
	}
	out, err := json.Marshal(v)
	if err != nil {
		return body
	}
	return out
}

func forcePremiumPlanValue(v interface{}) bool {
	changed := false
	switch t := v.(type) {
	case map[string]interface{}:
		if plan, ok := t["plan"].(string); ok {
			n := normalizeGPTZeroPlan(plan)
			if n != plan {
				t["plan"] = n
				changed = true
			}
		}
		if fp, ok := t["full_plan"].(map[string]interface{}); ok {
			if name, _ := fp["name"].(string); normalizeGPTZeroPlan(name) != name || name == "" {
				fp["name"] = "Premium (Annual)"
				changed = true
			}
			t["plan"] = "Premium (Annual)"
			changed = true
		}
		if planObj, ok := t["plan"].(map[string]interface{}); ok {
			if name, _ := planObj["name"].(string); normalizeGPTZeroPlan(name) != name || name == "" {
				planObj["name"] = "Premium (Annual)"
				changed = true
			}
		}
		for _, child := range t {
			if forcePremiumPlanValue(child) {
				changed = true
			}
		}
	case []interface{}:
		for _, child := range t {
			if forcePremiumPlanValue(child) {
				changed = true
			}
		}
	}
	return changed
}

// supabaseStorageKey matches supabase-js: `sb-${hostname.split(".")[0]}-auth-token`.
// After we rewrite supabase URL to 127.0.0.1, the SPA looks for sb-127-auth-token,
// not the original sb-<projectRef>-auth-token from GoAuto.
func supabaseStorageKey(publicHost string) string {
	host := publicHost
	if h, _, err := net.SplitHostPort(publicHost); err == nil {
		host = h
	}
	host = strings.TrimSpace(host)
	if host == "" {
		host = "127.0.0.1"
	}
	first := strings.Split(host, ".")[0]
	if first == "" {
		first = host
	}
	return "sb-" + first + "-auth-token"
}

func lsJSONForBrowser(ls map[string]string, plan, publicHost string) []byte {
	out := map[string]string{}
	for k, v := range ls {
		// Skip huge analytics blobs that bloat inject
		if strings.HasPrefix(k, "AMP_") || strings.HasPrefix(k, "amplitude.") ||
			strings.HasPrefix(k, "EXP_") || k == "gbFeaturesCache" {
			continue
		}
		out[k] = v
	}
	// Keep auth token but bump expires so SPA does not thrash refresh (GoAuto RT often truncated).
	var bumpedAuth string
	for k, v := range out {
		if !strings.HasPrefix(k, "sb-") || !strings.HasSuffix(k, "-auth-token") {
			continue
		}
		var sess map[string]interface{}
		if json.Unmarshal([]byte(v), &sess) != nil {
			continue
		}
		// Keep SPA from thrashing refresh; server still refreshes real JWT for API calls.
		sess["expires_at"] = float64(time.Now().Unix() + 7*24*3600)
		sess["expires_in"] = float64(7 * 24 * 3600)
		if b, err := json.Marshal(sess); err == nil {
			out[k] = string(b)
			bumpedAuth = string(b)
		}
	}
	// Alias under the key supabase-js derives from the rewritten proxy hostname.
	if bumpedAuth != "" {
		out[supabaseStorageKey(publicHost)] = bumpedAuth
		// Common local variants
		out["sb-127-auth-token"] = bumpedAuth
		out["sb-localhost-auth-token"] = bumpedAuth
	}
	out["plan"] = normalizeGPTZeroPlan(plan)
	if traits := strings.TrimSpace(out["_ca_user_traits"]); traits != "" {
		var m map[string]interface{}
		if json.Unmarshal([]byte(traits), &m) == nil {
			m["plan"] = normalizeGPTZeroPlan(plan)
			if b, err := json.Marshal(m); err == nil {
				out["_ca_user_traits"] = string(b)
			}
		}
	}
	out["hasViewedPostLoginOnboarding"] = "true"
	out["backToSchoolPromoPopupDismissed"] = "true"
	out["shouldShowExtensionOnboarding"] = "false"
	out["redirectToFreeTrial"] = "false"
	out["anonymousCheckoutPlan"] = "null"
	out["isInitialSession"] = "false"
	b, err := json.Marshal(out)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func isBlockedPath(path string, blocked []string) bool {
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		path = "/"
	}
	for _, p := range blocked {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = strings.TrimSuffix(p, "/")
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	c := getConfig()
	path := r.URL.Path

	// Soft-map auth pages to home (no 302 — avoids reload loops).
	if p := strings.TrimSuffix(path, "/"); p == "/login" || p == "/signup" {
		path = "/"
		r.URL.Path = "/"
		r.URL.RawQuery = ""
	}

	accID := 0
	accountName := ""
	accountUA := ""
	accountProxy := ""
	panelUser := ""
	if usesPanelAccountMode(c) && !c.BypassAuth {
		name, err := panelSessionUsername(r)
		if err != nil {
			log.Printf("[AUTH] Denied path=%s ip=%s host=%s err=%v", path, realClientIP(r), r.Host, err)
			if wantsLightDenied(r, path) {
				renderAccessDeniedPage(w, c)
			} else {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"access_denied","message":"Open this tool again from your access link."}`))
			}
			return
		}
		panelUser = name
		if rejectPanelDevice(w, r, c) {
			return
		}
		if tok := ctSessionToken(r); tok != "" {
			if acc, err := loadPanelSessionAccount(c, tok); err == nil {
				accID = acc.ID
				accountName = acc.Name
				accountUA = strings.TrimSpace(acc.UserAgent)
				accountProxy = strings.TrimSpace(acc.Proxy)
				if accountProxy != "" {
					if px := parseProxyString(accountProxy); px != nil {
						log.Printf("[PROXY] account=%s via %s://%s", acc.Name, px.Scheme, px.Host)
					}
				}
			}
		}
	}

	if isBlockedPath(path, c.BlockedPaths) {
		log.Printf("[BLOCK] user=%s path=%s", panelUser, path)
		recordBlockedPath(c, r, panelUser)
		home := c.HomePath
		if home == "" {
			home = "/"
		}
		http.Redirect(w, r, home, http.StatusFound)
		return
	}

	// Disk CDN cache (JS/CSS/fonts/images) — server HIT + browser Cache-Control.
	cdnKey := cdnCacheKey(r)
	if serveCachedCDN(w, r) {
		return
	}
	if cdnKey != "" {
		defer completeCDNFlight(cdnKey)
	}

	// Quiet stubs that spam during boot.
	if r.Method == http.MethodPost && strings.Contains(path, "/v3/auth/extension-session/revoke") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
		return
	}
	if r.Method == http.MethodPost && strings.Contains(path, "/auth/v1/signup") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"signup_disabled"}`))
		return
	}

	raw, _ := sessionRawForRequest(r, c)
	if refreshed, err := ensureFreshSessionRaw(raw, accID, accountProxy); err == nil {
		raw = refreshed
	} else if isProxyDialError(err) {
		log.Printf("[PROXY] auth refresh dial failed: %v", err)
		recordProxyFailure(c, r, panelUser, err.Error())
		renderProxyProblem(w, r)
		return
	} else if usesPanelAccountMode(c) && !c.BypassAuth && accID > 0 && gptzeroAuthRefreshFailed(err) {
		log.Printf("[AUTH] session refresh failed account=%s(%d): %v", accountName, accID, err)
		if tryGPTZeroAccountSwap(w, r, c, panelUser, accID, accountName, "supabase_refresh:"+err.Error()) {
			return
		}
	}
	if raw == "" {
		raw = loadSession()
	}
	cookieHdr, ls := parseSession(raw)
	token := accessTokenFromLS(ls)
	plan := planFromSession(cookieHdr, ls)
	if accountUA != "" {
		c.UserAgent = accountUA
	}

	// Browser refresh → real Supabase (short GPTZero RTs are valid). Serve
	// server-refreshed session only when client posts a dummy/missing refresh.
	if r.Method == http.MethodPost && strings.Contains(path, "/auth/v1/token") && token != "" {
		bodyPeek, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		r.Body = io.NopCloser(bytes.NewReader(bodyPeek))
		var bodyObj map[string]interface{}
		_ = json.Unmarshal(bodyPeek, &bodyObj)
		reqRT, _ := bodyObj["refresh_token"].(string)
		grant := r.URL.Query().Get("grant_type")
		if grant == "" {
			grant, _ = bodyObj["grant_type"].(string)
		}
		if grant == "refresh_token" && (reqRT == "" || strings.HasPrefix(reqRT, "dummy_")) {
			resp := map[string]interface{}{
				"access_token":  token,
				"token_type":    "bearer",
				"expires_in":    3600,
				"expires_at":    time.Now().Unix() + 3600,
				"refresh_token": refreshTokenFromLS(ls),
			}
			for k, v := range ls {
				if strings.HasPrefix(k, "sb-") && strings.HasSuffix(k, "-auth-token") {
					var sess map[string]interface{}
					if json.Unmarshal([]byte(v), &sess) == nil {
						if u := sess["user"]; u != nil {
							resp["user"] = u
						}
						if exp, ok := sess["expires_at"]; ok {
							resp["expires_at"] = exp
						}
						if ei, ok := sess["expires_in"]; ok {
							resp["expires_in"] = ei
						}
					}
				}
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
	}

	target, _ := url.Parse(c.TargetURL)
	upstream := *r.URL
	upstream.Scheme = target.Scheme
	upstream.Host = target.Host

	// Route /extra-cdn-N/* and /api-proxy/*
	isAPI := false
	for i, extra := range c.ExtraDomains {
		prefix := fmt.Sprintf("/extra-cdn-%d/", i)
		if strings.HasPrefix(path, prefix) {
			host := strings.TrimPrefix(strings.TrimPrefix(extra, "https://"), "http://")
			host = strings.Split(host, "/")[0]
			upstream.Scheme = "https"
			upstream.Host = host
			upstream.Path = "/" + strings.TrimPrefix(path, prefix)
			if host == "api.gptzero.me" {
				isAPI = true
			}
			break
		}
	}
	if strings.HasPrefix(path, "/api-proxy/") {
		upstream.Scheme = "https"
		upstream.Host = "api.gptzero.me"
		upstream.Path = "/" + strings.TrimPrefix(path, "/api-proxy/")
		isAPI = true
	}
	if strings.HasPrefix(path, "/v2/") || strings.HasPrefix(path, "/v3/") {
		upstream.Scheme = "https"
		upstream.Host = "api.gptzero.me"
		isAPI = true
	}
	// Wild gptzero subdomains (analytics etc.)
	if strings.HasPrefix(path, "/extra-cdn-wild/") {
		rest := strings.TrimPrefix(path, "/extra-cdn-wild/")
		host, sub, ok := strings.Cut(rest, "/")
		if !ok {
			host, sub = rest, ""
		}
		if strings.HasSuffix(host, ".gptzero.me") {
			upstream.Scheme = "https"
			upstream.Host = host
			if sub == "" {
				upstream.Path = "/"
			} else {
				upstream.Path = "/" + sub
			}
		}
	}

	// Word-credit gate: 1 word = 1 credit per billable scan/AI POST.
	billAmount := 0
	reqBody := r.Body
	if usesPanelAccountMode(c) && !c.BypassAuth && panelUser != "" &&
		isBillableGPTZeroPath(r.Method, path) {
		bodyBytes, _ := io.ReadAll(io.LimitReader(r.Body, 8<<20))
		_ = r.Body.Close()
		reqBody = io.NopCloser(bytes.NewReader(bodyBytes))
		billAmount = creditAmountFromBody(bodyBytes)
		if !panelCreditsAllowAmount(panelUser, billAmount) {
			rejectCreditLimit(w, r, c)
			return
		}
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstream.String(), reqBody)
	if err != nil {
		http.Error(w, "bad request", 500)
		return
	}
	for k, vv := range r.Header {
		lk := strings.ToLower(k)
		if lk == "host" || lk == "connection" || lk == "content-length" ||
			lk == "x-device-fp" || lk == "x-device-proof" ||
			lk == "x-forwarded-for" || lk == "x-real-ip" {
			continue
		}
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Del("Accept-Encoding")
	req.Header.Set("Accept-Encoding", "gzip, deflate")

	hostOnly := upstream.Host
	if h, _, err := net.SplitHostPort(hostOnly); err == nil {
		hostOnly = h
	}
	sendAuth := hostOnly == target.Hostname() ||
		strings.HasSuffix(hostOnly, ".gptzero.me") ||
		strings.Contains(hostOnly, "supabase.co") ||
		hostOnly == "api.gptzero.me"
	if sendAuth && cookieHdr != "" {
		req.Header.Set("Cookie", cookieHdr)
	}
	// Always attach a fresh server token for GPTZero/Supabase API — browser LS JWT may be stale.
	if sendAuth && token != "" && (isAPI || hostOnly == "api.gptzero.me" || strings.Contains(hostOnly, "supabase.co")) {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if isAPI || hostOnly == "api.gptzero.me" {
		req.Header.Set("Origin", "https://app.gptzero.me")
		req.Header.Set("Referer", "https://app.gptzero.me/")
	}

	resp, err := upstreamHTTPClient(accountProxy).Do(req)
	if err != nil {
		log.Printf("[UPSTREAM] %s %s: %v", r.Method, path, err)
		if isProxyDialError(err) {
			recordProxyFailure(c, r, panelUser, err.Error())
			renderProxyProblem(w, r)
			return
		}
		http.Error(w, "upstream error", 502)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "read error", 502)
		return
	}
	// Usage discovery: log billable-looking API traffic (scan/AI/write) so we can wire limits.
	if isAPI || strings.HasPrefix(path, "/v2/") || strings.HasPrefix(path, "/v3/") || strings.HasPrefix(path, "/api-proxy/") {
		method := strings.ToUpper(r.Method)
		lp := strings.ToLower(path)
		interesting := method == http.MethodPost || method == http.MethodPut ||
			method == http.MethodPatch || method == http.MethodDelete ||
			strings.Contains(lp, "scan") || strings.Contains(lp, "predict") ||
			strings.Contains(lp, "ai") || strings.Contains(lp, "generat") ||
			strings.Contains(lp, "rewrite") || strings.Contains(lp, "origin") ||
			strings.Contains(lp, "document") || strings.Contains(lp, "credit")
		if interesting || resp.StatusCode >= 400 {
			msg := truncate(string(body), 120)
			if resp.StatusCode < 400 {
				msg = ""
			}
			if msg != "" {
				log.Printf("[USAGE] %s %s → %d user=%s %s", method, path, resp.StatusCode, panelUser, msg)
			} else {
				log.Printf("[USAGE] %s %s → %d user=%s", method, path, resp.StatusCode, panelUser)
			}
		}
	}
	if billAmount > 0 && panelUser != "" && resp.StatusCode > 0 && resp.StatusCode < 400 {
		panelCreditsChargeAmount(panelUser, path, billAmount)
	}
	body = decompress(resp.Header.Get("Content-Encoding"), body)
	resp.Header.Del("Content-Encoding")
	resp.Header.Del("Content-Length")
	resp.Header.Del("Content-Security-Policy")
	resp.Header.Del("Content-Security-Policy-Report-Only")
	resp.Header.Del("X-Frame-Options")

	// Dead GPTZero cookie / JWT → rotate panel account (log [SWAP]) and reload SPA.
	if usesPanelAccountMode(c) && !c.BypassAuth && accID > 0 &&
		gptzeroLooksLoggedOut(resp.StatusCode, path, body) {
		reason := fmt.Sprintf("upstream_%d:%s", resp.StatusCode, path)
		if tryGPTZeroAccountSwap(w, r, c, panelUser, accID, accountName, reason) {
			return
		}
	}

	ct := resp.Header.Get("Content-Type")
	publicBase := c.PublicScheme + "://" + c.PublicHost
	if strings.EqualFold(c.PublicScheme, "https") ||
		strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") || r.TLS != nil {
		publicBase = "https://" + c.PublicHost
	}
	pairs := domainPairs(c, publicBase)

	if strings.Contains(ct, "text/html") {
		body = rewriteBytes(body, pairs)
		inject := buildInject(c, ls, plan, token, publicBase, panelUser)
		if usesPanelAccountMode(c) && !c.BypassAuth {
			inject += limitWidgetScript(toolDisplayName(c))
		}
		body = injectHead(body, inject)
		if usesPanelAccountMode(c) && !c.BypassAuth {
			body = injectDeviceHTML(body)
		}
		// Browser cookies for plan / accessToken4 (JS readable)
		http.SetCookie(w, &http.Cookie{Name: "plan", Value: plan, Path: "/", MaxAge: 7 * 24 * 3600})
		if token != "" {
			http.SetCookie(w, &http.Cookie{Name: "accessToken4", Value: token, Path: "/", MaxAge: 7 * 24 * 3600})
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
	} else if strings.Contains(ct, "javascript") || strings.Contains(ct, "application/json") ||
		strings.Contains(ct, "text/css") || strings.Contains(ct, "text/x-component") {
		body = rewriteBytes(body, pairs)
		if strings.Contains(ct, "application/json") || strings.Contains(ct, "+json") {
			body = forceGPTZeroPremiumPlan(path, body)
		}
	}

	// Store rewritten static assets for next HIT (no Content-Encoding — already decompressed).
	storedCDN := false
	if cdnKey != "" && resp.StatusCode == http.StatusOK {
		storedCDN = storeCDNCache(r, resp.StatusCode, ct, "", body)
	}

	for k, vv := range resp.Header {
		lk := strings.ToLower(k)
		if lk == "content-length" || lk == "content-encoding" || strings.HasPrefix(lk, "content-security") {
			continue
		}
		// Prefer long browser cache on static CDN paths (users + reverse proxies).
		if lk == "cache-control" && storedCDN {
			continue
		}
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	if storedCDN {
		w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
		w.Header().Set("X-Proxy-Cache", "MISS")
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

func domainPairs(c Config, publicBase string) [][2]string {
	var pairs [][2]string
	add := func(oldHost, newBase string) {
		pairs = append(pairs,
			[2]string{"https://" + oldHost, newBase},
			[2]string{"http://" + oldHost, newBase},
		)
	}
	t, _ := url.Parse(c.TargetURL)
	if t != nil && t.Host != "" {
		add(t.Host, publicBase)
	}
	for i, extra := range c.ExtraDomains {
		host := strings.TrimPrefix(strings.TrimPrefix(extra, "https://"), "http://")
		host = strings.Split(host, "/")[0]
		// NEVER rewrite GrowthBook — /sub SSE hangs boot.
		if strings.Contains(strings.ToLower(host), "growthbook") {
			continue
		}
		add(host, fmt.Sprintf("%s/extra-cdn-%d", publicBase, i))
	}
	add("api.gptzero.me", publicBase+"/api-proxy")
	// longest first
	for i := 0; i < len(pairs); i++ {
		for j := i + 1; j < len(pairs); j++ {
			if len(pairs[j][0]) > len(pairs[i][0]) {
				pairs[i], pairs[j] = pairs[j], pairs[i]
			}
		}
	}
	return pairs
}

func rewriteBytes(body []byte, pairs [][2]string) []byte {
	for _, p := range pairs {
		body = bytes.ReplaceAll(body, []byte(p[0]), []byte(p[1]))
	}
	return body
}

func decompress(enc string, body []byte) []byte {
	enc = strings.ToLower(strings.TrimSpace(enc))
	switch {
	case strings.Contains(enc, "br"):
		r := brotli.NewReader(bytes.NewReader(body))
		out, err := io.ReadAll(r)
		if err == nil {
			return out
		}
	case strings.Contains(enc, "gzip"):
		r, err := gzip.NewReader(bytes.NewReader(body))
		if err == nil {
			out, err2 := io.ReadAll(r)
			_ = r.Close()
			if err2 == nil {
				return out
			}
		}
	case strings.Contains(enc, "deflate"):
		r := flate.NewReader(bytes.NewReader(body))
		out, err := io.ReadAll(r)
		_ = r.Close()
		if err == nil {
			return out
		}
	}
	return body
}

func injectHead(body []byte, inject string) []byte {
	re := regexp.MustCompile(`(?i)<head(?:\s[^>]*)?>`)
	if loc := re.FindIndex(body); loc != nil {
		out := make([]byte, 0, len(body)+len(inject))
		out = append(out, body[:loc[1]]...)
		out = append(out, inject...)
		out = append(out, body[loc[1]:]...)
		return out
	}
	return append([]byte(inject), body...)
}

func buildInject(c Config, ls map[string]string, plan, token, publicBase, panelUsername string) string {
	lsJSON := string(lsJSONForBrowser(ls, plan, c.PublicHost))
	lsJSON = strings.ReplaceAll(lsJSON, "</", "<\\/")

	var extra strings.Builder
	extra.WriteString("[")
	for i, d := range c.ExtraDomains {
		host := strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
		host = strings.Split(host, "/")[0]
		if strings.Contains(strings.ToLower(host), "growthbook") {
			continue
		}
		proxy := fmt.Sprintf("/extra-cdn-%d", i)
		extra.WriteString(fmt.Sprintf(`["https://%s",%q],["http://%s",%q],`, host, proxy, host, proxy))
	}
	extra.WriteString(`["https://api.gptzero.me","/api-proxy"],["http://api.gptzero.me","/api-proxy"]`)
	extra.WriteString("]")

	return fmt.Sprintf(`<script data-tm-gz="1">
(function(){
  var TM_USER = %s;
  var PLAN = %s;
  // Restore session
  try {
    var storageData = %s;
    for (var key in storageData) {
      if (Object.prototype.hasOwnProperty.call(storageData, key) && storageData[key] != null) {
        localStorage.setItem(key, typeof storageData[key] === 'string' ? storageData[key] : JSON.stringify(storageData[key]));
      }
    }
    // supabase-js storage key = sb-<hostname.firstLabel>-auth-token
    // Rewritten supabase host is 127.0.0.1 → key becomes sb-127-auth-token.
    var authVal = null;
    for (var k in storageData) {
      if (k.indexOf('sb-') === 0 && k.indexOf('-auth-token') === k.length - 11) { authVal = storageData[k]; break; }
    }
    if (authVal) {
      var derived = 'sb-' + String(location.hostname || '127').split('.')[0] + '-auth-token';
      localStorage.setItem(derived, typeof authVal === 'string' ? authVal : JSON.stringify(authVal));
      localStorage.setItem('sb-127-auth-token', typeof authVal === 'string' ? authVal : JSON.stringify(authVal));
      localStorage.setItem('sb-localhost-auth-token', typeof authVal === 'string' ? authVal : JSON.stringify(authVal));
    }
    localStorage.setItem('plan', PLAN);
    document.cookie = 'plan=' + encodeURIComponent(PLAN) + '; path=/; max-age=604800; samesite=lax';
    try {
      var traits = localStorage.getItem('_ca_user_traits');
      if (traits) {
        var tm = JSON.parse(traits);
        tm.plan = PLAN;
        localStorage.setItem('_ca_user_traits', JSON.stringify(tm));
      }
    } catch (e2) {}
    %s
    console.log('[GPTZero local] restored localStorage plan=' + PLAN + ' authKey=sb-' + String(location.hostname||'').split('.')[0] + '-auth-token');
  } catch (e) { console.warn('[GPTZero local] LS restore failed', e); }

  // Soft-block login/signup + config blocked_paths (SPA, no full reload).
  var BLOCKED = %s;
  var HOME = %s;
  function pathOf(u) {
    if (typeof u !== 'string') return '';
    try { return (new URL(u, location.origin).pathname || '').replace(/\/$/, '') || '/'; }
    catch (e) { return ''; }
  }
  function isBlocked(u) {
    var p = pathOf(u);
    if (!p) {
      try { return String(u).indexOf('/login') !== -1 || String(u).indexOf('/signup') !== -1; } catch (e) { return false; }
    }
    if (p === '/login' || p === '/signup') return true;
    for (var i = 0; i < BLOCKED.length; i++) {
      var b = String(BLOCKED[i] || '').replace(/\/$/, '');
      if (!b) continue;
      if (p === b || p.indexOf(b + '/') === 0) return true;
    }
    return false;
  }
  function stayHome() {
    var h = HOME || '/';
    if ((location.pathname || '/') !== h) history.replaceState(null, '', h);
  }
  try {
    var _push = history.pushState.bind(history);
    history.pushState = function(s, t, u) { if (isBlocked(u)) return _push(s, t, HOME || '/'); return _push(s, t, u); };
    var _rep = history.replaceState.bind(history);
    history.replaceState = function(s, t, u) { if (isBlocked(u)) return _rep(s, t, HOME || '/'); return _rep(s, t, u); };
    var _assign = location.assign.bind(location);
    location.assign = function(u) { if (isBlocked(u)) return stayHome(); return _assign(u); };
    var _replace = location.replace.bind(location);
    location.replace = function(u) { if (isBlocked(u)) return stayHome(); return _replace(u); };
    document.addEventListener('click', function(ev) {
      var a = ev.target && ev.target.closest ? ev.target.closest('a[href]') : null;
      if (!a) return;
      var href = a.getAttribute('href') || '';
      if (isBlocked(href)) { ev.preventDefault(); stayHome(); }
    }, true);
    if (isBlocked(location.pathname)) stayHome();
  } catch (e) {}

  // URL patcher — GrowthBook stays on real CDN.
  var O = location.origin;
  var EXTRA = %s;
  function patchURL(u) {
    if (typeof u !== 'string') return u;
    if (u.indexOf('cdn.growthbook.io') !== -1) return u;
    if (u.indexOf('api.gptzero.me') !== -1) {
      u = u.replace(/https?:\/\/api\.gptzero\.me/g, O + '/api-proxy');
    }
    if (u.indexOf('/v2/') === 0 || u.indexOf('/v3/') === 0) {
      u = O + '/api-proxy' + u;
    }
    u = u.replace(/https?:\/\/app\.gptzero\.me/g, O);
    for (var i = 0; i < EXTRA.length; i++) {
      u = u.replace(EXTRA[i][0], O + EXTRA[i][1]);
    }
    u = u.replace(/https?:\/\/([a-z0-9]+)\.gptzero\.me/gi, function(m, sub) {
      var s = (sub || '').toLowerCase();
      if (s === 'app' || s === 'api' || s === 'cdn' || s === 'auth' || s === 'www') return m;
      return O + '/extra-cdn-wild/' + sub + '.gptzero.me';
    });
    return u;
  }
  var xo = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function(m, u) {
    return xo.apply(this, [m, patchURL(u)].concat([].slice.call(arguments, 2)));
  };
  var fo = window.fetch;
  var __tmSwapReload = false;
  function tmForcePremiumPlanJSON(data) {
    if (!data || typeof data !== 'object') return data;
    if (Array.isArray(data)) {
      for (var i = 0; i < data.length; i++) tmForcePremiumPlanJSON(data[i]);
      return data;
    }
    if (typeof data.plan === 'string') {
      var pl = data.plan.toLowerCase();
      if (!data.plan || pl === 'free' || pl.indexOf('essential') === 0 || pl === 'null') data.plan = PLAN;
    }
    if (data.full_plan && typeof data.full_plan === 'object') {
      data.full_plan.name = PLAN;
      data.plan = PLAN;
    }
    if (data.plan && typeof data.plan === 'object' && data.plan.name != null) {
      var pn = String(data.plan.name || '').toLowerCase();
      if (!data.plan.name || pn === 'free' || pn.indexOf('essential') === 0) data.plan.name = PLAN;
    }
    for (var k in data) {
      if (Object.prototype.hasOwnProperty.call(data, k) && data[k] && typeof data[k] === 'object') {
        tmForcePremiumPlanJSON(data[k]);
      }
    }
    return data;
  }
  function tmShouldPatchPlanURL(u) {
    u = String(u || '').toLowerCase();
    return u.indexOf('profile') !== -1 || u.indexOf('subscription') !== -1 ||
      u.indexOf('/plans') !== -1 || u.indexOf('rest/v1/profiles') !== -1 ||
      u.indexOf('rest/v1/plans') !== -1;
  }
  function tmPatchPlanResponse(res, reqUrl) {
    if (!res || !res.ok || !tmShouldPatchPlanURL(reqUrl)) return Promise.resolve(res);
    var ct = (res.headers && res.headers.get('content-type')) || '';
    if (ct.indexOf('json') === -1) return Promise.resolve(res);
    return res.clone().text().then(function(txt) {
      if (!txt || (txt[0] !== '{' && txt[0] !== '[')) return res;
      if (txt.indexOf('"plan"') === -1 && txt.indexOf('full_plan') === -1) return res;
      try {
        var data = tmForcePremiumPlanJSON(JSON.parse(txt));
        return new Response(JSON.stringify(data), {
          status: res.status,
          statusText: res.statusText,
          headers: res.headers
        });
      } catch (e) { return res; }
    }).catch(function(){ return res; });
  }
  window.fetch = function(inp, init) {
    var req = inp;
    var reqUrl = '';
    if (typeof inp === 'string') {
      reqUrl = inp;
      var su = patchURL(inp);
      if (su !== inp) req = su;
    } else if (inp instanceof Request) {
      reqUrl = inp.url;
      var ru = patchURL(inp.url);
      if (ru !== inp.url) req = new Request(ru, inp);
    }
    if (typeof req === 'string') reqUrl = req;
    else if (req && req.url) reqUrl = req.url;
    return fo(req, init).then(function(res) {
      try {
        if (res && res.headers && res.headers.get('X-TM-Account-Switch') && !__tmSwapReload) {
          __tmSwapReload = true;
          setTimeout(function(){ location.reload(); }, 400);
        }
      } catch (e) {}
      return tmPatchPlanResponse(res, reqUrl);
    });
  };

  // Dismiss free-upsell once if it appears.
  var dismissed = false;
  function dismiss() {
    if (dismissed) return;
    document.querySelectorAll('button, a').forEach(function(el) {
      var t = (el.textContent || '').replace(/\s+/g, ' ').trim();
      if (t === 'Pass on premium, continue free') { dismissed = true; try { el.click(); } catch (e) {} }
    });
  }
  setInterval(dismiss, 1000);

  // Hide only the listed profile/sidebar/header chrome (keep Scans + main app).
  try {
    var st = document.createElement('style');
    st.setAttribute('data-tm-gz-hide', '1');
    st.textContent = [
      '#chrome-extension-link,',
      'li:has(>#chrome-extension-link),',
      'li:has(#chrome-extension-link),',
      '[data-testid="mobile-nav-chrome-extension"],',
      'a[href*="chromewebstore.google.com"],',
      '#more-menus,',
      'li:has(>#more-menus),',
      '#account-settings-link,',
      'li:has(>#account-settings-link),',
      '[data-testid="profile-menu-logout-button"],',
      'button.amplitude-survey-feedback,',
      '.amplitude-survey-feedback',
      '{display:none!important;visibility:hidden!important;pointer-events:none!important;max-height:0!important;overflow:hidden!important;margin:0!important;padding:0!important;border:0!important}'
    ].join('');
    (document.head || document.documentElement).appendChild(st);
  } catch (e) {}

  var HIDE_LABELS = {
    'Account details': 1,
    'Refer and earn': 1,
    'Help & support': 1,
    'Help &amp; support': 1,
    'Upgrade': 1,
    'Upgrade to Premium': 1,
    'Get started with Premium': 1,
    'Upgrade for Plagiarism scans.': 1,
    'Upgrade for Plagiarism scans': 1,
    'Log out': 1,
    'Tell us what you think': 1,
    'Chrome Extension': 1,
    'Settings': 1,
    'More': 1,
    'Upgrade for more': 1
  };
  function gzLabel(el) {
    var t = (el.textContent || '').replace(/\s+/g, ' ').trim();
    t = t.replace(/\s*Get 10k credits free\s*$/i, '').trim();
    t = t.replace(/\s*Unlock Advanced Scan\s*$/i, '').trim();
    return t;
  }
  // Never hide left rail / right nexus / editor chrome — previous parent-climb hid whole panels.
  function gzIsLayoutChrome(el) {
    if (!el || !el.closest) return false;
    if (el.closest('#documents-link, #g0-nav-menu-button-basic, [class*="nexus-nav"], [class*="nexus-results"], [class*="ProseMirror"], [contenteditable="true"]')) return true;
    if (el.id && String(el.id).indexOf('g0-nav-menu-button') === 0) return true;
    var cls = (el.className && String(el.className)) || '';
    if (/nexus|sidebar|ProseMirror|editor|document/i.test(cls)) return true;
    try {
      var r = el.getBoundingClientRect ? el.getBoundingClientRect() : null;
      if (r && (r.width > 280 || r.height > 220)) return true;
    } catch (e) {}
    return false;
  }
  function gzHide(el) {
    if (!el || el.getAttribute('data-tm-gz-hide') === '1') return;
    if (gzIsLayoutChrome(el)) return;
    el.setAttribute('data-tm-gz-hide', '1');
    el.style.setProperty('display', 'none', 'important');
    el.style.setProperty('visibility', 'hidden', 'important');
    el.style.setProperty('pointer-events', 'none', 'important');
    var li = el.closest ? el.closest('li') : null;
    if (li && !gzIsLayoutChrome(li) && (el.id === 'chrome-extension-link' || el.id === 'account-settings-link' || el.id === 'more-menus')) {
      li.style.setProperty('display', 'none', 'important');
      li.setAttribute('data-tm-gz-hide', '1');
    }
    // Only collapse tiny single-child wrappers (profile menu rows), never large panels.
    var p = el.parentElement;
    if (p && p.children && p.children.length === 1 && p !== document.body && !gzIsLayoutChrome(p)) {
      try {
        var pr = p.getBoundingClientRect ? p.getBoundingClientRect() : null;
        if (pr && pr.width <= 420 && pr.height <= 120) {
          p.setAttribute('data-tm-gz-hide', '1');
          p.style.setProperty('display', 'none', 'important');
          p.style.setProperty('visibility', 'hidden', 'important');
          p.style.setProperty('pointer-events', 'none', 'important');
        }
      } catch (e) {}
    }
  }
  function gzUnhideLayout() {
    try {
      document.querySelectorAll('[data-tm-gz-hide="1"]').forEach(function(el) {
        if (!gzIsLayoutChrome(el)) return;
        el.removeAttribute('data-tm-gz-hide');
        el.style.removeProperty('display');
        el.style.removeProperty('visibility');
        el.style.removeProperty('pointer-events');
      });
    } catch (e) {}
  }
  function scrubChrome() {
    try {
      gzUnhideLayout();
      document.querySelectorAll(
        '#chrome-extension-link, #account-settings-link, #more-menus, ' +
        '[data-testid="profile-menu-logout-button"], [data-testid="mobile-nav-chrome-extension"], ' +
        'a[href*="chromewebstore.google.com"], .amplitude-survey-feedback'
      ).forEach(gzHide);
      document.querySelectorAll('button, a').forEach(function(el) {
        // Never touch Scans sidebar item / nav tabs.
        if (el.id === 'documents-link' || gzIsLayoutChrome(el)) return;
        var t = gzLabel(el);
        var href = el.getAttribute('href') || '';
        var testid = el.getAttribute('data-testid') || '';
        if (testid === 'mobile-nav-chrome-extension' || href.indexOf('chromewebstore.google.com') !== -1) {
          gzHide(el);
          return;
        }
        if (HIDE_LABELS[t] || /^Upgrade to Premium$/i.test(t)) {
          // "More" / "Settings" / bare "Upgrade" only for known menu targets.
          if (t === 'More' && el.id !== 'more-menus') return;
          if (t === 'Settings' && el.id !== 'account-settings-link' && href !== '/account-settings') return;
          if (t === 'Upgrade' && !(el.closest && el.closest('[data-testid], [role="menu"], [class*="menu"]'))) return;
          gzHide(el);
        }
      });
      // Hide compact upsell copy only — never climb to section/panel parents.
      document.querySelectorAll('button, a, p, span, h1, h2, h3').forEach(function(el) {
        if (gzIsLayoutChrome(el)) return;
        if (el.children && el.children.length > 2) return;
        var t = (el.textContent || '').replace(/\s+/g, ' ').trim();
        if (!t || t.length > 90) return;
        if (/^Upgrade for more$/i.test(t) || /^Upgrade for Plagiarism scans\.?$/i.test(t) ||
            /^Get started with Premium$/i.test(t)) {
          gzHide(el);
        }
      });
    } catch (e) {}
  }
  // Show panel username instead of shared-account email in profile UI.
  function looksLikeEmail(s) {
    return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(String(s || '').trim());
  }
  function setPanelUser(el) {
    if (!el || !TM_USER) return;
    if (el.getAttribute('data-tm-user-set') === TM_USER) return;
    el.textContent = TM_USER;
    el.setAttribute('data-tm-user-set', TM_USER);
  }
  function scrubUsername() {
    if (!TM_USER) return;
    try {
      // Profile menu plan label → panel username (e.g. "Personal Plan" → sagar).
      document.querySelectorAll('[data-testid="profile-menu-plan-name"]').forEach(setPanelUser);
      document.querySelectorAll('div.break-all, div[class*="break-all"], span.break-all, span[class*="break-all"]').forEach(function(el) {
        var t = (el.textContent || '').replace(/\s+/g, ' ').trim();
        if (looksLikeEmail(t) || el.getAttribute('data-tm-user-set')) setPanelUser(el);
      });
      // Fallback: short leaf nodes that are only an email (profile menu / header).
      document.querySelectorAll('div, span, p').forEach(function(el) {
        if (el.children && el.children.length) return;
        var t = (el.textContent || '').replace(/\s+/g, ' ').trim();
        if (t.length > 80) return;
        if (looksLikeEmail(t)) setPanelUser(el);
      });
    } catch (e) {}
  }

  // Rewrite GPTZero native credit UI to this member's panel limit (remaining / total).
  var TM_LIMITS = null;
  function fmtCredits(n) {
    n = Math.max(0, Math.floor(Number(n) || 0));
    // Keep exact counts for typical panel limits (e.g. 10,000); K-format only for huge pools.
    if (n < 100000) return String(n);
    return Math.round(n / 1000) + 'K';
  }
  function panelCreditNums(d) {
    if (!d || !(Number(d.credit_limit) >= 0)) return null;
    var limit = Math.max(0, Math.floor(Number(d.credit_limit) || 0));
    var used = Math.max(0, Math.floor(Number(d.credit_used) || 0));
    var left = Math.max(0, limit - used);
    return { limit: limit, used: used, left: left, pct: limit === 0 ? 0 : (left / limit) * 100 };
  }
  function paintNativeCredits(d) {
    if (d) TM_LIMITS = d;
    var nums = panelCreditNums(TM_LIMITS);
    if (!nums) return;
    var leftS = fmtCredits(nums.left);
    var limS = fmtCredits(nums.limit);
    var sig = leftS + '/' + limS + '/' + nums.left + '/' + nums.limit;
    try {
      var creditLineRe = /^[\d.,]+\s*[KkMm]?\s+credits\s+of\s+[\d.,]+\s*[KkMm]?\s+remaining$/i;
      var creditsLeftRe = /^[\d.,]+\s*[KkMm]?\s*\/\s*[\d.,]+\s*[KkMm]?\s+credits\s+left$/i;
      function setCreditText(el, text) {
        // Single text node only — nested spans inside flex+space-between stretch across the header.
        el.textContent = text;
        try {
          el.style.whiteSpace = 'nowrap';
          el.style.display = 'inline';
          el.style.flex = '0 0 auto';
          el.style.justifyContent = 'flex-start';
          el.style.gap = '0';
        } catch (e) {}
        el.setAttribute('data-tm-credit-line', '1');
        el.setAttribute('data-tm-credit-sig', sig);
      }
      document.querySelectorAll('[data-testid="profile-menu-credits"]').forEach(function(el) {
        if (el.getAttribute('data-tm-credit-sig') === sig) return;
        setCreditText(el, leftS + ' / ' + limS);
      });
      // Header / footer credit labels — keep one text node so flex layouts don't stretch.
      document.querySelectorAll('span, div, p').forEach(function(el) {
        if (el.getAttribute('data-testid') === 'profile-menu-credits') return;
        if (el.closest && el.closest('[data-testid="profile-menu-credits"]')) return;
        if (el.getAttribute('data-tm-credit-sig') === sig) return;
        var t = (el.textContent || '').replace(/\s+/g, ' ').trim();
        if (!t || t.length > 70) return;
        var isRemaining = creditLineRe.test(t);
        var isCreditsLeft = creditsLeftRe.test(t);
        if (!isRemaining && !isCreditsLeft) return;
        // Skip tiny nested helper nodes when parent already is the full line.
        if (el.parentElement) {
          var pt = (el.parentElement.textContent || '').replace(/\s+/g, ' ').trim();
          if ((creditLineRe.test(pt) || creditsLeftRe.test(pt)) && pt.length > t.length + 2) return;
        }
        if (isCreditsLeft) {
          // Match GPTZero style: <p>…<span class="text-body-xs-500">LEFT</span>/TOTAL credits left</p>
          if (el.tagName === 'P' || (el.className && String(el.className).indexOf('text-body-xs') !== -1)) {
            el.innerHTML = '<span class="text-body-xs-500">' + leftS + '</span>/' + limS + ' credits left';
            try { el.style.whiteSpace = 'nowrap'; } catch (e) {}
            el.setAttribute('data-tm-credit-line', '1');
            el.setAttribute('data-tm-credit-sig', sig);
            return;
          }
          setCreditText(el, leftS + '/' + limS + ' credits left');
          return;
        }
        setCreditText(el, leftS + ' credits of ' + limS + ' remaining');
      });
      document.querySelectorAll('progress.progress, progress').forEach(function(el) {
        var maxAttr = el.getAttribute('max');
        var valAttr = el.getAttribute('value');
        // Only touch credit-style bars (large max like GPTZero monthly pool, or already patched).
        if (el.getAttribute('data-tm-credit-bar') !== '1') {
          var maxN = Number(maxAttr);
          var valN = Number(valAttr);
          if (!(maxN >= 1000) && !(valN >= 1000)) return;
        }
        el.setAttribute('data-tm-credit-bar', '1');
        el.setAttribute('max', String(nums.limit));
        el.setAttribute('value', String(nums.left));
        try { el.max = nums.limit; el.value = nums.left; } catch (e) {}
      });
      document.querySelectorAll('div.h-full[style*="width"], div[class*="bg-text-blue"][style*="width"], div.bg-text-blue[style*="width"]').forEach(function(el) {
        var st = el.getAttribute('style') || '';
        if (st.indexOf('width') === -1) return;
        // Parent progress track usually wraps this fill.
        var parent = el.parentElement;
        if (!parent) return;
        var cls = (parent.className || '') + ' ' + (el.className || '');
        if (cls.indexOf('progress') === -1 && cls.indexOf('bg-') === -1 && !parent.querySelector('progress')) {
          // still allow known fill class
          if ((el.className || '').indexOf('bg-text-blue') === -1) return;
        }
        var pct = Math.max(0, Math.min(100, nums.pct));
        var next = st.replace(/width\s*:\s*[^;]+/i, 'width: ' + pct.toFixed(4) + '%');
        if (next === st && st.indexOf('width') !== -1) {
          next = st.replace(/width\s*:\s*[^;]+/i, 'width: ' + pct.toFixed(4) + '%');
        }
        if (!/width\s*:/i.test(next)) next = 'width: ' + pct.toFixed(4) + '%;' + st;
        if (el.getAttribute('data-tm-credit-sig') === sig && el.style.width === pct.toFixed(4) + '%') return;
        el.setAttribute('style', next);
        el.style.width = pct.toFixed(4) + '%';
        el.setAttribute('data-tm-credit-sig', sig);
      });
    } catch (e) {}
  }
  window.__tmPaintNativeCredits = paintNativeCredits;
  function refreshPanelLimits() {
    fetch('/api/user-limits', { credentials: 'same-origin', cache: 'no-store' })
      .then(function(r) { return r.json(); })
      .then(function(d) { paintNativeCredits(d); })
      .catch(function() {});
  }

  function scrubAll() { scrubChrome(); scrubUsername(); paintNativeCredits(TM_LIMITS); }
  scrubAll();
  refreshPanelLimits();
  setInterval(scrubAll, 700);
  setInterval(refreshPanelLimits, 4000);
  try {
    new MutationObserver(function() { scrubAll(); }).observe(document.documentElement, { childList: true, subtree: true });
  } catch (e) {}
})();
</script>`,
		mustJSON(strings.TrimSpace(panelUsername)),
		mustJSON(normalizeGPTZeroPlan(plan)),
		lsJSON,
		tokenCookieJS(token),
		blockedPathsJSON(c.BlockedPaths),
		mustJSON(homePathOrDefault(c)),
		extra.String(),
	)
}

func homePathOrDefault(c Config) string {
	if strings.TrimSpace(c.HomePath) != "" {
		return c.HomePath
	}
	return "/"
}

func blockedPathsJSON(paths []string) string {
	if paths == nil {
		paths = []string{}
	}
	b, err := json.Marshal(paths)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func mustJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func tokenCookieJS(token string) string {
	if token == "" {
		return ""
	}
	return fmt.Sprintf(`document.cookie = 'accessToken4=' + %s + '; path=/; max-age=604800; samesite=lax';`, mustJSON(token))
}
