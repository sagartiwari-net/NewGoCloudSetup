package main

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Regional Azure blob hosts use random subdomains (*.oaiusercontent.com).
// Bare hostname replacement corrupts them — route via /extra-cdn-wild/HOST/ instead.

var extraCDNWildcardSuffixes = []string{
	"oaiusercontent.com",
	"oaistatic.com",
}

var apiGatewayHostRe = regexp.MustCompile(`(?i)(https?|wss?)://([a-z0-9.-]+\.api\.openai\.com)`)

var extraCDNWildcardRes []*regexp.Regexp

func init() {
	for _, suffix := range extraCDNWildcardSuffixes {
		quoted := regexp.QuoteMeta(suffix)
		extraCDNWildcardRes = append(extraCDNWildcardRes,
			regexp.MustCompile(`(?i)(https?://)([a-z0-9-]+)\.`+quoted))
	}
}

func extraCDNHostFromURL(raw string) string {
	raw = strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	return strings.Split(raw, "/")[0]
}

func isExplicitExtraCDNHost(host string, cfg Config) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	for _, extra := range cfg.ExtraCDNDomains {
		if host == strings.ToLower(extraCDNHostFromURL(extra)) {
			return true
		}
	}
	return false
}

func isExtraCDNWildcardHost(host string, cfg Config) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" || isExplicitExtraCDNHost(host, cfg) {
		return false
	}
	if strings.HasSuffix(host, ".api.openai.com") {
		return true
	}
	for _, suffix := range extraCDNWildcardSuffixes {
		if strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

func rewriteAPIGatewayURL(u, publicBase string) string {
	if u == "" || publicBase == "" || !strings.Contains(strings.ToLower(u), ".api.openai.com") {
		return u
	}
	wsBase := strings.Replace(publicBase, "https://", "wss://", 1)
	wsBase = strings.Replace(wsBase, "http://", "ws://", 1)
	return apiGatewayHostRe.ReplaceAllStringFunc(u, func(match string) string {
		sub := apiGatewayHostRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		base := publicBase
		if strings.EqualFold(sub[1], "wss") || strings.EqualFold(sub[1], "ws") {
			base = wsBase
		}
		return base + "/extra-cdn-wild/" + strings.ToLower(sub[2])
	})
}

func rewriteExtraCDNWildcardURL(u, publicBase string) string {
	if u == "" || publicBase == "" {
		return u
	}
	for i, suffix := range extraCDNWildcardSuffixes {
		if !strings.Contains(strings.ToLower(u), "."+suffix) {
			continue
		}
		re := extraCDNWildcardRes[i]
		u = re.ReplaceAllStringFunc(u, func(match string) string {
			submatch := re.FindStringSubmatch(match)
			if len(submatch) < 3 {
				return match
			}
			sub := strings.ToLower(submatch[2])
			return publicBase + "/extra-cdn-wild/" + sub + "." + strings.ToLower(suffix)
		})
	}
	return u
}

func rewriteExtraCDNWildcards(body []byte, publicScheme, publicHost string) []byte {
	if !bytes.Contains(body, []byte("oaiusercontent.com")) && !bytes.Contains(body, []byte("oaistatic.com")) && !bytes.Contains(body, []byte("api.openai.com")) {
		return body
	}
	publicBase := publicScheme + "://" + publicHost
	out := rewriteExtraCDNWildcardURL(string(body), publicBase)
	return []byte(rewriteAPIGatewayURL(out, publicBase))
}

func isProxyStaticPath(path string) bool {
	if strings.HasPrefix(path, "/extra-cdn-wild/") {
		rest := strings.TrimPrefix(path, "/extra-cdn-wild/")
		if i := strings.Index(rest, "/backend-api/"); i >= 0 {
			return false
		}
	}
	if strings.HasPrefix(path, "/cdn/") ||
		strings.HasPrefix(path, "/cdn-proxy/") ||
		strings.HasPrefix(path, "/extra-cdn-") {
		return true
	}
	if strings.HasPrefix(path, "/assets/") {
		return true
	}
	// Hashed CDN chunks: 2340486e-mthtkvzhzdssjwmn.js
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return immutableAssetNameRe.MatchString(path[i+1:])
	}
	return immutableAssetNameRe.MatchString(strings.TrimPrefix(path, "/"))
}

var immutableAssetNameRe = regexp.MustCompile(`^[a-f0-9]{6,}-[a-z0-9._-]+\.(js|css|mjs|svg|woff2?|ico|png|webp)$`)

