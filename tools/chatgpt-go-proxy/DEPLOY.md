# ChatGPT Proxy — Production Deploy Guide

ToolsMandi reverse proxy for `https://chatgpt.com`  
Example domain: `chat.1clkaccess.store` | Port: **6041**

---

## 1. Files — kya upload karna hai

### Server folder
```
/www/wwwroot/1clkaccess.store/chat/
```

### Zaroori files (ye hi upload karo)

| File | Kaam |
|------|------|
| `main.go` | Proxy code — **must show `chatgpt-v7`** on server |
| `security_bridge.go` | Security module bridge |
| `ip_blocklist.go` | IP blocklist |
| `go.mod` | Go dependencies |
| `go.sum` | Go checksums |
| `config.production.json` | Production settings (`cloudflare_bypass: true`) |
| `server-deploy.sh` | Auto build + systemd setup |

**Shared library (proxy-security) — bhi update karo server par:**

| Server path | Local path |
|-------------|------------|
| `/www/wwwroot/toolsmandi.com/proxy-security/ip.go` | `oneclickgo/proxy-security/ip.go` |
| `/www/wwwroot/toolsmandi.com/proxy-security/headers.go` | `oneclickgo/proxy-security/headers.go` |

### Mac se upload (scp example)

```bash
# Apna server IP lagao
SERVER=root@vmi3043169

scp /Users/sagartiwari/Desktop/oneclickgo/chatgpt-go-proxy/{main.go,security_bridge.go,ip_blocklist.go,go.mod,go.sum,config.production.json,server-deploy.sh} \
  $SERVER:/www/wwwroot/1clkaccess.store/chat/

scp /Users/sagartiwari/Desktop/oneclickgo/proxy-security/{ip.go,headers.go} \
  $SERVER:/www/wwwroot/toolsmandi.com/proxy-security/
```

Upload ke baad server par verify:
```bash
grep proxyBuildTag /www/wwwroot/1clkaccess.store/chat/main.go
# Expected: chatgpt-v7
```

### Optional
| File | Kaam |
|------|------|
| `cookie.txt` | Sirf local test ke liye; production me DB se cookies aati hain |

### Upload **MAT** karo
- `node_modules/`
- `package.json`
- `config.json` (local localhost wala)
- test screenshots / browser scripts

### Server par pehle se hona chahiye (dusre tools ki tarah)
```
/www/wwwroot/toolsmandi.com/proxy-security/
```
Ye shared library hai — har naye tool ke liye copy nahi karni, sirf ek baar server par honi chahiye.

### Dusri jagah (toolsmandi.com server)
| File | Change |
|------|--------|
| `/www/wwwroot/toolsmandi.com/route_tool.php` | `chatgpt` block me `website_id`, `domain`, `secret_key` |

---

## 2. Kaun si files me kya change hota hai (version history)

| File | Important changes |
|------|-------------------|
| **main.go** | v5: brotli/Accept-Encoding fix (icons) · v6: `wss://` HTTPS WebSocket + CDN cache |
| **config.production.json** | `port`, `public_host`, `public_scheme: https`, `use_database: true`, `bypass_auth: false` |
| **go.mod** | Local: `../proxy-security` · Server: `/www/wwwroot/toolsmandi.com/proxy-security` |
| **server-deploy.sh** | Build + systemd + go.mod path fix |
| **route_tool.php** | Member area handshake — `website_id` + domain match ctrl panel se |

Build tag check: `X-Proxy-Build: chatgpt-v7`

---

## 3. Ctrl Panel setup (ek baar)

1. **Website Domains** → Add:
   - Domain: `chat.1clkaccess.store` (apne dost ke liye alag subdomain)
   - Handshake Key: `toolsmandi_chatgpt_secret_xyz123`
   - Security: **ON**
   - Session: 30 min (ya jo chaho)
   - Note karo **website_id** (e.g. `51`)

2. **Mapped Accounts** → ChatGPT account:
   - JSON cookies (browser export)
   - `automation_ingest_key`: `chatgpt_acc1`

3. **route_tool.php** me same `website_id` + domain:
```php
'chatgpt' => [
    'website_id' => 51,   // ctrl panel wala ID
    'domain'     => 'chat.1clkaccess.store',
    'secret_key' => 'toolsmandi_chatgpt_secret_xyz123'
],
```

---

## 4. Server deploy — copy-paste commands

SSH root se:

```bash
# Step 1 — folder me jao
cd /www/wwwroot/1clkaccess.store/chat

# Step 2 — MySQL password set karo (sirf pehli baar)
nano config.production.json
# "mysql_password": "CHANGE_ME" → apna real password

# Step 3 — proxy-security path fix + build + start (RECOMMENDED)
chmod +x server-deploy.sh
bash server-deploy.sh
```

**Agar manually build karna ho** (server-deploy.sh ke bina):

