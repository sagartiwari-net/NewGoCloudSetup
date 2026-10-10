package main

import (
	"log"
	"net/http"
	"regexp"
	"strings"
)

var digenCreditField = regexp.MustCompile(`(?i)"([a-zA-Z0-9_]*(?:credit|quota|balance|remain|point|cost|consume|usage)[a-zA-Z0-9_]*)"\s*:\s*(-?\d+(?:\.\d+)?)`)

func logDigenUse(resp *http.Response, body []byte) {
	if resp == nil || resp.Request == nil {
		return
	}
	req := resp.Request
	path := req.URL.Path
	if digenSkipUseLog(req.Method, path, resp.Header.Get("Content-Type")) {
		return
	}
	q := req.URL.RawQuery
	if digenQuietQuery(q) {
		q = ""
	}
	if q != "" {
		path += "?" + q
	}
	log.Printf("[USE] %s %s%s %d %dB %s", req.Method, req.Host, path, resp.StatusCode, len(body), digenCreditHint(body))
}

func digenSkipUseLog(method, path, contentType string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return false
	}
	lower := strings.ToLower(path)
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "image/") || strings.Contains(ct, "font/") || strings.Contains(ct, "video/") || strings.Contains(ct, "audio/") {
		return true
	}
	for _, part := range []string{"/_next/", "/assets/", "/static/", "/fonts/", "/favicon"} {
		if strings.Contains(lower, part) {
			return true
		}
	}
	for _, ext := range []string{".js", ".css", ".map", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".woff", ".woff2", ".ttf", ".mp4", ".webm", ".mp3", ".wasm"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func digenQuietQuery(q string) bool {
	if q == "" || len(q) > 180 {
		return true
	}
	lower := strings.ToLower(q)
	for _, word := range []string{"token", "cookie", "auth", "secret", "password"} {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

func digenCreditHint(body []byte) string {
	if len(body) == 0 || len(body) > 2<<20 {
		return "-"
	}
	matches := digenCreditField.FindAllSubmatch(body, 8)
	if len(matches) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		parts = append(parts, string(m[1])+"="+string(m[2]))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " ")
}
