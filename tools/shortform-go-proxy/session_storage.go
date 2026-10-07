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

	// GoAuto sometimes stores localStorage values as objects. The strict
	// map[string]string decode then drops the whole blob, including auth_token.
	if ls := flexibleLocalStorage(raw); len(ls) > 0 {
		return "", ls
	}

	var bundle sessionBundleV2
	if json.Unmarshal([]byte(raw), &bundle) == nil && len(bundle.LocalStorage) > 0 {
		return cookiesFromRaw(bundle.Cookies), cloneStringMap(bundle.LocalStorage)
	}

	return "", nil
}

func flexibleLocalStorage(raw string) map[string]string {
	var root map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &root) != nil {
		return nil
	}
	var bag json.RawMessage
	if st, ok := root["storage"]; ok {
		var storage map[string]json.RawMessage
		if json.Unmarshal(st, &storage) == nil {
			bag = storage["localStorage"]
		}
	}
	if len(bag) == 0 {
		bag = root["local_storage"]
	}
	if len(bag) == 0 {
		bag = root["localStorage"]
	}
	if len(bag) == 0 {
		return nil
	}
	var loose map[string]json.RawMessage
	if json.Unmarshal(bag, &loose) != nil {
		return nil
	}
	out := make(map[string]string, len(loose))
	for k, v := range loose {
		s := strings.TrimSpace(string(v))
		if s == "" || s == "null" {
			continue
		}
		if len(s) >= 2 && s[0] == '"' {
			var str string
			if json.Unmarshal(v, &str) == nil {
				out[k] = str
				continue
			}
		}
		out[k] = s
	}
	return out
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
	// DigitaVision → ToolsMandi inside stored JSON strings (e.g. user.email)
	// Never rewrite auth_token — Basic-auth username; corruption → 401.
	out := make(map[string]string, len(ls))
	for k, v := range ls {
		if k == "auth_token" {
			out[k] = v
			continue
		}
		v = strings.ReplaceAll(v, "DigitaVision", "ToolsMandi")
		v = strings.ReplaceAll(v, "Digitavision", "ToolsMandi")
		v = strings.ReplaceAll(v, "digitavision", "ToolsMandi")
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
	if tok := strings.TrimSpace(ls["auth_token"]); tok != "" {
		return tok
	}
	// Last resort if the blob is truncated or not strict JSON.
	const key = `"auth_token"`
	i := strings.Index(sessionRaw, key)
	if i < 0 {
		return ""
	}
	rest := strings.TrimSpace(sessionRaw[i+len(key):])
	if !strings.HasPrefix(rest, ":") {
		return ""
	}
	rest = strings.TrimSpace(rest[1:])
	if !strings.HasPrefix(rest, `"`) {
		return ""
	}
	var tok string
	if err := json.Unmarshal([]byte(rest), &tok); err != nil {
		return ""
	}
	return strings.TrimSpace(tok)
}
