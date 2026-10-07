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

For each tool: access-link → login shell → one real action → tick below.  
If fail: note in “Open issues”, fix, re-tick.

### Priority

- [x] `refs` — Ahrefs (:5291) — HTTP access-link LIVE  
- [x] `smrs` — Semrush (:5141) — **HTTPS OK** (mixed-content https rewrite + panel account swap; proxy optional)  
- [x] `cgpt` — ChatGPT (:5151) — **HTTPS OK** (panel access-link); prefer **ChatGPT 1** (ID:65); ChatGPT 2 cookies flaky — refresh later  
- [x] `clud` — Claude AI (:5171) — **HTTPS OK** (`claude-v32-name`): Proxy Manager + bootstrap decompress + user-menu hide + panel username on footer  
- [x] `envt` — Envato (:5261) — Download count OK on HTTP; re-verify after SSL  
- [x] `cnva` — Canva (:4501) — **HTTPS OK** (2026-10-07): verified OK — panel access-link working  
- [x] `helium10` — Helium10 (:5201) — **HTTPS OK** (2026-10-06): token-only access + panel.db cookies verified  
- [x] `mgfc` — Magnific (:5121) — **DONE / HTTPS OK** (2026-10-06): verified OK — `mgfc.gt4rents.com` (Safe Browsing rename from `magnific`) + disk CDN cache (`e62a9ad`)  
- [x] `junglescout` — JungleScout (:5211) — **DONE / HTTPS OK** (2026-10-06): verified OK — panel access working  
- [x] `grammarly` — Grammarly (:4911) — **DONE / HTTPS OK** (2026-10-06): verified OK — panel access + disk CDN cache + warm extra-cdn + “connection unstable” banner fixed (`411234d`)  
- [x] `placeit` — Placeit (:4611) — **HTTPS OK** (2026-10-07): verified OK — panel access working  

- [x] `wordtune` — Wordtune (:4631) — **HTTPS OK** (2026-10-05): app.wordtune.com, device reveal, chrome hide + panel username, panel.db cookie reload, logout→switch/`logged_out` + Analytics Logouts — **redeploy if Access Denied**  
- [x] `seositecheckup` — SEO Site Checkup (:4661) — **HTTPS OK** (2026-10-07): panel.db cookies, device lock, chrome hide + panel username; logout→`logged_out`; disk `cdn-cache` (72h) + memory L1 for JS/CSS (**cache added / redeploy**)
- [x] `wrank` — WooRank (:4561) — **HTTPS OK** (2026-10-05): `wrank` host (not brand SB), HTTP/2 ALPN route, host-jail `https://x/` blocked, device force-visible (no `visibility:hidden`/SW), panel.db cookies + overview
- [x] `erank` — eRank (:5191) — **READY / HTTPS OK** (2026-10-06): panel access-link, h1 upstream, no browser cookie leak, CDN disk+browser cache (`erank-v7`)
- [x] `selleramp` — SellerAmp (:5161) — **DONE / HTTPS OK** (2026-10-07): verified OK — panel access, search `/sas/lookup`, `__tm_s` keepalive + static bypass + device-lock (cookie-copy → Access Denied)
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
### Rest (alphabetical)

- [ ] `airbrush` (:5181)  
- [x] `answerthepublic` (:4931) — **HTTPS OK** (2026-10-07): verified OK — panel access + disk CDN cache (`60c3335`)  
- [ ] `artistly` (:4821)  
- [ ] `branalyzer` (:5081) — **parked** — home loads; Be Curious still snack “Introduce any valid URL” on server (`59905db` local OK) — resume later; see Open issues  
- [ ] `chatbotapp` (:5061) — **parked** — home/chats list OK; `/api/v2/chat` still 4002 `x_token` on server (`91f9ad8` inject) — resume later; see Open issues  
- [ ] `closerscopy` (:5241)  
- [ ] `copyspace` (:4741)  
- [ ] `copywritely` (:4521)  
- [ ] `coursera` (:4701)  
- [ ] `cramly` (:4841)  
- [ ] `creaitor` (:4621) — **PARKED** (2026-10-07): ChatGPT-target fix pushed (`app.creaitor.ai`); retry later  


