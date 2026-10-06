package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	_ "github.com/go-sql-driver/mysql"
	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
	"golang.org/x/net/proxy"
	proxysec "toolsmandi.com/proxy-security"
)

// ── CONFIG ────────────────────────────────────────────────────────────────────

// Config holds all runtime-configurable settings loaded from config.json.
// Hot-reloadable: file is re-read on every request if it has changed.
type Config struct {
	Port                   string `json:"port"`
	TargetURL              string `json:"target_url"`
	CDNURL                 string `json:"cdn_url"`
	PublicHost             string `json:"public_host"`
	PublicScheme           string `json:"public_scheme"`
	MySQLHost              string `json:"mysql_host"`
	MySQLPort              string `json:"mysql_port"`
	MySQLUser              string `json:"mysql_user"`
	MySQLPassword          string `json:"mysql_password"`
	MySQLDB                string `json:"mysql_db"`
	SecretKey              string `json:"secret_key"`
	SessionDurationMinutes int    `json:"session_duration_minutes"`
	MemberAreaURL          string `json:"member_area_url"`
	// ── Generic tool settings ──
	ToolName    string `json:"tool_name"`
	CreditLabel string `json:"credit_label"`
	ExportLabel string `json:"export_label"`
	HomePath    string `json:"home_path"`
	// CountedPaths: exact API paths that consume 1 credit each
	CountedPaths []string `json:"counted_paths"`
	// CountedPrefixes: path prefixes that consume 1 credit (e.g. "/dashboard/")
	CountedPrefixes []string `json:"counted_prefixes"`
	// BlockedPaths: exact paths to block (account/billing/settings pages)
	BlockedPaths []string `json:"blocked_paths"`
	// StubPaths: return local JSON stub without forwarding to upstream
	StubPaths []string `json:"stub_paths"`
	// BlockedPrefixes: path prefixes to block
	BlockedPrefixes []string `json:"blocked_prefixes"`
	// ExtraCDNDomains: additional domains to rewrite in HTML responses
	ExtraCDNDomains []string `json:"extra_cdn_domains"`
	// SensitiveCookies: cookie names to strip from user's browser (prevent hijack)
	SensitiveCookies []string `json:"sensitive_cookies"`
	// WatchdogTriggers: client-side text patterns that trigger account rotation
	WatchdogTriggers []string `json:"watchdog_triggers"`
	// DisableExportTracking: set true for tools that don't have CSV export (ChatGPT, etc.)
	DisableExportTracking bool                  `json:"disable_export_tracking"`
	DisableLimits         bool                  `json:"disable_limits"`
	BypassAuth            bool                  `json:"bypass_auth"`
	UseDatabase           bool                  `json:"use_database"`
	LogoutDetection       LogoutDetectionConfig `json:"logout_detection"`
	Automation            AutomationConfig      `json:"automation"`
	SessionSecurity       proxysec.Config       `json:"session_security"`
	// UserAgent override
	UserAgent string `json:"user_agent"`
	// CookieFile: path to cookie.txt file (legacy, optional)
	CookieFile string `json:"cookie_file"`
	// PanelDB is the local panel database. When set, Open comes from the panel access link.
	PanelDB string `json:"panel_db"`
	// Local test mode — bypass aMember/DB auth; use cookie.txt directly (dev only)
	LocalTestMode bool `json:"local_test_mode"`
	BindLocalhost bool `json:"bind_localhost"`
	DebugLogging  bool `json:"debug_logging"`
}

type LogoutDetectionConfig struct {
	Enabled        bool     `json:"enabled"`
	URLPaths       []string `json:"url_paths"`
	TextSniffs     []string `json:"text_sniffs"`
	HTMLSniffs     []string `json:"html_sniffs"`
	ShowOverlay    bool     `json:"show_overlay"`
	OverlayTitle   string   `json:"overlay_title"`
	OverlayMessage string   `json:"overlay_message"`
	RefreshSeconds int      `json:"refresh_seconds"`
}

type AutomationConfig struct {
	Enabled         bool                   `json:"enabled"`
	URL             string                 `json:"url"`
	Method          string                 `json:"method"`
	Payload         map[string]interface{} `json:"payload"`
	Headers         map[string]string      `json:"headers"`
	CooldownSeconds int                    `json:"cooldown_seconds"`
}

func getConfigFile() string {
	if p := strings.TrimSpace(os.Getenv("CONFIG_FILE")); p != "" {
		return p
	}
	return "config.json"
}

var (
	creditHitCache                sync.Map
	currentConfig                 Config
	configModTime                 time.Time
	currentWebsiteID              int  = 1
	dbConnected                   bool = false
	currentWebsiteSecurityEnabled bool = true
	automationStateMu             sync.Mutex
	defaultConfig                 = Config{
		UserAgent:              "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
		Port:                   "7860",
		CookieFile:             "cookie.txt",
		TargetURL:              "https://chatgpt.com",
		CDNURL:                 "https://cdn.oaistatic.com",
		PublicHost:             "gpt.yourdomain.com",
		PublicScheme:           "https",
		MySQLHost:              "127.0.0.1",
		MySQLPort:              "3306",
		MySQLUser:              "root",
		MySQLPassword:          "",
		MySQLDB:                "toolsmandi_db",
		SecretKey:              "your_secret_key_here",
		SessionDurationMinutes: 120,
		MemberAreaURL:          "https://members.yourdomain.com/",
		ToolName:               "Tool",
		CreditLabel:            "Credits",
		ExportLabel:            "Exports",
		HomePath:               "/",
		CountedPaths:           []string{},
		CountedPrefixes:        []string{},
		BlockedPaths:           []string{},
		StubPaths:              []string{},
		BlockedPrefixes:        []string{},
		ExtraCDNDomains:        []string{},
		SensitiveCookies:       []string{},
		WatchdogTriggers:       []string{},
		DisableExportTracking:  false,
	}
)

func resolveWebsiteID(publicHost string) {
	if !dbConnected {
		log.Printf("[LOCAL] Database not connected, website_id = 1")
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
		log.Printf("[DB] ⚠️ Domain '%s' not registered in ahrefs_websites table! Using website_id = 1", publicHost)
		currentWebsiteID = 1
	} else if err != nil {
		log.Printf("[DB] ⚠️ Error querying website_id for '%s': %v. Using website_id = 1", publicHost, err)
		currentWebsiteID = 1
	} else {
		currentWebsiteID = wid
		log.Printf("[DB] Resolved website_id = %d for domain '%s' ✅", currentWebsiteID, publicHost)
	}
	refreshWebsiteSecurityFromDB()
	refreshBlockedIPsFromDB()
}

func loadConfig() Config {
	configFile := getConfigFile()
	info, err := os.Stat(configFile)
	if err != nil {
		return currentConfig
	}
	if !info.ModTime().After(configModTime) {
		return currentConfig
	}
	data, err := os.ReadFile(configFile)
	if err != nil {
		log.Printf("[CONFIG] Read error: %v — using previous config", err)
		return currentConfig
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("[CONFIG] Parse error: %v — using previous config", err)
		return currentConfig
	}
	// Fill defaults
	if cfg.UserAgent == "" {
		cfg.UserAgent = defaultConfig.UserAgent
	}
	if cfg.Port == "" {
		cfg.Port = defaultConfig.Port
	}
	if cfg.CookieFile == "" {
		cfg.CookieFile = defaultConfig.CookieFile
	}
	if cfg.TargetURL == "" {
		cfg.TargetURL = defaultConfig.TargetURL
	}
	if cfg.CDNURL == "" {
		cfg.CDNURL = defaultConfig.CDNURL
	}
	if cfg.PublicScheme == "" {
		cfg.PublicScheme = "https"
	}
	if cfg.MySQLHost == "" {
		cfg.MySQLHost = defaultConfig.MySQLHost
	}
	if cfg.MySQLPort == "" {
		cfg.MySQLPort = defaultConfig.MySQLPort
	}
	if cfg.MySQLUser == "" {
		cfg.MySQLUser = defaultConfig.MySQLUser
	}
	if cfg.MySQLDB == "" {
		cfg.MySQLDB = defaultConfig.MySQLDB
	}
	if cfg.SecretKey == "" {
		cfg.SecretKey = defaultConfig.SecretKey
	}
	if cfg.SessionDurationMinutes <= 0 {
		cfg.SessionDurationMinutes = defaultConfig.SessionDurationMinutes
	}
	if cfg.ToolName == "" {
		cfg.ToolName = defaultConfig.ToolName
	}
	if cfg.CreditLabel == "" {
		cfg.CreditLabel = defaultConfig.CreditLabel
	}
	if cfg.HomePath == "" {
		cfg.HomePath = defaultConfig.HomePath
	}

	currentConfig = cfg
	configModTime = info.ModTime()
	log.Printf("[CONFIG] Reloaded from %s ✅ (tool: %s, target: %s, local_test=%v)", configFile, cfg.ToolName, cfg.TargetURL, cfg.LocalTestMode)
	return currentConfig
}

// ── DATABASE SYSTEM ───────────────────────────────────────────────────────────

var db *sql.DB

func initDB(cfg Config) {
	if !cfg.UseDatabase {
		log.Printf("[DB] use_database=false — skipping MySQL (cookie.txt / local mode)")
		dbConnected = false
		return
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=Asia%%2FKolkata",
		cfg.MySQLUser, cfg.MySQLPassword, cfg.MySQLHost, cfg.MySQLPort, cfg.MySQLDB)
	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("[DB] Failed to open database pool: %v", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		log.Printf("[DB] ⚠️ Database not reachable: %v. Running in STANDALONE mode using '%s'.", err, cfg.CookieFile)
		dbConnected = false
		return
	}
	dbConnected = true
	log.Printf("[DB] Connected to MySQL successfully! Database: %s ✅", cfg.MySQLDB)

	tables := []string{
		`CREATE TABLE IF NOT EXISTS ahrefs_accounts (
			id INT AUTO_INCREMENT PRIMARY KEY,
			website_id INT NOT NULL DEFAULT 1,
			name VARCHAR(100) NOT NULL,
			cookie TEXT NOT NULL,
			user_agent TEXT NOT NULL,
			proxy VARCHAR(255) DEFAULT '',
			show_limit TINYINT(1) DEFAULT 1,
			status ENUM('active','logged_out','blocked') DEFAULT 'active',
			last_used_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			failure_count INT DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX (website_id), INDEX (status)
		) ENGINE=InnoDB;`,
		`CREATE TABLE IF NOT EXISTS ahrefs_users (
			id INT AUTO_INCREMENT PRIMARY KEY,
			website_id INT NOT NULL DEFAULT 1,
			username VARCHAR(100) NOT NULL,
			credit_limit INT DEFAULT 50,
			export_limit INT DEFAULT 100000,
			custom_limit_expire_at DATETIME DEFAULT NULL,
			status ENUM('active','suspended') DEFAULT 'active',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX (website_id), INDEX (username)
		) ENGINE=InnoDB;`,
		`CREATE TABLE IF NOT EXISTS ahrefs_credit_logs (
			id INT AUTO_INCREMENT PRIMARY KEY,
			website_id INT NOT NULL DEFAULT 1,
			username VARCHAR(100) NOT NULL,
			endpoint VARCHAR(255) NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			INDEX (website_id), INDEX (username), INDEX (timestamp)
		) ENGINE=InnoDB;`,
		`CREATE TABLE IF NOT EXISTS ahrefs_export_logs (
			id INT AUTO_INCREMENT PRIMARY KEY,
			website_id INT NOT NULL DEFAULT 1,
			username VARCHAR(100) NOT NULL,
			rows_count INT NOT NULL,
			endpoint VARCHAR(255) NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			INDEX (website_id), INDEX (username), INDEX (timestamp)
		) ENGINE=InnoDB;`,
		`CREATE TABLE IF NOT EXISTS ahrefs_products (
			id INT AUTO_INCREMENT PRIMARY KEY,
			website_id INT NOT NULL DEFAULT 1,
			product_id VARCHAR(100) NOT NULL,
			product_name VARCHAR(200) DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX (website_id)
		) ENGINE=InnoDB;`,
		`CREATE TABLE IF NOT EXISTS ahrefs_tokens (
			id INT AUTO_INCREMENT PRIMARY KEY,
			website_id INT NOT NULL DEFAULT 1,
			token VARCHAR(128) NOT NULL UNIQUE,
			username VARCHAR(100) NOT NULL,
			client_ip VARCHAR(64) NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX (token), INDEX (expires_at)
		) ENGINE=InnoDB;`,
		`CREATE TABLE IF NOT EXISTS ahrefs_sessions (
			id INT AUTO_INCREMENT PRIMARY KEY,
			website_id INT NOT NULL DEFAULT 1,
			session_token VARCHAR(128) NOT NULL UNIQUE,
			username VARCHAR(100) NOT NULL,
			client_ip VARCHAR(64) NOT NULL,
			assigned_account_id INT DEFAULT NULL,
			expires_at DATETIME NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX (website_id), INDEX (session_token), INDEX (expires_at)
		) ENGINE=InnoDB;`,
		`CREATE TABLE IF NOT EXISTS ahrefs_violations_logs (
			id INT AUTO_INCREMENT PRIMARY KEY,
			website_id INT NOT NULL DEFAULT 1,
			username VARCHAR(100) NOT NULL,
			client_ip VARCHAR(50) NOT NULL,
			attempted_path VARCHAR(255) NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			INDEX (website_id), INDEX (username), INDEX (timestamp)
		) ENGINE=InnoDB;`,
		`CREATE TABLE IF NOT EXISTS ahrefs_switch_logs (
			id INT AUTO_INCREMENT PRIMARY KEY,
			website_id INT NOT NULL DEFAULT 1,
			session_token VARCHAR(128),
			username VARCHAR(100),
			from_account_id INT,
			from_account_name VARCHAR(100),
			to_account_id INT,
			to_account_name VARCHAR(100),
			reason VARCHAR(255),
			switched_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			INDEX (website_id), INDEX (switched_at)
		) ENGINE=InnoDB;`,
		`CREATE TABLE IF NOT EXISTS ahrefs_login_logs (
			id INT AUTO_INCREMENT PRIMARY KEY,
			website_id INT NOT NULL DEFAULT 1,
			username VARCHAR(100) NOT NULL,
			client_ip VARCHAR(64) NOT NULL,
			user_agent TEXT,
			logged_in_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			INDEX (website_id), INDEX (username), INDEX (logged_in_at)
		) ENGINE=InnoDB;`,
		`CREATE TABLE IF NOT EXISTS ahrefs_websites (
			id INT AUTO_INCREMENT PRIMARY KEY,
			tool_id INT NOT NULL DEFAULT 1,
			name VARCHAR(100) NOT NULL,
			domain VARCHAR(255) NOT NULL UNIQUE,
			secret_key VARCHAR(255) NOT NULL,
			session_duration INT DEFAULT 120,
			default_credit_limit INT DEFAULT 50,
			default_export_limit INT DEFAULT 100000,
			limit_label_1 VARCHAR(50) DEFAULT 'Credits',
			limit_label_2 VARCHAR(50) DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB;`,
	}
	for i, q := range tables {
		if _, err := db.Exec(q); err != nil {
			log.Printf("[DB] ⚠️ Table %d create error: %v", i+1, err)
		}
	}
	_, _ = db.Exec("DELETE FROM ahrefs_tokens WHERE expires_at < NOW()")
	_, _ = db.Exec("DELETE FROM ahrefs_sessions WHERE expires_at < NOW()")
	_, _ = db.Exec("ALTER TABLE ahrefs_sessions ADD COLUMN device_token VARCHAR(64) DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE ahrefs_websites ADD COLUMN session_security_enabled TINYINT(1) DEFAULT 1")
	log.Printf("[DB] Startup cleanup done ✅")
}

// ── CREDIT / LIMIT SYSTEM ─────────────────────────────────────────────────────

func isCountedPath(path string, cfg Config) bool {
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}
	path = strings.TrimSuffix(path, "/")
	for _, p := range cfg.CountedPaths {
		if path == p {
			return true
		}
	}
	for _, prefix := range cfg.CountedPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func isStubPath(path string, cfg Config) bool {
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}
	path = strings.TrimSuffix(path, "/")
	for _, p := range cfg.StubPaths {
		if path == p {
			return true
		}
	}
	return false
}

func stubPathResponse(path string) []byte {
	switch strings.TrimSuffix(path, "/") {
	case "/api/account/member-preferences":
		return []byte(`{}`)
	default:
		return []byte(`{}`)
	}
}

