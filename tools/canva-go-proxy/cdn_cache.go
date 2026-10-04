package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
)

type cdnMeta struct {
	ContentType string `json:"content_type"`
	Status      int    `json:"status"`
}

type cdnFlight struct {
	done chan struct{}
}

var cdnFlights sync.Map

func canvaHostDenied(raw string) bool {
	h := strings.ToLower(strings.TrimSpace(raw))
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimPrefix(h, "http://")
	h = strings.Split(h, "/")[0]
	if h == "" {
		return true
	}
	if strings.Contains(h, "document-export") {
		return true
	}
	if h == "export.canva.com" {
		return true
	}
	if strings.HasPrefix(h, "api.") || strings.Contains(h, "api.canva.com") {
		return true
	}
	if strings.Contains(h, "graphql") {
		return true
	}
	return false
}

func canvaCDNHostForPath(path string) (host string, ok bool) {
	cfg := loadConfig()
	if strings.HasPrefix(path, "/cdn-proxy/") {
		if u := strings.TrimSpace(cfg.CDNURL); u != "" {
			u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
			return strings.Split(u, "/")[0], true
		}
		return "static.canva.com", true
	}
	for i, extra := range cfg.ExtraCDNDomains {
		prefix := fmt.Sprintf("/extra-cdn-%d/", i)
		if strings.HasPrefix(path, prefix) {
			h := strings.TrimPrefix(strings.TrimPrefix(extra, "https://"), "http://")
			return strings.Split(h, "/")[0], true
		}
	}
	return "", false
}

func canvaSafeExt(path string) bool {
	lower := strings.ToLower(path)
	for _, ext := range []string{".js", ".css", ".mjs", ".woff", ".woff2", ".ttf", ".eot", ".otf", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp", ".avif", ".wasm", ".map"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func canvaHostAllowsExtensionless(host string) bool {
	h := strings.ToLower(host)
	return strings.Contains(h, "static.canva") ||
		strings.Contains(h, "media.canva") ||
		strings.Contains(h, "media-public.canva") ||
		strings.Contains(h, "template.canva") ||
		strings.Contains(h, "category-public.canva") ||
		strings.Contains(h, "chunk-composing.canva") ||
		strings.Contains(h, "sdk.canva") ||
		strings.Contains(h, "content-management-files") ||
		strings.Contains(h, "content-management-public")
}

func isCanvaCDNPath(path string) bool {
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	if !strings.HasPrefix(path, "/cdn-proxy/") && !strings.HasPrefix(path, "/extra-cdn-") {
		return false
	}
	host, ok := canvaCDNHostForPath(path)
	if !ok || canvaHostDenied(host) {
		return false
	}
	if canvaSafeExt(path) {
		return true
	}
	return canvaHostAllowsExtensionless(host)
}

func cdnCacheKey(r *http.Request) string {
	if r == nil || r.Method != http.MethodGet {
		return ""
	}
	if isCanvaCDNPath(r.URL.Path) {
		return r.URL.RequestURI()
	}
	return ""
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

func serveCachedCDN(w http.ResponseWriter, r *http.Request) bool {
	key := cdnCacheKey(r)
	if key == "" {
		return false
	}
	if serveDiskCDN(w, key) {
		return true
	}
	leader, done := joinCDNFlight(key)
	if leader {
		return false
	}
	<-done
	return serveDiskCDN(w, key)
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
	w.Header().Set("X-Canva-Cache", "disk")
	if meta.Status == 0 {
		meta.Status = http.StatusOK
	}
	w.WriteHeader(meta.Status)
	n, _ := io.Copy(w, file)
	log.Printf("[CDN_CACHE] HIT %s (%d bytes)", key, n)
	return true
}

func storeCDNCache(r *http.Request, status int, contentType string, body []byte) {
	key := cdnCacheKey(r)
	if key == "" || status != http.StatusOK || len(body) == 0 || len(body) > cdnCacheMax {
		return
	}
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "text/html") || strings.Contains(ct, "json") ||
		strings.Contains(ct, "text/event-stream") || strings.Contains(ct, "application/grpc") {
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
	meta, _ := json.Marshal(cdnMeta{ContentType: contentType, Status: status})
	_ = os.WriteFile(metaPath, meta, 0o644)
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
