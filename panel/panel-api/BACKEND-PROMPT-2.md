Finish the remaining Update Panel work except automation. Do not rebuild either app. Do not edit `pending-tools/ahrefs-admin`. Do not wipe `pending-tools/panel-api/data/panel.db`. Do not touch Automate Task, ingest logs, GoAuto, `/api/updateauto`, or `POST /api/websites/{id}/use-account`.

Frontend: `/Users/sagartiwari/Desktop/oneclickgo/pending-tools/update-panel`
API: `/Users/sagartiwari/Desktop/oneclickgo/pending-tools/panel-api`

The panel already calls `http://127.0.0.1:8090` with `Authorization: Bearer <token>`. Keep the JSON shapes in `src/lib/api/types.ts`.

## Leave as they are

78 websites (`127.0.0.1:<port>`) and 78 mapped accounts. Login, tools, accounts, proxies, user agents, user limits, sessions, quota, analytics, violations, security, blocked IPs, resellers, and Telegram chats, routes, and sent log.

## 1. Product Mapping must save

`GET /api/products` and `POST /api/products` in `routes.go` still return an empty page and `{status:ok}` without a row. `DELETE` already removes by id.

Store `website_id`, `product_name`, and `product_ids` (a JSON array). One website plus one name is one row. Saving the same id updates it. The same product id cannot sit on two rows.

`GET` is paged like the other lists (`page`, `pageSize`, `query`, `websiteId`) and returns `{ items, total, page, pageSize }` with `website_name` and `product_ids`. Search matches the name, any id, and the website name. A reseller only sees their websites.

After a refresh, a mapping created in the panel is still there.

## 2. Host report page

`POST /api/host-reports` already inserts. Add `GET /api/host-reports` with the same paging and filters (`query`, `websiteId`, and `type` for `hosting`, `proxy`, `vpn`, `residential`). Each item has `id`, `website_id`, `website_name`, `username`, `client_ip`, `ip_type`, `org`, `location`, `created_at`.

Add a panel page **Host reports** under Safety, route `/host-reports`. Columns: username, tool, IP, location, company, type, time, and **Block**. Block calls the existing `blockIP` for that tool with the reason set to the IP type. The tool stays open until that button is used. `GET /api/access-check` is already the gate. Do not deny access just because the type is hosting or proxy.

Rows: 10, 20, or 50, using the same saved choice as the other lists. Search filters as you type. Previous and Next use `total`.

## Done when

`go build` in `panel-api` passes and `npm run build` in `update-panel` passes. A new product mapping survives refresh. Host reports lists a posted row and Block adds that IP to Blocked IPs. Automate Task is unchanged.