func isBlockedPath(path string, cfg Config) bool {
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}
	path = strings.TrimSuffix(path, "/")
	for _, p := range cfg.BlockedPaths {
		if path == p {
			return true
		}
	}
	for _, prefix := range cfg.BlockedPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func startDailyResetCron() {
	go func() {
		for {
			loc, err := time.LoadLocation("Asia/Kolkata")
			if err != nil {
				loc = time.Local
			}
			now := time.Now().In(loc)
			next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)
			time.Sleep(next.Sub(now))
			if db != nil {
				if _, err := db.Exec("DELETE FROM ahrefs_credit_logs WHERE website_id = ? AND DATE(timestamp) < CURDATE()", currentWebsiteID); err != nil {
					log.Printf("[CRON] Credit log cleanup failed: %v", err)
				} else {
					log.Printf("[CRON] Midnight IST: daily credit logs reset for website_id=%d ✅", currentWebsiteID)
				}
			}
		}
	}()
}

// ── ACCOUNT SYSTEM ────────────────────────────────────────────────────────────

type ToolAccount struct {
	ID                int
	Name              string
	Cookie            string
	UserAgent         string
	Proxy             string
	ShowLimit         bool
	AutomationTaskUID string
}

func selectActiveAccount() (ToolAccount, error) {
	var acc ToolAccount
	var showLimitVal int
	q := "SELECT id, name, cookie, user_agent, proxy, show_limit FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' ORDER BY last_used_at ASC LIMIT 1"
	err := db.QueryRow(q, currentWebsiteID).Scan(&acc.ID, &acc.Name, &acc.Cookie, &acc.UserAgent, &acc.Proxy, &showLimitVal)
	if err != nil {
		return acc, err
	}
	acc.ShowLimit = showLimitVal == 1
	_, _ = db.Exec("UPDATE ahrefs_accounts SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?", acc.ID)
	return acc, nil
}

func getSessionAssignedAccount(sessionToken string) (ToolAccount, bool) {
	var acc ToolAccount
	var showLimitVal int
	var assignedAccountID sql.NullInt64
	if db == nil || !dbConnected {
		return acc, false
	}
	err := db.QueryRow("SELECT assigned_account_id FROM ahrefs_sessions WHERE session_token = ? AND website_id = ?", sessionToken, currentWebsiteID).Scan(&assignedAccountID)
	if err != nil || !assignedAccountID.Valid {
		return acc, false
	}
	err = db.QueryRow("SELECT id, name, cookie, user_agent, proxy, show_limit FROM ahrefs_accounts WHERE id = ? AND website_id = ? AND status = 'active'",
		assignedAccountID.Int64, currentWebsiteID).Scan(&acc.ID, &acc.Name, &acc.Cookie, &acc.UserAgent, &acc.Proxy, &showLimitVal)
	if err != nil {
		return acc, false
	}
	acc.ShowLimit = showLimitVal == 1
	return acc, true
}

func autoAssignNextAccount(sessionToken string) (ToolAccount, error) {
	acc, err := selectActiveAccount()
	if err != nil {
		return acc, err
	}
	_, _ = db.Exec("UPDATE ahrefs_sessions SET assigned_account_id = ? WHERE session_token = ? AND website_id = ?", acc.ID, sessionToken, currentWebsiteID)
	log.Printf("[LB] Assigned account '%s' (ID:%d) to session '%s'", acc.Name, acc.ID, sessionToken)
	return acc, nil
}

func switchToNextAccount(sessionToken string, currentAccID int, currentAccName, username, reason string) (ToolAccount, error) {
	var acc ToolAccount
	var showLimitVal int
	log.Printf("[LB] Rotating out '%s' (ID:%d) | Trigger: %s", currentAccName, currentAccID, reason)
	// Try next higher-ID account
	err := db.QueryRow(
		"SELECT id, name, cookie, user_agent, proxy, show_limit FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' AND id > ? ORDER BY id ASC LIMIT 1",
		currentWebsiteID, currentAccID,
	).Scan(&acc.ID, &acc.Name, &acc.Cookie, &acc.UserAgent, &acc.Proxy, &showLimitVal)
	if err == sql.ErrNoRows {
		// Wrap around
		err = db.QueryRow(
			"SELECT id, name, cookie, user_agent, proxy, show_limit FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' AND id != ? ORDER BY id ASC LIMIT 1",
			currentWebsiteID, currentAccID,
		).Scan(&acc.ID, &acc.Name, &acc.Cookie, &acc.UserAgent, &acc.Proxy, &showLimitVal)
	}
	if err != nil {
		// Reuse same
		err = db.QueryRow(
			"SELECT id, name, cookie, user_agent, proxy, show_limit FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' ORDER BY id ASC LIMIT 1",
			currentWebsiteID,
		).Scan(&acc.ID, &acc.Name, &acc.Cookie, &acc.UserAgent, &acc.Proxy, &showLimitVal)
	}
	if err != nil {
		return acc, fmt.Errorf("no active accounts for website_id %d", currentWebsiteID)
	}
	acc.ShowLimit = showLimitVal == 1
	_, _ = db.Exec("UPDATE ahrefs_accounts SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?", acc.ID)
	if sessionToken != "" {
		_, _ = db.Exec("UPDATE ahrefs_sessions SET assigned_account_id = ? WHERE session_token = ? AND website_id = ?", acc.ID, sessionToken, currentWebsiteID)
	}
	_, _ = db.Exec(
		"INSERT INTO ahrefs_switch_logs (website_id, session_token, username, from_account_id, from_account_name, to_account_id, to_account_name, reason) VALUES (?,?,?,?,?,?,?,?)",
		currentWebsiteID, sessionToken, username, currentAccID, currentAccName, acc.ID, acc.Name, reason,
	)
	log.Printf("[LB] 🔄 Switched '%s'→'%s' for user '%s' | %s", currentAccName, acc.Name, username, reason)
	return acc, nil
}

// ── COOKIE HELPERS ────────────────────────────────────────────────────────────

func sanitizeCookieHeader(s string) string {
	return strings.TrimSpace(strings.NewReplacer("\r", "", "\n", "", "\x00", "", "\t", " ").Replace(s))
}

func parseCookieFromDB(raw string) string {
	raw = sanitizeCookieHeader(raw)
	if raw == "" {
		return ""
	}
	trimmed := strings.TrimSpace(raw)
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
	if !strings.HasPrefix(trimmed, "[") {
		return raw
	}
	var cookies []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(trimmed), &cookies); err != nil {
		return raw
	}
	parts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		if c.Name != "" {
			parts = append(parts, c.Name+"="+c.Value)
		}
	}
	return strings.Join(parts, "; ")
}

func loadCookiesFromFile(path string) string {
	if path == "" {
		path = "cookie.txt"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[COOKIE] Error reading %s: %v", path, err)
		return ""
	}
	return parseCookieFromDB(strings.TrimSpace(string(data)))
}

func getLocalTestAccount(cfg Config) ToolAccount {
	return ToolAccount{
		ID:        0,
		Name:      "local-cookie.txt",
		Cookie:    loadCookiesFromFile(cfg.CookieFile),
		UserAgent: cfg.UserAgent,
		ShowLimit: false,
	}
}

func localBrowserCookieNames() map[string]bool {
	return map[string]bool{
		"sid_er": true, "XSRF-TOKEN": true, "er_sess_x": true,
	}
}

func forwardUpstreamSetCookies(w http.ResponseWriter, r *http.Request, upstreamResp *http.Response, cfg Config) {
	if !cfg.LocalTestMode {
		return
	}
	secure := strings.EqualFold(cfg.PublicScheme, "https") || r.TLS != nil
	for _, sc := range upstreamResp.Header.Values("Set-Cookie") {
		c, err := http.ParseSetCookie(sc)
		if err != nil || c.Name == "" {
			continue
		}
		if localBrowserCookieNames()[c.Name] && c.Value != "" {
			localCookieMu.Lock()
			if localCookieCache == nil {
				localCookieCache = make(map[string]string)
			}
			localCookieCache[c.Name] = c.Value
			localCookieAt = time.Now()
			localCookieMu.Unlock()
		}
		c.Domain = ""
		if secure {
			c.Secure = true
		}
		if c.SameSite == 0 {
			c.SameSite = http.SameSiteLaxMode
		}
		http.SetCookie(w, c)
	}
}

