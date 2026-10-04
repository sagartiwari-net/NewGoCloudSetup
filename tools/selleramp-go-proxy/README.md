# SellerAmp local recloud

Local proxy for `https://sas.selleramp.com` plus a host-bound Chrome extension.

## Start proxy

```bash
cd selleramp-go-proxy
./start-local.sh
```

Open: `http://127.0.0.1:7852/`

## Load extension

1. Chrome → `chrome://extensions`
2. Developer mode ON
3. Load unpacked → `selleramp-go-proxy/extension`

The extension iframe/API host is set to `http://127.0.0.1:7852`.

Put fresh SellerAmp cookies in `cookie.txt` (not committed).
