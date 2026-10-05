package main

import (
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

func magnificLooksLoggedOut(path string, body []byte, status int, location string) (bool, string) {
	p := strings.ToLower(path)
	loc := strings.ToLower(strings.TrimSpace(location))
	for _, m := range []string{"/login", "/sign-in", "/signin", "/sign-up", "/signup", "/auth/login"} {
		if strings.Contains(p, m) || strings.Contains(loc, m) {
			return true, "url:" + m
		}
	}
	if status == 401 || status == 403 {
		return true, fmt.Sprintf("status_%d", status)
	}
	if len(body) == 0 {
		return false, ""
	}
	low := strings.ToLower(string(body))
	if len(low) > 14000 {
		low = low[:14000]
	}
	hits := 0
	hit := ""
	for _, mk := range []string{
		`sign in`,
		`log in`,
		`create an account`,
		`continue with google`,
	} {
		if strings.Contains(low, mk) {
			hits++
			if hit == "" {
				hit = mk
			}
		}
	}
	if hits >= 2 {
		return true, "html:" + hit
	}
	return false, ""
}

func renderMagnificContactAdmin(w http.ResponseWriter, cfg Config, reason string) {
	writeLightCard(w, http.StatusServiceUnavailable, lightCard{
		Title:   "Account logged out",
		Heading: "Account logged out",
		Message: "Account logged out (" + html.EscapeString(tmSanitizeReason(reason)) + "). Contact Admin/Provider to fix it ASAP.",
		Footer:  "Update the cookie in the panel, set status Active, then open a new access link",
	})
}

func renderMagnificSwitchPage(w http.ResponseWriter, cfg Config, accountName, returnPath string) {
	if returnPath == "" || !strings.HasPrefix(returnPath, "/") {
		returnPath = cfg.HomePath
	}
	if returnPath == "" {
		returnPath = "/"
	}
	msg := "Account logged out. Switching to another account..."
	if accountName != "" {
		msg = "Account logged out. Switching to " + html.EscapeString(accountName) + "..."
	}
	writeLightCard(w, http.StatusOK, lightCard{
		Title: "Switching account", Heading: "Switching account", Message: msg,
		Badge: "Checking the next account", Footer: "This page refreshes automatically",
		Spin: true, Redirect: returnPath,
	})
}

func serveMagnificCookieFailover(w http.ResponseWriter, r *http.Request, cfg Config, sessionToken, currentUser string, activeAcc ToolAccount, reason string) {
	log.Printf("[FAILOVER] start user=%s account=%s(%d) reason=%s", currentUser, activeAcc.Name, activeAcc.ID, reason)
	if activeAcc.ID > 0 {
		panelMarkAccountLoggedOut(cfg, activeAcc.ID, reason)
	}
	next, nextName, err := panelSwitchToOtherAccount(cfg, sessionToken, reason)
	if err == nil && next.ID != activeAcc.ID {
		renderMagnificSwitchPage(w, cfg, nextName, "")
		return
	}
	if db, dbErr := openPanelDB(cfg); dbErr == nil {
		var websiteID int
		_ = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID)
		tmRecordSwitchLogout(db, websiteID, currentUser, activeAcc.Name, "(none)", "no_other_active:"+reason)
	}
	renderMagnificContactAdmin(w, cfg, reason)
}

var (
	magnificFOMu   sync.Mutex
	magnificFOLast = map[string]time.Time{}
)

func magnificFailoverRecently(sessionToken string) bool {
	magnificFOMu.Lock()
	defer magnificFOMu.Unlock()
	t, ok := magnificFOLast[sessionToken]
	if ok && time.Since(t) < 8*time.Second {
		return true
	}
	magnificFOLast[sessionToken] = time.Now()
	return false
}
