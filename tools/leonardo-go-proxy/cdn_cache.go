package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	cdnCacheDir = "cdn-cache"
	cdnCacheTTL = 72 * time.Hour
	cdnCacheMax = 20 << 20 // 20MB
)

type cdnMeta struct {
	ContentType string `json:"content_type"`
	Encoding    string `json:"encoding,omitempty"`
	Status      int    `json:"status"`
}

type cdnFlight struct {
	done chan struct{}
}

var cdnFlights sync.Map

func leoSafeExt(path string) bool {
	lower := strings.ToLower(path)
	for _, ext := range []string{
		".js", ".css", ".mjs", ".woff", ".woff2", ".ttf", ".eot", ".otf",
		".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp", ".avif",
		".mp4", ".webm", ".mov",
		".wasm", ".map",
	} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func leoURLHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Host)
}

func leoExtraCDNHost(path string) (host string, rest string, ok bool) {
	cfg := loadConfig()
	if !strings.HasPrefix(path, "/extra-cdn-") {
		return "", "", false
	}
	restAll := strings.TrimPrefix(path, "/extra-cdn-")
	idxStr := ""
	for i := 0; i < len(restAll); i++ {
		if restAll[i] >= '0' && restAll[i] <= '9' {
			idxStr += string(restAll[i])
		} else {
			break
		}
	}
	idx, err := strconv.Atoi(idxStr)
	if err != nil || idx < 0 || idx >= len(cfg.ExtraCDNDomains) {
		return "", "", false
	}
	h := leoURLHost(cfg.ExtraCDNDomains[idx])
	if h == "" {
		return "", "", false
	}
	rest = restAll[len(idxStr):]
	if rest == "" {
		rest = "/"
	}
	return h, rest, true
}

func leoHostDenied(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.Split(h, "/")[0]
	if h == "" {
		return true
	}
	switch h {
	case "cloud.leonardo.ai", "api.leonardo.ai", "auth.leonardo.ai",
		"js.stripe.com", "api.stripe.com", "checkout.stripe.com",
		"m.stripe.com", "m.stripe.network", "hooks.stripe.com":
		return true
	}
	if strings.HasPrefix(h, "api.") {
		return true
	}
	return false
}

func leoExtraCDNAllowed(host, rest string) bool {
	restLower := strings.ToLower(rest)
	if strings.Contains(restLower, "/users/") ||
		strings.Contains(restLower, "/generations/") ||
		strings.Contains(restLower, "/graphql") ||
		strings.Contains(restLower, "/api/") {
		return false
	}
	switch host {
	case "cdn.leonardo.ai":
		// UI/static assets only — never user-generated media.
		return strings.Contains(restLower, "/static/") && leoSafeExt(rest)
	case "assets.leonardo.ai":
		return leoSafeExt(rest)
	default:
		return false
	}
}

func isLeonardoCDNPath(path string) bool {
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	lower := strings.ToLower(path)
	if strings.HasPrefix(lower, "/api/") ||
		strings.HasPrefix(lower, "/access") ||
		strings.HasPrefix(lower, "/user/") ||
		strings.HasPrefix(lower, "/__") ||
		strings.HasPrefix(lower, "/_next/data/") ||
		strings.HasPrefix(lower, "/_next/image") ||
		strings.Contains(lower, "graphql") ||
		strings.Contains(lower, "/v1/query") {
		return false
	}

	if strings.HasPrefix(lower, "/extra-cdn-") {
		host, rest, ok := leoExtraCDNHost(path)
		if !ok || leoHostDenied(host) || !leoExtraCDNAllowed(host, rest) {
			return false
		}
		return true
	}

	if strings.HasPrefix(lower, "/cdn-proxy/") {
		return leoSafeExt(path)
	}

	// Next.js hashed bundles — primary splash weight.
	if strings.HasPrefix(lower, "/_next/static/") {
		return leoSafeExt(path)
	}

	return leoSafeExt(path)
}

func cdnRequestKey(r *http.Request) string {
	if r == nil {
		return ""
	}
	if r.RequestURI != "" && r.RequestURI != "*" {
		return r.RequestURI
	}
	if r.URL == nil {
		return ""
	}
	return r.URL.RequestURI()
}

func cdnCacheKey(r *http.Request) string {
	if r == nil || r.Method != http.MethodGet {
		return ""
	}
	path := ""
	if r.URL != nil {
		path = r.URL.Path
	}
	if path == "" && r.RequestURI != "" {
		path = r.RequestURI
		if i := strings.Index(path, "?"); i >= 0 {
			path = path[:i]
		}
	}
	if !isLeonardoCDNPath(path) {
		return ""
	}
	return cdnRequestKey(r)
}

