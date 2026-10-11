package main

import (
	"bytes"
	"database/sql"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type speechLogoutHit struct {
	at       time.Time
	next     string
	switched bool
}

var (
	speechLogoutMu   sync.Mutex
	speechLogoutSeen = map[int]speechLogoutHit{}
	speechSessionHop = map[string]time.Time{}
)

func speechLogoutPath(path string) bool {
	p := strings.ToLower(path)
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	p = strings.TrimSuffix(p, "/")
	switch p {
	case "/auth", "/login", "/signin", "/sign-in", "/signup", "/sign-up", "/logout", "/register", "/forgot", "/forgot-password":
		return true
	}
	return strings.HasPrefix(p, "/login/") || strings.HasPrefix(p, "/signin/") || strings.HasPrefix(p, "/sign-in/")
}

func speechAuthLocation(loc string) bool {
	l := strings.ToLower(strings.TrimSpace(loc))
	if l == "" {
		return false
	}
	if strings.Contains(l, "speechify.com/auth") || strings.Contains(l, "speechify.com/login") || strings.Contains(l, "speechify.com/sign-in") || strings.Contains(l, "speechify.com/signup") {
		return true
	}
	path := l
	if i := strings.Index(path, "://"); i >= 0 {
		if slash := strings.Index(path[i+3:], "/"); slash >= 0 {
			path = path[i+3+slash:]
		} else {
			return false
		}
	}
	if q := strings.Index(path, "?"); q >= 0 {
		path = path[:q]
	}
	path = strings.TrimSuffix(path, "/")
	return path == "/auth" || path == "/login" || path == "/signin" || path == "/sign-in"
}

func speechLoggedOutHTML(body []byte) bool {
	s := strings.ToLower(string(body))
	return strings.Contains(s, "welcome to speechify") && strings.Contains(s, "continue with google") && strings.Contains(s, "log in")
}

func speechUpstreamLogout(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	return speechAuthLocation(resp.Header.Get("Location")) || speechAuthLocation(resp.Header.Get("X-Inertia-Location"))
}

func noteSpeechLogout(cfg Config, r *http.Request, acc ToolAccount, sessionToken, reason string) (switched bool, next string) {
	if !usesPanelAccountMode(cfg) || acc.ID <= 0 {
		return false, ""
	}
	key := sessionToken
	if key == "" {
		key = "acc:" + acc.Name
	}
	speechLogoutMu.Lock()
	defer speechLogoutMu.Unlock()
	if prev, seen := speechSessionHop[key]; seen && time.Since(prev) < 2*time.Minute {
		last := speechLogoutSeen[acc.ID]
		if time.Since(prev) < 2*time.Second {
			return last.switched, last.next
		}
		return false, last.next
	}
	speechSessionHop[key] = time.Now()

	db, err := openPanelDB(cfg)
	if err != nil {
		log.Printf("[LB] logout db open failed: %v", err)
		delete(speechSessionHop, key)
		return false, ""
	}
	now := time.Now().UTC().Format(time.RFC3339)
	nextName := "(none)"
	skipIDs := []any{cfg.PublicHost, acc.ID}
	placeholders := "?"
	for id, hit := range speechLogoutSeen {
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

	websiteID, username, loginIP := speechLogoutMeta(db, cfg, sessionToken)
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
	speechLogoutSeen[acc.ID] = speechLogoutHit{at: time.Now(), next: nextName, switched: switched}
	return switched, nextName
}

func speechLogoutMeta(db *sql.DB, cfg Config, sessionToken string) (websiteID int, username, loginIP string) {
	if sessionToken != "" {
		_ = db.QueryRow(`SELECT website_id, username, COALESCE(client_ip, '') FROM live_sessions WHERE session_token=?`, sessionToken).Scan(&websiteID, &username, &loginIP)
	}
	if websiteID <= 0 {
		_ = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID)
	}
	return websiteID, username, loginIP
}

func speechHome(cfg Config) string {
	if cfg.HomePath != "" {
		return cfg.HomePath
	}
	return "/"
}

func serveSpeechLogout(w http.ResponseWriter, r *http.Request, cfg Config, acc ToolAccount, sessionToken, reason string) {
	for k := range w.Header() {
		w.Header().Del(k)
	}
	switched, _ := noteSpeechLogout(cfg, r, acc, sessionToken, reason)
	log.Printf("[LB] expired session path=%s account=%s switched=%v", r.URL.Path, acc.Name, switched)
	if switched {
		http.Redirect(w, r, speechHome(cfg), http.StatusFound)
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

func finishSpeechUpstreamLogout(w http.ResponseWriter, r *http.Request, resp *http.Response, cfg Config, acc ToolAccount, sessionToken string) {
	if resp != nil && resp.Body != nil {
		io.Copy(io.Discard, resp.Body)
	}
	serveSpeechLogout(w, r, cfg, acc, sessionToken, "session_expired")
}

func speechifyFileProxyHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	return strings.Contains(h, "speechify.com") && strings.Contains(h, "proxy")
}

// speechify sibling hosts (proxy.api, cdn, user-profile) must be fetched on
// this origin. The PDF worker calls them directly, and they only allow app.speechify.com.
var speechifyAbsHost = regexp.MustCompile(`(https?:|wss?:)(\\/\\/|//)((?:[a-z0-9-]+\.)+speechify\.com)`)

func rewriteSpeechifySiblingHosts(body []byte, publicBase string) []byte {
	if len(body) == 0 || !bytes.Contains(body, []byte("speechify.com")) {
		return body
	}
	baseHost := strings.TrimPrefix(strings.TrimPrefix(strings.TrimRight(publicBase, "/"), "https://"), "http://")
	if baseHost == "" {
		return body
	}
	return speechifyAbsHost.ReplaceAllFunc(body, func(match []byte) []byte {
		sub := speechifyAbsHost.FindSubmatch(match)
		if len(sub) < 4 {
			return match
		}
		host := strings.ToLower(string(sub[3]))
		switch host {
		case "app.speechify.com", "www.speechify.com", "speechify.com":
			return match
		}
		sep := string(sub[2])
		path := "/ext-host/" + host
		if sep == `\/\/` {
			return []byte(string(sub[1]) + `\/\/` + baseHost + strings.ReplaceAll(path, "/", `\/`))
		}
		return []byte(string(sub[1]) + "//" + baseHost + path)
	})
}

// restoreSpeechifyProxyTarget puts real hosts back inside a Speechify file-proxy
// URL. The page rewriter turns https://firebasestorage.googleapis.com into
// /extra-cdn-N, and proxy.api.speechify.com then rejects that address.
func restoreSpeechifyProxyTarget(raw string, cfg Config, publicHost string) string {
	hosts := []string{}
	addHost := func(h string) {
		h = strings.TrimSpace(h)
		if h == "" {
			return
		}
		for _, have := range hosts {
			if have == h {
				return
			}
		}
		hosts = append(hosts, h)
	}
	addHost(publicHost)
	addHost(cfg.PublicHost)
	out := raw
	for i, extra := range cfg.ExtraCDNDomains {
		originHost := strings.TrimPrefix(strings.TrimPrefix(extra, "https://"), "http://")
		originHost = strings.Split(originHost, "/")[0]
		if originHost == "" {
			continue
		}
		token := "/extra-cdn-" + strconv.Itoa(i)
		real := "https://" + originHost
		for _, h := range hosts {
			for _, scheme := range []string{"https", "http"} {
				base := scheme + "://" + h + token
				out = strings.ReplaceAll(out, base, real)
				out = strings.ReplaceAll(out, url.QueryEscape(base), url.QueryEscape(real))
			}
		}
	}
	return out
}

func speechLoginWatchHTML() string {
	return `<script data-tm-speech-login="1">
(function(){
  function isAuthURL(v){
    return /speechify\.com\/(?:auth|login|sign-in|signup)(?:[/?#]|$)/i.test(String(v || ""));
  }
  function loggedOut(){
    var t = "";
    try { t = (document.body.innerText || "").replace(/\s+/g, " ").toLowerCase(); } catch (e) {}
    return t.indexOf("welcome to speechify") !== -1 && t.indexOf("continue with google") !== -1 && t.indexOf("log in") !== -1;
  }
  function hop(){
    var last = 0;
    try { last = parseInt(sessionStorage.getItem("tm_speech_hop") || "0", 10) || 0; } catch (e) {}
    if (Date.now() - last < 8000) return;
    try { sessionStorage.setItem("tm_speech_hop", String(Date.now())); } catch (e) {}
    location.replace("/auth?_tm=" + Date.now());
  }
  try {
    var lp = Location.prototype;
    ["assign", "replace"].forEach(function(name){
      var orig = lp[name];
      if (!orig) return;
      lp[name] = function(u){
        if (isAuthURL(u)) { hop(); return; }
        return orig.apply(this, arguments);
      };
    });
    var d = Object.getOwnPropertyDescriptor(lp, "href");
    if (d && d.set && d.get) {
      Object.defineProperty(lp, "href", {
        configurable: true,
        enumerable: d.enumerable,
        get: function(){ return d.get.call(this); },
        set: function(v){ if (isAuthURL(v)) { hop(); return; } return d.set.call(this, v); }
      });
    }
  } catch (e) {}
  function go(){ if (loggedOut()) hop(); }
  go();
  setInterval(go, 1000);
})();
</script>`
}
