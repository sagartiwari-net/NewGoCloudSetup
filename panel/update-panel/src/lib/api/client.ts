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
  TelegramInboxRow,
  TelegramEvent,
  TelegramRoute,
  TelegramTemplates,
  CookieLink,
  CookieLinkCopy,
  CookieLinkSources,
  AutomationCredential,
  AutomationJob,
  AutomationLog,
  AutomationOptions,
  AutomationStorageDetail,
  AutomationStorageRow,
  RecloudTool,
  RecloudToolDetail,
  Tool,
  UsageEvent,
  UserAgentProfile,
  UserReport,
  UserReportSecurityRow,
  Violation,
  Website,
} from "@/lib/api/types"

const catalogCache = new Map<string, Promise<unknown>>()

function cachedCatalog<T>(key: string, load: () => Promise<T>): Promise<T> {
  const existing = catalogCache.get(key) as Promise<T> | undefined
  if (existing) return existing
  const pending = load().catch((error: unknown) => {
    catalogCache.delete(key)
    throw error
  })
  catalogCache.set(key, pending)
  return pending
}

export function clearCatalogCache(key?: string) {
  if (!key) {
    catalogCache.clear()
    return
  }
  catalogCache.delete(key)
  if (key === "websites") catalogCache.delete("account-websites")
}

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

export async function listCookieLinks(): Promise<{ items: CookieLink[] }> {
  return api("/api/cookie-links")
}

export async function cookieLinkSources(accountId: number): Promise<CookieLinkSources> {
  return api(`/api/cookie-links/sources${qs({ accountId })}`)
}

export async function saveCookieLink(followerAccountId: number, sourceAccountId: number): Promise<CookieLinkCopy> {
  return api("/api/cookie-links", {
    method: "POST",
    body: JSON.stringify({ follower_account_id: followerAccountId, source_account_id: sourceAccountId }),
  })
}