func cdnFilePath(key string) (bodyPath, metaPath string) {
	sum := sha256.Sum256([]byte(key))
	name := hex.EncodeToString(sum[:])
	return filepath.Join(cdnCacheDir, name), filepath.Join(cdnCacheDir, name+".meta")
}

func joinCDNFlight(key string) (leader bool, done <-chan struct{}) {
	flight := &cdnFlight{done: make(chan struct{})}
	actual, loaded := cdnFlights.LoadOrStore(key, flight)
	if loaded {
		return false, actual.(*cdnFlight).done
	}
	return true, flight.done
}

func completeCDNFlight(key string) {
	if key == "" {
		return
	}
	if value, ok := cdnFlights.LoadAndDelete(key); ok {
		close(value.(*cdnFlight).done)
	}
}

func hasCachedCDNPath(path string) bool {
	if !isLeonardoCDNPath(path) {
		return false
	}
	_, metaPath := cdnFilePath(path)
	_, err := os.Stat(metaPath)
	return err == nil
}

func serveCachedCDN(w http.ResponseWriter, r *http.Request) bool {
	key := cdnCacheKey(r)
	if key == "" {
		return false
	}
	if serveDiskCDN(w, r, key) {
		return true
	}
	leader, done := joinCDNFlight(key)
	if leader {
		return false
	}
	<-done
	return serveDiskCDN(w, r, key)
}

func serveDiskCDN(w http.ResponseWriter, r *http.Request, key string) bool {
	bodyPath, metaPath := cdnFilePath(key)
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		return false
	}
	var meta cdnMeta
	if json.Unmarshal(metaBytes, &meta) != nil {
		return false
	}
	file, err := os.Open(bodyPath)
	if err != nil {
		return false
	}
	defer file.Close()
	now := time.Now()
	_ = os.Chtimes(bodyPath, now, now)
	_ = os.Chtimes(metaPath, now, now)
	if meta.ContentType != "" {
		w.Header().Set("Content-Type", meta.ContentType)
	}
	if meta.Encoding != "" {
		w.Header().Set("Content-Encoding", meta.Encoding)
	}
	// Match setProxyCacheHeaders: long browser TTL on disk hits.
	lowerKey := strings.ToLower(key)
	if strings.Contains(lowerKey, "/_next/static/") || strings.Contains(lowerKey, "/static/") {
		w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	}
	w.Header().Del("Pragma")
	w.Header().Del("Expires")
	w.Header().Set("X-Proxy-Cache", "HIT")
	w.Header().Set("X-Leonardo-Cache", "disk")
	if r != nil {
		applyResponseCORS(w, r, loadConfig())
	}
	if meta.Status == 0 {
		meta.Status = http.StatusOK
	}
	w.WriteHeader(meta.Status)
	n, _ := io.Copy(w, file)
	log.Printf("[CDN_CACHE] HIT %s (%d bytes)", key, n)
	return true
}

func storeCDNCache(r *http.Request, status int, contentType, encoding string, body []byte) {
	key := cdnCacheKey(r)
	if key == "" || status != http.StatusOK || len(body) == 0 || len(body) > cdnCacheMax {
		return
	}
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "text/html") || strings.Contains(ct, "json") ||
		strings.Contains(ct, "text/event-stream") {
		return
	}
	if err := os.MkdirAll(cdnCacheDir, 0o755); err != nil {
		return
	}
	bodyPath, metaPath := cdnFilePath(key)
	tmp := bodyPath + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, bodyPath); err != nil {
		_ = os.Remove(tmp)
		return
	}
	meta, _ := json.Marshal(cdnMeta{ContentType: contentType, Encoding: encoding, Status: status})
	_ = os.WriteFile(metaPath, meta, 0o644)
	log.Printf("[CDN_CACHE] STORE %s (%d bytes)", key, len(body))
}

func storeCDNCachePath(path string, status int, contentType, encoding string, body []byte) {
	if path == "" {
		return
	}
	r := &http.Request{
		Method:     http.MethodGet,
		RequestURI: path,
		URL:        &url.URL{Path: path},
	}
	if i := strings.Index(path, "?"); i >= 0 {
		r.URL.Path = path[:i]
		r.URL.RawQuery = path[i+1:]
	}
	storeCDNCache(r, status, contentType, encoding, body)
}

func sweepCDNCache() {
	entries, err := os.ReadDir(cdnCacheDir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-cdnCacheTTL)
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(cdnCacheDir, entry.Name()))
	}
}

func startCDNCacheSweep() {
	go func() {
		for {
			time.Sleep(30 * time.Minute)
			sweepCDNCache()
		}
	}()
}