- [ ] `creattie` (:4861)  
- [ ] `digen` (:5271)  
- [ ] `educative` (:4851)  
- [ ] `epidemicsound` (:4641)  
- [x] `erank` (:5191) — **READY / HTTPS OK** (2026-10-06): see Priority  
- [ ] `fishaudio` (:4991)  
- [ ] `flaticon` (:4921) — **PARKED** (2026-10-07): Freepik WAF; blank/refresh fixed; still blocked even with proxy — retry later with fresh cookies on same residential proxy IP (or another proxy)  


- [ ] `flexclip` (:4671)  
- [ ] `glorify` (:4681)  
- [x] `grammarly` (:4911) — **DONE / HTTPS OK** (2026-10-06): verified OK — see Priority  
- [x] `gptzero` (:5301) — **HTTPS OK** (2026-10-04): panel credits, logout→swap/`[SWAP]`, Premium features stub, CDN fast open, Upgrade/upsell hide  
- [ ] `grok` (:4831)  
- [x] `helium10` (:5201) — **HTTPS OK** (2026-10-06)  
- [ ] `heliumlearning` (:5221)  
- [x] `ilovepdf` (:4531) — **HTTPS OK** (2026-10-07): verified OK — force `www.ilovepdf.com`; HTTP/1.1-only TLS (h2→h1 fallback caused malformed SETTINGS → 502)  


- [ ] `imgupscaler` (:5091)  
- [ ] `indexification` (:5231)  
- [ ] `jasper` (:4511)  
- [ ] `joggai` (:5031)  
- [x] `junglescout` (:5211) — **DONE / HTTPS OK** (2026-10-06): verified OK — see Priority  
- [ ] `kalodata` (:4951)  
- [x] `leonardo` (:4601) — **HTTPS OK** (2026-10-07): verified OK — panel access working  
- [ ] `linkedinlearning` (:4571)  
- [x] `mgfc` (:5121) — **DONE / HTTPS OK** (2026-10-06): verified OK — see Priority  
- [ ] `merchinformer` (:4651)  
- [ ] `minvo` (:4871)  
- [ ] `mojo` (:4881)  
- [ ] `perplexity` (:5101)  
- [ ] `piktochart` (:4541)  
- [ ] `pixlr` (:4901)  
- [x] `placeit` (:4611) — **HTTPS OK** (2026-10-07): see Priority  
- [ ] `ppspy` (:4961)  
- [x] `prezi` (:4751) — **HTTPS OK** (2026-10-08): verified OK — panel username + UserDropdown blocked; Craft CDN/API device soft-allow (was stuck on “Laying out the canvas”)  


- [ ] `rivalflow` (:4811)  
- [ ] `scite` (:4761)  
- [ ] `screpy` (:4721)  
- [x] `scribd` (:4791) — **HTTPS OK** (2026-10-07): verified OK — live working on `scribd.gt4rents.com`  

- [ ] `searchatlas` (:4711)  
- [x] `selleramp` (:5161) — **DONE / HTTPS OK** (2026-10-07): see Priority  

- [x] `sellthetrend` (:4591) — **HTTPS OK** (2026-10-07): verified OK — `www.sellthetrend.com`, profile dropdowns hidden (left+top), panel username/initials on avatar bar  
- [x] `seobility` (:4731) — **HTTPS OK** (2026-10-07): verified OK — `app.seobility.net`, account Profile/Subscription/Billing/Members/MCP block hidden, panel username in dropdown; logout→`/user/login` wall → switch / panel `logged_out` + contact-admin (**failover verified**)  


- [ ] `seobuddy` (:5131)  
- [x] `seositecheckup` (:4661) — see Priority  
- [x] `seotesteronline` (:4801) — **HTTPS OK** (2026-10-07): verified OK — cookie slim keeps `ct_session`, device bind works, `suite.seotesteronline.com`; no-subscription / dead cookie → soft-revive then switch / panel `logged_out` + contact-admin (**failover verified**)  


- [ ] `shortform` (:4771) — **IN PROGRESS** (2026-10-08): access token OK then device page “could not verify” because oversized Cookie header was deleted (wiped `ct_session`). Slim keeps session. URL is token-only (no `user=`).  

