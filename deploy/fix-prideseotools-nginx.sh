#!/usr/bin/env bash
# Patch aaPanel prideseotools.com vhost for Method A (wildcard + $tool_port).
# Keeps existing SSL / listen 443 cert lines. Apex @ DNS optional (homepage elsewhere OK).
# DNS grey A required: * → 65.109.16.196
set -euo pipefail

REPO="${REPO:-/www/wwwroot/gt4rents.com/_repo}"
MAP_SRC="$REPO/deploy/nginx-host-port.map.conf"
MAP_DST="/www/server/panel/vhost/nginx/gt4rents-host-port.map.conf"
VHOST="/www/server/panel/vhost/nginx/prideseotools.com.conf"
REWRITE="/www/server/panel/vhost/rewrite/prideseotools.com.conf"

if [[ ! -f "$VHOST" ]]; then
  echo "ERROR: missing $VHOST — create aaPanel site prideseotools.com first"
  exit 1
fi

cp -a "$VHOST" "${VHOST}.bak-pride-$(date +%Y%m%d%H%M%S)"
touch "$REWRITE"

if [[ -f "$MAP_SRC" ]]; then
  cp "$MAP_SRC" "$MAP_DST"
  echo "OK map → $MAP_DST"
fi

python3 - "$VHOST" <<'PY'
import pathlib, re, sys

path = pathlib.Path(sys.argv[1])
txt = path.read_text()

# Every server_name → wildcard + apex
txt2, n = re.subn(
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
    }"""

# Match location / { ... } with nested braces depth tracking
def replace_root_locations(src: str) -> str:
    out = []
    i = 0
    replaced = 0
    while i < len(src):
        m = re.match(r"(?m)^(\s*)location\s+/\s*\{", src[i:])
        if not m:
            out.append(src[i])
            i += 1
            continue
        # start of location /
        start = i + m.start()
        brace_at = i + m.end() - 1  # position of '{'
        depth = 0
        j = brace_at
        while j < len(src):
            if src[j] == "{":
                depth += 1
            elif src[j] == "}":
                depth -= 1
                if depth == 0:
                    j += 1
                    break
            j += 1
        out.append(PROXY)
        if not out[-1].endswith("\n"):
            out.append("\n")
        i = j
        replaced += 1
    print(f"location / replacements: {replaced}")
    return "".join(out)

if "proxy_pass http://127.0.0.1:$tool_port" in txt2 and "location /" in txt2:
    # still rewrite root locations so headers match template
    txt2 = replace_root_locations(txt2)
else:
    txt2 = replace_root_locations(txt2)

# Buffers once after first root line if missing
if "client_header_buffer_size" not in txt2:
    txt2 = re.sub(
        r"(?m)^(\s*)root\s+[^;]+;",
        r"\1root /www/wwwroot/gt4rents.com;\n\n\1client_max_body_size 100m;\n\1client_header_buffer_size 256k;\n\1large_client_header_buffers 8 256k;",
        txt2,
        count=1,
    )

path.write_text(txt2)
print("PATCHED:", path)
PY

echo "== nginx -t && reload =="
nginx -t
nginx -s reload

echo "== key lines =="
grep -nE 'server_name|proxy_pass|tool_port|listen |ssl_certificate ' "$VHOST" | head -40
echo "== map =="
grep prideseotools "$MAP_DST" || true
echo "== local Host test (HTTP) =="
curl -sI -H 'Host: refs.prideseotools.com' http://127.0.0.1/ | head -8 || true
echo "DONE. Padlock needs wildcard LE (DNS) later; routing/config is set."
