package main

import (
	"context"
	"html"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type sfAccountKey struct{}

type sfAccountCtx struct {
	ID       int
	Name     string
	Session  string
	ClientIP string
}

func withShortformAccount(r *http.Request, acc ToolAccount) *http.Request {
	sess := ""
	if c, err := r.Cookie("ct_session"); err == nil {
		sess = c.Value
	}
	return r.WithContext(context.WithValue(r.Context(), sfAccountKey{}, sfAccountCtx{
		ID: acc.ID, Name: acc.Name, Session: sess, ClientIP: panelClientIP(r),
	}))
}

func shortformAccountFrom(r *http.Request) (sfAccountCtx, bool) {
	if r == nil {
		return sfAccountCtx{}, false
	}
	v, ok := r.Context().Value(sfAccountKey{}).(sfAccountCtx)
	return v, ok && v.ID > 0
}

var (
	sfLogoutMu   sync.Mutex
	sfLogoutSeen = map[int]time.Time{}
)

// noteShortformLogout marks the panel account logged_out and writes logout_events
// so Analytics Logouts shows the entry. Bursts of 401s count as one event.
func noteShortformLogout(cfg Config, r *http.Request, reason string) {
	if !usesPanelAccountMode(cfg) || r == nil {
		return
	}
	acc, ok := shortformAccountFrom(r)
	if !ok {
		return
	}
	sfLogoutMu.Lock()
	if last, seen := sfLogoutSeen[acc.ID]; seen && time.Since(last) < 2*time.Minute {
		sfLogoutMu.Unlock()
		return
	}
	sfLogoutSeen[acc.ID] = time.Now()
	sfLogoutMu.Unlock()

	db, err := openPanelDB(cfg)
	if err != nil {
		log.Printf("[LB] logout db open failed: %v", err)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = db.Exec(`UPDATE accounts SET status='logged_out', failure_count=failure_count+1 WHERE id=?`, acc.ID)

	var websiteID int
	var username, loginIP string
	if acc.Session != "" {
		_ = db.QueryRow(`SELECT website_id, username, COALESCE(client_ip, '') FROM live_sessions WHERE session_token=?`, acc.Session).Scan(&websiteID, &username, &loginIP)
	}
	if websiteID <= 0 {
		_ = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID)
	}
	ip := acc.ClientIP
	if ip == "" || ip == "127.0.0.1" || ip == "::1" {
		if loginIP != "" && loginIP != "127.0.0.1" && loginIP != "::1" {
			ip = loginIP
		}
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "logout"
	}
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS logout_events (
		id INTEGER PRIMARY KEY,
		website_id INTEGER NOT NULL,
		username TEXT NOT NULL DEFAULT '',
		account_name TEXT NOT NULL DEFAULT '',
		next_account_name TEXT NOT NULL DEFAULT '',
		reason TEXT NOT NULL DEFAULT '',
		client_ip TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	)`)
	if _, err = db.Exec(`INSERT INTO logout_events (website_id, username, account_name, next_account_name, reason, client_ip, created_at) VALUES (?,?,?,?,?,?,?)`,
		websiteID, username, acc.Name, "(none)", reason, ip, now); err != nil {
		log.Printf("[LB] logout_events insert failed: %v", err)
		return
	}
	log.Printf("[LB] logout recorded account=%s user=%s ip=%s reason=%s", acc.Name, username, ip, reason)
}

func renderShortformLoggedOut(w http.ResponseWriter, cfg Config) {
	name := html.EscapeString(toolDisplayName(cfg))
	writeLightCard(w, http.StatusUnauthorized, lightCard{
		Title:   "Logged out",
		Heading: "Logged out",
		Message: "The <span class=\"brand\">" + name + "</span> account session ended. Contact Admin/Provider.",
		Footer:  "This logout was saved in the panel",
	})
}
