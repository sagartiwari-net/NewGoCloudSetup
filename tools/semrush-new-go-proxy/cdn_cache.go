package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	cdnCacheDir = "cdn-cache"
	cdnCacheTTL = 72 * time.Hour
	cdnCacheMax = 20 << 20 // 20MB
	cdnFlightWait = 45 * time.Second
)

type cdnMeta struct {
	ContentType string `json:"content_type"`
	Status      int    `json:"status"`
}

type cdnFlight struct {
	done chan struct{}
}

var cdnFlights sync.Map

func initCDNCacheDir() {
	if err := os.MkdirAll(cdnCacheDir, 0o755); err != nil {
		log.Printf("[CDN_CACHE] mkdir failed: %v", err)
		return
	}
	wd, _ := os.Getwd()
	log.Printf("[CDN_CACHE] ready dir=%s/%s", wd, cdnCacheDir)
}

func isSemrushCDNPath(path string) bool {
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	lower := strings.ToLower(path)
	if !strings.HasPrefix(lower, "/static-proxy/") &&
		!strings.HasPrefix(lower, "/secure-proxy/") &&
		!strings.HasPrefix(lower, "/cdn-proxy/") &&
		!strings.HasPrefix(lower, "/ai-proxy/") {
		return false
	}
	for _, ext := range []string{
		".js", ".mjs", ".css", ".woff", ".woff2", ".ttf", ".otf",
		".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp", ".avif",
		".wasm",
	} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// cdnCacheKey returns a disk-cache key for inbound browser requests (proxy prefixes intact).
func cdnCacheKey(r *http.Request) string {
	if r == nil || r.Method != http.MethodGet {
		return ""
	}
	if isSemrushCDNPath(r.URL.Path) {
		return r.URL.RequestURI()
	}
	return ""
}

// cdnCacheKeyFromUpstream reconstructs the browser-facing URI after Director strips the prefix.
func cdnCacheKeyFromUpstream(r *http.Request) string {
	if r == nil || r.Method != http.MethodGet {
		return ""
	}
	host := strings.ToLower(r.Host)
	if host == "" {
		host = strings.ToLower(r.URL.Host)
	}
	var prefix string
	switch host {
	case "static.semrush.com":
		prefix = "/static-proxy"
	case "secure.semrush.com":
		prefix = "/secure-proxy"
	case "cdn.semrush.com":
		prefix = "/cdn-proxy"
	case "ai-visibility-index.semrush.com":
		prefix = "/ai-proxy"
	default:
		return ""
	}
	path := r.URL.Path
	if path == "" {
		path = "/"
	}
	fullPath := prefix + path
	if !isSemrushCDNPath(fullPath) {
		return ""
	}
	if r.URL.RawQuery != "" {
		return fullPath + "?" + r.URL.RawQuery
	}
	return fullPath
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

// serveCachedCDN serves from disk when possible.
// Returns (served, isLeader). Only the leader should fetch upstream + complete the flight.
func serveCachedCDN(w http.ResponseWriter, r *http.Request) (served bool, isLeader bool) {
	key := cdnCacheKey(r)
	if key == "" {
		return false, false
	}
	if serveDiskCDN(w, key) {
		return true, false
	}
	leader, done := joinCDNFlight(key)
	if leader {
		return false, true
	}
	select {
	case <-done:
	case <-time.After(cdnFlightWait):
		log.Printf("[CDN_CACHE] WAIT timeout key=%s — falling through to upstream", trimKey(key))
	}
	if serveDiskCDN(w, key) {
		return true, false
	}
	return false, false
}

func trimKey(key string) string {
	if len(key) > 120 {
		return key[:120] + "…"
	}
	return key
}

func serveDiskCDN(w http.ResponseWriter, key string) bool {
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
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Del("Pragma")
	w.Header().Del("Expires")
	w.Header().Set("X-Proxy-Cache", "HIT")
	w.Header().Set("X-Semrush-Cache", "disk")
	if meta.Status == 0 {
		meta.Status = http.StatusOK
	}
	w.WriteHeader(meta.Status)
	n, _ := io.Copy(w, file)
	log.Printf("[CDN_CACHE] HIT %s (%d bytes)", trimKey(key), n)
	return true
}

func storeCDNCache(r *http.Request, status int, contentType string, body []byte) {
	key := cdnCacheKey(r)
	if key == "" {
		key = cdnCacheKeyFromUpstream(r)
	}
	storeCDNCacheKey(key, status, contentType, body)
}

func storeCDNCacheKey(key string, status int, contentType string, body []byte) {
	if key == "" {
		return
	}
	if status != http.StatusOK {
		log.Printf("[CDN_CACHE] SKIP status=%d key=%s", status, trimKey(key))
		return
	}
	if len(body) == 0 || len(body) > cdnCacheMax {
		log.Printf("[CDN_CACHE] SKIP size=%d key=%s", len(body), trimKey(key))
		return
	}
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		return
	}
	if err := os.MkdirAll(cdnCacheDir, 0o755); err != nil {
		log.Printf("[CDN_CACHE] STORE mkdir failed: %v", err)
		return
	}
	bodyPath, metaPath := cdnFilePath(key)
	tmp := bodyPath + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		log.Printf("[CDN_CACHE] STORE write failed: %v key=%s", err, trimKey(key))
		return
	}
	if err := os.Rename(tmp, bodyPath); err != nil {
		_ = os.Remove(tmp)
		log.Printf("[CDN_CACHE] STORE rename failed: %v key=%s", err, trimKey(key))
		return
	}
	meta, _ := json.Marshal(cdnMeta{ContentType: contentType, Status: status})
	_ = os.WriteFile(metaPath, meta, 0o644)
	log.Printf("[CDN_CACHE] STORE %s (%d bytes)", trimKey(key), len(body))
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
	initCDNCacheDir()
	go func() {
		for {
			time.Sleep(30 * time.Minute)
			sweepCDNCache()
		}
	}()
}
