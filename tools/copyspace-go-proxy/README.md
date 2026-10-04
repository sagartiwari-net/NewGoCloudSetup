# CopySpace Go Proxy (local)

Reverse proxy for **https://space.copyspace.ai** using DigitaVision / GoAuto **cookies**.

## Setup

1. Log into [space.copyspace.ai](https://space.copyspace.ai/)
2. Export cookies (JSON array **or** GoAuto `{ "cookies": [...] }`) — **referer optional**
3. Paste into `cookie.txt`
4. Run (Cursor terminal):

```bash
export PATH="/usr/local/go/bin:$PATH"
chmod +x start-local.sh
./start-local.sh
# or:
go build -o copyspace-go-proxy .
COPYSPACE_CONFIG=config.local.json ./copyspace-go-proxy
```

5. Open **http://localhost:4741/**

## cookie.txt

Required: **`copyspaceai_session`** + **`XSRF-TOKEN`** (prefer `.copyspace.ai`). Also useful: `remember_web_*`.

GoAuto wrap (referer may be omitted):

```json
{
  "includedFormats": ["cookies"],
  "cookies": [
    { "name": "copyspaceai_session", "value": "...", "domain": ".copyspace.ai" },
    { "name": "XSRF-TOKEN", "value": "...", "domain": ".copyspace.ai" }
  ]
}
```

Plain JSON array also works. DigitaVision → ToolsMandi replacements applied in HTML/JSON.
