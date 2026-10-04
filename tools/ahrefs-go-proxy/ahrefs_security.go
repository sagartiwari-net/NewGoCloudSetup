package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"html"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ahrefsGateSession struct {
	mu          sync.Mutex
	username    string
	expires     time.Time
	fp          string
	proof       string
	tracked     bool
	liveChecked time.Time
	liveOK      bool
	duration    time.Duration
}

var ahrefsSessions sync.Map

func serveAhrefsPanelAccess(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	user := r.URL.Query().Get("user")
	token := r.URL.Query().Get("token")
	if user == "" || token == "" {
		renderAhrefsDenied(w)
		return
	}
	db, err := openAhrefsPanel()
	if err != nil {
		log.Printf("[PANEL] database open failed: %v", err)
		renderAhrefsDenied(w)
		return
	}
	var dbUser, clientIP string
	err = db.QueryRow(`DELETE FROM access_tokens
		WHERE token = ? AND expires_at > ?
		AND website_id IN (SELECT id FROM websites WHERE domain IN ('127.0.0.1:5291', ?))
		RETURNING username, COALESCE(client_ip, '')`,
		token, time.Now().UTC().Format(time.RFC3339), cfg.PublicHost).Scan(&dbUser, &clientIP)
	if err != nil || dbUser != user {
		log.Printf("[PANEL] token rejected user=%s err=%v", user, err)
		renderAhrefsDenied(w)
		return
	}
	var status string
	_ = db.QueryRow(`SELECT status FROM panel_users WHERE website_id IN (SELECT id FROM websites WHERE domain IN ('127.0.0.1:5291', ?)) AND username=?`,
		cfg.PublicHost, user).Scan(&status)
	if status == "suspended" {
		renderAhrefsDenied(w)
		return
	}
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		renderAhrefsDenied(w)
		return
	}
	sessionToken := hex.EncodeToString(buf)
	websiteID, minutes := ahrefsSite(db, cfg.PublicHost)
	if minutes < 1 {
		minutes = 120
	}
	expires := time.Now().Add(time.Duration(minutes) * time.Minute)
	seen := ahrefsClientIP(r, clientIP)
	ahrefsSessions.Store(sessionToken, &ahrefsGateSession{
		username: user, expires: expires, tracked: true, liveOK: true,
		duration: time.Duration(minutes) * time.Minute,
	})
	recordAhrefsLogin(db, websiteID, user, sessionToken, seen, r.UserAgent(), expires)
	http.SetCookie(w, &http.Cookie{
		Name: "ahrefs_session", Value: sessionToken, Path: "/", Expires: expires,
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	log.Printf("[PANEL] access granted user=%s ip=%s", user, seen)
	page := ahrefsLightPage("Authenticating", "Authenticating...",
		`You are using <span class="brand">Ahrefs</span>. Please wait a moment while we verify your secure access request.`,
		`<div class="pill"><span class="dot"></span>Verifying your request...</div><p class="foot">Secure session initialization in progress</p>`,
		true, ahrefsBootScript())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func ahrefsSite(db *sql.DB, publicHost string) (websiteID, minutes int) {
	_ = db.QueryRow(`SELECT id, COALESCE(session_duration, 120) FROM websites WHERE domain IN ('127.0.0.1:5291', ?) LIMIT 1`, publicHost).Scan(&websiteID, &minutes)
	return websiteID, minutes
}

func ahrefsClientIP(r *http.Request, tokenIP string) string {
	seen := realClientIP(r)
	if seen == "" || seen == "127.0.0.1" || seen == "::1" {
		if strings.TrimSpace(tokenIP) != "" {
			return tokenIP
		}
	}
	return seen
}

func recordAhrefsLogin(db *sql.DB, websiteID int, username, sessionToken, clientIP, userAgent string, expires time.Time) {
	if websiteID == 0 {
		return
	}
	var userID int
	err := db.QueryRow(`SELECT id FROM panel_users WHERE website_id=? AND username=?`, websiteID, username).Scan(&userID)
	if err != nil {
		res, insertErr := db.Exec(`INSERT INTO panel_users (website_id, username, status, custom_limit_expire_at) VALUES (?,?,?,NULL)`, websiteID, username, "active")
		if insertErr == nil {
			id, _ := res.LastInsertId()
			userID = int(id)
		}
	}
	_ = userID
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = db.Exec(`DELETE FROM live_sessions WHERE username=? AND website_id=?`, username, websiteID)
	_, err = db.Exec(`INSERT INTO live_sessions (website_id, session_token, username, client_ip, fingerprint, expires_at, created_at, assigned_account_id)
		VALUES (?,?,?,?,?,?,?,?)`, websiteID, sessionToken, username, clientIP, "", expires.UTC().Format(time.RFC3339), now, 0)
	if err != nil {
		log.Printf("[PANEL] live session insert failed: %v", err)
		return
	}
	if _, err = db.Exec(`INSERT INTO login_events (website_id, username, client_ip, user_agent, logged_in_at) VALUES (?,?,?,?,?)`,
		websiteID, username, clientIP, userAgent, now); err != nil {
		log.Printf("[PANEL] login event insert failed: %v", err)
	}
	noteAhrefsAccess(db, websiteID, username, clientIP, userAgent)
}

func ahrefsSessionOK(r *http.Request) bool {
	cookie, err := r.Cookie("ahrefs_session")
	if err != nil || cookie.Value == "" {
		return false
	}
	raw, ok := ahrefsSessions.Load(cookie.Value)
	if !ok {
		if !restoreAhrefsSession(cookie.Value) {
			return false
		}
		raw, ok = ahrefsSessions.Load(cookie.Value)
		if !ok {
			return false
		}
	}
	sess := raw.(*ahrefsGateSession)
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if time.Now().After(sess.expires) {
		ahrefsSessions.Delete(cookie.Value)
		return false
	}
	if sess.tracked {
		if time.Since(sess.liveChecked) > 3*time.Second {
			sess.liveChecked = time.Now()
			sess.liveOK = ahrefsLiveExists(cookie.Value)
		}
		if !sess.liveOK {
			ahrefsSessions.Delete(cookie.Value)
			return false
		}
	}
	return true
}

func restoreAhrefsSession(token string) bool {
	db, err := openAhrefsPanel()
	if err != nil {
		return false
	}
	var username, expRaw, fp string
	err = db.QueryRow(`SELECT username, expires_at, COALESCE(fingerprint, '') FROM live_sessions WHERE session_token=?`, token).Scan(&username, &expRaw, &fp)
	if err != nil {
		return false
	}
	expires, err := time.Parse(time.RFC3339, expRaw)
	if err != nil || time.Now().After(expires) {
		return false
	}
	ahrefsSessions.Store(token, &ahrefsGateSession{username: username, expires: expires, fp: fp, tracked: true, liveOK: true, duration: 120 * time.Minute})
	return true
}

func ahrefsLiveExists(token string) bool {
	db, err := openAhrefsPanel()
	if err != nil {
		return true
	}
	var id int
	return db.QueryRow(`SELECT id FROM live_sessions WHERE session_token=?`, token).Scan(&id) == nil
}

func handleAhrefsDeviceBind(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	cookie, err := r.Cookie("ahrefs_session")
	if err != nil || cookie.Value == "" {
		log.Printf("[DEVICE] bind rejected: no ahrefs_session cookie")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"ok":false,"error":"no_session"}`)
		return
	}
	raw, ok := ahrefsSessions.Load(cookie.Value)
	if !ok {
		log.Printf("[DEVICE] bind rejected: unknown session")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"ok":false,"error":"unknown_session"}`)
		return
	}
	fp := strings.TrimSpace(r.Header.Get("X-Device-Fp"))
	proof := strings.TrimSpace(r.Header.Get("X-Device-Proof"))
	if err = bindAhrefsDevice(cookie.Value, raw.(*ahrefsGateSession), fp, proof); err != nil {
		log.Printf("[DEVICE] bind rejected: %v", err)
		if strings.Contains(err.Error(), "device mismatch") {
			recordAhrefsCookieShare(r, cookie.Value)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"ok":false,"error":"bind_failed"}`)
		return
	}
	log.Printf("[DEVICE] bind ok")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"ok"}`)
}

