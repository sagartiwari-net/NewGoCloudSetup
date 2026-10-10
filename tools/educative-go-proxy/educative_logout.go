package main

import (
	"database/sql"
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type eduLogoutHit struct {
	at       time.Time
	next     string
	switched bool
}

var (
	eduLogoutMu   sync.Mutex
	eduLogoutSeen = map[int]eduLogoutHit{}
	eduSessionHop = map[string]time.Time{}
)

func educativeLogoutPath(path string) bool {
	p := strings.ToLower(strings.TrimSuffix(path, "/"))
	switch p {
	case "/login", "/signup", "/sign-up", "/sign-in", "/register", "/logout", "/forgot-password":
		return true
	}
	return strings.HasPrefix(p, "/login/") || strings.HasPrefix(p, "/signup/") ||
		strings.HasPrefix(p, "/sign-in/") || strings.HasPrefix(p, "/sign-up/") ||
		strings.HasPrefix(p, "/logout/")
}

// A dead Educative cookie is HTTP 200 with {"status":401}, not an HTTP 401.
func educativeLoggedOutAPI(path string, body []byte) bool {
	p := strings.ToLower(path)
	if p != "/api/user/info" && p != "/api/userdata/user_config" {
		return false
	}
	s := string(body)
	return strings.Contains(s, `"status":401`) || strings.Contains(s, `"status": 401`)
}

func noteEducativeLogout(cfg Config, r *http.Request, acc ToolAccount, sessionToken, reason string) (switched bool, next string) {
	if !usesPanelAccountMode(cfg) || acc.ID <= 0 {
		return false, ""
	}
	key := sessionToken
	if key == "" {
		key = "acc:" + acc.Name
	}
	eduLogoutMu.Lock()
	defer eduLogoutMu.Unlock()
	if prev, seen := eduSessionHop[key]; seen && time.Since(prev) < 2*time.Minute {
		last := eduLogoutSeen[acc.ID]
		if time.Since(prev) < 2*time.Second {
			return last.switched, last.next
		}
		return false, last.next
	}
	eduSessionHop[key] = time.Now()

	db, err := openPanelDB(cfg)
	if err != nil {
		log.Printf("[LB] logout db open failed: %v", err)
		delete(eduSessionHop, key)
		return false, ""
	}
	now := time.Now().UTC().Format(time.RFC3339)
	nextName := "(none)"
	skipIDs := []any{cfg.PublicHost, acc.ID}
	placeholders := "?"
	for id, hit := range eduLogoutSeen {
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

	websiteID, username, loginIP := educativeLogoutMeta(db, cfg, sessionToken)
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
	eduLogoutSeen[acc.ID] = eduLogoutHit{at: time.Now(), next: nextName, switched: switched}
	return switched, nextName
}

func educativeLogoutMeta(db *sql.DB, cfg Config, sessionToken string) (websiteID int, username, loginIP string) {
	if sessionToken != "" {
		_ = db.QueryRow(`SELECT website_id, username, COALESCE(client_ip, '') FROM live_sessions WHERE session_token=?`, sessionToken).Scan(&websiteID, &username, &loginIP)
	}
	if websiteID <= 0 {
		_ = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID)
	}
	return websiteID, username, loginIP
}

func educativePublicBase(cfg Config, r *http.Request, publicBase string) string {
	if publicBase != "" {
		return strings.TrimRight(publicBase, "/")
	}
	scheme := cfg.PublicScheme
	if scheme == "" {
		scheme = "https"
	}
	host := cfg.PublicHost
	if host == "" && r != nil {
		host = r.Host
	}
	return strings.TrimRight(scheme+"://"+host, "/")
}

func educativeHome(cfg Config) string {
	if cfg.HomePath != "" {
		return cfg.HomePath
	}
	return "/learn/home"
}

// educativeLogoutTarget switches once and returns the URL the page should open.
// A second hit in the same session opens the contact-admin card.
func educativeLogoutTarget(cfg Config, r *http.Request, acc ToolAccount, sessionToken, reason, publicBase string) string {
	switched, _ := noteEducativeLogout(cfg, r, acc, sessionToken, reason)
	base := educativePublicBase(cfg, r, publicBase)
	if switched {
		return base + educativeHome(cfg)
	}
	return base + "/__tm_contact_admin"
}

func setEducativeLogoutHeader(w http.ResponseWriter, loc string) {
	if loc == "" {
		return
	}
	w.Header().Set("X-TM-Logout", loc)
	expose := w.Header().Get("Access-Control-Expose-Headers")
	if !strings.Contains(strings.ToLower(expose), "x-tm-logout") {
		if expose == "" {
			w.Header().Set("Access-Control-Expose-Headers", "X-TM-Logout")
		} else {
			w.Header().Set("Access-Control-Expose-Headers", expose+", X-TM-Logout")
		}
	}
}

func serveEducativeLogout(w http.ResponseWriter, r *http.Request, cfg Config, acc ToolAccount, sessionToken, reason, publicBase string) {
	for k := range w.Header() {
		w.Header().Del(k)
	}
	loc := educativeLogoutTarget(cfg, r, acc, sessionToken, reason, publicBase)
	if r != nil && !isDocumentNavigation(r) {
		setEducativeLogoutHeader(w, loc)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":401}`)
		return
	}
	if strings.HasSuffix(loc, "/__tm_contact_admin") {
		name := html.EscapeString(toolDisplayName(cfg))
		writeLightCard(w, http.StatusOK, lightCard{
			Title:   "Contact admin",
			Heading: "Contact admin",
			Message: "The <span class=\"brand\">" + name + "</span> session ended and no other account is available. Contact Admin/Provider.",
			Footer:  "Saved in panel Analytics → Logouts. Account status was not changed.",
		})
		return
	}
	http.Redirect(w, r, educativeHome(cfg), http.StatusFound)
}

func writeEducativeContactAdmin(w http.ResponseWriter, cfg Config) {
	name := html.EscapeString(toolDisplayName(cfg))
	writeLightCard(w, http.StatusOK, lightCard{
		Title:   "Contact admin",
		Heading: "Contact admin",
		Message: "The <span class=\"brand\">" + name + "</span> session ended and no other account is available. Contact Admin/Provider.",
		Footer:  "Saved in panel Analytics → Logouts. Account status was not changed.",
	})
}
