import type {
  AccessAlerts,
  AnalyticsSummary,
  BlockedIP,
  HostReport,
  DashboardStats,
  DirectoryUser,
  IpReport,
  IngestLog,
  LiveSession,
  ExtensionEvent,
  LoginEvent,
  LogoutEvent,
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
  UsageEvent,
  UserAgentProfile,
  UserReport,
  UserReportSecurityRow,
  Violation,
  Website,
} from "@/lib/api/types"

export type Session = {
  username: string
  role: Role
  token: string
}

export type Reseller = Operator & {
  status: "active" | "suspended"
}

export type AccountInput = Omit<
  MappedAccount,
  "id" | "website_name" | "last_used_at" | "failure_count" | "cookie_updated_at"
> & { id?: number }

export type AccountListItem = MappedAccount & {
  latest_ingest_status: "" | "saved" | "failed"
}

export type UserInput = {
  id?: number
  website_id: number
  username: string
  status: "active" | "suspended"
  custom_limit_expire_at: string | null
  limit_visibility: "" | "show" | "hide"
  meters: Array<{ key: string; limit: number; used: number }>
}

export type ProductInput = Omit<ProductMap, "id" | "website_name"> & { id?: number }

export type ProxyInput = Omit<ProxyEndpoint, "id" | "created_at"> & { id?: number }

export type WebsiteInput = Omit<Website, "id"> & { id?: number }

export type ToolInput = Omit<Tool, "id"> & { id?: number }

export type ResellerInput = {
  id?: number
  username: string
  status: "active" | "suspended"
  website_ids: number[]
}

export type Ok = { status: "ok" }

const SESSION_KEY = "update-panel-session"

export function readSessionRaw(): string {
  if (typeof window === "undefined") return ""
  const current = window.localStorage.getItem(SESSION_KEY)
  if (current) return current
  const legacy = window.sessionStorage.getItem(SESSION_KEY)
  if (!legacy) return ""
  window.localStorage.setItem(SESSION_KEY, legacy)
  window.sessionStorage.removeItem(SESSION_KEY)
  return legacy
}

export function readSession(): Session | null {
  const raw = readSessionRaw()
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw) as Session
    if (!parsed.username || !parsed.token || (parsed.role !== "master" && parsed.role !== "reseller")) {
      return null
    }
    return parsed
  } catch {
    return null
  }
}

export function writeSession(session: Session) {
  window.localStorage.setItem(SESSION_KEY, JSON.stringify(session))
  window.sessionStorage.removeItem(SESSION_KEY)
}

export function clearSession() {
  window.localStorage.removeItem(SESSION_KEY)
  window.sessionStorage.removeItem(SESSION_KEY)
}

const API = process.env.NEXT_PUBLIC_PANEL_API ?? "http://127.0.0.1:8090"

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const token = readSession()?.token
  const response = await fetch(`${API}${path}`, {
    ...init,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(init?.headers ?? {}),
    },
  })
  const text = await response.text()
  const data = text ? JSON.parse(text) as T & { error?: string } : ({} as T)
  if (response.status === 401 && path !== "/api/login") {
    clearSession()
    window.dispatchEvent(new Event("update-panel-session"))
    if (window.location.pathname !== "/login") {
      window.location.replace("/login")
    }
    return new Promise(() => undefined) as Promise<T>
  }
  if (!response.ok) {
    throw new Error((data as { error?: string }).error || response.statusText)
  }
  return data
}

function qs(query: Record<string, string | number | boolean | undefined>) {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined && value !== "") params.set(key, String(value))
  }
  const text = params.toString()
  return text ? `?${text}` : ""
}

export async function login(username: string, password: string): Promise<Operator> {
  const operator = await api<Operator & { token?: string }>("/api/login", {
    method: "POST",
    body: JSON.stringify({ username: username.trim(), password }),
  })
  if (!operator.token) {
    throw new Error("Sign in failed")
  }
  writeSession({ username: operator.username, role: operator.role, token: operator.token })
  return operator
}

export async function logout(): Promise<Ok> {
  try {
    await api<Ok>("/api/logout", { method: "POST" })
  } finally {
    clearSession()
  }
  return { status: "ok" }
}

export async function currentOperator(): Promise<Operator> {
  return api<Operator>("/api/me")
}

export async function getDashboard(): Promise<DashboardStats> {
  return api<DashboardStats>("/api/dashboard")
}

export type PageResult<T> = {
  items: T[]
  total: number
  page: number
  pageSize: number
}

export type QuotaRow = {
  user: PanelUser
  events: UsageEvent[]
}

export type AutomateTask = MappedAccount & {
  last_status: "" | "saved" | "failed"
  last_time: string
  last_bytes: number
  last_error: string
}

