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

const MOCK_TOKEN = "bypass-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"

// Config holds all runtime-configurable settings loaded from config.json.
// Hot-reloadable: file is re-read on every request if it has changed.
type Config struct {
	UserAgent              string `json:"user_agent"`
	Port                   string `json:"port"`
	CookieFile             string `json:"cookie_file"`
	TargetURL              string `json:"target_url"`
	CDNURL                 string `json:"cdn_url"`
	PublicHost             string `json:"public_host"`
	PublicScheme           string `json:"public_scheme"`
	PanelDB                string `json:"panel_db"`
	MySQLHost              string `json:"mysql_host"`
	MySQLPort              string `json:"mysql_port"`
	MySQLUser              string `json:"mysql_user"`
	MySQLPassword          string `json:"mysql_password"`
	MySQLDB                string `json:"mysql_db"`
	SecretKey              string `json:"secret_key"`
	SessionDurationMinutes int    `json:"session_duration_minutes"`
	MemberAreaURL          string `json:"member_area_url"`
}

var CONFIG_FILE = func() string {
	if v := strings.TrimSpace(os.Getenv("CONFIG_FILE")); v != "" {
		return v
	}
	if _, err := os.Stat("config.local.json"); err == nil {
		return "config.local.json"
	}
	return "config.json"
}()

// overviewSwitchState tracks the last Overview target charged for a user so we can
// ignore / reverse Ahrefs' stale re-fetch of the previous domain when switching searches.
type overviewSwitchState struct {
	current       string
	prev          string
	suppressUntil time.Time
	lastCommitAt  time.Time
}

func creditUserKey(user string, websiteID int) string {
	return fmt.Sprintf("%s:%d", user, websiteID)
}

func reverseRecentOverviewCredit(username string, websiteID int, target string) {
	if db == nil || target == "" || target == "unknown_target" {
		return
	}
	endpoint := "Overview: " + target
	if len(endpoint) > 255 {
		endpoint = endpoint[:255]
	}
	var id int64
	err := db.QueryRow(
		`SELECT id FROM ahrefs_credit_logs
		 WHERE username = ? AND website_id = ? AND endpoint = ?
		   AND timestamp >= DATE_SUB(NOW(), INTERVAL 30 SECOND)
		 ORDER BY id DESC LIMIT 1`,
		username, websiteID, endpoint,
	).Scan(&id)
	if err != nil {
		return
	}
	res, err := db.Exec(`DELETE FROM ahrefs_credit_logs WHERE id = ?`, id)
	if err != nil {
		log.Printf("[LIMITS] Failed reversing stale Overview credit id=%d: %v", id, err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("[LIMITS] Reversed stale Overview credit for '%s' target=%s (id=%d) under website_id %d", username, target, id, websiteID)
	}
}

var (
	creditHitCache sync.Map
	// When user searches domain B, Ahrefs often re-fetches Overview for previous domain A
	// within a few seconds. Suppress only that stale A Overview briefly — intentional
	// re-search of A later (or another tab after the grace window) still bills.
	creditOverviewSwitch sync.Map
	currentConfig        Config
	configModTime        time.Time
	currentWebsiteID     int = 1
	defaultConfig            = Config{
		UserAgent:              "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36",
		Port:                   "7842",
		CookieFile:             "cookie.txt",
		TargetURL:              "https://app.ahrefs.com",
		CDNURL:                 "https://static-app.ahrefs.com",
		PublicHost:             "",
		PublicScheme:           "https",
		MySQLHost:              "127.0.0.1",
		MySQLPort:              "3306",
		MySQLUser:              "root",
		MySQLPassword:          "",
		MySQLDB:                "ahrefs_db",
		SecretKey:              "toolsmandi_ahrefs_secret_xyz123",
		SessionDurationMinutes: 30,
	}
)

func resolveWebsiteID(publicHost string) {
	if db == nil {
		currentWebsiteID = 1
		log.Printf("[DB] MySQL is not used. website_id stays %d", currentWebsiteID)
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

// loadConfig reads config.json, hot-reloads only when file has changed.
// Falls back to defaultConfig if file is missing or malformed.
func loadConfig() Config {
	info, err := os.Stat(CONFIG_FILE)
	if err != nil {
		return currentConfig
	}
	if !info.ModTime().After(configModTime) {
		return currentConfig // No change — return cached
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

	// Fill defaults for any missing fields
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

	// Only log if something actually changed
	if cfg.UserAgent != currentConfig.UserAgent {
		log.Printf("[CONFIG] User-Agent updated → %s", cfg.UserAgent)
	}
	if cfg.CookieFile != currentConfig.CookieFile {
		log.Printf("[CONFIG] Cookie file updated → %s", cfg.CookieFile)
	}

	currentConfig = cfg
	configModTime = info.ModTime()
	log.Printf("[CONFIG] Reloaded from %s ✅", CONFIG_FILE)
	return currentConfig
}

// ── DATABASE SYSTEM ──────────────────────────────────────────────────────────

var db *sql.DB

func initDB(cfg Config) {
	// parseTime=true is required to map DATETIME to time.Time in Go
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
		log.Printf("[DB] MySQL is not used for this local copy: %v", err)
		_ = db.Close()
		db = nil
		return
	}

	log.Printf("[DB] Connected to MySQL successfully! Database: %s ✅", cfg.MySQLDB)

	// Auto-create tables if they don't exist
	tables := []string{
		`CREATE TABLE IF NOT EXISTS ahrefs_accounts (
			id INT AUTO_INCREMENT PRIMARY KEY,
			name VARCHAR(100) NOT NULL,
			cookie TEXT NOT NULL,
			user_agent TEXT NOT NULL,
			proxy VARCHAR(255) DEFAULT '',
			status ENUM('active', 'logged_out', 'blocked') DEFAULT 'active',
			last_used_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			failure_count INT DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX (status)
		) ENGINE=InnoDB;`,

		`CREATE TABLE IF NOT EXISTS ahrefs_users (
			id INT AUTO_INCREMENT PRIMARY KEY,
			username VARCHAR(100) NOT NULL UNIQUE,
			credit_limit INT DEFAULT 50,
			export_limit INT DEFAULT 100000,
			export_cycle_start DATETIME DEFAULT CURRENT_TIMESTAMP,
			status ENUM('active', 'suspended') DEFAULT 'active',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX (username)
		) ENGINE=InnoDB;`,

		`CREATE TABLE IF NOT EXISTS ahrefs_credit_logs (
			id INT AUTO_INCREMENT PRIMARY KEY,
			username VARCHAR(100) NOT NULL,
			endpoint VARCHAR(255) NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			INDEX (username),
			INDEX (timestamp)
		) ENGINE=InnoDB;`,

		`CREATE TABLE IF NOT EXISTS ahrefs_export_logs (
			id INT AUTO_INCREMENT PRIMARY KEY,
			username VARCHAR(100) NOT NULL,
			rows_count INT NOT NULL,
			endpoint VARCHAR(255) NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			INDEX (username),
			INDEX (timestamp)
		) ENGINE=InnoDB;`,

		// ahrefs_products: aMember product IDs jo Ahrefs access ke liye authorized hain
		`CREATE TABLE IF NOT EXISTS ahrefs_products (
			id INT AUTO_INCREMENT PRIMARY KEY,
			product_id INT NOT NULL UNIQUE,
			product_name VARCHAR(200) DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB;`,

		// ahrefs_tokens: One-Time Tokens (OTT) for aMember → proxy handshake
		`CREATE TABLE IF NOT EXISTS ahrefs_tokens (
			id INT AUTO_INCREMENT PRIMARY KEY,
			token VARCHAR(128) NOT NULL UNIQUE,
			username VARCHAR(100) NOT NULL,
			client_ip VARCHAR(64) NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX (token),
			INDEX (expires_at)
		) ENGINE=InnoDB;`,

		// ahrefs_sessions: Active user proxy sessions with absolute expiry
		`CREATE TABLE IF NOT EXISTS ahrefs_sessions (
			id INT AUTO_INCREMENT PRIMARY KEY,
			session_token VARCHAR(128) NOT NULL UNIQUE,
			username VARCHAR(100) NOT NULL,
			client_ip VARCHAR(64) NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX (session_token),
			INDEX (expires_at)
		) ENGINE=InnoDB;`,

		// ahrefs_login_logs: Tracks every user login (IP, device, time)
		`CREATE TABLE IF NOT EXISTS ahrefs_login_logs (
			id INT AUTO_INCREMENT PRIMARY KEY,
			website_id INT NOT NULL,
			username VARCHAR(100) NOT NULL,
			client_ip VARCHAR(64) NOT NULL,
			user_agent TEXT,
			logged_in_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			INDEX (website_id),
			INDEX (username),
			INDEX (logged_in_at)
		) ENGINE=InnoDB;`,

		// ahrefs_switch_logs: Tracks every account switch with reason
		`CREATE TABLE IF NOT EXISTS ahrefs_switch_logs (
			id INT AUTO_INCREMENT PRIMARY KEY,
			website_id INT NOT NULL,
			session_token VARCHAR(128),
			username VARCHAR(100),
			from_account_id INT,
			from_account_name VARCHAR(100),
			to_account_id INT,
			to_account_name VARCHAR(100),
			reason VARCHAR(255),
			switched_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			INDEX (website_id),
			INDEX (switched_at)
		) ENGINE=InnoDB;`,

		// ahrefs_violations_logs: Tracks unauthorized URL access attempts (logs automatically cleaned after 30 days)
		`CREATE TABLE IF NOT EXISTS ahrefs_violations_logs (
			id INT AUTO_INCREMENT PRIMARY KEY,
			website_id INT NOT NULL,
			username VARCHAR(100) NOT NULL,
			client_ip VARCHAR(50) NOT NULL,
			attempted_path VARCHAR(255) NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			INDEX (website_id),
			INDEX (username),
			INDEX (timestamp)
		) ENGINE=InnoDB;`,
	}

	for i, q := range tables {
		if _, err := db.Exec(q); err != nil {
			log.Printf("[DB] ⚠️ Auto-create table %d failed: %v", i+1, err)
		}
	}

	// Cleanup expired tokens and sessions on startup
	_, _ = db.Exec("DELETE FROM ahrefs_tokens WHERE expires_at < NOW()")
	_, _ = db.Exec("DELETE FROM ahrefs_sessions WHERE expires_at < NOW()")
	log.Printf("[DB] Expired OTT tokens and sessions cleaned on startup ✅")
}

// ── LIMITS CONFIG & CRON ─────────────────────────────────────────────────────

var countedEndpoints = map[string]bool{
	"/v4/caGetKeywords": true, "/v4/seGetDomainRating": true, "/v4/seBacklinks": true,
	"/v4/seGetTopPages": true, "/v4/seDetectAi": true, "/v4/seGetHtmlSnapshot": true, "/v4/keGetTopPositionsHistory": true,
	"/v4/keIdeas": true, "/v4/seRefdomains": true, "/v4/seGetOrganicKeywords": true, "/v4/keKeywordsOverview": true,
	"/v4/seGetPaidKeywordsV2": true, "/v4/seGetSiteStructureMetrics": true, "/v4/seInternalBacklinks": true,
	"/v4/seAnchors": true, "/v4/ceSearchResults": true, "/v4/keUpdate": true, "/v4/ceAuthorsReport": true,
	"/v4/keGetClustersByParentTopics": true, "/v4/seSerpOverview": true, "/v4/seGetOrganicCompetitors": true,
	"/v4/seGetPagesMovements": true, "/v4/seRefdomainsCalendar": true, "/v4/seGetPositionsMovements": true,
	"/v4/keGetClustersByTerms": true, "/v4/ceWebsitesReport": true, "/v4/seBrokenBacklinks": true,
	"/v4/ceLanguagesReport": true, "/v4/seLinkedDomains": true, "/v4/seGetPaidPagesV2": true, "/v4/seGetAdsV2": true,
	"/v4/caLinkIntersect": true, "/v4/seRefIPs": true, "/v4/seAuthors": true, "/v4/seGetTopPagesByLinks": true,
	"/v4/seLinks": true, "/v4/seLinkedAnchors": true, "/v4/keGetTrafficByDomains": true, "/v4/keGetTrafficByPages": true,
	"/v4/seGetOrganicPositionsByCountry": true, "/v4/seCrawledPagesList": true, "/v4/tkGetProjectsAvailability": true,
	"/v4/seBacklinksCalendarData": true, "/v4/seGetAds": true, "/v4/seGetTopLandingPagesHistoryByPage": true,
	"/v4/seGetTopLandingPages": true, "/v4/keAdsByDomains": true,
}

func isBlockedPath(path string) bool {
	// Strip query parameters
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}
	path = strings.TrimSuffix(path, "/")

	blocked := map[string]bool{
		"/account/my-account":            true,
		"/account/security":              true,
		"/account/my-notifications":      true,
		"/account/my-certificates":       true,
		"/account/settings":              true,
		"/account/saml-sso":              true,
		"/account/members/confirmed":     true,
		"/account/members/pending":       true,
		"/account/tools-and-permissions": true,
		"/account/limits-and-usage/web":  true,
		"/account/billing/subscriptions": true,
		"/account/api-keys":              true,
		"/account/integrations":          true,
		"/account/applications":          true,
		"/account/agency-profile/0":      true,
		"/account/audit-log":             true,
		"/account/api-log":               true,
		"/account/academy":               true,
		"/academy":                       true,
	}

	return blocked[path]
}

func isCounted(path string) bool {
	// Strip query parameters
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}
	// Only charge on real Ahrefs data APIs — NOT HTML page navigations.
	// HTML /site-explorer/overview + /v4/...Overview API used to each cost 1 credit
	// for the same search; page loads are free, APIs are metered.
	if countedEndpoints[path] {
		return true
	}
	return false
}

func batchAnalysisEndpoint(path string) bool {
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}
	// Only the analysis run itself consumes Ahrefs credits (not every /v4/ba* helper call).
	return path == "/v4/baTable"
}

// batchAnalysisCreditCost: Ahrefs charges 1 credit per 200 targets (1–200 → 1, 201–400 → 2, …).
func batchAnalysisCreditCost(targetCount int) int {
	if targetCount <= 0 {
		return 0
	}
	return (targetCount + 199) / 200
}

