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
	// UserAgent override
	UserAgent string `json:"user_agent"`
	// CookieFile: path to cookie.txt file (legacy, optional)
	CookieFile string `json:"cookie_file"`
	// BypassAuth: bypasses database user authentication and loads cookie.txt directly (useful for testing without security)
	BypassAuth bool `json:"bypass_auth"`
	// PanelDB: local panel SQLite path. When set, ChatGPT requires a panel access token and loads the mapped account from that database.
	PanelDB string `json:"panel_db"`
	// UseDatabase: when false, skip MySQL — local dev with cookie.txt only
	UseDatabase bool `json:"use_database"`
	// CloudflareBypass: stub /cdn-cgi challenge JS on proxy host
	CloudflareBypass bool `json:"cloudflare_bypass"`
	// LogoutDetection: config-driven rules to detect premium account logout
	LogoutDetection LogoutDetectionConfig `json:"logout_detection"`
	// Automation: external webhook to re-login / refresh cookies when logout detected
	Automation AutomationConfig `json:"automation"`
	// SessionSecurity: anti-sharing layers (see security.md)
	SessionSecurity proxysec.Config `json:"session_security"`
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

const CONFIG_FILE = "config.json"
const AUTOMATION_STATE_FILE = "automation_state.json"

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
	cfg.SessionSecurity = normalizeSessionSecurity(cfg)

	currentConfig = cfg
	configModTime = info.ModTime()
	log.Printf("[CONFIG] Reloaded from %s ✅ (tool: %s, target: %s)", CONFIG_FILE, cfg.ToolName, cfg.TargetURL)
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
			if dbConnected && db != nil {
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
	acc, err := selectActiveAccount()
	if err != nil {
		return acc, err
	}
	_, _ = db.Exec("UPDATE ahrefs_sessions SET assigned_account_id = ? WHERE session_token = ? AND website_id = ?", acc.ID, sessionToken, currentWebsiteID)
	log.Printf("[LB] Assigned account '%s' (ID:%d) to session '%s'", acc.Name, acc.ID, sessionToken)
	return acc, nil
}

func switchToNextAccount(sessionToken string, currentAccID int, currentAccName, username, reason string) (ToolAccount, error) {
	cfg := loadConfig()
	if usesPanelAccountMode(cfg) {
		return panelSwitchAccount(cfg, sessionToken, currentAccID, currentAccName, username, reason)
	}
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
	if trimmed == "{}" || trimmed == "[]" {
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

func stripSensitiveCookies(cookieHeader string, cfg Config) string {
	if cookieHeader == "" {
		return ""
	}
	sensitiveSet := make(map[string]bool)
	for _, n := range cfg.SensitiveCookies {
		sensitiveSet[strings.ToLower(n)] = true
	}
	sensitiveSet["ct_session"] = true // Always strip our internal session cookie!
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

// expireSessionCookies clears proxy auth cookies so the browser does not keep sending
// a stale ct_session after the DB row was deleted (prevents Access Denied loops).
func expireSessionCookies(w http.ResponseWriter, r *http.Request, cfg Config) {
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
	ss := normalizeSessionSecurity(cfg)
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
}

func isStaleSessionError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "session not found") ||
		strings.Contains(msg, "session expired") ||
		strings.Contains(msg, "session ended") ||
		strings.Contains(msg, "device validation failed") ||
		strings.Contains(msg, "session sharing detected")
}

func isTelemetryPath(path string) bool {
	return strings.HasPrefix(path, "/ces/") ||
		strings.Contains(path, "/telemetry/") ||
		strings.Contains(path, "/sdk_exception")
}

// cookieSecure returns true when the client connection is HTTPS (direct TLS or reverse-proxy headers).
// Falls back to config public_scheme when nginx omits X-Forwarded-Proto (common on aaPanel).
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

// requestScheme builds external URLs (redirects). Prefers real HTTPS, else config.
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
	return cfg.BypassAuth || !cfg.UseDatabase || !dbConnected || isLocalDev(cfg)
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

		// Host check — skip when nginx forwards internal Host (127.0.0.1:6001)
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
			if score >= ss.DatacenterScore.ThresholdWarn {
				log.Printf("[SECURITY] Risk score warn | user=%s score=%d", username, score)
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
	redirectURL := fmt.Sprintf("%s://%s/access?token=%s",
		scheme, host, url.QueryEscape(ott))
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
	if err != nil {
		log.Printf("[ACCESS] ❌ OTT lookup failed user=%s website_id=%d token=%s… err=%v",
			username, currentWebsiteID, token[:min(8, len(token))], err)
		recordSecurityEvent(r, username, "access_denied", "invalid or used token (website_id="+strconv.Itoa(currentWebsiteID)+")")
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
		log.Printf("[ACCESS] ❌ OTT IP mismatch | build=%s mode=%s user=%s token_ip=%s req_ip=%s match16=%v",
			proxysec.BuildID, ottMode, username, dbClientIP, clientIP, proxysec.SameSubnet16(dbClientIP, clientIP))
		recordSecurityEvent(r, username, "ott_ip_mismatch", "mode="+ottMode+" token_ip="+dbClientIP+" req_ip="+clientIP)
		_, _ = db.Exec("DELETE FROM ahrefs_tokens WHERE token = ? AND website_id = ?", token, currentWebsiteID)
		renderAccessDeniedPage(w, cfg)
		return
	}

	_, _ = db.Exec("DELETE FROM ahrefs_tokens WHERE token = ? AND website_id = ?", token, currentWebsiteID)

	ss := normalizeSessionSecurity(cfg)

	// Single session per user — kill existing sessions before creating new one
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
	// SameSite=Lax: required for member-area → /access → / redirect chain (Strict drops cookies on that redirect).
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

func panelAssignedShowLimit(r *http.Request, cfg Config, username string) bool {
	db, err := openPanelDB(cfg)
	if err != nil {
		return false
	}
	var websiteID, accountID int
	if err = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID); err != nil || websiteID == 0 {
		return false
	}
	if username != "" {
		var mode string
		_ = db.QueryRow(`SELECT COALESCE(limit_visibility, '') FROM panel_users WHERE website_id=? AND username=?`, websiteID, username).Scan(&mode)
		switch strings.ToLower(strings.TrimSpace(mode)) {
		case "show":
			return true
		case "hide":
			return false
		}
	}
	token := ""
	if c, err := r.Cookie("ct_session"); err == nil {
		token = c.Value
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if token != "" {
		_ = db.QueryRow(`SELECT assigned_account_id FROM live_sessions WHERE session_token=? AND website_id=? AND expires_at>?`, token, websiteID, now).Scan(&accountID)
	}
	if accountID == 0 && username != "" {
		_ = db.QueryRow(`SELECT assigned_account_id FROM live_sessions WHERE username=? AND website_id=? AND expires_at>? ORDER BY id DESC LIMIT 1`, username, websiteID, now).Scan(&accountID)
	}
	if accountID == 0 {
		return false
	}
	var show int
	if err = db.QueryRow(`SELECT COALESCE(show_limit, 0) FROM accounts WHERE id=?`, accountID).Scan(&show); err != nil {
		return false
	}
	return show == 1
}

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

	creditLabel, exportLabel := cfg.CreditLabel, cfg.ExportLabel
	if creditLabel == "" {
		creditLabel = "Credits"
	}

	// Local / cookie.txt mode: no MySQL — never touch nil db (was panicking and breaking UI).
	if usesPanelAccountMode(cfg) {
		state, err := panelCreditStateFor(currentUser)
		if err != nil {
			state.limit = 0
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"show_limit":     panelAssignedShowLimit(r, cfg, currentUser),
			"username":       currentUser,
			"credit_limit":   state.limit,
			"credit_used":    state.used,
			"export_limit":   0,
			"export_used":    0,
			"credit_label":   creditLabel,
			"export_label":   exportLabel,
			"tool_name":      cfg.ToolName,
			"disable_export": true,
		})
		return
	}
	if usesCookieFileMode(cfg) || !dbConnected || db == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"show_limit":     false,
			"username":       currentUser,
			"credit_limit":   999999,
			"credit_used":    0,
			"export_limit":   999999,
			"export_used":    0,
			"credit_label":   creditLabel,
			"export_label":   exportLabel,
			"tool_name":      cfg.ToolName,
			"disable_export": cfg.DisableExportTracking,
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
	nextAcc, switchErr := switchToNextAccount(sessionToken, activeAcc.ID, activeAcc.Name, currentUser, reason)
	if switchErr != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"error":"no_other_accounts","message":"%v"}`, switchErr)
		return
	}
	fmt.Fprintf(w, `{"status":"ok","switched_to":"%s","id":%d}`, nextAcc.Name, nextAcc.ID)
}

// ── LOGOUT DETECTION & AUTOMATION ─────────────────────────────────────────────

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
	loggedInShell := chatGPTLoggedInHTML(body)
	if !loggedInShell {
		for _, t := range ld.TextSniffs {
			if t == "" {
				continue
			}
			if strings.Contains(bodyStr, t) || strings.Contains(strings.ToLower(bodyStr), strings.ToLower(t)) {
				return true, "text_sniff:" + t
			}
		}
		for _, h := range ld.HTMLSniffs {
			if h != "" && strings.Contains(bodyStr, h) {
				return true, "html_sniff:" + h[:min(60, len(h))]
			}
		}
	}
	// Fallback: partial match for common login button text
	if strings.Contains(bodyStr, "Sign In") && strings.Contains(bodyStr, "Sign in to Airbrush") {
		return true, "html_sniff:login_page_heuristic"
	}
	return false, ""
}

func handleLogoutDetected(cfg Config, reason string, acc ToolAccount, username, sessionToken string, skipAccountSwitch bool) bool {
	log.Printf("[LOGOUT] ✅ Detected | user=%s account=%s (ID:%d) reason=%s", username, acc.Name, acc.ID, reason)

	triggered := false
	if cfg.Automation.Enabled {
		triggered = tryTriggerAutomationForAccount(cfg, reason, acc, username)
	}

	if !skipAccountSwitch && sessionToken != "" && (dbConnected || usesPanelAccountMode(cfg)) {
		if nextAcc, err := switchToNextAccount(sessionToken, acc.ID, acc.Name, username, "logout: "+reason); err == nil {
			log.Printf("[LOGOUT] 🔄 Next account for session: '%s' (ID:%d)", nextAcc.Name, nextAcc.ID)
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

// ── FILE-BASED PER-ACCOUNT AUTOMATION COOLDOWN ────────────────────────────────

type persistedAutomationState struct {
	Accounts map[string]string `json:"accounts"`
}

func readAutomationStateFile() persistedAutomationState {
	automationStateMu.Lock()
	defer automationStateMu.Unlock()
	data, err := os.ReadFile(AUTOMATION_STATE_FILE)
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
	_ = os.WriteFile(AUTOMATION_STATE_FILE, out, 0644)
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
		log.Printf("[AUTOMATION] ⏭️ Cooldown active for account '%s' (ID:%d) — max 1 trigger per %ds",
			acc.Name, acc.ID, cooldown)
		return false
	}

	taskUID := getAccountTaskUID(acc, cfg)
	if taskUID == "" {
		log.Printf("[AUTOMATION] ⏭️ No task_uid for account '%s' (ID:%d)", acc.Name, acc.ID)
		return false
	}

	autoURL, err := normalizeAutomationURL(auto.URL)
	if err != nil {
		log.Printf("[AUTOMATION] ❌ Invalid automation.url: %v", err)
		return false
	}

	markAutomationTriggeredForAccount(acc.ID)
	log.Printf("[AUTOMATION] 🔔 Triggering | account=%s (ID:%d) task_uid=%s reason=%s",
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
			log.Printf("[AUTOMATION] ❌ Call failed for account '%s': %v", acc.Name, err)
			return
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		respStr := string(respBody[:min(len(respBody), 500)])
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			log.Printf("[AUTOMATION] ✅ Success | account=%s task_uid=%s status=%d response=%s",
				acc.Name, taskUID, resp.StatusCode, respStr)
			return
		}
		log.Printf("[AUTOMATION] ❌ Failed | account=%s status=%d response=%s", acc.Name, resp.StatusCode, respStr)
		if strings.Contains(respStr, "task not found") {
			log.Printf("[AUTOMATION] 💡 Hint: task_uid '%s' not found in GoAuto — set automation_task_uid on account ID %d in ctrl panel", taskUID, acc.ID)
		}
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
		return "", fmt.Errorf("must be absolute URL with scheme and host (e.g. https://host/api/tasks/run)")
	}
	path := u.Path
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	u.Path = path
	return u.String(), nil
}

// securityPingHandler — heartbeat for anti-recloud; mirrors on foreign domains cannot satisfy this.
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
	triggered := handleLogoutDetected(cfg, payload.Reason, acc, username, sessionToken, false)

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
	writeLightCard(w, http.StatusServiceUnavailable, lightCard{
		Title:   "ChatGPT cookies expired",
		Heading: "ChatGPT cookies expired",
		Message: "Every mapped ChatGPT account cookie is logged out. Open Panel → Accounts → paste fresh ChatGPT cookies for this site, then open a <b>new</b> access link.",
		Footer:  "Proxy is fine — account session needs refresh",
	})
}

func renderLimitReachedPage(w http.ResponseWriter, limitType string, cfg Config) {
	name := html.EscapeString(toolDisplayName(cfg))
	creditLabel := cfg.CreditLabel
	if creditLabel == "" {
		creditLabel = "Credits"
	}
	title := "Daily Limit Reached"
	message := "You have used the daily " + html.EscapeString(creditLabel) + " limit for <span class=\"brand\">" + name + "</span>. The limit resets at midnight (12:00 AM IST)."
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

// ── PROXY SYSTEM (Database with Local fallback) ──────────────────────────────

// parseProxyString converts any proxy string format to *url.URL.
// Supported formats:
//
//	socks5://user:pass@host:port
//	http://user:pass@host:port  (or https://)
//	host:port:user:pass         (shorthand, assumed SOCKS5)
//	host:port                   (no auth, assumed SOCKS5)
func parseProxyString(s string) *url.URL {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	// Already has a scheme
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

	// Shorthand: host:port:user:pass  OR  host:port
	parts := strings.SplitN(s, ":", 4)
	switch len(parts) {
	case 2: // host:port — no auth
		raw := fmt.Sprintf("socks5://%s:%s", parts[0], parts[1])
		u, _ := url.Parse(raw)
		return u
	case 4: // host:port:user:pass
		raw := fmt.Sprintf("socks5://%s:%s@%s:%s",
			url.QueryEscape(parts[2]),
			url.QueryEscape(parts[3]),
			parts[0], parts[1])
		u, _ := url.Parse(raw)
		return u
	}
	log.Printf("[PROXY] Unrecognized proxy format '%s' — skipping", s)
	return nil
}

var (
	currentProxy    *url.URL
	currentProxyStr string
	proxyMu         sync.RWMutex
)

func loadProxyFromDB() *url.URL {
	if !dbConnected || db == nil {
		return nil
	}
	var proxyStr string
	err := db.QueryRow("SELECT COALESCE(proxy, '') FROM ahrefs_websites WHERE id = ?", currentWebsiteID).Scan(&proxyStr)
	if err != nil || proxyStr == "" {
		proxyMu.Lock()
		currentProxy = nil
		currentProxyStr = ""
		proxyMu.Unlock()
		return nil
	}

	proxyMu.RLock()
	cached := currentProxyStr
	proxyMu.RUnlock()
	if cached == proxyStr && currentProxy != nil {
		return currentProxy
	}

	parsed := parseProxyString(proxyStr) // handles all formats
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

func getProxy() *url.URL { return loadProxyFromDB() }

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

func dialTCPPreferIPv4(ctx context.Context, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	// Try IPv4 first to avoid flaky IPv6 paths to cdn.oaistatic.com.
	if ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host); err == nil && len(ips) > 0 {
		d := &net.Dialer{Timeout: 20 * time.Second}
		if c, err := d.DialContext(ctx, "tcp4", net.JoinHostPort(ips[0].String(), port)); err == nil {
			return c, nil
		}
	}
	return (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, "tcp", addr)
}

func dialChrome(ctx context.Context, addr string) (*uTLSConn, error) {
	return dialChromeALPN(ctx, addr, nil)
}

func dialChromeHTTP1(ctx context.Context, addr string) (*uTLSConn, error) {
	return dialChromeALPN(ctx, addr, []string{"http/1.1"})
}

func dialChromeALPN(ctx context.Context, addr string, nextProtos []string) (*uTLSConn, error) {
	host, _, _ := net.SplitHostPort(addr)
	var tcpConn net.Conn
	var err error
	var px *url.URL
	if ctxPx, ok := ctx.Value(proxyContextKey).(string); ok && ctxPx != "" {
		px = parseProxyString(ctxPx)
	}
	if px == nil {
		px = getProxy()
	}
	if px != nil {
		tcpConn, err = dialThroughProxy(ctx, addr, px)
		if err != nil {
			return nil, fmt.Errorf("proxy dial %s: %w", px.Host, err)
		}
	} else {
		tcpConn, err = dialTCPPreferIPv4(ctx, addr)
		if err != nil {
			return nil, fmt.Errorf("TCP dial: %w", err)
		}
	}
	tlsCfg := &utls.Config{ServerName: host, InsecureSkipVerify: false}
	if len(nextProtos) > 0 {
		tlsCfg.NextProtos = nextProtos
	}
	uConn := utls.UClient(tcpConn, tlsCfg, utls.HelloChrome_Auto)
	if len(nextProtos) > 0 {
		if err := uConn.BuildHandshakeState(); err != nil {
			tcpConn.Close()
			return nil, fmt.Errorf("uTLS handshake state: %w", err)
		}
		for _, ext := range uConn.Extensions {
			alpn, ok := ext.(*utls.ALPNExtension)
			if ok {
				alpn.AlpnProtocols = nextProtos
			}
		}
	}
	if err := uConn.HandshakeContext(ctx); err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("uTLS handshake: %w", err)
	}
	return &uTLSConn{uConn}, nil
}

type roundTripper struct {
	h2      *http2.Transport
	h1      *http.Transport
	mu      sync.Mutex
	h2cc    map[string]*http2.ClientConn
	dialing map[string]*h2Dial
}

type h2Dial struct {
	done chan struct{}
	cc   *http2.ClientConn
	err  error
}

func upstreamConnKey(ctx context.Context, addr string) string {
	px, _ := ctx.Value(proxyContextKey).(string)
	return addr + "\n" + strings.TrimSpace(px)
}

func (rt *roundTripper) sharedH2(key string) *http2.ClientConn {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	cc := rt.h2cc[key]
	if cc != nil && cc.CanTakeNewRequest() {
		return cc
	}
	return nil
}

func (rt *roundTripper) rememberH2(key string, cc *http2.ClientConn) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.h2cc == nil {
		rt.h2cc = map[string]*http2.ClientConn{}
	}
	rt.h2cc[key] = cc
}

func primeUpstream(cfg Config) {
	if usesPanelAccountMode(cfg) {
		return
	}
	target, err := url.Parse(cfg.TargetURL)
	if err != nil || target.Host == "" {
		return
	}
	req, err := http.NewRequest(http.MethodHead, target.Scheme+"://"+target.Host+"/", nil)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[PROXY] warmup skipped: %v", err)
		return
	}
	resp.Body.Close()
	log.Printf("[PROXY] warmup ready %s", target.Host)
}

func (rt *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	addr := req.URL.Host
	if !strings.Contains(addr, ":") {
		if req.URL.Scheme == "https" {
			addr += ":443"
		} else {
			addr += ":80"
		}
	}
	// Probe ALPN only — do NOT call http2.Transport.NewClientConn (panics on
	// golang.org/x/net@v0.55+ when wrap t1 is nil). Same pattern as Ahrefs.
	conn, err := dialChrome(req.Context(), addr)
	if err != nil {
		return nil, err
	}
	proto := conn.ConnectionState().NegotiatedProtocol
	_ = conn.Close()
	if proto == "h2" {
		return rt.h2.RoundTrip(req)
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
		Timeout:       0,
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
	}
}

var httpClient = buildChromeHTTPClient()

const proxyBuildTag = "chatgpt-v24-probe-waf"

// ── CLOUDFLARE BYPASS (challenge scripts break on proxy hostname) ─────────────

func cfChallengeBypassJS() string {
	return `/* ToolsMandi CF challenge bypass */
