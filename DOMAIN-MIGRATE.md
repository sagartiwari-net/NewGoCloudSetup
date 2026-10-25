# Domain migrate — gt4rents.com → upays.in

Date started: 2026-10-25.

`gt4rents.com` is suspended at the registrar (Namecheap phishing hold). Live public host is now **`upays.in`**.

Tick a row only after that piece is proven on the new host. Do not tick from a git pull alone.

Tracker for the move. Old per-tool notes stay in `ROLLOUT.md`.

---

## Locked (do not change these)

| Keep | Why |
|------|-----|
| Disk `/www/wwwroot/gt4rents.com/` | Binaries, systemd cwd, `_repo`, `_secrets`, `cdn-cache` already live here. Do not copy 100+ folders. **aaPanel path for `upays.in` is this same folder.** |
| Panel path `/www/wwwroot/gt4rents.com/panel` | Same for `panel.upays.in`. Do not make a second panel folder. |
| MySQL database `gt4rents` / user `gt4rents` | One database forever. Accounts, cookies, users, tokens stay on the same `website_id`. No second DB, no duplicate website rows. |
| Env `GT4RENTS_MYSQL_PASSWORD` and `_secrets/mysql.env` | Same secret file. |
| systemd `gt4-<sub>` and ports | Same process, new hostname only. |
| Old aaPanel sites `gt4rents.com` + `panel.gt4rents.com` | Leave them. If the registrar releases the old domain, that vhost + cert is already there. |
| Friend / reseller hosts (`ernk.amzpremiumsoftware.com`, toolwaly, t4toolz, …) | Those are separate aaPanel sites. Do not rewrite them to upays.in. |
| Orange-cloud on tool records | **Never.** DNS only (grey). Let's Encrypt on aaPanel. |

If `gt4rents.com` comes back: same DB, same processes. Tell me and we flip the public apex (`upays.in` ↔ `gt4rents.com`). Nginx will list **both** hostnames on the same ports, so recovered DNS can hit the live binary without a data copy.

Public names only:

```text
panel.gt4rents.com     →  panel.upays.in
<sub>.gt4rents.com     →  <sub>.upays.in
https://<sub>.upays.in →  127.0.0.1:<same-port>
```

---

## Order (do not skip)

1. **Cloudflare DNS fix** (you) — A records are OK. AAAA is wrong. See Phase 0.
2. **aaPanel sites + nginx** (you, this file) — no domain is on aaPanel yet.
3. **SSL** (you) — wildcard LE for `upays.in` + `*.upays.in`, then panel cert or same wildcard.
4. **Code + DB host rewrite** (me, then you `git pull`) — `tools.json`, nginx map, overlays, panel CORS, `websites.domain`.
5. **Rebuild panel + tools** (you) — panel first, then one default tool (`refs`), then the rest.
6. **Tick this file** as each host answers on `upays.in`.

I cannot SSH. You run aaPanel, DNS, and the rebuild commands. I change the repo.

---

## Phase 0 — Cloudflare (NOW, before aaPanel)

Screenshot already has:

- [x] `upays.in` A `65.109.16.196` — DNS only
- [x] `*.upays.in` A `65.109.16.196` — DNS only
- [x] AAAA records removed (2026-10-25 screenshot: only those 2 A records)
- [ ] SSL/TLS mode **Full** after origin cert exists (not Flexible)
- [x] No extra per-tool A records. The `*` record covers `panel`, `refs`, `smrs`, everything
- Mail CNAMEs were removed with the cleanup. Add them later only if email on `upays.in` is needed.

Prove DNS:

```bash
dig +short upays.in A
dig +short panel.upays.in A
dig +short refs.upays.in A
# all three must be 65.109.16.196
dig +short upays.in AAAA
# must be empty
```

---

## Phase 1 — aaPanel sites

Same clicks as `setup.md` §4c, new names. Path stays the old folder.

### Site A — tools wildcard

Website → PHP Project → Add site:

| Field | Value |
|--------|--------|
| Domain | `upays.in` (never `*.upays.in` as primary — aaPanel rejects that) |
| Apply SSL | Unchecked for now |
| Path | `/www/wwwroot/gt4rents.com` |
| FTP / Database | Do not create |
| PHP | **Static** |

