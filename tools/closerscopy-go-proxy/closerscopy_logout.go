package main

import (
	"database/sql"
	"html"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ccLogoutHit struct {
	at       time.Time
	next     string
	switched bool
}

var (
	ccLogoutMu   sync.Mutex
	ccLogoutSeen = map[int]ccLogoutHit{}
	ccSessionHop = map[string]time.Time{}
)

func closersLogoutPath(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	p := strings.ToLower(strings.TrimSuffix(r.URL.Path, "/"))
	switch p {
	case "/signin", "/sign-in", "/login", "/signup", "/register", "/logout", "/forgot", "/forgot-password":
		return true
	}
	return strings.HasPrefix(p, "/signin/") || strings.HasPrefix(p, "/login/") || strings.HasPrefix(p, "/sign-in/")
}

func closersLoggedOutHTML(body []byte) bool {
	s := strings.ToLower(string(body))
	if strings.Contains(s, "/signin") && strings.Contains(s, "sign in") {
		return true
	}
	return strings.Contains(s, "don't have an account") && strings.Contains(s, "password")
}

func closersUpstreamLogout(resp *http.Response) bool {
	if resp == nil || resp.StatusCode < 300 || resp.StatusCode > 399 {
		return false
	}
	loc := strings.ToLower(resp.Header.Get("Location"))
	return strings.Contains(loc, "/signin") || strings.Contains(loc, "/sign-in") ||
		strings.Contains(loc, "/login") || strings.Contains(loc, "/logout")
}

func noteClosersLogout(cfg Config, r *http.Request, acc ToolAccount, sessionToken, reason string) (switched bool, next string) {
	if !usesPanelAccountMode(cfg) || acc.ID <= 0 {
		return false, ""
	}
	ccLogoutMu.Lock()
	if last, seen := ccLogoutSeen[acc.ID]; seen && time.Since(last.at) < 2*time.Minute {
		ccLogoutMu.Unlock()
		return false, last.next
	}
	ccLogoutMu.Unlock()

	db, err := openPanelDB(cfg)
	if err != nil {
		log.Printf("[LB] logout db open failed: %v", err)
		return false, ""
	}
	now := time.Now().UTC().Format(time.RFC3339)
	nextName := "(none)"
	skipIDs := []any{cfg.PublicHost, acc.ID}
	placeholders := "?"
	ccLogoutMu.Lock()
	for id, hit := range ccLogoutSeen {
		if id == acc.ID || time.Since(hit.at) >= 10*time.Minute {
			continue
		}
		skipIDs = append(skipIDs, id)
		placeholders += ",?"
	}
	ccLogoutMu.Unlock()
	if other, err := scanPanelAccount(db.QueryRow(panelAccountSelect+`
		WHERE w.domain = ? AND a.status = 'active' AND a.cookie != '' AND a.id NOT IN (`+placeholders+`)
		`+panelAccountOrder+` LIMIT 1`, skipIDs...)); err == nil {
		_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, other.ID)
		if sessionToken != "" {
			_, _ = db.Exec(`UPDATE live_sessions SET assigned_account_id=? WHERE session_token=?`, other.ID, sessionToken)
		}
		nextName = other.Name
		switched = true
		log.Printf("[LB] switched %s -> %s (status unchanged)", acc.Name, other.Name)
	}

	websiteID, username, loginIP := closersLogoutMeta(db, cfg, sessionToken)
	ip := ""
	if r != nil {
		ip = realClientIP(r)
	}
	if ip == "" || ip == "127.0.0.1" || ip == "::1" {
		if loginIP != "" && loginIP != "127.0.0.1" && loginIP != "::1" {
			ip = loginIP
		}
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "session_expired"
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
	ccLogoutMu.Lock()
	ccLogoutSeen[acc.ID] = ccLogoutHit{at: time.Now(), next: nextName, switched: switched}
	ccLogoutMu.Unlock()
	return switched, nextName
}

func closersLogoutMeta(db *sql.DB, cfg Config, sessionToken string) (websiteID int, username, loginIP string) {
	if sessionToken != "" {
		_ = db.QueryRow(`SELECT website_id, username, COALESCE(client_ip, '') FROM live_sessions WHERE session_token=?`, sessionToken).Scan(&websiteID, &username, &loginIP)
	}
	if websiteID <= 0 {
		_ = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID)
	}
	return websiteID, username, loginIP
}

func clearProxyHeaders(w http.ResponseWriter) {
	h := w.Header()
	for k := range h {
		h.Del(k)
	}
}

func serveClosersLogout(w http.ResponseWriter, r *http.Request, cfg Config, acc ToolAccount, sessionToken, reason string) {
	clearProxyHeaders(w)
	switched, _ := noteClosersLogout(cfg, r, acc, sessionToken, reason)
	if switched && closersCanHop(sessionToken) {
		home := cfg.HomePath
		if home == "" {
			home = "/"
		}
		http.Redirect(w, r, home, http.StatusFound)
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

func closersCanHop(sessionToken string) bool {
	if sessionToken == "" {
		return false
	}
	ccLogoutMu.Lock()
	defer ccLogoutMu.Unlock()
	if last, ok := ccSessionHop[sessionToken]; ok && time.Since(last) < 2*time.Minute {
		return false
	}
	ccSessionHop[sessionToken] = time.Now()
	return true
}
