# SEO Tester Online Go Proxy

Reverse proxy for **https://suite.seotesteronline.com** using **localStorage session** (GoAuto export).

## Local setup

1. Paste DigitaVision **GoAuto localStorage** into `cookie.txt` (`includedFormats: ["localStorage"]`)
2. Required keys: **`accessToken`**, **`session`**, **`userInfo`**, **`ngStorage-currentUser`**
3. Referer is **optional** (dump with or without `referer` works)
4. `./start-local.sh` → open http://localhost:4801/

```json
{
  "referer": "https://suite.seotesteronline.com/",
  "includedFormats": ["localStorage"],
  "storage": {
    "localStorage": {
      "accessToken": "eyJ...",
      "session": "...",
      "userInfo": "{...}",
      "ngStorage-currentUser": "{...}"
    }
  }
}
```

Auth: Angular interceptor sets `Authorization: Bearer` from `localStorage.accessToken` for `api.seotesteronline.com`. Proxy hydrates localStorage on every HTML page (and re-applies briefly so ngStorage cannot wipe it) and injects Bearer on API hosts only. DigitaVision → ToolsMandi uses word-boundary replace only — never corrupts emails in `accessToken` / `session` / `userInfo` / `ngStorage-currentUser`. Localhost does **not** spoof `location` to suite (Angular html5Mode would stick on the splash).
