package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
)

// logVeoUse records one line for a call that can spend credits.
// Prompts, cookies, tokens, and emails are left out.
func logVeoUse(method, path string, status int, user string, reqBody, respBody []byte) {
	path = veoLogPath(path)
	req := veoUsageSummary(reqBody)
	resp := veoUsageSummary(respBody)
	if !shouldLogVeoUse(method, path, resp) {
		return
	}
	if user == "" {
		user = "-"
	}
	log.Printf("[USE] user=%s %s %s status=%d req%s resp%s", user, method, path, status, brace(req), brace(resp))
}

func veoSessionUser(r *http.Request) string {
	_, sess, ok := sessionFromRequest(r)
	if !ok || sess == nil {
		return ""
	}
	sess.mu.Lock()
	name := sess.username
	sess.mu.Unlock()
	return strings.TrimSpace(name)
}

func snapVeoBody(r *http.Request) []byte {
	if r == nil || r.Body == nil || !veoWriteMethod(r.Method) {
		return nil
	}
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	if !strings.Contains(ct, "json") && !strings.Contains(ct, "text") {
		return nil
	}
	const max = 256 << 10
	raw, err := io.ReadAll(io.LimitReader(r.Body, max+1))
	if err != nil {
		return nil
	}
	rest := r.Body
	if len(raw) > max {
		r.Body = &bodyRest{Reader: io.MultiReader(bytes.NewReader(raw), rest), Closer: rest}
		return nil
	}
	r.Body = &bodyRest{Reader: bytes.NewReader(raw), Closer: rest}
	return raw
}

type bodyRest struct {
	io.Reader
	io.Closer
}

func veoWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func shouldLogVeoUse(method, path, respSummary string) bool {
	if veoStaticPath(path) || strings.HasPrefix(path, "/_next/") || strings.Contains(path, "/cdn-cgi/") {
		return false
	}
	if veoWriteMethod(method) {
		return true
	}
	if veoCreditPath(path) {
		return true
	}
	return respSummary != ""
}

func veoLogPath(path string) string {
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func veoStaticPath(path string) bool {
	p := strings.ToLower(veoLogPath(path))
	for _, ext := range []string{".js", ".css", ".map", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".woff", ".woff2", ".mp4", ".webm", ".m3u8"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

func veoCreditPath(path string) bool {
	p := strings.ToLower(path)
	for _, part := range []string{"credit", "quota", "usage", "billing", "balance", "subscription", "plan"} {
		if strings.Contains(p, part) {
			return true
		}
	}
	return false
}

func brace(s string) string {
	if s == "" {
		return "{}"
	}
	return "{" + s + "}"
}

func veoUsageSummary(body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) == 0 || (body[0] != '{' && body[0] != '[') {
		return ""
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return ""
	}
	var parts []string
	veoWalkUsage(v, "", 0, &parts)
	return strings.Join(parts, " ")
}

func veoWalkUsage(v any, key string, depth int, parts *[]string) {
	if depth > 6 || len(*parts) >= 16 {
		return
	}
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			next := k
			if key != "" {
				next = key + "." + k
			}
			veoWalkUsage(child, next, depth+1, parts)
		}
	case []any:
		if len(t) > 6 {
			t = t[:6]
		}
		for _, child := range t {
			veoWalkUsage(child, key, depth+1, parts)
		}
	case string:
		if veoLabelKey(key) && !veoSecretKey(key) && len(*parts) < 16 {
			s := strings.TrimSpace(t)
			if s == "" || len(s) > 40 || strings.Contains(s, " ") {
				return
			}
			*parts = append(*parts, key+"="+s)
		}
	case json.Number:
		if veoNumberKey(key) && !veoSecretKey(key) && len(*parts) < 16 {
			*parts = append(*parts, key+"="+t.String())
		}
	}
}

func veoSecretKey(key string) bool {
	k := strings.ToLower(key)
	for _, part := range []string{"token", "cookie", "secret", "password", "authorization", "email", "prompt", "jwt", "phone", "session"} {
		if strings.Contains(k, part) {
			return true
		}
	}
	return false
}

func veoNumberKey(key string) bool {
	k := strings.ToLower(key)
	for _, part := range []string{"credit", "quota", "usage", "remain", "balance", "cost", "point", "consume", "limit", "duration", "second"} {
		if strings.Contains(k, part) {
			return true
		}
	}
	return false
}

func veoLabelKey(key string) bool {
	k := strings.ToLower(key)
	if i := strings.LastIndex(k, "."); i >= 0 {
		k = k[i+1:]
	}
	switch k {
	case "model", "duration", "resolution", "ratio", "quality", "mode", "type":
		return true
	default:
		return false
	}
}