func needsBodyRewrite(body []byte, cfg Config) bool {
	if bytes.Contains(body, []byte(".oaiusercontent.com")) || bytes.Contains(body, []byte(".oaistatic.com")) || bytes.Contains(body, []byte(".api.openai.com")) {
		return true
	}
	if targetParsed, err := url.Parse(cfg.TargetURL); err == nil && targetParsed.Host != "" {
		if bytes.Contains(body, []byte(targetParsed.Host)) {
			return true
		}
	}
	if cdnParsed, err := url.Parse(cfg.CDNURL); err == nil && cdnParsed.Host != "" {
		if bytes.Contains(body, []byte(cdnParsed.Host)) {
			return true
		}
	}
	for _, extra := range cfg.ExtraCDNDomains {
		if h := extraCDNHostFromURL(extra); h != "" && bytes.Contains(body, []byte(h)) {
			return true
		}
	}
	return false
}

func passthroughStaticResponse(w http.ResponseWriter, upstreamResp *http.Response) {
	if enc := upstreamResp.Header.Get("Content-Encoding"); enc != "" {
		w.Header().Set("Content-Encoding", enc)
	}
	if cl := upstreamResp.Header.Get("Content-Length"); cl != "" {
		w.Header().Set("Content-Length", cl)
	}
	if cc := upstreamResp.Header.Get("Cache-Control"); cc != "" {
		w.Header().Set("Cache-Control", cc)
	} else {
		w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	}
	w.WriteHeader(upstreamResp.StatusCode)
	_, _ = io.Copy(w, upstreamResp.Body)
}

func parseExtraCDNWildcardPath(path string, cfg Config) (host, subPath string, ok bool) {
	if !strings.HasPrefix(path, "/extra-cdn-wild/") {
		return "", "", false
	}
	rest := strings.TrimPrefix(path, "/extra-cdn-wild/")
	if rest == "" {
		return "", "", false
	}
	host = rest
	subPath = "/"
	if i := strings.Index(rest, "/"); i >= 0 {
		host = rest[:i]
		subPath = rest[i:]
	}
	if !isExtraCDNWildcardHost(host, cfg) {
		return "", "", false
	}
	return host, subPath, true
}

func applyExtraCDNCORS(w http.ResponseWriter, r *http.Request, cfg Config) {
	applyProxyCORS(w, r, cfg)
}

// applyProxyCORS — ChatGPT SPA sends custom headers (oai-device-id, sentinel tokens, etc.)
// which trigger OPTIONS preflight. Must respond before auth or browser shows CORS error.
func applyProxyCORS(w http.ResponseWriter, r *http.Request, cfg Config) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		if o := corsOrigin(cfg); o != "" {
			origin = o
		}
	}
	if origin == "" {
		origin = "*"
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	if origin != "*" {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS, HEAD")
	if req := r.Header.Get("Access-Control-Request-Headers"); req != "" {
		w.Header().Set("Access-Control-Allow-Headers", req)
	} else {
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, Cookie, oai-device-id, oai-language, openai-sentinel-chat-requirements-token, openai-sentinel-proof-token, chatgpt-conduit-token, x-conduit-token, x-requested-with, statsig-api-key, statsig-client-time, statsig-encoded, x-ms-blob-type, x-ms-version, x-ms-date, x-ms-client-request-id")
	}
	w.Header().Set("Access-Control-Max-Age", "86400")
}

func proxyHandlerPreflight(w http.ResponseWriter, r *http.Request, cfg Config) bool {
	if r.Method != http.MethodOptions {
		return false
	}
	applyProxyCORS(w, r, cfg)
	w.WriteHeader(http.StatusNoContent)
	return true
}

// skipSessionAuthPath — CDN chunks and third-party SDK calls (Statsig rgstr, etc.)
// often use fetch() without credentials; requiring ct_session causes 401 → CORS errors.
func skipSessionAuthPath(path string) bool {
	return strings.HasPrefix(path, "/cdn/") ||
		strings.HasPrefix(path, "/cdn-proxy/") ||
		strings.HasPrefix(path, "/extra-cdn-") ||
		strings.HasPrefix(path, "/extra-cdn-wild/") ||
		isProxyStaticPath(path)
}

func proxyHandlerExtraCDNCORS(w http.ResponseWriter, r *http.Request, cfg Config) bool {
	return proxyHandlerPreflight(w, r, cfg)
}
