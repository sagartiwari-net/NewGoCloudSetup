package main

import (
	"html"
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
	"github.com/klauspost/compress/zstd"
	utls "github.com/refraction-networking/utls"
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
	// CookieDomainSuffix: only forward upstream cookies to hosts ending with this (e.g. "selleramp.com")
	CookieDomainSuffix string `json:"cookie_domain_suffix"`
	// InjectCSS: raw CSS injected into every HTML response (e.g. hide nav bar)
	InjectCSS string `json:"inject_css"`
	// UserAgent override
	UserAgent string `json:"user_agent"`
	// CookieFile: path to cookie.txt file (legacy, optional)
	CookieFile string `json:"cookie_file"`
	// PanelDB is the local panel database. When set, Open comes from the panel access link.
	PanelDB string `json:"panel_db"`
	WebsiteID  int    `json:"website_id"`
	// BypassAuth: bypasses database user authentication and loads cookie.txt directly (useful for testing without security)
	BypassAuth bool `json:"bypass_auth"`
	// Replacements: multiple find/replace pairs for HTML + JSON + live DOM text.
	// Example: [{"search":"DigitaVision","replace":"ToolsMandi"},{"search":"Digitavision","replace":"ToolsMandi"}]
	Replacements []ReplacementPair `json:"replacements"`
}

// ReplacementPair is one config-driven text substitution.
type ReplacementPair struct {
	Search  string `json:"search"`
	Replace string `json:"replace"`
}

const defaultConfigFile = "config.json"

func configFile() string {
	if path := strings.TrimSpace(os.Getenv("ARTISTLY_CONFIG")); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("SELLTHETREND_CONFIG")); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("STT_CONFIG")); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("VISTACREATE_CONFIG")); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("VISTA_CONFIG")); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("WOORANK_CONFIG")); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("STORYBASE_CONFIG")); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("PIKTOCHART_CONFIG")); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("ILOVEPDF_CONFIG")); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("KEYWORDTOOL_CONFIG")); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("COPYWRITELY_CONFIG")); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("CANVA_CONFIG")); path != "" {
		return path
	}
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
	cachedAPIUID     string
	cachedAPIToken   string
	apiCredMu        sync.RWMutex
	defaultConfig    = Config{
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
		Replacements:           []ReplacementPair{},
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
		p = strings.TrimSuffix(strings.TrimSpace(p), "/")
		if p != "" && path == p {
			return true
		}
	}
	for _, prefix := range cfg.BlockedPrefixes {
		prefix = strings.TrimSuffix(strings.TrimSpace(prefix), "/")
		if prefix == "" {
			continue
		}
		// Exact prefix OR prefix/… — avoids "/member" matching "/membership"
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
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

func captureAPICredentials(body []byte) {
	text := string(body)
	token := ""
	uid := ""
	// Next.js RSC escaped JSON: apiToken\":\"....\"
	if i := strings.Index(text, `apiToken\":\"`); i >= 0 {
		rest := text[i+len(`apiToken\":\"`):]
		if j := strings.Index(rest, `\"`); j > 0 {
			token = rest[:j]
		}
	}
	if token == "" {
		if i := strings.Index(text, `"apiToken":"`); i >= 0 {
			rest := text[i+len(`"apiToken":"`):]
			if j := strings.Index(rest, `"`); j > 0 {
				token = rest[:j]
			}
		}
	}
	if token == "" {
		if i := strings.Index(text, `"api_token":"`); i >= 0 {
			rest := text[i+len(`"api_token":"`):]
			if j := strings.Index(rest, `"`); j > 0 {
				token = rest[:j]
			}
		}
	}
	if i := strings.Index(text, `userId\":\"`); i >= 0 {
		rest := text[i+len(`userId\":\"`):]
		if j := strings.Index(rest, `\"`); j > 0 {
			uid = rest[:j]
		}
	}
	if uid == "" {
		if i := strings.Index(text, `"userId":"`); i >= 0 {
			rest := text[i+len(`"userId":"`):]
			if j := strings.Index(rest, `"`); j > 0 {
				uid = rest[:j]
			}
		}
	}
	if token == "" {
		return
	}
	apiCredMu.Lock()
	cachedAPIToken = token
	if uid != "" {
		cachedAPIUID = uid
	}
	apiCredMu.Unlock()
	log.Printf("[API-AUTH] cached credentials uid=%s token_len=%d", uid, len(token))
}

func ensureAPIAuthHeaders(req *http.Request) {
	if !strings.HasPrefix(req.URL.Path, "/api/v2/") && !strings.HasPrefix(req.URL.Path, "/api/v1/") {
		return
	}
	apiCredMu.RLock()
	token := cachedAPIToken
	uid := cachedAPIUID
	apiCredMu.RUnlock()
	if token == "" {
		return
	}
	if strings.TrimSpace(req.Header.Get("X-API-TOKEN")) == "" {
		req.Header.Set("X-API-TOKEN", token)
	}
	if strings.TrimSpace(req.Header.Get("X-UID")) == "" && uid != "" {
		req.Header.Set("X-UID", uid)
	}
}

func sanitizeCookieHeader(s string) string {
	return strings.TrimSpace(strings.NewReplacer("\r", "", "\n", "", "\x00", "", "\t", " ").Replace(s))
}

type browserCookieEntry struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	HostOnly bool   `json:"hostOnly"`
}

// parseCookieFromDB accepts:
//  1. Plain Cookie header: "ASI=...; CID=..."
//  2. Browser export JSON array: [ {"name":"ASI","value":"..."}, ... ]
//  3. GoAuto wrap: {"referer":"...","includedFormats":["cookies"],"cookies":[ ... ]}
func parseCookieFromDB(raw string) string {
	raw = sanitizeCookieHeader(raw)
	if raw == "" {
		return ""
	}
	trimmed := strings.TrimSpace(raw)

	// GoAuto object with "cookies" array
	if strings.HasPrefix(trimmed, "{") {
		var wrap struct {
			Cookies []browserCookieEntry `json:"cookies"`
		}
		if err := json.Unmarshal([]byte(trimmed), &wrap); err == nil && len(wrap.Cookies) > 0 {
			return cookieEntriesToHeader(wrap.Cookies)
		}
		// Not a cookie wrap — fall through to plain header dedupe
		return dedupeCookieHeader(raw)
	}

	if !strings.HasPrefix(trimmed, "[") {
		return dedupeCookieHeader(raw)
	}

	var cookies []browserCookieEntry
	if err := json.Unmarshal([]byte(trimmed), &cookies); err != nil {
		return raw
	}
	return cookieEntriesToHeader(cookies)
}

func cookieEntriesToHeader(cookies []browserCookieEntry) string {
	type scored struct {
		value string
		score int
	}
	best := map[string]scored{}
	skip := map[string]bool{"_ga": true, "_gid": true, "_gat": true}
	for _, c := range cookies {
		if c.Name == "" || skip[c.Name] || strings.HasPrefix(c.Name, "_ga_") {
			continue
		}
		if strings.TrimSpace(c.Value) == "" {
			continue // skip empty (e.g. cf_zaraz_client)
		}
		score := 0
		if c.HostOnly {
			score += 2
		}
		d := strings.TrimPrefix(strings.ToLower(c.Domain), ".")
		switch {
		// Artistly: prefer hostOnly app.artistly.ai, then .artistly.ai
		case d == "app.artistly.ai":
			score += 6
		case d == "artistly.ai":
			score += 5
		case strings.HasSuffix(d, ".artistly.ai"):
			score += 3
		// Sell The Trend: hostOnly www > .www > .sellthetrend.com
		case d == "www.sellthetrend.com":
			score += 5
		case d == "sellthetrend.com" || strings.HasSuffix(d, ".sellthetrend.com"):
			score += 3
		case d == "create.vista.com":
			score += 5
		case d == "vista.com" || strings.HasSuffix(d, ".vista.com"):
			score += 3
		case d == "www.woorank.com":
			score += 5
		case d == "woorank.com" || strings.HasSuffix(d, ".woorank.com"):
			score += 3
		case d == "www.storybase.com":
			score += 5
		case d == "storybase.com" || strings.HasSuffix(d, ".storybase.com"):
			score += 3
		case d == "create.piktochart.com":
			score += 5
		case d == "piktochart.com" || strings.HasSuffix(d, ".piktochart.com"):
			score += 3
		case d == "www.ilovepdf.com":
			score += 4
		case d == "ilovepdf.com" || strings.HasSuffix(d, ".ilovepdf.com"):
			score += 3
		case d == "www.copywritely.com" || d == "copywritely.com":
			score += 3
		case strings.HasSuffix(d, ".copywritely.com"):
			score += 2
		case d == "www.canva.com":
			score += 3
		case d == "canva.com" || strings.HasSuffix(d, ".canva.com"):
			score += 2
		case d == "sas.selleramp.com":
			score += 2
		case strings.HasSuffix(d, "selleramp.com"):
			score += 1
		}
		prev, ok := best[c.Name]
		if !ok || score >= prev.score {
			best[c.Name] = scored{value: c.Value, score: score}
		}
	}
	// Prefer Artistly auth cookies first (hostOnly app.artistly.ai preferred via score)
	order := []string{
		"artistly_session", "XSRF-TOKEN", "CSRF",
		"cf_clearance", "__cf_bm", "_cfuvid",
		"intercom-session-giu8m6ur", "intercom-device-id-giu8m6ur",
		"sell_the_trend_session", "PHPSESSID",
		"token", "vpsession", "vpauth", "userid", "vpconfig", "langKey", "features",
		"assistant_private_key",
		"fakesessid", "pmpro_visit", "wordpress_test_cookie",
		"user_session", "remember_user_token", "_piktov3_final_session", "lantern", "pkto_pricing_segment",
		"_identity-ilovepdf", "_identity_ulc", "_ilovepdf", "_csrf-ilovepdf", "auth_tz",
		"PPHPSESSID", "socket_token", "qtrans_front_language", "mdd",
		"ASI", "CID", "CAZ", "CB", "CAU", "CUI", "CUL", "CTC", "CDI", "CL", "CS",
	}
	seen := map[string]bool{}
	parts := make([]string, 0, len(best))
	for name, s := range best {
		if strings.HasPrefix(name, "wordpress_logged_in_") || strings.HasPrefix(name, "remember_web_") {
			parts = append(parts, name+"="+s.value)
			seen[name] = true
		}
	}
	for _, name := range order {
		if s, ok := best[name]; ok {
			parts = append(parts, name+"="+s.value)
			seen[name] = true
		}
	}
	for name, s := range best {
		if !seen[name] {
			parts = append(parts, name+"="+s.value)
		}
	}
	return strings.Join(parts, "; ")
}