func bindAhrefsDevice(token string, sess *ahrefsGateSession, fp, proof string) error {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if time.Now().After(sess.expires) {
		ahrefsSessions.Delete(token)
		return fmt.Errorf("session expired")
	}
	if fp == "" || proof == "" {
		return fmt.Errorf("missing proof")
	}
	if fp == "missing" || proof == "missing" || (sess.proof != "" && (sess.proof != proof || sess.fp != fp)) || (sess.fp != "" && sess.proof == "" && sess.fp != fp) {
		ahrefsSessions.Delete(token)
		return fmt.Errorf("device mismatch")
	}
	if sess.proof == "" {
		sess.fp = fp
		sess.proof = proof
	}
	if db, err := openAhrefsPanel(); err == nil {
		_, _ = db.Exec(`UPDATE live_sessions SET fingerprint=? WHERE session_token=?`, fp, token)
	}
	return nil
}

func ahrefsRejectDevice(w http.ResponseWriter, r *http.Request) bool {
	cookie, err := r.Cookie("ahrefs_session")
	if err != nil || cookie.Value == "" {
		return false
	}
	raw, ok := ahrefsSessions.Load(cookie.Value)
	if !ok {
		return false
	}
	sess := raw.(*ahrefsGateSession)
	fp := strings.TrimSpace(r.Header.Get("X-Device-Fp"))
	proof := strings.TrimSpace(r.Header.Get("X-Device-Proof"))
	sess.mu.Lock()
	bound := sess.proof != ""
	storedFp, storedProof, username := sess.fp, sess.proof, sess.username
	sess.mu.Unlock()
	if !bound || (fp == storedFp && proof == storedProof) || ahrefsSubresource(r) {
		return false
	}
	if fp == "" && proof == "" {
		if ahrefsDocument(r) {
			return false
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"device_required","message":"Open this tool again from your access link."}`)
		return true
	}
	ahrefsSessions.Delete(cookie.Value)
	recordAhrefsCookieShare(r, cookie.Value)
	log.Printf("[DEVICE] session killed user=%s", username)
	if ahrefsDocument(r) || strings.Contains(r.Header.Get("Accept"), "text/html") {
		renderAhrefsDenied(w)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"device_mismatch","message":"Open this tool again from your access link."}`)
	}
	return true
}

