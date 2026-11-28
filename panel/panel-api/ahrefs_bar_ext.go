package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func ahrefsExtDir() string {
	if v := strings.TrimSpace(os.Getenv("AHREFS_BAR_EXT_DIR")); v != "" {
		return v
	}
	candidates := []string{
		"../../../ahrefs/extension",
		"../../ahrefs/extension",
		"../ahrefs/extension",
		"ahrefs/extension",
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}

func ahrefsTemplateDir() string {
	if v := strings.TrimSpace(os.Getenv("AHREFS_BAR_TEMPLATE_DIR")); v != "" {
		return v
	}
	candidates := []string{
		"../../../ahrefs/templates",
		"../../ahrefs/templates",
		"../ahrefs/templates",
		"ahrefs/templates",
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}

func publicBarAPIBase() string {
	if v := strings.TrimSpace(os.Getenv("AHREFS_BAR_PUBLIC_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://barapi.upays.in"
}

func (s *server) downloadAhrefsBarExtension(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	resellerID := op.ID
	if op.Role == "master" {
		if n, err := strconv.Atoi(r.URL.Query().Get("resellerId")); err == nil && n > 0 {
			resellerID = n
		}
	}
	src := ahrefsExtDir()
	if src == "" {
		writeErr(w, 500, "Extension source not found (set AHREFS_BAR_EXT_DIR)")
		return
	}
	apiBase := publicBarAPIBase()
	brand := "ToolsMandi"
	var uname string
	_ = s.db.QueryRow(`SELECT username FROM operators WHERE id=?`, resellerID).Scan(&uname)
	if uname != "" {
		brand = uname
	}

	portals := s.ahrefsPortalURLs(resellerID)
	zipBytes, err := buildAhrefsExtensionZip(src, apiBase, brand, resellerID, portals)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	name := fmt.Sprintf("ahrefs-bar-%s-%d.zip", sanitizeFilePart(brand), resellerID)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(zipBytes)))
	_, _ = w.Write(zipBytes)
}

func (s *server) ahrefsPortalURLs(resellerID int) []string {
	rows, err := s.db.Query(`SELECT site_url FROM ahrefs_bar_portals WHERE reseller_id=? AND enabled=1`, resellerID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if rows.Scan(&u) == nil && strings.TrimSpace(u) != "" {
			out = append(out, strings.TrimSpace(u))
		}
	}
	return out
}

func (s *server) downloadAhrefsBarTMCheck(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authAhrefsBar(w, r); !ok {
		return
	}
	dir := ahrefsTemplateDir()
	if dir == "" {
		writeErr(w, 500, "Templates not found (set AHREFS_BAR_TEMPLATE_DIR)")
		return
	}
	path := filepath.Join(dir, "tm-check.php")
	raw, err := os.ReadFile(path)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/x-php")
	w.Header().Set("Content-Disposition", `attachment; filename="tm-check.php"`)
	_, _ = w.Write(raw)
}

func (s *server) ahrefsBarExtInfo(w http.ResponseWriter, r *http.Request) {
	op, ok := s.authAhrefsBar(w, r)
	if !ok {
		return
	}
	resellerID := op.ID
	if op.Role == "master" {
		if n, err := strconv.Atoi(r.URL.Query().Get("resellerId")); err == nil && n > 0 {
			resellerID = n
		}
	}
	src := ahrefsExtDir()
	writeJSON(w, 200, map[string]any{
		"extension_dir": src,
		"extension_ok":  src != "",
		"template_dir":  ahrefsTemplateDir(),
		"template_ok":   ahrefsTemplateDir() != "",
		"public_api":    publicBarAPIBase(),
		"reseller_id":   resellerID,
		"portals":       s.ahrefsPortalURLs(resellerID),
	})
}

func buildAhrefsExtensionZip(srcDir, apiBase, brand string, resellerID int, portals []string) ([]byte, error) {
	apiBase = strings.TrimRight(strings.TrimSpace(apiBase), "/")
	u, err := url.Parse(apiBase)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("invalid api base")
	}
	proxyFilter := "||" + u.Host + "/v4/"
	buildID := fmt.Sprintf("r%d-%s", resellerID, time.Now().UTC().Format("20060102-1504"))

	panelURL := strings.TrimSpace(os.Getenv("AHREFS_BAR_PANEL_DOWNLOAD_URL"))
	if panelURL == "" {
		panelURL = "https://panel.gt4rents.com/ahrefs-bar/download"
	}
	updateURL := strings.TrimRight(apiBase, "/") + "/v1/extension/download.zip"
	extVer := "1.0.1"
	if rawMan, err := os.ReadFile(filepath.Join(srcDir, "manifest.json")); err == nil {
		var m struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(rawMan, &m) == nil && strings.TrimSpace(m.Version) != "" {
			extVer = strings.TrimSpace(m.Version)
		}
	}
	configJS := fmt.Sprintf(`/*
 * tm-config.js — stamped build %s
 */
self.TM_BAR_CONFIG = {
  apiBase: %s,
  proxyHostFilter: %s,
  brandName: %s,
  resellerId: %d,
  buildId: %s,
  version: %s,
  panelUrl: %s,
  updateUrl: %s,
};
`,
		buildID,
		strconv.Quote(apiBase),
		strconv.Quote(proxyFilter),
		strconv.Quote(brand),
		resellerID,
		strconv.Quote(buildID),
		strconv.Quote(extVer),
		strconv.Quote(panelURL),
		strconv.Quote(updateURL),
	)

	// Chrome blocks DNR redirects from https://ahrefs.com → http://127.0.0.1.
	// Local/http API bases rely on tm-guard fetch rewrite instead.
	var rules []map[string]any
	if strings.HasPrefix(strings.ToLower(apiBase), "https://") {
		rules = []map[string]any{{
			"id":       1,
			"priority": 1,
			"action": map[string]any{
				"type": "redirect",
				"redirect": map[string]any{
					"regexSubstitution": apiBase + "/v4/\\1",
				},
			},
			"condition": map[string]any{
				"regexFilter":   `^https://ahrefs\.com/v4/(.*)`,
				"resourceTypes": []string{"xmlhttprequest", "other", "main_frame", "sub_frame"},
			},
		}}
	}
	if rules == nil {
		rules = []map[string]any{}
	}
	rulesJSON, _ := json.MarshalIndent(rules, "", "  ")

	manifestRaw, err := os.ReadFile(filepath.Join(srcDir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return nil, err
	}
	manifest["name"] = brand + " Ahrefs Bar"
	manifest["description"] = brand + " Ahrefs toolbar proxy (FP-bound)"
	hosts := []any{
		apiBase + "/*",
		"https://ahrefs.com/*",
		"https://app.ahrefs.com/*",
	}
	seen := map[string]bool{apiBase + "/*": true}
	for _, p := range portals {
		origin := portalOrigin(p)
		if origin == "" || seen[origin+"/*"] {
			continue
		}
		seen[origin+"/*"] = true
		hosts = append(hosts, origin+"/*")
	}
	// keep common reseller sites from base manifest if present
	if prev, ok := manifest["host_permissions"].([]any); ok {
		for _, h := range prev {
			hs, _ := h.(string)
			if hs == "" || seen[hs] {
				continue
			}
			if strings.Contains(hs, "127.0.0.1") || strings.Contains(hs, "localhost") {
				continue
			}
			seen[hs] = true
			hosts = append(hosts, hs)
		}
	}
	manifest["host_permissions"] = hosts
	manifestJSON, _ := json.MarshalIndent(manifest, "", "  ")

	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	skip := map[string]bool{".DS_Store": true}
	err = filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(srcDir, path)
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		base := filepath.Base(path)
		if skip[base] || strings.HasPrefix(base, ".") || base == "_metadata" || strings.HasPrefix(rel, "_metadata/") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		var body []byte
		switch rel {
		case "tm-config.js":
			body = []byte(configJS)
		case "tm-rules.json":
			body = rulesJSON
		case "manifest.json":
			body = manifestJSON
		default:
			body, err = os.ReadFile(path)
			if err != nil {
				return err
			}
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
	readme := fmt.Sprintf("ToolsMandi Ahrefs Bar — %s (reseller %d)\nAPI: %s\nBuild: %s\n\nLoad unpacked: chrome://extensions → Developer mode → Load unpacked → select ahrefs-bar folder (or unzip first).\n",
		brand, resellerID, apiBase, buildID)
	rw, _ := zw.Create("README.txt")
	_, _ = io.WriteString(rw, readme)
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func portalOrigin(site string) string {
	u, err := url.Parse(strings.TrimSpace(site))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func sanitizeFilePart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "reseller"
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "reseller"
	}
	return out
}