- [x] `similarweb` (:5071) — **HTTPS OK** (2026-10-07): verified OK — live working; CDN disk cache already present (`cdn_cache.go` → `cdn-cache/`, 72h TTL)  

- [x] `sketchgenius` (:4781) — **HTTPS OK** (2026-10-07): verified OK — panel username on nav; dropdown blocked; login/`Unauthenticated` → panel `logged_out` + switch/contact-admin (**failover verified**)  


- [ ] `slidebean` (:5021)  
- [ ] `speechify` (:5011)  
- [ ] `spyfu` (:4941)  
- [x] `storybase` (:4551) — **HTTPS OK** (2026-10-07): verified OK — `www.storybase.com`, HTTP/1.1 ALPN (was 502/`chatgpt.com`); `#profile-widget` hidden (Settings/Billing/Logout + sidebar avatar/name)  


- [ ] `storyblocks` (:4971)  
- [ ] `syntx` (:4981)  
- [ ] `ubersuggest` (:5281)  
- [ ] `uncensoredchat` (:4891)  
- [ ] `videotoblog` (:5041)  
- [ ] `vistacreate` (:4581)  
- [x] `wrank` (:4561) — see Priority  


- [x] `wordtune` (:4631) — **HTTPS OK** (2026-10-05): app.wordtune.com target, device-lock reveal, avatar menu hide + panel username, cookie reload from panel.db, logout detect → switch / `logged_out` + Analytics Logouts  

- [ ] `writecream` (:5051)  
- [ ] `zebracat` (:5111)  
- [ ] `zikaanalytics` (:5251)  
- [ ] `zonguru` (:4691) — **PARKED** (2026-10-07): dashboard widgets still “error loading your data” while official `my.zonguru.com` shows zeros OK. **Seen so far:** panel access + device-bind OK; `[REQ]` only `/signalr/hubs` + `/api/device-bind` — **no** `/api/dashboard/*` ever hits the Go proxy (browser not posting tiles through us). Local curl through proxy: `POST /api/dashboard/EssentialBusinessData` → 200 zeros when `FbaToken` set. Attempted: DigitaVision rewrite off JSON/emails; `baseUrl`/cookieDomain rewrite; device soft-allow + skip gate on `/api`; Angular `$http`/XHR re-hook. **Next:** DevTools Network — confirm tile calls URL host (`zonguru.gt4rents.com` vs `my.zonguru.com` CORS); why Angular never fires `/api/dashboard/*` on proxy; SignalR hubs / `lib-bundle` API client. Cookie = GoAuto `localStorage` `token`+`me` (account ZonGuru 1).

---

## Open issues (don’t block Phase A)

| Tool | Issue | Action |
|------|--------|--------|
| `branalyzer` | Be Curious → snack “Introduce any valid URL or domain” on `branalyzer.gt4rents.com` (home OK; local `59905db` worked) | **Parked.** Resume later: compare server main.js patch + searchText/DOM vs local; wipe `cdn-cache`; hard-refresh |
| `chatbotapp` | `/api/v2/chat` → 4002 `x_token header is required` (sidebar/history OK; `91f9ad8` inject) | **Parked.** Resume later: confirm panel GoAuto has IndexedDB `stsTokenManager`; check `[CHATBOT] api auth OK` in app.log |
| `cgpt` | ChatGPT 2 cookies flaky (`no_access_token` / login wall) | Prefer ChatGPT 1 (verified OK); refresh ChatGPT 2 cookies in panel when free |
| `mgfc` (Magnific) | `/photos` still WAF if someone bypasses redirect | **OK on `mgfc`**; `/photos` → `/people-emotions` |
| `flaticon` | Freepik/Akamai WAF 403 on Hetzner | Parked until Proxy Manager |
| `zonguru` | Dashboard tiles error; proxy never sees `/api/dashboard/*` (only signalr + device-bind). Official OK (zeros). | **Parked.** Resume: DevTools host of tile XHRs; Angular API client / lib-bundle; why posts never reach `:4691` |

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
- [x] Priority tools green on **https://** access-links (incl. `cnva` `placeit` `selleramp` `answerthepublic` `leonardo` 2026-10-07)  
- [ ] Open issues closed or accepted
