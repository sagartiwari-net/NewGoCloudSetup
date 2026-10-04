# Airbrush deploy checklist (with proxy-security)

## Server paths

| Path | Purpose |
|------|---------|
| `/www/wwwroot/toolsmandi.com/proxy-security` | Shared security library |
| `/www/wwwroot/1clkaccess.store/airbrush` | Airbrush proxy |

## Files to upload to airbrush folder

Upload **all** of these together (partial upload causes build errors):

- `main.go`
- `security_bridge.go`
- `go.mod`
- `go.sum`
- `config.json`

## Files to DELETE on server (if present)

- `security_score.go` ← old, causes duplicate `computeRiskScore`
- `security_log.go` ← old

## Build commands

```bash
cd /www/wwwroot/1clkaccess.store/airbrush

# go.mod uses absolute path to library:
# replace toolsmandi.com/proxy-security => /www/wwwroot/toolsmandi.com/proxy-security

ls /www/wwwroot/toolsmandi.com/proxy-security/go.mod
ls security_bridge.go
ls security_score.go 2>/dev/null && rm -f security_score.go security_log.go

go mod tidy
go build -o airbrush-proxy .
systemctl restart airbrush-proxy

journalctl -u airbrush-proxy -n 5 --no-pager | grep SECURITY
```

## Success log

```
lib=20260612-central-v1
```

## Old build (wrong — do not keep running)

```
build=20260612-ott-soft16-v2
```
