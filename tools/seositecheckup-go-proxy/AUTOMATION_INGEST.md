# Automation Cookie Ingest API

GoAuto (browser automation) login ke baad cookies/localStorage/IndexedDB yahan POST karega — **sahi tool ke sahi account** me save ho jayega.

## Flow

```
Logout detect → Proxy GoAuto trigger karta hai
              → payload me callback_url + account_id + website_id
              → GoAuto browser login karta hai
              → POST /api/automation-ingest (session_data)
              → DB me ahrefs_accounts.cookie update
              → Proxy turant naya session use karta hai
```

## Endpoints (2 options)

### Option A — Tool proxy (recommended per tool)

```
POST https://seosite.1clkaccess.store/api/automation-ingest
```

Automation trigger me `callback_url` auto bheja jata hai jab `public_host` set ho.

### Option B — Central ctrl panel (sab tools ke liye ek URL)

```
POST https://YOUR_CTRL_PANEL_HOST:7843/api/automation/ingest
```

`website_id` body me bhejna zaroori — kisi bhi tool ka account update ho sakta hai.

---

## Auth (dono me se ek)

### 1) Bearer token (GoAuto ke liye sabse aasaan)

```
Authorization: Bearer goauto_secret_token_12345
```

Same token jo `config.json` → `automation.headers.Authorization` me hai.

### 2) HMAC signature (zyada secure)

```
signature = HMAC-SHA256( secret_key, "website_id:account_id:timestamp" )
```

Example: `49:27:1718234567` → hex signature

`timestamp` 10 minute se purana na ho.

---

## Request body

```json
{
  "website_id": 49,
  "account_id": 27,
  "task_uid": "gfx_runSeositecheckup",
  "timestamp": 1718234567,
  "signature": "optional_if_using_bearer",
  "user_agent": "Mozilla/5.0 ...",
  "session_data": {
    "cookies": [ ... browser cookie JSON array ... ],
    "local_storage": {
      "ssc.iam": "{\"access_token\":\"...\",\"refresh_token\":\"...\"}"
    },
    "indexed_db": { }
  }
}
```

### `session_data` formats (sab accept hote hain)

| Format | Example |
|--------|---------|
| Full bundle | `{ cookies, local_storage, indexed_db }` |
| Cookies only | `[ {"name":"ssc.iam","value":"..."}, ... ]` |
| IAM only | `{"access_token":"...","refresh_token":"..."}` |

DB me **v2 bundle** save hota hai (max ~15 MB per request).

---

## GoAuto task — last step example (Python)

```python
import json, time, requests

# Trigger payload se mile (GoAuto context)
callback_url = context.get("callback_url") or "https://seosite.1clkaccess.store/api/automation-ingest"
website_id = context.get("website_id", 49)
account_id = context.get("account_id", 27)
task_uid = context.get("task_uid", "gfx_runSeositecheckup")

# Browser se collect karo (Playwright/Puppeteer)
cookies = await context.cookies()  # list of dicts
local_storage = await page.evaluate("() => Object.assign({}, window.localStorage)")
# indexed_db optional — bada ho sakta hai

payload = {
    "website_id": website_id,
    "account_id": account_id,
    "task_uid": task_uid,
    "timestamp": int(time.time()),
    "user_agent": await browser.user_agent(),
    "session_data": {
        "cookies": cookies,
        "local_storage": local_storage,
    }
}

resp = requests.post(
    callback_url,
    json=payload,
    headers={
        "Content-Type": "application/json",
        "Authorization": "Bearer goauto_secret_token_12345",
    },
    timeout=120,
)
print(resp.status_code, resp.text)
```

---

## Deploy (seosite server)

```bash
cd /www/wwwroot/1clkaccess.store/seosite
# Upload updated main.go
go build -o seositecheckup-proxy .
systemctl restart seositecheckup-proxy
```

Success log:

```
[INGEST] ✅ Saved session | account=Account 1 (ID:27) task_uid=gfx_runSeositecheckup bytes=...
```

---

## Test (curl)

```bash
curl -X POST https://seosite.1clkaccess.store/api/automation-ingest \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer goauto_secret_token_12345" \
  -d '{
    "website_id": 49,
    "account_id": 27,
    "task_uid": "test",
    "timestamp": '"$(date +%s)"',
    "session_data": {
      "cookies": [{"name":"ssc.iam","value":"{\"access_token\":\"x\",\"refresh_token\":\"y\"}"}]
    }
  }'
```

Expected: `{"status":"ok","account_id":27,...}`
