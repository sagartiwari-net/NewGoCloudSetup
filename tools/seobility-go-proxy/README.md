# Seobility Go Proxy (local)

Reverse proxy for **https://app.seobility.net** using DigitaVision / GoAuto **cookies**.

## Setup

1. Log into [app.seobility.net/dashboard](https://app.seobility.net/dashboard)
2. Export cookies (JSON array **or** GoAuto `{ "cookies": [...] }`) — **referer optional**
3. Paste into `cookie.txt`
4. Run (Cursor terminal):

```bash
export PATH="/usr/local/go/bin:$PATH"
chmod +x start-local.sh
./start-local.sh
# or:
go build -o seobility-go-proxy .
SEOBILITY_CONFIG=config.local.json ./seobility-go-proxy
```

5. Open **http://localhost:4731/dashboard**

## cookie.txt

Required: **`seobility_app_session`** + **`XSRF-TOKEN`** (prefer `.seobility.net`). Also useful: `seobility_everauth`, `__cflb`, `cf_clearance`.

GoAuto wrap (referer may be omitted):

```json
{
  "includedFormats": ["cookies"],
  "cookies": [
    { "name": "seobility_app_session", "value": "...", "domain": ".seobility.net" },
    { "name": "XSRF-TOKEN", "value": "...", "domain": ".seobility.net" }
  ]
}
```

Plain JSON array also works. DigitaVision → ToolsMandi replacements applied in HTML/JSON.
