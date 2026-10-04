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

var (
	gptzeroSwapMu   sync.Mutex
	gptzeroSwapAt   = map[string]time.Time{}
	gptzeroTriedMu  sync.Mutex
	gptzeroTriedAcc = map[string]map[int]bool{}
)

func gptzeroMarkTried(sessionToken string, id int) {
	if sessionToken == "" || id <= 0 {
		return
	}
	gptzeroTriedMu.Lock()
	defer gptzeroTriedMu.Unlock()
	if gptzeroTriedAcc[sessionToken] == nil {
		gptzeroTriedAcc[sessionToken] = map[int]bool{}
	}
	gptzeroTriedAcc[sessionToken][id] = true
}

func gptzeroAllowSwap(sessionToken string) bool {
	if sessionToken == "" {
		return false
	}
	gptzeroSwapMu.Lock()
	defer gptzeroSwapMu.Unlock()
	if t, ok := gptzeroSwapAt[sessionToken]; ok && time.Since(t) < 8*time.Second {
		return false
	}
	gptzeroSwapAt[sessionToken] = time.Now()
	return true
}

// panelSwitchAccount moves this session to the next active account.
// failure_count goes up. status stays active so the account can be used again later.
func panelSwitchAccount(cfg Config, sessionToken string, currentID int, currentName, username, reason string) (ToolAccount, error) {
	panelPickMu.Lock()
	defer panelPickMu.Unlock()
	db, err := openPanelDB(cfg)
	if err != nil {
		return ToolAccount{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	gptzeroMarkTried(sessionToken, currentID)

	gptzeroTriedMu.Lock()
	tried := gptzeroTriedAcc[sessionToken]
	gptzeroTriedMu.Unlock()

	rows, err := db.Query(panelAccountSelect+`
		WHERE w.domain = ? AND a.status = 'active' AND a.cookie != ''
		`+panelAccountOrder, cfg.PublicHost)
	if err != nil {
		return ToolAccount{}, err
	}
	defer rows.Close()

	var next ToolAccount
	found := false
	for rows.Next() {
		acc, scanErr := scanPanelAccount(rows)
		if scanErr != nil {
			continue
		}
		if acc.ID == currentID || (tried != nil && tried[acc.ID]) {
			continue
		}
		next = acc
		found = true
		break
	}
	_ = rows.Close()

	if !found {
		// Fallback: any other active account once all "tried" are exhausted.
		acc, err := scanPanelAccount(db.QueryRow(panelAccountSelect+`
			WHERE w.domain = ? AND a.status = 'active' AND a.cookie != '' AND a.id != ?
			`+panelAccountOrder+` LIMIT 1`, cfg.PublicHost, currentID))
		if err != nil {
			return ToolAccount{}, fmt.Errorf("no other active account")
		}
		next = acc
	}

	if currentID > 0 {
		_, _ = db.Exec(`UPDATE accounts SET failure_count=failure_count+1 WHERE id=?`, currentID)
	}
	_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, next.ID)
	if sessionToken != "" {
		_, _ = db.Exec(`UPDATE live_sessions SET assigned_account_id=? WHERE session_token=?`, next.ID, sessionToken)
	}
	var websiteID int
	_ = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID)
	if websiteID > 0 {
		_, _ = db.Exec(`INSERT INTO switch_events (website_id, username, from_account_name, to_account_name, reason, switched_at) VALUES (?,?,?,?,?,?)`,
			websiteID, username, currentName, next.Name, reason, now)
	}
	log.Printf("[SWAP] '%s' (ID:%d) → '%s' (ID:%d) reason=%s user=%s",
		currentName, currentID, next.Name, next.ID, reason, username)
	log.Printf("[LB] switched '%s' (ID:%d) -> '%s' (ID:%d) reason=%s", currentName, currentID, next.Name, next.ID, reason)
	return next, nil
}

