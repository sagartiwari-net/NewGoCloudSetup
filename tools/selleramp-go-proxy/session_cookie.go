package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const ctSessionCookie = "ct_session"

type enterTicket struct {
	sessionToken string
	expires      time.Time
}

var enterTickets sync.Map // nonce → enterTicket

func issueEnterTicket(sessionToken string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	nonce := hex.EncodeToString(buf)
	enterTickets.Store(nonce, enterTicket{
		sessionToken: sessionToken,
		expires:      time.Now().Add(2 * time.Minute),
	})
	return nonce, nil
}

func consumeEnterTicket(nonce string) (string, bool) {
	nonce = strings.TrimSpace(nonce)
	if nonce == "" {
		return "", false
	}
	raw, ok := enterTickets.Load(nonce)
	if !ok {
		return "", false
	}
	enterTickets.Delete(nonce)
	t := raw.(enterTicket)
	if time.Now().After(t.expires) {
		return "", false
	}
	return t.sessionToken, true
}

func setCtSessionCookie(w http.ResponseWriter, r *http.Request, cfg Config, sessionToken string, expiry time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     ctSessionCookie,
		Value:    sessionToken,
		Path:     "/",
		Expires:  expiry,
		HttpOnly: true,
		Secure:   cookieSecure(r, cfg),
		SameSite: http.SameSiteLaxMode,
	})
}

// sessionEnterHandler finishes panel login on a same-site navigation.
// Panel → /access is cross-site; browsers often drop Set-Cookie there while still
// allowing the follow-up same-site /__tm_enter hop to store ct_session.
func sessionEnterHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	nonce := strings.TrimSpace(r.URL.Query().Get("n"))
	sessionToken, ok := consumeEnterTicket(nonce)
	if !ok {
		log.Printf("[PANEL] enter ticket invalid/missing")
		renderAccessDeniedPage(w, cfg)
		return
	}
	sess, ok := loadPanelSessionToken(sessionToken)
	if !ok {
		log.Printf("[PANEL] enter session missing")
		renderAccessDeniedPage(w, cfg)
		return
	}
	sess.mu.Lock()
	exp := sess.expires
	user := sess.username
	sess.mu.Unlock()
	// Drop any stale ct_session first, then set the live one on a 200 document.
	// Set-Cookie on 302 redirects is frequently ignored by browsers/CDNs — that
	// left ct_candidates=1 with a dead orphan token and "session not found" on /.
	clearStaleCtSessionCookies(w, r, cfg)
	setCtSessionCookie(w, r, cfg, sessionToken, exp)
	home := cfg.HomePath
	if home == "" {
		home = "/"
	}
	log.Printf("[PANEL] enter ok user=%s → %s (cookie via 200)", user, home)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	esc := html.EscapeString(home)
	fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8">
<meta http-equiv="refresh" content="0;url=%s">
<title>Signing in…</title></head><body>
<script>location.replace(%q)</script>
<p>Signing in… <a href="%s">continue</a></p>
</body></html>`, esc, home, esc)
}
