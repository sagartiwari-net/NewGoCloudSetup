package main

import (
	"bytes"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"unicode"
)

// pictoryUseLog prints one line when a Pictory call can spend credits.
// Text, tokens, emails, and other secrets are not included.
func logPictoryUse(method, host, path string, status int, user string, reqBody, respBody []byte) {
	host = strings.ToLower(strings.TrimSpace(host))
	path = pictoryLogPath(path)
	if !shouldLogPictoryUse(method, host, path, respBody) {
		return
	}
	req := pictoryBodySummary(reqBody)
	resp := pictoryBodySummary(respBody)
	log.Printf("[P-USE] %s %s%s status=%d user=%s req%s resp%s", method, host, path, status, user, req, resp)
}

func pictoryLogPath(path string) string {
	if path == "" {
		return "/"
	}
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func shouldLogPictoryUse(method, host, path string, respBody []byte) bool {
	if pictoryStaticPath(path) || pictoryNoiseHost(host) {
		return false
	}
	switch method {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	}
	if pictoryCreditPath(path) {
		return true
	}
	return pictorySummaryHasNumbers(pictoryBodySummary(respBody))
}

func pictoryStaticPath(path string) bool {
	p := strings.ToLower(path)
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	for _, ext := range []string{
		".js", ".css", ".map", ".woff", ".woff2", ".ttf", ".eot",
		".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp",
		".mp4", ".webm", ".m3u8", ".mp3", ".wav",
	} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

func pictoryNoiseHost(host string) bool {
	h := strings.ToLower(host)
	for _, part := range []string{
		"fonts.googleapis.com", "fonts.gstatic.com", "googletagmanager.com",
		"google-analytics.com", "amplitude.com", "bing.com", "bing.net",
		"firstpromoter.com", "intercom.io", "intercomcdn.com", "customer.io",
		"gist.build", "launchdarkly.com", "sentry.io", "highlight.io",
		"appcues.net", "stripe.com", "cloudflare.com", "amazoncognito.com",
		"cognito-idp.", "cognito-identity.", "front-end-prod.pictory.ai",
		"pictory-static.pictorycontent.com", "prod-proxy-feature-flags.pictorycontent.com",
	} {
		if strings.Contains(h, part) {
			return true
		}
	}
	return false
}

func pictoryCreditPath(path string) bool {
	p := strings.ToLower(path)
	for _, part := range []string{"credit", "quota", "usage", "subscription", "billing", "plan"} {
		if strings.Contains(p, part) {
			return true
		}
	}
	return false
}

func pictorySummaryHasNumbers(summary string) bool {
	return strings.Contains(summary, "=")
}

func pictoryBodySummary(body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return "{}"
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return "{bytes=" + strconv.Itoa(len(body)) + "}"
	}
	var nums []string
	var keys []string
	var words, chars int
	pictoryWalkUsage(v, "", 0, &nums, &keys, &words, &chars)
	var b strings.Builder
	b.WriteString("{bytes=")
	b.WriteString(strconv.Itoa(len(body)))
	if words > 0 {
		b.WriteString(" words=")
		b.WriteString(strconv.Itoa(words))
		b.WriteString(" chars=")
		b.WriteString(strconv.Itoa(chars))
	}
	if len(keys) > 0 {
		b.WriteString(" keys=")
		b.WriteString(strings.Join(keys, ","))
	}
	for _, n := range nums {
		b.WriteByte(' ')
		b.WriteString(n)
	}
	b.WriteByte('}')
	return b.String()
}

func pictoryWalkUsage(v any, key string, depth int, nums, keys *[]string, words, chars *int) {
	if depth > 6 || len(*nums) >= 24 {
		return
	}
	switch t := v.(type) {
	case map[string]any:
		if depth == 0 {
			for k := range t {
				if len(*keys) >= 12 || pictorySecretKey(k) {
					continue
				}
				*keys = append(*keys, k)
			}
		}
		for k, child := range t {
			next := k
			if key != "" {
				next = key + "." + k
			}
			pictoryWalkUsage(child, next, depth+1, nums, keys, words, chars)
		}
	case []any:
		if len(t) > 8 {
			t = t[:8]
		}
		for _, child := range t {
			pictoryWalkUsage(child, key, depth+1, nums, keys, words, chars)
		}
	case string:
		if pictoryTextKey(key) {
			*chars += len(t)
			*words += pictoryWordCount(t)
		}
	case json.Number:
		if pictoryCreditKey(key) && !pictorySecretKey(key) && len(*nums) < 24 {
			*nums = append(*nums, key+"="+t.String())
		}
	case float64:
		if pictoryCreditKey(key) && !pictorySecretKey(key) && len(*nums) < 24 {
			*nums = append(*nums, key+"="+strconv.FormatFloat(t, 'f', -1, 64))
		}
	case bool:
		return
	}
}

func pictorySecretKey(key string) bool {
	k := strings.ToLower(key)
	for _, part := range []string{
		"token", "cookie", "secret", "password", "authorization", "email",
		"refresh", "jwt", "card", "phone", "address", "bearer", "session",
	} {
		if strings.Contains(k, part) {
			return true
		}
	}
	return false
}

func pictoryCreditKey(key string) bool {
	k := strings.ToLower(key)
	if k == "" {
		return false
	}
	for _, part := range []string{
		"credit", "quota", "usage", "remaining", "consumed", "balance",
		"duration", "seconds", "minutes", "videos", "images", "projects",
		"transcription", "voice", "speech", "cost", "billed", "deduct",
	} {
		if strings.Contains(k, part) {
			return true
		}
	}
	return false
}

func pictoryTextKey(key string) bool {
	k := strings.ToLower(key)
	base := k
	if i := strings.LastIndex(k, "."); i >= 0 {
		base = k[i+1:]
	}
	switch base {
	case "text", "script", "prompt", "content", "input", "message", "article", "transcript", "output", "result":
		return !pictorySecretKey(k)
	default:
		return false
	}
}

func pictoryWordCount(s string) int {
	n := 0
	in := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if !in {
				n++
				in = true
			}
			continue
		}
		in = false
	}
	return n
}
