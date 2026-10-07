package main

import (
	"crypto/rand"
	"encoding/hex"
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
	setCtSessionCookie(w, r, cfg, sessionToken, exp)
	home := cfg.HomePath
	if home == "" {
		home = "/"
	}
	log.Printf("[PANEL] enter ok user=%s → %s", user, home)
	http.Redirect(w, r, home, http.StatusFound)
}
