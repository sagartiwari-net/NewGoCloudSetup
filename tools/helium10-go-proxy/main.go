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
	WebsiteID  int    `json:"website_id"`
	// BypassAuth: bypasses database user authentication and loads cookie.txt directly (useful for testing without security)
	BypassAuth bool `json:"bypass_auth"`
	// InjectCSS: raw CSS injected into every HTML response (hide account menu, etc.)
	InjectCSS string `json:"inject_css"`
}

const defaultConfigFile = "config.json"

func configFile() string {
	if path := strings.TrimSpace(os.Getenv("H10_CONFIG")); path != "" {
		return path
	}
	return defaultConfigFile
}

var (
	creditHitCache   sync.Map
	currentConfig    Config
	configModTime    time.Time
	currentWebsiteID int  = 1
	dbConnected      bool = false
	defaultConfig         = Config{
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
	cfg := loadConfig()
	if cfg.WebsiteID > 0 {
		currentWebsiteID = cfg.WebsiteID
		log.Printf("[DB] Using config website_id = %d (public_host=%s)", currentWebsiteID, publicHost)
	}

	if !dbConnected {
		if currentWebsiteID <= 0 {
			currentWebsiteID = 1
		}
		log.Printf("[LOCAL] Running in Standalone/Local mode, website_id = %d", currentWebsiteID)
		return
	}
	if publicHost == "" {
		if currentWebsiteID <= 0 {
			currentWebsiteID = 1
			log.Printf("[DB] public_host empty — website_id = 1")
		}
		return
	}
	var wid int
	err := db.QueryRow("SELECT id FROM ahrefs_websites WHERE domain = ? OR domain = ?", publicHost, strings.TrimPrefix(publicHost, "www.")).Scan(&wid)
	if err == sql.ErrNoRows {
		if currentWebsiteID > 0 {
			log.Printf("[DB] ⚠️ Domain '%s' not found — keeping config website_id = %d", publicHost, currentWebsiteID)
			return
		}
		log.Printf("[DB] ⚠️ Domain '%s' not registered in ahrefs_websites! Using website_id = 1", publicHost)
		currentWebsiteID = 1
	} else if err != nil {
		if currentWebsiteID > 0 {
			log.Printf("[DB] ⚠️ Domain lookup error (%v) — keeping config website_id = %d", err, currentWebsiteID)
			return
		}
		log.Printf("[DB] ⚠️ Error querying website_id for '%s': %v. Using website_id = 1", publicHost, err)
		currentWebsiteID = 1
	} else {
		if currentWebsiteID > 0 && currentWebsiteID != wid {
			log.Printf("[DB] Domain '%s' maps to id=%d but config website_id=%d — using config value", publicHost, wid, currentWebsiteID)
		} else {
			currentWebsiteID = wid
			log.Printf("[DB] Resolved website_id = %d for domain '%s' ✅", currentWebsiteID, publicHost)
		}
	}
}

func loadConfig() Config {
	path := configFile()
	info, err := os.Stat(path)
	if err != nil {
		return currentConfig
	}
	if !info.ModTime().After(configModTime) {
		return currentConfig
	}
	data, err := os.ReadFile(path)
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
	log.Printf("[CONFIG] Reloaded from %s ✅ (tool: %s, target: %s)", path, cfg.ToolName, cfg.TargetURL)
	return currentConfig
}

// ── DATABASE SYSTEM ───────────────────────────────────────────────────────────

var db *sql.DB

func initDB(cfg Config) {
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
	ID        int
	Name      string
	Cookie    string
	UserAgent string
	Proxy     string
	ShowLimit bool
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

// loadLocalCookieAccount reads cookie.txt on every request (hot-reload).
func loadLocalCookieAccount(cfg Config) (ToolAccount, error) {
	path := cfg.CookieFile
	if strings.TrimSpace(path) == "" {
		path = "cookie.txt"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolAccount{}, err
	}
	parsedCookie := parseCookieFromDB(string(data))
	if strings.TrimSpace(parsedCookie) == "" {
		return ToolAccount{}, fmt.Errorf("%s is empty after parse", path)
	}
	return ToolAccount{
		ID:        1,
		Name:      "Local Standalone Account",
		Cookie:    parsedCookie,
		UserAgent: cfg.UserAgent,
		Proxy:     "",
		ShowLimit: false,
	}, nil
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

func getAuthenticatedUser(r *http.Request, cfg Config) (string, error) {
	if usesPanelAccountMode(cfg) {
		return panelSessionUsername(r)
	}
	if cfg.BypassAuth || !dbConnected {
		return "local_dev", nil
	}
	cookie, err := r.Cookie("ct_session")
	if err != nil {
		return "", err
	}
	sessionToken := cookie.Value
	if sessionToken == "" {
		return "", fmt.Errorf("empty session token")
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
		return "", fmt.Errorf("db session lookup: %w", err)
	}
	if time.Now().After(expiresAt) {
		_, _ = db.Exec("DELETE FROM ahrefs_sessions WHERE session_token = ? AND website_id = ?", sessionToken, currentWebsiteID)
		return "", fmt.Errorf("session expired")
	}
	return username, nil
}

// ── AUTH HANDSHAKE HANDLER (aMemberPro → Proxy) ──────────────────────────────

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
		log.Printf("[HANDSHAKE] ❌ HMAC failed for user=%s website_id=%d secret_len=%d", payload.Username, currentWebsiteID, len(dbSecretKey))
		http.Error(w, "Forbidden: Invalid signature", http.StatusForbidden)
		return
	}
	if time.Now().Unix()-payload.Timestamp > 300 {
		log.Printf("[HANDSHAKE] ❌ timestamp expired user=%s ts=%d now=%d", payload.Username, payload.Timestamp, time.Now().Unix())
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

	redirectURL := fmt.Sprintf("%s://%s/access?user=%s&token=%s",
		cfg.PublicScheme, cfg.PublicHost,
		url.QueryEscape(payload.Username), url.QueryEscape(ott),
	)
	log.Printf("[HANDSHAKE] ✅ OTT generated for user=%s website_id=%d redirect=%s", payload.Username, currentWebsiteID, redirectURL)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","redirect_url":%q}`, redirectURL)
}

// ── ACCESS HANDLER (OTT → Session Cookie) ────────────────────────────────────

func accessHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	if usesPanelAccountMode(cfg) {
		servePanelAccess(w, r, cfg)
		return
	}
	clientIP := realClientIP(r)
	log.Printf("[ACCESS] hit path=%s query=%s ip=%s website_id=%d db=%v", r.URL.Path, r.URL.RawQuery, clientIP, currentWebsiteID, dbConnected)

	if !dbConnected {
		log.Printf("[ACCESS] ❌ DB offline — cannot redeem OTT")
		renderAccessDeniedPage(w, cfg)
		return
	}
	username := r.URL.Query().Get("user")
	token := r.URL.Query().Get("token")
	if username == "" || token == "" {
		log.Printf("[ACCESS] ❌ missing user/token (user_empty=%v token_empty=%v) — open via Member Area Access button", username == "", token == "")
		renderAccessDeniedPage(w, cfg)
		return
	}
	var dbUsername, dbClientIP string
	var expiresAt time.Time
	err := db.QueryRow(
		"SELECT username, client_ip, expires_at FROM ahrefs_tokens WHERE token = ? AND website_id = ?",
		token, currentWebsiteID,
	).Scan(&dbUsername, &dbClientIP, &expiresAt)
	if err == sql.ErrNoRows {
		// Helpful: check if token exists under another website_id
		var otherWID int
		_ = db.QueryRow("SELECT website_id FROM ahrefs_tokens WHERE token = ? LIMIT 1", token).Scan(&otherWID)
		if otherWID > 0 {
			log.Printf("[ACCESS] ❌ token found under website_id=%d but proxy is using website_id=%d (mismatch)", otherWID, currentWebsiteID)
		} else {
			log.Printf("[ACCESS] ❌ token not found for user=%s website_id=%d (expired/used/wrong secret handshake?)", username, currentWebsiteID)
		}
		renderAccessDeniedPage(w, cfg)
		return
	}
	if err != nil {
		log.Printf("[ACCESS] ❌ token lookup error: %v", err)
		renderAccessDeniedPage(w, cfg)
		return
	}
	if time.Now().After(expiresAt) {
		_, _ = db.Exec("DELETE FROM ahrefs_tokens WHERE token = ? AND website_id = ?", token, currentWebsiteID)
		log.Printf("[ACCESS] ❌ token expired for user=%s (expires_at=%s)", username, expiresAt.Format(time.RFC3339))
		renderAccessDeniedPage(w, cfg)
		return
	}
	if dbUsername != username {
		log.Printf("[ACCESS] ❌ username mismatch query=%s token_user=%s", username, dbUsername)
		renderAccessDeniedPage(w, cfg)
		return
	}
	_, _ = db.Exec("DELETE FROM ahrefs_tokens WHERE token = ? AND website_id = ?", token, currentWebsiteID)

	// Generate session token
	sessionBytes := make([]byte, 32)
	if _, err := rand.Read(sessionBytes); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	sessionToken := hex.EncodeToString(sessionBytes)
	var sessionDuration int
	err = db.QueryRow("SELECT COALESCE(session_duration, 120) FROM ahrefs_websites WHERE id = ?", currentWebsiteID).Scan(&sessionDuration)
	if err != nil {
		sessionDuration = cfg.SessionDurationMinutes
	}
	sessionExpiry := time.Now().Add(time.Duration(sessionDuration) * time.Minute)

	_, err = db.Exec(
		"INSERT INTO ahrefs_sessions (session_token, username, client_ip, expires_at, website_id) VALUES (?,?,?,?,?)",
		sessionToken, username, dbClientIP, sessionExpiry, currentWebsiteID,
	)
	if err != nil {
		log.Printf("[ACCESS] ❌ session insert failed: %v", err)
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

	isHTTPS := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{
		Name:     "ct_session",
		Value:    sessionToken,
		Path:     "/",
		Expires:  sessionExpiry,
		HttpOnly: true,
		Secure:   isHTTPS,
		// Lax: required so member-area → /access cross-site navigation can set/use the session cookie
		SameSite: http.SameSiteLaxMode,
	})
	homePath := cfg.HomePath
	if homePath == "" {
		homePath = "/"
	}
	log.Printf("[ACCESS] ✅ session OK user=%s website_id=%d → %s", username, currentWebsiteID, homePath)
	http.Redirect(w, r, homePath, http.StatusFound)
}

// ── USER LIMITS API ───────────────────────────────────────────────────────────

func userLimitsAPIHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
	cfg := loadConfig()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	sessionToken := ""
	if c, err := r.Cookie("ct_session"); err == nil {
		sessionToken = c.Value
	}
	reason := r.URL.Query().Get("reason")
	if reason == "" {
		reason = "client-side watchdog trigger"
	}

	if usesPanelAccountMode(cfg) {
		if sessionToken == "" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"unauthorized"}`)
			return
		}
		username, err := panelSessionUsername(r)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"unauthorized"}`)
			return
		}
		activeAcc, err := loadPanelSessionAccount(cfg, sessionToken)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"error":"no_active_accounts"}`)
			return
		}
		nextAcc, switchErr := panelSwitchAccount(cfg, sessionToken, activeAcc.ID, activeAcc.Name, username, reason)
		if switchErr != nil {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"status":"waiting","account":"%s","id":%d,"message":%q}`, activeAcc.Name, activeAcc.ID, switchErr.Error())
			return
		}
		fmt.Fprintf(w, `{"status":"ok","switched_to":"%s","id":%d}`, nextAcc.Name, nextAcc.ID)
		return
	}

	if !dbConnected {
		fmt.Fprint(w, `{"status":"ok","switched_to":"Local Dev","id":1}`)
		return
	}
	if sessionToken == "" {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"unauthorized"}`)
		return
	}
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
	var currentUser string
	err := db.QueryRow("SELECT username FROM ahrefs_sessions WHERE session_token = ? AND website_id = ?", sessionToken, currentWebsiteID).Scan(&currentUser)
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

// ── URL REWRITING HELPERS ─────────────────────────────────────────────────────

// heliumPathHosts maps proxy path prefixes to dedicated Helium upstream hosts.
// Extension + SPA bake these as __H10_PROXY_ORIGIN__/research-tools etc.
func heliumPathHosts() [][2]string {
	return [][2]string{
		{"/research-tools/", "research-tools.helium10.com"},
		{"/control-center/", "control-center.helium10.com"},
	}
}

func matchHeliumPathHost(path string) (host, strippedPath string, ok bool) {
	for _, ph := range heliumPathHosts() {
		prefix, upstream := ph[0], ph[1]
		if strings.HasPrefix(path, prefix) {
			rest := strings.TrimPrefix(path, prefix)
			return upstream, "/" + rest, true
		}
	}
	return "", "", false
}

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
	for _, ph := range heliumPathHosts() {
		prefix, host := strings.TrimSuffix(ph[0], "/"), ph[1]
		pairs = append(pairs, [2]string{"https://" + host, publicBase + prefix})
		pairs = append(pairs, [2]string{"http://" + host, publicBase + prefix})
	}
	// Also replace raw hostnames to handle Javascript comparisons (e.g. location.host == 'members.helium10.com')
	if targetParsed != nil {
		pairs = append(pairs, [2]string{targetParsed.Host, publicHost})
	}
	for i, extra := range cfg.ExtraCDNDomains {
		extra = strings.TrimPrefix(strings.TrimPrefix(extra, "https://"), "http://")
		extra = strings.Split(extra, "/")[0]
		pairs = append(pairs, [2]string{extra, fmt.Sprintf("%s/extra-cdn-%d", publicHost, i)})
	}
	for _, ph := range heliumPathHosts() {
		prefix, host := strings.TrimSuffix(ph[0], "/"), ph[1]
		pairs = append(pairs, [2]string{host, publicHost + prefix})
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
	for _, ph := range heliumPathHosts() {
		prefix, host := strings.TrimSuffix(ph[0], "/"), ph[1]
		extraCDNJS.WriteString(fmt.Sprintf(`["https://%s",%q],["http://%s",%q],`, host, prefix, host, prefix))
	}
	extraCDNJS.WriteString("]")

	return fmt.Sprintf(`<script>
