package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const noxKeepaliveInterval = 15 * time.Minute

func isNoxToolsTarget(cfg Config) bool {
	t := strings.ToLower(cfg.TargetURL + " " + cfg.ToolName + " " + cfg.PublicHost)
	return strings.Contains(t, "noxtools") || strings.Contains(t, "noxh.")
}

// warmNoxSession hits /secure/member on a timer so PHPSESSID stays alive overnight
// without a browser refresh extension. Works in both file and panel-account modes.
func warmNoxSession(cfg Config) {
	if !isNoxToolsTarget(cfg) {
		return
	}
	go func() {
		time.Sleep(5 * time.Second)
		for {
			noxKeepaliveOnce(cfg)
			time.Sleep(noxKeepaliveInterval)
		}
	}()
}

func noxKeepaliveOnce(cfg Config) {
	accID, rawCookie, ua := noxLoadWarmCookie(cfg)
	parsed := parseCookieFromDB(rawCookie)
	if strings.TrimSpace(parsed) == "" {
		log.Printf("[NOX-KEEP] no cookie available — skip")
		return
	}
	cookieHdr := applyLocalCookieOverlay(parsed)
	home := strings.TrimSpace(cfg.HomePath)
	if home == "" {
		home = "/secure/member"
	}
	warmURL := strings.TrimRight(cfg.TargetURL, "/") + home
	req, err := http.NewRequest(http.MethodGet, warmURL, nil)
	if err != nil {
		log.Printf("[NOX-KEEP] request build failed: %v", err)
		return
	}
	if ua == "" {
		ua = cfg.UserAgent
	}
	if ua == "" {
		ua = "Mozilla/5.0"
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cookie", cookieHdr)
	req.Header.Set("Referer", strings.TrimRight(cfg.TargetURL, "/")+"/")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[NOX-KEEP] warm failed: %v", err)
		return
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()

	changed := absorbUpstreamSetCookies(resp.Header, resp.StatusCode)
	loggedOut := noxLooksLoggedOut(resp.StatusCode, body, resp.Header.Get("Location"))
	if loggedOut {
		log.Printf("[NOX-KEEP] WARN possible logout status=%d url=%s", resp.StatusCode, warmURL)
	} else {
		log.Printf("[NOX-KEEP] ok status=%d changed=%v bytes=%d", resp.StatusCode, changed, len(body))
	}
	if changed {
		noxPersistOverlayCookies(cfg, accID, rawCookie)
	}
}

func noxLooksLoggedOut(status int, body []byte, location string) bool {
	loc := strings.ToLower(location)
	if strings.Contains(loc, "login") || strings.Contains(loc, "/member/login") {
		return true
	}
	if status >= 300 && status < 400 && (strings.Contains(loc, "login") || strings.Contains(loc, "signin")) {
		return true
	}
	low := strings.ToLower(string(body))
	if strings.Contains(low, "name=\"amember_login\"") || strings.Contains(low, "id=\"amember_login\"") {
		return true
	}
	if strings.Contains(low, "please login") && strings.Contains(low, "password") {
		return true
	}
	return false
}

func noxLoadWarmCookie(cfg Config) (accountID int, raw, ua string) {
	if usesPanelAccountMode(cfg) && dbConnected {
		if id, c, u, err := noxLoadFirstActiveAccount(cfg); err == nil && strings.TrimSpace(c) != "" {
			return id, c, u
		}
	}
	path := strings.TrimSpace(cfg.CookieFile)
	if path == "" {
		path = "cookie.txt"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, "", cfg.UserAgent
	}
	return 0, string(data), cfg.UserAgent
}

func noxLoadFirstActiveAccount(cfg Config) (id int, cookie, ua string, err error) {
	db, err := openPanelDB(cfg)
	if err != nil {
		return 0, "", "", err
	}
	err = db.QueryRow(`
		SELECT a.id, a.cookie, COALESCE(a.user_agent, '')
		FROM accounts a
		JOIN websites w ON w.id = a.website_id
		WHERE w.domain = ? AND a.status = 'active' AND a.cookie != ''
		ORDER BY a.id ASC LIMIT 1`, cfg.PublicHost).Scan(&id, &cookie, &ua)
	return id, cookie, ua, err
}

func noxPersistOverlayCookies(cfg Config, accountID int, rawCookie string) {
	merged := applyLocalCookieOverlay(parseCookieFromDB(rawCookie))
	if strings.TrimSpace(merged) == "" {
		return
	}
	// Rebuild a minimal Chrome-style cookie array for PHPSESSID + amember_nr.
	updated := noxCookieArrayFromHeader(merged)
	if path := strings.TrimSpace(cfg.CookieFile); path != "" {
		_ = os.WriteFile(path, []byte(updated+"\n"), 0644)
		cookieFileMu.Lock()
		cookieFileParsed = ""
		cookieFileMod = time.Time{}
		cookieFileMu.Unlock()
	}
	if accountID > 0 && usesPanelAccountMode(cfg) {
		if db, err := openPanelDB(cfg); err == nil {
			now := time.Now().UTC().Format(time.RFC3339)
			if _, err := db.Exec(`UPDATE accounts SET cookie=?, cookie_updated_at=? WHERE id=?`, updated, now, accountID); err != nil {
				_, _ = db.Exec(`UPDATE accounts SET cookie=? WHERE id=?`, updated, accountID)
			}
			log.Printf("[NOX-KEEP] persisted rotated cookies to account id=%d", accountID)
		}
	}
}

func noxCookieArrayFromHeader(header string) string {
	m := cookieHeaderToMap(header)
	type item struct {
		Domain   string `json:"domain"`
		HostOnly bool   `json:"hostOnly"`
		HTTPOnly bool   `json:"httpOnly"`
		Name     string `json:"name"`
		Path     string `json:"path"`
		Secure   bool   `json:"secure"`
		Session  bool   `json:"session"`
		Value    string `json:"value"`
	}
	out := make([]item, 0, 2)
	for _, name := range []string{"PHPSESSID", "amember_nr"} {
		if v := strings.TrimSpace(m[name]); v != "" {
			out = append(out, item{
				Domain:   ".noxtools.com",
				HTTPOnly: true,
				Name:     name,
				Path:     "/",
				Secure:   true,
				Session:  true,
				Value:    v,
			})
		}
	}
	if len(out) == 0 {
		return header
	}
	b, err := json.Marshal(out)
	if err != nil {
		return header
	}
	return string(b)
}
