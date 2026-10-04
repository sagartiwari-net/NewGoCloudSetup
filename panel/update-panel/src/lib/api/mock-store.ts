import { cookieHealth } from "@/lib/cookie-health"
import { dayKey, dayLabel } from "@/lib/format"
import { isOlderThan } from "@/lib/retention"
import type {
  BlockedIP,
  DashboardStats,
  ExtensionEvent,
  IngestLog,
  LiveSession,
  LoginEvent,
  MappedAccount,
  Operator,
  PanelUser,
  ProductMap,
  ProxyEndpoint,
  Role,
  SecurityEvent,
  SpamReport,
  SpamRule,
  SwitchEvent,
  TelegramDelivery,
  TelegramDestination,
  TelegramEvent,
  TelegramRoute,
  TelegramTemplates,
  Tool,
  ToolLimitDef,
  UsageEvent,
  UserAgentProfile,
  UserLimitMeter,
  Violation,
  Website,
} from "@/lib/api/types"

const USER_AGENT =
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36 ToolsMandi/1.0"

const CHROME_MAC =
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36"

function iso(daysAgo: number, hour = 9) {
  const date = new Date()
  date.setHours(hour, 12, 0, 0)
  date.setDate(date.getDate() - daysAgo)
  return date.toISOString()
}

function dateOnly(daysAhead: number) {
  const date = new Date()
  date.setDate(date.getDate() + daysAhead)
  const month = String(date.getMonth() + 1).padStart(2, "0")
  const day = String(date.getDate()).padStart(2, "0")
  return `${date.getFullYear()}-${month}-${day}`
}

const tools: Tool[] = [
  {
    id: 1,
    name: "Ahrefs",
    category: "SEO",
    limits: [
      { key: "credits", label: "Credits", reset_days: 1 },
      { key: "exports", label: "Exports", reset_days: 7 },
    ],
  },
  {
    id: 2,
    name: "Semrush",
    category: "SEO",
    limits: [
      { key: "credits", label: "Credits", reset_days: 1 },
      { key: "reports", label: "Reports", reset_days: 7 },
    ],
  },
  {
    id: 3,
    name: "SeoSite Checkup",
    category: "SEO",
    limits: [
      { key: "credits", label: "Credits", reset_days: 1 },
      { key: "exports", label: "Exports", reset_days: 7 },
    ],
  },
  {
    id: 4,
    name: "BuzzSumo",
    category: "Content",
    limits: [
      { key: "credits", label: "Credits", reset_days: 30 },
      { key: "exports", label: "Exports", reset_days: 7 },
    ],
  },
  {
    id: 5,
    name: "ChatGPT",
    category: "AI",
    limits: [{ key: "credits", label: "Credits", reset_days: 1 }],
  },
  { id: 6, name: "Canva", category: "Design", limits: [] },
]

const userAgents: UserAgentProfile[] = [
  { id: 1, name: "Chrome on Mac", user_agent: CHROME_MAC, created_at: iso(20) },
  { id: 2, name: "ToolsMandi", user_agent: USER_AGENT, created_at: iso(18) },
]

const proxies: ProxyEndpoint[] = [
  {
    id: 1,
    name: "US East",
    proxy_type: "SOCKS5",
    endpoint: "socks5://user:secret_fake_px01@10.1.0.4:1080",
    status: "active",
    created_at: iso(20),
  },
  {
    id: 2,
    name: "EU HTTP",
    proxy_type: "HTTP",
    endpoint: "http://user:secret_fake_px02@10.2.0.8:8080",
    status: "active",
    created_at: iso(14),
  },
  {
    id: 3,
    name: "Backup HTTPS",
    proxy_type: "HTTPS",
    endpoint: "https://10.3.0.2:443",
    status: "inactive",
    created_at: iso(6),
  },
]

const websites: Website[] = [
  {
    id: 1,
    tool_id: 1,
    name: "Ahrefs",
    domain: "ct.example.com",
    secret_key: "secret_fake_ahrefs0001",
    session_duration: 120,
    default_limits: { credits: 500, exports: 20 },
    session_security_enabled: true,
  },
  {
    id: 2,
    tool_id: 2,
    name: "Semrush",
    domain: "sm.example.com",
    secret_key: "secret_fake_semrush0002",
    session_duration: 90,
    default_limits: { credits: 400, reports: 15 },
    session_security_enabled: true,
  },
  {
    id: 3,
    tool_id: 3,
    name: "SeoSite Checkup",
    domain: "sc.example.com",
    secret_key: "secret_fake_seosite0003",
    session_duration: 60,
    default_limits: { credits: 200, exports: 10 },
    session_security_enabled: false,
  },
  {
    id: 4,
    tool_id: 4,
    name: "BuzzSumo",
    domain: "bz.example.com",
    secret_key: "secret_fake_buzz0004",
    session_duration: 180,
    default_limits: { credits: 300, exports: 12 },
    session_security_enabled: true,
  },
  {
    id: 5,
    tool_id: 5,
    name: "ChatGPT",
    domain: "cg.example.com",
    secret_key: "secret_fake_chatgpt0005",
    session_duration: 60,
    default_limits: { credits: 100 },
    session_security_enabled: true,
  },
  {
    id: 6,
    tool_id: 6,
    name: "Canva",
    domain: "cv.example.com",
    secret_key: "secret_fake_canva0006",
    session_duration: 90,
    default_limits: {},
    session_security_enabled: false,
  },
  { id: 7, tool_id: 1, name: "Ahrefs EU", domain: "ahrefs-eu.example.com", secret_key: "secret_fake_ahrefs0007", session_duration: 120, default_limits: { credits: 500, exports: 20 }, session_security_enabled: true },
  { id: 8, tool_id: 2, name: "Semrush Asia", domain: "sm-asia.example.com", secret_key: "secret_fake_semrush0008", session_duration: 90, default_limits: { credits: 400, reports: 15 }, session_security_enabled: true },
  { id: 9, tool_id: 3, name: "Checkup US", domain: "checkup-us.example.com", secret_key: "secret_fake_seosite0009", session_duration: 60, default_limits: { credits: 200, exports: 10 }, session_security_enabled: false },
  { id: 10, tool_id: 5, name: "ChatGPT Team", domain: "gpt-team.example.com", secret_key: "secret_fake_chatgpt0010", session_duration: 60, default_limits: { credits: 80 }, session_security_enabled: true },
  { id: 11, tool_id: 4, name: "BuzzSumo EU", domain: "buzz-eu.example.com", secret_key: "secret_fake_buzz0011", session_duration: 180, default_limits: { credits: 300, exports: 12 }, session_security_enabled: true },
  { id: 12, tool_id: 6, name: "Canva Teams", domain: "canva-teams.example.com", secret_key: "secret_fake_canva0012", session_duration: 90, default_limits: {}, session_security_enabled: false },
]

const operators: Operator[] = [
  { id: 1, username: "master", role: "master", website_ids: [1, 2, 3, 4, 5, 6] },
  { id: 2, username: "reseller", role: "reseller", website_ids: [1, 3, 5] },
  { id: 3, username: "partner", role: "reseller", website_ids: [2] },
]

const resellerStatus = new Map<number, "active" | "suspended">([
  [2, "active"],
  [3, "suspended"],
])

let botToken = "secret_fake_telegram0001"
let repeatMinutes = 60

let telegramTemplates: TelegramTemplates = {
  logout: "{tool} logged out for {username} on {website} at {time}.",
  spam: "{username} may be sharing {tool}. {count} opens in the window from {ip}. {reason} at {time}.",
}

let spamRule: SpamRule = {
  window_minutes: 10,
  max_opens: 8,
  max_distinct_ips: 2,
}

tools.push(
  { id: 7, name: "Envato", category: "Assets", limits: [] },
  { id: 8, name: "Ubersuggest", category: "SEO", limits: [] },
  { id: 9, name: "Helium10", category: "Amazon", limits: [] },
  { id: 10, name: "Claude AI", category: "AI", limits: [] },
)

const catalogSites: Array<[number, number, string, string]> = [
  [40, 1, "Ahrefs", "ahrefs.toolwaly.example"],
  [41, 2, "Semrush", "semrush.toolwaly.example"],
  [42, 7, "Envato", "envato.toolwaly.example"],
  [43, 5, "ChatGPT", "chatgpt.toolwaly.example"],
  [44, 1, "Ahrefs", "ahrefs.semrushtoolz.example"],
  [45, 2, "Semrush", "semrush.semrushtoolz.example"],
  [46, 5, "ChatGPT", "chatgpt.semrushtoolz.example"],
  [47, 7, "Envato", "envato.semrushtoolz.example"],
  [48, 8, "Ubersuggest", "ubersuggest.semrushtoolz.example"],
  [49, 9, "Helium10", "helium10.semrushtoolz.example"],
  [50, 2, "Semrush", "semrush.seogroupbuy.example"],
  [51, 9, "Helium10", "helium10.amzpremiumtoolz.example"],
  [52, 5, "ChatGPT", "chatgpt.amzpremiumtoolz.example"],
  [53, 10, "Claude AI", "claude.amzpremiumtoolz.example"],
]
for (const [id, toolId, name, domain] of catalogSites) {
  websites.push({
    id,
    tool_id: toolId,
    name,
    domain,
    secret_key: `secret_fake_${domain.replace(/\./g, "")}`,
    session_duration: 120,
    default_limits: {},
    session_security_enabled: true,
  })
}

operators.push(
  { id: 40, username: "ToolWaly", role: "reseller", website_ids: [40, 41, 42, 43] },
  { id: 41, username: "SemrushToolz", role: "reseller", website_ids: [44, 45, 46, 47, 48, 49] },
  { id: 42, username: "SeoGroupBuy", role: "reseller", website_ids: [50] },
  { id: 43, username: "AmzPremiumToolz", role: "reseller", website_ids: [51, 52, 53] },
)
for (const resellerId of [40, 41, 42, 43]) resellerStatus.set(resellerId, "active")

const telegramDestinations: TelegramDestination[] = [
  { id: 1, reseller_id: 40, reseller_name: "ToolWaly", label: "ToolWaly 1", chat_id: "-1002001", enabled: true },
  { id: 2, reseller_id: 40, reseller_name: "ToolWaly", label: "ToolWaly 2", chat_id: "-1002002", enabled: true },
  { id: 3, reseller_id: 40, reseller_name: "ToolWaly", label: "ToolWaly 3", chat_id: "-1002003", enabled: true },
  { id: 4, reseller_id: 41, reseller_name: "SemrushToolz", label: "SemrushToolz", chat_id: "-1002004", enabled: true },
  { id: 5, reseller_id: 42, reseller_name: "SeoGroupBuy", label: "SeoGroupBuy 1", chat_id: "-1002005", enabled: true },
  { id: 6, reseller_id: 42, reseller_name: "SeoGroupBuy", label: "SeoGroupBuy 2", chat_id: "-1002006", enabled: true },
  { id: 7, reseller_id: 43, reseller_name: "AmzPremiumToolz", label: "AmzPremiumToolz", chat_id: "-1002007", enabled: true },
]