```bash
cd /www/wwwroot/1clkaccess.store/chat

# go.mod me server path — YE ZAROORI HAI (warna build fail)
sed -i 's|replace toolsmandi.com/proxy-security => .*|replace toolsmandi.com/proxy-security => /www/wwwroot/toolsmandi.com/proxy-security|' go.mod

cp config.production.json config.json
go mod tidy
go build -o chatgpt-proxy .
chmod +x chatgpt-proxy
systemctl restart chatgpt-proxy
```

Verify:
```bash
curl -sI http://127.0.0.1:6041/ | grep X-Proxy-Build
# Expected: X-Proxy-Build: chatgpt-v7

journalctl -u chatgpt-proxy -n 10 --no-pager | grep website_id
# Expected: Resolved website_id = 51 for domain 'chat.1clkaccess.store'
```

---

## 5. Port map (same server par conflict na ho)

| Tool | Port |
|------|------|
| SEO Site Checkup | 6031 |
| **ChatGPT** | **6041** |
| BuzzSumo | 6011 |
| Airbrush | 6001 |

Naya tool add karte waqt free port check:
```bash
ss -tlnp | grep -E '6010|6011|6031|6041'
```

---

## 6. aaPanel — Nginx reverse proxy (WebSocket ke liye)

### Kahan add karna hai

1. aaPanel → **Website** → `chat.1clkaccess.store` → **Config** (ya Reverse proxy)
2. `location / { ... }` block dhundho — usually file ke end me
3. Purana `proxy_pass` replace karo ya Reverse Proxy UI me Target URL: `http://127.0.0.1:6041`

### Config file me ye block hona chahiye

```nginx
location / {
    proxy_pass http://127.0.0.1:6041;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_buffering off;
    proxy_read_timeout 86400s;
    proxy_send_timeout 86400s;
}
```

Save → Test → Reload:
```bash
nginx -t && nginx -s reload
```

> **Note:** aaPanel ka "Reverse proxy" button bhi use kar sakte ho — bas Target URL `http://127.0.0.1:6041` set karo aur WebSocket support ON rakho agar option ho.

---

## 7. SSL

aaPanel → Website → `chat.1clkaccess.store` → **SSL** → Let's Encrypt → Apply

`config.production.json` me confirm:
```json
"public_scheme": "https",
"public_host": "chat.1clkaccess.store"
```

---

## 8. Final test checklist

```bash
# 1. Proxy running
curl -sI http://127.0.0.1:6041/ | grep X-Proxy-Build

# 2. HTTPS through nginx
curl -sI https://chat.1clkaccess.store/ | grep X-Proxy-Build

# 3. DB domain registered
mysql -u toolsmandirefct -p toolsmandirefct -e \
  "SELECT id,domain,secret_key FROM ahrefs_websites WHERE domain='chat.1clkaccess.store';"
```

Browser:
1. `https://toolsmandi.com/route_tool.php?tool=chatgpt` → redirect hona chahiye
2. ChatGPT open → message bhejo → **turant reply** (refresh ki zaroorat nahi)
3. DevTools Console → **Mixed Content / ws:// error nahi** hona chahiye

---

## 9. Dost ke liye naya tool add karna (quick)

1. Ctrl panel → naya subdomain (e.g. `chat2.1clkaccess.store`) → note `website_id`
2. Naya folder: `/www/wwwroot/1clkaccess.store/chat2/` — saari files copy
3. `config.production.json` edit:
   - `port`: naya free port (e.g. `6042`)
   - `public_host`: naya domain
   - `session_security.domain_check.expected_host`: naya domain
   - `session_security.headers.cors_allow_origin`: `https://naya-domain`
4. `server-deploy.sh` me `APP_DIR` aur `PORT` update
5. systemd service alag naam: `chatgpt2-proxy.service`
6. aaPanel → naya site + reverse proxy → naya port
7. `route_tool.php` me naya entry

---

## 10. Common errors

| Error | Fix |
|-------|-----|
| `replacement directory ../proxy-security does not exist` | `sed` command Step 4 me chalao, ya `bash server-deploy.sh` |
| `bind: address already in use` | Port change karo — doosra tool same port use kar raha hai |
| `Invalid signature` handshake | DB me domain + secret_key match karo; `website_id` route_tool.php me same |
| `website_id = 1` in logs | `ahrefs_websites` me domain register karo, proxy restart |
| Messages refresh ke baad dikhte | `main.go` v6+ upload karo (`wss://` fix) + nginx WebSocket headers |
| Access Denied after hard reload / unauthorized flood | v7+ deploy — stale session cookie fix + softer security |
| Icons missing | v5+ build chahiye (brotli fix) |
| `X-Proxy-Build: chatgpt-v6` (purana) | Naya `main.go` build karo — v7 hona chahiye |

---

## 11. Useful commands

```bash
# Logs
journalctl -u chatgpt-proxy -f

# Restart
systemctl restart chatgpt-proxy

# Status
systemctl status chatgpt-proxy

# Manual run (debug)
cd /www/wwwroot/1clkaccess.store/chat && ./chatgpt-proxy
```
