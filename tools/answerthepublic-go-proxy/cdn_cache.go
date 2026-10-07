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
	cdnCacheMax = 20 << 20 // 20MB
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

func atpSafeExt(path string) bool {
	lower := strings.ToLower(path)
	if i := strings.Index(lower, "?"); i >= 0 {
		lower = lower[:i]
	}
	for _, ext := range []string{
		".js", ".mjs", ".css", ".woff", ".woff2", ".ttf", ".eot", ".otf",
		".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp", ".avif",
		".wasm", ".map",
	} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func atpStaticCDNHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.Split(h, "/")[0]
	switch {
	case h == "answerthepublic.com",
		h == "www.answerthepublic.com",
		h == "js.stripe.com",
		h == "fonts.googleapis.com",
		h == "fonts.gstatic.com",
		h == "static.cloudflareinsights.com":
		return true
	default:
		return strings.HasSuffix(h, ".answerthepublic.com") &&
			!strings.HasPrefix(h, "api.")
	}
}

func atpExtraCDNHost(path string) (host string, rest string, ok bool) {
	cfg := loadConfig()
	var restAll string
	switch {
	case strings.HasPrefix(path, "/extra-cdn-"):
		restAll = strings.TrimPrefix(path, "/extra-cdn-")
	default:
		return "", "", false
	}
	idxStr := ""
	for i := 0; i < len(restAll); i++ {
		if restAll[i] >= '0' && restAll[i] <= '9' {
			idxStr += string(restAll[i])
		} else {
			break
		}
	}
	idx, err := strconv.Atoi(idxStr)
	if err != nil || idx < 0 || idx >= len(cfg.ExtraCDNDomains) {
		return "", "", false
	}
	raw := strings.TrimSpace(cfg.ExtraCDNDomains[idx])
	if raw == "" {
		return "", "", false
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", "", false
	}
	rest = restAll[len(idxStr):]
	if rest == "" {
		rest = "/"
	}
	return strings.ToLower(u.Host), rest, true
}

// isATPCDNPath — UI/static assets only. Never API/auth/HTML JSON.
func isATPCDNPath(path string) bool {
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	lower := strings.ToLower(path)
	if strings.HasPrefix(lower, "/api/") ||
		strings.HasPrefix(lower, "/access") ||
		strings.HasPrefix(lower, "/user/") ||
		strings.HasPrefix(lower, "/__") ||
		strings.Contains(lower, "graphql") ||
		strings.Contains(lower, "/auth/") ||
		strings.Contains(lower, "/subscription") ||
		strings.Contains(lower, "/billing") {
		return false
	}

	if strings.HasPrefix(lower, "/extra-cdn-") {
		host, rest, ok := atpExtraCDNHost(path)
		if !ok || !atpStaticCDNHost(host) {
			return false
		}
		return atpSafeExt(rest) ||
			strings.Contains(strings.ToLower(rest), "/assets/") ||
			strings.Contains(strings.ToLower(rest), "/static/") ||
			strings.Contains(strings.ToLower(rest), "/dist/")
	}

	if strings.HasPrefix(lower, "/cdn-proxy/") {
		return atpSafeExt(path)
	}

	if strings.HasPrefix(lower, "/wp-content/") ||
		strings.HasPrefix(lower, "/wp-includes/") ||
		strings.HasPrefix(lower, "/dist/") ||
		strings.HasPrefix(lower, "/img/") {
		return true
	}
	if strings.HasPrefix(lower, "/assets/") ||
		strings.HasPrefix(lower, "/static/") ||
		strings.HasPrefix(lower, "/_next/static/") {
		return atpSafeExt(path)
	}

	return atpSafeExt(path)
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
	if !isATPCDNPath(path) {
		return ""
	}
	return path
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

func setATPCDNBrowserCache(w http.ResponseWriter, path string) {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".js") || strings.HasSuffix(lower, ".css") ||
		strings.HasSuffix(lower, ".woff2") || strings.HasSuffix(lower, ".woff") ||
		strings.Contains(lower, "/static/") || strings.Contains(lower, "/assets/") ||
		strings.HasPrefix(lower, "/extra-cdn-") || strings.HasPrefix(lower, "/cdn-proxy/") {
		w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	}
	w.Header().Del("Pragma")
	w.Header().Del("Expires")
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
	setATPCDNBrowserCache(w, key)
	w.Header().Set("X-Proxy-Cache", "HIT")
	w.Header().Set("X-ATP-Cache", "disk")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = r
	if meta.Status == 0 {
		meta.Status = http.StatusOK
	}
	w.WriteHeader(meta.Status)
	n, _ := io.Copy(w, file)
	log.Printf("[CDN_CACHE] HIT %s (%d bytes)", key, n)
	return true
}

func storeCDNCache(r *http.Request, status int, contentType, encoding string, body []byte) {
	key := cdnCacheKey(r)
	if key == "" || status != http.StatusOK || len(body) == 0 || len(body) > cdnCacheMax {
		return
	}
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "text/html") || strings.Contains(ct, "json") ||
		strings.Contains(ct, "text/event-stream") {
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

func initCDNCacheDir() {
	if err := os.MkdirAll(cdnCacheDir, 0o755); err != nil {
		log.Printf("[CDN_CACHE] mkdir failed: %v", err)
		return
	}
	wd, _ := os.Getwd()
	log.Printf("[CDN_CACHE] ready dir=%s/%s", wd, cdnCacheDir)
}
