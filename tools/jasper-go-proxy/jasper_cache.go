package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
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
	cdnCacheMax = 20 << 20
)

type cdnMeta struct {
	ContentType string `json:"content_type"`
	Encoding    string `json:"encoding,omitempty"`
	Status      int    `json:"status"`
}

type staticCacheEntry struct {
	status      int
	contentType string
	encoding    string
	body        []byte
	expires     time.Time
}

var staticAssetCache sync.Map

func requestCachePath(r *http.Request) string {
	if r.URL.RawQuery == "" {
		return r.URL.Path
	}
	return r.URL.Path + "?" + r.URL.RawQuery
}

func staticCacheKey(method, path string) string {
	return method + " " + path
}

func isCacheableStaticPath(path string) bool {
	p := path
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	p = strings.ToLower(p)
	if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/access") {
		return false
	}
	if strings.Contains(p, "/assets/") || strings.HasPrefix(p, "/images/") || strings.Contains(p, "/fonts/") {
		return true
	}
	for _, ext := range []string{".js", ".css", ".wasm", ".woff2", ".woff", ".ttf", ".eot", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp", ".map"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

func cdnFilePath(key string) (string, string) {
	sum := sha256.Sum256([]byte(key))
	name := hex.EncodeToString(sum[:])
	return filepath.Join(cdnCacheDir, name), filepath.Join(cdnCacheDir, name+".meta")
}

func loadDiskStatic(path string) *staticCacheEntry {
	bodyPath, metaPath := cdnFilePath(staticCacheKey(http.MethodGet, path))
	info, err := os.Stat(bodyPath)
	if err != nil || time.Since(info.ModTime()) > cdnCacheTTL {
		return nil
	}
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		return nil
	}
	var meta cdnMeta
	if json.Unmarshal(metaBytes, &meta) != nil || meta.Status != http.StatusOK {
		return nil
	}
	ct := strings.ToLower(meta.ContentType)
	if strings.Contains(ct, "text/html") || strings.Contains(ct, "json") {
		return nil
	}
	body, err := os.ReadFile(bodyPath)
	if err != nil || len(body) == 0 || len(body) > cdnCacheMax {
		return nil
	}
	now := time.Now()
	_ = os.Chtimes(bodyPath, now, now)
	_ = os.Chtimes(metaPath, now, now)
	return &staticCacheEntry{
		status:      meta.Status,
		contentType: meta.ContentType,
		encoding:    meta.Encoding,
		body:        body,
		expires:     now.Add(6 * time.Hour),
	}
}

func storeDiskStatic(path string, status int, contentType, encoding string, body []byte) {
	if len(body) == 0 || len(body) > cdnCacheMax {
		return
	}
	if err := os.MkdirAll(cdnCacheDir, 0o755); err != nil {
		return
	}
	bodyPath, metaPath := cdnFilePath(staticCacheKey(http.MethodGet, path))
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
}

func getStaticCached(method, path string) *staticCacheEntry {
	if method != http.MethodGet && method != http.MethodHead {
		return nil
	}
	if !isCacheableStaticPath(path) {
		return nil
	}
	key := staticCacheKey(http.MethodGet, path)
	if v, ok := staticAssetCache.Load(key); ok {
		ent := v.(*staticCacheEntry)
		if time.Now().Before(ent.expires) {
			return ent
		}
		staticAssetCache.Delete(key)
	}
	if disk := loadDiskStatic(path); disk != nil {
		staticAssetCache.Store(key, disk)
		return disk
	}
	return nil
}

func putStaticCached(method, path string, status int, contentType, encoding string, body []byte) {
	if method != http.MethodGet || status != http.StatusOK || !isCacheableStaticPath(path) {
		return
	}
	if len(body) == 0 || len(body) > cdnCacheMax {
		return
	}
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "text/html") || strings.Contains(ct, "json") || strings.HasPrefix(ct, "video/") || strings.HasPrefix(ct, "audio/") {
		return
	}
	cp := make([]byte, len(body))
	copy(cp, body)
	staticAssetCache.Store(staticCacheKey(http.MethodGet, path), &staticCacheEntry{
		status:      status,
		contentType: contentType,
		encoding:    encoding,
		body:        cp,
		expires:     time.Now().Add(6 * time.Hour),
	})
	storeDiskStatic(path, status, contentType, encoding, cp)
}

func serveStaticCached(w http.ResponseWriter, r *http.Request, ent *staticCacheEntry) {
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	w.Header().Del("Pragma")
	w.Header().Del("Expires")
	if ent.contentType != "" {
		w.Header().Set("Content-Type", ent.contentType)
	}
	if ent.encoding != "" {
		w.Header().Set("Content-Encoding", ent.encoding)
	}
	w.Header().Set("X-OCG-Cache", "HIT")
	w.Header().Set("Content-Length", strconv.Itoa(len(ent.body)))
	w.WriteHeader(ent.status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(ent.body)
	}
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
	log.Printf("[CDN_CACHE] static files kept for %s", cdnCacheTTL)
}
