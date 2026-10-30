package main

import (
	"strconv"
	"strings"

	utls "github.com/refraction-networking/utls"
)

func chromeHelloID() utls.ClientHelloID {
	return utls.HelloChrome_Auto
}

func chromeAutoMajor() int {
	ver := strings.TrimSpace(chromeHelloID().Version)
	n, _ := strconv.Atoi(strings.Split(ver, ".")[0])
	if n <= 0 {
		return 133
	}
	return n
}

func chromeAutoUserAgent() string {
	major := strconv.Itoa(chromeAutoMajor())
	return "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + major + ".0.0.0 Safari/537.36"
}

func chromeMajorFromUA(ua string) int {
	i := strings.Index(ua, "Chrome/")
	if i < 0 {
		return 0
	}
	rest := ua[i+len("Chrome/"):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	n, _ := strconv.Atoi(rest[:end])
	return n
}

// canvaUpstreamUA keeps TLS and User-Agent on the same Chrome generation.
// Stale panel/config UAs (Chrome 120 while Auto is 133+) get replaced automatically.
func canvaUpstreamUA(accountUA, cfgUA string) string {
	auto := chromeAutoUserAgent()
	floor := chromeAutoMajor() - 2
	if floor < 1 {
		floor = 1
	}
	for _, raw := range []string{accountUA, cfgUA} {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if chromeMajorFromUA(raw) >= floor {
			return raw
		}
	}
	return auto
}
