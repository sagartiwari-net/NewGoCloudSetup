#!/usr/bin/env bash
# Fix https://barapi.upays.in nginx 404.
# Same multi-map sync pattern as fix-nox / fix-t4toolz (one map copy is not enough).
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

echo "=== nginx map includes (live) ==="
nginx -T 2>/dev/null | grep -E 'host-port\.map|include.*map' | head -30 || true

echo "=== sync map → all aaPanel map paths ==="
grep -E 'barapi\.(upays|gt4rents)' "${MAP_SRC}"
for dest in "${MAP_DSTS[@]}"; do
  dir=$(dirname "${dest}")
  [[ -d "${dir}" ]] || continue
  cp -f "${MAP_SRC}" "${dest}"
  echo "OK → ${dest}"
done

# Dedicated vhost (Method B) — does not depend on $tool_port map
# Reuse SSL cert lines from upays.in wildcard when present.
echo "=== write ${BAR_VHOST} ==="
SSL_CERT=""
SSL_KEY=""
if [[ -f "${UPAYS_VHOST}" ]]; then
  SSL_CERT=$(grep -E '^\s*ssl_certificate\s+' "${UPAYS_VHOST}" | head -1 | awk '{print $2}' | tr -d ';')
  SSL_KEY=$(grep -E '^\s*ssl_certificate_key\s+' "${UPAYS_VHOST}" | head -1 | awk '{print $2}' | tr -d ';')
fi

{
  cat <<'EOF'
# Ahrefs Bar API — dedicated host (Method B). Do not point this at refs :5291.
server {
    listen 80;
    server_name barapi.upays.in barapi.gt4rents.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name barapi.upays.in barapi.gt4rents.com;
EOF
  if [[ -n "${SSL_CERT}" && -n "${SSL_KEY}" ]]; then
    echo "    ssl_certificate ${SSL_CERT};"
    echo "    ssl_certificate_key ${SSL_KEY};"
  else
    cat <<'EOF'
    # aaPanel: set SSL on this site or copy ssl_certificate lines from upays.in.conf
EOF
  fi
  cat <<'EOF'

    client_max_body_size 8m;

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
echo "wrote ${BAR_VHOST} (cert=${SSL_CERT:-missing})"

nginx -t
nginx -s reload

echo "=== smoke ==="
echo -n "local :8088 → "
curl -s http://127.0.0.1:8088/v1/portals; echo
echo -n "https://barapi.upays.in/v1/portals → "
curl -sk https://barapi.upays.in/v1/portals; echo
curl -skI https://barapi.upays.in/v1/portals | head -6
