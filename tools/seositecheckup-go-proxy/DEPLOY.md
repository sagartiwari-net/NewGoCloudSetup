# SEO Site Checkup — Production deploy (seosite.1clkaccess.store)

## Server paths

| Path | Purpose |
|------|---------|
| `/www/wwwroot/toolsmandi.com/proxy-security` | Shared security library |
| `/www/wwwroot/1clkaccess.store/seosite` | SEO Site Checkup proxy (port **6031**) |

## Pre-requisites (ctrl panel)

- Website **ID 49** — domain `seosite.1clkaccess.store`, Security **ON**
- Handshake key: `toolsmandi_seosite_secret_xyz123`
- **Mapped Accounts** — paste full browser cookie export JSON in `cookie` field (must include `ssc.iam` with access + refresh tokens)
- `route_tool.php` — `seosite` entry with `website_id` => **49**

## Config changes vs local dev

| Setting | Local | Production |
|---------|-------|------------|
| `port` | 6023 | **6031** |
| `use_database` | false | **true** |
| `bypass_auth` | true | **false** |
| `public_scheme` | http | **https** |
| `secret_key` | — | **toolsmandi_seosite_secret_xyz123** |
| `session_duration_minutes` | 120 | **30** (match ctrl panel) |
| `session_security.enabled` | false | **true** |
| `domain_check.expected_host` | localhost | **seosite.1clkaccess.store** |
| `logout_detection.enabled` | false | **true** |
| `automation.enabled` | false | **true** |

## One-shot server commands

**Option A — full script (recommended):**

```bash
cd /www/wwwroot/1clkaccess.store/seosite

# Set MySQL password in production config (once):
sed -i 's/"mysql_password": "CHANGE_ME"/"mysql_password": "YOUR_DB_PASSWORD"/' config.production.json

bash server-deploy.sh
```

**Option B — manual steps:**

Run as **root** on the server:

```bash
cd /www/wwwroot/1clkaccess.store/seosite

cp config.production.json config.json
nano config.json   # set mysql_password if needed

sed -i 's|replace toolsmandi.com/proxy-security => .*|replace toolsmandi.com/proxy-security => /www/wwwroot/toolsmandi.com/proxy-security|' go.mod

rm -f security_score.go security_log.go

go mod tidy
go build -o seositecheckup-proxy .
chmod +x seositecheckup-proxy

cp seositecheckup-proxy.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable seositecheckup-proxy
systemctl restart seositecheckup-proxy

journalctl -u seositecheckup-proxy -n 30 --no-pager
```

Success logs should include:

```
[DB] Connected to MySQL successfully!
[DB] Resolved website_id = 49 for domain 'seosite.1clkaccess.store'
[PROXY] build_tag=seositecheckup-v5 public_host=seosite.1clkaccess.store use_database=true
[SECURITY] ... enabled=true ...
```

## Nginx (aaPanel / site seosite.1clkaccess.store)

Reverse proxy to **6031**:

```nginx
location / {
    proxy_pass http://127.0.0.1:6031;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 300s;
    proxy_buffering off;
}
```

## Account cookie format (Mapped Accounts)

Paste the **full JSON cookie export** from browser (same as `cookie.txt` locally). The `ssc.iam` entry must contain:

```json
{"access_token":"...","refresh_token":"..."}
```

The proxy reads IAM tokens from DB per session and auto-refreshes access tokens every ~10 minutes.

## Test

1. Member area → SEO Site Checkup → opens `https://seosite.1clkaccess.store`
2. DevTools → any response → `X-Proxy-Build: seositecheckup-v5`
3. Domain check blocks iframe/recloud on wrong host
