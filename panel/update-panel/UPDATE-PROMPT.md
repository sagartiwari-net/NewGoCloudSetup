Update the existing Update Panel frontend in this folder. Do not rebuild it, and do not touch `pending-tools/ahrefs-admin` or any other proxy.

`/Users/sagartiwari/Desktop/oneclickgo/pending-tools/update-panel`

Read `src/lib/api/types.ts`, `src/lib/api/mock-store.ts`, `src/lib/api/client.ts`, `src/components/data-table.tsx`, `src/components/row-menu.tsx`, `src/components/app-shell.tsx`, `src/components/providers.tsx`, and the pages under `src/app/(panel)/` before editing.

Follow the shadcn skill at `/Users/sagartiwari/.cursor/plugins/cache/cursor-public/shadcn/10f1717a3e2a3c16cfbd43877c1e44063d9d749a/skills/shadcn/SKILL.md`. Keep GSAP as it is. Still no real backend: mock store and `client.ts` only.

## 1. Direct Edit, no Actions menu

Remove the row dropdown whose button says "Actions" (`src/components/row-menu.tsx`). On every list, the main action is a visible **Edit** button.

Secondary actions stay visible as their own buttons, with their real names:

- User Limits: Edit, Reset, Delete
- Active Logins: Assign, End
- Blocked IPs: Unblock
- Quota Logs: Reset
- Other editable lists (products, proxies, websites, resellers, blocked IP create stays): Edit and Delete

Buttons stay on one line on desktop and wrap on the phone cards. Destructive actions still confirm.

## 2. Mapped Accounts: full page, Basic and Advanced

Stop editing accounts in `FormOverlay`. List stays at `/accounts`. Create and edit are routes:

- `/accounts/new`
- `/accounts/[id]`

That page is the account. Two tabs at the top, one form, one Save. Switching tabs does not lose typed values.

**Basic** — the fields changed every day:

- cookie (masked textarea, reveal until the page is left)
- user agent
- proxy (pool or manual `socks5://` / `http://` URL)
- status `active | inactive`

**Advanced** — the rest:

- account name
- website
- show limit
- GoAuto task UID
- automation ingest key (`[a-z0-9_]`, unique, optional)
- read-only: last used, failure count, last ingest status and time
- Delete, with confirm

Save from either tab. New account starts on Basic; website and name are required before save, so put name and website on Basic as well if Advanced has not been opened yet. Keep cookie, user agent, proxy, and status as the Basic focus. Name and website can sit above the tabs because both modes need them.

List columns stay readable: name, website, status, ingest key, last ingest status, and an Edit button that links to `/accounts/[id]`. Cookie values stay masked in the table.

## 3. Light theme, with dark still available

Default theme is **light**. `ThemeProvider` in `src/components/providers.tsx` uses `defaultTheme="light"` and `enableSystem={false}`. Add a Light / Dark control in the header of `src/components/app-shell.tsx`, next to the role switch. It must persist with `next-themes`. Both themes use the existing semantic tokens. Check login, the sidebar, tables, badges, and dialogs in both themes.

## 4. Tool limits are per tool, and can be 0, 1, or 2

Replace the fixed credit + export pair. A tool defines its own meters.

```ts
export type LimitPeriod = "daily" | "weekly" | "monthly"

export type ToolLimitDef = {
  key: string
  label: string
  period: LimitPeriod
}

export type Tool = {
  id: number
  name: string
  category: string
  limits: ToolLimitDef[]
}
```

`limits` length is 0, 1, or 2. Examples the seed must include:

| Tool | Limits |
|---|---|
| Ahrefs | Credits, daily + Exports, weekly |
| ChatGPT | Credits, daily only |
| Canva | none |

Keep Semrush and the other seeded tools on two limits so the tables stay full. Add a ChatGPT website and a Canva website, mapped accounts optional, and at least one end user on Canva.

Website defaults follow the tool. Replace `default_credit_limit` and `default_export_limit` with:

```ts
default_limits: Record<string, number>
```

Only keys that exist on that website's tool. The website form shows one number input per tool limit, labeled with the limit name and period. A tool with no limits shows no default-limit inputs.

Each end user stores one row per tool limit, not two hardcoded numbers:

