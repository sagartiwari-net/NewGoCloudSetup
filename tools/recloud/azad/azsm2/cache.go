package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	cdnCacheMax = 8 << 20
)

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

func isCacheableStaticPath(path string) bool {
	p := path
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	p = strings.ToLower(p)
	if strings.HasPrefix(p, "/__static__/") || strings.HasPrefix(p, "/static-semrush-com/") {
		return true
	}
	for _, ext := range []string{".js", ".css", ".woff2", ".woff", ".ttf", ".eot", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

func staticCacheKey(path string) string {
	return "GET " + path
}

func cdnFilePath(key string) (string, string) {
	sum := sha256.Sum256([]byte(key))
	name := hex.EncodeToString(sum[:])
	return filepath.Join(cdnCacheDir, name), filepath.Join(cdnCacheDir, name+".meta")
}

func loadDiskStatic(path string) *staticCacheEntry {
	if !isCacheableStaticPath(path) {
		return nil
	}
	bodyPath, metaPath := cdnFilePath(staticCacheKey(path))
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
		expires:     now.Add(6 * time.Hour),
	}
}

func storeDiskStatic(path string, status int, contentType string, body []byte) {
	if status != http.StatusOK || !isCacheableStaticPath(path) {
		return
	}
	if len(body) == 0 || len(body) > cdnCacheMax {
		return
	}
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "text/html") || strings.Contains(ct, "json") {
		return
	}
	if err := os.MkdirAll(cdnCacheDir, 0o755); err != nil {
		return
	}
	bodyPath, metaPath := cdnFilePath(staticCacheKey(path))
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

func getStaticCached(path string) *staticCacheEntry {
	if !isCacheableStaticPath(path) {
		return nil
	}
	key := staticCacheKey(path)
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

func putStaticCached(path string, status int, contentType string, body []byte) {
	if status != http.StatusOK || !isCacheableStaticPath(path) || len(body) == 0 || len(body) > cdnCacheMax {
		return
	}
	cp := append([]byte(nil), body...)
	staticAssetCache.Store(staticCacheKey(path), &staticCacheEntry{
		status:      status,
		contentType: contentType,
		body:        cp,
		expires:     time.Now().Add(6 * time.Hour),
	})
	storeDiskStatic(path, status, contentType, cp)
}

func startCDNCacheSweep() {
	go func() {
		for {
			time.Sleep(30 * time.Minute)
			entries, err := os.ReadDir(cdnCacheDir)
			if err != nil {
				continue
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
	}()
}
