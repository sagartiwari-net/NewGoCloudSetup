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
	// BlockedPrefixes: path prefixes to block
	BlockedPrefixes []string `json:"blocked_prefixes"`
	// ExtraCDNDomains: additional domains to rewrite in HTML responses
	ExtraCDNDomains []string `json:"extra_cdn_domains"`
	// SensitiveCookies: cookie names to strip from user's browser (prevent hijack)
	SensitiveCookies []string `json:"sensitive_cookies"`
	// WatchdogTriggers: client-side text patterns that trigger account rotation
	WatchdogTriggers []string `json:"watchdog_triggers"`
	// DisableExportTracking: set true for tools that don't have CSV export (ChatGPT, etc.)
	DisableExportTracking bool `json:"disable_export_tracking"`
	// DisableLimits: no credit widget, no limit enforcement, no usage logging
	DisableLimits bool `json:"disable_limits"`
	// UserAgent override
	UserAgent string `json:"user_agent"`
	// CookieFile: path to cookie.txt file (legacy, optional)
	CookieFile string `json:"cookie_file"`
	// PanelDB is the local panel database. When set, Open comes from the panel access link.
	PanelDB string `json:"panel_db"`
	// BypassAuth: bypasses database user authentication and loads cookie.txt directly
	BypassAuth bool `json:"bypass_auth"`
	// UseDatabase: when false, skip MySQL — local dev with cookie.txt only
	UseDatabase bool `json:"use_database"`
	// CloudflareBypass: stub /cdn-cgi challenge JS on proxy host
	CloudflareBypass bool `json:"cloudflare_bypass"`
	// LogoutDetection: config-driven rules to detect premium account logout
	LogoutDetection LogoutDetectionConfig `json:"logout_detection"`
	// Automation: external webhook to re-login / refresh cookies when logout detected
	Automation AutomationConfig `json:"automation"`
	// SessionSecurity: anti-sharing layers
	SessionSecurity proxysec.Config `json:"session_security"`
	// Local test mode — bypass aMember/DB auth; use cookie.txt directly (dev only)
	LocalTestMode bool `json:"local_test_mode"`
	BindLocalhost bool `json:"bind_localhost"`
	DebugLogging  bool `json:"debug_logging"`
}

// LogoutDetectionConfig holds rules that indicate the upstream premium session is logged out.
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

// AutomationConfig holds the external automation API call settings.
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
	automationStateMu             sync.Mutex
	currentConfig                 Config
	configModTime                 time.Time
	currentWebsiteID              int  = 1
	currentWebsiteSecurityEnabled bool = true
	dbConnected                   bool = false
	defaultConfig                      = Config{
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
		BlockedPrefixes:        []string{},
		ExtraCDNDomains:        []string{},
		SensitiveCookies:       []string{},
		WatchdogTriggers:       []string{},
		DisableExportTracking:  false,
	}
)

