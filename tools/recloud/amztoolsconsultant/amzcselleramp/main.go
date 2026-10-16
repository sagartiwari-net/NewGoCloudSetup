package main

import (
	"log"
	"net/http"
	"strings"

	"recloud.dev/cdncache"
)

func main() {
	cfg := loadConfig()
	if usesPanelAccountMode(cfg) {
		ensureRecloudSite(cfg)
	}
	cdncache.ImmutablePrefixes = []string{"/__static__/", "/static-semrush-com/", "/assets/"}
	cdncache.StartSweep()
	mux := http.NewServeMux()
	mux.HandleFunc("/", handle)
	addr := ":" + strings.TrimSpace(cfg.Port)
	if addr == ":" {
		addr = ":6303"
	}
	log.Printf("[AMZCSELLERAMP] listening %s host=%s", addr, cfg.PublicHost)
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
	}
	if r.Method == http.MethodGet && (r.URL.Path == "/ext-install" || r.URL.Path == "/extension.zip") {
		if r.URL.Path == "/extension.zip" {
			extensionZipHandler(w, r)
			return
		}
		extensionInstallPageHandler(w, r)
		return
	}
	if r.URL.Path == "/api/user-limits" {
		userLimitsHandler(w, r)
		return
	}
	if r.Method == http.MethodGet && (r.URL.Path == "/" || r.URL.Path == "") && cfg.HomePath != "" && cfg.HomePath != "/" {
		http.Redirect(w, r, cfg.HomePath, http.StatusFound)
		return
	}
	if isHardLogoutPath(r.URL.Path) {
		switched, _ := noteLogout(cfg, r, acc, token, "logout")
		if switched && cfg.HomePath != "" {
			http.Redirect(w, r, cfg.HomePath, http.StatusFound)
			return
		}
		renderLoggedOutPage(w, cfg)
		return
	}
	proxyUpstream(w, r, cfg, acc)
}
