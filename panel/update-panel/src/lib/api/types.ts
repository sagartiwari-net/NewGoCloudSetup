export type Role = "master" | "reseller"

export type Operator = {
  id: number
  username: string
  role: Role
  website_ids: number[]
}

export type ToolLimitDef = {
  key: string
  label: string
  reset_days: number
}

export type Tool = {
  id: number
  name: string
  category: string
  limits: ToolLimitDef[]
}

export type Website = {
  id: number
  tool_id: number
  name: string
  domain: string
  secret_key: string
  session_duration: number
  default_limits: Record<string, number>
  session_security_enabled: boolean
}

export type UserAgentProfile = {
  id: number
  name: string
  user_agent: string
  created_at: string
}

export type ProxyEndpoint = {
  id: number
  name: string
  proxy_type: "SOCKS5" | "HTTP" | "HTTPS"
  endpoint: string
  status: "active" | "inactive"
  created_at: string
}

export type MappedAccount = {
  id: number
  website_id: number
  website_name: string
  name: string
  cookie: string
  user_agent_id: number | null
  user_agent: string
  proxy_id: number | null
  proxy: string
  status: "active" | "inactive"
  last_used_at: string
  failure_count: number
  show_limit: boolean
  description: string
  automation_task_uid: string
  automation_ingest_key: string
  cookie_updated_at: string
}

export type UserLimitMeter = {
  key: string
  label: string
  reset_days: number
  used: number
  limit: number
  is_custom: boolean
}

export type PanelUser = {
  id: number
  website_id: number
  website_name: string
  tool_name: string
  username: string
  status: "active" | "suspended"
  custom_limit_expire_at: string | null
  limit_visibility: "" | "show" | "hide"
  meters: UserLimitMeter[]
}

export type LiveSession = {
  id: number
  website_id: number
  website_name: string
  session_token: string
  username: string
  client_ip: string
  expires_at: string
  created_at: string
  assigned_account_id: number
  assigned_account_name: string
}

export type UsageEvent = {
  id: number
  website_id: number
  website_name: string
  tool_name: string
  username: string
  limit_key: string
  limit_label: string
  reset_days: number
  action: string
  target_path: string
  amount: number
  timestamp: string
}

export type LoginEvent = {
  id: number
  website_id: number
  domain: string
  username: string
  client_ip: string
  user_agent: string
  logged_in_at: string
}

export type LogoutEvent = {
  id: number
  website_id: number
  domain: string
  username: string
  account_name: string
  next_account_name: string
  reason: string
  client_ip: string
  created_at: string
}

export type SwitchEvent = {
  id: number
  website_id: number
  domain: string
  username: string
  from_account_name: string
  to_account_name: string
  reason: string
  switched_at: string
}

export type ExtensionEvent = {
  id: number
  website_id: number
  domain: string
  tool_name: string
  username: string
  tool_key: string
  source: string
  action: string
  target_path: string
  page_url: string
  asin: string
  marketplace: string
  query_text: string
  status_code: number
  client_ip: string
  user_agent: string
  created_at: string
}

export type ProductMap = {
  id: number
  website_id: number
  website_name: string
  product_ids: string[]
  product_name: string
}

export type Violation = {
  id: number
  website_id: number
  website_name: string
  username: string
  client_ip: string
  reason: string
  created_at: string
}

export type SecurityEvent = {
  id: number
  website_id: number
  website_name: string
  username: string
  client_ip: string
  event_type: string
  attempted_url: string
  details: string
  user_agent: string
  created_at: string
}

export type TelegramEvent = "logout" | "spam"

export type TelegramDestination = {
  id: number
  reseller_id: number
  reseller_name: string
  label: string
  chat_id: string
  enabled: boolean
}

export type TelegramRoute = {
  id: number
  website_id: number
  website_name: string
  events: TelegramEvent[]
  destination_ids: number[]
}

export type TelegramDelivery = {
  id: number
  website_id?: number
  reseller_names: string[]
  website_name: string
  website_domain: string
  username: string
  event: TelegramEvent
  recipients: Array<{ label: string; chat_id: string }>
  status: "sent" | "skipped"
  created_at: string
}

export type TelegramTemplates = {
  logout: string
  spam: string
}

export type AccessAlerts = {
  same_tool_count: number
  same_tool_minutes: number
  multi_tool_count: number
  multi_tool_minutes: number
  repeat_minutes: number
  repeat_hours: number
  repeat_min_opens: number
}

