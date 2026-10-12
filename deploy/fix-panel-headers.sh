#!/usr/bin/env bash
# Panel Chrome error "This page couldn't load" is a reset when the Cookie
# header is bigger than nginx or Node will accept. Raise both, then restart the UI.
set -euo pipefail
CONF="/www/server/panel/vhost/nginx/panel.gt4rents.com.conf"

python3 - "$CONF" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1])
if not p.is_file():
    raise SystemExit("missing " + str(p))
t = p.read_text()
orig = t
t = t.replace("client_header_buffer_size 128k;", "client_header_buffer_size 512k;")
t = t.replace("client_header_buffer_size 32k;", "client_header_buffer_size 512k;")
t = t.replace("client_header_buffer_size 16k;", "client_header_buffer_size 512k;")
t = t.replace("client_header_buffer_size 8k;", "client_header_buffer_size 512k;")
t = t.replace("large_client_header_buffers 4 8k;", "large_client_header_buffers 8 512k;")
t = t.replace("large_client_header_buffers 4 16k;", "large_client_header_buffers 8 512k;")
t = t.replace("large_client_header_buffers 4 32k;", "large_client_header_buffers 8 512k;")
t = t.replace("large_client_header_buffers 8 128k;", "large_client_header_buffers 8 512k;")
t = t.replace("proxy_set_header Connection $connection_upgrade;", 'proxy_set_header Connection "";')
t = t.replace("proxy_set_header Connection $http_connection;", 'proxy_set_header Connection "";')
if "client_header_buffer_size" not in t:
    t = t.replace(
        "server_name panel.gt4rents.com;",
        "server_name panel.gt4rents.com;\n    client_header_buffer_size 512k;\n    large_client_header_buffers 8 512k;",
    )
snippet = """
    # Missing /_next/static files are the previous build. Reload the document once.
    location /_next/static/ {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header Connection "";
        proxy_intercept_errors on;
        error_page 404 = @panel_stale_chunk;
    }

    location @panel_stale_chunk {
        default_type application/javascript;
        add_header Cache-Control "no-store" always;
        return 200 '(function(){try{if(sessionStorage.getItem("panel-static-reload")==="1")return;sessionStorage.setItem("panel-static-reload","1");var u=new URL(location.href);u.searchParams.set("__v",String(Date.now()));location.replace(u.toString())}catch(e){}})();';
    }

"""
parts = t.split("server {")
out = [parts[0]]
patched_server = False
for part in parts[1:]:
    if "location / {" in part and "@panel_stale_chunk" not in part and "127.0.0.1:3000" in part:
        part = part.replace("location / {", snippet + "    location / {", 1)
        patched_server = True
    out.append(part)
if len(parts) > 1 and not patched_server and "@panel_stale_chunk" not in t:
    raise SystemExit("panel nginx has no UI location / block")
t = "server {".join(out)

if t == orig:
    print("panel nginx already accepts large cookies and reloads a stale build")
else:
    p.with_name(p.name + ".bak-headers").write_text(orig)
    p.write_text(t)
    print("patched", p)
PY

nginx -t
nginx -s reload
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
"$ROOT/deploy/start-panel-ui.sh"
echo "Panel UI restarted. Reload panel.gt4rents.com/accounts."
