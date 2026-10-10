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

type artLogoutHit struct {
	at       time.Time
	next     string
	switched bool
}

var (
	artLogoutMu   sync.Mutex
	artLogoutSeen = map[int]artLogoutHit{}
	artSessionHop = map[string]time.Time{}
)

func artistlyLogoutPath(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	p := strings.ToLower(r.URL.Path)
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	p = strings.TrimSuffix(p, "/")
	switch p {
	case "/login", "/logout", "/sign-in", "/signin", "/sign-out", "/signout":
		return true
	}
	return strings.HasPrefix(p, "/login/") || strings.HasPrefix(p, "/logout/") ||
		strings.HasPrefix(p, "/sign-in/") || strings.Contains(p, "sign-out") || strings.Contains(p, "signout")
}

func artistlyUpstreamLogout(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	loc := strings.ToLower(resp.Header.Get("Location"))
	inertia := strings.ToLower(resp.Header.Get("X-Inertia-Location"))
	if resp.StatusCode == http.StatusConflict && artistlyLoginLocation(inertia) {
		return true
	}
	if resp.StatusCode < 300 || resp.StatusCode > 399 {
		return false
	}
	return artistlyLoginLocation(loc)
}

func artistlyLoginLocation(loc string) bool {
	return strings.Contains(loc, "/login") || strings.Contains(loc, "/logout") ||
		strings.Contains(loc, "sign-in") || strings.Contains(loc, "sign-out")
}

func artistlyLoggedOutHTML(body []byte) bool {
	s := strings.ToLower(string(body))
	if strings.Contains(s, "write your prompt") || strings.Contains(s, "ai image designer") {
		return false
	}
	return strings.Contains(s, "name=\"password\"") &&
		(strings.Contains(s, "name=\"email\"") || strings.Contains(s, "forgot password") || strings.Contains(s, ">sign in<"))
}

func artistlyUnauthenticatedJSON(status int, body []byte) bool {
	if status != http.StatusUnauthorized {
		return false
	}
	return strings.Contains(strings.ToLower(string(body)), "unauthenticated")
}

// noteArtistlyLogout writes Analytics → Logouts and moves this session to
// another active account when one exists. Account status is left unchanged.
func noteArtistlyLogout(cfg Config, r *http.Request, acc ToolAccount, sessionToken, reason string) (switched bool, next string) {
	if !usesPanelAccountMode(cfg) || acc.ID <= 0 {
		return false, ""
	}
	artLogoutMu.Lock()
	defer artLogoutMu.Unlock()
	if last, seen := artLogoutSeen[acc.ID]; seen && time.Since(last.at) < 2*time.Minute {
		return last.switched, last.next
	}

	db, err := openPanelDB(cfg)
	if err != nil {
		log.Printf("[LB] logout db open failed: %v", err)
		return false, ""
	}
	now := time.Now().UTC().Format(time.RFC3339)
	nextName := "(none)"
	skipIDs := []any{cfg.PublicHost, acc.ID}
	placeholders := "?"
	for id, hit := range artLogoutSeen {
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

	websiteID, username, loginIP := artistlyLogoutMeta(db, cfg, sessionToken)
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
	artLogoutSeen[acc.ID] = artLogoutHit{at: time.Now(), next: nextName, switched: switched}
	return switched, nextName
}

func artistlyLogoutMeta(db *sql.DB, cfg Config, sessionToken string) (websiteID int, username, loginIP string) {
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

func artistlyWantsDocument(r *http.Request) bool {
	if r == nil {
		return true
	}
	if isDocumentNavigation(r) || r.Header.Get("X-Inertia") != "" {
		return true
	}
	return r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/html")
}

func serveArtistlyLogout(w http.ResponseWriter, r *http.Request, cfg Config, acc ToolAccount, sessionToken, reason string) {
	clearProxyHeaders(w)
	switched, _ := noteArtistlyLogout(cfg, r, acc, sessionToken, reason)
	home := cfg.HomePath
	if home == "" {
		home = "/ai/ai-image-designer"
	}
	if switched && artistlyCanHop(sessionToken) {
		if artistlyWantsDocument(r) {
			http.Redirect(w, r, home, http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Artistly-Reload", "1")
		w.Header().Set("X-Inertia-Location", home)
		w.WriteHeader(http.StatusConflict)
		fmt.Fprintf(w, `{"reload":true}`)
		return
	}
	if artistlyWantsDocument(r) {
		name := html.EscapeString(toolDisplayName(cfg))
		writeLightCard(w, http.StatusOK, lightCard{
			Title:   "Contact admin",
			Heading: "Contact admin",
			Message: "The <span class=\"brand\">" + name + "</span> session ended and no other account is available. Contact Admin/Provider.",
			Footer:  "Saved in panel Analytics → Logouts. Account status was not changed.",
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"error":"contact_admin"}`)
}

func artistlyCanHop(sessionToken string) bool {
	if sessionToken == "" {
		return false
	}
	artLogoutMu.Lock()
	defer artLogoutMu.Unlock()
	if last, ok := artSessionHop[sessionToken]; ok && time.Since(last) < 2*time.Minute {
		return false
	}
	artSessionHop[sessionToken] = time.Now()
	return true
}

func artistlyReloadScript(home string) string {
	return fmt.Sprintf(`<script>(function(){var HOME=%q;var once=false;function go(){if(once)return;once=true;location.replace(HOME);}function reload(res){return res&&res.headers&&res.headers.get&&res.headers.get("X-Artistly-Reload")==="1";}var of=window.fetch;if(of){window.fetch=function(){return of.apply(this,arguments).then(function(res){if(reload(res))go();return res;});};}var xs=XMLHttpRequest.prototype.send;XMLHttpRequest.prototype.send=function(){this.addEventListener("load",function(){if(this.getResponseHeader("X-Artistly-Reload")==="1")go();});return xs.apply(this,arguments);};})();</script>`, home)
}
