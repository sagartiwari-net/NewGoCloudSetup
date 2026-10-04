# Shortform Go Proxy (local)

Reverse proxy for [Shortform](https://www.shortform.com/app/discover).

## Quick start (Cursor terminal)

1. Paste DigitaVision **GoAuto localStorage** into `cookie.txt` (`includedFormats: ["localStorage"]`)
2. Required keys: **`auth_token`**, **`user`**
3. `chmod +x start-local.sh && ./start-local.sh`
4. Open http://localhost:4771/app/discover

**Referer optional.** Cookies-only export will not work — need localStorage.

## Auth

App uses HTTP Basic auth: username = `auth_token`, password empty (axios `_addAuth`). Proxy hydrates `localStorage` on every HTML page.

## Port

Local: **4771**