func dedupeCookieHeader(header string) string {
	best := map[string]string{}
	order := []string{}
	for _, part := range strings.Split(header, ";") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		eq := strings.Index(trimmed, "=")
		if eq <= 0 {
			continue
		}
		name := strings.TrimSpace(trimmed[:eq])
		val := strings.TrimSpace(trimmed[eq+1:])
		if name == "" {
			continue
		}
		if _, ok := best[name]; !ok {
			order = append(order, name)
		}
		best[name] = val // last wins for plain headers
	}
	parts := make([]string, 0, len(order))
	for _, name := range order {
		parts = append(parts, name+"="+best[name])
	}
	return strings.Join(parts, "; ")
}

// xsrfTokenFromCookie extracts Laravel XSRF-TOKEN and URL-decodes it (Axios style).
func xsrfTokenFromCookie(cookieHeader string) string {
	for _, part := range strings.Split(cookieHeader, ";") {
		trimmed := strings.TrimSpace(part)
		if !strings.HasPrefix(trimmed, "XSRF-TOKEN=") {
			continue
		}
		val := strings.TrimSpace(trimmed[len("XSRF-TOKEN="):])
		if decoded, err := url.QueryUnescape(val); err == nil && decoded != "" {
			return decoded
		}
		return val
	}
	return ""
}

// ensureArtistlyCsrf sets X-XSRF-TOKEN from the URL-decoded XSRF-TOKEN cookie when missing.
// Laravel AJAX/mutating requests return 419 without it.
func ensureArtistlyCsrf(upstreamReq *http.Request, cookieHeader string) {
	if strings.TrimSpace(upstreamReq.Header.Get("X-XSRF-TOKEN")) != "" {
		return
	}
	switch upstreamReq.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return
	}
	token := xsrfTokenFromCookie(cookieHeader)
	if token == "" {
		token = xsrfTokenFromCookie(upstreamReq.Header.Get("Cookie"))
	}
	if token == "" {
		return
	}
	upstreamReq.Header.Set("X-XSRF-TOKEN", token)
}

// Runtime overlay for cookies Canva/CF rotates via Set-Cookie (__cf_bm etc.).
// cookie.txt alone goes stale → Cloudflare 429 "bad request".
var (
	localCookieMu      sync.Mutex
	localCookieOverlay = map[string]string{}
)

// Parsed cookie.txt cache — avoid re-reading/parsing JSON on every asset request.
var (
	cookieFileMu     sync.Mutex
	cookieFileMod    time.Time
	cookieFileParsed string
)

func loadParsedCookieFile(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	cookieFileMu.Lock()
	defer cookieFileMu.Unlock()
	if cookieFileParsed != "" && st.ModTime().Equal(cookieFileMod) {
		return cookieFileParsed, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	cookieFileParsed = parseCookieFromDB(string(data))
	cookieFileMod = st.ModTime()
	return cookieFileParsed, nil
}

// In-memory static asset cache — Copywritely loads 40–60 JS/CSS on splash;
// re-fetching each through uTLS makes the logo screen hang for tens of seconds.
type staticCacheEntry struct {
	status      int
	contentType string
	encoding    string
	body        []byte
	expires     time.Time
}

var staticAssetCache sync.Map // key -> *staticCacheEntry

func staticCacheKey(method, path string) string {
	if i := strings.Index(path, "?"); i >= 0 {
		// Keep cache-buster query (?0.999) in key so versioned assets stay correct
		return method + " " + path
	}
	return method + " " + path
}

func isCacheableStaticPath(path string) bool {
	p := path
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	p = strings.ToLower(p)
	if strings.HasPrefix(p, "/wp-content/") || strings.HasPrefix(p, "/wp-includes/") || strings.HasPrefix(p, "/dist/") || strings.HasPrefix(p, "/img/") {
		return true
	}
	for _, ext := range []string{".js", ".css", ".woff2", ".woff", ".ttf", ".eot", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp", ".map"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

func getStaticCached(method, path string) *staticCacheEntry {
	if method != http.MethodGet && method != http.MethodHead {
		return nil
	}
	if !isCacheableStaticPath(path) {
		return nil
	}
	v, ok := staticAssetCache.Load(staticCacheKey(method, path))
	if !ok {
		// HEAD can reuse GET cache
		if method == http.MethodHead {
			v, ok = staticAssetCache.Load(staticCacheKey(http.MethodGet, path))
		}
		if !ok {
			return nil
		}
	}
	ent := v.(*staticCacheEntry)
	if time.Now().After(ent.expires) {
		staticAssetCache.Delete(staticCacheKey(http.MethodGet, path))
		return nil
	}
	return ent
}

func putStaticCached(method, path string, status int, contentType, encoding string, body []byte) {
	if method != http.MethodGet || status != 200 || !isCacheableStaticPath(path) {
		return
	}
	if len(body) == 0 || len(body) > 8<<20 { // skip empty / >8MB
		return
	}
	cp := make([]byte, len(body))
	copy(cp, body)
	staticAssetCache.Store(staticCacheKey(method, path), &staticCacheEntry{
		status:      status,
		contentType: contentType,
		encoding:    encoding,
		body:        cp,
		expires:     time.Now().Add(6 * time.Hour),
	})
}

func serveStaticCached(w http.ResponseWriter, r *http.Request, ent *staticCacheEntry) {
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Del("Pragma")
	w.Header().Del("Expires")
	if ent.contentType != "" {
		w.Header().Set("Content-Type", ent.contentType)
	}
	if ent.encoding != "" {
		w.Header().Set("Content-Encoding", ent.encoding)
	}
	w.Header().Set("X-OCG-Cache", "HIT")
	w.Header().Set("Content-Length", strconv.Itoa(len(ent.body)))
	w.WriteHeader(ent.status)
	if r.Method != http.MethodHead {
		w.Write(ent.body)
	}
}

func setProxyCacheHeaders(w http.ResponseWriter, path, contentType string) {
	if isCacheableStaticPath(path) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Del("Pragma")
		w.Header().Del("Expires")
		return
	}
	// HTML / JSON / AJAX — never cache
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	_ = contentType
}

var absorbCookieNames = map[string]bool{
	"__cf_bm": true, "cf_clearance": true, "ASI": true, "CID": true,
	"CAE": true, "CAZ": true, "CB": true, "CUI": true, "CUL": true, "CAU": true,
	"user_session": true, "remember_user_token": true, "_piktov3_final_session": true,
	"_ilovepdf": true, "_csrf-ilovepdf": true, "_identity-ilovepdf": true, "_identity_ulc": true,
}

func cookieHeaderToMap(header string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(header, ";") {
		trimmed := strings.TrimSpace(part)
		eq := strings.Index(trimmed, "=")
		if eq <= 0 {
			continue
		}
		out[strings.TrimSpace(trimmed[:eq])] = strings.TrimSpace(trimmed[eq+1:])
	}
	return out
}

func mapToCookieHeader(m map[string]string) string {
	prefer := []string{"ASI", "CID", "CAE", "__cf_bm", "cf_clearance", "CAZ", "CB", "CUI", "CUL", "CAU"}
	seen := map[string]bool{}
	parts := make([]string, 0, len(m))
	for _, n := range prefer {
		if v, ok := m[n]; ok && v != "" {
			parts = append(parts, n+"="+v)
			seen[n] = true
		}
	}
	for n, v := range m {
		if seen[n] || v == "" {
			continue
		}
		parts = append(parts, n+"="+v)
	}
	return strings.Join(parts, "; ")
}

func applyLocalCookieOverlay(base string) string {
	localCookieMu.Lock()
	defer localCookieMu.Unlock()
	if len(localCookieOverlay) == 0 {
		return base
	}
	m := cookieHeaderToMap(base)
	for k, v := range localCookieOverlay {
		if v != "" {
			m[k] = v
		}
	}
	return mapToCookieHeader(m)
}

func absorbUpstreamSetCookies(h http.Header, statusCode int) (changed bool) {
	// Never absorb CF/session cookies from challenge responses (403 "Just a moment...").
	// Those overwrite a good DigitaVision cf_clearance with unusable challenge cookies.
	if statusCode < 200 || statusCode > 299 {
		return false
	}
	localCookieMu.Lock()
	defer localCookieMu.Unlock()
	for _, sc := range h.Values("Set-Cookie") {
		parts := strings.SplitN(sc, ";", 2)
		if len(parts) == 0 {
			continue
		}
		nv := strings.TrimSpace(parts[0])
		eq := strings.Index(nv, "=")
		if eq <= 0 {
			continue
		}
		name := strings.TrimSpace(nv[:eq])
		val := strings.TrimSpace(nv[eq+1:])
		if !absorbCookieNames[name] || val == "" || strings.EqualFold(val, "deleted") {
			continue
		}
		if localCookieOverlay[name] != val {
			localCookieOverlay[name] = val
			changed = true
			log.Printf("[COOKIE] Absorbed Set-Cookie %s (len=%d)", name, len(val))
		}
	}
	return changed
}

func doUpstreamWith429Retry(req *http.Request, cookieStr string) (*http.Response, error) {
	// Canva media (document-image) rejects GET with any body / Content-Length.
	// Go server Request.Body is non-nil even for GET — never forward an empty body.
	if req.Method == http.MethodGet || req.Method == http.MethodHead {
		if req.Body != nil {
			io.Copy(io.Discard, req.Body)
			req.Body.Close()
		}
		req.Body = nil
		req.GetBody = nil
		req.ContentLength = 0
		req.Header.Del("Content-Length")
		req.Header.Del("Transfer-Encoding")
	} else if req.Body != nil && req.GetBody == nil {
		bodyBytes, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		req.ContentLength = int64(len(bodyBytes))
	}

	var lastResp *http.Response
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			if req.GetBody != nil {
				b, err := req.GetBody()
				if err != nil {
					return lastResp, err
				}
				req.Body = b
			}
			// Refresh CF cookies onto the retried request
			req.Header.Set("Cookie", applyLocalCookieOverlay(cookieStr))
			delay := time.Duration(attempt*attempt) * 800 * time.Millisecond
			if lastResp != nil {
				if ra := lastResp.Header.Get("Retry-After"); ra != "" {
					if secs, err := strconv.Atoi(ra); err == nil && secs > 0 && secs < 30 {
						delay = time.Duration(secs) * time.Second
					}
				}
				lastResp.Body.Close()
			}
			time.Sleep(delay)
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		absorbUpstreamSetCookies(resp.Header, resp.StatusCode)
		if resp.StatusCode != http.StatusTooManyRequests {
			return resp, nil
		}
		lastResp = resp
		log.Printf("[PROXY] 429 from upstream %s (attempt %d/3)", req.URL.Path, attempt+1)
	}
	return lastResp, nil
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
			if sensitiveSet[name] || strings.HasPrefix(name, "wordpress_logged_in_") || strings.HasPrefix(name, "remember_web_") {
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

	redirectURL := fmt.Sprintf("%s://%s/access?token=%s",
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
	return dialChromeALPN(ctx, addr, nil) // default Chrome ALPN (h2, http/1.1)
}

// dialChromeHTTP1 forces http/1.1 only — required for WebSocket upgrades
// (h2 SETTINGS frames look like "malformed HTTP response" to http.ReadResponse).
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
		tcpConn, err = (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("TCP dial: %w", err)
		}
	}

	cfg := &utls.Config{ServerName: host, InsecureSkipVerify: false}
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
		cfg.NextProtos = append([]string(nil), nextProtos...)
		uConn = utls.UClient(tcpConn, cfg, utls.HelloCustom)
		if err := uConn.ApplyPreset(&spec); err != nil {
			tcpConn.Close()
			return nil, fmt.Errorf("uTLS preset: %w", err)
		}
	} else {
		uConn = utls.UClient(tcpConn, cfg, utls.HelloChrome_120)
	}
	if err := uConn.HandshakeContext(ctx); err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("uTLS handshake: %w", err)
	}
	return &uTLSConn{uConn}, nil
}

