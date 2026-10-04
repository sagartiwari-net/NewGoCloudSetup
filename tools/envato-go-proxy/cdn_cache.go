package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	cdnCacheDir = "cdn-cache"
	cdnCacheTTL = 72 * time.Hour
	cdnCacheMax = 20 << 20 // 20MB per asset file

	// Thumbnails (/__resized__ only): small per-file + hard total disk budget.
	cdnThumbMaxFile = 2 << 20  // 2MB per thumb
	cdnThumbBudget  = 30 << 20 // 30MB total thumbs on server
)

type cdnMeta struct {
	ContentType string `json:"content_type"`
	Status      int    `json:"status"`
	Kind        string `json:"kind,omitempty"` // "asset" | "thumb"
	Key         string `json:"key,omitempty"`
}

type cdnFlight struct {
	done chan struct{}
}

var (
	cdnFlights  sync.Map
	cdnStoreMu  sync.Mutex
)

func pathNoQuery(path string) string {
	if i := strings.Index(path, "?"); i >= 0 {
		return path[:i]
	}
	return path
}

func isEnvatoThumbPath(path string) bool {
	lower := strings.ToLower(pathNoQuery(path))
	if !strings.HasPrefix(lower, "/__resized__/") {
		return false
	}
	for _, bad := range []string{
		"/files/", "download", "license",
		".zip", ".wav", ".mp3", ".m4a",
		".mp4", ".webm", ".m4v", ".mov", ".avi",
	} {
		if strings.Contains(lower, bad) {
			return false
		}
	}
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp", ".avif"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	// Resized CDN often uses extensionless image URLs — allow; store checks image/*.
	base := lower
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if base == "" || strings.Contains(base, ".") {
		return false
	}
	return true
}

// isEnvatoCDNPath — /assets-proxy static + /__resized__ thumbnails.
// Never video (__video__), downloads, or arbitrary __domain__ media.
func isEnvatoCDNPath(path string) bool {
	if isEnvatoThumbPath(path) {
		return true
	}
	lower := strings.ToLower(pathNoQuery(path))
	if !strings.HasPrefix(lower, "/assets-proxy/") {
		return false
	}
	for _, bad := range []string{
		"/files/", "download", "license",
		".zip", ".wav", ".mp3", ".m4a",
		".mp4", ".webm", ".m4v", ".mov", ".avi",
	} {
		if strings.Contains(lower, bad) {
			return false
		}
	}
	for _, ext := range []string{
		".js", ".css", ".woff", ".woff2", ".ttf", ".otf",
		".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp",
	} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func cdnCacheKind(key string) string {
	path := key
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	if isEnvatoThumbPath(path) {
		return "thumb"
	}
	return "asset"
}

func cdnCacheKey(r *http.Request) string {
	if r == nil || r.Method != http.MethodGet {
		return ""
	}
	if isEnvatoCDNPath(r.URL.Path) {
		return r.URL.RequestURI()
	}
	return ""
}

// cdnCacheKeyFromUpstream rebuilds the browser URI after Director strips prefixes.
func cdnCacheKeyFromUpstream(r *http.Request) string {
	if r == nil || r.Method != http.MethodGet {
		return ""
	}
	host := strings.ToLower(r.Host)
	if host == "" {
		host = strings.ToLower(r.URL.Host)
	}
	var prefix string
	switch host {
	case "assets.envato.com", "assets.elements.envato.com":
		prefix = "/assets-proxy"
	case "elements-resized.envatousercontent.com":
		prefix = "/__resized__"
	default:
		return ""
	}
	path := r.URL.Path
	if path == "" {
		path = "/"
	}
	fullPath := prefix + path
	if !isEnvatoCDNPath(fullPath) {
		return ""
	}
	if r.URL.RawQuery != "" {
		return fullPath + "?" + r.URL.RawQuery
	}
	return fullPath
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
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Del("Pragma")
	w.Header().Del("Expires")
	w.Header().Set("X-Proxy-Cache", "HIT")
	w.Header().Set("X-Envato-Cache", "disk")
	if meta.Status == 0 {
		meta.Status = http.StatusOK
	}
	w.WriteHeader(meta.Status)
	n, _ := io.Copy(w, file)
	log.Printf("[CDN_CACHE] HIT %s (%d bytes)", key, n)
	return true
}

func storeCDNCache(r *http.Request, status int, contentType string, body []byte) {
	key := cdnCacheKey(r)
	if key == "" {
		key = cdnCacheKeyFromUpstream(r)
	}
	storeCDNCacheKey(key, status, contentType, body)
}

type thumbEntry struct {
	bodyPath string
	metaPath string
	size     int64
	mod      time.Time
}

func listThumbEntries() (entries []thumbEntry, total int64) {
	dirEntries, err := os.ReadDir(cdnCacheDir)
	if err != nil {
		return nil, 0
	}
	for _, entry := range dirEntries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".meta") {
			continue
		}
		metaPath := filepath.Join(cdnCacheDir, name)
		metaBytes, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var meta cdnMeta
		if json.Unmarshal(metaBytes, &meta) != nil {
			continue
		}
		kind := meta.Kind
		if kind == "" && meta.Key != "" {
			kind = cdnCacheKind(meta.Key)
		}
		if kind != "thumb" {
			continue
		}
		bodyPath := strings.TrimSuffix(metaPath, ".meta")
		info, err := os.Stat(bodyPath)
		if err != nil {
			continue
		}
		entries = append(entries, thumbEntry{
			bodyPath: bodyPath,
			metaPath: metaPath,
			size:     info.Size(),
			mod:      info.ModTime(),
		})
		total += info.Size()
	}
	return entries, total
}

