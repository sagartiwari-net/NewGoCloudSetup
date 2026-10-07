package main

import (
	"encoding/json"
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

func localStorageJSONForBrowser(sessionRaw string) []byte {
	_, ls := parseSessionStorage(sessionRaw)
	if len(ls) == 0 {
		return []byte("{}")
	}
	// Keep token/me untouched — lowercase "digitavision" lives inside the real
	// account email and must not become "ToolsMandi" or auth/profile drift.
	out := make(map[string]string, len(ls))
	for k, v := range ls {
		lk := strings.ToLower(strings.TrimSpace(k))
		if lk != "token" && lk != "me" {
			v = strings.ReplaceAll(v, "DigitaVision", "ToolsMandi")
			v = strings.ReplaceAll(v, "Digitavision", "ToolsMandi")
		}
		out[k] = v
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
	return strings.TrimSpace(ls["token"])
}

// fbaTokenFromSession extracts the raw FbaToken value Angular sends on API calls.
// GoAuto stores token as either a plain string or {"_data":"..."}.
func fbaTokenFromSession(sessionRaw string) string {
	tok := sessionToken(sessionRaw)
	if tok == "" {
		return ""
	}
	tok = strings.TrimSpace(tok)
	if strings.HasPrefix(tok, "{") {
		var wrap struct {
			Data json.RawMessage `json:"_data"`
		}
		if json.Unmarshal([]byte(tok), &wrap) == nil && len(wrap.Data) > 0 {
			s := strings.TrimSpace(string(wrap.Data))
			if len(s) >= 2 && s[0] == '"' {
				var unquoted string
				if json.Unmarshal(wrap.Data, &unquoted) == nil {
					return strings.TrimSpace(unquoted)
				}
			}
			return strings.Trim(s, `"`)
		}
	}
	return tok
}
