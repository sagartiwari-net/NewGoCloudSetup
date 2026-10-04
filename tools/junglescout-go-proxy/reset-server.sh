#!/bin/bash
# Server setup — run inside /www/wwwroot/1clkaccess.store/jungle/
set -e

DIR="/www/wwwroot/1clkaccess.store/jungle"
PORT="8785"
BIN="junglescout-go-proxy"
BUILD_TAG="jungle-v11-datadome-fix"
SERVICE="jungle-scout"

port_in_use() {
  ss -tlnp 2>/dev/null | grep -qE ":${PORT}([^0-9]|$)"
}

port_holders() {
  ss -tlnp 2>/dev/null | grep -E ":${PORT}([^0-9]|$)" || true
}

kill_port_holders() {
  systemctl stop "$SERVICE" 2>/dev/null || true
  # Old service name from previous Jungle Scout installs
  systemctl stop junglescout-go-proxy 2>/dev/null || true
  sleep 1

  local pids
  pids=$(ss -tlnp 2>/dev/null | grep -E ":${PORT}([^0-9]|$)" | grep -oE 'pid=[0-9]+' | cut -d= -f2 | sort -u || true)
  if [ -n "$pids" ]; then
    echo "Killing port ${PORT} PIDs: $pids"
    kill -9 $pids 2>/dev/null || true
  fi

  fuser -k ${PORT}/tcp 2>/dev/null || true
  pkill -9 -f "${DIR}/${BIN}" 2>/dev/null || true
  pkill -9 -f "junglescout-go-proxy" 2>/dev/null || true
  sleep 2
}

ensure_port_free() {
  local label="$1"
  for i in 1 2 3 4 5 6; do
    if ! port_in_use; then
      return 0
    fi
    echo "WARN: port $PORT still in use during ${label} (attempt $i)"
    port_holders
    kill_port_holders
  done
  if port_in_use; then
    echo "ERROR: port $PORT still in use after ${label}. Run:"
    echo "  ss -tlnp | grep ${PORT}"
    port_holders
    exit 1
  fi
}

read_mysql_pass() {
  local file="$1"
  [ -f "$file" ] || return 0
  python3 -c 'import json,sys
try:
  p=json.load(open(sys.argv[1])).get("mysql_password","")
  if p and p != "CHANGE_ME_ON_SERVER":
    print(p)
except Exception:
  pass' "$file" 2>/dev/null || true
}

cd "$DIR"
export PATH="$PATH:/usr/local/go/bin:/root/go/bin"
export GOFLAGS="-buildvcs=false"
git config --global --add safe.directory "$DIR" 2>/dev/null || true

echo "=== Jungle Scout SERVER SETUP ==="
echo "Folder: $DIR | Port: $PORT"
echo ""

if [ -d .git ]; then
  git pull --ff-only origin main 2>/dev/null && echo "Git pull OK" || echo "WARN: git pull skipped (already up to date or offline)"
fi

echo "=== 1) Stop service + free port $PORT ==="
systemctl stop "$SERVICE" 2>/dev/null || true
systemctl reset-failed "$SERVICE" 2>/dev/null || true
kill_port_holders
ensure_port_free "initial cleanup"

chmod 644 cookie.txt 2>/dev/null || true

if [ -f config.production.json ]; then
  # Snapshot DB credentials BEFORE overwriting config.json.
  # Never re-read $DIR/config.json after the cp — that file is replaced.
  PRESERVED_SOURCE=""
  LOCAL_PASS=$(read_mysql_pass "$DIR/config.json")
  if [ -n "$LOCAL_PASS" ]; then
    cp "$DIR/config.json" /tmp/jungle-config-preserve.json
    PRESERVED_SOURCE="/tmp/jungle-config-preserve.json"
  else
    for candidate in \
      "/www/wwwroot/1clkaccess.store/hm/config.json" \
      "/www/wwwroot/1clkaccess.store/sellamp/config.json"; do
      pass=$(read_mysql_pass "$candidate")
      if [ -n "$pass" ]; then
        PRESERVED_SOURCE="$candidate"
        break
      fi
    done
  fi

  cp config.production.json config.json
  echo "Applied config.production.json → config.json"

  if [ -n "$PRESERVED_SOURCE" ] && [ -f "$PRESERVED_SOURCE" ]; then
    PRESERVED_SOURCE="$PRESERVED_SOURCE" python3 - <<'PY'
