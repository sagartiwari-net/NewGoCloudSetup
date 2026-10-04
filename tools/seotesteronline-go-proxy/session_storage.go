package main

import (
	"encoding/json"
	"regexp"
	"strings"
)

type goAutoSessionExport struct {
	Referer         string          `json:"referer"`
	IncludedFormats []string        `json:"includedFormats"`
	Cookies         json.RawMessage `json:"cookies"`
	Storage         struct {
		LocalStorage map[string]string `json:"localStorage"`
		Cookies      json.RawMessage   `json:"cookies"`
	} `json:"storage"`
}

type sessionBundleV2 struct {
	V            int               `json:"v"`
	LocalStorage map[string]string `json:"local_storage,omitempty"`
	Cookies      json.RawMessage   `json:"cookies,omitempty"`
}

func parseSessionStorage(raw string) (cookieHeader string, localStorage map[string]string) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" || raw == "[]" {
		return "", nil
	}

	var goAuto goAutoSessionExport
	if json.Unmarshal([]byte(raw), &goAuto) == nil {
		if len(goAuto.Storage.LocalStorage) > 0 {
			return cookiesFromRaw(goAuto.Storage.Cookies), cloneStringMap(goAuto.Storage.LocalStorage)
		}
		if h := cookiesFromRaw(goAuto.Cookies); h != "" {
			return h, nil
		}
		if h := cookiesFromRaw(goAuto.Storage.Cookies); h != "" {
			return h, cloneStringMap(goAuto.Storage.LocalStorage)
		}
	}

	var bundle sessionBundleV2
	if json.Unmarshal([]byte(raw), &bundle) == nil && len(bundle.LocalStorage) > 0 {
		return cookiesFromRaw(bundle.Cookies), cloneStringMap(bundle.LocalStorage)
	}

	return "", nil
}

func cookiesFromRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "[]" || s == "null" {
		return ""
	}
	return parseCookieHeader(s)
}

func parseCookieHeader(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "[") {
		var items []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}
		if json.Unmarshal([]byte(raw), &items) == nil {
			parts := make([]string, 0, len(items))
			for _, it := range items {
				if it.Name != "" {
					parts = append(parts, it.Name+"="+it.Value)
				}
			}
			return strings.Join(parts, "; ")
		}
	}
	return raw
}

func cloneStringMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func neverRewriteValueKey(k string) bool {
	// Auth / identity blobs — naive DigitaVision→ToolsMandi corrupts emails
	// like digitavision220926@… and breaks ngStorage / API identity checks.
	switch k {
	case "accessToken", "session", "userInfo", "ngStorage-currentUser":
		return true
	default:
		return false
	}
}

// brandWordRes matches standalone DigitaVision brand tokens only (not email local-parts).
var brandWordRes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bDigitaVision\b`),
}

func brandReplaceSafe(s string) string {
	for _, re := range brandWordRes {
		s = re.ReplaceAllString(s, "ToolsMandi")
	}
	return s
}

func localStorageJSONForBrowser(sessionRaw string) []byte {
	_, ls := parseSessionStorage(sessionRaw)
	if len(ls) == 0 {
		return []byte("{}")
	}
	out := make(map[string]string, len(ls))
	for k, v := range ls {
		if neverRewriteValueKey(k) {
			out[k] = v
			continue
		}
		out[k] = brandReplaceSafe(v)
	}
	// Always restore critical auth keys from source (never brand-rewritten)
	for _, k := range []string{"accessToken", "session", "userInfo", "ngStorage-currentUser"} {
		if tok, ok := ls[k]; ok && strings.TrimSpace(tok) != "" {
			out[k] = tok
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func sessionToken(sessionRaw string) string {
	_, ls := parseSessionStorage(sessionRaw)
	if ls == nil {
		return ""
	}
	tok := strings.TrimSpace(ls["accessToken"])
	tok = strings.TrimPrefix(tok, "Bearer ")
	tok = strings.TrimPrefix(tok, "bearer ")
	return strings.TrimSpace(tok)
}
