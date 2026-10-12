package main

import (
	"database/sql"
	"os"
	"strings"
)

// ensureScaleIndexes keeps list and cleanup queries on the index when log
// tables grow. Duplicate-index errors are ignored so a restart is safe.
func ensureScaleIndexes(db *sql.DB) {
	mysql := strings.TrimSpace(os.Getenv("GT4RENTS_MYSQL_PASSWORD")) != ""
	stmts := []string{
		`CREATE INDEX idx_ingest_logs_account ON ingest_logs (account_id, id)`,
		`CREATE INDEX idx_accounts_website ON accounts (website_id)`,
		`CREATE INDEX idx_panel_users_website ON panel_users (website_id)`,
		`CREATE INDEX idx_user_meters_user ON user_meters (user_id)`,
	}
	if mysql {
		stmts = append(stmts,
			`CREATE INDEX idx_live_sessions_expires ON live_sessions (website_id, expires_at(32))`,
			`CREATE INDEX idx_violations_created ON violations (created_at(32))`,
			`CREATE INDEX idx_violations_site ON violations (website_id, created_at(32))`,
			`CREATE INDEX idx_security_created ON security_events (created_at(32))`,
			`CREATE INDEX idx_security_site ON security_events (website_id, created_at(32))`,
			`CREATE INDEX idx_security_user ON security_events (username(64), created_at(32))`,
			`CREATE INDEX idx_host_reports_site ON host_reports (website_id, created_at(32))`,
			`CREATE INDEX idx_host_reports_user ON host_reports (username(64), created_at(32))`,
			`CREATE INDEX idx_usage_reset ON usage_events (reset_days, timestamp(32))`,
			`CREATE INDEX idx_usage_lookup ON usage_events (website_id, username(64), limit_key(64), timestamp(32))`,
			`CREATE INDEX idx_login_user ON login_events (username(64), website_id, logged_in_at(32))`,
			`CREATE INDEX idx_ingest_created ON ingest_logs (created_at(32))`,
			`CREATE INDEX idx_extension_user_time ON extension_events (username(64), created_at(32))`,
			`CREATE INDEX idx_login_time ON login_events (logged_in_at(32))`,
			`CREATE INDEX idx_switch_time ON switch_events (switched_at(32))`,
			`CREATE INDEX idx_logout_time ON logout_events (created_at(32))`,
			`CREATE INDEX idx_extension_time ON extension_events (created_at(32))`,
			`CREATE INDEX idx_host_reports_time ON host_reports (created_at(32))`,
			`CREATE INDEX idx_telegram_time ON telegram_deliveries (created_at(32))`,
		)
	} else {
		stmts = append(stmts,
			`CREATE INDEX idx_live_sessions_expires ON live_sessions (website_id, expires_at)`,
			`CREATE INDEX idx_violations_created ON violations (created_at)`,
			`CREATE INDEX idx_violations_site ON violations (website_id, created_at)`,
			`CREATE INDEX idx_security_created ON security_events (created_at)`,
			`CREATE INDEX idx_security_site ON security_events (website_id, created_at)`,
			`CREATE INDEX idx_security_user ON security_events (username, created_at)`,
			`CREATE INDEX idx_host_reports_site ON host_reports (website_id, created_at)`,
			`CREATE INDEX idx_host_reports_user ON host_reports (username, created_at)`,
			`CREATE INDEX idx_usage_reset ON usage_events (reset_days, timestamp)`,
			`CREATE INDEX idx_usage_lookup ON usage_events (website_id, username, limit_key, timestamp)`,
			`CREATE INDEX idx_login_user ON login_events (username, website_id, logged_in_at)`,
			`CREATE INDEX idx_ingest_created ON ingest_logs (created_at)`,
			`CREATE INDEX idx_extension_user_time ON extension_events (username, created_at)`,
			`CREATE INDEX idx_login_time ON login_events (logged_in_at)`,
			`CREATE INDEX idx_switch_time ON switch_events (switched_at)`,
			`CREATE INDEX idx_logout_time ON logout_events (created_at)`,
			`CREATE INDEX idx_extension_time ON extension_events (created_at)`,
			`CREATE INDEX idx_host_reports_time ON host_reports (created_at)`,
			`CREATE INDEX idx_telegram_time ON telegram_deliveries (created_at)`,
		)
	}
	for _, stmt := range stmts {
		_, _ = db.Exec(stmt)
	}
}
