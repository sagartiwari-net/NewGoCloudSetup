#!/usr/bin/env bash
# markhke / other tools: nginx 400 "Request Header Fields Too Large" when Chrome's
# Cookie header outgrows large_client_header_buffers. Raise buffers on the live
# upays.in vhost (and gt4rents.com if present), then reload nginx.
set -euo pipefail

patch_conf() {
  local conf="$1"
  [[ -f "${conf}" ]] || return 0
  python3 - "$conf" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1])
t = p.read_text()
orig = t
replacements = [
    ("client_header_buffer_size 8k;", "client_header_buffer_size 512k;"),
    ("client_header_buffer_size 16k;", "client_header_buffer_size 512k;"),
    ("client_header_buffer_size 32k;", "client_header_buffer_size 512k;"),
    ("client_header_buffer_size 64k;", "client_header_buffer_size 512k;"),
    ("client_header_buffer_size 128k;", "client_header_buffer_size 512k;"),
    ("client_header_buffer_size 256k;", "client_header_buffer_size 512k;"),
    ("large_client_header_buffers 4 8k;", "large_client_header_buffers 8 512k;"),
    ("large_client_header_buffers 4 16k;", "large_client_header_buffers 8 512k;"),
    ("large_client_header_buffers 4 32k;", "large_client_header_buffers 8 512k;"),
    ("large_client_header_buffers 4 64k;", "large_client_header_buffers 8 512k;"),
    ("large_client_header_buffers 8 128k;", "large_client_header_buffers 8 512k;"),
    ("large_client_header_buffers 8 256k;", "large_client_header_buffers 8 512k;"),
]
for a, b in replacements:
    t = t.replace(a, b)
if "client_header_buffer_size" not in t:
    for needle in (
        "server_name *.upays.in upays.in;",
        "server_name upays.in *.upays.in;",
        "server_name *.gt4rents.com gt4rents.com;",
        "server_name gt4rents.com *.gt4rents.com;",
    ):
        if needle in t:
            t = t.replace(
                needle,
                needle + "\n    client_header_buffer_size 512k;\n    large_client_header_buffers 8 512k;",
                1,
            )
            break
if t == orig:
    print("already ok", p)
else:
    p.with_name(p.name + ".bak-header-buffers").write_text(orig)
    p.write_text(t)
    print("patched", p)
PY
}

for conf in \
  /www/server/panel/vhost/nginx/upays.in.conf \
  /www/server/panel/vhost/nginx/gt4rents.com.conf \
  /www/wwwroot/gt4rents.com/deploy/nginx-wildcard.server.conf
do
  patch_conf "${conf}" || true
done

# Also patch any extension / include files that still ship 8k defaults.
while IFS= read -r conf; do
  patch_conf "${conf}" || true
done < <(grep -rl 'large_client_header_buffers\|client_header_buffer_size' /www/server/panel/vhost/nginx 2>/dev/null | head -40 || true)

nginx -t
nginx -s reload
echo "nginx header buffers raised. Clear site cookies for markhke.upays.in once, then reopen from the access link."