Then Nginx config for this site:

- `server_name upays.in *.upays.in;`
- Same `proxy_pass` / header buffers as `deploy/nginx-wildcard.server.conf`
- `http {}` must include the host→port map (live copy, see Phase 2)

### Site B — panel (separate site)

| Field | Value |
|--------|--------|
| Domain | `panel.upays.in` |
| Apply SSL | Unchecked for now |
| Path | `/www/wwwroot/gt4rents.com/panel` |
| FTP / Database | Do not create |
| PHP | **Static** |

Then paste `deploy/nginx-panel.server.conf` with `server_name panel.upays.in`.

- [x] Site A created, PHP Static, path is the existing tools folder (`upays.in`, SSL not set — wildcard next)
- [x] Site B created, PHP Static (`panel.upays.in`, LE ~89 days — paste proxy into the **443** block)
- [ ] `nginx -t` then reload

Do **not** add 100 separate sites. The `*` record plus one wildcard vhost is the whole tool list.

---

## Phase 2 — nginx map (after code is on the server)

The live include is a **copy**, not the repo file. Same lesson as Veo3 404.

After `git pull` of the upays map:

```bash
cp /www/wwwroot/gt4rents.com/_repo/deploy/nginx-host-port.map.conf \
   /www/server/panel/vhost/nginx/gt4rents-host-port.map.conf
nginx -t && nginx -s reload
```

If the include filename is different on this box, copy to that path instead. The map keys must be `cnva.upays.in`, `refs.upays.in`, … not `*.gt4rents.com`.

- [ ] Map copied
- [ ] `nginx -t` OK
- [ ] Reload done

Unknown host on the wildcard still returns 404 (`$tool_port = 0`). That is correct.

---

## Phase 3 — SSL

Wildcard HTTP-01 cannot work. Use DNS (Cloudflare token in aaPanel).

1. Site `upays.in` → SSL → Let's Encrypt
2. Domains: `upays.in` and `*.upays.in`
3. Method: **DNS** / Cloudflare
4. Apply → Force HTTPS → Auto Renew
5. Site `panel.upays.in` → Let's Encrypt for `panel.upays.in` (HTTP-01 is enough) → Force HTTPS → Auto Renew
6. Cloudflare SSL/TLS → **Full**

- [ ] Wildcard cert on tools site
- [ ] Panel cert (or wildcard already covers it if you attached that cert)
- [ ] Force HTTPS on both
- [ ] Auto Renew on
- [ ] `curl -sI https://panel.upays.in` → 200 (or login page, not 404)
- [ ] `curl -sI http://refs.upays.in` → 301 to https
- [ ] `curl -sI https://refs.upays.in` → 403 Access Denied without a session (that is OK)

---

## Recover later (`gt4rents.com` released)

Same MySQL. Do not export/import. Do not clone `websites` / `accounts`.

What we will have done on this move:

- Live Open links use `*.upays.in`
- nginx map has **both** `refs.upays.in` and `refs.gt4rents.com` → same port (and the same for every sub)
- Panel CORS allows both `panel.upays.in` and `panel.gt4rents.com`
- Token lookup accepts either apex for the same `website_id`

When you say the old domain is live again:

1. Confirm DNS `*.gt4rents.com` still points at `65.109.16.196` (grey cloud).
2. Old aaPanel SSL site should already exist — renew cert if it expired during the hold.
3. Flip the public apex (I change `tools.json` / Open-link host, you rebuild panel). Optional SQL if `websites.domain` was rewritten:

```sql
UPDATE websites
   SET domain = REPLACE(domain, 'upays.in', 'gt4rents.com')
 WHERE domain LIKE '%upays.in';
```

Friend domains are not in that statement.

Until you flip Open links, typing `refs.gt4rents.com` can still reach the same process because the map keeps both names.

---

## Phase 4 — code + MySQL (I change the repo, you pull)

Files that must say `upays.in`:

