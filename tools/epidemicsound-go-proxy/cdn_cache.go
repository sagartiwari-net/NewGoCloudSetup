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
	key := staticCacheKey(http.MethodGet, path)
	bodyPath, metaPath := cdnFilePath(key)
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
	if err != nil || len(body) == 0 {
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

func storeDiskStatic(method, path string, status int, contentType, encoding string, body []byte) {
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
	if err := os.MkdirAll(cdnCacheDir, 0o755); err != nil {
		return
	}
	key := staticCacheKey(method, path)
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