const routeSeeds: Array<[number, string, number[]]> = [
  [40, "Ahrefs", [1, 2, 3]],
  [41, "Semrush", [1, 2, 3]],
  [42, "Envato", [1, 2, 3]],
  [43, "ChatGPT", [1, 2, 3]],
  [44, "Ahrefs", [4]],
  [45, "Semrush", [4, 5, 6]],
  [46, "ChatGPT", [4]],
  [47, "Envato", [4]],
  [48, "Ubersuggest", [4]],
  [49, "Helium10", [4]],
  [50, "Semrush", [5, 6]],
  [51, "Helium10", [7]],
  [52, "ChatGPT", [7]],
  [53, "Claude AI", [7]],
]
const telegramRoutes: TelegramRoute[] = routeSeeds.map(([website_id, website_name, destination_ids], index) => ({
  id: index + 1,
  website_id,
  website_name,
  events: ["logout", "spam"],
  destination_ids,
}))

const telegramDeliveries: TelegramDelivery[] = [
  {
    id: 1,
    reseller_names: ["ToolWaly"],
    website_name: "Ahrefs",
    website_domain: "ahrefs.toolwaly.example",
    username: "nina",
    event: "logout",
    recipients: [
      { label: "ToolWaly 1", chat_id: "-1002001" },
      { label: "ToolWaly 2", chat_id: "-1002002" },
    ],
    status: "sent",
    created_at: iso(0, 11),
  },
  {
    id: 2,
    reseller_names: ["ToolWaly"],
    website_name: "Ahrefs",
    website_domain: "ahrefs.toolwaly.example",
    username: "nina",
    event: "spam",
    recipients: [
      { label: "ToolWaly 1", chat_id: "-1002001" },
      { label: "ToolWaly 2", chat_id: "-1002002" },
      { label: "ToolWaly 3", chat_id: "-1002003" },
    ],
    status: "sent",
    created_at: iso(0, 9),
  },
  {
    id: 3,
    reseller_names: ["SemrushToolz", "SeoGroupBuy"],
    website_name: "Semrush",
    website_domain: "semrush.semrushtoolz.example",
    username: "omar",
    event: "logout",
    recipients: [
      { label: "SemrushToolz", chat_id: "-1002004" },
      { label: "SeoGroupBuy 1", chat_id: "-1002005" },
      { label: "SeoGroupBuy 2", chat_id: "-1002006" },
    ],
    status: "skipped",
    created_at: iso(1, 8),
  },
  {
    id: 4,
    reseller_names: ["SeoGroupBuy"],
    website_name: "Semrush",
    website_domain: "semrush.seogroupbuy.example",
    username: "leo",
    event: "logout",
    recipients: [
      { label: "SeoGroupBuy 1", chat_id: "-1002005" },
      { label: "SeoGroupBuy 2", chat_id: "-1002006" },
    ],
    status: "sent",
    created_at: iso(1, 6),
  },
  {
    id: 5,
    reseller_names: ["SemrushToolz"],
    website_name: "Helium10",
    website_domain: "helium10.semrushtoolz.example",
    username: "priya",
    event: "spam",
    recipients: [{ label: "SemrushToolz", chat_id: "-1002004" }],
    status: "sent",
    created_at: iso(2, 10),
  },
  {
    id: 6,
    reseller_names: ["AmzPremiumToolz"],
    website_name: "Claude AI",
    website_domain: "claude.amzpremiumtoolz.example",
    username: "ada",
    event: "logout",
    recipients: [{ label: "AmzPremiumToolz", chat_id: "-1002007" }],
    status: "sent",
    created_at: iso(2, 7),
  },
  {
    id: 7,
    reseller_names: ["ToolWaly"],
    website_name: "Envato",
    website_domain: "envato.toolwaly.example",
    username: "gita",
    event: "spam",
    recipients: [{ label: "ToolWaly 1", chat_id: "-1002001" }],
    status: "skipped",
    created_at: iso(3, 5),
  },
  {
    id: 8,
    reseller_names: ["SemrushToolz"],
    website_name: "ChatGPT",
    website_domain: "chatgpt.semrushtoolz.example",
    username: "ben",
    event: "logout",
    recipients: [{ label: "SemrushToolz", chat_id: "-1002004" }],
    status: "sent",
    created_at: iso(3, 4),
  },
  {
    id: 9,
    reseller_names: ["AmzPremiumToolz"],
    website_name: "Helium10",
    website_domain: "helium10.amzpremiumtoolz.example",
    username: "caleb",
    event: "spam",
    recipients: [{ label: "AmzPremiumToolz", chat_id: "-1002007" }],
    status: "sent",
    created_at: iso(4, 3),
  },
]

const spamReports: Array<Omit<SpamReport, "chat_labels">> = [
  {
    id: 1,
    website_id: 1,
    username: "nina",
    tool_name: "Ahrefs",
    reason: "8 opens from 2 IPs",
    created_at: iso(0, 10),
  },
  {
    id: 2,
    website_id: 2,
    username: "priya",
    tool_name: "Semrush",
    reason: "9 opens in 10 minutes",
    created_at: iso(1, 8),
  },
]

const accounts: MappedAccount[] = [
  {
    id: 1,
    website_id: 1,
    website_name: "Ahrefs",
    name: "Ahrefs Main",
    cookie: "cookie_fake_ahrefs_1111aaaa",
    user_agent_id: 1,
    user_agent: CHROME_MAC,
    proxy_id: 1,
    proxy: proxies[0].endpoint,
    status: "active",
    last_used_at: iso(0, 8),
    failure_count: 0,
    show_limit: true,
    description: "Morning Ahrefs seat for keyword overview and the Canada menu research queue",
    automation_task_uid: "",
    automation_ingest_key: "",
    cookie_updated_at: iso(0, 8),
  },
  {
    id: 2,
    website_id: 1,
    website_name: "Ahrefs",
    name: "Ahrefs Backup",
    cookie: "cookie_fake_ahrefs_2222bbbb",
    user_agent_id: 2,
    user_agent: USER_AGENT,
    proxy_id: 2,
    proxy: proxies[1].endpoint,
    status: "inactive",
    last_used_at: iso(2, 11),
    failure_count: 3,
    show_limit: false,
    description: "",
    automation_task_uid: "",
    automation_ingest_key: "",
    cookie_updated_at: iso(3, 11),
  },
  {
    id: 3,
    website_id: 2,
    website_name: "Semrush",
    name: "Semrush Primary",
    cookie: "cookie_fake_semrush_3333cccc",
    user_agent_id: 2,
    user_agent: USER_AGENT,
    proxy_id: 2,
    proxy: proxies[1].endpoint,
    status: "active",
    last_used_at: iso(0, 7),
    failure_count: 1,
    show_limit: true,
    description: "",
    automation_task_uid: "",
    automation_ingest_key: "",
    cookie_updated_at: iso(0, 7),
  },
  {
    id: 4,
    website_id: 3,
    website_name: "SeoSite Checkup",
    name: "SeoSite Primary",
    cookie: "cookie_fake_seosite_4444dddd",
    user_agent_id: 1,
    user_agent: CHROME_MAC,
    proxy_id: 1,
    proxy: proxies[0].endpoint,
    status: "active",
    last_used_at: iso(0, 6),
    failure_count: 0,
    show_limit: true,
    description: "Primary login for checkup runs",
    automation_task_uid: "gfx_runSeositecheckup",
    automation_ingest_key: "seosite_acc1",
    cookie_updated_at: iso(0, 6),
  },
  {
    id: 5,
    website_id: 4,
    website_name: "BuzzSumo",
    name: "Buzz Primary",
    cookie: "cookie_fake_buzzsumo_5555eeee",
    user_agent_id: null,
    user_agent: "Mozilla/5.0 (compatible; BuzzOneOff/1.0)",
    proxy_id: null,
    proxy: "",
    status: "active",
    last_used_at: iso(1, 15),
    failure_count: 2,
    show_limit: false,
    description: "BuzzSumo content account used by the morning batch",
    automation_task_uid: "gfx_runBuzzsumo",
    automation_ingest_key: "buzzsumo_acc1",
    cookie_updated_at: iso(2, 4),
  },
  {
    id: 6,
    website_id: 3,
    website_name: "SeoSite Checkup",
    name: "SeoSite Spare",
    cookie: "cookie_fake_seosite_6666ffff",
    user_agent_id: null,
    user_agent: "Mozilla/5.0 (compatible; SpareOneOff/1.0)",
    proxy_id: null,
    proxy: "socks5://10.9.0.5:1080",
    status: "active",
    last_used_at: iso(3, 10),
    failure_count: 0,
    show_limit: true,
    description: "",
    automation_task_uid: "",
    automation_ingest_key: "",
    cookie_updated_at: "",
  },
]

function meter(
  key: string,
  label: string,
  resetDays: number,
  used: number,
  limit: number,
  websiteDefault: number,
): UserLimitMeter {
  return { key, label, reset_days: resetDays, used, limit, is_custom: limit !== websiteDefault }
}

function seededUser(
  id: number,
  websiteId: number,
  username: string,
  status: PanelUser["status"],
  expire: string | null,
  meters: UserLimitMeter[],
): PanelUser {
  const website = websites.find((row) => row.id === websiteId)
  const tool = tools.find((row) => row.id === website?.tool_id)
  return {
    id,
    website_id: websiteId,
    website_name: website?.name ?? "",
    tool_name: tool?.name ?? "",
    username,
    status,
    custom_limit_expire_at: expire,
    limit_visibility: "",
    meters,
  }
}

