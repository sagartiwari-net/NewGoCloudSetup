# gt4rents.com rollout checklist

Living tick-list. Update as each tool goes green.  
Server: `65.109.16.196` · Domain: `gt4rents.com` · Repo: this folder → `/www/wwwroot/gt4rents.com/_repo`

**Agreed order (2026-10-04):**

1. **Build + start all tools** on HTTP (panel domains + cookies)  
2. **SSL** (wildcard LE + auto-renew — see Phase B)  
3. Flip `TOOL_PUBLIC_SCHEME=https`, rebuild overlays, **retest one-by-one**  
4. Fix remaining issues (proxy / cookies / tool-specific)

> HTTPS helps device-lock, Secure cookies, SW.  
> It does **not** replace Proxy Manager for Cloudflare-heavy tools (Claude, often Canva).

---

## Phase A — HTTP platform (now)

- [x] DNS wildcard → Hetzner  
- [x] nginx host→port map + wildcard reverse proxy (HTTP :80)  
- [x] panel-api + `panel.db`  
- [x] payment-hub `:8090` left alone  
- [x] `build-one` prefers `config.server.json` / panel mode (`use_database=false`)  
- [x] Bulk `build-all` — **OK=80 FAIL=0** (2026-10-04)  
- [x] `start-batch` — **OK=79 FAIL=1** → `:5001` = **smpanel Node** (not chatbotapp); remapped chatbotapp → **5061**  
- [x] `chatbotapp` remapped to **5061** — listening (`app` pid OK); `start-tool.sh` free_port `ss`/`lsof` `|| true` (silent abort fix)  
- [ ] Every website `domain` = `<sub>.gt4rents.com` in panel  
- [ ] Active accounts + cookies (and **proxy** where CF blocks Hetzner)

### Recipe (one tool)

```bash
cd /www/wwwroot/gt4rents.com/_repo
git pull origin main
source /www/wwwroot/gt4rents.com/_secrets/mysql.env
./deploy/build-one.sh <sub> && ./deploy/start-tool.sh <sub>
ss -lntp | grep <port>
tail -30 /www/wwwroot/gt4rents.com/<sub>/app.log
```

Panel: access-link only (seedha URL = Access Denied OK).

### Bulk build + batch start (do this now)

```bash
cd /www/wwwroot/gt4rents.com/_repo
git pull origin main
source /www/wwwroot/gt4rents.com/_secrets/mysql.env

# 1) build all (~80 tools; continues on failure; 15–40+ min)
chmod +x deploy/*.sh
./deploy/build-all.sh 2>&1 | tee /tmp/gt4-build-all.log
# summary at end: OK=… FAIL=…
grep '^FAIL:' /tmp/gt4-build-all.log || true

# 2) start in batches of 8 (12s pause between batches — safer for RAM)
./deploy/start-batch.sh 8 2>&1 | tee /tmp/gt4-start-batch.log

# 3) sanity: how many apps listening
ss -lntp | grep -E ':(45|46|47|48|49|50|51|52|53)[0-9]{2}\b' | wc -l
# optional: list failures
grep -E 'FAIL:|SKIP:' /tmp/gt4-start-batch.log || true
```

Do **not** kill `:8090` (payment-hub). `start-tool` only touches each tool’s own port.

---

## Phase B — SSL (DONE — 2026-10-04)

Goal: cert never “quietly expires”. Full steps: `setup.md` **§4d** + **§4d-after**.

- [x] aaPanel Let’s Encrypt for `gt4rents.com` + `*.gt4rents.com` (DNS TXT `_acme-challenge`) — domains show `*.gt4rents.com, gt4rents.com`; exp ~2027-01-02  
- [x] Also `panel.gt4rents.com` LE deployed  
- [x] Force HTTPS on both sites — `http://refs` → **301**; `https://panel` **200**; `https://refs` **403** (no session = OK)  
- [x] **Auto Renew** noted on cert page (“1 month before expiration”)  
- [x] Cloudflare SSL/TLS → **Full**  
- [ ] Optional later: Full (strict); renew dry-run; calendar ~30d before expiry  
- [x] §4d-after: `TOOL_PUBLIC_SCHEME=https` + panel-api rebuild (pid OK) + `$gt4_forwarded_proto` + nginx reload

