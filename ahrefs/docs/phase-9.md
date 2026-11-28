# Phase 9 — Deploy bar-api

## Done (repo)

| Piece | Path |
|-------|------|
| systemd unit | `deploy/ahrefs-bar-api.service` |
| Env example | `deploy/ahrefs-bar-api.env.example` |
| Nginx vhost | `deploy/nginx-barapi.upays.in.conf` |
| FP bind | ON by default (`AHREFS_BAR_REQUIRE_FP=1`) |
| Extension | `tm-fp.js` + DNR `X-TM-Bar-FP` |

## From Mac (needs working SSH)

```bash
cd ahrefs
SSH_HOST=root@65.109.16.196 SSH_KEY=~/.ssh/YOUR_KEY ./deploy/push-from-mac.sh
ssh $SSH_HOST 'bash /www/wwwroot/gt4rents.com/_upload/ahrefs-bar/install-on-server.sh'
```

`install-on-server.sh` will: copy binary → apply `001_ahrefs_bar.sql` via `_secrets/mysql.env` → systemd `ahrefs-bar-api` → sync extension/templates for panel Download.

## Nginx (aaPanel)

1. New site `barapi.upays.in` (DNS A → same VPS)
2. Paste `deploy/nginx-barapi.upays.in.conf` into Conf (+ SSL)
3. `nginx -t && nginx -s reload`

## Panel Download stamp

Ensure `gt4-panel-api` can see:

```bash
# in _secrets/ahrefs-bar-api.env (install adds these)
AHREFS_BAR_PUBLIC_URL=https://barapi.upays.in
AHREFS_BAR_EXT_DIR=/www/wwwroot/gt4rents.com/ahrefs-bar-api/extension
AHREFS_BAR_TEMPLATE_DIR=/www/wwwroot/gt4rents.com/ahrefs-bar-api/templates
```

Source that file from panel run script, or export the same vars in `mysql.env`, then `systemctl restart gt4-panel-api`.

## Smoke

```bash
curl -s https://barapi.upays.in/v1/portals
# verify with fingerprint header required
curl -s -X POST https://barapi.upays.in/v1/session/verify \
  -H 'Content-Type: application/json' -H 'X-TM-Bar-FP: tmfp.test' \
  -d '{"token":"…","portal":"https://…","fingerprint":"tmfp.test"}'
```

Stolen `tb.*` without matching `X-TM-Bar-FP` → `fingerprint_mismatch` / `fingerprint_required`.

## Next

Phase 10 done — see `docs/phase-10.md`.