// gptzeroLooksLoggedOut detects dead GPTZero/Supabase sessions that should rotate accounts.
func gptzeroLooksLoggedOut(status int, path string, body []byte) bool {
	if status == http.StatusUnauthorized {
		return true
	}
	lower := strings.ToLower(string(body))
	pathL := strings.ToLower(path)
	if strings.Contains(lower, "invalid jwt") || strings.Contains(lower, "jwt expired") ||
		strings.Contains(lower, "invalid_grant") || strings.Contains(lower, "refresh_token_not_found") ||
		strings.Contains(lower, "not authenticated") || strings.Contains(lower, "session_not_found") ||
		strings.Contains(lower, "user not found") {
		return true
	}
	if status == http.StatusForbidden {
		if strings.Contains(lower, "rate limit") || strings.Contains(lower, "quota") ||
			strings.Contains(lower, "credit") || strings.Contains(lower, "limit reached") {
			return false
		}
		if strings.Contains(lower, "unauthorized") || strings.Contains(lower, "forbidden") ||
			strings.Contains(lower, "jwt") || strings.Contains(lower, "token") {
			return true
		}
		if strings.Contains(pathL, "feature") || strings.Contains(pathL, "permission") ||
			strings.Contains(pathL, "entitlement") || strings.Contains(pathL, "/me") ||
			strings.Contains(pathL, "profile") || strings.Contains(pathL, "subscription") {
			return true
		}
	}
	// Client toast: "Failed to fetch your feature access permissions."
	if strings.Contains(pathL, "feature") || strings.Contains(pathL, "permission") ||
		strings.Contains(pathL, "entitlement") || strings.Contains(pathL, "growthbook") ||
		strings.Contains(pathL, "feature-access") || strings.Contains(pathL, "feature_access") {
		if status >= 400 {
			return true
		}
		// Some GPTZero APIs return 200 with an auth error body.
		if strings.Contains(lower, "unauthorized") || strings.Contains(lower, "not authenticated") ||
			strings.Contains(lower, "invalid jwt") || strings.Contains(lower, "jwt expired") ||
			strings.Contains(lower, "\"error\"") && (strings.Contains(lower, "auth") || strings.Contains(lower, "token")) {
			return true
		}
	}
	return false
}

func gptzeroAuthRefreshFailed(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "refresh") || strings.Contains(msg, "invalid_grant") ||
		strings.Contains(msg, "http 4") || strings.Contains(msg, "no refresh_token") ||
		strings.Contains(msg, "empty session")
}

func renderGPTZeroSwitchingPage(w http.ResponseWriter, cfg Config, nextName string) {
	home := cfg.HomePath
	if home == "" {
		home = "/"
	}
	name := strings.TrimSpace(nextName)
	msg := "Account logged out. Switching to another account..."
	if name != "" {
		msg = "Account logged out. Switching to " + html.EscapeString(name) + "..."
	}
	writeLightCard(w, http.StatusOK, lightCard{
		Title:    "Switching account",
		Heading:  "Switching account",
		Message:  msg,
		Badge:    "Trying one account at a time",
		Footer:   "This keeps trying until an account is available",
		Spin:     true,
		Redirect: home,
	})
}

func renderGPTZeroLoggedOutPage(w http.ResponseWriter, cfg Config) {
	writeLightCard(w, http.StatusOK, lightCard{
		Title:   "Account logged out",
		Heading: "Account logged out",
		Message: "Account logged out. Contact the admin.",
	})
}

// tryGPTZeroAccountSwap rotates the panel account and writes either a switching page
// (documents) or a JSON response with X-TM-Account-Switch (APIs).
func tryGPTZeroAccountSwap(w http.ResponseWriter, r *http.Request, cfg Config, panelUser string, currentID int, currentName, reason string) bool {
	if !usesPanelAccountMode(cfg) || cfg.BypassAuth {
		return false
	}
	token := ctSessionToken(r)
	if token == "" || currentID <= 0 {
		return false
	}
	if !gptzeroAllowSwap(token) {
		log.Printf("[SWAP] debounce skip reason=%s", reason)
		return false
	}
	next, err := panelSwitchAccount(cfg, token, currentID, currentName, panelUser, reason)
	if err != nil {
		log.Printf("[SWAP] failed reason=%s current=%s(%d): %v", reason, currentName, currentID, err)
		if wantsLightDenied(r, r.URL.Path) || isDocumentNavigation(r) {
			renderGPTZeroLoggedOutPage(w, cfg)
			return true
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"account_logged_out","message":"Account logged out. Contact the admin."}`))
		return true
	}
	if isDocumentNavigation(r) || strings.Contains(r.Header.Get("Accept"), "text/html") {
		renderGPTZeroSwitchingPage(w, cfg, next.Name)
		return true
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-TM-Account-Switch", "1")
	w.Header().Set("Access-Control-Expose-Headers", "X-TM-Account-Switch")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = fmt.Fprintf(w, `{"error":"account_switched","message":"retry","next":%q}`, next.Name)
	return true
}
