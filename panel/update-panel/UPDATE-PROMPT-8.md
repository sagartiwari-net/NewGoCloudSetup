Update the existing Update Panel in this folder. Do not rebuild it. Do not edit `pending-tools/ahrefs-admin` or any proxy. Do not call the Telegram API.

`/Users/sagartiwari/Desktop/oneclickgo/pending-tools/update-panel`

Read `src/app/(panel)/telegram/page.tsx` and the Telegram section of `src/lib/api/mock-store.ts` before editing.

## Sent log is one row per event

Nina's logout that reached two chats is two rows. That is wrong. One username + one tool + one event + one time is one row, even when several chats receive it.

```ts
export type TelegramDelivery = {
  id: number
  reseller_names: string[]
  website_name: string
  website_domain: string
  username: string
  event: "logout" | "spam"
  recipients: Array<{ label: string; chat_id: string }>
  status: "sent" | "skipped"
  created_at: string
}
```

The table shows Username, Tool, Event, Recipients, Status, Time. Recipients is the labels in one cell: `ToolWaly 1, ToolWaly 2, ToolWaly 3`. Status `sent` means those chats were notified. Status `skipped` means the repeat wait blocked this whole event, still one row. Do not split skipped rows by chat either.

Seed at least 9 events so page 2 exists. Include one event that lists two recipient labels on a single `sent` row.

## Repeat wait is minutes

The field label is `Repeat wait (min)`. The helper text says the number is minutes. Default stays 60.

## Tabs need space

Configuration, Routing, Format, and Sent are separate buttons with a visible gap between them, not one joined pill. Leave space between the tab row and the panel under it. The selected tab stays filled and obvious.

## Resellers, chats, and tools

Replace the Desk / Night seed. Each tool is its own website with its own domain. Two Semrush rows are not the same site. The tool picker shows name, domain, and reseller, for example `Semrush — semrush.semrushtoolz.example — SemrushToolz`.

| Reseller | Chats | Websites |
|---|---|---|
| ToolWaly | ToolWaly 1, ToolWaly 2, ToolWaly 3 | Ahrefs `ahrefs.toolwaly.example`, Semrush `semrush.toolwaly.example`, Envato `envato.toolwaly.example`, ChatGPT `chatgpt.toolwaly.example` |
| SemrushToolz | SemrushToolz | Ahrefs `ahrefs.semrushtoolz.example`, Semrush `semrush.semrushtoolz.example`, ChatGPT `chatgpt.semrushtoolz.example`, Envato `envato.semrushtoolz.example`, Ubersuggest `ubersuggest.semrushtoolz.example`, Helium10 `helium10.semrushtoolz.example` |
| SeoGroupBuy | SeoGroupBuy 1, SeoGroupBuy 2 | Semrush `semrush.seogroupbuy.example` |
| AmzPremiumToolz | AmzPremiumToolz | Helium10 `helium10.amzpremiumtoolz.example`, ChatGPT `chatgpt.amzpremiumtoolz.example`, Claude AI `claude.amzpremiumtoolz.example` |

Chat ids stay obvious fakes (`-1002001` and so on).

A route may include chats from another reseller. Remove the rule that every chat must belong to the tool's owner.

Default routes:

- Each reseller's own websites notify all of that reseller's chats, for both logout and spam.
- `semrush.semrushtoolz.example` also notifies SeoGroupBuy 1 and SeoGroupBuy 2, for both logout and spam. SeoGroupBuy updates that Semrush, so they receive it along with SemrushToolz. SeoGroupBuy's own Semrush (`semrush.seogroupbuy.example`) still goes only to SeoGroupBuy's two chats.

The route form lists every chat, grouped by reseller name, not only the owner's chats. Saving the same tool and event updates the one existing route.

## Done when

- `npm run build` passes.
- The sent table has one row for a message that reached several chats, and that row's status is `sent`.
- Repeat wait reads `(min)`.
- The four tabs have a gap and do not sit in one stuck-together control.
- The chat list shows ToolWaly three times, SeoGroupBuy twice, and SemrushToolz and AmzPremiumToolz once.
- Editing the route for `semrush.semrushtoolz.example` shows SemrushToolz's chat and both SeoGroupBuy chats already checked.
