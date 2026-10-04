Update the existing Update Panel in this folder. Do not rebuild it. Do not edit `pending-tools/ahrefs-admin` or any proxy. Do not call the Telegram API.

`/Users/sagartiwari/Desktop/oneclickgo/pending-tools/update-panel`

Read `src/app/(panel)/accounts/page.tsx` and `src/app/(panel)/telegram/page.tsx` before editing.

Follow the shadcn skill. `client.ts` stays the only data boundary. Each Telegram tab loads only its own page of data when that tab is open. Do not fetch destinations, routes, and the sent log in one request.

## 1. Remove the cookie badges

On Mapped Accounts, delete the Fresh, Stale, Failed, and Never badges. Keep the **Cookie updated** time. An account with no save shows "—". Do not replace the badges with another word.

## 2. Telegram is four tabs

Replace the single scrolling Telegram page with tabs. The tab content is the only block under the title. Opening a tab resets nothing on the other tabs, and it requests only that tab's list.

Reseller login still sees only their own chats, routes, and sent rows. They do not see the bot token. Master sees every reseller.

### Configuration

Bot token stays here, masked, master only.

Under it, **Repeat wait**: one number, minutes, master only. Default 60. Meaning: after a message is sent for a username + tool + event (logout or spam), no second message for that same trio until the wait has passed. The sent log records the skipped one as `skipped`.

**Chats** is a paged table, not a nickname like "chatid1". Each row is:

| Reseller | Label | Chat id | Enabled |
|---|---|---|---|

The label is a short name the operator types, such as `Desk` or `Night`. The same reseller has as many rows as they need. Example: reseller / Desk / `-100111` and reseller / Night / `-100222`.

Add, edit, delete. Delete confirms. Search matches label, chat id, and reseller name. Master also filters by reseller. Page size 8. Previous, Next, and `Page X of Y`.

`listTelegramDestinations` takes `{ page, pageSize, query, resellerId }` and returns one page.

### Routing

This tab answers which tool sends which event to which chats. It does not edit the bot or the chat id.

```ts
export type TelegramRoute = {
  id: number
  website_id: number
  website_name: string
  event: "logout" | "spam"
  destination_ids: number[]
}
```

A route's chats must belong to the reseller who owns that website. Ahrefs cannot notify a Semrush partner chat.

Table columns: Tool, Event, Chats (the labels, not only the raw ids). Search by tool. Filters: event, and reseller for master. Page size 8, one page from `client.ts`.

The form picks one tool, one event, and one or more of that reseller's chats. Saving a second route for the same tool and event updates the existing row instead of duplicating it.

Seed Ahrefs logout to Desk and Night. Ahrefs spam only to Desk. Semrush logout only to that partner's chat.

### Format

Two templates stay here: Logout and Spam report. Master edits. Resellers see the preview only.

Placeholders: `{username}`, `{tool}`, `{website}`, `{time}`, `{ip}`, `{reason}`, `{count}`.

Keep the current default sentences. Preview fills them with sample values. This tab has no table and no pager.

### Sent

One row per attempt:

```ts
export type TelegramDelivery = {
  id: number
  reseller_id: number
  reseller_name: string
  website_name: string
  username: string
  event: "logout" | "spam"
  destination_label: string
  chat_id: string
  body: string
  status: "sent" | "skipped"
  created_at: string
}
```

`skipped` means the repeat wait blocked a duplicate for that username, tool, and event.

Search matches username, tool, label, and chat id. Filters: event, status, and reseller for master. Page size 8 from `listTelegramDeliveries`. Do not return the full history to the page.

**Clear** asks for confirmation, then deletes the deliveries. It does not delete chats, routes, or templates.

Seed more than 8 rows, including one skipped row, so page 2 exists. A delivery older than the story is fine. Do not load them until this tab is open.

## Done when

- `npm run build` passes.
- Mapped Accounts shows the cookie time and does not show Fresh, Stale, Failed, or Never.
- Telegram opens on Configuration only. Routing, Format, and Sent are not in that first response.
- One reseller can have Desk and Night as two chats. Ahrefs logout lists both labels. Sent has search, filters, page 2, and Clear.
- A reseller session does not show another reseller's chats or the bot token.
