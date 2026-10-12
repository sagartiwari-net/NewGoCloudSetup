package main

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type staticCacheEntry struct {
	status      int
	contentType string
	encoding    string
	body        []byte
	expires     time.Time
}

var staticAssetCache sync.Map

func staticCacheKey(method, path string) string {
	return method + " " + path
}

func isCacheableStaticPath(path string) bool {
	p := path
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	p = strings.ToLower(p)
	if strings.Contains(p, "/api/") || strings.HasSuffix(p, ".html") || strings.HasSuffix(p, ".json") {
		return false
	}
	for _, ext := range []string{".js", ".css", ".woff2", ".woff", ".ttf", ".eot", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp", ".avif"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
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
	v, ok := staticAssetCache.Load(staticCacheKey(method, path))
	if !ok {
		if method == http.MethodHead {
			v, ok = staticAssetCache.Load(staticCacheKey(http.MethodGet, path))
		}
		if !ok {
			if disk := loadDiskStatic(method, path); disk != nil {
				staticAssetCache.Store(staticCacheKey(http.MethodGet, path), disk)
				return disk
			}
			return nil
		}
	}
	ent := v.(*staticCacheEntry)
	if time.Now().After(ent.expires) {
		staticAssetCache.Delete(staticCacheKey(http.MethodGet, path))
		if disk := loadDiskStatic(method, path); disk != nil {
			staticAssetCache.Store(staticCacheKey(http.MethodGet, path), disk)
			return disk
		}
		return nil
	}
	return ent
}

func putStaticCached(method, path string, status int, contentType, encoding string, body []byte) {
	if method != http.MethodGet || status != http.StatusOK || !isCacheableStaticPath(path) {
		return
	}
	if len(body) == 0 || len(body) > cdnCacheMax {
		return
	}
	cp := make([]byte, len(body))
	copy(cp, body)
	staticAssetCache.Store(staticCacheKey(method, path), &staticCacheEntry{
		status:      status,
		contentType: contentType,
		encoding:    encoding,
		body:        cp,
		expires:     time.Now().Add(6 * time.Hour),
	})
	storeDiskStatic(method, path, status, contentType, encoding, cp)
}

func serveStaticCached(w http.ResponseWriter, r *http.Request, ent *staticCacheEntry) {
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
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
