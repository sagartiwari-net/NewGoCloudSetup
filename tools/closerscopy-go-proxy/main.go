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
	// PanelDB is the local panel database. When set, Open comes from the panel access link.
	PanelDB string `json:"panel_db"`
	// BypassAuth: bypasses database user authentication and loads cookie.txt directly (useful for testing without security)
	BypassAuth bool `json:"bypass_auth"`
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
		TargetURL:              "https://www.closerscopy.com",
		CDNURL:                 "https://www.closerscopy.com",
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
		ToolName:               "ClosersCopy",
		CreditLabel:            "Credits",
		ExportLabel:            "Exports",
		HomePath:               "/dashboard",
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

// Laravel CSRF: mirror XSRF-TOKEN to browser (session cookies stay server-side only).
func isBrowserMirroredCookie(name string) bool {
	return strings.EqualFold(name, "XSRF-TOKEN")
}

// Upstream Laravel apps rotate session/CSRF via Set-Cookie — track server-side (never forward session to browser).
var dynamicUpstreamCookies sync.Map

func absorbUpstreamSetCookies(h http.Header) {
	for _, raw := range h.Values("Set-Cookie") {
		part := strings.SplitN(raw, ";", 2)[0]
		nv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(nv) != 2 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(nv[0]))
		switch name {
		case "xsrf-token", "closerscopy_session_ccv2", "closerscopy_session":
			dynamicUpstreamCookies.Store(name, nv[1])
		}
	}
}

func applyDynamicCookies(cookieHeader string) string {
	if cookieHeader == "" {
		return cookieHeader
	}
	parts := strings.Split(cookieHeader, ";")
	out := make([]string, 0, len(parts)+2)
	seen := map[string]bool{}
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		nv := strings.SplitN(trimmed, "=", 2)
		if len(nv) != 2 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(nv[0]))
		if v, ok := dynamicUpstreamCookies.Load(name); ok {
			out = append(out, nv[0]+"="+v.(string))
		} else {
			out = append(out, trimmed)
		}
		seen[name] = true
	}
	addIfMissing := func(canonical, key string) {
		if seen[key] {
			return
		}
		if v, ok := dynamicUpstreamCookies.Load(key); ok {
			out = append(out, canonical+"="+v.(string))
			seen[key] = true
		}
	}
	addIfMissing("closerscopy_session_ccv2", "closerscopy_session_ccv2")
	addIfMissing("closerscopy_session", "closerscopy_session")
	addIfMissing("XSRF-TOKEN", "xsrf-token")
	return strings.Join(out, "; ")
}

// ClosersCopy cookie.txt: only session + CSRF matter for upstream auth (ignore _ga/_fbp/etc).
var essentialUpstreamCookies = map[string]bool{
	"closerscopy_session_ccv2": true,
	"closerscopy_session":      true,
	"xsrf-token":               true,
}

func filterEssentialCookies(cookieHeader string) string {
	if cookieHeader == "" {
		return cookieHeader
	}
	var kept []string
	for _, part := range strings.Split(cookieHeader, ";") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if eq := strings.Index(trimmed, "="); eq > 0 {
			if essentialUpstreamCookies[strings.ToLower(strings.TrimSpace(trimmed[:eq]))] {
				kept = append(kept, trimmed)
			}
		}
	}
	return strings.Join(kept, "; ")
}

// keepBrowserCookie is the only state the browser must send back.
// Account session + CSRF stay on the server; a fat jar makes nginx/Apache refuse the request.
func keepBrowserCookie(name string) bool {
	nl := strings.ToLower(strings.TrimSpace(name))
	return nl == "ct_session" || strings.HasPrefix(nl, "tm_")
}

// slimBrowserCookies drops everything except ct_session / tm_* and returns names to expire.
func slimBrowserCookies(r *http.Request) []string {
	raw := r.Header.Get("Cookie")
	if raw == "" {
		return nil
	}
	var kept []string
	var dropped []string
	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name := part
		if eq := strings.Index(part, "="); eq >= 0 {
			name = strings.TrimSpace(part[:eq])
		}
		if keepBrowserCookie(name) {
			kept = append(kept, part)
			continue
		}
		if name != "" && name != part {
			dropped = append(dropped, name)
		}
	}
	if len(dropped) == 0 {
		return nil
	}
	slim := strings.Join(kept, "; ")
	log.Printf("[COOKIE] slimmed browser Cookie %d → %d bytes path=%s", len(raw), len(slim), r.URL.Path)
	if slim == "" {
		r.Header.Del("Cookie")
	} else {
		r.Header.Set("Cookie", slim)
	}
	return dropped
}

func expireDroppedCookies(w http.ResponseWriter, r *http.Request, names []string, cfg Config) {
	seen := map[string]bool{}
	for _, name := range names {
		nl := strings.ToLower(strings.TrimSpace(name))
		if nl == "" || seen[nl] || keepBrowserCookie(nl) {
			continue
		}
		seen[nl] = true
		domains := []string{""}
		if host := strings.TrimSpace(cfg.PublicHost); host != "" {
			if i := strings.Index(host, "."); i > 0 {
				domains = append(domains, host[i:])
			}
		}
		for _, domain := range domains {
			c := &http.Cookie{
				Name:     name,
				Value:    "",
				Path:     "/",
				Expires:  time.Unix(0, 0),
				MaxAge:   -1,
				Secure:   cookieSecure(r, cfg),
				SameSite: http.SameSiteLaxMode,
			}
			if domain != "" {
				c.Domain = domain
			}
			http.SetCookie(w, c)
		}
	}
}

var metaCsrfRe = regexp.MustCompile(`(?i)<meta\s+name="csrf-token"\s+content="([^"]+)"`)
var laravelFormIDs = []string{"form-project", "form-copy"}

func storeCsrfPlainFromHTML(body []byte) {
	if m := metaCsrfRe.FindSubmatch(body); len(m) > 1 {
		dynamicUpstreamCookies.Store("csrf-plain", string(m[1]))
	}
}

