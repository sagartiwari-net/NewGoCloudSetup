package main

import (
	"context"
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
	cdnCacheDir  = "cdn-cache"
	cdnCacheTTL  = 72 * time.Hour
	cdnCacheMax  = 20 << 20
	cdnMemoryMax = 8 << 20
	cdnMemoryTTL = 6 * time.Hour
	cdnSoundMax  = 2 << 20
)

type staticCacheCtx struct{}

type staticCacheEntry struct {
	status      int
	contentType string
	body        []byte
	expires     time.Time
}

type cdnMeta struct {
	ContentType string `json:"content_type"`
	Status      int    `json:"status"`
}

var staticAssetCache sync.Map

func browserCacheKey(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	if r.URL.RawQuery == "" {
		return r.URL.Path
	}
	return r.URL.Path + "?" + r.URL.RawQuery
}

func isCacheableStaticPath(path string) bool {
	p := path
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	p = strings.ToLower(p)
	if strings.Contains(p, "/api/") || strings.Contains(p, "/ingest/") {
		return false
	}
	for _, ext := range []string{
		".js", ".mjs", ".css", ".map", ".woff", ".woff2", ".ttf",
		".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico",
		".wav", ".mp3", ".ogg", ".m4a",
	} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

func staticCacheKey(method, path string) string {
	return method + " " + path
}

func cdnFilePath(key string) (string, string) {
	sum := sha256.Sum256([]byte(key))
	name := hex.EncodeToString(sum[:])
	return filepath.Join(cdnCacheDir, name), filepath.Join(cdnCacheDir, name+".meta")
}

func loadDiskStatic(method, path string) *staticCacheEntry {
	if method != http.MethodGet && method != http.MethodHead {
		return nil
	}
	if !isCacheableStaticPath(path) {
		return nil
	}
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
		body:        body,
		expires:     now.Add(cdnMemoryTTL),
	}
}

func storeDiskStatic(method, path, contentType string, status int, body []byte) {
	if method != http.MethodGet || status != http.StatusOK || !isCacheableStaticPath(path) {
		return
	}
	if len(body) == 0 || len(body) > cdnCacheMax || skipCacheBody(contentType, len(body)) {
		return
	}
	if err := os.MkdirAll(cdnCacheDir, 0o755); err != nil {
		return
	}
	bodyPath, metaPath := cdnFilePath(staticCacheKey(method, path))
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
	log.Printf("[CDN_CACHE] STORE %s (%d bytes)", path, len(body))
}

func skipCacheBody(contentType string, n int) bool {
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "text/html") || strings.Contains(ct, "json") || strings.HasPrefix(ct, "video/") {
		return true
	}
	if strings.HasPrefix(ct, "audio/") && n > cdnSoundMax {
		return true
	}
	return false
}

func getStaticCached(method, path string) *staticCacheEntry {
	if method != http.MethodGet && method != http.MethodHead {
		return nil
	}
	if !isCacheableStaticPath(path) {
		return nil
	}
	key := staticCacheKey(http.MethodGet, path)
	v, ok := staticAssetCache.Load(key)
	if !ok {
		if disk := loadDiskStatic(method, path); disk != nil {
			if len(disk.body) <= cdnMemoryMax {
				staticAssetCache.Store(key, disk)
			}
			return disk
		}
		return nil
	}
	ent := v.(*staticCacheEntry)
	if time.Now().After(ent.expires) {
		staticAssetCache.Delete(key)
		if disk := loadDiskStatic(method, path); disk != nil {
			if len(disk.body) <= cdnMemoryMax {
				staticAssetCache.Store(key, disk)
			}
			return disk
		}
		return nil
	}
	return ent
}

func putStaticCached(path, contentType string, status int, body []byte) {
	if status != http.StatusOK || !isCacheableStaticPath(path) || skipCacheBody(contentType, len(body)) {
		return
	}
	if len(body) == 0 || len(body) > cdnCacheMax {
		return
	}
	cp := make([]byte, len(body))
	copy(cp, body)
	storeDiskStatic(http.MethodGet, path, contentType, status, cp)
	if len(cp) > cdnMemoryMax {
		return
	}
	staticAssetCache.Store(staticCacheKey(http.MethodGet, path), &staticCacheEntry{
		status:      status,
		contentType: contentType,
		body:        cp,
		expires:     time.Now().Add(cdnMemoryTTL),
	})
}

func serveStaticCached(w http.ResponseWriter, r *http.Request, ent *staticCacheEntry) {
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if ent.contentType != "" {
		w.Header().Set("Content-Type", ent.contentType)
	}
	w.Header().Set("X-OCG-Cache", "HIT")
	w.Header().Set("Content-Length", strconv.Itoa(len(ent.body)))
	w.WriteHeader(ent.status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(ent.body)
	}
}

func serveCachedStatic(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	key := browserCacheKey(r)
	ent := getStaticCached(r.Method, key)
	if ent == nil {
		return false
	}
	log.Printf("[CDN_CACHE] HIT %s", key)
	serveStaticCached(w, r, ent)
	return true
}

func tagStaticCache(dst, browser *http.Request) *http.Request {
	if dst == nil || browser == nil || !isCacheableStaticPath(browser.URL.Path) {
		return dst
	}
	return dst.WithContext(context.WithValue(dst.Context(), staticCacheCtx{}, browserCacheKey(browser)))
}

func staticCacheKeyFrom(r *http.Request) string {
	if r == nil {
		return ""
	}
	key, _ := r.Context().Value(staticCacheCtx{}).(string)
	return key
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
