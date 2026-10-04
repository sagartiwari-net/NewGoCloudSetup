# Grok Go Proxy (local)

Reverse proxy for **https://grok.com** using DigitaVision / GoAuto **cookies**.

## Quick start

1. Log into [grok.com](https://grok.com/)
2. Export cookies (EditThisCookie / DigitaVision) → save as `cookie.txt`
3. Run:

```bash
chmod +x start-local.sh
./start-local.sh
```

Or:

```bash
go build -o grok-go-proxy .
GROK_CONFIG=config.local.json ./grok-go-proxy
```

Open: **http://localhost:4831/**

## Auth cookies

Critical: **`sso`**, **`sso-rw`** (JWT), **`x-userid`**. Also: `x-anonuserid`, `x-challenge`, `x-signature`, `grok_device_id`, `cf_clearance`, `__cf_bm`.

Prefer domain **`.grok.com`**, then hostOnly **`grok.com`**.

Array or GoAuto wrap both OK — **referer optional**.