// enforceThumbBudget frees LRU thumb files until total+need <= cdnThumbBudget.
func enforceThumbBudget(need int) {
	entries, total := listThumbEntries()
	if total+int64(need) <= int64(cdnThumbBudget) {
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].mod.Before(entries[j].mod)
	})
	for _, e := range entries {
		if total+int64(need) <= int64(cdnThumbBudget) {
			break
		}
		_ = os.Remove(e.bodyPath)
		_ = os.Remove(e.metaPath)
		total -= e.size
		log.Printf("[CDN_CACHE] EVICT thumb %s (%d bytes) budget", filepath.Base(e.bodyPath), e.size)
	}
}

func storeCDNCacheKey(key string, status int, contentType string, body []byte) {
	if key == "" || status != http.StatusOK || len(body) == 0 {
		return
	}
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "text/html") || strings.Contains(ct, "json") ||
		strings.HasPrefix(ct, "video/") || strings.HasPrefix(ct, "audio/") {
		return
	}
	kind := cdnCacheKind(key)
	maxFile := cdnCacheMax
	if kind == "thumb" {
		maxFile = cdnThumbMaxFile
		if !strings.HasPrefix(ct, "image/") {
			return
		}
	}
	if len(body) > maxFile {
		return
	}

	cdnStoreMu.Lock()
	defer cdnStoreMu.Unlock()

	if err := os.MkdirAll(cdnCacheDir, 0o755); err != nil {
		return
	}
	if kind == "thumb" {
		enforceThumbBudget(len(body))
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
	meta, _ := json.Marshal(cdnMeta{
		ContentType: contentType,
		Status:      status,
		Kind:        kind,
		Key:         key,
	})
	_ = os.WriteFile(metaPath, meta, 0o644)
	log.Printf("[CDN_CACHE] STORE %s kind=%s (%d bytes)", key, kind, len(body))
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
	// Also keep thumb budget honest after TTL deletes of mixed files.
	cdnStoreMu.Lock()
	enforceThumbBudget(0)
	cdnStoreMu.Unlock()
}

func startCDNCacheSweep() {
	go func() {
		for {
			time.Sleep(30 * time.Minute)
			sweepCDNCache()
		}
	}()
}