func resolveWebsiteID(publicHost string) {
	if !dbConnected {
		log.Printf("[LOCAL] Running in Standalone/Local mode, website_id = 1")
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
	if cfg.LogoutDetection.Enabled {
		if cfg.LogoutDetection.OverlayTitle == "" {
			cfg.LogoutDetection.OverlayTitle = "Session Update"
		}
		if cfg.LogoutDetection.OverlayMessage == "" {
			cfg.LogoutDetection.OverlayMessage = "Account logout detected. We are updating your session, please wait..."
		}
		if cfg.LogoutDetection.RefreshSeconds <= 0 {
			cfg.LogoutDetection.RefreshSeconds = 5
		}
	}
	if cfg.Automation.Enabled && cfg.Automation.CooldownSeconds <= 0 {
		cfg.Automation.CooldownSeconds = 300
	}
	cfg.SessionSecurity = normalizeSessionSecurity(cfg)

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
		log.Printf("[DB] ⚠️ Database not reachable: %v. Running in STANDALONE/LOCAL mode using '%s' fallback.", err, cfg.CookieFile)
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
		`CREATE TABLE IF NOT EXISTS ahrefs_security_logs (
			id INT AUTO_INCREMENT PRIMARY KEY,
			website_id INT NOT NULL DEFAULT 1,
			username VARCHAR(100) NOT NULL DEFAULT '',
			client_ip VARCHAR(64) NOT NULL DEFAULT '',
			event_type VARCHAR(64) NOT NULL,
			attempted_url VARCHAR(512) NOT NULL DEFAULT '',
			details VARCHAR(255) DEFAULT '',
			user_agent TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			INDEX (website_id), INDEX (username), INDEX (event_type), INDEX (created_at)
		) ENGINE=InnoDB;`,
		`CREATE TABLE IF NOT EXISTS ahrefs_blocked_ips (
			id INT AUTO_INCREMENT PRIMARY KEY,
			website_id INT NOT NULL DEFAULT 0,
			client_ip VARCHAR(64) NOT NULL,
			reason VARCHAR(255) DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE KEY uk_website_ip (website_id, client_ip),
			INDEX (client_ip)
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
	_, _ = db.Exec("ALTER TABLE ahrefs_websites ADD COLUMN proxy VARCHAR(255) DEFAULT ''")
	_, _ = db.Exec("DELETE FROM ahrefs_tokens WHERE expires_at < NOW()")
	_, _ = db.Exec("DELETE FROM ahrefs_sessions WHERE expires_at < NOW()")
	_, _ = db.Exec("ALTER TABLE ahrefs_accounts ADD COLUMN automation_task_uid VARCHAR(255) DEFAULT ''")
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

func scanToolAccount(row interface{ Scan(...interface{}) error }) (ToolAccount, error) {
	var acc ToolAccount
	var showLimitVal int
	err := row.Scan(&acc.ID, &acc.Name, &acc.Cookie, &acc.UserAgent, &acc.Proxy, &showLimitVal, &acc.AutomationTaskUID)
	if err != nil {
		return acc, err
	}
	acc.ShowLimit = showLimitVal == 1
	return acc, nil
}

const accountSelectCols = "id, name, cookie, user_agent, proxy, show_limit, COALESCE(automation_task_uid, '')"

func selectActiveAccount() (ToolAccount, error) {
	q := "SELECT " + accountSelectCols + " FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' ORDER BY last_used_at ASC LIMIT 1"
	row := db.QueryRow(q, currentWebsiteID)
	acc, err := scanToolAccount(row)
	if err != nil {
		return acc, err
	}
	_, _ = db.Exec("UPDATE ahrefs_accounts SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?", acc.ID)
	return acc, nil
}

func getSessionAssignedAccount(sessionToken string) (ToolAccount, bool) {
	if db == nil || !dbConnected || sessionToken == "" {
		return ToolAccount{}, false
	}
	var assignedAccountID sql.NullInt64
	err := db.QueryRow("SELECT assigned_account_id FROM ahrefs_sessions WHERE session_token = ? AND website_id = ?", sessionToken, currentWebsiteID).Scan(&assignedAccountID)
	if err != nil || !assignedAccountID.Valid {
		return ToolAccount{}, false
	}
	row := db.QueryRow("SELECT "+accountSelectCols+" FROM ahrefs_accounts WHERE id = ? AND website_id = ? AND status = 'active'",
		assignedAccountID.Int64, currentWebsiteID)
	acc, err := scanToolAccount(row)
	if err != nil {
		return ToolAccount{}, false
	}
	return acc, true
}

func autoAssignNextAccount(sessionToken string) (ToolAccount, error) {
	if db == nil || !dbConnected {
		return ToolAccount{}, fmt.Errorf("database not connected")
	}
	acc, err := selectActiveAccount()
	if err != nil {
		return acc, err
	}
	_, _ = db.Exec("UPDATE ahrefs_sessions SET assigned_account_id = ? WHERE session_token = ? AND website_id = ?", acc.ID, sessionToken, currentWebsiteID)
	log.Printf("[LB] Assigned account '%s' (ID:%d) to session '%s'", acc.Name, acc.ID, sessionToken)
	return acc, nil
}

func switchToNextAccount(sessionToken string, currentAccID int, currentAccName, username, reason string) (ToolAccount, error) {
	log.Printf("[LB] Rotating out '%s' (ID:%d) | Trigger: %s", currentAccName, currentAccID, reason)
	var acc ToolAccount
	var err error
	row := db.QueryRow(
		"SELECT "+accountSelectCols+" FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' AND id > ? ORDER BY id ASC LIMIT 1",
		currentWebsiteID, currentAccID,
	)
	acc, err = scanToolAccount(row)
	if err == sql.ErrNoRows {
		row = db.QueryRow(
			"SELECT "+accountSelectCols+" FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' AND id != ? ORDER BY id ASC LIMIT 1",
			currentWebsiteID, currentAccID,
		)
		acc, err = scanToolAccount(row)
	}
	if err != nil {
		row = db.QueryRow(
			"SELECT "+accountSelectCols+" FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' ORDER BY id ASC LIMIT 1",
			currentWebsiteID,
		)
		acc, err = scanToolAccount(row)
	}
	if err != nil {
		return acc, fmt.Errorf("no active accounts for website_id %d", currentWebsiteID)
	}
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
	if strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "{{") {
		var wrap struct {
			Cookies json.RawMessage `json:"cookies"`
		}
		if err := json.Unmarshal([]byte(trimmed), &wrap); err == nil {
			if inner := strings.TrimSpace(string(wrap.Cookies)); strings.HasPrefix(inner, "[") {
				trimmed = inner
			}
		}
	}
	// Cookie-Editor / EditThisCookie dumps are sometimes wrapped as {{[...]}}, {[...]}, etc.
	if i, j := strings.Index(trimmed, "["), strings.LastIndex(trimmed, "]"); i >= 0 && j > i {
		trimmed = trimmed[i : j+1]
	}
	if !strings.HasPrefix(trimmed, "[") {
		return raw
	}
	var cookies []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(trimmed), &cookies); err != nil {
		log.Printf("[COOKIE] JSON parse failed: %v", err)
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
		"auth_token": true, "STYXKEY_auth_token": true,
		"userId": true, "userEmail": true, "membershipType": true,
		"datadome": true, "ajs_user_id": true,
	}
}

func writeBrowserAuthCookies(w http.ResponseWriter, r *http.Request, cfg Config, cookieStr string) int {
	cookieStr = parseCookieFromDB(cookieStr)
	if cookieStr == "" {
		return 0
	}
	secure := cookieSecure(r, cfg)
	if cfg.LocalTestMode {
		secure = strings.EqualFold(cfg.PublicScheme, "https") || r.TLS != nil
	}
	authNames := localBrowserCookieNames()
	browserMap := parseCookieMap(r.Header.Get("Cookie"))
	n := 0
	for name, value := range parseCookieMap(cookieStr) {
		if !authNames[name] || value == "" {
			continue
		}
		// Never overwrite a fresh browser datadome (post-captcha) with stale account cookie.
		if name == "datadome" && browserMap["datadome"] != "" {
			continue
		}
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    value,
			Path:     "/",
			MaxAge:   86400,
			Secure:   secure,
			HttpOnly: false,
			SameSite: http.SameSiteLaxMode,
		})
		n++
	}
	return n
}

// setProxyBrowserAuthCookies puts account auth cookies on the proxy host so the
// SPA does not client-redirect to https://login.junglescout.com.
func setProxyBrowserAuthCookies(w http.ResponseWriter, r *http.Request, cfg Config, activeAcc ToolAccount) {
	if strings.TrimSpace(activeAcc.Cookie) == "" {
		return
	}
	// Panel + local_test must still set cookies (old code skipped LocalTestMode).
	if !usesPanelAccountMode(cfg) && (cfg.LocalTestMode || usesCookieFileMode(cfg)) {
		return
	}
	if n := writeBrowserAuthCookies(w, r, cfg, activeAcc.Cookie); n > 0 && debugEnabled(cfg) {
		log.Printf("[COOKIE] set %d browser auth cookies from account %s", n, activeAcc.Name)
	}
}

func setLocalBrowserAuthCookies(w http.ResponseWriter, r *http.Request, cfg Config) {
	if !cfg.LocalTestMode {
		return
	}
	// Prefer panel/account cookies when panel mode is on; cookie.txt is fallback.
	if usesPanelAccountMode(cfg) {
		return
	}
	// Host-only cookies on the proxy domain.
	cookieStr := loadCookiesFromFile(cfg.CookieFile)
	if n := writeBrowserAuthCookies(w, r, cfg, cookieStr); n > 0 && debugEnabled(cfg) {
		log.Printf("[COOKIE] set %d browser auth cookies from cookie.txt", n)
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
	accountToken := ""
	for _, part := range strings.Split(cookieStr, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "auth_token=") {
			accountToken = strings.TrimPrefix(part, "auth_token=")
			break
		}
	}
	auth := strings.TrimSpace(upstreamReq.Header.Get("Authorization"))
	// Panel mode: always prefer the mapped account JWT. Stale browser Bearer eyJ
	// cookies otherwise win and every API returns 401 Access Denied.
	if usesPanelAccountMode(cfg) && accountToken != "" {
		upstreamReq.Header.Set("Authorization", "Bearer "+accountToken)
		return
	}
	// Extension authenticate() uses a JWT daily_token (eyJ...). Do not replace it
	// with the raw auth_token — that breaks product/historical APIs (403/upsell UI).
	if strings.HasPrefix(auth, "Bearer eyJ") {
		return
	}
	if accountToken != "" {
		upstreamReq.Header.Set("Authorization", "Bearer "+accountToken)
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
	accountCookieStr := parseCookieFromDB(activeAcc.Cookie)
	accountMap := parseCookieMap(accountCookieStr)
	browserRaw := parseCookieMap(r.Header.Get("Cookie"))

	if cfg.LocalTestMode {
		fileMap := accountMap
		if len(fileMap) == 0 {
			fileMap = parseCookieMap(loadCookiesFromFile(cfg.CookieFile))
		}
		fileDatadome := fileMap["datadome"]
		delete(fileMap, "datadome")
		if v := browserRaw["datadome"]; v != "" {
			fileMap["datadome"] = v
		} else if fileDatadome != "" {
			fileMap["datadome"] = fileDatadome
		}
		return cookieMapToString(fileMap)
	}

	// Production: auth from account DB. Prefer fresh browser datadome (post-captcha),
	// else account datadome (must be from same residential proxy egress IP).
	accountDatadome := accountMap["datadome"]
	delete(accountMap, "datadome")
	clientCookies := stripSensitiveCookies(r.Header.Get("Cookie"), cfg)
	merged := parseCookieMap(clientCookies)
	for k, v := range accountMap {
		merged[k] = v
	}
	if v := browserRaw["datadome"]; v != "" {
		merged["datadome"] = v
	} else if accountDatadome != "" {
		merged["datadome"] = accountDatadome
	}
	return cookieMapToString(merged)
}

func stripSensitiveCookies(cookieHeader string, cfg Config) string {
	if cookieHeader == "" {
		return ""
	}
	sensitiveSet := make(map[string]bool)
	for _, n := range cfg.SensitiveCookies {
		sensitiveSet[strings.ToLower(n)] = true
	}
	sensitiveSet["ct_session"] = true
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
		if !proxysec.CloudflareRequestOK(r, cfg.SessionSecurity, toolCtx(cfg), currentWebsiteSecurityEnabled) {
			log.Printf("[SECURITY] CF-Ray required but missing | user=%s", username)
			recordSecurityEvent(r, username, "cloudflare_fail", "missing CF-Ray header")
			killSession(sessionToken, "cloudflare_ray_missing")
			return "", fmt.Errorf("cloudflare verification failed")
		}

		if cfg.PublicHost != "" && !isLocalDev(cfg) {
			reqHost := requestHost(r)
			if !isInternalRequestHost(reqHost) && !hostMatchesPublic(reqHost, cfg.PublicHost, ss.DomainCheck.AllowedHosts) {
				log.Printf("[SECURITY] Host mismatch | req=%s expected=%s user=%s", reqHost, cfg.PublicHost, username)
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
				log.Printf("[SECURITY] Device mismatch | user=%s has_cookie=%v", username, errD == nil)
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

		if ss.DatacenterScore.Enabled {
			concurrentFlag := ipTracker.ConcurrentSubnetCount(sessionToken) >= 2
			score := computeRiskScore(r, cfg, true, concurrentFlag)
			if score >= ss.DatacenterScore.ThresholdKill {
				log.Printf("[SECURITY] Risk score kill | user=%s score=%d threshold=%d", username, score, ss.DatacenterScore.ThresholdKill)
				recordSecurityEvent(r, username, "risk_score_kill", fmt.Sprintf("score=%d threshold=%d", score, ss.DatacenterScore.ThresholdKill))
				killSession(sessionToken, "risk_score_high")
				return "", fmt.Errorf("high risk score")
			}
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
		recordSecurityEvent(r, payload.Username, "invalid_handshake", "HMAC signature mismatch")
		http.Error(w, "Forbidden: Invalid signature", http.StatusForbidden)
		return
	}
	if time.Now().Unix()-payload.Timestamp > 300 {
		http.Error(w, "Forbidden: Request expired", http.StatusForbidden)
		return
	}
	if db == nil {
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

	scheme := requestScheme(r, cfg)
	host := cfg.PublicHost
	if host == "" {
		host = r.Host
	}
	redirectURL := fmt.Sprintf("%s://%s/access?user=%s&token=%s",
		scheme, host,
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
	username := r.URL.Query().Get("user")
	token := r.URL.Query().Get("token")
	if username == "" || token == "" {
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
	if err != nil {
		log.Printf("[ACCESS] ❌ OTT lookup failed for user=%s: %v", username, err)
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
	if dbUsername != username {
		recordSecurityEvent(r, username, "access_denied", "username mismatch")
		renderAccessDeniedPage(w, cfg)
		return
	}

	clientIP := realClientIP(r)
	ottOK, ottMode := proxysec.ValidateOTTClientIP(dbClientIP, clientIP, cfg.SessionSecurity, toolCtx(cfg), currentWebsiteSecurityEnabled)
	if !ottOK {
		log.Printf("[ACCESS] ❌ OTT IP mismatch | build=%s mode=%s user=%s token_ip=%s req_ip=%s",
			proxysec.BuildID, ottMode, username, dbClientIP, clientIP)
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
		log.Printf("[ACCESS] ❌ Session insert failed for user=%s: %v", username, err)
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
	if cfg.DisableLimits || usesPanelAccountMode(cfg) || db == nil || !dbConnected {
		currentUser := ""
		showLimit := false
		if name, err := getAuthenticatedUser(r, cfg); err == nil {
			currentUser = name
		}
		// disable_limits always wins — never surface ToolsMandi credit badge.
		if !cfg.DisableLimits && usesPanelAccountMode(cfg) {
			if c, err := r.Cookie("ct_session"); err == nil {
				if acc, accErr := loadPanelSessionAccount(cfg, c.Value); accErr == nil {
					showLimit = acc.ShowLimit
				}
			}
		}
		creditLabel := cfg.CreditLabel
		if creditLabel == "" {
			creditLabel = "Credits"
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"show_limit":     showLimit,
			"username":       currentUser,
			"credit_limit":   0,
			"credit_used":    0,
			"export_limit":   0,
			"export_used":    0,
			"credit_label":   creditLabel,
			"export_label":   cfg.ExportLabel,
			"tool_name":      cfg.ToolName,
			"disable_export": cfg.DisableExportTracking,
		})
		return
	}
	currentUser, err := getAuthenticatedUser(r, cfg)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "unauthorized", "message": err.Error()})
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
	if !dbConnected {
		fmt.Fprint(w, `{"status":"ok","switched_to":"Local Dev","id":1}`)
		return
	}
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
	oldAcc := activeAcc
	nextAcc, switchErr := switchToNextAccount(sessionToken, activeAcc.ID, activeAcc.Name, currentUser, reason)
	if switchErr != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"error":"no_other_accounts","message":"%v"}`, switchErr)
		return
	}
	cfg := loadConfig()
	tryTriggerAutomationForAccount(cfg, reason, oldAcc, currentUser)
	fmt.Fprintf(w, `{"status":"ok","switched_to":"%s","id":%d}`, nextAcc.Name, nextAcc.ID)
}

// ── ERROR PAGE RENDERERS ──────────────────────────────────────────────────────

func renderAccessDeniedPage(w http.ResponseWriter, cfg Config) {
	name := html.EscapeString(toolDisplayName(cfg))
	writeLightCard(w, http.StatusForbidden, lightCard{
		Title:   "Access Denied",
		Heading: "Access Denied",
		Message: "You cannot open <span class=\"brand\">" + name + "</span> directly. Open it again from your access link.",
		Footer:  "Your session ended or this browser is not authorized",
	})
}

// renderNoActiveAccountsPage shows the light "Waiting for an account" card and
// keeps retrying (eRank/Helium-style) until a mapped account is available.
func renderNoActiveAccountsPage(w http.ResponseWriter, cfg Config) {
	home := jungleAppHome
	if h := strings.TrimSpace(cfg.HomePath); h != "" && strings.HasPrefix(h, "/") {
		home = jungleReturnPath(h)
	}
	retry := "/login-proxy/?redirectRoute=" + url.QueryEscape(home)
	renderPanelWaitingForAccountPage(w, cfg, retry)
}

