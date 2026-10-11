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