const users: PanelUser[] = [
  seededUser(1, 1, "nina", "active", dateOnly(30), [
    meter("credits", "Credits", 1, 120, 250, 500),
    meter("exports", "Exports", 7, 4, 20, 20),
  ]),
  seededUser(2, 1, "nora", "active", null, [
    meter("credits", "Credits", 1, 10, 500, 500),
    meter("exports", "Exports", 7, 0, 20, 20),
  ]),
  seededUser(3, 3, "omar", "active", null, [
    meter("credits", "Credits", 1, 180, 200, 200),
    meter("exports", "Exports", 7, 9, 10, 10),
  ]),
  seededUser(4, 2, "priya", "suspended", dateOnly(-2), [
    meter("credits", "Credits", 1, 40, 400, 400),
    meter("reports", "Reports", 7, 1, 15, 15),
  ]),
  seededUser(5, 4, "leo", "active", null, [
    meter("credits", "Credits", 30, 16, 300, 300),
    meter("exports", "Exports", 7, 0, 12, 12),
  ]),
  seededUser(6, 5, "ada", "active", null, [meter("credits", "Credits", 1, 12, 100, 100)]),
  seededUser(7, 5, "ben", "active", null, [meter("credits", "Credits", 1, 5, 40, 100)]),
  seededUser(8, 1, "caleb", "active", null, [
    meter("credits", "Credits", 1, 2, 500, 500),
    meter("exports", "Exports", 7, 0, 20, 20),
  ]),
  seededUser(9, 2, "diana", "active", null, [
    meter("credits", "Credits", 1, 8, 400, 400),
    meter("reports", "Reports", 7, 2, 15, 15),
  ]),
  seededUser(10, 3, "elena", "active", null, [
    meter("credits", "Credits", 1, 20, 200, 200),
    meter("exports", "Exports", 7, 1, 10, 10),
  ]),
  seededUser(11, 4, "farah", "active", null, [
    meter("credits", "Credits", 30, 4, 300, 300),
    meter("exports", "Exports", 7, 1, 12, 12),
  ]),
  seededUser(12, 1, "gita", "active", dateOnly(14), [
    meter("credits", "Credits", 1, 6, 500, 500),
    meter("exports", "Exports", 7, 0, 20, 20),
  ]),
  seededUser(13, 2, "harun", "active", null, [
    meter("credits", "Credits", 1, 1, 400, 400),
    meter("reports", "Reports", 7, 0, 15, 15),
  ]),
  seededUser(14, 3, "ira", "active", null, [
    meter("credits", "Credits", 1, 3, 200, 200),
    meter("exports", "Exports", 7, 0, 10, 10),
  ]),
  seededUser(15, 6, "maya", "active", null, []),
  seededUser(16, 5, "nina", "active", null, [meter("credits", "Credits", 1, 3, 100, 100)]),
]

const sessions: LiveSession[] = [
  {
    id: 1,
    website_id: 1,
    website_name: "Ahrefs",
    session_token: "token_fake_session_a1",
    username: "nina",
    client_ip: "203.0.113.10",
    created_at: iso(0, 8),
    expires_at: iso(-1, 8),
    assigned_account_id: 1,
    assigned_account_name: "Ahrefs Main",
  },
  {
    id: 2,
    website_id: 3,
    website_name: "SeoSite Checkup",
    session_token: "token_fake_session_b2",
    username: "omar",
    client_ip: "198.51.100.24",
    created_at: iso(0, 7),
    expires_at: iso(0, 19),
    assigned_account_id: 0,
    assigned_account_name: "Auto",
  },
  {
    id: 3,
    website_id: 2,
    website_name: "Semrush",
    session_token: "token_fake_session_c3",
    username: "priya",
    client_ip: "192.0.2.55",
    created_at: iso(0, 6),
    expires_at: iso(0, 18),
    assigned_account_id: 3,
    assigned_account_name: "Semrush Primary",
  },
  {
    id: 4,
    website_id: 4,
    website_name: "BuzzSumo",
    session_token: "token_fake_session_d4",
    username: "leo",
    client_ip: "203.0.113.80",
    created_at: iso(0, 5),
    expires_at: iso(0, 20),
    assigned_account_id: 5,
    assigned_account_name: "Buzz Primary",
  },
  ...[
    ["ada", 5, "ChatGPT", "203.0.113.15"],
    ["ben", 5, "ChatGPT", "203.0.113.16"],
    ["caleb", 1, "Ahrefs", "198.51.100.30"],
    ["diana", 2, "Semrush", "192.0.2.61"],
    ["elena", 3, "SeoSite Checkup", "198.51.100.40"],
    ["maya", 6, "Canva", "203.0.113.90"],
  ].map((row, index) => ({
    id: 5 + index,
    website_id: Number(row[1]),
    website_name: String(row[2]),
    session_token: `token_fake_session_e${index}`,
    username: String(row[0]),
    client_ip: String(row[3]),
    created_at: iso(0, 4),
    expires_at: iso(0, 18),
    assigned_account_id: 0,
    assigned_account_name: "Auto",
  })),
]

const usage: UsageEvent[] = []
const loginEvents: LoginEvent[] = []
const extensionEvents: ExtensionEvent[] = [
  {
    id: 1,
    website_id: 1,
    domain: "hel.example.com",
    tool_name: "Helium10",
    username: "nina",
    tool_key: "helium10",
    source: "extension",
    action: "xray_keywords",
    target_path: "/research-tools/api/xray/keywords",
    page_url: "https://www.amazon.in/dp/B0EXAMPLE1",
    asin: "B0EXAMPLE1",
    marketplace: "amazon.in",
    query_text: "wireless earbuds",
    status_code: 200,
    client_ip: "203.0.113.10",
    user_agent: USER_AGENT,
    created_at: iso(0, 8),
  },
  {
    id: 2,
    website_id: 1,
    domain: "hel.example.com",
    tool_name: "Helium10",
    username: "caleb",
    tool_key: "helium10",
    source: "extension",
    action: "xray",
    target_path: "/research-tools/api/xray/overview",
    page_url: "https://www.amazon.com/dp/B0EXAMPLE2",
    asin: "B0EXAMPLE2",
    marketplace: "amazon.com",
    query_text: "",
    status_code: 200,
    client_ip: "198.51.100.22",
    user_agent: USER_AGENT,
    created_at: iso(1, 12),
  },
]
{
  const seeds: Array<Omit<UsageEvent, "id">> = [
    { website_id: 1, website_name: "Ahrefs", tool_name: "Ahrefs", username: "nina", limit_key: "credits", limit_label: "Credits", reset_days: 1, action: "Keyword overview", target_path: "Overview: menupricesincanada.com", amount: 1, timestamp: iso(0, 6) },
    { website_id: 1, website_name: "Ahrefs", tool_name: "Ahrefs", username: "nina", limit_key: "credits", limit_label: "Credits", reset_days: 1, action: "Keyword overview", target_path: "Organic Search: freeinvoicemaker.ca", amount: 1, timestamp: iso(1, 4) },
    { website_id: 1, website_name: "Ahrefs", tool_name: "Ahrefs", username: "nina", limit_key: "credits", limit_label: "Credits", reset_days: 1, action: "Keyword overview", target_path: "Site Explorer: example.org", amount: 1, timestamp: iso(2, 9) },
    { website_id: 1, website_name: "Ahrefs", tool_name: "Ahrefs", username: "nina", limit_key: "exports", limit_label: "Exports", reset_days: 7, action: "Batch export", target_path: "/v4/seGetOrganicKeywordsExport", amount: 1000, timestamp: iso(6, 2) },
    { website_id: 5, website_name: "ChatGPT", tool_name: "ChatGPT", username: "nina", limit_key: "credits", limit_label: "Credits", reset_days: 1, action: "Chat message", target_path: "Chat: draft.example.com", amount: 1, timestamp: iso(0, 11) },
  ]
  const extras = [
    ["omar", 3, "SeoSite Checkup", "SeoSite Checkup", "exports", "Exports", 7, "Batch export", "/export/report"],
    ["priya", 2, "Semrush", "Semrush", "credits", "Credits", 1, "Keyword overview", "Organic Research: shop.example.com"],
    ["leo", 4, "BuzzSumo", "BuzzSumo", "credits", "Credits", 30, "Content search", "Content: buzz.example.com"],
    ["ada", 5, "ChatGPT", "ChatGPT", "credits", "Credits", 1, "Chat message", "Chat: notes.example.com"],
    ["ben", 5, "ChatGPT", "ChatGPT", "credits", "Credits", 1, "Chat message", "Chat: brief.example.com"],
    ["caleb", 1, "Ahrefs", "Ahrefs", "credits", "Credits", 1, "Keyword overview", "Overview: cafe.example.com"],
    ["diana", 2, "Semrush", "Semrush", "reports", "Reports", 7, "Rank report", "/reports/rank"],
    ["elena", 3, "SeoSite Checkup", "SeoSite Checkup", "credits", "Credits", 1, "Site audit", "Audit: blog.example.com"],
    ["farah", 4, "BuzzSumo", "BuzzSumo", "exports", "Exports", 7, "Batch export", "/export/shares"],
    ["gita", 1, "Ahrefs", "Ahrefs", "credits", "Credits", 1, "Keyword overview", "Overview: news.example.com"],
    ["harun", 2, "Semrush", "Semrush", "credits", "Credits", 1, "Keyword overview", "Domain: tools.example.com"],
    ["ira", 3, "SeoSite Checkup", "SeoSite Checkup", "exports", "Exports", 7, "Batch export", "/export/issues"],
  ] as const
  extras.forEach((row, index) => {
    seeds.push({
      website_id: row[1],
      website_name: row[2],
      tool_name: row[3],
      username: row[0],
      limit_key: row[4],
      limit_label: row[5],
      reset_days: row[6],
      action: row[7],
      target_path: row[8],
      amount: 1,
      timestamp: iso(index % 5, 8),
    })
  })
  seeds.forEach((event, index) => {
    usage.push({ ...event, id: index + 1 })
  })
  const logins = [
    ["nina", 1, "ct.example.com", "203.0.113.10"],
    ["omar", 3, "sc.example.com", "198.51.100.24"],
    ["ada", 5, "cg.example.com", "203.0.113.15"],
  ] as const
  for (let day = 0; day < 7; day += 1) {
    logins.forEach((actor, index) => {
      if (index > day % 3) return
      loginEvents.push({
        id: loginEvents.length + 1,
        website_id: actor[1],
        domain: actor[2],
        username: actor[0],
        client_ip: actor[3],
        user_agent: USER_AGENT,
        logged_in_at: iso(day, 7 + index),
      })
    })
  }
}

const switches: SwitchEvent[] = [
  {
    id: 1,
    website_id: 1,
    domain: "ct.example.com",
    username: "nina",
    from_account_name: "Ahrefs Backup",
    to_account_name: "Ahrefs Main",
    reason: "failure_threshold",
    switched_at: iso(1, 11),
  },
  {
    id: 2,
    website_id: 3,
    domain: "sc.example.com",
    username: "omar",
    from_account_name: "SeoSite Spare",
    to_account_name: "SeoSite Primary",
    reason: "manual_reassign",
    switched_at: iso(0, 9),
  },
  {
    id: 3,
    website_id: 4,
    domain: "bz.example.com",
    username: "leo",
    from_account_name: "Auto",
    to_account_name: "Buzz Primary",
    reason: "auto_switch",
    switched_at: iso(2, 16),
  },
]