type roundTripper struct {
	h1 *http.Transport
}

func (rt *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if px, ok := req.Context().Value(proxyContextKey).(string); ok && strings.TrimSpace(px) != "" {
		if rt.h1 != nil {
			rt.h1.CloseIdleConnections()
		}
	}
	return rt.h1.RoundTrip(req)
}

func buildChromeHTTPClient() *http.Client {
	// Force ALPN http/1.1 — h2 ClientHello triggers CF "Just a moment..." on artboard.
	dialTLS := func(ctx context.Context, network, addr string) (net.Conn, error) { return dialChromeHTTP1(ctx, addr) }
	h1 := &http.Transport{
		DialTLSContext: dialTLS, MaxIdleConns: 200, MaxIdleConnsPerHost: 32,
		IdleConnTimeout: 120 * time.Second, TLSHandshakeTimeout: 15 * time.Second,
		DisableCompression: false, ForceAttemptHTTP2: false,
		ResponseHeaderTimeout: 60 * time.Second,
	}
	return &http.Client{
		Transport:     &roundTripper{h1: h1},
		Timeout:       120 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
	}
}

var httpClient = buildChromeHTTPClient()

// ── URL REWRITING HELPERS ─────────────────────────────────────────────────────

// jsonSlashEscape turns https://host into https:\/\/host (Laravel Ziggy / JSON-in-HTML).
func jsonSlashEscape(s string) string {
	return strings.ReplaceAll(s, "/", `\/`)
}

// buildDomainReplacements creates a list of old→new domain pairs for HTML rewriting.
func buildDomainReplacements(publicScheme, publicHost string, cfg Config) [][2]string {
	targetParsed, _ := url.Parse(cfg.TargetURL)
	cdnParsed, _ := url.Parse(cfg.CDNURL)
	publicBase := fmt.Sprintf("%s://%s", publicScheme, publicHost)
	var pairs [][2]string
	addPair := func(from, to string) {
		if from == "" || to == "" || from == to {
			return
		}
		pairs = append(pairs, [2]string{from, to})
		// Escaped slashes: Ziggy embeds "url":"https:\/\/app.artistly.ai"
		escFrom, escTo := jsonSlashEscape(from), jsonSlashEscape(to)
		if escFrom != from {
			pairs = append(pairs, [2]string{escFrom, escTo})
		}
	}
	// Custom extension install (never send users to Chrome Web Store)
	addPair("https://selleramp.com/extension", publicBase+"/ext-install")
	addPair("http://selleramp.com/extension", publicBase+"/ext-install")
	addPair("https://www.selleramp.com/extension", publicBase+"/ext-install")
	addPair("http://www.selleramp.com/extension", publicBase+"/ext-install")
	if targetParsed != nil {
		addPair("https://"+targetParsed.Host, publicBase)
		addPair("http://"+targetParsed.Host, publicBase)
	}
	if cdnParsed != nil && (targetParsed == nil || cdnParsed.Host != targetParsed.Host) {
		addPair("https://"+cdnParsed.Host, publicBase+"/cdn-proxy")
		addPair("http://"+cdnParsed.Host, publicBase+"/cdn-proxy")
	}
	wsScheme := "ws"
	if publicScheme == "https" {
		wsScheme = "wss"
	}
	for i, extra := range cfg.ExtraCDNDomains {
		extra = strings.TrimPrefix(strings.TrimPrefix(extra, "https://"), "http://")
		extra = strings.Split(extra, "/")[0]
		if targetParsed != nil && extra == targetParsed.Host {
			continue // already mapped to publicBase
		}
		proxyBase := fmt.Sprintf("%s/extra-cdn-%d", publicBase, i)
		wsProxy := fmt.Sprintf("%s://%s/extra-cdn-%d", wsScheme, publicHost, i)
		addPair("https://"+extra, proxyBase)
		addPair("http://"+extra, proxyBase)
		addPair("wss://"+extra, wsProxy)
		addPair("ws://"+extra, wsProxy)
	}
	// Prefer ://host (not bare host) so subdomains like wss.copywritely.com
	// are not mangled into wss.127.0.0.1:4521.
	if targetParsed != nil {
		addPair("://"+targetParsed.Host, "://"+publicHost)
	}
	return pairs
}

func rewriteBody(body []byte, pairs [][2]string) []byte {
	for _, pair := range pairs {
		body = bytes.ReplaceAll(body, []byte(pair[0]), []byte(pair[1]))
	}
	return body
}

// applyTextReplacements runs config.json "replacements" on response bodies
// (HTML/JSON/etc). Supports multiple search→replace pairs. Empty search skipped.
func applyTextReplacements(body []byte, cfg Config) []byte {
	if len(cfg.Replacements) == 0 || len(body) == 0 {
		return body
	}
	for _, repl := range cfg.Replacements {
		if repl.Search == "" {
			continue
		}
		body = bytes.ReplaceAll(body, []byte(repl.Search), []byte(repl.Replace))
	}
	return body
}

// buildTextReplaceInjectHTML installs a light DOM walker so Vue/React-rendered
// text also picks up config replacements after hydration.
func buildTextReplaceInjectHTML(cfg Config) string {
	if len(cfg.Replacements) == 0 {
		return ""
	}
	type pair struct {
		Search  string `json:"s"`
		Replace string `json:"r"`
	}
	list := make([]pair, 0, len(cfg.Replacements))
	for _, repl := range cfg.Replacements {
		if repl.Search == "" {
			continue
		}
		list = append(list, pair{Search: repl.Search, Replace: repl.Replace})
	}
	if len(list) == 0 {
		return ""
	}
	b, err := json.Marshal(list)
	if err != nil {
		return ""
	}
	js := strings.ReplaceAll(string(b), "</", "<\\/")
	return `<script data-tm-text-replace="1">
(function(){
  var PAIRS = ` + js + `;
  if (!PAIRS || !PAIRS.length) return;
  function sub(s) {
    if (!s) return s;
    for (var i = 0; i < PAIRS.length; i++) {
      if (s.indexOf(PAIRS[i].s) !== -1) s = s.split(PAIRS[i].s).join(PAIRS[i].r);
    }
    return s;
  }
  function walk(root) {
    if (!root) return;
    var w = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, null);
    var n;
    while ((n = w.nextNode())) {
      var v = n.nodeValue;
      var nv = sub(v);
      if (nv !== v) n.nodeValue = nv;
    }
    if (root.querySelectorAll) {
      root.querySelectorAll('[placeholder],[title],[aria-label],[alt]').forEach(function(el){
        ['placeholder','title','aria-label','alt'].forEach(function(a){
          var v = el.getAttribute(a);
          if (v) {
            var nv = sub(v);
            if (nv !== v) el.setAttribute(a, nv);
          }
        });
      });
    }
  }
  function run(){ try { walk(document.body || document.documentElement); } catch(e){} }
  run();
  try {
    new MutationObserver(function(){ run(); }).observe(document.documentElement, {childList:true, subtree:true, characterData:true});
  } catch(e) {}
  setInterval(run, 2000);
})();
</script>`
}

