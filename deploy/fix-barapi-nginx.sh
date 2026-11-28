#!/usr/bin/env bash
# Fix https://barapi.upays.in nginx 404.
# Diagnose DNS vs local, pin barapi on upays wildcard, keep dedicated vhost.
set -euo pipefail

REPO="${REPO:-/www/wwwroot/gt4rents.com/_repo}"
MAP_SRC="${REPO}/deploy/nginx-host-port.map.conf"
MAP_DSTS=(
  /www/server/panel/vhost/nginx/gt4rents-host-port.map.conf
  /www/server/panel/vhost/nginx/nginx-host-port.map.conf
  /www/server/panel/vhost/nginx/extension/nginx-host-port.map.conf
  /www/server/nginx/conf/nginx-host-port.map.conf
)
UPAYS_VHOST="/www/server/panel/vhost/nginx/upays.in.conf"
BAR_VHOST="/www/server/panel/vhost/nginx/barapi.upays.in.conf"

echo "=== DNS / local IPs ==="
getent ahosts barapi.upays.in 2>/dev/null | head -5 || getent hosts barapi.upays.in || true
getent ahosts refs.upays.in 2>/dev/null | head -3 || true
echo -n "local: "; hostname -I 2>/dev/null | awk '{print $1}'

echo "=== barapi in live map ==="
grep -n 'barapi' "${MAP_SRC}" || echo "WARN: no barapi in ${MAP_SRC}"
nginx -T 2>/dev/null | grep -E 'include .*host-port|barapi\.(upays|gt4rents)' | head -30 || true

echo "=== sync map copies ==="
for dest in "${MAP_DSTS[@]}"; do
  dir=$(dirname "${dest}")
  [[ -d "${dir}" ]] || continue
  cp -f "${MAP_SRC}" "${dest}"
  echo "OK → ${dest}"
done

if [[ ! -f "${UPAYS_VHOST}" ]]; then
  echo "ERROR: missing ${UPAYS_VHOST}"
  exit 1
fi

cp -a "${UPAYS_VHOST}" "${UPAYS_VHOST}.bak-barapi-$(date +%Y%m%d%H%M%S)"

# Per server{} that already proxies via $tool_port: inject pin + optional debug,
# retarget that block's 404/proxy_pass to $gt4_port (never global replace).
python3 - "${UPAYS_VHOST}" <<'PY'
import pathlib, re, sys

path = pathlib.Path(sys.argv[1])
txt = path.read_text()

PIN_MARK = "# gt4 barapi pin"
PIN = """    # gt4 barapi pin — Ahrefs Bar API :8088
    set $gt4_port $tool_port;
    if ($host = barapi.upays.in) { set $gt4_port 8088; }
    if ($host = barapi.gt4rents.com) { set $gt4_port 8088; }
"""
DEBUG = """    location = /__gt4bar {
        default_type text/plain;
        return 200 "host=$host tool_port=$tool_port gt4_port=$gt4_port\\n";
    }
"""

def brace_end(src: str, i: int) -> int:
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

def find_servers(src: str):
    for m in re.finditer(r"(?m)^[ \t]*server\b", src):
        open_b = src.find("{", m.start())
        if open_b < 0:
            continue
        mid = src[m.end() : open_b]
        if re.match(r"^[ \t\r\n]*$", mid):
            yield m.start(), brace_end(src, open_b)