(function(){try{
  window._cf_chl_opt=window._cf_chl_opt||{};
  if(typeof window.__cfBeacon!=='object'){window.__cfBeacon={u:function(){}};}
}catch(e){}})();`
}

func cfTurnstileBypassJS() string {
	return `(function(){
  window.turnstile={
    render:function(el,p){if(p&&p.callback)setTimeout(function(){p.callback('proxy-bypass');},50);return'mock';},
    reset:function(){},remove:function(){},
    getResponse:function(){return'proxy-bypass';},
    isExpired:function(){return false;},
    execute:function(el,p){if(p&&p.callback)setTimeout(function(){p.callback('proxy-bypass');},50);}
  };
})();`
}

func serveCloudflareBypass(w http.ResponseWriter, path string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	if strings.Contains(path, "turnstile") || path == "/cf-turnstile-bypass.js" {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		fmt.Fprint(w, cfTurnstileBypassJS())
		return
	}
	if strings.Contains(path, "/oneshot/") || strings.HasSuffix(path, ".json") {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprint(w, `{"success":true}`)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	fmt.Fprint(w, cfChallengeBypassJS())
}

func stripCloudflareChallengeHTML(body []byte, publicScheme, publicHost string) []byte {
	if len(body) == 0 {
		return body
	}
	publicBase := publicScheme + "://" + publicHost
	s := string(body)
	reScript := regexp.MustCompile(`(?is)<script[^>]*\bsrc=["'][^"']*cdn-cgi/challenge-platform[^"']*["'][^>]*>\s*</script>`)
	s = reScript.ReplaceAllString(s, "")
	reIframeCF := regexp.MustCompile(`(?is)<script>\(function\(\)\{function c\(\)\{var b=a\.contentDocument\|\|a\.contentWindow\.document;if\(b\)\{[^<]*cdn-cgi/challenge-platform[^<]*</script>`)
	s = reIframeCF.ReplaceAllString(s, "")
	reInlineCF := regexp.MustCompile(`(?is)window\.__CF\$cv\$params=\{[^}]*\};var a=document\.createElement\('script'\);a\.src='/cdn-cgi/challenge-platform[^;]*;document\.getElementsByTagName\('head'\)\[0\]\.appendChild\(a\);`)
	s = reInlineCF.ReplaceAllString(s, "/*cf-bypass*/")
	reTurnstile := regexp.MustCompile(`(?is)<script[^>]*\bsrc=["'][^"']*challenges\.cloudflare\.com[^"']*["'][^>]*>\s*</script>`)
	s = reTurnstile.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "https://challenges.cloudflare.com/turnstile/v0/api.js", publicBase+"/cf-turnstile-bypass.js")
	s = strings.ReplaceAll(s, "//challenges.cloudflare.com/turnstile/v0/api.js", publicBase+"/cf-turnstile-bypass.js")
	return []byte(s)
}

// ── URL REWRITING HELPERS ─────────────────────────────────────────────────────

// buildDomainReplacements creates a list of old→new domain pairs for HTML rewriting.
// Longer/more-specific hostnames (e.g. ws.chatgpt.com) must be replaced before the
// root domain (chatgpt.com) to avoid corrupting subdomains.
func buildDomainReplacements(publicScheme, publicHost string, cfg Config) [][2]string {
	targetParsed, _ := url.Parse(cfg.TargetURL)
	cdnParsed, _ := url.Parse(cfg.CDNURL)
	publicBase := fmt.Sprintf("%s://%s", publicScheme, publicHost)
	var pairs [][2]string
	addURL := func(oldHost, newBase string) {
		// HTTPS public host must emit wss:// (browser blocks ws:// on HTTPS pages)
		wsBase := strings.Replace(newBase, "https://", "wss://", 1)
		wsBase = strings.Replace(wsBase, "http://", "ws://", 1)
		pairs = append(pairs,
			[2]string{"https://" + oldHost, newBase},
			[2]string{"http://" + oldHost, newBase},
			[2]string{"wss://" + oldHost, wsBase},
			[2]string{"ws://" + oldHost, wsBase},
		)
	}
	if targetParsed != nil {
		addURL(targetParsed.Host, publicBase)
	}
	if cdnParsed != nil {
		addURL(cdnParsed.Host, publicBase+"/cdn-proxy")
	}
	for i, extra := range cfg.ExtraCDNDomains {
		extra = strings.TrimPrefix(strings.TrimPrefix(extra, "https://"), "http://")
		extra = strings.Split(extra, "/")[0]
		addURL(extra, fmt.Sprintf("%s/extra-cdn-%d", publicBase, i))
	}
	if targetParsed != nil {
		pairs = append(pairs, [2]string{targetParsed.Host, publicHost})
		newHost := publicHost
		if idx := strings.Index(publicHost, "/"); idx >= 0 {
			newHost = publicHost[:idx]
		}
		pairs = append(pairs, [2]string{"//" + targetParsed.Host, "//" + newHost})
	}
	return pairs
}

