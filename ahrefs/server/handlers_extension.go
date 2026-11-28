package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *server) extensionVersion() string {
	return envOr("AHREFS_BAR_EXT_VERSION", "1.0.1")
}

func (s *server) extensionUpdateURL() string {
	if v := strings.TrimSpace(os.Getenv("AHREFS_BAR_UPDATE_URL")); v != "" {
		return v
	}
	return envOr("AHREFS_BAR_PANEL_DOWNLOAD_URL", "http://127.0.0.1:3000/ahrefs-bar/download")
}

func (s *server) extensionDir() string {
	if v := strings.TrimSpace(os.Getenv("AHREFS_BAR_EXT_DIR")); v != "" {
		return v
	}
	candidates := []string{
		"../extension",
		"../../ahrefs/extension",
		"ahrefs/extension",
		"extension",
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}

func (s *server) publicAPIBase() string {
	if v := strings.TrimSpace(os.Getenv("AHREFS_BAR_PUBLIC_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://" + s.cfg.Addr
}

func (s *server) handleExtensionUpdate(w http.ResponseWriter, r *http.Request) {
	dir := s.extensionDir()
	base := s.publicAPIBase()
	writeJSON(w, 200, map[string]any{
		"success":      true,
		"version":      s.extensionVersion(),
		"build_id":     envOr("AHREFS_BAR_BUILD_ID", "dev-local"),
		"download_url": strings.TrimRight(base, "/") + "/v1/extension/download.zip",
		"panel_url":    s.extensionUpdateURL(),
		"extension_ok": dir != "",
		"notes":        "Download zip → unzip → chrome://extensions → Load unpacked (select ahrefs-bar folder).",
	})
}

func (s *server) handleExtensionDownload(w http.ResponseWriter, r *http.Request) {
	dir := s.extensionDir()
	if dir == "" {
		writeJSON(w, 500, map[string]any{"success": false, "error": "extension source missing (AHREFS_BAR_EXT_DIR)"})
		return
	}
	raw, err := zipExtensionDir(dir)
	if err != nil {
		writeJSON(w, 500, map[string]any{"success": false, "error": err.Error()})
		return
	}
	name := fmt.Sprintf("ahrefs-bar-%s.zip", s.extensionVersion())
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(raw)))
	w.Header().Set("X-TM-Bar-Ext-Version", s.extensionVersion())
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(raw)
}

func zipExtensionDir(srcDir string) ([]byte, error) {
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(srcDir, path)
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		base := filepath.Base(path)
		if base == ".DS_Store" || strings.HasPrefix(base, ".") || base == "_metadata" || strings.HasPrefix(rel, "_metadata/") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		w, err := zw.Create("ahrefs-bar/" + rel)
		if err != nil {
			return err
		}
		_, err = w.Write(body)
		return err
	})
	if err != nil {
		_ = zw.Close()
		return nil, err
	}
	readme := fmt.Sprintf("ToolsMandi Ahrefs Bar\nPacked: %s UTC\n\nUnzip → chrome://extensions → Developer mode → Load unpacked → select ahrefs-bar/\n",
		time.Now().UTC().Format(time.RFC3339))
	if rw, e := zw.Create("README.txt"); e == nil {
		_, _ = io.WriteString(rw, readme)
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	if buf.Len() < 100 {
		return nil, fmt.Errorf("zip too small")
	}
	return buf.Bytes(), nil
}
