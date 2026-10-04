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
	cdnCacheMax = 20 << 20 // 20MB — covers GPTZero's large hashed JS bundles
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

func gzSafeExt(path string) bool {
	lower := strings.ToLower(path)
	for _, ext := range []string{
		".js", ".css", ".mjs", ".woff", ".woff2", ".ttf", ".eot", ".otf",
		".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp", ".avif",
		".wasm", ".map",
	} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func gzURLHost(raw string) string {
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

func gzExtraCDNHost(path string) (host string, rest string, ok bool) {
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
	if err != nil || idx < 0 || idx >= len(cfg.ExtraDomains) {
		return "", "", false
	}
	h := gzURLHost(cfg.ExtraDomains[idx])
	if h == "" {
		return "", "", false
	}
	rest = restAll[len(idxStr):]
	if rest == "" {
		rest = "/"
	}
	if !strings.HasPrefix(rest, "/") {
		rest = "/" + rest
	}
	return h, rest, true
}

func gzHostDenied(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.Split(h, "/")[0]
	if h == "" {
		return true
	}
	if strings.Contains(h, "supabase") {
		return true
	}
	switch h {
	case "api.gptzero.me", "auth.gptzero.me":
		return true
	}
	if strings.HasPrefix(h, "api.") {
		return true
	}
	return false
}

func isGPTZeroCDNPath(path string) bool {
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	lower := strings.ToLower(path)
	if strings.HasPrefix(lower, "/v2/") ||
		strings.HasPrefix(lower, "/v3/") ||
		strings.HasPrefix(lower, "/api-proxy/") ||
		strings.HasPrefix(lower, "/api/") ||
		strings.HasPrefix(lower, "/access") ||
		strings.HasPrefix(lower, "/auth/") ||
		strings.HasPrefix(lower, "/__") ||
		strings.Contains(lower, "supabase") {
		return false
	}

	if strings.HasPrefix(lower, "/extra-cdn-wild/") {
		return gzSafeExt(path)
	}
	if strings.HasPrefix(lower, "/extra-cdn-") {
		host, rest, ok := gzExtraCDNHost(path)
		if !ok || gzHostDenied(host) {
			return false
		}
		// Static CDNs only (cdn.gptzero.me / marketing assets).
		if host == "cdn.gptzero.me" || host == "gptzero.me" || host == "www.gptzero.me" {
			return gzSafeExt(rest) || gzSafeExt(path)
		}
		return false
	}

	// Only hashed app bundles / known static trees — never bare "/*.js" (SPA HTML fallback).
	if strings.HasPrefix(lower, "/assets/") ||
		strings.HasPrefix(lower, "/static/") ||
		strings.HasPrefix(lower, "/fonts/") ||
		strings.HasPrefix(lower, "/_next/static/") {
		return gzSafeExt(path)
	}
	// Vite/app entry helpers (not SPA HTML routes).
	if lower == "/manifest.js" || strings.HasPrefix(lower, "/manifest.") {
		return gzSafeExt(path)
	}
	return false
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
	if !isGPTZeroCDNPath(path) {
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
	// Browser + edge: cache static assets for a day (disk TTL is longer).
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	w.Header().Del("Pragma")
	w.Header().Del("Expires")
	w.Header().Set("X-Proxy-Cache", "HIT")
	if meta.Status == 0 {
		meta.Status = http.StatusOK
	}
	w.WriteHeader(meta.Status)
	n, _ := io.Copy(w, file)
	log.Printf("[CDN_CACHE] HIT %s (%d bytes)", key, n)
	return true
}

func storeCDNCache(r *http.Request, status int, contentType, encoding string, body []byte) bool {
	key := cdnCacheKey(r)
	if key == "" || status != http.StatusOK || len(body) == 0 || len(body) > cdnCacheMax {
		return false
	}
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "text/html") || strings.Contains(ct, "text/event-stream") {
		return false
	}
	// Reject SPA HTML masquerading as JS (wrong Content-Type).
	trim := body
	if len(trim) > 64 {
		trim = trim[:64]
	}
	head := strings.ToLower(string(trim))
	if strings.Contains(head, "<!doctype") || strings.Contains(head, "<html") {
		return false
	}
	if err := os.MkdirAll(cdnCacheDir, 0o755); err != nil {
		return false
	}
	bodyPath, metaPath := cdnFilePath(key)
	tmp := bodyPath + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return false
	}
	if err := os.Rename(tmp, bodyPath); err != nil {
		_ = os.Remove(tmp)
		return false
	}
	meta, _ := json.Marshal(cdnMeta{ContentType: contentType, Encoding: encoding, Status: status})
	_ = os.WriteFile(metaPath, meta, 0o644)
	log.Printf("[CDN_CACHE] STORE %s (%d bytes)", key, len(body))
	return true
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