const products: ProductMap[] = [
  { id: 1, website_id: 1, website_name: "Ahrefs", product_ids: ["1001", "1002"], product_name: "Ahrefs Monthly" },
  { id: 2, website_id: 3, website_name: "SeoSite Checkup", product_ids: ["2204"], product_name: "Checkup Annual" },
  { id: 3, website_id: 2, website_name: "Semrush", product_ids: ["3180", "3181"], product_name: "Semrush Pro" },
  { id: 4, website_id: 4, website_name: "BuzzSumo", product_ids: ["4412"], product_name: "BuzzSumo Starter" },
  { id: 5, website_id: 5, website_name: "ChatGPT", product_ids: ["5501"], product_name: "ChatGPT Plus" },
  { id: 6, website_id: 1, website_name: "Ahrefs", product_ids: ["1008"], product_name: "Ahrefs Agency" },
  { id: 7, website_id: 7, website_name: "Ahrefs EU", product_ids: ["1701"], product_name: "Ahrefs EU" },
  { id: 8, website_id: 8, website_name: "Semrush Asia", product_ids: ["1802"], product_name: "Semrush Asia" },
  { id: 9, website_id: 9, website_name: "Checkup US", product_ids: ["1903"], product_name: "Checkup US" },
]

const violations: Violation[] = [
  {
    id: 1,
    website_id: 1,
    website_name: "Ahrefs",
    username: "nina",
    client_ip: "203.0.113.10",
    reason: "export limit reached",
    created_at: iso(0, 10),
  },
  {
    id: 2,
    website_id: 3,
    website_name: "SeoSite Checkup",
    username: "omar",
    client_ip: "198.51.100.24",
    reason: "parallel session",
    created_at: iso(1, 13),
  },
  {
    id: 3,
    website_id: 2,
    website_name: "Semrush",
    username: "priya",
    client_ip: "192.0.2.55",
    reason: "suspended account login",
    created_at: iso(2, 9),
  },
]

const security: SecurityEvent[] = [
  {
    id: 1,
    website_id: 1,
    website_name: "Ahrefs",
    username: "nina",
    client_ip: "203.0.113.10",
    event_type: "path_blocked",
    attempted_url: "https://ct.example.com/admin/export-all",
    details: "path not on the allow list",
    user_agent: USER_AGENT,
    created_at: iso(0, 11),
  },
  {
    id: 2,
    website_id: 3,
    website_name: "SeoSite Checkup",
    username: "omar",
    client_ip: "198.51.100.77",
    event_type: "ip_blocked",
    attempted_url: "https://sc.example.com/login",
    details: "client IP is on the block list",
    user_agent: USER_AGENT,
    created_at: iso(1, 8),
  },
  {
    id: 3,
    website_id: 2,
    website_name: "Semrush",
    username: "priya",
    client_ip: "192.0.2.55",
    event_type: "session_mismatch",
    attempted_url: "https://sm.example.com/projects",
    details: "cookie session did not match the panel token",
    user_agent: USER_AGENT,
    created_at: iso(0, 6),
  },
  {
    id: 4,
    website_id: 4,
    website_name: "BuzzSumo",
    username: "leo",
    client_ip: "203.0.113.80",
    event_type: "rate_limited",
    attempted_url: "https://bz.example.com/search",
    details: "more than 60 requests in a minute",
    user_agent: USER_AGENT,
    created_at: iso(3, 14),
  },
]

const blocked: BlockedIP[] = [
  {
    id: 1,
    website_id: 1,
    website_name: "Ahrefs",
    client_ip: "203.0.113.44",
    reason: "credential stuffing",
    created_at: iso(4),
  },
  {
    id: 2,
    website_id: 0,
    website_name: "All websites",
    client_ip: "198.51.100.20",
    reason: "scanner",
    created_at: iso(8),
  },
  {
    id: 3,
    website_id: 3,
    website_name: "SeoSite Checkup",
    client_ip: "2001:db8::10",
    reason: "quota bypass",
    created_at: iso(1),
  },
]

const ingests: IngestLog[] = [
  {
    id: 1,
    website_id: 3,
    account_id: 4,
    ingest_key: "seosite_acc1",
    account_name: "SeoSite Primary",
    website_name: "SeoSite Checkup",
    status: "saved",
    error_message: "",
    bytes_saved: 1840,
    client_ip: "203.0.113.9",
    created_at: iso(0, 6),
  },
  {
    id: 2,
    website_id: 3,
    account_id: 4,
    ingest_key: "seosite_acc1",
    account_name: "SeoSite Primary",
    website_name: "SeoSite Checkup",
    status: "failed",
    error_message: "cookie array empty",
    bytes_saved: 0,
    client_ip: "203.0.113.9",
    created_at: iso(1, 6),
  },
  {
    id: 3,
    website_id: 4,
    account_id: 5,
    ingest_key: "buzzsumo_acc1",
    account_name: "Buzz Primary",
    website_name: "BuzzSumo",
    status: "failed",
    error_message: "handshake rejected",
    bytes_saved: 0,
    client_ip: "198.51.100.4",
    created_at: iso(0, 4),
  },
  {
    id: 4,
    website_id: 4,
    account_id: 5,
    ingest_key: "buzzsumo_acc1",
    account_name: "Buzz Primary",
    website_name: "BuzzSumo",
    status: "saved",
    error_message: "",
    bytes_saved: 960,
    client_ip: "198.51.100.4",
    created_at: iso(2, 4),
  },
  {
    id: 5,
    website_id: 2,
    account_id: 3,
    ingest_key: "",
    account_name: "Semrush Primary",
    website_name: "Semrush",
    status: "failed",
    error_message: "cookie rejected",
    bytes_saved: 0,
    client_ip: "192.0.2.9",
    created_at: iso(0, 12),
  },
]

let nextId = 100

function id() {
  nextId += 1
  return nextId
}

function websiteName(websiteId: number) {
  if (websiteId === 0) return "All websites"
  return websites.find((website) => website.id === websiteId)?.name ?? "Unknown"
}

function visibleIds(operator: Operator) {
  if (operator.role === "master") return null
  return new Set(operator.website_ids)
}

function chatLabelsFor(websiteId: number, event: TelegramEvent) {
  const route = telegramRoutes.find((row) => row.website_id === websiteId && row.events.includes(event))
  if (!route) return []
  return route.destination_ids
    .map((id) => telegramDestinations.find((row) => row.id === id)?.label ?? "")
    .filter(Boolean)
}

function canSee(operator: Operator, websiteId: number) {
  if (operator.role === "master") return true
  if (websiteId === 0) return true
  return operator.website_ids.includes(websiteId)
}

function assertOwned(operator: Operator, websiteId: number) {
  if (operator.role === "master") return
  if (!operator.website_ids.includes(websiteId)) {
    throw new Error("You cannot manage this website")
  }
}

function assertMaster(operator: Operator) {
  if (operator.role !== "master") {
    throw new Error("Only a master operator can do that")
  }
}

function ingestKeyTaken(key: string, exceptId?: number) {
  return accounts.some(
    (account) => account.automation_ingest_key === key && account.id !== exceptId
  )
}

for (let index = 0; index < 8; index += 1) {
  accounts.push({
    id: 30 + index,
    website_id: 1,
    website_name: "Ahrefs",
    name: `Ahrefs Seat ${index + 1}`,
    cookie: `cookie_fake_seat_${index}9999`,
    user_agent_id: 2,
    user_agent: USER_AGENT,
    proxy_id: 1,
    proxy: proxies[0].endpoint,
    status: index % 2 === 0 ? "active" : "inactive",
    last_used_at: iso(index, 9),
    failure_count: index,
    show_limit: true,
    description: "",
    automation_task_uid: `gfx_seat_${index}`,
    automation_ingest_key: `seat_key_${index}`,
    cookie_updated_at: index === 0 ? iso(0, 5) : "",
  })
  ingests.push({
    id: 20 + index,
    website_id: 1,
    account_id: 30 + index,
    ingest_key: `seat_key_${index}`,
    account_name: `Ahrefs Seat ${index + 1}`,
    website_name: "Ahrefs",
    status: index % 2 === 0 ? "saved" : "failed",
    error_message: index % 2 === 0 ? "" : "cookie array empty",
    bytes_saved: index % 2 === 0 ? 400 + index : 0,
    client_ip: "203.0.113.40",
    created_at: iso(index, 5),
  })
  violations.push({
    id: 10 + index,
    website_id: 1,
    website_name: "Ahrefs",
    username: `guest${index}`,
    client_ip: `203.0.113.${50 + index}`,
    reason: "repeated export",
    created_at: iso(index, 4),
  })
  security.push({
    id: 10 + index,
    website_id: 1,
    website_name: "Ahrefs",
    username: `guest${index}`,
    client_ip: `198.51.100.${10 + index}`,
    event_type: index % 2 === 0 ? "rate_limited" : "path_blocked",
    attempted_url: `https://ct.example.com/blocked/${index}`,
    details: "seeded security row",
    user_agent: USER_AGENT,
    created_at: iso(index, 3),
  })
  blocked.push({
    id: 10 + index,
    website_id: 1,
    website_name: "Ahrefs",
    client_ip: `192.0.2.${10 + index}`,
    reason: "scanner",
    created_at: iso(index, 2),
  })
  switches.push({
    id: 10 + index,
    website_id: 1,
    domain: "ct.example.com",
    username: `guest${index}`,
    from_account_name: "Ahrefs Backup",
    to_account_name: "Ahrefs Main",
    reason: "auto_switch",
    switched_at: iso(index, 1),
  })
  proxies.push({
    id: 10 + index,
    name: `Pool ${index + 1}`,
    proxy_type: "HTTP",
    endpoint: `http://10.8.0.${index + 1}:8080`,
    status: index % 2 === 0 ? "active" : "inactive",
    created_at: iso(index, 2),
  })
  userAgents.push({
    id: 10 + index,
    name: `Saved agent ${index + 1}`,
    user_agent: `${USER_AGENT} Extra/${index}`,
    created_at: iso(index, 2),
  })
  if (index < 7) {
    operators.push({
      id: 20 + index,
      username: `shop${index}`,
      role: "reseller",
      website_ids: [1],
    })
    resellerStatus.set(20 + index, index % 2 === 0 ? "active" : "suspended")
  }
}