func ahrefsSubresource(r *http.Request) bool {
	switch strings.ToLower(r.Header.Get("Sec-Fetch-Dest")) {
	case "image", "style", "font", "audio", "video", "script":
		return true
	}
	return false
}

func ahrefsDocument(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	mode := r.Header.Get("Sec-Fetch-Mode")
	dest := r.Header.Get("Sec-Fetch-Dest")
	if mode == "navigate" || dest == "document" || dest == "iframe" {
		return true
	}
	return mode == "" && dest == "" && strings.Contains(r.Header.Get("Accept"), "text/html")
}

func recordAhrefsCookieShare(r *http.Request, token string) {
	db, err := openAhrefsPanel()
	if err != nil {
		return
	}
	var websiteID int
	var username, ip string
	err = db.QueryRow(`SELECT website_id, username, COALESCE(client_ip, '') FROM live_sessions WHERE session_token=?`, token).Scan(&websiteID, &username, &ip)
	_, _ = db.Exec(`DELETE FROM live_sessions WHERE session_token=?`, token)
	if err != nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = db.Exec(`INSERT INTO security_events (website_id, username, client_ip, event_type, attempted_url, details, user_agent, created_at)
		VALUES (?,?,?,?,?,?,?,?)`, websiteID, username, ip, "cookie_share", r.URL.RequestURI(),
		"Copied the session cookie into another browser. Both browsers were signed out.", r.UserAgent(), now)
	log.Printf("[PANEL] cookie share recorded user=%s", username)
}

func noteAhrefsAccess(db *sql.DB, websiteID int, username, clientIP, userAgent string) {
	now := time.Now().UTC()
	var same int
	_ = db.QueryRow(`SELECT COUNT(*) FROM login_events WHERE username=? AND website_id=? AND logged_in_at>=?`,
		username, websiteID, now.Add(-30*time.Minute).Format(time.RFC3339)).Scan(&same)
	if same < 4 {
		return
	}
	reason := "Ahrefs opened " + strconv.Itoa(same) + " times in 30 minutes"
	var recent int
	_ = db.QueryRow(`SELECT COUNT(*) FROM security_events WHERE username=? AND event_type='access_pattern' AND details=? AND created_at>=?`,
		username, reason, now.Add(-30*time.Minute).Format(time.RFC3339)).Scan(&recent)
	if recent > 0 {
		return
	}
	_, _ = db.Exec(`INSERT INTO security_events (website_id, username, client_ip, event_type, attempted_url, details, user_agent, created_at)
		VALUES (?,?,?,?,?,?,?,?)`, websiteID, username, clientIP, "access_pattern", "", reason, userAgent, now.Format(time.RFC3339))
}

