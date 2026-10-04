#!/bin/bash
# Update Mapped Account cookie in MySQL from cookie.txt
# Usage (on server as root):
#   cd /www/wwwroot/1clkaccess.store/seosite
#   # Upload fresh cookie.txt (must include ssc.iam while logged in)
#   bash update-db-cookie.sh
set -e

APP_DIR="/www/wwwroot/1clkaccess.store/seosite"
COOKIE_FILE="${APP_DIR}/cookie.txt"
CONFIG_FILE="${APP_DIR}/config.json"
WEBSITE_ID=49
ACCOUNT_ID=27

cd "$APP_DIR"

if [ ! -f "$COOKIE_FILE" ]; then
  echo "ERROR: cookie.txt not found at $COOKIE_FILE"
  echo ""
  echo "Steps:"
  echo "  1. Login to https://app.seositecheckup.com in Chrome"
  echo "  2. Cookie-Editor extension → Export → JSON"
  echo "  3. Save as cookie.txt and upload to server"
  exit 1
fi

if ! grep -q 'ssc.iam' "$COOKIE_FILE"; then
  echo "ERROR: cookie.txt has no ssc.iam!"
  echo "Export cookies WHILE LOGGED IN to app.seositecheckup.com"
  exit 1
fi

MYSQL_USER=$(python3 -c "import json; print(json.load(open('$CONFIG_FILE'))['mysql_user'])")
MYSQL_PASS=$(python3 -c "import json; print(json.load(open('$CONFIG_FILE'))['mysql_password'])")
MYSQL_DB=$(python3 -c "import json; print(json.load(open('$CONFIG_FILE'))['mysql_db'])")

COOKIE_SIZE=$(wc -c < "$COOKIE_FILE" | tr -d ' ')
echo "OK: cookie.txt has ssc.iam ($COOKIE_SIZE bytes)"

python3 << PY
import json, pathlib, subprocess, sys

cookie = pathlib.Path("$COOKIE_FILE").read_text()
user, pwd, db = "$MYSQL_USER", "$MYSQL_PASS", "$MYSQL_DB"
acc_id, wid = $ACCOUNT_ID, $WEBSITE_ID

if "ssc.iam" not in cookie:
    print("ERROR: no ssc.iam in cookie.txt")
    sys.exit(1)

sql = (
    "UPDATE ahrefs_accounts "
    "SET cookie=%s, status='active', failure_count=0 "
    f"WHERE id={acc_id} AND website_id={wid}"
)

# mysql CLI with -e and escaped string
import shlex
proc = subprocess.run(
    ["mysql", f"-u{user}", f"-p{pwd}", db, "-e", sql.replace("%s", "%s")],
    input=None,
    capture_output=True,
    text=True,
)
# Use pymysql-free approach: write SQL file with proper escaping
escaped = cookie.replace("\\", "\\\\").replace("'", "''")
sql_file = pathlib.Path("/tmp/seosite_cookie_update.sql")
sql_file.write_text(
    f"UPDATE ahrefs_accounts SET cookie='{escaped}', status='active', failure_count=0 "
    f"WHERE id={acc_id} AND website_id={wid};\n"
)
r = subprocess.run(["mysql", f"-u{user}", f"-p{pwd}", db], stdin=sql_file.open(), capture_output=True, text=True)
if r.returncode != 0:
    print("MySQL error:", r.stderr)
    sys.exit(1)
sql_file.unlink(missing_ok=True)
print("OK: cookie updated in database")
PY

mysql -u "$MYSQL_USER" -p"$MYSQL_PASS" "$MYSQL_DB" -e "
SELECT id, name, status,
  CASE WHEN cookie LIKE '%ssc.iam%' THEN 'YES' ELSE 'NO' END AS has_iam,
  LENGTH(cookie) AS cookie_bytes
FROM ahrefs_accounts WHERE website_id = ${WEBSITE_ID};
"

echo ""
echo "Done. Restart: systemctl restart seositecheckup-proxy"
