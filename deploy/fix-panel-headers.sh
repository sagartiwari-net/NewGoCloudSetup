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
if t == orig:
    print("panel nginx already accepts large cookies")
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
