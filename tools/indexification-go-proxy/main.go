package main

import (
	"html"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const CONFIG_FILE = "config.json"

// ReplacementPair defines a search and replace pair
type ReplacementPair struct {
	Search  string `json:"search"`
	Replace string `json:"replace"`
}

// Config structures
type Config struct {
	Port              string            `json:"port"`
	TargetURL         string            `json:"target_url"`
	CookiePath        string            `json:"cookie_path"`
	PublicHost        string            `json:"public_host"`
	PublicScheme      string            `json:"public_scheme"`
	UserAgent         string            `json:"user_agent"`
	LogoutDetectText  string            `json:"logout_detect_text"`
	AutomationURL     string            `json:"automation_url"`
	AutomationPayload map[string]string `json:"automation_payload"`
	AutomationHeaders map[string]string `json:"automation_headers"`
	LogoText          string            `json:"logo_text"`
	CooldownFilePath  string            `json:"cooldown_file_path"`
	CooldownSeconds   int               `json:"cooldown_seconds"`
	Replacements      []ReplacementPair `json:"replacements"`

	// Generic Master Proxy variables
	ToolName              string            `json:"tool_name"`
	LogoutDetectTexts     []string          `json:"logout_detect_texts"`
	LoginRedirectKeywords []string          `json:"login_redirect_keywords"`
	ExtraRewriteDomains   []string          `json:"extra_rewrite_domains"`
	DomainPathMap         map[string]string `json:"domain_path_map"`
	RefreshLoopSeconds    int               `json:"refresh_loop_seconds"`
	InjectExportTool      bool              `json:"inject_export_tool"`
	CustomCSS             string            `json:"custom_css"`
	CustomJS              string            `json:"custom_js"`
	BlockedPaths          []string          `json:"blocked_paths"`
	BlockedSuffixes       []string          `json:"blocked_suffixes"`
	HomePath              string            `json:"home_path"`
	LocalTestMode         bool              `json:"local_test_mode"`
	DebugLog              bool              `json:"debug_log"`

	// Database configurations
	MySQLHost     string `json:"mysql_host"`
	MySQLPort     string `json:"mysql_port"`
	MySQLUser     string `json:"mysql_user"`
	MySQLPassword string `json:"mysql_password"`
	MySQLDB       string `json:"mysql_db"`
	PanelDB       string `json:"panel_db"`
}

var (
	currentConfig    Config
	configModTime    time.Time
	db               *sql.DB
	currentWebsiteID int = 1

	// Regexp for stripping Subresource Integrity (SRI)
	integrityRegex      = regexp.MustCompile(`(?i)\s*integrity=(?:"[^"]*"|'[^']*')`)
	webpackAttrRegex    = regexp.MustCompile(`(?i)(?:"integrity"|'integrity')\s*:`)
	webpackSetAttrRegex = regexp.MustCompile(`(?i)setAttribute\(\s*[\'"]integrity[\'"]`)

	// Automation variables
	lastAutomationTime time.Time
	automationMutex    sync.Mutex
)

// loadConfig loads the configuration from config.json with hot-reloading
func loadConfig() Config {
	info, err := os.Stat(CONFIG_FILE)
	if err != nil {
		return currentConfig
	}
	if !info.ModTime().After(configModTime) {
		return currentConfig
	}

	data, err := os.ReadFile(CONFIG_FILE)
	if err != nil {
		log.Printf("[CONFIG] Error reading config file: %v", err)
		return currentConfig
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("[CONFIG] Error parsing config JSON: %v", err)
		return currentConfig
	}

	if cfg.Port == "" {
		cfg.Port = "7860"
	}
	if cfg.TargetURL == "" {
		cfg.TargetURL = "https://1.semrush.com.in"
	}
	if cfg.CookiePath == "" {
		cfg.CookiePath = "cookie.txt"
	}
	if cfg.PublicHost == "" {
		cfg.PublicHost = "localhost:" + cfg.Port
	}
	if cfg.PublicScheme == "" {
		cfg.PublicScheme = "http"
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"
	}
	if cfg.LogoutDetectText == "" {
		cfg.LogoutDetectText = "Session expired, access again from Dashboard"
	}
	if cfg.AutomationURL == "" {
		cfg.AutomationURL = "https://dashboard.seotoolscart.com/api/tasks/run"
	}
	if cfg.AutomationPayload == nil {
		cfg.AutomationPayload = map[string]string{
			"task_uid": "noxtools_runNoxtoolsSemrush",
		}
	}
	if cfg.LogoText == "" {
		cfg.LogoText = "ToolsMandi.com"
	}
	if cfg.CooldownFilePath == "" {
		cfg.CooldownFilePath = "cooldown.txt"
	}
	if cfg.CooldownSeconds <= 0 {
		cfg.CooldownSeconds = 180 // 3 minutes default
	}
	if len(cfg.Replacements) == 0 {
		cfg.Replacements = []ReplacementPair{
			{Search: "NoxTools.com", Replace: "ToolsMandi.com"},
			{Search: "noxtools.com", Replace: "toolsmandi.com"},
			{Search: "NoxTools", Replace: "ToolsMandi"},
			{Search: "noxtools", Replace: "toolsmandi"},
			{Search: "Nox Tools", Replace: "ToolsMandi"},
		}
	}
	if cfg.ToolName == "" {
		cfg.ToolName = "Semrush"
	}
	if len(cfg.LogoutDetectTexts) == 0 {
		cfg.LogoutDetectTexts = []string{cfg.LogoutDetectText}
	}
	if len(cfg.LoginRedirectKeywords) == 0 {
		cfg.LoginRedirectKeywords = []string{"login", "signin", "amember", "noxtools", "toolbaazar", "xemrush"}
	}
	if cfg.RefreshLoopSeconds <= 0 {
		cfg.RefreshLoopSeconds = 5
	}

	currentConfig = cfg
	configModTime = info.ModTime()
	log.Printf("[CONFIG] Config loaded successfully (Port: %s, Target: %s, CookiePath: %s) ✅", cfg.Port, cfg.TargetURL, cfg.CookiePath)
	return cfg
}

// resolveWebsiteID queries the central database to resolve this proxy's website ID based on domain
func resolveWebsiteID(publicHost string) {
	if db == nil {
		currentWebsiteID = 1
		return
	}
	if publicHost == "" {
		log.Printf("[DB] public_host not specified in config.json, using default website_id = 1")
		currentWebsiteID = 1
		return
	}
	var wid int
	err := db.QueryRow("SELECT id FROM ahrefs_websites WHERE domain = ?", publicHost).Scan(&wid)
	if err == sql.ErrNoRows {
		log.Printf("[DB] ⚠️ Domain '%s' not registered in ahrefs_websites table! Using default website_id = 1", publicHost)
		currentWebsiteID = 1
	} else if err != nil {
		log.Printf("[DB] ⚠️ Error querying website_id for domain '%s': %v. Using default website_id = 1", publicHost, err)
		currentWebsiteID = 1
	} else {
		currentWebsiteID = wid
		log.Printf("[DB] Resolved website_id = %d for domain '%s' ✅", currentWebsiteID, publicHost)
	}
}

// realClientIP extracts the real client IP from headers
func realClientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.TrimSpace(strings.Split(ip, ",")[0])
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return strings.TrimSpace(ip)
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

func isBlockedPath(path string, cfg Config) bool {
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}
	for _, blocked := range cfg.BlockedPaths {
		if path == blocked {
			return true
		}
	}
	for _, suffix := range cfg.BlockedSuffixes {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

// debugLog prints detailed logs when debug_log is true in config.json
func debugLog(cfg Config, format string, args ...interface{}) {
	if cfg.DebugLog {
		log.Printf("[DEBUG] "+format, args...)
	}
}

// resolveProxyHost returns the public-facing host:port for URL rewriting
func resolveProxyHost(r *http.Request, cfg Config) string {
	host := r.Host
	if xf := r.Header.Get("X-Forwarded-Host"); xf != "" {
		host = strings.TrimSpace(strings.Split(xf, ",")[0])
	}

	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}

	isInternal := hostname == "" || hostname == "localhost" || hostname == "127.0.0.1" || strings.HasPrefix(hostname, "127.")
	if isInternal && cfg.PublicHost != "" {
		return cfg.PublicHost // keep port e.g. localhost:7862
	}

	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

// resolveProxyScheme returns https/http for URL rewriting behind nginx reverse proxy
func resolveProxyScheme(r *http.Request, cfg Config) string {
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		return p
	}
	if r.TLS != nil {
		return "https"
	}
	host := resolveProxyHost(r, cfg)
	cfgHost := cfg.PublicHost
	if h, _, err := net.SplitHostPort(cfgHost); err == nil {
		cfgHost = h
	}
	if host == cfgHost && cfg.PublicScheme != "" {
		return cfg.PublicScheme
	}
	if strings.Contains(host, "localhost") || strings.Contains(host, "127.0.0.1") {
		return "http"
	}
	if cfg.PublicScheme != "" {
		return cfg.PublicScheme
	}
	return "http"
}

// generateOTT generates a cryptographically secure 64-character token
func generateOTT() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// getAuthenticatedUser checks the session cookie against database sessions
func getAuthenticatedUser(r *http.Request, cfg Config) (string, error) {
	cookie, err := r.Cookie("ct_session")
	if err != nil {
		return "", err
	}

	sessionToken := cookie.Value
	if sessionToken == "" {
		return "", fmt.Errorf("empty session token")
	}

	if db == nil {
		return "", fmt.Errorf("database not connected")
	}

	var username string
	var expiresAt time.Time
	err = db.QueryRow(
		"SELECT username, expires_at FROM ahrefs_sessions WHERE session_token = ? AND website_id = ?",
		sessionToken, currentWebsiteID,
	).Scan(&username, &expiresAt)

	if err == sql.ErrNoRows {
		return "", fmt.Errorf("session not found")
	}
	if err != nil {
		return "", fmt.Errorf("database session lookup error: %w", err)
	}

	if time.Now().After(expiresAt) {
		// Delete expired session
		_, _ = db.Exec("DELETE FROM ahrefs_sessions WHERE session_token = ? AND website_id = ?", sessionToken, currentWebsiteID)
		return "", fmt.Errorf("session expired")
	}

	return username, nil
}

// authHandshakeHandler handles secure server-to-server auth handshake requests from aMemberPro
func authHandshakeHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()

	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Username      string        `json:"username"`
		ProductIDsRaw []interface{} `json:"product_ids"`
		ClientIP      string        `json:"client_ip"`
		Timestamp     int64         `json:"timestamp"`
		Signature     string        `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Printf("[HANDSHAKE] Bad JSON payload: %v", err)
		http.Error(w, "Bad Request: Invalid JSON", http.StatusBadRequest)
		return
	}

	var productIDs []int
	for _, v := range payload.ProductIDsRaw {
		switch n := v.(type) {
		case float64:
			productIDs = append(productIDs, int(n))
		case string:
			if i, err := strconv.Atoi(n); err == nil {
				productIDs = append(productIDs, i)
			}
		case json.Number:
			if i, err := n.Int64(); err == nil {
				productIDs = append(productIDs, int(i))
			}
		}
	}

	if payload.Username == "" || payload.ClientIP == "" || payload.Signature == "" {
		http.Error(w, "Bad Request: Missing required fields", http.StatusBadRequest)
		return
	}

	if db == nil {
		http.Error(w, "Service Unavailable: Database not connected (disable local_test_mode for auth)", http.StatusServiceUnavailable)
		return
	}

	var dbSecretKey string
	var dbSessionDuration int
	err := db.QueryRow("SELECT secret_key, session_duration FROM ahrefs_websites WHERE id = ?", currentWebsiteID).Scan(&dbSecretKey, &dbSessionDuration)
	if err != nil {
		log.Printf("[HANDSHAKE] Failed to fetch secret_key from DB: %v (using fallback)", err)
		dbSecretKey = "toolsmandi_semrush_secret_xyz123"
	}

	// Verify HMAC signature: HMAC-SHA256(username + timestamp, secret_key)
	h := hmac.New(sha256.New, []byte(dbSecretKey))
	h.Write([]byte(fmt.Sprintf("%s:%d", payload.Username, payload.Timestamp)))
	expectedSig := hex.EncodeToString(h.Sum(nil))
	if !hmac.Equal([]byte(payload.Signature), []byte(expectedSig)) {
		log.Printf("[HANDSHAKE] ❌ HMAC verification failed for user: %s", payload.Username)
		http.Error(w, "Forbidden: Invalid signature", http.StatusForbidden)
		return
	}

	if time.Now().Unix()-payload.Timestamp > 300 {
		log.Printf("[HANDSHAKE] ❌ Stale request rejected: %s", payload.Username)
		http.Error(w, "Forbidden: Request expired", http.StatusForbidden)
		return
	}

	// Check status
	var dbStatus string
	err = db.QueryRow("SELECT status FROM ahrefs_users WHERE username = ? AND website_id = ?", payload.Username, currentWebsiteID).Scan(&dbStatus)
	if err == sql.ErrNoRows {
		_, err = db.Exec("INSERT INTO ahrefs_users (username, website_id, credit_limit, export_limit) VALUES (?, ?, 50, 100000)", payload.Username, currentWebsiteID)
		if err != nil {
			log.Printf("[HANDSHAKE] Auto-create user failed: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
	} else if err != nil {
		log.Printf("[HANDSHAKE] DB lookup error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	} else if dbStatus == "suspended" {
		http.Error(w, "User suspended", http.StatusForbidden)
		return
	}

	ott, err := generateOTT()
	if err != nil {
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	expires := time.Now().Add(60 * time.Second)
	_, err = db.Exec(
		"INSERT INTO ahrefs_tokens (token, username, client_ip, expires_at, website_id) VALUES (?, ?, ?, ?, ?)",
		ott, payload.Username, payload.ClientIP, expires, currentWebsiteID,
	)
	if err != nil {
		log.Printf("[HANDSHAKE] OTT insert failed: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	redirectURL := fmt.Sprintf("%s://%s/access?token=%s",
		cfg.PublicScheme, cfg.PublicHost,
		url.QueryEscape(payload.Username),
		url.QueryEscape(ott),
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","redirect_url":%q}`, redirectURL)
}

