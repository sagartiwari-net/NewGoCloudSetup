Build the new ToolsMandi Update Panel frontend from scratch inside this folder:

`/Users/sagartiwari/Desktop/oneclickgo/pending-tools/update-panel`

Read this file fully, then read and follow these skills before writing UI:

- `/Users/sagartiwari/.cursor/plugins/cache/cursor-public/shadcn/10f1717a3e2a3c16cfbd43877c1e44063d9d749a/skills/shadcn/SKILL.md`
- `/Users/sagartiwari/.cursor/plugins/cache/cursor-public/gsap-skills/aed9cfd3277740755f6bfc1155c7aa645403b760/skills/gsap-react/SKILL.md`
- `/Users/sagartiwari/.cursor/plugins/cache/cursor-public/gsap-skills/aed9cfd3277740755f6bfc1155c7aa645403b760/skills/gsap-core/SKILL.md`

The old panel source is reference only. Do not edit it, do not copy its HTML/CSS, and do not copy its secrets or real cookies into this app:

`/Users/sagartiwari/Desktop/oneclickgo/pending-tools/ahrefs-admin`

Keep `README.md` and `BUILD-PROMPT.md` in this folder.

## What this product is

This is the operator panel for every tool proxy. From here an admin will later manage websites, reseller access, mapped login accounts, user quotas, live sessions, security, and automatic cookie updates.

GoAuto (external automation) will later POST a session to the Go backend at `POST /api/updateauto?key=<ingest_key>`. This frontend does not implement that endpoint. It only shows the Automate Task screen: which account owns which unique ingest key, the exact URL the automation should call, and the last save or fail result.

No backend in this task. Every screen reads and writes an in-memory mock store whose JSON shapes match the future Go API, so swapping the store for `fetch` later does not change the pages.

## Stack

Create the Next.js app in this folder (App Router, TypeScript, Tailwind, `src/` directory, ESLint). Then add shadcn/ui with the project package runner.

- Next.js App Router + TypeScript
- Tailwind + shadcn/ui (sidebar, card, table, badge, button, dialog, sheet, dropdown menu, tabs, input, textarea, select, checkbox, switch, label, separator, skeleton, sonner, empty, alert, avatar, scroll-area, tooltip, chart)
- Forms: react-hook-form + zod, composed with shadcn Field / FieldGroup rules from the shadcn skill
- Tables: shadcn Table + TanStack Table (sort, search, pagination)
- Charts: shadcn Chart (Recharts) on Overview and Analytics
- Motion: `gsap` + `@gsap/react`, using `useGSAP` with a scope ref
- Icons: lucide-react, passed as components, sized by the shadcn component
- Theme: `next-themes`, default dark

Leave a single data boundary:

- `src/lib/api/types.ts` — the contracts below
- `src/lib/api/mock-store.ts` — in-memory CRUD, seeded with fake data
- `src/lib/api/client.ts` — the only module pages import. Every function is async and returns the same shape a future Go handler returns (`{ status: "ok" }` or the list/object). Later this file becomes `fetch` to the Go panel. Pages never import the mock store directly.

Do not add Redux. Do not call MySQL, Go, or any network API.

## Design

Dark control panel. One accent. Dense enough for daily ops, still comfortable to read. Semantic Tailwind tokens only (`bg-background`, `text-muted-foreground`, `bg-primary`). No raw hex in components, no glassmorphism, no gradient text, no emoji in the UI.

Responsive without shrinking type to fit:

- Desktop: fixed shadcn sidebar + scrollable main
- Under `md`: sidebar becomes a sheet, tables become stacked cards, filters stack, dialogs become full-width sheets
- Type scale stays the same on every breakpoint. Text wraps or truncates with a tooltip. It never drops below the body size to squeeze a row onto one line.
- Touch targets stay at least 40px on phone.

GSAP only for: login card enter, overview metric count-up, and the main page content fade/slide when the route changes. Scope every tween. Clean up with `useGSAP`. If `prefers-reduced-motion: reduce`, skip tweens. Do not animate table rows, dialogs, or layout.

Cookie and secret values are secrets even in the mock. Show a masked preview (`••••` + last 4 chars) and a reveal toggle that lasts until the dialog closes. Never print a full cookie or handshake secret in a table cell, toast, or chart.

## Auth for this frontend

