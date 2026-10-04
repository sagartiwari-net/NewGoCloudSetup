Update the existing Update Panel frontend. Do not rebuild it. Do not edit `pending-tools/ahrefs-admin`. Do not switch data back to the mock store.

`/Users/sagartiwari/Desktop/oneclickgo/pending-tools/update-panel`

The panel already talks to the Go API at `http://127.0.0.1:8090`. Login is `master` / `toolsmandi`. The API reads `page` and `pageSize` and returns `{ items, total, page, pageSize }`. Keep that. Every list request must send the size the user picked.

## Already done

- 78 tools are in SQLite, each domain is `127.0.0.1:<port>` (Canva 4501, Semrush 5141, ChatGPT 5151, and the rest).
- 78 mapped accounts exist, one per tool, cookie loaded from that tool's `cookie.txt` or `cookie.json`.
- A signed-out API response sends the browser to `/login`.
- Lists are still hardcoded to 8 rows. That is the only change in this task.

## Rows per page

Every paged report gets a **Rows** control next to Previous / Next: `10`, `20`, `50`.

- One choice for the whole panel, saved in `localStorage`, default `10`.
- Changing it sets the page back to 1 and fetches that many rows from the API. Do not slice a bigger array in the browser.
- The label under the table stays `Page X of Y`, using `total` and the selected size.
- Phone: the control wraps onto its own line. It does not cover the page buttons.

Apply it on every list that already passes `pageSize: 8`:

Mapped Accounts, User Limits, Active Logins, Quota Logs, Analytics (Logins and Switches separately), Automate Task (Tasks and Logs), Product Mapping, Violations, Security, Blocked IPs, Proxy Manager, User Agents, Website Domains, Resellers, and the three Telegram tables (Chats, Routing, Sent).

`DataTable` must use the passed `pageSize` for its pager. It must not fall back to 8.

## Done when

- `npm run build` passes.
- Mapped Accounts opens with 10 rows. Choosing 50 reloads page 1 with 50 accounts. Refresh keeps 50.
- Website Domains and the Sent tab use the same choice.
