# Screpy Go Proxy (local)

Reverse proxy for **https://app.screpy.com** using DigitaVision / GoAuto **cookies**.

## Setup

1. Log into [app.screpy.com/dashboard](https://app.screpy.com/dashboard)
2. Export cookies (JSON array **or** GoAuto `{ "cookies": [...] }`) — **referer optional**
3. Paste into `cookie.txt`
4. Run (Cursor terminal):

```bash
export PATH="/usr/local/go/bin:$PATH"
chmod +x start-local.sh
./start-local.sh
# or:
go build -o screpy-go-proxy .
SCREPY_CONFIG=config.local.json ./screpy-go-proxy
```

5. Open **http://localhost:4721/dashboard**

## cookie.txt

Required: **`screpy-session`** + **`XSRF-TOKEN`** (prefer `.screpy.com`). Also useful: `cf_clearance`.

GoAuto wrap (referer may be omitted):

```json
{
  "includedFormats": ["cookies"],
  "cookies": [
    { "name": "screpy-session", "value": "...", "domain": ".screpy.com" },
    { "name": "XSRF-TOKEN", "value": "...", "domain": ".screpy.com" }
  ]
}
```

Plain JSON array also works. DigitaVision → ToolsMandi replacements applied in HTML/JSON.
