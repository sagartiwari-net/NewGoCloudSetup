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

	// Last swap outcome per panel session so debounce still blocks dead upstream traffic.
	gptzeroOutcomeMu sync.Mutex
	gptzeroOutcome   = map[string]string{} // "logged_out" | "switched:<name>"

	deadRefreshMu    sync.Mutex
	deadRefreshUntil = map[int]time.Time{}
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

func gptzeroSetOutcome(sessionToken, outcome string) {
	if sessionToken == "" || outcome == "" {
		return
	}
	gptzeroOutcomeMu.Lock()
	gptzeroOutcome[sessionToken] = outcome
	gptzeroOutcomeMu.Unlock()
}

func gptzeroGetOutcome(sessionToken string) string {
	gptzeroOutcomeMu.Lock()
	defer gptzeroOutcomeMu.Unlock()
	return gptzeroOutcome[sessionToken]
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

func markDeadRefresh(accountID int) {
	if accountID <= 0 {
		return
	}
	deadRefreshMu.Lock()
	deadRefreshUntil[accountID] = time.Now().Add(15 * time.Minute)
	deadRefreshMu.Unlock()
}

func isDeadRefresh(accountID int) bool {
	if accountID <= 0 {
		return false
	}
	deadRefreshMu.Lock()
	defer deadRefreshMu.Unlock()
	until, ok := deadRefreshUntil[accountID]
	return ok && time.Now().Before(until)
}

func clearDeadRefresh(accountID int) {
	if accountID <= 0 {
		return
	}
	deadRefreshMu.Lock()
	delete(deadRefreshUntil, accountID)
	deadRefreshMu.Unlock()
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
			if currentID > 0 {
				_, _ = db.Exec(`UPDATE accounts SET status='logged_out', failure_count=failure_count+1 WHERE id=?`, currentID)
			}
			var websiteID int
			_ = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID)
			if websiteID > 0 {
				tmRecordSwitchLogout(db, websiteID, username, currentName, "(none)", "no_other_active:"+reason)
			}
			log.Printf("[SWAP] no other active account current=%s(%d) reason=%s user=%s",
				currentName, currentID, reason, username)
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
		tmRecordSwitchLogout(db, websiteID, username, currentName, next.Name, reason)
	}
	log.Printf("[SWAP] '%s' (ID:%d) → '%s' (ID:%d) reason=%s user=%s",
		currentName, currentID, next.Name, next.ID, reason, username)
	log.Printf("[LB] switched '%s' (ID:%d) -> '%s' (ID:%d) reason=%s", currentName, currentID, next.Name, next.ID, reason)
	return next, nil
}

// gptzeroLooksLoggedOut detects dead GPTZero/Supabase sessions that should rotate accounts.
// Do NOT treat every 401 (features table, grammarly-bundle, etc.) as logout — that causes
// swap loops, toasts, and slow reloads.
func gptzeroLooksLoggedOut(status int, path string, body []byte) bool {
	pathL := strings.ToLower(path)
	if i := strings.Index(pathL, "?"); i >= 0 {
		pathL = pathL[:i]
	}
	// Supabase feature catalog noise — stubbed elsewhere; never swap on these.
	if isSupabaseFeaturesPath(pathL) || strings.Contains(pathL, "grammarly-bundle") ||
		strings.Contains(pathL, "growthbook") || strings.Contains(pathL, "/manifest") {
		return false
	}
	lower := strings.ToLower(string(body))
	if strings.Contains(lower, "invalid jwt") || strings.Contains(lower, "jwt expired") ||
		strings.Contains(lower, "invalid_grant") || strings.Contains(lower, "refresh_token_not_found") ||
		strings.Contains(lower, "refresh_token_already_used") ||
		strings.Contains(lower, "not authenticated") || strings.Contains(lower, "session_not_found") ||
		strings.Contains(lower, "userid is missing") ||
		strings.Contains(lower, "require valid cookie") || strings.Contains(lower, "requires login with email") {
		return true
	}
	// Critical auth surfaces only.
	critical := strings.Contains(pathL, "/auth/v1/") ||
		strings.Contains(pathL, "assignments/access") ||
		strings.Contains(pathL, "/v2/user/callhistory") ||
		(strings.Contains(pathL, "/v3/scan") && status == http.StatusUnauthorized)
	if critical && (status == http.StatusUnauthorized || status == http.StatusForbidden) {
		if strings.Contains(lower, "rate limit") || strings.Contains(lower, "quota") ||
			strings.Contains(lower, "credit") || strings.Contains(lower, "limit reached") {
			return false
		}
		return true
	}
	return false
}

func gptzeroAuthRefreshFailed(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "refresh") || strings.Contains(msg, "invalid_grant") ||
		strings.Contains(msg, "already used") || strings.Contains(msg, "http 4") ||
		strings.Contains(msg, "no refresh_token") || strings.Contains(msg, "empty session")
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

func writeGPTZeroSwapResponse(w http.ResponseWriter, r *http.Request, cfg Config, outcome string) bool {
	if strings.HasPrefix(outcome, "switched:") {
		nextName := strings.TrimPrefix(outcome, "switched:")
		if isDocumentNavigation(r) || strings.Contains(r.Header.Get("Accept"), "text/html") {
			renderGPTZeroSwitchingPage(w, cfg, nextName)
			return true
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-TM-Account-Switch", "1")
		w.Header().Set("Access-Control-Expose-Headers", "X-TM-Account-Switch")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprintf(w, `{"error":"account_switched","message":"retry","next":%q}`, nextName)
		return true
	}
	// logged_out / unknown → stop dead API spam
	if wantsLightDenied(r, r.URL.Path) || isDocumentNavigation(r) ||
		strings.Contains(r.Header.Get("Accept"), "text/html") {
		renderGPTZeroLoggedOutPage(w, cfg)
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-TM-Account-Switch", "1")
	w.Header().Set("Access-Control-Expose-Headers", "X-TM-Account-Switch")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"account_logged_out","message":"Account logged out. Contact the admin."}`))
	return true
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
	markDeadRefresh(currentID)

	if !gptzeroAllowSwap(token) {
		outcome := gptzeroGetOutcome(token)
		if outcome == "" {
			outcome = "logged_out"
		}
		log.Printf("[SWAP] debounce reuse outcome=%s reason=%s", outcome, reason)
		return writeGPTZeroSwapResponse(w, r, cfg, outcome)
	}

	next, err := panelSwitchAccount(cfg, token, currentID, currentName, panelUser, reason)
	if err != nil {
		log.Printf("[SWAP] failed reason=%s current=%s(%d): %v", reason, currentName, currentID, err)
		gptzeroSetOutcome(token, "logged_out")
		return writeGPTZeroSwapResponse(w, r, cfg, "logged_out")
	}
	clearDeadRefresh(next.ID)
	outcome := "switched:" + next.Name
	gptzeroSetOutcome(token, outcome)
	return writeGPTZeroSwapResponse(w, r, cfg, outcome)
}
