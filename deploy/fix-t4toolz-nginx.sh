#!/usr/bin/env bash
# Patch aaPanel t4toolz.com vhost for Method A (wildcard + $tool_port).
# Needed so cgpt.t4toolz.com / clud.t4toolz.com resolve via map (not only ah2 hardcoded site).
# Keeps SSL cert lines.
set -euo pipefail

REPO="${REPO:-/www/wwwroot/gt4rents.com/_repo}"
MAP_SRC="$REPO/deploy/nginx-host-port.map.conf"
VHOST="/www/server/panel/vhost/nginx/t4toolz.com.conf"
REWRITE="/www/server/panel/vhost/rewrite/t4toolz.com.conf"
# Live nginx may include ANY of these — sync all (same as fix-nox-nginx-map.sh).
MAP_DSTS=(
  /www/server/panel/vhost/nginx/gt4rents-host-port.map.conf
  /www/server/panel/vhost/nginx/extension/nginx-host-port.map.conf
  /www/server/nginx/conf/nginx-host-port.map.conf
  /etc/nginx/nginx-host-port.map.conf
)

echo "=== diagnose which vhosts exist ==="
ls -la /www/server/panel/vhost/nginx/*t4toolz* 2>/dev/null || true
ls -la /www/server/panel/vhost/nginx/*ah2* 2>/dev/null || true

echo "=== which map file(s) nginx actually includes ==="
grep -Rsn 'host-port\.map\|tool_port\|map \$host' /www/server/nginx/conf/nginx.conf /www/server/panel/vhost/nginx/*.conf /www/server/panel/vhost/nginx/extension/*.conf 2>/dev/null | head -60 || true

if [[ ! -f "$VHOST" ]]; then
  echo "ERROR: missing $VHOST — create aaPanel site t4toolz.com (wildcard Method A) first"
  exit 1
fi

cp -a "$VHOST" "${VHOST}.bak-t4-$(date +%Y%m%d%H%M%S)"
touch "$REWRITE"

echo "=== sync host→port map to all known destinations ==="
for dest in "${MAP_DSTS[@]}"; do
  dir=$(dirname "$dest")
  if [[ -f "$dest" ]] || [[ -d "$dir" ]]; then
    mkdir -p "$dir"
    cp "$MAP_SRC" "$dest"
    echo "OK map → $dest"
    grep -E 'cgpt\.t4toolz|clud\.t4toolz|ah2\.t4toolz' "$dest" || echo "WARN: t4 lines missing in $dest"
  fi
done

# Also patch any other on-disk map $host $tool_port blocks missing cgpt/clud
while IFS= read -r mapfile; do
  [[ -z "$mapfile" ]] && continue
  if grep -q 'map \$host \$tool_port' "$mapfile" 2>/dev/null; then
    if ! grep -q 'cgpt\.t4toolz\.com' "$mapfile"; then
      echo "WARN: $mapfile has tool_port map but no cgpt.t4toolz — copying full map from repo"
      cp "$MAP_SRC" "$mapfile"
    fi
  fi
done < <(grep -Rl 'map \$host \$tool_port' /www/server/nginx/conf /www/server/panel/vhost/nginx 2>/dev/null || true)

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

spans = list(find_blocks(txt, r"(?m)^[ \t]*location[ \t]+/[ \t]*\{"))
if not spans:
    spans = list(find_blocks(txt, r"(?m)^[ \t]*location[ \t]+/\{"))
print(f"found location / blocks: {len(spans)}")
for start, end in sorted(spans, reverse=True):
    txt = txt[:start] + txt[end:]

txt = re.sub(
    r"(?m)^(\s*)include\s+enable-php[^;]*;",
    r"\1#include enable-php-00.conf;",
    txt,
)

# Remove hardcoded single-port proxies that break multi-tool Method A
txt = re.sub(
    r"(?m)^[ \t]*proxy_pass\s+http://127\.0\.0\.1:5291\s*;",
    "        proxy_pass http://127.0.0.1:$tool_port;",
    txt,
)

if "proxy_pass http://127.0.0.1:$tool_port" not in txt:
    inserted = False
    m = re.search(r"(?m)^[ \t]*include\s+/www/server/panel/vhost/rewrite/t4toolz\.com\.conf\s*;", txt)
    if m:
        txt = txt[: m.end()] + "\n\n" + PROXY + txt[m.end() :]
        inserted = True
    if not inserted:
        m = re.search(r"(?m)^[ \t]*access_log\s+", txt)
        if m:
            txt = txt[: m.start()] + PROXY + "\n" + txt[m.start() :]
            inserted = True
    if not inserted:
        idx = txt.rfind("}")
        txt = txt[:idx] + "\n" + PROXY + "\n" + txt[idx:]
        inserted = True
    print(f"proxy inserted: {inserted}")
else:
    print("proxy already present ($tool_port)")

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

echo "== verify conf =="
grep -nE 'server_name|location /|proxy_pass|tool_port' "$VHOST" | head -40

echo "== tools up? =="
ss -lptn | grep -E ':5151|:5171|:5291' || true

echo "== local Host tests (expect Access Denied / tool HTML, NOT nginx 404) =="
for h in cgpt.t4toolz.com clud.t4toolz.com ah2.t4toolz.com; do
  echo "--- $h ---"
  curl -skI --resolve "$h:443:127.0.0.1" "https://$h/" | head -8 || true
  curl -sk --resolve "$h:443:127.0.0.1" "https://$h/" | head -c 180; echo
done