func parseBatchTargetCount(body []byte) int {
	if len(body) == 0 {
		return 0
	}
	var payload struct {
		Targets []json.RawMessage `json:"targets"`
		Input   *struct {
			Targets []json.RawMessage `json:"targets"`
		} `json:"input"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0
	}
	if n := len(payload.Targets); n > 0 {
		return n
	}
	if payload.Input != nil {
		return len(payload.Input.Targets)
	}
	return 0
}

func parseBatchResponseRowCount(body []byte) int {
	var arr []json.RawMessage
	if err := json.Unmarshal(body, &arr); err != nil || len(arr) < 2 {
		return 0
	}
	var data struct {
		Rows []json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(arr[1], &data); err != nil {
		return 0
	}
	return len(data.Rows)
}

func writeWeeklyExportLimitJSON(w http.ResponseWriter, used, limit int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Limit-Reached", "weekly_export")
	w.WriteHeader(http.StatusForbidden)
	fmt.Fprintf(w, `{"error":"weekly_export_limit_reached","message":"Weekly Export Rows Limit Reached. You have reached your weekly CSV export row limit (%d / %d rows).","code":403}`, used, limit)
}

func writeDailyCreditLimitJSON(w http.ResponseWriter, used, limit int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Limit-Reached", "daily_credit")
	w.WriteHeader(http.StatusForbidden)
	fmt.Fprintf(w, `{"error":"daily_credit_limit_reached","message":"Your daily Ahrefs credit limit has been reached (%d / %d). Batch analysis uses 1 credit per 200 URLs. Please try again tomorrow at midnight IST.","code":403}`, used, limit)
}

// normalizeTarget collapses toolsmandi.com / toolsmandi.com/ / https://... into one dedup key.
func normalizeTarget(raw string) string {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" || s == "unknown_target" {
		return "unknown_target"
	}
	// If extractTarget accidentally got a JSON blob, pull domain/url out of it.
	if strings.HasPrefix(s, "{") {
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &payload); err == nil {
			if extracted := targetFromJSONMap(payload); extracted != "" {
				s = strings.TrimSpace(strings.ToLower(extracted))
			}
		}
	}
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "www.")
	s = strings.TrimRight(s, "/")
	if s == "" {
		return "unknown_target"
	}
	return s
}

func targetFromJSONMap(payload map[string]interface{}) string {
	keys := []string{"target", "keyword", "domain", "url", "query", "q", "input"}
	for _, key := range keys {
		if val, ok := payload[key]; ok {
			if strVal, ok := val.(string); ok && strings.TrimSpace(strVal) != "" {
				return strVal
			}
		}
	}
	if args, ok := payload["args"].(map[string]interface{}); ok {
		if t := targetFromJSONMap(args); t != "" {
			return t
		}
	}
	if mt, ok := payload["multiTarget"].([]interface{}); ok {
		for _, item := range mt {
			if m, ok := item.(map[string]interface{}); ok {
				if t := targetFromJSONMap(m); t != "" {
					return t
				}
			}
		}
	}
	if args, ok := payload["args"].(map[string]interface{}); ok {
		if mt, ok := args["multiTarget"].([]interface{}); ok {
			for _, item := range mt {
				if m, ok := item.(map[string]interface{}); ok {
					if t := targetFromJSONMap(m); t != "" {
						return t
					}
				}
			}
		}
	}
	for _, key := range []string{"keywords", "targets", "domains"} {
		if val, ok := payload[key]; ok {
			if arr, ok := val.([]interface{}); ok && len(arr) > 0 {
				if firstVal, ok := arr[0].(string); ok && strings.TrimSpace(firstVal) != "" {
					return firstVal
				}
			}
		}
	}
	return ""
}

func extractTarget(r *http.Request) string {
	if r == nil {
		return "unknown_target"
	}

	// Helper to extract target from query map
	extractFromValues := func(vals url.Values) string {
		keys := []string{"target", "keyword", "domain", "url", "query", "q", "input"}
		for _, k := range keys {
			if v := vals.Get(k); v != "" {
				return v
			}
		}
		arrKeys := []string{"keyword[]", "domain[]", "domains[]", "targets[]"}
		for _, k := range arrKeys {
			if list, ok := vals[k]; ok && len(list) > 0 && list[0] != "" {
				return list[0]
			}
		}
		return ""
	}

	// 1. Check URL query parameters
	if t := extractFromValues(r.URL.Query()); t != "" {
		return t
	}

	// 2. POST/PUT JSON body (including nested args.url / multiTarget)
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		if r.Body != nil {
			bodyBytes, err := io.ReadAll(r.Body)
			if err == nil {
				r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
				if len(bodyBytes) > 0 {
					var payload map[string]interface{}
					if err := json.Unmarshal(bodyBytes, &payload); err == nil {
						if t := targetFromJSONMap(payload); t != "" {
							return t
						}
					}
				}
			}
		}
	}

	// 3. Fallback RequestURI query
	if idx := strings.Index(r.RequestURI, "?"); idx != -1 {
		query := r.RequestURI[idx+1:]
		vals, err := url.ParseQuery(query)
		if err == nil {
			if t := extractFromValues(vals); t != "" {
				return t
			}
		}
	}

	return "unknown_target"
}

func getReportGroup(path string) string {
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}
	if strings.Contains(path, "overview") || strings.Contains(path, "DomainRating") || strings.Contains(path, "keIdeas") {
		return "Overview"
	}
	if strings.Contains(path, "Backlink") || strings.Contains(path, "Refdomain") || strings.Contains(path, "LinkedDomain") || strings.Contains(path, "RefIP") || strings.Contains(path, "Anchor") || strings.Contains(path, "Links") || strings.Contains(path, "LinkIntersect") {
		return "Backlinks"
	}
	if strings.Contains(path, "Organic") || strings.Contains(path, "Serp") || strings.Contains(path, "Movement") {
		return "Organic Search"
	}
	if strings.Contains(path, "Paid") || strings.Contains(path, "Ads") {
		return "Paid Search"
	}
	if strings.Contains(path, "TopPages") || strings.Contains(path, "LandingPages") || strings.Contains(path, "SiteStructure") || strings.Contains(path, "Authors") || strings.Contains(path, "CrawledPages") {
		return "Pages"
	}
	if strings.Contains(path, "keKeyword") || strings.Contains(path, "keGetClusters") || strings.Contains(path, "TrafficBy") || strings.Contains(path, "TopPositions") {
		return "Keywords Explorer"
	}
	if strings.Contains(path, "ceSearch") || strings.Contains(path, "ceAuthor") || strings.Contains(path, "ceWebsites") || strings.Contains(path, "ceLanguages") {
		return "Content Explorer"
	}
	return "Other"
}

func startDailyResetCron() {
	go func() {
		for {
			now := time.Now()
			// Load Asia/Kolkata timezone safely
			loc, err := time.LoadLocation("Asia/Kolkata")
			if err != nil {
				log.Printf("[CRON] ⚠️ Location Asia/Kolkata not found, using Local time: %v", err)
				loc = time.Local
			}

			nowInIST := now.In(loc)

			// Schedule sleep duration until next midnight (00:00:00) IST
			tomorrowMidnight := time.Date(nowInIST.Year(), nowInIST.Month(), nowInIST.Day()+1, 0, 0, 0, 0, loc)
			duration := tomorrowMidnight.Sub(nowInIST)

			log.Printf("[CRON] Daily credit reset scheduled in %v (at %v IST) ⏰", duration, tomorrowMidnight)
			time.Sleep(duration)

			// Reset credit logs at midnight IST!
			if db != nil {
				_, err := db.Exec("TRUNCATE TABLE ahrefs_credit_logs")
				if err != nil {
					log.Printf("[CRON] ⚠️ Credit logs truncate failed: %v", err)
				} else {
					log.Printf("[CRON] Midnight IST Credit Logs Reset completed successfully! Daily limit refreshed. 🔄✅")
				}
			}
		}
	}()
}

type AhrefsAccount struct {
	ID        int
	Name      string
	Cookie    string
	UserAgent string
	Proxy     string
	ShowLimit bool
}

func selectActiveAccount() (AhrefsAccount, error) {
	var acc AhrefsAccount
	var showLimitVal int
	if db == nil {
		return acc, fmt.Errorf("database not connected")
	}
	// Least Recently Used: distributes requests evenly, isolated by website_id
	query := "SELECT id, name, cookie, user_agent, proxy, show_limit FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' ORDER BY last_used_at ASC LIMIT 1"
	err := db.QueryRow(query, currentWebsiteID).Scan(&acc.ID, &acc.Name, &acc.Cookie, &acc.UserAgent, &acc.Proxy, &showLimitVal)
	if err != nil {
		return acc, err
	}
	acc.ShowLimit = showLimitVal == 1

	// Move this account to the back of the line by updating its last_used_at timestamp
	_, _ = db.Exec("UPDATE ahrefs_accounts SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?", acc.ID)

	return acc, nil
}

func getSessionAssignedAccount(sessionToken string) (AhrefsAccount, bool) {
	var acc AhrefsAccount
	var showLimitVal int
	var assignedAccountID sql.NullInt64

	// 1. Get assigned_account_id from session record
	err := db.QueryRow("SELECT assigned_account_id FROM ahrefs_sessions WHERE session_token = ? AND website_id = ?", sessionToken, currentWebsiteID).Scan(&assignedAccountID)
	if err != nil || !assignedAccountID.Valid {
		return acc, false
	}

	// 2. Load healthy account from accounts table (status = active only)
	err = db.QueryRow("SELECT id, name, cookie, user_agent, proxy, show_limit FROM ahrefs_accounts WHERE id = ? AND website_id = ? AND status = 'active'", assignedAccountID.Int64, currentWebsiteID).Scan(&acc.ID, &acc.Name, &acc.Cookie, &acc.UserAgent, &acc.Proxy, &showLimitVal)
	if err != nil {
		return acc, false
	}

	acc.ShowLimit = showLimitVal == 1
	return acc, true
}

// autoAssignNextAccount picks the LRU active account and maps it to the session.
func autoAssignNextAccount(sessionToken string) (AhrefsAccount, error) {
	acc, err := selectActiveAccount()
	if err != nil {
		return acc, err
	}
	// Map this session to the newly assigned account ID
	_, _ = db.Exec("UPDATE ahrefs_sessions SET assigned_account_id = ? WHERE session_token = ? AND website_id = ?", acc.ID, sessionToken, currentWebsiteID)
	log.Printf("[LB] Assigned active account '%s' (ID: %d) to session '%s'", acc.Name, acc.ID, sessionToken)
	return acc, nil
}

// sanitizeCookieHeader removes invalid HTTP/2 header characters from a cookie string.
// Newlines, carriage returns, and null bytes are stripped; surrounding whitespace trimmed.
func sanitizeCookieHeader(s string) string {
	replacer := strings.NewReplacer("\r", "", "\n", "", "\x00", "", "\t", " ")
	return strings.TrimSpace(replacer.Replace(s))
}

// stripSensitiveCookies strips out critical upstream session cookies (BSSESSID, etc.)
// so the user's browser never gets access to them, and any malicious/hijacked
// cookie sent by the browser is ignored.
func stripSensitiveCookies(cookieHeader string) string {
	if cookieHeader == "" {
		return ""
	}
	parts := strings.Split(cookieHeader, ";")
	var kept []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		eqIdx := strings.Index(trimmed, "=")
		if eqIdx != -1 {
			name := strings.TrimSpace(trimmed[:eqIdx])
			nameLower := strings.ToLower(name)
			if nameLower == "bssessid" ||
				nameLower == "ahrefs_sid" ||
				nameLower == "ahrefs_uid" ||
				nameLower == "lhc" {
				continue
			}
		}
		kept = append(kept, trimmed)
	}
	return strings.Join(kept, "; ")
}

// parseCookieFromDB converts the stored cookie to HTTP Cookie header format.
// It auto-detects two input formats:
//
//  1. JSON array  : [{"name":"a","value":"b"}, ...]  — exported by browser extensions (EditThisCookie, Cookie-Editor etc.)
//  2. Raw string  : name=value; name2=value2           — Netscape/raw format, pasted directly
//
// Both formats are accepted. If JSON parsing fails, the raw string is used as-is.
func parseCookieFromDB(raw string) string {
	raw = sanitizeCookieHeader(raw)
	if raw == "" {
		return ""
	}

	// Detect JSON array, or GoAuto {"referer"?: "...", "cookies":[...]} — referer is optional.
	trimmed := strings.TrimSpace(raw)
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
		// Already raw format (name=value; ...) — return as-is
		return raw
	}

	// Parse JSON cookie array (format exported by browser extension tools)
	var cookies []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(trimmed), &cookies); err != nil {
		// JSON parse failed — fall back to raw value
		log.Printf("[COOKIE] ⚠️ Could not parse JSON cookie array: %v — using raw value", err)
		return raw
	}

	parts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		if c.Name != "" {
			parts = append(parts, c.Name+"="+c.Value)
		}
	}
	result := strings.Join(parts, "; ")
	log.Printf("[COOKIE] ✅ Parsed %d cookies from JSON array format", len(parts))
	return result
}

// switchToNextAccount performs a circular round-robin switch to the next active account.
// It does NOT change any account status — accounts remain 'active' always.
// It picks the next account by ID that comes after currentAccID, wrapping around.
// username and reason are stored in ahrefs_switch_logs for analytics.
func switchToNextAccount(sessionToken string, currentAccID int, currentAccName string, username string, reason string) (AhrefsAccount, error) {
	var acc AhrefsAccount
	var showLimitVal int

	// Do NOT change account status dynamically. Accounts remain 'active' globally.
	// We simply rotate to the next active account in the round-robin loop.
	log.Printf("[LB] 🛡️ Rotating out failed account '%s' (ID: %d) for this session due to trigger: %s", currentAccName, currentAccID, reason)

	// Try the next higher-ID active account first (circular)
	err := db.QueryRow(
		"SELECT id, name, cookie, user_agent, proxy, show_limit FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' AND id > ? ORDER BY id ASC LIMIT 1",
		currentWebsiteID, currentAccID,
	).Scan(&acc.ID, &acc.Name, &acc.Cookie, &acc.UserAgent, &acc.Proxy, &showLimitVal)

	if err == sql.ErrNoRows {
		// Wrap around — pick the first/lowest-ID active account (circular)
		err = db.QueryRow(
			"SELECT id, name, cookie, user_agent, proxy, show_limit FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' AND id != ? ORDER BY id ASC LIMIT 1",
			currentWebsiteID, currentAccID,
		).Scan(&acc.ID, &acc.Name, &acc.Cookie, &acc.UserAgent, &acc.Proxy, &showLimitVal)
	}
	if err != nil {
		// Only one account or none — re-use the same one
		err = db.QueryRow(
			"SELECT id, name, cookie, user_agent, proxy, show_limit FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' ORDER BY id ASC LIMIT 1",
			currentWebsiteID,
		).Scan(&acc.ID, &acc.Name, &acc.Cookie, &acc.UserAgent, &acc.Proxy, &showLimitVal)
	}
	if err != nil {
		return acc, fmt.Errorf("no active accounts available for website_id %d", currentWebsiteID)
	}

	acc.ShowLimit = showLimitVal == 1

	// Update last_used_at for round-robin fairness
	_, _ = db.Exec("UPDATE ahrefs_accounts SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?", acc.ID)

	// Re-assign session to new account
	if sessionToken != "" {
		_, _ = db.Exec("UPDATE ahrefs_sessions SET assigned_account_id = ? WHERE session_token = ? AND website_id = ?", acc.ID, sessionToken, currentWebsiteID)
	}

	// Log the switch to analytics table
	_, _ = db.Exec(
		"INSERT INTO ahrefs_switch_logs (website_id, session_token, username, from_account_id, from_account_name, to_account_id, to_account_name, reason) VALUES (?,?,?,?,?,?,?,?)",
		currentWebsiteID, sessionToken, username, currentAccID, currentAccName, acc.ID, acc.Name, reason,
	)

	log.Printf("[LB] 🔄 Switched from '%s' (ID: %d) → '%s' (ID: %d) for user '%s' | Reason: %s", currentAccName, currentAccID, acc.Name, acc.ID, username, reason)
	return acc, nil
}

// ── AUTHENTICATION SYSTEM ────────────────────────────────────────────────────

func generateSignature(username, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(username))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func verifySignature(username, sig, secret string) bool {
	expected := generateSignature(username, secret)
	return hmac.Equal([]byte(sig), []byte(expected))
}

// generateOTT creates a cryptographically secure random 64-byte hex token
func generateOTT() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// realClientIP extracts the real client IP from the request
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
		return "", fmt.Errorf("database session lookup error: %w", err)
	}

	if time.Now().After(expiresAt) {
		// Delete expired session immediately on access
		_, _ = db.Exec("DELETE FROM ahrefs_sessions WHERE session_token = ? AND website_id = ?", sessionToken, currentWebsiteID)
		return "", fmt.Errorf("session expired")
	}

	return username, nil
}

// ── AUTH HANDSHAKE HANDLER (aMemberPro → Go Proxy) ───────────────────────────
//
// Called server-to-server by ahrefs_access.php on toolsmandi.com.
// Verifies HMAC signature, checks product authorization, creates a 60s OTT.

func authHandshakeHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()

	// Only allow POST
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse JSON payload
	// NOTE: ProductIDs accept both int [5] and string ["5"] from PHP
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

	// Convert ProductIDsRaw (interface{}) → []int (handles PHP strings AND numbers)
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

	// Validate required fields
	if payload.Username == "" || payload.ClientIP == "" || payload.Signature == "" {
		http.Error(w, "Bad Request: Missing required fields", http.StatusBadRequest)
		return
	}

	// Fetch dynamic secret_key and session_duration for this website_id
	var dbSecretKey string
	var dbSessionDuration int
	err := db.QueryRow("SELECT secret_key, session_duration FROM ahrefs_websites WHERE id = ?", currentWebsiteID).Scan(&dbSecretKey, &dbSessionDuration)
	if err != nil {
		log.Printf("[HANDSHAKE] Failed to fetch secret_key from DB for website_id %d: %v (using config fallback)", currentWebsiteID, err)
		dbSecretKey = cfg.SecretKey
		dbSessionDuration = cfg.SessionDurationMinutes
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

	// Reject stale requests (> 5 minutes old)
	if time.Now().Unix()-payload.Timestamp > 300 {
		log.Printf("[HANDSHAKE] ❌ Stale request rejected for user: %s (timestamp diff: %ds)", payload.Username, time.Now().Unix()-payload.Timestamp)
		http.Error(w, "Forbidden: Request expired", http.StatusForbidden)
		return
	}

	// Check if DB is available
	if db == nil {
		http.Error(w, "Service Unavailable: Database not connected", http.StatusServiceUnavailable)
		return
	}

	// ── Product Authorization Check ──────────────────────────────────────────
	// If ahrefs_products table has rows for this website, at least one product_id must match.
	// If table is empty for this website → open access (admin hasn't restricted yet).
	var productCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM ahrefs_products WHERE website_id = ?", currentWebsiteID).Scan(&productCount)

	if productCount > 0 && len(productIDs) > 0 {
		hasAccess := false
		rows, err := db.Query("SELECT product_id FROM ahrefs_products WHERE website_id = ?", currentWebsiteID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var allowedProductIDsStr string
				if err := rows.Scan(&allowedProductIDsStr); err == nil {
					// Split by commas and check
					parts := strings.Split(allowedProductIDsStr, ",")
					for _, part := range parts {
						part = strings.TrimSpace(part)
						if part == "" {
							continue
						}
						if allowedPid, convErr := strconv.Atoi(part); convErr == nil {
							for _, userPid := range productIDs {
								if userPid == allowedPid {
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
				if hasAccess {
					break
				}
			}
		}
		if !hasAccess {
			log.Printf("[HANDSHAKE] ❌ Product authorization failed for user: %s (product_ids: %v)", payload.Username, productIDs)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintf(w, `{"error":"no_product_access","message":"You do not have an authorized plan to access Ahrefs."}`)
			return
		}
	} else if productCount > 0 && len(productIDs) == 0 {
		// Table has products but user has none
		log.Printf("[HANDSHAKE] ❌ User %s has no product_ids", payload.Username)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `{"error":"no_product_access","message":"Your account does not have an active plan."}`)
		return
	}

	// Automatically expire custom limits that have passed their expiration date
	_, _ = db.Exec(`
		UPDATE ahrefs_users u
		JOIN ahrefs_websites w ON u.website_id = w.id
		SET u.credit_limit = COALESCE(w.default_credit_limit, 50),
			u.export_limit = COALESCE(w.default_export_limit, 100000),
			u.custom_limit_expire_at = NULL
		WHERE u.custom_limit_expire_at IS NOT NULL AND u.custom_limit_expire_at < NOW()
	`)

	var dbStatus string
	err = db.QueryRow("SELECT status FROM ahrefs_users WHERE username = ? AND website_id = ?", payload.Username, currentWebsiteID).Scan(&dbStatus)
	if err == sql.ErrNoRows {
		var defCredits, defExports int
		err = db.QueryRow("SELECT COALESCE(default_credit_limit, 50), COALESCE(default_export_limit, 100000) FROM ahrefs_websites WHERE id = ?", currentWebsiteID).Scan(&defCredits, &defExports)
		if err != nil {
			defCredits = 50
			defExports = 100000
		}
		_, err = db.Exec("INSERT INTO ahrefs_users (username, website_id, credit_limit, export_limit) VALUES (?, ?, ?, ?)", payload.Username, currentWebsiteID, defCredits, defExports)
		if err != nil {
			log.Printf("[HANDSHAKE] DB auto-create user failed for '%s': %v", payload.Username, err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		log.Printf("[HANDSHAKE] Auto-created user profile: %s under website_id %d ✅", payload.Username, currentWebsiteID)
	} else if err != nil {
		log.Printf("[HANDSHAKE] DB lookup error for '%s': %v", payload.Username, err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	} else if dbStatus == "suspended" {
		log.Printf("[HANDSHAKE] ❌ Suspended user attempted login: %s", payload.Username)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `{"error":"account_suspended","message":"Your account is suspended. Please contact support for assistance."}`)
		return
	}

	// ── Generate One-Time Token (OTT) ────────────────────────────────────────
	ott, err := generateOTT()
	if err != nil {
		log.Printf("[HANDSHAKE] OTT generation failed: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
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

	// Clean up expired tokens periodically
	go func() { _, _ = db.Exec("DELETE FROM ahrefs_tokens WHERE expires_at < NOW()") }()

	redirectURL := fmt.Sprintf("%s://%s/access?user=%s&token=%s",
		cfg.PublicScheme, cfg.PublicHost,
		url.QueryEscape(payload.Username),
		url.QueryEscape(ott),
	)

	log.Printf("[HANDSHAKE] ✅ OTT generated for user: %s (IP: %s, expires: %v)", payload.Username, payload.ClientIP, expires)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","redirect_url":%q}`, redirectURL)
}

func userLimitsAPIHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	cookie, err := r.Cookie("ahrefs_session")
	if err != nil || cookie.Value == "" {
		cookie, err = r.Cookie("ct_session")
	}
	username := ""
	if err == nil {
		username = ahrefsSessionUser(cookie.Value)
	}
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"show_limit":false}`))
		return
	}
	credits, _, _ := ahrefsMeterView(username, "credits", 30, 1)
	exports, _, _ := ahrefsMeterView(username, "exports", 1000, 7)
	token := ""
	if cookie != nil {
		token = cookie.Value
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"show_limit":   ahrefsLimitVisible(username, token),
		"username":     username,
		"credit_limit": credits.limit,
		"credit_used":  credits.used,
		"export_limit": exports.limit,
		"export_used":  exports.used,
	})
}

func ahrefsLimitVisible(username, sessionToken string) bool {
	db, err := openAhrefsPanel()
	if err != nil || username == "" {
		return false
	}
	var websiteID int
	if err = db.QueryRow(`SELECT id FROM websites WHERE domain IN ('127.0.0.1:5291', ?) LIMIT 1`, loadConfig().PublicHost).Scan(&websiteID); err != nil || websiteID == 0 {
		return false
	}
	var mode string
	_ = db.QueryRow(`SELECT COALESCE(limit_visibility, '') FROM panel_users WHERE website_id=? AND username=?`, websiteID, username).Scan(&mode)
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "show":
		return true
	case "hide":
		return false
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var accountID int
	if sessionToken != "" {
		_ = db.QueryRow(`SELECT COALESCE(assigned_account_id, 0) FROM live_sessions WHERE session_token=? AND website_id=? AND expires_at>?`, sessionToken, websiteID, now).Scan(&accountID)
	}
	if accountID == 0 {
		_ = db.QueryRow(`SELECT COALESCE(assigned_account_id, 0) FROM live_sessions WHERE username=? AND website_id=? AND expires_at>? ORDER BY id DESC LIMIT 1`, username, websiteID, now).Scan(&accountID)
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

func safeAhrefsSwitchNext(raw string) string {
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.Contains(raw, "\\") || len(raw) > 2000 {
		return "/dashboard"
	}
	return raw
}

func ahrefsSwitchReturnPath(r *http.Request) string {
	path := r.URL.RequestURI()
	if r.URL.Path == "/user/login" || strings.HasPrefix(r.URL.Path, "/user/login/") || r.URL.Path == "/api/account-switch" {
		if ref := r.Referer(); ref != "" {
			if u, err := url.Parse(ref); err == nil && u.RequestURI() != "" {
				path = u.RequestURI()
			}
		}
	}
	return safeAhrefsSwitchNext(path)
}

func handleAhrefsAccountSwitch(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("ahrefs_session")
	if err != nil || cookie.Value == "" {
		cookie, err = r.Cookie("ct_session")
	}
	sessionToken := ""
	if err == nil {
		sessionToken = cookie.Value
	}
	cfg := loadConfig()
	current, ok := ahrefsPanelAccount(cfg.PublicHost, sessionToken)
	if !ok {
		renderAccountSwitchPage(w, "", ahrefsSwitchReturnPath(r))
		return
	}
	userName := ahrefsSessionUser(sessionToken)
	reason := strings.TrimSpace(r.URL.Query().Get("reason"))
	if reason == "" {
		reason = "suspicious activity detected"
	}
	next, switchErr := switchAhrefsPanelAccount(cfg.PublicHost, sessionToken, current, userName, reason)
	nextName := ""
	if switchErr != nil {
		log.Printf("[LB] still looking for another account: %v", switchErr)
	} else {
		nextName = next.Name
	}
	renderAccountSwitchPage(w, nextName, safeAhrefsSwitchNext(r.URL.Query().Get("next")))
}

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

	// Resolve current assigned account
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

	// Resolve switch reason
	reason := r.URL.Query().Get("reason")
	if reason == "" {
		reason = "client-side watchdog trigger"
	}

	// Get current username
	var currentUser string
	err = db.QueryRow("SELECT username FROM ahrefs_sessions WHERE session_token = ? AND website_id = ?", sessionToken, currentWebsiteID).Scan(&currentUser)
	if err != nil {
		currentUser = "unknown"
	}

	// Rotate session to the next active account
	nextAcc, switchErr := switchToNextAccount(sessionToken, activeAcc.ID, activeAcc.Name, currentUser, reason)
	if switchErr != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"error":"no_other_accounts","message":"%v"}`, switchErr)
		return
	}

	fmt.Fprintf(w, `{"status":"ok","switched_to":"%s","id":%d}`, nextAcc.Name, nextAcc.ID)
}

func renderAccessDeniedPage(w http.ResponseWriter) {
	writeLightCard(w, http.StatusForbidden, lightCard{
		Title:   "Access Denied",
		Heading: "Access Denied",
		Message: "You cannot open <span class=\"brand\">Ahrefs</span> directly. Open it again from your access link.",
		Footer:  "Your session ended or this browser is not authorized",
	})
}

func renderAccountSwitchPage(w http.ResponseWriter, accountName, reloadPath string) {
	if reloadPath == "" {
		reloadPath = "/dashboard"
	}
	jsURL := strings.ReplaceAll(strconv.Quote(reloadPath), "<", `\u003c`)
	message := "Account logged out. Switching to another account..."
	if accountName != "" {
		message = "Account logged out. Switching to " + html.EscapeString(accountName) + "..."
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	page := `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Switching account</title>
<style>
* { box-sizing:border-box;margin:0;padding:0; }
body { min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;background:#eef3f8;color:#0f172a;font-family:system-ui,-apple-system,Segoe UI,sans-serif; }
.card { width:min(440px,100%);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px 36px;text-align:center; }
.ring { width:78px;height:78px;margin:0 auto 22px;border-radius:50%;background:conic-gradient(#3b82f6 0 70deg,#e7eef8 70deg 360deg);display:grid;place-items:center;animation:spin .9s linear infinite; }
.lock { width:64px;height:64px;border-radius:50%;background:#fff;display:grid;place-items:center;font-size:26px;animation:spin .9s linear infinite reverse; }
h1 { font-size:28px;line-height:1.2;font-weight:800;letter-spacing:-.03em;margin-bottom:12px; }
.msg { color:#64748b;font-size:15px;line-height:1.55; }
.pill { margin:22px auto 0;display:inline-flex;align-items:center;gap:8px;padding:8px 14px;border:1px solid #e6ebf2;border-radius:999px;color:#334155;font-size:14px;background:#fff; }
.dot { width:14px;height:14px;border-radius:50%;border:2px solid #dbe4f0;border-top-color:#3b82f6;animation:spin .8s linear infinite; }
.foot { margin-top:18px;color:#94a3b8;font-size:13px; }
@keyframes spin { to { transform:rotate(360deg); } }
</style>
</head>
<body>
<div class="card">
<div class="ring"><div class="lock">&#8635;</div></div>
<h1>Switching account</h1>
<p class="msg">` + message + `</p>
<div class="pill"><span class="dot"></span>Trying one account at a time</div>
<p class="foot">This keeps trying until an account is available</p>
</div>
<script>
setTimeout(function () { window.location.replace(` + jsURL + `); }, 1200);
</script>
</body>
</html>`
	fmt.Fprint(w, page)
}

func renderNoActiveAccountsPage(w http.ResponseWriter) {
	writeLightCard(w, http.StatusServiceUnavailable, lightCard{
		Title:   "Temporarily Unavailable",
		Heading: "Temporarily Unavailable",
		Message: "All mapped <span class=\"brand\">Ahrefs</span> accounts are currently undergoing maintenance. Please try again in a few minutes.",
		Footer:  "No active account is available right now",
	})
}

func renderLimitReachedPage(w http.ResponseWriter, limitType string) {
	title := "Daily Credit Limit Reached"
	msg := "You have used up your <span class=\"brand\">Ahrefs</span> daily credit limit for today. Your limit resets at midnight (12:00 AM IST)."
	if limitType == "export" {
		title = "Weekly Export Rows Limit Reached"
		msg = "You have reached your weekly CSV export row limit for <span class=\"brand\">Ahrefs</span>."
	}
	writeLightCard(w, http.StatusForbidden, lightCard{
		Title:   title,
		Heading: title,
		Message: msg,
		Footer:  "Open this tool again from your access link later",
	})
}

// ── PROXY SYSTEM ─────────────────────────────────────────────────────────────

//
// Hot-Reloadable: proxy.txt file change hote hi next request se proxy change
// ho jaata hai — server restart ya rebuild ki zaroorat nahi.

const PROXY_FILE = "proxy.txt"

var (
	currentProxy *url.URL
	proxyModTime time.Time
)

// loadProxy reads proxy.txt and hot-reloads only when file has changed.
// Returns nil if no proxy is configured or file doesn't exist.
func loadProxy() *url.URL {
	info, err := os.Stat(PROXY_FILE)
	if err != nil {
		// File doesn't exist — no proxy
		if currentProxy != nil {
			log.Printf("[PROXY] proxy.txt not found, proxy disabled")
			currentProxy = nil
		}
		return nil
	}

	// Return cached value if file hasn't changed
	if !info.ModTime().After(proxyModTime) {
		return currentProxy
	}

	data, err := os.ReadFile(PROXY_FILE)
	if err != nil {
		log.Printf("[PROXY] Read error: %v", err)
		return currentProxy
	}

	// Find first non-comment, non-empty line
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
		if currentProxy != nil {
			log.Printf("[PROXY] ⚠️  Proxy disabled (no active line in proxy.txt)")
		}
		currentProxy = nil
		return nil
	}

	parsed, err := url.Parse(proxyStr)
	if err != nil {
		log.Printf("[PROXY] Parse error '%s': %v", proxyStr, err)
		return currentProxy
	}

	// Validate scheme
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "socks5" && scheme != "socks5h" && scheme != "http" && scheme != "https" {
		log.Printf("[PROXY] Unsupported scheme '%s'. Use: socks5, socks5h, http, https", parsed.Scheme)
		return currentProxy
	}

	// Log only if changed
	newStr := parsed.String()
	oldStr := ""
	if currentProxy != nil {
		oldStr = currentProxy.String()
	}
	if newStr != oldStr {
		user := ""
		if parsed.User != nil {
			user = parsed.User.Username() + ":****@"
		}
		log.Printf("[PROXY] ✅ Updated → %s://%s%s", parsed.Scheme, user, parsed.Host)
	}

	currentProxy = parsed
	return currentProxy
}

// getProxy returns current proxy URL (with hot-reload check)
func getProxy() *url.URL {
	return loadProxy()
}

// ── PROXY DIALERS ─────────────────────────────────────────────────────────────

// dialThroughProxy routes a TCP connection through the configured proxy.
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

// dialThroughSocks5 connects to targetAddr via a SOCKS5 proxy.
func dialThroughSocks5(ctx context.Context, targetAddr string, proxyURL *url.URL) (net.Conn, error) {
	var auth *proxy.Auth
	if proxyURL.User != nil {
		pass, _ := proxyURL.User.Password()
		auth = &proxy.Auth{
			User:     proxyURL.User.Username(),
			Password: pass,
		}
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

	// Use DialContext if supported (newer golang.org/x/net)
	if cd, ok := socks5Dialer.(proxy.ContextDialer); ok {
		return cd.DialContext(ctx, "tcp", targetAddr)
	}
	return socks5Dialer.Dial("tcp", targetAddr)
}

// dialThroughHTTPProxy connects to targetAddr via HTTP/HTTPS CONNECT tunnel.
func dialThroughHTTPProxy(ctx context.Context, targetAddr string, proxyURL *url.URL) (net.Conn, error) {
	proxyHost := proxyURL.Host
	if !strings.Contains(proxyHost, ":") {
		if proxyURL.Scheme == "https" {
			proxyHost += ":443"
		} else {
			proxyHost += ":80"
		}
	}

	// Connect to the proxy server
	conn, err := (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, "tcp", proxyHost)
	if err != nil {
		return nil, fmt.Errorf("connect to HTTP proxy %s: %w", proxyHost, err)
	}

	// Build HTTP CONNECT request
	connectLine := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", targetAddr, targetAddr)
	if proxyURL.User != nil {
		pass, _ := proxyURL.User.Password()
		creds := base64.StdEncoding.EncodeToString([]byte(proxyURL.User.Username() + ":" + pass))
		connectLine += "Proxy-Authorization: Basic " + creds + "\r\n"
	}
	connectLine += "\r\n"

	if _, err := conn.Write([]byte(connectLine)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("send CONNECT: %w", err)
	}

	// Read proxy response
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read CONNECT response: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("HTTP proxy CONNECT refused: %s", resp.Status)
	}

	return conn, nil
}

// ── CHROME TLS FINGERPRINT TRANSPORT ─────────────────────────────────────────
//
// Cloudflare detects Go's standard TLS fingerprint and blocks it with a
// "CF-Mitigated: challenge" 403. This custom transport replaces the TLS
// handshake with Chrome 120's exact cipher suites, extensions, and GREASE
// values — making our requests indistinguishable from a real browser.

// uTLSConn wraps a uTLS connection so we can inspect negotiated protocol
type uTLSConn struct {
	*utls.UConn
}

func (c *uTLSConn) ConnectionState() tls.ConnectionState {
	cs := c.UConn.ConnectionState()
	return tls.ConnectionState{
		Version:            cs.Version,
		HandshakeComplete:  cs.HandshakeComplete,
		DidResume:          cs.DidResume,
		CipherSuite:        cs.CipherSuite,
		NegotiatedProtocol: cs.NegotiatedProtocol,
		ServerName:         cs.ServerName,
	}
}

// dialChrome performs a TCP dial (optionally through proxy) and wraps
// the connection with Chrome 120's uTLS fingerprint.
// If proxy is set but fails, it automatically falls back to direct connection.
type contextKey string

const proxyContextKey contextKey = "account_proxy"

func dialChrome(ctx context.Context, addr string) (*uTLSConn, error) {
	host, _, _ := net.SplitHostPort(addr)

	var tcpConn net.Conn
	var err error

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
		tcpConn, err = dialThroughProxy(ctx, addr, px)
		if err != nil {
			return nil, fmt.Errorf("proxy dial %s: %w", px.Host, err)
		}
	} else {
		// No proxy — direct connection
		tcpConn, err = (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("TCP dial: %w", err)
		}
	}

	uConn := utls.UClient(tcpConn, &utls.Config{
		ServerName:         host,
		InsecureSkipVerify: false,
	}, utls.HelloChrome_120)

	if err := uConn.HandshakeContext(ctx); err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("uTLS handshake: %w", err)
	}

	return &uTLSConn{uConn}, nil
}

// roundTripper is a custom RoundTripper that:
//  1. Dials with Chrome 120 uTLS fingerprint
//  2. Detects ALPN ("h2" or "http/1.1")
//  3. Routes to http2.Transport or http.Transport accordingly
type roundTripper struct {
	h2    *http2.Transport
	h1    *http.Transport
	mu    sync.Mutex
	pools map[string]*h2Pool
}

type h2Pool struct {
	conns   []*http2.ClientConn
	dialing int
}

const maxAhrefsH2Conns = 6

func (rt *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	addr := req.URL.Host
	if !strings.Contains(addr, ":") {
		if req.URL.Scheme == "https" {
			addr += ":443"
		} else {
			addr += ":80"
		}
	}

	proxyKey := ""
	if raw, ok := req.Context().Value(proxyContextKey).(string); ok {
		proxyKey = raw
	}
	cacheKey := addr + "\n" + proxyKey
	rt.mu.Lock()
	pool := rt.pools[cacheKey]
	if pool == nil {
		pool = &h2Pool{}
		rt.pools[cacheKey] = pool
	}
	for _, cc := range pool.conns {
		if cc != nil && cc.CanTakeNewRequest() {
			rt.mu.Unlock()
			return cc.RoundTrip(req)
		}
	}
	if len(pool.conns)+pool.dialing >= maxAhrefsH2Conns && len(pool.conns) > 0 {
		cc := pool.conns[len(pool.conns)-1]
		rt.mu.Unlock()
		return cc.RoundTrip(req)
	}
	pool.dialing++
	rt.mu.Unlock()

	conn, err := dialChrome(req.Context(), addr)
	if err != nil {
		rt.mu.Lock()
		pool.dialing--
		rt.mu.Unlock()
		return nil, err
	}

	proto := conn.ConnectionState().NegotiatedProtocol
	log.Printf("[TLS] %s → ALPN=%q via proxy", req.URL.Host, proto)

	if proto == "h2" {
		cc, err := rt.h2.NewClientConn(conn)
		if err != nil {
			conn.Close()
			rt.mu.Lock()
			pool.dialing--
			rt.mu.Unlock()
			return nil, err
		}
		rt.mu.Lock()
		pool.dialing--
		pool.conns = append(pool.conns, cc)
		rt.mu.Unlock()
		return cc.RoundTrip(req)
	}

	rt.mu.Lock()
	pool.dialing--
	rt.mu.Unlock()
	conn.Close()
	return rt.h1.RoundTrip(req)
}

