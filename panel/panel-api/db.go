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

	"golang.org/x/crypto/bcrypt"

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
	_, _ = db.Exec(`ALTER TABLE panel_users ADD COLUMN limit_visibility TEXT NOT NULL DEFAULT ''`)
	ensureUserMeterColumns(db)
	ensureCatalogOwners(db)
	ensureAccessHandoff(db)
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
	ensureWordHero(db)
	ensurePictory(db)
	ensureVeo3(db)
	ensureResellers(db)
	ensureToolWalySites(db)
	ensureT4ToolzAhrefs(db)
	ensureT4ToolzCgptClud(db)
	ensurePrideSeoToolsAhrefs(db)
	ensureSeoGroupBuyExtraSites(db)
	ensureAmzPremiumSoftwareSites(db)
	ensureAmzPremiumExtraSellerAmp(db)
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
	ensureUserMeterColumns(db)
	ensureCatalogOwners(db)
	ensureAccessHandoff(db)
	seed(db)
	ensureQuillbot(db)
	ensureWriteHuman(db)
	ensureWriteHumanLimits(db)
	ensureWordHero(db)
	ensurePictory(db)
	ensureVeo3(db)
	ensureResellers(db)
	ensureToolWalySites(db)
	ensureT4ToolzAhrefs(db)
	ensureT4ToolzCgptClud(db)
	ensurePrideSeoToolsAhrefs(db)
	ensureSeoGroupBuyExtraSites(db)
	ensureAmzPremiumSoftwareSites(db)
	ensureAmzPremiumExtraSellerAmp(db)
	ensureAccessAlertSettings(db)
	return db
}

func ensureUserMeterColumns(db *sql.DB) {
	// Daily/weekly limit reset stores the IST period window here. Live MySQL was
	// missing it (SQLite open path used to ADD it; openMySQL did not).
	if _, err := db.Exec(`ALTER TABLE user_meters ADD COLUMN period_start VARCHAR(64) NOT NULL DEFAULT ''`); err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "duplicate column") && !strings.Contains(msg, "exists") {
			log.Printf("[DB] user_meters.period_start: %v", err)
		}
	} else {
		log.Printf("[DB] added user_meters.period_start for limit resets")
	}
	ensureUserMetersUnique(db)
	ensurePanelUsersUnique(db)
	ensureCollapseOrphanDownloadMeters(db)
}

// ensurePanelUsersUnique merges duplicate (website_id, username) rows then adds a
// unique index. Duplicate rows made custom Ahrefs limits lose to auto default rows.
func ensurePanelUsersUnique(db *sql.DB) {
	rows, err := db.Query(`SELECT website_id, username, COUNT(*), MIN(id) FROM panel_users GROUP BY website_id, username HAVING COUNT(*) > 1`)
	if err != nil {
		log.Printf("[DB] panel_users dedupe query: %v", err)
	} else {
		type dup struct {
			websiteID, keepID int
			username          string
		}
		var list []dup
		for rows.Next() {
			var d dup
			var n int
			if rows.Scan(&d.websiteID, &d.username, &n, &d.keepID) != nil {
				continue
			}
			list = append(list, d)
		}
		rows.Close()
		for _, d := range list {
			keepID := d.keepID
			// Prefer a custom/expiry/show row over the auto-created default shell.
			var preferred int
			_ = db.QueryRow(`SELECT u.id FROM panel_users u
				WHERE u.website_id=? AND u.username=?
				ORDER BY
					CASE WHEN COALESCE(u.custom_limit_expire_at,'') <> '' THEN 1 ELSE 0 END DESC,
					CASE WHEN EXISTS (SELECT 1 FROM user_meters m WHERE m.user_id=u.id AND m.is_custom=1) THEN 1 ELSE 0 END DESC,
					CASE WHEN LOWER(COALESCE(u.limit_visibility,''))='show' THEN 1 ELSE 0 END DESC,
					u.id DESC
				LIMIT 1`, d.websiteID, d.username).Scan(&preferred)
			if preferred > 0 {
				keepID = preferred
			}
			// Max used + best custom limit per key across all duplicate user ids.
			type meterMerge struct {
				used, limit, custom, reset int
				label                      string
			}
			merged := map[string]meterMerge{}
			mRows, mErr := db.Query(`SELECT m."key", m.used, m."limit", m.is_custom, m.label, m.reset_days
				FROM user_meters m JOIN panel_users u ON u.id=m.user_id
				WHERE u.website_id=? AND u.username=?`, d.websiteID, d.username)
			if mErr == nil {
				for mRows.Next() {
					var key, label string
					var used, limit, custom, reset int
					if mRows.Scan(&key, &used, &limit, &custom, &label, &reset) != nil {
						continue
					}
					cur, ok := merged[key]
					if !ok {
						merged[key] = meterMerge{used: used, limit: limit, custom: custom, reset: reset, label: label}
						continue
					}
					if used > cur.used {
						cur.used = used
					}
					if custom == 1 && (cur.custom == 0 || keepID > 0) {
						cur.limit, cur.custom, cur.label, cur.reset = limit, 1, label, reset
					}
					if cur.label == "" {
						cur.label = label
					}
					if cur.reset == 0 {
						cur.reset = reset
					}
					merged[key] = cur
				}
				mRows.Close()
			}
			// Prefer limits already on the kept (custom) row when both sides are custom.
			keepRows, kErr := db.Query(`SELECT "key", used, "limit", is_custom, label, reset_days FROM user_meters WHERE user_id=?`, keepID)
			if kErr == nil {
				for keepRows.Next() {
					var key, label string
					var used, limit, custom, reset int
					if keepRows.Scan(&key, &used, &limit, &custom, &label, &reset) != nil {
						continue
					}
					cur := merged[key]
					if used > cur.used {
						cur.used = used
					}
					if custom == 1 {
						cur.limit, cur.custom, cur.label, cur.reset = limit, 1, label, reset
					}
					merged[key] = cur
				}
				keepRows.Close()
			}
			_, _ = db.Exec(`DELETE FROM user_meters WHERE user_id IN (SELECT id FROM panel_users WHERE website_id=? AND username=?)`, d.websiteID, d.username)
			for key, m := range merged {
				if m.label == "" {
					m.label = key
				}
				if m.reset <= 0 {
					m.reset = 1
				}
				_, _ = db.Exec(`INSERT INTO user_meters (user_id, "key", label, reset_days, used, "limit", is_custom) VALUES (?,?,?,?,?,?,?)`,
					keepID, key, m.label, m.reset, m.used, m.limit, m.custom)
			}
			res, delErr := db.Exec(`DELETE FROM panel_users WHERE website_id=? AND username=? AND id<>?`, d.websiteID, d.username, keepID)
			removed := int64(0)
			if delErr == nil && res != nil {
				removed, _ = res.RowsAffected()
			}
			log.Printf("[DB] panel_users deduped site=%d user=%s keep_id=%d removed=%d", d.websiteID, d.username, keepID, removed)
		}
	}
	mysql := strings.TrimSpace(os.Getenv("GT4RENTS_MYSQL_PASSWORD")) != ""
	stmt := `CREATE UNIQUE INDEX idx_panel_users_site_user ON panel_users (website_id, username)`
	if mysql {
		stmt = `CREATE UNIQUE INDEX idx_panel_users_site_user ON panel_users (website_id, username(64))`
	}
	if _, err := db.Exec(stmt); err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "duplicate") && !strings.Contains(msg, "exists") && !strings.Contains(msg, "already") {
			log.Printf("[DB] panel_users unique index: %v", err)
		}
	}
}

