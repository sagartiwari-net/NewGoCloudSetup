Update the existing Update Panel in this folder. Do not rebuild it. Do not edit `pending-tools/ahrefs-admin` or any proxy.

`/Users/sagartiwari/Desktop/oneclickgo/pending-tools/update-panel`

Read `src/components/app-shell.tsx`, `src/components/ui/sidebar.tsx`, `src/components/account-editor.tsx`, `src/app/(panel)/accounts/page.tsx`, `src/app/(panel)/usage/page.tsx`, `src/app/(panel)/websites/page.tsx`, `src/app/(panel)/proxies/page.tsx`, `src/components/data-table.tsx`, `src/lib/api/types.ts`, and `src/lib/api/mock-store.ts` before editing.

Follow the shadcn skill. Mock data only. `client.ts` stays the only data boundary.

## 1. Quota logs open in a wide dialog

The log list is a narrow side sheet, so the target path is cut off and the time column falls outside the panel. Open logs in a centered dialog instead.

Desktop: dialog wide enough for four columns on one row (`max-w-4xl`). Target path wraps (`break-all`). Time stays on one line. Nothing is clipped and there is no horizontal scroll.

Phone: each deduction is its own card, in this order, every value fully visible:

- Log type
- Usage
- Target path
- Time (IST)

Title stays `username · tool`. Newest first. Empty text stays "No deductions for this user yet."

## 2. Basic tab holds the note and show limit

In the account dialog, move Description and Show limit onto **Basic**, under Status. Basic is then: cookie, user agent, proxy, status, description, show limit.

**Advanced** keeps only GoAuto task UID, automation ingest key, and the read-only ingest details. Master still sees that tab.

When the role is reseller, do not render the Advanced tab at all. The reseller dialog is Basic only. Name and website stay above the tabs.

## 3. Selected state must be obvious

The selected Light, Dark, Master, Reseller, Basic, Advanced, All users, and Custom limits controls are too faint. A selected item uses a solid `bg-primary text-primary-foreground`. The unselected item stays muted. This applies to every `ToggleGroup` used as a mode switch.

Account status is different:

- Active selected: green background, white text
- Inactive selected: red background, white text

Use the same green and red treatment for proxy Active / Inactive. User access Active stays green. Suspended stays red. The color is the selected state, so it is obvious which one is on.

## 4. Saved user agents, same as the proxy pool

Add a master-only page **User Agents** next to Proxy Manager (`/user-agents`). A saved agent has a name and the full user-agent string. Create, edit, delete.

On the account dialog, User agent is a searchable picker of saved agents, plus a button **Use this browser**. That button writes `navigator.userAgent` into the field for this account only. It does not create a pool row.

Accounts reference a pool row by id, they do not copy the string:

```ts
user_agent_id: number | null
user_agent: string
```

`user_agent_id` set means "use that saved agent". Editing the saved string updates every account that points at it. `user_agent_id` null means a one-off string (including Use this browser) that stays on that account when the pool changes.

Do the same for proxies. An account that picks a pool proxy stores `proxy_id`. Editing that pool endpoint updates those accounts. A typed `socks5://` or `http://` URL keeps `proxy_id` null and stays local to the account.

Seed two saved user agents and point some accounts at them so an edit is visible on more than one row.

## 5. Mobile header must not cover the title

On a 390px width the page title sits on top of Light, Dark, Master, Reseller, and Log out. Put the shell controls on their own wrapping row. The page title starts below that row, with space between them. Check Website Domains in light and dark. No overlap, no clipped buttons.

## 6. Website Domains loads one page at a time

This list can grow to 150–200 domains. `listWebsites` returns one page, not the full array.

```ts
listWebsites({ page, pageSize, query }: { page: number; pageSize: number; query: string }): Promise<{
  items: Website[]
  total: number
  page: number
  pageSize: number
}>
```

Page size is 8. The mock slices the filtered list. Previous and Next request the next page from `client.ts`. Show `Page X of Y` from `total`, including when there is only one page (Next disabled). Changing the search resets to page 1.

Seed at least 12 websites so page 2 exists. Canva and the tools with no limits stay in this list. User Limits and Quota still omit tools with no limits.

## 7. Collapsed sidebar keeps the icons

The desktop sidebar currently slides fully away. Set it to icon collapse (`collapsible="icon"`). Collapsed, each route is still an icon with a tooltip of the name. The active route stays highlighted. Group labels hide. Clicking the trigger expands back to labels. Phone still uses the sheet drawer.

Reseller still does not get Automate Task, Proxy Manager, User Agents, Website Domains, or Resellers, including in the icon rail.

## Done when

- `npm run build` passes.
- Logs for nina · Ahrefs show the full target path and the IST time, in a dialog on desktop and in stacked cards at 390px.
- Reseller account edit has no Advanced tab. Description and Show limit are on Basic.
- Active is green and Inactive is red, and the selected theme or role control is clearly filled.
- Editing a saved user agent changes every account that selected it. Use this browser fills only the open account.
- Editing a saved proxy endpoint changes every account that selected that proxy.
- Website Domains has Previous and Next, and page 2 loads without rendering the whole list first.
- Collapsing the sidebar on desktop leaves the icons. The mobile header no longer covers the page title.