// buildChromeHTTPClient creates an http.Client that:
//   - Uses Chrome 120's exact TLS fingerprint (via uTLS) to bypass CF Bot Management
//   - Supports both HTTP/2 and HTTP/1.1 based on ALPN negotiation
func buildChromeHTTPClient() *http.Client {
	// Shared dial function used by both transports
	dialTLS := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialChrome(ctx, addr)
	}

	h1Transport := &http.Transport{
		DialTLSContext:        dialTLS,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   20 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    false,
		ForceAttemptHTTP2:     false, // We handle h2 manually
	}

	h2Transport := &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			return dialChrome(ctx, addr)
		},
		DisableCompression: false,
	}

	return &http.Client{
		Transport: &roundTripper{h2: h2Transport, h1: h1Transport, pools: map[string]*h2Pool{}},
		Timeout:   60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// Global HTTP client — Chrome TLS fingerprint
var httpClient = buildChromeHTTPClient()

// ── COOKIE PARSING ────────────────────────────────────────────────────────────

type BrowserCookie struct {
	Name           string      `json:"name"`
	Value          string      `json:"value"`
	Domain         string      `json:"domain"`
	Path           string      `json:"path"`
	ExpirationDate interface{} `json:"expirationDate"`
	HTTPOnly       bool        `json:"httpOnly"`
	Secure         bool        `json:"secure"`
	Session        bool        `json:"session"`
	SameSite       interface{} `json:"sameSite"`
}

func accountFromCookieFile(cfg Config) (AhrefsAccount, bool) {
	path := cfg.CookieFile
	if path == "" {
		path = "cookie.txt"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return AhrefsAccount{}, false
	}
	raw := strings.TrimSpace(string(data))
	if raw == "" || parseCookieFromDB(raw) == "" {
		return AhrefsAccount{}, false
	}
	ua := cfg.UserAgent
	if ua == "" {
		ua = defaultConfig.UserAgent
	}
	return AhrefsAccount{Name: "cookie.txt", Cookie: raw, UserAgent: ua}, true
}

// parseCookies reads cookie.txt — supports JSON array, GoAuto wrap, and Netscape format
func parseCookies(filePath string) string {
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("[COOKIE] Cannot read %s: %v", filePath, err)
		return ""
	}

	trimmed := strings.TrimSpace(string(data))
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

	// Try JSON array
	var cookies []BrowserCookie
	if strings.HasPrefix(trimmed, "[") && json.Unmarshal([]byte(trimmed), &cookies) == nil && len(cookies) > 0 {
		var parts []string
		for _, c := range cookies {
			if c.Name != "" {
				parts = append(parts, c.Name+"="+c.Value)
			}
		}
		log.Printf("[COOKIE] Loaded %d cookies from JSON", len(parts))
		return strings.Join(parts, "; ")
	}

	// Fallback: Netscape format
	var parts []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) >= 7 && cols[5] != "" {
			parts = append(parts, cols[5]+"="+strings.TrimSpace(cols[6]))
		}
	}
	log.Printf("[COOKIE] Loaded %d cookies from Netscape format", len(parts))
	return strings.Join(parts, "; ")
}

var cachedCookieStr string

// ── CF TURNSTILE MOCK SCRIPT ──────────────────────────────────────────────────

func cfBypassScript() string {
	t := MOCK_TOKEN
	return fmt.Sprintf(`(function() {
    'use strict';
    function fill(token) {
        document.querySelectorAll('input[name="cf-turnstile-response"]').forEach(function(i) {
            i.value = token;
            i.dispatchEvent(new Event('input', { bubbles: true }));
            i.dispatchEvent(new Event('change', { bubbles: true }));
        });
        var e = document.getElementById('login-error');
        if (e) e.style.display = 'none';
    }
    window.turnstile = {
        render: function(el, p) {
            var c = typeof el === 'string' ? document.querySelector(el) : el;
            if (c) {
                var i = c.querySelector('input[name="cf-turnstile-response"]') || document.createElement('input');
                i.type='hidden'; i.name='cf-turnstile-response'; i.value='%s'; c.appendChild(i);
            }
            setTimeout(function() { fill('%s'); if(p && p.callback) p.callback('%s'); }, 100);
            return 'mock';
        },
        reset:       function() { setTimeout(function() { fill('%s'); }, 100); },
        remove:      function() {},
        getResponse: function() { return '%s'; },
        isExpired:   function() { return false; },
        execute:     function(el, p) { setTimeout(function() { fill('%s'); if(p&&p.callback) p.callback('%s'); }, 100); }
    };
    setTimeout(function() { fill('%s'); }, 400);
    document.addEventListener('DOMContentLoaded', function() { setTimeout(function() { fill('%s'); }, 200); });
})();`, t, t, t, t, t, t, t, t, t)
}