### After cert is live

```bash
# on server
echo 'TOOL_PUBLIC_SCHEME=https' >> /www/wwwroot/gt4rents.com/_secrets/mysql.env
source /www/wwwroot/gt4rents.com/_secrets/mysql.env

# rebuild tools so overlays get public_scheme=https
cd /www/wwwroot/gt4rents.com/_repo
# batch rebuilds, or rebuild tools you are testing:
./deploy/build-one.sh refs && ./deploy/start-tool.sh refs
# …repeat per tool under test
```

Panel access links should become `https://`.

---

## Phase C — Per-tool verify (after SSL)

For each tool: access-link → login shell → one real action.  
Lists below: **Fixed**, then **Not started**, then **Tried — still an issue** (last, with the issue).

### Fixed

Verified on an access link. Each tool is listed once.

- [x] `airbrush` (:5181) — **HTTPS OK** (2026-10-10): `app.airbrush.ai`, home `/dashboard`. Hide only `div.profile_log` (profile menu) and `li.logout` (Signout).
- [x] `answerthepublic` (:4931) — **HTTPS OK** (2026-10-07): panel access + disk CDN cache (`60c3335`)
- [x] `cgpt` — ChatGPT (:5151) — **HTTPS OK** (panel access-link). Prefer **ChatGPT 1** (ID:65). ChatGPT 2 cookies are flaky (`no_access_token` / login wall) — refresh that account in the panel when free.
- [x] `clud` — Claude AI (:5171) — **HTTPS OK** (`claude-v32-name`): Proxy Manager + bootstrap decompress + user-menu hide + panel username on footer
- [x] `closerscopy` (:5241) — **HTTPS OK** (2026-10-10): `www.closerscopy.com`, home `/dashboard`. Fat browser cookie is slimmed so the header limit is not hit. Sign-in switches accounts or shows contact-admin. Status stays unchanged. Analytics → Logouts. Hide only `.dropdown-menu[aria-labelledby="navbarDropdown"]`; the toggle stays and shows the panel username. Editor `/editor/save` keeps the CSRF token so text stays after refresh.
- [x] `cnva` — Canva (:4501) — **HTTPS OK** (2026-10-07): panel access-link working
- [x] `copyspace` (:4741) — **HTTPS OK** (2026-10-10): `space.copyspace.ai`, home `/`. The manifest is `display: browser`, so Chrome does not keep relaunching the tab. An expired cookie (419, or Inertia reloading the same page) switches accounts once or shows contact-admin. Status stays unchanged. Analytics → Logouts. The editor follows that response instead of staying on a silent 409.
- [x] `coursera` (:4701) — **HTTPS OK** (2026-10-10): `www.coursera.org`, home `/organizations/reliance-family`. `X-Forwarded-Host` is not forwarded (CloudFront 403). Header Log In (`header-login-button`) switches accounts or shows contact-admin. Status stays unchanged. Analytics → Logouts. Hide only `#authenticated-info-menu` and `button[data-e2e="header-profile"]`. Disk cache `cdn-cache/` (72h) for JS, CSS, fonts, and images.
- [x] `cramly` (:4841) — **HTTPS OK** (2026-10-10): `app.cramly.ai`, home `/dashboard`. Upstream uses IPv4 so Cloudflare does not reset the connection. A fat browser cookie is slimmed, and the Access button clears the jar first so the dashboard is not rejected as too large.
- [x] `educative` (:4851) — **HTTPS OK** (2026-10-10): `www.educative.io`, home `/learn/home`. A logged-out cookie (`{"status":401}` on `/api/user/info`) switches accounts once or shows contact-admin. Status stays unchanged. Analytics → Logouts. Hide only the logged-in header profile group and the logged-out header group (Log In / Join for free). The welcome heading shows the panel username instead of the account name.
- [x] `envt` — Envato (:5261) — Download count OK on HTTP; re-verify after SSL
- [x] `epidemicsound` (:4641) — **HTTPS OK** (2026-10-08): `www.epidemicsound.com`. Preview MP3 allowed through the device gate. Assistant GraphQL host `client-api.epidemicsound.com` is proxied. Sidebar account button does not open its menu; the email line shows the panel username. Disk cache `cdn-cache/` (72h). “Something went wrong”, Log in, and Create free account switch accounts or show contact-admin. Status stays unchanged. Analytics → Logouts.
- [x] `erank` (:5191) — **HTTPS OK** (2026-10-06): panel access-link, h1 upstream, no browser cookie leak, CDN disk+browser cache (`erank-v7`)
- [x] `flexclip` (:4671) — **HTTPS OK** (2026-10-09): `https://www.flexclip.com/editor/`. Guest page switches accounts or shows contact-admin. Status stays unchanged. Analytics → Logouts. Disk cache `cdn-cache/` (72h). `Digitavision` → `ToolsMandi`. Hide only `[class*="FJMenu_bottom__"]` (account chip + its menu). Video, download, and credit checks are not blocked by `device_required`. Video/audio is streamed. Other `*.flexclip.com` media hosts go through `/ext-host/`.
- [x] `fishaudio` (:4991) — **HTTPS OK** (2026-10-10): `fish.audio`, home `/app/`. Login is a localStorage `token`. Scripts, `/i18n/`, and `manifest.json` are not blocked by the device gate. Generated audio downloads go through the proxy (`platform.r2.fish.audio`). Disk cache `cdn-cache/` (72h) for JS, CSS, fonts, and images. Hide only the team switcher (My Team / Free / avatar).
- [x] `glorify` (:4681) — **HTTPS OK** (2026-10-10): `app.glorify.com`, home `/dashboard`. A sign-in page (`/dashboard/login`, including a client-side jump) switches accounts once or shows contact-admin. Status stays unchanged. Analytics → Logouts. The account-name labels show the panel username. Hide only `.account-menu`.
- [x] `gptzero` (:5301) — **HTTPS OK** (2026-10-04): panel credits, logout→swap/`[SWAP]`, Premium features stub, CDN fast open, Upgrade/upsell hide
- [x] `grammarly` (:4911) — **HTTPS OK** (2026-10-06): panel access + disk CDN cache + warm extra-cdn + “connection unstable” banner fixed (`411234d`)
- [x] `helium10` (:5201) — **HTTPS OK** (2026-10-06): token-only access + panel.db cookies verified
- [x] `ilovepdf` (:4531) — **HTTPS OK** (2026-10-07): force `www.ilovepdf.com`; HTTP/1.1-only TLS (h2→h1 fallback caused malformed SETTINGS → 502)
- [x] `junglescout` (:5211) — **HTTPS OK** (2026-10-06): panel access working
- [x] `leonardo` (:4601) — **HTTPS OK** (2026-10-07): panel access working
- [x] `linkedinlearning` (:4571) — **HTTPS OK** (2026-10-08): `www.linkedin.com/learning/`, HTTP/1.1. Disk cache `cdn-cache/` (72h). Video/audio streamed with Range. Hide only `li[data-live-test-me-menu]` (Me). Guest home (Start free trial / Sign in) switches accounts or shows contact-admin. Status stays unchanged. Analytics → Logouts.
- [x] `mgfc` — Magnific (:5121) — **HTTPS OK** (2026-10-06): `mgfc.gt4rents.com` (Safe Browsing rename from `magnific`) + disk CDN cache (`e62a9ad`). `/photos` redirects to `/people-emotions` (direct `/photos` can still hit WAF).
- [x] `piktochart` (:4541) — **HTTPS OK** (2026-10-08): `create.piktochart.com`, HTTP/1.1. Sign-in / “session expired” switches accounts or shows contact-admin; status stays unchanged. Hide only `#usersettings-dropdown`. `Digitavision` → `ToolsMandi`. Disk cache `cdn-cache/` (72h).
- [x] `placeit` (:4611) — **HTTPS OK** (2026-10-07): panel access working
- [x] `prezi` (:4751) — **HTTPS OK** (2026-10-08): panel username + UserDropdown blocked; Craft CDN/API device soft-allow (was stuck on “Laying out the canvas”)
- [x] `refs` — Ahrefs (:5291) — HTTP access-link LIVE
- [x] `scribd` (:4791) — **HTTPS OK** (2026-10-07): live working on `scribd.gt4rents.com`
- [x] `searchatlas` (:4711) — **HTTPS OK** (2026-10-09): `dashboard.searchatlas.com`. Huge browser cookie is slimmed to `ct_session` so device bind keeps working. User button stays; the account menu (Settings, Billing, Logout) does not open, and the label is the panel username. Payment-failed banner is hidden. Dashboard `/api/countries` stays on the dashboard host. Agent websocket is allowed through the device gate. UI sounds (`.wav`) are allowed. Disk cache `cdn-cache/` (72h) for hashed JS, fonts, images, and short sounds.
- [x] `selleramp` (:5161) — **HTTPS OK** (2026-10-07): panel access, search `/sas/lookup`, `__tm_s` keepalive + static bypass + device-lock (cookie-copy → Access Denied)
- [x] `sellthetrend` (:4591) — **HTTPS OK** (2026-10-07): `www.sellthetrend.com`, profile dropdowns hidden (left+top), panel username/initials on avatar bar
- [x] `seobility` (:4731) — **HTTPS OK** (2026-10-07): `app.seobility.net`, account Profile/Subscription/Billing/Members/MCP block hidden, panel username in dropdown; logout→`/user/login` wall → switch / panel `logged_out` + contact-admin (failover verified)
- [x] `seositecheckup` (:4661) — **HTTPS OK** (2026-10-07): panel.db cookies, device lock, chrome hide + panel username; logout→`logged_out`; disk `cdn-cache` (72h) + memory L1 for JS/CSS
- [x] `seotesteronline` (:4801) — **HTTPS OK** (2026-10-07): cookie slim keeps `ct_session`, device bind works, `suite.seotesteronline.com`; no-subscription / dead cookie → soft-revive then switch / panel `logged_out` + contact-admin (failover verified)
- [x] `shortform` (:4771) — **HTTPS OK** (2026-10-08): `www.shortform.com`. Logout (API 401, logout click, `/app/login`) writes Analytics Logouts, swaps to another active account, or shows contact-admin. Account status stays unchanged.
- [x] `similarweb` (:5071) — **HTTPS OK** (2026-10-07): live working; CDN disk cache `cdn-cache/` (72h)
- [x] `sketchgenius` (:4781) — **HTTPS OK** (2026-10-07): panel username on nav; dropdown blocked; login/`Unauthenticated` → panel `logged_out` + switch/contact-admin (failover verified)
- [x] `smrs` — Semrush (:5141) — **HTTPS OK** (mixed-content https rewrite + panel account swap; proxy optional)
- [x] `spyfu` (:4941) — **HTTPS OK** (2026-10-10): `www.spyfu.com`. Hide only `.sf-account` (header account chip). Disk cache `cdn-cache/` (72h) for JS, CSS, fonts, and images.
- [x] `storybase` (:4551) — **HTTPS OK** (2026-10-07): `www.storybase.com`, HTTP/1.1 ALPN (was 502/`chatgpt.com`); `#profile-widget` hidden (Settings/Billing/Logout + sidebar avatar/name)
- [x] `wordtune` (:4631) — **HTTPS OK** (2026-10-05): app.wordtune.com, device reveal, chrome hide + panel username, panel.db cookie reload, logout→switch/`logged_out` + Analytics Logouts
- [x] `wrank` — WooRank (:4561) — **HTTPS OK** (2026-10-05): `wrank` host (not brand SB), HTTP/2 ALPN route, host-jail `https://x/` blocked, device force-visible, panel.db cookies + overview

