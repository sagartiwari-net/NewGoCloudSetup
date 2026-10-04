# Server setup — gt4rents.com subdomains (first draft)

Pull this repo on the server, build, point nginx. Expect errors; fix tool-by-tool.

**Server IP (current DNS):** `65.109.16.196`  
**Root path:** `/www/wwwroot/gt4rents.com`  
**Repo path:** `/www/wwwroot/gt4rents.com/_repo`

---

## 0) Prerequisites (aaPanel / SSH)

Install if missing:

- Git
- Go (`go version` → 1.21+)
- Node.js 20+ (`node -v`, `npm -v`) — panel UI
- MySQL DB already created: `gt4rents` / user `gt4rents` (password only on server, not in git)

```bash
# quick checks
go version
node -v
npm -v
mysql -ugt4rents -p -e 'USE gt4rents; SELECT 1;'
```

---

## 1) Clone repo

```bash
mkdir -p /www/wwwroot/gt4rents.com
cd /www/wwwroot/gt4rents.com

# if empty first time:
git clone https://github.com/sagartiwari-net/NewGoCloudSetup.git _repo

# if already cloned:
cd /www/wwwroot/gt4rents.com/_repo && git pull origin main

cd /www/wwwroot/gt4rents.com/_repo
chmod +x deploy/*.sh
```

---

## 2) One-shot bootstrap (dirs + panel builds)

```bash
cd /www/wwwroot/gt4rents.com/_repo
./deploy/server-first-setup.sh
```

This will:

- create `/www/wwwroot/gt4rents.com/<subdomain>/` for all 80 tools + `panel/`
- build panel-api → `/www/wwwroot/gt4rents.com/panel/panel-api`
- try `npm install` + `next build` for UI

---

## 3) Upload `panel.db` (from Mac — not in git)

On **Mac**:

```bash
scp /Users/sagartiwari/Desktop/oneclickgo/NewGoCloudSetup/migrate-bundle/panel.db \
  root@65.109.16.196:/www/wwwroot/gt4rents.com/panel/data/panel.db
```

On server, confirm:

```bash
ls -lh /www/wwwroot/gt4rents.com/panel/data/panel.db
```

---

## 4) MySQL password on server (secrets file)

```bash
mkdir -p /www/wwwroot/gt4rents.com/_secrets
cat > /www/wwwroot/gt4rents.com/_secrets/mysql.env <<'EOF'
export GT4RENTS_MYSQL_PASSWORD='PASTE_PASSWORD_HERE'
EOF
chmod 600 /www/wwwroot/gt4rents.com/_secrets/mysql.env
```

Load before builds/starts:

```bash
source /www/wwwroot/gt4rents.com/_secrets/mysql.env
```

---

## 5) aaPanel / Nginx (subdomains)

### 5a) Tools wildcard

1. **Website → Add site**
   - Domain: `*.gt4rents.com` (and optionally `gt4rents.com`)
   - Root: `/www/wwwroot/gt4rents.com` (ok)
2. **Nginx custom / config**
   - In **http {}** context (aaPanel “nginx config” top level, not only one site), include:

```nginx
include /www/wwwroot/gt4rents.com/_repo/deploy/nginx-host-port.map.conf;
```

3. For the `*.gt4rents.com` site, replace reverse-proxy / server body with content from:

`_repo/deploy/nginx-wildcard.server.conf`

(or paste its `location /` + `map` already loaded)

4. Reload nginx (aaPanel → Nginx → Reload)

### 5b) Panel site (separate)

1. **Add site:** `panel.gt4rents.com`  
   Root can be `/www/wwwroot/gt4rents.com/panel`
2. Use config from:

`_repo/deploy/nginx-panel.server.conf`

- `/` → `127.0.0.1:3000` (Next UI)  
- `/api/` → `127.0.0.1:8090` (Go API)

3. SSL later (Let’s Encrypt / Cloudflare)

DNS: Cloudflare already has `panel` + `*` → this server IP.

---

## 6) Start panel

```bash
source /www/wwwroot/gt4rents.com/_secrets/mysql.env 2>/dev/null || true

# API (must run with cwd = panel dir so data/panel.db resolves)
cd /www/wwwroot/gt4rents.com/_repo
./deploy/start-panel-api.sh

# UI (separate terminal / screen / systemd later)
./deploy/start-panel-ui.sh
```

Open: `http://panel.gt4rents.com` (HTTPS after SSL)

---

## 7) Build + start tools (subdomains)

**Recommended:** one tool first.

```bash
cd /www/wwwroot/gt4rents.com/_repo
source /www/wwwroot/gt4rents.com/_secrets/mysql.env

./deploy/build-one.sh refs          # Ahrefs → :5291
./deploy/start-tool.sh refs

# more examples:
./deploy/build-one.sh smrs && ./deploy/start-tool.sh smrs
./deploy/build-one.sh cgpt && ./deploy/start-tool.sh cgpt
./deploy/build-one.sh cnva && ./deploy/start-tool.sh cnva
```

**All tools (slow, expect failures):**

```bash
./deploy/build-all.sh
# then start survivors one by one with start-tool.sh
```

Check:

```bash
ss -lptn | grep -E '5291|5141|5151|18090|3000' || netstat -lptn | head
curl -sI -H 'Host: refs.gt4rents.com' http://127.0.0.1/ | head
```

Browser: `http://refs.gt4rents.com` (after nginx + process up)

---

## 8) Day-2 updates

```bash
cd /www/wwwroot/gt4rents.com/_repo
git pull origin main
source /www/wwwroot/gt4rents.com/_secrets/mysql.env

./deploy/build-one.sh refs
./deploy/start-tool.sh refs          # restarts if using start script pid pattern

./deploy/build-panel.sh
./deploy/build-panel-ui.sh
./deploy/start-panel-api.sh
./deploy/start-panel-ui.sh
```

---

## 9) Map cheat-sheet

Full list: `tools.json` and `deploy/nginx-host-port.map.conf`

| Subdomain | Port | Tool |
|-----------|-----:|------|
| panel | 3000+18090 | UI + API |
| cnva | 4501 | Canva |
| smrs | 5141 | Semrush |
| cgpt | 5151 | ChatGPT |
| clud | 5171 | Claude |
| envt | 5261 | Envato |
| refs | 5291 | Ahrefs |

---

## 10) First-draft honesty

- Some `go build` may fail → fix that tool later  
- Panel websites in DB still may point at old hosts → update `public_host` / panel websites to `*.gt4rents.com`  
- SSL / Cloudflare orange cloud decide later  
- systemd services later; for now `start-*.sh` + `nohup` / screen  
- Reseller multi-domain later (owner docs)

---

## Quick copy-paste (after clone + db + secrets)

```bash
cd /www/wwwroot/gt4rents.com/_repo
chmod +x deploy/*.sh
source /www/wwwroot/gt4rents.com/_secrets/mysql.env
./deploy/server-first-setup.sh
./deploy/start-panel-api.sh
./deploy/start-panel-ui.sh
./deploy/build-one.sh refs
./deploy/start-tool.sh refs
# then configure aaPanel nginx from deploy/*.conf
```
