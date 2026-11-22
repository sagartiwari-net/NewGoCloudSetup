#!/usr/bin/env bash
# Patch aaPanel t4toolz.com vhost for Method A (wildcard + $tool_port).
# Live nginx.conf includes the REPO map file directly — also force host overrides
# for cgpt/clud/ah2 so a stale/duplicate map cannot 404 them.
set -euo pipefail

REPO="${REPO:-/www/wwwroot/gt4rents.com/_repo}"
MAP_SRC="$REPO/deploy/nginx-host-port.map.conf"
VHOST="/www/server/panel/vhost/nginx/t4toolz.com.conf"
REWRITE="/www/server/panel/vhost/rewrite/t4toolz.com.conf"
MAP_DSTS=(
  /www/server/panel/vhost/nginx/gt4rents-host-port.map.conf
  /www/server/panel/vhost/nginx/nginx-host-port.map.conf
  /www/server/panel/vhost/nginx/extension/nginx-host-port.map.conf
  /www/server/nginx/conf/nginx-host-port.map.conf
  /etc/nginx/nginx-host-port.map.conf
)

echo "=== diagnose which vhosts exist ==="
ls -la /www/server/panel/vhost/nginx/*t4toolz* 2>/dev/null || true
ls -la /www/server/panel/vhost/nginx/*ah2* 2>/dev/null || true

echo "=== nginx map includes ==="
grep -Rsn 'host-port\.map\|map \$host \$tool_port' \
  /www/server/nginx/conf/nginx.conf \
  /www/server/panel/vhost/nginx/ \
  /www/server/nginx/conf/ 2>/dev/null | head -60 || true

echo "=== nginx -T loaded t4 map keys ==="
nginx -T 2>/dev/null | grep -E 'cgpt\.t4toolz|clud\.t4toolz|ah2\.t4toolz' || echo "WARN: no t4 keys in nginx -T"

if [[ ! -f "$VHOST" ]]; then
  echo "ERROR: missing $VHOST — create aaPanel site t4toolz.com (wildcard Method A) first"
  exit 1
fi

cp -a "$VHOST" "${VHOST}.bak-t4-$(date +%Y%m%d%H%M%S)"
touch "$REWRITE"

echo "=== sync host→port map to all known destinations (+ repo is live include) ==="
grep -E 'cgpt\.t4toolz|clud\.t4toolz|ah2\.t4toolz' "$MAP_SRC" || {
  echo "ERROR: repo map missing t4 lines — git pull first"
  exit 1
}
for dest in "${MAP_DSTS[@]}"; do
  dir=$(dirname "$dest")
  if [[ -f "$dest" ]] || [[ -d "$dir" ]]; then
    mkdir -p "$dir"
    cp -f "$MAP_SRC" "$dest"
    echo "OK map → $dest"
  fi
done

python3 - "$VHOST" <<'PY'
import pathlib, re, sys

path = pathlib.Path(sys.argv[1])
txt = path.read_text()

txt, n = re.subn(
    r"(?m)^(\s*)server_name\s+[^;]+;",
    r"\1server_name *.t4toolz.com t4toolz.com;",
    txt,
)
print(f"server_name replacements: {n}")

# Per-host overrides: do not rely only on map (debug showed ah2 OK, cgpt/clud 404).
PROXY = """    # gt4 t4toolz Method A
    set $gt4_port $tool_port;
    if ($host = cgpt.t4toolz.com) { set $gt4_port 5151; }
    if ($host = clud.t4toolz.com) { set $gt4_port 5171; }
    if ($host = ah2.t4toolz.com) { set $gt4_port 5291; }

    location = /__gt4port {
        default_type text/plain;
        return 200 "host=$host tool_port=$tool_port gt4_port=$gt4_port\\n";
    }

    location /
    {
        if ($gt4_port = 0) { return 404; }
        proxy_pass http://127.0.0.1:$gt4_port;
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

# Strip old gt4 / location / / __gt4port blocks so re-runs are clean
for pat in (
    r"(?m)^[ \t]*location[ \t]+=[ \t]*/__gt4port[ \t]*\{",
    r"(?m)^[ \t]*location[ \t]+/[ \t]*\{",
    r"(?m)^[ \t]*location[ \t]+/\{",
):
    spans = list(find_blocks(txt, pat))
    for start, end in sorted(spans, reverse=True):
        txt = txt[:start] + txt[end:]

# Remove prior gt4 set/if lines
txt = re.sub(r"(?m)^[ \t]*# gt4 t4toolz Method A\n", "", txt)
txt = re.sub(r"(?m)^[ \t]*set \$gt4_port \$tool_port;\n", "", txt)
txt = re.sub(r"(?m)^[ \t]*if \(\$host = (?:cgpt|clud|ah2)\.t4toolz\.com\) \{ set \$gt4_port \d+; \}\n", "", txt)
txt = re.sub(r"(?m)^[ \t]*if \(\$tool_port = 0\) \{ return 404; \}\n", "", txt)

txt = re.sub(
    r"(?m)^(\s*)include\s+enable-php[^;]*;",
    r"\1#include enable-php-00.conf;",
    txt,
)

# Insert PROXY into EVERY server { } block (80 + 443)
server_spans = list(find_blocks(txt, r"(?m)^[ \t]*server[ \t]*\{"))
print(f"server blocks: {len(server_spans)}")
if not server_spans:
    raise SystemExit("no server blocks found")

for start, end in sorted(server_spans, reverse=True):
    block = txt[start:end]
    inner, close = block[:-1], block[-1]
    if "set $gt4_port $tool_port" not in inner:
        inner = inner.rstrip() + "\n\n" + PROXY + "\n"
    txt = txt[:start] + inner + close + txt[end:]

if "client_header_buffer_size" not in txt:
    txt = re.sub(
        r"(?m)^(\s*)root\s+[^;]+;",
        r"\1root /www/wwwroot/gt4rents.com;\n\n\1client_max_body_size 100m;\n\1client_header_buffer_size 256k;\n\1large_client_header_buffers 8 256k;",
        txt,
        count=1,
    )

path.write_text(txt)
print("PATCHED:", path)
print("gt4_port overrides:", txt.count("set $gt4_port"))
print("location / count:", len(re.findall(r"(?m)^[ \t]*location[ \t]+/", txt)))
PY

echo "== nginx -t && reload =="
nginx -t
nginx -s reload

echo "== verify conf =="
grep -nE 'server_name|listen|gt4_port|location|proxy_pass|tool_port' "$VHOST" | head -80

echo "== tools up? =="
ss -lptn | grep -E ':5151|:5171|:5291' || true

echo "== debug /__gt4port (must show gt4_port=5151/5171) =="
for h in cgpt.t4toolz.com clud.t4toolz.com ah2.t4toolz.com; do
  echo "--- $h ---"
  curl -sk --resolve "$h:443:127.0.0.1" "https://$h/__gt4port" || true
  echo
  curl -skI --resolve "$h:443:127.0.0.1" "https://$h/" | head -8 || true
  echo
done