// ensureCollapseOrphanDownloadMeters merges leftover Envato-style meters
// (Downloads / download / download_limit) into the tool's current download
// definition only. Ahrefs credits/exports and other keys are left alone.
func ensureCollapseOrphanDownloadMeters(db *sql.DB) {
	rows, err := db.Query(`SELECT w.id, COALESCE(t.limits_json, '[]'), COALESCE(w.default_limits_json, '{}')
		FROM websites w JOIN tools t ON t.id=w.tool_id`)
	if err != nil {
		return
	}
	type siteDef struct {
		id      int
		key     string
		label   string
		reset   int
		limit   int
		hasDown bool
	}
	type limitDefJSON struct {
		Key       string `json:"key"`
		Label     string `json:"label"`
		ResetDays int    `json:"reset_days"`
	}
	var sites []siteDef
	for rows.Next() {
		var id int
		var toolRaw, siteRaw string
		if rows.Scan(&id, &toolRaw, &siteRaw) != nil {
			continue
		}
		var defs []limitDefJSON
		_ = json.Unmarshal([]byte(toolRaw), &defs)
		defaults := map[string]int{}
		_ = json.Unmarshal([]byte(siteRaw), &defaults)
		pick := limitDefJSON{}
		for _, d := range defs {
			k := strings.ToLower(strings.TrimSpace(d.Key))
			l := strings.ToLower(strings.TrimSpace(d.Label))
			if strings.Contains(k, "download") || strings.Contains(l, "download") {
				pick = d
				break
			}
		}
		if pick.Key == "" {
			continue
		}
		limit := -1
		if n, ok := defaults[pick.Key]; ok {
			limit = n
		}
		reset := pick.ResetDays
		if reset < 1 {
			reset = 1
		}
		label := strings.TrimSpace(pick.Label)
		if label == "" {
			label = "Download Limit"
		}
		sites = append(sites, siteDef{id: id, key: pick.Key, label: label, reset: reset, limit: limit, hasDown: true})
	}
	rows.Close()

	aliases := map[string]bool{"download": true, "downloads": true, "download_limit": true}
	for _, site := range sites {
		if !site.hasDown {
			continue
		}
		users, err := db.Query(`SELECT id FROM panel_users WHERE website_id=?`, site.id)
		if err != nil {
			continue
		}
		var userIDs []int
		for users.Next() {
			var uid int
			if users.Scan(&uid) == nil {
				userIDs = append(userIDs, uid)
			}
		}
		users.Close()
		for _, uid := range userIDs {
			meters, mErr := db.Query(`SELECT id, "key", label, used, "limit", is_custom FROM user_meters WHERE user_id=?`, uid)
			if mErr != nil {
				continue
			}
			type mrow struct {
				id, used, limit, custom int
				key, label              string
			}
			var list []mrow
			for meters.Next() {
				var m mrow
				if meters.Scan(&m.id, &m.key, &m.label, &m.used, &m.limit, &m.custom) == nil {
					list = append(list, m)
				}
			}
			meters.Close()
			keepID := 0
			keepUsed := 0
			keepCustom := 0
			var drop []int
			for _, m := range list {
				lk := strings.ToLower(m.key)
				ll := strings.ToLower(m.label)
				isDown := aliases[lk] || strings.Contains(ll, "download")
				if m.key == site.key {
					if keepID == 0 || m.id < keepID {
						if keepID > 0 {
							drop = append(drop, keepID)
						}
						keepID = m.id
						keepUsed = m.used
						keepCustom = m.custom
					} else {
						drop = append(drop, m.id)
						if m.used > keepUsed {
							keepUsed = m.used
						}
					}
					continue
				}
				if !isDown {
					continue
				}
				// Orphan download-shaped meter not in the tool catalog for this site.
				drop = append(drop, m.id)
				if m.used > keepUsed {
					keepUsed = m.used
				}
			}
			if keepID == 0 && len(drop) > 0 {
				limit := site.limit
				if limit < -1 {
					limit = -1
				}
				res, insErr := db.Exec(`INSERT INTO user_meters (user_id, "key", label, reset_days, used, "limit", is_custom) VALUES (?,?,?,?,?,?,0)`,
					uid, site.key, site.label, site.reset, keepUsed, limit)
				if insErr == nil {
					id, _ := res.LastInsertId()
					keepID = int(id)
				}
			}
			if keepID > 0 {
				if keepCustom == 1 {
					_, _ = db.Exec(`UPDATE user_meters SET used=?, label=?, reset_days=? WHERE id=?`, keepUsed, site.label, site.reset, keepID)
				} else {
					_, _ = db.Exec(`UPDATE user_meters SET used=?, label=?, reset_days=?, "limit"=? WHERE id=?`, keepUsed, site.label, site.reset, site.limit, keepID)
				}
			}
			for _, id := range drop {
				if id == keepID {
					continue
				}
				_, _ = db.Exec(`DELETE FROM user_meters WHERE id=?`, id)
			}
		}
	}
}

