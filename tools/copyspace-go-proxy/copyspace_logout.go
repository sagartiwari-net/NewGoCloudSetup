package main

import (
	"database/sql"
	"html"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type csLogoutHit struct {
	at       time.Time
	next     string
	switched bool
}

var (
	csLogoutMu   sync.Mutex
	csLogoutSeen = map[int]csLogoutHit{}
	csSessionHop = map[string]time.Time{}
)

func copyspaceLogoutPath(path string) bool {
	p := strings.ToLower(path)
	if i := strings.Index(p, "/extra-cdn-"); i >= 0 {
		rest := p[i+len("/extra-cdn-"):]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			p = rest[slash:]
		}
	}
	p = strings.TrimSuffix(p, "/")
	switch p {
	case "/signin", "/sign-in", "/login", "/signup", "/register", "/logout", "/forgot-password":
		return true
	}
	return strings.HasPrefix(p, "/signin/") || strings.HasPrefix(p, "/login/") || strings.HasPrefix(p, "/sign-in/")
}

func copyspaceLoggedOutHTML(body []byte) bool {
	s := strings.ToLower(string(body))
	if strings.Contains(s, "sign in to your account") && strings.Contains(s, "welcome back") {
		return true
	}
	return strings.Contains(s, "forgot password") && strings.Contains(s, "don't have an account")
}

func copyspaceUpstreamLogout(resp *http.Response) bool {
	if resp == nil || resp.StatusCode < 300 || resp.StatusCode > 399 {
		return false
	}
	loc := strings.ToLower(resp.Header.Get("Location"))
	return strings.Contains(loc, "/signin") || strings.Contains(loc, "/sign-in") ||
		strings.Contains(loc, "/login") || strings.Contains(loc, "/logout")
}

// An expired account cookie comes back as 419, or as an Inertia 409 that
// points at the page already open. That 409 is the refresh loop.
func copyspaceExpiredUpstream(r *http.Request, resp *http.Response) bool {
	if resp == nil {
		return false
	}
	if resp.StatusCode == 419 || copyspaceUpstreamLogout(resp) {
		return true
	}
	if resp.StatusCode != http.StatusConflict {
		return false
	}
	loc := resp.Header.Get("X-Inertia-Location")
	if loc == "" {
		return false
	}
	low := strings.ToLower(loc)
	if strings.Contains(low, "/signin") || strings.Contains(low, "/sign-in") ||
		strings.Contains(low, "/login") || strings.Contains(low, "/logout") {
		return true
	}
	locURL, err := url.Parse(loc)
	refURL, err2 := url.Parse(r.Referer())
	if err != nil || err2 != nil || locURL.Path == "" || refURL.Path == "" {
		return false
	}
	trim := func(p string) string {
		p = strings.TrimSuffix(p, "/")
		if p == "" {
			return "/"
		}
		return p
	}
	return trim(locURL.Path) == trim(refURL.Path)
}

func clearCopyspaceSessionOverlay() {
	localCookieMu.Lock()
	delete(localCookieOverlay, "copyspaceai_session")
	delete(localCookieOverlay, "XSRF-TOKEN")
	localCookieMu.Unlock()
}

func noteCopyspaceLogout(cfg Config, r *http.Request, acc ToolAccount, sessionToken, reason string) (switched bool, next string) {
	if !usesPanelAccountMode(cfg) || acc.ID <= 0 {
		return false, ""
	}
	key := sessionToken
	if key == "" {
		key = "acc:" + acc.Name
	}
	csLogoutMu.Lock()
	defer csLogoutMu.Unlock()
	if prev, seen := csSessionHop[key]; seen && time.Since(prev) < 2*time.Minute {
		last := csLogoutSeen[acc.ID]
		if time.Since(prev) < 2*time.Second {
			return last.switched, last.next
		}
		return false, last.next
	}
	csSessionHop[key] = time.Now()

	db, err := openPanelDB(cfg)
	if err != nil {
		log.Printf("[LB] logout db open failed: %v", err)
		delete(csSessionHop, key)
		return false, ""
	}
	now := time.Now().UTC().Format(time.RFC3339)
	nextName := "(none)"
	skipIDs := []any{cfg.PublicHost, acc.ID}
	placeholders := "?"
	for id, hit := range csLogoutSeen {
		if id == acc.ID || time.Since(hit.at) >= 10*time.Minute {
			continue
		}
		skipIDs = append(skipIDs, id)
		placeholders += ",?"
	}
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

	websiteID, username, loginIP := copyspaceLogoutMeta(db, cfg, sessionToken)
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
	csLogoutSeen[acc.ID] = csLogoutHit{at: time.Now(), next: nextName, switched: switched}
	return switched, nextName
}

func copyspaceLogoutMeta(db *sql.DB, cfg Config, sessionToken string) (websiteID int, username, loginIP string) {
	if sessionToken != "" {
		_ = db.QueryRow(`SELECT website_id, username, COALESCE(client_ip, '') FROM live_sessions WHERE session_token=?`, sessionToken).Scan(&websiteID, &username, &loginIP)
	}
	if websiteID <= 0 {
		_ = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID)
	}
	return websiteID, username, loginIP
}

func serveCopyspaceLogout(w http.ResponseWriter, r *http.Request, cfg Config, acc ToolAccount, sessionToken, reason, publicBase string) {
	clearCopyspaceSessionOverlay()
	for k := range w.Header() {
		w.Header().Del(k)
	}
	switched, _ := noteCopyspaceLogout(cfg, r, acc, sessionToken, reason)
	home := cfg.HomePath
	if home == "" {
		home = "/"
	}
	if publicBase == "" {
		scheme := cfg.PublicScheme
		if scheme == "" {
			scheme = "https"
		}
		host := cfg.PublicHost
		if host == "" && r != nil {
			host = r.Host
		}
		publicBase = scheme + "://" + host
	}
	if switched {
		if isDocumentNavigation(r) {
			http.Redirect(w, r, home, http.StatusFound)
			return
		}
		w.Header().Set("X-Inertia-Location", strings.TrimRight(publicBase, "/")+home)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusConflict)
		return
	}
	if r != nil && !isDocumentNavigation(r) {
		w.Header().Set("X-Inertia-Location", strings.TrimRight(publicBase, "/")+"/__tm_contact_admin")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusConflict)
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
