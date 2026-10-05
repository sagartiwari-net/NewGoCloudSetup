CREATE TABLE IF NOT EXISTS operators (
  id INTEGER PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active'
);

CREATE TABLE IF NOT EXISTS admin_sessions (
  token TEXT PRIMARY KEY,
  operator_id INTEGER NOT NULL,
  expires_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS operator_websites (
  operator_id INTEGER NOT NULL,
  website_id INTEGER NOT NULL,
  PRIMARY KEY (operator_id, website_id)
);

CREATE TABLE IF NOT EXISTS tools (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  category TEXT NOT NULL,
  limits_json TEXT NOT NULL DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS websites (
  id INTEGER PRIMARY KEY,
  tool_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  domain TEXT NOT NULL,
  secret_key TEXT NOT NULL,
  session_duration INTEGER NOT NULL DEFAULT 120,
  default_limits_json TEXT NOT NULL DEFAULT '{}',
  session_security_enabled INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS proxies (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  proxy_type TEXT NOT NULL,
  endpoint TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS user_agents (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  user_agent TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS accounts (
  id INTEGER PRIMARY KEY,
  website_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  cookie TEXT NOT NULL DEFAULT '',
  user_agent_id INTEGER,
  user_agent TEXT NOT NULL DEFAULT '',
  proxy_id INTEGER,
  proxy TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  last_used_at TEXT NOT NULL DEFAULT '',
  failure_count INTEGER NOT NULL DEFAULT 0,
  show_limit INTEGER NOT NULL DEFAULT 1,
  description TEXT NOT NULL DEFAULT '',
  automation_task_uid TEXT NOT NULL DEFAULT '',
  automation_ingest_key TEXT NOT NULL DEFAULT '',
  cookie_updated_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS panel_users (
  id INTEGER PRIMARY KEY,
  website_id INTEGER NOT NULL,
  username TEXT NOT NULL,
  status TEXT NOT NULL,
  custom_limit_expire_at TEXT,
  limit_visibility TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS user_meters (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL,
  key TEXT NOT NULL,
  label TEXT NOT NULL,
  reset_days INTEGER NOT NULL,
  used INTEGER NOT NULL DEFAULT 0,
  "limit" INTEGER NOT NULL,
  is_custom INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS live_sessions (
  id INTEGER PRIMARY KEY,
  website_id INTEGER NOT NULL,
  session_token TEXT NOT NULL,
  username TEXT NOT NULL,
  client_ip TEXT NOT NULL DEFAULT '',
  fingerprint TEXT NOT NULL DEFAULT '',
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  assigned_account_id INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS usage_events (
  id INTEGER PRIMARY KEY,
  website_id INTEGER NOT NULL,
  username TEXT NOT NULL,
  limit_key TEXT NOT NULL,
  limit_label TEXT NOT NULL,
  reset_days INTEGER NOT NULL,
  action TEXT NOT NULL,
  target_path TEXT NOT NULL DEFAULT '',
  amount INTEGER NOT NULL,
  timestamp TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS extension_events (
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
);

CREATE INDEX IF NOT EXISTS idx_extension_events_created ON extension_events(created_at);
CREATE INDEX IF NOT EXISTS idx_extension_events_website ON extension_events(website_id, created_at);
CREATE INDEX IF NOT EXISTS idx_extension_events_user ON extension_events(username, created_at);

CREATE TABLE IF NOT EXISTS login_events (
  id INTEGER PRIMARY KEY,
  website_id INTEGER NOT NULL,
  username TEXT NOT NULL,
  client_ip TEXT NOT NULL,
  user_agent TEXT NOT NULL DEFAULT '',
  logged_in_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS switch_events (
  id INTEGER PRIMARY KEY,
  website_id INTEGER NOT NULL,
  username TEXT NOT NULL,
  from_account_name TEXT NOT NULL DEFAULT '',
  to_account_name TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL DEFAULT '',
  switched_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS logout_events (
  id INTEGER PRIMARY KEY,
  website_id INTEGER NOT NULL,
  username TEXT NOT NULL DEFAULT '',
  account_name TEXT NOT NULL DEFAULT '',
  next_account_name TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL DEFAULT '',
  client_ip TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS products (
  id INTEGER PRIMARY KEY,
  website_id INTEGER NOT NULL,
  product_name TEXT NOT NULL,
  product_ids_json TEXT NOT NULL DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS violations (
  id INTEGER PRIMARY KEY,
  website_id INTEGER NOT NULL,
  username TEXT NOT NULL,
  client_ip TEXT NOT NULL,
  reason TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS security_events (
  id INTEGER PRIMARY KEY,
  website_id INTEGER NOT NULL,
  username TEXT NOT NULL DEFAULT '',
  client_ip TEXT NOT NULL DEFAULT '',
  event_type TEXT NOT NULL,
  attempted_url TEXT NOT NULL DEFAULT '',
  details TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS blocked_ips (
  id INTEGER PRIMARY KEY,
  website_id INTEGER NOT NULL DEFAULT 0,
  client_ip TEXT NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS ingest_logs (
  id INTEGER PRIMARY KEY,
  website_id INTEGER NOT NULL,
  account_id INTEGER NOT NULL,
  ingest_key TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  error_message TEXT NOT NULL DEFAULT '',
  bytes_saved INTEGER NOT NULL DEFAULT 0,
  client_ip TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS telegram_destinations (
  id INTEGER PRIMARY KEY,
  reseller_id INTEGER NOT NULL,
  label TEXT NOT NULL,
  chat_id TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS telegram_routes (
  id INTEGER PRIMARY KEY,
  website_id INTEGER NOT NULL UNIQUE,
  events_json TEXT NOT NULL DEFAULT '["logout","spam"]'
);

CREATE TABLE IF NOT EXISTS telegram_route_chats (
  route_id INTEGER NOT NULL,
  destination_id INTEGER NOT NULL,
  PRIMARY KEY (route_id, destination_id)
);

CREATE TABLE IF NOT EXISTS telegram_deliveries (
  id INTEGER PRIMARY KEY,
  website_id INTEGER NOT NULL,
  username TEXT NOT NULL,
  event TEXT NOT NULL,
  recipients_json TEXT NOT NULL DEFAULT '[]',
  status TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS host_reports (
  id INTEGER PRIMARY KEY,
  website_id INTEGER NOT NULL,
  username TEXT NOT NULL,
  client_ip TEXT NOT NULL,
  ip_type TEXT NOT NULL,
  org TEXT NOT NULL DEFAULT '',
  location TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
