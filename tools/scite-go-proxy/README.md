# Scite Go Proxy (local)

Reverse proxy for [Scite Assistant](https://scite.ai/assistant).

## Quick start (Cursor terminal)

1. Paste DigitaVision cookies into `cookie.txt` (JSON array **or** GoAuto `{ cookies: [...] }`)
2. `chmod +x start-local.sh && ./start-local.sh`
3. Open http://localhost:4761/assistant

**Referer optional** — cookie file me `referer` field zaroori nahi.

## cookie.txt

Required: **`connect.sid`** (HttpOnly session). Also useful: `aws-waf-token`, `userSession`, `anonId`, `visid_incap_*`.

Prefer hostOnly `scite.ai`.

```json
{
  "includedFormats": ["cookies"],
  "cookies": [
    { "name": "connect.sid", "value": "s%3A...", "domain": "scite.ai", "hostOnly": true }
  ]
}
```

## Port

Local: **4761** (`config.local.json`).
