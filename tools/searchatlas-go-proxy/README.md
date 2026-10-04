# SearchAtlas Go Proxy (local)

Reverse proxy for **https://dashboard.searchatlas.com** using **localStorage session** (GoAuto export).

## Setup

1. Log into [dashboard.searchatlas.com](https://dashboard.searchatlas.com/home)
2. Export session with GoAuto → `includedFormats: ["localStorage"]`
3. Paste into `cookie.txt` (valid JSON)
4. Run:

```bash
export PATH="/usr/local/go/bin:$PATH"
./start-local.sh
# or:
go build -o searchatlas-go-proxy .
CONFIG_FILE=config.local.json ./searchatlas-go-proxy
```

5. Open **http://localhost:4711/home**

## cookie.txt format

```json
{
  "referer": "https://dashboard.searchatlas.com/home",
  "includedFormats": ["localStorage"],
  "storage": {
    "localStorage": {
      "token": "eyJ..."
    }
  }
}
```

Required key: **`token`** (JWT). DigitaVision → ToolsMandi applied in HTML + localStorage inject.