- `tools.json` (`domain` + every `fqdn`)
- `deploy/nginx-host-port.map.conf`
- `deploy/nginx-wildcard.server.conf`
- `deploy/nginx-panel.server.conf`
- `deploy/configs/*.config.overlay.json` (`public_host`)
- `deploy/start-panel-ui.sh` (public URL check)
- `deploy/fix-panel-headers.sh`
- `panel/panel-api/server.go` CORS origins
- `panel/amember/gt4acc.php` handoff URL
- `panel/update-panel/.env.production.example`
- seed domains for new tools (`quillbot`, `writehuman`, `pictory`, `veo3`, …)
- `setup.md`, `ROLLOUT.md`, `SERVER-SETUP.md`, `README.md`

Leave as `gt4rents` on purpose: MySQL name, systemd prefix, disk path, env var names, tests that only need *a* host string until they are updated.

MySQL after the new binaries are built (websites that are official/recloud on the old apex only):

```sql
-- preview
SELECT id, name, domain FROM websites
 WHERE domain LIKE '%.gt4rents.com' OR domain = 'gt4rents.com';

-- apply
UPDATE websites
   SET domain = REPLACE(domain, 'gt4rents.com', 'upays.in')
 WHERE domain LIKE '%gt4rents.com';
```

Do **not** run that UPDATE on friend domains. They do not contain `gt4rents.com`.

Allowed-sites row is the aMember domain (for example toolsmandi.com), not the panel host. Only `gt4acc.php` panel URL changes.

- [ ] Repo hosts rewritten and pushed
- [ ] `git pull` on `/www/wwwroot/gt4rents.com/_repo`
- [ ] SQL preview looks right
- [ ] SQL apply done
- [ ] Map copied + nginx reload

---

## Phase 5 — rebuild

Panel first, then one tool, then the rest.

```bash
cd /www/wwwroot/gt4rents.com/_repo && git pull
source /www/wwwroot/gt4rents.com/_secrets/mysql.env
./deploy/build-panel.sh && ./deploy/start-panel-api.sh
./deploy/build-panel-ui.sh && ./deploy/start-panel-ui.sh
grep -m1 '[DB] panel API using MySQL' /www/wwwroot/gt4rents.com/panel/panel-api.log
```

Smoke:

```bash
./deploy/build-one.sh refs && ./deploy/start-tool.sh refs
```

Open a **fresh** panel access link (old tokens are one-time). Confirm the browser bar is `refs.upays.in`.

Then either one-by-one or:

```bash
./deploy/build-all.sh 2>&1 | tee /tmp/upays-build-all.log
./deploy/start-batch.sh 8 2>&1 | tee /tmp/upays-start-batch.log
```

Do not kill `:8090` or `:5001`.

- [ ] Panel API MySQL proof line
- [ ] `https://panel.upays.in` login
- [ ] `refs` access link on `refs.upays.in`

---

## Phase 6 — aMember button

File: `panel/amember/gt4acc.php`

```
PANEL_HANDOFF_URL = https://panel.upays.in/api/access/handoff
```

Copy that file onto the aMember box after the panel URL is live. Site key stays the Allowed-sites key (do not put it in git).

- [ ] PHP on aMember points at `panel.upays.in`
- [ ] One live button opens a tool on `*.upays.in`

---

## Platform checklist

| Done | Piece | New URL / note |
|------|--------|----------------|
| [ ] | Cloudflare AAAA removed | IPv6 must not hit StackCP |
| [x] | aaPanel `upays.in` + `*.upays.in` | path `/www/wwwroot/gt4rents.com` — nginx proxy still needed |
| [x] | aaPanel `panel.upays.in` | path `.../panel` — SSL on, nginx proxy still needed |
| [ ] | Wildcard SSL + auto-renew | DNS challenge |
| [ ] | Panel SSL + Force HTTPS | |
| [ ] | nginx map live copy | `*.upays.in` hosts |
| [ ] | Panel API CORS | `https://panel.upays.in` |
| [ ] | Panel UI | `https://panel.upays.in` |
| [ ] | Panel API process | `127.0.0.1:18090` — do not use `:8090` |
| [ ] | MySQL `websites.domain` | `%.upays.in` for official + recloud |
| [ ] | aMember `gt4acc.php` | handoff URL |


