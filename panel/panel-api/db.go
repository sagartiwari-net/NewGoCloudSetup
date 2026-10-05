package main

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaFS embed.FS

func openDB(path string) *sql.DB {
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
		_, _ = db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO NOTHING`, pair[0], pair[1])
	}
}

func settingInt(db *sql.DB, key string, fallback int) int {
	var raw string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&raw); err != nil {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return fallback
	}
	return n
}

func saveSettingValue(db *sql.DB, key string, value int) {
	_, _ = db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, strconv.Itoa(value))
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
	_, _ = db.Exec(`INSERT INTO settings (key, value) VALUES ('bot_token', ''), ('repeat_minutes', '60'),
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