func rewriteHTMLBaseTag(body []byte, publicScheme, publicHost string) []byte {
	publicBase := fmt.Sprintf("%s://%s/", publicScheme, publicHost)
	re := regexp.MustCompile(`(?i)<base\s+[^>]*href\s*=\s*["'][^"']*["'][^>]*>`)
	if re.Match(body) {
		return re.ReplaceAll(body, []byte(`<base href="`+publicBase+`">`))
	}
	return body
}

func rewriteBody(body []byte, pairs [][2]string) []byte {
	// Apply longest source strings first to avoid partial subdomain corruption.
	sorted := make([][2]string, len(pairs))
	copy(sorted, pairs)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if len(sorted[j][0]) > len(sorted[i][0]) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	for _, pair := range sorted {
		body = bytes.ReplaceAll(body, []byte(pair[0]), []byte(pair[1]))
	}
	return body
}

func rewriteBodyWithWildcards(body []byte, publicScheme, publicHost string, cfg Config) []byte {
	if !needsBodyRewrite(body, cfg) {
		return body
	}
	pairs := buildDomainReplacements(publicScheme, publicHost, cfg)
	body = rewriteBody(body, pairs)
	return rewriteExtraCDNWildcards(body, publicScheme, publicHost)
}

// ── CLIENT-SIDE PATCHER SCRIPT ────────────────────────────────────────────────

func applyServerThemeClass(body []byte) []byte {
	dark := bytes.Contains(body, []byte(`"theme":"dark"`)) || bytes.Contains(body, []byte(`"theme": "dark"`))
	light := bytes.Contains(body, []byte(`"theme":"light"`)) || bytes.Contains(body, []byte(`"theme": "light"`))
	mode := ""
	if dark {
		mode = "dark"
	} else if light {
		mode = "light"
	}
	if mode == "" {
		return body
	}
	re := regexp.MustCompile(`(?i)<html\b[^>]*>`)
	loc := re.FindIndex(body)
	if loc == nil {
		return body
	}
	tag := body[loc[0]:loc[1]]
	lower := bytes.ToLower(tag)
	if bytes.Contains(lower, []byte(mode)) {
		return body
	}
	var newTag []byte
	if i := bytes.Index(lower, []byte(`class="`)); i >= 0 {
		newTag = make([]byte, 0, len(tag)+len(mode)+1)
		newTag = append(newTag, tag[:i+7]...)
		newTag = append(newTag, mode...)
		newTag = append(newTag, ' ')
		newTag = append(newTag, tag[i+7:]...)
	} else {
		newTag = bytes.Replace(tag, []byte("<html"), []byte(`<html class="`+mode+`"`), 1)
		if bytes.Equal(newTag, tag) {
			newTag = append([]byte(`<html class="`+mode+`"`), tag[5:]...)
		}
	}
	out := make([]byte, 0, len(body)+len(newTag)-len(tag))
	out = append(out, body[:loc[0]]...)
	out = append(out, newTag...)
	out = append(out, body[loc[1]:]...)
	return out
}

func patcherHeadScript(cfg Config) string {
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
	var extraCDNJS strings.Builder
	extraCDNJS.WriteString("[")
	for i, extra := range cfg.ExtraCDNDomains {
		extraClean := strings.TrimPrefix(strings.TrimPrefix(extra, "https://"), "http://")
		extraClean = strings.Split(extraClean, "/")[0]
		proxyPath := fmt.Sprintf("/extra-cdn-%d", i)
		extraCDNJS.WriteString(fmt.Sprintf(`["https://%s",%q],["http://%s",%q],`, extraClean, proxyPath, extraClean, proxyPath))
	}
	extraCDNJS.WriteString("]")

	return fmt.Sprintf(`<script data-tm-proxy-patcher="1">
(function() {
    var T = '%s', C = '%s', O = window.location.origin;
    var EXTRA = %s;
    function hostOnly(o) {
        if (o.indexOf('https://') === 0) return o.slice(8);
        if (o.indexOf('http://') === 0) return o.slice(7);
        return o;
    }
    function patchURL(u) {
        if (typeof u !== 'string') return u;
        if (u.charAt(0) === '/' && u.charAt(1) !== '/') return O + u;
        u = u.replace(/https?:\/\/([a-z0-9.-]+\.api\.openai\.com)/gi, function(m, h) {
            return O + '/extra-cdn-wild/' + h.toLowerCase();
        });
        u = u.replace(/https?:\/\/([a-z0-9-]+)\.(oaiusercontent\.com)/gi, function(m, sub, dom) {
            var h = sub + '.' + dom.toLowerCase();
            if (h === 'files.oaiusercontent.com' || h === 'oaiusercontent.com') return m;
            return O + '/extra-cdn-wild/' + h;
        });
        u = u.replace('https://'+T, O).replace('http://'+T, O).replace('//'+T, '//'+hostOnly(O));
        if (C) {
            u = u.replace('https://'+C, O+'/cdn-proxy').replace('http://'+C, O+'/cdn-proxy').replace('//'+C, '//'+hostOnly(O)+'/cdn-proxy');
        }
        for (var e=0; e<EXTRA.length; e++) {
            u = u.replace(EXTRA[e][0], O+EXTRA[e][1]);
            u = u.replace(EXTRA[e][0].replace('https://','//'), '//'+hostOnly(O)+EXTRA[e][1]);
        }
        return u;
    }
    var OrigRequest = window.Request;
    window.Request = function(input, init) {
        if (typeof input === 'string') input = patchURL(input);
        return init === undefined ? new OrigRequest(input) : new OrigRequest(input, init);
    };
    window.Request.prototype = OrigRequest.prototype;
    var xo = XMLHttpRequest.prototype.open;
    XMLHttpRequest.prototype.open = function(m, u) {
        return xo.apply(this, [m, patchURL(u)].concat(Array.prototype.slice.call(arguments, 2)));
    };
    var fo = window.fetch;
    function tmDevHeaders(input, init) {
        var url = typeof input === "string" ? input : (input && input.url) || "";
        var same = false;
        try { same = new URL(url, location.href).origin === location.origin; } catch (e) {}
        if (!same) return null;
        var fp = "", proof = "";
        try { fp = sessionStorage.getItem("tm_device_fp") || localStorage.getItem("tm_device_fp") || ""; } catch (e) {}
        try { proof = localStorage.getItem("tm_device_proof") || ""; } catch (e) {}
        if (!fp || !proof) return null;
        init = init ? Object.assign({}, init) : {};
        var headers = new Headers(init.headers || (input && input.headers) || undefined);
        if (!headers.get("X-Device-Fp")) headers.set("X-Device-Fp", fp);
        if (!headers.get("X-Device-Proof")) headers.set("X-Device-Proof", proof);
        init.headers = headers;
        return init;
    }
    window.fetch = function(inp, init) {
        var dev = tmDevHeaders(inp, init);
        if (dev) init = dev;
        if (typeof inp === 'string') {
            var su = patchURL(inp);
            return su === inp ? fo(inp, init) : fo(su, init);
        }
        if (inp instanceof Request) {
            var ru = patchURL(inp.url);
            if (ru === inp.url) return fo(inp, init);
            // Never use new Request(ru, inp) — it breaks streaming POST bodies (conversation SSE).
            var opts = init ? Object.assign({}, init) : {};
            if (!init) {
                opts.method = inp.method;
                opts.headers = inp.headers;
                opts.body = inp.body;
                opts.mode = inp.mode;
                opts.credentials = inp.credentials;
                opts.cache = inp.cache;
                opts.redirect = inp.redirect;
                opts.referrer = inp.referrer;
                opts.referrerPolicy = inp.referrerPolicy;
                opts.integrity = inp.integrity;
                opts.keepalive = inp.keepalive;
                if (inp.signal) opts.signal = inp.signal;
            } else if (init.headers && inp.headers) {
                var merged = new Headers(inp.headers);
                new Headers(init.headers).forEach(function(v, k) { merged.set(k, v); });
                opts.headers = merged;
            }
            if (opts.body != null && opts.duplex == null) opts.duplex = 'half';
            return fo(ru, opts);
        }
        return fo(inp, init);
    };
    if (navigator.sendBeacon) {
        var ob = navigator.sendBeacon.bind(navigator);
        navigator.sendBeacon = function(url, data) {
            if (typeof url === 'string') url = patchURL(url);
            return ob(url, data);
        };
    }
    function patchSrcValue(v) {
        if (typeof v !== 'string') return v;
        if (v.indexOf(',') !== -1 && v.indexOf(' ') !== -1) {
            return v.split(',').map(function(part) {
                var bits = part.trim().split(/\s+/);
                bits[0] = patchURL(bits[0]);
                return bits.join(' ');
            }).join(', ');
        }
        return patchURL(v);
    }
    try {
        var srcDesc = Object.getOwnPropertyDescriptor(HTMLImageElement.prototype, 'src');
        if (srcDesc && srcDesc.set) {
            Object.defineProperty(HTMLImageElement.prototype, 'src', {
                configurable: true,
                enumerable: srcDesc.enumerable,
                get: srcDesc.get,
                set: function(v) { srcDesc.set.call(this, patchSrcValue(v)); }
            });
        }
    } catch (e) {}
    var origSetAttr = Element.prototype.setAttribute;
    Element.prototype.setAttribute = function(name, value) {
        if ((name === 'src' || name === 'srcset') && typeof value === 'string') value = patchSrcValue(value);
        return origSetAttr.call(this, name, value);
    };
    var OrigWebSocket = window.WebSocket;
    function patchWsURL(u) {
        if (typeof u !== 'string') return u;
        var isWss = u.indexOf('wss://') === 0;
        var isWs  = u.indexOf('ws://') === 0;
        if (!isWss && !isWs) return u;
        var asHttp = u.replace(/^wss:\/\//, 'https://').replace(/^ws:\/\//, 'http://');
        var patched = patchURL(asHttp);
        if (patched.indexOf('%%25') !== -1) patched = patched.replace(/%%25/g, '%%');
        var out = patched.replace(/^https:\/\//, 'wss://').replace(/^http:\/\//, 'ws://');
        if (window.location.protocol === 'https:' && out.indexOf('ws://') === 0) {
            out = 'wss:' + out.slice(3);
        }
        return out;
    }
    window.WebSocket = function(url, protocols) {
        if (typeof url === 'string') url = patchWsURL(url);
        if (protocols !== undefined) return new OrigWebSocket(url, protocols);
        return new OrigWebSocket(url);
    };
    window.WebSocket.prototype = OrigWebSocket.prototype;
    window.WebSocket.CONNECTING = OrigWebSocket.CONNECTING;
    window.WebSocket.OPEN = OrigWebSocket.OPEN;
    window.WebSocket.CLOSING = OrigWebSocket.CLOSING;
    window.WebSocket.CLOSED = OrigWebSocket.CLOSED;
    if (window.EventSource) {
        var OrigES = window.EventSource;
        window.EventSource = function(url, opts) {
            if (typeof url === 'string') {
                var eu = patchURL(url);
                return eu === url ? new OrigES(url, opts) : new OrigES(eu, opts);
            }
            return new OrigES(url, opts);
        };
        window.EventSource.prototype = OrigES.prototype;
    }
})();
</script>`, targetHost, cdnHost, extraCDNJS.String())
}

func patcherBodyScript(cfg Config) string {
	targetParsed, _ := url.Parse(cfg.TargetURL)
	targetHost := ""
	if targetParsed != nil {
		targetHost = targetParsed.Host
	}
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
	logoutAutoJS := buildLogoutAutomationJS(cfg)

	return fmt.Sprintf(`<script data-tm-proxy-aux="1">
(function() {
    var T = '%s', O = window.location.origin, HOME = '%s', BLOCKED = [%s];
    function rewriteLinks() {
        document.querySelectorAll('a[href*="'+T+'"]').forEach(function(a) {
            var h = a.href.replace('https://'+T, O).replace('http://'+T, O);
            if (a.href !== h) a.href = h;
        });
    }
    function tmStartAux() {
        rewriteLinks();
        setInterval(rewriteLinks, 5000);
    }
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', tmStartAux, { once: true });
    } else {
        tmStartAux();
    }
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
                try { var p = new URL(url, O); if (isBlocked(p.pathname)) { window.location.href = HOME; return; } } catch(e) {}
            }
            return _push(state, title, url);
        };
        var _replace = history.replaceState.bind(history);
        history.replaceState = function(state, title, url) {
            if (typeof url === 'string') {
                try { var p = new URL(url, O); if (isBlocked(p.pathname)) { window.location.href = HOME; return; } } catch(e) {}
            }
            return _replace(state, title, url);
        };
    }
    %s
    %s
})();
</script>`, targetHost, homePath, blockedListJS.String(), buildProfileOverrideJS(cfg), logoutAutoJS)
}