func serveAhrefsDeviceSW(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("self.addEventListener('fetch', function () { return; });\n"))
}

func renderAhrefsDenied(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	page := ahrefsLightPage("Access Denied", "Access Denied",
		`You cannot open <span class="brand">Ahrefs</span> directly. Open it from your access link.`,
		`<p class="foot">A direct visit is not allowed</p>`, false, "")
	_, _ = w.Write([]byte(page))
}

func ahrefsLightPage(title, heading, message, extra string, spin bool, script string) string {
	ring := "ring"
	if spin {
		ring = "ring spin"
	}
	scriptTag := ""
	if script != "" {
		scriptTag = "<script>" + script + "</script>"
	}
	return `<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1.0"><title>` + html.EscapeString(title) + `</title>
<style>*{box-sizing:border-box;margin:0;padding:0}body{min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;background:#eef3f8;color:#0f172a;font-family:system-ui,-apple-system,Segoe UI,sans-serif}.card{width:min(440px,100%);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px 36px;text-align:center}.ring{width:78px;height:78px;margin:0 auto 22px;border-radius:50%;background:conic-gradient(#3b82f6 0 70deg,#e7eef8 70deg 360deg);display:grid;place-items:center}.ring.spin{animation:spin .9s linear infinite}.lock{width:64px;height:64px;border-radius:50%;background:#fff;display:grid;place-items:center;font-size:26px}.ring.spin .lock{animation:spin .9s linear infinite reverse}h1{font-size:28px;line-height:1.2;font-weight:800;letter-spacing:-.03em;margin-bottom:12px}.msg{color:#64748b;font-size:15px;line-height:1.55}.brand{color:#2563eb;font-weight:700}.pill{margin:22px auto 0;display:inline-flex;align-items:center;gap:8px;padding:8px 14px;border:1px solid #e6ebf2;border-radius:999px;color:#334155;font-size:14px;background:#fff}.dot{width:14px;height:14px;border-radius:50%;border:2px solid #dbe4f0;border-top-color:#3b82f6;animation:spin .8s linear infinite}.foot{margin-top:18px;color:#94a3b8;font-size:13px}@keyframes spin{to{transform:rotate(360deg)}}</style></head>
<body><div class="card"><div class="` + ring + `"><div class="lock">&#128274;</div></div><h1>` + html.EscapeString(heading) + `</h1><p class="msg">` + message + `</p>` + extra + `</div>` + scriptTag + `</body></html>`
}

