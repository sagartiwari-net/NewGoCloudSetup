Update the existing Update Panel in this folder. Do not rebuild it. Do not edit `pending-tools/ahrefs-admin` or any proxy. Do not add an admin audit log or a CSV export.

`/Users/sagartiwari/Desktop/oneclickgo/pending-tools/update-panel`

Read `src/components/choice-select.tsx`, `src/components/ui/combobox.tsx`, `src/app/(panel)/accounts/page.tsx`, `src/app/(panel)/page.tsx`, `src/app/(panel)/violations/page.tsx`, `src/app/(panel)/security/page.tsx`, `src/app/(panel)/blocked-ips/page.tsx`, `src/lib/api/mock-store.ts`, and `src/lib/api/types.ts` before editing.

Follow the shadcn skill. Mock data only. `client.ts` stays the only data boundary.

## 1. Every dropdown must open

Website, Reseller, Tool, and the other `ChoiceSelect` menus do not open on click. Violations, Security, and Blocked IPs show this, and the same control is used on the other pages and inside dialogs.

`ChoiceSelect` passes `container={null}` into the combobox portal whenever the control is not inside a dialog. That is the first place to fix. On a normal page, omit `container` so the menu portals to `document.body`. Inside a dialog or sheet, portal into that dialog so the menu is not clipped, and keep it above the dialog.

After the change, a click on the trigger opens a menu with a search box. Choosing an item updates the filter or the form. Typing narrows the items. Check these controls in the browser:

- Violations: Website, Reseller
- Security: Event type, Website
- Blocked IPs: Website, and the website field inside Block IP
- Mapped Accounts: Website filter, and Website, Proxy, and User agent inside Edit
- User Limits: Tool, Reseller, and the website field inside New user
- Automate Task: Website, Status
- Product Mapping, Analytics, Proxies, Website Domains: their dropdowns

A menu that paints underneath the page or inside a clipped card is still broken.

## 2. Mapped Accounts shows when the cookie was last saved

Ingest key and last ingest status do not belong on this table. Both roles see a **Cookie updated** column instead. The key and the ingest status stay on Automate Task and in the master Advanced tab.

Add `cookie_updated_at: string` on `MappedAccount`. Set it when the cookie is saved in the account dialog, and when an ingest log is `saved`. A failed ingest does not move it.

The cell shows the time in IST, the same format as the other tables. Next to the time, a health badge:

| Badge | Rule |
|---|---|
| Fresh | Saved less than 2 days ago, and the latest ingest is not `failed` |
| Stale | Saved 2 days ago or earlier |
| Failed | The latest ingest for that account is `failed` |
| Never | No `cookie_updated_at` |

Fresh is green. Stale is amber. Failed is red. Never is muted. Reseller sees the badge and the time, not the ingest key.

Seed a mix: one fresh, one stale, one failed, one never.

## 3. Overview alerts

On `/`, under the metrics, add **Needs attention**. Three groups, each a short list of at most 5 rows, plus a count:

- **Cookies** — accounts that are Stale, Failed, or Never. Link each name to Mapped Accounts.
- **Limits** — users with any meter at 80% or more (`used / limit >= 0.8`). Show username, tool, and `used / limit`.
- **Expiring** — custom limits whose `custom_limit_expire_at` is within the next 3 days, or already past. Show username, tool, and the date.

When a group is empty, that group says "None". Do not add charts for this.

## 4. Bulk proxy and user agent

Mapped Accounts rows have a checkbox. The header checkbox selects every row on the current page only.

When one or more rows are checked, a bar shows the count and two actions: **Set proxy** and **Set user agent**. Each opens a confirm dialog with one searchable picker of saved pool rows. Apply sets `proxy_id` or `user_agent_id` on the checked accounts and clears a one-off URL or string on those accounts. Other fields stay. Cancel applies nothing. Save is the confirm button, labeled with the count (`Update 3 accounts`).

The pool edit rule stays: later edits to that saved proxy or user agent still update these accounts.

## 5. Old rows delete themselves

Put the rule in `src/lib/retention.ts` and run it inside the mock store before those lists are returned. The page never receives an expired row.

**Quota logs.** A line lives for that meter's `reset_days`. A credit line with `reset_days: 1` is gone after 1 day. An export line with `reset_days: 7` stays 7 days. A line with `reset_days: 30` stays 30 days. The clock starts at `timestamp`.

**Violations and security.** Keep 7 days, then drop the row. Same clock, from `created_at`.

Do not purge accounts, blocked IPs, analytics, sessions, or ingest logs in this pass.

On Quota Logs, one line under the title: "Each line is kept for that limit's reset period, then removed." On Violations and Security: "Kept for 7 days, then removed."

Seed proof:

- A 1-day credit event dated 3 days ago must not appear.
- A 7-day export event dated 3 days ago must appear.
- A 30-day meter event dated 10 days ago must appear.
- A violation and a security event dated 8 days ago must not appear.
- A violation dated 2 days ago must appear.

## Done when

- `npm run build` passes.
- In the browser, Website and Reseller on Violations open and filter the table. The same check passes for Security, Blocked IPs, and the account dialog.
- Mapped Accounts has Cookie updated and the health badge, and does not have Ingest key or Last ingest status.
- Overview lists a stale or failed cookie and a user near their limit.
- Checking two accounts and setting a saved proxy updates both.
- A 3-day-old 1-day credit line is absent from Quota Logs. A 2-day-old violation is present.
