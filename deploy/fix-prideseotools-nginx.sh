#!/usr/bin/env bash
# Patch aaPanel prideseotools.com for Method A.
# Removes aaPanel static js|css locations (they 404 /cdn/*.css → unstyled Ahrefs).
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

# If current conf is broken (duplicate location), restore newest bak first
if ! nginx -t 2>/dev/null; then
  bak=$(ls -1t /www/server/panel/vhost/nginx/prideseotools.com.conf.bak-pride-* 2>/dev/null | head -1 || true)
  if [[ -n "${bak:-}" ]]; then
    echo "restoring broken vhost from $bak"
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

def strip_matching_locations(src, pred):
    """Remove location blocks where the location line matches pred(line)."""
    out = src
    while True:
        m = re.search(r"(?m)^[ \t]*location[ \t]+", out)
        if not m:
            break
        # find end of location directive line(s) until {
        open_b = out.find("{", m.start())
        if open_b < 0:
            break
        header = out[m.start():open_b]
        # normalize whitespace for predicate
        header_one = re.sub(r"\s+", " ", header).strip()
        end = brace_end(out, open_b)
        if pred(header_one):
            print("remove:", header_one[:120])
            out = out[: m.start()] + out[end:]
            continue
        # skip this block: advance by replacing with placeholder? use mark
        # Instead search from end of this block next iteration — use a non-greedy walk
        # Mark kept blocks by temporarily renaming "location" → "loc_kept"
        out = out[: m.start()] + "loc_kept" + out[m.start() + len("location") :]
    return out.replace("loc_kept", "location")

# Wildcard server_name
txt, n = re.subn(
    r"(?m)^(\s*)server_name\s+[^;]+;",
    r"\1server_name *.prideseotools.com prideseotools.com;",
    txt,
)
print(f"server_name replacements: {n}")

# Remove exact location / and any static regex asset locations
def should_remove(header: str) -> bool:
    # exact /
    if re.match(r"location\s+/\s*\{?$", header) or re.match(r"location\s+/\s*$", header.rstrip("{").strip()):
        # location /  or location / {
        if re.match(r"location\s+/\s*\{?$", header.replace(" {", "").strip() + ("{" if header.endswith("{") else "")):
            pass
    h = header.strip()
    # location / {   or  location /
    if re.fullmatch(r"location\s+/", h.rstrip("{").strip()) or re.fullmatch(r"location\s+/\s*\{?", h):
        return True
    if re.match(r"location\s+/\s*\{", h) or h in ("location /", "location / {"):
        return True
    # aaPanel static
    if "location ~" in h and re.search(r"js|css|woff|gif|jpg|png|bmp|swf|webp|svg", h, re.I):
        return True
    return False

# Simpler strip loop
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
        # exact root location /
        if re.fullmatch(r"location /", header) or re.fullmatch(r"location / \{?", header):
            remove = True
        if header == "location /" or header.startswith("location / {"):
            remove = True
        if re.match(r"^location /$", header) or re.match(r"^location /\s*$", header):
            remove = True
        # only exact / — not /foo
        if re.match(r"^location\s+/\s*$", header):
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

# Insert one PROXY per server block
def find_servers(src):
    for m in re.finditer(r"(?m)^[ \t]*server[ \t]*\{", src):
        open_b = src.find("{", m.start())
        yield m.start(), brace_end(src, open_b)

servers = list(find_servers(txt))
print(f"server blocks: {len(servers)}")
for start, end in sorted(servers, reverse=True):
    block = txt[start:end]
    inner, close = block[:-1], block[-1]
    # drop any leftover proxy-only crumbs without location (shouldn't happen)
    if "proxy_pass http://127.0.0.1:$tool_port" in inner and re.search(r"(?m)^[ \t]*location[ \t]+/", inner):
        # already has root location with proxy — leave
        roots = len(re.findall(r"(?m)^[ \t]*location[ \t]+/[ \t]*(\{|$)", inner))
        if roots == 1:
            print("server already has one location /")
            continue
    # strip any remaining root locations inside this block
    while True:
        m = re.search(r"(?m)^[ \t]*location[ \t]+/[ \t]*(\{|$)", inner)
        if not m:
            break
        open_b = inner.find("{", m.start())
        if open_b < 0:
            break
        e = brace_end(inner, open_b)
        inner = inner[: m.start()] + inner[e:]
    inner = inner.rstrip() + "\n\n" + PROXY + "\n"
    txt = txt[:start] + inner + close + txt[end:]

# Sanity
for i, (start, end) in enumerate(find_servers(txt)):
    b = txt[start:end]
    n = len(re.findall(r"(?m)^[ \t]*location[ \t]+/[ \t]*(\{|$)", b))
    print(f"verify server[{i}] location / count={n}")
    if n != 1:
        raise SystemExit(f"bad location / count {n} in server {i}")
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
grep -nE 'location|proxy_pass|js\|css' "$VHOST" | head -40

echo "== CDN local (must text/css) =="
curl -skI --resolve refs.prideseotools.com:443:127.0.0.1 \
  "https://refs.prideseotools.com/cdn/assets/css/fonts.css" | head -12
curl -sk --resolve refs.prideseotools.com:443:127.0.0.1 \
  "https://refs.prideseotools.com/cdn/assets/css/fonts.css" | head -c 80; echo
