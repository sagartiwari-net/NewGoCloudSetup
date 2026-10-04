Update the existing Update Panel in this folder. Do not rebuild it. Do not edit `pending-tools/ahrefs-admin` or any proxy. Do not add features that are not listed here.

`/Users/sagartiwari/Desktop/oneclickgo/pending-tools/update-panel`

Read `src/lib/format.ts`, `src/components/data-table.tsx`, `src/lib/api/client.ts`, `src/app/(panel)/automate/page.tsx`, `src/app/(panel)/products/page.tsx`, and the other pages under `src/app/(panel)/` before editing.

Follow the shadcn skill. Mock data only. `client.ts` stays the only data boundary.

## 1. Space between actions, and confirm every delete

Row buttons sit against each other, so a click meant for Edit can hit Delete. Every group of Edit, Reset, Delete, Unblock, Open, and Logs uses at least `gap-3`. Dialog footers put Delete on the left and Save on the right, with space between them, never flush.

In the header, the theme switch, the role switch, and Log out are separate groups with space between the groups. Inside one switch (Light | Dark) the segments can stay joined.

Every delete and every unblock opens the existing confirm dialog before `client.ts` is called. The first click only opens the dialog. Account delete, user delete, product delete, website delete, proxy delete, user-agent delete, reseller delete, and unblock all follow this. Reset stays one click.

## 2. Limit text stays short

Lists must not repeat the reset period. People set "every N days" on the tool form, and that form keeps the day input.

Change `formatMeter` so a row reads:

`Credits 120 / 250 · Exports 4 / 20`

No `every 1 day` and no `every 7 days` on User Limits, Quota Logs, or the Website Domains default-limits column. One meter stays `Credits 12 / 100`. The website and user edit forms still show the day count where the value is edited.

## 3. Automate Task: filters, and two tabs

`/automate` is master only. Put **Tasks** and **Logs** as tabs on the top right of the page header. One tab is visible at a time.

**Tasks** is the account table (website, account, task UID, ingest key, last status, last time, bytes, error, Open).

**Logs** is the recent ingest list that currently sits under the table. Remove that stacked card.

Both tabs filter as you type and request one page at a time:

- Search. Tasks match website, account, task UID, and ingest key. Logs match ingest key, account, error, and website.
- Website dropdown, searchable, including All websites.
- Status dropdown. Tasks: All, saved, failed, no ingest yet. Logs: All, saved, failed.

Page size 8. Previous, Next, and `Page X of Y` stay visible. Next is disabled on the last page. Search or filter change resets to page 1.

Seed enough accounts and ingest lines that each tab has a second page.

## 4. One product mapping can hold many ids

A mapping is one website plus one product name plus a list of aMember product ids.

```ts
export type ProductMap = {
  id: number
  website_id: number
  website_name: string
  product_ids: string[]
  product_name: string
}
```

The form has a website picker, a product name, and a way to add more than one id. Each id is its own chip. Add with Enter or a button. Remove a chip with its own control. At least one id is required. The same id cannot be added twice on that mapping, and it cannot already belong to another mapping.

The table shows the ids as chips in one cell, not one row per id. Search matches any id, the name, or the website. Add a website filter. Pagination is one page from `client.ts`, same as the other lists.

Replace the single `product_id` field everywhere it is still used.

## 5. Every list is one page, with filters

These pages must not load the full collection into the table:

- Mapped Accounts
- User Limits
- Active Logins
- Quota Logs
- Analytics, both Login and Switch tabs
- Automate Task and Automate Logs
- Product Mapping
- Violations
- Security
- Blocked IPs
- Proxy Manager
- User Agents
- Website Domains
- Resellers

Each list function in `client.ts` takes `{ page, pageSize, query }` plus the filters that page shows, and returns `{ items, total, page, pageSize }`. The mock slices in the store. The page holds only `items` for the current page. Page size is 8.

`DataTable` always shows Previous, Next, and `Page X of Y`. On one page, Next is disabled. It does not paginate again on top of an already paged array. Pass the rows as the current page and the total from the server result.

Filters, all live as you type:

| Page | Filters |
|---|---|
| Mapped Accounts | search, website |
| User Limits | search, tool, reseller for master, plus the All / Custom tabs |
| Active Logins | search, tool, reseller for master |
| Quota Logs | search, tool, reseller for master |
| Analytics | search, website |
| Automate | as in section 3 |
| Product Mapping | search, website |
| Violations | search, website, reseller for master |
| Security | search, event type, website |
| Blocked IPs | search, website |
| Proxies | search, status |
| User Agents | search |
| Website Domains | search |
| Resellers | search, status |

Reseller login does not show the reseller dropdown. A reseller still only receives rows for their websites.

Seed a second page on Violations, Security, Blocked IPs, Product Mapping, and Analytics if those lists are still shorter than 8.

## Done when

- `npm run build` passes.
- Edit and Delete on a user row have a clear gap. Delete opens a confirm dialog and does not remove the row on the first click.
- User Limits reads `Credits 120 / 250 · Exports 4 / 20` with no day text. The tool form still edits the day count.
- Automate Task switches between Tasks and Logs from the top right. Both have search, website, status, and page 2.
- One product mapping saves two ids and shows both in one row.
- Violations page 2 loads a new slice. The table does not already contain every row.
- Check a 390px width: filters wrap, and action buttons do not sit on top of each other.