// ── CLIENT-SIDE PATCHER SCRIPT ────────────────────────────────────────────────
func patcherScript() string {
	return `<script>
(function() {
    var T = 'app.ahrefs.com', C = 'static-app.ahrefs.com', O = window.location.origin;
    function keepLocal(u) {
        if (typeof u !== 'string') return u;
        return u
            .replace('https://static-app.ahrefs.com', O + '/cdn')
            .replace('http://static-app.ahrefs.com', O + '/cdn')
            .replace('https://app.ahrefs.com', O)
            .replace('http://app.ahrefs.com', O)
            .replace('https://www.ahrefs.com', O)
            .replace('http://www.ahrefs.com', O)
            .replace('https://ahrefs.com', O)
            .replace('http://ahrefs.com', O);
    }
    try {
        var locAssign = Location.prototype.assign;
        var locReplace = Location.prototype.replace;
        Location.prototype.assign = function(u) { return locAssign.call(this, keepLocal(String(u))); };
        Location.prototype.replace = function(u) { return locReplace.call(this, keepLocal(String(u))); };
    } catch (err) {}

    // ── Block "Leave site?" beforeunload prompt ─────────────────────────────────
    window.addEventListener('beforeunload', function(e) {
        e.stopImmediatePropagation();
    }, true);
    Object.defineProperty(window, 'onbeforeunload', {
        get: function() { return null; },
        set: function() { /* do nothing */ }
    });

    var suspiciousSwitched = false;
    function watchSuspicious() {
        if (suspiciousSwitched || !document.body) return;
        var txt = (document.body.innerText || '').toLowerCase();
        if (txt.indexOf('suspicious activity') === -1 && txt.indexOf('suspicious_activity') === -1 && txt.indexOf('temporarily restricted') === -1 && txt.indexOf('potential violations') === -1) return;
        suspiciousSwitched = true;
        var next = window.location.pathname + window.location.search;
        window.location.replace('/api/account-switch?reason=' + encodeURIComponent('suspicious activity detected') + '&next=' + encodeURIComponent(next));
    }
    setTimeout(watchSuspicious, 800);
    setInterval(watchSuspicious, 2000);

    // ── 1. Link href rewriter (runs periodically for React re-renders) ──────────
    function rl() {
        document.querySelectorAll('a[href*="'+T+'"]').forEach(function(a) {
            var h = a.href.replace('https://'+T, O).replace('http://'+T, O);
            if (a.href !== h) a.href = h;
        });
    }
    rl();
    document.addEventListener('DOMContentLoaded', rl);
    window.addEventListener('load', rl);
    setInterval(rl, 100); // 100ms — catches React re-renders faster

    // ── 2. Click interceptor (capture phase = fires before React handlers) ──────
    document.addEventListener('click', function(e) {
        var a = e.target.closest('a');
        if (!a) return;

        var href = a.getAttribute('href') || a.href || '';
        if (!href || href.startsWith('#') || href.startsWith('javascript:')) return;

        // Open in new tab normally if target="_blank"
        if (a.target === '_blank') return;

        // Normalize URL to handle absolute app.ahrefs.com URLs
        var targetUrl = href
            .replace('https://app.ahrefs.com', O)
            .replace('http://app.ahrefs.com', O)
            .replace('https://www.ahrefs.com', O)
            .replace('http://www.ahrefs.com', O)
            .replace('https://ahrefs.com', O)
            .replace('http://ahrefs.com', O);

        // Parse absolute URL
        var parsed;
        try {
            parsed = new URL(targetUrl, O);
        } catch(err) {
            return;
        }
        var inTool = /^\/(site-audit|site-explorer|keywords-explorer|content-explorer|content-helper|competitive-analysis|rank-tracker|web-analytics|brand-radar)(\/|$)/.test(window.location.pathname);
        if (inTool && parsed.pathname === '/') {
            e.preventDefault();
            return;
        }

        // Intercept if same-origin navigation
        if (parsed.origin !== O) return;
        if (parsed.href !== new URL(href, O).href) {
            e.preventDefault();
            e.stopImmediatePropagation();
            window.location.href = parsed.href;
            return;
        }
        var curFirst = window.location.pathname.split('/')[1] || '';
        var newFirst = parsed.pathname.split('/')[1] || '';
        if (curFirst !== newFirst) {
            e.preventDefault();
            e.stopImmediatePropagation();
            window.location.href = parsed.href;
            return;
        }
    }, true); // true = capture phase, fires BEFORE React's synthetic events

    // ── 3. History API patch (React Router uses pushState/replaceState for navigation) ────────
    var _push    = history.pushState.bind(history);
    var _replace = history.replaceState.bind(history);

    history.pushState = function(state, title, url) {
        if (typeof url === 'string') {
            url = url
                .replace('https://app.ahrefs.com', O)
                .replace('http://app.ahrefs.com', O)
                .replace('https://www.ahrefs.com', O)
                .replace('http://www.ahrefs.com', O)
                .replace('https://ahrefs.com', O)
                .replace('http://ahrefs.com', O);
            // Block restricted paths client-side
            var pathToCheck = url;
            try {
                var parsedUrl = new URL(url, O);
                pathToCheck = parsedUrl.pathname;
            } catch(e) {
                try {
                    var parsedUrl = new URL(url, window.location.origin);
                    pathToCheck = parsedUrl.pathname;
                } catch(err) {}
            }
            if (typeof pathToCheck === 'string') {
                var cleanPath = pathToCheck.trim().replace(/\/$/, '');
                var blockedList = [
                    "/account/my-account",
                    "/account/security",
                    "/account/my-notifications",
                    "/account/my-certificates",
                    "/account/settings",
                    "/account/saml-sso",
                    "/account/members/confirmed",
                    "/account/members/pending",
                    "/account/tools-and-permissions",
                    "/account/limits-and-usage/web",
                    "/account/billing/subscriptions",
                    "/account/api-keys",
                    "/account/integrations",
                    "/account/applications",
                    "/account/agency-profile/0",
                    "/account/audit-log",
                    "/account/api-log",
                    "/account/academy",
                    "/academy"
                ];
                for (var i = 0; i < blockedList.length; i++) {
                    if (cleanPath === blockedList[i] || cleanPath.startsWith(blockedList[i] + '/')) {
                        window.location.href = O + '/site-explorer';
                        return;
                    }
                }
            }

            if (url.includes(T)) {
                url = url.replace('https://'+T, O).replace('http://'+T, O);
            }
            var parsed;
            try { parsed = new URL(url, O); } catch(e) {}
            if (parsed && parsed.origin === O) {
                var inTool = /^\/(site-audit|site-explorer|keywords-explorer|content-explorer|content-helper|competitive-analysis|rank-tracker|web-analytics|brand-radar)(\/|$)/.test(window.location.pathname);
                if (inTool && parsed.pathname === '/') {
                    return _push(state, title, window.location.href);
                }
                var curFirst = window.location.pathname.split('/')[1] || '';
                var newFirst = parsed.pathname.split('/')[1] || '';
                if (curFirst !== newFirst) {
                    window.location.href = parsed.href;
                    return;
                }
            }
        }
        return _push(state, title, url);
    };

    history.replaceState = function(state, title, url) {
        if (typeof url === 'string') {
            url = url
                .replace('https://app.ahrefs.com', O)
                .replace('http://app.ahrefs.com', O)
                .replace('https://www.ahrefs.com', O)
                .replace('http://www.ahrefs.com', O)
                .replace('https://ahrefs.com', O)
                .replace('http://ahrefs.com', O);
            // Block restricted paths client-side
            var pathToCheck = url;
            try {
                var parsedUrl = new URL(url, O);
                pathToCheck = parsedUrl.pathname;
            } catch(e) {
                try {
                    var parsedUrl = new URL(url, window.location.origin);
                    pathToCheck = parsedUrl.pathname;
                } catch(err) {}
            }
            if (typeof pathToCheck === 'string') {
                var cleanPath = pathToCheck.trim().replace(/\/$/, '');
                var blockedList = [
                    "/account/my-account",
                    "/account/security",
                    "/account/my-notifications",
                    "/account/my-certificates",
                    "/account/settings",
                    "/account/saml-sso",
                    "/account/members/confirmed",
                    "/account/members/pending",
                    "/account/tools-and-permissions",
                    "/account/limits-and-usage/web",
                    "/account/billing/subscriptions",
                    "/account/api-keys",
                    "/account/integrations",
                    "/account/applications",
                    "/account/agency-profile/0",
                    "/account/audit-log",
                    "/account/api-log",
                    "/account/academy",
                    "/academy"
                ];
                for (var i = 0; i < blockedList.length; i++) {
                    if (cleanPath === blockedList[i] || cleanPath.startsWith(blockedList[i] + '/')) {
                        window.location.href = O + '/site-explorer';
                        return;
                    }
                }
            }

            if (url.includes(T)) {
                url = url.replace('https://'+T, O).replace('http://'+T, O);
            }
            var parsed;
            try { parsed = new URL(url, O); } catch(e) {}
            if (parsed && parsed.origin === O) {
                var inTool = /^\/(site-audit|site-explorer|keywords-explorer|content-explorer|content-helper|competitive-analysis|rank-tracker|web-analytics|brand-radar)(\/|$)/.test(window.location.pathname);
                if (inTool && parsed.pathname === '/') {
                    return _replace(state, title, window.location.href);
                }
                var curFirst = window.location.pathname.split('/')[1] || '';
                var newFirst = parsed.pathname.split('/')[1] || '';
                if (curFirst !== newFirst) {
                    window.location.href = parsed.href;
                    return;
                }
            }
        }
        return _replace(state, title, url);
    };

    // ── 4. window.open override ──────────────────────────────────────────────────
    var _open = window.open;
    window.open = function(url) {
        if (typeof url === 'string' && url.includes(T)) {
            url = url.replace('https://'+T, O);
        }
        return _open.apply(this, arguments);
    };

    // ── 5. XHR patch (API calls) ─────────────────────────────────────────────────
    var xo = XMLHttpRequest.prototype.open;
    XMLHttpRequest.prototype.open = function(m, u) {
        if (typeof u==='string') { u=u.replace('https://'+T,O).replace('https://'+C,O+'/cdn'); }
        return xo.apply(this, arguments);
    };

    // ── 6. Fetch patch (API calls) ───────────────────────────────────────────────
    var fo = window.fetch;
    window.fetch = function(inp, init) {
        if (typeof inp==='string') { inp=inp.replace('https://'+T,O).replace('https://'+C,O+'/cdn'); }
        return fo(inp, init);
    };

    // ── 7. Export Interceptor & Global Limits Error Handler ──────────────────────
    function showLimitPopup(title, msg) {
        if (document.getElementById('tm-limit-popup')) return;
        var overlay = document.createElement('div');
        overlay.id = 'tm-limit-popup';
        overlay.style.cssText = 'position:fixed;inset:0;z-index:2147483647;display:flex;align-items:center;justify-content:center;background:#eef3f8;font-family:system-ui,sans-serif;';
        var card = document.createElement('div');
        card.style.cssText = 'width:min(440px,calc(100% - 32px));background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px;text-align:center;box-sizing:border-box;';
        var h = document.createElement('h1');
        h.textContent = title || 'Limit Reached';
        h.style.cssText = 'font-size:28px;font-weight:700;color:#0f172a;margin:0 0 12px;letter-spacing:-.03em;';
        var p = document.createElement('p');
        p.textContent = msg || 'Limit reached.';
        p.style.cssText = 'color:#64748b;line-height:1.5;margin:0 0 24px;font-size:15px;';
        var btn = document.createElement('button');
        btn.textContent = 'Close';
        btn.style.cssText = 'border:0;background:#0f172a;color:#fff;border-radius:12px;padding:12px 22px;font:600 15px system-ui,sans-serif;cursor:pointer;';
        btn.onclick = function() { overlay.remove(); };
        card.appendChild(h);
        card.appendChild(p);
        card.appendChild(btn);
        overlay.appendChild(card);
        document.body.appendChild(overlay);
    }
    function limitPopupFrom(data) {
        if (!data) return;
        var title = 'Limit Reached';
        if (data.error === 'weekly_export_limit_reached') title = 'Export Limit Reached';
        if (data.error === 'daily_credit_limit_reached') title = 'Daily Limit Reached';
        showLimitPopup(title, data.message || 'Limit reached.');
    }

    function dl(text, ru, body) {
        // Auto-close the Ahrefs export modal drawer after success.
        setTimeout(function() {
            var c = document.querySelector('button[aria-label="Close"],.modal-close,[class*="CloseButton"]');
            if (c) { c.click(); return; }
            document.body.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true}));
        }, 1500);
    }

    var xs = XMLHttpRequest.prototype.send;
    XMLHttpRequest.prototype.send = function(b) {
        if (this._ex) this._exb = b;
        return xs.apply(this, arguments);
    };

    var xo2 = XMLHttpRequest.prototype.open;
    XMLHttpRequest.prototype.open = function(m, u) {
        var self = this;
        this.addEventListener('readystatechange', function() {
            if (self.readyState === 4) {
                if (self.status === 403) {
                    try {
                        var data = JSON.parse(self.responseText);
                        if (data && (data.error === 'weekly_export_limit_reached' || data.error === 'daily_credit_limit_reached')) {
                            limitPopupFrom(data);
                        }
                    } catch(e) {}
                }
            }
        });

        if (typeof u === 'string' && (u.includes('Export') || /Export/i.test(u))) {
            this._ex = true;
            this.addEventListener('load', function() {
                if (self.status === 200) dl(self.responseText, u, self._exb);
            });
        }
        return xo2.apply(this, arguments);
    };

    var fo2 = window.fetch;
    window.fetch = function(inp, init) {
        return fo2(inp, init).then(function(r) {
            if (r.status === 403) {
                r.clone().json().then(function(data) {
                    if (data && (data.error === 'weekly_export_limit_reached' || data.error === 'daily_credit_limit_reached')) {
                        limitPopupFrom(data);
                    }
                }).catch(function(){});
            }
            if (typeof inp === 'string' && (inp.includes('Export') || /Export/i.test(inp))) {
                if (r.ok) {
                    r.clone().text().then(function(t) { dl(t, inp, (init && init.body) || ''); });
                }
            }
            return r;
        });
    };

    // ── 8. Quotas/Limits Check Slider Widget Drawer (Admin Panel Switchable) ──
    function initQuotasWidget() {
        var open = false;
        function removeDock() {
            var old = document.getElementById('tm-limit-dock');
            if (old) old.remove();
        }
        function host() {
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
                + '.meter + .meter{margin-top:8px}'
                + '.meter-top{display:flex;justify-content:space-between;align-items:center;margin-bottom:8px;color:#166534;font-size:13px}'
                + '.count{font-weight:750}'
                + '.bar{height:6px;border-radius:999px;background:#e7f6ec;overflow:hidden}'
                + '.barfill{height:100%;width:0;border-radius:999px;background:#22c55e;transition:width .35s ease}'
                + '.barfill.warn{background:#f59e0b}.barfill.low{background:#ef4444}'
                + '<' + '/style>'
                + '<button class="tab" id="tm-tab" type="button" aria-label="Limits"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="8" fill="none" stroke="currentColor" stroke-width="2" opacity=".35"><' + '/circle><path d="M12 4a8 8 0 0 1 8 8" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round"><' + '/path><' + '/svg><' + '/button>'
                + '<div class="back" id="tm-back"><' + '/div>'
                + '<aside class="card" id="tm-drawer">'
                + '<div class="head"><div class="title">Credits<' + '/div><button class="x" id="tm-close" type="button" aria-label="Close">×<' + '/button><' + '/div>'
                + '<div class="ringwrap"><svg class="ring" viewBox="0 0 120 120"><circle class="track" cx="60" cy="60" r="46"><' + '/circle><circle class="fill" id="tm-ring" cx="60" cy="60" r="46"><' + '/circle><' + '/svg>'
                + '<div class="center"><div class="num" id="tm-left">0<' + '/div><div class="sub" id="tm-sub">left<' + '/div><' + '/div><' + '/div>'
                + '<div class="meter"><div class="meter-top"><span>Credits<' + '/span><span class="count" id="tm-count">0 / 0<' + '/span><' + '/div><div class="bar"><div class="barfill" id="tm-bar"><' + '/div><' + '/div><' + '/div>'
                + '<div class="meter"><div class="meter-top"><span>Exports<' + '/span><span class="count" id="tm-export">0 / 0<' + '/span><' + '/div><div class="bar"><div class="barfill" id="tm-export-bar"><' + '/div><' + '/div><' + '/div>'
                + '<' + '/aside>';
            (document.body || document.documentElement).appendChild(el);
            root.getElementById('tm-tab').addEventListener('click', function(ev){ ev.stopPropagation(); setOpen(true); });
            root.getElementById('tm-close').addEventListener('click', function(ev){ ev.stopPropagation(); setOpen(false); });
            root.getElementById('tm-back').addEventListener('click', function(){ setOpen(false); });
            root.getElementById('tm-drawer').addEventListener('click', function(ev){ ev.stopPropagation(); });
            return el;
        }
        function setOpen(next) {
            open = next;
            var el = host();
            if (!el) return;
            var shadow = el.shadowRoot;
            shadow.getElementById('tm-drawer').classList.toggle('show', open);
            shadow.getElementById('tm-back').classList.toggle('show', open);
            shadow.getElementById('tm-tab').classList.toggle('hide', open);
        }
        function paint(d) {
            if (!d || !d.show_limit) { removeDock(); return; }
            var el = host();
            if (!el) return;
            var shadow = el.shadowRoot;
            var tab = shadow.getElementById('tm-tab');
            var ring = shadow.getElementById('tm-ring');
            var bar = shadow.getElementById('tm-bar');
            var used = Number(d.credit_used) || 0;
            var limit = Number(d.credit_limit);
            var unlimited = !(limit >= 0);
            var left = unlimited ? used : Math.max(0, limit - used);
            var ratio = unlimited ? 1 : (limit === 0 ? 0 : left / limit);
            var expUsed = Number(d.export_used) || 0;
            var expLimit = Number(d.export_limit);
            var expUnlimited = !(expLimit >= 0);
            var expLeft = expUnlimited ? expUsed : Math.max(0, expLimit - expUsed);
            var expRatio = expUnlimited ? 1 : (expLimit === 0 ? 0 : expLeft / expLimit);
            var expOver = !expUnlimited && expLimit > 0 && expUsed >= expLimit;
            shadow.getElementById('tm-left').textContent = unlimited ? '∞' : String(left);
            shadow.getElementById('tm-sub').textContent = unlimited ? 'unlimited' : 'left';
            shadow.getElementById('tm-count').textContent = unlimited ? (used + ' used') : (used + ' / ' + limit);
            shadow.getElementById('tm-export').textContent = expUnlimited ? (expUsed + ' used') : (expUsed + ' / ' + expLimit);
            ring.style.strokeDasharray = '289';
            ring.style.strokeDashoffset = String(289 * (1 - ratio));
            bar.style.width = Math.round(ratio * 100) + '%';
            var expBar = shadow.getElementById('tm-export-bar');
            expBar.style.width = (expOver ? 100 : Math.round(expRatio * 100)) + '%';
            expBar.classList.toggle('low', expOver);
            var low = !unlimited && left < 3;
            var warn = !unlimited && left >= 3 && left < 10;
            tab.classList.toggle('low', low);
            tab.classList.toggle('warn', warn);
            ring.classList.toggle('low', low);
            ring.classList.toggle('warn', warn);
            bar.classList.toggle('low', low);
            bar.classList.toggle('warn', warn);
        }
        function updateBadge() {
            fetch('/api/user-limits', { credentials: 'same-origin', cache: 'no-store' })
                .then(function(r){ return r.json(); })
                .then(paint)
                .catch(function(){});
        }
        updateBadge();
        if (!window.__tmLimitTimer) {
            window.__tmLimitTimer = setInterval(updateBadge, 4000);
        }
    }

    function checkBlockedURL() {
        var p = window.location.pathname;
        if (typeof p === 'string') {
            p = p.trim().replace(/\/$/, '');
            var blockedList = [
                "/account/my-account",
                "/account/security",
                "/account/my-notifications",
                "/account/my-certificates",
                "/account/settings",
                "/account/saml-sso",
                "/account/members/confirmed",
                "/account/members/pending",
                "/account/tools-and-permissions",
                "/account/limits-and-usage/web",
                "/account/billing/subscriptions",
                "/account/api-keys",
                "/account/integrations",
                "/account/applications",
                "/account/agency-profile/0",
                "/account/audit-log",
                "/account/api-log",
                "/account/academy",
                "/academy"
            ];
            for (var i = 0; i < blockedList.length; i++) {
                if (p === blockedList[i] || p.startsWith(blockedList[i] + '/')) {
                    window.location.href = O + '/site-explorer';
                    break;
                }
            }
        }
    }
    setInterval(checkBlockedURL, 150);

    function hideAhrefsAgent() {
        var launcher = document.getElementById('ahrefs-agent-launcher');
        if (!launcher) return;
        var node = launcher;
        var target = null;
        while (node && node !== document.body && node !== document.documentElement) {
            var st = node.getAttribute('style') || '';
            if (st.indexOf('--aboveIntercom') !== -1) {
                target = node;
                break;
            }
            node = node.parentElement;
        }
        if (!target) return;
        target.style.setProperty('display', 'none', 'important');
    }

    function hideLookerSettings() {
        var buttons = document.querySelectorAll('button');
        for (var i = 0; i < buttons.length; i++) {
            var label = (buttons[i].textContent || '').replace(/\s+/g, ' ').trim();
            if (label !== 'Looker Studio') continue;
            var node = buttons[i];
            var depth = 0;
            while (node && node !== document.body && depth < 10) {
                var cls = typeof node.className === 'string' ? node.className : '';
                if (cls.indexOf('standardOnly') !== -1) {
                    node.style.setProperty('display', 'none', 'important');
                    break;
                }
                node = node.parentElement;
                depth++;
            }
        }
    }

    function hideMoreMenu() {
        var uses = document.querySelectorAll('use');
        for (var i = 0; i < uses.length; i++) {
            var href = uses[i].getAttribute('href') || uses[i].getAttribute('xlink:href') || '';
            if (href.indexOf('#More') !== href.length - 5) continue;
            var btn = uses[i].closest ? uses[i].closest('button') : null;
            if (!btn) continue;
            if ((btn.textContent || '').replace(/\s+/g, '').trim() !== '') continue;
            var inner = btn.parentElement;
            if (!inner || inner.getAttribute('data-state') == null) continue;
            var innerCls = typeof inner.className === 'string' ? inner.className : '';
            if (innerCls.indexOf('fullHeight') === -1) continue;
            var outer = inner.parentElement;
            var outerCls = outer && typeof outer.className === 'string' ? outer.className : '';
            if (outer && outerCls.indexOf('fullHeight') !== -1 && outer.children.length === 1) {
                outer.style.setProperty('display', 'none', 'important');
            } else {
                inner.style.setProperty('display', 'none', 'important');
            }
        }
    }

    function applyPanelUsername() {
        var name = window.TM_USERNAME;
        if (!name) return;
        var nodes = document.querySelectorAll('.updateable__workspace-name, [class*="workspaceName"]');
        for (var i = 0; i < nodes.length; i++) {
            if (nodes[i].children && nodes[i].children.length) continue;
            if (nodes[i].textContent !== name) nodes[i].textContent = name;
        }
    }

    // Hide restricted tooltip/profile boxes before painting (MutationObserver)
    var tmObserver = new MutationObserver(function(mutations) {
        hideAhrefsAgent();
        hideLookerSettings();
        hideMoreMenu();
        applyPanelUsername();

        mutations.forEach(function(mutation) {
            mutation.addedNodes.forEach(function(node) {
                if (node.nodeType === 1) {
                    var elements = [];
                    if (node.matches && (node.matches('[role="tooltip"]') || node.matches('div[class*="-box"]') || node.matches('[data-floating-ui-focusable]'))) {
                        elements.push(node);
                    }
                    var matches = node.querySelectorAll ? node.querySelectorAll('[role="tooltip"], div[class*="-box"], [data-floating-ui-focusable]') : [];
                    matches.forEach(function(el) { elements.push(el); });

                    elements.forEach(function(el) {
                        if (el.querySelector('.updateable__workspace-name') || 
                            el.querySelector('a[href*="/user/logout"]') || 
                            el.querySelector('a[href*="/account"]') ||
                            el.querySelector('a[href*="/members"]') ||
                            (el.innerText && (el.innerText.includes("workspace") || el.innerText.includes("Sign out") || el.innerText.includes("Invite members")))) {
                            el.style.setProperty('display', 'none', 'important');
                            el.style.setProperty('visibility', 'hidden', 'important');
                            el.style.setProperty('opacity', '0', 'important');
                            el.style.setProperty('pointer-events', 'none', 'important');
                        }
                    });
                }
            });
        });
    });
    tmObserver.observe(document.documentElement, { childList: true, subtree: true });
    hideAhrefsAgent();
    hideLookerSettings();
    hideMoreMenu();
    applyPanelUsername();
    setInterval(function() { hideAhrefsAgent(); hideLookerSettings(); hideMoreMenu(); applyPanelUsername(); }, 500);

    function hideRestrictedElements() {
        // Replace workspace names with user's own username
        if (window.TM_USERNAME) applyPanelUsername();

        // Sweep tooltips
        var tooltips = document.querySelectorAll('[role="tooltip"], div[class*="-box"], [data-floating-ui-focusable]');
        tooltips.forEach(function(el) {
            if (el.querySelector('.updateable__workspace-name') || 
                el.querySelector('a[href*="/user/logout"]') || 
                el.querySelector('a[href*="/account"]') ||
                el.querySelector('a[href*="/members"]') ||
                (el.innerText && (el.innerText.includes("workspace") || el.innerText.includes("Sign out") || el.innerText.includes("Invite members")))) {
                el.style.setProperty('display', 'none', 'important');
                el.style.setProperty('visibility', 'hidden', 'important');
                el.style.setProperty('opacity', '0', 'important');
                el.style.setProperty('pointer-events', 'none', 'important');
            }
        });

        var links = document.querySelectorAll('a[href*="/account"]');
        links.forEach(function(a) {
            a.style.setProperty('display', 'none', 'important');
            var parent = a.parentElement;
            while (parent && parent !== document.body) {
                if (parent.classList.contains('css-1hvp2c1-menuItem') || 
                    parent.classList.contains('css-kalzs4-inviteButton') ||
                    parent.classList.contains('css-1nqgiwd') ||
                    parent.classList.contains('css-1jubevc-displayGrid')) {
                    parent.style.setProperty('display', 'none', 'important');
                    var prev = parent.previousElementSibling;
                    if (prev && (prev.tagName === 'HR' || prev.querySelector('hr'))) {
                        prev.style.setProperty('display', 'none', 'important');
                    }
                    var next = parent.nextElementSibling;
                    if (next && (next.tagName === 'HR' || next.querySelector('hr'))) {
                        next.style.setProperty('display', 'none', 'important');
                    }
                    break;
                }
                parent = parent.parentElement;
            }
        });

        var buttons = document.querySelectorAll('button');
        buttons.forEach(function(btn) {
            var text = btn.innerText || '';
            if (text.includes('plan,') || text.includes('plan\n') || text.includes('Free plan') || text.includes('Standard plan')) {
                btn.style.setProperty('display', 'none', 'important');
                var parent = btn.parentElement;
                if (parent && parent.classList.contains('css-1nqgiwd')) {
                    parent.style.setProperty('display', 'none', 'important');
                    var prev = parent.previousElementSibling;
                    if (prev && (prev.tagName === 'HR' || prev.querySelector('hr'))) {
                        prev.style.setProperty('display', 'none', 'important');
                    }
                    var next = parent.nextElementSibling;
                    if (next && (next.tagName === 'HR' || next.querySelector('hr'))) {
                        next.style.setProperty('display', 'none', 'important');
                    }
                }
            }
        });
    }
    setInterval(hideRestrictedElements, 100);

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initQuotasWidget);
    } else {
        initQuotasWidget();
    }
})();
</script>`
}

// ── URL REWRITER ──────────────────────────────────────────────────────────────

var (
	reIntegrity      = regexp.MustCompile(`integrity="[^"]*"`)
	reNonce          = regexp.MustCompile(`nonce="[^"]*"`)
	reCrossOrigin    = regexp.MustCompile(`crossorigin="[^"]*"`)
	reReferrerPolicy = regexp.MustCompile(`referrerpolicy="[^"]*"`)
)

// rewriteHTML rewrites all Ahrefs URLs to point to our proxy
// and injects client-side patcher into <head>
func rewriteHTML(body, proxyRoot string) string {
	return rewriteHTMLWithConfig(body, proxyRoot, currentConfig)
}

// rewriteHTMLWithConfig rewrites using provided config (supports hot-reload)
func rewriteHTMLWithConfig(body, proxyRoot string, cfg Config) string {
	host := strings.TrimPrefix(strings.TrimPrefix(proxyRoot, "https://"), "http://")
	body = rewriteAhrefsURL(body, proxyRoot, host, cfg)
	body = rewriteStaticAssetPaths(body)

	// Redirect Turnstile script to our mock
	body = strings.ReplaceAll(body,
		"https://challenges.cloudflare.com/turnstile/v0/api.js",
		proxyRoot+"/cf-turnstile-bypass.js",
	)

	// Strip SRI / CSP / CORS attributes
	body = reIntegrity.ReplaceAllString(body, "")
	body = reNonce.ReplaceAllString(body, "")
	body = reCrossOrigin.ReplaceAllString(body, "")
	body = reReferrerPolicy.ReplaceAllString(body, "")

	// URL-encoded variants inside JSON state blobs
	body = strings.ReplaceAll(body, url.QueryEscape(cfg.TargetURL), url.QueryEscape(proxyRoot))
	body = rewriteStaticAssetPaths(body)

	// Inject client-side patcher
	if idx := strings.Index(strings.ToLower(body), "<head>"); idx != -1 {
		body = body[:idx+6] + patcherScript() + body[idx+6:]
	}

	return body
}

// rewriteURLsOnly only replaces domain URLs in JS/JSON — no HTML injection.
// Used for JavaScript bundles so React Router links use the correct proxy domain.
func rewriteURLsOnly(body, proxyRoot string, cfg Config) string {
	host := strings.TrimPrefix(strings.TrimPrefix(proxyRoot, "https://"), "http://")
	body = rewriteAhrefsURL(body, proxyRoot, host, cfg)
	body = strings.ReplaceAll(body, url.QueryEscape(cfg.TargetURL), url.QueryEscape(proxyRoot))
	return rewriteStaticAssetPaths(body)
}

func ahrefsPublicLogoutLocation(loc string) bool {
	u, err := url.Parse(strings.TrimSpace(loc))
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "ahrefs.com" || host == "www.ahrefs.com"
}