var inlineCsrfSingleRe = regexp.MustCompile(`'X-CSRF-TOKEN'\s*:\s*'[^']*'`)
var inlineCsrfDoubleRe = regexp.MustCompile(`"X-CSRF-TOKEN"\s*:\s*"[^"]*"`)

// resyncInlineCsrfTokens replaces stale hardcoded jQuery CSRF headers with the live meta token.
func resyncInlineCsrfTokens(body []byte) []byte {
	m := metaCsrfRe.FindSubmatch(body)
	if len(m) < 2 {
		return body
	}
	token := string(m[1])
	body = inlineCsrfSingleRe.ReplaceAll(body, []byte(`'X-CSRF-TOKEN':'`+token+`'`))
	body = inlineCsrfDoubleRe.ReplaceAll(body, []byte(`"X-CSRF-TOKEN":"`+token+`"`))
	return body
}

// injectLaravelFormCsrf adds _token to ClosersCopy forms (jQuery FormData POSTs often skip X-CSRF-TOKEN header).
func injectLaravelFormCsrf(body []byte) []byte {
	m := metaCsrfRe.FindSubmatch(body)
	if len(m) < 2 {
		return body
	}
	token := string(m[1])
	inject := []byte(`<input type="hidden" name="_token" value="` + token + `">`)
	for _, formID := range laravelFormIDs {
		formRe := regexp.MustCompile(`(?i)(<form[^>]*\bid=["']` + regexp.QuoteMeta(formID) + `["'][^>]*>)`)
		loc := formRe.FindIndex(body)
		if loc == nil {
			continue
		}
		formEnd := bytes.Index(body[loc[1]:], []byte("</form>"))
		if formEnd < 0 {
			continue
		}
		formBlock := body[loc[0] : loc[1]+formEnd]
		if bytes.Contains(formBlock, []byte(`name="_token"`)) {
			continue
		}
		out := make([]byte, 0, len(body)+len(inject))
		out = append(out, body[:loc[1]]...)
		out = append(out, inject...)
		out = append(out, body[loc[1]:]...)
		body = out
	}
	return body
}

func injectUpstreamCsrf(upstreamReq *http.Request, r *http.Request, cfg Config) {
	switch strings.ToUpper(upstreamReq.Method) {
	case "POST", "PUT", "PATCH", "DELETE":
	default:
		return
	}
	serverCsrf := ""
	if v, ok := dynamicUpstreamCookies.Load("csrf-plain"); ok {
		serverCsrf = v.(string)
	}
	csrf := r.Header.Get("X-CSRF-TOKEN")
	if csrf == "" {
		csrf = r.Header.Get("X-Csrf-Token")
	}
	// Prefer server-side token: browser inline jQuery headers are often stale vs rotated session.
	if serverCsrf != "" && (usesCookieFileMode(cfg) || csrf == "") {
		csrf = serverCsrf
	}
	if csrf != "" {
		upstreamReq.Header.Set("X-CSRF-TOKEN", csrf)
		upstreamReq.Header.Set("X-XSRF-TOKEN", csrf)
	}
}

// dropCookieNames removes stale static CSRF from cookie.txt (dynamic Set-Cookie is source of truth).
func dropCookieNames(cookieHeader string, names ...string) string {
	if cookieHeader == "" {
		return cookieHeader
	}
	drop := map[string]bool{}
	for _, n := range names {
		drop[strings.ToLower(n)] = true
	}
	var kept []string
	for _, part := range strings.Split(cookieHeader, ";") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if eq := strings.Index(trimmed, "="); eq > 0 {
			if drop[strings.ToLower(strings.TrimSpace(trimmed[:eq]))] {
				continue
			}
		}
		kept = append(kept, trimmed)
	}
	return strings.Join(kept, "; ")
}

func forwardAllowlistedSetCookies(w http.ResponseWriter, r *http.Request, upstream http.Header, cfg Config) {
	for _, raw := range upstream.Values("Set-Cookie") {
		parts := strings.SplitN(raw, ";", 2)
		nv := strings.SplitN(strings.TrimSpace(parts[0]), "=", 2)
		if len(nv) != 2 || !isBrowserMirroredCookie(nv[0]) {
			continue
		}
		http.SetCookie(w, &http.Cookie{
			Name:     nv[0],
			Value:    nv[1],
			Path:     "/",
			Secure:   cookieSecure(r, cfg),
			SameSite: http.SameSiteLaxMode,
			HttpOnly: false,
		})
	}
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
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		recordSecurityEvent(r, "", "access_denied", "missing token")
		renderAccessDeniedPage(w, cfg)
		return
	}
	username := strings.TrimSpace(r.URL.Query().Get("user"))
	var dbUsername, dbClientIP string
	var expiresAt time.Time
	err := db.QueryRow(
		"SELECT username, client_ip, expires_at FROM ahrefs_tokens WHERE token = ? AND website_id = ?",
		token, currentWebsiteID,
	).Scan(&dbUsername, &dbClientIP, &expiresAt)
	if err != nil {
		log.Printf("[ACCESS] ❌ OTT lookup failed: %v", err)
		recordSecurityEvent(r, "", "access_denied", "invalid or used token")
		renderAccessDeniedPage(w, cfg)
		return
	}
	username = strings.TrimSpace(dbUsername)
	if username == "" {
		recordSecurityEvent(r, "", "access_denied", "token has empty username")
		renderAccessDeniedPage(w, cfg)
		return
	}
	if time.Now().After(expiresAt) {
		_, _ = db.Exec("DELETE FROM ahrefs_tokens WHERE token = ? AND website_id = ?", token, currentWebsiteID)
		recordSecurityEvent(r, username, "access_denied", "token expired")
		renderAccessDeniedPage(w, cfg)
		return
	}

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
	for _, t := range ld.TextSniffs {
		if t != "" && (strings.Contains(bodyStr, t) || strings.Contains(strings.ToLower(bodyStr), strings.ToLower(t))) {
			return true, "text_sniff:" + t
		}
	}
	for _, h := range ld.HTMLSniffs {
		if h != "" && strings.Contains(bodyStr, h) {
			return true, "html_sniff:" + h[:min(60, len(h))]
		}
	}
	// Fallback: partial match for common login button text
	if strings.Contains(bodyStr, "Sign In") && strings.Contains(bodyStr, "Sign in to Airbrush") {
		return true, "html_sniff:login_page_heuristic"
	}
	return false, ""
}

