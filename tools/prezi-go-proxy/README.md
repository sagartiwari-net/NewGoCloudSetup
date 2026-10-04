# Prezi Go Proxy (local)

Reverse proxy for **https://prezi.com/dashboard/next/** using DigitaVision / GoAuto **cookies**.

## Setup

1. Log into [prezi.com/dashboard/next/#/all](https://prezi.com/dashboard/next/#/all)
2. Export cookies (JSON array **or** GoAuto `{ "cookies": [...] }`) — **referer optional**
3. Paste into `cookie.txt`
4. Run (Cursor terminal):

```bash
export PATH="/usr/local/go/bin:$PATH"
chmod +x start-local.sh
./start-local.sh
# or:
go build -o prezi-go-proxy .
PREZI_CONFIG=config.local.json ./prezi-go-proxy
```

5. Open **http://localhost:4751/dashboard/next/#/all**

## cookie.txt

Required: **`prezi-auth`** + **`p26-auth`** / **`auth-sessionid`** (prefer `.prezi.com`). Also: `csrftoken`, `safehousetoken`.

GoAuto wrap (referer may be omitted):

```json
{
  "includedFormats": ["cookies"],
  "cookies": [
    { "name": "prezi-auth", "value": "...", "domain": ".prezi.com" },
    { "name": "p26-auth", "value": "...", "domain": ".prezi.com" },
    { "name": "csrftoken", "value": "...", "domain": ".prezi.com" }
  ]
}
```

Plain JSON array also works. DigitaVision → ToolsMandi replacements applied in HTML/JSON.
