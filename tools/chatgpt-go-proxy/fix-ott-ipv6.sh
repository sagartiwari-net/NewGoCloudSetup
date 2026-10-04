#!/bin/bash
# Fix OTT IPv6 vs IPv4 mismatch — run on proxy server as root
set -e

SEC="/www/wwwroot/toolsmandi.com/proxy-security"
AMZ="/www/wwwroot/amzpremiumsoftware.com/chat"
CLK="/www/wwwroot/1clkaccess.store/chat"

echo "=== 1) Update proxy-security (ott.go + BuildID) ==="
grep -q "dualstack-ott" "$SEC/ott.go" 2>/dev/null && echo "ott.go already patched" || {
  cat > "$SEC/ott.go" << 'EOF'
package proxysecurity

import (
	"net"
	"strings"
)

// ValidateOTTClientIP checks handshake token IP vs /access request IP.
// soft = /16 (Jio/Airtel rotation), soft24 = /24, strict = exact.
func ValidateOTTClientIP(tokenIP, requestIP string, ss Config, tool ToolContext, dbToggle bool) (ok bool, mode string) {
	ss = Normalize(ss, tool)
	mode = ss.OTTIPValidation.Mode
	if !Enabled(ss, tool, dbToggle) || !ss.OTTIPValidation.Enabled {
		return true, mode
	}
	tokenIP = NormalizeClientIP(strings.TrimSpace(tokenIP))
	requestIP = NormalizeClientIP(strings.TrimSpace(requestIP))
	if tokenIP == "" || requestIP == "" {
		return true, mode
	}
	if mode != "strict" && ipv4IPv6DualStack(tokenIP, requestIP) {
		return true, mode
	}
	switch mode {
	case "strict":
		return tokenIP == requestIP, mode
	case "soft24":
		return SameSubnet24(tokenIP, requestIP), mode
	default:
		return SameSubnet16(tokenIP, requestIP), mode
	}
}

func ipv4IPv6DualStack(a, b string) bool {
	pa, pb := net.ParseIP(a), net.ParseIP(b)
	if pa == nil || pb == nil {
		return false
	}
	a4, b4 := pa.To4(), pb.To4()
	aIs4 := a4 != nil && len(pa) == net.IPv4len
	bIs4 := b4 != nil && len(pb) == net.IPv4len
	aIs6 := !aIs4 && len(pa) == net.IPv6len
	bIs6 := !bIs4 && len(pb) == net.IPv6len
	return (aIs4 && bIs6) || (aIs6 && bIs4)
}
EOF
  sed -i 's|BuildID = ".*"|BuildID = "20260614-dualstack-ott-v1"|' "$SEC/config.go"
  echo "OK: ott.go updated"
}

echo "=== 2) Disable ott_ip_validation in Amz config (belt + suspenders) ==="
python3 << 'PY'
import json
p = "/www/wwwroot/amzpremiumsoftware.com/chat/config.json"
c = json.load(open(p))
c.setdefault("session_security", {}).setdefault("ott_ip_validation", {})["enabled"] = False
json.dump(c, open(p, "w"), indent=2)
print("OK:", p, "ott_ip_validation=false")
PY

echo "=== 3) Rebuild proxies ==="
for dir in "$AMZ" "$CLK"; do
  if [ -d "$dir" ]; then
    echo "Building $dir ..."
    cd "$dir"
    sed -i "s|replace toolsmandi.com/proxy-security => .*|replace toolsmandi.com/proxy-security => ${SEC}|" go.mod
    go build -o chatgpt-proxy .
  fi
done

systemctl restart chatgpt-amz-proxy 2>/dev/null || true
systemctl restart chatgpt-proxy 2>/dev/null || true
sleep 2

echo ""
echo "=== 4) Verify ==="
journalctl -u chatgpt-amz-proxy -n 5 --no-pager | grep -E 'build=|BuildID|dualstack' || journalctl -u chatgpt-amz-proxy -n 3 --no-pager
echo ""
echo "Expected build lib: 20260614-dualstack-ott-v1"
echo ""
echo "=== 5) IMPORTANT: Fix route_tool.php on amzpremiumsoftware.com ==="
echo "Replace getRealClientIP() to prefer IPv4 (see amb/route_tool.php in repo)"
echo "Then test Demouser login again."
