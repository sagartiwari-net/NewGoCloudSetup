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

type sfLogoutHit struct {
	at       time.Time
	next     string
	switched bool
}

var (
	sfLogoutMu   sync.Mutex
	sfLogoutSeen = map[int]sfLogoutHit{}
)

// noteShortformLogout writes a panel Analytics logout row and moves this
// session to another active account when one exists. Account status is left as-is.
func noteShortformLogout(cfg Config, r *http.Request, reason string) (switched bool, next string) {
	if !usesPanelAccountMode(cfg) || r == nil {
		return false, ""
	}
	acc, ok := shortformAccountFrom(r)
	if !ok {
		return false, ""
	}
	sfLogoutMu.Lock()
	if last, seen := sfLogoutSeen[acc.ID]; seen && time.Since(last.at) < 2*time.Minute {
		sfLogoutMu.Unlock()
		return last.switched, last.next
	}
	sfLogoutMu.Unlock()

	db, err := openPanelDB(cfg)
	if err != nil {
		log.Printf("[LB] logout db open failed: %v", err)
		return false, ""
	}
	now := time.Now().UTC().Format(time.RFC3339)
	nextName := "(none)"
	skipIDs := []any{cfg.PublicHost, acc.ID}
	placeholders := "?"
	sfLogoutMu.Lock()
	for id, hit := range sfLogoutSeen {
		if id == acc.ID || time.Since(hit.at) >= 10*time.Minute {
			continue
		}
		skipIDs = append(skipIDs, id)
		placeholders += ",?"
	}
	sfLogoutMu.Unlock()
	if other, err := scanPanelAccount(db.QueryRow(panelAccountSelect+`
		WHERE w.domain = ? AND a.status = 'active' AND a.cookie != '' AND a.id NOT IN (`+placeholders+`)
		`+panelAccountOrder+` LIMIT 1`, skipIDs...)); err == nil {
		_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, other.ID)
		if acc.Session != "" {
			_, _ = db.Exec(`UPDATE live_sessions SET assigned_account_id=? WHERE session_token=?`, other.ID, acc.Session)
		}
		nextName = other.Name
		switched = true
		log.Printf("[LB] switched %s -> %s (status unchanged)", acc.Name, other.Name)
	}

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
		websiteID, username, acc.Name, nextName, reason, ip, now); err != nil {
		log.Printf("[LB] logout_events insert failed: %v", err)
	} else {
		log.Printf("[LB] logout recorded account=%s next=%s user=%s ip=%s reason=%s", acc.Name, nextName, username, ip, reason)
	}
	sfLogoutMu.Lock()
	sfLogoutSeen[acc.ID] = sfLogoutHit{at: time.Now(), next: nextName, switched: switched}
	sfLogoutMu.Unlock()
	return switched, nextName
}

func shortformLoginPath(p string) bool {
	p = strings.ToLower(strings.TrimSpace(p))
	return p == "/app/login" || p == "/login" || strings.HasPrefix(p, "/app/login/")
}

func serveShortformLogout(w http.ResponseWriter, r *http.Request, cfg Config, reason string) {
	switched, _ := noteShortformLogout(cfg, r, reason)
	if switched {
		http.Redirect(w, r, "/app/discover", http.StatusFound)
		return
	}
	name := html.EscapeString(toolDisplayName(cfg))
	writeLightCard(w, http.StatusOK, lightCard{
		Title:   "Contact admin",
		Heading: "Contact admin",
		Message: "The <span class=\"brand\">" + name + "</span> session ended and no other account is available. Contact Admin/Provider.",
		Footer:  "Saved in panel Analytics → Logouts. Account status was not changed.",
	})
}
