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
	// DigitaVision → ToolsMandi inside stored JSON strings (e.g. me.email)
	// Skip analytics / huge caches — they bloat the inject and can push
	// localhost Cookie headers over Chrome/Go limits (HTTP 431).
	out := make(map[string]string, len(ls))
	for k, v := range ls {
		if skipLocalStorageKey(k, v) {
			continue
		}
		v = strings.ReplaceAll(v, "DigitaVision", "ToolsMandi")
		v = strings.ReplaceAll(v, "Digitavision", "ToolsMandi")
		v = strings.ReplaceAll(v, "digitavision", "ToolsMandi")
		out[k] = v
	}
	// Always keep auth token if present
	if tok, ok := ls["token"]; ok && strings.TrimSpace(tok) != "" {
		out["token"] = tok
	}
	b, err := json.Marshal(out)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func skipLocalStorageKey(k, v string) bool {
	kl := strings.ToLower(k)
	// Always keep auth + minimal session flags
	switch k {
	case "token", "hasRegistered", "nextUrl", "signedInFrom", "__transferData", "whitelabelOtto",
		"isUserMailWithTestDomain", "isDocked", "aiAgentIframeVisible", "date", "compDate",
		"lastProperty", "lastPeriod", "lastPeriodSE", "onboardingData", "showUpdates", "countryCode":
		return false
	}
	if strings.HasPrefix(k, "onboarding") {
		return false
	}
	// Never overwrite this browser's device id with the account export.
	if k == "tm_device_fp" || k == "tm_device_proof" || strings.HasPrefix(kl, "tm_") {
		return true
	}
	// Drop analytics / caches / SDKs
	prefixes := []string{
		"ph_", "_mp", "__mpq", "_uet", "_gcl", "_grecaptcha", "countries_data",
		"intercom.", "userpilot", "_worker", "firebase:", "_vwo", "vwo", "bhpx_",
		"_li_", "li_", "t11ud", "wickedfu", "_fbp", "notifications",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(kl, p) || strings.Contains(kl, p) {
			return true
		}
	}
	// Hard size cap per value (~4KB) — token JWT is ~1KB
	if len(v) > 4096 && k != "token" {
		return true
	}
	return false
}

func sessionToken(sessionRaw string) string {
	_, ls := parseSessionStorage(sessionRaw)
	if ls == nil {
		return ""
	}
	return strings.TrimSpace(ls["token"])
}
