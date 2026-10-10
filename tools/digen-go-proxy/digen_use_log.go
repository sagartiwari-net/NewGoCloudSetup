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
	if digenSkipUseLog(req.Method, path) {
		return
	}
	q := req.URL.RawQuery
	if digenQuietQuery(q) {
		q = ""
	}
	if q != "" {
		path += "?" + q
	}
	log.Printf("[USE] %s %s%s %d %s", req.Method, req.Host, path, resp.StatusCode, digenCreditHint(path, body))
}

func digenSkipUseLog(method, path string) bool {
	lower := strings.ToLower(path)
	if strings.Contains(lower, "/credit/") {
		return false
	}
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		if strings.Contains(lower, "cdn-cgi/") {
			return true
		}
		return false
	}
	return true
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

func digenCreditHint(path string, body []byte) string {
	if len(body) == 0 || len(body) > 64*1024 {
		return "-"
	}
	if !strings.Contains(strings.ToLower(path), "/credit/") && len(body) > 4096 {
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