(function() {
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
            over.style.cssText = 'position:fixed;inset:0;background:#eef3f8;color:#0f172a;z-index:99999999;display:flex;flex-direction:column;align-items:center;justify-content:center;font-family:system-ui,-apple-system,Segoe UI,sans-serif;gap:16px;padding:24px;';
            over.innerHTML = '<div style="width:78px;height:78px;border-radius:50%;background:conic-gradient(#3b82f6 0 70deg,#e7eef8 70deg 360deg);animation:tmspin .9s linear infinite;display:grid;place-items:center;"><div style="width:64px;height:64px;border-radius:50%;background:#fff;display:grid;place-items:center;font-size:26px;">🔒</div></div><div style="width:min(440px,92vw);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:36px;text-align:center;"><h1 style="font-size:24px;font-weight:800;margin:0 0 10px;letter-spacing:-.02em;">Reconnecting...</h1><p style="margin:0;color:#64748b;font-size:15px;line-height:1.55;">Please wait while we switch your secure session.</p></div><style>@keyframes tmspin{to{transform:rotate(360deg)}}</style>';
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

    // ── XHR patch ──
    var xo = XMLHttpRequest.prototype.open;
    XMLHttpRequest.prototype.open = function(m, u) {
        return xo.apply(this, [m, patchURL(u)].concat(Array.prototype.slice.call(arguments, 2)));
    };

    // ── Fetch patch ──
    var fo = window.fetch;
    window.fetch = function(inp, init) {
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
})();
</script>`,
		targetHost, cdnHost, homePath, blockedListJS.String(), extraCDNJS.String(), triggerChecks.String(),
		func() string {
			if len(cfg.WatchdogTriggers) > 0 {
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

// isPublicAssetPath allows CDN/static files without ct_session.
// <img src="/extra-cdn-…/Flag-….svg"> often omits cookies → previously 401.
func isPublicAssetPath(path string) bool {
	if strings.HasPrefix(path, "/extra-cdn-") || strings.HasPrefix(path, "/cdn-proxy/") {
		return true
	}
	lower := strings.ToLower(path)
	for _, ext := range []string{".svg", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".woff", ".woff2", ".ttf", ".eot", ".otf", ".map"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	path := r.URL.Path

	// ── 0. Skip proxy for admin API routes ──────────────────────────────────────
	if strings.HasPrefix(path, "/api/auth-handshake") ||
		strings.HasPrefix(path, "/api/device-bind") ||
		path == "/tm-device-sw.js" ||
		strings.HasPrefix(path, "/api/user-limits") ||
		strings.HasPrefix(path, "/api/rotate-session") ||
		strings.HasPrefix(path, "/access") ||
		path == "/ext-install" ||
		path == "/extension.zip" ||
		path == "/user/logout" {
		return // These are handled by their own handlers
	}

	// ── 1. Authenticate user (require ct_session cookie) ─────────────────────────
	isFavicon := strings.Contains(strings.ToLower(path), "favicon")
	isPublicAsset := isPublicAssetPath(path)
	currentUser, authErr := getAuthenticatedUser(r, cfg)
	if authErr != nil && !isFavicon && !isPublicAsset {
		_, hasSess := r.Cookie("ct_session")
		log.Printf("[AUTH] ❌ denied path=%s host=%s err=%v website_id=%d has_ct_session=%v",
			path, r.Host, authErr, currentWebsiteID, hasSess == nil)
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
	if (isFavicon || isPublicAsset) && authErr != nil {
		currentUser = "public_asset"
	}

	// ── 1a. Disk CDN cache (static/extra-cdn only) ────────────────────────────────
	cdnKey := cdnCacheKey(r)
	if serveCachedCDN(w, r) {
		return
	}
	if cdnKey != "" {
		defer completeCDNFlight(cdnKey)
	}

	// ── 1b. Root → configured home (e.g. /black-box/products) ─────────────────────
	if (path == "/" || path == "") && cfg.HomePath != "" && cfg.HomePath != "/" {
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
	} else if cfg.BypassAuth || !dbConnected {
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
		var err error
		activeAcc, err = loadLocalCookieAccount(cfg)
		if err != nil {
			log.Printf("[LOCAL] Failed to read local cookie file: %v", err)
			renderNoActiveAccountsPage(w, cfg)
			return
		}
	} else {
		var found bool
		activeAcc, found = getSessionAssignedAccount(sessionToken)
		if !found {
			var assignErr error
			if sessionToken == "" && isPublicAsset {
				// CDN/static assets (SVG flags etc.) often load without cookies.
				activeAcc, assignErr = selectActiveAccount()
				if assignErr != nil {
					activeAcc = ToolAccount{ID: 0, Name: "public-cdn", Cookie: "", UserAgent: cfg.UserAgent}
					assignErr = nil
				}
			} else {
				activeAcc, assignErr = autoAssignNextAccount(sessionToken)
			}
			if assignErr != nil {
				renderNoActiveAccountsPage(w, cfg)
				return
			}
		}
	}

	// ── 4. Credit/Limit check — DISABLED (bypass_auth mode) ─────────────────────
	// Limits are not enforced in standalone/bypass mode.

	// ── 4b. Login wall → next account (panel) ─────────────────────────────────────
	if usesPanelAccountMode(cfg) && !cfg.BypassAuth && currentUser != "" {
		home := cfg.HomePath
		if home == "" {
			home = "/dashboard"
		}
		// Extension "Log in" opens /user/signup?type=chrome-extension (or signin).
		// Stay in-tool and land on home; the extension SW then hits /extension/check-login
		// (cookie bridge attaches ct_session) to finish login.
		if path == "/user/signup" || (h10IsLoginPath(path) && r.URL.Query().Get("type") == "chrome-extension") {
			http.Redirect(w, r, home, http.StatusFound)
			return
		}
		if path == "/__h10_switch" {
			serveH10AccountSwitch(w, r, cfg, sessionToken, currentUser, activeAcc, "h10_login_wall")
			return
		}
		if h10LoginDocument(r, path) {
			serveH10AccountSwitch(w, r, cfg, sessionToken, currentUser, activeAcc, "h10_login_path")
			return
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

	// Helium research-tools / control-center (Xray Keywords, Cerebro APIs, etc.)
	pathHost := ""
	isPathHost := false
	if !isExtraCDN {
		if host, stripped, ok := matchHeliumPathHost(path); ok {
			upstreamURL.Scheme = "https"
			upstreamURL.Host = host
			upstreamURL.Path = stripped
			pathHost = host
			isPathHost = true
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
	// Only send Cookie header to helium10.com domains
	hostWithoutPort := upstreamURL.Host
	if h, _, err := net.SplitHostPort(upstreamURL.Host); err == nil {
		hostWithoutPort = h
	}
	if strings.HasSuffix(hostWithoutPort, "helium10.com") {
		if clientCookies != "" {
			upstreamReq.Header.Set("Cookie", clientCookies)
		}
	} else {
		upstreamReq.Header.Del("Cookie")
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
	} else if isPathHost && pathHost != "" {
		upstreamReq.Host = pathHost
	}

	// Remove proxy headers
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
		for _, ph := range heliumPathHosts() {
			newReferer = strings.ReplaceAll(newReferer, ph[0], "/")
		}
		upstreamReq.Header.Set("Referer", newReferer)
	} else {
		upstreamReq.Header.Set("Referer", targetBase+"/")
	}
	// Research-tools APIs expect a members Origin when called from the extension SW.
	if isPathHost {
		if origOrigin == "" || strings.HasPrefix(origOrigin, "chrome-extension://") {
			upstreamReq.Header.Set("Origin", targetBase)
		}
	}

	upstreamResp, err := httpClient.Do(upstreamReq)
	if err != nil {
		log.Printf("[PROXY] Upstream request failed for user '%s' path '%s': %v", currentUser, path, err)
		maybeLogExtensionEvent(cfg, r, currentUser, path, http.StatusBadGateway)
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
	maybeLogExtensionEvent(cfg, r, currentUser, path, upstreamResp.StatusCode)

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

	// Browser cache: allow 1-day for safe CDN static; keep no-store for everything else.
	canBrowserCache := cdnKey != "" && upstreamResp.StatusCode == http.StatusOK &&
		upstreamResp.Header.Get("Set-Cookie") == "" &&
		!strings.Contains(strings.ToLower(contentType), "text/html") &&
		!strings.Contains(strings.ToLower(contentType), "json")
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

		// Rewrite domain references
		pairs := buildDomainReplacements(publicScheme, publicHost, cfg)
		bodyBytes = rewriteBody(bodyBytes, pairs)

		// Remove CSP header (prevents our injected scripts)
		w.Header().Del("Content-Security-Policy")
		w.Header().Del("Content-Security-Policy-Report-Only")
		w.Header().Del("X-Frame-Options")
		if usesPanelAccountMode(cfg) && !cfg.BypassAuth && currentUser != "" && h10HTMLLooksLikeLogin(bodyBytes) {
			serveH10AccountSwitch(w, r, cfg, sessionToken, currentUser, activeAcc, "h10_login_html")
			return
		}
		// Device-lock CSS (html{visibility:hidden}) causes a blank white page until
		// /api/device-bind succeeds. Skip entirely in local bypass_auth mode.
		if usesPanelAccountMode(cfg) && !cfg.BypassAuth {
			bodyBytes = injectDeviceHTML(bodyBytes)
		}

		// Inject our patcher script before </head> (no limit widgets)
		injectStr := ""
		if usesPanelAccountMode(cfg) && !cfg.BypassAuth {
			injectStr += h10LoginWatchScript(cfg)
		}
		injectStr += patcherScript(cfg)
		if strings.TrimSpace(cfg.InjectCSS) != "" {
			injectStr += "<style>" + cfg.InjectCSS + "</style>"
		}
		// Hide Helium10 header account chip (avatar initials + chevron). Hashed
		// styled-components classes change often, so also hide by structure.
		injectStr += `<script>(function(){function hideH10AccountChip(){document.querySelectorAll('svg[data-icon="chevron-up"],svg[data-icon="chevron-down"]').forEach(function(svg){var node=svg.parentElement;for(var i=0;i<6&&node;i++){var t=(node.textContent||"").replace(/\s+/g,"");if(/^[A-Z]{1,3}$/.test(t)){var hide=node;for(var j=0;j<2&&hide.parentElement;j++)hide=hide.parentElement;hide.style.setProperty("display","none","important");return;}node=node.parentElement;}});}hideH10AccountChip();new MutationObserver(hideH10AccountChip).observe(document.documentElement,{childList:true,subtree:true});})();</script>`
		bodyBytes = regexp.MustCompile(`(?i)</head>`).ReplaceAll(bodyBytes, []byte(injectStr+"</head>"))

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(upstreamResp.StatusCode)
		w.Write(bodyBytes)
		return
	}

	// ── 10. For JSON/JS/CSS/binary: rewrite and stream ────────────────────────────
	isRewritable := strings.Contains(contentType, "javascript") || strings.Contains(contentType, "application/json") || strings.Contains(contentType, "text/css")
	if isRewritable {
		bodyBytes, err := decompressBody(upstreamResp)
		if err == nil {
			pairs := buildDomainReplacements(publicScheme, publicHost, cfg)
			bodyBytes = rewriteBody(bodyBytes, pairs)
			if canBrowserCache {
				storeCDNCache(r, upstreamResp.StatusCode, contentType, bodyBytes)
			}
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(bodyBytes)))
			w.WriteHeader(upstreamResp.StatusCode)
			w.Write(bodyBytes)
			return
		}
	}

	// ── 11. Pass through everything else (SVG/PNG/fonts…) ─────────────────────────
	// Content-Encoding was stripped above — must decompress or browsers get gzip
	// bytes labeled as image/svg+xml and icons break.
	bodyBytes, err := decompressBody(upstreamResp)
	if err != nil {
		w.WriteHeader(upstreamResp.StatusCode)
		return
	}
	if canBrowserCache {
		storeCDNCache(r, upstreamResp.StatusCode, contentType, bodyBytes)
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(bodyBytes)))
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(upstreamResp.StatusCode)
	w.Write(bodyBytes)
}

// ── CORS MIDDLEWARE ───────────────────────────────────────────────────────────

func withCORS(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
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
	initDB(cfg)
	resolveWebsiteID(cfg.PublicHost)
	startDailyResetCron()
	startCDNCacheSweep()

	mux := http.NewServeMux()

	// ── API routes ────────────────────────────────────────────────────────────────
	mux.HandleFunc("/api/auth-handshake", withCORS(authHandshakeHandler))
	mux.HandleFunc("/api/user-limits", withCORS(userLimitsAPIHandler))
	mux.HandleFunc("/api/rotate-session", withCORS(rotateSessionHandler))

	// ── Access handler (OTT → session cookie) ────────────────────────────────────
	mux.HandleFunc("/api/device-bind", deviceBindHandler)
	mux.HandleFunc("/__tm_access_denied", serveAccessDeniedHTML)
	mux.HandleFunc("/tm-device-sw.js", serveDeviceSW)
	mux.HandleFunc("/access", accessHandler)

	// ── Extension install (multi-tenant ZIP bound to this host) ───────────────────
	mux.HandleFunc("/ext-install", extensionInstallPageHandler)
	mux.HandleFunc("/extension.zip", extensionZipHandler)

	// ── Logout ────────────────────────────────────────────────────────────────────
	mux.HandleFunc("/user/logout", func(w http.ResponseWriter, r *http.Request) {
		cfg := loadConfig()
		if c, err := r.Cookie("ct_session"); err == nil {
			panelSess.Delete(c.Value)
			if usesPanelAccountMode(cfg) {
				if pdb, err := openPanelDB(cfg); err == nil {
					_, _ = pdb.Exec(`DELETE FROM live_sessions WHERE session_token=?`, c.Value)
				}
			}
			if dbConnected && db != nil {
				_, _ = db.Exec("DELETE FROM ahrefs_sessions WHERE session_token = ? AND website_id = ?", c.Value, currentWebsiteID)
			}
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "ct_session",
			Value:    "",
			Path:     "/",
			Expires:  time.Unix(0, 0),
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   cookieSecure(r, cfg),
			SameSite: http.SameSiteLaxMode,
		})
		if usesPanelAccountMode(cfg) {
			renderAccessDeniedPage(w, cfg)
			return
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
		// Force HTTPS redirect (when running behind reverse proxy with X-Forwarded-Proto)
		if r.Header.Get("X-Forwarded-Proto") == "http" && !usesPanelAccountMode(cfg) && strings.EqualFold(cfg.PublicScheme, "https") {
			target := "https://" + r.Host + r.URL.RequestURI()
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		}
		// Security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		mux.ServeHTTP(w, r)
	})

	addr := ":" + cfg.Port
	log.Printf("✅ Generic Tool Proxy listening on %s", addr)
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