func handleLogoutDetected(cfg Config, reason string, acc ToolAccount, username, sessionToken string) bool {
	log.Printf("[LOGOUT] ✅ Detected | user=%s account=%s (ID:%d) reason=%s", username, acc.Name, acc.ID, reason)

	triggered := false
	if cfg.Automation.Enabled {
		triggered = tryTriggerAutomationForAccount(cfg, reason, acc, username)
	}

	if dbConnected && sessionToken != "" {
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
	triggered := handleLogoutDetected(cfg, payload.Reason, acc, username, sessionToken)

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
		Message: "You cannot open <span class=\"brand\">" + name + "</span> directly. Open it again from your access link.",
		Footer:  "Your session ended or this browser is not authorized",
	})
}

func renderNoActiveAccountsPage(w http.ResponseWriter, cfg Config) {
	name := html.EscapeString(toolDisplayName(cfg))
	writeLightCard(w, http.StatusServiceUnavailable, lightCard{
		Title:   "Temporarily Unavailable",
		Heading: "Temporarily Unavailable",
		Message: "All mapped <span class=\"brand\">" + name + "</span> accounts are currently undergoing maintenance. Please try again in a few minutes.",
		Footer:  "No active account is available right now",
	})
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

func dialChrome(ctx context.Context, addr string) (*uTLSConn, error) {
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
		tcpConn, err = (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, "tcp", addr)
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

const proxyBuildTag = "closerscopy-v8"

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
func buildDomainReplacements(publicScheme, publicHost string, cfg Config) [][2]string {
	targetParsed, _ := url.Parse(cfg.TargetURL)
	cdnParsed, _ := url.Parse(cfg.CDNURL)
	publicBase := fmt.Sprintf("%s://%s", publicScheme, publicHost)
	var pairs [][2]string
	if targetParsed != nil {
		pairs = append(pairs, [2]string{"https://" + targetParsed.Host, publicBase})
		pairs = append(pairs, [2]string{"http://" + targetParsed.Host, publicBase})
	}
	if cdnParsed != nil {
		pairs = append(pairs, [2]string{"https://" + cdnParsed.Host, publicBase + "/cdn-proxy"})
		pairs = append(pairs, [2]string{"http://" + cdnParsed.Host, publicBase + "/cdn-proxy"})
	}
	for i, extra := range cfg.ExtraCDNDomains {
		extra = strings.TrimPrefix(strings.TrimPrefix(extra, "https://"), "http://")
		extra = strings.Split(extra, "/")[0]
		pairs = append(pairs, [2]string{"https://" + extra, fmt.Sprintf("%s/extra-cdn-%d", publicBase, i)})
		pairs = append(pairs, [2]string{"http://" + extra, fmt.Sprintf("%s/extra-cdn-%d", publicBase, i)})
	}
	// Also replace raw hostnames to handle Javascript comparisons (e.g. location.host == 'members.helium10.com')
	if targetParsed != nil {
		pairs = append(pairs, [2]string{targetParsed.Host, publicHost})
	}
	// Local HTTP proxy: never leave https://localhost links (browser → ERR_SSL_PROTOCOL_ERROR)
	if isLocalDev(cfg) || strings.EqualFold(publicScheme, "http") {
		pairs = append(pairs, [2]string{"https://" + publicHost, publicBase})
	}
	for i, extra := range cfg.ExtraCDNDomains {
		extra = strings.TrimPrefix(strings.TrimPrefix(extra, "https://"), "http://")
		extra = strings.Split(extra, "/")[0]
		pairs = append(pairs, [2]string{extra, fmt.Sprintf("%s/extra-cdn-%d", publicHost, i)})
	}
	return pairs
}

func rewriteBody(body []byte, pairs [][2]string) []byte {
	for _, pair := range pairs {
		body = bytes.ReplaceAll(body, []byte(pair[0]), []byte(pair[1]))
	}
	return body
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

	// Build extra CDN domain replacements for XHR/fetch patching.
	// Each extra CDN domain gets proxied through /extra-cdn-N/ on our server.
	var extraCDNJS strings.Builder
	extraCDNJS.WriteString("[")
	for i, extra := range cfg.ExtraCDNDomains {
		extraClean := strings.TrimPrefix(strings.TrimPrefix(extra, "https://"), "http://")
		extraClean = strings.Split(extraClean, "/")[0]
		proxyPath := fmt.Sprintf("/extra-cdn-%d", i)
		extraCDNJS.WriteString(fmt.Sprintf(`["https://%s",%q],["http://%s",%q],`, extraClean, proxyPath, extraClean, proxyPath))
	}
	extraCDNJS.WriteString("]")

	logoutAutoJS := buildLogoutAutomationJS(cfg)

	return fmt.Sprintf(`<script>
(function() {
    try {
      document.cookie.split(';').forEach(function(part) {
        var name = (part.split('=')[0] || '').trim();
        if (!name || name === 'ct_session' || name.indexOf('tm_') === 0) return;
        document.cookie = name + '=;expires=Thu, 01 Jan 1970 00:00:00 GMT;path=/';
      });
    } catch (e) {}
    var T = '%s', C = '%s', O = window.location.origin;
    var HOME = '%s';
    var BLOCKED = [%s];
    // Extra CDN domains proxied through our server (fixes CORS on external APIs)
    var EXTRA = %s;

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
            over.style.cssText = 'position:fixed;top:0;left:0;width:100vw;height:100vh;background:#0f172a;color:#fff;z-index:99999999;display:flex;flex-direction:column;align-items:center;justify-content:center;font-family:sans-serif;gap:16px;';
            over.innerHTML = '<div style="border:4px solid #f8fafc;border-top-color:#4f46e5;border-radius:50%%;width:40px;height:40px;"></div><span>Reconnecting... Please wait</span>';
            document.body.appendChild(over);
            fetch('/api/rotate-session?reason=' + encodeURIComponent(reason))
            .then(function() { setTimeout(function() { window.location.href = HOME; }, 1200); })
            .catch(function() { setTimeout(function() { window.location.href = HOME; }, 1200); });
        }
    }
    if (%s) { setTimeout(checkWatchdog, 1000); setInterval(checkWatchdog, 4000); }

    // ── Link rewriter ──
    function rewriteLinks() {
        document.querySelectorAll('a[href*="'+T+'"]').forEach(function(a) {
            var h = a.href.replace('https://'+T, O).replace('http://'+T, O);
            if (a.href !== h) a.href = h;
        });
    }
    rewriteLinks();
    setInterval(rewriteLinks, 200);

    function patchURL(u) {
        if (typeof u !== 'string') return u;
        u = u.replace('https://'+T, O).replace('http://'+T, O);
        if (C) u = u.replace('https://'+C, O+'/cdn-proxy').replace('http://'+C, O+'/cdn-proxy');
        for (var e=0; e<EXTRA.length; e++) u = u.replace(EXTRA[e][0], O+EXTRA[e][1]);
        return u;
    }

    // Laravel CSRF — proxy blocks upstream Set-Cookie; inject token on POST/AJAX
    function getCsrfToken() {
        var m = document.querySelector('meta[name="csrf-token"]');
        if (m && m.content) return m.content;
        var match = document.cookie.match(/(?:^|;\s*)XSRF-TOKEN=([^;]+)/);
        if (match) { try { return decodeURIComponent(match[1]); } catch(e) {} }
        return '';
    }
    function applyCsrfToHeaders(h) {
        var t = getCsrfToken();
        if (!t) return h;
        if (h instanceof Headers) {
            if (!h.has('X-CSRF-TOKEN')) h.set('X-CSRF-TOKEN', t);
            if (!h.has('X-XSRF-TOKEN')) h.set('X-XSRF-TOKEN', t);
            if (!h.has('X-Requested-With')) h.set('X-Requested-With', 'XMLHttpRequest');
            return h;
        }
        h = h ? Object.assign({}, h) : {};
        if (!h['X-CSRF-TOKEN']) h['X-CSRF-TOKEN'] = t;
        if (!h['X-XSRF-TOKEN']) h['X-XSRF-TOKEN'] = t;
        if (!h['X-Requested-With']) h['X-Requested-With'] = 'XMLHttpRequest';
        return h;
    }

    // ── XHR patch ──
    var xo = XMLHttpRequest.prototype.open;
    XMLHttpRequest.prototype.open = function(m, u) {
        return xo.apply(this, [m, patchURL(u)].concat(Array.prototype.slice.call(arguments, 2)));
    };
    // ClosersCopy uses X-CSRF-TOKEN (meta csrf-token), not only Laravel X-XSRF-TOKEN
    var xsend = XMLHttpRequest.prototype.send;
    XMLHttpRequest.prototype.send = function(body) {
        try {
            var t = getCsrfToken();
            if (t) {
                this.setRequestHeader('X-CSRF-TOKEN', t);
                this.setRequestHeader('X-XSRF-TOKEN', t);
                this.setRequestHeader('X-Requested-With', 'XMLHttpRequest');
            }
        } catch(e) {}
        return xsend.apply(this, arguments);
    };

    // jQuery.ajax (ClosersCopy) — override stale inline CSRF with live meta token
    function installJQueryCsrf() {
        if (!window.jQuery) return false;
        jQuery.ajaxPrefilter(function(options) {
            var t = getCsrfToken();
            if (!t) return;
            options.headers = options.headers || {};
            options.headers['X-CSRF-TOKEN'] = t;
            options.headers['X-XSRF-TOKEN'] = t;
            options.headers['X-Requested-With'] = options.headers['X-Requested-With'] || 'XMLHttpRequest';
        });
        return true;
    }
    if (!installJQueryCsrf()) {
        var jqPoll = setInterval(function() {
            if (installJQueryCsrf()) clearInterval(jqPoll);
        }, 100);
        document.addEventListener('DOMContentLoaded', function() { installJQueryCsrf(); });
    }

    // ── Fetch patch ──
    var fo = window.fetch;
    window.fetch = function(inp, init) {
        init = init ? Object.assign({}, init) : {};
        init.headers = applyCsrfToHeaders(init.headers);
        if (typeof inp === 'string') inp = patchURL(inp);
        else if (inp instanceof Request) inp = new Request(patchURL(inp.url), inp);
        return fo(inp, init);
    };

    // ── WebSocket patch ──
    // Rewrites wss://target-domain/... → wss://our-proxy/...
    var OrigWebSocket = window.WebSocket;
    function patchWsURL(u) {
        if (typeof u !== 'string') return u;
        var isWss = u.indexOf('wss://') === 0;
        var isWs  = u.indexOf('ws://') === 0;
        if (!isWss && !isWs) return u;
        var asHttp = u.replace(/^wss:\/\//, 'https://').replace(/^ws:\/\//, 'http://');
        var patched = patchURL(asHttp);
        if (isWss) return patched.replace(/^https:\/\//, 'wss://').replace(/^http:\/\//, 'ws://');
        return patched.replace(/^https:\/\//, 'wss://').replace(/^http:\/\//, 'ws://');
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
                try { var p = new URL(url, O); if (isBlocked(p.pathname)) { window.location.href = HOME; return; } } catch(e) {}
            }
            var r = _push(state, title, url);
            if (typeof checkLogoutAutomation === 'function') setTimeout(checkLogoutAutomation, 300);
            return r;
        };
        var _replace = history.replaceState.bind(history);
        history.replaceState = function(state, title, url) {
            if (typeof url === 'string') {
                try { var p = new URL(url, O); if (isBlocked(p.pathname)) { window.location.href = HOME; return; } } catch(e) {}
            }
            var r = _replace(state, title, url);
            if (typeof checkLogoutAutomation === 'function') setTimeout(checkLogoutAutomation, 300);
            return r;
        };
    }

    %s
})();
</script>`,
		targetHost, cdnHost, homePath, blockedListJS.String(), extraCDNJS.String(), triggerChecks.String(),
		func() string {
			if len(cfg.WatchdogTriggers) > 0 {
				return "true"
			}
			return "false"
		}(),
		logoutAutoJS,
	)
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
		msg = "Account logout detected. We are updating your session, please wait..."
	}
	refreshSec := ld.RefreshSeconds
	if refreshSec <= 0 {
		refreshSec = 5
	}
	urlPathsJSON, _ := json.Marshal(ld.URLPaths)
	textSniffsJSON, _ := json.Marshal(ld.TextSniffs)
	htmlSniffsJSON, _ := json.Marshal(ld.HTMLSniffs)
	return fmt.Sprintf(`
    // ── Logout detection → overlay → 5s refresh loop until logged in ──
    var AUTO_TRIGGER_ENABLED = %v;
    var SHOW_LOGOUT_OVERLAY = %v;
    var REFRESH_MS = %d;
    var OVERLAY_TITLE = %q;
    var OVERLAY_MSG = %q;
    var refreshTimer = null;
    var countdownTimer = null;
    var apiCalledThisPage = false;

    function showTmLogoutOverlay() {
        if (!SHOW_LOGOUT_OVERLAY || document.getElementById('tm-logout-overlay')) return;
        var o = document.createElement('div');
        o.id = 'tm-logout-overlay';
        o.style.cssText = 'position:fixed;inset:0;background:rgba(15,23,42,0.93);backdrop-filter:blur(10px);z-index:2147483647;display:flex;align-items:center;justify-content:center;font-family:-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;padding:20px;';
        o.innerHTML = '<div style="background:#fff;border-radius:20px;padding:36px 32px;max-width:420px;width:100%%;text-align:center;box-shadow:0 25px 60px rgba(0,0,0,0.35);">' +
            '<div style="width:52px;height:52px;border:4px solid #e2e8f0;border-top-color:#4f46e5;border-radius:50%%;margin:0 auto 20px;animation:tmSpin 0.8s linear infinite;"></div>' +
            '<h2 style="font-size:20px;font-weight:700;color:#0f172a;margin:0 0 10px;">' + OVERLAY_TITLE + '</h2>' +
            '<p style="font-size:15px;color:#64748b;line-height:1.6;margin:0 0 8px;">' + OVERLAY_MSG + '</p>' +
            '<p id="tm-logout-countdown" style="font-size:13px;color:#94a3b8;margin:0;">Refreshing in ' + (REFRESH_MS/1000) + ' seconds...</p></div>';
        if (!document.getElementById('tm-logout-spin-style')) {
            var s = document.createElement('style');
            s.id = 'tm-logout-spin-style';
            s.textContent = '@keyframes tmSpin{to{transform:rotate(360deg)}}';
            document.head.appendChild(s);
        }
        (document.body || document.documentElement).appendChild(o);
    }

    function startCountdown() {
        var el = document.getElementById('tm-logout-countdown');
        if (!el) return;
        var sec = Math.ceil(REFRESH_MS / 1000);
        el.textContent = 'Refreshing in ' + sec + ' seconds...';
        if (countdownTimer) clearInterval(countdownTimer);
        countdownTimer = setInterval(function() {
            sec--;
            if (sec <= 0) {
                clearInterval(countdownTimer);
                countdownTimer = null;
                el.textContent = 'Refreshing now...';
                return;
            }
            el.textContent = 'Refreshing in ' + sec + ' second' + (sec === 1 ? '' : 's') + '...';
        }, 1000);
    }

    function hideTmLogoutOverlay() {
        var o = document.getElementById('tm-logout-overlay');
        if (o) o.remove();
        if (refreshTimer) { clearTimeout(refreshTimer); refreshTimer = null; }
        if (countdownTimer) { clearInterval(countdownTimer); countdownTimer = null; }
        apiCalledThisPage = false;
    }

    function scheduleRefresh() {
        if (refreshTimer) return;
        startCountdown();
        refreshTimer = setTimeout(function() {
            refreshTimer = null;
            window.location.reload();
        }, REFRESH_MS);
    }

    function detectLogoutReason() {
        var path = (window.location.pathname || '/').replace(/\/$/, '') || '/';
        var html = document.documentElement ? document.documentElement.innerHTML : '';
        var text = document.body ? (document.body.innerText || document.body.textContent || '') : '';
        var URL_PATHS = %s;
        var TEXT_SNIFFS = %s;
        var HTML_SNIFFS = %s;
        for (var i = 0; i < URL_PATHS.length; i++) {
            var p = (URL_PATHS[i] || '').replace(/\/$/, '');
            if (p && (path === p || path.indexOf(p + '/') === 0)) return 'url_path:' + URL_PATHS[i];
        }
        for (var j = 0; j < TEXT_SNIFFS.length; j++) {
            if (TEXT_SNIFFS[j] && (html.indexOf(TEXT_SNIFFS[j]) !== -1 || text.indexOf(TEXT_SNIFFS[j]) !== -1))
                return 'text_sniff:' + TEXT_SNIFFS[j];
        }
        for (var k = 0; k < HTML_SNIFFS.length; k++) {
            if (HTML_SNIFFS[k] && html.indexOf(HTML_SNIFFS[k]) !== -1) return 'html_sniff:' + HTML_SNIFFS[k].substring(0, 40);
        }
        return '';
    }

    function onLogoutDetected(reason) {
        console.log('[ToolsMandi] Logout detected:', reason);
        showTmLogoutOverlay();
        scheduleRefresh();
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
        var reason = detectLogoutReason();
        if (reason) {
            onLogoutDetected(reason);
        } else {
            hideTmLogoutOverlay();
        }
    }

    window.addEventListener('popstate', function() { setTimeout(checkLogoutAutomation, 300); });
    if (window.__tm_show_logout_overlay) {
        if (window.__tm_overlay_title) OVERLAY_TITLE = window.__tm_overlay_title;
        if (window.__tm_overlay_msg) OVERLAY_MSG = window.__tm_overlay_msg;
        if (window.__tm_refresh_seconds) REFRESH_MS = window.__tm_refresh_seconds * 1000;
        apiCalledThisPage = true;
        showTmLogoutOverlay();
        scheduleRefresh();
    }
    setTimeout(checkLogoutAutomation, 800);
    setInterval(checkLogoutAutomation, 3000);
`, cfg.Automation.Enabled, ld.ShowOverlay, refreshSec*1000, title, msg,
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
	return fmt.Sprintf(`<script>
(function() {
    var CREDIT_LABEL = '%s';
    var DISABLE_EXPORT = %v;
    var badge = document.createElement('div');
    badge.id = 'tm-limit-badge';
    badge.style.cssText = 'position:fixed;bottom:20px;right:20px;background:rgba(15,23,42,0.9);color:#e2e8f0;border:1px solid rgba(255,255,255,0.08);border-radius:14px;padding:10px 16px;font-family:sans-serif;font-size:13px;z-index:999999;backdrop-filter:blur(12px);box-shadow:0 8px 32px rgba(0,0,0,0.4);min-width:180px;';
    badge.innerHTML = '<div style="font-weight:700;font-size:11px;color:#64748b;letter-spacing:0.5px;margin-bottom:4px;">ToolsMandi</div><div id="tm-credit-text">Loading...</div>';
    function updateBadge() {
        fetch('/api/user-limits').then(function(r){ return r.json(); }).then(function(d) {
            if (!d.show_limit) { badge.style.display='none'; return; }
            var remaining = Math.max(0, d.credit_limit - d.credit_used);
            var color = remaining > 10 ? '#4ade80' : (remaining > 3 ? '#fb923c' : '#f87171');
            badge.querySelector('#tm-credit-text').innerHTML = '<span style="color:'+color+';font-weight:700;font-size:16px;">'+remaining+'</span> <span style="color:#64748b;">/ '+d.credit_limit+' '+CREDIT_LABEL+' left</span>';
        }).catch(function(){});
    }
    // Wait for body to be ready before appending (script is injected inside <head>)
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', function() {
            document.body.appendChild(badge);
            updateBadge();
            setInterval(updateBadge, 30000);
        });
    } else {
        document.body.appendChild(badge);
        updateBadge();
        setInterval(updateBadge, 30000);
    }
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

	// Connect to upstream (TLS or plain)
	var upstreamConn net.Conn
	var err error
	if strings.TrimSpace(accountProxy) != "" {
		ctx, cancel := context.WithTimeout(context.WithValue(r.Context(), proxyContextKey, accountProxy), 20*time.Second)
		upstreamConn, err = dialChrome(ctx, upstreamAddr)
		cancel()
	} else if useTLS {
		upstreamConn, err = tls.Dial("tcp", upstreamAddr, &tls.Config{ServerName: upstreamURL.Hostname()})
	} else {
		upstreamConn, err = net.Dial("tcp", upstreamAddr)
	}
	if err != nil {
		log.Printf("[WS] Upstream dial failed for %s: %v", upstreamAddr, err)
		if strings.Contains(err.Error(), "proxy dial") {
			renderProxyProblem(w, r)
			return
		}
		http.Error(w, "WebSocket upstream unavailable", http.StatusBadGateway)
		return
	}
	defer upstreamConn.Close()

	// Build HTTP/1.1 upgrade request
	var reqBuf bytes.Buffer
	upstreamPath := upstreamURL.RequestURI()
	reqBuf.WriteString(fmt.Sprintf("GET %s HTTP/1.1\r\n", upstreamPath))
	reqBuf.WriteString(fmt.Sprintf("Host: %s\r\n", upstreamURL.Host))
	for k, vv := range r.Header {
		kl := strings.ToLower(k)
		if kl == "host" || kl == "cookie" {
			continue
		} // override below
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

	// Forward 101 Switching Protocols to client
	var respBuf bytes.Buffer
	respBuf.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	for k, vv := range upstreamResp.Header {
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
	done := make(chan struct{}, 2)
	go func() { io.Copy(upConn, clConn); done <- struct{}{} }()
	go func() { io.Copy(clConn, upConn); done <- struct{}{} }()
	<-done
	log.Printf("[WS] WebSocket closed for %s", upstreamURL.Host)
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

	if cfg.CloudflareBypass {
		if strings.HasPrefix(path, "/cdn-cgi/") || path == "/cf-turnstile-bypass.js" {
			serveCloudflareBypass(w, path)
			return
		}
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

	// ── 1. Authenticate user (require ct_session cookie) ─────────────────────────
	currentUser, err := getAuthenticatedUser(r, cfg)
	isFavicon := strings.Contains(strings.ToLower(path), "favicon")
	if err != nil && !isFavicon {
		log.Printf("[AUTH] Denied path=%s ip=%s host=%s err=%v", path, realClientIP(r), requestHost(r), err)
		denyUser := ""
		if c, cErr := r.Cookie("ct_session"); cErr == nil && c.Value != "" {
			denyUser = "(invalid session)"
		}
		recordSecurityEvent(r, denyUser, "auth_denied", err.Error())
		// Check if request is browser navigation (not API)
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
	if isFavicon && err != nil {
		currentUser = "guest_favicon"
	}

	if (path == "/" || path == "") && cfg.HomePath != "" && cfg.HomePath != "/" && isDocumentNavigation(r) {
		http.Redirect(w, r, cfg.HomePath, http.StatusFound)
		return
	}

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
	} else {
		var found bool
		activeAcc, found = getSessionAssignedAccount(sessionToken)
		if !found {
			activeAcc, err = autoAssignNextAccount(sessionToken)
			if err != nil {
				renderNoActiveAccountsPage(w, cfg)
				return
			}
		}
	}

	if usesPanelAccountMode(cfg) && activeAcc.ID > 0 && isDocumentNavigation(r) && closersLogoutPath(r) {
		reason := "session_expired"
		if strings.Contains(strings.ToLower(path), "logout") {
			reason = "user_logout"
		}
		serveClosersLogout(w, r, cfg, activeAcc, sessionToken, reason)
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
	if strings.HasPrefix(path, "/cdn-proxy/") && cdnParsed != nil {
		upstreamURL.Scheme = cdnParsed.Scheme
		upstreamURL.Host = cdnParsed.Host
		upstreamURL.Path = "/" + strings.TrimPrefix(path, "/cdn-proxy/")
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

	var bodyReader io.Reader = r.Body
	if r.Body != nil && (r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch) {
		bodyBytes, readErr := io.ReadAll(r.Body)
		if readErr == nil {
			bodyReader = bytes.NewReader(bodyBytes)
		}
	}

	upstreamReq, err := http.NewRequestWithContext(
		context.WithValue(r.Context(), proxyContextKey, activeAcc.Proxy),
		r.Method, upstreamURL.String(), bodyReader,
	)
	if err != nil {
		http.Error(w, "Failed to build upstream request", http.StatusInternalServerError)
		return
	}
	if br, ok := bodyReader.(*bytes.Reader); ok {
		upstreamReq.ContentLength = int64(br.Len())
	}

	// Copy headers (skip Content-Length — recalc from buffered body)
	for k, vv := range r.Header {
		if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Cookie") {
			continue
		}
		for _, v := range vv {
			upstreamReq.Header.Add(k, v)
		}
	}
	if upstreamReq.ContentLength > 0 {
		upstreamReq.Header.Set("Content-Length", strconv.FormatInt(upstreamReq.ContentLength, 10))
	}

	// Set account cookie and user-agent — only closerscopy_session_ccv2 + XSRF-TOKEN from cookie.txt
	accountCookieStr := filterEssentialCookies(parseCookieFromDB(activeAcc.Cookie))
	if _, ok := dynamicUpstreamCookies.Load("xsrf-token"); ok {
		accountCookieStr = dropCookieNames(accountCookieStr, "XSRF-TOKEN")
	}
	accountCookieStr = applyDynamicCookies(accountCookieStr)
	// Never forward the browser jar. nginx already rejects a fat Cookie, and Apache
	// (LimitRequestFieldSize 8k) returns 400 if analytics/XSRF leftovers are appended.
	clientCookies := accountCookieStr
	// Only inject premium cookies when upstream is the target tool domain (not third-party CDNs)
	hostWithoutPort := upstreamURL.Host
	if h, _, err := net.SplitHostPort(upstreamURL.Host); err == nil {
		hostWithoutPort = h
	}
	targetRoot := targetParsed.Hostname()
	sendCookies := hostWithoutPort == targetRoot || strings.HasSuffix(hostWithoutPort, "."+targetRoot)
	if sendCookies {
		if clientCookies != "" {
			upstreamReq.Header.Set("Cookie", clientCookies)
		}
		injectUpstreamCsrf(upstreamReq, r, cfg)
	}

	// Anti-recloud: same premium account cookie from multiple /16 subnets (PHP mirror + real users)
	ss := normalizeSessionSecurity(cfg)
	if securityEnabled(cfg) && sendCookies && activeAcc.ID > 0 {
		if accountIPTracker.Check(activeAcc.ID, realClientIP(r), ss.AccountCookieProtection) {
			log.Printf("[SECURITY] Account cookie sharing | account=%s (ID:%d) ip=%s user=%s", activeAcc.Name, activeAcc.ID, realClientIP(r), currentUser)
			recordSecurityEvent(r, currentUser, "account_cookie_sharing", "account_id="+strconv.Itoa(activeAcc.ID))
			handleLogoutDetected(cfg, "account_cookie_sharing_recloud", activeAcc, currentUser, sessionToken)
			renderAccessDeniedPage(w, cfg)
			return
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
	if strings.HasPrefix(path, "/cdn-proxy/") && cdnParsed != nil {
		upstreamReq.Host = cdnParsed.Host
	} else if isExtraCDN && extraCDNIndex >= 0 {
		extraClean := strings.TrimPrefix(strings.TrimPrefix(cfg.ExtraCDNDomains[extraCDNIndex], "https://"), "http://")
		upstreamReq.Host = strings.Split(extraClean, "/")[0]
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
		upstreamReq.Header.Set("Referer", newReferer)
	} else {
		upstreamReq.Header.Set("Referer", targetBase+"/")
	}

	upstreamResp, err := httpClient.Do(upstreamReq)
	if err != nil {
		log.Printf("[PROXY] Upstream request failed for user '%s' path '%s': %v", currentUser, path, err)
		if strings.Contains(err.Error(), "proxy dial") {
			renderProxyProblem(w, r)
			return
		}
		if dbConnected {
			activeAcc, _ = switchToNextAccount(sessionToken, activeAcc.ID, activeAcc.Name, currentUser, "upstream_connection_error")
		}
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer upstreamResp.Body.Close()

	if usesPanelAccountMode(cfg) && activeAcc.ID > 0 && isDocumentNavigation(r) && closersUpstreamLogout(upstreamResp) {
		serveClosersLogout(w, r, cfg, activeAcc, sessionToken, "session_expired")
		return
	}

	absorbUpstreamSetCookies(upstreamResp.Header)
	// Mirror Laravel XSRF-TOKEN to browser (session cookie stays server-side)
	forwardAllowlistedSetCookies(w, r, upstreamResp.Header, cfg)

	// ── 7. Handle Set-Cookie and Location headers from upstream ──────────────────
	contentType := upstreamResp.Header.Get("Content-Type")
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
		if kLower == "location" {
			for _, v := range vv {
				newLoc := v
				for _, pair := range locationPairs {
					newLoc = strings.ReplaceAll(newLoc, pair[0], pair[1])
				}
				w.Header().Add(k, newLoc)
			}
			continue
		}
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}

	// Force disable browser cache for all dynamic/static/API responses
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

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
	isHTML := strings.Contains(contentType, "text/html")
	if isHTML {
		bodyBytes, err := decompressBody(upstreamResp)
		if err != nil {
			w.WriteHeader(upstreamResp.StatusCode)
			return
		}
		if usesPanelAccountMode(cfg) && activeAcc.ID > 0 && isDocumentNavigation(r) && closersLoggedOutHTML(bodyBytes) {
			serveClosersLogout(w, r, cfg, activeAcc, sessionToken, "session_expired")
			return
		}

		// Logout detection on HTML body — trigger automation + rotate account once per page
		logoutPageDetected := false
		if detected, reason := detectLogout(path, bodyBytes, cfg); detected {
			log.Printf("[LOGOUT] HTML detection | user=%s account=%s reason=%s", currentUser, activeAcc.Name, reason)
			handleLogoutDetected(cfg, reason, activeAcc, currentUser, sessionToken)
			logoutPageDetected = true
		}

		// Rewrite domain references
		pairs := buildDomainReplacements(publicScheme, publicHost, cfg)
		bodyBytes = rewriteBody(bodyBytes, pairs)
		if cfg.CloudflareBypass {
			bodyBytes = stripCloudflareChallengeHTML(bodyBytes, publicScheme, publicHost)
		}
		storeCsrfPlainFromHTML(bodyBytes)
		bodyBytes = resyncInlineCsrfTokens(bodyBytes)
		bodyBytes = injectLaravelFormCsrf(bodyBytes)

		// Remove CSP header (prevents our injected scripts)
		w.Header().Del("Content-Security-Policy")
		w.Header().Del("Content-Security-Policy-Report-Only")
		w.Header().Del("X-Frame-Options")
		if usesPanelAccountMode(cfg) {
			bodyBytes = injectDeviceHTML(bodyBytes)
		}

		injectStr := patcherScript(cfg)
		if usesPanelAccountMode(cfg) && strings.TrimSpace(currentUser) != "" {
			userJSON, _ := json.Marshal(currentUser)
			injectStr += `<style>.dropdown-menu[aria-labelledby="navbarDropdown"]{display:none!important;visibility:hidden!important;pointer-events:none!important}</style>` +
				`<script>(function(){var USER=` + string(userJSON) + `;function lock(){var btn=document.getElementById("navbarDropdown");if(btn){for(var i=0;i<btn.childNodes.length;i++){var n=btn.childNodes[i];if(n.nodeType===3&&n.nodeValue.trim()&&n.nodeValue.trim()!==USER)n.nodeValue=USER+" ";}if(btn.getAttribute("aria-expanded")!=="false")btn.setAttribute("aria-expanded","false");}document.querySelectorAll('.dropdown-menu[aria-labelledby="navbarDropdown"]').forEach(function(m){if(m.classList.contains("show"))m.classList.remove("show");});}function block(e){var t=e.target&&e.target.closest?e.target.closest("#navbarDropdown"):null;if(!t)return;e.preventDefault();e.stopPropagation();}["pointerdown","mousedown","mouseup","click","auxclick","keydown"].forEach(function(ev){document.addEventListener(ev,block,true);});lock();new MutationObserver(lock).observe(document.documentElement,{childList:true,subtree:true});})();</script>`
		}
		if !usesCookieFileMode(cfg) {
			injectStr += buildDomainCheckJS(cfg) + buildSecurityHeartbeatJS(cfg)
			if logoutPageDetected {
				injectStr = logoutOverlayImmediateScript(cfg) + injectStr
			}
		}
		if regexp.MustCompile(`(?i)<head`).Match(bodyBytes) {
			bodyBytes = regexp.MustCompile(`(?i)(<head[^>]*>)`).ReplaceAll(bodyBytes, []byte("${1}"+injectStr))
		} else {
			bodyBytes = regexp.MustCompile(`(?i)</head>`).ReplaceAll(bodyBytes, []byte(injectStr+"</head>"))
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
			pairs := buildDomainReplacements(publicScheme, publicHost, cfg)
			bodyBytes = rewriteBody(bodyBytes, pairs)
			if isRSC {
				w.Header().Set("Content-Type", "text/x-component")
			}
			w.WriteHeader(upstreamResp.StatusCode)
			w.Write(bodyBytes)
			return
		}
	}

	// ── 11. Pass through everything else ─────────────────────────────────────────
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

	// ── Access handler (OTT → session cookie) ────────────────────────────────────
	mux.HandleFunc("/api/device-bind", deviceBindHandler)
	mux.HandleFunc("/__tm_access_denied", serveAccessDeniedHTML)
	mux.HandleFunc("/tm-device-sw.js", serveDeviceSW)
	mux.HandleFunc("/access", accessHandler)

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

	// ── Security middleware wrapper ───────────────────────────────────────────────
	secureHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := loadConfig()
		if dropped := slimBrowserCookies(r); len(dropped) > 0 && isDocumentNavigation(r) {
			expireDroppedCookies(w, r, dropped, cfg)
		}
		// Production only: nginx may forward X-Forwarded-Proto:http for plain HTTP clients → redirect to HTTPS.
		// Skip on localhost / public_scheme=http — proxy listens HTTP-only; redirect causes ERR_SSL_PROTOCOL_ERROR.
		if r.Header.Get("X-Forwarded-Proto") == "http" && !isLocalDev(cfg) && strings.EqualFold(cfg.PublicScheme, "https") {
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
	server := &http.Server{
		Addr:           addr,
		Handler:        secureHandler,
		ReadTimeout:    120 * time.Second,
		WriteTimeout:   120 * time.Second,
		IdleTimeout:    180 * time.Second,
		MaxHeaderBytes: 4 << 20, // 4MB — accept the jar nginx forwards, then slim it
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("[SERVER] Fatal: %v", err)
	}
}