// rewriteWoorankHostURL maps https://*.woorank.com/... → /ext-host/...
// Skips the main www.woorank.com target host.
func rewriteWoorankHostURL(loc, publicBase string) string {
	re := regexp.MustCompile(`(?i)^(https?:)?//([a-z0-9.-]+\.woorank\.com|woorank\.com)(?::\d+)?(/.*)?$`)
	m := re.FindStringSubmatch(loc)
	if m == nil {
		return loc
	}
	host := strings.ToLower(m[2])
	if host == "www.woorank.com" {
		return loc
	}
	pathPart := m[3]
	if pathPart == "" {
		pathPart = "/"
	}
	return strings.TrimRight(publicBase, "/") + "/ext-host/" + host + pathPart
}

// stripSubresourceIntegrity removes browser SRI attributes and Canva's
// bootstrap/manifest hash fields so rewritten CDN URLs still load.
func stripSubresourceIntegrity(body []byte) []byte {
	body = regexp.MustCompile(`(?i)\s+integrity\s*=\s*("[^"]*"|'[^']*')`).ReplaceAll(body, nil)
	// Canva asset list: {"A":"https://.../file.js","C":"sha512-..."}
	body = regexp.MustCompile(`,"C":"sha[0-9]+-[A-Za-z0-9+/=]+"`).ReplaceAll(body, nil)
	body = regexp.MustCompile(`"C":"sha[0-9]+-[A-Za-z0-9+/=]+",`).ReplaceAll(body, nil)
	// Escaped JSON inside HTML string payloads: \"C\":\"sha512-...\"
	body = regexp.MustCompile(`,\\"C\\":\\"sha[0-9]+-[A-Za-z0-9+/=]+\\"`).ReplaceAll(body, nil)
	body = regexp.MustCompile(`\\"C\\":\\"sha[0-9]+-[A-Za-z0-9+/=]+\\",`).ReplaceAll(body, nil)
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
		extraCDNJS.WriteString(fmt.Sprintf(
			`["https://%s",%q],["http://%s",%q],["wss://%s",%q],["ws://%s",%q],`,
			extraClean, proxyPath, extraClean, proxyPath, extraClean, proxyPath, extraClean, proxyPath,
		))
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
            over.style.cssText = 'position:fixed;top:0;left:0;width:100vw;height:100vh;background:#0f172a;color:#fff;z-index:99999999;display:flex;flex-direction:column;align-items:center;justify-content:center;font-family:sans-serif;gap:16px;';
            over.innerHTML = '<div style="border:4px solid #f8fafc;border-top-color:#4f46e5;border-radius:50%%;width:40px;height:40px;"></div><span>Reconnecting... Please wait</span>';
            document.body.appendChild(over);
            fetch('/api/rotate-session?reason=' + encodeURIComponent(reason))
            .then(function() { setTimeout(function() { window.location.href = HOME; }, 1200); })
            .catch(function() { setTimeout(function() { window.location.href = HOME; }, 1200); });
        }
    }
    if (%s) { setTimeout(checkWatchdog, 1000); setInterval(checkWatchdog, 4000); }

    // ── Link + form rewriter (absolute official URLs → our origin) ──
    function rewriteLinks() {
        document.querySelectorAll('a[href*="'+T+'"]').forEach(function(a) {
            var h = a.href.replace('https://'+T, O).replace('http://'+T, O);
            if (a.href !== h) a.href = h;
        });
        document.querySelectorAll('form[action*="'+T+'"]').forEach(function(f) {
            var a = f.getAttribute('action') || '';
            var n = a.replace('https://'+T, O).replace('http://'+T, O);
            if (n !== a) f.setAttribute('action', n);
        });
    }
    rewriteLinks();
    setInterval(rewriteLinks, 200);

    // Canva builds chunk-composing.<parent-domain> from publicPath host
    // (127.0.0.1:4501 → chunk-composing.0.1:4501). Map those to our proxy.
    var CHUNK_COMPOSE = null;
    for (var ci = 0; ci < EXTRA.length; ci++) {
        if (EXTRA[ci][0].indexOf('chunk-composing.') !== -1) { CHUNK_COMPOSE = EXTRA[ci][1]; break; }
    }

    function patchURL(u) {
        if (typeof u !== 'string') return u;
        // Strip accidental wrapping quotes (broken relative URLs)
        if (u.length > 1 && ((u.charAt(0) === '"' && u.charAt(u.length-1) === '"') || (u.charAt(0) === "'" && u.charAt(u.length-1) === "'"))) {
            u = u.substring(1, u.length-1);
        }
        // WooRank sibling hosts via /ext-host/ (extra CDN list also covers these)
        u = u.replace(/(https?:)?\/\/((?:[a-z0-9.-]+\.)?woorank\.com)(?::\d+)?(?=\/|$)/gi, function(match, proto, host) {
            if (host === 'www.woorank.com') return match;
            return O + '/ext-host/' + host;
        });
        // Extra CDN / WSS first (longer hosts before main T)
        for (var e=0; e<EXTRA.length; e++) {
            var from = EXTRA[e][0], to = EXTRA[e][1];
            if (from.indexOf('wss://') === 0 || from.indexOf('ws://') === 0) {
                var wsO = O.replace(/^http/,'ws');
                u = u.split(from).join(wsO + to);
            } else {
                u = u.split(from).join(O + to);
            }
        }
        u = u.replace('https://'+T, O).replace('http://'+T, O);
        if (C && C !== T) u = u.replace('https://'+C, O+'/cdn-proxy').replace('http://'+C, O+'/cdn-proxy');
        // Dynamic chunk-composing hosts (any TLD / mangled IP-derived host)
        if (CHUNK_COMPOSE) {
            u = u.replace(/https?:\/\/chunk-composing\.[^/]+/g, O + CHUNK_COMPOSE);
        }
        return u;
    }

    function isNoiseURL(u) {
        return /telemetry\.canva\.com|ingest\.sentry\.io|\/traces\?/i.test(String(u || ''));
    }

    // Plupload stores worker URL at init — rewrite settings.url and setOption('url')
    function patchPlupload() {
        if (!window.plupload || !window.plupload.Uploader || window.plupload.__tmPatched) return;
        window.plupload.__tmPatched = true;
        var Orig = window.plupload.Uploader;
        window.plupload.Uploader = function(settings) {
            if (settings && typeof settings.url === 'string') settings.url = patchURL(settings.url);
            var up = new Orig(settings);
            if (up && typeof up.setOption === 'function') {
                var _set = up.setOption.bind(up);
                up.setOption = function(k, v) {
                    if ((k === 'url' || (k && k.url)) && typeof v === 'string') v = patchURL(v);
                    if (k && typeof k === 'object' && typeof k.url === 'string') k.url = patchURL(k.url);
                    return _set(k, v);
                };
            }
            return up;
        };
        window.plupload.Uploader.prototype = Orig.prototype;
    }
    patchPlupload();
    setInterval(patchPlupload, 400);

    // Soften Cloudflare 429 storms — backoff retry + light ajax concurrency cap
    var ajaxActive = 0, ajaxMax = 6, ajaxWait = [];
    function ajaxSlot(run) {
        return new Promise(function(resolve, reject) {
            var start = function() {
                ajaxActive++;
                Promise.resolve().then(run).then(function(v) {
                    ajaxActive--;
                    if (ajaxWait.length) ajaxWait.shift()();
                    resolve(v);
                }, function(e) {
                    ajaxActive--;
                    if (ajaxWait.length) ajaxWait.shift()();
                    reject(e);
                });
            };
            if (ajaxActive < ajaxMax) start();
            else ajaxWait.push(start);
        });
    }
    function sleep(ms) { return new Promise(function(r){ setTimeout(r, ms); }); }
    function shouldRetry429(url) {
        return /\/_ajax\//.test(String(url||''));
    }

    // ── XHR patch ──
    var xo = XMLHttpRequest.prototype.open;
    var xs = XMLHttpRequest.prototype.send;
    XMLHttpRequest.prototype.open = function(m, u) {
        this.__canvaNoise = isNoiseURL(u);
        this.__canvaURL = String(u || '');
        this.__canvaMethod = m;
        return xo.apply(this, [m, patchURL(u)].concat(Array.prototype.slice.call(arguments, 2)));
    };
    XMLHttpRequest.prototype.send = function() {
        if (this.__canvaNoise) {
            Object.defineProperty(this, 'status', {get: function(){ return 204; }});
            Object.defineProperty(this, 'readyState', {get: function(){ return 4; }});
            var self = this;
            setTimeout(function() {
                if (typeof self.onreadystatechange === 'function') self.onreadystatechange();
                if (typeof self.onload === 'function') self.onload();
            }, 0);
            return;
        }
        var args = arguments;
        var xhr = this;
        if (!shouldRetry429(xhr.__canvaURL)) {
            return xs.apply(xhr, args);
        }
        var attempt = 0;
        var origOnReady = xhr.onreadystatechange;
        var origOnLoad = xhr.onload;
        var origOnError = xhr.onerror;
        function armHandlers() {
            xhr.onreadystatechange = function() {
                if (xhr.readyState === 4 && xhr.status === 429 && attempt < 2) {
                    attempt++;
                    setTimeout(function() {
                        try {
                            xo.call(xhr, xhr.__canvaMethod || 'GET', patchURL(xhr.__canvaURL));
                            armHandlers();
                            xs.apply(xhr, args);
                        } catch (e) {}
                    }, 700 * attempt * attempt);
                    return;
                }
                if (typeof origOnReady === 'function') return origOnReady.apply(xhr, arguments);
            };
            xhr.onload = origOnLoad;
            xhr.onerror = origOnError;
        }
        armHandlers();
        return xs.apply(xhr, args);
    };

    // ── Fetch patch ──
    var fo = window.fetch;
    window.fetch = function(inp, init) {
        var urlStr = typeof inp === 'string' ? inp : (inp && inp.url) || '';
        if (isNoiseURL(urlStr)) {
            return Promise.resolve(new Response('', {status: 204, statusText: 'No Content'}));
        }
        if (typeof inp === 'string') inp = patchURL(inp);
        else if (inp instanceof Request) inp = new Request(patchURL(inp.url), inp);
        var finalURL = typeof inp === 'string' ? inp : (inp && inp.url) || urlStr;
        var doFetch = function() { return fo(inp, init); };
        var runner = shouldRetry429(finalURL) ? function() { return ajaxSlot(doFetch); } : doFetch;
        return runner().then(function(res) {
            if (res && res.status === 429 && shouldRetry429(finalURL)) {
                return sleep(800).then(function(){ return runner(); }).then(function(res2) {
                    if (res2 && res2.status === 429) {
                        return sleep(1600).then(function(){ return runner(); });
                    }
                    return res2;
                });
            }
            return res;
        });
    };

    // sendBeacon → telemetry also CORS-fails; pretend success
    try {
        var sb = navigator.sendBeacon && navigator.sendBeacon.bind(navigator);
        if (sb) {
            navigator.sendBeacon = function(url, data) {
                if (isNoiseURL(url)) return true;
                return sb(url, data);
            };
        }
    } catch (e) {}

    // Hide reconnect banner text node if WS briefly flaps (primary fix is HTTP/1.1 WS)
    try {
        var style = document.createElement('style');
        style.textContent = [
            '[class*="Reconnect"], [class*="reconnect"], [data-testid*="reconnect" i]',
            '{ display: none !important; visibility: hidden !important; }'
        ].join(' ');
        (document.head || document.documentElement).appendChild(style);
        var hideReconnect = function() {
            try {
                var nodes = document.querySelectorAll('div,span,section,aside');
                for (var i = 0; i < nodes.length && i < 400; i++) {
                    var t = nodes[i].childNodes && nodes[i].childNodes.length <= 3 ? (nodes[i].textContent || '') : '';
                    if (/Trying to reconnect/i.test(t) && t.length < 160) {
                        nodes[i].style.setProperty('display', 'none', 'important');
                    }
                }
            } catch (e) {}
        };
        setInterval(hideReconnect, 2000);
    } catch (e) {}

    // Hide ONLY the profile/account popover (Accounts + Teams + Log out + Your account).
    // Broader match + always re-hide (React often resets display after our first pass).
    try {
        var accHideStyle = document.createElement('style');
        accHideStyle.id = 'ocg-hide-account-menu-css';
        accHideStyle.textContent = [
            '[data-ocg-hide-account-menu="1"] {',
            '  display: none !important; visibility: hidden !important;',
            '  opacity: 0 !important; pointer-events: none !important;',
            '  width: 0 !important; height: 0 !important; overflow: hidden !important;',
            '}'
        ].join('');
        (document.head || document.documentElement).appendChild(accHideStyle);

        function textLooksLikeAccountMenu(t) {
            t = String(t || '');
            if (t.length < 40 || t.length > 8000) return false;
            if (!/Your account/i.test(t)) return false;
            if (!/Log out/i.test(t)) return false;
            if (!/Accounts/i.test(t)) return false;
            // Pricing OR Settings makes this the account popover, not File/Share menus
            return /Plans and pricing|Settings/i.test(t);
        }
        function markAndNuke(el) {
            if (!el || el.nodeType !== 1) return;
            el.setAttribute('data-ocg-hide-account-menu', '1');
            el.style.setProperty('display', 'none', 'important');
            el.style.setProperty('visibility', 'hidden', 'important');
            el.style.setProperty('opacity', '0', 'important');
            el.style.setProperty('pointer-events', 'none', 'important');
            try { el.remove(); } catch (e) {
                try { if (el.parentNode) el.parentNode.removeChild(el); } catch (e2) {}
            }
        }
        function hideAccountProfileMenu() {
            try {
                // iLovePDF: avatar / account dropdown only (keep "more" products menu)
                document.querySelectorAll('.nav-actions li.nav-has-dropdown[data-account="true"]').forEach(function(el) {
                    el.style.setProperty('display', 'none', 'important');
                    el.style.setProperty('visibility', 'hidden', 'important');
                    el.style.setProperty('pointer-events', 'none', 'important');
                });
                // 1) Prefer ul[role=menu] with account-settings link
                var menus = document.querySelectorAll('ul[role="menu"]');
                for (var i = 0; i < menus.length; i++) {
                    var ul = menus[i];
                    if (!ul.querySelector('a[href*="/settings/account"]')) continue;
                    if (!textLooksLikeAccountMenu(ul.textContent)) continue;
                    var node = ul, target = ul;
                    for (var d = 0; d < 10 && node; d++) {
                        var st = window.getComputedStyle(node);
                        if (st && (st.position === 'fixed' || st.position === 'absolute')) {
                            target = node;
                            if (st.position === 'fixed') break;
                        }
                        node = node.parentElement;
                    }
                    markAndNuke(target);
                }
                // 2) Fallback: any fixed portal whose text is clearly this menu
                var all = document.querySelectorAll('div');
                for (var j = 0; j < all.length; j++) {
                    var div = all[j];
                    if (div.getAttribute('data-ocg-hide-account-menu') === '1') continue;
                    var cs = window.getComputedStyle(div);
                    if (!cs || cs.position !== 'fixed') continue;
                    if (!div.querySelector('a[href*="/settings/account"]')) continue;
                    if (!textLooksLikeAccountMenu(div.textContent)) continue;
                    markAndNuke(div);
                }
            } catch (e) {}
        }
        hideAccountProfileMenu();
        new MutationObserver(function() { hideAccountProfileMenu(); })
            .observe(document.documentElement, { childList: true, subtree: true, attributes: true });
        setInterval(hideAccountProfileMenu, 250);
        // Close immediately if user opens avatar menu
        document.addEventListener('click', function() {
            setTimeout(hideAccountProfileMenu, 0);
            setTimeout(hideAccountProfileMenu, 50);
            setTimeout(hideAccountProfileMenu, 150);
        }, true);
    } catch (e) {}

    // Patch .src / .href property setters (webpack sets these, not setAttribute)
    function patchProp(proto, prop) {
        try {
            var desc = Object.getOwnPropertyDescriptor(proto, prop);
            if (!desc || !desc.set) return;
            Object.defineProperty(proto, prop, {
                configurable: true,
                enumerable: desc.enumerable,
                get: desc.get,
                set: function(v) {
                    if (typeof v === 'string') v = patchURL(v);
                    return desc.set.call(this, v);
                }
            });
        } catch (err) {}
    }
    try {
        patchProp(HTMLScriptElement.prototype, 'src');
        patchProp(HTMLLinkElement.prototype, 'href');
    } catch (e) {}

    // ── Strip integrity + rewrite src/href on dynamically created tags ──
    try {
        var ce = Document.prototype.createElement;
        Document.prototype.createElement = function(tag) {
            var el = ce.apply(this, arguments);
            var t = tag ? String(tag).toLowerCase() : '';
            if (t === 'script' || t === 'link') {
                el.setAttribute = (function(orig) {
                    return function(name, value) {
                        if (String(name).toLowerCase() === 'integrity') return;
                        if (String(name).toLowerCase() === 'src' || String(name).toLowerCase() === 'href') value = patchURL(String(value));
                        return orig.call(this, name, value);
                    };
                })(el.setAttribute.bind(el));
            }
            return el;
        };
    } catch (e) {}

    // Strip integrity + fix mangled chunk-composing URLs on insert
    try {
        new MutationObserver(function(muts) {
            muts.forEach(function(m) {
                m.addedNodes && m.addedNodes.forEach(function(n) {
                    if (!n || n.nodeType !== 1) return;
                    if (n.tagName === 'SCRIPT' || n.tagName === 'LINK') {
                        n.removeAttribute && n.removeAttribute('integrity');
                        try {
                            if (n.href && String(n.href).indexOf('chunk-composing.') !== -1) n.href = patchURL(n.href);
                            if (n.src && String(n.src).indexOf('chunk-composing.') !== -1) n.src = patchURL(n.src);
                        } catch (err) {}
                    }
                });
            });
        }).observe(document.documentElement, {childList:true, subtree:true});
    } catch (e) {}

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

    // Laravel Ziggy: official host in Ziggy.url makes route()/redirects leave the proxy
    try {
        function fixZiggy() {
            if (!window.Ziggy || typeof window.Ziggy !== 'object') return;
            window.Ziggy.url = O;
            try {
                var port = (window.location.port || '');
                window.Ziggy.port = port ? parseInt(port, 10) : null;
            } catch (e) {}
        }
        fixZiggy();
        setInterval(fixZiggy, 400);
    } catch (e) {}

    // Hard navigations / Livewire redirects that set location to absolute target URL
    try {
        var _locAssign = window.location.assign.bind(window.location);
        window.location.assign = function(u) { return _locAssign(patchURL(String(u))); };
        var _locReplace = window.location.replace.bind(window.location);
        window.location.replace = function(u) { return _locReplace(patchURL(String(u))); };
    } catch (e) {}

    // ── History API: rewrite absolute target URLs + block restricted paths ──
    function isBlocked(path) {
        var clean = path.split('?')[0].replace(/\/$/, '');
        for (var i = 0; i < BLOCKED.length; i++) {
            var b = String(BLOCKED[i] || '').replace(/\/$/, '');
            if (!b) continue;
            if (clean === b || clean.startsWith(b+'/')) return true;
        }
        return false;
    }
    var _push = history.pushState.bind(history);
    history.pushState = function(state, title, url) {
        if (typeof url === 'string') {
            url = patchURL(url);
            try { var p = new URL(url, O); if (isBlocked(p.pathname)) { window.location.href = HOME; return; } } catch(e) {}
        }
        return _push(state, title, url);
    };
    var _replace = history.replaceState.bind(history);
    history.replaceState = function(state, title, url) {
        if (typeof url === 'string') {
            url = patchURL(url);
            try { var p = new URL(url, O); if (isBlocked(p.pathname)) { window.location.href = HOME; return; } } catch(e) {}
        }
        return _replace(state, title, url);
    };
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
	switch strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding"))) {
	case "gzip":
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		reader = gr
	case "br":
		reader = brotli.NewReader(resp.Body)
	case "zstd":
		zr, err := zstd.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		reader = zr
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

	// Connect with HTTP/1.1-only uTLS — WebSocket cannot upgrade over h2
	var upstreamConn net.Conn
	var err error
	if useTLS {
		upstreamConn, err = dialChromeHTTP1(context.WithValue(r.Context(), proxyContextKey, accountProxy), upstreamAddr)
	} else {
		upstreamConn, err = net.Dial("tcp", upstreamAddr)
	}
	if err != nil {
		log.Printf("[WS] Upstream dial failed for %s: %v", upstreamAddr, err)
		http.Error(w, "WebSocket upstream unavailable", http.StatusBadGateway)
		return
	}
	defer upstreamConn.Close()

	// Build HTTP/1.1 upgrade request
	var reqBuf bytes.Buffer
	upstreamPath := upstreamURL.RequestURI()
	reqBuf.WriteString(fmt.Sprintf("GET %s HTTP/1.1\r\n", upstreamPath))
	reqBuf.WriteString(fmt.Sprintf("Host: %s\r\n", upstreamURL.Host))
	// Ensure required upgrade headers even if client omitted casing variants
	upgrade := r.Header.Get("Upgrade")
	if upgrade == "" {
		upgrade = "websocket"
	}
	connection := r.Header.Get("Connection")
	if connection == "" {
		connection = "Upgrade"
	}
	reqBuf.WriteString(fmt.Sprintf("Upgrade: %s\r\n", upgrade))
	reqBuf.WriteString(fmt.Sprintf("Connection: %s\r\n", connection))
	for _, h := range []string{"Sec-WebSocket-Key", "Sec-WebSocket-Version", "Sec-WebSocket-Extensions", "Sec-WebSocket-Protocol"} {
		if v := r.Header.Get(h); v != "" {
			reqBuf.WriteString(fmt.Sprintf("%s: %s\r\n", h, v))
		}
	}
	if cookieStr != "" {
		reqBuf.WriteString(fmt.Sprintf("Cookie: %s\r\n", cookieStr))
	}
	if userAgent != "" {
		reqBuf.WriteString(fmt.Sprintf("User-Agent: %s\r\n", userAgent))
	}
	origin := strings.TrimRight(cfg.TargetURL, "/")
	reqBuf.WriteString(fmt.Sprintf("Origin: %s\r\n", origin))
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
		bodyPeek, _ := io.ReadAll(io.LimitReader(upstreamResp.Body, 200))
		upstreamResp.Body.Close()
		log.Printf("[WS] Upstream did not upgrade: %s body=%q path=%s", upstreamResp.Status, bodyPeek, upstreamPath)
		http.Error(w, "WebSocket upstream rejected: "+upstreamResp.Status, http.StatusBadGateway)
		return
	}
	log.Printf("[WS] ✅ Upgraded %s → %s", r.URL.Path, upstreamURL.Host)

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

