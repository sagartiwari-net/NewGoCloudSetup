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
	ensureWriteHuman(db)
	ensureWriteHumanLimits(db)
	ensurePictory(db)
	ensureResellers(db)
	ensureToolWalySites(db)
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
	ensureWriteHuman(db)
	ensureWriteHumanLimits(db)
	ensurePictory(db)
	ensureResellers(db)
	ensureToolWalySites(db)
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
	ensureRecloudColumns(db)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS account_cookie_links (
		follower_account_id INTEGER PRIMARY KEY,
		source_account_id INTEGER NOT NULL,
		created_at TEXT NOT NULL
	)`)
	ensureAutomationTables(db)
}

// ensureRecloudColumns marks recloud websites apart from mapped accounts.
// upstream_url is the live origin a recloud proxy reads on each request.
func ensureRecloudColumns(db *sql.DB) {
	_, _ = db.Exec(`ALTER TABLE websites ADD COLUMN kind VARCHAR(32) NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE websites ADD COLUMN upstream_url VARCHAR(2048) NOT NULL DEFAULT ''`)
}

func ensureTelegramInbox(db *sql.DB) {
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS telegram_inbox (
		chat_id VARCHAR(64) PRIMARY KEY,
		username TEXT NOT NULL,
		display_name TEXT NOT NULL,
		last_text TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`)
	_, _ = db.Exec(`ALTER TABLE telegram_inbox ADD COLUMN user_id VARCHAR(64) NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE telegram_deliveries ADD COLUMN error_text TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE telegram_deliveries ADD COLUMN reason TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`INSERT INTO settings ("key", value) VALUES ('switch_template', ?) ON DUPLICATE KEY UPDATE value=value`,
		"{tool} {account} {problem}")
	_, _ = db.Exec(`INSERT INTO settings ("key", value) VALUES ('network_template', ?) ON DUPLICATE KEY UPDATE value=value`,
		"{tool} {username} {type} detected")
	_, _ = db.Exec(`UPDATE settings SET value=? WHERE "key"='switch_template' AND value=?`,
		"{tool} {account} {problem}", "{tool}: {username} switched from {from} to {to} on {website} at {time}. {reason}")
	_, _ = db.Exec(`UPDATE settings SET value=? WHERE "key"='logout_template' AND value IN (?, ?)`,
		"{tool} {account} {problem}",
		"{tool} logged out for {username} on {website} at {time}.",
		"{tool}: {from} logged out. Switched {username} to {to} on {website} at {time}. {reason}")
	_, _ = db.Exec(`UPDATE settings SET value=? WHERE "key"='network_template' AND value=?`,
		"{tool} {username} {type} detected", "{username} is using {type} on {tool} ({website}). IP {ip}. {org}. {location}. {time}")
	_, _ = db.Exec(`UPDATE settings SET value=? WHERE "key"='spam_template' AND value=?`,
		"{tool} {username} Cookie sharing detected", "{username} may be sharing {tool}. {count} opens in the window from {ip}. {reason} at {time}.")
	ensureTelegramRouteEvents(db)
}

func ensureTelegramRouteEvents(db *sql.DB) {
	rows, err := db.Query(`SELECT id, events_json FROM telegram_routes`)
	if err != nil {
		return
	}
	type routeUpdate struct {
		id  int
		raw string
	}
	var updates []routeUpdate
	for rows.Next() {
		var id int
		var raw string
		if rows.Scan(&id, &raw) != nil {
			continue
		}
		var events []string
		if json.Unmarshal([]byte(raw), &events) != nil {
			continue
		}
		seen := map[string]bool{}
		for _, event := range events {
			seen[event] = true
		}
		changed := false
		for _, event := range []string{"switch", "network"} {
			if !seen[event] {
				events = append(events, event)
				changed = true
			}
		}
		if !changed {
			continue
		}
		next, err := json.Marshal(events)
		if err != nil {
			continue
		}
		updates = append(updates, routeUpdate{id: id, raw: string(next)})
	}
	rows.Close()
	for _, item := range updates {
		_, _ = db.Exec(`UPDATE telegram_routes SET events_json=? WHERE id=?`, item.raw, item.id)
	}
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
			defaults[limit.Key] = defaultSeedLimit(limit.Key)
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
