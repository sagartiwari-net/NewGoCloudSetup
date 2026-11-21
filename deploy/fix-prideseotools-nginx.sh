#!/usr/bin/env bash
# Patch aaPanel prideseotools.com vhost for Method A (wildcard + $tool_port).
# Keeps SSL cert lines. Apex @ DNS optional.
set -euo pipefail

REPO="${REPO:-/www/wwwroot/gt4rents.com/_repo}"
MAP_SRC="$REPO/deploy/nginx-host-port.map.conf"
MAP_DST="/www/server/panel/vhost/nginx/gt4rents-host-port.map.conf"
VHOST="/www/server/panel/vhost/nginx/prideseotools.com.conf"
REWRITE="/www/server/panel/vhost/rewrite/prideseotools.com.conf"

if [[ ! -f "$VHOST" ]]; then
  echo "ERROR: missing $VHOST"
  exit 1
fi

cp -a "$VHOST" "${VHOST}.bak-pride-$(date +%Y%m%d%H%M%S)"
touch "$REWRITE"
cp "$MAP_SRC" "$MAP_DST"
echo "OK map → $MAP_DST"

python3 - "$VHOST" <<'PY'
import pathlib, re, sys

path = pathlib.Path(sys.argv[1])
txt = path.read_text()

txt, n = re.subn(
    r"(?m)^(\s*)server_name\s+[^;]+;",
    r"\1server_name *.prideseotools.com prideseotools.com;",
    txt,
)
print(f"server_name replacements: {n}")

PROXY = """    location /
    {
        if ($tool_port = 0) { return 404; }
        proxy_pass http://127.0.0.1:$tool_port;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-Proto $gt4_forwarded_proto;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_connect_timeout 10s;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
        proxy_buffering on;
        proxy_buffer_size 128k;
        proxy_buffers 8 256k;
        proxy_busy_buffers_size 512k;
        proxy_temp_file_write_size 512k;
    }
"""

def find_blocks(src: str, pattern: str):
    """Yield (start, end) of location blocks matching pattern prefix before {."""
    for m in re.finditer(pattern, src):
        brace = src.find("{", m.end() - 1)
        if brace < 0:
            continue
        depth = 0
        j = brace
        while j < len(src):
            if src[j] == "{":
                depth += 1
            elif src[j] == "}":
                depth -= 1
                if depth == 0:
                    yield m.start(), j + 1
                    break
            j += 1

# Remove ALL root location / blocks (exact / only), then insert one PROXY
spans = list(find_blocks(src := txt, r"(?m)^[ \t]*location[ \t]+/[ \t]*\{"))
# also match location / without space before {
if not spans:
    spans = list(find_blocks(txt, r"(?m)^[ \t]*location[ \t]+/\{"))

print(f"found location / blocks: {len(spans)}")
# delete from end so indexes stay valid
for start, end in sorted(spans, reverse=True):
    txt = txt[:start] + txt[end:]

# Comment out enable-php includes (Go tools — no PHP)
txt = re.sub(
    r"(?m)^(\s*)include\s+enable-php[^;]*;",
    r"\1#include enable-php-00.conf;",
    txt,
)

if "proxy_pass http://127.0.0.1:$tool_port" not in txt:
    inserted = False
    # Prefer after REWRITE include
    m = re.search(r"(?m)^[ \t]*include\s+/www/server/panel/vhost/rewrite/prideseotools\.com\.conf\s*;", txt)
    if m:
        txt = txt[: m.end()] + "\n\n" + PROXY + txt[m.end() :]
        inserted = True
    if not inserted:
        m = re.search(r"(?m)^[ \t]*access_log\s+", txt)
        if m:
            txt = txt[: m.start()] + PROXY + "\n" + txt[m.start() :]
            inserted = True
    if not inserted:
        # before last closing brace of file
        idx = txt.rfind("}")
        txt = txt[:idx] + "\n" + PROXY + "\n" + txt[idx:]
        inserted = True
    print(f"proxy inserted: {inserted}")
else:
    print("proxy already present")

if "client_header_buffer_size" not in txt:
    txt = re.sub(
        r"(?m)^(\s*)root\s+[^;]+;",
        r"\1root /www/wwwroot/gt4rents.com;\n\n\1client_max_body_size 100m;\n\1client_header_buffer_size 256k;\n\1large_client_header_buffers 8 256k;",
        txt,
        count=1,
    )

path.write_text(txt)
print("PATCHED:", path)
print("proxy_ok:", "proxy_pass http://127.0.0.1:$tool_port" in txt)
PY

echo "== nginx -t && reload =="
nginx -t
nginx -s reload

echo "== verify =="
grep -nE 'server_name|location /|proxy_pass|tool_port' "$VHOST" | head -40
echo "== must be Go/tool response (NOT static html length ~58k) =="
curl -sI -H 'Host: refs.prideseotools.com' http://127.0.0.1/ | head -12
curl -s -H 'Host: refs.prideseotools.com' http://127.0.0.1/ | head -c 200; echo
