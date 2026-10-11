package main

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
)

const cdnCacheTTL = 72 * time.Hour

// cdnCacheDir is the on-disk store. Tests point it at a temp directory.
var cdnCacheDir = "cdn-cache"

type cdnMeta struct {
	ContentType string `json:"content_type"`
	Status      int    `json:"status"`
}

type cdnFlight struct {
	done chan struct{}
}

var (
	cdnFlights    sync.Map
	cdnCompressMu sync.Map
)

func cdnCacheKey(r *http.Request) string {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return ""
	}
	if strings.HasPrefix(r.URL.Path, "/cdn/") || staticCDNPath(r.URL.Path) {
		return r.URL.RequestURI()
	}
	return ""
}

// cdnCacheControl is the browser policy. Hashed Ahrefs assets (release id,
// ?version=, or chunk filename) stay in the browser for a year, so a new
// release URL is fetched once and every later open comes from memory.
// HTML is never passed here.
func cdnCacheControl(key string) string {
	if strings.Contains(key, "release-") || strings.Contains(key, "version=") || strings.Contains(key, "/chunk-") {
		return "public, max-age=31536000, immutable"
	}
	return "public, max-age=86400"
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

func cdnCompressible(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "javascript") ||
		strings.Contains(ct, "text/css") ||
		strings.Contains(ct, "svg") ||
		strings.Contains(ct, "json") ||
		strings.Contains(ct, "text/plain")
}

// cdnEncoding prefers brotli when the browser accepts both. Chrome sends gzip
// before br, so the scan cannot return on the first match. woff2 and images
// are already compressed and stay as stored.
func cdnEncoding(accept, contentType string) string {
	if !cdnCompressible(contentType) {
		return ""
	}
	gzipOK := false
	for _, part := range strings.Split(strings.ToLower(accept), ",") {
		token := strings.TrimSpace(part)
		if i := strings.Index(token, ";"); i >= 0 {
			token = strings.TrimSpace(token[:i])
		}
		switch token {
		case "br":
			return "br"
		case "gzip", "x-gzip":
			gzipOK = true
		}
	}
	if gzipOK {
		return "gzip"
	}
	return ""
}

func lockCDNPath(path string) func() {
	value, _ := cdnCompressMu.LoadOrStore(path, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func writeAtomic(path string, write func(io.Writer) error) error {
	tmp := path + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	werr := write(out)
	cerr := out.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmp)
		if werr != nil {
			return werr
		}
		return cerr
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// writeCompressedPair stores the brotli and gzip bodies and leaves no raw file.
// JS/CSS shrink to roughly a quarter of the plain text Ahrefs sends.
func writeCompressedPair(bodyPath string, body []byte) error {
	unlock := lockCDNPath(bodyPath)
	defer unlock()
	if err := writeAtomic(bodyPath+".br", func(dst io.Writer) error {
		zw := brotli.NewWriterLevel(dst, brotli.DefaultCompression)
		if _, err := zw.Write(body); err != nil {
			_ = zw.Close()
			return err
		}
		return zw.Close()
	}); err != nil {
		return err
	}
	if err := writeAtomic(bodyPath+".gz", func(dst io.Writer) error {
		zw, _ := gzip.NewWriterLevel(dst, gzip.DefaultCompression)
		if _, err := zw.Write(body); err != nil {
			_ = zw.Close()
			return err
		}
		return zw.Close()
	}); err != nil {
		_ = os.Remove(bodyPath + ".br")
		return err
	}
	_ = os.Remove(bodyPath)
	return nil
}

// compactCDNBody turns an older raw cache file into the compressed pair.
func compactCDNBody(bodyPath, contentType string) {
	if !cdnCompressible(contentType) {
		return
	}
	if _, err := os.Stat(bodyPath); err != nil {
		return
	}
	raw, err := os.ReadFile(bodyPath)
	if err != nil || len(raw) == 0 {
		return
	}
	_ = writeCompressedPair(bodyPath, raw)
}

func prepareCDNHeaders(h http.Header, key string) {
	// A cookie on a hashed JS file stops the browser and Cloudflare from
	// keeping the shared copy.
	h.Del("Set-Cookie")
	h.Del("Pragma")
	h.Del("Expires")
	h.Set("Cache-Control", cdnCacheControl(key))
	h.Set("X-Ahrefs-Cache", "disk")
	h.Set("Vary", "Accept-Encoding")
}

func cdnNotModified(r *http.Request, etag string) bool {
	inm := r.Header.Get("If-None-Match")
	if inm == "" {
		return false
	}
	for _, part := range strings.Split(inm, ",") {
		part = strings.TrimSpace(part)
		if part == "*" || part == etag {
			return true
		}
	}
	return false
}

func serveDiskCDN(w http.ResponseWriter, r *http.Request, key string) bool {
	bodyPath, metaPath := cdnFilePath(key)
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		return false
	}
	var meta cdnMeta
	if json.Unmarshal(metaBytes, &meta) != nil || meta.Status != http.StatusOK {
		return false
	}
	compactCDNBody(bodyPath, meta.ContentType)
	enc := cdnEncoding(r.Header.Get("Accept-Encoding"), meta.ContentType)
	servePath, enc := pickCDNFile(bodyPath, enc)
	if servePath == "" {
		return false
	}
	if enc == "" && servePath != bodyPath {
		return serveDecodedCDN(w, r, key, meta.ContentType, servePath)
	}
	file, err := os.Open(servePath)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false
	}
	if time.Since(info.ModTime()) > time.Hour {
		now := time.Now()
		_ = os.Chtimes(servePath, now, now)
		_ = os.Chtimes(metaPath, now, now)
		for _, extra := range []string{bodyPath + ".br", bodyPath + ".gz"} {
			if extra != servePath {
				_ = os.Chtimes(extra, now, now)
			}
		}
	}

	h := w.Header()
	prepareCDNHeaders(h, key)
	if meta.ContentType != "" {
		h.Set("Content-Type", meta.ContentType)
	}
	etag := `"` + filepath.Base(bodyPath)[:16]
	if enc != "" {
		h.Set("Content-Encoding", enc)
		etag += "-" + enc
	}
	etag += `"`
	h.Set("ETag", etag)
	if cdnNotModified(r, etag) {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	if r.Method == http.MethodHead {
		h.Set("Content-Length", strconv.FormatInt(info.Size(), 10))
		w.WriteHeader(http.StatusOK)
		return true
	}
	h.Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, file)
	return true
}

