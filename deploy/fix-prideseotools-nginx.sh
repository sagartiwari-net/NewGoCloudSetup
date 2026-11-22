#!/usr/bin/env bash
# Patch aaPanel prideseotools.com vhost for Method A (wildcard + $tool_port).
# CRITICAL: aaPanel's location ~ \.(js|css) steals /cdn/*.css from the proxy → unstyled Ahrefs.
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

def find_blocks(src: str, pattern: str):
    for m in re.finditer(pattern, src):
        open_b = src.find("{", m.start())
        if open_b < 0:
            continue
        yield m.start(), brace_block_end(src, open_b)

# Remove exact location / (brace same line OR next line)
for pat in (
    r"(?m)^[ \t]*location[ \t]+/[ \t]*\{",
    r"(?m)^[ \t]*location[ \t]+/[ \t]*\n[ \t]*\{",
):
    spans = list(find_blocks(txt, pat))
    print(f"strip location / via {pat!r}: {len(spans)}")
    for start, end in sorted(spans, reverse=True):
        txt = txt[:start] + txt[end:]

# Remove aaPanel static asset regex locations — they steal /cdn/*.css|js from Go proxy
static_pats = [
    r"(?m)^[ \t]*location[ \t]+~[ \t]+.*\.(js\|css).*$",
    r"(?m)^[ \t]*location[ \t]+~[ \t]+.*\.(gif\|jpg\|jpeg\|png\|bmp\|swf).*$",
    r"(?m)^[ \t]*location[ \t]+~[ \t]+\.\*\.\(js\|css\)\?\$",
    r"(?m)^[ \t]*location[ \t]+~\s+\.\*\\\.\(js\|css\)\?\$",
]
# Broader: any location ~ line that mentions js|css or common static exts, then its { block
changed = True
removed_static = 0
while changed:
    changed = False
    for m in re.finditer(r"(?m)^[ \t]*location[ \t]+~[^\n]*$", txt):
        line = m.group(0)
        if not re.search(r"js|css|woff|gif|jpg|png|bmp|swf|webp|svg", line, re.I):
            continue
        open_b = txt.find("{", m.start())
        if open_b < 0:
            continue
        end = brace_block_end(txt, open_b)
        print("removing static location:", line.strip())
        txt = txt[: m.start()] + txt[end:]
        removed_static += 1
        changed = True
        break
print(f"removed static regex locations: {removed_static}")

txt = re.sub(
    r"(?m)^(\s*)include\s+enable-php[^;]*;",
    r"\1#include enable-php-00.conf;",
    txt,
)

if "proxy_pass http://127.0.0.1:$tool_port" not in txt:
    inserted = False
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
print("remaining js|css location ~ :", bool(re.search(r"(?m)location\s+~[^\n]*(js\|css|css\|js)", txt)))
PY

echo "== nginx -t && reload =="
nginx -t
nginx -s reload

echo "== verify locations =="
grep -nE 'location|proxy_pass|tool_port|js\|css' "$VHOST" | head -50

echo "== CDN must be text/css (not text/html 404) =="
curl -skI -H 'Host: refs.prideseotools.com' --resolve refs.prideseotools.com:443:127.0.0.1 \
  "https://refs.prideseotools.com/cdn/assets/css/fonts.css" | head -12
curl -skI "https://refs.upays.in/cdn/assets/css/fonts.css" | head -8