// isStreamingPath: Canva React hydration posts to /_stream; must be flushed
// through without buffering/rewrite or the home UI stays on skeleton forever.
func isStreamingPath(path string) bool {
	return path == "/_stream" || strings.HasPrefix(path, "/_stream/") ||
		path == "/_online" || strings.HasPrefix(path, "/_worker/")
}

func isStreamingContentType(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "text/event-stream") ||
		strings.Contains(ct, "text/x-component") ||
		strings.Contains(ct, "application/x-ndjson") ||
		strings.Contains(ct, "application/grpc") ||
		strings.Contains(ct, "application/octet-stream")
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
		path == "/__logs" ||
		path == "/__logs.json" ||
		path == "/user/logout" {
		return // These are handled by their own handlers
	}

	// ── 1. Authenticate user (require ct_session cookie) ─────────────────────────
	isFavicon := strings.Contains(strings.ToLower(path), "favicon")
	currentUser, authErr := getAuthenticatedUser(r, cfg)
	if authErr != nil && !isFavicon {
		_, hasSess := r.Cookie("ct_session")
		log.Printf("[AUTH] ❌ denied path=%s host=%s err=%v website_id=%d has_ct_session=%v",
			path, r.Host, authErr, currentWebsiteID, hasSess == nil)
		pushProxyLog(ProxyLogEntry{
			Source:  "AUTH",
			Level:   "error",
			Method:  r.Method,
			Path:    path,
			Status:  http.StatusUnauthorized,
			Detail:  authErr.Error(),
			HasUID:  r.Header.Get("X-UID") != "",
			HasAPI:  r.Header.Get("X-API-TOKEN") != "",
			CookieN: countCookieNames(r.Header.Get("Cookie")),
		})
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
	if isFavicon && authErr != nil {
		currentUser = "guest_favicon"
	}

	// Root → logged-in app home (WooRank marketing `/` is not useful behind proxy)
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

	// ── 2b. Serve cached static assets (skip upstream TLS) ───────────────────────
	if ent := getStaticCached(r.Method, path); ent != nil {
		serveStaticCached(w, r, ent)
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
		// Load dynamically from cookie.txt for hot-reloading (mtime-cached)
		parsedCookie, err := loadParsedCookieFile(cfg.CookieFile)
		if err != nil {
			log.Printf("[LOCAL] Failed to read local cookie file '%s': %v", cfg.CookieFile, err)
			renderNoActiveAccountsPage(w, cfg)
			return
		}
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
			var assignErr error
			activeAcc, assignErr = autoAssignNextAccount(sessionToken)
			if assignErr != nil {
				renderNoActiveAccountsPage(w, cfg)
				return
			}
		}
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

	// Prefer EscapedPath so signed media URLs keep uri:ifs%3A%2F%2F encoding.
	// Using decoded Path breaks Canva media csig → 403 "Signature invalid"
	// (blurry images + watermarks).
	escapedPath := r.URL.EscapedPath()
	if escapedPath == "" {
		escapedPath = path
	}

	// Handle CDN proxy routes
	cdnParsed, _ := url.Parse(cfg.CDNURL)
	if strings.HasPrefix(path, "/cdn-proxy/") && cdnParsed != nil {
		upstreamURL.Scheme = cdnParsed.Scheme
		upstreamURL.Host = cdnParsed.Host
		rawUp := "/" + strings.TrimPrefix(escapedPath, "/cdn-proxy/")
		upstreamURL.Path = "/" + strings.TrimPrefix(path, "/cdn-proxy/")
		if rawUp != upstreamURL.Path {
			upstreamURL.RawPath = rawUp
		} else {
			upstreamURL.RawPath = ""
		}
	}

	// Handle Extra CDN routes (/extra-cdn-N and /extra-cdn-N/...)
	isExtraCDN := false
	extraCDNIndex := -1
	isExtHost := false
	extHostName := ""
	for i, extra := range cfg.ExtraCDNDomains {
		base := fmt.Sprintf("/extra-cdn-%d", i)
		if path != base && !strings.HasPrefix(path, base+"/") {
			continue
		}
		extraClean := strings.TrimPrefix(strings.TrimPrefix(extra, "https://"), "http://")
		upstreamURL.Scheme = "https"
		if strings.HasPrefix(extra, "http://") {
			upstreamURL.Scheme = "http"
		}
		upstreamURL.Host = strings.Split(extraClean, "/")[0]
		rest := strings.TrimPrefix(path, base)
		restEsc := strings.TrimPrefix(escapedPath, base)
		if rest == "" || rest == "/" {
			upstreamURL.Path = "/"
			upstreamURL.RawPath = ""
		} else {
			upstreamURL.Path = rest
			if restEsc != rest {
				upstreamURL.RawPath = restEsc
			} else {
				upstreamURL.RawPath = ""
			}
		}
		isExtraCDN = true
		extraCDNIndex = i
		break
	}

	// Dynamic sibling hosts: /ext-host/<host.woorank.com>/...
	if !isExtraCDN && strings.HasPrefix(path, "/ext-host/") {
		rest := strings.TrimPrefix(path, "/ext-host/")
		restEsc := strings.TrimPrefix(escapedPath, "/ext-host/")
		slash := strings.Index(rest, "/")
		host := rest
		upPath := "/"
		upEsc := ""
		if slash >= 0 {
			host = rest[:slash]
			upPath = rest[slash:]
			if slash < len(restEsc) {
				upEsc = restEsc[slash:]
			}
		}
		host = strings.ToLower(strings.TrimSpace(host))
		suffix := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(cfg.CookieDomainSuffix)), ".")
		if suffix == "" {
			suffix = "woorank.com"
		}
		allowed := host == suffix || strings.HasSuffix(host, "."+suffix)
		// Never allow punching out to arbitrary hosts or looping to primary www host
		if !allowed || strings.Contains(host, "/") || strings.Contains(host, "..") || host == "www.woorank.com" {
			http.Error(w, "Forbidden host", http.StatusForbidden)
			return
		}
		upstreamURL.Scheme = "https"
		upstreamURL.Host = host
		upstreamURL.Path = upPath
		if upEsc != "" && upEsc != upPath {
			upstreamURL.RawPath = upEsc
		} else {
			upstreamURL.RawPath = ""
		}
		isExtHost = true
		extHostName = host
	}

	// Build request from EscapedPath+RawQuery so encoding survives NewRequest parse.
	upstreamURLStr := upstreamURL.Scheme + "://" + upstreamURL.Host + upstreamURL.EscapedPath()
	if upstreamURL.RawQuery != "" {
		upstreamURLStr += "?" + upstreamURL.RawQuery
	}
	var bodyReader io.Reader = r.Body
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		bodyReader = nil
	}
	upstreamReq, err := http.NewRequestWithContext(
		context.WithValue(r.Context(), proxyContextKey, activeAcc.Proxy),
		r.Method, upstreamURLStr, bodyReader,
	)
	if err != nil {
		http.Error(w, "Failed to build upstream request", http.StatusInternalServerError)
		return
	}

	// Copy headers
	for k, vv := range r.Header {
		kLower := strings.ToLower(k)
		// Never attach body-related headers on GET/HEAD (breaks Canva document-image)
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) &&
			(kLower == "content-length" || kLower == "content-type" || kLower == "transfer-encoding") {
			continue
		}
		// We set Accept-Encoding ourselves — don't forward Chrome's "…, zstd"
		if kLower == "accept-encoding" {
			continue
		}
		for _, v := range vv {
			upstreamReq.Header.Add(k, v)
		}
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		upstreamReq.Body = nil
		upstreamReq.ContentLength = 0
		upstreamReq.Header.Del("Content-Length")
		upstreamReq.Header.Del("Transfer-Encoding")
	}
	// Only encodings we can decompress (gzip/br/zstd). Prefer no zstd from origin.
	upstreamReq.Header.Del("Accept-Encoding")
	upstreamReq.Header.Set("Accept-Encoding", "gzip, deflate, br")

	// SellerAmp /api/v2 needs X-UID + X-API-TOKEN. Browser sometimes omits them
	// (session hydrate race) → 401 Unauthorized toasts. Inject from cached HTML.
	ensureAPIAuthHeaders(upstreamReq)

	// Set account cookie and user-agent.
	// IMPORTANT: use ONLY the premium/account cookie for upstream.
	// Merging the browser Cookie header caused "400 Request Header Or Cookie Too Large"
	// on sas.selleramp.com nginx and broke CSRF/session → Unauthorized toasts.
	accountCookieStr := applyLocalCookieOverlay(parseCookieFromDB(activeAcc.Cookie))
	upstreamReq.Header.Del("Cookie")
	hostWithoutPort := upstreamURL.Host
	if h, _, err := net.SplitHostPort(upstreamURL.Host); err == nil {
		hostWithoutPort = h
	}
	cookieSuffix := cfg.CookieDomainSuffix
	if cookieSuffix == "" {
		if tp, e := url.Parse(cfg.TargetURL); e == nil {
			h := tp.Host
			if hh, _, e2 := net.SplitHostPort(h); e2 == nil {
				h = hh
			}
			labels := strings.Split(h, ".")
			if len(labels) >= 2 {
				cookieSuffix = strings.Join(labels[len(labels)-2:], ".")
			} else {
				cookieSuffix = h
			}
		}
	}
	if accountCookieStr != "" && cookieSuffix != "" && strings.HasSuffix(hostWithoutPort, cookieSuffix) {
		upstreamReq.Header.Set("Cookie", accountCookieStr)
	}

	// Laravel: mutating/API requests need X-XSRF-TOKEN == URL-decoded XSRF-TOKEN cookie.
	ensureArtistlyCsrf(upstreamReq, accountCookieStr)

	// ── WebSocket upgrade: hijack and bidirectionally pipe ───────────────────────
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		ua := activeAcc.UserAgent
		if ua == "" {
			ua = cfg.UserAgent
		}
		proxyWebSocket(w, r, upstreamURL, accountCookieStr, ua, cfg, activeAcc.Proxy)
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
	} else if isExtHost && extHostName != "" {
		upstreamReq.Host = extHostName
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
		newReferer = regexp.MustCompile(`/ext-host/[^/]+`).ReplaceAllString(newReferer, "")
		upstreamReq.Header.Set("Referer", newReferer)
	} else {
		upstreamReq.Header.Set("Referer", targetBase+"/")
	}

	upstreamResp, err := doUpstreamWith429Retry(upstreamReq, accountCookieStr)
	if err != nil {
		log.Printf("[PROXY] Upstream request failed for user '%s' path '%s': %v", currentUser, path, err)
		if strings.Contains(err.Error(), "proxy dial") {
			renderProxyProblem(w, r)
			return
		}
		pushProxyLog(ProxyLogEntry{
			Source:  "PROXY",
			Level:   "error",
			Method:  r.Method,
			Path:    path,
			Status:  http.StatusBadGateway,
			User:    currentUser,
			Account: activeAcc.Name,
			Detail:  err.Error(),
			HasUID:  r.Header.Get("X-UID") != "",
			HasAPI:  r.Header.Get("X-API-TOKEN") != "",
			CookieN: countCookieNames(accountCookieStr),
		})
		if dbConnected {
			activeAcc, _ = switchToNextAccount(sessionToken, activeAcc.ID, activeAcc.Name, currentUser, "upstream_connection_error")
		}
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer upstreamResp.Body.Close()

	logUpstream := func(body []byte) {
		if !shouldLogUpstreamStatus(upstreamResp.StatusCode, path) {
			return
		}
		pushProxyLog(ProxyLogEntry{
			Source:  "UPSTREAM",
			Level:   "error",
			Method:  r.Method,
			Path:    path,
			Status:  upstreamResp.StatusCode,
			User:    currentUser,
			Account: activeAcc.Name,
			Detail:  snippetForLog(body, upstreamResp.Header.Get("Content-Type")),
			HasUID:  r.Header.Get("X-UID") != "" || upstreamReq.Header.Get("X-UID") != "",
			HasAPI:  r.Header.Get("X-API-TOKEN") != "" || upstreamReq.Header.Get("X-API-TOKEN") != "",
			CookieN: countCookieNames(accountCookieStr),
		})
	}

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
		} // Stripped here; restored on passthrough OR body is decompressed for rewrite
		if kLower == "content-length" {
			continue
		} // Will be recalculated
		if kLower == "transfer-encoding" {
			continue
		}
		if kLower == "strict-transport-security" {
			continue
		} // Strip HSTS to prevent HTTPS upgrades
		if kLower == "content-security-policy" || kLower == "content-security-policy-report-only" {
			continue
		} // CDN JS ships script-src 'none'; would break execution behind our domain
		if kLower == "x-frame-options" {
			continue
		}
		if kLower == "location" {
			for _, v := range vv {
				newLoc := rewriteWoorankHostURL(v, publicBase)
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

	// Cache static assets in browser; keep HTML/API uncached
	setProxyCacheHeaders(w, path, contentType)

	// ── 8. Streaming / SSE passthrough (Canva /_stream hydration) ────────────────
	if isSSEResponse(contentType) || isStreamingPath(path) || isStreamingContentType(contentType) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		// Restore encoding if upstream compressed a stream chunk bundle
		if ce := upstreamResp.Header.Get("Content-Encoding"); ce != "" && !isSSEResponse(contentType) {
			// For true streams leave encoding as set above (already stripped);
			// body was not decompressed — restore so browser can decode.
			w.Header().Set("Content-Encoding", ce)
		}
		w.WriteHeader(upstreamResp.StatusCode)
		flusher, canFlush := w.(http.Flusher)
		buf := make([]byte, 32*1024)
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
		logUpstream(bodyBytes)

		// Cache api token/uid from page payload for later /api/v2 calls
		captureAPICredentials(bodyBytes)

		// Rewrite domain references
		pairs := buildDomainReplacements(publicScheme, publicHost, cfg)
		bodyBytes = rewriteBody(bodyBytes, pairs)
		bodyBytes = applyTextReplacements(bodyBytes, cfg)

		// Strip SRI — HTML attributes AND Canva bootstrap asset manifests
		// (`"C":"sha512-..."`). Dynamic chunk loader verifies these hashes;
		// after URL rewrites they fail → app dies with ~15 asset requests and
		// no API calls ("Something went wrong").
		bodyBytes = stripSubresourceIntegrity(bodyBytes)

		// Remove CSP header (prevents our injected scripts)
		w.Header().Del("Content-Security-Policy")
		w.Header().Del("Content-Security-Policy-Report-Only")
		w.Header().Del("X-Frame-Options")
		if usesPanelAccountMode(cfg) {
			bodyBytes = injectDeviceHTML(bodyBytes)
		}

		// Inject our patcher script before </head> (no limit widgets)
		injectStr := patcherScript(cfg) + buildTextReplaceInjectHTML(cfg)
		if strings.TrimSpace(cfg.InjectCSS) != "" {
			injectStr += "<style>" + cfg.InjectCSS + "</style>"
			// Keep header nav hidden even after Next.js client navigations/re-renders
			injectStr += `<script>(function(){function hideSasNav(){document.querySelectorAll('a[href*="/r/sas/advanced-search"],a[href*="/sas/history"]').forEach(function(a){var ul=a.closest("ul");if(ul)ul.style.setProperty("display","none","important");});}hideSasNav();new MutationObserver(hideSasNav).observe(document.documentElement,{childList:true,subtree:true});})();</script>`
		}
		// Inject only before the FIRST </head>. Canva embeds a full error-page
		// HTML string (with its own </head>) in bootstrap — ReplaceAll would
		// splice our script into that JS string → SyntaxError and zero API calls.
		if loc := regexp.MustCompile(`(?i)</head>`).FindIndex(bodyBytes); loc != nil {
			inj := []byte(injectStr + "</head>")
			out := make([]byte, 0, len(bodyBytes)+len(inj))
			out = append(out, bodyBytes[:loc[0]]...)
			out = append(out, inj...)
			out = append(out, bodyBytes[loc[1]:]...)
			bodyBytes = out
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(upstreamResp.StatusCode)
		w.Write(bodyBytes)
		return
	}

	// ── 10. For JSON/CSS: rewrite and stream ─────────────────────────────────────
	// Do not rewrite JavaScript bodies — Canva uses SRI; mutating JS breaks loads.
	// Runtime URL rewriting is handled by the injected fetch/XHR patcher.
	isRewritable := strings.Contains(contentType, "application/json") || strings.Contains(contentType, "text/css")
	if isRewritable {
		bodyBytes, err := decompressBody(upstreamResp)
		if err == nil {
			logUpstream(bodyBytes)
			pairs := buildDomainReplacements(publicScheme, publicHost, cfg)
			bodyBytes = rewriteBody(bodyBytes, pairs)
			bodyBytes = applyTextReplacements(bodyBytes, cfg)
			if strings.Contains(contentType, "application/json") {
				bodyBytes = stripSubresourceIntegrity(bodyBytes)
			} else if strings.Contains(contentType, "text/css") {
				putStaticCached(r.Method, path, upstreamResp.StatusCode, contentType, "", bodyBytes)
			}
			w.WriteHeader(upstreamResp.StatusCode)
			w.Write(bodyBytes)
			return
		}
	}

	// ── 11. Pass through everything else ─────────────────────────────────────────
	if shouldLogUpstreamStatus(upstreamResp.StatusCode, path) {
		bodyBytes, err := decompressBody(upstreamResp)
		if err == nil {
			logUpstream(bodyBytes)
			w.WriteHeader(upstreamResp.StatusCode)
			w.Write(bodyBytes)
			return
		}
	}
	// Body may still be gzip/br compressed. We stripped Content-Encoding above
	// (rewrite paths decompress). Restoring it here is required — otherwise the
	// browser parses compressed bytes as JS → SyntaxError and Canva never fires APIs.
	ce := upstreamResp.Header.Get("Content-Encoding")
	if ce != "" {
		w.Header().Set("Content-Encoding", ce)
	}
	// Buffer static JS/fonts/images into memory cache so splash reloads are instant.
	if isCacheableStaticPath(path) && upstreamResp.StatusCode == 200 {
		bodyBytes, err := io.ReadAll(upstreamResp.Body)
		if err == nil {
			putStaticCached(r.Method, path, upstreamResp.StatusCode, contentType, ce, bodyBytes)
			w.Header().Set("Content-Length", strconv.Itoa(len(bodyBytes)))
			w.Header().Set("X-OCG-Cache", "MISS")
			w.WriteHeader(upstreamResp.StatusCode)
			w.Write(bodyBytes)
			return
		}
	}
	if cl := upstreamResp.Header.Get("Content-Length"); cl != "" {
		w.Header().Set("Content-Length", cl)
	}
	w.WriteHeader(upstreamResp.StatusCode)
	io.Copy(w, upstreamResp.Body)
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

func warmCFCookies(cfg Config) {
	if usesPanelAccountMode(cfg) {
		return
	}
	go func() {
		for {
			parsed, err := loadParsedCookieFile(cfg.CookieFile)
			if err != nil {
				time.Sleep(2 * time.Minute)
				continue
			}
			base := applyLocalCookieOverlay(parsed)
			home := cfg.HomePath
			if home == "" {
				home = "/"
			}
			warmURL := strings.TrimRight(cfg.TargetURL, "/") + home
			req, err := http.NewRequest("GET", warmURL, nil)
			if err != nil {
				time.Sleep(2 * time.Minute)
				continue
			}
			req.Header.Set("User-Agent", cfg.UserAgent)
			req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
			req.Header.Set("Accept-Language", "en-US,en;q=0.9")
			if base != "" {
				req.Header.Set("Cookie", base)
			}
			resp, err := httpClient.Do(req)
			if err != nil {
				log.Printf("[COOKIE] Warm-up failed: %v", err)
			} else {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode == http.StatusForbidden {
					log.Printf("[COOKIE] Warm-up got 403 from %s — skipping absorb", warmURL)
				} else {
					changed := absorbUpstreamSetCookies(resp.Header, resp.StatusCode)
					if changed {
						log.Printf("[COOKIE] Warm-up refreshed CF/session cookies (status %d)", resp.StatusCode)
					} else {
						log.Printf("[COOKIE] Warm-up ok status=%d (no new Set-Cookie)", resp.StatusCode)
					}
				}
			}
			// Prefetch common static assets into memory cache (fills splash path).
			warmStaticAssets(cfg, base)
			time.Sleep(3 * time.Minute)
		}
	}()
}

func warmStaticAssets(cfg Config, cookieHeader string) {
	base := strings.TrimRight(cfg.TargetURL, "/")
	home := cfg.HomePath
	if home == "" {
		home = "/"
	}
	assets := []string{}
	// Scrape homepage / dashboard HTML for common asset paths (js/css/gif under /assets, /static, /dist, etc.)
	// httpClient does not follow redirects — chase Location a few hops so / → /teams/.../dashboard works.
	scrapeURL := base + home
	var body []byte
	for hop := 0; hop < 5; hop++ {
		req, err := http.NewRequest("GET", scrapeURL, nil)
		if err != nil {
			break
		}
		req.Header.Set("User-Agent", cfg.UserAgent)
		req.Header.Set("Accept", "text/html")
		if cookieHeader != "" {
			req.Header.Set("Cookie", cookieHeader)
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			break
		}
		chunk, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := resp.Header.Get("Location")
			if loc == "" {
				break
			}
			if strings.HasPrefix(loc, "/") {
				scrapeURL = base + loc
			} else if strings.HasPrefix(loc, "http") {
				scrapeURL = loc
			} else {
				break
			}
			continue
		}
		if resp.StatusCode == 200 {
			body = chunk
		}
		break
	}
	if len(body) > 0 {
		re := regexp.MustCompile(`(?:src|href)=["'](/?(?:assets|static|dist|packs|webpack)[^"']+\.(?:js|css|gif|png|svg|woff2?)[^"']*)["']`)
		re2 := regexp.MustCompile(`(?:src|href)=(/assets/[^>\s]+\.(?:js|css))`)
		seen := map[string]bool{}
		for _, reX := range []*regexp.Regexp{re, re2} {
			for _, m := range reX.FindAllSubmatch(body, -1) {
				p := string(m[1])
				if !strings.HasPrefix(p, "/") {
					p = "/" + p
				}
				if !seen[p] {
					seen[p] = true
					assets = append(assets, p)
				}
			}
		}
	}
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	var mu sync.Mutex
	cached := 0
	for _, p := range assets {
		if getStaticCached(http.MethodGet, p) != nil {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(path string) {
			defer wg.Done()
			defer func() { <-sem }()
			req, err := http.NewRequest("GET", base+path, nil)
			if err != nil {
				return
			}
			req.Header.Set("User-Agent", cfg.UserAgent)
			req.Header.Set("Accept", "*/*")
			if cookieHeader != "" {
				req.Header.Set("Cookie", cookieHeader)
			}
			resp, err := httpClient.Do(req)
			if err != nil {
				return
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 {
				putStaticCached(http.MethodGet, path, 200, resp.Header.Get("Content-Type"), resp.Header.Get("Content-Encoding"), body)
				mu.Lock()
				cached++
				mu.Unlock()
			}
		}(p)
	}
	wg.Wait()
	log.Printf("[CACHE] Static warm-up done (%d assets fetched, %d total listed)", cached, len(assets))
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	cfg := loadConfig()
	log.Printf("🚀 Starting Generic Tool Proxy — Tool: %s | Target: %s | Port: %s", cfg.ToolName, cfg.TargetURL, cfg.Port)
	initDB(cfg)
	resolveWebsiteID(cfg.PublicHost)
	startDailyResetCron()
	warmCFCookies(cfg)

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
	mux.HandleFunc("/__logs", liveLogsPageHandler)
	mux.HandleFunc("/__logs.json", liveLogsJSONHandler)

	// ── Logout ────────────────────────────────────────────────────────────────────
	mux.HandleFunc("/user/logout", func(w http.ResponseWriter, r *http.Request) {
		if dbConnected {
			if c, err := r.Cookie("ct_session"); err == nil {
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
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
		})
		cfg := loadConfig()
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
		if r.Header.Get("X-Forwarded-Proto") == "http" && strings.EqualFold(cfg.PublicScheme, "https") {
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
		ReadTimeout:  600 * time.Second,
		WriteTimeout: 600 * time.Second,
		IdleTimeout:  180 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("[SERVER] Fatal: %v", err)
	}
}
