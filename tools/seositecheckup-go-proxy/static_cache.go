package main

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// In-memory static asset cache — SSC splash loads many JS/CSS through uTLS;
// re-fetching every time makes /dashboard feel stuck. Browser also gets max-age.
type staticCacheEntry struct {
	status      int
	contentType string
	encoding    string
	body        []byte
	expires     time.Time
}

var staticAssetCache sync.Map // key -> *staticCacheEntry

func staticCacheKey(method, path string) string {
	return method + " " + path
}

func isCacheableStaticPath(path string) bool {
	p := path
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	p = strings.ToLower(p)
	// Never cache HTML/API/auth
	if p == "/" || p == "/dashboard" || strings.HasPrefix(p, "/dashboard/") {
		return false
	}
	if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/auth/") || strings.HasPrefix(p, "/user/") {
		return false
	}
	// API-ish extra-cdn routes (execute-api /prod/...) — not static
	if strings.Contains(p, "/prod/") || strings.Contains(p, "/main/") || strings.Contains(p, "/g/collect") {
		return false
	}
	for _, ext := range []string{".js", ".css", ".woff2", ".woff", ".ttf", ".eot", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp", ".map", ".mjs"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	if strings.HasPrefix(p, "/static/") || strings.HasPrefix(p, "/assets/") ||
		strings.HasPrefix(p, "/cdn-proxy/") || strings.Contains(p, "/chunk") {
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
	v, ok := staticAssetCache.Load(staticCacheKey(method, path))
	if !ok {
		if method == http.MethodHead {
			v, ok = staticAssetCache.Load(staticCacheKey(http.MethodGet, path))
		}
		if !ok {
			return nil
		}
	}
	ent := v.(*staticCacheEntry)
	if time.Now().After(ent.expires) {
		staticAssetCache.Delete(staticCacheKey(http.MethodGet, path))
		return nil
	}
	return ent
}

func putStaticCached(method, path string, status int, contentType, encoding string, body []byte) {
	if method != http.MethodGet || status != 200 || !isCacheableStaticPath(path) {
		return
	}
	if len(body) == 0 || len(body) > 8<<20 {
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
}

func serveStaticCached(w http.ResponseWriter, r *http.Request, ent *staticCacheEntry) {
	w.Header().Set("Cache-Control", "public, max-age=86400")
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

func setProxyCacheHeaders(w http.ResponseWriter, path, contentType string) {
	if isCacheableStaticPath(path) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Del("Pragma")
		w.Header().Del("Expires")
		return
	}
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "javascript") || strings.Contains(ct, "text/css") ||
		strings.Contains(ct, "font/") || strings.Contains(ct, "image/") {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Del("Pragma")
		w.Header().Del("Expires")
		return
	}
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}
