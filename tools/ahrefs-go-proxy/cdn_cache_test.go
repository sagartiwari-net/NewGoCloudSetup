package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/andybalholm/brotli"
)

func TestVersionedAssetIsImmutableAndBrotli(t *testing.T) {
	dir := t.TempDir()
	prev := cdnCacheDir
	cdnCacheDir = dir
	t.Cleanup(func() { cdnCacheDir = prev })

	raw := bytes.Repeat([]byte("console.log('ahrefs-cache');"), 40)
	req := httptest.NewRequest(http.MethodGet, "/cdn/assets/esbuild/chunk-ABC-release-20261010-bk.js", nil)
	storeCDNCache(req, http.StatusOK, "application/javascript; charset=utf-8", raw)

	bodyPath, _ := cdnFilePath(req.URL.RequestURI())
	if _, err := os.Stat(bodyPath); !os.IsNotExist(err) {
		t.Fatal("raw JS file is still stored on disk")
	}
	brInfo, err := os.Stat(bodyPath + ".br")
	if err != nil {
		t.Fatal(err)
	}
	gzInfo, err := os.Stat(bodyPath + ".gz")
	if err != nil {
		t.Fatal(err)
	}
	if brInfo.Size() >= int64(len(raw)) || gzInfo.Size() >= int64(len(raw)) {
		t.Fatalf("compressed br=%d gz=%d raw=%d", brInfo.Size(), gzInfo.Size(), len(raw))
	}

	out := httptest.NewRecorder()
	gotReq := httptest.NewRequest(http.MethodGet, req.URL.RequestURI(), nil)
	gotReq.Header.Set("Accept-Encoding", "gzip, deflate, br")
	if !serveDiskCDN(out, gotReq, req.URL.RequestURI()) {
		t.Fatal("disk cache miss")
	}
	res := out.Result()
	if cc := res.Header.Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Fatalf("cache-control = %q", cc)
	}
	if res.Header.Get("Content-Encoding") != "br" {
		t.Fatalf("content-encoding = %q", res.Header.Get("Content-Encoding"))
	}
	if res.Header.Get("Set-Cookie") != "" {
		t.Fatal("static response kept a cookie")
	}
	decoded, err := io.ReadAll(brotli.NewReader(res.Body))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, raw) {
		t.Fatalf("decoded %d bytes, want %d", len(decoded), len(raw))
	}

	again := httptest.NewRecorder()
	reval := httptest.NewRequest(http.MethodGet, req.URL.RequestURI(), nil)
	reval.Header.Set("Accept-Encoding", "br")
	reval.Header.Set("If-None-Match", res.Header.Get("ETag"))
	if !serveDiskCDN(again, reval, req.URL.RequestURI()) {
		t.Fatal("revalidation miss")
	}
	if again.Code != http.StatusNotModified {
		t.Fatalf("status = %d", again.Code)
	}

	gzRec := httptest.NewRecorder()
	gzReq := httptest.NewRequest(http.MethodGet, req.URL.RequestURI(), nil)
	gzReq.Header.Set("Accept-Encoding", "gzip")
	if !serveDiskCDN(gzRec, gzReq, req.URL.RequestURI()) {
		t.Fatal("gzip miss")
	}
	if gzRec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("gzip encoding = %q", gzRec.Header().Get("Content-Encoding"))
	}
	zr, err := gzip.NewReader(gzRec.Body)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain, raw) {
		t.Fatal("gzip body mismatch")
	}
}

func TestUnversionedAssetKeepsShortCache(t *testing.T) {
	if got := cdnCacheControl("/cdn/assets/images/logo.svg"); got != "public, max-age=86400" {
		t.Fatalf("short cache = %q", got)
	}
	if got := cdnCacheControl("/assets/css/fonts.css?version=release-20261010"); !stringsContainsYear(got) {
		t.Fatalf("versioned query = %q", got)
	}
}

func stringsContainsYear(v string) bool {
	return v == "public, max-age=31536000, immutable"
}
