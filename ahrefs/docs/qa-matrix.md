# Local QA matrix (Phase 8)

Run against local `ahrefs-bar-api` + unpacked extension + panel Ahrefs Bar UI.

| # | Test | Expect | Result |
|---|------|--------|--------|
| 1 | No aMember cookie | Extension not authed | |
| 2 | Valid cookie + product | Authed + `/v1/me` limits | |
| 3 | Wrong product | Reject / no_access | |
| 4 | Proxy icon/header | 200 via local API | |
| 5 | Billable × N | meters increase | |
| 6 | Hourly cap | LimitReached hourly | |
| 7 | Daily cap | LimitReached daily | |
| 8 | Custom user limits | override defaults | |
| 9 | Account 401 | failover + log | |
| 10 | SERP then open domain | browser cache hit | |
| 11 | Fresh / Refresh | bypass cache | |
| 12 | Reseller A vs B (panel) | no data leak | |
| 13 | Page spam refresh | cheap/cached; billable stable | |
| 14 | Fingerprint off (default) | verify OK without FP | |
| 15 | `AHREFS_BAR_REQUIRE_FP=1` + mismatch | fingerprint_mismatch | |
| 16 | Pool empty | DataSourceUnavailable + Telegram (if configured) | |
| 17 | Panel CRUD account/portal/defaults | save + mask tokens | |

## Security / ops checklist (PLAN §13)

- [x] Token mask in panel list (never full Bearer in table)
- [x] Verify rate-limit (`AHREFS_BAR_VERIFY_RATE`, default 60/min/IP)
- [x] CORS: extension + localhost + panel origins
- [x] IST meters (Asia/Kolkata)
- [ ] Prod: encrypt tokens at rest / DB perms
- [ ] Prod: backups for `ahrefs_bar_accounts`
- [x] Usage log never stores full Bearer