```ts
export type UserLimitMeter = {
  key: string
  label: string
  period: LimitPeriod
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
  meters: UserLimitMeter[]
}
```

`is_custom` is true when that meter's limit differs from the website default. A user is in the custom set when any meter is custom or `custom_limit_expire_at` is set.

**A tool with `limits: []` never appears in User Limits or Quota Logs.** Its users, its rows, and its name in those dropdowns are omitted. It still appears under Website Domains and Mapped Accounts.

The tool-catalog form on the websites page edits 0, 1, or 2 limits. Each limit has a label and a period. Adding a second limit is optional. Clearing both is allowed. Saving a tool rewrites that tool's meters; user rows for that tool follow the new keys.

The new-user form shows only the meters of the chosen website. You cannot pick Canva there.

Seed at least one Ahrefs user whose credits or exports differ from the website default, and one user still on the defaults. Seed 12 or more users across limited tools so pagination is visible. Include a Canva user who must not show on `/users`.

## 5. User Limits filters

`/users` gets two tabs:

- **All users** — every user whose tool has at least one limit
- **Custom limits** — only the custom set defined above

Above the table, filters that apply inside the active tab:

- Username search. Filter on each keystroke. No submit button.
- Tool dropdown. All tools, then only tools that have at least one limit.
- Reseller dropdown, master only. Options are All resellers plus each reseller username. Choosing a reseller keeps rows whose `website_id` is in that reseller's `website_ids`. Hide this dropdown when the signed-in role is reseller.

Table cells show each meter the tool actually has, for example `Credits 120 / 500 · daily` and `Exports 4 / 20 · weekly`. A one-limit tool shows one meter. Do not render an empty exports column.

Edit opens the form with those same dynamic fields. Reset sets every meter `used` back to 0 and leaves the limit numbers. Delete still removes the user limit row.

## 6. Active Logins filters

Same filter bar on `/sessions`:

- Search filters as you type, matching username or client IP
- Tool dropdown (every tool is allowed here, including tools with no limits)
- Reseller dropdown for master, same website scoping
- Assign and End stay as labeled buttons

Seed enough sessions that Previous / Next appears before filtering.

## 7. Quota Logs

`/usage` is the record of what consumed a limit.

```ts
export type UsageEvent = {
  id: number
  website_id: number
  website_name: string
  tool_name: string
  username: string
  limit_key: string
  limit_label: string
  period: LimitPeriod
  action: string
  amount: number
  timestamp: string
}
```

`action` is the real reason, not a generic word. Seed lines such as `Keyword overview`, `Batch export`, `Chat message`. Amount is how much of that meter was used.

Drop events for tools with no limits. Canva must not appear.

Filters:

- Search as you type across username and action
- Tool dropdown, only tools with limits
- Reseller dropdown for master, same scoping

Columns: username, tool, limit (label + period), action, amount, time, Reset.

Reset on a log row calls the same user reset as User Limits: that user's meters on that website go back to 0 used. The log history stays. Toast confirms it.

Seed enough events to paginate.

## 8. Every search box filters while typing

Check every search field already in the app (accounts, users, sessions, usage, analytics, products, violations, security, websites, resellers). Each one filters on `onChange`. Pass the current search and dropdown values into `DataTable` `resetKey` so the page returns to 1 when the filter changes.

Previous and Next stay under the table whenever the filtered rows need more than one page. Page size stays 8. Show `Page X of Y`.

## Done when

- `npm run build` passes.
- Light theme loads first. Dark toggle still works after refresh.
- `/accounts/1` is a page with Basic and Advanced, not a dialog. Cookie, user agent, proxy, and status save from Basic.
- `/users` All vs Custom tabs work. Canva is absent. Ahrefs shows daily credits and weekly exports. ChatGPT shows only credits.
- Master can filter User Limits, Active Logins, and Quota Logs by tool and by reseller. Typing in search narrows the rows immediately. Previous and Next show when the list is long enough.
- Quota Reset clears used amounts for that user and leaves the log lines in place.
- Verify in the browser on the running dev server (it is on port 3210 if that process is up). Walk login, accounts edit page, user limits both tabs, sessions, and quota, on desktop and a 390px width.
