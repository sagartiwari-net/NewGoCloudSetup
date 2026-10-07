package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCtSessionCandidatesPicksAll(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: "ct_session", Value: "dead"})
	r.AddCookie(&http.Cookie{Name: "other", Value: "x"})
	r.Header.Set("X-Ct-Session", "from-header")
	// Second cookie with same name (parent-domain style) — net/http keeps both via AddCookie
	r.AddCookie(&http.Cookie{Name: "ct_session", Value: "live"})

	cands := ctSessionCandidates(r)
	if len(cands) < 2 {
		t.Fatalf("expected >=2 candidates, got %v", cands)
	}
	foundHeader := false
	for _, c := range cands {
		if c == "from-header" {
			foundHeader = true
		}
	}
	if !foundHeader {
		t.Fatalf("missing X-Ct-Session in %v", cands)
	}
}

func TestResolveCtSessionPrefersLive(t *testing.T) {
	live := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	dead := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	panelSess.Store(live, &panelGateSession{username: "sagar", expires: time.Now().Add(time.Hour)})
	defer panelSess.Delete(live)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: "ct_session", Value: dead})
	r.AddCookie(&http.Cookie{Name: "ct_session", Value: live})

	tok, sess, ok := resolveCtSession(r)
	if !ok || sess == nil {
		t.Fatalf("expected live session, ok=%v", ok)
	}
	if tok != live {
		t.Fatalf("got token %q want live", tok)
	}
	if sess.username != "sagar" {
		t.Fatalf("username=%q", sess.username)
	}
}

func TestEnterTicketRoundTrip(t *testing.T) {
	nonce, err := issueEnterTicket("sess-token-1")
	if err != nil || nonce == "" {
		t.Fatalf("issue: %v nonce=%q", err, nonce)
	}
	got, ok := consumeEnterTicket(nonce)
	if !ok || got != "sess-token-1" {
		t.Fatalf("consume got=%q ok=%v", got, ok)
	}
	if _, ok := consumeEnterTicket(nonce); ok {
		t.Fatal("ticket should be one-time")
	}
}

func TestSessionEnterSetsCookieAndRedirects(t *testing.T) {
	token := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	panelSess.Store(token, &panelGateSession{username: "sagar", expires: time.Now().Add(time.Hour)})
	defer panelSess.Delete(token)
	nonce, err := issueEnterTicket(token)
	if err != nil {
		t.Fatal(err)
	}

	oldCfg, oldMod := currentConfig, configModTime
	currentConfig = Config{PublicScheme: "http", PublicHost: "selleramp.test", HomePath: "/", ToolName: "SellerAmp SAS"}
	configModTime = time.Now().Add(time.Hour) // prevent reload from disk
	defer func() {
		currentConfig = oldCfg
		configModTime = oldMod
	}()

	req := httptest.NewRequest(http.MethodGet, "/__tm_enter?n="+nonce, nil)
	rec := httptest.NewRecorder()
	sessionEnterHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `location.replace("/")`) {
		t.Fatalf("missing JS redirect, body=%s", rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == ctSessionCookie && c.Value == token {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing ct_session Set-Cookie, got %#v", cookies)
	}
}
