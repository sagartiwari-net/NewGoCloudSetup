# ClosersCopy proxy — local + production

Target: `https://www.closerscopy.com` (Laravel — session cookie: `closerscopy_session_ccv2`)

## Local test (Mac)

1. Login at www.closerscopy.com → export cookies JSON → `cookie.txt`
2. ```bash
   cd closerscopy-go-proxy
   chmod +x restart-local.sh
   ./restart-local.sh
   ```
3. Open `http://localhost:6022`
4. Check `X-Proxy-Build: closerscopy-v1`

## Production

| Item | Value |
|------|--------|
| Domain | `closerscopy.1clkaccess.store` |
| Port | **6012** |
| Path | `/www/wwwroot/1clkaccess.store/closerscopy` |
| Handshake key | `toolsmandi_closerscopy_secret_xyz123` |
| Automation UID | `gfx_runCloserscopy` |

```bash
cd /www/wwwroot/1clkaccess.store/closerscopy
cp config.production.json config.json
nano config.json   # mysql_password
bash server-deploy.sh
```

Nginx reverse proxy → `http://127.0.0.1:6012`
