#!/usr/bin/env bash
# Patch aaPanel t4toolz.com vhost for Method A (wildcard + host→port).
# Forces cgpt/clud/ah2 ports (map alone was not enough in practice).
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
)

echo "=== restore vhost if current nginx -t would fail ==="
if [[ -f "$VHOST" ]] && ! nginx -t 2>/dev/null; then
  bak=$(ls -1t /www/server/panel/vhost/nginx/t4toolz.com.conf.bak-t4-* 2>/dev/null | head -1 || true)
  if [[ -n "${bak:-}" ]]; then
    echo "restoring broken vhost from $bak"
    cp -a "$bak" "$VHOST"
  fi
fi

echo "=== nginx map include (live) ==="
grep -n 'host-port\.map' /www/server/nginx/conf/nginx.conf || true
nginx -T 2>/dev/null | grep -E '^\s+(cgpt|clud|ah2)\.t4toolz\.com' | sort -u || true

if [[ ! -f "$VHOST" ]]; then
  echo "ERROR: missing $VHOST"
  exit 1
fi

cp -a "$VHOST" "${VHOST}.bak-t4-$(date +%Y%m%d%H%M%S)"
touch "$REWRITE"

echo "=== sync maps ==="
grep -E 'cgpt\.t4toolz|clud\.t4toolz|ah2\.t4toolz' "$MAP_SRC"
for dest in "${MAP_DSTS[@]}"; do
  dir=$(dirname "$dest")
  [[ -d "$dir" ]] || continue
  cp -f "$MAP_SRC" "$dest"
  echo "OK → $dest"
done

python3 - "$VHOST" <<'PY'
import pathlib, re, sys

path = pathlib.Path(sys.argv[1])
txt = path.read_text()

PROXY_INNER = """
    # gt4 t4toolz Method A
    set $gt4_port $tool_port;
    if ($host = cgpt.t4toolz.com) { set $gt4_port 5151; }
    if ($host = clud.t4toolz.com) { set $gt4_port 5171; }
    if ($host = ah2.t4toolz.com) { set $gt4_port 5291; }

    location = /__gt4port {
        default_type text/plain;
        return 200 "host=$host tool_port=$tool_port gt4_port=$gt4_port\\n";
    }

    location / {
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
""".strip("\n")

def brace_block_end(src: str, open_brace_idx: int) -> int:
    depth = 0
    j = open_brace_idx
    while j < len(src):
        if src[j] == "{":
            depth += 1
        elif src[j] == "}":
            depth -= 1
            if depth == 0:
                return j + 1
        j += 1
    raise SystemExit("unbalanced brace")

def find_server_blocks(src: str):
    out = []
    for m in re.finditer(r"(?m)^[ \t]*server[ \t]*\{", src):
        start = m.start()
        open_b = src.find("{", m.start())
        end = brace_block_end(src, open_b)
        out.append((start, end))
    return out

def strip_in_block(block: str) -> str:
    """Remove prior gt4 bits and any exact location / or /__gt4port (brace may be next line)."""
    # Remove gt4 set/if comment lines
    block = re.sub(r"(?m)^[ \t]*# gt4 t4toolz Method A[ \t]*\n", "", block)
    block = re.sub(r"(?m)^[ \t]*set \$gt4_port \$tool_port;[ \t]*\n", "", block)
    block = re.sub(
        r"(?m)^[ \t]*if \(\$host = (?:cgpt|clud|ah2)\.t4toolz\.com\) \{ set \$gt4_port \d+; \}[ \t]*\n",
        "",
        block,
    )

    # Remove location = /__gt4port and location / blocks ( { same line or next line )
    patterns = [
        r"(?m)^[ \t]*location[ \t]+=[ \t]*/__gt4port[ \t]*\{",
        r"(?m)^[ \t]*location[ \t]+=[ \t]*/__gt4port[ \t]*\n[ \t]*\{",
        r"(?m)^[ \t]*location[ \t]+/[ \t]*\{",
        r"(?m)^[ \t]*location[ \t]+/[ \t]*\n[ \t]*\{",
    ]
    changed = True
    while changed:
        changed = False
        for pat in patterns:
            m = re.search(pat, block)
            if not m:
                continue
            open_b = block.find("{", m.start())
            end = brace_block_end(block, open_b)
            block = block[: m.start()] + block[end:]
            changed = True
            break

    # Comment enable-php
    block = re.sub(
        r"(?m)^(\s*)include\s+enable-php[^;]*;",
        r"\1#include enable-php-00.conf;",
        block,
    )

    # Wildcard server_name
    block = re.sub(
        r"(?m)^(\s*)server_name\s+[^;]+;",
        r"\1server_name *.t4toolz.com t4toolz.com;",
        block,
    )
    return block

servers = find_server_blocks(txt)
print(f"server blocks: {len(servers)}")
if not servers:
    raise SystemExit("no server blocks")

# Rebuild file with each server block cleaned + PROXY_INNER inserted once
parts = []
prev = 0
for start, end in servers:
    parts.append(txt[prev:start])
    block = txt[start:end]
    block = strip_in_block(block)
    # Insert before final }
    inner, close = block[:-1], block[-1]
    if "set $gt4_port $tool_port" not in inner:
        inner = inner.rstrip() + "\n\n" + PROXY_INNER + "\n"
    # Sanity: exact root location count must be 1
    roots = len(re.findall(r"(?m)^[ \t]*location[ \t]+/", inner))
    # location / and location = /__gt4port both match location[ \t]+/ — count location / { specifically
    root_locs = len(re.findall(r"(?m)^[ \t]*location[ \t]+/[ \t]*(\{|$)", inner))
    print(f"  block root location markers: {root_locs}")
    if root_locs != 1:
        raise SystemExit(f"expected exactly 1 location / in server block, got {root_locs}")
    parts.append(inner + close)
    prev = end
parts.append(txt[prev:])
txt = "".join(parts)

if "client_header_buffer_size" not in txt:
    txt = re.sub(
        r"(?m)^(\s*)root\s+[^;]+;",
        r"\1root /www/wwwroot/gt4rents.com;\n\n\1client_max_body_size 100m;\n\1client_header_buffer_size 256k;\n\1large_client_header_buffers 8 256k;",
        txt,
        count=1,
    )

path.write_text(txt)
print("PATCHED:", path)
# verify no duplicate location / inside any server
for i, (start, end) in enumerate(find_server_blocks(txt)):
    b = txt[start:end]
    n = len(re.findall(r"(?m)^[ \t]*location[ \t]+/[ \t]*(\{|$)", b))
    print(f"verify server[{i}] location / count = {n}")
    if n != 1:
        raise SystemExit("duplicate or missing location /")
PY

echo "== nginx -t && reload =="
nginx -t
nginx -s reload

echo "== conf snippet =="
grep -nE 'listen|server_name|gt4_port|location|proxy_pass' "$VHOST" | head -60

echo "== /__gt4port + home =="
for h in cgpt.t4toolz.com clud.t4toolz.com ah2.t4toolz.com; do
  echo "--- $h ---"
  curl -sk --http1.1 --resolve "$h:443:127.0.0.1" "https://$h/__gt4port"; echo
  curl -skI --http1.1 --resolve "$h:443:127.0.0.1" "https://$h/" | head -8
  echo
done
