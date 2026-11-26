package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"recloud.dev/cdncache"
)

func loadLocalCookieAccount() ToolAccount {
	body, err := os.ReadFile("cookie.json")
	if err != nil || len(strings.TrimSpace(string(body))) == 0 {
		log.Printf("[LOCAL] cookie.json missing/empty: %v", err)
		return ToolAccount{ID: 1, Name: "Local", Cookie: ""}
	}
	return ToolAccount{
		ID:     1,
		Name:   "Local",
		Cookie: parseCookieFromDB(string(body)),
	}
}

func main() {
	cfg := loadConfig()
	if usesPanelAccountMode(cfg) {
		ensureRecloudSite(cfg)
	}
	cdncache.ImmutablePrefixes = []string{"/__static__/", "/css/", "/js/", "/images/"}
	cdncache.StartSweep()
	mux := http.NewServeMux()
	mux.HandleFunc("/", handle)
	addr := ":" + strings.TrimSpace(cfg.Port)
	if addr == ":" {
		addr = ":6508"
	}
	log.Printf("[RYKNS] listening %s host=%s target=%s", addr, cfg.PublicHost, cfg.TargetURL)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func handle(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	switch r.URL.Path {
	case "/access":
		if usesPanelAccountMode(cfg) {
			servePanelAccess(w, r, cfg)
			return
		}
	case "/api/device-bind":
		deviceBindHandler(w, r)
		return
	case "/tm-device-sw.js":
		serveDeviceSW(w, r)
		return
	case "/__tm_access_denied":
		renderAccessDeniedPage(w, cfg)
		return
	}
	if usesPanelAccountMode(cfg) {
		if _, err := panelSessionUsername(r); err != nil {
			if isDocumentNavigation(r) || strings.Contains(r.Header.Get("Accept"), "text/html") {
				renderAccessDeniedPage(w, cfg)
			} else {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"access_denied"}`))
			}
			return
		}
		if rejectPanelDevice(w, r, cfg) {
			return
		}
	}
	token := sessionTokenFromRequest(r)
	var acc ToolAccount
	if usesPanelAccountMode(cfg) {
		loaded, err := loadPanelSessionAccount(cfg, token)
		if err != nil {
			log.Printf("[PANEL] account load failed: %v", err)
			renderNoActiveAccountsPage(w, cfg)
			return
		}
		acc = loaded
	} else {
		acc = loadLocalCookieAccount()
	}
	// Bounce "/" → home_path (analytics overview). Loop guard in homeRedirectLoop
	// stops analytics → / when the upstream session is dead.
	if r.Method == http.MethodGet && (r.URL.Path == "/" || r.URL.Path == "") {
		home := strings.TrimSpace(cfg.HomePath)
		if home != "" && home != "/" {
			http.Redirect(w, r, home, http.StatusFound)
			return
		}
	}
	if isLogoutPath(r.URL.Path) {
		switched, _ := noteLogout(cfg, r, acc, token, "logout")
		if switched && cfg.HomePath != "" {
			http.Redirect(w, r, cfg.HomePath, http.StatusFound)
			return
		}
		renderLoggedOutPage(w, cfg)
		return
	}
	// Browser already mid return_url bounce (ERR_TOO_MANY_REDIRECTS in progress).
	if isDocumentNavigation(r) && isRankyReturnURLBounce(r, "", publicOrigin(r, cfg)) {
		_, _ = noteLogout(cfg, r, acc, token, "session_expired")
		renderLoggedOutPage(w, cfg)
		return
	}
	proxyUpstream(w, r, cfg, acc)
}