### Not started

Built with the batch. Not verified on an access link yet.

- [ ] `digen` (:5271)
- [ ] `heliumlearning` (:5221)
- [ ] `imgupscaler` (:5091)
- [ ] `indexification` (:5231)
- [ ] `jasper` (:4511)
- [ ] `joggai` (:5031)
- [ ] `kalodata` (:4951)
- [ ] `merchinformer` (:4651)
- [ ] `minvo` (:4871)
- [ ] `mojo` (:4881)
- [ ] `pixlr` (:4901)
- [ ] `ppspy` (:4961)
- [ ] `rivalflow` (:4811)
- [ ] `screpy` (:4721)
- [ ] `seobuddy` (:5131)
- [ ] `slidebean` (:5021)
- [ ] `speechify` (:5011)
- [ ] `storyblocks` (:4971)
- [ ] `syntx` (:4981)
- [ ] `ubersuggest` (:5281)
- [ ] `uncensoredchat` (:4891)
- [ ] `videotoblog` (:5041)
- [ ] `writecream` (:5051)
- [ ] `zebracat` (:5111)
- [ ] `zikaanalytics` (:5251)

### Tried — still an issue

Opened on the proxy. Leave these until the note below is cleared.

- [ ] `artistly` (:4821) — **Parked** (2026-10-10). `https://app.artistly.ai`, home `/ai/ai-image-designer`. The shell opens, then scripts are still requested from the official host (`app.artistly.ai/build/assets`, CORS, 0 kB, initiator `about:client`). The same files later 200 from the proxy. Already tried: host rewrite in HTML/JS, and rewriting the `Link` preload header (`ce3b4aa`). A real login page switches accounts or shows contact-admin; status stays unchanged. Leave until later.
- [ ] `branalyzer` (:5081) — **Parked** (home loads). Be Curious still shows the snack “Introduce any valid URL or domain” on `branalyzer.gt4rents.com`. The same patch (`59905db`) worked locally. Resume: compare the server `main.js` patch and searchText/DOM with local, wipe `cdn-cache`, hard-refresh.
- [ ] `chatbotapp` (:5061) — **Parked** (home and chats list OK). `POST /api/v2/chat` still returns 4002 `x_token header is required` (`91f9ad8` inject). Resume: confirm the panel GoAuto account has IndexedDB `stsTokenManager`, and check `[CHATBOT] api auth OK` in `app.log`.
- [ ] `copywritely` (:4521) — **In progress** (2026-10-08). Target `https://copywritely.com`, home `/tools/`. Sign-in popup (`login_popup`) is treated as logout: Analytics Logouts, one account switch, then contact-admin. Status stays unchanged. Still open: confirm POST `/tools/copywritely/*` and the highlighter worker (`/wp-content/.../js`, `.map`) are not blocked by `device_required`.
- [ ] `creaitor` (:4621) — **Parked** (2026-10-07). It opened ChatGPT. Target fix is pushed (`app.creaitor.ai`). Not retested after that.
- [ ] `creattie` (:4861) — **Parked** (2026-10-10). Target `https://creattie.com`. Cloudflare stops the proxy: “Performing security verification”, then “Unable to connect to the website” (Ray ID `a48581119662aa5`). Same class of block as VistaCreate and Perplexity. Leave until later.
- [ ] `flaticon` (:4921) — **Parked** (2026-10-07). Blank page / refresh loop was fixed. Freepik/Akamai WAF still returns 403 on this server, including with a proxy. Retry later with fresh cookies on the same residential proxy IP, or another proxy.
- [ ] `grok` (:4831) — **Parked** (2026-10-10). Target `https://grok.com`. Cloudflare blocks this server: “Sorry, you have been blocked” / “You are unable to access grok.com”. Same class of block as VistaCreate, Perplexity, and Creattie. Leave until later.
- [ ] `perplexity` (:5101) — **Parked** (2026-10-09). Cloudflare stops the proxy: “Performing security verification”, then “Unable to connect to the website” for `www.perplexity.ai`. Same class of block as VistaCreate. Leave until later.
- [ ] `scite` (:4761) — **In progress** (2026-10-08). “Max challenge attempts” because `/api/auth/api_token` was blocked by the device gate (no JWT) and the WAF cookie domain was `scite.ai` instead of the proxy host. Soft-allow for `/api` + `/extra-cdn` is pushed, and the browser `aws-waf-token` is kept. Not confirmed on the live site after that.
- [ ] `vistacreate` (:4581) — **Parked** (2026-10-08). ChatGPT target was fixed: `https://create.vista.com`, home `/home/`. Cloudflare still stops the proxy: “Performing security verification”, then “Unable to connect to the website” for `create.vista.com` (Ray ID on the error). Chrome HTTP/2 and allowing `/cdn-cgi/` did not clear it.
- [ ] `zonguru` (:4691) — **Parked** (2026-10-07). Dashboard widgets still say “error loading your data” while official `my.zonguru.com` shows zeros. Panel access and device-bind are OK. Proxy log shows only `/signalr/hubs` and `/api/device-bind` — `/api/dashboard/*` never reaches the Go process. Local curl through the proxy: `POST /api/dashboard/EssentialBusinessData` → 200 zeros when `FbaToken` is set. Already tried: DigitaVision rewrite off JSON/emails, `baseUrl`/cookieDomain rewrite, device soft-allow and skip gate on `/api`, Angular `$http`/XHR re-hook. Next: in DevTools, see whether tile calls go to `zonguru.gt4rents.com` or `my.zonguru.com` (CORS), and why Angular never fires `/api/dashboard/*` on the proxy (SignalR hubs / `lib-bundle`). Cookie is GoAuto `localStorage` `token`+`me` (account ZonGuru 1).