export type SpamRule = {
  window_minutes: number
  max_opens: number
  max_distinct_ips: number
}

export type SpamReport = {
  id: number
  website_id: number
  username: string
  tool_name: string
  reason: string
  created_at: string
  chat_labels: string[]
}

export type HostReport = {
  id: number
  website_id: number
  website_name: string
  username: string
  client_ip: string
  ip_type: "residential" | "hosting" | "proxy" | "vpn"
  org: string
  location: string
  created_at: string
  blocked: boolean
}

export type BlockedIP = {
  id: number
  website_id: number
  website_name: string
  client_ip: string
  reason: string
  created_at: string
}

export type IngestLog = {
  id: number
  website_id: number
  account_id: number
  ingest_key: string
  account_name: string
  website_name: string
  status: "saved" | "failed"
  error_message: string
  bytes_saved: number
  client_ip: string
  created_at: string
}

export type DashboardStats = {
  active_sessions: number
  accounts_total: number
  credit_hits_today: number
  active_users_list: Array<{
    username: string
    website_id?: number
    website_name: string
    client_ip: string
    created_at: string
    expires_at: string
    assigned_account_name: string
  }>
  credit_hits_7d: Array<{ day: string; hits: number }>
  attention: {
    cookies: {
      count: number
      items: Array<{ id: number; name: string; health: "fresh" | "stale" | "failed" | "never" }>
    }
    limits: {
      count: number
      items: Array<{ username: string; tool_name: string; used: number; limit: number }>
    }
    expiring: {
      count: number
      items: Array<{ username: string; tool_name: string; expires_on: string }>
    }
  }
}

export type AnalyticsSummary = {
  days: number
  counts: {
    logins: number
    unique_users: number
    extension: number
    switches: number
  }
  series: Array<{ day: string; logins: number; extension: number; switches: number }>
  top_users: Array<{ username: string; logins: number; extension: number; total: number }>
  top_actions: Array<{ name: string; count: number }>
  tools: Array<{ tool_key: string; tool_name: string; count: number }>
  switch_reasons: Array<{ name: string; count: number }>
}

export type UserReportWebsite = {
  id: number
  domain: string
  name: string
  tool_name: string
}

export type DirectoryUser = {
  username: string
  status: string
  website_count: number
  website_ids: number[]
  website_names: string[]
  tool_names: string[]
  primary_website_id: number
  login_count: number
  distinct_ips: number
  last_login_at: string
}

export type UserReportIP = {
  client_ip: string
  ip_type: string
  org: string
  location: string
  login_count: number
  last_seen_at: string
  domains: string
  blocked: boolean
}

export type UserReport = {
  user: PanelUser | null
  websites: UserReportWebsite[]
  meters: UserLimitMeter[]
  ips: UserReportIP[]
  summary: {
    login_count: number
    switch_count: number
    extension_count: number
    usage_hit_count: number
    last_login_at: string
    active_session: LiveSession | null
    distinct_ips: number
  }
}

export type UserReportSecurityRow = {
  kind: "violation" | "security"
  id: number
  website_id: number
  website_name: string
  username: string
  client_ip: string
  reason: string
  event_type: string
  attempted_url: string
  details: string
  user_agent: string
  created_at: string
}

export type IpReportUser = {
  username: string
  login_count: number
  last_login_at: string
  website_names: string[]
  website_ids: number[]
  source?: string
}

export type IpReport = {
  ip: string
  classification: {
    ip_type: string
    type_label: string
    org: string
    location: string
    classified_at: string
    blocked: boolean
    lookup: {
      country?: string
      region?: string
      city?: string
      isp?: string
      org?: string
      as?: string
      asname?: string
      proxy?: boolean
      hosting?: boolean
      mobile?: boolean
      type_label?: string
    }
  }
  summary: {
    login_count: number
    distinct_users: number
    user_count: number
    first_seen_at: string
    last_seen_at: string
  }
  users: IpReportUser[]
  recent_logins: Array<{
    id: number
    website_id: number
    domain: string
    website_name: string
    username: string
    user_agent: string
    logged_in_at: string
  }>
  host_reports: Array<{
    id: number
    website_id: number
    website_name: string
    username: string
    ip_type: string
    org: string
    location: string
    created_at: string
  }>
}