usage.push(
  {
    id: 9001,
    website_id: 1,
    website_name: "Ahrefs",
    tool_name: "Ahrefs",
    username: "nora",
    limit_key: "credits",
    limit_label: "Credits",
    reset_days: 1,
    action: "Keyword overview",
    target_path: "old-daily-credit",
    amount: 1,
    timestamp: iso(3, 8),
  },
  {
    id: 9002,
    website_id: 1,
    website_name: "Ahrefs",
    tool_name: "Ahrefs",
    username: "nora",
    limit_key: "exports",
    limit_label: "Exports",
    reset_days: 7,
    action: "Batch export",
    target_path: "kept-export-3d",
    amount: 1,
    timestamp: iso(3, 8),
  },
  {
    id: 9003,
    website_id: 4,
    website_name: "BuzzSumo",
    tool_name: "BuzzSumo",
    username: "leo",
    limit_key: "credits",
    limit_label: "Credits",
    reset_days: 30,
    action: "Content search",
    target_path: "kept-month-meter",
    amount: 1,
    timestamp: iso(10, 8),
  },
)
violations.push({
  id: 9001,
  website_id: 1,
  website_name: "Ahrefs",
  username: "nora",
  client_ip: "203.0.113.90",
  reason: "old-violation-8d",
  created_at: iso(8, 9),
})
violations.push({
  id: 9002,
  website_id: 1,
  website_name: "Ahrefs",
  username: "nina",
  client_ip: "203.0.113.10",
  reason: "recent-window-2d",
  created_at: iso(2, 9),
})
security.push({
  id: 9001,
  website_id: 1,
  website_name: "Ahrefs",
  username: "nora",
  client_ip: "203.0.113.90",
  event_type: "path_blocked",
  attempted_url: "https://ct.example.com/old-security",
  details: "old-security-8d",
  user_agent: USER_AGENT,
  created_at: iso(8, 9),
})

for (const log of ingests) {
  if (log.status !== "saved") continue
  const account = accounts.find((row) => row.id === log.account_id)
  if (!account) continue
  if (!account.cookie_updated_at || log.created_at > account.cookie_updated_at) {
    account.cookie_updated_at = log.created_at
  }
}

function toolForWebsite(websiteId: number) {
  const website = websites.find((row) => row.id === websiteId)
  const tool = tools.find((row) => row.id === website?.tool_id)
  return { website, tool }
}

function pageResult<T>(rows: T[], page: number, pageSize: number) {
  const size = Math.max(1, pageSize)
  const total = rows.length
  const pageCount = Math.max(1, Math.ceil(total / size))
  const safePage = Math.min(Math.max(1, page), pageCount)
  const start = (safePage - 1) * size
  return { items: rows.slice(start, start + size), total, page: safePage, pageSize: size }
}

function latestIngestStatus(accountId: number): "" | "saved" | "failed" {
  const latest = ingests
    .filter((log) => log.account_id === accountId)
    .sort((a, b) => (a.created_at < b.created_at ? 1 : -1))[0]
  return latest?.status ?? ""
}

function applyRetention(now = Date.now()) {
  for (let index = usage.length - 1; index >= 0; index -= 1) {
    if (isOlderThan(usage[index].timestamp, usage[index].reset_days, now)) usage.splice(index, 1)
  }
  for (let index = violations.length - 1; index >= 0; index -= 1) {
    if (isOlderThan(violations[index].created_at, 7, now)) violations.splice(index, 1)
  }
  for (let index = security.length - 1; index >= 0; index -= 1) {
    if (isOlderThan(security[index].created_at, 7, now)) security.splice(index, 1)
  }
}

function isCustomLimit(user: PanelUser) {
  return Boolean(user.custom_limit_expire_at) || user.meters.some((meter) => meter.is_custom)
}

function matchesTool(websiteId: number, toolId: number) {
  if (!toolId) return true
  return toolForWebsite(websiteId).tool?.id === toolId
}

function matchesReseller(websiteId: number, resellerId: number) {
  if (!resellerId) return true
  const reseller = operators.find((row) => row.id === resellerId)
  return Boolean(reseller?.website_ids.includes(websiteId))
}

function toolHasLimits(websiteId: number) {
  return (toolForWebsite(websiteId).tool?.limits.length ?? 0) > 0
}

function metersFor(
  tool: Tool,
  defaults: Record<string, number>,
  incoming: Array<{ key: string; limit: number; used: number }>,
) {
  return tool.limits.map((definition) => {
    const found = incoming.find((row) => row.key === definition.key)
    const limit = found?.limit ?? defaults[definition.key] ?? 0
    const used = found?.used ?? 0
    return {
      key: definition.key,
      label: definition.label,
      reset_days: definition.reset_days,
      used,
      limit,
      is_custom: limit !== (defaults[definition.key] ?? 0),
    }
  })
}

function syncWebsiteUsers(websiteId: number) {
  const { website, tool } = toolForWebsite(websiteId)
  if (!website || !tool) return
  for (const user of users) {
    if (user.website_id !== websiteId) continue
    user.website_name = website.name
    user.tool_name = tool.name
    user.meters = metersFor(
      tool,
      website.default_limits,
      user.meters.map((row) => ({ key: row.key, limit: row.limit, used: row.used })),
    )
  }
}

function normalizeLimits(definitions: ToolLimitDef[]) {
  const seen = new Set<string>()
  return definitions.slice(0, 2).map((definition) => {
    const key = definition.key.trim().toLowerCase()
    if (!key || seen.has(key)) {
      throw new Error("Each limit needs a unique key")
    }
    seen.add(key)
    if (!Number.isInteger(definition.reset_days) || definition.reset_days < 1 || definition.reset_days > 365) {
      throw new Error("Reset days must be from 1 to 365")
    }
    return { key, label: definition.label.trim(), reset_days: definition.reset_days }
  })
}

