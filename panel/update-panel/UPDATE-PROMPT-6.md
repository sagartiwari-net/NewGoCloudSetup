Update the existing Update Panel in this folder. Do not rebuild it. Do not edit `pending-tools/ahrefs-admin` or any proxy. Do not call the Telegram API. This pass is the management UI and mock data only.

`/Users/sagartiwari/Desktop/oneclickgo/pending-tools/update-panel`

Read `src/components/app-shell.tsx`, `src/app/(panel)/analytics/page.tsx`, `src/app/(panel)/profile/page.tsx`, `src/lib/api/types.ts`, and `src/lib/api/client.ts` before editing.

Follow the shadcn skill. `client.ts` stays the only data boundary.

## 1. Header becomes a profile menu

Remove Light, Dark, Master, Reseller, and Log out from the header bar. The header keeps the sidebar trigger on the left and, on the right, one button: avatar plus the username. On a 390px width show only the avatar, so the title never collides with it.

Clicking that button opens a dropdown aligned to the right:

- Theme, as a Light / Dark switch inside the menu. The selected side stays clearly filled.
- View as, Master / Reseller, the same mock role switch that exists today.
- Change password, a link to `/profile`.
- Log out.

The password form on `/profile` stays as it is. The menu only navigates there.

## 2. Analytics chart, room for the tables, and Clear

The Logins per day chart is a row of flat grey blocks. Draw real bars in the theme primary color, with a gap between bars, a Y axis that starts at 0, and a tooltip that shows the day and the login count. Bar height must follow the count. Give the chart a max height of about 220px so it does not eat the page.

Under the chart, the Logins and Switches tabs and their tables use the full content width. Put the tab control and a **Clear** button on the same row, Clear at the right of the active tab. Clear asks for confirmation, then deletes only that tab's rows (logins or switches) through `client.ts`. The other tab stays. After clear, the table is empty and the chart drops to zero for logins. Cancelling the dialog deletes nothing.

## 3. Telegram alerts page

Add `/telegram` to the sidebar for both master and reseller. Label it Telegram.

A reseller only sees and edits destinations whose `reseller_id` is their own. They never see the bot token or another reseller's chats. Master sees every destination and the bot token.

One panel bot sends every message. Master has a masked field **Bot token** on this page. Saving it stores the fake token in the mock. Resellers do not get this field.

### Destinations

A destination is one Telegram chat. A reseller adds as many as they want for the same tool.

```ts
export type TelegramDestination = {
  id: number
  reseller_id: number
  reseller_name: string
  label: string
  chat_id: string
  website_ids: number[]
  events: Array<"logout" | "spam">
  enabled: boolean
}
```

`website_ids` empty means every website that reseller can access. Otherwise only the checked tools.

Form fields: label, chat id, tools (searchable multi-select, plus "All my tools"), events (Logout and Spam report, both can be on), enabled. Create, edit, delete. Delete confirms.

Seed the reseller who owns Ahrefs and SeoSite Checkup with two chats, both receiving Ahrefs logout, and only one of them receiving spam. Seed a second reseller with one chat for Semrush only. Master sees all three. Reseller login sees only their own.

### Templates

Two templates, edited on the same page. Master edits them. Resellers can read the preview, not the editor.

- Logout
- Spam report

Placeholders they can insert: `{username}`, `{tool}`, `{website}`, `{time}`, `{ip}`, `{reason}`, `{count}`.

Defaults:

```text
Logout
{tool} logged out for {username} on {website} at {time}.

Spam report
{username} may be sharing {tool}. {count} opens in the window from {ip}. {reason} at {time}.
```

A preview under each template fills the placeholders with sample values so the operator sees the exact message.

### What triggers a message later

Show this as a short note on the page, not as a second product:

The Go proxy reports a logout or a spam hit to the panel. The panel finds the reseller who owns that website. It sends the rendered template only to that reseller's enabled destinations whose tool list includes the website and whose events include that type. Other resellers get nothing. Two chats on the same tool both receive it.

Spam rule, editable by master, visible to resellers:

```ts
export type SpamRule = {
  window_minutes: number
  max_opens: number
  max_distinct_ips: number
}
```

Default: 10 minutes, 8 opens, 2 different IPs. A hit is when one username crosses either number on one tool inside the window. That hit becomes one spam report. The proxy will enforce this later. The panel only stores the rule.

Under the rule, a **Recent reports** list of mock rows: username, tool, reason, time, and which chat labels would have received it. A reseller sees reports only for their websites.

Add `saveTelegramDestination`, `deleteTelegramDestination`, `listTelegramDestinations`, `saveTelegramTemplates`, `saveSpamRule`, `listSpamReports`, and `saveBotToken` on `client.ts`. No `fetch` to Telegram.

## Done when

- `npm run build` passes.
- At 390px the header is the menu button and the page title. Light, Dark, and Log out are inside the menu. Change password opens `/profile`.
- The analytics bars are colored and uneven when the daily counts differ. Clear on Logins asks, then empties logins only.
- As reseller, Telegram shows only that reseller's chats, and a second chat can be added for the same tool. As master, the bot token and both resellers' chats are visible. The logout preview contains the sample username and tool name.
