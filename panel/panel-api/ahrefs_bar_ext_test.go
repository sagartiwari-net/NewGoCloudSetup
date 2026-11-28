package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildAhrefsExtensionZip(t *testing.T) {
	src := filepath.Join("..", "..", "..", "ahrefs", "extension")
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		t.Skip("extension dir not found from panel-api cwd")
	}
	raw, err := buildAhrefsExtensionZip(src, "https://barapi.upays.in", "ToolWaly", 2, []string{"https://members.toolwaly.com"})
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 1000 {
		t.Fatalf("zip too small: %d", len(raw))
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"tm-config.js":  false,
		"tm-rules.json": false,
		"manifest.json": false,
		"tm-guard.js":   false,
		"tm-fp.js":      false,
		"event.js":      false,
		"content.js":    false,
	}
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "ahrefs-bar/") || f.Name == "README.txt" {
			t.Fatalf("unexpected nested/readme entry: %s", f.Name)
		}
		if _, ok := want[f.Name]; ok {
			want[f.Name] = true
		}
		if f.Name == "tm-config.js" {
			rc, _ := f.Open()
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(rc)
			_ = rc.Close()
			if !bytes.Contains(buf.Bytes(), []byte("https://barapi.upays.in")) {
				t.Fatal("config missing api base")
			}
			if !bytes.Contains(buf.Bytes(), []byte("ToolWaly")) {
				t.Fatal("config missing brand")
			}
		}
	}
	for name, ok := range want {
		if !ok {
			t.Fatalf("missing %s", name)
		}
	}
}

func TestSanitizeFilePart(t *testing.T) {
	if sanitizeFilePart("Tool Waly!") != "Tool-Waly" && sanitizeFilePart("Tool Waly!") != "Tool-Waly-" {
		got := sanitizeFilePart("Tool Waly!")
		if got == "" {
			t.Fatal(got)
		}
	}
}