export async function listAccounts(query: {
  page: number
  pageSize: number
  query: string
  websiteId: number
}): Promise<PageResult<AccountListItem>> {
  return api(`/api/accounts${qs(query)}`)
}

export async function assignAccountsProxy(ids: number[], proxyId: number): Promise<Ok> {
  await api("/api/accounts/proxy", { method: "POST", body: JSON.stringify({ ids, proxy_id: proxyId }) })
  return { status: "ok" }
}

export async function assignAccountsUserAgent(ids: number[], userAgentId: number): Promise<Ok> {
  await api("/api/accounts/user-agent", { method: "POST", body: JSON.stringify({ ids, user_agent_id: userAgentId }) })
  return { status: "ok" }
}

export async function getAccount(id: number): Promise<MappedAccount | null> {
  return api(`/api/accounts/${id}`)
}

export async function saveAccount(input: AccountInput): Promise<Ok> {
  await api("/api/accounts", { method: "POST", body: JSON.stringify(input) })
  return { status: "ok" }
}

export async function deleteAccount(id: number): Promise<Ok> {
  await api(`/api/accounts${qs({ id })}`, { method: "DELETE" })
  return { status: "ok" }
}

export async function listUsers(query: {
  page: number
  pageSize: number
  query: string
  toolId: number
  resellerId: number
  custom: boolean
}): Promise<PageResult<PanelUser>> {
  return api(`/api/users${qs(query)}`)
}

export async function listDirectoryUsers(query: {
  page: number
  pageSize: number
  query: string
  toolId: number
  resellerId: number
}): Promise<PageResult<DirectoryUser>> {
  return api(`/api/directory-users${qs(query)}`)
}

export async function saveUser(input: UserInput): Promise<Ok> {
  await api("/api/users", { method: "POST", body: JSON.stringify(input) })
  return { status: "ok" }
}

export async function deleteUser(id: number): Promise<Ok> {
  await api(`/api/users${qs({ id })}`, { method: "DELETE" })
  return { status: "ok" }
}

export async function resetUserUsage(id: number): Promise<Ok> {
  await api(`/api/users/${id}/reset`, { method: "POST" })
  return { status: "ok" }
}

export async function listSessions(query: {
  page: number
  pageSize: number
  query: string
  toolId: number
  resellerId: number
}): Promise<PageResult<LiveSession>> {
  return api(`/api/sessions${qs(query)}`)
}

export async function assignSession(sessionId: number, accountId: number): Promise<Ok> {
  await api("/api/sessions/assign", { method: "POST", body: JSON.stringify({ session_id: sessionId, account_id: accountId }) })
  return { status: "ok" }
}

export async function endSession(sessionId: number): Promise<Ok> {
  await api(`/api/sessions${qs({ id: sessionId })}`, { method: "DELETE" })
  return { status: "ok" }
}

export async function listQuota(query: {
  page: number
  pageSize: number
  query: string
  toolId: number
  resellerId: number
}): Promise<PageResult<QuotaRow>> {
  return api(`/api/quota${qs(query)}`)
}

export async function listLogins(query: {
  page: number
  pageSize: number
  query: string
  websiteId: number
}): Promise<PageResult<LoginEvent>> {
  return api(`/api/logins${qs(query)}`)
}

export async function clearLogins(): Promise<Ok> {
  await api("/api/logins", { method: "DELETE" })
  return { status: "ok" }
}

export async function clearSwitches(): Promise<Ok> {
  await api("/api/switches", { method: "DELETE" })
  return { status: "ok" }
}

export async function listSwitches(query: {
  page: number
  pageSize: number
  query: string
  websiteId: number
}): Promise<PageResult<SwitchEvent>> {
  return api(`/api/switches${qs(query)}`)
}

export async function clearLogouts(): Promise<Ok> {
  await api("/api/logouts", { method: "DELETE" })
  return { status: "ok" }
}

export async function listLogouts(query: {
  page: number
  pageSize: number
  query: string
  websiteId: number
}): Promise<PageResult<LogoutEvent>> {
  return api(`/api/logouts${qs(query)}`)
}

export async function listExtensionEvents(query: {
  page: number
  pageSize: number
  query: string
  websiteId: number
  toolKey?: string
  source?: string
}): Promise<PageResult<ExtensionEvent>> {
  return api(`/api/extension-events${qs(query)}`)
}

export async function clearExtensionEvents(): Promise<Ok> {
  await api("/api/extension-events", { method: "DELETE" })
  return { status: "ok" }
}

export async function getAnalyticsSummary(query: {
  websiteId: number
  days: number
  query: string
}): Promise<AnalyticsSummary> {
  return api(`/api/analytics/summary${qs(query)}`)
}

export async function getUserReport(query: {
  username: string
  websiteId?: number
}): Promise<UserReport> {
  return api(`/api/user-report${qs(query)}`)
}

export type UserReportSection = "logins" | "switches" | "extension" | "usage" | "security"

