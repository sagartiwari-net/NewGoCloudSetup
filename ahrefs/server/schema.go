package main

import "database/sql"

func schemaStatements(driver string) []string {
	if driver == "mysql" {
		return mysqlSchema
	}
	return sqliteSchema
}

var sqliteSchema = []string{
	`CREATE TABLE IF NOT EXISTS ahrefs_bar_reseller_settings (
  reseller_id      INTEGER PRIMARY KEY,
  hourly_limit     INTEGER NOT NULL DEFAULT 50,
  daily_limit      INTEGER NOT NULL DEFAULT 150,
  cache_ttl_hours  INTEGER NOT NULL DEFAULT 72,
  sticky_minutes   INTEGER NOT NULL DEFAULT 20,
  refund_on_5xx    INTEGER NOT NULL DEFAULT 1,
  updated_at       TEXT NOT NULL DEFAULT (datetime('now'))
)`,
	`CREATE TABLE IF NOT EXISTS ahrefs_bar_portals (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  reseller_id   INTEGER NOT NULL,
  name          TEXT NOT NULL DEFAULT '',
  site_url      TEXT NOT NULL,
  cookie_name   TEXT NOT NULL DEFAULT 'PHPSESSID',
  check_url     TEXT NOT NULL DEFAULT '',
  product_ids   TEXT NOT NULL DEFAULT '',
  shared_secret TEXT NOT NULL DEFAULT '',
  enabled       INTEGER NOT NULL DEFAULT 1,
  created_at    TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
)`,
	`CREATE INDEX IF NOT EXISTS idx_ab_portals_reseller ON ahrefs_bar_portals (reseller_id, enabled)`,
	`CREATE TABLE IF NOT EXISTS ahrefs_bar_accounts (
  id                 INTEGER PRIMARY KEY AUTOINCREMENT,
  reseller_id        INTEGER NOT NULL,
  name               TEXT NOT NULL,
  access_token_json  TEXT NOT NULL,
  status             TEXT NOT NULL DEFAULT 'active',
  last_error         TEXT NOT NULL DEFAULT '',
  cooldown_until     TEXT NULL,
  last_used_at       TEXT NULL,
  created_at         TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at         TEXT NOT NULL DEFAULT (datetime('now'))
)`,
	`CREATE INDEX IF NOT EXISTS idx_ab_accounts_pool ON ahrefs_bar_accounts (reseller_id, status, last_used_at)`,
	`CREATE TABLE IF NOT EXISTS ahrefs_bar_sessions (
  token               TEXT PRIMARY KEY,
  reseller_id         INTEGER NOT NULL,
  portal_id           INTEGER NULL,
  amember_login       TEXT NOT NULL,
  email               TEXT NOT NULL DEFAULT '',
  name                TEXT NOT NULL DEFAULT '',
  uid                 INTEGER NOT NULL DEFAULT 0,
  portal_url          TEXT NOT NULL DEFAULT '',
  fingerprint         TEXT NOT NULL DEFAULT '',
  expires_at          TEXT NOT NULL,
  last_seen           TEXT NOT NULL,
  sticky_account_id   INTEGER NULL,
  sticky_until        TEXT NULL,
  created_at          TEXT NOT NULL DEFAULT (datetime('now'))
)`,
	`CREATE INDEX IF NOT EXISTS idx_ab_sessions_reseller_user ON ahrefs_bar_sessions (reseller_id, amember_login)`,
	`CREATE INDEX IF NOT EXISTS idx_ab_sessions_expires ON ahrefs_bar_sessions (expires_at)`,
	`CREATE TABLE IF NOT EXISTS ahrefs_bar_user_limits (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  reseller_id    INTEGER NOT NULL,
  amember_login  TEXT NOT NULL,
  hourly_limit   INTEGER NOT NULL,
  daily_limit    INTEGER NOT NULL,
  is_custom      INTEGER NOT NULL DEFAULT 1,
  updated_at     TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE (reseller_id, amember_login)
)`,
	`CREATE TABLE IF NOT EXISTS ahrefs_bar_usage_hourly (
  reseller_id    INTEGER NOT NULL,
  amember_login  TEXT NOT NULL,
  bucket_start   TEXT NOT NULL,
  used           INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (reseller_id, amember_login, bucket_start)
)`,
	`CREATE TABLE IF NOT EXISTS ahrefs_bar_usage_daily (
  reseller_id    INTEGER NOT NULL,
  amember_login  TEXT NOT NULL,
  day_ist        TEXT NOT NULL,
  used           INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (reseller_id, amember_login, day_ist)
)`,
	`CREATE TABLE IF NOT EXISTS ahrefs_bar_usage_log (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  reseller_id    INTEGER NOT NULL,
  amember_login  TEXT NOT NULL DEFAULT '',
  account_id     INTEGER NULL,
  endpoint       TEXT NOT NULL DEFAULT '',
  target         TEXT NOT NULL DEFAULT '',
  credits        INTEGER NOT NULL DEFAULT 0,
  cache_hit      INTEGER NOT NULL DEFAULT 0,
  status_code    INTEGER NOT NULL DEFAULT 0,
  created_at     TEXT NOT NULL DEFAULT (datetime('now'))
)`,
	`CREATE INDEX IF NOT EXISTS idx_ab_usage_log_reseller_time ON ahrefs_bar_usage_log (reseller_id, created_at)`,
	`CREATE TABLE IF NOT EXISTS ahrefs_bar_endpoint_costs (
  endpoint   TEXT PRIMARY KEY,
  credits    INTEGER NOT NULL DEFAULT 1,
  class      TEXT NOT NULL DEFAULT 'billable'
)`,
	`CREATE TABLE IF NOT EXISTS ahrefs_bar_server_cache (
  cache_key   TEXT PRIMARY KEY,
  body        BLOB NOT NULL,
  status_code INTEGER NOT NULL DEFAULT 200,
  expires_at  TEXT NOT NULL,
  created_at  TEXT NOT NULL DEFAULT (datetime('now'))
)`,
	`CREATE INDEX IF NOT EXISTS idx_ab_server_cache_expires ON ahrefs_bar_server_cache (expires_at)`,
}

