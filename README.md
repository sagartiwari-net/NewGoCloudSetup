# NewGoCloudSetup — gt4rents.com (first draft)

Lean deploy repo for tools + panel on the server.

- Domain: `gt4rents.com`
- Method A: `git pull` → `go build` → run
- Server root: `/www/wwwroot/gt4rents.com/<subdomain>/`
- Map: see `tools.json` + owner local `setup.md` (not in this repo)

**First draft:** expect errors; fix one-by-one later.  
**Secrets:** MySQL password and `panel.db` are **not** in git. Use `migrate-bundle/` only on Mac / scp.

Repo: https://github.com/sagartiwari-net/NewGoCloudSetup.git

---

## Layout

```text
tools/                      Go proxy sources (80 tools)
panel/panel-api/            Panel API (Go, :8090)
panel/update-panel/         Panel UI (Next.js) — no node_modules in git
deploy/
  nginx-host-port.map.conf
  nginx-wildcard.server.conf
  nginx-panel.server.conf   panel.gt4rents.com → UI:3000 + /api:8090
  build-panel.sh
  build-panel-ui.sh
  ...
tools.json
migrate-bundle/             LOCAL ONLY — panel.db
```

### Panel ports

| Piece | Port | URL |
|--------|-----:|-----|
| panel-api | 8090 | `https://panel.gt4rents.com/api/...` (via nginx) |
| update-panel (Next) | 3000 | `https://panel.gt4rents.com/` (via nginx) |

Server needs: Go + Node.js 20+ for first UI build (`npm install && npm run build`).

---

## Mac → GitHub

```bash
cd /Users/sagartiwari/Desktop/oneclickgo/NewGoCloudSetup
git init
git remote add origin https://github.com/sagartiwari-net/NewGoCloudSetup.git
git add .
git commit -m "first draft: tools + panel sources + nginx map"
git branch -M main
git push -u origin main
```

Do not force-add `migrate-bundle/` or `setup.md`.

---

## Server (first time) — after SSH works

```bash
# 1) clone monorepo
mkdir -p /www/wwwroot/gt4rents.com
cd /www/wwwroot/gt4rents.com
git clone https://github.com/sagartiwari-net/NewGoCloudSetup.git _repo
cd _repo
chmod +x deploy/*.sh
./deploy/bootstrap-dirs.sh
./deploy/server-first-setup.sh   # creates dirs, builds panel, prints next steps

# 2) scp panel.db from Mac (not in git)
# from Mac:
# scp migrate-bundle/panel.db root@SERVER:/www/wwwroot/gt4rents.com/panel/data/panel.db

# 3) MySQL password on server (env or file — not git)
# export GT4RENTS_MYSQL_PASSWORD='...'

# 4) aaPanel nginx: load map in http{}, include wildcard server (or paste)
# 5) start panel-api + one test tool (e.g. refs / cgpt)
# 6) fix errors tool-by-tool
```

---

## Reseller note

Same process/port can later serve multiple domains (see owner `setup.md` §3c).  
Phase 1 = only `*.gt4rents.com`.

---

## Ports (examples)

| Subdomain | Port | Tool |
|-----------|-----:|------|
| cnva | 4501 | Canva |
| smrs | 5141 | Semrush |
| cgpt | 5151 | ChatGPT |
| clud | 5171 | Claude |
| envt | 5261 | Envato |
| refs | 5291 | Ahrefs |
| panel | 8090 | panel-api (draft) |
