package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const extensionTemplateDir = "extension"

// publicOriginFromRequest resolves the public origin/host this extension should
// talk to. Prefers config.json public_host/scheme, falls back to request host.
func publicOriginFromRequest(r *http.Request, cfg Config) (origin, host string) {
	host = strings.TrimSpace(cfg.PublicHost)
	if host == "" {
		host = r.Host
		if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
			host = strings.TrimSpace(strings.Split(fwd, ",")[0])
		}
	}
	scheme := strings.TrimSpace(cfg.PublicScheme)
	if scheme == "" {
		scheme = "https"
		if r.TLS == nil && !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "http"
		}
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	origin = scheme + "://" + host
	return origin, host
}

// bakeExtensionBytes builds a zip of the extension folder with host placeholders
// replaced for the given origin/host.
func bakeExtensionBytes(origin, host string) ([]byte, error) {
	hostNoPort := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostNoPort = h
	}
	hostEsc := strings.ReplaceAll(host, ".", `\.`)

	// externally_connectable match pattern cannot contain a port.
	match := "*://" + hostNoPort + "/*"

	replacer := strings.NewReplacer(
		"__SAS_PROXY_ORIGIN__", origin,
		"__SAS_PROXY_HOST_ESC__", hostEsc,
		"__SAS_PROXY_HOST__", host,
		"__SAS_PROXY_MATCH__", match,
	)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err := filepath.WalkDir(extensionTemplateDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(extensionTemplateDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, ".") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		ext := strings.ToLower(filepath.Ext(path))
		content := raw
		if ext == ".js" || ext == ".html" || ext == ".json" || ext == ".css" || ext == ".txt" {
			content = []byte(replacer.Replace(string(raw)))
		}
		w, err := zw.Create(rel)
		if err != nil {
			return err
		}
		_, err = w.Write(content)
		return err
	})
	if err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func extensionInstallPageHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Install SellerAmp SAS Extension</title>
<style>
*{box-sizing:border-box}body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;background:#f5f6f8;color:#20242a;font-family:Arial,sans-serif}
.card{width:100%;max-width:560px;padding:34px;background:#fff;border-radius:18px;box-shadow:0 16px 45px rgba(0,0,0,.09)}
.brand{color:#ef7622;font-size:14px;font-weight:700;margin-bottom:10px}h1{margin:0 0 10px;font-size:27px}.lead{color:#626b76;line-height:1.55}
.btn{display:block;margin:24px 0;padding:14px 20px;border-radius:10px;background:#ef7622;color:#fff;text-align:center;text-decoration:none;font-weight:700}.btn:hover{background:#d96010}
h2{font-size:15px;margin:24px 0 10px}ol{padding-left:22px;color:#626b76;line-height:1.75}code{padding:2px 6px;border-radius:5px;background:#eef0f3;color:#303740}
.note{font-size:12px;color:#8a929c;line-height:1.5}
</style>
</head>
<body><main class="card">
<div class="brand">SellerAmp SAS</div>
<h1>Install Chrome Extension</h1>
<p class="lead">Download the SellerAmp SAS extension and install it in Chrome.</p>
<a class="btn" href="/extension.zip">Download Extension (.zip)</a>
<h2>Installation steps</h2>
<ol>
<li>Download and unzip the file.</li>
<li>Open <code>chrome://extensions</code> in Chrome.</li>
<li>Enable <strong>Developer mode</strong>.</li>
<li>Click <strong>Load unpacked</strong> and select the unzipped folder.</li>
</ol>
<p class="note">Chrome requires manually loading custom extensions through Developer mode.</p>
</main></body></html>`)
}

func extensionZipHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	origin, host := publicOriginFromRequest(r, cfg)
	zipBytes, err := bakeExtensionBytes(origin, host)
	if err != nil {
		log.Printf("[EXT] bake failed host=%s: %v", host, err)
		http.Error(w, "Failed to build extension package", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="selleramp-sas-extension.zip"`)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipBytes)))
	w.Header().Set("Cache-Control", "no-store")
	log.Printf("[EXT] served extension.zip origin=%s (%d bytes)", origin, len(zipBytes))
	_, _ = w.Write(zipBytes)
}