Login screen at `/login`. Any non-empty username and password of 8+ characters signs in. Two seeded operators:

- `master` / role `master` — sees every page
- `reseller` / role `reseller` — sees only websites assigned to that reseller

Persist the fake session in `sessionStorage`. Protect the app shell. A small role switch in the header (visible only in this mock build) reloads the same screens as the other role so both layouts can be checked. Reseller nav must omit Proxy Manager, Website Domains, Resellers, and tool-catalog management.

## Screens

Use one app shell. Routes:

| Route | Who | What the user can do |
|---|---|---|
| `/login` | public | Sign in |
| `/` | both | Overview |
| `/accounts` | both | Mapped accounts |
| `/users` | both | User limits |
| `/sessions` | both | Active logins |
| `/usage` | both | Quota logs |
| `/analytics` | both | Login history + account switches |
| `/automate` | both | Automate Task (cookie update) |
| `/products` | both | aMember product id mapping |
| `/violations` | both | Violation log |
| `/security` | both | Security log |
| `/blocked-ips` | both | Blocked IPs |
| `/proxies` | master | Shared proxy pool |
| `/websites` | master | Tool websites |
| `/resellers` | master | Reseller accounts |
| `/profile` | both | Change password (mock only) |

Every list has search, an empty state (shadcn Empty), a skeleton while the async client resolves, and a toast on save, update, or delete (sonner). Deletes confirm in a dialog. Forms validate with zod before the mock store changes.

### Overview `/`

Metrics: active sessions, mapped accounts, credit hits today. Chart: credit hits for the last 7 days (mock). Table of live users: username, website, client IP, assigned account, started, expires.

### Mapped Accounts `/accounts`

The core ops screen. Filter by website. Create, edit, delete.

Fields: website, account name, cookie (masked textarea), user agent, proxy (pick from the proxy pool or a manual `socks5://` / `http://` string), status `active | inactive`, show limit toggle, GoAuto task UID (optional, example `gfx_runSeositecheckup`), automation ingest key.

Ingest key rules, matching the future API: optional; if present, lowercase, only `[a-z0-9_]`, unique across accounts. Show the key in monospace. Table columns: name, website, status, failure count, last used, ingest key, last ingest status.

### User Limits `/users`

Per end-user quota on a website. Create, edit, delete, reset usage.

Fields: username, website, credit limit, export limit, status `active | suspended`, optional custom limit expiry date `YYYY-MM-DD`, reset usage.

Table: username, website, credits used / limit, exports used / limit, status, expiry.

### Active Logins `/sessions`

Live sessions. Columns: username, website, client IP, assigned account, created, expires. Actions: force logout; reassign the session to a specific mapped account, or back to auto-switch (`assigned_account_id = 0`).

### Quota Logs `/usage`

Credit log table: username, website, action, amount, timestamp. Search by username. Filter by website.

### Analytics `/analytics`

Two tabs.

- Logins: username, website domain, client IP, user agent (truncated), time. Search username or IP.
- Switches: username, website, from account, to account, reason, time.

A small chart of logins per day for the last 7 days.

### Automate Task `/automate`

This is the update panel. One row per mapped account.

Show: website, account name, task UID, ingest key, last ingest status (`saved` or `failed`), last ingest time, bytes saved, error message when failed.

Selecting a row opens a detail sheet:

- Ingest URL preview: `https://ctrl.toolsmandi.com/api/updateauto?key=<ingest_key>`
- Method POST, header `Authorization: Bearer <token>` shown as masked `Bearer ••••`
- Accepted body shapes, as readable examples, not a live request:
  - `{ "cookie": [ { "name": "session", "value": "…" } ] }`
  - `{ "session_data": { "cookies": [], "local_storage": {}, "indexed_db": {} } }`
  - `{ "key": "<ingest_key>", "cookie": [] }`
- A log list for that key: status, time, bytes, client IP, error

Do not send the request. Copy-URL can write the preview URL to the clipboard.

Also show a recent global ingest log under the table.

### Product mapping `/products`

Map an aMember product id to a website. Fields: website, product id (string), product name. Create, edit, delete. Search.

### Violations `/violations`

Read-only log: username, website, client IP, reason, time. Search username or IP.

### Security `/security`

Read-only log: username, website, client IP, event type, attempted URL, details, time. Search username, IP, or URL. Filter by event type. Seed event types: `path_blocked`, `ip_blocked`, `session_mismatch`, `rate_limited`.