func storeCDNCache(r *http.Request, status int, contentType string, body []byte) {
	key := cdnCacheKey(r)
	if key == "" || status != http.StatusOK || len(body) == 0 || len(body) > 20<<20 {
		return
	}
	if err := os.MkdirAll(cdnCacheDir, 0o755); err != nil {
		return
	}
	bodyPath, metaPath := cdnFilePath(key)
	if cdnCompressible(contentType) {
		if err := writeCompressedPair(bodyPath, body); err != nil {
			return
		}
	} else if err := writeAtomic(bodyPath, func(dst io.Writer) error {
		_, err := dst.Write(body)
		return err
	}); err != nil {
		return
	}
	meta, _ := json.Marshal(cdnMeta{ContentType: contentType, Status: status})
	_ = os.WriteFile(metaPath, meta, 0o644)
}

// pickCDNFile chooses the compressed file already stored on disk.
// A raw path is only returned for images and fonts, or for cache files
// written before compressed storage.
func pickCDNFile(bodyPath, enc string) (string, string) {
	brPath := bodyPath + ".br"
	gzPath := bodyPath + ".gz"
	switch enc {
	case "br":
		if cdnFileExists(brPath) {
			return brPath, "br"
		}
		if cdnFileExists(gzPath) {
			return gzPath, "gzip"
		}
	case "gzip":
		if cdnFileExists(gzPath) {
			return gzPath, "gzip"
		}
		if cdnFileExists(brPath) {
			return brPath, "br"
		}
	}
	if cdnFileExists(bodyPath) {
		return bodyPath, ""
	}
	if cdnFileExists(brPath) {
		return brPath, ""
	}
	if cdnFileExists(gzPath) {
		return gzPath, ""
	}
	return "", ""
}

func cdnFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func serveDecodedCDN(w http.ResponseWriter, r *http.Request, key, contentType, path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	var reader io.Reader = file
	switch {
	case strings.HasSuffix(path, ".br"):
		reader = brotli.NewReader(file)
	case strings.HasSuffix(path, ".gz"):
		zr, zerr := gzip.NewReader(file)
		if zerr != nil {
			return false
		}
		defer zr.Close()
		reader = zr
	default:
		return false
	}
	plain, err := io.ReadAll(reader)
	if err != nil {
		return false
	}
	h := w.Header()
	prepareCDNHeaders(h, key)
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	h.Set("Content-Length", strconv.Itoa(len(plain)))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return true
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(plain)
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