func renderLimitReachedPage(w http.ResponseWriter, limitType string, cfg Config) {
	name := html.EscapeString(toolDisplayName(cfg))
	title := "Daily Limit Reached"
	msg := "You have used up your daily limit for <span class=\"brand\">" + name + "</span>. Your limit resets at midnight (12:00 AM IST)."
	if limitType == "export" {
		title = "Export Limit Reached"
		msg = "You have reached your export limit for <span class=\"brand\">" + name + "</span>. Please contact support to increase your limit."
	}
	writeLightCard(w, http.StatusForbidden, lightCard{
		Title:   title,
		Heading: title,
		Message: msg,
		Footer:  "Open this tool again from your access link later",
	})
}

// ── PROXY SYSTEM (account → website DB → proxy.txt) ───────────────────────────
// Jungle Scout DataDome binds cookies to the outbound IP. Contabo datacenter IP
// will loop captchas forever — set a residential proxy on the website/account
// (or proxy.txt) so upstream exits a clean residential IP.

const PROXY_FILE = "proxy.txt"

var (
	currentProxy    *url.URL
	currentProxyStr string
	proxyMu         sync.RWMutex
	proxyFileMod    time.Time
	proxyFileURL    *url.URL
)

// parseProxyString converts common proxy formats to *url.URL.
//
//	socks5://user:pass@host:port
//	http://user:pass@host:port
//	host:port:user:pass   (shorthand → socks5)
//	host:port             (shorthand → socks5)
func parseProxyString(s string) *url.URL {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return nil
		}
		scheme := strings.ToLower(u.Scheme)
		if scheme != "socks5" && scheme != "socks5h" && scheme != "http" && scheme != "https" {
			log.Printf("[PROXY] Unknown proxy scheme '%s' — skipping", scheme)
			return nil
		}
		return u
	}
	parts := strings.SplitN(s, ":", 4)
	switch len(parts) {
	case 2:
		u, _ := url.Parse(fmt.Sprintf("socks5://%s:%s", parts[0], parts[1]))
		return u
	case 4:
		u, _ := url.Parse(fmt.Sprintf("socks5://%s:%s@%s:%s",
			url.QueryEscape(parts[2]), url.QueryEscape(parts[3]), parts[0], parts[1]))
		return u
	}
	log.Printf("[PROXY] Unrecognized proxy format '%s' — skipping", s)
	return nil
}

func loadProxyFromDB() *url.URL {
	if !dbConnected || db == nil {
		return nil
	}
	var proxyStr string
	err := db.QueryRow("SELECT COALESCE(proxy, '') FROM ahrefs_websites WHERE id = ?", currentWebsiteID).Scan(&proxyStr)
	if err != nil || strings.TrimSpace(proxyStr) == "" {
		proxyMu.Lock()
		currentProxy = nil
		currentProxyStr = ""
		proxyMu.Unlock()
		return nil
	}
	proxyMu.RLock()
	cached := currentProxyStr
	cachedURL := currentProxy
	proxyMu.RUnlock()
	if cached == proxyStr && cachedURL != nil {
		return cachedURL
	}
	parsed := parseProxyString(proxyStr)
	if parsed == nil {
		return nil
	}
	proxyMu.Lock()
	currentProxy = parsed
	currentProxyStr = proxyStr
	proxyMu.Unlock()
	log.Printf("[PROXY] Loaded website-level proxy from DB: %s://%s ✅", parsed.Scheme, parsed.Host)
	return parsed
}

func loadProxyFromFile() *url.URL {
	info, err := os.Stat(PROXY_FILE)
	if err != nil {
		proxyFileURL = nil
		return nil
	}
	if !info.ModTime().After(proxyFileMod) && proxyFileURL != nil {
		return proxyFileURL
	}
	data, err := os.ReadFile(PROXY_FILE)
	if err != nil {
		return proxyFileURL
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
	proxyFileMod = info.ModTime()
	if proxyStr == "" {
		proxyFileURL = nil
		return nil
	}
	parsed := parseProxyString(proxyStr)
	if parsed == nil {
		return proxyFileURL
	}
	proxyFileURL = parsed
	log.Printf("[PROXY] Loaded proxy.txt: %s://%s ✅", parsed.Scheme, parsed.Host)
	return proxyFileURL
}

// getProxy: website DB first, then proxy.txt (account proxy is via request context).
func getProxy() *url.URL {
	if px := loadProxyFromDB(); px != nil {
		return px
	}
	return loadProxyFromFile()
}

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
		px = parseProxyString(ctxPx)
	}
	if px == nil {
		px = getProxy()
	}
	if px != nil {
		tcpConn, err = dialThroughProxy(ctx, dialAddr, px)
		if err != nil {
			// Never fall back to Contabo direct — DataDome will infinite-loop captchas.
			log.Printf("[PROXY] ❌ Proxy failed (%v) — NOT falling back to direct (DataDome)", err)
			return nil, fmt.Errorf("proxy dial: %w", err)
		}
	} else {
		tcpConn, err = dialBypassLocalHosts(ctx, "tcp", dialAddr)
		if err != nil {
			return nil, fmt.Errorf("TCP dial: %w", err)
		}
	}
	uConn := utls.UClient(tcpConn, &utls.Config{ServerName: host, InsecureSkipVerify: false}, utls.HelloChrome_120)
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
	// HTTP/2 forbids Connection/Upgrade; nginx often attaches Connection: upgrade.
	req.Header.Del("Connection")
	req.Header.Del("Upgrade")
	req.Header.Del("Proxy-Connection")
	req.Header.Del("Keep-Alive")
	req.Header.Del("TE")
	req.Header.Del("Trailer")
	req.Header.Del("Transfer-Encoding")
	if px, ok := req.Context().Value(proxyContextKey).(string); ok && strings.TrimSpace(px) != "" {
		if rt.h1 != nil {
			rt.h1.CloseIdleConnections()
		}
		if rt.h2 != nil {
			rt.h2.CloseIdleConnections()
		}
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
	if proto == "h2" {
		return rt.h2.RoundTripOpt(req, http2.RoundTripOpt{})
	}
	return rt.h1.RoundTrip(req)
}

