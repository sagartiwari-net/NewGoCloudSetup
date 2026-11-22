#!/usr/bin/env bash
# Patch aaPanel prideseotools.com for Method A.
# Removes aaPanel static js|css locations (they 404 /cdn/*.css → unstyled Ahrefs).
# Handles aaPanel "server\n{" (brace on next line).
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

# Prefer original-ish bak (largest / oldest with SSL) if current has no proxy
if ! grep -q 'proxy_pass http://127.0.0.1:\$tool_port\|proxy_pass http://127.0.0.1:\$gt4_port' "$VHOST" 2>/dev/null; then
  bak=$(ls -1t /www/server/panel/vhost/nginx/prideseotools.com.conf.bak-pride-* 2>/dev/null | head -1 || true)
  if [[ -n "${bak:-}" ]]; then
    echo "no proxy in current conf — restoring $bak first"
    cp -a "$bak" "$VHOST"
  fi
fi

if ! nginx -t 2>/dev/null; then
  bak=$(ls -1t /www/server/panel/vhost/nginx/prideseotools.com.conf.bak-pride-* 2>/dev/null | head -1 || true)
  if [[ -n "${bak:-}" ]]; then
    echo "nginx -t fail — restoring $bak"
    cp -a "$bak" "$VHOST"
  fi
fi

cp -a "$VHOST" "${VHOST}.bak-pride-$(date +%Y%m%d%H%M%S)"
touch "$REWRITE"
cp -f "$MAP_SRC" "$MAP_DST"
echo "OK map → $MAP_DST"

python3 - "$VHOST" <<'PY'
import pathlib, re, sys

path = pathlib.Path(sys.argv[1])
txt = path.read_text()

def brace_end(src, i):
    depth = 0
    while i < len(src):
        if src[i] == "{":
            depth += 1
        elif src[i] == "}":
            depth -= 1
            if depth == 0:
                return i + 1
        i += 1
    raise SystemExit("unbalanced brace")

def find_servers(src):
    # aaPanel: "server\n{" OR "server {"
    for m in re.finditer(r"(?m)^[ \t]*server[ \t]*(?:\{|\n[ \t]*\{)", src):
        open_b = src.find("{", m.start())
        yield m.start(), brace_end(src, open_b)

txt, n = re.subn(
    r"(?m)^(\s*)server_name\s+[^;]+;",
    r"\1server_name *.prideseotools.com prideseotools.com;",
    txt,
)
print(f"server_name replacements: {n}")

# Strip location / and static asset regex locations
changed = True
while changed:
    changed = False
    for m in re.finditer(r"(?m)^[ \t]*location\b", txt):
        open_b = txt.find("{", m.start())
        if open_b < 0:
            continue
        header = re.sub(r"\s+", " ", txt[m.start():open_b]).strip()
        end = brace_end(txt, open_b)
        remove = False
        if re.match(r"^location /$", header) or header == "location /":
            remove = True
        if "location ~" in header and re.search(r"js|css|woff|gif|jpg|png|bmp|swf|webp|svg", header, re.I):
            remove = True
        if remove:
            print("remove:", header)
            txt = txt[: m.start()] + txt[end:]
            changed = True
            break

txt = re.sub(
    r"(?m)^(\s*)include\s+enable-php[^;]*;",
    r"\1#include enable-php-00.conf;",
    txt,
)

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

servers = list(find_servers(txt))
print(f"server blocks: {len(servers)}")
if not servers:
    raise SystemExit("ERROR: no server blocks found — aborting so we do not write a hollow conf")

for start, end in sorted(servers, reverse=True):
    block = txt[start:end]
    inner, close = block[:-1], block[-1]
    # strip any root location left inside
    while True:
        m = re.search(r"(?m)^[ \t]*location[ \t]+/[ \t]*(?:\{|$)", inner)
        if not m:
            break
        open_b = inner.find("{", m.start())
        if open_b < 0:
            break
        e = brace_end(inner, open_b)
        inner = inner[: m.start()] + inner[e:]
    if "proxy_pass http://127.0.0.1:$tool_port" not in inner:
        inner = inner.rstrip() + "\n\n" + PROXY + "\n"
    else:
        # ensure exactly one location /
        roots = len(re.findall(r"(?m)^[ \t]*location[ \t]+/[ \t]*(?:\{|$)", inner))
        if roots == 0:
            inner = inner.rstrip() + "\n\n" + PROXY + "\n"
    txt = txt[:start] + inner + close + txt[end:]

for i, (start, end) in enumerate(find_servers(txt)):
    b = txt[start:end]
    n = len(re.findall(r"(?m)^[ \t]*location[ \t]+/[ \t]*(?:\{|$)", b))
    print(f"verify server[{i}] location / count={n}")
    if n != 1:
        raise SystemExit(f"bad location / count {n}")
    if not re.search(r"proxy_pass http://127\.0\.0\.1:\$tool_port", b):
        raise SystemExit("missing proxy_pass in server block")
    if re.search(r"(?m)location\s+~[^\n]*(js\|css|css\|js)", b):
        raise SystemExit("static js|css location still present")

if "client_header_buffer_size" not in txt:
    txt = re.sub(
        r"(?m)^(\s*)root\s+[^;]+;",
        r"\1root /www/wwwroot/gt4rents.com;\n\n\1client_max_body_size 100m;\n\1client_header_buffer_size 256k;\n\1large_client_header_buffers 8 256k;",
        txt,
        count=1,
    )

path.write_text(txt)
print("PATCHED:", path)
PY

echo "== nginx -t && reload =="
nginx -t
nginx -s reload

echo "== locations =="
grep -nE 'listen|server_name|location|proxy_pass|js\|css' "$VHOST" | head -50

echo "== map pride =="
grep 'prideseotools' "$MAP_DST" || grep 'prideseotools' "$MAP_SRC" || true

echo "== CDN + home (local) =="
for path in /cdn/assets/css/fonts.css /site-explorer /; do
  echo "--- $path ---"
  curl -skI --resolve refs.prideseotools.com:443:127.0.0.1 \
    "https://refs.prideseotools.com$path" | head -8
done
curl -sk --resolve refs.prideseotools.com:443:127.0.0.1 \
  "https://refs.prideseotools.com/cdn/assets/css/fonts.css" | head -c 80; echo
