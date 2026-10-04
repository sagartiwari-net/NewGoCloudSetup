Finish the Go API in `/Users/sagartiwari/Desktop/oneclickgo/pending-tools/panel-api`. Do not rebuild it. Do not edit `pending-tools/ahrefs-admin`. Do not switch the Next.js panel back to the mock store. Do not delete `data/panel.db`.

The panel at `pending-tools/update-panel` already calls `http://127.0.0.1:8090` with `Authorization: Bearer <token>`. Keep every existing JSON shape in `pending-tools/update-panel/src/lib/api/types.ts` and `client.ts`.

## Already working, leave it

- Login `master` / `toolsmandi`. A 401 from any route except `/api/login` is handled by the frontend.
- 78 websites, domain `127.0.0.1:<port>`. 78 mapped accounts, one per tool, cookie already loaded. Do not reseed or wipe these.
- Create, edit, and delete for websites, tools, accounts, proxies, and user agents.
- `GET /api/websites/{id}/next-account` lists active accounts, least recently used first. A failure must not set `status` to `inactive`.

These routes currently return an empty page or `{status:ok}` without writing a row. Make them read and write SQLite:

User Limits, Active Logins, Quota Logs, Analytics logins and switches, Violations, Security, Blocked IPs, Resellers, Telegram chats, Telegram routes, Telegram sent log, spam rule.

`POST /api/blocked-ips` already inserts. `GET /api/blocked-ips` must return those rows.

Lists stay paged: `page`, `pageSize` (the UI sends 10, 20, or 50), `query`, and the filters the frontend already sends. Return `{ items, total, page, pageSize }`.

## Account switch

`POST /api/websites/{id}/use-account` with `{ "username": "...", "reason": "" }`.

Pick the next active account for that website, least recently used first, skipping accounts already named in this request's `failed_ids`. Set `last_used_at`. Do not change `status`.

If `reason` is not empty, the previous account failed:

- Increment `failure_count` only.
- Write one `switch_events` row.
- Write one `telegram_deliveries` row for that tool, event `logout`, status `sent` or `skipped`. One row even when several chats are on the route. `skipped` when the same username, tool, and event was sent inside `repeat_minutes`.
- If `bot_token` is empty, do not call Telegram. Still store the row.
- If `bot_token` is set, POST the rendered template to each chat. A failed HTTP call still leaves one delivery row.

Return the chosen account id. If no active account remains, return `{ "account_id": 0 }` and do not mark anyone inactive.

## Sessions

One live session per username and website.

- A new login for that pair deletes the previous live session.
- `POST /api/sessions/check` with the session id and a fingerprint. The same fingerprint, even with a new IP, keeps the session. A different fingerprint deletes that session everywhere.
- IP is stored on the session and in Security. It never ends a session by itself.

## Host report

`POST /api/host-reports` records username, website, IP, type (`residential`, `hosting`, `proxy`, or `vpn`), org, and location. The tool stays open. Access stops only after that IP is in `blocked_ips` for that website or for every website (`website_id = 0`). `GET /api/access-check?website_id=&ip=` returns `{ "allowed": true }` or `{ "allowed": false }`.

## Retention

Run this before the matching list queries:

- Quota rows older than that row's `reset_days` are deleted.
- Violations and security rows older than 7 days are deleted.
- Accounts, blocked IPs, and websites are not purged.

## Reseller seed, only if those usernames are missing

Do not touch the 78 tools. Add these operators and point their `operator_websites` at the existing localhost websites. Skip a tool that is not in the table.

| Reseller | Chats | Tools |
|---|---|---|
| ToolWaly | ToolWaly 1, ToolWaly 2, ToolWaly 3 | Semrush, Envato, ChatGPT |
| SemrushToolz | SemrushToolz | Semrush, ChatGPT, Envato, Ubersuggest, Helium 10 |
| SeoGroupBuy | SeoGroupBuy 1, SeoGroupBuy 2 | Semrush |
| AmzPremiumToolz | AmzPremiumToolz | Helium 10, ChatGPT, Claude AI |

Password for each is `toolsmandi`. Each of their websites gets one route with both `logout` and `spam`, to that reseller's chats. The Semrush website whose domain is `127.0.0.1:5141` also includes both SeoGroupBuy chats.

## Done when

`go build` passes. Restart the API. As master, saving a user limit, a blocked IP, a reseller, and a Telegram chat still shows after refresh. `POST /api/websites/{id}/use-account` twice returns an active account and does not set `status` to `inactive`.
