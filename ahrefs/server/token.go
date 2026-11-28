package main

import (
	"encoding/json"
	"strings"
)

func extractAccessToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.HasPrefix(raw, "{") && !strings.HasPrefix(raw, "[") {
		// plain bearer
		return raw
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return ""
	}
	return digAccessToken(v)
}

func digAccessToken(v any) string {
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return ""
		}
		if strings.HasPrefix(s, "{") {
			var nested any
			if json.Unmarshal([]byte(s), &nested) == nil {
				return digAccessToken(nested)
			}
		}
		return s
	case map[string]any:
		for _, key := range []string{"accessToken", "access_token", "token"} {
			if s, ok := t[key].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
		for _, key := range []string{"ahToken", "ah_token", "activationData", "data", "ahFullJson"} {
			if nested, ok := t[key]; ok {
				if got := digAccessToken(nested); got != "" {
					return got
				}
			}
		}
	case []any:
		for _, item := range t {
			if got := digAccessToken(item); got != "" {
				return got
			}
		}
	}
	return ""
}