func setProxyBrowserAuthCookies(w http.ResponseWriter, r *http.Request, cfg Config, activeAcc ToolAccount) {
	// Panel mode: NEVER put Mapped Account cookies on the browser.
	// That bloated Cookie headers → nginx "400 Request Header Or Cookie Too Large".
	// Upstream auth uses buildUpstreamCookies() from panel.db only.
	if usesPanelAccountMode(cfg) || usesCookieFileMode(cfg) || activeAcc.ID == 0 {
		return
	}
	cookieStr := parseCookieFromDB(activeAcc.Cookie)
	if cookieStr == "" {
		return
	}
	secure := cookieSecure(r, cfg)
	authNames := localBrowserCookieNames()
	for name, value := range parseCookieMap(cookieStr) {
		if !authNames[name] || value == "" {
			continue
		}
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    value,
			Path:     "/",
			MaxAge:   86400,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

// expireProxyHostJunkCookies clears upstream/session cookies previously leaked onto
// erank.gt4rents.com so nginx stops rejecting oversized Cookie headers.
func expireProxyHostJunkCookies(w http.ResponseWriter, r *http.Request, cfg Config) {
	if !usesPanelAccountMode(cfg) || r == nil {
		return
	}
	keep := map[string]bool{
		"ct_session": true,
	}
	ss := normalizeSessionSecurity(cfg)
	if ss.DeviceCookie.Enabled {
		name := ss.DeviceCookie.CookieName
		if name == "" {
			name = "tm_device"
		}
		keep[name] = true
	}
	secure := cookieSecure(r, cfg)
	for name := range parseCookieMap(r.Header.Get("Cookie")) {
		if keep[name] || strings.HasPrefix(name, "tm_device") {
			continue
		}
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			Expires:  time.Unix(0, 0),
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

func setLocalBrowserAuthCookies(w http.ResponseWriter, r *http.Request, cfg Config) {
	if !cfg.LocalTestMode {
		return
	}
	secure := strings.EqualFold(cfg.PublicScheme, "https") || r.TLS != nil
	best := localEssentialCookies(getLocalUpstreamCookieMap(cfg, false))
	for name, value := range best {
		httpOnly := name == "sid_er" || name == "er_sess_x"
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    value,
			Path:     "/",
			MaxAge:   86400,
			Secure:   secure,
			HttpOnly: httpOnly,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

func enrichUpstreamDatadomeHeaders(upstreamReq, clientReq *http.Request, cfg Config) {
	for _, h := range []string{
		"X-Dd-B", "X-Datadome-Clientid", "X-Datadome",
		"Sec-Ch-Ua", "Sec-Ch-Ua-Mobile", "Sec-Ch-Ua-Platform", "Sec-Ch-Ua-Arch",
		"Sec-Ch-Ua-Full-Version-List", "Sec-Ch-Ua-Model", "Sec-Ch-Device-Memory",
	} {
		if v := clientReq.Header.Get(h); v != "" {
			upstreamReq.Header.Set(h, v)
		}
	}
	if cr := clientReq.Header.Get("Client-Referer"); cr != "" {
		publicBase := fmt.Sprintf("%s://%s", cfg.PublicScheme, cfg.PublicHost)
		targetParsed, _ := url.Parse(cfg.TargetURL)
		officialBase := cfg.TargetURL
		if targetParsed != nil && targetParsed.Scheme != "" && targetParsed.Host != "" {
			officialBase = targetParsed.Scheme + "://" + targetParsed.Host
		}
		upstreamReq.Header.Set("Client-Referer", strings.ReplaceAll(cr, publicBase, officialBase))
	}
}

func enrichUpstreamAuthHeaders(upstreamReq *http.Request, cfg Config, activeAcc ToolAccount) {
	cookieStr := parseCookieFromDB(activeAcc.Cookie)
	if cookieStr == "" {
		cookieStr = loadCookiesFromFile(cfg.CookieFile)
	}
	for _, part := range strings.Split(cookieStr, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "auth_token=") {
			token := strings.TrimPrefix(part, "auth_token=")
			if token != "" {
				upstreamReq.Header.Set("Authorization", "Bearer "+token)
			}
			break
		}
	}
	if ck := upstreamReq.Header.Get("Cookie"); ck != "" && !cfg.LocalTestMode {
		enrichUpstreamCSRFHeaders(upstreamReq, ck)
	} else if cookieStr != "" && !cfg.LocalTestMode {
		enrichUpstreamCSRFHeaders(upstreamReq, cookieStr)
	}
}

func enrichUpstreamCSRFHeaders(upstreamReq *http.Request, cookieStr string) {
	if upstreamReq.Header.Get("X-XSRF-TOKEN") != "" {
		return
	}
	for _, part := range strings.Split(cookieStr, ";") {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(part, "XSRF-TOKEN=") {
			continue
		}
		token := strings.TrimPrefix(part, "XSRF-TOKEN=")
		if decoded, err := url.QueryUnescape(token); err == nil {
			token = decoded
		}
		if token != "" {
			upstreamReq.Header.Set("X-XSRF-TOKEN", token)
			upstreamReq.Header.Set("X-CSRF-TOKEN", token)
		}
		break
	}
}

func parseCookieMap(cookieStr string) map[string]string {
	m := make(map[string]string)
	for _, part := range strings.Split(cookieStr, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if eqIdx := strings.Index(part, "="); eqIdx != -1 {
			m[strings.TrimSpace(part[:eqIdx])] = part[eqIdx+1:]
		}
	}
	return m
}

func cookieMapToString(m map[string]string) string {
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "; ")
}

func buildUpstreamCookies(r *http.Request, cfg Config, activeAcc ToolAccount) string {
	if usesPanelAccountMode(cfg) && strings.TrimSpace(activeAcc.Cookie) != "" {
		// Panel: use Mapped Account cookie ONLY — never merge bloated browser cookies.
		return parseCookieFromDB(activeAcc.Cookie)
	}
	if cfg.LocalTestMode {
		return cookieMapToString(getLocalUpstreamCookieMap(cfg, false))
	}
	accountMap := parseCookieMap(parseCookieFromDB(activeAcc.Cookie))
	clientCookies := stripSensitiveCookies(r.Header.Get("Cookie"), cfg)
	merged := parseCookieMap(clientCookies)
	for k, v := range accountMap {
		merged[k] = v
	}
	return cookieMapToString(merged)
}

func applyUpstreamCSRF(upstreamReq *http.Request) {
	switch upstreamReq.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return
	}
	cookieStr := upstreamReq.Header.Get("Cookie")
	if cookieStr == "" {
		return
	}
	m := parseCookieMap(cookieStr)
	raw := m["XSRF-TOKEN"]
	if raw == "" {
		return
	}
	// Laravel expects URL-decoded cookie value in X-XSRF-TOKEN (same as Axios).
	token, err := url.QueryUnescape(raw)
	if err != nil || token == "" {
		token = raw
	}
	upstreamReq.Header.Set("X-XSRF-TOKEN", token)
	upstreamReq.Header.Del("X-CSRF-TOKEN")
}

var (
	localCookieMu    sync.Mutex
	localCookieCache map[string]string
	localCookieAt    time.Time
)

func localEssentialCookies(m map[string]string) map[string]string {
	out := make(map[string]string)
	for _, k := range []string{"sid_er", "XSRF-TOKEN", "er_sess_x"} {
		if v := m[k]; v != "" {
			out[k] = v
		}
	}
	return out
}

func refreshCSRFViaSanctum(cfg Config, base map[string]string) map[string]string {
	ess := localEssentialCookies(base)
	if ess["sid_er"] == "" {
		return base
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(cfg.TargetURL, "/")+"/sanctum/csrf-cookie", nil)
	if err != nil {
		return base
	}
	req.Header.Set("Cookie", cookieMapToString(ess))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", cfg.TargetURL+"/")
	req.Header.Set("Origin", cfg.TargetURL)
	if cfg.UserAgent != "" {
		req.Header.Set("User-Agent", cfg.UserAgent)
	}
	resp, err := pickHTTPClient(cfg, "/sanctum/csrf-cookie").Do(req)
	if err != nil {
		log.Printf("[PROXY:CSRF] sanctum refresh failed: %v", err)
		return base
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("[PROXY:CSRF] sanctum refresh status=%d", resp.StatusCode)
		return base
	}
	out := make(map[string]string)
	for k, v := range base {
		out[k] = v
	}
	for _, sc := range resp.Header.Values("Set-Cookie") {
		c, err := http.ParseSetCookie(sc)
		if err != nil || c.Name == "" || c.Value == "" {
			continue
		}
		out[c.Name] = c.Value
	}
	if out["XSRF-TOKEN"] != "" {
		log.Printf("[PROXY:CSRF] sanctum refreshed XSRF-TOKEN ok")
	}
	return out
}

func invalidateLocalCookieCache() {
	localCookieMu.Lock()
	localCookieCache = nil
	localCookieAt = time.Time{}
	localCookieMu.Unlock()
}

func getLocalUpstreamCookieMap(cfg Config, forceRefresh bool) map[string]string {
	localCookieMu.Lock()
	defer localCookieMu.Unlock()
	if !forceRefresh && len(localCookieCache) > 0 && time.Since(localCookieAt) < 3*time.Minute {
		out := make(map[string]string, len(localCookieCache))
		for k, v := range localCookieCache {
			out[k] = v
		}
		return out
	}
	base := parseCookieMap(loadCookiesFromFile(cfg.CookieFile))
	if forceRefresh {
		base = refreshCSRFViaSanctum(cfg, base)
	}
	if forceRefresh || len(localCookieCache) == 0 {
		localCookieCache = base
		localCookieAt = time.Now()
	}
	out := make(map[string]string, len(base))
	for k, v := range base {
		out[k] = v
	}
	return out
}

func getLocalUpstreamCookieHeader(cfg Config, forceRefresh bool) string {
	return cookieMapToString(getLocalUpstreamCookieMap(cfg, forceRefresh))
}

func stripSensitiveCookies(cookieHeader string, cfg Config) string {
	if cookieHeader == "" {
		return ""
	}
	sensitiveSet := make(map[string]bool)
	for _, n := range cfg.SensitiveCookies {
		sensitiveSet[strings.ToLower(n)] = true
	}
	if len(sensitiveSet) == 0 {
		return cookieHeader
	}
	parts := strings.Split(cookieHeader, ";")
	var kept []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if eqIdx := strings.Index(trimmed, "="); eqIdx != -1 {
			name := strings.ToLower(strings.TrimSpace(trimmed[:eqIdx]))
			if sensitiveSet[name] {
				continue
			}
		}
		kept = append(kept, trimmed)
	}
	return strings.Join(kept, "; ")
}

// ── AUTHENTICATION SYSTEM ─────────────────────────────────────────────────────

func generateOTT() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func getAuthenticatedUser(r *http.Request, cfg Config) (string, error) {
	if usesPanelAccountMode(cfg) {
		return panelSessionUsername(r)
	}
	if usesCookieFileMode(cfg) {
		return "local_dev", nil
	}
	cookie, err := r.Cookie("ct_session")
	if err != nil {
		return "", fmt.Errorf("ct_session: %w", err)
	}
	sessionToken := cookie.Value
	if sessionToken == "" {
		return "", fmt.Errorf("empty session token")
	}
	var username, sessionIP, deviceToken string
	var expiresAt time.Time
	err = db.QueryRow(
		"SELECT username, expires_at, client_ip, COALESCE(device_token, '') FROM ahrefs_sessions WHERE session_token = ? AND website_id = ?",
		sessionToken, currentWebsiteID,
	).Scan(&username, &expiresAt, &sessionIP, &deviceToken)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("session not found")
	}
	if err != nil {
		return "", fmt.Errorf("db session lookup: %w", err)
	}
	if time.Now().After(expiresAt) {
		recordSecurityEvent(r, username, "session_expired", "")
		killSession(sessionToken, "expired")
		return "", fmt.Errorf("session expired")
	}

	ss := normalizeSessionSecurity(cfg)
	if securityEnabled(cfg) {
		if cfg.PublicHost != "" && !isLocalDev(cfg) {
			reqHost := requestHost(r)
			if !isInternalRequestHost(reqHost) && !hostMatchesPublic(reqHost, cfg.PublicHost, ss.DomainCheck.AllowedHosts) {
				recordSecurityEvent(r, username, "host_mismatch", "req="+reqHost+" expected="+cfg.PublicHost)
				killSession(sessionToken, "host_mismatch:"+reqHost)
				return "", fmt.Errorf("host mismatch")
			}
		}
		if ss.DeviceCookie.Enabled {
			cookieName := ss.DeviceCookie.CookieName
			if cookieName == "" {
				cookieName = "tm_device"
			}
			devCookie, errD := r.Cookie(cookieName)
			if errD != nil || devCookie.Value == "" || deviceToken == "" || devCookie.Value != deviceToken {
				recordSecurityEvent(r, username, "device_mismatch", "")
				killSession(sessionToken, "device_mismatch")
				return "", fmt.Errorf("device validation failed")
			}
		}
		currentIP := realClientIP(r)
		newIP, allow, kill := checkSessionIP(sessionToken, currentIP, sessionIP, ss.IPProtection)
		if kill {
			recordSecurityEvent(r, username, "ip_sharing", "concurrent /16 subnets")
			killSession(sessionToken, "ip_sharing_detected")
			return "", fmt.Errorf("session sharing detected")
		}
		if allow && newIP != sessionIP && newIP != "" {
			_, _ = db.Exec(
				"UPDATE ahrefs_sessions SET client_ip = ? WHERE session_token = ? AND website_id = ?",
				newIP, sessionToken, currentWebsiteID,
			)
		}
	}
	return username, nil
}

// ── AUTH HANDSHAKE HANDLER (aMemberPro → Proxy) ──────────────────────────────

func authHandshakeHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	if rejectIfIPBlocked(w, r, cfg) {
		return
	}
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
		}
	}
	if payload.Username == "" || payload.ClientIP == "" || payload.Signature == "" {
		http.Error(w, "Bad Request: Missing required fields", http.StatusBadRequest)
		return
	}

	// Fetch website config from DB
	var dbSecretKey string
	var dbSessionDuration int
	err := db.QueryRow("SELECT secret_key, session_duration FROM ahrefs_websites WHERE id = ?", currentWebsiteID).Scan(&dbSecretKey, &dbSessionDuration)
	if err != nil {
		dbSecretKey = cfg.SecretKey
		dbSessionDuration = cfg.SessionDurationMinutes
	}

	// Verify HMAC-SHA256 signature
	h := hmac.New(sha256.New, []byte(dbSecretKey))
	h.Write([]byte(fmt.Sprintf("%s:%d", payload.Username, payload.Timestamp)))
	expectedSig := hex.EncodeToString(h.Sum(nil))
	if !hmac.Equal([]byte(payload.Signature), []byte(expectedSig)) {
		log.Printf("[HANDSHAKE] ❌ HMAC failed for user: %s", payload.Username)
		http.Error(w, "Forbidden: Invalid signature", http.StatusForbidden)
		return
	}
	if time.Now().Unix()-payload.Timestamp > 300 {
		http.Error(w, "Forbidden: Request expired", http.StatusForbidden)
		return
	}
	if !dbConnected {
		http.Error(w, "Service Unavailable: Database not connected", http.StatusServiceUnavailable)
		return
	}

	// Product authorization check
	var productCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM ahrefs_products WHERE website_id = ?", currentWebsiteID).Scan(&productCount)
	if productCount > 0 && len(productIDs) > 0 {
		hasAccess := false
		rows, err := db.Query("SELECT product_id FROM ahrefs_products WHERE website_id = ?", currentWebsiteID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var allowedPid string
				if err := rows.Scan(&allowedPid); err == nil {
					for _, userPid := range productIDs {
						if strconv.Itoa(userPid) == allowedPid {
							hasAccess = true
							break
						}
					}
				}
				if hasAccess {
					break
				}
			}
		}
		if !hasAccess {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintf(w, `{"error":"no_product_access","message":"You do not have an authorized plan."}`)
			return
		}
	} else if productCount > 0 && len(productIDs) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `{"error":"no_product_access","message":"Your account does not have an active plan."}`)
		return
	}

	// Auto-expire custom limits
	_, _ = db.Exec(`UPDATE ahrefs_users SET credit_limit = COALESCE((SELECT default_credit_limit FROM ahrefs_websites WHERE id = ahrefs_users.website_id), 50),
		export_limit = COALESCE((SELECT default_export_limit FROM ahrefs_websites WHERE id = ahrefs_users.website_id), 100000),
		custom_limit_expire_at = NULL WHERE website_id = ? AND custom_limit_expire_at IS NOT NULL AND custom_limit_expire_at < NOW()`, currentWebsiteID)

	// Auto-create user
	var dbStatus string
	err = db.QueryRow("SELECT status FROM ahrefs_users WHERE username = ? AND website_id = ?", payload.Username, currentWebsiteID).Scan(&dbStatus)
	if err == sql.ErrNoRows {
		var defCredits, defExports int
		err = db.QueryRow("SELECT COALESCE(default_credit_limit,50), COALESCE(default_export_limit,100000) FROM ahrefs_websites WHERE id = ?", currentWebsiteID).Scan(&defCredits, &defExports)
		if err != nil {
			defCredits, defExports = 50, 100000
		}
		_, err = db.Exec("INSERT INTO ahrefs_users (username, website_id, credit_limit, export_limit) VALUES (?,?,?,?)", payload.Username, currentWebsiteID, defCredits, defExports)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		log.Printf("[HANDSHAKE] Auto-created user: %s under website_id %d ✅", payload.Username, currentWebsiteID)
	} else if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	} else if dbStatus == "suspended" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `{"error":"account_suspended","message":"Your account is suspended. Please contact support."}`)
		return
	}

	// Generate OTT
	ott, err := generateOTT()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	expires := time.Now().Add(60 * time.Second)
	_, err = db.Exec(
		"INSERT INTO ahrefs_tokens (token, username, client_ip, expires_at, website_id) VALUES (?,?,?,?,?)",
		ott, payload.Username, payload.ClientIP, expires, currentWebsiteID,
	)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	go func() { _, _ = db.Exec("DELETE FROM ahrefs_tokens WHERE expires_at < NOW()") }()

	redirectURL := fmt.Sprintf("%s://%s/access?token=%s",
		requestScheme(r, cfg), cfg.PublicHost,
		url.QueryEscape(payload.Username), url.QueryEscape(ott),
	)
	log.Printf("[HANDSHAKE] ✅ OTT generated for user: %s → %s", payload.Username, redirectURL)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","redirect_url":%q}`, redirectURL)
}

// ── ACCESS HANDLER (OTT → Session Cookie) ────────────────────────────────────

func accessHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	if rejectIfIPBlocked(w, r, cfg) {
		return
	}
	if usesPanelAccountMode(cfg) {
		servePanelAccess(w, r, cfg)
		return
	}
	if !dbConnected {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	username := strings.TrimSpace(r.URL.Query().Get("user"))
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		recordSecurityEvent(r, username, "access_denied", "missing user or token")
		renderAccessDeniedPage(w, cfg)
		return
	}
	var dbUsername, dbClientIP string
	var expiresAt time.Time
	err := db.QueryRow(
		"SELECT username, client_ip, expires_at FROM ahrefs_tokens WHERE token = ? AND website_id = ?",
		token, currentWebsiteID,
	).Scan(&dbUsername, &dbClientIP, &expiresAt)
	if err == sql.ErrNoRows || err != nil {
		recordSecurityEvent(r, username, "access_denied", "invalid or used token")
		renderAccessDeniedPage(w, cfg)
		return
	}
	if time.Now().After(expiresAt) {
		_, _ = db.Exec("DELETE FROM ahrefs_tokens WHERE token = ? AND website_id = ?", token, currentWebsiteID)
		recordSecurityEvent(r, username, "access_denied", "token expired")
		renderAccessDeniedPage(w, cfg)
		return
	}
	if username != "" && dbUsername != username {
		recordSecurityEvent(r, username, "access_denied", "username mismatch")
		renderAccessDeniedPage(w, cfg)
		return
	}
	username = dbUsername

	clientIP := realClientIP(r)
	ottOK, ottMode := proxysec.ValidateOTTClientIP(dbClientIP, clientIP, cfg.SessionSecurity, toolCtx(cfg), currentWebsiteSecurityEnabled)
	if !ottOK {
		log.Printf("[ACCESS] ❌ OTT IP mismatch | mode=%s user=%s token_ip=%s req_ip=%s", ottMode, username, dbClientIP, clientIP)
		recordSecurityEvent(r, username, "ott_ip_mismatch", "mode="+ottMode+" token_ip="+dbClientIP+" req_ip="+clientIP)
		_, _ = db.Exec("DELETE FROM ahrefs_tokens WHERE token = ? AND website_id = ?", token, currentWebsiteID)
		renderAccessDeniedPage(w, cfg)
		return
	}
	_, _ = db.Exec("DELETE FROM ahrefs_tokens WHERE token = ? AND website_id = ?", token, currentWebsiteID)

	ss := normalizeSessionSecurity(cfg)
	if securityEnabled(cfg) && ss.SingleSessionPerUser {
		_, _ = db.Exec("DELETE FROM ahrefs_sessions WHERE username = ? AND website_id = ?", username, currentWebsiteID)
		log.Printf("[ACCESS] Purged old sessions for user '%s' (single-session)", username)
	}

	// Generate session token
	sessionBytes := make([]byte, 32)
	if _, err := rand.Read(sessionBytes); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	sessionToken := hex.EncodeToString(sessionBytes)

	deviceToken := ""
	if securityEnabled(cfg) && ss.DeviceCookie.Enabled {
		devBytes := make([]byte, 32)
		if _, err := rand.Read(devBytes); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		deviceToken = hex.EncodeToString(devBytes)
	}

	var sessionDuration int
	err = db.QueryRow("SELECT COALESCE(session_duration, 120) FROM ahrefs_websites WHERE id = ?", currentWebsiteID).Scan(&sessionDuration)
	if err != nil {
		sessionDuration = cfg.SessionDurationMinutes
	}
	sessionExpiry := time.Now().Add(time.Duration(sessionDuration) * time.Minute)

	_, err = db.Exec(
		"INSERT INTO ahrefs_sessions (session_token, username, client_ip, expires_at, website_id, device_token) VALUES (?,?,?,?,?,?)",
		sessionToken, username, clientIP, sessionExpiry, currentWebsiteID, deviceToken,
	)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	// Auto-assign account
	if _, assignErr := autoAssignNextAccount(sessionToken); assignErr != nil {
		log.Printf("[ACCESS] ⚠️ No active accounts for website_id=%d: %v", currentWebsiteID, assignErr)
	}
	// Log login
	_, _ = db.Exec(
		"INSERT INTO ahrefs_login_logs (website_id, username, client_ip, user_agent, logged_in_at) VALUES (?,?,?,?,NOW())",
		currentWebsiteID, username, clientIP, r.Header.Get("User-Agent"),
	)
	secure := cookieSecure(r, cfg)
	sameSite := http.SameSiteLaxMode
	http.SetCookie(w, &http.Cookie{
		Name:     "ct_session",
		Value:    sessionToken,
		Path:     "/",
		Expires:  sessionExpiry,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	})
	if deviceToken != "" {
		cookieName := ss.DeviceCookie.CookieName
		if cookieName == "" {
			cookieName = "tm_device"
		}
		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    deviceToken,
			Path:     "/",
			Expires:  sessionExpiry,
			HttpOnly: true,
			Secure:   secure,
			SameSite: sameSite,
		})
	}
	log.Printf("[ACCESS] ✅ Session set for user: %s (secure=%v device=%v)", username, secure, deviceToken != "")
	homePath := cfg.HomePath
	if homePath == "" {
		homePath = "/"
	}
	http.Redirect(w, r, homePath, http.StatusFound)
}

// ── USER LIMITS API ───────────────────────────────────────────────────────────

func userLimitsAPIHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	if origin := corsOrigin(cfg); origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Cookie")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	currentUser, err := getAuthenticatedUser(r, cfg)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "unauthorized", "message": err.Error()})
		return
	}
	if cfg.DisableLimits || !dbConnected {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"show_limit": false, "username": currentUser, "tool_name": cfg.ToolName,
		})
		return
	}
	var creditLimit, exportLimit int
	var showLimitVal int
	var assignedAccountID sql.NullInt64
	err = db.QueryRow("SELECT credit_limit, export_limit FROM ahrefs_users WHERE username = ? AND website_id = ?", currentUser, currentWebsiteID).Scan(&creditLimit, &exportLimit)
	if err == sql.ErrNoRows {
		creditLimit, exportLimit = 50, 100000
	}
	var creditUsed int
	_ = db.QueryRow("SELECT COUNT(*) FROM ahrefs_credit_logs WHERE username = ? AND website_id = ? AND DATE(timestamp) = CURDATE()", currentUser, currentWebsiteID).Scan(&creditUsed)
	var exportUsed int
	_ = db.QueryRow("SELECT COALESCE(SUM(rows_count),0) FROM ahrefs_export_logs WHERE username = ? AND website_id = ?", currentUser, currentWebsiteID).Scan(&exportUsed)
	showLimit := true
	if cookie, errC := r.Cookie("ct_session"); errC == nil {
		_ = db.QueryRow("SELECT assigned_account_id FROM ahrefs_sessions WHERE session_token = ? AND website_id = ?", cookie.Value, currentWebsiteID).Scan(&assignedAccountID)
	}
	if assignedAccountID.Valid {
		_ = db.QueryRow("SELECT show_limit FROM ahrefs_accounts WHERE id = ? AND website_id = ?", assignedAccountID.Int64, currentWebsiteID).Scan(&showLimitVal)
		showLimit = showLimitVal == 1
	}

	creditLabel, exportLabel := cfg.CreditLabel, cfg.ExportLabel
	if creditLabel == "" {
		creditLabel = "Credits"
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"show_limit":     showLimit,
		"username":       currentUser,
		"credit_limit":   creditLimit,
		"credit_used":    creditUsed,
		"export_limit":   exportLimit,
		"export_used":    exportUsed,
		"credit_label":   creditLabel,
		"export_label":   exportLabel,
		"tool_name":      cfg.ToolName,
		"disable_export": cfg.DisableExportTracking,
	})
}

// ── ROTATE SESSION HANDLER ────────────────────────────────────────────────────

func rotateSessionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	cookie, err := r.Cookie("ct_session")
	if err != nil || cookie.Value == "" {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"unauthorized"}`)
		return
	}
	sessionToken := cookie.Value
	activeAcc, found := getSessionAssignedAccount(sessionToken)
	if !found {
		var errSelect error
		activeAcc, errSelect = selectActiveAccount()
		if errSelect != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"error":"no_active_accounts"}`)
			return
		}
	}
	reason := r.URL.Query().Get("reason")
	if reason == "" {
		reason = "client-side watchdog trigger"
	}
	var currentUser string
	err = db.QueryRow("SELECT username FROM ahrefs_sessions WHERE session_token = ? AND website_id = ?", sessionToken, currentWebsiteID).Scan(&currentUser)
	if err != nil {
		currentUser = "unknown"
	}
	nextAcc, switchErr := switchToNextAccount(sessionToken, activeAcc.ID, activeAcc.Name, currentUser, reason)
	if switchErr != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"error":"no_other_accounts","message":"%v"}`, switchErr)
		return
	}
	fmt.Fprintf(w, `{"status":"ok","switched_to":"%s","id":%d}`, nextAcc.Name, nextAcc.ID)
}

// ── ERROR PAGE RENDERERS ──────────────────────────────────────────────────────

func renderAccessDeniedPage(w http.ResponseWriter, cfg Config) {
	name := html.EscapeString(toolDisplayName(cfg))
	writeLightCard(w, http.StatusForbidden, lightCard{
		Title:   "Access Denied",
		Heading: "Access Denied",
		Message: "You cannot open <span class=\"brand\">" + name + "</span> directly. Open it from your access link.",
		Footer:  "A direct visit is not allowed",
	})
}

func renderNoActiveAccountsPage(w http.ResponseWriter, cfg Config) {
	name := html.EscapeString(toolDisplayName(cfg))
	writeLightCard(w, http.StatusServiceUnavailable, lightCard{
		Title:   "Temporarily Unavailable",
		Heading: "Temporarily Unavailable",
		Message: "All mapped <span class=\"brand\">" + name + "</span> accounts are currently undergoing maintenance. Please try again in a few minutes.",
		Footer:  "This keeps trying until an account is available",
	})
}

func renderLimitReachedPage(w http.ResponseWriter, limitType string, cfg Config) {
	name := html.EscapeString(toolDisplayName(cfg))
	creditLabel := html.EscapeString(cfg.CreditLabel)
	if creditLabel == "" {
		creditLabel = "Credits"
	}
	title := "Daily Limit Reached"
	message := "You have used the daily " + creditLabel + " limit for <span class=\"brand\">" + name + "</span>. The limit resets at midnight (12:00 AM IST)."
	if limitType == "export" {
		title = "Export Limit Reached"
		message = "You have reached the export limit for <span class=\"brand\">" + name + "</span>."
	}
	writeLightCard(w, http.StatusForbidden, lightCard{
		Title:   title,
		Heading: title,
		Message: message,
		Footer:  "This limit applies to the current access",
	})
}

// ── PROXY SYSTEM (Hot-reloadable proxy.txt) ───────────────────────────────────

const PROXY_FILE = "proxy.txt"

var (
	currentProxy *url.URL
	proxyModTime time.Time
)

func loadProxy() *url.URL {
	info, err := os.Stat(PROXY_FILE)
	if err != nil {
		currentProxy = nil
		return nil
	}
	if !info.ModTime().After(proxyModTime) {
		return currentProxy
	}
	data, err := os.ReadFile(PROXY_FILE)
	if err != nil {
		return currentProxy
	}
	var proxyStr string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		proxyStr = line
		break
	}
	proxyModTime = info.ModTime()
	if proxyStr == "" {
		currentProxy = nil
		return nil
	}
	parsed, err := url.Parse(proxyStr)
	if err != nil {
		return currentProxy
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "socks5" && scheme != "socks5h" && scheme != "http" && scheme != "https" {
		return currentProxy
	}
	currentProxy = parsed
	return currentProxy
}

func getProxy() *url.URL { return loadProxy() }

// ── PROXY DIALERS ─────────────────────────────────────────────────────────────

func dialThroughProxy(ctx context.Context, targetAddr string, proxyURL *url.URL) (net.Conn, error) {
	switch strings.ToLower(proxyURL.Scheme) {
	case "socks5", "socks5h":
		return dialThroughSocks5(ctx, targetAddr, proxyURL)
	case "http", "https":
		return dialThroughHTTPProxy(ctx, targetAddr, proxyURL)
	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s", proxyURL.Scheme)
	}
}

func dialThroughSocks5(ctx context.Context, targetAddr string, proxyURL *url.URL) (net.Conn, error) {
	var auth *proxy.Auth
	if proxyURL.User != nil {
		pass, _ := proxyURL.User.Password()
		auth = &proxy.Auth{User: proxyURL.User.Username(), Password: pass}
	}
	proxyHost := proxyURL.Host
	if !strings.Contains(proxyHost, ":") {
		proxyHost += ":1080"
	}
	baseDialer := &net.Dialer{Timeout: 20 * time.Second}
	socks5Dialer, err := proxy.SOCKS5("tcp", proxyHost, auth, baseDialer)
	if err != nil {
		return nil, fmt.Errorf("socks5 dialer init: %w", err)
	}
	if cd, ok := socks5Dialer.(proxy.ContextDialer); ok {
		return cd.DialContext(ctx, "tcp", targetAddr)
	}
	return socks5Dialer.Dial("tcp", targetAddr)
}

func dialThroughHTTPProxy(ctx context.Context, targetAddr string, proxyURL *url.URL) (net.Conn, error) {
	proxyHost := proxyURL.Host
	if !strings.Contains(proxyHost, ":") {
		if proxyURL.Scheme == "https" {
			proxyHost += ":443"
		} else {
			proxyHost += ":80"
		}
	}
	conn, err := (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, "tcp", proxyHost)
	if err != nil {
		return nil, fmt.Errorf("connect to HTTP proxy: %w", err)
	}
	connectLine := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", targetAddr, targetAddr)
	if proxyURL.User != nil {
		pass, _ := proxyURL.User.Password()
		creds := base64.StdEncoding.EncodeToString([]byte(proxyURL.User.Username() + ":" + pass))
		connectLine += "Proxy-Authorization: Basic " + creds + "\r\n"
	}
	connectLine += "\r\n"
	if _, err := conn.Write([]byte(connectLine)); err != nil {
		conn.Close()
		return nil, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		conn.Close()
		return nil, err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("HTTP proxy CONNECT refused: %s", resp.Status)
	}
	return conn, nil
}

// ── CHROME TLS FINGERPRINT TRANSPORT ─────────────────────────────────────────

type uTLSConn struct{ *utls.UConn }

func (c *uTLSConn) ConnectionState() tls.ConnectionState {
	cs := c.UConn.ConnectionState()
	return tls.ConnectionState{
		Version: cs.Version, HandshakeComplete: cs.HandshakeComplete,
		DidResume: cs.DidResume, CipherSuite: cs.CipherSuite,
		NegotiatedProtocol: cs.NegotiatedProtocol, ServerName: cs.ServerName,
	}
}

type contextKey string

const proxyContextKey contextKey = "account_proxy"

func dialChrome(ctx context.Context, addr string) (*uTLSConn, error) {
	return dialChromeALPN(ctx, addr, nil)
}

// dialChromeH1 forces ALPN http/1.1 so net/http Transport never speaks h2
// (nginx Connection: upgrade breaks Go's HTTP/2 client).
func dialChromeH1(ctx context.Context, addr string) (*uTLSConn, error) {
	return dialChromeALPN(ctx, addr, []string{"http/1.1"})
}

func dialChromeALPN(ctx context.Context, addr string, nextProtos []string) (*uTLSConn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	dialAddr := addr
	if hostsMapsHostToLocalhost(host) {
		if realIP := resolveUpstreamIP(ctx, host); realIP != "" && realIP != host {
			dialAddr = net.JoinHostPort(realIP, port)
		}
	}
	var tcpConn net.Conn
	var px *url.URL
	if ctxPx, ok := ctx.Value(proxyContextKey).(string); ok && ctxPx != "" {
		if parsed, err := url.Parse(ctxPx); err == nil {
			px = parsed
		}
	}
	if px == nil {
		px = getProxy()
	}
	if px != nil {
		tcpConn, err = dialThroughProxy(ctx, dialAddr, px)
		if err != nil {
			return nil, fmt.Errorf("proxy dial %s: %w", px.Host, err)
		}
	} else {
		tcpConn, err = dialBypassLocalHosts(ctx, "tcp", dialAddr)
		if err != nil {
			return nil, fmt.Errorf("TCP dial: %w", err)
		}
	}
	tlsCfg := &utls.Config{ServerName: host, InsecureSkipVerify: false}
	var uConn *utls.UConn
	if len(nextProtos) > 0 {
		spec, specErr := utls.UTLSIdToSpec(utls.HelloChrome_120)
		if specErr != nil {
			tcpConn.Close()
			return nil, fmt.Errorf("uTLS spec: %w", specErr)
		}
		for _, ext := range spec.Extensions {
			if alpn, ok := ext.(*utls.ALPNExtension); ok {
				alpn.AlpnProtocols = append([]string(nil), nextProtos...)
			}
		}
		tlsCfg.NextProtos = append([]string(nil), nextProtos...)
		uConn = utls.UClient(tcpConn, tlsCfg, utls.HelloCustom)
		if err := uConn.ApplyPreset(&spec); err != nil {
			tcpConn.Close()
			return nil, fmt.Errorf("uTLS preset: %w", err)
		}
	} else {
		uConn = utls.UClient(tcpConn, tlsCfg, utls.HelloChrome_120)
	}
	if err := uConn.HandshakeContext(ctx); err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("uTLS handshake: %w", err)
	}
	return &uTLSConn{uConn}, nil
}

type roundTripper struct {
	h2 *http2.Transport
	h1 *http.Transport
}

func (rt *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// HTTP/2 forbids Connection/Upgrade; nginx often attaches Connection: upgrade
	// → "http2: invalid Connection request header". Strip + prefer HTTP/1.1 for
	// members.erank.com (Laravel) — h2 is unnecessary and brittle behind nginx.
	req.Header.Del("Connection")
	req.Header.Del("Upgrade")
	req.Header.Del("Proxy-Connection")
	req.Header.Del("Keep-Alive")
	req.Header.Del("TE")
	req.Header.Del("Trailer")
	req.Header.Del("Transfer-Encoding")
	req.Close = false
	if px, ok := req.Context().Value(proxyContextKey).(string); ok && strings.TrimSpace(px) != "" {
		if rt.h1 != nil {
			rt.h1.CloseIdleConnections()
		}
		if rt.h2 != nil {
			rt.h2.CloseIdleConnections()
		}
	}
	host := ""
	if req.URL != nil {
		host = strings.ToLower(req.URL.Hostname())
	}
	// Force HTTP/1.1 for eRank origins — avoids h2 Connection header rejection.
	if strings.HasSuffix(host, "erank.com") && rt.h1 != nil {
		return rt.h1.RoundTrip(req)
	}
	addr := req.URL.Host
	if !strings.Contains(addr, ":") {
		if req.URL.Scheme == "https" {
			addr += ":443"
		} else {
			addr += ":80"
		}
	}
	conn, err := dialChrome(req.Context(), addr)
	if err != nil {
		return nil, err
	}
	proto := conn.ConnectionState().NegotiatedProtocol
	if proto == "h2" && rt.h2 != nil {
		return rt.h2.RoundTripOpt(req, http2.RoundTripOpt{})
	}
	return rt.h1.RoundTrip(req)
}

func buildChromeHTTPClient() *http.Client {
	// h1 dialer: ALPN http/1.1 only — used for *.erank.com to avoid h2 Connection errors.
	dialH1 := func(ctx context.Context, network, addr string) (net.Conn, error) { return dialChromeH1(ctx, addr) }
	h1 := &http.Transport{
		DialTLSContext: dialH1, MaxIdleConns: 100, MaxIdleConnsPerHost: 10,
		IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 20 * time.Second,
		DisableCompression: false, ForceAttemptHTTP2: false,
	}
	h2 := &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			return dialChrome(ctx, addr)
		}, DisableCompression: false,
	}
	return &http.Client{
		Transport:     &roundTripper{h2: h2, h1: h1},
		Timeout:       120 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
	}
}

var httpClient = buildChromeHTTPClient()

var dnsBypassCache sync.Map

func isNonRoutableIP(ip string) bool {
	p := net.ParseIP(ip)
	if p == nil {
		return true
	}
	return p.IsLoopback() || p.IsPrivate() || p.IsLinkLocalUnicast()
}

func hostsMapsHostToLocalhost(host string) bool {
	data, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return false
	}
	re := regexp.MustCompile(`(?m)^127\.0\.0\.1\s+` + regexp.QuoteMeta(host) + `(\s|$)`)
	return re.Match(data)
}

func fallbackUpstreamIP(host string) string {
	// CloudFront/Cloudflare — used when /etc/hosts poisons DNS to 127.0.0.1
	fallbacks := map[string]string{
		"members.erank.com": "54.192.142.69",
		"assets.erank.com":  "54.192.142.64",
		"public.erank.com":  "54.192.142.36",
		"hc.erank.com":      "54.192.142.92",
	}
	return fallbacks[host]
}

func resolveUpstreamIP(ctx context.Context, host string) string {
	if ip, err := lookupPublicDNS(ctx, host); err == nil && ip != "" && !isNonRoutableIP(ip) {
		return ip
	}
	if fb := fallbackUpstreamIP(host); fb != "" {
		log.Printf("[PROXY:DNS] static fallback %s → %s (/etc/hosts may be overriding DNS)", host, fb)
		return fb
	}
	return ""
}

func lookupPublicDNS(ctx context.Context, host string) (string, error) {
	if ip := net.ParseIP(host); ip != nil {
		return host, nil
	}
	if v, ok := dnsBypassCache.Load(host); ok {
		return v.(string), nil
	}
	for _, dnsServer := range []string{"8.8.8.8:53", "1.1.1.1:53"} {
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 4 * time.Second}
				return d.DialContext(ctx, "udp", dnsServer)
			},
		}
		addrs, err := resolver.LookupIPAddr(ctx, host)
		if err != nil {
			continue
		}
		for _, ia := range addrs {
			if v4 := ia.IP.To4(); v4 != nil {
				ip := v4.String()
				if isNonRoutableIP(ip) {
					log.Printf("[PROXY:DNS] skip bad IP %s for %s from %s", ip, host, dnsServer)
					continue
				}
				dnsBypassCache.Store(host, ip)
				log.Printf("[PROXY:DNS] real IP for %s → %s (via %s)", host, ip, dnsServer)
				return ip, nil
			}
		}
	}
	return "", fmt.Errorf("no public IP for %s", host)
}

func dialBypassLocalHosts(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	// Bypass /etc/hosts when it hijacks upstream hostnames to localhost
	if hostsMapsHostToLocalhost(host) {
		if realIP := resolveUpstreamIP(ctx, host); realIP != "" && realIP != host {
			addr = net.JoinHostPort(realIP, port)
		}
	}
	d := net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	if network == "tcp" || network == "tcp6" {
		network = "tcp4"
	}
	return d.DialContext(ctx, network, addr)
}

func buildFastHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext:           dialBypassLocalHosts,
			MaxIdleConns:          256,
			MaxIdleConnsPerHost:   64,
			MaxConnsPerHost:       64,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			ForceAttemptHTTP2:     true,
		},
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

var fastHTTPClient = buildFastHTTPClient()

func pickHTTPClient(cfg Config, path string) *http.Client {
	if usesPanelAccountMode(cfg) {
		return httpClient
	}
	if cfg.LocalTestMode {
		// HTML shell needs uTLS Chrome fingerprint (Cloudflare challenge bypass).
		if path == "/" || path == "/index.html" {
			return httpClient
		}
		// Static assets, fonts, media: pooled fast client (no per-request TLS handshake).
		if strings.HasPrefix(path, "/static/") || strings.HasPrefix(path, "/cdn-proxy/") {
			return fastHTTPClient
		}
		if strings.HasPrefix(path, "/extra-cdn-") && !strings.HasPrefix(path, "/extra-cdn-0") {
			return fastHTTPClient
		}
	}
	return httpClient
}

// ── URL REWRITING HELPERS ─────────────────────────────────────────────────────

// buildDomainReplacements creates a list of old→new domain pairs for HTML rewriting.
func buildDomainReplacements(cfg Config) [][2]string {
	targetParsed, _ := url.Parse(cfg.TargetURL)
	cdnParsed, _ := url.Parse(cfg.CDNURL)
	publicBase := fmt.Sprintf("%s://%s", cfg.PublicScheme, cfg.PublicHost)
	var pairs [][2]string
	if targetParsed != nil {
		pairs = append(pairs, [2]string{"https://" + targetParsed.Host, publicBase})
		pairs = append(pairs, [2]string{"http://" + targetParsed.Host, publicBase})
	}
	if cdnParsed != nil && (targetParsed == nil || cdnParsed.Host != targetParsed.Host) {
		pairs = append(pairs, [2]string{"https://" + cdnParsed.Host, publicBase + "/cdn-proxy"})
		pairs = append(pairs, [2]string{"http://" + cdnParsed.Host, publicBase + "/cdn-proxy"})
	}
	for i, extra := range cfg.ExtraCDNDomains {
		extra = strings.TrimPrefix(strings.TrimPrefix(extra, "https://"), "http://")
		extra = strings.Split(extra, "/")[0]
		pairs = append(pairs, [2]string{"https://" + extra, fmt.Sprintf("%s/extra-cdn-%d", publicBase, i)})
		pairs = append(pairs, [2]string{"http://" + extra, fmt.Sprintf("%s/extra-cdn-%d", publicBase, i)})
	}
	// Common marketing / app aliases → stay on proxy host.
	for _, alias := range []string{"www.erank.com", "erank.com", "app.erank.com", "login.erank.com", "lime.erank.com"} {
		if targetParsed != nil && alias == targetParsed.Host {
			continue
		}
		pairs = append(pairs, [2]string{"https://" + alias, publicBase})
		pairs = append(pairs, [2]string{"http://" + alias, publicBase})
	}
	return pairs
}

func rewriteLocationHeader(loc string, pairs [][2]string) string {
	if loc == "" {
		return loc
	}
	for _, pair := range pairs {
		loc = strings.ReplaceAll(loc, pair[0], pair[1])
	}
	return loc
}

func stripLocalTestScripts(body []byte) []byte {
	domains := []string{
		"cdn.optimizely.com", "cdn.segment.com", "googletagmanager.com",
		"googleoptimize.com", "navattic.com", "profitwell",
		"dna8twue3dlxq.cloudfront.net", "js.stripe.com", "mxpnl.com",
		"mixpanel", "gtm.js", "analytics.min.js",
	}
	for _, d := range domains {
		re := regexp.MustCompile(`(?is)<script[^>]*src="[^"]*` + regexp.QuoteMeta(d) + `[^"]*"[^>]*>\s*</script>`)
		body = re.ReplaceAll(body, []byte(""))
		re2 := regexp.MustCompile(`(?is)<script[^>]*>[\s\S]*?` + regexp.QuoteMeta(d) + `[\s\S]*?</script>`)
		body = re2.ReplaceAll(body, []byte(""))
	}
	return body
}

func localTestStubScript() string {
	return `<script>window.dataLayer=window.dataLayer||[];window.analytics=window.analytics||{track:function(){},identify:function(){},page:function(){},group:function(){},load:function(){},ready:function(f){if(f)f();}};window.optimizely=window.optimizely||[];window.mixpanel=window.mixpanel||{track:function(){},identify:function(){},init:function(){}};</script>`
}

func antiClickjackFixScript() string {
	return `<script>(function(){try{var e=document.getElementById('antiClickjack');if(e)e.remove();if(self===top&&document.body)document.body.style.setProperty('display','block','important');}catch(x){}})();</script>`
}

func csrfBootstrapScript() string {
	return `<script>
(function(){
  try { fetch('/sanctum/csrf-cookie', {credentials:'include'}); } catch(e) {}
})();
</script>`
}

func assetsProxyPathScript(cfg Config) string {
	base := fmt.Sprintf("%s://%s/cdn-proxy/", cfg.PublicScheme, cfg.PublicHost)
	return fmt.Sprintf(`<script>try{__webpack_public_path__='%s';}catch(e){}</script>`, base)
}

func rewriteBody(body []byte, pairs [][2]string) []byte {
	for _, pair := range pairs {
		body = bytes.ReplaceAll(body, []byte(pair[0]), []byte(pair[1]))
	}
	return body
}

// rewriteStaticToOrigin points /static/ assets at the real CDN so the browser
// loads them directly (HTTP/2 multiplex + browser cache) instead of through Go.
func rewriteStaticToOrigin(body []byte, originHost string, publicBase string) []byte {
	origin := "https://" + originHost
	replacements := [][2]string{
		{`src="/static/`, `src="` + origin + `/static/`},
		{`href="/static/`, `href="` + origin + `/static/`},
		{`src='/static/`, `src='` + origin + `/static/`},
		{`href='/static/`, `href='` + origin + `/static/`},
		{`url(/static/`, `url(` + origin + `/static/`},
		{`url("/static/`, `url("` + origin + `/static/`},
		{`url('/static/`, `url('` + origin + `/static/`},
	}
	if publicBase != "" {
		replacements = append(replacements,
			[2]string{publicBase + `/static/`, origin + `/static/`},
		)
	}
	for _, pair := range replacements {
		body = bytes.ReplaceAll(body, []byte(pair[0]), []byte(pair[1]))
	}
	return body
}

func applyResponseCORS(w http.ResponseWriter, r *http.Request, cfg Config) {
	origin := r.Header.Get("Origin")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	} else {
		publicBase := fmt.Sprintf("%s://%s", cfg.PublicScheme, cfg.PublicHost)
		w.Header().Set("Access-Control-Allow-Origin", publicBase)
	}
	if isExtraCDNPath(r.URL.Path) {
		w.Header().Add("Access-Control-Expose-Headers", "x-dd-b, x-set-cookie, x-datadome, x-datadome-cid")
	}
}

func isCORSHeader(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), "access-control-")
}

func isExtraCDNPath(path string) bool {
	return strings.HasPrefix(path, "/extra-cdn-")
}

func forwardDatadomeHeaders(w http.ResponseWriter, upstreamResp *http.Response, path string, cfg Config) {
	if !isExtraCDNPath(path) {
		return
	}
	for _, name := range []string{"X-Set-Cookie", "X-Datadome", "X-Datadome-Cid", "X-Dd-B"} {
		if v := upstreamResp.Header.Get(name); v != "" {
			w.Header().Set(name, v)
		}
	}
	// Panel / production proxy: never write datadome onto the public host — values
	// are huge and trip nginx "Request Header Or Cookie Too Large".
	if usesPanelAccountMode(cfg) || !usesCookieFileMode(cfg) {
		return
	}
	secure := strings.EqualFold(cfg.PublicScheme, "https")
	for _, sc := range upstreamResp.Header.Values("Set-Cookie") {
		if !strings.Contains(strings.ToLower(sc), "datadome=") {
			continue
		}
		if c, err := http.ParseSetCookie(sc); err == nil && c.Name == "datadome" && c.Value != "" {
			maxAge := c.MaxAge
			if maxAge == 0 {
				maxAge = 31536000
			}
			http.SetCookie(w, &http.Cookie{
				Name:     "datadome",
				Value:    c.Value,
				Path:     "/",
				MaxAge:   maxAge,
				Secure:   secure,
				SameSite: http.SameSiteLaxMode,
			})
		}
	}
}

// ── SERVICE WORKER (disabled — was intercepting API → api.junglescout.com causing CORS 502) ─

func serviceWorkerJS() string {
	return `self.addEventListener('install',function(e){self.skipWaiting();});
self.addEventListener('activate',function(e){e.waitUntil(self.clients.claim());});`
}

func serviceWorkerRegisterScript(cfg Config) string {
	if !usesCookieFileMode(cfg) {
		return ""
	}
	return `<script>
if('serviceWorker' in navigator){
  navigator.serviceWorker.getRegistrations().then(function(regs){
    regs.forEach(function(r){r.unregister();});
  });
}
</script>`
}

func serviceWorkerHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[PROXY:SW] serving /sw.js | host=%q referer=%q", r.Host, r.Header.Get("Referer"))
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Service-Worker-Allowed", "/")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(serviceWorkerJS()))
}

// ── CLIENT-SIDE PATCHER SCRIPT ────────────────────────────────────────────────

func erankHideChromeStyle(username string) string {
	nameJSON, _ := json.Marshal(strings.TrimSpace(username))
	return `<style id="tm-erank-hide">
a[href="/extension"],
a[href="/settings"],
a[href^="/settings/"],
button[aria-label="Upgrade Plan"],
div:has(> button[aria-label="Upgrade Plan"]),
#user-profile-panel,
div.panel:has(#user-profile-panel),
a.setting-link {
  display: none !important;
}
</style>
<script>
(function(){
  var name = ` + string(nameJSON) + `;
  if (!name) return;
  function apply() {
    var nodes = document.querySelectorAll('span.notranslate[translate="no"]');
    for (var i = 0; i < nodes.length; i++) {
      var el = nodes[i];
      if (!el.classList.contains('truncate')) continue;
      if (el.textContent !== name) el.textContent = name;
    }
  }
  function boot() {
    apply();
    new MutationObserver(apply).observe(document.documentElement, {childList:true, subtree:true, characterData:true});
  }
  if (document.body) boot();
  else document.addEventListener('DOMContentLoaded', boot);
})();
</script>`
}

func patcherScript(cfg Config) string {
	targetParsed, _ := url.Parse(cfg.TargetURL)
	cdnParsed, _ := url.Parse(cfg.CDNURL)
	targetHost := ""
	if targetParsed != nil {
		targetHost = targetParsed.Host
	}
	cdnHost := ""
	if cdnParsed != nil {
		cdnHost = cdnParsed.Host
	}

	// Build JS blocked list from config
	var blockedListJS strings.Builder
	for _, p := range cfg.BlockedPaths {
		blockedListJS.WriteString(fmt.Sprintf("%q,", p))
	}
	for _, p := range cfg.BlockedPrefixes {
		blockedListJS.WriteString(fmt.Sprintf("%q,", p))
	}
	homePath := cfg.HomePath
	if homePath == "" {
		homePath = "/"
	}

	// Build watchdog triggers
	var triggerChecks strings.Builder
	for _, trigger := range cfg.WatchdogTriggers {
		triggerChecks.WriteString(fmt.Sprintf("if(txt.includes(%q)){isTrigger=true;reason=%q;}\n", trigger, trigger[:min(20, len(trigger))]))
	}

	var extraCDNsJS strings.Builder
	for i, extra := range cfg.ExtraCDNDomains {
		extraClean := strings.TrimPrefix(strings.TrimPrefix(extra, "https://"), "http://")
		extraClean = strings.Split(extraClean, "/")[0]
		extraCDNsJS.WriteString(fmt.Sprintf("{host:%q,path:%q},", extraClean, fmt.Sprintf("/extra-cdn-%d", i)))
	}

	proxyHostname := cfg.PublicHost
	if host, _, err := net.SplitHostPort(cfg.PublicHost); err == nil {
		proxyHostname = host
	}

	localHostFix := ""
	if cfg.LocalTestMode {
		fakeHostname := "members.erank.com"
		if targetParsed != nil && targetParsed.Hostname() != "" {
			fakeHostname = targetParsed.Hostname()
		}
		// Only spoof hostname (auth checks). Keep real host/port/origin so fetches stay on 127.0.0.1:7853.
		localHostFix = fmt.Sprintf(`
    try {
        var FAKE_NAME = %q;
        var PROXY_NAME = %q;
        function isProxyHost(h) {
            return h === '127.0.0.1' || h === 'localhost' || h === PROXY_NAME;
        }
        var _hostDesc = Object.getOwnPropertyDescriptor(Location.prototype, 'hostname');
        if (_hostDesc && _hostDesc.get) {
            var _realHostGet = _hostDesc.get;
            Object.defineProperty(Location.prototype, 'hostname', {
                get: function() {
                    var h = _realHostGet.call(this);
                    if (isProxyHost(h)) return FAKE_NAME;
                    return h;
                },
                configurable: true
            });
        }
    } catch(e) {}
`, fakeHostname, proxyHostname)
	}

	return fmt.Sprintf(`<script>
(function() {
    var T = '%s', C = '%s', O = window.location.origin;
    var HOME = '%s';
    var BLOCKED = [%s];
    var EXTRAS = [%s];

    var STATIC_ORIGIN = 'https://' + T;
    var CDN_ORIGIN = C ? ('https://' + C) : null;

    function directStatic(u) {
        if (typeof u !== 'string') return null;
        if (u.charAt(0) === '/' && u.indexOf('/static/') === 0) return STATIC_ORIGIN + u;
        if (u.indexOf(O + '/static/') === 0) return STATIC_ORIGIN + u.slice(O.length);
        if (u.indexOf(STATIC_ORIGIN + '/static/') === 0) return u;
        return null;
    }
    function patchUrl(u) {
        if (typeof u !== 'string') return u;
        var direct = directStatic(u);
        if (direct) return direct;
        // Official HTTPS (no port) → stay on local proxy
        if (u.indexOf('https://'+T) === 0) u = O + u.slice(('https://'+T).length);
        else if (u.indexOf('http://'+T) === 0) u = O + u.slice(('http://'+T).length);
        u = u.replace('https://erank.com', O).replace('http://erank.com', O);
        u = u.replace('https://www.erank.com', O).replace('http://www.erank.com', O);
        u = u.replace('https://app.erank.com', O).replace('http://app.erank.com', O);
        u = u.replace('https://login.erank.com', O).replace('http://login.erank.com', O);
        if (C && C !== T) u = u.replace('https://'+C, O+'/cdn-proxy').replace('http://'+C, O+'/cdn-proxy');
        if (u.indexOf('//'+T) === 0) u = O + u.slice(('//'+T).length);
        if (C && C !== T && u.indexOf('//'+C) === 0) u = O + '/cdn-proxy' + u.slice(('//'+C).length);
        for (var i = 0; i < EXTRAS.length; i++) {
            var eh = EXTRAS[i].host;
            if (u.indexOf('//'+eh) === 0) u = O + EXTRAS[i].path + u.slice(('//'+eh).length);
        }
        for (var j = 0; j < EXTRAS.length; j++) {
            var re = new RegExp('(https?:)?\\/\\/' + EXTRAS[j].host.replace(/\\./g, '\\\\.'), 'g');
            u = u.replace(re, O + EXTRAS[j].path);
        }
        return u;
    }
    function fixNavUrl(u) { return patchUrl(u); }
    function urlPathBlocked(u) {
        if (!u || typeof u !== 'string' || BLOCKED.length === 0) return false;
        try {
            var p = new URL(u, O).pathname.split('?')[0].replace(/\/$/, '') || '/';
            for (var i = 0; i < BLOCKED.length; i++) {
                var b = (BLOCKED[i] || '').replace(/\/$/, '');
                if (!b) continue;
                if (p === b || p.indexOf(b + '/') === 0) return true;
            }
        } catch(e) {}
        return false;
    }
    function patchImgSrc(v) {
        if (typeof v !== 'string') return v;
        return patchUrl(v);
    }
    try {
        var _imgSrc = Object.getOwnPropertyDescriptor(HTMLImageElement.prototype, 'src');
        if (_imgSrc && _imgSrc.set) {
            Object.defineProperty(HTMLImageElement.prototype, 'src', {
                get: _imgSrc.get,
                set: function(v) { _imgSrc.set.call(this, patchImgSrc(v)); },
                configurable: true
            });
        }
    } catch(eImg) {}
%s

    // ── Watchdog: Auto-rotate account on session drop ──
    var watchdogDone = false;
    function checkWatchdog() {
        if (watchdogDone) return;
        var txt = document.body ? document.body.innerHTML || '' : '';
        var isTrigger = false, reason = '';
        %s
        if (isTrigger) {
            watchdogDone = true;
            var over = document.createElement('div');
            over.style.cssText = 'position:fixed;inset:0;background:#eef3f8;color:#0f172a;z-index:99999999;display:flex;align-items:center;justify-content:center;font-family:system-ui,sans-serif;padding:24px;';
            over.innerHTML = '<div style="width:min(440px,92vw);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px;text-align:center;">' +
                '<div style="width:78px;height:78px;margin:0 auto 22px;border-radius:50%%;display:grid;place-items:center;background:conic-gradient(#3b82f6 0 70deg,#e7eef8 70deg 360deg);animation:tm-spin .9s linear infinite;"><div style="width:64px;height:64px;border-radius:50%%;background:#fff;display:grid;place-items:center;font-size:26px;animation:tm-spin .9s linear infinite reverse;">&#128274;</div></div>' +
                '<h1 style="font-size:28px;line-height:1.2;font-weight:800;letter-spacing:-.03em;margin:0 0 12px;">Reconnecting session</h1>' +
                '<p style="color:#64748b;font-size:15px;line-height:1.55;margin:0;">Please wait a moment while we restore your access.</p>' +
                '<p style="margin:18px 0 0;color:#94a3b8;font-size:13px;">This page refreshes automatically</p>' +
                '<style>@keyframes tm-spin{to{transform:rotate(360deg)}}</style></div>';
            document.body.appendChild(over);
            fetch('/api/rotate-session?reason=' + encodeURIComponent(reason))
            .then(function() { setTimeout(function() { window.location.href = HOME; }, 1200); })
            .catch(function() { setTimeout(function() { window.location.href = HOME; }, 1200); });
        }
    }
    if (%s) { setTimeout(checkWatchdog, 1000); setInterval(checkWatchdog, 4000); }

    // ── Link / media rewriter ──
    function rewriteLinks() {
        document.querySelectorAll('a[href]').forEach(function(a) {
            var h = patchUrl(a.href);
            if (urlPathBlocked(h)) { a.style.display = 'none'; a.removeAttribute('href'); return; }
            if (a.href !== h) a.href = h;
        });
        document.querySelectorAll('img[src]').forEach(function(img) {
            var s = img.getAttribute('src');
            if (!s) return;
            var patched = patchUrl(s);
            if (patched !== s) img.setAttribute('src', patched);
        });
    }
    rewriteLinks();
    setInterval(rewriteLinks, 200);
    try {
        var mediaObs = new MutationObserver(function() { rewriteLinks(); });
        mediaObs.observe(document.documentElement, {childList: true, subtree: true, attributes: true, attributeFilter: ['src', 'href']});
    } catch(eObs) {}

    // Recover only when browser actually left the proxy (real URL bar on official site)
    var recoverHandler = { fn: function() {} };
    recoverHandler.fn();
    setInterval(function() { recoverHandler.fn(); }, 800);

    try {
        var OFFICIAL = 'https://' + T;
        function fakeHref(real) {
            if (typeof real !== 'string') return real;
            if (real.indexOf(O) === 0) return OFFICIAL + real.slice(O.length);
            if (real.indexOf('http://' + T) === 0) return OFFICIAL + real.slice(('http://' + T).length);
            return real;
        }
        var _open = window.open;
        window.open = function(url, target, features) {
            if (typeof url === 'string') url = patchUrl(url);
            return _open.call(window, url, target, features);
        };
        var _assign = window.location.assign.bind(window.location);
        window.location.assign = function(u) {
            u = fixNavUrl(u);
            if (urlPathBlocked(u)) return;
            return _assign(u);
        };
        var _locReplace = window.location.replace.bind(window.location);
        window.location.replace = function(u) {
            u = fixNavUrl(u);
            if (urlPathBlocked(u)) return;
            return _locReplace(u);
        };
        var _hrefDesc = Object.getOwnPropertyDescriptor(Location.prototype, 'href');
        if (_hrefDesc && _hrefDesc.get && _hrefDesc.set) {
            var _hrefGet = _hrefDesc.get, _hrefSet = _hrefDesc.set;
            Object.defineProperty(Location.prototype, 'href', {
                get: function() { return fakeHref(_hrefGet.call(this)); },
                set: function(v) { return _hrefSet.call(this, fixNavUrl(v)); },
                configurable: true
            });
            Location.prototype.toString = function() { return fakeHref(_hrefGet.call(this)); };
            recoverHandler.fn = function() {
                var real = _hrefGet.call(window.location);
                if (real.indexOf(O) === 0) return;
                if (real.indexOf(OFFICIAL) === 0) {
                    window.location.replace(O + real.slice(OFFICIAL.length));
                }
            };
            try {
                Object.defineProperty(document, 'URL', { get: function() { return fakeHref(_hrefGet.call(window.location)); }, configurable: true });
                Object.defineProperty(document, 'documentURI', { get: function() { return fakeHref(_hrefGet.call(window.location)); }, configurable: true });
            } catch(e2) {}
        }
    } catch(e) {}

    // ── XHR patch ──
    function readXsrf() {
        var m = document.cookie.match(/(?:^|;\\s*)XSRF-TOKEN=([^;]*)/);
        if (!m) return '';
        try { return decodeURIComponent(m[1].replace(/\\+/g, ' ')); } catch(e) { return m[1]; }
    }
    function applyCsrfHeaders(h) {
        if (!h || typeof h.set !== 'function') return h;
        var t = readXsrf();
        if (t) {
            if (!h.has('X-XSRF-TOKEN')) h.set('X-XSRF-TOKEN', t);
            if (!h.has('X-CSRF-TOKEN')) h.set('X-CSRF-TOKEN', t);
        }
        if (!h.has('X-Requested-With')) h.set('X-Requested-With', 'XMLHttpRequest');
        return h;
    }
    var xo = XMLHttpRequest.prototype.open;
    XMLHttpRequest.prototype.open = function(m, u) {
        if (typeof u === 'string') {
            u = patchUrl(u);
            if (urlPathBlocked(u)) { this._tmBlocked = true; arguments[1] = 'about:blank'; }
            else { arguments[1] = u; }
        }
        return xo.apply(this, arguments);
    };
    var xs = XMLHttpRequest.prototype.send;
    XMLHttpRequest.prototype.send = function(body) {
        if (this._tmBlocked) return;
        var t = readXsrf();
        if (t) {
            try { this.setRequestHeader('X-XSRF-TOKEN', t); } catch(e) {}
            try { this.setRequestHeader('X-CSRF-TOKEN', t); } catch(e) {}
        }
        try { this.setRequestHeader('X-Requested-With', 'XMLHttpRequest'); } catch(e2) {}
        return xs.call(this, body);
    };

    // ── Fetch patch ──
    var fo = window.fetch;
    window.fetch = function(inp, init) {
        init = init || {};
        if (!init.credentials) init.credentials = 'include';
        var hdrs = new Headers(init.headers || {});
        applyCsrfHeaders(hdrs);
        init.headers = hdrs;
        if (typeof inp === 'string') {
            if (urlPathBlocked(inp)) return Promise.reject(new Error('blocked_path'));
            inp = patchUrl(inp);
        } else if (inp && typeof inp === 'object' && inp.url) {
            if (urlPathBlocked(inp.url)) return Promise.reject(new Error('blocked_path'));
            inp = new Request(patchUrl(inp.url), inp);
        }
        return fo(inp, init);
    };

    if (BLOCKED.length > 0) {
        function isBlocked(path) {
            return urlPathBlocked(path.startsWith('/') ? O + path : path);
        }
        var _push = history.pushState.bind(history);
        history.pushState = function(state, title, url) {
            if (typeof url === 'string') {
                url = patchUrl(url);
                try { var p = new URL(url, O); if (isBlocked(p.pathname)) { window.location.href = HOME; return; } } catch(e) {}
            }
            return _push(state, title, url);
        };
        var _replace = history.replaceState.bind(history);
        history.replaceState = function(state, title, url) {
            if (typeof url === 'string') {
                url = patchUrl(url);
                try { var p = new URL(url, O); if (isBlocked(p.pathname)) { window.location.href = HOME; return; } } catch(e) {}
            }
            return _replace(state, title, url);
        };
    }
})();
</script>`,
		targetHost, cdnHost, homePath, blockedListJS.String(), extraCDNsJS.String(), localHostFix, triggerChecks.String(),
		func() string {
			if len(cfg.WatchdogTriggers) > 0 && !cfg.LocalTestMode {
				return "true"
			}
			return "false"
		}(),
	)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ── LIMIT OVERLAY SCRIPT ──────────────────────────────────────────────────────

func limitOverlayScript() string {
	return `<script>
(function() {
    function showLimitPopup(msg) {
        if (document.getElementById('tm-limit-popup')) return;
        var overlay = document.createElement('div');
        overlay.id = 'tm-limit-popup';
        overlay.style.cssText = 'position:fixed;inset:0;background:#eef3f8;z-index:99999999;display:flex;align-items:center;justify-content:center;padding:24px;font-family:system-ui,-apple-system,Segoe UI,sans-serif;';
        var modal = document.createElement('div');
        modal.style.cssText = 'width:min(440px,92vw);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px 36px;text-align:center;color:#0f172a;';
        modal.innerHTML = '<div style="width:78px;height:78px;margin:0 auto 22px;border-radius:50%;background:#fff;display:grid;place-items:center;font-size:28px;border:1px solid #e6ebf2;">&#128274;</div>' +
            '<h1 style="font-size:28px;line-height:1.2;font-weight:800;letter-spacing:-.03em;margin:0 0 12px;">Limit Reached</h1>' +
            '<p style="color:#64748b;font-size:15px;line-height:1.55;margin:0;">' + msg + '</p>' +
            '<p style="margin:18px 0 0;color:#94a3b8;font-size:13px;">This limit applies to the current access</p>';
        overlay.appendChild(modal);
        document.body.appendChild(overlay);
    }
    window.__toolsmandi_showLimitPopup = showLimitPopup;
})();
</script>`
}

// ── LIMIT COUNTER WIDGET SCRIPT ───────────────────────────────────────────────

func limitWidgetScript(cfg Config) string {
	creditLabel := cfg.CreditLabel
	if creditLabel == "" {
		creditLabel = "Credits"
	}
	return fmt.Sprintf(`<script>
(function() {
    var CREDIT_LABEL = '%s';
    var DISABLE_EXPORT = %v;
    var badge = document.createElement('div');
    badge.id = 'tm-limit-badge';
    badge.style.cssText = 'position:fixed;bottom:20px;right:20px;background:rgba(15,23,42,0.9);color:#e2e8f0;border:1px solid rgba(255,255,255,0.08);border-radius:14px;padding:10px 16px;font-family:sans-serif;font-size:13px;z-index:999999;backdrop-filter:blur(12px);box-shadow:0 8px 32px rgba(0,0,0,0.4);min-width:180px;';
    badge.innerHTML = '<div style="font-weight:700;font-size:11px;color:#64748b;letter-spacing:0.5px;margin-bottom:4px;">ToolsMandi</div><div id="tm-credit-text">Loading...</div>';
    function mount() {
        if (!document.body || document.getElementById('tm-limit-badge')) return;
        document.body.appendChild(badge);
        updateBadge();
        setInterval(updateBadge, 30000);
    }
    function updateBadge() {
        fetch('/api/user-limits').then(function(r){ return r.json(); }).then(function(d) {
            if (!d.show_limit) { badge.style.display='none'; return; }
            var remaining = Math.max(0, d.credit_limit - d.credit_used);
            var color = remaining > 10 ? '#4ade80' : (remaining > 3 ? '#fb923c' : '#f87171');
            badge.querySelector('#tm-credit-text').innerHTML = '<span style="color:'+color+';font-weight:700;font-size:16px;">'+remaining+'</span> <span style="color:#64748b;">/ '+d.credit_limit+' '+CREDIT_LABEL+' left</span>';
        }).catch(function(){});
    }
    if (document.body) mount();
    else document.addEventListener('DOMContentLoaded', mount);
})();
</script>`, creditLabel, cfg.DisableExportTracking)
}

// ── DECOMPRESSION HELPERS ─────────────────────────────────────────────────────

func decompressBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	var reader io.Reader = resp.Body
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		reader = gr
	case "br":
		reader = brotli.NewReader(resp.Body)
	}
	return io.ReadAll(reader)
}

// ── PROXY DIAGNOSTIC LOGGING ──────────────────────────────────────────────────

func debugEnabled(cfg Config) bool {
	return cfg.LocalTestMode || cfg.DebugLogging
}

type requestLogger struct {
	start      time.Time
	reqHost    string
	method     string
	path       string
	query      string
	origin     string
	clientIP   string
	upstream   string
	status     int
	upstreamMS int64
	category   string
	errMsg     string
	ddHeaders  string
	cookieInfo string
	bodyHint   string
}

type proxyDiagCounters struct {
	sync.Mutex
	total, api2xx, api403, apiOther            int
	staticOK, staticSlow, html, errors, slow3s int
}

var proxyDiag proxyDiagCounters

func targetHostname(cfg Config) string {
	if tp, err := url.Parse(cfg.TargetURL); err == nil && tp.Hostname() != "" {
		return tp.Hostname()
	}
	return "members.erank.com"
}

func hasHostsEntry(cfg Config) bool {
	data, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return false
	}
	host := regexp.QuoteMeta(targetHostname(cfg))
	matched, _ := regexp.Match(`(?m)^127\.0\.0\.1\s+`+host+`(\s|$)`, data)
	return matched
}

func classifyPath(path string) string {
	switch {
	case strings.HasPrefix(path, "/api"):
		return "API"
	case strings.HasPrefix(path, "/extra-cdn-0"):
		return "CDN"
	case strings.HasPrefix(path, "/extra-cdn-3"):
		return "DATADOME"
	case strings.HasPrefix(path, "/extra-cdn-"):
		return "CDN"
	case strings.HasPrefix(path, "/cdn-proxy/"):
		return "CDN"
	case path == "/sw.js":
		return "SW"
	case path == "/" || path == "/index.html":
		return "HTML"
	case strings.Contains(path, "/static/js/"):
		return "JS"
	case strings.Contains(path, "/static/css/"):
		return "CSS"
	case strings.HasPrefix(path, "/static/"):
		return "STATIC"
	default:
		return "OTHER"
	}
}

func summarizeCookies(cookieHeader string) string {
	if cookieHeader == "" {
		return "browser_cookies=NONE"
	}
	flags := make([]string, 0, 4)
	for _, name := range []string{"sid_er", "XSRF-TOKEN", "er_sess_x"} {
		if strings.Contains(cookieHeader, name+"=") {
			flags = append(flags, name+"=yes")
		} else {
			flags = append(flags, name+"=no")
		}
	}
	return "browser_" + strings.Join(flags, " ")
}

func truncateLog(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func datadomeHeaderSummary(h http.Header) string {
	parts := make([]string, 0, 4)
	for _, k := range []string{"X-Datadome", "X-Datadome-Cid", "X-Dd-B"} {
		if v := h.Get(k); v != "" {
			parts = append(parts, k+"="+truncateLog(v, 36))
		}
	}
	if len(parts) == 0 {
		return "dd_headers=none"
	}
	return strings.Join(parts, " ")
}

func bodyDiagnostic(status int, peek []byte) string {
	if len(peek) == 0 {
		return ""
	}
	s := string(peek)
	if status == 403 {
		if strings.Contains(s, "captcha-delivery") || strings.Contains(s, "geo.captcha") {
			return "DATADOME_CAPTCHA_CHALLENGE"
		}
		if strings.Contains(s, "Verifying") {
			return "DATADOME_VERIFY_PAGE"
		}
		return truncateLog(s, 100)
	}
	if status == 401 {
		return "UNAUTHORIZED"
	}
	if status >= 400 {
		return truncateLog(s, 80)
	}
	if strings.Contains(s, "captcha") {
		return "contains_captcha"
	}
	return ""
}

func peekAndRestoreBody(body io.ReadCloser, limit int) ([]byte, io.ReadCloser) {
	peek, err := io.ReadAll(io.LimitReader(body, int64(limit)))
	if err != nil {
		return nil, body
	}
	rest, _ := io.ReadAll(body)
	return peek, io.NopCloser(bytes.NewReader(append(peek, rest...)))
}

func (rl *requestLogger) finish(cfg Config) {
	if !debugEnabled(cfg) {
		return
	}
	totalMS := time.Since(rl.start).Milliseconds()
	cat := rl.category
	if cat == "" {
		cat = classifyPath(rl.path)
	}

	proxyDiag.Lock()
	proxyDiag.total++
	switch cat {
	case "API":
		switch {
		case rl.status >= 200 && rl.status < 300:
			proxyDiag.api2xx++
		case rl.status == 403:
			proxyDiag.api403++
		default:
			proxyDiag.apiOther++
		}
	case "JS", "CSS", "STATIC":
		if totalMS > 500 {
			proxyDiag.staticSlow++
		} else {
			proxyDiag.staticOK++
		}
	case "HTML":
		proxyDiag.html++
	}
	if rl.errMsg != "" {
		proxyDiag.errors++
	}
	if totalMS > 3000 {
		proxyDiag.slow3s++
	}
	proxyDiag.Unlock()

	if (cat == "JS" || cat == "STATIC") && rl.errMsg == "" && rl.status >= 200 && rl.status < 400 && totalMS < 500 {
		return
	}

	q := rl.query
	if q != "" {
		q = "?" + q
	}
	msg := fmt.Sprintf("[PROXY:%s] %s %s%s | host=%q status=%d total=%dms upstream=%dms ip=%s",
		cat, rl.method, rl.path, q, rl.reqHost, rl.status, totalMS, rl.upstreamMS, rl.clientIP)
	if rl.upstream != "" {
		msg += " | up=" + truncateLog(rl.upstream, 80)
	}
	if rl.cookieInfo != "" {
		msg += " | " + rl.cookieInfo
	}
	if rl.ddHeaders != "" && (cat == "API" || cat == "DATADOME" || rl.status == 403) {
		msg += " | " + rl.ddHeaders
	}
	if rl.bodyHint != "" {
		msg += " | hint=" + rl.bodyHint
	}
	if rl.errMsg != "" {
		msg += " | ERR=" + truncateLog(rl.errMsg, 120)
	}
	if rl.origin != "" {
		msg += " | origin=" + rl.origin
	}
	log.Println(msg)

	if cat == "API" && rl.status == 419 {
		log.Printf("[PROXY:WARN] CSRF 419 on %s %s — refresh page or re-export cookie.txt", rl.method, rl.path)
	}
	if cfg.LocalTestMode && !hasHostsEntry(cfg) && strings.HasPrefix(rl.path, "/extra-cdn-0") && !strings.Contains(cfg.PublicHost, "127.0.0.1") {
		log.Printf("[PROXY:WARN] extra-cdn API may need /etc/hosts — use http://%s", cfg.PublicHost)
	}
	if totalMS > 3000 {
		log.Printf("[PROXY:SLOW] %s %s took %dms (upstream %dms)", rl.method, rl.path, totalMS, rl.upstreamMS)
	}
}

func startStatsReporter(cfg Config) {
	if !debugEnabled(cfg) {
		return
	}
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			proxyDiag.Lock()
			s := proxyDiag
			proxyDiag.total = 0
			proxyDiag.api2xx = 0
			proxyDiag.api403 = 0
			proxyDiag.apiOther = 0
			proxyDiag.staticOK = 0
			proxyDiag.staticSlow = 0
			proxyDiag.html = 0
			proxyDiag.errors = 0
			proxyDiag.slow3s = 0
			proxyDiag.Unlock()
			if s.total == 0 {
				continue
			}
			log.Printf("[PROXY:STATS] 30s | total=%d api_2xx=%d api_403=%d api_err=%d static_fast=%d static_slow=%d errors=%d slow>3s=%d | hosts_ok=%v",
				s.total, s.api2xx, s.api403, s.apiOther, s.staticOK, s.staticSlow, s.errors, s.slow3s, hasHostsEntry(cfg))
			if s.api403 > 0 && s.api2xx == 0 {
				log.Printf("[PROXY:STATS] ⚠️  All API calls returned 403 — run sudo ./setup-local-hosts.sh + use http://%s", cfg.PublicHost)
			}
			if s.staticSlow > 20 {
				log.Printf("[PROXY:STATS] ⚠️  Many slow static files (%d) — network or upstream slow", s.staticSlow)
			}
		}
	}()
}

func logStartupDiagnostics(cfg Config) {
	if !debugEnabled(cfg) {
		return
	}
	log.Printf("[PROXY:STARTUP] Debug logging enabled (local_test=%v debug_logging=%v)", cfg.LocalTestMode, cfg.DebugLogging)
	log.Printf("[PROXY:STARTUP] public_host=%s://%s | cookie_file=%s", cfg.PublicScheme, cfg.PublicHost, cfg.CookieFile)
	if cfg.LocalTestMode {
		log.Printf("[PROXY:STARTUP] ✅ open http://%s (no /etc/hosts needed)", cfg.PublicHost)
	}
	if _, err := os.Stat(cfg.CookieFile); err != nil {
		log.Printf("[PROXY:STARTUP] ❌ cookie file missing: %s", cfg.CookieFile)
	} else {
		ck := loadCookiesFromFile(cfg.CookieFile)
		log.Printf("[PROXY:STARTUP] cookie.txt | %s", summarizeCookies(ck))
	}
	log.Printf("[PROXY:STARTUP] Log tags: [PROXY:API] [PROXY:WARN] [PROXY:SLOW] [PROXY:STATS] [PROXY:ERR]")
}

// ── MAIN PROXY HANDLER ────────────────────────────────────────────────────────

// isSSERequest returns true if the client expects a streaming SSE response.
func isSSEResponse(contentType string) bool {
	return strings.Contains(contentType, "text/event-stream")
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	w.Header().Set("X-Proxy-Build", proxyBuildTag)
	if rejectIfIPBlocked(w, r, cfg) {
		return
	}
	path := r.URL.Path

	rl := &requestLogger{
		start:      time.Now(),
		reqHost:    r.Host,
		method:     r.Method,
		path:       path,
		query:      r.URL.RawQuery,
		origin:     r.Header.Get("Origin"),
		clientIP:   realClientIP(r),
		category:   classifyPath(path),
		cookieInfo: summarizeCookies(r.Header.Get("Cookie")),
	}
	defer rl.finish(cfg)

	if r.Method == http.MethodOptions {
		applyResponseCORS(w, r, cfg)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS, HEAD")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Cookie, Authorization, X-Requested-With, Accept, Origin, Referer, sec-ch-ua, sec-ch-ua-mobile, sec-ch-ua-platform")
		w.Header().Set("Access-Control-Max-Age", "86400")
		rl.status = http.StatusOK
		w.WriteHeader(http.StatusOK)
		return
	}

	// ── 0b. Skip dev-only source maps (DevTools hammers these through the proxy)
	if cfg.LocalTestMode && strings.HasSuffix(strings.ToLower(path), ".map") {
		rl.status = http.StatusNotFound
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// ── 0. Skip proxy for admin API routes ──────────────────────────────────────
	if strings.HasPrefix(path, "/api/auth-handshake") ||
		strings.HasPrefix(path, "/api/device-bind") ||
		path == "/tm-device-sw.js" ||
		strings.HasPrefix(path, "/api/user-limits") ||
		strings.HasPrefix(path, "/api/rotate-session") ||
		strings.HasPrefix(path, "/api/trigger-automation") ||
		strings.HasPrefix(path, "/api/security-ping") ||
		strings.HasPrefix(path, "/access") ||
		path == "/user/logout" {
		return // These are handled by their own handlers
	}

	var currentUser string
	var activeAcc ToolAccount
	var sessionToken string

	if usesPanelAccountMode(cfg) {
		if sessionToken == "" {
			if c, cErr := r.Cookie("ct_session"); cErr == nil {
				sessionToken = c.Value
			}
		}
		name, sessionErr := panelSessionUsername(r)
		if sessionErr != nil {
			if strings.Contains(strings.ToLower(path), "favicon") {
				activeAcc = ToolAccount{}
			} else {
				renderAccessDeniedPage(w, cfg)
				return
			}
		} else if rejectPanelDevice(w, r, cfg) {
			return
		} else {
			currentUser = name
			var panelErr error
			activeAcc, panelErr = loadPanelSessionAccount(cfg, sessionToken)
			if panelErr != nil {
				log.Printf("[PANEL] mapped account unavailable: %v", panelErr)
				renderNoActiveAccountsPage(w, cfg)
				return
			}
		}
	} else if usesCookieFileMode(cfg) {
		currentUser = "local_dev"
		activeAcc = getLocalTestAccount(cfg)
		if activeAcc.Cookie == "" {
			if cfg.LocalTestMode {
				http.Error(w, "cookie.txt empty or missing — add eRank cookies first", http.StatusServiceUnavailable)
			} else {
				renderNoActiveAccountsPage(w, cfg)
			}
			return
		}
	} else {
		// ── 1. Authenticate user (require ct_session cookie) ─────────────────────────
		var err error
		currentUser, err = getAuthenticatedUser(r, cfg)
		if err != nil {
			acceptHeader := r.Header.Get("Accept")
			isNavigation := strings.Contains(acceptHeader, "text/html")
			if isNavigation {
				renderAccessDeniedPage(w, cfg)
			} else {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintf(w, `{"error":"unauthorized","message":"Please log in via the member area."}`)
			}
			return
		}
	}

	// ── 2. Stub paths (block upstream, return local JSON) ─────────────────────────
	if isStubPath(path, cfg) {
		log.Printf("[STUB] User '%s' stubbed path: %s", currentUser, path)
		w.Header().Set("Content-Type", "application/json")
		rl.status = http.StatusOK
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(stubPathResponse(path))
		return
	}

	// ── 3. Check blocked paths ────────────────────────────────────────────────────
	if isBlockedPath(path, cfg) {
		log.Printf("[BLOCK] User '%s' tried to access blocked path: %s", currentUser, path)
		if dbConnected && !usesCookieFileMode(cfg) {
			_, _ = db.Exec(
				"INSERT INTO ahrefs_violations_logs (website_id, username, client_ip, attempted_path) VALUES (?,?,?,?)",
				currentWebsiteID, currentUser, realClientIP(r), path,
			)
		}
		if cfg.HomePath != "" {
			http.Redirect(w, r, cfg.HomePath, http.StatusFound)
		} else {
			http.Redirect(w, r, "/", http.StatusFound)
		}
		return
	}

	if !usesCookieFileMode(cfg) && !usesPanelAccountMode(cfg) {
		// ── 3. Get active account for this session ────────────────────────────────────
		if c, err := r.Cookie("ct_session"); err == nil {
			sessionToken = c.Value
		}
		var found bool
		activeAcc, found = getSessionAssignedAccount(sessionToken)
		if !found {
			var assignErr error
			activeAcc, assignErr = autoAssignNextAccount(sessionToken)
			if assignErr != nil {
				renderNoActiveAccountsPage(w, cfg)
				return
			}
		}
	}

	// ── 4. Credit limit check ────────────────────────────────────────────────────
	if dbConnected && !usesCookieFileMode(cfg) && !usesPanelAccountMode(cfg) && !cfg.DisableLimits && isCountedPath(path, cfg) {
		var creditLimit, creditUsed int
		_ = db.QueryRow("SELECT credit_limit FROM ahrefs_users WHERE username = ? AND website_id = ?", currentUser, currentWebsiteID).Scan(&creditLimit)
		_ = db.QueryRow("SELECT COUNT(*) FROM ahrefs_credit_logs WHERE username = ? AND website_id = ? AND DATE(timestamp) = CURDATE()", currentUser, currentWebsiteID).Scan(&creditUsed)
		if creditLimit > 0 && creditUsed >= creditLimit {
			// Check dedup: only count unique paths per 5 seconds
			cacheKey := fmt.Sprintf("%s:%s", currentUser, path)
			if _, alreadyCounted := creditHitCache.Load(cacheKey); !alreadyCounted {
				renderLimitReachedPage(w, "credit", cfg)
				return
			}
		}
		// Log credit usage (with dedup)
		cacheKey := fmt.Sprintf("%s:%s", currentUser, path)
		if _, alreadyCounted := creditHitCache.LoadOrStore(cacheKey, true); !alreadyCounted {
			go func() {
				time.Sleep(5 * time.Second)
				creditHitCache.Delete(cacheKey)
			}()
			_, _ = db.Exec("INSERT INTO ahrefs_credit_logs (website_id, username, endpoint) VALUES (?,?,?)", currentWebsiteID, currentUser, path)
		}
	}

	// ── 5. Build upstream request ─────────────────────────────────────────────────
	targetParsed, err := url.Parse(cfg.TargetURL)
	if err != nil {
		http.Error(w, "Bad gateway config", http.StatusBadGateway)
		return
	}

	upstreamURL := *r.URL
	upstreamURL.Scheme = targetParsed.Scheme
	upstreamURL.Host = targetParsed.Host

	// Handle CDN proxy routes
	cdnParsed, _ := url.Parse(cfg.CDNURL)
	if strings.HasPrefix(path, "/cdn-proxy/") && cdnParsed != nil {
		upstreamURL.Scheme = cdnParsed.Scheme
		upstreamURL.Host = cdnParsed.Host
		upstreamURL.Path = "/" + strings.TrimPrefix(path, "/cdn-proxy/")
	}
	for i, extra := range cfg.ExtraCDNDomains {
		prefix := fmt.Sprintf("/extra-cdn-%d/", i)
		if strings.HasPrefix(path, prefix) {
			extraClean := strings.TrimPrefix(strings.TrimPrefix(extra, "https://"), "http://")
			upstreamURL.Scheme = "https"
			if strings.HasPrefix(extra, "http://") {
				upstreamURL.Scheme = "http"
			}
			upstreamURL.Host = strings.Split(extraClean, "/")[0]
			upstreamURL.Path = "/" + strings.TrimPrefix(path, prefix)
			break
		}
	}
	upstreamReq, err := http.NewRequestWithContext(
		context.WithValue(r.Context(), proxyContextKey, activeAcc.Proxy),
		r.Method, upstreamURL.String(), nil,
	)
	if err != nil {
		http.Error(w, "Failed to build upstream request", http.StatusInternalServerError)
		return
	}
	var reqBody []byte
	if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
		reqBody, _ = io.ReadAll(r.Body)
		r.Body.Close()
	}
	if len(reqBody) > 0 {
		upstreamReq.Body = io.NopCloser(bytes.NewReader(reqBody))
		upstreamReq.ContentLength = int64(len(reqBody))
		upstreamReq.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(reqBody)), nil
		}
	}

	// Copy headers (skip Host/Cookie — set below for upstream)
	for k, vv := range r.Header {
		if strings.EqualFold(k, "Host") || strings.EqualFold(k, "Cookie") {
			continue
		}
		// Hop-by-hop — never forward (HTTP/2 rejects Connection: upgrade from nginx)
		if strings.EqualFold(k, "Connection") || strings.EqualFold(k, "Upgrade") ||
			strings.EqualFold(k, "Keep-Alive") || strings.EqualFold(k, "Proxy-Connection") ||
			strings.EqualFold(k, "Transfer-Encoding") || strings.EqualFold(k, "TE") {
			continue
		}
		if cfg.LocalTestMode && (strings.EqualFold(k, "X-XSRF-TOKEN") || strings.EqualFold(k, "X-CSRF-TOKEN")) {
			continue
		}
		for _, v := range vv {
			upstreamReq.Header.Add(k, v)
		}
	}

	// Set account cookie and user-agent
	upstreamCookies := buildUpstreamCookies(r, cfg, activeAcc)
	if upstreamCookies != "" {
		upstreamReq.Header.Set("Cookie", upstreamCookies)
	}
	ua := activeAcc.UserAgent
	if ua == "" {
		ua = cfg.UserAgent
	}
	if cfg.LocalTestMode {
		if clientUA := r.Header.Get("User-Agent"); clientUA != "" {
			ua = clientUA
		}
	}
	if ua != "" {
		upstreamReq.Header.Set("User-Agent", ua)
	}
	enrichUpstreamAuthHeaders(upstreamReq, cfg, activeAcc)
	if cfg.LocalTestMode {
		applyUpstreamCSRF(upstreamReq)
		upstreamReq.Header.Set("X-Requested-With", "XMLHttpRequest")
	}

	// Set upstream host header
	upstreamReq.Host = upstreamURL.Host
	if upstreamReq.Host == "" {
		upstreamReq.Host = targetParsed.Host
	}

	// Remove hop-by-hop + proxy headers. nginx "Connection: upgrade" must never
	// reach HTTP/2 upstream (Go: invalid Connection request header).
	upstreamReq.Header.Del("Connection")
	upstreamReq.Header.Del("Upgrade")
	upstreamReq.Header.Del("Keep-Alive")
	upstreamReq.Header.Del("Proxy-Connection")
	upstreamReq.Header.Del("X-Device-Fp")
	upstreamReq.Header.Del("X-Device-Proof")
	upstreamReq.Header.Del("X-Forwarded-For")
	upstreamReq.Header.Del("X-Real-IP")
	upstreamReq.Header.Del("Origin")
	upstreamReq.Header.Set("Origin", cfg.TargetURL)
	referer := cfg.TargetURL + path
	if isExtraCDNPath(path) {
		referer = cfg.TargetURL + "/"
	}
	upstreamReq.Header.Set("Referer", referer)
	if cfg.LocalTestMode && isExtraCDNPath(path) {
		if clientIP := realClientIP(r); clientIP != "" {
			upstreamReq.Header.Set("X-Forwarded-For", clientIP)
		}
	}

	if usesPanelAccountMode(cfg) && currentUser != "" && erankLoginDocument(r, path) {
		serveErankAccountSwitch(w, r, cfg, sessionToken, currentUser, activeAcc)
		return
	}

	// ── 6. Perform upstream request ───────────────────────────────────────────────
	rl.upstream = upstreamURL.String()
	upstreamStart := time.Now()
	client := pickHTTPClient(cfg, path)
	upstreamResp, err := client.Do(upstreamReq)
	if err == nil && cfg.LocalTestMode && upstreamResp.StatusCode == 419 && strings.HasPrefix(path, "/api/") &&
		r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
		upstreamResp.Body.Close()
		invalidateLocalCookieCache()
		if cookies := getLocalUpstreamCookieHeader(cfg, true); cookies != "" {
			upstreamReq.Header.Set("Cookie", cookies)
		}
		applyUpstreamCSRF(upstreamReq)
		if len(reqBody) > 0 {
			upstreamReq.Body = io.NopCloser(bytes.NewReader(reqBody))
			upstreamReq.ContentLength = int64(len(reqBody))
		}
		log.Printf("[PROXY:CSRF] retrying %s %s after sanctum refresh", r.Method, path)
		upstreamResp, err = client.Do(upstreamReq)
	}
	rl.upstreamMS = time.Since(upstreamStart).Milliseconds()
	if err != nil {
		rl.errMsg = err.Error()
		rl.status = http.StatusBadGateway
		log.Printf("[PROXY:ERR] upstream fail | %s %s | up=%s | took=%dms | err=%v | %s",
			r.Method, path, upstreamURL.String(), rl.upstreamMS, err, summarizeCookies(r.Header.Get("Cookie")))
		if !cfg.LocalTestMode && db != nil {
			activeAcc, _ = switchToNextAccount(sessionToken, activeAcc.ID, activeAcc.Name, currentUser, "upstream_connection_error")
		}
		if strings.Contains(err.Error(), "proxy dial") {
			renderProxyProblem(w, r)
			return
		}
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer upstreamResp.Body.Close()

	rl.status = upstreamResp.StatusCode
	rl.ddHeaders = datadomeHeaderSummary(upstreamResp.Header)
	if debugEnabled(cfg) && (isExtraCDNPath(path) || upstreamResp.StatusCode >= 400) {
		peek, restored := peekAndRestoreBody(upstreamResp.Body, 600)
		upstreamResp.Body = restored
		rl.bodyHint = bodyDiagnostic(upstreamResp.StatusCode, peek)
	}

	rewritePairs := buildDomainReplacements(cfg)

	// ── 7. Handle response headers ───────────────────────────────────────────────
	contentType := upstreamResp.Header.Get("Content-Type")
	for k, vv := range upstreamResp.Header {
		kLower := strings.ToLower(k)
		if kLower == "set-cookie" {
			continue
		}
		if kLower == "content-encoding" {
			continue
		}
		if kLower == "content-length" {
			continue
		}
		if kLower == "transfer-encoding" {
			continue
		}
		if kLower == "location" {
			continue
		}
		if isCORSHeader(k) {
			continue
		}
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	applyResponseCORS(w, r, cfg)
	forwardDatadomeHeaders(w, upstreamResp, path, cfg)
	if cfg.LocalTestMode && strings.HasPrefix(path, "/api/") {
		forwardUpstreamSetCookies(w, r, upstreamResp, cfg)
	}
	if loc := upstreamResp.Header.Get("Location"); loc != "" {
		newLoc := rewriteLocationHeader(loc, rewritePairs)
		w.Header().Set("Location", newLoc)
		if newLoc != loc {
			log.Printf("[REDIRECT] %s %s -> %s", r.Method, loc, newLoc)
		}
	}

	// ── 8. SSE (Server-Sent Events) passthrough ───────────────────────────────────
	if isSSEResponse(contentType) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(upstreamResp.StatusCode)
		flusher, canFlush := w.(http.Flusher)
		buf := make([]byte, 4096)
		for {
			n, err := upstreamResp.Body.Read(buf)
			if n > 0 {
				if _, werr := w.Write(buf[:n]); werr != nil {
					break
				}
				if canFlush {
					flusher.Flush()
				}
			}
			if err != nil {
				break
			}
		}
		return
	}

	// ── 9. Process HTML responses (inject scripts, rewrite URLs) ──────────────────
	isHTML := strings.Contains(contentType, "text/html") &&
		!strings.HasPrefix(path, "/api/") &&
		!strings.HasPrefix(path, "/extra-cdn-0") &&
		!strings.HasPrefix(path, "/cdn-proxy/") &&
		!strings.HasPrefix(path, "/cdn-cgi/") &&
		!strings.HasSuffix(path, ".js") &&
		!strings.HasSuffix(path, ".css") &&
		!strings.HasSuffix(path, ".json") &&
		!strings.HasSuffix(path, ".woff2") &&
		!strings.HasSuffix(path, ".woff") &&
		!strings.HasSuffix(path, ".ico") &&
		!strings.HasSuffix(path, ".png")
	if isHTML {
		bodyBytes, err := decompressBody(upstreamResp)
		if err != nil {
			w.WriteHeader(upstreamResp.StatusCode)
			return
		}

		// Rewrite domain references
		bodyBytes = rewriteBody(bodyBytes, rewritePairs)
		bodyBytes = regexp.MustCompile(`(?i)<base[^>]+href=["']https?://[^"']*erank\.com[^"']*["'][^>]*>`).ReplaceAll(bodyBytes, []byte(""))

		// Remove CSP header (prevents our injected scripts)
		w.Header().Del("Content-Security-Policy")
		w.Header().Del("Content-Security-Policy-Report-Only")
		w.Header().Del("X-Frame-Options")
		if usesPanelAccountMode(cfg) {
			bodyBytes = injectDeviceHTML(bodyBytes)
			expireProxyHostJunkCookies(w, r, cfg)
		}
		if cfg.LocalTestMode {
			setLocalBrowserAuthCookies(w, r, cfg)
		} else if !usesPanelAccountMode(cfg) {
			setProxyBrowserAuthCookies(w, r, cfg, activeAcc)
		}

		// Inject patcher at start of <head> (before app scripts run)
		earlyInject := serviceWorkerRegisterScript(cfg) + antiClickjackFixScript() + patcherScript(cfg) + erankHideChromeStyle(currentUser)
		if cfg.LocalTestMode {
			earlyInject = serviceWorkerRegisterScript(cfg) + antiClickjackFixScript() + assetsProxyPathScript(cfg) + localTestStubScript() + patcherScript(cfg) + erankHideChromeStyle(currentUser)
		}
		earlyInject += buildDomainCheckJS(cfg) + buildSecurityHeartbeatJS(cfg) + `<script>` + buildLogoutAutomationJS(cfg) + `</script>` + erankLoginWatchScript()
		lateInject := ""
		if !usesCookieFileMode(cfg) && !cfg.DisableLimits {
			lateInject = limitWidgetScript(cfg) + limitOverlayScript()
		}
		bodyBytes = regexp.MustCompile(`(?i)<head[^>]*>`).ReplaceAllFunc(bodyBytes, func(m []byte) []byte {
			out := make([]byte, 0, len(m)+len(earlyInject))
			out = append(out, m...)
			out = append(out, []byte(earlyInject)...)
			return out
		})
		bodyBytes = regexp.MustCompile(`(?i)</head>`).ReplaceAll(bodyBytes, []byte(lateInject+"</head>"))

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(upstreamResp.StatusCode)
		w.Write(bodyBytes)
		if debugEnabled(cfg) {
			jsBundles := bytes.Count(bodyBytes, []byte("/static/js/"))
			log.Printf("[PROXY:HTML] served %s | injected_scripts=yes size=%d static_js_refs=%d", path, len(bodyBytes), jsBundles)
			if jsBundles < 5 && (path == "/" || path == "/index.html") {
				log.Printf("[PROXY:WARN] HTML missing React bundles — likely Cloudflare challenge. Run: sudo ./remove-local-hosts.sh then restart")
			}
		}
		return
	}

	// ── 10. For JSON/JS/CSS/binary: rewrite and stream ────────────────────────────
	isRewritable := strings.Contains(contentType, "javascript") || strings.Contains(contentType, "application/json") || strings.Contains(contentType, "text/css")
	if isRewritable {
		bodyBytes, err := decompressBody(upstreamResp)
		if err == nil {
			bodyBytes = rewriteBody(bodyBytes, rewritePairs)
			w.WriteHeader(upstreamResp.StatusCode)
			w.Write(bodyBytes)
			return
		}
	}

	// ── 11. Pass through everything else ─────────────────────────────────────────
	switch strings.ToLower(upstreamResp.Header.Get("Content-Encoding")) {
	case "gzip", "br":
		bodyBytes, err := decompressBody(upstreamResp)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(bodyBytes)))
		w.WriteHeader(upstreamResp.StatusCode)
		w.Write(bodyBytes)
		return
	}
	w.WriteHeader(upstreamResp.StatusCode)
	io.Copy(w, upstreamResp.Body)
}

// ── CORS MIDDLEWARE ───────────────────────────────────────────────────────────

func withCORS(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := loadConfig()
		if origin := corsOrigin(cfg); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Cookie, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		h(w, r)
	}
}

// ── MAIN ──────────────────────────────────────────────────────────────────────

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	cfg := loadConfig()
	log.Printf("🚀 Starting Generic Tool Proxy — Tool: %s | Target: %s | Port: %s", cfg.ToolName, cfg.TargetURL, cfg.Port)
	log.Printf("[AUTOMATION] logout_detection=%v automation=%v", cfg.LogoutDetection.Enabled, cfg.Automation.Enabled)
	if cfg.LocalTestMode {
		log.Printf("🧪 LOCAL TEST MODE — cookies from %s", cfg.CookieFile)
		if cfg.BindLocalhost {
			log.Printf("🔒 Binding to 127.0.0.1 only (not exposed on LAN)")
		}
		logStartupDiagnostics(cfg)
		startStatsReporter(cfg)
	}
	initDB(cfg)
	resolveWebsiteID(cfg.PublicHost)
	ss := normalizeSessionSecurity(cfg)
	initScoreEngine(cfg)
	log.Printf("[PROXY] build_tag=%s public_host=%s use_database=%v db_connected=%v", proxyBuildTag, cfg.PublicHost, cfg.UseDatabase, dbConnected)
	log.Printf("[SECURITY] build=%s enabled=%v db_toggle=%v single_session=%v device=%v domain_check=%v",
		proxysec.BuildID, securityEnabled(cfg), currentWebsiteSecurityEnabled, ss.SingleSessionPerUser,
		ss.DeviceCookie.Enabled, ss.DomainCheck.Enabled)
	startDailyResetCron()
	startBlockedIPRefreshLoop()

	mux := http.NewServeMux()

	mux.HandleFunc("/api/auth-handshake", withCORS(authHandshakeHandler))
	mux.HandleFunc("/api/user-limits", withCORS(userLimitsAPIHandler))
	mux.HandleFunc("/api/rotate-session", withCORS(rotateSessionHandler))
	mux.HandleFunc("/api/trigger-automation", withCORS(triggerAutomationHandler))
	mux.HandleFunc("/api/security-ping", withCORS(securityPingHandler))
	mux.HandleFunc("/sw.js", serviceWorkerHandler)
	mux.HandleFunc("/api/device-bind", deviceBindHandler)
	mux.HandleFunc("/__tm_access_denied", serveAccessDeniedHTML)
	mux.HandleFunc("/tm-device-sw.js", serveDeviceSW)
	mux.HandleFunc("/access", accessHandler)

	mux.HandleFunc("/user/logout", func(w http.ResponseWriter, r *http.Request) {
		cfg := loadConfig()
		if c, err := r.Cookie("ct_session"); err == nil {
			panelSess.Delete(c.Value)
			if usesPanelAccountMode(cfg) {
				if db, err := openPanelDB(cfg); err == nil {
					_, _ = db.Exec(`DELETE FROM live_sessions WHERE session_token=?`, c.Value)
				}
			}
		}
		ss := normalizeSessionSecurity(cfg)
		if dbConnected {
			if c, err := r.Cookie("ct_session"); err == nil {
				killSession(c.Value, "user_logout")
			}
		}
		secure := cookieSecure(r, cfg)
		http.SetCookie(w, &http.Cookie{Name: "ct_session", Value: "", Path: "/", Expires: time.Unix(0, 0), MaxAge: -1, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
		if ss.DeviceCookie.Enabled {
			cookieName := ss.DeviceCookie.CookieName
			if cookieName == "" {
				cookieName = "tm_device"
			}
			http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", Expires: time.Unix(0, 0), MaxAge: -1, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
		}
		// Local/panel: never bounce users to production member areas like toolsmandi.com.
		if cfg.LocalTestMode || cfg.BindLocalhost || usesPanelAccountMode(cfg) {
			renderAccessDeniedPage(w, cfg)
			return
		}
		memberAreaURL := cfg.MemberAreaURL
		if memberAreaURL == "" {
			memberAreaURL = "/"
		}
		http.Redirect(w, r, memberAreaURL, http.StatusFound)
	})

	mux.HandleFunc("/", proxyHandler)

	secureHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := loadConfig()
		if r.Header.Get("X-Forwarded-Proto") == "http" && !cfg.LocalTestMode && strings.EqualFold(cfg.PublicScheme, "https") {
			target := "https://" + r.Host + r.URL.RequestURI()
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		applySecurityHeaders(w, cfg)
		mux.ServeHTTP(w, r)
	})

	addr := ":" + cfg.Port
	if cfg.LocalTestMode && cfg.BindLocalhost {
		addr = "127.0.0.1:" + cfg.Port
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[SERVER] Fatal: listen %s: %v", addr, err)
	}
	log.Printf("✅ Generic Tool Proxy listening on http://%s", addr)
	server := &http.Server{
		Addr:         addr,
		Handler:      secureHandler,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  180 * time.Second,
	}
	if err := server.Serve(ln); err != nil {
		log.Fatalf("[SERVER] Fatal: %v", err)
	}
}