func rewriteAhrefsURL(raw, proxyRoot, host string, cfg Config) string {
	raw = strings.ReplaceAll(raw, cfg.CDNURL, proxyRoot+"/cdn")
	raw = strings.ReplaceAll(raw, cfg.TargetURL, proxyRoot)
	raw = strings.ReplaceAll(raw, "https://www.ahrefs.com", proxyRoot)
	raw = strings.ReplaceAll(raw, "http://www.ahrefs.com", proxyRoot)
	raw = strings.ReplaceAll(raw, "https://ahrefs.com", proxyRoot)
	raw = strings.ReplaceAll(raw, "http://ahrefs.com", proxyRoot)
	raw = strings.ReplaceAll(raw, "//static-app.ahrefs.com", "//"+host+"/cdn")
	raw = strings.ReplaceAll(raw, "//app.ahrefs.com", "//"+host)
	raw = strings.ReplaceAll(raw, "//www.ahrefs.com", "//"+host)
	raw = strings.ReplaceAll(raw, "//ahrefs.com", "//"+host)
	return raw
}

func rewriteStaticAssetPaths(body string) string {
	pairs := [][2]string{
		{`"/assets/esbuild/`, `"/cdn/assets/esbuild/`},
		{`'/assets/esbuild/`, `'/cdn/assets/esbuild/`},
		{`"/assets/fonts/`, `"/cdn/assets/fonts/`},
		{`'/assets/fonts/`, `'/cdn/assets/fonts/`},
		{`"/assets/css/`, `"/cdn/assets/css/`},
		{`'/assets/css/`, `'/cdn/assets/css/`},
	}
	for _, pair := range pairs {
		body = strings.ReplaceAll(body, pair[0], pair[1])
	}
	return body
}

func staticCDNPath(path string) bool {
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	return strings.HasPrefix(path, "/assets/esbuild/") ||
		strings.HasPrefix(path, "/assets/fonts/") ||
		strings.HasPrefix(path, "/assets/css/")
}

// ── RESPONSE HEADER FILTER ────────────────────────────────────────────────────

var dropHeaders = map[string]bool{
	// Standard hop-by-hop headers
	"content-length":    true,
	"content-encoding":  true,
	"transfer-encoding": true,
	"connection":        true,
	"keep-alive":        true,

	// Security headers that break proxied content
	"content-security-policy":             true, // Blocks scripts due to nonce mismatch
	"content-security-policy-report-only": true,
	"strict-transport-security":           true, // Forces HTTPS on our HTTP proxy
	"x-frame-options":                     true, // Blocks iframe embedding
	"x-content-type-options":              true,

	// Cross-origin isolation headers (break CDN asset loading across proxy)
	"cross-origin-embedder-policy": true,
	"cross-origin-opener-policy":   true,
	"cross-origin-resource-policy": true,
	"permissions-policy":           true,
	"referrer-policy":              true,

	// Cloudflare internal headers (not needed by browser)
	"cf-cache-status": true,
	"cf-ray":          true,
	"cf-mitigated":    true,
	"nel":             true,
	"report-to":       true,
}

var (
	reDomain   = regexp.MustCompile(`(?i)domain=[^;]+;?\s*`)
	reSecure   = regexp.MustCompile(`(?i)\bsecure\b;?\s*`)
	reSameNone = regexp.MustCompile(`(?i)SameSite=None;?\s*`)
)

func copyResponseHeaders(dst http.ResponseWriter, src http.Header) {
	for key, values := range src {
		lk := strings.ToLower(key)
		if dropHeaders[lk] {
			continue
		}
		if lk == "set-cookie" {
			for _, v := range values {
				// Parse cookie name (everything before the first '=')
				parts := strings.SplitN(v, "=", 2)
				if len(parts) > 0 {
					cookieName := strings.TrimSpace(parts[0])
					cookieNameLower := strings.ToLower(cookieName)
					// Completely drop sensitive session/authentication cookies
					if cookieNameLower == "bssessid" {
						continue
					}
				}
				dst.Header().Add("Set-Cookie", v)
			}
			continue
		}
		for _, v := range values {
			dst.Header().Add(key, v)
		}
	}
	dst.Header().Set("Access-Control-Allow-Origin", "*")
	dst.Header().Set("Access-Control-Expose-Headers", "Content-Disposition, Content-Length, Content-Type")
}

// ── MAIN PROXY HANDLER ────────────────────────────────────────────────────────

// buildChromeHeaders returns request headers using the User-Agent from config.
// Called per-request so User-Agent changes in config.json are picked up live.
func buildChromeHeaders(ua string) map[string]string {
	// Extract Chrome version from UA for Sec-Ch-Ua header
	secChUa := `"Chromium";v="124", "Google Chrome";v="124", "Not-A.Brand";v="99"`
	if idx := strings.Index(ua, "Chrome/"); idx != -1 {
		versionStr := ua[idx+7:]
		if dot := strings.Index(versionStr, "."); dot != -1 {
			major := versionStr[:dot]
			secChUa = fmt.Sprintf(`"Chromium";v="%s", "Google Chrome";v="%s", "Not-A.Brand";v="99"`, major, major)
		}
	}
	return map[string]string{
		"User-Agent":                ua,
		"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
		"Accept-Language":           "en-US,en;q=0.9",
		"Accept-Encoding":           "gzip, deflate, br",
		"Sec-Ch-Ua":                 secChUa,
		"Sec-Ch-Ua-Mobile":          "?0",
		"Sec-Ch-Ua-Platform":        `"macOS"`,
		"Sec-Fetch-Dest":            "document",
		"Sec-Fetch-Mode":            "navigate",
		"Sec-Fetch-Site":            "same-origin",
		"Sec-Fetch-User":            "?1",
		"Upgrade-Insecure-Requests": "1",
	}
}