func patcherScript(cfg Config) string {
	return patcherHeadScript(cfg) + patcherBodyScript(cfg)
}

func buildProfileOverrideJS(cfg Config) string {
	toolLabel := cfg.ToolName
	if toolLabel == "" {
		toolLabel = "Member"
	}
	return fmt.Sprintf(`
    // ── Sidebar profile: show proxy member username, block OpenAI account menu ──
    (function(){
      var TOOL_LABEL = %q;
      var tmUser = null;
      var styleId = 'tm-profile-override-style';

      function tmInitials(name) {
        if (!name) return '?';
        var clean = name.replace(/[^a-zA-Z0-9._@-]/g, ' ').trim();
        var parts = clean.split(/\s+/).filter(Boolean);
        if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase();
        if (clean.length >= 2) return clean.substring(0, 2).toUpperCase();
        return clean.substring(0, 1).toUpperCase() || '?';
      }

      function tmFetchUser() {
        if (tmUser) return Promise.resolve(tmUser);
        return fetch('/api/user-limits', { credentials: 'include', cache: 'no-store' })
          .then(function(r) { return r.ok ? r.json() : null; })
          .then(function(d) {
            if (d && d.username) tmUser = d.username;
            return tmUser;
          })
          .catch(function() { return null; });
      }

      function tmInjectStyle() {
        if (document.getElementById(styleId)) return;
        var st = document.createElement('style');
        st.id = styleId;
        st.textContent = [
          '[data-testid="accounts-profile-button"]{cursor:default!important;pointer-events:none!important}',
          '[data-testid="accounts-profile-button"] .trailing{display:none!important}',
          '[data-testid="accounts-profile-button"] img{display:none!important}',
          '.tm-proxy-user-initials{display:flex!important;align-items:center!important;justify-content:center!important;height:24px!important;width:24px!important;border-radius:9999px!important;background:rgba(100,116,139,.35)!important;font-size:11px!important;font-weight:600!important;color:#fff!important;text-transform:uppercase!important}'
        ].join('');
        (document.head || document.documentElement).appendChild(st);
      }

      function tmProfileTrigger(node) {
        if (!node || !node.closest) return null;
        return node.closest('button[aria-label="Open profile menu"], [data-testid="accounts-profile-button"]');
      }

      function tmCloseProfileMenus() {
        document.querySelectorAll('button[aria-label="Open profile menu"], [data-testid="accounts-profile-button"]').forEach(function(btn) {
          var id = btn.getAttribute('aria-controls');
          btn.setAttribute('aria-expanded', 'false');
          btn.setAttribute('data-state', 'closed');
          btn.removeAttribute('aria-controls');
          var menu = id ? document.getElementById(id) : null;
          if (menu) {
            var wrap = menu.closest('[data-radix-popper-content-wrapper]') || menu;
            if (wrap.parentElement) wrap.parentElement.removeChild(wrap);
          }
        });
        document.querySelectorAll('[role="menu"]').forEach(function(menu) {
          var txt = (menu.textContent || '').toLowerCase();
          if (txt.indexOf('log out') === -1 && txt.indexOf('settings') === -1 && txt.indexOf('upgrade') === -1) return;
          var wrap = menu.closest('[data-radix-popper-content-wrapper]') || menu;
          if (wrap.parentElement) wrap.parentElement.removeChild(wrap);
        });
      }

      function tmReplaceMemberName() {
        if (!tmUser) return;
        var plans = { go: 1, plus: 1, pro: 1, free: 1, team: 1, business: 1, enterprise: 1 };
        document.querySelectorAll('button[aria-label="Open profile menu"], [data-testid="accounts-profile-button"]').forEach(function(btn) {
          var roots = [btn];
          if (btn.parentElement) roots.push(btn.parentElement);
          roots.forEach(function(root) {
            root.querySelectorAll('span').forEach(function(el) {
              if (el.children.length) return;
              var text = (el.textContent || '').trim();
              if (!text || text === tmUser || text === 'Loading profile' || plans[text.toLowerCase()] || text.length > 80) return;
              if (String(el.className || '').indexOf('truncate') === -1) return;
              el.textContent = tmUser;
            });
          });
        });
      }

      function tmPatchProfile() {
        if (!tmUser) return;
        tmReplaceMemberName();
        var btn = document.querySelector('[data-testid="accounts-profile-button"]');
        if (!btn) return;
        tmCloseProfileMenus();
        btn.setAttribute('aria-expanded', 'false');
        btn.setAttribute('data-state', 'closed');
        btn.setAttribute('aria-label', tmUser + ', member account');
        btn.setAttribute('tabindex', '-1');
        var nameEl = btn.querySelector('.truncate');
        if (nameEl) nameEl.textContent = tmUser;
        var sub = btn.querySelector('.text-caption-regular span, .text-caption-regular');
        if (sub) sub.textContent = TOOL_LABEL;
        var avatarWrap = btn.querySelector('.rounded-full');
        if (avatarWrap) {
          var ini = avatarWrap.querySelector('.tm-proxy-user-initials');
          if (!ini) {
            ini = document.createElement('span');
            ini.className = 'tm-proxy-user-initials';
            avatarWrap.appendChild(ini);
          }
          ini.textContent = tmInitials(tmUser);
        }
      }

      function tmBlockProfileInteraction(e) {
        if (!tmProfileTrigger(e.target)) return;
        e.preventDefault();
        e.stopPropagation();
        e.stopImmediatePropagation();
        tmCloseProfileMenus();
      }

      function tmStartProfileOverride() {
        tmInjectStyle();
        tmFetchUser().then(function() {
          tmReplaceMemberName();
          tmPatchProfile();
          setInterval(tmReplaceMemberName, 400);
          setInterval(tmPatchProfile, 5000);
        });
        var btnOnlyEvents = ['pointerdown', 'mousedown', 'click', 'pointerup', 'keydown'];
        btnOnlyEvents.forEach(function(ev) {
          document.addEventListener(ev, tmBlockProfileInteraction, true);
        });
      }

      tmStartProfileOverride();
    })();
`, toolLabel)
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
		msg = "Account logout detected. Switching to next account, please wait..."
	}
	refreshSec := ld.RefreshSeconds
	if refreshSec <= 0 {
		refreshSec = 5
	}
	urlPathsJSON, _ := json.Marshal(ld.URLPaths)
	allSniffs := mergeWatchdogTriggers(cfg)
	textSniffsJSON, _ := json.Marshal(allSniffs)
	htmlSniffsJSON, _ := json.Marshal(ld.HTMLSniffs)
	homePath := cfg.HomePath
	if homePath == "" {
		homePath = "/"
	}
	return fmt.Sprintf(`
    // ── Logout detection → rotate DB account → reload (loop until valid session) ──
    var AUTO_TRIGGER_ENABLED = %v;
    var SHOW_LOGOUT_OVERLAY = %v;
    var REFRESH_MS = %d;
    var OVERLAY_TITLE = %q;
    var OVERLAY_MSG = %q;
    var TM_HOME = %q;
    var TM_MAX_FAILOVER = %d;
    var TM_FAILOVER_KEY = 'tm_account_failover_attempts';
    var refreshTimer = null;
    var countdownTimer = null;
    var apiCalledThisPage = false;
    var rotateInProgress = false;

    function showTmLogoutOverlay() {
        if (!SHOW_LOGOUT_OVERLAY || document.getElementById('tm-logout-overlay')) return;
        var o = document.createElement('div');
        o.id = 'tm-logout-overlay';
        o.style.cssText = 'position:fixed;inset:0;background:rgba(15,23,42,0.93);backdrop-filter:blur(10px);z-index:2147483647;display:flex;align-items:center;justify-content:center;font-family:-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;padding:20px;';
        o.innerHTML = '<div style="background:#fff;border-radius:20px;padding:36px 32px;max-width:420px;width:100%%;text-align:center;box-shadow:0 25px 60px rgba(0,0,0,0.35);">' +
            '<div style="width:52px;height:52px;border:4px solid #e2e8f0;border-top-color:#4f46e5;border-radius:50%%;margin:0 auto 20px;animation:tmSpin 0.8s linear infinite;"></div>' +
            '<h2 style="font-size:20px;font-weight:700;color:#0f172a;margin:0 0 10px;">' + OVERLAY_TITLE + '</h2>' +
            '<p style="font-size:15px;color:#64748b;line-height:1.6;margin:0 0 8px;">' + OVERLAY_MSG + '</p>' +
            '<p id="tm-logout-countdown" style="font-size:13px;color:#94a3b8;margin:0;">Switching account...</p></div>';
        if (!document.getElementById('tm-logout-spin-style')) {
            var s = document.createElement('style');
            s.id = 'tm-logout-spin-style';
            s.textContent = '@keyframes tmSpin{to{transform:rotate(360deg)}}';
            document.head.appendChild(s);
        }
        (document.body || document.documentElement).appendChild(o);
    }

    function hideTmLogoutOverlay() {
        var o = document.getElementById('tm-logout-overlay');
        if (o) o.remove();
        if (refreshTimer) { clearTimeout(refreshTimer); refreshTimer = null; }
        if (countdownTimer) { clearInterval(countdownTimer); countdownTimer = null; }
        rotateInProgress = false;
    }

    function tmClearFailoverAttempts() {
        try { sessionStorage.removeItem(TM_FAILOVER_KEY); } catch(e) {}
    }

    function tmGetFailoverAttempts() {
        try { return parseInt(sessionStorage.getItem(TM_FAILOVER_KEY) || '0', 10) || 0; } catch(e) { return 0; }
    }

    function tmDetectLogoutReason() {
        var path = (window.location.pathname || '/').replace(/\/$/, '') || '/';
        var html = document.documentElement ? document.documentElement.innerHTML : '';
        var text = document.body ? (document.body.innerText || document.body.textContent || '') : '';
        var textLower = text.toLowerCase();
        var URL_PATHS = %s;
        var TEXT_SNIFFS = %s;
        var HTML_SNIFFS = %s;
        function tmLooksLoggedIn() {
            return html.indexOf('conversation-small') !== -1 ||
                html.indexOf('oai-client-auth-info') !== -1 ||
                html.indexOf('"connectionType"') !== -1;
        }
        function tmIsStrongSniff(s) {
            return /session has expired|please log in again/i.test(s || '');
        }
        for (var i = 0; i < URL_PATHS.length; i++) {
            var p = (URL_PATHS[i] || '').replace(/\/$/, '');
            if (p && (path === p || path.indexOf(p + '/') === 0)) return 'url_path:' + URL_PATHS[i];
        }
        var loggedIn = tmLooksLoggedIn();
        if (loggedIn) {
            if (textLower.indexOf('session has expired') !== -1) return 'text_sniff:session_expired';
            if (textLower.indexOf('please log in again') !== -1) return 'text_sniff:please_log_in_again';
            return '';
        }
        for (var j = 0; j < TEXT_SNIFFS.length; j++) {
            var sniff = TEXT_SNIFFS[j];
            if (!sniff) continue;
            if (loggedIn && !tmIsStrongSniff(sniff)) continue;
            if (tmIsStrongSniff(sniff)) {
                if (html.indexOf(sniff) !== -1 || text.indexOf(sniff) !== -1) return 'text_sniff:' + sniff;
            } else if (text.indexOf(sniff) !== -1) {
                return 'text_sniff:' + sniff;
            }
        }
        for (var k = 0; k < HTML_SNIFFS.length; k++) {
            if (HTML_SNIFFS[k] && html.indexOf(HTML_SNIFFS[k]) !== -1) return 'html_sniff:' + HTML_SNIFFS[k].substring(0, 40);
        }
        if (textLower.indexOf('session has expired') !== -1) return 'text_sniff:session_expired';
        if (textLower.indexOf('please log in again') !== -1) return 'text_sniff:please_log_in_again';
        return '';
    }

    function tmRotateAccountAndReload(reason) {
        if (rotateInProgress) return;
        var attempts = tmGetFailoverAttempts();
        if (attempts >= TM_MAX_FAILOVER) {
            console.warn('[ToolsMandi] Max account failover attempts reached');
            return;
        }
        rotateInProgress = true;
        try { sessionStorage.setItem(TM_FAILOVER_KEY, String(attempts + 1)); } catch(e) {}
        showTmLogoutOverlay();
        fetch('/api/rotate-session?reason=' + encodeURIComponent(reason), { credentials: 'include', cache: 'no-store' })
        .then(function(r) { return r.json().catch(function() { return {}; }); })
        .then(function(data) {
            if (data && data.error) {
                console.warn('[ToolsMandi] No more accounts:', data.error);
                rotateInProgress = false;
                tmClearFailoverAttempts();
                return;
            }
            setTimeout(function() { window.location.href = TM_HOME; }, 900);
        })
        .catch(function() {
            setTimeout(function() { window.location.reload(); }, 900);
        });
    }

    function tmOnLogoutDetected(reason) {
        console.log('[ToolsMandi] Logout detected:', reason);
        tmRotateAccountAndReload(reason);
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
        var reason = tmDetectLogoutReason();
        if (reason) {
            tmOnLogoutDetected(reason);
        } else {
            hideTmLogoutOverlay();
            tmClearFailoverAttempts();
        }
    }

    window.addEventListener('popstate', function() { setTimeout(checkLogoutAutomation, 1000); });
    if (window.__tm_show_logout_overlay) {
        if (window.__tm_overlay_title) OVERLAY_TITLE = window.__tm_overlay_title;
        if (window.__tm_overlay_msg) OVERLAY_MSG = window.__tm_overlay_msg;
        tmOnLogoutDetected('server_html_logout');
    }
    function tmStartLogoutWatch() {
        setTimeout(checkLogoutAutomation, 8000);
        setInterval(checkLogoutAutomation, 12000);
    }
    if (document.readyState === 'complete') {
        tmStartLogoutWatch();
    } else {
        window.addEventListener('load', tmStartLogoutWatch, { once: true });
    }
`, cfg.Automation.Enabled, ld.ShowOverlay, refreshSec*1000, title, msg, homePath, maxAccountFailoverAttempts,
		string(urlPathsJSON), string(textSniffsJSON), string(htmlSniffsJSON))
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
	return fmt.Sprintf(`<script data-tm-credits="1">
(function() {
    var CREDIT_LABEL = '%s';
    var open = false;
    var latest = null;
    function removeDock() {
        var el = document.getElementById('tm-limit-dock');
        if (el) el.remove();
    }
    function host() {
        if (latest && !latest.show_limit) { removeDock(); return null; }
        var el = document.getElementById('tm-limit-dock');
        if (el) return el;
        el = document.createElement('div');
        el.id = 'tm-limit-dock';
        var root = el.attachShadow({ mode: 'open' });
        root.innerHTML = ''
            + '<style>'
            + ':host{all:initial}'
            + '.tab{position:fixed;left:16px;bottom:24px;z-index:2147483646;width:48px;height:48px;border:0;border-radius:999px;background:#22c55e;color:#fff;display:grid;place-items:center;box-shadow:0 10px 24px rgba(22,163,74,.35);cursor:pointer}'
            + '.tab svg{width:22px;height:22px;display:block}'
            + '.tab.warn{background:#f59e0b}.tab.low{background:#ef4444}.tab.hide{opacity:0;pointer-events:none}'
            + '.back{position:fixed;inset:0;z-index:2147483646;background:rgba(15,23,42,.18);opacity:0;pointer-events:none;transition:opacity .2s}'
            + '.back.show{opacity:1;pointer-events:auto}'
            + '.card{position:fixed;left:16px;bottom:24px;z-index:2147483647;width:232px;background:#f3fbf6;color:#14532d;border-radius:28px;box-shadow:0 22px 50px rgba(15,23,42,.18);padding:16px 16px 14px;box-sizing:border-box;font:500 14px/1.3 system-ui,sans-serif;transform:translateY(10px) scale(.96);opacity:0;pointer-events:none;transition:transform .22s ease,opacity .22s ease}'
            + '.card.show{transform:none;opacity:1;pointer-events:auto}'
            + '.head{display:flex;align-items:center;justify-content:space-between;margin-bottom:6px}'
            + '.title{font-weight:750;font-size:15px}'
            + '.x{border:0;background:#e7f6ec;color:#166534;width:28px;height:28px;border-radius:999px;cursor:pointer;font:700 16px/1 system-ui,sans-serif}'
            + '.ringwrap{position:relative;width:168px;height:168px;margin:4px auto 8px}'
            + '.ring{width:168px;height:168px;transform:rotate(-90deg)}'
            + '.track{fill:none;stroke:#d9f3e3;stroke-width:10}'
            + '.fill{fill:none;stroke:#22c55e;stroke-width:10;stroke-linecap:round;stroke-dasharray:289;stroke-dashoffset:0;transition:stroke-dashoffset .35s ease,stroke .2s}'
            + '.fill.warn{stroke:#f59e0b}.fill.low{stroke:#ef4444}'
            + '.center{position:absolute;inset:0;display:flex;flex-direction:column;align-items:center;justify-content:center}'
            + '.num{font-size:40px;font-weight:800;letter-spacing:-.04em;color:#14532d}'
            + '.sub{margin-top:2px;color:#4d7c5e;font-size:13px}'
            + '.meter{background:#fff;border-radius:16px;padding:10px 12px}'
            + '.meter-top{display:flex;justify-content:space-between;align-items:center;margin-bottom:8px;color:#166534;font-size:13px}'
            + '.count{font-weight:750}'
            + '.bar{height:6px;border-radius:999px;background:#e7f6ec;overflow:hidden}'
            + '.barfill{height:100%%;width:0;border-radius:999px;background:#22c55e;transition:width .35s ease}'
            + '.barfill.warn{background:#f59e0b}.barfill.low{background:#ef4444}'
            + '<' + '/style>'
            + '<button class="tab" id="tm-tab" type="button" aria-label="Credits"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="8" fill="none" stroke="currentColor" stroke-width="2" opacity=".35"><' + '/circle><path d="M12 4a8 8 0 0 1 8 8" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round"><' + '/path><' + '/svg><' + '/button>'
            + '<div class="back" id="tm-back"><' + '/div>'
            + '<aside class="card" id="tm-drawer">'
            + '<div class="head"><div class="title" id="tm-title">Credits<' + '/div><button class="x" id="tm-close" type="button" aria-label="Close">×<' + '/button><' + '/div>'
            + '<div class="ringwrap"><svg class="ring" viewBox="0 0 120 120"><circle class="track" cx="60" cy="60" r="46"><' + '/circle><circle class="fill" id="tm-ring" cx="60" cy="60" r="46"><' + '/circle><' + '/svg>'
            + '<div class="center"><div class="num" id="tm-left">0<' + '/div><div class="sub" id="tm-sub">left<' + '/div><' + '/div><' + '/div>'
            + '<div class="meter"><div class="meter-top"><span id="tm-meter-label">Credits<' + '/span><span class="count" id="tm-count">0 / 0<' + '/span><' + '/div><div class="bar"><div class="barfill" id="tm-bar"><' + '/div><' + '/div><' + '/div>'
            + '<' + '/aside>';
        (document.body || document.documentElement).appendChild(el);
        var shadow = root;
        shadow.getElementById('tm-tab').addEventListener('click', function(ev){ ev.stopPropagation(); setOpen(true); });
        shadow.getElementById('tm-close').addEventListener('click', function(ev){ ev.stopPropagation(); setOpen(false); });
        shadow.getElementById('tm-back').addEventListener('click', function(){ setOpen(false); });
        shadow.getElementById('tm-drawer').addEventListener('click', function(ev){ ev.stopPropagation(); });
        return el;
    }
    function setOpen(next) {
        open = next;
        var el = host();
        if (!el) return;
        var root = el.shadowRoot;
        root.getElementById('tm-drawer').classList.toggle('show', open);
        root.getElementById('tm-back').classList.toggle('show', open);
        root.getElementById('tm-tab').classList.toggle('hide', open);
    }
    function paint(d) {
        if (!d || !d.show_limit) { removeDock(); return; }
        var el = host();
        if (!el) return;
        var root = el.shadowRoot;
        root.getElementById('tm-title').textContent = CREDIT_LABEL;
        root.getElementById('tm-meter-label').textContent = CREDIT_LABEL;
        var tab = root.getElementById('tm-tab');
        var ring = root.getElementById('tm-ring');
        var bar = root.getElementById('tm-bar');
        var used = Number(d.credit_used) || 0;
        var limit = Number(d.credit_limit);
        var unlimited = !(limit >= 0);
        var left = unlimited ? used : Math.max(0, limit - used);
        var ratio = unlimited ? 1 : (limit === 0 ? 0 : left / limit);
        root.getElementById('tm-left').textContent = unlimited ? '∞' : String(left);
        root.getElementById('tm-sub').textContent = unlimited ? 'unlimited' : 'left';
        root.getElementById('tm-count').textContent = unlimited ? (String(used) + ' used') : (String(used) + ' / ' + String(limit));
        ring.style.strokeDasharray = '289';
        ring.style.strokeDashoffset = String(289 * (1 - ratio));
        bar.style.width = Math.round(ratio * 100) + '%%';
        var low = !unlimited && left < 3;
        var warn = !unlimited && left >= 3 && left < 10;
        tab.classList.toggle('low', low);
        tab.classList.toggle('warn', warn);
        ring.classList.toggle('low', low);
        ring.classList.toggle('warn', warn);
        bar.classList.toggle('low', low);
        bar.classList.toggle('warn', warn);
    }
    function showLimitCard() {
        if (document.getElementById('tm-limit-screen')) return;
        var el = document.createElement('div');
        el.id = 'tm-limit-screen';
        el.style.cssText = 'position:fixed;inset:0;z-index:2147483647;background:#eef3f8;color:#0f172a;display:flex;align-items:center;justify-content:center;padding:24px;font-family:system-ui,-apple-system,Segoe UI,sans-serif;';
        el.innerHTML = '<div style="width:min(440px,92vw);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px 36px;text-align:center;">'
            + '<div style="width:78px;height:78px;margin:0 auto 22px;border-radius:50%%;background:#fff;display:grid;place-items:center;font-size:28px;border:1px solid #e6ebf2;">&#128274;<' + '/div>'
            + '<h1 style="font-size:28px;line-height:1.2;font-weight:800;letter-spacing:-.03em;margin:0 0 12px;">Daily Limit Reached<' + '/h1>'
            + '<p style="color:#64748b;font-size:15px;line-height:1.55;margin:0;">You have used the daily ' + CREDIT_LABEL + ' limit for ChatGPT. The limit resets at midnight (12:00 AM IST).<' + '/p>'
            + '<p style="margin:22px 0 0;color:#94a3b8;font-size:13px;">This limit applies to the current access<' + '/p><' + '/div>';
        (document.body || document.documentElement).appendChild(el);
    }
    function applyThemeText(text) {
        var found = /"theme"\s*:\s*"(dark|light|system)"/.exec(text || '');
        if (!found) return;
        var mode = found[1];
        if (mode === 'system') {
            mode = (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) ? 'dark' : 'light';
        }
        var root = document.documentElement;
        root.classList.remove('dark');
        root.classList.remove('light');
        root.classList.add(mode);
        root.style.colorScheme = mode;
        try { localStorage.setItem('theme', mode); } catch (e) {}
    }
    function noCreditsLeft() {
        if (!latest || !latest.show_limit) return false;
        var limit = Number(latest.credit_limit);
        if (!(limit >= 0)) return false;
        var used = Number(latest.credit_used) || 0;
        return used >= limit;
    }
    function isChatSend(url) {
        return String(url || '').indexOf('/backend-api/f/conversation') !== -1;
    }
    function blockedChatResponse() {
        showLimitCard();
        return new Response('{"error":{"message":"Daily limit reached","code":"limit_reached"}}', {
            status: 403,
            headers: { 'Content-Type': 'application/json', 'X-TM-Limit': 'credit' }
        });
    }
    function updateBadge() {
        window.fetch('/api/user-limits', { credentials: 'same-origin', cache: 'no-store' })
            .then(function(r){ return r.json(); })
            .then(function(d){ latest = d; paint(d); })
            .catch(function(){});
    }
    window.__tmUpdateCredits = updateBadge;
    function boot() { updateBadge(); }
    if (document.body) boot();
    else document.addEventListener('DOMContentLoaded', boot);
    setInterval(function(){ updateBadge(); }, 4000);
    var orig = window.fetch;
    window.fetch = function(input, init) {
        var url = typeof input === 'string' ? input : (input && input.url) || '';
        var target = String(url);
        if (noCreditsLeft() && isChatSend(target)) return Promise.resolve(blockedChatResponse());
        var done = orig.apply(this, arguments);
        if (target.indexOf('/backend-api/settings/user') !== -1) {
            return done.then(function(res) {
                if (res && res.clone) res.clone().text().then(applyThemeText).catch(function(){});
                return res;
            });
        }
        if (target.indexOf('/backend-api/f/conversation') !== -1 && target.indexOf('prepare') === -1) {
            return done.then(function(res) {
                if (res && res.headers && res.headers.get('X-TM-Limit')) {
                    showLimitCard();
                    return new Response('{"error":"limit_reached"}', { status: 403, headers: { 'Content-Type': 'application/json' } });
                }
                setTimeout(updateBadge, 400);
                return res;
            }).catch(function(err) { throw err; });
        }
        return done;
    };
})();
</script>`, creditLabel)
}

