# Helium10 Go Proxy — hm.1clkaccess.store

- Domain: `https://hm.1clkaccess.store`
- Path: `/www/wwwroot/1clkaccess.store/hm`
- Port: `5151`
- Website ID: `130`
- Handshake secret: `toolsmandi_helium_secret_xyz123`
- Extension folder: `extension/` (points at this domain)

## Server deploy

```bash
cd /www/wwwroot/1clkaccess.store/hm
# first time only if folder dirty:
# find . -mindepth 1 -delete && git clone https://github.com/sagartiwari-net/demoone.git .
git fetch origin && git reset --hard origin/main
export PATH="$PATH:/usr/local/go/bin:/root/go/bin"
chmod +x reset-server.sh
./reset-server.sh
```

aaPanel: reverse proxy `hm.1clkaccess.store` → `http://127.0.0.1:5151`

Add Helium cookies in ctrl panel for website_id **130**.

## Extension

Load unpacked from `extension/` or use `helium10-toolsmandi-hm.zip`.