export async function deleteCookieLink(followerAccountId: number): Promise<Ok> {
  await api(`/api/cookie-links${qs({ follower: followerAccountId })}`, { method: "DELETE" })
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

export function listProxyChoices() {
  return cachedCatalog("proxies", () => listProxies({ page: 1, pageSize: 50, query: "", status: "all" }))
}

export async function saveProxy(input: ProxyInput): Promise<Ok> {
  clearCatalogCache("proxies")
  await api("/api/proxies", { method: "POST", body: JSON.stringify(input) })
  return { status: "ok" }
}

export async function deleteProxy(id: number): Promise<Ok> {
  clearCatalogCache("proxies")
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

export function listAgentChoices() {
  return cachedCatalog("agents", () => listUserAgents({ page: 1, pageSize: 50, query: "" }))
}

export async function saveUserAgent(input: UserAgentInput): Promise<Ok> {
  clearCatalogCache("agents")
  await api("/api/user-agents", { method: "POST", body: JSON.stringify(input) })
  return { status: "ok" }
}

export async function deleteUserAgent(id: number): Promise<Ok> {
  clearCatalogCache("agents")
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

function mapWebsiteOptions(
  page: { items?: Array<Pick<Website, "id" | "tool_id" | "name" | "domain" | "default_limits">> },
): Website[] {
  return (page.items ?? []).map((website) => ({
    ...website,
    secret_key: "",
    session_duration: 0,
    session_security_enabled: false,
  }))
}

export function listAllWebsites(): Promise<Website[]> {
  return loadWebsiteChoices()
}

// Product Mapping must see a website the moment a tool creates it. The catalog
// cache is filled by other pages and is not cleared when a proxy inserts a row.
export function listWebsiteChoices(): Promise<Website[]> {
  return loadWebsiteChoices()
}

function loadWebsiteChoices(): Promise<Website[]> {
  return api<{ items: Array<Pick<Website, "id" | "tool_id" | "name" | "domain" | "default_limits">> }>(
    "/api/websites/options",
  ).then(mapWebsiteOptions)
}

export function listAccountWebsites(): Promise<Website[]> {
  return cachedCatalog("account-websites", async () => {
    const page = await api<{ items: Array<Pick<Website, "id" | "tool_id" | "name" | "domain" | "default_limits">> }>(
      "/api/websites/options?exclude=recloud",
    )
    return mapWebsiteOptions(page)
  })
}

export async function listRecloud(query: {
  page: number
  pageSize: number
  query: string
  cookie: string
}): Promise<PageResult<RecloudTool>> {
  return api(`/api/recloud${qs(query)}`)
}

export async function getRecloud(websiteId: number): Promise<RecloudToolDetail> {
  return api(`/api/recloud/${websiteId}`)
}

export async function saveRecloud(input: { website_id: number; upstream_url: string; cookie: string }): Promise<Ok> {
  await api("/api/recloud", { method: "POST", body: JSON.stringify(input) })
  return { status: "ok" }
}

export function automationOptions(): Promise<AutomationOptions> {
  return api("/api/automation/options")
}

export async function listAutomationCredentials(query: {
  page: number
  pageSize: number
  query: string
}): Promise<PageResult<AutomationCredential>> {
  return api(`/api/automation/credentials${qs(query)}`)
}

export async function saveAutomationCredential(input: {
  id?: number
  name: string
  provider: string
  username: string
  password?: string
}): Promise<Ok> {
  await api("/api/automation/credentials", { method: "POST", body: JSON.stringify(input) })
  return { status: "ok" }
}

export async function deleteAutomationCredential(id: number): Promise<Ok> {
  await api(`/api/automation/credentials${qs({ id })}`, { method: "DELETE" })
  return { status: "ok" }
}

export async function listAutomationStorage(query: {
  page: number
  pageSize: number
  query: string
}): Promise<PageResult<AutomationStorageRow>> {
  return api(`/api/automation/storage${qs(query)}`)
}

export async function getAutomationStorage(credentialId: number): Promise<AutomationStorageDetail> {
  return api(`/api/automation/storage/${credentialId}`)
}

export async function saveAutomationStorage(credentialId: number, cookie: string): Promise<Ok> {
  await api("/api/automation/storage", { method: "POST", body: JSON.stringify({ credential_id: credentialId, cookie }) })
  return { status: "ok" }
}

export async function listAutomationJobs(query: {
  page: number
  pageSize: number
  query: string
}): Promise<PageResult<AutomationJob>> {
  return api(`/api/automation/jobs${qs(query)}`)
}

export async function saveAutomationJob(input: {
  id?: number
  name: string
  provider: string
  credential_id: number
  target_website_id: number
  target_account_id: number
  interval_minutes: number
  enabled: boolean
}): Promise<Ok> {
  await api("/api/automation/jobs", { method: "POST", body: JSON.stringify(input) })
  return { status: "ok" }
}

export async function deleteAutomationJob(id: number): Promise<Ok> {
  await api(`/api/automation/jobs${qs({ id })}`, { method: "DELETE" })
  return { status: "ok" }
}

export async function runAutomationJob(id: number): Promise<Ok> {
  await api(`/api/automation/jobs/${id}/run`, { method: "POST" })
  return { status: "ok" }
}

export async function listAutomationLogs(query: {
  page: number
  pageSize: number
  query: string
  jobId?: number
  level?: string
}): Promise<PageResult<AutomationLog>> {
  return api(`/api/automation/logs${qs(query)}`)
}

export async function saveWebsite(input: WebsiteInput): Promise<Ok> {
  clearCatalogCache("websites")
  await api("/api/websites", { method: "POST", body: JSON.stringify(input) })
  return { status: "ok" }
}

export async function deleteWebsite(id: number): Promise<Ok> {
  clearCatalogCache("websites")
  await api(`/api/websites${qs({ id })}`, { method: "DELETE" })
  return { status: "ok" }
}

export function listTools(): Promise<Tool[]> {
  return cachedCatalog("tools", () => api("/api/tools"))
}

export async function saveTool(input: ToolInput): Promise<Tool> {
  clearCatalogCache("tools")
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

export function listResellerChoices() {
  return cachedCatalog("resellers", () => listResellers({ page: 1, pageSize: 50, query: "", status: "all" }))
}

export async function saveReseller(input: ResellerInput): Promise<Ok> {
  clearCatalogCache("resellers")
  await api("/api/resellers", { method: "POST", body: JSON.stringify(input) })
  return { status: "ok" }
}

export async function deleteReseller(id: number): Promise<Ok> {
  clearCatalogCache("resellers")
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

export async function listTelegramInbox(): Promise<TelegramInboxRow[]> {
  const page = await api<{ items: TelegramInboxRow[] }>("/api/telegram/inbox")
  return page.items ?? []
}

export async function deleteTelegramInbox(chatIds: string[]): Promise<Ok> {
  await api("/api/telegram/inbox/delete", { method: "POST", body: JSON.stringify({ chat_ids: chatIds }) })
  return { status: "ok" }
}

export async function clearTelegramInbox(): Promise<Ok> {
  await api("/api/telegram/inbox/delete", { method: "POST", body: JSON.stringify({ all: true }) })
  return { status: "ok" }
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
