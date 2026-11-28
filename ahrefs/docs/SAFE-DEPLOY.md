# Ahrefs Bar — safe deploy (no mix with live tools)

## Isolation rules

| Piece | Where | Touches live tools? |
|-------|--------|---------------------|
| Process | systemd `ahrefs-bar-api` only | **No** — never restarts `gt4-*` / panel |
| Port | `127.0.0.1:8088` | **No** — outside 4501–5301 tool ports |
| Disk binary | `/www/wwwroot/gt4rents.com/ahrefs-bar-api/` | **No** — separate folder |
| File cache | `/var/lib/toolsmandi/ahrefs-cache/` | **No** — not tool `cdn-cache` |
| MySQL tables | **`ahrefs_bar_*` only** (`CREATE IF NOT EXISTS`) | **No DROP/ALTER** on websites/accounts/meters |

Panel UI / `gt4-panel-api` restart = **optional later step** (merge). First install can be bar-api only.

## 1) GitHub (Mac)

Repo: `NewGoCloudSetup` → `ahrefs/`

```bash
cd /Users/sagartiwari/Desktop/oneclickgo/NewGoCloudSetup
git push origin main   # after commit
```

## 2) Backup + install on server

```bash
# From Mac (needs working SSH key)
cd ahrefs
SSH_HOST=root@65.109.16.196 SSH_KEY=~/.ssh/YOUR_KEY ./deploy/push-from-mac.sh

# On server — backup runs automatically inside install
ssh -i ~/.ssh/YOUR_KEY root@65.109.16.196 \
  'bash /www/wwwroot/gt4rents.com/_upload/ahrefs-bar/install-on-server.sh'
```

Backup lands in:

```text
/www/wwwroot/gt4rents.com/_backups/ahrefs-bar-<stamp>/
  gt4rents-full.sql.gz      ← full DB restore point
  ahrefs_bar-only.sql.gz    ← bar tables only (if any)
  live-services.txt
```

Or backup alone:

```bash
bash /www/wwwroot/gt4rents.com/_upload/ahrefs-bar/backup-before-ahrefs-bar.sh
```

## 3) Verify nothing else broke

```bash
systemctl is-active ahrefs-bar-api
curl -s http://127.0.0.1:8088/v1/portals
# existing tools still up:
systemctl is-active gt4-panel-api || true
ss -lntp | grep ':5291'   # refs Ahrefs site proxy
```

## 4) Panel merge (later)

When ready: deploy panel-api with `ahrefs_bar.go` + UI, set `AHREFS_BAR_*` in panel env, `systemctl restart gt4-panel-api` **once**. That only adds `operators.ahrefs_bar_enabled` (additive column) + routes — still no tool proxy restart.
