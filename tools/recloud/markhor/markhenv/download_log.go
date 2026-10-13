package main

import (
	"log"
	"net/http"
	"net/url"
	"strings"
	"unicode"
)

// Temporary download probe. Remove this file and the noteDownloadProbe call
// after the Envato limit check is done.

func noteDownloadProbe(r *http.Request, status int, header http.Header, body []byte) {
	if r == nil {
		return
	}
	path := r.URL.Path
	disposition := ""
	contentType := ""
	if header != nil {
		disposition = header.Get("Content-Disposition")
		contentType = header.Get("Content-Type")
	}
	if !looksLikeDownload(path, disposition) && !downloadLimitSignal(contentType, body) {
		return
	}
	limited, snippet := downloadLimitDetail(contentType, body)
	log.Printf("[DOWNLOAD] method=%s path=%s query=%s status=%d type=%s bytes=%d file=%q limit=%t snippet=%q",
		r.Method, path, safeQuery(r.URL.Query()), status, contentType, len(body), downloadFileLabel(disposition), limited, snippet)
}

func looksLikeDownload(path, disposition string) bool {
	p := strings.ToLower(path)
	d := strings.ToLower(disposition)
	return strings.Contains(p, "download") || strings.Contains(d, "attachment") || strings.Contains(d, "filename=")
}

func downloadLimitSignal(contentType string, body []byte) bool {
	limited, _ := downloadLimitDetail(contentType, body)
	return limited
}

func downloadLimitDetail(contentType string, body []byte) (bool, string) {
	if !textLike(contentType) || len(body) == 0 || len(body) > 256<<10 {
		return false, ""
	}
	text := strings.ToLower(string(body))
	phrases := []string{
		"download limit",
		"fair use",
		"quota",
		"too many download",
		"limit reached",
		"daily limit",
		"no downloads left",
		"out of downloads",
		"you have reached",
		"you've reached",
		"download exceeded",
	}
	for _, phrase := range phrases {
		if i := strings.Index(text, phrase); i >= 0 {
			return true, clipSnippet(string(body), i)
		}
	}
	return false, ""
}

func textLike(contentType string) bool {
	ct := strings.ToLower(contentType)
	if ct == "" {
		return true
	}
	return strings.Contains(ct, "text/") ||
		strings.Contains(ct, "json") ||
		strings.Contains(ct, "xml") ||
		strings.Contains(ct, "javascript")
}

func clipSnippet(body string, at int) string {
	start := at - 40
	if start < 0 {
		start = 0
	}
	end := at + 120
	if end > len(body) {
		end = len(body)
	}
	snippet := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, body[start:end])
	return strings.TrimSpace(snippet)
}

func downloadFileLabel(disposition string) string {
	lower := strings.ToLower(disposition)
	idx := strings.Index(lower, "filename=")
	if idx < 0 {
		return ""
	}
	raw := strings.TrimSpace(disposition[idx+len("filename="):])
	if cut := strings.Index(raw, ";"); cut >= 0 {
		raw = raw[:cut]
	}
	return strings.Trim(raw, `"'`)
}

func safeQuery(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	drop := map[string]bool{
		"token": true, "signature": true, "sig": true, "expires": true, "auth": true, "key": true,
	}
	var parts []string
	for key, vals := range values {
		if drop[strings.ToLower(key)] {
			parts = append(parts, key+"=(hidden)")
			continue
		}
		for _, val := range vals {
			if len(val) > 80 {
				val = val[:80] + "…"
			}
			parts = append(parts, key+"="+val)
		}
	}
	return strings.Join(parts, "&")
}