## Official tools (was `*.gt4rents.com`)

| Done | Sub | Tool | Port | Old host | New host |
|------|-----|------|------|----------|----------|
| [ ] | `cnva` | canva | `4501` | `cnva.gt4rents.com` | `cnva.upays.in` |
| [ ] | `jasper` | jasper | `4511` | `jasper.gt4rents.com` | `jasper.upays.in` |
| [ ] | `copywritely` | copywritely | `4521` | `copywritely.gt4rents.com` | `copywritely.upays.in` |
| [ ] | `ilovepdf` | ilovepdf | `4531` | `ilovepdf.gt4rents.com` | `ilovepdf.upays.in` |
| [ ] | `piktochart` | piktochart | `4541` | `piktochart.gt4rents.com` | `piktochart.upays.in` |
| [ ] | `storybase` | storybase | `4551` | `storybase.gt4rents.com` | `storybase.upays.in` |
| [ ] | `wrank` | woorank | `4561` | `wrank.gt4rents.com` | `wrank.upays.in` |
| [ ] | `linkedinlearning` | linkedin-learning | `4571` | `linkedinlearning.gt4rents.com` | `linkedinlearning.upays.in` |
| [ ] | `vistacreate` | vistacreate | `4581` | `vistacreate.gt4rents.com` | `vistacreate.upays.in` |
| [ ] | `sellthetrend` | sellthetrend | `4591` | `sellthetrend.gt4rents.com` | `sellthetrend.upays.in` |
| [ ] | `leonardo` | leonardo | `4601` | `leonardo.gt4rents.com` | `leonardo.upays.in` |
| [ ] | `placeit` | placeit | `4611` | `placeit.gt4rents.com` | `placeit.upays.in` |
| [ ] | `creaitor` | creaitor | `4621` | `creaitor.gt4rents.com` | `creaitor.upays.in` |
| [ ] | `wordtune` | wordtune | `4631` | `wordtune.gt4rents.com` | `wordtune.upays.in` |
| [ ] | `epidemicsound` | epidemicsound | `4641` | `epidemicsound.gt4rents.com` | `epidemicsound.upays.in` |
| [ ] | `merchinformer` | merchinformer | `4651` | `merchinformer.gt4rents.com` | `merchinformer.upays.in` |
| [ ] | `seositecheckup` | seositecheckup | `4661` | `seositecheckup.gt4rents.com` | `seositecheckup.upays.in` |
| [ ] | `flexclip` | flexclip | `4671` | `flexclip.gt4rents.com` | `flexclip.upays.in` |
| [ ] | `glorify` | glorify | `4681` | `glorify.gt4rents.com` | `glorify.upays.in` |
| [ ] | `zonguru` | zonguru | `4691` | `zonguru.gt4rents.com` | `zonguru.upays.in` |
| [ ] | `coursera` | coursera | `4701` | `coursera.gt4rents.com` | `coursera.upays.in` |
| [ ] | `searchatlas` | searchatlas | `4711` | `searchatlas.gt4rents.com` | `searchatlas.upays.in` |
| [ ] | `screpy` | screpy | `4721` | `screpy.gt4rents.com` | `screpy.upays.in` |
| [ ] | `seobility` | seobility | `4731` | `seobility.gt4rents.com` | `seobility.upays.in` |
| [ ] | `copyspace` | copyspace | `4741` | `copyspace.gt4rents.com` | `copyspace.upays.in` |
| [ ] | `prezi` | prezi | `4751` | `prezi.gt4rents.com` | `prezi.upays.in` |
| [ ] | `scite` | scite | `4761` | `scite.gt4rents.com` | `scite.upays.in` |
| [ ] | `shortform` | shortform | `4771` | `shortform.gt4rents.com` | `shortform.upays.in` |
| [ ] | `sketchgenius` | sketchgenius | `4781` | `sketchgenius.gt4rents.com` | `sketchgenius.upays.in` |
| [ ] | `scribd` | scribd | `4791` | `scribd.gt4rents.com` | `scribd.upays.in` |
| [ ] | `seotesteronline` | seotesteronline | `4801` | `seotesteronline.gt4rents.com` | `seotesteronline.upays.in` |
| [ ] | `rivalflow` | rivalflow | `4811` | `rivalflow.gt4rents.com` | `rivalflow.upays.in` |
| [ ] | `artistly` | artistly | `4821` | `artistly.gt4rents.com` | `artistly.upays.in` |
| [ ] | `grok` | grok | `4831` | `grok.gt4rents.com` | `grok.upays.in` |
| [ ] | `cramly` | cramly | `4841` | `cramly.gt4rents.com` | `cramly.upays.in` |
| [ ] | `educative` | educative | `4851` | `educative.gt4rents.com` | `educative.upays.in` |
| [ ] | `creattie` | creattie | `4861` | `creattie.gt4rents.com` | `creattie.upays.in` |
| [ ] | `minvo` | minvo | `4871` | `minvo.gt4rents.com` | `minvo.upays.in` |
| [ ] | `mojo` | mojo | `4881` | `mojo.gt4rents.com` | `mojo.upays.in` |
| [ ] | `uncensoredchat` | uncensoredchat | `4891` | `uncensoredchat.gt4rents.com` | `uncensoredchat.upays.in` |
| [ ] | `pixlr` | pixlr | `4901` | `pixlr.gt4rents.com` | `pixlr.upays.in` |
| [ ] | `grammarly` | grammarly | `4911` | `grammarly.gt4rents.com` | `grammarly.upays.in` |
| [ ] | `flaticon` | flaticon | `4921` | `flaticon.gt4rents.com` | `flaticon.upays.in` |
| [ ] | `answerthepublic` | answerthepublic | `4931` | `answerthepublic.gt4rents.com` | `answerthepublic.upays.in` |
| [ ] | `spyfu` | spyfu | `4941` | `spyfu.gt4rents.com` | `spyfu.upays.in` |
| [ ] | `kalodata` | kalodata | `4951` | `kalodata.gt4rents.com` | `kalodata.upays.in` |
| [ ] | `ppspy` | ppspy | `4961` | `ppspy.gt4rents.com` | `ppspy.upays.in` |
| [ ] | `storyblocks` | storyblocks | `4971` | `storyblocks.gt4rents.com` | `storyblocks.upays.in` |
| [ ] | `syntx` | syntx | `4981` | `syntx.gt4rents.com` | `syntx.upays.in` |
| [ ] | `fishaudio` | fishaudio | `4991` | `fishaudio.gt4rents.com` | `fishaudio.upays.in` |
| [ ] | `chatbotapp` | chatbotapp | `5061` | `chatbotapp.gt4rents.com` | `chatbotapp.upays.in` |
| [x] | `speechify` | — | `5011` | **removed** (phishing risk — not on `upays.in`) | — |
| [ ] | `slidebean` | slidebean | `5021` | `slidebean.gt4rents.com` | `slidebean.upays.in` |
| [ ] | `joggai` | joggai | `5031` | `joggai.gt4rents.com` | `joggai.upays.in` |
| [ ] | `videotoblog` | videotoblog | `5041` | `videotoblog.gt4rents.com` | `videotoblog.upays.in` |
| [ ] | `writecream` | writecream | `5051` | `writecream.gt4rents.com` | `writecream.upays.in` |
| [ ] | `similarweb` | similarweb | `5071` | `similarweb.gt4rents.com` | `similarweb.upays.in` |
| [ ] | `branalyzer` | branalyzer | `5081` | `branalyzer.gt4rents.com` | `branalyzer.upays.in` |
| [ ] | `imgupscaler` | imgupscaler | `5091` | `imgupscaler.gt4rents.com` | `imgupscaler.upays.in` |
| [ ] | `perplexity` | perplexity | `5101` | `perplexity.gt4rents.com` | `perplexity.upays.in` |
| [ ] | `zebracat` | zebracat | `5111` | `zebracat.gt4rents.com` | `zebracat.upays.in` |
| [ ] | `mgfc` | magnific | `5121` | `mgfc.gt4rents.com` | `mgfc.upays.in` |
| [ ] | `seobuddy` | seobuddy | `5131` | `seobuddy.gt4rents.com` | `seobuddy.upays.in` |
| [ ] | `smrs` | semrush-new | `5141` | `smrs.gt4rents.com` | `smrs.upays.in` |
| [ ] | `cgpt` | chatgpt | `5151` | `cgpt.gt4rents.com` | `cgpt.upays.in` |
| [ ] | `selleramp` | selleramp | `5161` | `selleramp.gt4rents.com` | `selleramp.upays.in` |
| [ ] | `clud` | claude-ai | `5171` | `clud.gt4rents.com` | `clud.upays.in` |
| [ ] | `airbrush` | airbrush | `5181` | `airbrush.gt4rents.com` | `airbrush.upays.in` |
| [ ] | `erank` | erank | `5191` | `erank.gt4rents.com` | `erank.upays.in` |
| [ ] | `helium10` | helium10 | `5201` | `helium10.gt4rents.com` | `helium10.upays.in` |
| [ ] | `junglescout` | junglescout | `5211` | `junglescout.gt4rents.com` | `junglescout.upays.in` |
| [ ] | `heliumlearning` | helium-learning | `5221` | `heliumlearning.gt4rents.com` | `heliumlearning.upays.in` |
| [ ] | `indexification` | indexification | `5231` | `indexification.gt4rents.com` | `indexification.upays.in` |
| [ ] | `closerscopy` | closerscopy | `5241` | `closerscopy.gt4rents.com` | `closerscopy.upays.in` |
| [ ] | `zikaanalytics` | zikaanalytics | `5251` | `zikaanalytics.gt4rents.com` | `zikaanalytics.upays.in` |
| [ ] | `envt` | envato | `5261` | `envt.gt4rents.com` | `envt.upays.in` |
| [ ] | `digen` | digen | `5271` | `digen.gt4rents.com` | `digen.upays.in` |
| [ ] | `ubersuggest` | ubersuggest | `5281` | `ubersuggest.gt4rents.com` | `ubersuggest.upays.in` |
| [ ] | `refs` | ahrefs | `5291` | `refs.gt4rents.com` | `refs.upays.in` |
| [ ] | `gptzero` | gptzero | `5301` | `gptzero.gt4rents.com` | `gptzero.upays.in` |
| [ ] | `quillbot` | quillbot | `5311` | `quillbot.gt4rents.com` | `quillbot.upays.in` |
| [ ] | `writehuman` | writehuman | `5321` | `writehuman.gt4rents.com` | `writehuman.upays.in` |
| [ ] | `pictory` | pictory | `5331` | `pictory.gt4rents.com` | `pictory.upays.in` |
| [x] | `veo3` | — | `5341` | **removed** | — |