export async function listUserReportEvents(query: {
  section: UserReportSection
  username: string
  websiteId?: number
  page: number
  pageSize: number
}): Promise<PageResult<LoginEvent | SwitchEvent | ExtensionEvent | UsageEvent | UserReportSecurityRow>> {
  return api(`/api/user-report/events${qs(query)}`)
}

export async function getIpReport(query: { ip: string }): Promise<IpReport> {
  return api(`/api/ip-report${qs(query)}`)
}

export async function listAutomateTasks(query: {
  page: number
  pageSize: number
  query: string
  websiteId: number
  status: string
}): Promise<PageResult<AutomateTask>> {
  return api(`/api/automate-tasks${qs(query)}`)
}

export async function listIngestLogs(query: {
  page: number
  pageSize: number
  query: string
  websiteId: number
  status: string
  accountId: number
}): Promise<PageResult<IngestLog>> {
  return api(`/api/ingest-logs${qs(query)}`)
}

export async function listProducts(query: {
  page: number
  pageSize: number
  query: string
  websiteId: number
}): Promise<PageResult<ProductMap>> {
  return api(`/api/products${qs(query)}`)
}

export async function saveProduct(input: ProductInput): Promise<Ok> {
  await api("/api/products", { method: "POST", body: JSON.stringify(input) })
  return { status: "ok" }
}

export async function deleteProduct(id: number): Promise<Ok> {
  await api(`/api/products${qs({ id })}`, { method: "DELETE" })
  return { status: "ok" }
}

export async function listViolations(query: {
  page: number
  pageSize: number
  query: string
  websiteId: number
  resellerId: number
}): Promise<PageResult<Violation>> {
  return api(`/api/violations${qs(query)}`)
}

export async function listSecurity(query: {
  page: number
  pageSize: number
  query: string
  websiteId: number
  eventType: string
}): Promise<PageResult<SecurityEvent>> {
  return api(`/api/security${qs(query)}`)
}

export async function getAccessAlerts(): Promise<AccessAlerts> {
  return api("/api/access-alerts")
}

export async function saveAccessAlerts(input: AccessAlerts): Promise<Ok> {
  await api("/api/access-alerts", { method: "POST", body: JSON.stringify(input) })
  return { status: "ok" }
}

export async function listHostReports(query: {
  page: number
  pageSize: number
  query: string
  websiteId: number
  type: string
}): Promise<PageResult<HostReport>> {
  return api(`/api/host-reports${qs(query)}`)
}

export async function listBlockedIPs(query: {
  page: number
  pageSize: number
  query: string
  websiteId: number
}): Promise<PageResult<BlockedIP>> {
  return api(`/api/blocked-ips${qs(query)}`)
}

export async function blockIP(input: {
  website_id: number
  client_ip: string
  reason: string
}): Promise<Ok> {
  await api("/api/blocked-ips", { method: "POST", body: JSON.stringify(input) })
  return { status: "ok" }
}

export async function unblockIP(id: number): Promise<Ok> {
  await api(`/api/blocked-ips${qs({ id })}`, { method: "DELETE" })
  return { status: "ok" }
}

export async function listProxies(query: {
  page: number
  pageSize: number
  query: string
  status: string
}): Promise<PageResult<ProxyEndpoint>> {
  return api(`/api/proxies${qs(query)}`)
}

export async function saveProxy(input: ProxyInput): Promise<Ok> {
  await api("/api/proxies", { method: "POST", body: JSON.stringify(input) })
  return { status: "ok" }
}

export async function deleteProxy(id: number): Promise<Ok> {
  await api(`/api/proxies${qs({ id })}`, { method: "DELETE" })
  return { status: "ok" }
}

export type UserAgentInput = {
  id?: number
  name: string
  user_agent: string
}

export async function listUserAgents(query: {
  page: number
  pageSize: number
  query: string
}): Promise<PageResult<UserAgentProfile>> {
  return api(`/api/user-agents${qs(query)}`)
}

export async function saveUserAgent(input: UserAgentInput): Promise<Ok> {
  await api("/api/user-agents", { method: "POST", body: JSON.stringify(input) })
  return { status: "ok" }
}

export async function deleteUserAgent(id: number): Promise<Ok> {
  await api(`/api/user-agents${qs({ id })}`, { method: "DELETE" })
  return { status: "ok" }
}

export async function listWebsites(query: {
  page: number
  pageSize: number
  query: string
}): Promise<PageResult<Website>> {
  return api(`/api/websites${qs(query)}`)
}

export async function listAllWebsites(): Promise<Website[]> {
  const pageSize = 50
  const first = await listWebsites({ page: 1, pageSize, query: "" })
  const items = [...first.items]
  const pages = Math.max(1, Math.ceil(first.total / pageSize))
  for (let page = 2; page <= pages; page += 1) {
    const next = await listWebsites({ page, pageSize, query: "" })
    items.push(...next.items)
  }
  return items
}

