#!/usr/bin/env bash
# markhke / other tools: nginx 400 "Request Header Fields Too Large" when Chrome's
# Cookie header outgrows large_client_header_buffers. Raise buffers on the live
# upays.in vhost AND the http{} defaults (default-server selection can ignore
# per-vhost values until Host is known). Then reload nginx.
set -euo pipefail

TARGET_SIZE="${HEADER_BUFFER_SIZE:-2m}"

patch_text() {
  python3 - "$1" "$TARGET_SIZE" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1])
size = sys.argv[2]
if not p.is_file():
    raise SystemExit(0)
t = p.read_text()
orig = t
# Normalize any prior buffer sizes up to TARGET.
for old in ("8k", "16k", "32k", "64k", "128k", "256k", "512k", "1m", "2m", "4m"):
    t = t.replace(f"client_header_buffer_size {old};", f"client_header_buffer_size {size};")
    t = t.replace(f"large_client_header_buffers 4 {old};", f"large_client_header_buffers 8 {size};")
    t = t.replace(f"large_client_header_buffers 8 {old};", f"large_client_header_buffers 8 {size};")
if "client_header_buffer_size" not in t:
    for needle in (
        "server_name *.upays.in upays.in;",
        "server_name upays.in *.upays.in;",
        "server_name *.gt4rents.com gt4rents.com;",
        "server_name gt4rents.com *.gt4rents.com;",
        "http {",
    ):
        if needle in t:
            insert = f"{needle}\n    client_header_buffer_size {size};\n    large_client_header_buffers 8 {size};"
            # Only inject once for http {
            t = t.replace(needle, insert, 1)
            break
if t == orig:
    print("already ok", p)
else:
    bak = p.with_name(p.name + ".bak-header-buffers")
    if not bak.exists():
        bak.write_text(orig)
    p.write_text(t)
    print("patched", p)
PY
}

# Prefer live aaPanel vhosts + main nginx.conf http{} (default server trap).
for conf in \
  /www/server/nginx/conf/nginx.conf \
  /www/server/panel/vhost/nginx/upays.in.conf \
  /www/server/panel/vhost/nginx/gt4rents.com.conf \
  /www/wwwroot/gt4rents.com/deploy/nginx-wildcard.server.conf \
  /www/wwwroot/gt4rents.com/_repo/deploy/nginx-wildcard.server.conf
do
  patch_text "${conf}" || true
done

# Active vhosts only — skip *.bak* so we do not rewrite backup piles.
while IFS= read -r conf; do
  case "${conf}" in
    *.bak*|*.bak-*|*.old|*.rpmsave) continue ;;
  esac
  patch_text "${conf}" || true
done < <(grep -rl 'large_client_header_buffers\|client_header_buffer_size\|server_name.*upays' /www/server/panel/vhost/nginx 2>/dev/null | head -40 || true)

nginx -t
nginx -s reload
echo "nginx header buffers -> ${TARGET_SIZE}. Clear Chrome cookies for markhke.upays.in (or use Incognito), then reopen from the access link."