// ── DECOMPRESSION HELPERS ─────────────────────────────────────────────────────

func decompressBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(upstreamBodyReader(resp))
}

func upstreamBodyReader(resp *http.Response) io.Reader {
	switch strings.ToLower(resp.Header.Get("Content-Encoding")) {
	case "gzip":
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return resp.Body
		}
		return gr
	case "br":
		return brotli.NewReader(resp.Body)
	default:
		return resp.Body
	}
}

func streamSSEResponse(w http.ResponseWriter, upstreamResp *http.Response, publicScheme, publicHost string, cfg Config) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(upstreamResp.StatusCode)
	flusher, canFlush := w.(http.Flusher)
	pairs := buildDomainReplacements(publicScheme, publicHost, cfg)
	scanner := bufio.NewScanner(upstreamBodyReader(upstreamResp))
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := rewriteExtraCDNWildcards(rewriteBody(scanner.Bytes(), pairs), publicScheme, publicHost)
		if _, err := w.Write(line); err != nil {
			break
		}
		if _, err := w.Write([]byte("\n")); err != nil {
			break
		}
		if canFlush {
			flusher.Flush()
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("[SSE] stream stopped: %v", err)
	}
}

// ── WEBSOCKET PROXY ───────────────────────────────────────────────────────────

// bufferedConn wraps a net.Conn so already-buffered bytes (from bufio.Reader)
// are replayed before reading from the underlying connection.
type bufferedConn struct {
	net.Conn
	reader io.Reader
}