// mysqlSchema mirrors sql/001_ahrefs_bar.sql for auto-migrate when AHREFS_BAR_DSN is set.
var mysqlSchema = []string{
	`CREATE TABLE IF NOT EXISTS ahrefs_bar_reseller_settings (
  reseller_id      INT NOT NULL,
  hourly_limit     INT NOT NULL DEFAULT 50,
  daily_limit      INT NOT NULL DEFAULT 150,
  cache_ttl_hours  INT NOT NULL DEFAULT 72,
  sticky_minutes   INT NOT NULL DEFAULT 20,
  refund_on_5xx    TINYINT(1) NOT NULL DEFAULT 1,
  updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (reseller_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS ahrefs_bar_portals (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  reseller_id   INT NOT NULL,
  name          VARCHAR(128) NOT NULL DEFAULT '',
  site_url      VARCHAR(512) NOT NULL,
  cookie_name   VARCHAR(128) NOT NULL DEFAULT 'PHPSESSID',
  check_url     VARCHAR(512) NOT NULL DEFAULT '',
  product_ids   VARCHAR(255) NOT NULL DEFAULT '',
  shared_secret VARCHAR(128) NOT NULL DEFAULT '',
  enabled       TINYINT(1) NOT NULL DEFAULT 1,
  created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_ab_portals_reseller (reseller_id, enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS ahrefs_bar_accounts (
  id                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  reseller_id        INT NOT NULL,
  name               VARCHAR(128) NOT NULL,
  access_token_json  MEDIUMTEXT NOT NULL,
  status             VARCHAR(32) NOT NULL DEFAULT 'active',
  last_error         VARCHAR(512) NOT NULL DEFAULT '',
  cooldown_until     DATETIME NULL,
  last_used_at       DATETIME NULL,
  created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_ab_accounts_pool (reseller_id, status, last_used_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS ahrefs_bar_sessions (
  token          VARCHAR(128) NOT NULL,
  reseller_id    INT NOT NULL,
  portal_id      BIGINT UNSIGNED NULL,
  amember_login  VARCHAR(255) NOT NULL,
  email          VARCHAR(255) NOT NULL DEFAULT '',
  name           VARCHAR(255) NOT NULL DEFAULT '',
  uid            BIGINT NOT NULL DEFAULT 0,
  portal_url     VARCHAR(512) NOT NULL DEFAULT '',
  fingerprint    VARCHAR(512) NOT NULL DEFAULT '',
  expires_at     DATETIME NOT NULL,
  last_seen      DATETIME NOT NULL,
  sticky_account_id BIGINT UNSIGNED NULL,
  sticky_until   DATETIME NULL,
  created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (token),
  KEY idx_ab_sessions_reseller_user (reseller_id, amember_login),
  KEY idx_ab_sessions_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS ahrefs_bar_user_limits (
  id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  reseller_id    INT NOT NULL,
  amember_login  VARCHAR(255) NOT NULL,
  hourly_limit   INT NOT NULL,
  daily_limit    INT NOT NULL,
  is_custom      TINYINT(1) NOT NULL DEFAULT 1,
  updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uq_ab_user_limits (reseller_id, amember_login)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS ahrefs_bar_usage_hourly (
  reseller_id    INT NOT NULL,
  amember_login  VARCHAR(255) NOT NULL,
  bucket_start   DATETIME NOT NULL,
  used           INT NOT NULL DEFAULT 0,
  PRIMARY KEY (reseller_id, amember_login, bucket_start)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS ahrefs_bar_usage_daily (
  reseller_id    INT NOT NULL,
  amember_login  VARCHAR(255) NOT NULL,
  day_ist        DATE NOT NULL,
  used           INT NOT NULL DEFAULT 0,
  PRIMARY KEY (reseller_id, amember_login, day_ist)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS ahrefs_bar_usage_log (
  id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  reseller_id    INT NOT NULL,
  amember_login  VARCHAR(255) NOT NULL DEFAULT '',
  account_id     BIGINT UNSIGNED NULL,
  endpoint       VARCHAR(128) NOT NULL DEFAULT '',
  target         VARCHAR(512) NOT NULL DEFAULT '',
  credits        INT NOT NULL DEFAULT 0,
  cache_hit      TINYINT(1) NOT NULL DEFAULT 0,
  status_code    INT NOT NULL DEFAULT 0,
  created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_ab_usage_log_reseller_time (reseller_id, created_at),
  KEY idx_ab_usage_log_user_time (reseller_id, amember_login, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS ahrefs_bar_endpoint_costs (
  endpoint   VARCHAR(128) NOT NULL,
  credits    INT NOT NULL DEFAULT 1,
  class      VARCHAR(32) NOT NULL DEFAULT 'billable',
  PRIMARY KEY (endpoint)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS ahrefs_bar_server_cache (
  cache_key   VARCHAR(191) NOT NULL,
  body        MEDIUMBLOB NOT NULL,
  status_code INT NOT NULL DEFAULT 200,
  expires_at  DATETIME NOT NULL,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (cache_key),
  KEY idx_ab_server_cache_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
}

func seedEndpointCosts(db *sql.DB, driver string) error {
	rows := []struct {
		endpoint string
		credits  int
		class    string
	}{
		{"tbGetUserInfoV3", 0, "free"},
		{"tbGetStarterPlanPrice", 0, "free"},
		{"tbGetHeaderV3", 0, "cheap"},
		{"tbGetIconV3", 0, "cheap"},
		{"tbGetKeywordStatsV3", 1, "billable"},
	}
	for _, r := range rows {
		var err error
		if driver == "mysql" {
			_, err = db.Exec(`
INSERT INTO ahrefs_bar_endpoint_costs (endpoint, credits, class) VALUES (?, ?, ?)
ON DUPLICATE KEY UPDATE credits=VALUES(credits), class=VALUES(class)`, r.endpoint, r.credits, r.class)
		} else {
			_, err = db.Exec(`
INSERT INTO ahrefs_bar_endpoint_costs (endpoint, credits, class) VALUES (?, ?, ?)
ON CONFLICT(endpoint) DO UPDATE SET credits=excluded.credits, class=excluded.class`, r.endpoint, r.credits, r.class)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func seedOwnerDefaults(db *sql.DB, driver string) error {
	if driver == "mysql" {
		_, err := db.Exec(`
INSERT INTO ahrefs_bar_reseller_settings (reseller_id, hourly_limit, daily_limit, cache_ttl_hours, sticky_minutes)
VALUES (1, 50, 150, 72, 20)
ON DUPLICATE KEY UPDATE reseller_id=reseller_id`)
		return err
	}
	_, err := db.Exec(`
INSERT INTO ahrefs_bar_reseller_settings (reseller_id, hourly_limit, daily_limit, cache_ttl_hours, sticky_minutes)
VALUES (1, 50, 150, 72, 20)
ON CONFLICT(reseller_id) DO NOTHING`)
	return err
}
