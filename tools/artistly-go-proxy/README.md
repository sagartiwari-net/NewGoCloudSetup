# Artistly Go Proxy (local)

Reverse proxy for **https://app.artistly.ai** using DigitaVision / GoAuto **cookies**.

## Quick start

1. Log into [app.artistly.ai](https://app.artistly.ai/ai/ai-image-designer)
2. Export cookies (EditThisCookie / DigitaVision) → save as `cookie.txt`
3. Run:

```bash
chmod +x start-local.sh
./start-local.sh
```

Or:

```bash
go build -o artistly-go-proxy .
ARTISTLY_CONFIG=config.local.json ./artistly-go-proxy
```

Open: **http://localhost:4821/ai/ai-image-designer**

## Auth cookies

Required: **`artistly_session`** (HttpOnly) + **`XSRF-TOKEN`**. Optional: `CSRF`, `remember_web_*`. Prefer hostOnly `app.artistly.ai`, then `.artistly.ai`.

Array or GoAuto wrap both OK — **referer optional**. Proxy injects `X-XSRF-TOKEN` = URL-decoded `XSRF-TOKEN`.