func ahrefsStayOnTool(r *http.Request) string {
	ref := r.Referer()
	u, err := url.Parse(ref)
	if err != nil || u.Path == "" || u.Path == "/" {
		return ""
	}
	if u.Host != "" && !strings.EqualFold(u.Host, r.Host) {
		return ""
	}
	switch {
	case strings.HasPrefix(u.Path, "/site-audit"),
		strings.HasPrefix(u.Path, "/site-explorer"),
		strings.HasPrefix(u.Path, "/keywords-explorer"),
		strings.HasPrefix(u.Path, "/content-explorer"),
		strings.HasPrefix(u.Path, "/content-helper"),
		strings.HasPrefix(u.Path, "/competitive-analysis"),
		strings.HasPrefix(u.Path, "/rank-tracker"),
		strings.HasPrefix(u.Path, "/web-analytics"),
		strings.HasPrefix(u.Path, "/brand-radar"):
		return u.RequestURI()
	default:
		return ""
	}
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	// Hot-reload config on every request (reads file only if modified)
	cfg := loadConfig()

	reqPath := r.URL.RequestURI()
	cdnKey := cdnCacheKey(r)
	if serveCachedCDN(w, r) {
		return
	}
	if cdnKey != "" {
		defer completeCDNFlight(cdnKey)
	}

	// ── 0. AUTHENTICATION & ACCESS TOKENS ──────────────────────────────────────

	// A. Handle /access — OTT (One-Time Token) validation from aMemberPro
	if r.URL.Path == "/access" {
		serveAhrefsPanelAccess(w, r)
		return
	}
	if r.URL.Path == "/" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		if back := ahrefsStayOnTool(r); back != "" {
			http.Redirect(w, r, back, http.StatusFound)
			return
		}
	}

	// B. Secure all endpoints except assets/cdn which Ahrefs needs statically
	isAsset := strings.HasPrefix(reqPath, "/cdn/") ||
		strings.Contains(reqPath, ".css") ||
		strings.Contains(reqPath, ".js") ||
		strings.Contains(reqPath, ".png") ||
		strings.Contains(reqPath, ".jpg") ||
		strings.Contains(reqPath, ".svg") ||
		strings.Contains(reqPath, ".woff") ||
		strings.Contains(reqPath, "/assets/") ||
		strings.HasPrefix(reqPath, "/cf-turnstile-bypass.js")

	var currentUser string
	var batchAnalysisPendingCredits int
	var batchAnalysisTargetCount int
	// Site Explorer / counted APIs: reserve dedup before upstream, insert credit only on success.
	var pendingCreditCharge bool
	var pendingCreditLog string
	var pendingCreditDedupKey string
	// Old ct_session gate is off on this copy. New fingerprint and limits will be added next.
	if !isAsset && !ahrefsSessionOK(r) {
		renderAhrefsDenied(w)
		return
	}
	if !isAsset && ahrefsRejectDevice(w, r) {
		return
	}
	if !isAsset {
		if cookie, cErr := r.Cookie("ahrefs_session"); cErr == nil {
			currentUser = ahrefsSessionUser(cookie.Value)
		}
		if currentUser == "" {
			if cookie, cErr := r.Cookie("ct_session"); cErr == nil {
				currentUser = ahrefsSessionUser(cookie.Value)
			}
		}
	}

	// Dynamic Path Protection: Prevent unauthorized access to restricted account settings URLs
	if !isAsset && isBlockedPath(r.URL.Path) {
		clientIP := realClientIP(r)
		if db != nil {
			_, errLog := db.Exec(
				"INSERT INTO ahrefs_violations_logs (website_id, username, client_ip, attempted_path) VALUES (?, ?, ?, ?)",
				currentWebsiteID, currentUser, clientIP, r.URL.RequestURI(),
			)
			if errLog != nil {
				log.Printf("[DB] Failed to log blocked access attempt for user '%s': %v", currentUser, errLog)
			}
		}
		safeRedirect := cfg.PublicScheme + "://" + cfg.PublicHost + "/site-explorer"
		log.Printf("[BLOCK] 🚫 Blocked user '%s' (IP: %s) from restricted path: %s — Redirecting to %s", currentUser, clientIP, r.URL.RequestURI(), safeRedirect)
		http.Redirect(w, r, safeRedirect, http.StatusFound)
		return
	}

	// Old MySQL credit meter is off until the new panel limit is wired.
	if false && !isAsset && db != nil && currentUser != "" {
		// Automatically expire custom limits that have passed their expiration date
		_, _ = db.Exec(`
			UPDATE ahrefs_users u
			JOIN ahrefs_websites w ON u.website_id = w.id
			SET u.credit_limit = COALESCE(w.default_credit_limit, 50),
				u.export_limit = COALESCE(w.default_export_limit, 100000),
				u.custom_limit_expire_at = NULL
			WHERE u.custom_limit_expire_at IS NOT NULL AND u.custom_limit_expire_at < NOW()
		`)

		var creditLimit, exportLimit int
		var exportCycleStart sql.NullTime
		var userStatus string

		err = db.QueryRow("SELECT credit_limit, export_limit, export_cycle_start, status FROM ahrefs_users WHERE username = ? AND website_id = ?", currentUser, currentWebsiteID).Scan(&creditLimit, &exportLimit, &exportCycleStart, &userStatus)
		if err == sql.ErrNoRows {
			// Auto-register user with default limits if missing
			_, err = db.Exec("INSERT INTO ahrefs_users (username, website_id, credit_limit, export_limit) VALUES (?, ?, 50, 100000)", currentUser, currentWebsiteID)
			if err != nil {
				log.Printf("[DB] Auto-creating missing user '%s' failed: %v", currentUser, err)
				http.Error(w, "Database lookup error", http.StatusInternalServerError)
				return
			}
			creditLimit = 50
			exportLimit = 100000
			exportCycleStart = sql.NullTime{Time: time.Now(), Valid: true}
			userStatus = "active"
			log.Printf("[DB] Auto-created missing user profile for: %s under website_id %d ✅", currentUser, currentWebsiteID)
		} else if err != nil {
			log.Printf("[DB] Error fetching limits for user '%s': %v", currentUser, err)
			http.Error(w, "Database lookup error", http.StatusInternalServerError)
			return
		}

		if userStatus == "suspended" {
			http.Error(w, "Account Suspended. Contact support.", http.StatusForbidden)
			return
		}

		// Check weekly export cycle reset (Dynamic Reset)
		var cycleStart time.Time
		if exportCycleStart.Valid {
			cycleStart = exportCycleStart.Time
		} else {
			cycleStart = time.Now()
		}
		if time.Now().After(cycleStart.Add(7 * 24 * time.Hour)) {
			_, _ = db.Exec("UPDATE ahrefs_users SET export_cycle_start = CURRENT_TIMESTAMP WHERE username = ? AND website_id = ?", currentUser, currentWebsiteID)
			_, _ = db.Exec("DELETE FROM ahrefs_export_logs WHERE username = ? AND website_id = ?", currentUser, currentWebsiteID)
			log.Printf("[LIMITS] Dynamically reset weekly export cycle and logs for user: %s scoped to website_id %d ✅", currentUser, currentWebsiteID)
		}

		// ── Global credit exhaustion check ─────────────────────────────────────────
		// Block ALL proxy access (not just counted paths) once the daily credit
		// limit is fully consumed. This prevents users from browsing /dashboard,
		// /site-explorer etc. freely after their quota is exhausted.
		// Assets (JS/CSS/images) are excluded via the outer !isAsset guard.
		var usedCreditsToday int
		_ = db.QueryRow(
			"SELECT COUNT(*) FROM ahrefs_credit_logs WHERE username = ? AND website_id = ? AND DATE(timestamp) = CURDATE()",
			currentUser, currentWebsiteID,
		).Scan(&usedCreditsToday)

		if creditLimit > 0 && usedCreditsToday >= creditLimit {
			// Only allow session-management paths so the user can still
			// authenticate / log out. Everything else (including /dashboard,
			// /site-explorer, API calls etc.) is blocked with the limit page.
			allowedWhenLimitReached := []string{
				"/access",      // session creation after aMember login
				"/user/login",  // Ahrefs login page (passthrough)
				"/user/logout", // allow logout
			}
			blocked := true
			for _, allowed := range allowedWhenLimitReached {
				if strings.HasPrefix(reqPath, allowed) {
					blocked = false
					break
				}
			}
			if blocked {
				log.Printf("[LIMITS] 🚫 Blocked user '%s' — daily credit limit reached (%d/%d) on website_id %d — path: %s",
					currentUser, usedCreditsToday, creditLimit, currentWebsiteID, reqPath)

				// API calls (e.g. /v4/seBacklinks) expect JSON — returning the HTML limit
				// page causes JavaScript to crash with "Couldn't fetch data".
				// Return a proper JSON error so the browser can handle it cleanly.
				isAPICall := strings.HasPrefix(reqPath, "/v4/") ||
					strings.Contains(r.Header.Get("Accept"), "application/json") ||
					strings.Contains(r.Header.Get("X-Requested-With"), "XMLHttpRequest")

				if isAPICall {
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.Header().Set("X-Limit-Reached", "daily_credit")
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprintf(w, `{"error":"daily_credit_limit_reached","message":"Your daily Ahrefs credit limit has been reached. Please try again tomorrow at midnight IST.","code":403}`)
				} else {
					renderLimitReachedPage(w, "credit")
				}
				return
			}
		}

		// Counted APIs: reserve a dedup slot now; DB insert happens only after a successful
		// upstream response (so suspicious / login / blank / account-switch failures do not bill).
		if isCounted(reqPath) {
			rawTarget := extractTarget(r)
			target := normalizeTarget(rawTarget)
			reportGroup := getReportGroup(reqPath)
			// Short burst dedup: parallel Overview APIs on one page load share 1 credit.
			// Different reports (Backlinks vs Overview) and other domains still bill.
			// Same domain searched again after the burst (or in another tab later) bills again.
			dedupKey := fmt.Sprintf("%s:%d:%s:%s", currentUser, currentWebsiteID, reportGroup, target)
			dedupWindow := 8 * time.Second

			shouldCharge := true
			now := time.Now()
			if prev, loaded := creditHitCache.LoadOrStore(dedupKey, now); loaded {
				if time.Since(prev.(time.Time)) < dedupWindow {
					shouldCharge = false
					log.Printf("[LIMITS] Deduplicated credit hit for '%s' (Group: %s, Target: %s) within %v window", currentUser, reportGroup, target, dedupWindow)
				} else {
					creditHitCache.Store(dedupKey, now)
				}
			}

			// Ahrefs SPA: searching B re-fires Overview for previous A within ~seconds.
			// Suppress only that stale prev Overview; do NOT block intentional re-search of A.
			if shouldCharge && reportGroup == "Overview" && target != "" && target != "unknown_target" {
				userKey := creditUserKey(currentUser, currentWebsiteID)
				if raw, ok := creditOverviewSwitch.Load(userKey); ok {
					st := raw.(*overviewSwitchState)
					if st.prev != "" && target == st.prev && now.Before(st.suppressUntil) {
						shouldCharge = false
						creditHitCache.Delete(dedupKey)
						log.Printf("[LIMITS] Suppressed stale Overview re-fetch for '%s' target=%s (switched to %s)", currentUser, target, st.current)
					}
				}
			}

			if shouldCharge {
				if usedCreditsToday >= creditLimit {
					creditHitCache.Delete(dedupKey)
					log.Printf("[LIMITS] Blocked counted path '%s' for user '%s' — limit reached", reqPath, currentUser)
					if strings.HasPrefix(reqPath, "/v4/") {
						w.Header().Set("Content-Type", "application/json; charset=utf-8")
						w.WriteHeader(http.StatusForbidden)
						fmt.Fprintf(w, `{"error":"daily_credit_limit_reached","message":"Your daily Ahrefs credit limit has been reached. Please try again tomorrow at midnight IST.","code":403}`)
					} else {
						renderLimitReachedPage(w, "credit")
					}
					return
				}

				logString := fmt.Sprintf("%s: %s", reportGroup, target)
				if len(logString) > 255 {
					logString = logString[:255]
				}
				pendingCreditCharge = true
				pendingCreditLog = logString
				pendingCreditDedupKey = dedupKey
				log.Printf("[LIMITS] Pending 1 credit for '%s' on %s (will commit after success) under website_id %d", currentUser, logString, currentWebsiteID)
			}
		}

		// Batch Analysis (/v4/baTable) consumes Ahrefs credits: 1 credit per 200 URLs.
		if batchAnalysisEndpoint(r.URL.Path) && r.Method == http.MethodPost && creditLimit > 0 {
			bodyBytes, readErr := io.ReadAll(r.Body)
			r.Body.Close()
			if readErr != nil {
				log.Printf("[LIMITS] Failed reading batch analysis body for '%s': %v", currentUser, readErr)
				http.Error(w, "Bad request", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

			targetCount := parseBatchTargetCount(bodyBytes)
			creditCost := batchAnalysisCreditCost(targetCount)
			if creditCost > 0 {
				batchAnalysisTargetCount = targetCount
				batchAnalysisPendingCredits = creditCost
				if usedCreditsToday+creditCost > creditLimit {
					log.Printf("[LIMITS] Blocked batch analysis for '%s' — credit limit (%d + %d > %d) for %d targets on website_id %d",
						currentUser, usedCreditsToday, creditCost, creditLimit, targetCount, currentWebsiteID)
					writeDailyCreditLimitJSON(w, usedCreditsToday, creditLimit)
					return
				}
				log.Printf("[LIMITS] Batch analysis pre-check OK for '%s': %d targets → %d credit(s) (%d/%d used today)",
					currentUser, targetCount, creditCost, usedCreditsToday, creditLimit)
			}
		}
	}

	// ── 1. CF Turnstile Mock endpoint ─────────────────────────────────────────
	if strings.HasPrefix(reqPath, "/cf-turnstile-bypass.js") {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		fmt.Fprint(w, cfBypassScript())
		log.Printf("[BYPASS] Served CF Turnstile mock ✅")
		return
	}

	// ── 2. Build target URL (from config) ───────────────────────────────────────
	var targetStr string
	if strings.HasPrefix(reqPath, "/cdn/") {
		targetStr = cfg.CDNURL + "/" + strings.TrimPrefix(reqPath, "/cdn/")
	} else if staticCDNPath(r.URL.Path) {
		targetStr = cfg.CDNURL + r.URL.Path
		if r.URL.RawQuery != "" {
			targetStr += "?" + r.URL.RawQuery
		}
	} else {
		targetStr = cfg.TargetURL + reqPath
	}

	log.Printf("[PROXY] %s %s", r.Method, reqPath)

	// ── 3. Build upstream request ─────────────────────────────────────────────
	// ── 3. Build & Execute Upstream Request (Load Balanced & Self-Healing Retry Loop) ──
	// ── 3. Build & Execute Upstream Request (Load Balanced & Self-Healing Retry Loop) ──
	var activeAcc AhrefsAccount
	var useDBAccount bool
	var sessionToken string

	cookie, errCookie := r.Cookie("ahrefs_session")
	if errCookie != nil {
		cookie, errCookie = r.Cookie("ct_session")
	}
	if errCookie == nil {
		sessionToken = cookie.Value
	}

	if panelAcc, ok := ahrefsPanelAccount(cfg.PublicHost, sessionToken); ok {
		activeAcc = panelAcc
		useDBAccount = true
	} else if sessionToken != "" {
		var found bool
		activeAcc, found = getSessionAssignedAccount(sessionToken)
		if found {
			useDBAccount = true
			log.Printf("[LB] Loaded pre-assigned account: %s (ID: %d) for user session", activeAcc.Name, activeAcc.ID)
		} else {
			activeAcc, err = autoAssignNextAccount(sessionToken)
			if err == nil {
				useDBAccount = true
			} else if acc, ok := accountFromCookieFile(cfg); ok {
				activeAcc = acc
				log.Printf("[COOKIE] Using %s (plain array or GoAuto, referer optional)", cfg.CookieFile)
			} else {
				log.Printf("[LB] ⚠️ No active Ahrefs accounts available in DB: %v", err)
				renderNoActiveAccountsPage(w)
				return
			}
		}
	}

	if !useDBAccount {
		activeAcc, err = selectActiveAccount()
		if err == nil {
			useDBAccount = true
			log.Printf("[LB] Fallback LRU selected active Ahrefs account: %s (ID: %d)", activeAcc.Name, activeAcc.ID)
		} else if acc, ok := accountFromCookieFile(cfg); ok {
			activeAcc = acc
			log.Printf("[COOKIE] Using %s (plain array or GoAuto, referer optional)", cfg.CookieFile)
		} else {
			log.Printf("[LB] ⚠️ No active Ahrefs accounts found in DB (%v)", err)
			if sessionToken != "" {
				renderNoActiveAccountsPage(w)
				return
			}
		}
	}

	safeForward := map[string]bool{"accept": true, "accept-language": true, "content-type": true}
	limitUser := currentUser
	if limitUser == "" {
		limitUser = ahrefsSessionUser(sessionToken)
	}
	var pendingLimit ahrefsLimitHit
	pendingLimitOK := false
	if limitUser != "" && !isAsset {
		limitBody := ahrefsReadLimitBody(r)
		if hit, ok := ahrefsLimitHitFor(r, limitBody); ok {
			fallback := 30
			if hit.isExport {
				fallback = 1000
			}
			view, _, found := ahrefsMeterView(limitUser, hit.key, fallback, hit.reset)
			ask := 0
			if hit.isExport {
				ask = ahrefsExportAsk(r, limitBody)
			}
			over := found && view.limit > 0 && view.used >= view.limit
			exportShort := hit.isExport && found && view.limit > 0 && ask > 0 && view.used+ask > view.limit
			if over || exportShort {
				ahrefsBlockLimit(w, r, hit, view.used, view.limit, ask)
				return
			}
			pendingLimit = hit
			pendingLimitOK = true
		}
	}
	if useDBAccount && db == nil && (r.URL.Path == "/user/login" || strings.HasPrefix(r.URL.Path, "/user/login/")) {
		userName := currentUser
		if userName == "" {
			userName = ahrefsSessionUser(sessionToken)
		}
		nextAcc, switchErr := switchAhrefsPanelAccount(cfg.PublicHost, sessionToken, activeAcc, userName, "opened /user/login")
		nextName := ""
		if switchErr != nil {
			log.Printf("[LB] still looking for another account after /user/login: %v", switchErr)
		} else {
			nextName = nextAcc.Name
		}
		renderAccountSwitchPage(w, nextName, ahrefsSwitchReturnPath(r))
		return
	}
	var resp *http.Response
	var execErr error

	for attempt := 1; attempt <= 3; attempt++ {
		// If using DB/panel accounts, switch active account on retry
		if attempt > 1 && useDBAccount {
			if db == nil {
				userName := currentUser
				if userName == "" {
					userName = ahrefsSessionUser(sessionToken)
				}
				nextAcc, switchErr := switchAhrefsPanelAccount(cfg.PublicHost, sessionToken, activeAcc, userName, fmt.Sprintf("retry attempt %d", attempt))
				if switchErr != nil {
					log.Printf("[LB] Retry %d: still looking for another panel account: %v", attempt, switchErr)
					break
				}
				activeAcc = nextAcc
				log.Printf("[LB] Retry %d: panel account %s (ID: %d)", attempt, activeAcc.Name, activeAcc.ID)
			} else {
				if sessionToken != "" {
					activeAcc, err = autoAssignNextAccount(sessionToken)
				} else {
					activeAcc, err = selectActiveAccount()
				}
				if err != nil {
					log.Printf("[LB] Retry %d failed to select active DB account: %v", attempt, err)
					break
				}
				log.Printf("[LB] Retry %d: Selected active Ahrefs account: %s (ID: %d)", attempt, activeAcc.Name, activeAcc.ID)
			}
		}

		// Recreate request body reader if needed (rewind body)
		upReq, err := http.NewRequest(r.Method, targetStr, r.Body)
		if err != nil {
			http.Error(w, "Bad gateway: "+err.Error(), http.StatusBadGateway)
			return
		}

		// ── Inject Cookies, User-Agent & Account-specific Proxy ──────────────────
		var combinedCookie string
		var currentUserAgent string
		if useDBAccount {
			// parseCookieFromDB handles both JSON array format and raw name=value format
			combinedCookie = parseCookieFromDB(activeAcc.Cookie)
			currentUserAgent = activeAcc.UserAgent
			// Attach account-specific SOCKS5/HTTP proxy to context
			ctx := context.WithValue(r.Context(), proxyContextKey, activeAcc.Proxy)
			upReq = upReq.WithContext(ctx)
		} else {
			// DB-only mode: use config user-agent, no cookie fallback
			currentUserAgent = cfg.UserAgent
			combinedCookie = ""
		}

		browserCookies := stripSensitiveCookies(r.Header.Get("Cookie"))
		if browserCookies != "" && combinedCookie != "" {
			combinedCookie += "; " + browserCookies
		} else if browserCookies != "" {
			combinedCookie = browserCookies
		}
		upReq.Header.Set("Cookie", sanitizeCookieHeader(combinedCookie))

		// Set Chrome-like headers
		for k, v := range buildChromeHeaders(currentUserAgent) {
			upReq.Header.Set(k, v)
		}
		upReq.Header.Set("Referer", cfg.TargetURL+"/")
		upReq.Header.Set("Origin", cfg.TargetURL)

		// Forward safe headers
		for key, values := range r.Header {
			if safeForward[strings.ToLower(key)] {
				for _, v := range values {
					upReq.Header.Set(key, v)
				}
			}
		}

		// ── Execute upstream request ──────────────────────────────────────────────
		resp, execErr = httpClient.Do(upReq)
		if execErr != nil {
			// Network failure — just retry, do NOT change any account status
			log.Printf("[LB] Attempt %d network error: %v — retrying...", attempt, execErr)
			continue
		}

		// ── Check for redirect to login page ─────────────────────────────────────
		shouldSwitch := false
		switchReason := ""

		if resp.StatusCode >= 301 && resp.StatusCode <= 308 {
			loc := resp.Header.Get("Location")
			log.Printf("[REDIRECT] %s → %s", r.URL.Path, loc)
			if strings.Contains(loc, "/user/login") || strings.Contains(loc, "/account/login") || strings.Contains(loc, "/login") || ahrefsPublicLogoutLocation(loc) {
				shouldSwitch = true
				switchReason = "opened /user/login"
			}
		}

		// ── Scan HTML body for session/plan trigger strings ───────────────────────
		// Only for HTML responses — read body, scan, then buffer it back.
		// IMPORTANT: if gzip is decompressed here, we strip Content-Encoding so that
		// the downstream decode step (section 7) does not try to decompress again.
		if !shouldSwitch && useDBAccount {
			contentTypeHdr := resp.Header.Get("Content-Type")
			if strings.Contains(contentTypeHdr, "text/html") {
				var bodyReader io.Reader = resp.Body
				wasGzip := false
				if resp.Header.Get("Content-Encoding") == "gzip" {
					gzr, gzErr := gzip.NewReader(resp.Body)
					if gzErr == nil {
						defer gzr.Close()
						bodyReader = gzr
						wasGzip = true
					}
				}

				// Read the FULL response body for accurate trigger scanning.
				// Previously only 8KB was read which missed trigger phrases embedded
				// deeper in Ahrefs's React/Next.js server-rendered HTML (e.g. "Suspicious
				// activity detected" appears far into the HTML body). Cap at 512KB for safety.
				fullBody, _ := io.ReadAll(bodyReader)
				resp.Body = io.NopCloser(bytes.NewReader(fullBody))

				// ── Critical: if we already decoded gzip above, strip the header ──
				// Section 7 checks Content-Encoding to decide whether to decompress.
				// If we leave it as "gzip" but body is now plain bytes → double-decode → garbage.
				if wasGzip {
					resp.Header.Del("Content-Encoding")
					resp.Header.Del("Content-Length") // length changed after decompress
				}

				// Build scan snippet: first 512KB of body, lowercased for case-insensitive matching
				scanLen := len(fullBody)
				if scanLen > 524288 {
					scanLen = 524288
				}
				snippet := string(fullBody[:scanLen])
				snippetLower := strings.ToLower(snippet)

				// ── Trigger conditions: account switch required ──────────────────────
				switch {
				case strings.Contains(snippet, "Sign in to Ahrefs"), strings.Contains(snippet, "Sign in to your account"):
					shouldSwitch = true
					switchReason = "sign in to your account"
				case strings.Contains(snippetLower, "/user/login") && strings.Contains(snippetLower, "/pricing?utm_source=dashboard"):
					shouldSwitch = true
					switchReason = "sign in and sign up shown"

				case strings.Contains(snippetLower, "suspicious activity") ||
					strings.Contains(snippetLower, "temporarily restricted") ||
					strings.Contains(snippetLower, "suspicious_activity") ||
					strings.Contains(snippetLower, "potential violations"):
					shouldSwitch = true
					switchReason = "suspicious activity detected"

				case strings.Contains(snippet, "Domain not verified"):
					shouldSwitch = true
					switchReason = "domain not verified / plan expired"

				case (strings.Contains(snippet, "See pricing") && strings.Contains(snippetLower, "/pricing")) ||
					strings.Contains(snippetLower, "see pricing") ||
					strings.Contains(snippetLower, "see_pricing"):
					shouldSwitch = true
					switchReason = "account plan downgraded to free / see pricing shown"
				}
			}
		}

		// ── Perform circular account switch if trigger detected ───────────────────
		if shouldSwitch && useDBAccount {
			resp.Body.Close()
			log.Printf("[LB] account %s id=%d trigger: %s", activeAcc.Name, activeAcc.ID, switchReason)
			var nextAcc AhrefsAccount
			var switchErr error
			if db == nil {
				userName := currentUser
				if userName == "" {
					userName = ahrefsSessionUser(sessionToken)
				}
				nextAcc, switchErr = switchAhrefsPanelAccount(cfg.PublicHost, sessionToken, activeAcc, userName, switchReason)
				nextName := ""
				if switchErr != nil {
					log.Printf("[LB] still looking for another account: %v", switchErr)
				} else {
					nextName = nextAcc.Name
				}
				renderAccountSwitchPage(w, nextName, ahrefsSwitchReturnPath(r))
				return
			} else {
				nextAcc, switchErr = switchToNextAccount(sessionToken, activeAcc.ID, activeAcc.Name, currentUser, switchReason)
			}
			if switchErr != nil {
				log.Printf("[LB] ⚠️ No other active accounts to switch to: %v", switchErr)
				// Do not bill credits when every account failed (suspicious / login / etc.).
				if pendingCreditDedupKey != "" {
					creditHitCache.Delete(pendingCreditDedupKey)
					pendingCreditCharge = false
				}
				renderNoActiveAccountsPage(w)
				return
			}
			activeAcc = nextAcc
			continue
		}

		// ── Success — exit retry loop ─────────────────────────────────────────────
		break
	}

	if execErr != nil {
		log.Printf("[ERROR] Upstream execution failed: %v", execErr)
		if pendingCreditDedupKey != "" {
			creditHitCache.Delete(pendingCreditDedupKey)
			pendingCreditCharge = false
		}
		http.Error(w, "Upstream persistent connection failure: "+execErr.Error(), http.StatusBadGateway)
		return
	}
	if resp == nil {
		log.Printf("[ERROR] Upstream returned nil response without error for %s", reqPath)
		if pendingCreditDedupKey != "" {
			creditHitCache.Delete(pendingCreditDedupKey)
			pendingCreditCharge = false
		}
		http.Error(w, "Upstream empty response", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	log.Printf("[STATUS] %d ← %s", resp.StatusCode, targetStr)

	// ── 6. Handle redirects ───────────────────────────────────────────────────
	if resp.StatusCode >= 301 && resp.StatusCode <= 308 {
		loc := resp.Header.Get("Location")
		// Redirects are not billable data responses (login hop or client navigation).
		if pendingCreditDedupKey != "" {
			creditHitCache.Delete(pendingCreditDedupKey)
			pendingCreditCharge = false
		}
		if ahrefsPublicLogoutLocation(loc) {
			userName := currentUser
			if userName == "" {
				userName = ahrefsSessionUser(sessionToken)
			}
			nextName := ""
			if nextAcc, switchErr := switchAhrefsPanelAccount(cfg.PublicHost, sessionToken, activeAcc, userName, "opened /user/login"); switchErr != nil {
				log.Printf("[LB] still looking for another account: %v", switchErr)
			} else {
				nextName = nextAcc.Name
			}
			renderAccountSwitchPage(w, nextName, ahrefsSwitchReturnPath(r))
			return
		}
		host := cfg.PublicHost
		root := cfg.PublicScheme + "://" + host
		if host == "" {
			host = r.Host
			root = "http://" + host
			if r.TLS != nil {
				root = "https://" + host
			}
		}
		loc = rewriteAhrefsURL(loc, root, host, cfg)
		if strings.HasPrefix(loc, root) {
			loc = strings.TrimPrefix(loc, root)
		}
		if loc == "" {
			loc = "/dashboard"
		}
		if loc == r.URL.Path || loc == r.URL.RequestURI() {
			userName := currentUser
			if userName == "" {
				userName = ahrefsSessionUser(sessionToken)
			}
			nextName := ""
			if nextAcc, switchErr := switchAhrefsPanelAccount(cfg.PublicHost, sessionToken, activeAcc, userName, "opened /user/login"); switchErr != nil {
				log.Printf("[LB] still looking for another account: %v", switchErr)
			} else {
				nextName = nextAcc.Name
			}
			renderAccountSwitchPage(w, nextName, ahrefsSwitchReturnPath(r))
			return
		}
		copyResponseHeaders(w, resp.Header)
		http.Redirect(w, r, loc, resp.StatusCode)
		return
	}

	// ── 7. Decode body ────────────────────────────────────────────────────────
	var reader io.Reader = resp.Body
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		gz, err := gzip.NewReader(resp.Body)
		if err == nil {
			defer gz.Close()
			reader = gz
		}
	case "br":
		reader = brotli.NewReader(resp.Body)
	}

	rawBody, err := io.ReadAll(reader)
	if err != nil {
		log.Printf("[ERROR] Read body: %v", err)
		if pendingCreditDedupKey != "" {
			creditHitCache.Delete(pendingCreditDedupKey)
			pendingCreditCharge = false
		}
		http.Error(w, "Read error", http.StatusBadGateway)
		return
	}
	if pendingLimitOK && resp.StatusCode >= 200 && resp.StatusCode < 300 && ahrefsUsableResult(rawBody) {
		if pendingLimit.isExport {
			pendingLimit.amount = ahrefsCSVRows(rawBody)
			view, _, found := ahrefsMeterView(limitUser, pendingLimit.key, 1000, pendingLimit.reset)
			if found && view.limit > 0 && view.used+pendingLimit.amount > view.limit {
				ahrefsBlockLimit(w, r, pendingLimit, view.used, view.limit, pendingLimit.amount)
				return
			}
		}
		ahrefsSaveUsage(limitUser, pendingLimit)
	} else if pendingLimitOK {
		log.Printf("[LIMITS] skip credit user=%s path=%s status=%d — result not delivered", limitUser, r.URL.Path, resp.StatusCode)
	}

	// Commit reserved Site Explorer / counted-API credit only after a usable response.
	if pendingCreditCharge && currentUser != "" && db != nil {
		commitCredit := resp.StatusCode >= 200 && resp.StatusCode < 400
		if commitCredit {
			bodyLower := strings.ToLower(string(rawBody))
			if strings.Contains(bodyLower, "sign in to ahrefs") ||
				strings.Contains(bodyLower, "suspicious activity") ||
				strings.Contains(bodyLower, "suspicious_activity") ||
				strings.Contains(bodyLower, "temporarily restricted") ||
				strings.Contains(bodyLower, "potential violations") ||
				strings.Contains(bodyLower, "domain not verified") ||
				(strings.Contains(bodyLower, "see pricing") && strings.Contains(bodyLower, "/pricing")) {
				commitCredit = false
				log.Printf("[LIMITS] Skipping credit for '%s' — failure/suspicious content in response (%s)", currentUser, pendingCreditLog)
			}
		} else {
			log.Printf("[LIMITS] Skipping credit for '%s' — upstream status %d (%s)", currentUser, resp.StatusCode, pendingCreditLog)
		}
		if commitCredit {
			_, err = db.Exec(
				"INSERT INTO ahrefs_credit_logs (username, website_id, endpoint) VALUES (?, ?, ?)",
				currentUser, currentWebsiteID, pendingCreditLog,
			)
			if err != nil {
				log.Printf("[DB] Failed logging credit hit for '%s' under website_id %d: %v", currentUser, currentWebsiteID, err)
				if pendingCreditDedupKey != "" {
					creditHitCache.Delete(pendingCreditDedupKey)
				}
			} else {
				log.Printf("[LIMITS] Logged 1 credit hit for '%s' on %s under website_id %d", currentUser, pendingCreditLog, currentWebsiteID)

				// Overview domain switch: Ahrefs often bills the previous domain again right
				// before/after the new search. Reverse that stale hit; keep intentional re-search.
				if strings.HasPrefix(pendingCreditLog, "Overview: ") {
					newTarget := strings.TrimSpace(strings.TrimPrefix(pendingCreditLog, "Overview: "))
					userKey := creditUserKey(currentUser, currentWebsiteID)
					now := time.Now()
					prevTarget := ""
					lastAt := time.Time{}
					if raw, ok := creditOverviewSwitch.Load(userKey); ok {
						st := raw.(*overviewSwitchState)
						prevTarget = st.current
						lastAt = st.lastCommitAt
					}
					if prevTarget != "" && prevTarget != newTarget && !lastAt.IsZero() && now.Sub(lastAt) < 20*time.Second {
						reverseRecentOverviewCredit(currentUser, currentWebsiteID, prevTarget)
					}
					st := &overviewSwitchState{
						current:      newTarget,
						lastCommitAt: now,
					}
					if prevTarget != "" && prevTarget != newTarget {
						st.prev = prevTarget
						st.suppressUntil = now.Add(20 * time.Second)
					}
					creditOverviewSwitch.Store(userKey, st)
				}
			}
		} else if pendingCreditDedupKey != "" {
			creditHitCache.Delete(pendingCreditDedupKey)
		}
		pendingCreditCharge = false
	}

	// ── 7.5. CSV Export Row Limit Checking ─────────────────────────────────────
	isExportRequest := strings.Contains(reqPath, "Export") ||
		strings.Contains(reqPath, "export") ||
		strings.Contains(reqPath, "mode=csv") ||
		strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/csv")

	if isExportRequest && !isAsset && db != nil {
		// Automatically expire custom limits that have passed their expiration date
		_, _ = db.Exec(`
			UPDATE ahrefs_users u
			JOIN ahrefs_websites w ON u.website_id = w.id
			SET u.credit_limit = COALESCE(w.default_credit_limit, 50),
				u.export_limit = COALESCE(w.default_export_limit, 100000),
				u.custom_limit_expire_at = NULL
			WHERE u.custom_limit_expire_at IS NOT NULL AND u.custom_limit_expire_at < NOW()
		`)

		var creditLimit, exportLimit int
		var exportCycleStart sql.NullTime
		var userStatus string

		err = db.QueryRow("SELECT credit_limit, export_limit, export_cycle_start, status FROM ahrefs_users WHERE username = ? AND website_id = ?", currentUser, currentWebsiteID).Scan(&creditLimit, &exportLimit, &exportCycleStart, &userStatus)
		if err == sql.ErrNoRows {
			creditLimit = 50
			exportLimit = 100000
			userStatus = "active"
			err = nil
		}
		if err == nil {
			// Highly robust newline counter supporting UTF-16 LE, UTF-16 BE, and UTF-8/ASCII
			rowCount := 0
			if len(rawBody) >= 2 && rawBody[0] == 0xff && rawBody[1] == 0xfe {
				// UTF-16 LE BOM (Standard Ahrefs Export)
				rowCount = bytes.Count(rawBody, []byte{0x0a, 0x00})
			} else if len(rawBody) >= 2 && rawBody[0] == 0xfe && rawBody[1] == 0xff {
				// UTF-16 BE BOM
				rowCount = bytes.Count(rawBody, []byte{0x00, 0x0a})
			} else {
				// Fallback: check if the body has a lot of null bytes (indicating UTF-16 LE without BOM)
				nullCount := bytes.Count(rawBody, []byte{0x00})
				if nullCount > len(rawBody)/3 {
					// Likely UTF-16 LE
					rowCount = bytes.Count(rawBody, []byte{0x0a, 0x00})
				} else {
					// Standard UTF-8 / ASCII
					rowCount = bytes.Count(rawBody, []byte("\n"))
				}
			}

			// If rowCount is still 0, try counting standard newlines as fallback
			if rowCount == 0 && len(rawBody) > 0 {
				rowCount = bytes.Count(rawBody, []byte("\n"))
			}

			// Subtract 1 for the CSV header row
			if rowCount > 0 {
				rowCount--
			}

			var totalExportedRows int
			err = db.QueryRow("SELECT COALESCE(SUM(rows_count), 0) FROM ahrefs_export_logs WHERE username = ? AND website_id = ?", currentUser, currentWebsiteID).Scan(&totalExportedRows)
			if err != nil {
				log.Printf("[DB] Error scanning export logs for '%s' under website_id %d: %v", currentUser, currentWebsiteID, err)
			}

			if totalExportedRows+rowCount > exportLimit {
				log.Printf("[LIMITS] Blocked export for user '%s' — Row limit reached. Row count: %d, current cycle: %d/%d on website_id %d", currentUser, rowCount, totalExportedRows, exportLimit, currentWebsiteID)

				isAPICall := strings.HasPrefix(reqPath, "/v4/") ||
					strings.Contains(r.Header.Get("Accept"), "application/json") ||
					strings.Contains(r.Header.Get("X-Requested-With"), "XMLHttpRequest") ||
					strings.Contains(reqPath, "Export") ||
					strings.Contains(reqPath, "export")

				if isAPICall {
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.Header().Set("X-Limit-Reached", "weekly_export")
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprintf(w, `{"error":"weekly_export_limit_reached","message":"Weekly Export Rows Limit Reached. You have reached your weekly CSV export row limit (%d / %d rows).","code":403}`, totalExportedRows, exportLimit)
				} else {
					renderLimitReachedPage(w, "export")
				}
				return
			}

			// Log this export rows count (strip query parameters to fit VARCHAR(255) safely)
			logPath := reqPath
			if idx := strings.Index(logPath, "?"); idx != -1 {
				logPath = logPath[:idx]
			}
			if len(logPath) > 255 {
				logPath = logPath[:255]
			}
			_, err = db.Exec("INSERT INTO ahrefs_export_logs (username, website_id, rows_count, endpoint) VALUES (?, ?, ?, ?)", currentUser, currentWebsiteID, rowCount, logPath)
			if err != nil {
				log.Printf("[DB] Failed logging export rows for '%s' under website_id %d: %v", currentUser, currentWebsiteID, err)
			} else {
				log.Printf("[LIMITS] Logged %d export rows for '%s' on %s (%d/%d used this week) under website_id %d", currentUser, rowCount, logPath, totalExportedRows+rowCount, exportLimit, currentWebsiteID)
			}
		}
	}

	// ── 7.55 Batch Analysis credit logging (/v4/baTable) ─────────────────────
	// Analysis run uses credits (1 per 200 URLs). CSV export of the report still
	// goes through the normal export path above (rows − 1).
	if batchAnalysisEndpoint(r.URL.Path) && resp.StatusCode == http.StatusOK && currentUser != "" && db != nil && !isAsset {
		creditCost := batchAnalysisPendingCredits
		if creditCost <= 0 {
			creditCost = batchAnalysisCreditCost(parseBatchResponseRowCount(rawBody))
		}
		if creditCost <= 0 && batchAnalysisTargetCount > 0 {
			creditCost = batchAnalysisCreditCost(batchAnalysisTargetCount)
		}
		if creditCost > 0 {
			logString := fmt.Sprintf("Batch Analysis: %d targets (%d credits)", batchAnalysisTargetCount, creditCost)
			if batchAnalysisTargetCount <= 0 {
				logString = fmt.Sprintf("Batch Analysis (%d credits)", creditCost)
			}
			if len(logString) > 255 {
				logString = logString[:255]
			}
			for i := 0; i < creditCost; i++ {
				_, err = db.Exec(
					"INSERT INTO ahrefs_credit_logs (username, website_id, endpoint) VALUES (?, ?, ?)",
					currentUser, currentWebsiteID, logString,
				)
				if err != nil {
					log.Printf("[DB] Failed logging batch analysis credit for '%s': %v", currentUser, err)
					break
				}
			}
			if err == nil {
				log.Printf("[LIMITS] Logged %d batch analysis credit(s) for '%s' (%d targets) on website_id %d",
					creditCost, currentUser, batchAnalysisTargetCount, currentWebsiteID)
			}
		}
	}

	// ── 8. Content type ───────────────────────────────────────────────────────
	contentType := resp.Header.Get("Content-Type")
	isHTML := strings.Contains(contentType, "text/html")
	isJS := strings.Contains(contentType, "javascript")

	// ── 9. Determine public-facing URL ────────────────────────────────────────
	// Apache reverse proxy (ProxyPreserveHost Off) passes r.Host = "127.0.0.1:7842"
	// Fix: use public_host from config.json if set.
	var scheme, publicHost string
	if cfg.PublicHost != "" {
		publicHost = cfg.PublicHost
		scheme = cfg.PublicScheme
	} else {
		publicHost = r.Host
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		}
		if xfh := r.Header.Get("X-Forwarded-Host"); xfh != "" {
			publicHost = xfh
		}
		if strings.HasPrefix(publicHost, "127.") || strings.HasPrefix(publicHost, "localhost") {
			log.Printf("[WARN] Host=%s — config.json mein 'public_host' set karo!", publicHost)
		}
	}
	proxyRoot := scheme + "://" + publicHost

	// ── 10. Send response ─────────────────────────────────────────────────────
	copyResponseHeaders(w, resp.Header)

	if isHTML {
		processed := rewriteHTMLWithConfig(string(rawBody), proxyRoot, cfg)
		if currentUser != "" {
			injection := fmt.Sprintf("<script>window.TM_USERNAME = %q;</script>", currentUser)
			if idx := strings.Index(strings.ToLower(processed), "<head>"); idx != -1 {
				processed = processed[:idx+6] + injection + processed[idx+6:]
			}
		}
		processed = injectAhrefsDeviceScript(processed)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(resp.StatusCode)
		w.Write([]byte(processed))
		log.Printf("[HTML] %d bytes → %d bytes", len(rawBody), len(processed))
	} else if isJS {
		processed := string(rawBody)
		if bytes.Contains(rawBody, []byte("ahrefs.com")) || bytes.Contains(rawBody, []byte("/assets/esbuild/")) || bytes.Contains(rawBody, []byte("/assets/fonts/")) || bytes.Contains(rawBody, []byte("/assets/css/")) {
			processed = rewriteURLsOnly(processed, proxyRoot, cfg)
		}
		bodyOut := []byte(processed)
		if cdnCacheKey(r) != "" && resp.StatusCode == http.StatusOK {
			w.Header().Set("Cache-Control", "public, max-age=86400")
			storeCDNCache(r, resp.StatusCode, contentType, bodyOut)
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(resp.StatusCode)
		w.Write(bodyOut)
	} else {
		if ahrefsExportPath(r.URL.Path) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Del("Expires")
		} else if cdnCacheKey(r) != "" && resp.StatusCode == http.StatusOK && len(rawBody) <= 20<<20 {
			w.Header().Set("Cache-Control", "public, max-age=86400")
			storeCDNCache(r, resp.StatusCode, contentType, rawBody)
		}
		w.WriteHeader(resp.StatusCode)
		n, _ := io.Copy(w, bytes.NewReader(rawBody))
		log.Printf("[STREAM] %s — %d bytes", contentType, n)
	}
}

func startCleanupCron() {
	go func() {
		for {
			time.Sleep(30 * time.Second)
			if db != nil {
				_, err1 := db.Exec("DELETE FROM ahrefs_tokens WHERE expires_at < ?", time.Now())
				_, err2 := db.Exec("DELETE FROM ahrefs_sessions WHERE expires_at < ?", time.Now())
				if err1 != nil || err2 != nil {
					log.Printf("[CLEANUP] ⚠️ Expired sessions/tokens cleanup error: tokens=%v, sessions=%v", err1, err2)
				}
				// Purge analytics logs older than 30 days
				_, _ = db.Exec("DELETE FROM ahrefs_login_logs WHERE logged_in_at < DATE_SUB(NOW(), INTERVAL 30 DAY)")
				_, _ = db.Exec("DELETE FROM ahrefs_switch_logs WHERE switched_at < DATE_SUB(NOW(), INTERVAL 30 DAY)")
				_, _ = db.Exec("DELETE FROM ahrefs_violations_logs WHERE timestamp < DATE_SUB(NOW(), INTERVAL 30 DAY)")
			}
		}
	}()
}

// ── MAIN ──────────────────────────────────────────────────────────────────────

func main() {
	// Load config at startup
	currentConfig = defaultConfig
	startupCfg := loadConfig()

	// Initialize MySQL DB
	initDB(startupCfg)

	// Resolve dynamic website_id from database
	resolveWebsiteID(startupCfg.PublicHost)

	// Start daily reset cron
	startDailyResetCron()

	// Start expired session and token cleanup cron
	startCleanupCron()
	startCDNCacheSweep()

	// DB-only mode: cookies and user-agent are loaded exclusively from the database.
	// cookie.txt / proxy.txt / user-agent files are ignored.
	cachedCookieStr = "" // Not used — all credentials come from ahrefs_accounts table
	log.Printf("[OK] DB-only mode: All cookies, user-agents and proxies loaded from database accounts. ✅")

	mux := http.NewServeMux()
	// aMemberPro server-to-server handshake endpoint
	mux.HandleFunc("/api/device-bind", handleAhrefsDeviceBind)
	mux.HandleFunc("/tm-device-sw.js", serveAhrefsDeviceSW)
	mux.HandleFunc("/api/auth-handshake", authHandshakeHandler)
	mux.HandleFunc("/api/user-limits", userLimitsAPIHandler)
	mux.HandleFunc("/api/rotate-session", rotateSessionHandler)
	mux.HandleFunc("/api/account-switch", handleAhrefsAccountSwitch)
	mux.HandleFunc("/", proxyHandler)

	addr := ":" + startupCfg.Port
	log.Printf("╔══════════════════════════════════════════════════╗")
	log.Printf("║  🚀 Ahrefs Go Proxy — Chrome TLS Fingerprint      ║")
	log.Printf("║  http://localhost%s                               ║", addr)
	log.Printf("║  Target  → %s                  ║", startupCfg.TargetURL)
	log.Printf("║  User-Agent → ...%s║", startupCfg.UserAgent[len(startupCfg.UserAgent)-20:])
	log.Printf("║  Config: %s (hot-reload enabled ✅)         ║", CONFIG_FILE)
	if px := getProxy(); px != nil {
		user := ""
		if px.User != nil {
			user = px.User.Username() + ":****@"
		}
		log.Printf("║  Proxy: %s://%s%s ✅", px.Scheme, user, px.Host)
	} else {
		log.Printf("║  Proxy: disabled (add to proxy.txt to enable) ║")
	}
	log.Printf("╚══════════════════════════════════════════════════╝")

	// Wrap mux so a panic becomes 500 instead of killing the process (nginx 502).
	var handler http.Handler = mux
	handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[PANIC] %s %s: %v", r.Method, r.URL.RequestURI(), rec)
				http.Error(w, "Internal error", http.StatusInternalServerError)
			}
		}()
		mux.ServeHTTP(w, r)
	})

	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		// Ahrefs via SOCKS can exceed 90s on first dashboard HTML; short WriteTimeout
		// closes the conn mid-response → nginx shows its own 502 page.
		ReadTimeout:  180 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  180 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("[FATAL] %v", err)
	}
}