func buildChromeHTTPClient() *http.Client {
	dialTLS := func(ctx context.Context, network, addr string) (net.Conn, error) { return dialChrome(ctx, addr) }
	h1 := &http.Transport{
		DialTLSContext: dialTLS, MaxIdleConns: 100, MaxIdleConnsPerHost: 10,
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
		"members.junglescout.com": "104.26.14.247",
		"api.junglescout.com":     "54.192.142.34",
		"app.junglescout.com":     "104.26.14.247",
		"get.junglescout.com":     "104.26.14.247",
		"dd.junglescout.com":      "18.66.63.94",
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
		if strings.HasPrefix(path, "/pr/") {
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
	// Extension install — never send users to Chrome Web Store / official extension pages.
	pairs = append(pairs,
		[2]string{"https://www.junglescout.com/amazon-browser-extension", publicBase + "/ext-install"},
		[2]string{"http://www.junglescout.com/amazon-browser-extension", publicBase + "/ext-install"},
		[2]string{"https://www.junglescout.com/extension", publicBase + "/ext-install"},
		[2]string{"http://www.junglescout.com/extension", publicBase + "/ext-install"},
		[2]string{"https://junglescout.com/extension", publicBase + "/ext-install"},
		[2]string{"http://junglescout.com/extension", publicBase + "/ext-install"},
		[2]string{"https://members.junglescout.com/extension", publicBase + "/ext-install"},
		[2]string{"http://members.junglescout.com/extension", publicBase + "/ext-install"},
	)
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
	// Login / signup must stay on our domain (cookie expiry → /login-proxy, not official site).
	pairs = append(pairs,
		[2]string{"https://login.junglescout.com", publicBase + "/login-proxy"},
		[2]string{"http://login.junglescout.com", publicBase + "/login-proxy"},
		[2]string{"https://signup.junglescout.com", publicBase + "/signup-proxy"},
		[2]string{"http://signup.junglescout.com", publicBase + "/signup-proxy"},
	)
	// Other marketing aliases → proxy root.
	for _, alias := range []string{"www.junglescout.com", "junglescout.com", "app.junglescout.com"} {
		if targetParsed != nil && alias == targetParsed.Host {
			continue
		}
		pairs = append(pairs, [2]string{"https://" + alias, publicBase})
		pairs = append(pairs, [2]string{"http://" + alias, publicBase})
	}
	return pairs
}

func isAuthProxyPath(path string) bool {
	return path == "/login-proxy" || strings.HasPrefix(path, "/login-proxy/") ||
		path == "/signup-proxy" || strings.HasPrefix(path, "/signup-proxy/")
}

func authProxyUpstream(path string) (host, upstreamPath string, ok bool) {
	switch {
	case path == "/login-proxy" || strings.HasPrefix(path, "/login-proxy/"):
		rest := strings.TrimPrefix(path, "/login-proxy")
		if rest == "" {
			rest = "/"
		}
		return "login.junglescout.com", rest, true
	case path == "/signup-proxy" || strings.HasPrefix(path, "/signup-proxy/"):
		rest := strings.TrimPrefix(path, "/signup-proxy")
		if rest == "" {
			rest = "/"
		}
		return "signup.junglescout.com", rest, true
	default:
		return "", "", false
	}
}

// rewriteRootRelativeToPrefix rewrites src="/…" and href="/…" (not protocol-relative //)
// so login/signup shells load assets through the proxy prefix.
func rewriteRootRelativeToPrefix(body []byte, prefix string) []byte {
	if prefix == "" || len(body) == 0 {
		return body
	}
	result := body
	for _, attr := range []string{`src="`, `href="`, `src='`, `href='`} {
		search := []byte(attr + "/")
		var out bytes.Buffer
		data := result
		for {
			i := bytes.Index(data, search)
			if i < 0 {
				out.Write(data)
				break
			}
			out.Write(data[:i])
			rest := data[i+len(search):]
			if len(rest) > 0 && rest[0] == '/' {
				// protocol-relative URL (//cdn...) — leave untouched
				out.Write(search)
				data = rest
				continue
			}
			out.WriteString(attr)
			out.WriteString(prefix)
			out.WriteByte('/')
			data = rest
		}
		result = out.Bytes()
	}
	return result
}

func extensionRouteScript() string {
	return `<script>(function(){
  function isExtHash(h){
    if(!h) return false;
    h = h.split('?')[0];
    return h === '#/extension' || h.indexOf('#/extension/') === 0 ||
      h === '#/chrome-extension' || h.indexOf('#/browser-extension') === 0;
  }
  function go(){ try { location.replace('/ext-install'); } catch(e) { location.href='/ext-install'; } }
  function check(){ if (isExtHash(location.hash||'')) go(); }
  check();
  window.addEventListener('hashchange', check);
  document.addEventListener('click', function(e){
    var a = e.target && e.target.closest ? e.target.closest('a') : null;
    if (!a) return;
    var href = a.getAttribute('href') || '';
    if (isExtHash(href) || href.indexOf('/ext-install') === 0) {
      e.preventDefault();
      go();
      return;
    }
    var t = (a.textContent || '').replace(/\s+/g,' ').trim().toLowerCase();
    if (t === 'extension' || t === 'browser extension') {
      e.preventDefault();
      go();
    }
  }, true);
})();</script>`
}

// mapOfficialLoginToProxy keeps login.junglescout.com on our host as /login-proxy/...
// Example: https://login.junglescout.com/?redirectRoute=%2Fdashboard
//       → http://127.0.0.1:5211/login-proxy/?redirectRoute=%2Fdashboard
func mapOfficialLoginToProxy(loc, publicBase string) string {
	if publicBase == "" {
		publicBase = ""
	}
	u, err := url.Parse(loc)
	if err != nil {
		return publicBase + "/login-proxy/?redirectRoute=" + url.QueryEscape(jungleAppHome)
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	out := publicBase + "/login-proxy" + path
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		out += "#" + u.Fragment
	}
	return out
}

func rewriteLocationHeader(loc string, pairs [][2]string) string {
	if loc == "" {
		return loc
	}
	if strings.Contains(strings.ToLower(loc), "login.junglescout.com") {
		publicBase := ""
		for _, pair := range pairs {
			if strings.HasPrefix(pair[0], "https://members.junglescout.com") || strings.HasPrefix(pair[0], "http://members.junglescout.com") {
				publicBase = pair[1]
				break
			}
		}
		return mapOfficialLoginToProxy(loc, publicBase)
	}
	for _, pair := range pairs {
		loc = strings.ReplaceAll(loc, pair[0], pair[1])
	}
	return loc
}

// loginStayScript forces official login navigations onto /login-proxy (same origin).
func loginStayScript() string {
	return `<script data-tm-login-stay="1">(function(){
  var O=location.origin;
  function asStr(u){
    if(typeof u==='string') return u;
    if(u&&typeof u==='object'){
      if(typeof u.href==='string') return u.href;
      try{return String(u);}catch(e){}
    }
    return u;
  }
  function stay(u){
    u=asStr(u);
    if(typeof u!=='string'||!/login\.junglescout\.com/i.test(u)) return u;
    try{
      var x=new URL(u,O);
      var path=x.pathname||'/';
      return O+'/login-proxy'+path+(x.search||'')+(x.hash||'');
    }catch(e){return O+'/login-proxy/?redirectRoute=/dashboard';}
  }
  try{
    var _a=Location.prototype.assign;
    Location.prototype.assign=function(u){return _a.call(this,stay(u));};
    var _r=Location.prototype.replace;
    Location.prototype.replace=function(u){return _r.call(this,stay(u));};
    var d=Object.getOwnPropertyDescriptor(Location.prototype,'href');
    if(d&&d.set&&d.get){
      Object.defineProperty(Location.prototype,'href',{
        get:d.get, set:function(v){return d.set.call(this,stay(v));}, configurable:true
      });
    }
    if(window.navigation&&navigation.navigate){
      var _nav=navigation.navigate.bind(navigation);
      navigation.navigate=function(u,opts){return _nav(stay(asStr(u)),opts);};
    }
  }catch(e){}
  document.addEventListener('click',function(e){
    var a=e.target&&e.target.closest&&e.target.closest('a[href]');
    if(!a||!/login\.junglescout\.com/i.test(a.getAttribute('href')||a.href||'')) return;
    e.preventDefault(); e.stopPropagation();
    location.assign(stay(a.href));
  },true);
})();</script>`
}

func stripLocalTestScripts(body []byte) []byte {
	// Only strip external <script src="..."> tags. Never use a cross-script
	// content match — patterns like "mixpanel" appear deep in the SPA and a
	// greedy <script>…mixpanel…</script> regex deletes all React bundles.
	domains := []string{
		"cdn.optimizely.com", "cdn.segment.com", "googletagmanager.com",
		"googleoptimize.com", "navattic.com", "profitwell",
		"dna8twue3dlxq.cloudfront.net", "js.stripe.com", "mxpnl.com",
		"gtm.js", "analytics.min.js",
	}
	for _, d := range domains {
		re := regexp.MustCompile(`(?is)<script[^>]*\ssrc=["'][^"']*` + regexp.QuoteMeta(d) + `[^"']*["'][^>]*>\s*</script>`)
		body = re.ReplaceAll(body, []byte(""))
	}
	return body
}

func localTestStubScript() string {
	return `<script>window.dataLayer=window.dataLayer||[];window.analytics=window.analytics||{track:function(){},identify:function(){},page:function(){},group:function(){},load:function(){},ready:function(f){if(f)f();}};window.optimizely=window.optimizely||[];window.mixpanel=window.mixpanel||{track:function(){},identify:function(){},init:function(){}};</script>`
}

func redirectRouteScript() string {
	return `<script>(function(){try{var p=new URLSearchParams(location.search);var r=p.get('redirectRoute');if(r&&r.charAt(0)==='/'){var hash='#'+r;history.replaceState(null,'',hash);if(!location.hash)location.hash=r;}}catch(e){}})();</script>`
}

func hostnameSpoofInner(cfg Config) string {
	// Spoof hostname/host (not href/origin) so embeds like Wistia Academy
	// treat the page as members.junglescout.com. patchUrl must rewrite any
	// absolute members/api URLs the SPA builds back onto the proxy origin.
	proxyHostname := cfg.PublicHost
	port := strings.TrimSpace(cfg.Port)
	if host, p, err := net.SplitHostPort(cfg.PublicHost); err == nil {
		proxyHostname = host
		if p != "" {
			port = p
		}
	}
	low := strings.ToLower(proxyHostname)
	if strings.Contains(low, "junglescout.com") {
		return ""
	}
	targetParsed, _ := url.Parse(cfg.TargetURL)
	fakeHostname := "members.junglescout.com"
	if targetParsed != nil && targetParsed.Hostname() != "" {
		fakeHostname = targetParsed.Hostname()
	}
	// Prefer hostname without port for Wistia domain allow-lists.
	fakeHostWithPort := fakeHostname
	_ = port
	return fmt.Sprintf(`
    try {
        var FAKE_NAME = %q;
        var FAKE_HOST = %q;
        var PROXY_NAME = %q;
        function isLocalDevHost(h) {
            if (!h) return false;
            h = String(h).toLowerCase();
            return h === '127.0.0.1' || h === 'localhost' || h === '::1' || h === PROXY_NAME.toLowerCase() ||
              h.indexOf('127.0.0.1:') === 0 || h.indexOf('localhost:') === 0 || h.indexOf(PROXY_NAME.toLowerCase()+':') === 0;
        }
        var _hostDesc = Object.getOwnPropertyDescriptor(Location.prototype, 'hostname');
        if (_hostDesc && _hostDesc.get) {
            var _realHostGet = _hostDesc.get;
            Object.defineProperty(Location.prototype, 'hostname', {
                get: function() {
                    var h = _realHostGet.call(this);
                    if (isLocalDevHost(h)) return FAKE_NAME;
                    return h;
                },
                configurable: true
            });
        }
        var _hostFullDesc = Object.getOwnPropertyDescriptor(Location.prototype, 'host');
        if (_hostFullDesc && _hostFullDesc.get) {
            var _realHostFullGet = _hostFullDesc.get;
            Object.defineProperty(Location.prototype, 'host', {
                get: function() {
                    var h = _realHostFullGet.call(this);
                    if (isLocalDevHost(h)) return FAKE_HOST;
                    return h;
                },
                configurable: true
            });
        }
        // Do not spoof location.origin — that makes same-origin checks / device-bind
        // resolve against members.junglescout.com and triggers CORS failures.
	} catch(e) {}
`, fakeHostname, fakeHostWithPort, proxyHostname)
}

// hideProfileMenuItemsScript hides only the listed profile-menu bits and shows
// the panel username in place of the Jungle Scout account full name.
func hideProfileMenuItemsScript(panelUsername string) string {
	userJS, _ := json.Marshal(strings.TrimSpace(panelUsername))
	return fmt.Sprintf(`<script>
(function(){
  var TM_USER = %s;
  var LABELS = {
    'connect an amazon account': 1,
    'all settings': 1,
    'contact support': 1,
    'log out': 1
  };
  function norm(s) {
    return String(s || '').replace(/\s+/g, ' ').trim().toLowerCase();
  }
  function hideEl(el) {
    if (!el || (el.dataset && el.dataset.tmHideMenu === '1')) return;
    el.style.setProperty('display', 'none', 'important');
    el.setAttribute('aria-hidden', 'true');
    if (el.dataset) el.dataset.tmHideMenu = '1';
  }
  function hideMenuRow(el) {
    if (!el) return;
    var target = el.closest('[role="menuitem"]') ||
      el.closest('[class*="ButtonWrapper-sc-1jjq2tp"]') ||
      el.closest('button') ||
      el.closest('[class*="ButtonWrapper"]') ||
      el;
    // Prefer the outer profile-menu ButtonWrapper so empty padding disappears.
    var outer = el.closest('[class*="ButtonWrapper-sc-1jjq2tp"]');
    if (outer) target = outer;
    hideEl(target);
    var prev = target.previousElementSibling;
    if (prev && prev.getAttribute && prev.getAttribute('role') === 'separator') hideEl(prev);
    var parent = target.parentElement;
    if (parent) {
      var pprev = parent.previousElementSibling;
      if (pprev && pprev.getAttribute && pprev.getAttribute('role') === 'separator') hideEl(pprev);
    }
  }
  function setName(el) {
    if (!el || !TM_USER) return;
    if (el.dataset && el.dataset.tmUserSet === TM_USER) return;
    el.textContent = TM_USER;
    if (el.dataset) el.dataset.tmUserSet = TM_USER;
  }
  // Sagar → SA, Abhishek → AB (first 2 letters of the panel username).
  function initials(name) {
    var s = String(name || '').replace(/[^a-zA-Z0-9]/g, '');
    if (!s) return '';
    return s.slice(0, 2).toUpperCase();
  }
  function setInitials(el) {
    if (!el || !TM_USER) return;
    var ini = initials(TM_USER);
    if (!ini) return;
    if (el.dataset && el.dataset.tmUserSet === TM_USER) return;
    el.textContent = ini;
    if (el.dataset) el.dataset.tmUserSet = TM_USER;
  }
  function scan(root) {
    if (!root || !root.querySelectorAll) return;
    var nodes = root.querySelectorAll('button, [role="menuitem"], [class*="ItemContent"], [class*="ContentWrapper"], [class*="ButtonContent"]');
    for (var i = 0; i < nodes.length; i++) {
      var el = nodes[i];
      var text = norm(el.textContent);
      if (!text || text.length > 48) continue;
      if (LABELS[text]) hideMenuRow(el);
    }
    // Hide only the profile email line (class contains Email-).
    var emails = root.querySelectorAll('[class*="Email-sc-"], [class*="Email-"]');
    for (var e = 0; e < emails.length; e++) {
      var em = emails[e];
      if (em.textContent && em.textContent.indexOf('@') !== -1) hideEl(em);
    }
    // Replace Jungle Scout full name with panel username.
    var names = root.querySelectorAll('[class*="FullName-sc-"], [class*="FullName-"]');
    for (var n = 0; n < names.length; n++) setName(names[n]);
    // Avatar fallback letters (e.g. <span class="Fallback-...">JJ</span>) → SA / AB.
    var falls = root.querySelectorAll('span[class*="Fallback-sc-"], span[class*="Fallback-"]');
    for (var f = 0; f < falls.length; f++) setInitials(falls[f]);
  }
  function run() { try { scan(document); } catch (e) {} }
  run();
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', run);
  try {
    new MutationObserver(function(){ run(); }).observe(document.documentElement, { childList: true, subtree: true });
  } catch (e) {}
  setInterval(run, 1500);
})();
</script>`, string(userJS))
}

// academyVideoScript preloads Wistia and keeps protocol-relative player URLs on https.
func academyVideoScript() string {
	return `<script>
(function(){
  try {
    if (!document.querySelector('script[data-tm-wistia]')) {
      var s = document.createElement('script');
      s.src = 'https://fast.wistia.com/assets/external/E-v1.js';
      s.async = true;
      s.setAttribute('data-tm-wistia','1');
      (document.head || document.documentElement).appendChild(s);
    }
    function fixWistiaSrc(v) {
      if (typeof v !== 'string') return v;
      if (v.indexOf('//fast.wistia.') === 0 || v.indexOf('//embed.wistia.') === 0 || v.indexOf('//embed-ssl.wistia.') === 0) {
        return 'https:' + v;
      }
      return v;
    }
    var desc = Object.getOwnPropertyDescriptor(HTMLScriptElement.prototype, 'src');
    if (desc && desc.set) {
      Object.defineProperty(HTMLScriptElement.prototype, 'src', {
        get: desc.get,
        set: function(v) { return desc.set.call(this, fixWistiaSrc(v)); },
        configurable: true,
        enumerable: true
      });
    }
    var _setAttr = Element.prototype.setAttribute;
    Element.prototype.setAttribute = function(name, value) {
      if (String(name).toLowerCase() === 'src') value = fixWistiaSrc(value);
      return _setAttr.call(this, name, value);
    };
  } catch (e) {}
})();
</script>`
}

func antiClickjackFixScript() string {
	return `<script>(function(){function fix(){try{var e=document.getElementById('antiClickjack');if(e)e.remove();if(self===top&&document.body)document.body.style.setProperty('display','block','important');}catch(x){}}fix();if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',fix);})();</script>`
}

func staticDirectScript(targetHost string) string {
	return fmt.Sprintf(`<script>try{__webpack_public_path__='https://%s/';}catch(e){}</script>`, targetHost)
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

func isDatadomeChallengeHTML(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	s := strings.ToLower(string(body))
	markers := []string{
		"verification required",
		"slide right to secure",
		"captcha-delivery.com",
		"geo.captcha-delivery",
		"datadome",
		"dd.js",
	}
	hits := 0
	for _, m := range markers {
		if strings.Contains(s, m) {
			hits++
		}
	}
	// Real Catalyst shells also mention datadome lightly — require stronger signal.
	if strings.Contains(s, "verification required") || strings.Contains(s, "slide right to secure") {
		return true
	}
	if strings.Contains(s, "captcha-delivery.com") && bytes.Count(body, []byte("/static/js/")) < 5 {
		return true
	}
	return hits >= 3 && bytes.Count(body, []byte("/static/js/")) < 5
}

func datadomeChallengePatchScript(cfg Config) string {
	// Minimal patch for captcha pages only: keep DataDome traffic on our host so
	// solved datadome cookies land on jungle.1clkaccess.store (not members.junglescout.com).
	var extras strings.Builder
	for i, extra := range cfg.ExtraCDNDomains {
		extraClean := strings.TrimPrefix(strings.TrimPrefix(extra, "https://"), "http://")
		extraClean = strings.Split(extraClean, "/")[0]
		extras.WriteString(fmt.Sprintf("{h:%q,p:%q},", extraClean, fmt.Sprintf("/extra-cdn-%d", i)))
	}
	publicBase := fmt.Sprintf("%s://%s", cfg.PublicScheme, cfg.PublicHost)
	if cfg.PublicScheme == "" || cfg.PublicHost == "" {
		publicBase = ""
	}
	return fmt.Sprintf(`<script>(function(){
  var O=%q||location.origin;
  var EXTRAS=[%s];
  function patch(u){
    if(typeof u!=='string') return u;
    var pairs=[
      ['https://members.junglescout.com', O],
      ['http://members.junglescout.com', O],
      ['https://login.junglescout.com', O+'/login-proxy'],
      ['http://login.junglescout.com', O+'/login-proxy']
    ];
    for(var i=0;i<pairs.length;i++){
      if(u.indexOf(pairs[i][0])===0) return pairs[i][1]+u.slice(pairs[i][0].length);
    }
    for(var j=0;j<EXTRAS.length;j++){
      var h=EXTRAS[j].h, p=EXTRAS[j].p;
      var a='https://'+h, b='http://'+h;
      if(u.indexOf(a)===0) return O+p+u.slice(a.length);
      if(u.indexOf(b)===0) return O+p+u.slice(b.length);
    }
    return u;
  }
  function applyDD(h){
    if(!h) return;
    var m=String(h).match(/datadome=([^;\s]+)/i);
    if(!m) return;
    var max=31536000, mm=String(h).match(/max-age=(\d+)/i);
    if(mm) max=parseInt(mm[1],10)||max;
    document.cookie='datadome='+m[1]+'; path=/; max-age='+max+'; SameSite=Lax';
  }
  var xo=XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open=function(m,u){
    if(typeof u==='string') arguments[1]=patch(u);
    this.addEventListener('load',function(){
      try{applyDD(this.getResponseHeader('x-set-cookie')||this.getResponseHeader('X-Set-Cookie'));}catch(e){}
    });
    return xo.apply(this,arguments);
  };
  var fo=window.fetch;
  window.fetch=function(inp,init){
    init=init||{};
    if(!init.credentials) init.credentials='include';
    if(typeof inp==='string') inp=patch(inp);
    else if(inp&&typeof inp==='object'&&inp.url) inp=new Request(patch(inp.url),inp);
    return fo(inp,init).then(function(res){
      try{applyDD(res.headers.get('x-set-cookie')||res.headers.get('X-Set-Cookie'));}catch(e){}
      return res;
    });
  };
})();</script>`, publicBase, extras.String())
}

func forwardDatadomeHeaders(w http.ResponseWriter, upstreamResp *http.Response, path string, cfg Config) {
	// Always forward DataDome response headers + cookie so captcha solves stick
	// on the proxy domain (not only on /extra-cdn-* paths).
	for _, name := range []string{"X-Set-Cookie", "X-Datadome", "X-Datadome-Cid", "X-Dd-B"} {
		if v := upstreamResp.Header.Get(name); v != "" {
			w.Header().Set(name, v)
			w.Header().Add("Access-Control-Expose-Headers", strings.ToLower(name))
		}
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
	_ = path
}

// ── SERVICE WORKER (disabled — was intercepting API → api.junglescout.com causing CORS 502) ─

func serviceWorkerJS() string {
	return `self.addEventListener('install',function(e){self.skipWaiting();});
self.addEventListener('activate',function(e){e.waitUntil(self.clients.claim());});`
}

func unregisterServiceWorkerScript() string {
	return `<script>
if('serviceWorker' in navigator){
  navigator.serviceWorker.getRegistrations().then(function(regs){
    regs.forEach(function(r){r.unregister();});
  });
}
</script>`
}

func serviceWorkerRegisterScript(cfg Config) string {
	if !cfg.LocalTestMode {
		return ""
	}
	return unregisterServiceWorkerScript()
}

func serviceWorkerHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[PROXY:SW] serving /sw.js | host=%q referer=%q", r.Host, r.Header.Get("Referer"))
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Service-Worker-Allowed", "/")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(serviceWorkerJS()))
}

// ── CLIENT-SIDE PATCHER SCRIPT ────────────────────────────────────────────────

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

	logoutAutoJS := ""
	if !usesCookieFileMode(cfg) {
		logoutAutoJS = buildLogoutAutomationJS(cfg)
	}

	hostnameSpoof := hostnameSpoofInner(cfg)
	publicBase := fmt.Sprintf("%s://%s", cfg.PublicScheme, cfg.PublicHost)
	if cfg.PublicScheme == "" || cfg.PublicHost == "" {
		publicBase = ""
	}

	return fmt.Sprintf(`<script>
(function() {
    var T = '%s', C = '%s', O = '%s' || window.location.origin;
    var HOME = '%s';
    var BLOCKED = [%s];
    var EXTRAS = [%s];
    window.__TM_REAL_ORIGIN__ = O;

    var STATIC_ORIGIN = 'https://' + T;

    function directStatic(u) {
        if (typeof u !== 'string') return null;
        if (u.charAt(0) === '/' && u.indexOf('/static/') === 0) return STATIC_ORIGIN + u;
        if (u.indexOf(O + '/static/') === 0) return STATIC_ORIGIN + u.slice(O.length);
        if (u.indexOf(STATIC_ORIGIN + '/static/') === 0) return u;
        return null;
    }

    function patchUrl(u) {
        if (u && typeof u === 'object' && typeof u.href === 'string') u = u.href;
        if (typeof u !== 'string') return u;
        var direct = directStatic(u);
        if (direct) return direct;
        // Official login → /login-proxy on our host (keeps query for account-switch later).
        if (/login\.junglescout\.com/i.test(u)) {
            try {
                var lu = new URL(u, O);
                return O + '/login-proxy' + (lu.pathname || '/') + (lu.search || '') + (lu.hash || '');
            } catch (e) { return O + '/login-proxy/?redirectRoute=/dashboard'; }
        }
        if (u.indexOf('https://signup.junglescout.com') === 0) return O + '/signup-proxy' + u.slice('https://signup.junglescout.com'.length);
        if (u.indexOf('http://signup.junglescout.com') === 0) return O + '/signup-proxy' + u.slice('http://signup.junglescout.com'.length);
        // Extension install pages → our installer
        if (/junglescout\.com\/(amazon-browser-)?extension/i.test(u)) {
            return O + '/ext-install';
        }
        // Official members (incl. protocol-relative) → stay on local proxy
        if (u.indexOf('https://'+T) === 0) u = O + u.slice(('https://'+T).length);
        else if (u.indexOf('http://'+T) === 0) u = O + u.slice(('http://'+T).length);
        else if (u.indexOf('//'+T) === 0) u = O + u.slice(('//'+T).length);
        // Hostname spoof may build members URLs without going through https:// prefix helpers.
        if (u.indexOf('https://members.junglescout.com') === 0) u = O + u.slice('https://members.junglescout.com'.length);
        else if (u.indexOf('http://members.junglescout.com') === 0) u = O + u.slice('http://members.junglescout.com'.length);
        else if (u.indexOf('//members.junglescout.com') === 0) u = O + u.slice('//members.junglescout.com'.length);
        u = u.replace('https://junglescout.com', O).replace('http://junglescout.com', O);
        u = u.replace('https://www.junglescout.com', O).replace('http://www.junglescout.com', O);
        u = u.replace('https://app.junglescout.com', O).replace('http://app.junglescout.com', O);
        if (C && C !== T) u = u.replace('https://'+C, O+'/cdn-proxy').replace('http://'+C, O+'/cdn-proxy');
        for (var i = 0; i < EXTRAS.length; i++) {
            var re = new RegExp('(https?:)?\\/\\/' + EXTRAS[i].host.replace(/\\./g, '\\\\.'), 'g');
            u = u.replace(re, O + EXTRAS[i].path);
        }
        return u;
    }
    function fixNavUrl(u) { return patchUrl(u); }
%s

    // ── Watchdog: Auto-rotate account on session drop ──
    var watchdogDone = false;
    function checkWatchdog() {
        if (watchdogDone) return;
        var txt = document.body ? (document.body.innerText || document.body.textContent || '').trim() : '';
        if (txt.length > 8000) return;
        var isTrigger = false, reason = '';
        %s
        if (isTrigger) {
            watchdogDone = true;
            var over = document.createElement('div');
            over.style.cssText = 'position:fixed;top:0;left:0;width:100vw;height:100vh;background:#0f172a;color:#fff;z-index:99999999;display:flex;flex-direction:column;align-items:center;justify-content:center;font-family:sans-serif;gap:16px;';
            over.innerHTML = '<div style="width:40px;height:40px;border:4px solid #f8fafc;border-top-color:#4f46e5;border-radius:50%%;animation:tm-spin 1s linear infinite;"></div><span style="font-weight:600;">Reconnecting session... Please wait...</span><style>@keyframes tm-spin{0%%{transform:rotate(0deg)}100%%{transform:rotate(360deg)}}</style>';
            document.body.appendChild(over);
            fetch('/api/rotate-session?reason=' + encodeURIComponent(reason))
            .then(function() { setTimeout(function() { window.location.href = HOME; }, 1200); })
            .catch(function() { setTimeout(function() { window.location.href = HOME; }, 1200); });
        }
    }
    if (%s) { setTimeout(checkWatchdog, 1000); setInterval(checkWatchdog, 4000); }

    // ── Link rewriter ──
    function rewriteLinks() {
        document.querySelectorAll('a[href]').forEach(function(a) {
            var h = patchUrl(a.href);
            if (a.href !== h) a.href = h;
        });
    }
    rewriteLinks();
    setInterval(rewriteLinks, 200);

    // Recover only when browser actually left the proxy (real URL bar on official site)
    var recoverHandler = { fn: function() {} };
    recoverHandler.fn();
    setInterval(function() { recoverHandler.fn(); }, 800);

    try {
        var OFFICIAL = 'https://' + T;
        // Present members.junglescout.com to embeds (Wistia Academy domain allow-list).
        // Navigation/setter still force URLs back through patchUrl → local proxy.
        function fakeHref(real) {
            if (typeof real !== 'string') return real;
            if (real.indexOf(O) === 0) return OFFICIAL + real.slice(O.length);
            return real;
        }
        var _open = window.open;
        window.open = function(url, target, features) {
            if (typeof url === 'string') url = patchUrl(url);
            return _open.call(window, url, target, features);
        };
        var _assign = window.location.assign.bind(window.location);
        window.location.assign = function(u) { return _assign(fixNavUrl(u)); };
        var _locReplace = window.location.replace.bind(window.location);
        window.location.replace = function(u) { return _locReplace(fixNavUrl(u)); };
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
                    return;
                }
                if (/login\.junglescout\.com/i.test(real)) {
                    window.location.replace(fixNavUrl(real));
                }
            };
            try {
                Object.defineProperty(document, 'URL', { get: function() { return fakeHref(_hrefGet.call(window.location)); }, configurable: true });
                Object.defineProperty(document, 'documentURI', { get: function() { return fakeHref(_hrefGet.call(window.location)); }, configurable: true });
            } catch(e2) {}
        }
    } catch(e) {}

    // ── XHR patch ──
    var xo = XMLHttpRequest.prototype.open;
    XMLHttpRequest.prototype.open = function(m, u) {
        if (typeof u === 'string') { arguments[1] = patchUrl(u); }
        return xo.apply(this, arguments);
    };

    // ── Fetch patch ──
    // Only force credentials on our proxy origin. Cross-origin (Wistia, etc.)
    // must keep default credentials — ACAO:* rejects credentialed fetches (CORS).
    var fo = window.fetch;
    window.fetch = function(inp, init) {
        init = init ? Object.assign({}, init) : {};
        if (typeof inp === 'string') { inp = patchUrl(inp); }
        else if (inp && typeof inp === 'object' && inp.url) {
            var rewritten = patchUrl(inp.url);
            if (rewritten !== inp.url) {
                inp = new Request(rewritten, inp);
            }
        }
        var abs = '';
        try {
            abs = typeof inp === 'string' ? inp : (inp && inp.url) || '';
            var resolved = new URL(abs, O);
            if (resolved.origin === O && init.credentials === undefined) {
                init.credentials = 'include';
            }
        } catch (e) {
            if (typeof abs === 'string' && abs.charAt(0) === '/' && init.credentials === undefined) {
                init.credentials = 'include';
            }
        }
        return fo(inp, init);
    };

    // ── Block restricted paths via History API ──
    if (BLOCKED.length > 0) {
        function isBlocked(path) {
            var clean = path.split('?')[0].replace(/\/$/, '');
            for (var i = 0; i < BLOCKED.length; i++) {
                if (clean === BLOCKED[i] || clean.startsWith(BLOCKED[i]+'/')) return true;
            }
            return false;
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

    %s
})();
</script>`,
		targetHost, cdnHost, publicBase, homePath, blockedListJS.String(), extraCDNsJS.String(), hostnameSpoof, triggerChecks.String(),
		func() string {
			if len(cfg.WatchdogTriggers) > 0 && !cfg.LocalTestMode {
				return "true"
			}
			return "false"
		}(),
		logoutAutoJS,
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
        overlay.style.cssText = 'position:fixed;top:0;left:0;width:100vw;height:100vh;background:rgba(8,12,20,0.85);backdrop-filter:blur(12px);z-index:99999999;display:flex;align-items:center;justify-content:center;font-family:sans-serif;';
        var modal = document.createElement('div');
        modal.style.cssText = 'width:90%;max-width:480px;background:rgba(15,23,42,0.9);border:1px solid rgba(255,255,255,0.08);border-radius:24px;padding:32px;display:flex;flex-direction:column;align-items:center;text-align:center;gap:20px;';
        modal.innerHTML = '<div style="font-size:48px;background:rgba(239,68,68,0.1);width:80px;height:80px;border-radius:40px;display:flex;align-items:center;justify-content:center;border:1px solid rgba(239,68,68,0.2);">⚠️</div>' +
            '<h2 style="font-size:22px;font-weight:700;color:#f87171;margin:0;">Limit Reached</h2>' +
            '<p style="font-size:15px;color:#94a3b8;line-height:1.6;margin:0;">' + msg + '</p>' +
            '<button onclick="window.location.reload()" style="background:linear-gradient(135deg,#f59e0b,#d97706);color:#fff;border:none;padding:12px 24px;border-radius:12px;font-size:15px;font-weight:600;cursor:pointer;width:100%;">Refresh Page</button>';
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

func hasHostsEntry() bool {
	data, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return false
	}
	matched, _ := regexp.Match(`(?m)^127\.0\.0\.1\s+members\.junglescout\.com(\s|$)`, data)
	return matched
}

func classifyPath(path string) string {
	switch {
	case strings.HasPrefix(path, "/extra-cdn-0"):
		return "API"
	case strings.HasPrefix(path, "/extra-cdn-3"):
		return "DATADOME"
	case strings.HasPrefix(path, "/extra-cdn-"):
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

func summarizeCookieFlags(cookieHeader string, prefix string) string {
	if cookieHeader == "" {
		return prefix + "_cookies=NONE"
	}
	flags := make([]string, 0, 4)
	for _, name := range []string{"auth_token", "STYXKEY_auth_token", "datadome", "userId"} {
		if strings.Contains(cookieHeader, name+"=") {
			flags = append(flags, name+"=yes")
		} else {
			flags = append(flags, name+"=no")
		}
	}
	return prefix + "_" + strings.Join(flags, " ")
}

func summarizeCookies(cookieHeader string) string {
	return summarizeCookieFlags(cookieHeader, "browser")
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
	totalMS := time.Since(rl.start).Milliseconds()
	cat := rl.category
	if cat == "" {
		cat = classifyPath(rl.path)
	}
	shouldLog := debugEnabled(cfg) || rl.errMsg != "" || rl.status >= 400 || cat == "HTML" ||
		strings.HasPrefix(rl.path, "/access") || strings.HasPrefix(rl.path, "/api/")
	if !shouldLog {
		return
	}

	if debugEnabled(cfg) {
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

	if cat == "API" && rl.status == 403 {
		log.Printf("[PROXY:WARN] API 403 on %s — Datadome blocking; run sudo ./setup-local-hosts.sh + fresh cookie.txt", rl.path)
	}
	if cfg.LocalTestMode && !hasHostsEntry() && (cat == "API" || strings.HasPrefix(rl.path, "/extra-cdn-0")) {
		log.Printf("[PROXY:WARN] API needs /etc/hosts — run: sudo ./setup-local-hosts.sh then use http://members.junglescout.com:%s", cfg.Port)
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
				s.total, s.api2xx, s.api403, s.apiOther, s.staticOK, s.staticSlow, s.errors, s.slow3s, hasHostsEntry())
			if s.api403 > 0 && s.api2xx == 0 {
				log.Printf("[PROXY:STATS] ⚠️  All API calls returned 403 — run sudo ./setup-local-hosts.sh + use members.junglescout.com URL")
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
		targetHost := "members.junglescout.com"
		if tp, err := url.Parse(cfg.TargetURL); err == nil && tp.Hostname() != "" {
			targetHost = tp.Hostname()
		}
		if strings.Contains(cfg.PublicHost, targetHost) && hasHostsEntry() {
			log.Printf("[PROXY:STARTUP] ✅ /etc/hosts OK — use http://%s", cfg.PublicHost)
		} else if strings.Contains(cfg.PublicHost, targetHost) {
			log.Printf("[PROXY:STARTUP] ⚠️  Run: sudo ./setup-local-hosts.sh (local dev needs %s in /etc/hosts)", cfg.PublicHost)
		} else {
			log.Printf("[PROXY:STARTUP] ✅ proxy domain=%s://%s (no /etc/hosts needed)", cfg.PublicScheme, cfg.PublicHost)
		}
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
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Cookie, Authorization, X-Requested-With, Accept, Origin, Referer, sec-ch-ua, sec-ch-ua-mobile, sec-ch-ua-platform, X-Device-Fp, X-Device-Proof, X-Datadome-Clientid")
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

	// ── 0c. Disk CDN cache (static / safe CDN; never API or DataDome) ───────────
	cdnKey := cdnCacheKey(r)
	if serveCachedCDN(w, r) {
		rl.status = http.StatusOK
		rl.category = "CDN_CACHE"
		return
	}
	if cdnKey != "" {
		defer completeCDNFlight(cdnKey)
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
			if strings.Contains(strings.ToLower(path), "favicon") || cdnKey != "" {
				activeAcc = ToolAccount{}
				if cdnKey != "" {
					currentUser = "public_cdn"
				}
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
			// Soft bounce only — path-only failover loops forever with one account
			// ("Switching account" ↔ login-proxy). Real logout is handled after
			// upstream HTML/API checks. Re-bake browser auth cookies and go home.
			if isDocumentNavigation(r) && isAuthProxyPath(path) && sessionToken != "" {
				returnPath := jungleReturnPath(r.URL.Query().Get("redirectRoute"))
				setProxyBrowserAuthCookies(w, r, cfg, activeAcc)
				log.Printf("[LB] login-proxy soft bounce user=%s account=%s → %s", currentUser, activeAcc.Name, returnPath)
				http.Redirect(w, r, returnPath, http.StatusFound)
				return
			}
		}
	} else if usesCookieFileMode(cfg) {
		currentUser = "local_dev"
		if cfg.LocalTestMode {
			activeAcc = getLocalTestAccount(cfg)
		} else {
			parsedCookie := loadCookiesFromFile(cfg.CookieFile)
			activeAcc = ToolAccount{
				ID:        1,
				Name:      "Local Standalone Account",
				Cookie:    parsedCookie,
				UserAgent: cfg.UserAgent,
				ShowLimit: false,
			}
		}
		if activeAcc.Cookie == "" {
			if usesCookieFileMode(cfg) && !cfg.LocalTestMode {
				renderNoActiveAccountsPage(w, cfg)
			} else {
				http.Error(w, "cookie.txt empty or missing — add Jungle Scout cookies first", http.StatusServiceUnavailable)
			}
			return
		}
	} else {
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

	// ── 2. Check blocked paths ────────────────────────────────────────────────────
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

	// MySQL session assign only — panel mode already pinned the account above.
	if dbConnected && db != nil && !usesPanelAccountMode(cfg) && !usesCookieFileMode(cfg) {
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

	if detected, reason := detectLogout(path, nil, cfg); detected && cfg.LogoutDetection.Enabled {
		log.Printf("[LOGOUT] Path hint | user=%s account=%s reason=%s", currentUser, activeAcc.Name, reason)
	}

	// ── 4. Credit limit check ────────────────────────────────────────────────────
	if dbConnected && db != nil && !cfg.DisableLimits && !usesPanelAccountMode(cfg) && !usesCookieFileMode(cfg) && isCountedPath(path, cfg) {
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
	// GrowSumo partner scripts (get.junglescout.com) — must not hit members HTML shell.
	if strings.HasPrefix(path, "/pr/") {
		upstreamURL.Scheme = "https"
		upstreamURL.Host = "get.junglescout.com"
	}
	// Login / signup shells — keep expired sessions on our domain.
	if host, upPath, ok := authProxyUpstream(path); ok {
		upstreamURL.Scheme = "https"
		upstreamURL.Host = host
		upstreamURL.Path = upPath
	}

	upstreamReq, err := http.NewRequestWithContext(
		context.WithValue(r.Context(), proxyContextKey, activeAcc.Proxy),
		r.Method, upstreamURL.String(), r.Body,
	)
	if err != nil {
		http.Error(w, "Failed to build upstream request", http.StatusInternalServerError)
		return
	}

	// Copy headers
	for k, vv := range r.Header {
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
	if isExtraCDNPath(path) || strings.Contains(upstreamURL.Host, "junglescout.com") {
		enrichUpstreamDatadomeHeaders(upstreamReq, r, cfg)
	}
	if isExtraCDNPath(path) {
		if clientIP := realClientIP(r); clientIP != "" {
			upstreamReq.Header.Set("X-Forwarded-For", clientIP)
		}
	}

	// Set upstream host header
	upstreamReq.Host = upstreamURL.Host
	if upstreamReq.Host == "" {
		upstreamReq.Host = targetParsed.Host
	}

	// Remove hop-by-hop + proxy headers. nginx/browser "Connection: upgrade"
	// must never reach HTTP/2 upstream (Go: invalid Connection request header).
	upstreamReq.Header.Del("Connection")
	upstreamReq.Header.Del("Upgrade")
	upstreamReq.Header.Del("Proxy-Connection")
	upstreamReq.Header.Del("Keep-Alive")
	upstreamReq.Header.Del("Transfer-Encoding")
	upstreamReq.Header.Del("TE")
	upstreamReq.Header.Del("X-Device-Fp")
	upstreamReq.Header.Del("X-Device-Proof")
	upstreamReq.Header.Del("X-Forwarded-For")
	upstreamReq.Header.Del("X-Real-IP")
	upstreamReq.Header.Del("Origin")
	// API host must see a junglescout.com origin — not the proxy (127.0.0.1).
	originHost := cfg.TargetURL
	if strings.HasPrefix(path, "/extra-cdn-0/") {
		originHost = "https://members.junglescout.com"
	}
	upstreamReq.Header.Set("Origin", originHost)
	referer := originHost + "/"
	if !isExtraCDNPath(path) {
		referer = originHost + path
	}
	upstreamReq.Header.Set("Referer", referer)

	// ── 6. Perform upstream request ───────────────────────────────────────────────
	rl.upstream = upstreamURL.String()
	upstreamStart := time.Now()
	client := pickHTTPClient(cfg, path)
	upstreamResp, err := client.Do(upstreamReq)
	rl.upstreamMS = time.Since(upstreamStart).Milliseconds()
	if err != nil {
		rl.errMsg = err.Error()
		rl.status = http.StatusBadGateway
		log.Printf("[PROXY:ERR] upstream fail | %s %s | up=%s | took=%dms | err=%v | %s | %s",
			r.Method, path, upstreamURL.String(), rl.upstreamMS, err,
			summarizeCookies(r.Header.Get("Cookie")),
			summarizeCookieFlags(upstreamCookies, "account"))
		// Do not rotate accounts on transient upstream errors (context canceled, extra-cdn, etc.)
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
	if cfg.LocalTestMode {
		setLocalBrowserAuthCookies(w, r, cfg)
	}
	if loc := upstreamResp.Header.Get("Location"); loc != "" {
		newLoc := rewriteLocationHeader(loc, rewritePairs)
		w.Header().Set("Location", newLoc)
		if newLoc != loc {
			log.Printf("[REDIRECT] rewrite %s %s | %s -> %s", r.Method, path, loc, newLoc)
		} else if upstreamResp.StatusCode >= 300 && upstreamResp.StatusCode < 400 {
			log.Printf("[REDIRECT] %s %s | status=%d location=%s", r.Method, path, upstreamResp.StatusCode, loc)
		}
	}

	// Browser cache: 1-day for safe static/CDN; no-store for HTML/API/DataDome.
	// Allow even if upstream set CF cookies (we already strip Set-Cookie above).
	canBrowserCache := cdnKey != "" && upstreamResp.StatusCode == http.StatusOK &&
		!strings.Contains(strings.ToLower(contentType), "text/html") &&
		!strings.Contains(strings.ToLower(contentType), "json") &&
		!strings.Contains(strings.ToLower(upstreamResp.Header.Get("Content-Disposition")), "attachment")
	if canBrowserCache {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Del("Pragma")
		w.Header().Del("Expires")
	} else {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
	}

	// ── 8. SSE (Server-Sent Events) passthrough ───────────────────────────────────
	if !canBrowserCache && isSSEResponse(contentType) {
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
		!strings.HasPrefix(path, "/extra-cdn-0") &&
		!strings.HasPrefix(path, "/pr/") &&
		!strings.HasPrefix(path, "/cdn-cgi/")
	if isHTML {
		bodyBytes, err := decompressBody(upstreamResp)
		if err != nil {
			w.WriteHeader(upstreamResp.StatusCode)
			return
		}

		// Logout detection on HTML body (skip overlay for SPA false positives)
		logoutPageDetected := false
		if cfg.LogoutDetection.Enabled {
			if detected, reason := detectLogout(path, bodyBytes, cfg); detected {
				if strings.HasPrefix(reason, "text_sniff:") && len(bodyBytes) > 20000 {
					log.Printf("[LOGOUT] Ignored SPA false-positive | user=%s reason=%s body_len=%d", currentUser, reason, len(bodyBytes))
				} else {
					log.Printf("[LOGOUT] HTML detection | user=%s account=%s reason=%s", currentUser, activeAcc.Name, reason)
					handleLogoutDetected(cfg, reason, activeAcc, currentUser, sessionToken)
					logoutPageDetected = true
				}
			}
		}

		// Rewrite domain references
		bodyBytes = rewriteBody(bodyBytes, rewritePairs)
		bodyBytes = regexp.MustCompile(`(?i)<base[^>]+href=["']https?://[^"']*junglescout\.com[^"']*["'][^>]*>`).ReplaceAll(bodyBytes, []byte(""))
		configuredPublic := fmt.Sprintf("%s://%s", cfg.PublicScheme, cfg.PublicHost)
		if r.Host != "" && r.Host != cfg.PublicHost {
			actualPublic := fmt.Sprintf("%s://%s", cfg.PublicScheme, r.Host)
			bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(configuredPublic), []byte(actualPublic))
		}

		// DataDome challenge: keep page clean but patch fetch/XHR so solved cookies
		// land on our proxy host (otherwise loop forever on Contabo IP).
		if isDatadomeChallengeHTML(bodyBytes) {
			proxyHint := "none"
			if strings.TrimSpace(activeAcc.Proxy) != "" {
				proxyHint = "account"
			} else if getProxy() != nil {
				proxyHint = "website/file"
			}
			log.Printf("[PROXY:WARN] DataDome challenge on %s | account=%s outbound_proxy=%s — Contabo IP needs residential proxy (ctrl website/account proxy or proxy.txt)", path, activeAcc.Name, proxyHint)
			inject := datadomeChallengePatchScript(cfg) + antiClickjackFixScript()
			bodyBytes = regexp.MustCompile(`(?i)<head[^>]*>`).ReplaceAllFunc(bodyBytes, func(m []byte) []byte {
				out := make([]byte, 0, len(m)+len(inject))
				out = append(out, m...)
				out = append(out, []byte(inject)...)
				return out
			})
			w.Header().Del("Content-Security-Policy")
			w.Header().Del("Content-Security-Policy-Report-Only")
			w.Header().Del("X-Frame-Options")
		localDev := isLocalDevHost(cfg)
		if usesPanelAccountMode(cfg) && !localDev {
			bodyBytes = injectDeviceHTML(bodyBytes)
		}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			applySecurityHeaders(w, cfg)
			w.WriteHeader(upstreamResp.StatusCode)
			w.Write(bodyBytes)
			return
		}

		if isAuthProxyPath(path) {
			prefix := "/login-proxy"
			if strings.HasPrefix(path, "/signup-proxy") {
				prefix = "/signup-proxy"
			}
			bodyBytes = rewriteRootRelativeToPrefix(bodyBytes, prefix)
		} else if cfg.LocalTestMode && !usesPanelAccountMode(cfg) {
			// Panel mode keeps /static on the proxy so login URL rewrites always apply.
			bodyBytes = stripLocalTestScripts(bodyBytes)
			targetParsed, _ := url.Parse(cfg.TargetURL)
			staticHost := "members.junglescout.com"
			if targetParsed != nil && targetParsed.Hostname() != "" {
				staticHost = targetParsed.Hostname()
			}
			publicBase := fmt.Sprintf("%s://%s", cfg.PublicScheme, cfg.PublicHost)
			if r.Host != "" && r.Host != cfg.PublicHost {
				publicBase = fmt.Sprintf("%s://%s", cfg.PublicScheme, r.Host)
			}
			bodyBytes = rewriteStaticToOrigin(bodyBytes, staticHost, publicBase)
		} else if cfg.LocalTestMode {
			bodyBytes = stripLocalTestScripts(bodyBytes)
		}

		// Remove CSP header (prevents our injected scripts)
		w.Header().Del("Content-Security-Policy")
		w.Header().Del("Content-Security-Policy-Report-Only")
		w.Header().Del("X-Frame-Options")
		localDev := isLocalDevHost(cfg)
		if usesPanelAccountMode(cfg) && !localDev {
			bodyBytes = injectDeviceHTML(bodyBytes)
		} else if usesPanelAccountMode(cfg) && localDev {
			log.Printf("[DEVICE] skip inject (local/bypass) host=%s bypass=%v", cfg.PublicHost, cfg.BypassAuth)
		}

		setProxyBrowserAuthCookies(w, r, cfg, activeAcc)
		displayUser := strings.TrimSpace(currentUser)
		if displayUser == "" || displayUser == "local_dev" {
			if name, uErr := getAuthenticatedUser(r, cfg); uErr == nil {
				displayUser = strings.TrimSpace(name)
			}
		}
		earlyInject := loginStayScript() + serviceWorkerRegisterScript(cfg) + antiClickjackFixScript() + academyVideoScript() + hideProfileMenuItemsScript(displayUser) + redirectRouteScript() + extensionRouteScript() + patcherScript(cfg)
		if cfg.LocalTestMode {
			targetParsed, _ := url.Parse(cfg.TargetURL)
			staticHost := "members.junglescout.com"
			if targetParsed != nil && targetParsed.Hostname() != "" {
				staticHost = targetParsed.Hostname()
			}
			earlyInject = loginStayScript() + serviceWorkerRegisterScript(cfg) + antiClickjackFixScript() + academyVideoScript() + hideProfileMenuItemsScript(displayUser)
			// Panel mode: do not point webpack at real members CDN (breaks login stay).
			if !usesPanelAccountMode(cfg) {
				earlyInject += staticDirectScript(staticHost)
			}
			earlyInject += redirectRouteScript() + extensionRouteScript() + localTestStubScript() + patcherScript(cfg)
		}
		if !usesCookieFileMode(cfg) {
			ss := normalizeSessionSecurity(cfg)
			if securityEnabled(cfg) && currentWebsiteSecurityEnabled {
				if ss.DomainCheck.Enabled {
					earlyInject += buildDomainCheckJS(cfg)
				}
				if ss.SecurityHeartbeat.Enabled {
					earlyInject += buildSecurityHeartbeatJS(cfg)
				}
			}
			if logoutPageDetected && cfg.LogoutDetection.Enabled {
				earlyInject = logoutOverlayImmediateScript(cfg) + earlyInject
			}
		}
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
		applySecurityHeaders(w, cfg)
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
			if canBrowserCache {
				storeCDNCache(r, upstreamResp.StatusCode, contentType, bodyBytes)
			}
			w.Header().Del("Content-Encoding")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(bodyBytes)))
			w.WriteHeader(upstreamResp.StatusCode)
			w.Write(bodyBytes)
			return
		}
	}

	// ── 11. Pass through everything else ─────────────────────────────────────────
	if canBrowserCache {
		bodyBytes, err := decompressBody(upstreamResp)
		if err == nil {
			storeCDNCache(r, upstreamResp.StatusCode, contentType, bodyBytes)
			w.Header().Del("Content-Encoding")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(bodyBytes)))
			w.WriteHeader(upstreamResp.StatusCode)
			w.Write(bodyBytes)
			return
		}
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
	startCDNCacheSweep()
	log.Printf("🚀 Starting Generic Tool Proxy — Tool: %s | Target: %s | Port: %s", cfg.ToolName, cfg.TargetURL, cfg.Port)
	log.Printf("[AUTOMATION] logout_detection=%v automation=%v url=%s task_uid=%v",
		cfg.LogoutDetection.Enabled, cfg.Automation.Enabled, cfg.Automation.URL, cfg.Automation.Payload["task_uid"])
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
	refreshWebsiteSecurityFromDB()
	initScoreEngine(cfg)
	ottMode := ss.OTTIPValidation.Mode
	if ottMode == "" {
		ottMode = "soft"
	}
	log.Printf("[PROXY] build_tag=%s public_host=%s use_database=%v db_connected=%v", proxyBuildTag, cfg.PublicHost, cfg.UseDatabase, dbConnected)
	if px := getProxy(); px != nil {
		log.Printf("[PROXY] outbound residential proxy ready: %s://%s", px.Scheme, px.Host)
	} else {
		log.Printf("[PROXY] ⚠️ no website/proxy.txt outbound proxy — Contabo IP will hit DataDome loops; set residential proxy")
	}
	log.Printf("[SECURITY] build=%s lib=%s enabled=%v db_toggle=%v single_session=%v device=%v domain_check=%v ott_ip=%v",
		proxysec.BuildID, proxysec.BuildID, securityEnabled(cfg), currentWebsiteSecurityEnabled, ss.SingleSessionPerUser,
		ss.DeviceCookie.Enabled, ss.DomainCheck.Enabled, ss.OTTIPValidation.Enabled)
	startDailyResetCron()
	startBlockedIPRefreshLoop()

	mux := http.NewServeMux()

	// ── API routes ────────────────────────────────────────────────────────────────
	mux.HandleFunc("/api/auth-handshake", withCORS(authHandshakeHandler))
	mux.HandleFunc("/api/user-limits", withCORS(userLimitsAPIHandler))
	mux.HandleFunc("/api/rotate-session", withCORS(rotateSessionHandler))
	mux.HandleFunc("/api/trigger-automation", withCORS(triggerAutomationHandler))
	mux.HandleFunc("/api/security-ping", withCORS(securityPingHandler))

	// ── Service worker (local test API bypass) ───────────────────────────────────
	mux.HandleFunc("/sw.js", serviceWorkerHandler)

	// ── Access handler (OTT → session cookie) ────────────────────────────────────
	mux.HandleFunc("/api/device-bind", deviceBindHandler)
	mux.HandleFunc("/__tm_access_denied", serveAccessDeniedHTML)
	mux.HandleFunc("/tm-device-sw.js", serveDeviceSW)
	mux.HandleFunc("/access", accessHandler)

	// ── Custom extension installer (host-bound ZIP) ──────────────────────────────
	mux.HandleFunc("/ext-install", extensionInstallPageHandler)
	mux.HandleFunc("/extension.zip", extensionZipHandler)
	mux.HandleFunc("/sessions_ext", sessionsExtHandler)
	mux.HandleFunc("/isauth", isauthRedirectHandler)
	mux.HandleFunc("/jssessions", sessionsExtHandler)

	// ── Logout ────────────────────────────────────────────────────────────────────
	mux.HandleFunc("/user/logout", func(w http.ResponseWriter, r *http.Request) {
		cfg := loadConfig()
		ss := normalizeSessionSecurity(cfg)
		if dbConnected {
			if c, err := r.Cookie("ct_session"); err == nil {
				killSession(c.Value, "user_logout")
			}
		}
		secure := cookieSecure(r, cfg)
		sameSite := http.SameSiteLaxMode
		http.SetCookie(w, &http.Cookie{
			Name:     "ct_session",
			Value:    "",
			Path:     "/",
			Expires:  time.Unix(0, 0),
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   secure,
			SameSite: sameSite,
		})
		if ss.DeviceCookie.Enabled {
			cookieName := ss.DeviceCookie.CookieName
			if cookieName == "" {
				cookieName = "tm_device"
			}
			http.SetCookie(w, &http.Cookie{
				Name:     cookieName,
				Value:    "",
				Path:     "/",
				Expires:  time.Unix(0, 0),
				MaxAge:   -1,
				HttpOnly: true,
				Secure:   secure,
				SameSite: sameSite,
			})
		}
		memberAreaURL := cfg.MemberAreaURL
		if memberAreaURL == "" {
			memberAreaURL = "/"
		}
		http.Redirect(w, r, memberAreaURL, http.StatusFound)
	})

	// ── Proxy catch-all ───────────────────────────────────────────────────────────
	mux.HandleFunc("/", proxyHandler)

	secureHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := loadConfig()
		if r.Header.Get("X-Forwarded-Proto") == "http" && !cfg.LocalTestMode && strings.EqualFold(cfg.PublicScheme, "https") {
			target := "https://" + r.Host + r.URL.RequestURI()
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
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
