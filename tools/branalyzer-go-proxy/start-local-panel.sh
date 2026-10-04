#!/bin/bash
# Local panel-mode test (closer to gt4rents server path).
# Prints an /access URL — open that in Chrome (not /home directly).
set -e
cd "$(dirname "$0")"

PANEL_DB="${PANEL_DB:-/Users/sagartiwari/Desktop/oneclickgo/pending-tools/panel-api/data/panel.db}"
PORT=5081
HOST="127.0.0.1:${PORT}"

if [[ ! -f "$PANEL_DB" ]]; then
  echo "Missing panel.db: $PANEL_DB"
  exit 1
fi
if [[ ! -f cookie.txt ]]; then
  echo "Missing cookie.txt (GoAuto localStorage with @@auth0spajs@@)"
  exit 1
fi

python3 <<PY
import json
from pathlib import Path
cfg = json.loads(Path("config.json").read_text()) if Path("config.json").exists() else {}
# start from local template fields
base = json.loads(Path("config.local.json").read_text()) if Path("config.local.json").exists() else cfg
base.update({
    "panel_db": "$PANEL_DB",
    "port": "$PORT",
    "bypass_auth": False,
    "public_host": "$HOST",
    "public_scheme": "http",
    "local_test_mode": True,
    "home_path": "/home",
    "cookie_file": "cookie.txt",
})
Path("config.local.panel.json").write_text(json.dumps(base, indent=2) + "\n")
Path("config.json").write_text(json.dumps(base, indent=2) + "\n")
print("wrote config.local.panel.json")
PY

# Keep panel website domain + cookie in sync with cookie.txt
python3 <<PY
import sqlite3
from pathlib import Path
db = sqlite3.connect("$PANEL_DB")
cookie = Path("cookie.txt").read_text()
db.execute("UPDATE websites SET domain=? WHERE id=58", ("$HOST",))
db.execute(
    "UPDATE accounts SET cookie=?, status='active', cookie_updated_at=datetime('now') WHERE website_id=58",
    (cookie,),
)
db.commit()
print("synced panel website 58 → $HOST + cookie.txt")
PY

export BRANALYZER_CONFIG="config.local.panel.json"
cp config.local.panel.json config.json

echo "Building Branalyzer proxy (panel local)..."
go build -o branalyzer-go-proxy .

if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

# Mint one-time access token
TOKEN=$(python3 <<PY
import secrets, sqlite3
from datetime import datetime, timedelta, timezone
db = sqlite3.connect("$PANEL_DB")
tok = secrets.token_hex(16)
now = datetime.now(timezone.utc)
exp = (now + timedelta(hours=2)).strftime("%Y-%m-%dT%H:%M:%SZ")
created = now.strftime("%Y-%m-%dT%H:%M:%SZ")
db.execute(
    "INSERT INTO access_tokens(token,website_id,username,product_id,expires_at,created_at,client_ip) VALUES (?,?,?,?,?,?,?)",
    (tok, 58, "localtest", "bran-local", exp, created, "127.0.0.1"),
)
db.commit()
print(tok)
PY
)

ACCESS="http://${HOST}/access?user=localtest&token=${TOKEN}"

echo "=========================================="
echo "  Branalyzer LOCAL (panel mode)"
echo "  Open THIS access link (once):"
echo "  $ACCESS"
echo "  Then you should land on /home with search UI"
echo "=========================================="

# Write link for convenience
echo "$ACCESS" > /tmp/branalyzer-local-access.url
echo "(also saved: /tmp/branalyzer-local-access.url)"

exec ./branalyzer-go-proxy