// accessHandler validates One-Time Token from handshake, sets session cookie, redirects
func accessHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	username := r.URL.Query().Get("user")
	ott := r.URL.Query().Get("token")

	if username == "" || ott == "" {
		http.Error(w, "Bad Request: Missing user or token", http.StatusBadRequest)
		return
	}

	if db == nil {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	// Look up OTT in DB
	var storedUsername, storedIP string
	var expiresAt time.Time
	var tokenID int
	err := db.QueryRow(
		"SELECT id, username, client_ip, expires_at FROM ahrefs_tokens WHERE token = ? AND website_id = ?",
		ott, currentWebsiteID,
	).Scan(&tokenID, &storedUsername, &storedIP, &expiresAt)

	if err == sql.ErrNoRows {
		log.Printf("[ACCESS] ❌ OTT not found for user: %s under website_id %d", username, currentWebsiteID)
		renderAccessDeniedPage(w)
		return
	}
	if err != nil {
		log.Printf("[ACCESS] DB error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Delete OTT immediately
	_, _ = db.Exec("DELETE FROM ahrefs_tokens WHERE id = ?", tokenID)

	if time.Now().After(expiresAt) {
		log.Printf("[ACCESS] ❌ Expired OTT for user: %s", username)
		renderAccessDeniedPage(w)
		return
	}

	if storedUsername != username {
		log.Printf("[ACCESS] ❌ Username mismatch: token for '%s', got '%s'", storedUsername, username)
		renderAccessDeniedPage(w)
		return
	}

	clientIP := realClientIP(r)

	// Check status
	var dbStatus string
	err = db.QueryRow("SELECT status FROM ahrefs_users WHERE username = ? AND website_id = ?", username, currentWebsiteID).Scan(&dbStatus)
	if err == sql.ErrNoRows {
		_, err = db.Exec("INSERT INTO ahrefs_users (username, website_id, credit_limit, export_limit) VALUES (?, ?, 50, 100000)", username, currentWebsiteID)
		if err != nil {
			log.Printf("[ACCESS] Auto-create user failed: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
	} else if err != nil {
		log.Printf("[ACCESS] DB error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	} else if dbStatus == "suspended" {
		http.Error(w, "Account Suspended", http.StatusForbidden)
		return
	}

	sessionToken, err := generateOTT()
	if err != nil {
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	sessionDuration := 30
	var dbSessionDuration int
	if errDb := db.QueryRow("SELECT session_duration FROM ahrefs_websites WHERE id = ?", currentWebsiteID).Scan(&dbSessionDuration); errDb == nil && dbSessionDuration > 0 {
		sessionDuration = dbSessionDuration
	}

	sessionExpires := time.Now().Add(time.Duration(sessionDuration) * time.Minute)

	// Single-Session Enforcement
	_, _ = db.Exec("DELETE FROM ahrefs_sessions WHERE username = ? AND website_id = ?", username, currentWebsiteID)

	_, err = db.Exec(
		"INSERT INTO ahrefs_sessions (session_token, username, client_ip, expires_at, website_id) VALUES (?, ?, ?, ?, ?)",
		sessionToken, username, clientIP, sessionExpires, currentWebsiteID,
	)
	if err != nil {
		log.Printf("[ACCESS] Session insert failed: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Log login
	userAgentStr := r.Header.Get("User-Agent")
	_, _ = db.Exec(
		"INSERT INTO ahrefs_login_logs (website_id, username, client_ip, user_agent) VALUES (?, ?, ?, ?)",
		currentWebsiteID, username, clientIP, userAgentStr,
	)

	http.SetCookie(w, &http.Cookie{
		Name:     "ct_session",
		Value:    sessionToken,
		Path:     "/",
		Domain:   cfg.PublicHost,
		Expires:  sessionExpires,
		HttpOnly: true,
		Secure:   cfg.PublicScheme == "https",
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/", http.StatusFound)
	log.Printf("[ACCESS] ✅ Session established for user: %s (cookie set)", username)
}

// renderAccessDeniedPage renders a simple, always-visible auth error page
func renderAccessDeniedPage(w http.ResponseWriter) {
	name := html.EscapeString(toolDisplayName(loadConfig()))
	writeLightCard(w, http.StatusForbidden, lightCard{
		Title:   "Access Denied",
		Heading: "Access Denied",
		Message: "You cannot open <span class=\"brand\">" + name + "</span> directly. Open it again from your access link.",
		Footer:  "Your session ended or this browser is not authorized",
	})
}

// mergeCookies merges incoming client cookies with the premium cookies.
// Premium cookies override incoming client cookies if there are conflicts.
func mergeCookies(clientCookieStr, premiumCookieStr string) string {
	if premiumCookieStr == "" {
		return clientCookieStr
	}
	if clientCookieStr == "" {
		return premiumCookieStr
	}

	cookies := make(map[string]string)
	parts := strings.Split(clientCookieStr, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		subparts := strings.SplitN(part, "=", 2)
		if len(subparts) == 2 {
			cookies[subparts[0]] = subparts[1]
		}
	}

	premiumParts := strings.Split(premiumCookieStr, ";")
	for _, part := range premiumParts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		subparts := strings.SplitN(part, "=", 2)
		if len(subparts) == 2 {
			cookies[subparts[0]] = subparts[1]
		}
	}

	var merged []string
	for k, v := range cookies {
		merged = append(merged, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(merged, "; ")
}

// loadCookiesFromFile reads and parses the cookie string from Netscape format or JSON
func loadCookiesFromFile(path string) string {
	if path == "" {
		path = "cookie.txt"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[COOKIE] Error reading cookie file %s: %v", path, err)
		return ""
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return ""
	}

	// GoAuto {"referer"?: "...", "cookies":[...]} — referer is optional.
	if strings.HasPrefix(trimmed, "{") {
		var wrap struct {
			Cookies json.RawMessage `json:"cookies"`
		}
		if err := json.Unmarshal([]byte(trimmed), &wrap); err == nil {
			if inner := strings.TrimSpace(string(wrap.Cookies)); strings.HasPrefix(inner, "[") {
				trimmed = inner
			}
		}
	}

	// 1. If it's a JSON array, parse dynamically
	if strings.HasPrefix(trimmed, "[") {
		var cookieList []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal([]byte(trimmed), &cookieList); err == nil {
			var cookieParts []string
			for _, c := range cookieList {
				if c.Name != "" && c.Value != "" {
					cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", c.Name, c.Value))
				}
			}
			return strings.Join(cookieParts, "; ")
		}
	}

	// 2. Netscape format or Raw lines
	lines := strings.Split(trimmed, "\n")
	var cookieParts []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || (strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "#HttpOnly_")) || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "#HttpOnly_") {
			line = strings.TrimPrefix(line, "#HttpOnly_")
		}

		columns := strings.Split(line, "\t")
		if len(columns) >= 7 {
			name := strings.TrimSpace(columns[5])
			value := strings.TrimSpace(columns[6])
			if name != "" {
				cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", name, value))
			}
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 7 {
			name := strings.TrimSpace(fields[5])
			value := strings.TrimSpace(fields[6])
			if name != "" {
				cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", name, value))
			}
			continue
		}

		if strings.Contains(line, "=") && !strings.Contains(line, " ") {
			cookieParts = append(cookieParts, line)
		}
	}
	return strings.Join(cookieParts, "; ")
}

// isLoginRedirectURL checks if a redirect Location header points to a login page
func isLoginRedirectURL(loc string, cfg Config) bool {
	locLower := strings.ToLower(loc)
	for _, keyword := range cfg.LoginRedirectKeywords {
		if keyword != "" && strings.Contains(locLower, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

// renderSessionRefreshHTML returns a looping refresh page shown while session is expired
func renderSessionRefreshHTML(cfg Config) string {
	seconds := cfg.RefreshLoopSeconds
	if seconds <= 0 {
		seconds = 5
	}
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Refreshing Session — ` + cfg.ToolName + `</title>
    <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@300;400;600;800&display=swap" rel="stylesheet">
    <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/5.15.3/css/all.min.css">
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: 'Outfit', sans-serif;
            background: radial-gradient(circle at center, #111b2d 0%, #080c14 100%);
            color: #ffffff;
            height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            overflow: hidden;
            text-align: center;
        }
        .container {
            padding: 40px;
            background: rgba(255, 255, 255, 0.03);
            border: 1px solid rgba(255, 255, 255, 0.05);
            border-radius: 24px;
            backdrop-filter: blur(16px);
            box-shadow: 0 20px 50px rgba(0, 0, 0, 0.5);
            max-width: 500px;
            width: 90%;
        }
        .icon {
            font-size: 48px;
            color: #ff8e53;
            margin-bottom: 20px;
            animation: spin 2s linear infinite;
            display: inline-block;
        }
        h2 {
            font-size: 28px;
            font-weight: 800;
            margin-bottom: 15px;
            background: linear-gradient(135deg, #ff6366 0%, #ff8e53 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        p { font-size: 16px; color: #a0aec0; line-height: 1.6; margin-bottom: 25px; }
        .timer { font-weight: 800; color: #ff8e53; }
        @keyframes spin {
            0% { transform: rotate(0deg); }
            100% { transform: rotate(360deg); }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="icon"><i class="fas fa-sync-alt"></i></div>
        <h2>Refreshing Session</h2>
        <p>
            Your ` + cfg.ToolName + ` session has expired.<br>
            <strong>Auto-fixing and logging back in...</strong><br>
            Retrying in <span id="refresh-timer" class="timer">` + strconv.Itoa(seconds) + `</span> seconds.
        </p>
        <script>
            (function() {
                const LOOP_SECONDS = ` + strconv.Itoa(seconds) + `;
                let timeLeft = LOOP_SECONDS;
                const timerElem = document.getElementById("refresh-timer");
                setInterval(function() {
                    timeLeft--;
                    if (timerElem) timerElem.textContent = timeLeft;
                    if (timeLeft <= 0) {
                        timeLeft = LOOP_SECONDS;
                        window.location.href = "/";
                    }
                }, 1000);
            })();
        </script>
    </div>
</body>
</html>`
}

// applyExtraDomainRewrites rewrites additional domains from config to the proxy host
func applyExtraDomainRewrites(bodyStr, proxySchemeHost, proxyHost string, cfg Config) string {
	for _, domain := range cfg.ExtraRewriteDomains {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		bodyStr = strings.ReplaceAll(bodyStr, "https://"+domain, proxySchemeHost)
		bodyStr = strings.ReplaceAll(bodyStr, "http://"+domain, proxySchemeHost)
		bodyStr = strings.ReplaceAll(bodyStr, "//"+domain, "//"+proxyHost)
		bodyStr = strings.ReplaceAll(bodyStr, `\/`+domain+`\/`, `\/`+proxyHost+`\/`)
		bodyStr = strings.ReplaceAll(bodyStr, "/"+domain, "/"+proxyHost)
	}
	return bodyStr
}

// proxyWsOrigin returns ws:// or wss:// origin matching the proxy page scheme
func proxyWsOrigin(proxyScheme, proxyHost string) string {
	if proxyScheme == "https" {
		return "wss://" + proxyHost
	}
	return "ws://" + proxyHost
}

// applyDomainPathMap rewrites external tool subdomains to NoxTools-style path prefixes on the proxy
func applyDomainPathMap(bodyStr, proxySchemeHost, proxyWsOrigin string, cfg Config) string {
	for domain, pathPrefix := range cfg.DomainPathMap {
		domain = strings.TrimSpace(domain)
		pathPrefix = strings.TrimSpace(pathPrefix)
		if domain == "" {
			continue
		}
		if pathPrefix == "" {
			pathPrefix = ""
		}
		httpBase := proxySchemeHost + pathPrefix
		wsBase := proxyWsOrigin + pathPrefix
		escHttpBase := strings.ReplaceAll(httpBase, "/", `\/`)
		escWsBase := strings.ReplaceAll(wsBase, "/", `\/`)

		bodyStr = strings.ReplaceAll(bodyStr, "https://"+domain, httpBase)
		bodyStr = strings.ReplaceAll(bodyStr, "http://"+domain, httpBase)
		bodyStr = strings.ReplaceAll(bodyStr, "wss://"+domain, wsBase)
		bodyStr = strings.ReplaceAll(bodyStr, "ws://"+domain, wsBase)
		bodyStr = strings.ReplaceAll(bodyStr, `https:\/\/`+domain, escHttpBase)
		bodyStr = strings.ReplaceAll(bodyStr, `http:\/\/`+domain, escHttpBase)
		bodyStr = strings.ReplaceAll(bodyStr, `wss:\/\/`+domain, escWsBase)
		bodyStr = strings.ReplaceAll(bodyStr, `ws:\/\/`+domain, escWsBase)
	}
	return bodyStr
}

// normalizeProxyWebSocketURLs rewrites WebSocket URLs for the local HTTP proxy
func normalizeProxyWebSocketURLs(bodyStr, targetHost, proxyHost, proxyScheme string) string {
	proxyWs := proxyWsOrigin(proxyScheme, proxyHost)
	escProxyWs := strings.ReplaceAll(proxyWs, "/", `\/`)

	replacements := []struct{ old, new string }{
		{"wss://" + targetHost, proxyWs},
		{"ws://" + targetHost, proxyWs},
		{`wss:\/\/` + targetHost, escProxyWs},
		{`ws:\/\/` + targetHost, escProxyWs},
	}
	if proxyScheme == "http" {
		replacements = append(replacements,
			struct{ old, new string }{"wss://" + proxyHost, proxyWs},
			struct{ old, new string }{`wss:\/\/` + proxyHost, escProxyWs},
		)
	}
	for _, r := range replacements {
		bodyStr = strings.ReplaceAll(bodyStr, r.old, r.new)
	}
	return bodyStr
}

// rewriteProxyResponseHeaders rewrites target host references in response headers (CORS, etc.)
func rewriteProxyResponseHeaders(resp *http.Response, targetHost, targetScheme, proxyHost, proxyScheme string) {
	proxySchemeHost := proxyScheme + "://" + proxyHost
	targetSchemeHost := targetScheme + "://" + targetHost
	corsHeaders := []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Expose-Headers",
		"Timing-Allow-Origin",
	}
	for _, headerName := range corsHeaders {
		values := resp.Header.Values(headerName)
		if len(values) == 0 {
			continue
		}
		var updated []string
		for _, v := range values {
			v = strings.ReplaceAll(v, targetSchemeHost, proxySchemeHost)
			v = strings.ReplaceAll(v, "https://"+targetHost, proxySchemeHost)
			v = strings.ReplaceAll(v, "http://"+targetHost, proxySchemeHost)
			updated = append(updated, v)
		}
		resp.Header.Del(headerName)
		for _, v := range updated {
			resp.Header.Add(headerName, v)
		}
	}
}

// buildDomainPathMapJS returns client-side JS for dynamic domain→path rewriting
func buildDomainPathMapJS(cfg Config) string {
	if len(cfg.DomainPathMap) == 0 {
		return "const DOMAIN_PATH_MAP = {};"
	}
	b, err := json.Marshal(cfg.DomainPathMap)
	if err != nil {
		return "const DOMAIN_PATH_MAP = {};"
	}
	return "const DOMAIN_PATH_MAP = " + string(b) + ";"
}

// isLogoutBody detects session-expired responses without false positives on large app pages
func isLogoutBody(bodyStr string, cfg Config) bool {
	trimmed := strings.TrimSpace(bodyStr)
	for _, detectText := range cfg.LogoutDetectTexts {
		if detectText == "" {
			continue
		}
		if trimmed == detectText {
			return true
		}
		if strings.Contains(bodyStr, detectText) && len(bodyStr) < 1000 {
			return true
		}
	}
	if strings.Contains(bodyStr, "Access Restricted") && strings.Contains(bodyStr, "premium SEO tool") && len(bodyStr) < 2000 {
		return true
	}
	if strings.Contains(bodyStr, "Proxy server not selected") && strings.Contains(bodyStr, "proxy_server") && len(bodyStr) < 8000 {
		return true
	}
	return false
}

// triggerSemrushAutomation sends a request to trigger Nox/Semrush auto-login automation
func triggerSemrushAutomation(cfg Config) {
	automationMutex.Lock()
	defer automationMutex.Unlock()

	now := time.Now()

	if now.Sub(lastAutomationTime) < time.Duration(cfg.CooldownSeconds)*time.Second {
		log.Printf("[AUTOMATION] Process-level Cooldown active, skipping API trigger.")
		return
	}

	if cfg.CooldownFilePath != "" {
		data, err := os.ReadFile(cfg.CooldownFilePath)
		if err == nil {
			var lastTimeUnix int64
			_, err = fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &lastTimeUnix)
			if err == nil {
				lastTime := time.Unix(lastTimeUnix, 0)
				if now.Sub(lastTime) < time.Duration(cfg.CooldownSeconds)*time.Second {
					log.Printf("[AUTOMATION] Global File-based Cooldown active, skipping API trigger.")
					return
				}
			}
		}

		err = os.WriteFile(cfg.CooldownFilePath, []byte(fmt.Sprintf("%d", now.Unix())), 0644)
		if err != nil {
			log.Printf("[AUTOMATION] Warning: failed to write cooldown file: %v", err)
		}
	}

	if cfg.AutomationURL == "" {
		log.Printf("[AUTOMATION] Automation URL is empty, skipping trigger.")
		return
	}

	lastAutomationTime = now
	log.Printf("[AUTOMATION] Session logout detected! Triggering auto-login API...")

	go func(urlStr string, payload map[string]string, headers map[string]string) {
		jsonBytes, err := json.Marshal(payload)
		if err != nil {
			log.Printf("[AUTOMATION] Error marshalling automation payload: %v", err)
			return
		}

		req, err := http.NewRequest("POST", urlStr, bytes.NewBuffer(jsonBytes))
		if err != nil {
			log.Printf("[AUTOMATION] Error creating automation request: %v", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		for k, v := range headers {
			req.Header.Set(k, v)
		}

		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client := &http.Client{
			Timeout:   10 * time.Second,
			Transport: tr,
		}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[AUTOMATION] Error calling automation API: %v", err)
			return
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		log.Printf("[AUTOMATION] Automation API response status: %d, body: %s", resp.StatusCode, string(body))
	}(cfg.AutomationURL, cfg.AutomationPayload, cfg.AutomationHeaders)
}

// isWebSocket checks if the request is a WebSocket upgrade request
func isWebSocket(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") &&
		strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// handleWebSocket proxies WebSocket upgrades to the upstream target
func handleWebSocket(w http.ResponseWriter, r *http.Request, targetURL *url.URL) {
	cfg := loadConfig()

	targetAddr := targetURL.Host
	if !strings.Contains(targetAddr, ":") {
		if targetURL.Scheme == "https" {
			targetAddr += ":443"
		} else {
			targetAddr += ":80"
		}
	}

	var targetConn net.Conn
	var err error
	if marked, px := panelUpstreamProxy(r.Context()); marked && px != "" {
		tcpConn, dialErr := panelDialThrough(r.Context(), targetAddr, px)
		if dialErr != nil {
			log.Printf("[WS] proxy dial failed %s: %v", targetAddr, dialErr)
			renderProxyProblem(w, r)
			return
		}
		if targetURL.Scheme == "https" {
			tlsConn := tls.Client(tcpConn, &tls.Config{ServerName: targetURL.Hostname(), InsecureSkipVerify: true})
			if dialErr = tlsConn.Handshake(); dialErr != nil {
				tcpConn.Close()
				renderProxyProblem(w, r)
				return
			}
			targetConn = tlsConn
		} else {
			targetConn = tcpConn
		}
	} else if targetURL.Scheme == "https" {
		targetConn, err = tls.Dial("tcp", targetAddr, &tls.Config{
			ServerName:         targetURL.Hostname(),
			InsecureSkipVerify: true,
		})
	} else {
		targetConn, err = net.Dial("tcp", targetAddr)
	}
	if err != nil {
		log.Printf("[WS] Failed to dial target %s: %v", targetAddr, err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	publicHost := resolveProxyHost(r, cfg)
	publicScheme := resolveProxyScheme(r, cfg)
	targetHost := targetURL.Host

	r.Header.Set("Host", targetHost)
	r.Header.Del("X-Proxy-Host")
	r.Header.Del("X-Proxy-Scheme")
	if cfg.UserAgent != "preserve" && cfg.UserAgent != "" {
		r.Header.Set("User-Agent", cfg.UserAgent)
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		ref = strings.ReplaceAll(ref, publicHost, targetHost)
		ref = strings.ReplaceAll(ref, publicScheme+"://", targetURL.Scheme+"://")
		r.Header.Set("Referer", ref)
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		origin = strings.ReplaceAll(origin, publicHost, targetHost)
		origin = strings.ReplaceAll(origin, publicScheme+"://", targetURL.Scheme+"://")
		r.Header.Set("Origin", origin)
	}

	premiumCookies := applyPanelPremiumCookie(r, loadCookiesFromFile(cfg.CookiePath))
	mergedCookies := mergeCookies(r.Header.Get("Cookie"), premiumCookies)
	if mergedCookies != "" {
		r.Header.Set("Cookie", mergedCookies)
	}

	var reqBuf bytes.Buffer
	fmt.Fprintf(&reqBuf, "%s %s HTTP/1.1\r\n", r.Method, r.URL.RequestURI())
	r.Header.Write(&reqBuf)
	reqBuf.WriteString("\r\n")

	if _, err = targetConn.Write(reqBuf.Bytes()); err != nil {
		targetConn.Close()
		log.Printf("[WS] Failed to write handshake to %s%s: %v", targetHost, r.URL.RequestURI(), err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	br := bufio.NewReader(targetConn)
	resp, err := http.ReadResponse(br, r)
	if err != nil {
		targetConn.Close()
		log.Printf("[WS] Failed to read upstream response for %s: %v", r.URL.RequestURI(), err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	if resp.StatusCode != http.StatusSwitchingProtocols {
		defer resp.Body.Close()
		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
		targetConn.Close()
		log.Printf("[WS] Upgrade rejected for %s: %d", r.URL.RequestURI(), resp.StatusCode)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		resp.Body.Close()
		targetConn.Close()
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		resp.Body.Close()
		targetConn.Close()
		log.Printf("[WS] Hijack failed: %v", err)
		return
	}

	var respBuf bytes.Buffer
	if err := resp.Write(&respBuf); err != nil {
		clientConn.Close()
		targetConn.Close()
		log.Printf("[WS] Failed to serialize upgrade response: %v", err)
		return
	}
	resp.Body.Close()

	if _, err = clientConn.Write(respBuf.Bytes()); err != nil {
		clientConn.Close()
		targetConn.Close()
		log.Printf("[WS] Failed to write upgrade response to client: %v", err)
		return
	}

	log.Printf("[WS] ✅ Tunnel established: %s%s", targetHost, r.URL.RequestURI())

	upstream := &bufferedConn{Conn: targetConn, r: br}
	errChan := make(chan error, 2)
	go func() { _, err := io.Copy(upstream, clientConn); errChan <- err }()
	go func() { _, err := io.Copy(clientConn, upstream); errChan <- err }()
	<-errChan
	clientConn.Close()
	targetConn.Close()
}

type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

// CustomRoundTripper wraps a transport to drop headers like X-Forwarded-For before forwarding
type CustomRoundTripper struct {
	Transport http.RoundTripper
}

func (crt *CustomRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clean WAF tracking / IP headers
	req.Header.Del("X-Forwarded-For")
	req.Header.Del("X-Real-IP")

	// Delete client hints to avoid mismatch with spoofed User-Agent
	req.Header.Del("Sec-Ch-Ua")
	req.Header.Del("Sec-Ch-Ua-Mobile")
	req.Header.Del("Sec-Ch-Ua-Platform")
	req.Header.Del("Sec-Ch-Ua-Model")
	req.Header.Del("Sec-Ch-Ua-Arch")
	req.Header.Del("Sec-Ch-Ua-Full-Version")
	req.Header.Del("Sec-Ch-Ua-Full-Version-List")
	req.Header.Del("Sec-Ch-Ua-Platform-Version")
	req.Header.Del("Sec-Ch-Ua-Bitness")

	if marked, px := panelUpstreamProxy(req.Context()); marked && px != "" {
		return panelProxyTransport().RoundTrip(req)
	}
	return crt.Transport.RoundTrip(req)
}

func main() {
	cfg := loadConfig()

	// Always connect MySQL when credentials are set (needed for /api/auth-handshake even in local_test_mode)
	if cfg.MySQLHost != "" && cfg.MySQLDB != "" {
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=Asia%%2FKolkata",
			cfg.MySQLUser, cfg.MySQLPassword, cfg.MySQLHost, cfg.MySQLPort, cfg.MySQLDB)
		var err error
		db, err = sql.Open("mysql", dsn)
		if err != nil {
			log.Printf("[DB] ⚠️ MySQL Connection failed to initialize: %v", err)
		} else {
			db.SetMaxOpenConns(25)
			db.SetMaxIdleConns(5)
			db.SetConnMaxLifetime(5 * time.Minute)
			if err := db.Ping(); err != nil {
				log.Printf("[DB] ⚠️ Could not connect to MySQL: %v. Database mode is offline.", err)
			} else {
				log.Printf("[DB] Connected to MySQL successfully! Database: %s ✅", cfg.MySQLDB)
				resolveWebsiteID(cfg.PublicHost)
			}
		}
	}
	if cfg.LocalTestMode {
		log.Printf("[CONFIG] Running in LOCAL TEST MODE. Page auth bypassed; handshake still uses DB if connected. 🧪")
	}

	targetUrl, err := url.Parse(cfg.TargetURL)
	if err != nil {
		log.Fatalf("[FATAL] Invalid target URL: %v", err)
	}

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(targetUrl)

	proxy.Transport = &CustomRoundTripper{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}

	// Proxy Director
	proxy.Director = func(req *http.Request) {
		cfg = loadConfig()

		tUrl, err := url.Parse(cfg.TargetURL)
		if err != nil {
			log.Printf("[PROXY] Error parsing dynamic target URL: %v", err)
			return
		}
		targetHost := tUrl.Host

		publicHost := resolveProxyHost(req, cfg)
		publicScheme := resolveProxyScheme(req, cfg)

		req.Header.Set("X-Proxy-Host", publicHost)
		req.Header.Set("X-Proxy-Scheme", publicScheme)
		debugLog(cfg, "Director %s %s → host=%s scheme=%s target=%s", req.Method, req.URL.Path, publicHost, publicScheme, targetHost)

		req.Header.Set("Host", targetHost)
		req.Host = targetHost
		req.URL.Host = targetHost
		req.URL.Scheme = tUrl.Scheme

		if cfg.UserAgent != "preserve" && cfg.UserAgent != "" {
			req.Header.Set("User-Agent", cfg.UserAgent)
		}
		req.Header.Set("Accept-Encoding", "gzip")

		req.Header.Del("X-Forwarded-For")
		req.Header.Del("X-Real-IP")
		req.Header.Del("X-Forwarded-Host")
		req.Header.Del("X-Forwarded-Proto")

		premiumCookies := applyPanelPremiumCookie(req, loadCookiesFromFile(cfg.CookiePath))
		clientCookies := req.Header.Get("Cookie")
		mergedCookies := mergeCookies(clientCookies, premiumCookies)
		if mergedCookies != "" {
			req.Header.Set("Cookie", mergedCookies)
		}

		if ref := req.Header.Get("Referer"); ref != "" {
			replacedRef := strings.ReplaceAll(ref, publicHost, targetHost)
			replacedRef = strings.ReplaceAll(replacedRef, publicScheme+"://", tUrl.Scheme+"://")
			req.Header.Set("Referer", replacedRef)
		}
		if origin := req.Header.Get("Origin"); origin != "" {
			replacedOrigin := strings.ReplaceAll(origin, publicHost, targetHost)
			replacedOrigin = strings.ReplaceAll(replacedOrigin, publicScheme+"://", tUrl.Scheme+"://")
			req.Header.Set("Origin", replacedOrigin)
		}

		rawQuery := req.URL.RawQuery
		if rawQuery != "" {
			encodedProxy := url.QueryEscape(publicScheme + "://" + publicHost)
			encodedTarget := url.QueryEscape(tUrl.Scheme + "://" + targetHost)
			rawQuery = strings.ReplaceAll(rawQuery, encodedProxy, encodedTarget)

			encodedProxyNoScheme := url.QueryEscape(publicHost)
			encodedTargetNoScheme := url.QueryEscape(targetHost)
			rawQuery = strings.ReplaceAll(rawQuery, encodedProxyNoScheme, encodedTargetNoScheme)

			rawQuery = strings.ReplaceAll(rawQuery, publicHost, targetHost)
			req.URL.RawQuery = rawQuery
		}

		if req.Method == "POST" {
			req.Header.Set("X-Kl-Ajax-Request", "Ajax_Request")
		}
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		cfg = loadConfig()

		tUrl, err := url.Parse(cfg.TargetURL)
		if err != nil {
			return err
		}
		targetHost := tUrl.Host

		proxyHost := ""
		proxyScheme := ""
		if resp.Request != nil {
			proxyHost = resp.Request.Header.Get("X-Proxy-Host")
			proxyScheme = resp.Request.Header.Get("X-Proxy-Scheme")
		}
		if proxyHost == "" {
			proxyHost = cfg.PublicHost
		}
		if proxyScheme == "" {
			proxyScheme = cfg.PublicScheme
		}

		debugLog(cfg, "Response %s status=%d proxy=%s://%s ctype=%s",
			resp.Request.URL.Path, resp.StatusCode, proxyScheme, proxyHost, resp.Header.Get("Content-Type"))

		if loc := resp.Header.Get("Location"); loc != "" {
			if isLoginRedirectURL(loc, cfg) {
				triggerSemrushAutomation(cfg)

				resp.StatusCode = http.StatusOK
				resp.Status = "200 OK"
				resp.Header.Del("Location")
				resp.Header.Set("Content-Type", "text/html; charset=utf-8")
				resp.Header.Del("Content-Length")
				resp.Header.Del("Content-Encoding")

				resp.Body = io.NopCloser(strings.NewReader(renderSessionRefreshHTML(cfg)))
				return nil
			}

			replacedLoc := strings.ReplaceAll(loc, cfg.TargetURL, proxyScheme+"://"+proxyHost)
			replacedLoc = strings.ReplaceAll(replacedLoc, "https://"+targetHost, proxyScheme+"://"+proxyHost)
			replacedLoc = strings.ReplaceAll(replacedLoc, "http://"+targetHost, proxyScheme+"://"+proxyHost)
			resp.Header.Set("Location", replacedLoc)
		}

		resp.Header.Del("Strict-Transport-Security")
		resp.Header.Del("Content-Security-Policy")
		resp.Header.Del("Content-Security-Policy-Report-Only")
		resp.Header.Del("X-Content-Security-Policy")

		targetScheme := tUrl.Scheme
		if targetScheme == "" {
			targetScheme = "https"
		}
		rewriteProxyResponseHeaders(resp, targetHost, targetScheme, proxyHost, proxyScheme)

		for _, cookie := range resp.Cookies() {
			cookie.Domain = ""
			cookie.Secure = false
		}

		contentType := resp.Header.Get("Content-Type")
		isText := (strings.Contains(contentType, "text") ||
			strings.Contains(contentType, "javascript") ||
			strings.Contains(contentType, "json") ||
			strings.Contains(contentType, "xml")) &&
			!strings.Contains(contentType, "event-stream")

		if isText {
			encoding := resp.Header.Get("Content-Encoding")
			isGzip := strings.EqualFold(encoding, "gzip")

			var reader io.Reader
			var err error
			if isGzip {
				gzipReader, err := gzip.NewReader(resp.Body)
				if err != nil {
					return err
				}
				defer gzipReader.Close()
				reader = gzipReader
			} else {
				reader = resp.Body
			}

			bodyBytes, err := io.ReadAll(reader)
			if err != nil {
				return err
			}
			resp.Body.Close()

			bodyStr := string(bodyBytes)
			proxySchemeHost := proxyScheme + "://" + proxyHost

			bodyStr = strings.ReplaceAll(bodyStr, cfg.TargetURL, proxySchemeHost)
			bodyStr = strings.ReplaceAll(bodyStr, "https://"+targetHost, proxySchemeHost)
			bodyStr = strings.ReplaceAll(bodyStr, "http://"+targetHost, proxySchemeHost)
			bodyStr = strings.ReplaceAll(bodyStr, "//"+targetHost, "//"+proxyHost)
			bodyStr = strings.ReplaceAll(bodyStr, `\/`+targetHost+`\/`, `\/`+proxyHost+`\/`)
			bodyStr = strings.ReplaceAll(bodyStr, `/`+targetHost, `/`+proxyHost)
			bodyStr = applyExtraDomainRewrites(bodyStr, proxySchemeHost, proxyHost, cfg)
			bodyStr = applyDomainPathMap(bodyStr, proxySchemeHost, proxyWsOrigin(proxyScheme, proxyHost), cfg)

			for _, repl := range cfg.Replacements {
				if repl.Search != "" {
					bodyStr = strings.ReplaceAll(bodyStr, repl.Search, repl.Replace)
				}
			}
			bodyStr = normalizeProxyWebSocketURLs(bodyStr, targetHost, proxyHost, proxyScheme)

			// Check for session expired message
			isLogout := isLogoutBody(bodyStr, cfg)
			if !isLogout && resp.Request != nil && (resp.Request.URL.Path == "/access-denied" || strings.HasSuffix(resp.Request.URL.Path, "/access-denied")) {
				isLogout = true
			}

			if isLogout {
				reqPath := "/"
				if resp.Request != nil {
					reqPath = resp.Request.URL.Path
				}
				log.Printf("[LOGOUT] Session expired detected on %s (body %d bytes) — update cookie.txt proxy_token",
					reqPath, len(bodyStr))
				triggerSemrushAutomation(cfg)
				bodyStr = renderSessionRefreshHTML(cfg)
				resp.Header.Set("Content-Type", "text/html; charset=utf-8")
				contentType = "text/html; charset=utf-8"
			}

			if strings.Contains(bodyStr, "localhost/") {
				warnPath := "?"
				if resp.Request != nil {
					warnPath = resp.Request.URL.Path
				}
				log.Printf("[WARN] Response contains localhost URLs after rewrite! path=%s proxyHost=%s scheme=%s",
					warnPath, proxyHost, proxyScheme)
			}

			bodyStr = integrityRegex.ReplaceAllString(bodyStr, "")

			if strings.Contains(contentType, "javascript") {
				bodyStr = strings.ReplaceAll(bodyStr, ".integrity=", "._ntegrity=")
				bodyStr = webpackAttrRegex.ReplaceAllString(bodyStr, `"_ntegrity":`)
				bodyStr = webpackSetAttrRegex.ReplaceAllString(bodyStr, `setAttribute("_ntegrity"`)
			}

			if strings.Contains(contentType, "text/html") && !isLogout {
				// Only inject blockScript and css/js if it doesn't use __proxy_inject.js to prevent React hydration error
				if !strings.Contains(bodyStr, "__proxy_inject.js") {
					customExportJS := ""
					if cfg.InjectExportTool {
						customExportJSBytes, err := os.ReadFile("export-tool.js")
						if err == nil {
							customExportJS = string(customExportJSBytes)
						}
					}

					blockScript := `<script>
						` + customExportJS + `
						` + buildDomainPathMapJS(cfg) + `

						window.intercomSettings = { app_id: "" };
						window.Intercom = function() { return false; };
						window.Hotjar = function() { return false; };
						window.hj = function() { return false; };

						function proxyUrl(url) {
							if (typeof url !== "string") return url;
							let u = url.trim();
							const wsOrigin = window.location.origin.replace(/^http/i, "ws");
							for (const [domain, pathPrefix] of Object.entries(DOMAIN_PATH_MAP)) {
								const pairs = [
									["https://", window.location.origin + pathPrefix],
									["http://", window.location.origin + pathPrefix],
									["wss://", wsOrigin + pathPrefix],
									["ws://", wsOrigin + pathPrefix],
									["//", window.location.protocol + "//" + window.location.host + pathPrefix]
								];
								for (const [proto, base] of pairs) {
									const prefix = proto + domain;
									if (u.startsWith(prefix)) {
										return base + u.slice(prefix.length);
									}
								}
							}
							if (u.startsWith("https://` + targetHost + `")) {
								return u.replace("https://` + targetHost + `", window.location.origin);
							}
							if (u.startsWith("http://` + targetHost + `")) {
								return u.replace("http://` + targetHost + `", window.location.origin);
							}
							if (u.startsWith("wss://` + targetHost + `")) {
								return u.replace("wss://` + targetHost + `", wsOrigin);
							}
							if (u.startsWith("ws://` + targetHost + `")) {
								return u.replace("ws://` + targetHost + `", wsOrigin);
							}
							if (u.startsWith("//` + targetHost + `")) {
								return window.location.protocol + u.replace("//` + targetHost + `", "//" + window.location.host);
							}
							return url;
						}
						
						const __originalFetch = window.fetch;
						window.fetch = function(input, init) {
							if (input) {
								if (typeof input === "string") {
									const proxied = proxyUrl(input);
									if (proxied !== input) {
										input = proxied;
									}
								} else if (input instanceof Request) {
									const originalUrl = input.url;
									const proxied = proxyUrl(originalUrl);
									if (proxied !== originalUrl) {
										const newRequest = new Request(proxied, {
											method: input.method,
											headers: input.headers,
											body: input.body,
											mode: input.mode,
											credentials: input.credentials,
											cache: input.cache,
											redirect: input.redirect,
											referrer: input.referrer,
											integrity: input.integrity,
											keepalive: input.keepalive,
											signal: input.signal
										});
										input = newRequest;
									}
								} else if (input instanceof URL) {
									const urlStr = input.toString();
									const proxied = proxyUrl(urlStr);
									if (proxied !== urlStr) {
										input = new URL(proxied);
									}
								}
							}
							return __originalFetch.call(this, input, init);
						};
						
						const __originalSendBeacon = window.navigator.sendBeacon;
						window.navigator.sendBeacon = function(url, data) {
							if (typeof url === "string") {
								url = proxyUrl(url);
							}
							return __originalSendBeacon.call(window.navigator, url, data);
						};
						
						const __originalXHR = window.XMLHttpRequest.prototype.open;
						window.XMLHttpRequest.prototype.open = function(method, url, async, user, password) {
							if (url) {
								let urlStr = (url instanceof URL) ? url.toString() : String(url);
								const proxied = proxyUrl(urlStr);
								if (proxied !== urlStr) {
									url = proxied;
								}
							}
							return __originalXHR.call(this, method, url, async, user, password);
						};

						const __originalWebSocket = window.WebSocket;
						window.WebSocket = function(url, protocols) {
							if (typeof url === "string") {
								url = proxyUrl(url);
								if (window.location.protocol === "http:") {
									url = url.replace(/^wss:/i, "ws:");
								}
							}
							if (protocols) {
								return new __originalWebSocket(url, protocols);
							}
							return new __originalWebSocket(url);
						};
						window.WebSocket.prototype = __originalWebSocket.prototype;
					</script>`

					cssToInject := ""
					if cfg.CustomCSS != "" {
						cssToInject = `<style type="text/css">` + cfg.CustomCSS + `</style>`
					} else if strings.EqualFold(cfg.ToolName, "Semrush") {
						cssToInject = `<style type="text/css">
						.srf-navbar__right, .ch2-visible, .srf-layout__notification, .srf-header__menu, .whatsapp-float {
							display: none !important;
						}
						.srf-header__logo::after {
							content: "` + cfg.LogoText + `" !important;
							display: inline-block !important;
							margin-left: 5px !important;
							font-weight: bold !important;
						}
					</style>`
					}

					jsToInject := ""
					if cfg.CustomJS != "" {
						jsToInject = `<script>` + cfg.CustomJS + `</script>`
					}

					headIdx := strings.Index(strings.ToLower(bodyStr), "<head>")
					if headIdx != -1 {
						bodyStr = bodyStr[:headIdx] + "<head>" + "\n" + cssToInject + "\n" + jsToInject + "\n" + blockScript + bodyStr[headIdx+6:]
					}
				}
			}

			// Append custom scripts, CSS, and blocks if this is the target's proxy injector script
			if resp.Request != nil && strings.HasSuffix(resp.Request.URL.Path, "/__proxy_inject.js") {
				customExportJS := ""
				if cfg.InjectExportTool {
					customExportJSBytes, err := os.ReadFile("export-tool.js")
					if err == nil {
						customExportJS = string(customExportJSBytes)
					}
				}

				extraJS := "\n" + `
(function() {
	// WebSocket Override
	const __originalWebSocket = window.WebSocket;
	window.WebSocket = function(url, protocols) {
		if (typeof url === "string") {
			let wsOrigin = window.location.origin.replace(/^http/i, "ws");
			url = url.replace(/wss?:\/\/` + targetHost + `/gi, wsOrigin)
			         .replace(/wss?:\/\/` + proxyHost + `/gi, wsOrigin);
			if (window.location.protocol === "http:") {
				url = url.replace(/^wss:/i, "ws:");
			}
		}
		if (protocols) {
			return new __originalWebSocket(url, protocols);
		}
		return new __originalWebSocket(url);
	};
	window.WebSocket.prototype = __originalWebSocket.prototype;

	// Intercom / Hotjar Block
	window.intercomSettings = { app_id: "" };
	window.Intercom = function() { return false; };
	window.Hotjar = function() { return false; };
	window.hj = function() { return false; };

	` + customExportJS + `
`

				if cfg.CustomCSS != "" {
					extraJS += `
	const style = document.createElement('style');
	style.type = 'text/css';
	style.innerHTML = ` + strconv.Quote(cfg.CustomCSS) + `;
	document.head.appendChild(style);
`
				} else if strings.EqualFold(cfg.ToolName, "Semrush") {
					extraJS += `
	const style = document.createElement('style');
	style.type = 'text/css';
	style.innerHTML = ` + strconv.Quote(`.srf-navbar__right, .ch2-visible, .srf-layout__notification, .srf-header__menu, .whatsapp-float {
		display: none !important;
	}
	.srf-header__logo::after {
		content: "`+cfg.LogoText+`" !important;
		display: inline-block !important;
		margin-left: 5px !important;
		font-weight: bold !important;
	}`) + `;
	document.head.appendChild(style);
`
				}

				if cfg.CustomJS != "" {
					extraJS += "\n" + cfg.CustomJS + "\n"
				}

				extraJS += "\n})();"
				bodyStr += extraJS
			}

			if usesPanelAccountMode(cfg) && strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
				bodyStr = string(injectDeviceHTML([]byte(bodyStr)))
			}
			modifiedBytes := []byte(bodyStr)

			if isGzip {
				var buf bytes.Buffer
				gzipWriter := gzip.NewWriter(&buf)
				if _, err := gzipWriter.Write(modifiedBytes); err != nil {
					return err
				}
				gzipWriter.Close()
				resp.Body = io.NopCloser(&buf)
				resp.ContentLength = int64(buf.Len())
				resp.Header.Set("Content-Length", fmt.Sprintf("%d", buf.Len()))
			} else {
				resp.Body = io.NopCloser(bytes.NewBuffer(modifiedBytes))
				resp.ContentLength = int64(len(modifiedBytes))
				resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(modifiedBytes)))
			}
			resp.Header.Del("Transfer-Encoding")
		}

		return nil
	}

	addr := ":" + cfg.Port
	log.Printf("╔══════════════════════════════════════════════════╗")
	log.Printf("║  🚀 Recloud Master Generic Proxy Server Online   ║")
	log.Printf("║  Local Address: http://localhost%s                ║", addr)
	log.Printf("║  Target Host  : %s                                ║", targetUrl.Host)
	log.Printf("║  Cookie Path  : %s                                ║", cfg.CookiePath)
	log.Printf("║  Hot-Reload   : Enabled for config & cookies ✅   ║")
	log.Printf("╚══════════════════════════════════════════════════╝")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := loadConfig()
		reqPath := r.URL.RequestURI()

		acc, nextReq, handled := preparePanelRequest(w, r, cfg)
		if handled {
			return
		}
		if usesPanelAccountMode(cfg) {
			r = stampPanelAccount(nextReq, acc)
		}

		// 1. Handle Handshake
		if r.URL.Path == "/api/auth-handshake" {
			authHandshakeHandler(w, r)
			return
		}

		// 2. Handle /access
		if r.URL.Path == "/access" {
			accessHandler(w, r)
			return
		}

		// 2.5 Block sensitive account/billing pages
		if isBlockedPath(r.URL.Path, cfg) {
			log.Printf("[BLOCK] Blocked path: %s", r.URL.Path)
			home := cfg.HomePath
			if home == "" {
				home = "/"
			}
			http.Redirect(w, r, home, http.StatusFound)
			return
		}

		// 3. Asset exceptions
		isAsset := strings.HasPrefix(reqPath, "/cdn/") ||
			strings.Contains(reqPath, ".css") ||
			strings.Contains(reqPath, ".js") ||
			strings.Contains(reqPath, ".png") ||
			strings.Contains(reqPath, ".jpg") ||
			strings.Contains(reqPath, ".svg") ||
			strings.Contains(reqPath, ".woff") ||
			strings.Contains(reqPath, "/assets/")

		// 4. Session Validation (bypassed in local test mode)
		if !isAsset && !cfg.LocalTestMode {
			_, authErr := getAuthenticatedUser(r, cfg)
			if authErr != nil {
				log.Printf("[AUTH] Unauthenticated access attempt to: %s (%v)", reqPath, authErr)
				renderAccessDeniedPage(w)
				return
			}
		}

		// 4.5. Handle WebSocket requests
		if isWebSocket(r) {
			debugLog(cfg, "WebSocket upgrade %s", reqPath)
			tUrl, err := url.Parse(cfg.TargetURL)
			if err == nil {
				handleWebSocket(w, r, tUrl)
				return
			}
		}

		debugLog(cfg, "Proxy %s %s rawHost=%s resolved=%s://%s",
			r.Method, reqPath, r.Host, resolveProxyScheme(r, cfg), resolveProxyHost(r, cfg))

		// 5. Proxy the request
		proxy.ServeHTTP(w, r)
	})

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("[FATAL] Server crash: %v", err)
	}
}
