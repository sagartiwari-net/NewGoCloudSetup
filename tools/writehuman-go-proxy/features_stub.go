package main

import (
	"bytes"
	"net/http"
	"strings"
)

// Supabase feature tables power the SPA toast
// "Failed to fetch your feature access permissions."
func isSupabaseFeaturesPath(path string) bool {
	p := strings.ToLower(path)
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	return strings.Contains(p, "rest/v1/features")
}

func isSupabaseFeaturesAccessPath(path string) bool {
	p := strings.ToLower(path)
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	return strings.Contains(p, "rest/v1/features_access")
}

func stubGPTZeroFeaturesJSON(path string) []byte {
	if isSupabaseFeaturesAccessPath(path) {
		// Empty grants OK — generally_available rows from /features unlock the UI.
		return []byte(`[]`)
	}
	// Minimal catalog with generally_available so TMn() does not toast.
	return []byte(`[
{"id":"advanced_scan","name":"advanced_scan","generally_available":true},
{"id":"plagiarism","name":"plagiarism","generally_available":true},
{"id":"writing_feedback","name":"writing_feedback","generally_available":true},
{"id":"bibliography_scan","name":"bibliography_scan","generally_available":true},
{"id":"ai_vocab","name":"ai_vocab","generally_available":true},
{"id":"ai_grader","name":"ai_grader","generally_available":true}
]`)
}

func maybeStubGPTZeroFeatures(path string, status int, body []byte) (int, []byte, bool) {
	if !isSupabaseFeaturesPath(path) {
		return status, body, false
	}
	trim := bytes.TrimSpace(body)
	fail := status >= 400 || len(trim) == 0
	if !fail && len(trim) > 0 && trim[0] == '{' {
		// supabase error objects
		low := strings.ToLower(string(trim))
		if strings.Contains(low, `"message"`) && (strings.Contains(low, "jwt") ||
			strings.Contains(low, "permission") || strings.Contains(low, "unauthorized") ||
			strings.Contains(low, "not find")) {
			fail = true
		}
	}
	if !fail && isSupabaseFeaturesAccessPath(path) {
		return status, body, false
	}
	if !fail && !isSupabaseFeaturesAccessPath(path) {
		// Non-empty features array — keep upstream.
		if len(trim) > 1 && trim[0] == '[' && !(len(trim) == 2 && trim[1] == ']') {
			return status, body, false
		}
		fail = true
	}
	if !fail {
		return status, body, false
	}
	return http.StatusOK, stubGPTZeroFeaturesJSON(path), true
}