func (b *bufferedConn) Read(p []byte) (int, error) { return b.reader.Read(p) }

func proxyWebSocket(w http.ResponseWriter, r *http.Request, upstreamURL url.URL, cookieStr string, userAgent string, cfg Config, accountProxy string) {
	upstreamAddr := upstreamURL.Host
	useTLS := upstreamURL.Scheme == "https" || upstreamURL.Scheme == "wss"
	if !strings.Contains(upstreamAddr, ":") {
		if useTLS {
			upstreamAddr += ":443"
		} else {
			upstreamAddr += ":80"
		}
	}

	// Connect to upstream with a Chrome TLS fingerprint. Plain tls.Dial is often
	// closed by Cloudflare, which makes the live socket fail until a refresh.
	var upstreamConn net.Conn
	var err error
	if useTLS {
		ctx, cancel := context.WithTimeout(context.WithValue(r.Context(), proxyContextKey, accountProxy), 20*time.Second)
		upstreamConn, err = dialChromeHTTP1(ctx, upstreamAddr)
		cancel()
	} else {
		upstreamConn, err = net.Dial("tcp", upstreamAddr)
	}
	if err != nil {
		log.Printf("[WS] Upstream dial failed for %s: %v", upstreamAddr, err)
		http.Error(w, "WebSocket upstream unavailable", http.StatusBadGateway)
		return
	}
	defer upstreamConn.Close()

	// The browser encodes an already-encoded verify token a second time (%2B -> %252B).
	// The upstream socket accepts the upgrade, then closes because the signature is wrong.
	if strings.Contains(upstreamURL.RawQuery, "%25") {
		upstreamURL.RawQuery = strings.ReplaceAll(upstreamURL.RawQuery, "%25", "%")
	}

	// Build HTTP/1.1 upgrade request
	var reqBuf bytes.Buffer
	upstreamPath := upstreamURL.RequestURI()
	reqBuf.WriteString(fmt.Sprintf("GET %s HTTP/1.1\r\n", upstreamPath))
	reqBuf.WriteString(fmt.Sprintf("Host: %s\r\n", upstreamURL.Host))
	skipWSHeader := map[string]bool{
		"host": true, "cookie": true, "origin": true, "referer": true,
		"user-agent": true, "content-length": true, "accept-encoding": true,
		"sec-websocket-extensions": true,
	}
	for k, vv := range r.Header {
		if skipWSHeader[strings.ToLower(k)] {
			continue
		}
		for _, v := range vv {
			reqBuf.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
		}
	}
	if cookieStr != "" {
		reqBuf.WriteString(fmt.Sprintf("Cookie: %s\r\n", cookieStr))
	}
	if userAgent != "" {
		reqBuf.WriteString(fmt.Sprintf("User-Agent: %s\r\n", userAgent))
	}
	reqBuf.WriteString(fmt.Sprintf("Origin: %s\r\n", cfg.TargetURL))
	reqBuf.WriteString(fmt.Sprintf("Referer: %s/\r\n", strings.TrimRight(cfg.TargetURL, "/")))
	reqBuf.WriteString("\r\n")

	if _, err = upstreamConn.Write(reqBuf.Bytes()); err != nil {
		log.Printf("[WS] Upstream write failed: %v", err)
		http.Error(w, "WebSocket upstream write failed", http.StatusBadGateway)
		return
	}

	// Read upstream 101 response
	upstreamBR := bufio.NewReader(upstreamConn)
	upstreamResp, err := http.ReadResponse(upstreamBR, nil)
	if err != nil {
		log.Printf("[WS] Upstream response read error: %v", err)
		http.Error(w, "WebSocket upstream response failed", http.StatusBadGateway)
		return
	}
	if upstreamResp.StatusCode != http.StatusSwitchingProtocols {
		log.Printf("[WS] Upstream did not upgrade: %s", upstreamResp.Status)
		http.Error(w, "WebSocket upstream rejected: "+upstreamResp.Status, http.StatusBadGateway)
		return
	}

	// Hijack client connection
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "WebSocket hijack unsupported", http.StatusInternalServerError)
		return
	}
	clientConn, clientBR, err := hj.Hijack()
	if err != nil {
		log.Printf("[WS] Hijack failed: %v", err)
		return
	}
	defer clientConn.Close()
	_ = clientConn.SetDeadline(time.Time{})
	_ = upstreamConn.SetDeadline(time.Time{})

	// Extra upgrade headers, including compression, make the browser reset the socket.
	var respBuf bytes.Buffer
	respBuf.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	respBuf.WriteString("Upgrade: websocket\r\n")
	respBuf.WriteString("Connection: Upgrade\r\n")
	allow101 := map[string]bool{
		"sec-websocket-accept":   true,
		"sec-websocket-protocol": true,
	}
	for k, vv := range upstreamResp.Header {
		if !allow101[strings.ToLower(k)] {
			continue
		}
		for _, v := range vv {
			respBuf.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
		}
	}
	respBuf.WriteString("\r\n")
	if _, err = clientConn.Write(respBuf.Bytes()); err != nil {
		log.Printf("[WS] Client 101 write failed: %v", err)
		return
	}

	// Wrap connections to drain any already-buffered bytes
	var upConn io.ReadWriter = upstreamConn
	if upstreamBR.Buffered() > 0 {
		upConn = &bufferedConn{Conn: upstreamConn, reader: io.MultiReader(upstreamBR, upstreamConn)}
	}
	var clConn io.ReadWriter = clientConn
	if clientBR.Reader.Buffered() > 0 {
		clConn = &bufferedConn{Conn: clientConn, reader: io.MultiReader(clientBR.Reader, clientConn)}
	}

	log.Printf("[WS] ✅ WebSocket proxying: client ↔ %s%s", upstreamURL.Host, upstreamPath)
	started := time.Now()
	errc := make(chan error, 2)
	go func() {
		_, err := io.Copy(upConn, clConn)
		errc <- fmt.Errorf("client side: %w", err)
	}()
	go func() {
		_, err := io.Copy(clConn, upConn)
		errc <- fmt.Errorf("upstream side: %w", err)
	}()
	log.Printf("[WS] WebSocket closed for %s after %s (%v)", upstreamURL.Host, time.Since(started).Round(time.Millisecond), <-errc)
}

// ── MAIN PROXY HANDLER ────────────────────────────────────────────────────────

// isSSERequest returns true if the client expects a streaming SSE response.
func isSSEResponse(contentType string) bool {
	return strings.Contains(contentType, "text/event-stream")
}

