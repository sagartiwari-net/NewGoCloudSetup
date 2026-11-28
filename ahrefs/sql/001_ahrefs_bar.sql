-- ToolsMandi Ahrefs Bar — Phase 1 schema (MySQL)
-- Apply on same MySQL as panel (gt4rents) or dedicated DB.
-- All tables prefixed ahrefs_bar_* so they never collide with tool meters.

SET NAMES utf8mb4;
SET time_zone = '+05:30';

CREATE TABLE IF NOT EXISTS ahrefs_bar_reseller_settings (
  reseller_id      INT NOT NULL COMMENT 'operators.id (owner may use 0 or own id)',
  hourly_limit     INT NOT NULL DEFAULT 50,
  daily_limit      INT NOT NULL DEFAULT 150,
  cache_ttl_hours  INT NOT NULL DEFAULT 72,
  sticky_minutes   INT NOT NULL DEFAULT 20,
  refund_on_5xx    TINYINT(1) NOT NULL DEFAULT 1,
  updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (reseller_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ahrefs_bar_portals (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  reseller_id   INT NOT NULL,
  name          VARCHAR(128) NOT NULL DEFAULT '',
  site_url      VARCHAR(512) NOT NULL,
  cookie_name   VARCHAR(128) NOT NULL DEFAULT 'PHPSESSID',
  check_url     VARCHAR(512) NOT NULL DEFAULT '' COMMENT 'tm-check.php or empty for local Am_Lite',
  product_ids   VARCHAR(255) NOT NULL DEFAULT '' COMMENT 'comma-separated aMember product ids',
  shared_secret VARCHAR(128) NOT NULL DEFAULT '' COMMENT 'X-TM-Check-Key for remote tm-check',
  enabled       TINYINT(1) NOT NULL DEFAULT 1,
  created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_ab_portals_reseller (reseller_id, enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ahrefs_bar_accounts (
  id                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  reseller_id        INT NOT NULL,
  name               VARCHAR(128) NOT NULL,
  access_token_json  MEDIUMTEXT NOT NULL COMMENT 'Ahrefs token JSON / export blob — never send to client',
  status             VARCHAR(32) NOT NULL DEFAULT 'active' COMMENT 'active|bad|disabled|cooldown',
  last_error         VARCHAR(512) NOT NULL DEFAULT '',
  cooldown_until     DATETIME NULL,
  last_used_at       DATETIME NULL,
  created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_ab_accounts_pool (reseller_id, status, last_used_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ahrefs_bar_sessions (
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ahrefs_bar_user_limits (
  id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  reseller_id    INT NOT NULL,
  amember_login  VARCHAR(255) NOT NULL,
  hourly_limit   INT NOT NULL,
  daily_limit    INT NOT NULL,
  is_custom      TINYINT(1) NOT NULL DEFAULT 1,
  updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uq_ab_user_limits (reseller_id, amember_login)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ahrefs_bar_usage_hourly (
  reseller_id    INT NOT NULL,
  amember_login  VARCHAR(255) NOT NULL,
  bucket_start   DATETIME NOT NULL COMMENT 'UTC or IST bucket start for rolling/clock hour',
  used           INT NOT NULL DEFAULT 0,
  PRIMARY KEY (reseller_id, amember_login, bucket_start)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ahrefs_bar_usage_daily (
  reseller_id    INT NOT NULL,
  amember_login  VARCHAR(255) NOT NULL,
  day_ist        DATE NOT NULL,
  used           INT NOT NULL DEFAULT 0,
  PRIMARY KEY (reseller_id, amember_login, day_ist)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ahrefs_bar_usage_log (
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ahrefs_bar_endpoint_costs (
  endpoint   VARCHAR(128) NOT NULL,
  credits    INT NOT NULL DEFAULT 1,
  class      VARCHAR(32) NOT NULL DEFAULT 'billable' COMMENT 'free|cheap|billable',
  PRIMARY KEY (endpoint)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ahrefs_bar_server_cache (
  cache_key   VARCHAR(191) NOT NULL,
  body        MEDIUMBLOB NOT NULL,
  status_code INT NOT NULL DEFAULT 200,
  expires_at  DATETIME NOT NULL,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (cache_key),
  KEY idx_ab_server_cache_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Default endpoint cost map (tunable later from panel)
INSERT INTO ahrefs_bar_endpoint_costs (endpoint, credits, class) VALUES
  ('tbGetUserInfoV3', 0, 'free'),
  ('tbGetStarterPlanPrice', 0, 'free'),
  ('tbGetHeaderV3', 0, 'cheap'),
  ('tbGetIconV3', 0, 'cheap'),
  ('tbGetKeywordStatsV3', 1, 'billable')
ON DUPLICATE KEY UPDATE credits=VALUES(credits), class=VALUES(class);

-- Owner defaults (reseller_id = 1 commonly; adjust after install)
INSERT INTO ahrefs_bar_reseller_settings (reseller_id, hourly_limit, daily_limit, cache_ttl_hours, sticky_minutes)
VALUES (1, 50, 150, 72, 20)
ON DUPLICATE KEY UPDATE hourly_limit=VALUES(hourly_limit);
