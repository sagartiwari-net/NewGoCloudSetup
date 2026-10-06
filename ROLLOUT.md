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
- [ ] `cnva` — Canva (:4501) — **rebuild required** (token-only `/access`); Partial (429 / proxy)  
- [x] `helium10` — Helium10 (:5201) — **HTTPS OK** (2026-10-06): token-only access + panel.db cookies verified  
- [ ] `magnific` — Magnific (:5121) — **rebuild required** after token-only access; Access Denied until redeploy  
- [ ] `junglescout` — JungleScout (:5211) — **rebuild required** after token-only access; was HTTPS OK before  
- [ ] `branalyzer` — Branalyzer (:5081) — **parked** + rebuild for token-only; blank `/home` still open  
- [ ] `grammarly` — grammarly (:4911) — **rebuild required** after token-only access; was HTTPS OK before  
- [ ] `placeit` — Placeit (:4611) — **rebuild required** after token-only access; was HTTPS OK before  
- [x] `wordtune` — Wordtune (:4631) — **HTTPS OK** (2026-10-05): app.wordtune.com, device reveal, chrome hide + panel username, panel.db cookie reload, logout→switch/`logged_out` + Analytics Logouts — **redeploy if Access Denied**  
- [x] `seositecheckup` — SEO Site Checkup (:4661) — **HTTPS OK** (2026-10-05, `v17-device-iam`): panel.db cookies, token-only `/access?token=`, device no false `device_required` 401, logout→switch/contact-admin, chrome hide + panel username, static asset cache
- [x] `wrank` — WooRank (:4561) — **HTTPS OK** (2026-10-05): `wrank` host (not brand SB), HTTP/2 ALPN route, host-jail `https://x/` blocked, device force-visible (no `visibility:hidden`/SW), panel.db cookies + overview

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
- [ ] `answerthepublic` (:4931)  
- [ ] `artistly` (:4821)  
- [ ] `branalyzer` (:5081) — parked; see Priority / Open issues  
- [x] `chatbotapp` (:5061) — up; smpanel keeps `:5001`  
- [ ] `closerscopy` (:5241)  
- [ ] `copyspace` (:4741)  
- [ ] `copywritely` (:4521)  
- [ ] `coursera` (:4701)  
- [ ] `cramly` (:4841)  
- [ ] `creaitor` (:4621)  
- [ ] `creattie` (:4861)  
- [ ] `digen` (:5271)  
- [ ] `educative` (:4851)  
- [ ] `epidemicsound` (:4641)  
- [ ] `erank` (:5191) — rebuild/verify (2026-10-06): was Bad Gateway / Access Denied without session  
- [ ] `fishaudio` (:4991)  
- [ ] `flaticon` (:4921) — **parked** (Freepik WAF 403 on Hetzner; needs Proxy Manager)  
- [ ] `flexclip` (:4671)  
- [ ] `glorify` (:4681)  
- [x] `grammarly` (:4911) — see Priority  
- [x] `gptzero` (:5301) — **HTTPS OK** (2026-10-04): panel credits, logout→swap/`[SWAP]`, Premium features stub, CDN fast open, Upgrade/upsell hide  
- [ ] `grok` (:4831)  
- [x] `helium10` (:5201) — **HTTPS OK** (2026-10-06)  
- [ ] `heliumlearning` (:5221)  
- [ ] `ilovepdf` (:4531)  
- [ ] `imgupscaler` (:5091)  
- [ ] `indexification` (:5231)  
- [ ] `jasper` (:4511)  
- [ ] `joggai` (:5031)  
- [x] `junglescout` (:5211) — see Priority  
- [ ] `kalodata` (:4951)  
- [ ] `leonardo` (:4601)  
- [ ] `linkedinlearning` (:4571)  
- [x] `magnific` (:5121) — see Priority  
- [ ] `merchinformer` (:4651)  
- [ ] `minvo` (:4871)  
- [ ] `mojo` (:4881)  
- [ ] `perplexity` (:5101)  
- [ ] `piktochart` (:4541)  
- [ ] `pixlr` (:4901)  
- [x] `placeit` (:4611) — see Priority  
- [ ] `ppspy` (:4961)  
- [ ] `prezi` (:4751)  
- [ ] `rivalflow` (:4811)  
- [ ] `scite` (:4761)  
- [ ] `screpy` (:4721)  
- [ ] `scribd` (:4791)  
- [ ] `searchatlas` (:4711)  
- [ ] `selleramp` (:5161)  
- [ ] `sellthetrend` (:4591)  
- [ ] `seobility` (:4731)  
- [ ] `seobuddy` (:5131)  
- [x] `seositecheckup` (:4661) — see Priority  
- [ ] `seotesteronline` (:4801)  
- [ ] `shortform` (:4771)  
- [ ] `similarweb` (:5071)  
- [ ] `sketchgenius` (:4781)  
- [ ] `slidebean` (:5021)  
- [ ] `speechify` (:5011)  
- [ ] `spyfu` (:4941)  
- [ ] `storybase` (:4551)  
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
- [ ] `zonguru` (:4691)

---

## Open issues (don’t block Phase A)

| Tool | Issue | Action |
|------|--------|--------|
| `branalyzer` | Server blank `/home` — Network shows no `GetAccountInfo` (local cookie mode OK) | **Parked.** Resume later: redeploy `a7551d9`+, hard-refresh access-link, confirm `extra-cdn-0` GetAccountInfo + Be Curious UI before ticking `[x]` |
| `cgpt` | ChatGPT 2 cookies flaky (`no_access_token` / login wall) | Prefer ChatGPT 1 (verified OK); refresh ChatGPT 2 cookies in panel when free |
| `cnva` | Upstream 429 / dial cancel | Account proxy + slower retest after SSL |
| `magnific` | `/photos` still WAF if someone bypasses redirect; full Photos landing needs residential Proxy Manager | `/photos` redirects to `/people-emotions`; optional Proxy Manager later for hard WAF paths |
| `flaticon` | Freepik/Akamai WAF 403 on Hetzner | Parked until Proxy Manager |

### Chrome Safe Browsing (“Dangerous site”) — who got hit

Not a proxy crash — Google flags phishing-like pages. Seen / noted on gt4rents:

| Host / tool | Why it triggered | Status |
|-------------|------------------|--------|
| `woorank.gt4rents.com` → `wrank` | Brand-name subdomain + lookalike UI | **OK** — live on `wrank.gt4rents.com` |
| `seositecheckup.gt4rents.com` | Access URL had `/access?user=…&token=…` (looks like credential phishing) | **Mitigated** — panel now opens `/access?token=` only; tool OK (`v17`) |
| `grammarly.gt4rents.com` | Same class of risk (brand subdomain + tokenized access) during HTTPS verify | Tool OK; if warning returns → [report false positive](https://safebrowsing.google.com/safebrowsing/report_error/?hl=en) |

**Platform fix (all tools):** access links no longer put `user=` in the URL (commit `fad80ac`+). Brand-heavy subs (`grammarly`, `woorank`, etc.) still higher SB risk than short slugs (`refs`, `smrs`, `cgpt`).

---

## Done when

- [ ] All tools built + started (or consciously skipped)  
- [ ] Wildcard SSL + auto-renew verified  
- `TOOL_PUBLIC_SCHEME=https` live  
- [ ] Priority 8 tools green on **https://** access-links  
- [ ] Open issues closed or accepted