out = txt
patched = 0
for start, end in sorted(find_servers(txt), reverse=True):
    block = out[start:end]
    if "proxy_pass http://127.0.0.1:$tool_port" not in block and "proxy_pass http://127.0.0.1:$gt4_port" not in block:
        continue
    # strip old pin
    block = re.sub(r"(?m)^[ \t]*# gt4 barapi pin[^\n]*\n", "", block)
    block = re.sub(r"(?m)^[ \t]*set \$gt4_port \$tool_port;[ \t]*\n", "", block)
    block = re.sub(
        r"(?m)^[ \t]*if \(\$host = barapi\.(?:upays\.in|gt4rents\.com)\) \{ set \$gt4_port \d+; \}[ \t]*\n",
        "",
        block,
    )
    # strip old debug location
    while True:
        m = re.search(r"(?m)^[ \t]*location[ \t]+=[ \t]*/__gt4bar[ \t]*(?:\{|\n[ \t]*\{)", block)
        if not m:
            break
        open_b = block.find("{", m.start())
        block = block[: m.start()] + block[brace_end(block, open_b) :]

    # insert pin before first location /
    m = re.search(r"(?m)^[ \t]*location[ \t]+/", block)
    if not m:
        continue
    block = block[: m.start()] + PIN + "\n" + DEBUG + "\n" + block[m.start() :]
    block = block.replace("if ($tool_port = 0)", "if ($gt4_port = 0)")
    block = block.replace(
        "proxy_pass http://127.0.0.1:$tool_port",
        "proxy_pass http://127.0.0.1:$gt4_port",
    )
    out = out[:start] + block + out[end:]
    patched += 1
    print(f"patched server@{start}")

if patched == 0:
    print("WARN: no $tool_port/$gt4_port proxy_pass in upays.in.conf — dedicated vhost only")
path.write_text(out)
print("PATCHED", path, "servers=", patched)
PY

SSL_CERT=$(grep -E '^\s*ssl_certificate\s+' "${UPAYS_VHOST}" | head -1 | awk '{print $2}' | tr -d ';' || true)
SSL_KEY=$(grep -E '^\s*ssl_certificate_key\s+' "${UPAYS_VHOST}" | head -1 | awk '{print $2}' | tr -d ';' || true)

{
  cat <<EOF
# Ahrefs Bar API — dedicated host (Method B). Exact name beats *.upays.in.
server {
    listen 80;
    listen [::]:80;
    server_name barapi.upays.in barapi.gt4rents.com;
    return 301 https://\$host\$request_uri;
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name barapi.upays.in barapi.gt4rents.com;
EOF
  if [[ -n "${SSL_CERT}" && -n "${SSL_KEY}" ]]; then
    echo "    ssl_certificate ${SSL_CERT};"
    echo "    ssl_certificate_key ${SSL_KEY};"
  fi
  cat <<'EOF'

    client_max_body_size 8m;

    location = /__gt4bar {
        default_type text/plain;
        return 200 "barapi-dedicated\n";
    }

    location / {
        proxy_pass http://127.0.0.1:8088;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_connect_timeout 10s;
        proxy_read_timeout 60s;
    }
}
EOF
} > "${BAR_VHOST}"
echo "wrote ${BAR_VHOST} cert=${SSL_CERT:-none}"

nginx -t
nginx -s reload

echo "=== smoke ==="
echo -n "1) local :8088 → "
curl -s --max-time 5 http://127.0.0.1:8088/v1/portals; echo

echo -n "2) HTTPS via 127.0.0.1 (--resolve) /__gt4bar → "
curl -sk --http1.1 --max-time 5 --resolve barapi.upays.in:443:127.0.0.1 \
  https://barapi.upays.in/__gt4bar; echo

echo -n "3) HTTPS via 127.0.0.1 /v1/portals → "
curl -sk --http1.1 --max-time 5 --resolve barapi.upays.in:443:127.0.0.1 \
  https://barapi.upays.in/v1/portals; echo

echo -n "4) HTTPS public -4 /v1/portals → "
curl -4 -sk --http1.1 --max-time 8 https://barapi.upays.in/v1/portals; echo

echo -n "5) refs.upays.in status → "
curl -4 -sk --max-time 8 -o /dev/null -w '%{http_code}\n' https://refs.upays.in/ || true

echo "If (3) OK but (4) 404 → DNS/Cloudflare for barapi does not hit this box (fix grey A → this IP)."
echo "If (3) shows host=… tool_port=0 → map not applied; check nginx -T map include."
echo "If (3) shows barapi-dedicated → Method B vhost is winning locally."