### Blocked IPs `/blocked-ips`

Create and delete. Fields: IP, website scope (a website or “all websites”), reason. Table: IP, scope, reason, created. Validate a real IPv4 or IPv6 string with zod.

### Proxy Manager `/proxies` (master)

Pool used by account forms. Fields: name, type `SOCKS5 | HTTP | HTTPS`, endpoint, status `active | inactive`. Create, edit, delete.

### Website Domains `/websites` (master)

Each website is one public tool domain. Create, edit, delete, plus a nested way to add a tool to the catalog (name, category, credit label, export label).

Website fields: tool, name, domain, handshake secret (masked), session duration minutes, default credit limit, default export limit, default proxy, session security toggle.

Table: name, domain, tool, security on/off, default credit limit.

### Resellers `/resellers` (master)

Create and edit. Fields: username, password (only on create or when changing), status `active | suspended`, checklist of websites this reseller may manage. Table: username, status, website count.

### Profile `/profile`

Change password. New password minimum 8 characters. Success toast. Nothing is stored except the mock session flag.

## Type contracts

Put these in `src/lib/api/types.ts`. Names match the future Go JSON.

```ts
export type Role = "master" | "reseller"

export type Operator = {
  id: number
  username: string
  role: Role
  website_ids: number[]
}

export type Tool = {
  id: number
  name: string
  category: string
  limit_label_1: string
  limit_label_2: string
}

export type Website = {
  id: number
  tool_id: number
  name: string
  domain: string
  secret_key: string
  session_duration: number
  default_credit_limit: number
  default_export_limit: number
  proxy: string
  session_security_enabled: boolean
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
  user_agent: string
  proxy: string
  status: "active" | "inactive"
  last_used_at: string
  failure_count: number
  show_limit: boolean
  automation_task_uid: string
  automation_ingest_key: string
}

export type PanelUser = {
  id: number
  website_id: number
  website_name: string
  username: string
  credit_limit: number
  credits_used: number
  export_limit: number
  exports_used: number
  status: "active" | "suspended"
  custom_limit_expire_at: string | null
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
  username: string
  action: string
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

export type ProductMap = {
  id: number
  website_id: number
  website_name: string
  product_id: string
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
    website_name: string
    client_ip: string
    created_at: string
    expires_at: string
    assigned_account_name: string
  }>
  credit_hits_7d: Array<{ day: string; hits: number }>
}
```

Seed about 4 websites (Ahrefs, Semrush, SeoSite Checkup, BuzzSumo style names and fake domains like `ct.example.com`), 6 mapped accounts, 2 with ingest keys `seosite_acc1` and `buzzsumo_acc1`, a mix of saved and failed ingest logs, sessions, users, and security rows. All cookies, secrets, tokens, and proxy passwords must be obvious fakes (`cookie_fake_…`, `secret_fake_…`).

Reseller seed can see only two of the four websites. Master sees all.

## Client functions to stub

`src/lib/api/client.ts` exports async functions. Implement them against the mock store now.

- `login`, `logout`, `currentOperator`
- `getDashboard`
- `listAccounts`, `saveAccount`, `deleteAccount`
- `listUsers`, `saveUser`, `deleteUser`, `resetUserUsage`
- `listSessions`, `assignSession`, `endSession`
- `listUsage`
- `listLogins`, `listSwitches`
- `listIngestLogs`
- `listProducts`, `saveProduct`, `deleteProduct`
- `listViolations`
- `listSecurity`
- `listBlockedIPs`, `blockIP`, `unblockIP`
- `listProxies`, `saveProxy`, `deleteProxy`
- `listWebsites`, `saveWebsite`, `deleteWebsite`
- `listTools`, `saveTool`
- `listResellers`, `saveReseller`
- `changePassword`

Reseller calls only return that reseller’s `website_ids`. Save and delete reject a website the operator does not own, with an error the toast can show.

## Done when

- `npm run build` passes.
- Desktop and a 390px viewport both work: login as master, open every route, create one account with an ingest key, see it on Automate Task, switch to reseller and confirm master-only routes are gone.
- No full cookie or secret is visible in any table.
- Motion respects reduced motion.
- The app stays inside `pending-tools/update-panel`. Do not modify `ahrefs-admin`, other proxies, or `TOOLS-STATUS.md`.
