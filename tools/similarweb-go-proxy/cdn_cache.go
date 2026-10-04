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

func similarSafeExt(path string) bool {
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

func similarURLHost(raw string) string {
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

func similarExtraCDNHost(path string) (host string, ok bool) {
	cfg := loadConfig()
	if !strings.HasPrefix(path, "/extra-cdn-") {
		return "", false
	}
	rest := strings.TrimPrefix(path, "/extra-cdn-")
	idxStr := ""
	for i := 0; i < len(rest); i++ {
		if rest[i] >= '0' && rest[i] <= '9' {
			idxStr += string(rest[i])
		} else {
			break
		}
	}
	idx, err := strconv.Atoi(idxStr)
	if err != nil || idx < 0 || idx >= len(cfg.ExtraCDNDomains) {
		return "", false
	}
	h := similarURLHost(cfg.ExtraCDNDomains[idx])
	return h, h != ""
}

func similarExtHost(path string) (host string, ok bool) {
	if !strings.HasPrefix(path, "/ext-host/") {
		return "", false
	}
	rest := strings.TrimPrefix(path, "/ext-host/")
	slash := strings.IndexByte(rest, '/')
	if slash <= 0 {
		return "", false
	}
	host = strings.ToLower(rest[:slash])
	return host, host != ""
}

func similarHostDenied(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimPrefix(h, "http://")
	h = strings.Split(h, "/")[0]
	if h == "" {
		return true
	}
	// Never disk-cache APIs, auth, payments, analytics, challenges.
	if strings.HasPrefix(h, "api.") || strings.Contains(h, "api.similarweb") {
		return true
	}
	if strings.HasPrefix(h, "account.") || strings.HasPrefix(h, "secure.") {
		return true
	}
	switch h {
	case "js.stripe.com", "m.stripe.com", "api.stripe.com",
		"www.googletagmanager.com", "www.google-analytics.com",
		"static.cloudflareinsights.com", "challenges.cloudflare.com":
		return true
	}
	return false
}

func similarExtraCDNAllowed(host string) bool {
	switch strings.ToLower(host) {
	case "fonts.googleapis.com", "fonts.gstatic.com":
		return true
	default:
		return false
	}
}

func isSimilarCDNPath(path string) bool {
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	lower := strings.ToLower(path)
	if strings.HasPrefix(lower, "/api/") ||
		strings.HasPrefix(lower, "/access") ||
		strings.HasPrefix(lower, "/user/") ||
		strings.HasPrefix(lower, "/__") {
		return false
	}

	if strings.HasPrefix(lower, "/extra-cdn-") {
		host, ok := similarExtraCDNHost(path)
		if !ok || similarHostDenied(host) || !similarExtraCDNAllowed(host) {
			return false
		}
		if similarSafeExt(path) {
			return true
		}
		// fonts.googleapis.com uses /css|/css2 with no file extension
		if host == "fonts.googleapis.com" {
			rest := lower
			if i := strings.Index(rest[len("/extra-cdn-"):], "/"); i >= 0 {
				rest = rest[len("/extra-cdn-")+i:]
			}
			return strings.HasPrefix(rest, "/css")
		}
		return false
	}

	if strings.HasPrefix(lower, "/ext-host/") {
		host, ok := similarExtHost(path)
		if !ok || similarHostDenied(host) {
			return false
		}
		return similarSafeExt(path)
	}

	if strings.HasPrefix(lower, "/cdn-proxy/") {
		return similarSafeExt(path)
	}

	// Same-origin hashed bundles (pro.similarweb.com) + classic static trees.
	return similarSafeExt(path)
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
	if !isSimilarCDNPath(path) {
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

func hasCachedCDNPath(path string) bool {
	if !isSimilarCDNPath(path) {
		return false
	}
	_, metaPath := cdnFilePath(path)
	_, err := os.Stat(metaPath)
	return err == nil
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
	if meta.Encoding != "" {
		w.Header().Set("Content-Encoding", meta.Encoding)
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Del("Pragma")
	w.Header().Del("Expires")
	w.Header().Set("X-Proxy-Cache", "HIT")
	w.Header().Set("X-Similar-Cache", "disk")
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

func storeCDNCachePath(path string, status int, contentType, encoding string, body []byte) {
	if path == "" {
		return
	}
	r := &http.Request{
		Method:     http.MethodGet,
		RequestURI: path,
		URL:        &url.URL{Path: path},
	}
	if i := strings.Index(path, "?"); i >= 0 {
		r.URL.Path = path[:i]
		r.URL.RawQuery = path[i+1:]
	}
	storeCDNCache(r, status, contentType, encoding, body)
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
