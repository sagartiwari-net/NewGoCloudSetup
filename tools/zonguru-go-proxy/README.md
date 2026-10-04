# ZonGuru Go Proxy (local)

Reverse proxy for **https://my.zonguru.com** using **localStorage session** (GoAuto export).

## Setup

1. Log into [my.zonguru.com](https://my.zonguru.com/#!/dashboard)
2. Export session with GoAuto → `includedFormats: ["localStorage"]`
3. Save JSON as `cookie.txt`
4. Run in **Cursor integrated terminal**:

```bash
chmod +x start-local.sh
./start-local.sh
```

5. Open **http://localhost:4691/#!/dashboard**

## cookie.txt format

```json
{
  "referer": "https://my.zonguru.com/#!/dashboard",
  "includedFormats": ["localStorage"],
  "storage": {
    "localStorage": {
      "token": "{\"_data\":\"...\"}",
      "me": "{\"_data\":{...}}"
    }
  }
}
```

Required keys: **`token`**, **`me`** (auth uses `FbaToken` header from `token`).

Referer optional. DigitaVision → ToolsMandi applied in HTML + localStorage inject.
