# Phase 10 — Reseller extension download

## Done

| Piece | Detail |
|-------|--------|
| `tm-config.js` | Stamped `apiBase`, `proxyHostFilter`, brand, resellerId |
| Packer | `GET /api/ahrefs-bar/extension.zip` builds branded zip |
| tm-check | `GET /api/ahrefs-bar/tm-check.php` + `ahrefs/templates/tm-check.php` |
| Panel UI | Ahrefs Bar → **Download** |
| Env | `AHREFS_BAR_EXT_DIR`, `AHREFS_BAR_TEMPLATE_DIR`, `AHREFS_BAR_PUBLIC_URL` |

## Local

```bash
# panel-api from NewGoCloudSetup/panel/panel-api (relative paths resolve ahrefs/)
export AHREFS_BAR_PUBLIC_URL=https://barapi.upays.in   # stamped into zip
# optional absolute overrides:
# export AHREFS_BAR_EXT_DIR=/path/to/ahrefs/extension
# export AHREFS_BAR_TEMPLATE_DIR=/path/to/ahrefs/templates
```

Panel → Ahrefs Bar → Download → Extension zip / tm-check.php.

## Zip contents

- `ahrefs-bar/` folder (load unpacked)
- `tm-config.js` + `tm-rules.json` pointed at public API
- `manifest.json` host_permissions include reseller portals
- `README.txt`

## Not in v1

- Signed CRX / Chrome Web Store publish
- Auto-upload tm-check to remote FTP
- Merge meters into global tools UI