export const mockStore = {
  operatorForRole(role: Role, username: string): Operator {
    if (role === "master") {
      return {
        id: 1,
        username,
        role: "master",
        website_ids: websites.map((website) => website.id),
      }
    }
    const seeded = operators.find((operator) => operator.username === "reseller")
    return {
      id: seeded?.id ?? 2,
      username,
      role: "reseller",
      website_ids: seeded ? [...seeded.website_ids] : [],
    }
  },

  dashboard(operator: Operator): DashboardStats {
    applyRetention()
    const sessionRows = sessions.filter((row) => canSee(operator, row.website_id))
    const accountRows = accounts.filter((row) => canSee(operator, row.website_id))
    const today = dayKey(new Date())
    const creditHitsToday = usage
      .filter((row) => canSee(operator, row.website_id))
      .filter((row) => row.limit_key === "credits" && dayKey(row.timestamp) === today)
      .reduce((sum, row) => sum + row.amount, 0)

    const creditHits7d = Array.from({ length: 7 }, (_, index) => {
      const date = new Date()
      date.setDate(date.getDate() - (6 - index))
      const key = dayKey(date)
      const hits = usage
        .filter((row) => canSee(operator, row.website_id))
        .filter((row) => row.limit_key === "credits" && dayKey(row.timestamp) === key)
        .reduce((sum, row) => sum + row.amount, 0)
      return { day: dayLabel(key), hits }
    })

    const cookieRows = accountRows
      .map((account) => ({
        id: account.id,
        name: account.name,
        health: cookieHealth(account.cookie_updated_at, latestIngestStatus(account.id)),
      }))
      .filter((account) => account.health !== "fresh")
    const limitRows = users
      .filter((user) => canSee(operator, user.website_id))
      .flatMap((user) =>
        user.meters
          .filter((meter) => meter.limit > 0 && meter.used / meter.limit >= 0.8)
          .map((meter) => ({
            username: user.username,
            tool_name: user.tool_name,
            used: meter.used,
            limit: meter.limit,
          })),
      )
    const start = new Date()
    start.setHours(0, 0, 0, 0)
    const soon = new Date(start)
    soon.setDate(soon.getDate() + 3)
    const expiringRows = users
      .filter((user) => canSee(operator, user.website_id) && user.custom_limit_expire_at)
      .filter((user) => {
        const expires = new Date(`${user.custom_limit_expire_at}T00:00:00`)
        return expires.getTime() <= soon.getTime()
      })
      .map((user) => ({
        username: user.username,
        tool_name: user.tool_name,
        expires_on: user.custom_limit_expire_at ?? "",
      }))

    return {
      active_sessions: sessionRows.length,
      accounts_total: accountRows.length,
      credit_hits_today: creditHitsToday,
      active_users_list: sessionRows.map((row) => ({
        username: row.username,
        website_name: row.website_name,
        client_ip: row.client_ip,
        created_at: row.created_at,
        expires_at: row.expires_at,
        assigned_account_name: row.assigned_account_name,
      })),
      credit_hits_7d: creditHits7d,
      attention: {
        cookies: { count: cookieRows.length, items: cookieRows.slice(0, 5) },
        limits: { count: limitRows.length, items: limitRows.slice(0, 5) },
        expiring: { count: expiringRows.length, items: expiringRows.slice(0, 5) },
      },
    }
  },

  listAccounts(
    operator: Operator,
    query: { page: number; pageSize: number; query: string; websiteId: number },
  ) {
    const needle = query.query.trim().toLowerCase()
    const rows = accounts.filter((row) => {
      if (!canSee(operator, row.website_id)) return false
      if (query.websiteId && row.website_id !== query.websiteId) return false
      if (!needle) return true
      const key = operator.role === "master" ? row.automation_ingest_key : ""
      return `${row.name} ${row.website_name} ${row.description} ${key}`.toLowerCase().includes(needle)
    })
    return pageResult(
      rows.map((row) => ({ ...row, latest_ingest_status: latestIngestStatus(row.id) })),
      query.page,
      query.pageSize,
    )
  },

  getAccount(operator: Operator, accountId: number) {
    const account = accounts.find((row) => row.id === accountId)
    if (!account || !canSee(operator, account.website_id)) return null
    return account
  },

  saveAccount(
    operator: Operator,
    input: Omit<MappedAccount, "id" | "website_name" | "last_used_at" | "failure_count" | "cookie_updated_at"> & {
      id?: number
    }
  ) {
    assertOwned(operator, input.website_id)
    const website = websites.find((row) => row.id === input.website_id)
    if (!website) throw new Error("Website not found")
    const key = input.automation_ingest_key.trim().toLowerCase()
    if (key && !/^[a-z0-9_]+$/.test(key)) {
      throw new Error("Ingest key must be lowercase letters, numbers, and underscores")
    }
    if (key && ingestKeyTaken(key, input.id)) {
      throw new Error("That ingest key is already used")
    }
    let userAgent = input.user_agent
    if (input.user_agent_id) {
      const saved = userAgents.find((row) => row.id === input.user_agent_id)
      if (!saved) throw new Error("User agent not found")
      userAgent = saved.user_agent
    }
    let proxy = input.proxy
    if (input.proxy_id) {
      const savedProxy = proxies.find((row) => row.id === input.proxy_id)
      if (!savedProxy) throw new Error("Proxy not found")
      proxy = savedProxy.endpoint
    }
    if (input.id) {
      const current = accounts.find((row) => row.id === input.id)
      if (!current) throw new Error("Account not found")
      assertOwned(operator, current.website_id)
      Object.assign(current, input, {
        website_name: website.name,
        automation_ingest_key: key,
        user_agent: userAgent,
        proxy,
        cookie_updated_at: new Date().toISOString(),
      })
      return current
    }
    const created: MappedAccount = {
      ...input,
      id: id(),
      website_name: website.name,
      automation_ingest_key: key,
      user_agent: userAgent,
      proxy,
      last_used_at: "",
      failure_count: 0,
      cookie_updated_at: new Date().toISOString(),
    }
    accounts.unshift(created)
    return created
  },

  deleteAccount(operator: Operator, accountId: number) {
    const index = accounts.findIndex((row) => row.id === accountId)
    if (index < 0) throw new Error("Account not found")
    assertOwned(operator, accounts[index].website_id)
    accounts.splice(index, 1)
    for (const session of sessions) {
      if (session.assigned_account_id === accountId) {
        session.assigned_account_id = 0
        session.assigned_account_name = "Auto"
      }
    }
  },

  listUsers(
    operator: Operator,
    query: { page: number; pageSize: number; query: string; toolId: number; resellerId: number; custom: boolean },
  ) {
    const needle = query.query.trim().toLowerCase()
    const rows = users.filter((row) => {
      if (!canSee(operator, row.website_id) || !toolHasLimits(row.website_id)) return false
      if (!matchesTool(row.website_id, query.toolId) || !matchesReseller(row.website_id, query.resellerId)) return false
      if (query.custom && !isCustomLimit(row)) return false
      if (!needle) return true
      return `${row.username} ${row.website_name} ${row.tool_name}`.toLowerCase().includes(needle)
    })
    return pageResult(rows, query.page, query.pageSize)
  },

  saveUser(
    operator: Operator,
    input: {
      id?: number
      website_id: number
      username: string
      status: PanelUser["status"]
      custom_limit_expire_at: string | null
      limit_visibility: "" | "show" | "hide"
      meters: Array<{ key: string; limit: number; used: number }>
    },
  ) {
    assertOwned(operator, input.website_id)
    const { website, tool } = toolForWebsite(input.website_id)
    if (!website || !tool) throw new Error("Website not found")
    if (tool.limits.length === 0) throw new Error("This tool has no limits")
    const meters = metersFor(tool, website.default_limits, input.meters)
    if (input.id) {
      const current = users.find((row) => row.id === input.id)
      if (!current) throw new Error("User not found")
      assertOwned(operator, current.website_id)
      current.website_id = website.id
      current.website_name = website.name
      current.tool_name = tool.name
      current.username = input.username.trim()
      current.status = input.status
      current.custom_limit_expire_at = input.custom_limit_expire_at
      current.limit_visibility = input.limit_visibility
      current.meters = meters
      return current
    }
    const created: PanelUser = {
      id: id(),
      website_id: website.id,
      website_name: website.name,
      tool_name: tool.name,
      username: input.username.trim(),
      status: input.status,
      custom_limit_expire_at: input.custom_limit_expire_at,
      limit_visibility: input.limit_visibility,
      meters,
    }
    users.unshift(created)
    return created
  },

  deleteUser(operator: Operator, userId: number) {
    const index = users.findIndex((row) => row.id === userId)
    if (index < 0) throw new Error("User not found")
    assertOwned(operator, users[index].website_id)
    users.splice(index, 1)
  },

  resetUserUsage(operator: Operator, userId: number) {
    const current = users.find((row) => row.id === userId)
    if (!current) throw new Error("User not found")
    assertOwned(operator, current.website_id)
    for (const item of current.meters) item.used = 0
    return current
  },

  listSessions(
    operator: Operator,
    query: { page: number; pageSize: number; query: string; toolId: number; resellerId: number },
  ) {
    const needle = query.query.trim().toLowerCase()
    const rows = sessions.filter((row) => {
      if (!canSee(operator, row.website_id)) return false
      if (!matchesTool(row.website_id, query.toolId) || !matchesReseller(row.website_id, query.resellerId)) return false
      if (!needle) return true
      return `${row.username} ${row.client_ip} ${row.website_name}`.toLowerCase().includes(needle)
    })
    return pageResult(rows, query.page, query.pageSize)
  },

  assignSession(operator: Operator, sessionId: number, accountId: number) {
    const current = sessions.find((row) => row.id === sessionId)
    if (!current) throw new Error("Session not found")
    assertOwned(operator, current.website_id)
    if (accountId === 0) {
      current.assigned_account_id = 0
      current.assigned_account_name = "Auto"
      return current
    }
    const account = accounts.find((row) => row.id === accountId)
    if (!account || account.website_id !== current.website_id) {
      throw new Error("Pick an account on the same website")
    }
    current.assigned_account_id = account.id
    current.assigned_account_name = account.name
    return current
  },

  endSession(operator: Operator, sessionId: number) {
    const index = sessions.findIndex((row) => row.id === sessionId)
    if (index < 0) throw new Error("Session not found")
    assertOwned(operator, sessions[index].website_id)
    sessions.splice(index, 1)
  },

  assignAccountsProxy(operator: Operator, ids: number[], proxyId: number) {
    const proxy = proxies.find((row) => row.id === proxyId)
    if (!proxy) throw new Error("Proxy not found")
    for (const accountId of ids) {
      const account = accounts.find((row) => row.id === accountId)
      if (!account) throw new Error("Account not found")
      assertOwned(operator, account.website_id)
      account.proxy_id = proxy.id
      account.proxy = proxy.endpoint
    }
  },

  assignAccountsUserAgent(operator: Operator, ids: number[], userAgentId: number) {
    const agent = userAgents.find((row) => row.id === userAgentId)
    if (!agent) throw new Error("User agent not found")
    for (const accountId of ids) {
      const account = accounts.find((row) => row.id === accountId)
      if (!account) throw new Error("Account not found")
      assertOwned(operator, account.website_id)
      account.user_agent_id = agent.id
      account.user_agent = agent.user_agent
    }
  },

  listQuota(
    operator: Operator,
    query: { page: number; pageSize: number; query: string; toolId: number; resellerId: number },
  ) {
    applyRetention()
    const needle = query.query.trim().toLowerCase()
    const rows = users
      .filter((user) => canSee(operator, user.website_id) && toolHasLimits(user.website_id))
      .filter((user) => matchesTool(user.website_id, query.toolId) && matchesReseller(user.website_id, query.resellerId))
      .map((user) => {
        const events = usage
          .filter((event) => event.username === user.username && event.website_id === user.website_id)
          .sort((a, b) => (a.timestamp < b.timestamp ? 1 : -1))
        return { user, events }
      })
      .filter((row) => {
        if (!needle) return true
        const paths = row.events.map((event) => `${event.target_path} ${event.action}`).join(" ")
        return `${row.user.username} ${row.user.tool_name} ${row.user.website_name} ${paths}`
          .toLowerCase()
          .includes(needle)
      })
    return pageResult(rows, query.page, query.pageSize)
  },

  listLogins(
    operator: Operator,
    query: { page: number; pageSize: number; query: string; websiteId: number },
  ) {
    const needle = query.query.trim().toLowerCase()
    const rows = loginEvents.filter((row) => {
      if (!canSee(operator, row.website_id)) return false
      if (query.websiteId && row.website_id !== query.websiteId) return false
      if (!needle) return true
      return `${row.username} ${row.client_ip} ${row.domain}`.toLowerCase().includes(needle)
    })
    return pageResult(rows, query.page, query.pageSize)
  },

  listSwitches(
    operator: Operator,
    query: { page: number; pageSize: number; query: string; websiteId: number },
  ) {
    const needle = query.query.trim().toLowerCase()
    const rows = switches.filter((row) => {
      if (!canSee(operator, row.website_id)) return false
      if (query.websiteId && row.website_id !== query.websiteId) return false
      if (!needle) return true
      return `${row.username} ${row.domain} ${row.reason}`.toLowerCase().includes(needle)
    })
    return pageResult(rows, query.page, query.pageSize)
  },

  listExtensionEvents(
    operator: Operator,
    query: { page: number; pageSize: number; query: string; websiteId: number; toolKey?: string; source?: string },
  ) {
    const needle = query.query.trim().toLowerCase()
    const rows = extensionEvents.filter((row) => {
      if (!canSee(operator, row.website_id)) return false
      if (query.websiteId && row.website_id !== query.websiteId) return false
      if (query.toolKey && row.tool_key !== query.toolKey) return false
      if (query.source && row.source !== query.source) return false
      if (!needle) return true
      return `${row.username} ${row.action} ${row.target_path} ${row.asin} ${row.query_text} ${row.domain} ${row.tool_name}`
        .toLowerCase()
        .includes(needle)
    })
    return pageResult(rows, query.page, query.pageSize)
  },

  listAutomateTasks(
    operator: Operator,
    query: { page: number; pageSize: number; query: string; websiteId: number; status: string },
  ) {
    assertMaster(operator)
    const needle = query.query.trim().toLowerCase()
    const rows = accounts.map((account) => {
      const latest = ingests
        .filter((log) => log.account_id === account.id)
        .sort((a, b) => (a.created_at < b.created_at ? 1 : -1))[0]
      return {
        ...account,
        last_status: latest?.status ?? "",
        last_time: latest?.created_at ?? "",
        last_bytes: latest?.bytes_saved ?? 0,
        last_error: latest?.error_message ?? "",
      }
    }).filter((row) => {
      if (query.websiteId && row.website_id !== query.websiteId) return false
      if (query.status === "saved" || query.status === "failed") {
        if (row.last_status !== query.status) return false
      }
      if (query.status === "none" && row.last_status) return false
      if (!needle) return true
      return `${row.website_name} ${row.name} ${row.automation_task_uid} ${row.automation_ingest_key}`
        .toLowerCase()
        .includes(needle)
    })
    return pageResult(rows, query.page, query.pageSize)
  },

  listIngestLogs(
    operator: Operator,
    query: { page: number; pageSize: number; query: string; websiteId: number; status: string; accountId: number },
  ) {
    assertMaster(operator)
    const needle = query.query.trim().toLowerCase()
    const rows = [...ingests]
      .sort((a, b) => (a.created_at < b.created_at ? 1 : -1))
      .filter((row) => {
        if (query.accountId && row.account_id !== query.accountId) return false
        if (query.websiteId && row.website_id !== query.websiteId) return false
        if ((query.status === "saved" || query.status === "failed") && row.status !== query.status) return false
        if (!needle) return true
        return `${row.ingest_key} ${row.account_name} ${row.error_message} ${row.website_name}`
          .toLowerCase()
          .includes(needle)
      })
    return pageResult(rows, query.page, query.pageSize)
  },

  listProducts(
    operator: Operator,
    query: { page: number; pageSize: number; query: string; websiteId: number },
  ) {
    const needle = query.query.trim().toLowerCase()
    const rows = products.filter((row) => {
      if (!canSee(operator, row.website_id)) return false
      if (query.websiteId && row.website_id !== query.websiteId) return false
      if (!needle) return true
      return `${row.product_ids.join(" ")} ${row.product_name} ${row.website_name}`.toLowerCase().includes(needle)
    })
    return pageResult(rows, query.page, query.pageSize)
  },

  saveProduct(
    operator: Operator,
    input: Omit<ProductMap, "id" | "website_name"> & { id?: number }
  ) {
    assertOwned(operator, input.website_id)
    const website = websites.find((row) => row.id === input.website_id)
    if (!website) throw new Error("Website not found")
    const productIds = input.product_ids.map((value) => value.trim()).filter(Boolean)
    if (productIds.length === 0) throw new Error("Add at least one product id")
    if (new Set(productIds).size !== productIds.length) throw new Error("That product id is already on this mapping")
    for (const productId of productIds) {
      const taken = products.find((row) => row.id !== input.id && row.product_ids.includes(productId))
      if (taken) throw new Error(`Product id ${productId} is already mapped`)
    }
    input = { ...input, product_ids: productIds }
    if (input.id) {
      const current = products.find((row) => row.id === input.id)
      if (!current) throw new Error("Product not found")
      assertOwned(operator, current.website_id)
      Object.assign(current, input, { website_name: website.name })
      return current
    }
    const created: ProductMap = { ...input, id: id(), website_name: website.name }
    products.unshift(created)
    return created
  },

  deleteProduct(operator: Operator, productId: number) {
    const index = products.findIndex((row) => row.id === productId)
    if (index < 0) throw new Error("Product not found")
    assertOwned(operator, products[index].website_id)
    products.splice(index, 1)
  },

  listViolations(
    operator: Operator,
    query: { page: number; pageSize: number; query: string; websiteId: number; resellerId: number },
  ) {
    applyRetention()
    const needle = query.query.trim().toLowerCase()
    const rows = violations.filter((row) => {
      if (!canSee(operator, row.website_id)) return false
      if (query.websiteId && row.website_id !== query.websiteId) return false
      if (!matchesReseller(row.website_id, query.resellerId)) return false
      if (!needle) return true
      return `${row.username} ${row.reason} ${row.client_ip} ${row.website_name}`.toLowerCase().includes(needle)
    })
    return pageResult(rows, query.page, query.pageSize)
  },

  listSecurity(
    operator: Operator,
    query: { page: number; pageSize: number; query: string; websiteId: number; eventType: string },
  ) {
    applyRetention()
    const needle = query.query.trim().toLowerCase()
    const rows = security.filter((row) => {
      if (!canSee(operator, row.website_id)) return false
      if (query.websiteId && row.website_id !== query.websiteId) return false
      if (query.eventType && row.event_type !== query.eventType) return false
      if (!needle) return true
      return `${row.username} ${row.details} ${row.attempted_url} ${row.event_type}`.toLowerCase().includes(needle)
    })
    return pageResult(rows, query.page, query.pageSize)
  },

  listBlockedIPs(
    operator: Operator,
    query: { page: number; pageSize: number; query: string; websiteId: number },
  ) {
    const needle = query.query.trim().toLowerCase()
    const rows = blocked.filter((row) => {
      if (!canSee(operator, row.website_id)) return false
      if (query.websiteId && row.website_id !== query.websiteId) return false
      if (!needle) return true
      return `${row.client_ip} ${row.reason} ${row.website_name}`.toLowerCase().includes(needle)
    })
    return pageResult(rows, query.page, query.pageSize)
  },

  blockIP(
    operator: Operator,
    input: { website_id: number; client_ip: string; reason: string }
  ) {
    if (input.website_id !== 0) assertOwned(operator, input.website_id)
    if (input.website_id === 0 && operator.role !== "master") {
      throw new Error("Only a master operator can block an IP for all websites")
    }
    if (input.website_id !== 0 && !websites.some((row) => row.id === input.website_id)) {
      throw new Error("Website not found")
    }
    const created: BlockedIP = {
      id: id(),
      website_id: input.website_id,
      website_name: websiteName(input.website_id),
      client_ip: input.client_ip.trim(),
      reason: input.reason.trim(),
      created_at: new Date().toISOString(),
    }
    blocked.unshift(created)
    return created
  },

  unblockIP(operator: Operator, blockedId: number) {
    const index = blocked.findIndex((row) => row.id === blockedId)
    if (index < 0) throw new Error("Blocked IP not found")
    const row = blocked[index]
    if (row.website_id === 0) assertMaster(operator)
    else assertOwned(operator, row.website_id)
    blocked.splice(index, 1)
  },

  listProxies(query: { page: number; pageSize: number; query: string; status: string }) {
    const needle = query.query.trim().toLowerCase()
    const rows = proxies.filter((row) => {
      if (query.status && query.status !== "all" && row.status !== query.status) return false
      if (!needle) return true
      return `${row.name} ${row.endpoint}`.toLowerCase().includes(needle)
    })
    return pageResult(rows, query.page, query.pageSize)
  },

  saveProxy(
    operator: Operator,
    input: Omit<ProxyEndpoint, "id" | "created_at"> & { id?: number }
  ) {
    assertMaster(operator)
    if (input.id) {
      const current = proxies.find((row) => row.id === input.id)
      if (!current) throw new Error("Proxy not found")
      Object.assign(current, input)
      for (const account of accounts) {
        if (account.proxy_id === current.id) account.proxy = current.endpoint
      }
      return current
    }
    const created: ProxyEndpoint = {
      ...input,
      id: id(),
      created_at: new Date().toISOString(),
    }
    proxies.unshift(created)
    return created
  },

  deleteProxy(operator: Operator, proxyId: number) {
    assertMaster(operator)
    const index = proxies.findIndex((row) => row.id === proxyId)
    if (index < 0) throw new Error("Proxy not found")
    proxies.splice(index, 1)
    for (const account of accounts) {
      if (account.proxy_id === proxyId) account.proxy_id = null
    }
  },

  listUserAgents(query: { page: number; pageSize: number; query: string }) {
    const needle = query.query.trim().toLowerCase()
    const rows = userAgents.filter((row) => {
      if (!needle) return true
      return `${row.name} ${row.user_agent}`.toLowerCase().includes(needle)
    })
    return pageResult(rows, query.page, query.pageSize)
  },

  saveUserAgent(
    operator: Operator,
    input: Omit<UserAgentProfile, "id" | "created_at"> & { id?: number },
  ) {
    assertMaster(operator)
    const name = input.name.trim()
    const user_agent = input.user_agent.trim()
    if (!name || !user_agent) throw new Error("Name and user agent are required")
    if (input.id) {
      const current = userAgents.find((row) => row.id === input.id)
      if (!current) throw new Error("User agent not found")
      current.name = name
      current.user_agent = user_agent
      for (const account of accounts) {
        if (account.user_agent_id === current.id) account.user_agent = current.user_agent
      }
      return current
    }
    const created: UserAgentProfile = { id: id(), name, user_agent, created_at: new Date().toISOString() }
    userAgents.unshift(created)
    return created
  },

  deleteUserAgent(operator: Operator, agentId: number) {
    assertMaster(operator)
    const index = userAgents.findIndex((row) => row.id === agentId)
    if (index < 0) throw new Error("User agent not found")
    userAgents.splice(index, 1)
    for (const account of accounts) {
      if (account.user_agent_id === agentId) account.user_agent_id = null
    }
  },

  listWebsites(
    operator: Operator,
    query: { page: number; pageSize: number; query: string },
  ) {
    const allowed = visibleIds(operator)
    const needle = query.query.trim().toLowerCase()
    const filtered = websites.filter((row) => {
      if (allowed && !allowed.has(row.id)) return false
      if (!needle) return true
      return `${row.name} ${row.domain}`.toLowerCase().includes(needle)
    })
    const pageSize = Math.max(1, query.pageSize)
    const pages = Math.max(1, Math.ceil(filtered.length / pageSize))
    const page = Math.min(Math.max(1, query.page), pages)
    const start = (page - 1) * pageSize
    return {
      items: filtered.slice(start, start + pageSize),
      total: filtered.length,
      page,
      pageSize,
    }
  },

  saveWebsite(
    operator: Operator,
    input: Omit<Website, "id"> & { id?: number }
  ) {
    assertMaster(operator)
    const tool = tools.find((row) => row.id === input.tool_id)
    if (!tool) throw new Error("Tool not found")
    const default_limits: Record<string, number> = {}
    for (const definition of tool.limits) {
      default_limits[definition.key] = input.default_limits[definition.key] ?? 0
    }
    const payload = { ...input, default_limits }
    if (input.id) {
      const current = websites.find((row) => row.id === input.id)
      if (!current) throw new Error("Website not found")
      Object.assign(current, payload)
      for (const account of accounts) {
        if (account.website_id === current.id) account.website_name = current.name
      }
      syncWebsiteUsers(current.id)
      return current
    }
    const created: Website = { ...payload, id: id() }
    websites.unshift(created)
    syncWebsiteUsers(created.id)
    const master = operators.find((row) => row.role === "master")
    if (master && !master.website_ids.includes(created.id)) {
      master.website_ids.push(created.id)
    }
    return created
  },

  deleteWebsite(operator: Operator, websiteId: number) {
    assertMaster(operator)
    const index = websites.findIndex((row) => row.id === websiteId)
    if (index < 0) throw new Error("Website not found")
    if (accounts.some((row) => row.website_id === websiteId)) {
      throw new Error("Remove mapped accounts for this website first")
    }
    websites.splice(index, 1)
    for (const operatorRow of operators) {
      operatorRow.website_ids = operatorRow.website_ids.filter((value) => value !== websiteId)
    }
  },

  listTools() {
    return [...tools]
  },

  saveTool(operator: Operator, input: Omit<Tool, "id"> & { id?: number }) {
    assertMaster(operator)
    const limits = normalizeLimits(input.limits)
    if (input.id) {
      const current = tools.find((row) => row.id === input.id)
      if (!current) throw new Error("Tool not found")
      Object.assign(current, { ...input, limits })
      for (const website of websites) {
        if (website.tool_id !== current.id) continue
        const nextDefaults: Record<string, number> = {}
        for (const definition of limits) {
          nextDefaults[definition.key] = website.default_limits[definition.key] ?? 0
        }
        website.default_limits = nextDefaults
        syncWebsiteUsers(website.id)
      }
      return current
    }
    const created: Tool = { ...input, limits, id: id() }
    tools.unshift(created)
    return created
  },

  listResellers(
    operator: Operator,
    query: { page: number; pageSize: number; query: string; status: string },
  ) {
    assertMaster(operator)
    const needle = query.query.trim().toLowerCase()
    const rows = operators
      .filter((row) => row.role === "reseller")
      .map((row) => ({
        ...row,
        status: resellerStatus.get(row.id) ?? "active",
      }))
      .filter((row) => {
        if (query.status && query.status !== "all" && row.status !== query.status) return false
        if (!needle) return true
        return row.username.toLowerCase().includes(needle)
      })
    return pageResult(rows, query.page, query.pageSize)
  },

  saveReseller(
    operator: Operator,
    input: {
      id?: number
      username: string
      status: "active" | "suspended"
      website_ids: number[]
    }
  ) {
    assertMaster(operator)
    const username = input.username.trim()
    if (!username) throw new Error("Username is required")
    for (const websiteId of input.website_ids) {
      if (!websites.some((row) => row.id === websiteId)) {
        throw new Error("Website not found")
      }
    }
    if (input.id) {
      const current = operators.find((row) => row.id === input.id && row.role === "reseller")
      if (!current) throw new Error("Reseller not found")
      if (operators.some((row) => row.username === username && row.id !== current.id)) {
        throw new Error("That username is already used")
      }
      current.username = username
      current.website_ids = [...input.website_ids]
      resellerStatus.set(current.id, input.status)
      return { ...current, status: input.status }
    }
    if (operators.some((row) => row.username === username)) {
      throw new Error("That username is already used")
    }
    const created: Operator = {
      id: id(),
      username,
      role: "reseller",
      website_ids: [...input.website_ids],
    }
    operators.push(created)
    resellerStatus.set(created.id, input.status)
    return { ...created, status: input.status }
  },

  clearLogins(operator: Operator) {
    for (let index = loginEvents.length - 1; index >= 0; index -= 1) {
      if (canSee(operator, loginEvents[index].website_id)) loginEvents.splice(index, 1)
    }
  },

  clearSwitches(operator: Operator) {
    for (let index = switches.length - 1; index >= 0; index -= 1) {
      if (canSee(operator, switches[index].website_id)) switches.splice(index, 1)
    }
  },

  clearExtensionEvents(operator: Operator) {
    for (let index = extensionEvents.length - 1; index >= 0; index -= 1) {
      if (canSee(operator, extensionEvents[index].website_id)) extensionEvents.splice(index, 1)
    }
  },

  telegramSettings(operator: Operator) {
    return {
      botToken: operator.role === "master" ? botToken : null,
      repeatMinutes,
    }
  },

  getTelegramTemplates() {
    return { ...telegramTemplates }
  },

  listTelegramDestinations(
    operator: Operator,
    query: { page: number; pageSize: number; query: string; resellerId: number; allChats?: boolean },
  ) {
    const needle = query.query.trim().toLowerCase()
    const rows = telegramDestinations.filter((row) => {
      if (!query.allChats && operator.role !== "master" && row.reseller_id !== operator.id) return false
      if (query.resellerId && row.reseller_id !== query.resellerId) return false
      if (!needle) return true
      return `${row.label} ${row.chat_id} ${row.reseller_name}`.toLowerCase().includes(needle)
    })
    return pageResult(rows, query.page, query.pageSize)
  },

  saveTelegramDestination(
    operator: Operator,
    input: Omit<TelegramDestination, "id" | "reseller_name"> & { id?: number },
  ) {
    const label = input.label.trim()
    const chatId = input.chat_id.trim()
    if (!label || !chatId) throw new Error("Label and chat id are required")
    const resellerId = operator.role === "master" ? input.reseller_id : operator.id
    const reseller = operators.find((row) => row.id === resellerId && row.role === "reseller")
    if (!reseller) throw new Error("Reseller not found")
    if (input.id) {
      const current = telegramDestinations.find((row) => row.id === input.id)
      if (!current) throw new Error("Destination not found")
      if (operator.role === "reseller" && current.reseller_id !== operator.id) throw new Error("Destination not found")
      Object.assign(current, {
        reseller_id: reseller.id,
        reseller_name: reseller.username,
        label,
        chat_id: chatId,
        enabled: input.enabled,
      })
      return current
    }
    const created: TelegramDestination = {
      id: id(),
      reseller_id: reseller.id,
      reseller_name: reseller.username,
      label,
      chat_id: chatId,
      enabled: input.enabled,
    }
    telegramDestinations.unshift(created)
    return created
  },

  deleteTelegramDestination(operator: Operator, destinationId: number) {
    const index = telegramDestinations.findIndex((row) => row.id === destinationId)
    if (index < 0) throw new Error("Destination not found")
    if (operator.role === "reseller" && telegramDestinations[index].reseller_id !== operator.id) {
      throw new Error("Destination not found")
    }
    telegramDestinations.splice(index, 1)
  },

  saveTelegramTemplates(operator: Operator, input: TelegramTemplates) {
    assertMaster(operator)
    const logout = input.logout.trim()
    const spam = input.spam.trim()
    if (!logout || !spam) throw new Error("Both templates are required")
    telegramTemplates = { logout, spam }
    return { ...telegramTemplates }
  },

  saveSpamRule(operator: Operator, input: SpamRule) {
    assertMaster(operator)
    if (input.window_minutes < 1 || input.max_opens < 1 || input.max_distinct_ips < 1) {
      throw new Error("Use numbers of at least 1")
    }
    spamRule = {
      window_minutes: input.window_minutes,
      max_opens: input.max_opens,
      max_distinct_ips: input.max_distinct_ips,
    }
    return { ...spamRule }
  },

  saveBotToken(operator: Operator, token: string) {
    assertMaster(operator)
    const next = token.trim()
    if (!next) throw new Error("Enter a bot token")
    botToken = next
  },

  saveRepeatMinutes(operator: Operator, minutes: number) {
    assertMaster(operator)
    if (!Number.isInteger(minutes) || minutes < 1) throw new Error("Use at least 1 minute")
    repeatMinutes = minutes
  },

  listTelegramRoutes(
    operator: Operator,
    query: { page: number; pageSize: number; query: string; event: string; resellerId: number },
  ) {
    const needle = query.query.trim().toLowerCase()
    const rows = telegramRoutes
      .map((route) => ({
        ...route,
        website_domain: websites.find((row) => row.id === route.website_id)?.domain ?? "",
        chat_labels: route.destination_ids
          .map((id) => telegramDestinations.find((row) => row.id === id)?.label ?? "")
          .filter(Boolean),
      }))
      .filter((route) => {
        const chats = telegramDestinations.filter((row) => route.destination_ids.includes(row.id))
        if (operator.role !== "master" && !chats.some((chat) => chat.reseller_id === operator.id)) return false
        if (query.resellerId && !chats.some((chat) => chat.reseller_id === query.resellerId)) return false
        if (query.event && !route.events.includes(query.event as TelegramEvent)) return false
        if (!needle) return true
        const domain = websites.find((row) => row.id === route.website_id)?.domain ?? ""
        return `${route.website_name} ${domain}`.toLowerCase().includes(needle)
      })
    return pageResult(rows, query.page, query.pageSize)
  },

  saveTelegramRoute(
    operator: Operator,
    input: { website_id: number; events: TelegramEvent[]; destination_ids: number[] },
  ) {
    const website = websites.find((row) => row.id === input.website_id)
    if (!website) throw new Error("Website not found")
    if (operator.role === "reseller" && !operator.website_ids.includes(website.id)) {
      throw new Error("You cannot manage this website")
    }
    const events = [...new Set(input.events)].filter((event) => event === "logout" || event === "spam")
    if (events.length === 0) throw new Error("Choose at least one event")
    if (input.destination_ids.length === 0) throw new Error("Choose at least one chat")
    if (input.destination_ids.some((destinationId) => !telegramDestinations.some((row) => row.id === destinationId))) {
      throw new Error("Chat not found")
    }
    const existing = telegramRoutes.find((row) => row.website_id === website.id)
    if (existing) {
      existing.events = events
      existing.destination_ids = [...input.destination_ids]
      existing.website_name = website.name
      return existing
    }
    const created: TelegramRoute = {
      id: id(),
      website_id: website.id,
      website_name: website.name,
      events,
      destination_ids: [...input.destination_ids],
    }
    telegramRoutes.unshift(created)
    return created
  },

  listTelegramDeliveries(
    operator: Operator,
    query: { page: number; pageSize: number; query: string; event: string; status: string; resellerId: number },
  ) {
    const needle = query.query.trim().toLowerCase()
    const resellerName = operators.find((row) => row.id === query.resellerId)?.username ?? ""
    const rows = telegramDeliveries.filter((row) => {
      if (operator.role !== "master" && !row.reseller_names.includes(operator.username)) return false
      if (resellerName && !row.reseller_names.includes(resellerName)) return false
      if (query.event && row.event !== query.event) return false
      if (query.status && row.status !== query.status) return false
      if (!needle) return true
      const recipients = row.recipients.map((item) => `${item.label} ${item.chat_id}`).join(" ")
      return `${row.username} ${row.website_name} ${row.website_domain} ${recipients}`.toLowerCase().includes(needle)
    })
    return pageResult(rows, query.page, query.pageSize)
  },

  clearTelegramDeliveries(operator: Operator) {
    for (let index = telegramDeliveries.length - 1; index >= 0; index -= 1) {
      if (operator.role === "master" || telegramDeliveries[index].reseller_names.includes(operator.username)) {
        telegramDeliveries.splice(index, 1)
      }
    }
  },

  listSpamReports(operator: Operator): SpamReport[] {
    return spamReports
      .filter((row) => canSee(operator, row.website_id))
      .map((row) => ({
        ...row,
        chat_labels: chatLabelsFor(row.website_id, "spam"),
      }))
  },

  deleteReseller(operator: Operator, resellerId: number) {
    assertMaster(operator)
    const index = operators.findIndex((row) => row.id === resellerId && row.role === "reseller")
    if (index < 0) throw new Error("Reseller not found")
    if (operators[index].username === "reseller") {
      throw new Error("The seeded reseller used by the role switch cannot be deleted")
    }
    operators.splice(index, 1)
    resellerStatus.delete(resellerId)
  },
}
