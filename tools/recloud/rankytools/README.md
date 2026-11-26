# RankyTools (recloud)

Group-buy brand: [rankytools.com](https://rankytools.com/) — multi-tool portal (Semrush, Moz, Ahrefs, KE, etc.).

## Agreed flow

1. **Local proxy only** — cookie + rewrite until the tool opens clean on `127.0.0.1`
2. **Security** — panel gate / device bind / logout → automation (same as Markhor / Nox)
3. **Server** — GitHub → `build-one` / `start-tool` / nginx map / panel website row

Do **not** add panel auth or deploy until local is green for that tool.

## Layout

```
tools/recloud/rankytools/
  README.md          ← this file
  URLS.md            ← fill as URLs arrive (one tool at a time)
  <tool>/            ← one Go proxy per tool (e.g. ryksm1, rykrefs)
```

Slug prefix: **`ryk`** (Ranky). Example: Semrush → `ryksm1`, Ahrefs → `rykrefs`.

## Required on every Ranky tool (incl. new ones)

**Logout detection** — dead `PHPSESSID` makes Ranky 302 to `https://member.rankytools.com?return_url=…`. If that Location is rewritten onto the local/tool origin, Chrome hits `ERR_TOO_MANY_REDIRECTS`.

Every `ryk*` proxy must:

1. Keep `member.rankytools.com` / `.co` **out** of host rewrite (`__TM_MEMBER_HOST__` guard in `rewriteText`)
2. On upstream 302: `isRankyMemberLogout` / `isRankyReturnURLBounce` → `renderLoggedOutPage` (“Updating tools”)
3. Early in `handle`: nested `?return_url=` document nav → same logged-out page
4. Rewrite **bare** Ranky hostname too (`upstreamHost` → public host) so Cloudflare 502 pages don’t show `sw.rankytools.com` / etc.

Copy from `rykns` (`logout.go` helpers + `proxy.go` / `main.go` wiring) when scaffolding the next tool.

## Semrush (first) — local

```bash
cd tools/recloud/rankytools/ryksm1
./start-local.sh
# → http://127.0.0.1:6500/home/
```

Cookie: `ryksm1/cookie.json` (gitignored). Refresh cookie if session dies.

## ChatGPT — local (port 6502)

```bash
cd tools/recloud/rankytools/rykcp
./start-local.sh
# → http://127.0.0.1:6502/
```

Cookie: `rykcp/cookie.json` (same RankyTools `PHPSESSID`). KE (`rykke`) removed — revisit later if captcha solvable.

## Ubersuggest — local

```bash
cd tools/recloud/rankytools/rykus
./start-local.sh
# → http://127.0.0.1:6503/en/dashboard
```

Cookie: `rykus/cookie.json` (same RankyTools `PHPSESSID`).

## Grammarly — local

```bash
cd tools/recloud/rankytools/rykgm
./start-local.sh
# → http://127.0.0.1:6504/
```

Cookie: `rykgm/cookie.json` (same RankyTools `PHPSESSID`).

## Envato Elements — local

```bash
cd tools/recloud/rankytools/ryken
./start-local.sh
# → http://127.0.0.1:6505/
```

Cookie: `ryken/cookie.json` (same RankyTools `PHPSESSID`).

## Storyblocks — local

```bash
cd tools/recloud/rankytools/ryksb
./start-local.sh
# → http://127.0.0.1:6506/
```

Cookie: `ryksb/cookie.json` (same RankyTools `PHPSESSID`).

## SpyFu — local

```bash
cd tools/recloud/rankytools/ryksf
./start-local.sh
# → http://127.0.0.1:6507/
```

Cookie: `ryksf/cookie.json` (same RankyTools `PHPSESSID`).

## Niche Scraper — local

```bash
cd tools/recloud/rankytools/rykns
./start-local.sh
# → http://127.0.0.1:6508/
```

Cookie: `rykns/cookie.json` (same RankyTools `PHPSESSID`).

## SpamZilla — local

```bash
cd tools/recloud/rankytools/ryksz02
./start-local.sh
# → http://127.0.0.1:6509/private-domains/
```

Cookie: `ryksz02/cookie.json` (same RankyTools `PHPSESSID`). Includes Ranky logout detection.

## SimilarWeb — local

```bash
cd tools/recloud/rankytools/ryksw
./start-local.sh
# → http://127.0.0.1:6510/
```

Cookie: `ryksw/cookie.json` (same RankyTools `PHPSESSID`). Includes Ranky logout detection.

## Next tools

Send tool name + member URL + cookie; we add `ryk…` the same way (always include logout detection).

## Server (upays.in)

Ports **6500–6510** (next recloud block after seoshop `6400`). Security: panel access link + device fingerprint (`bypass_auth=false` on server).

```bash
cd /www/wwwroot/gt4rents.com/_repo && git pull --ff-only origin main
./deploy/push-rankytools-ryk-tools.sh
```

Then Panel → Recloud → set cookie / upstream for each `ryk*` row.
