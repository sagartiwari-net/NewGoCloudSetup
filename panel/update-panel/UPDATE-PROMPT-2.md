Update the existing Update Panel in this folder. Do not rebuild the app. Do not edit `pending-tools/ahrefs-admin` or any proxy.

`/Users/sagartiwari/Desktop/oneclickgo/pending-tools/update-panel`

Read `src/components/choice-select.tsx`, `src/components/account-editor.tsx`, `src/components/app-shell.tsx`, `src/app/(panel)/accounts/page.tsx`, `src/app/(panel)/websites/page.tsx`, `src/app/(panel)/usage/page.tsx`, `src/lib/api/types.ts`, and `src/lib/api/mock-store.ts` before editing.

Follow the shadcn skill at `/Users/sagartiwari/.cursor/plugins/cache/cursor-public/shadcn/10f1717a3e2a3c16cfbd43877c1e44063d9d749a/skills/shadcn/SKILL.md`. Still mock data only. `client.ts` stays the only data boundary.

## 1. Search inside every picker

`ChoiceSelect` is a plain select. Tools, websites, resellers, proxies, and accounts will get long. Every `ChoiceSelect` in the app must filter as you type.

Replace the menu with a shadcn combobox: the closed control still shows the current label, and the open menu has a search input at the top. Typing narrows the items immediately. Arrow keys and Enter still select. An empty match shows "No matches".

Do this inside `src/components/choice-select.tsx` so accounts, user limits, sessions, quota, products, websites, blocked IPs, security, and the list filters all get it. Status toggles that are not `ChoiceSelect` can stay as they are. Two-option selects that do use `ChoiceSelect` still get the search box; it does no harm.

## 2. Mapped account edit is a popup again

The full page at `/accounts/new` and `/accounts/[id]` is too slow. Edit and New account open `FormOverlay` on `/accounts`. Remove those two routes and `AccountEditor`'s page navigation. Move the form into the dialog.

Keep two tabs inside the dialog, one form, one Save. Switching tabs must not wipe typed values.

**Basic:** cookie (masked, reveal until the dialog closes), user agent, proxy, status.

**Advanced:** description, show limit, and — master only — GoAuto task UID and automation ingest key. Read-only last used and failure count. Delete with confirm.

Name and website sit above the tabs because both tabs need them. Website uses the searchable picker.

Add `description: string` on `MappedAccount`. It is a short note, optional, max 280 characters. Show it on the accounts table, truncated, so the note is visible without opening the dialog.

Proxy and user agent stay on this account dialog only. Each account keeps its own.

## 3. Reset period is a number of days

Stop storing `period: "daily" | "weekly" | "monthly"`. The admin types how many days until that limit resets.

```ts
export type ToolLimitDef = {
  key: string
  label: string
  reset_days: number
}
```

`reset_days` is an integer from 1 to 365.

- 1 day shows as `every 1 day`
- 7 days shows as `every 7 days`
- 30 days shows as `every 30 days`
- Any other number shows as `every N days`

On the tool-catalog form, each limit has a label and a "Resets every (days)" number input. Preset chips for 1, 7, and 30 fill that input; a custom number is typed directly. Do not infer the period for the user.

`UserLimitMeter` and `UsageEvent` use `reset_days` instead of `period`. User Limits and Quota display `Credits · every 1 day`, `Exports · every 7 days`. Seed Ahrefs as credits every 1 day and exports every 7 days. ChatGPT credits every 1 day. Canva still has no limits and stays off User Limits and Quota.

## 4. Website form has no proxy

Remove Default proxy from the website create and edit form in `src/app/(panel)/websites/page.tsx`. A website does not carry a proxy. Drop `proxy` from the website save payload. Existing per-account proxy values stay. The Proxy Manager page stays for the pool that the account dialog picks from.

## 5. Automation is master only

A reseller must not see GoAuto or ingest data.

- Sidebar: Automate Task is `masterOnly`. Visiting `/automate` as reseller redirects home.
- Accounts table: hide Ingest key and Last ingest status when the role is reseller.
- Account dialog Advanced tab: hide task UID, ingest key, and last ingest when the role is reseller. Description, show limit, and delete stay.
- Do not put ingest keys in reseller toasts or search text.

Master still sees all of it.

## 6. Quota is one row per user and tool, with a log

The current Quota table repeats nina three times for Ahrefs. Group it.

One row per `username + website_id`. Columns:

- Username
- Tool
- Limits: each meter as `used / limit · every N days`
- Logs button
- Reset

Reset still clears that user's used amounts on that website and leaves the log lines.

**Logs** opens a sheet for that one user and that one tool. It lists every deduction, newest first, in the shape of the old Ahrefs panel:

| Log type | Usage | Target path | Time (IST) |
|---|---|---|---|
| Daily Credit Hit | 1 | Overview: menupricesincanada.com | 27/9/2026, 12:03:56 am |
| Export Rows | 1,000 | /v4/seGetOrganicKeywordsExport | 21/9/2026, 2:45:31 am |

Store this on each event:

```ts
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
```

`target_path` is the exact reason: a page label plus host (`Organic Search: freeinvoicemaker.ca`) or an API path (`/v4/seGetOrganicKeywordsExport`). Log type is derived from the meter: a 1-day credit meter reads `Daily Credit Hit`; any other meter reads `{label}` such as `Export Rows`.

Seed nina with three Ahrefs credit hits at different times and paths, plus one Ahrefs export. Those four lines are one table row. Opening Logs shows all four. If the same user has ChatGPT, that is a second row with its own log.

Search still filters as you type across username, tool, and target path, and it filters grouped rows. Tool and reseller dropdowns stay, and they use the searchable picker. Tools with zero limits never appear.

Previous / Next applies to the grouped rows.

## Done when

- `npm run build` passes.
- Opening any website or tool picker shows a search box, and typing filters the list.
- Mapped Accounts Edit opens a dialog on the same page. Description saves and shows on the table. Basic has cookie, user agent, proxy, and status.
- Website save has no proxy field.
- A tool limit's reset is a day count the admin sets, shown as `every N days`.
- Reseller login has no Automate Task, no ingest key, and no task UID.
- Quota shows one Ahrefs row for nina. Logs lists each hit with target path and IST time.
- Check desktop and a 390px width in the browser.