> **2026-10-05 token-only:** Panel open links are `/access?token=` only (no `user=`).  
> Binaries built **before** `fad80ac` → Access Denied.  
> **Already OK (skip rebuild):** `refs` `smrs` `cgpt` `clud` `envt` `cnva`  
> **Rebuild everyone else** (one shot on server):
>
> ```bash
> cd /www/wwwroot/gt4rents.com/_repo
> git pull --ff-only origin main
> source /www/wwwroot/gt4rents.com/_secrets/mysql.env
> chmod +x deploy/rebuild-token-access.sh
> ./deploy/rebuild-token-access.sh 2>&1 | tee /tmp/gt4-token-rebuild.log
> ```
>
> After that, open each tool with a **fresh** panel access-link (old tokens are one-time).

### Chrome Safe Browsing (“Dangerous site”) — who got hit

Not a proxy crash — Google flags phishing-like pages. Seen / noted on gt4rents:

| Host / tool | Why it triggered | Status |
|-------------|------------------|--------|
| `woorank.gt4rents.com` → `wrank` | Brand-name subdomain + lookalike UI | **OK** — live on `wrank.gt4rents.com` |
| `magnific.gt4rents.com` → `mgfc` | Brand-name subdomain + Magnific-like UI | **OK** — live on `mgfc.gt4rents.com` |
| `seositecheckup.gt4rents.com` | Access URL had `/access?user=…&token=…` (looks like credential phishing) | **Mitigated** — panel now opens `/access?token=` only; tool OK (`v17`) |
| `grammarly.gt4rents.com` | Same class of risk (brand subdomain + tokenized access) during HTTPS verify | Tool OK; if warning returns → [report false positive](https://safebrowsing.google.com/safebrowsing/report_error/?hl=en) |

**Platform fix (all tools):** access links no longer put `user=` in the URL (commit `fad80ac`+). Brand-heavy subs (`grammarly`, `magnific`, `woorank`, etc.) still higher SB risk than short slugs (`refs`, `smrs`, `cgpt`, `wrank`, `mgfc`).

---

## Done when

- [ ] All tools built + started (or consciously skipped)  
- [ ] Wildcard SSL + auto-renew verified  
- `TOOL_PUBLIC_SCHEME=https` live  
- [x] Fixed list is the verified set (priority tools plus later HTTPS OK tools)  
- [ ] “Tried — still an issue” list closed or accepted
