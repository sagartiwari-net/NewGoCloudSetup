package main

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaFS embed.FS

func openDB(path string) *sql.DB {
	// Server always sets GT4RENTS_MYSQL_PASSWORD and uses MySQL gt4rents.
	// The sqlite path is only a local Mac fallback. panel.db is a frozen backup on the server.
	if strings.TrimSpace(os.Getenv("GT4RENTS_MYSQL_PASSWORD")) != "" {
		return openMySQL()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(20000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	body, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(string(body)); err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS access_tokens (
		token TEXT PRIMARY KEY,
		website_id INTEGER NOT NULL,
		username TEXT NOT NULL,
		product_id TEXT NOT NULL,
		client_ip TEXT NOT NULL DEFAULT '',
		expires_at TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`); err != nil {
		log.Fatal(err)
	}
	_, _ = db.Exec(`ALTER TABLE access_tokens ADD COLUMN client_ip TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE user_meters ADD COLUMN period_start TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE panel_users ADD COLUMN limit_visibility TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS extension_events (
		id INTEGER PRIMARY KEY,
		website_id INTEGER NOT NULL,
		username TEXT NOT NULL,
		tool_key TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'extension',
		action TEXT NOT NULL,
		target_path TEXT NOT NULL DEFAULT '',
		page_url TEXT NOT NULL DEFAULT '',
		asin TEXT NOT NULL DEFAULT '',
		marketplace TEXT NOT NULL DEFAULT '',
		query_text TEXT NOT NULL DEFAULT '',
		status_code INTEGER NOT NULL DEFAULT 0,
		client_ip TEXT NOT NULL DEFAULT '',
		user_agent TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_extension_events_created ON extension_events(created_at)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_extension_events_website ON extension_events(website_id, created_at)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_extension_events_user ON extension_events(username, created_at)`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS logout_events (
		id INTEGER PRIMARY KEY,
		website_id INTEGER NOT NULL,
		username TEXT NOT NULL DEFAULT '',
		account_name TEXT NOT NULL DEFAULT '',
		next_account_name TEXT NOT NULL DEFAULT '',
		reason TEXT NOT NULL DEFAULT '',
		client_ip TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_logout_events_created ON logout_events(created_at)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_logout_events_website ON logout_events(website_id, created_at)`)
	seed(db)
	ensureQuillbot(db)
	ensureResellers(db)
	ensureAccessAlertSettings(db)
	return db
}

// openMySQL uses the phpMyAdmin database gt4rents. panel.db is a frozen backup
// and is not opened. Tables already exist, so this does not run the SQLite schema.
func openMySQL() *sql.DB {
	host := strings.TrimSpace(os.Getenv("GT4RENTS_MYSQL_HOST"))
	if host == "" {
		host = "127.0.0.1"
	}
	port := strings.TrimSpace(os.Getenv("GT4RENTS_MYSQL_PORT"))
	if port == "" {
		port = "3306"
	}
	user := strings.TrimSpace(os.Getenv("GT4RENTS_MYSQL_USER"))
	if user == "" {
		user = "gt4rents"
	}
	name := strings.TrimSpace(os.Getenv("GT4RENTS_MYSQL_DB"))
	if name == "" {
		name = "gt4rents"
	}
	dsn := url.UserPassword(user, os.Getenv("GT4RENTS_MYSQL_PASSWORD")).String() +
		"@tcp(" + host + ":" + port + ")/" + name +
		"?charset=utf8mb4&parseTime=false&sql_mode=ANSI_QUOTES"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		log.Fatalf("[DB] MySQL ping failed: %v", err)
	}
	log.Printf("[DB] panel API using MySQL %s/%s", host, name)
	seed(db)
	ensureQuillbot(db)
	ensureResellers(db)
	ensureAccessAlertSettings(db)
	return db
}

func hashPassword(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

func ensureAccessAlertSettings(db *sql.DB) {
	defaults := [][2]string{
		{"access_same_count", "4"},
		{"access_same_minutes", "30"},
		{"access_multi_count", "3"},
		{"access_multi_minutes", "3"},
		{"access_repeat_minutes", "30"},
		{"access_repeat_hours", "3"},
		{"access_repeat_opens", "4"},
	}
	for _, pair := range defaults {
		_, _ = db.Exec(`INSERT INTO settings ("key", value) VALUES (?, ?) ON DUPLICATE KEY UPDATE value=value`, pair[0], pair[1])
	}
	ensureScaleIndexes(db)
	ensureTelegramInbox(db)
}

func ensureTelegramInbox(db *sql.DB) {
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS telegram_inbox (
		chat_id VARCHAR(64) PRIMARY KEY,
		username TEXT NOT NULL,
		display_name TEXT NOT NULL,
		last_text TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`)
	_, _ = db.Exec(`ALTER TABLE telegram_deliveries ADD COLUMN error_text TEXT NOT NULL DEFAULT ''`)
}

func settingInt(db *sql.DB, key string, fallback int) int {
	var raw string
	if err := db.QueryRow(`SELECT value FROM settings WHERE "key"=?`, key).Scan(&raw); err != nil {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return fallback
	}
	return n
}

func saveSettingValue(db *sql.DB, key string, value int) {
	_, _ = db.Exec(`INSERT INTO settings ("key", value) VALUES (?, ?) ON DUPLICATE KEY UPDATE value=VALUES(value)`, key, strconv.Itoa(value))
}

func seed(db *sql.DB) {
	var n int
	_ = db.QueryRow("SELECT COUNT(*) FROM websites").Scan(&n)
	if n > 0 {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = db.Exec(`INSERT INTO operators (username, password_hash, role, status) VALUES (?,?,?,?)`,
		"master", hashPassword("toolsmandi"), "master", "active")
	_, _ = db.Exec(`INSERT INTO settings ("key", value) VALUES ('bot_token', ''), ('repeat_minutes', '60'),
		('logout_template', '{tool}: {from} logged out. Switched {username} to {to} on {website} at {time}. {reason}'),
		('spam_template', '{username} may be sharing {tool}. {count} opens in the window from {ip}. {reason} at {time}.'),
		('spam_window', '10'), ('spam_opens', '8'), ('spam_ips', '2')`)

	for _, tool := range localTools {
		limits, _ := json.Marshal(tool.limits)
		res, err := db.Exec(`INSERT INTO tools (name, category, limits_json) VALUES (?,?,?)`, tool.name, tool.category, string(limits))
		if err != nil {
			log.Fatal(err)
		}
		toolID, _ := res.LastInsertId()
		defaults := map[string]int{}
		for _, limit := range tool.limits {
			if limit.Key == "exports" {
				defaults[limit.Key] = 20
			} else {
				defaults[limit.Key] = 100
			}
		}
		rawDefaults, _ := json.Marshal(defaults)
		domain := "127.0.0.1:" + tool.port
		_, err = db.Exec(`INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled)
			VALUES (?,?,?,?,30,?,1)`, toolID, tool.name, domain, "secret_fake_"+tool.slug, string(rawDefaults))
		if err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("seeded %d local tools at %s", len(localTools), now)
}
