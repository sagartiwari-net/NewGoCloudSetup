// Package cdncache is the Ahrefs disk cache used by every recloud tool.
// JS and CSS are stored once as brotli and gzip. The browser keeps a hashed
// file for a year. HTML is never stored here.
package cdncache

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

const cacheTTL = 72 * time.Hour

// Dir is the on-disk store. Tests point it at a temp directory.
var Dir = "cdn-cache"

// ImmutablePrefixes are content-addressed asset roots, such as /__static__/.
// A file under one of these stays in the browser for a year.
var ImmutablePrefixes []string

type metaFile struct {
	ContentType string `json:"content_type"`
	Status      int    `json:"status"`
}

type flight struct {
	done chan struct{}
}

var (
	flights    sync.Map
	compressMu sync.Map
)

func RequestKey(r *http.Request, static bool) string {
	if !static || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		return ""
	}
	return r.URL.RequestURI()
}

func CacheControl(key string) string {
	lower := strings.ToLower(key)
	for _, prefix := range ImmutablePrefixes {
		prefix = strings.ToLower(strings.TrimSpace(prefix))
		if prefix != "" && strings.Contains(lower, prefix) {
			return "public, max-age=31536000, immutable"
		}
	}
	if strings.Contains(key, "release-") || strings.Contains(key, "version=") || strings.Contains(key, "/chunk-") {
		return "public, max-age=31536000, immutable"
	}
	return "public, max-age=86400"
}

func filePath(key string) (bodyPath, metaPath string) {
	sum := sha256.Sum256([]byte(key))
	name := hex.EncodeToString(sum[:])
	return filepath.Join(Dir, name), filepath.Join(Dir, name+".meta")
}

func joinFlight(key string) (leader bool, done <-chan struct{}) {
	created := &flight{done: make(chan struct{})}
	actual, loaded := flights.LoadOrStore(key, created)
	if loaded {
		return false, actual.(*flight).done
	}
	return true, created.done
}

func Complete(key string) {
	if key == "" {
		return
	}
	if value, ok := flights.LoadAndDelete(key); ok {
		close(value.(*flight).done)
	}
}

func Serve(w http.ResponseWriter, r *http.Request, key string) bool {
	if key == "" {
		return false
	}
	if ServeDisk(w, r, key) {
		return true
	}
	leader, done := joinFlight(key)
	if leader {
		return false
	}
	<-done
	return ServeDisk(w, r, key)
}

func compressible(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "javascript") ||
		strings.Contains(ct, "text/css") ||
		strings.Contains(ct, "svg") ||
		strings.Contains(ct, "json") ||
		strings.Contains(ct, "text/plain")
}

func encoding(accept, contentType string) string {
	if !compressible(contentType) {
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

func lockPath(path string) func() {
	value, _ := compressMu.LoadOrStore(path, &sync.Mutex{})
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

func writeCompressedPair(bodyPath string, body []byte) error {
	unlock := lockPath(bodyPath)
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

func compactBody(bodyPath, contentType string) {
	if !compressible(contentType) {
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

func prepareHeaders(h http.Header, key string) {
	h.Del("Set-Cookie")
	h.Del("Pragma")
	h.Del("Expires")
	h.Set("Cache-Control", CacheControl(key))
	h.Set("X-Recloud-Cache", "disk")
	h.Set("Vary", "Accept-Encoding")
}

func notModified(r *http.Request, etag string) bool {
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

func ServeDisk(w http.ResponseWriter, r *http.Request, key string) bool {
	if key == "" {
		return false
	}
	bodyPath, metaPath := filePath(key)
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		return false
	}
	var meta metaFile
	if json.Unmarshal(metaBytes, &meta) != nil || meta.Status != http.StatusOK {
		return false
	}
	if strings.Contains(strings.ToLower(meta.ContentType), "text/html") {
		return false
	}
	compactBody(bodyPath, meta.ContentType)
	enc := encoding(r.Header.Get("Accept-Encoding"), meta.ContentType)
	servePath, enc := pickFile(bodyPath, enc)
	if servePath == "" {
		return false
	}
	if enc == "" && servePath != bodyPath {
		return serveDecoded(w, r, key, meta.ContentType, servePath)
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
	prepareHeaders(h, key)
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
	if notModified(r, etag) {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	h.Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return true
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, file)
	return true
}

func Store(key string, status int, contentType string, body []byte) {
	if key == "" || status != http.StatusOK || len(body) == 0 || len(body) > 20<<20 {
		return
	}
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		return
	}
	if err := os.MkdirAll(Dir, 0o755); err != nil {
		return
	}
	bodyPath, metaPath := filePath(key)
	if compressible(contentType) {
		if err := writeCompressedPair(bodyPath, body); err != nil {
			return
		}
	} else if err := writeAtomic(bodyPath, func(dst io.Writer) error {
		_, err := dst.Write(body)
		return err
	}); err != nil {
		return
	}
	raw, _ := json.Marshal(metaFile{ContentType: contentType, Status: status})
	_ = os.WriteFile(metaPath, raw, 0o644)
}

func pickFile(bodyPath, enc string) (string, string) {
	brPath := bodyPath + ".br"
	gzPath := bodyPath + ".gz"
	switch enc {
	case "br":
		if fileExists(brPath) {
			return brPath, "br"
		}
		if fileExists(gzPath) {
			return gzPath, "gzip"
		}
	case "gzip":
		if fileExists(gzPath) {
			return gzPath, "gzip"
		}
		if fileExists(brPath) {
			return brPath, "br"
		}
	}
	if fileExists(bodyPath) {
		return bodyPath, ""
	}
	if fileExists(brPath) {
		return brPath, ""
	}
	if fileExists(gzPath) {
		return gzPath, ""
	}
	return "", ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func serveDecoded(w http.ResponseWriter, r *http.Request, key, contentType, path string) bool {
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
	prepareHeaders(h, key)
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

func sweep() {
	entries, err := os.ReadDir(Dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-cacheTTL)
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(Dir, entry.Name()))
	}
}

func StartSweep() {
	go func() {
		for {
			time.Sleep(30 * time.Minute)
			sweep()
		}
	}()
}