import json, os
source = json.load(open(os.environ["PRESERVED_SOURCE"]))
cfg = json.load(open("config.json"))
for key in ("mysql_host", "mysql_port", "mysql_user", "mysql_password", "mysql_db"):
    val = source.get(key)
    if val and val != "CHANGE_ME_ON_SERVER":
        cfg[key] = val
with open("config.json", "w") as f:
    json.dump(cfg, f, indent=2)
    f.write("\n")
print("Reused database credentials from", os.environ["PRESERVED_SOURCE"])
PY
  fi

  MYSQL_PASS=$(read_mysql_pass config.json)
  if [ -z "$MYSQL_PASS" ]; then
    echo "ERROR: mysql_password is not configured in config.json"
    echo "Run once:"
    echo "  python3 - <<'PY'"
    echo "import json"
    echo "src=json.load(open('/www/wwwroot/1clkaccess.store/hm/config.json'))"
    echo "cfg=json.load(open('config.json'))"
    echo "for k in ('mysql_host','mysql_port','mysql_user','mysql_password','mysql_db'):"
    echo "    if src.get(k): cfg[k]=src[k]"
    echo "json.dump(cfg, open('config.json','w'), indent=2); print('ok')"
    echo "PY"
    echo "Then re-run ./reset-server.sh"
    exit 1
  fi
elif [ -f config.server.json ]; then
  cp config.server.json config.json
  echo "Applied config.server.json → config.json"
fi

PROXY_SEC="/www/wwwroot/toolsmandi.com/proxy-security"
if [ ! -d "$PROXY_SEC" ]; then
  PROXY_SEC="/www/wwwroot/1clkaccess.store/proxy-security"
fi
if [ -d "$PROXY_SEC" ]; then
  sed -i "s|replace toolsmandi.com/proxy-security => .*|replace toolsmandi.com/proxy-security => ${PROXY_SEC}|" go.mod
  echo "Using proxy-security: $PROXY_SEC"
else
  echo "WARN: proxy-security not found — copy it before build"
fi

if ! command -v go &>/dev/null; then
  echo "ERROR: Go not installed. Try: export PATH=\$PATH:/usr/local/go/bin"
  exit 1
fi
echo "Go: $(command -v go) ($(go version))"

echo "=== 2) Build ==="
go mod tidy
go mod download
CGO_ENABLED=0 go build -buildvcs=false -ldflags="-s -w" -o "$BIN" .
chmod +x "$BIN"
file "$BIN"

echo "=== 3) Verify build tag ==="
strings "$BIN" | grep -q "$BUILD_TAG" && echo "OK: $BUILD_TAG in binary" || {
  echo "ERROR: expected build tag $BUILD_TAG — upload latest automation_security.go"
  exit 1
}

echo "=== 4) Install systemd service ==="
cp jungle-scout.service /etc/systemd/system/jungle-scout.service
systemctl daemon-reload
systemctl enable jungle-scout

echo "=== 5) Final port check before start ==="
systemctl stop "$SERVICE" 2>/dev/null || true
kill_port_holders
ensure_port_free "pre-start"

systemctl start "$SERVICE"
sleep 3

if ! systemctl is-active --quiet "$SERVICE"; then
  echo "ERROR: $SERVICE failed to start"
  echo "--- who holds port $PORT? ---"
  port_holders
  journalctl -u "$SERVICE" -n 30 --no-pager
  exit 1
fi

systemctl status jungle-scout --no-pager | head -12
curl -s -o /dev/null -w "localhost:${PORT}/ → HTTP %{http_code}\n" \
  "http://127.0.0.1:${PORT}/" \
  -H "User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36" || true

HDR=$(curl -sI "http://127.0.0.1:${PORT}/" -H "User-Agent: Mozilla/5.0" | grep -i x-proxy-build || true)
echo "$HDR"
echo "$HDR" | grep -q "$BUILD_TAG" && echo "OK: live build tag verified" || echo "WARN: header missing or old build"

echo ""
echo "aaPanel reverse proxy: http://127.0.0.1:${PORT}"
echo "Domain: https://jungle.1clkaccess.store/"
echo "Extension: https://jungle.1clkaccess.store/ext-install"
echo ""
echo "Live logs:"
echo "  journalctl -u jungle-scout -f"
