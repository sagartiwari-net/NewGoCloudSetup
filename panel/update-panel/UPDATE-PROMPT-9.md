Update Telegram routing in the existing Update Panel. Do not rebuild the app. Do not edit `pending-tools/ahrefs-admin`.

`/Users/sagartiwari/Desktop/oneclickgo/pending-tools/update-panel`

Logout and spam are two rows for the same tool. Merge them. One website has one route.

```ts
export type TelegramRoute = {
  id: number
  website_id: number
  website_name: string
  events: Array<"logout" | "spam">
  destination_ids: number[]
}
```

The Routing table columns are Tool, Events, Chats, Edit. Events shows `logout, spam` when both are on, or the single selected name. The event filter stays and keeps a row when that event is included. Page size stays 8. One page comes from `client.ts`.

The route form has Tool, then Events as two checkboxes, Logout and Spam. Both start checked on New route. Edit opens with whatever that route already has. At least one event is required. Chats stay grouped by reseller, and a route can still include another reseller's chats.

Saving the same website updates that one route. It does not create a second row.

Seed every current website once, with both events checked, and the same chats as today. `semrush.semrushtoolz.example` is one row whose chats are SemrushToolz plus SeoGroupBuy 1 and SeoGroupBuy 2.

The Sent log stays one row per message. A logout and a later spam are still two sent rows, because those are two messages. Do not merge the Sent table.

`npm run build` must pass. Routing shows one Semrush row per domain, and New route opens with Logout and Spam both checked.