export async function saveWebsite(input: WebsiteInput): Promise<Ok> {
  await api("/api/websites", { method: "POST", body: JSON.stringify(input) })
  return { status: "ok" }
}

export async function deleteWebsite(id: number): Promise<Ok> {
  await api(`/api/websites${qs({ id })}`, { method: "DELETE" })
  return { status: "ok" }
}

export async function listTools(): Promise<Tool[]> {
  return api("/api/tools")
}

export async function saveTool(input: ToolInput): Promise<Tool> {
  return api("/api/tools", { method: "POST", body: JSON.stringify(input) })
}

export async function listResellers(query: {
  page: number
  pageSize: number
  query: string
  status: string
}): Promise<PageResult<Reseller>> {
  return api(`/api/resellers${qs(query)}`)
}

export async function saveReseller(input: ResellerInput): Promise<Ok> {
  await api("/api/resellers", { method: "POST", body: JSON.stringify(input) })
  return { status: "ok" }
}

export async function deleteReseller(id: number): Promise<Ok> {
  await api(`/api/resellers${qs({ id })}`, { method: "DELETE" })
  return { status: "ok" }
}

export type TelegramSettings = {
  botToken: string | null
  repeatMinutes: number
}

export type TelegramRouteRow = TelegramRoute & { chat_labels: string[]; website_domain: string }

export type TelegramDestinationInput = Omit<TelegramDestination, "id" | "reseller_name"> & { id?: number }

export async function getTelegramSettings(): Promise<TelegramSettings> {
  return api("/api/telegram/settings")
}

export async function getTelegramTemplates(): Promise<TelegramTemplates> {
  return api("/api/telegram/templates")
}

export async function listTelegramDestinations(query: {
  page: number
  pageSize: number
  query: string
  resellerId: number
  allChats?: boolean
}): Promise<PageResult<TelegramDestination>> {
  return api(`/api/telegram/destinations${qs(query)}`)
}

export async function saveTelegramDestination(input: TelegramDestinationInput): Promise<Ok> {
  await api("/api/telegram/destinations", { method: "POST", body: JSON.stringify(input) })
  return { status: "ok" }
}

export async function deleteTelegramDestination(id: number): Promise<Ok> {
  await api(`/api/telegram/destinations${qs({ id })}`, { method: "DELETE" })
  return { status: "ok" }
}

export async function saveTelegramTemplates(input: TelegramTemplates): Promise<Ok> {
  await api("/api/telegram/templates", { method: "POST", body: JSON.stringify(input) })
  return { status: "ok" }
}

export async function saveSpamRule(input: SpamRule): Promise<Ok> {
  await api("/api/spam-rule", { method: "POST", body: JSON.stringify(input) })
  return { status: "ok" }
}

export async function listSpamReports(): Promise<SpamReport[]> {
  return api("/api/spam-reports")
}

export async function saveBotToken(token: string): Promise<Ok> {
  await api("/api/telegram/token", { method: "POST", body: JSON.stringify({ token }) })
  return { status: "ok" }
}

export async function saveRepeatMinutes(minutes: number): Promise<Ok> {
  await api("/api/telegram/repeat", { method: "POST", body: JSON.stringify({ minutes }) })
  return { status: "ok" }
}

export async function listTelegramRoutes(query: {
  page: number
  pageSize: number
  query: string
  event: string
  resellerId: number
}): Promise<PageResult<TelegramRouteRow>> {
  return api(`/api/telegram/routes${qs(query)}`)
}

export async function saveTelegramRoute(input: {
  website_id: number
  events: TelegramEvent[]
  destination_ids: number[]
}): Promise<Ok> {
  await api("/api/telegram/routes", { method: "POST", body: JSON.stringify(input) })
  return { status: "ok" }
}

export async function listTelegramDeliveries(query: {
  page: number
  pageSize: number
  query: string
  event: string
  status: string
  resellerId: number
}): Promise<PageResult<TelegramDelivery>> {
  return api(`/api/telegram/deliveries${qs(query)}`)
}

export async function clearTelegramDeliveries(): Promise<Ok> {
  await api("/api/telegram/deliveries", { method: "DELETE" })
  return { status: "ok" }
}

export async function openToolAccess(input: {
  website_id: number
  username: string
  product_id: string
  client_ip?: string
}): Promise<{ allowed: boolean; error?: string; open_url?: string; tool?: string }> {
  return api("/api/access", { method: "POST", body: JSON.stringify(input) })
}

export async function changePassword(password: string): Promise<Ok> {
  if (password.length < 8) {
    throw new Error("Use at least 8 characters")
  }
  await api("/api/password", { method: "POST", body: JSON.stringify({ password }) })
  return { status: "ok" }
}