// ensureUserMetersUnique collapses duplicate (user_id, key) rows then adds a
// unique index so Envato-style "Download Limit 3/2 · … ×6" cannot come back.
func ensureUserMetersUnique(db *sql.DB) {
	rows, err := db.Query(`SELECT user_id, "key", COUNT(*), MAX(used), MIN(id) FROM user_meters GROUP BY user_id, "key" HAVING COUNT(*) > 1`)
	if err != nil {
		log.Printf("[DB] user_meters dedupe query: %v", err)
	} else {
		type dup struct {
			userID, maxUsed, keepID int
			key                     string
		}
		var list []dup
		for rows.Next() {
			var d dup
			var n int
			if rows.Scan(&d.userID, &d.key, &n, &d.maxUsed, &d.keepID) != nil {
				continue
			}
			list = append(list, d)
		}
		rows.Close()
		for _, d := range list {
			_, _ = db.Exec(`UPDATE user_meters SET used=? WHERE id=?`, d.maxUsed, d.keepID)
			res, delErr := db.Exec(`DELETE FROM user_meters WHERE user_id=? AND "key"=? AND id<>?`, d.userID, d.key, d.keepID)
			if delErr != nil {
				log.Printf("[DB] user_meters dedupe delete user=%d key=%s: %v", d.userID, d.key, delErr)
				continue
			}
			removed, _ := res.RowsAffected()
			if removed > 0 {
				log.Printf("[DB] user_meters deduped user=%d key=%s removed=%d keep_id=%d used=%d", d.userID, d.key, removed, d.keepID, d.maxUsed)
			}
		}
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX idx_user_meters_user_key ON user_meters (user_id, "key")`); err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "duplicate") && !strings.Contains(msg, "exists") && !strings.Contains(msg, "already") {
			log.Printf("[DB] user_meters unique index: %v", err)
		}
	}
}

func ensureCatalogOwners(db *sql.DB) {
	_, _ = db.Exec(`ALTER TABLE proxies ADD COLUMN operator_id INTEGER NOT NULL DEFAULT 0`)
	_, _ = db.Exec(`ALTER TABLE user_agents ADD COLUMN operator_id INTEGER NOT NULL DEFAULT 0`)
}

func hashPassword(password string) string {
	sum, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return ""
	}
	return string(sum)
}

func hashPasswordSHA(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

func verifyPassword(stored, password string) bool {
	if strings.HasPrefix(stored, "$2") {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(password)) == nil
	}
	return stored != "" && stored == hashPasswordSHA(password)
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