func logUsageRequest(r *http.Request, path string) {
	watch := strings.HasPrefix(path, "/backend-api/") ||
		strings.HasPrefix(path, "/backend-anon/") ||
		strings.Contains(path, "/files") ||
		strings.Contains(path, "/conversation") ||
		strings.Contains(path, "/images") ||
		strings.Contains(path, "/upload")
	if r.Method == http.MethodPost || r.Method == http.MethodPut || watch {
		log.Printf("[USE] %s %s bytes=%d", r.Method, path, r.ContentLength)
	}
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	w.Header().Set("X-Proxy-Build", proxyBuildTag)
	if rejectIfIPBlocked(w, r, cfg) {
		return
	}
	path := r.URL.Path

	// The ChatGPT client calls /__codex-api/* when the page origin is not chatgpt.com.
	// Those paths are the same handlers as /backend-api/*; leaving them as-is returns the HTML shell.
	if path == "/__codex-api" || strings.HasPrefix(path, "/__codex-api/") {
		path = "/backend-api" + strings.TrimPrefix(path, "/__codex-api")
		r.URL.Path = path
	}

	if proxyHandlerExtraCDNCORS(w, r, cfg) {
		return
	}

	if cfg.CloudflareBypass {
		if strings.HasPrefix(path, "/cdn-cgi/") || path == "/cf-turnstile-bypass.js" {
			serveCloudflareBypass(w, path)
			return
		}
	}

	// ── 0. Skip proxy for admin API routes ──────────────────────────────────────
	if strings.HasPrefix(path, "/api/auth-handshake") ||
		strings.HasPrefix(path, "/api/user-limits") ||
		strings.HasPrefix(path, "/api/rotate-session") ||
		strings.HasPrefix(path, "/api/trigger-automation") ||
		strings.HasPrefix(path, "/api/security-ping") ||
		strings.HasPrefix(path, "/access") ||
		path == "/user/logout" {
		return // These are handled by their own handlers
	}

	// ── 1. Authenticate user (require ct_session cookie) ─────────────────────────
	currentUser, err := getAuthenticatedUser(r, cfg)
	isFavicon := strings.Contains(strings.ToLower(path), "favicon")
	skipAuth := skipSessionAuthPath(path) || isTelemetryPath(path)
	if err != nil && !isFavicon && !skipAuth {
		log.Printf("[AUTH] Denied path=%s ip=%s host=%s err=%v", path, realClientIP(r), requestHost(r), err)
		denyUser := ""
		hadStaleCookie := false
		if c, cErr := r.Cookie("ct_session"); cErr == nil && c.Value != "" {
			denyUser = "(invalid session)"
			hadStaleCookie = isStaleSessionError(err)
		}
		if denyUser != "" && !isTelemetryPath(path) {
			recordSecurityEvent(r, denyUser, "auth_denied", err.Error())
		}
		acceptHeader := r.Header.Get("Accept")
		isNavigation := strings.Contains(acceptHeader, "text/html")
		if hadStaleCookie {
			expireSessionCookies(w, r, cfg)
			if isNavigation {
				renderAccessDeniedPage(w, cfg)
				return
			}
			applyProxyCORS(w, r, cfg)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":"session_expired","message":"Open this tool from your access link."}`)
			return
		}
		if isNavigation {
			renderAccessDeniedPage(w, cfg)
		} else {
			applyProxyCORS(w, r, cfg)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":"unauthorized","message":"Open this tool from your access link."}`)
		}
		return
	}
	if err != nil && (isFavicon || skipAuth) {
		currentUser = "guest_passthrough"
	}

	if !skipAuth && !isFavicon && rejectPanelDevice(w, r, cfg) {
		return
	}
	logUsageRequest(r, path)

	// ── 2. Check blocked paths ────────────────────────────────────────────────────
	if isBlockedPath(path, cfg) {
		log.Printf("[BLOCK] User '%s' tried to access blocked path: %s", currentUser, path)
		if dbConnected {
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

	// ── 3. Get active account for this session ────────────────────────────────────
	sessionToken := ""
	if c, err := r.Cookie("ct_session"); err == nil {
		sessionToken = c.Value
	}
	var activeAcc ToolAccount
	if usesPanelAccountMode(cfg) {
		if _, sessionErr := panelSessionUsername(r); sessionErr != nil {
			if skipAuth || isFavicon {
				activeAcc = ToolAccount{}
			} else {
				renderAccessDeniedPage(w, cfg)
				return
			}
		} else {
			activeAcc, err = loadPanelSessionAccount(cfg, sessionToken)
			if err != nil {
				log.Printf("[PANEL] mapped account unavailable: %v", err)
				renderNoActiveAccountsPage(w, cfg)
				return
			}
		}
	} else if usesCookieFileMode(cfg) {
		// Detect if connection is actually over HTTPS (behind reverse proxy)
		isHTTPS := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
		// Auto-set session cookie if missing in local standalone mode
		if sessionToken == "" {
			sessionToken = "local_dev_session"
			http.SetCookie(w, &http.Cookie{
				Name:     "ct_session",
				Value:    sessionToken,
				Path:     "/",
				Expires:  time.Now().Add(24 * time.Hour),
				HttpOnly: true,
				Secure:   isHTTPS,              // Only set Secure flag when actually on HTTPS
				SameSite: http.SameSiteLaxMode, // Lax allows navigation across subdomains
			})
		}
		// Load dynamically from cookie.txt for hot-reloading
		data, err := os.ReadFile(cfg.CookieFile)
		if err != nil {
			log.Printf("[LOCAL] Failed to read local cookie file '%s': %v", cfg.CookieFile, err)
			renderNoActiveAccountsPage(w, cfg)
			return
		}
		// Parse cookie.txt — supports both raw 'name=val; ...' and JSON array format
		parsedCookie := parseCookieFromDB(string(data))
		activeAcc = ToolAccount{
			ID:        1,
			Name:      "Local Standalone Account",
			Cookie:    parsedCookie,
			UserAgent: cfg.UserAgent,
			Proxy:     "",
			ShowLimit: false,
		}
	} else if sessionToken != "" {
		var found bool
		activeAcc, found = getSessionAssignedAccount(sessionToken)
		if !found {
			activeAcc, err = autoAssignNextAccount(sessionToken)
			if err != nil {
				renderNoActiveAccountsPage(w, cfg)
				return
			}
		}
	} else if skipAuth {
		// CDN / Statsig passthrough — no member session cookie on this request
		activeAcc = ToolAccount{}
	} else {
		renderAccessDeniedPage(w, cfg)
		return
	}

	// ── 3b. Logout path hint (full handling on HTML response) ─────────────────────
	if detected, reason := detectLogout(path, nil, cfg); detected {
		log.Printf("[LOGOUT] Path hint | user=%s account=%s reason=%s", currentUser, activeAcc.Name, reason)
	}

	// ── 4. Credit/Limit check — DISABLED (bypass_auth mode) ─────────────────────
	// Limits are not enforced in standalone/bypass mode.

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
	isMainCDN := false
	if strings.HasPrefix(path, "/cdn/") && cdnParsed != nil {
		// /cdn/assets/foo.js → cdn.oaistatic.com/assets/foo.js
		upstreamURL.Scheme = cdnParsed.Scheme
		upstreamURL.Host = cdnParsed.Host
		upstreamURL.Path = "/" + strings.TrimPrefix(path, "/cdn/")
		isMainCDN = true
	} else if strings.HasPrefix(path, "/cdn-proxy/") && cdnParsed != nil {
		upstreamURL.Scheme = cdnParsed.Scheme
		upstreamURL.Host = cdnParsed.Host
		upstreamURL.Path = "/" + strings.TrimPrefix(path, "/cdn-proxy/")
	}

	// Handle wildcard extra CDN (regional Azure blob: *.oaiusercontent.com)
	isExtraCDNWild := false
	if wildHost, wildPath, ok := parseExtraCDNWildcardPath(path, cfg); ok {
		upstreamURL.Scheme = "https"
		upstreamURL.Host = wildHost
		upstreamURL.Path = wildPath
		isExtraCDNWild = true
	}

	// Handle Extra CDN routes
	isExtraCDN := false
	extraCDNIndex := -1
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
			isExtraCDN = true
			extraCDNIndex = i
			break
		}
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
	accountCookieStr := parseCookieFromDB(activeAcc.Cookie)
	clientCookies := stripSensitiveCookies(r.Header.Get("Cookie"), cfg)
	if accountCookieStr != "" {
		if clientCookies != "" {
			clientCookies += "; "
		}
		clientCookies += accountCookieStr
	}
	// Only inject premium cookies when upstream is the target tool domain (not third-party CDNs)
	hostWithoutPort := upstreamURL.Host
	if h, _, err := net.SplitHostPort(upstreamURL.Host); err == nil {
		hostWithoutPort = h
	}
	targetRoot := targetParsed.Hostname()
	sendCookies := hostWithoutPort == targetRoot || strings.HasSuffix(hostWithoutPort, "."+targetRoot) || strings.HasSuffix(hostWithoutPort, ".api.openai.com")
	if sendCookies {
		if clientCookies != "" {
			upstreamReq.Header.Set("Cookie", clientCookies)
		}
	} else {
		upstreamReq.Header.Del("Cookie")
	}
	upstreamReq.Header.Del("X-Device-Fp")
	upstreamReq.Header.Del("X-Device-Proof")

	// Anti-recloud: same premium account cookie from multiple /16 subnets (PHP mirror + real users).
	// ChatGPT serves many members on one upstream account — log only, do not block the user.
	ss := normalizeSessionSecurity(cfg)
	if securityEnabled(cfg) && sendCookies && activeAcc.ID > 0 {
		if accountIPTracker.Check(activeAcc.ID, realClientIP(r), ss.AccountCookieProtection) {
			log.Printf("[SECURITY] Account cookie sharing (warn) | account=%s (ID:%d) ip=%s user=%s", activeAcc.Name, activeAcc.ID, realClientIP(r), currentUser)
			recordSecurityEvent(r, currentUser, "account_cookie_sharing", "account_id="+strconv.Itoa(activeAcc.ID))
			go handleLogoutDetected(cfg, "account_cookie_sharing_recloud", activeAcc, currentUser, sessionToken, false)
		}
	}

	// ── WebSocket upgrade: hijack and bidirectionally pipe ───────────────────────
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		ua := activeAcc.UserAgent
		if ua == "" {
			ua = cfg.UserAgent
		}
		proxyWebSocket(w, r, upstreamURL, clientCookies, ua, cfg, activeAcc.Proxy)
		return
	}
	ua := activeAcc.UserAgent
	if ua == "" {
		ua = cfg.UserAgent
	}
	if ua != "" {
		upstreamReq.Header.Set("User-Agent", ua)
	}

	// Set upstream host header
	upstreamReq.Host = targetParsed.Host
	if (isMainCDN || strings.HasPrefix(path, "/cdn-proxy/")) && cdnParsed != nil {
		upstreamReq.Host = cdnParsed.Host
	} else if isExtraCDN && extraCDNIndex >= 0 {
		extraClean := strings.TrimPrefix(strings.TrimPrefix(cfg.ExtraCDNDomains[extraCDNIndex], "https://"), "http://")
		upstreamReq.Host = strings.Split(extraClean, "/")[0]
	} else if isExtraCDNWild {
		upstreamReq.Host = upstreamURL.Host
	}

	// Remove proxy hop-by-hop headers (nginx Connection: upgrade breaks HTTP/2 upstream)
	upstreamReq.Header.Del("Connection")
	upstreamReq.Header.Del("Upgrade")
	upstreamReq.Header.Del("Proxy-Connection")
	upstreamReq.Header.Del("Keep-Alive")
	upstreamReq.Header.Del("Transfer-Encoding")
	upstreamReq.Header.Del("TE")
	// Remove proxy headers
	upstreamReq.Header.Del("X-Forwarded-For")
	upstreamReq.Header.Del("X-Real-IP")
	// Never forward browser compression prefs — Cloudflare returns brotli (br) which Go's
	// http.Transport does not auto-decompress; we strip Content-Encoding so browsers get
	// corrupted SVG/woff2 assets and icons break.
	upstreamReq.Header.Del("Accept-Encoding")

	// Rewrite Origin and Referer — prefer config.json values (most reliable).
	// Falls back to request headers when config is blank (local dev mode).
	publicScheme := cfg.PublicScheme
	if publicScheme == "" {
		// Fallback: detect from reverse-proxy headers
		publicScheme = "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" || r.Header.Get("X-Forwarded-Ssl") == "on" {
			publicScheme = "https"
		}
	}
	publicHost := cfg.PublicHost
	if publicHost == "" {
		// Fallback: use Host header
		publicHost = r.Host
		if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
			publicHost = fwdHost
		}
	}
	publicBase := fmt.Sprintf("%s://%s", publicScheme, publicHost)
	targetBase := cfg.TargetURL

	origOrigin := r.Header.Get("Origin")
	origReferer := r.Header.Get("Referer")

	if origOrigin != "" {
		newOrigin := strings.ReplaceAll(origOrigin, publicBase, targetBase)
		u, err := url.Parse(newOrigin)
		if err == nil {
			newOrigin = u.Scheme + "://" + u.Host
		}
		upstreamReq.Header.Set("Origin", newOrigin)
	}

	if origReferer != "" {
		newReferer := strings.ReplaceAll(origReferer, publicBase, targetBase)
		for i := 0; i < 10; i++ {
			prefix := fmt.Sprintf("/extra-cdn-%d/", i)
			if strings.Contains(newReferer, prefix) {
				newReferer = strings.ReplaceAll(newReferer, prefix, "/")
			}
		}
		if strings.Contains(newReferer, "/extra-cdn-wild/") {
			re := regexp.MustCompile(`/extra-cdn-wild/[^/]+`)
			newReferer = re.ReplaceAllString(newReferer, "")
		}
		upstreamReq.Header.Set("Referer", newReferer)
	} else {
		upstreamReq.Header.Set("Referer", targetBase+"/")
	}

	if rejectIfChatCreditSpent(w, r, cfg, path, currentUser) {
		return
	}

	if serveConversation(w, conversationKey(r), 20*time.Second) {
		return
	}
	cacheKey, leader, flightDone := joinConversation(r)
	if !leader {
		<-flightDone
		if serveConversation(w, cacheKey, 2*time.Minute) {
			return
		}
	} else {
		defer finishConversation(cacheKey)
	}
	releaseList := gateConversationList(path)
	defer releaseList()

	upstreamResp, err := httpClient.Do(upstreamReq)
	if err != nil {
		log.Printf("[PROXY] Upstream request failed for user '%s' path '%s': %v", currentUser, path, err)
		if strings.Contains(err.Error(), "proxy dial") {
			renderProxyProblem(w, r)
			return
		}
		clientGone := strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), "can't assign requested address")
		if !clientGone && currentUser != "guest_passthrough" && sessionToken != "" && (dbConnected || usesPanelAccountMode(cfg)) {
			activeAcc, _ = switchToNextAccount(sessionToken, activeAcc.ID, activeAcc.Name, currentUser, "upstream_connection_error")
		}
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer upstreamResp.Body.Close()
	if cacheKey != "" && upstreamResp.StatusCode == http.StatusTooManyRequests {
		for try := 1; try <= 3 && upstreamResp.StatusCode == http.StatusTooManyRequests; try++ {
			time.Sleep(time.Duration(try) * 700 * time.Millisecond)
			retried, retryErr := httpClient.Do(upstreamReq)
			if retryErr != nil {
				break
			}
			upstreamResp.Body.Close()
			upstreamResp = retried
			defer upstreamResp.Body.Close()
			log.Printf("[API] conversation retry %d | user=%s path=%s", upstreamResp.StatusCode, currentUser, path)
		}
	}
	if strings.HasPrefix(path, "/backend-api/") && upstreamResp.StatusCode >= 400 {
		log.Printf("[API] upstream %d | user=%s account=%s path=%s", upstreamResp.StatusCode, currentUser, activeAcc.Name, path)
	}
	if nextAcc, nextResp, switched := recoverPanelAPIAccount(cfg, r, upstreamReq, upstreamResp, activeAcc, sessionToken, currentUser, path, sendCookies); switched {
		upstreamResp.Body.Close()
		upstreamResp = nextResp
		activeAcc = nextAcc
	}
	if usesPanelAccountMode(cfg) && isBillableChat(r, path) && upstreamResp.StatusCode >= 200 && upstreamResp.StatusCode < 300 {
		panelCreditsCharge(currentUser, path)
	}

	// ── HTML account failover: try next DB accounts when upstream returns logout page ──
	var preloadedHTML []byte
	contentType := upstreamResp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") {
		fo := runHTMLAccountFailover(r, upstreamReq, upstreamResp, activeAcc, sessionToken, currentUser, path, sendCookies, cfg)
		if fo.waiting {
			if fo.activeAcc.ID == 0 || fo.activeAcc.ID == activeAcc.ID {
				// Switch failed / only one dead account — do not keep ChatGPT open.
				renderNoActiveAccountsPage(w, cfg)
				return
			}
			writeAccountSwitchPage(w, fo.activeAcc.Name)
			return
		}
		if fo.ran {
			preloadedHTML = fo.bodyBytes
			activeAcc = fo.activeAcc
			if fo.upstreamResp != nil {
				upstreamResp = fo.upstreamResp
				contentType = upstreamResp.Header.Get("Content-Type")
			}
		}
	}

	// ── 7. Handle Set-Cookie and Location headers from upstream ──────────────────
	// Build all domain pairs for location header rewriting (same as HTML body rewriting)
	locationPairs := buildDomainReplacements(publicScheme, publicHost, cfg)
	for k, vv := range upstreamResp.Header {
		kLower := strings.ToLower(k)
		if kLower == "set-cookie" {
			continue
		} // Never forward upstream Set-Cookie to browser
		if kLower == "content-encoding" {
			continue
		} // We'll re-encode
		if kLower == "content-length" {
			continue
		} // Will be recalculated
		if kLower == "transfer-encoding" {
			continue
		}
		if kLower == "strict-transport-security" {
			continue
		} // Strip HSTS to prevent HTTPS upgrades
		if kLower == "cross-origin-opener-policy" || kLower == "cross-origin-embedder-policy" {
			continue
		}
		if kLower == "location" {
			for _, v := range vv {
				newLoc := v
				for _, pair := range locationPairs {
					newLoc = strings.ReplaceAll(newLoc, pair[0], pair[1])
				}
				newLoc = rewriteExtraCDNWildcardURL(newLoc, publicBase)
				w.Header().Add(k, newLoc)
			}
			continue
		}
		if (isExtraCDN || isExtraCDNWild) && kLower == "access-control-allow-origin" {
			continue
		}
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	if isExtraCDN || isExtraCDNWild {
		applyExtraCDNCORS(w, r, cfg)
	}
	if strings.HasPrefix(path, "/backend-api/") {
		applyProxyCORS(w, r, cfg)
	}

	// CDN/static assets: stream through without decompress+rewrite (10MB+ JS bundles)
	if isProxyStaticPath(path) {
		passthroughStaticResponse(w, upstreamResp)
		return
	}

	// Disable cache only for HTML/API — allow CDN assets to cache (major speed win)
	isStaticAsset := isProxyStaticPath(path)
	if !isStaticAsset {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
	}

	// ── 8. SSE (Server-Sent Events) — rewrite domains line-by-line, stream to client ──
	if isSSEResponse(contentType) {
		streamSSEResponse(w, upstreamResp, publicScheme, publicHost, cfg)
		return
	}

	// ── 9. Process HTML responses (inject scripts, rewrite URLs) ──────────────────
	isHTML := strings.Contains(contentType, "text/html")
	if isHTML {
		var bodyBytes []byte
		var err error
		if preloadedHTML != nil {
			bodyBytes = preloadedHTML
		} else {
			bodyBytes, err = decompressBody(upstreamResp)
			if err != nil {
				w.WriteHeader(upstreamResp.StatusCode)
				return
			}
		}

		// Logout still present after server-side failover — trigger cookie automation only
		logoutPageDetected := false
		if detected, reason := chatGPTLogoutDetected(path, bodyBytes, cfg); detected {
			log.Printf("[LOGOUT] HTML detection | user=%s account=%s reason=%s", currentUser, activeAcc.Name, reason)
			skipSwitch := preloadedHTML != nil
			handleLogoutDetected(cfg, reason, activeAcc, currentUser, sessionToken, skipSwitch)
			if isStrongLogoutReason(reason) || !chatGPTLoggedInHTML(bodyBytes) {
				logoutPageDetected = true
			} else {
				log.Printf("[LOGOUT] Skipping overlay — weak signal on logged-in page: %s", reason)
			}
		}

		// Rewrite domain references
		bodyBytes = rewriteBodyWithWildcards(bodyBytes, publicScheme, publicHost, cfg)
		bodyBytes = rewriteHTMLBaseTag(bodyBytes, publicScheme, publicHost)
		bodyBytes = applyServerThemeClass(bodyBytes)
		if cfg.CloudflareBypass {
			bodyBytes = stripCloudflareChallengeHTML(bodyBytes, publicScheme, publicHost)
		}

		// Remove CSP header (prevents our injected scripts)
		w.Header().Del("Content-Security-Policy")
		w.Header().Del("Content-Security-Policy-Report-Only")
		w.Header().Del("X-Frame-Options")

		headInject := patcherHeadScript(cfg)
		bodyInject := patcherBodyScript(cfg)
		if usesPanelAccountMode(cfg) {
			// Plain HTTP: skip devicePageScript (visibility/bind races → blank/black page).
			if !strings.EqualFold(strings.TrimSpace(cfg.PublicScheme), "http") {
				headInject = devicePageScript() + headInject
			}
			headInject += limitWidgetScript(cfg)
		}
		if !usesCookieFileMode(cfg) {
			bodyInject += buildDomainCheckJS(cfg) + buildSecurityHeartbeatJS(cfg)
			if logoutPageDetected {
				bodyInject = logoutOverlayImmediateScript(cfg) + bodyInject
			}
		}
		if !bytes.Contains(bodyBytes, []byte("data-tm-proxy-patcher")) {
			headRe := regexp.MustCompile(`(?i)<head(?:\s[^>]*)?>`)
			if loc := headRe.FindIndex(bodyBytes); loc != nil {
				insertAt := loc[1]
				newBody := make([]byte, 0, len(bodyBytes)+len(headInject)+len(bodyInject))
				newBody = append(newBody, bodyBytes[:insertAt]...)
				newBody = append(newBody, headInject...)
				newBody = append(newBody, bodyBytes[insertAt:]...)
				bodyBytes = newBody
			}
		}
		if !bytes.Contains(bodyBytes, []byte("data-tm-proxy-aux")) {
			bodyRe := regexp.MustCompile(`(?i)</body>`)
			if loc := bodyRe.FindIndex(bodyBytes); loc != nil {
				newBody := make([]byte, 0, len(bodyBytes)+len(bodyInject))
				newBody = append(newBody, bodyBytes[:loc[0]]...)
				newBody = append(newBody, bodyInject...)
				newBody = append(newBody, bodyBytes[loc[0]:]...)
				bodyBytes = newBody
			} else {
				bodyBytes = append(bodyBytes, bodyInject...)
			}
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		applySecurityHeaders(w, cfg)
		w.WriteHeader(upstreamResp.StatusCode)
		w.Write(bodyBytes)
		return
	}

	// ── 10. Rewrite JSON/JS/CSS/RSC payloads ───────────────────────────────────
	isRSC := strings.Contains(contentType, "text/x-component")
	isRewritable := isRSC ||
		strings.Contains(contentType, "javascript") ||
		strings.Contains(contentType, "application/json") ||
		strings.Contains(contentType, "text/css")
	if isRewritable {
		bodyBytes, err := decompressBody(upstreamResp)
		if err == nil {
			bodyBytes = rewriteBodyWithWildcards(bodyBytes, publicScheme, publicHost, cfg)
			if upstreamResp.StatusCode == http.StatusTooManyRequests {
				if serveConversation(w, conversationKey(r), 2*time.Minute) {
					return
				}
			}
			storeConversation(r, upstreamResp.StatusCode, bodyBytes)
			if isRSC {
				w.Header().Set("Content-Type", "text/x-component")
			}
			w.WriteHeader(upstreamResp.StatusCode)
			w.Write(bodyBytes)
			return
		}
	}

	// ── 11. Pass through everything else (SVG, fonts, images, etc.) ─────────────
	bodyBytes, err := decompressBody(upstreamResp)
	if err != nil {
		w.WriteHeader(upstreamResp.StatusCode)
		return
	}
	if strings.Contains(path, "/files/download/") && !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		switch {
		case bytes.HasPrefix(bodyBytes, []byte("\x89PNG")):
			w.Header().Set("Content-Type", "image/png")
		case bytes.HasPrefix(bodyBytes, []byte("\xff\xd8")):
			w.Header().Set("Content-Type", "image/jpeg")
		case bytes.HasPrefix(bodyBytes, []byte("GIF8")):
			w.Header().Set("Content-Type", "image/gif")
		case len(bodyBytes) > 12 && bytes.HasPrefix(bodyBytes, []byte("RIFF")) && bytes.Contains(bodyBytes[:12], []byte("WEBP")):
			w.Header().Set("Content-Type", "image/webp")
		}
		log.Printf("[FILE] %d %s bytes=%d path=%s", upstreamResp.StatusCode, w.Header().Get("Content-Type"), len(bodyBytes), path)
	}
	w.WriteHeader(upstreamResp.StatusCode)
	w.Write(bodyBytes)
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
	log.Printf("[AUTOMATION] logout_detection=%v automation=%v url=%s task_uid=%v",
		cfg.LogoutDetection.Enabled, cfg.Automation.Enabled, cfg.Automation.URL, cfg.Automation.Payload["task_uid"])
	initDB(cfg)
	resolveWebsiteID(cfg.PublicHost)
	ss := normalizeSessionSecurity(cfg)
	refreshWebsiteSecurityFromDB()
	initScoreEngine(cfg)
	ottMode := ss.OTTIPValidation.Mode
	if ottMode == "" {
		ottMode = "soft"
	}
	log.Printf("[PROXY] build_tag=%s public_host=%s use_database=%v", proxyBuildTag, cfg.PublicHost, cfg.UseDatabase)
	log.Printf("[SECURITY] build=%s lib=%s enabled=%v db_toggle=%v single_session=%v ip_soft=%v device=%v domain_check=%v ott_ip=%v ott_mode=%s datacenter=%v score_mode=%s cloudflare=%v",
		proxysec.BuildID, proxysec.BuildID, securityEnabled(cfg), currentWebsiteSecurityEnabled, ss.SingleSessionPerUser, ss.IPProtection.Enabled,
		ss.DeviceCookie.Enabled, ss.DomainCheck.Enabled, ss.OTTIPValidation.Enabled, ottMode,
		ss.DatacenterScore.Enabled, ss.DatacenterScore.ScoreMode, ss.Cloudflare.Enabled)
	startDailyResetCron()
	startBlockedIPRefreshLoop()

	mux := http.NewServeMux()

	// ── API routes ────────────────────────────────────────────────────────────────
	mux.HandleFunc("/api/auth-handshake", withCORS(authHandshakeHandler))
	mux.HandleFunc("/api/user-limits", withCORS(userLimitsAPIHandler))
	mux.HandleFunc("/api/rotate-session", withCORS(rotateSessionHandler))
	mux.HandleFunc("/api/trigger-automation", withCORS(triggerAutomationHandler))
	mux.HandleFunc("/api/security-ping", withCORS(securityPingHandler))
	mux.HandleFunc("/api/device-bind", deviceBindHandler)
	mux.HandleFunc("/__tm_access_denied", serveAccessDeniedHTML)
	mux.HandleFunc("/tm-device-sw.js", serveDeviceSW)

	// ── Access handler (OTT → session cookie) ────────────────────────────────────
	mux.HandleFunc("/access", accessHandler)

	// ── Logout ────────────────────────────────────────────────────────────────────
	mux.HandleFunc("/user/logout", func(w http.ResponseWriter, r *http.Request) {
		cfg := loadConfig()
		if c, err := r.Cookie("ct_session"); err == nil {
			panelSess.Delete(c.Value)
		}
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

	// ── Security middleware wrapper ───────────────────────────────────────────────
	secureHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := loadConfig()
		if r.Header.Get("X-Forwarded-Proto") == "http" && strings.EqualFold(cfg.PublicScheme, "https") {
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
	log.Printf("✅ Generic Tool Proxy listening on %s", addr)
	go primeUpstream(cfg)
	server := &http.Server{
		Addr:         addr,
		Handler:      secureHandler,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  180 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("[SERVER] Fatal: %v", err)
	}
}
