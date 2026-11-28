# Ahrefs Bar — deploy (same style as ROLLOUT / setup.md)

Mac → GitHub push already done. **Server pe** normal tool jaisa:

```text
_repo git pull  →  backup  →  build-ahrefs-bar  →  start-ahrefs-bar
```

Does **not** run `build-one` / `start-tool` on `refs` or any other sub.  
Does **not** restart `gt4-panel-api` until you choose panel merge.

---

## Recipe (on server — copy/paste)

```bash
cd /www/wwwroot/gt4rents.com/_repo
git pull origin main
source /www/wwwroot/gt4rents.com/_secrets/mysql.env

chmod +x deploy/*.sh ahrefs/deploy/*.sh

# 1) backup pehle (full gt4rents + inventory)
./deploy/backup-ahrefs-bar.sh

# 2) build (binary → …/ahrefs-bar-api/, ahrefs_bar_* tables, file-cache dir)
./deploy/build-ahrefs-bar.sh

# 3) start ONLY ahrefs-bar-api systemd
./deploy/start-ahrefs-bar.sh
```

### Smoke

```bash
systemctl is-active ahrefs-bar-api
curl -s http://127.0.0.1:8088/v1/portals
ls -lh /www/wwwroot/gt4rents.com/_backups/ahrefs-bar-latest/
# live tools still up (example):
ss -lntp | grep -E ':(5291|5141|18090)\b' | head
```

---

## Nginx (aaPanel — Method B, like other hosts)

| Item | Value |
|------|--------|
| Site | `barapi.upays.in` |
| Proxy | `127.0.0.1:8088` |
| Conf sample | `_repo/ahrefs/deploy/nginx-barapi.upays.in.conf` |
| DNS | already covered by `*.upays.in` → `65.109.16.196` |

```bash
nginx -t && nginx -s reload
```

---

## Isolation

| Piece | Path / name |
|--------|-------------|
| Runtime | `/www/wwwroot/gt4rents.com/ahrefs-bar-api/` |
| systemd | `ahrefs-bar-api` (not `gt4-*`) |
| Port | `8088` |
| Tables | `ahrefs_bar_*` only |
| File cache | `/var/lib/toolsmandi/ahrefs-cache` |
| Backup | `/www/wwwroot/gt4rents.com/_backups/ahrefs-bar-<stamp>/` |

---

## Panel merge (later — alag step)

```bash
cd /www/wwwroot/gt4rents.com/_repo
git pull origin main
source /www/wwwroot/gt4rents.com/_secrets/mysql.env
./deploy/build-panel.sh
./deploy/build-panel-ui.sh
# then restart panel-api once — NOT all tools
```

Export `AHREFS_BAR_*` from `_secrets/ahrefs-bar-api.env` into panel run env so Download zip stamps `https://barapi.upays.in`.