## Recloud tools (same apex — they also die with gt4rents)

| Done | Sub | Tool | Port | Old host | New host |
|------|-----|------|------|----------|----------|
| [ ] | `azsm2` | azsm2 | `6000` | `azsm2.gt4rents.com` | `azsm2.upays.in` |
| [ ] | `azsm3` | azsm3 | `6001` | `azsm3.gt4rents.com` | `azsm3.upays.in` |
| [ ] | `azsm4` | azsm4 | `6002` | `azsm4.gt4rents.com` | `azsm4.upays.in` |
| [ ] | `azsm5` | azsm5 | `6003` | `azsm5.gt4rents.com` | `azsm5.upays.in` |
| [ ] | `azsmaud1` | azsmaud1 | `6004` | `azsmaud1.gt4rents.com` | `azsmaud1.upays.in` |
| [ ] | `azkwfind` | azkwfind | `6005` | `azkwfind.gt4rents.com` | `azkwfind.upays.in` |
| [ ] | `azgram` | azgram | `6006` | `azgram.gt4rents.com` | `azgram.upays.in` |
| [ ] | `azleo` | azleo | `6007` | `azleo.gt4rents.com` | `azleo.upays.in` |
| [ ] | `azrewritify` | azrewritify | `6008` | `azrewritify.gt4rents.com` | `azrewritify.upays.in` |
| [ ] | `azjasper` | azjasper | `6009` | `azjasper.gt4rents.com` | `azjasper.upays.in` |
| [ ] | `azquill` | azquill | `6010` | `azquill.gt4rents.com` | `azquill.upays.in` |
| [ ] | `markhsm1` | markhsm1 | `6100` | `markhsm1.gt4rents.com` | `markhsm1.upays.in` |
| [ ] | `markhsm2` | markhsm2 | `6101` | `markhsm2.gt4rents.com` | `markhsm2.upays.in` |
| [ ] | `markhenv` | markhenv | `6102` | `markhenv.gt4rents.com` | `markhenv.upays.in` |
| [ ] | `markhgram` | markhgram | `6103` | `markhgram.gt4rents.com` | `markhgram.upays.in` |
| [ ] | `markhquill` | markhquill | `6104` | `markhquill.gt4rents.com` | `markhquill.upays.in` |
| [ ] | `markhgpt` | markhgpt | `6105` | `markhgpt.gt4rents.com` | `markhgpt.upays.in` |
| [ ] | `markhke` | markhke | `6106` | `markhke.gt4rents.com` | `markhke.upays.in` |
| [ ] | `markhuber` | markhuber | `6107` | `markhuber.gt4rents.com` | `markhuber.upays.in` |
| [ ] | `markhanswer` | markhanswer | `6108` | `markhanswer.gt4rents.com` | `markhanswer.upays.in` |
| [ ] | `tbaazarsm` | tbaazarsm | `6200` | `tbaazarsm.gt4rents.com` | `tbaazarsm.upays.in` |
| [ ] | `tbaazaraudit` | tbaazaraudit | `6201` | `tbaazaraudit.gt4rents.com` | `tbaazaraudit.upays.in` |
| [ ] | `amzckeepa` | amzckeepa | `6300` | `amzckeepa.gt4rents.com` | `amzckeepa.upays.in` |
| [ ] | `amzcheliumdia` | amzcheliumdia | `6301` | `amzcheliumdia.gt4rents.com` | `amzcheliumdia.upays.in` |
| [ ] | `amzcheliumplat` | amzcheliumplat | `6302` | `amzcheliumplat.gt4rents.com` | `amzcheliumplat.upays.in` |
| [ ] | `amzcselleramp` | amzcselleramp | `6303` | `amzcselleramp.gt4rents.com` | `amzcselleramp.upays.in` |
| [ ] | `amzccg` | amzccg | `6304` | `amzccg.gt4rents.com` | `amzccg.upays.in` |
| [ ] | `amzcft` | amzcft | `6305` | `amzcft.gt4rents.com` | `amzcft.upays.in` |
| [ ] | `amzcnichs` | amzcnichs | `6306` | `amzcnichs.gt4rents.com` | `amzcnichs.upays.in` |
| [ ] | `amzcenv` | amzcenv | `6307` | `amzcenv.gt4rents.com` | `amzcenv.upays.in` |
| [ ] | `shsm1` | shsm1 | `6400` | `shsm1.gt4rents.com` | `shsm1.upays.in` |
| [ ] | `shsm2` | shsm2 | `6401` | `shsm2.gt4rents.com` | `shsm2.upays.in` |

## Leave alone (not this move)

- Friend hosts such as `ernk.amzpremiumsoftware.com` — already their own aaPanel site, same Go port.
- `toolwaly.com` / `t4toolz.com` reseller sites — DNS only, no Cloudflare proxy, their own LE.
- Payment-hub `:8090` and smpanel `:5001`.
- MySQL name, disk path, systemd unit names.

## How to tick

After a tool is rebuilt, open a **new** panel access link. Mark Done when:

1. Browser host is `<sub>.upays.in` (not gt4rents).
2. Page is the tool shell, not Access Denied after a valid Open.
3. No cert warning.

If the document is 404, the live nginx map was not copied. If CORS / wrong host, that overlay still has `public_host` on gt4rents — rebuild that sub.

