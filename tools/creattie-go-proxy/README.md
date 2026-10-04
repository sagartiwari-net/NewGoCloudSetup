# Creattie Go Proxy (local)

Reverse proxy for **https://creattie.com** using DigitaVision / GoAuto **cookies**.

## Quick start

1. Log into [creattie.com](https://creattie.com/)
2. Export cookies (EditThisCookie / DigitaVision) → save as `cookie.txt`
3. Run:

```bash
chmod +x start-local.sh
./start-local.sh
```

Open: **http://localhost:4861/**

## Auth cookies

Required: **`creattie_session`** + **`XSRF-TOKEN`**. Also **`remember_web_*`**. Prefer hostOnly `creattie.com`.

Array or GoAuto wrap both OK — **referer optional**. Proxy injects `X-XSRF-TOKEN` = URL-decoded `XSRF-TOKEN`.