func ahrefsBootScript() string {
	// NOTE: crypto.subtle only exists on HTTPS (secure context). On plain HTTP it throws
	// and the UI shows Access Denied after a successful token — so we fallback-hash on HTTP.
	return `(function(){
function proof(){var k="tm_device_proof";var e="";try{e=localStorage.getItem(k)||"";}catch(x){}if(e)return Promise.resolve(e);var e2="";try{var b=new Uint8Array(32);(crypto.getRandomValues||function(){for(var i=0;i<32;i++)b[i]=Math.floor(Math.random()*256);})(b);e2=Array.from(b).map(function(n){return n.toString(16).padStart(2,"0");}).join("");}catch(x){e2=String(Date.now())+Math.random().toString(16).slice(2);}try{localStorage.setItem(k,e2);}catch(x){}return Promise.resolve(e2);}
function weakHash(s){var h=0;for(var i=0;i<s.length;i++){h=((h<<5)-h)+s.charCodeAt(i);h|=0;}var out="";for(var j=0;j<8;j++){out+=((h>>> (j*4)) & 15).toString(16);h=(h*1664525+1013904223)|0;}while(out.length<64)out+=out;return out.slice(0,64);}
function fp(){var c=document.createElement("canvas");c.width=220;c.height=30;var g=c.getContext("2d");var sample="";if(g){g.textBaseline="top";g.font="14px Arial";g.fillStyle="#f60";g.fillRect(0,0,220,30);g.fillStyle="#069";g.fillText("tm-fp",2,2);try{sample=c.toDataURL().slice(-48);}catch(e){sample="x";}}var zone="";try{zone=Intl.DateTimeFormat().resolvedOptions().timeZone||"";}catch(e){}var raw=[navigator.userAgent||"",navigator.platform||"",navigator.language||"",String(navigator.hardwareConcurrency||0),String(screen.width)+"x"+String(screen.height),zone,sample].join("|");if(window.crypto&&crypto.subtle&&window.isSecureContext){return crypto.subtle.digest("SHA-256", new TextEncoder().encode(raw)).then(function(buf){return Array.from(new Uint8Array(buf)).map(function(b){return b.toString(16).padStart(2,"0");}).join("");});}return Promise.resolve(weakHash(raw));}
function fail(err){var h=document.querySelector("h1");var m=document.querySelector(".msg");if(h)h.textContent="Access Denied";if(m)m.textContent="Open this tool again from your access link.";(window.console&&console.warn&&console.warn("ahrefs boot",err));}
proof().then(function(p){return fp().then(function(f){try{localStorage.setItem("tm_device_fp",f);}catch(e){}return fetch("/api/device-bind",{method:"POST",credentials:"same-origin",headers:{"X-Device-Fp":f,"X-Device-Proof":p}});});}).then(function(res){if(!res.ok)throw new Error("bind "+res.status);location.replace("/dashboard");}).catch(fail);
})();`
}

func injectAhrefsDeviceScript(body string) string {
	if strings.Contains(body, "data-tm-device") {
		return body
	}
	script := `<style data-tm-device>html{visibility:hidden !important}</style><script data-tm-device>
function tmDeny(){document.documentElement.style.cssText="visibility:visible;background:#eef3f8;margin:0";while(document.documentElement.firstChild)document.documentElement.removeChild(document.documentElement.firstChild);var b=document.createElement("body");b.style.cssText="min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;margin:0;background:#eef3f8;font-family:system-ui,sans-serif";b.innerHTML='<div style="width:min(440px,100%);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px;text-align:center"><div style="font-size:28px">&#128274;</div><h1 style="font-size:28px;margin:12px 0">Access Denied</h1><p style="color:#64748b">You cannot open <span style="color:#2563eb;font-weight:700">Ahrefs</span> directly. Open it from your access link.</p><p style="margin-top:18px;color:#94a3b8;font-size:13px">A direct visit is not allowed</p></div>';document.documentElement.appendChild(b);}
function tmReveal(){document.documentElement.style.visibility="visible";var s=document.querySelector("style[data-tm-device]");if(s)s.remove();}
function tmPatch(fp,proof){if(window.__tmDevicePatched)return;window.__tmDevicePatched=true;var orig=window.fetch;window.fetch=function(input,init){var url=typeof input==="string"?input:(input&&input.url)||"";var same=false;try{same=new URL(url,location.href).origin===location.origin;}catch(e){}if(same){init=init||{};var h=new Headers(init.headers||(input&&input.headers)||undefined);if(!h.get("X-Device-Fp"))h.set("X-Device-Fp",fp);if(!h.get("X-Device-Proof"))h.set("X-Device-Proof",proof);init.headers=h;}return orig.call(this,input,init);};}
(function(){var proof="";try{proof=localStorage.getItem("tm_device_proof")||"";}catch(e){}if(!proof){fetch("/api/device-bind",{method:"POST",credentials:"same-origin",headers:{"X-Device-Fp":"missing","X-Device-Proof":"missing"}}).finally(tmDeny);return;}tmReveal();var fp="";try{fp=localStorage.getItem("tm_device_fp")||"";}catch(e){}if(fp){tmPatch(fp,proof);setInterval(function(){fetch("/api/device-bind",{method:"POST",credentials:"same-origin",headers:{"X-Device-Fp":fp,"X-Device-Proof":proof}}).then(function(r){if(!r.ok)tmDeny();});},2000);}})();
</script>`
	lower := strings.ToLower(body)
	if h := strings.Index(lower, "<head"); h >= 0 {
		if gt := strings.IndexByte(lower[h:], '>'); gt >= 0 {
			at := h + gt + 1
			return body[:at] + script + body[at:]
		}
	}
	return script + body
}
