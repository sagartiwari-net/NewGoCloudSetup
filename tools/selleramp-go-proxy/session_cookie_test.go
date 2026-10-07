package main

import (
	"errors"
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
	body := rec.Body.String()
	if !strings.Contains(body, `location.replace("/")`) {
		t.Fatalf("missing JS redirect, body=%s", body)
	}
	if !strings.Contains(body, `setTimeout`) {
		t.Fatalf("expected delayed redirect, body=%s", body)
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

func TestDeviceBootScriptBootstrapsQuery(t *testing.T) {
	script := deviceBootScript("/", "sess-token", "nonce123")
	for _, want := range []string{
		`__tm_s=`,
		`tm_ct_session`,
		`X-Ct-Session`,
		`sess-token`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("boot script missing %q", want)
		}
	}
}

func TestCtSessionCandidatesPrefersBootstrapQuery(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/?__tm_s=live-bootstrap", nil)
	r.AddCookie(&http.Cookie{Name: "ct_session", Value: "dead-orphan"})
	cands := ctSessionCandidates(r)
	if len(cands) < 2 || cands[0] != "live-bootstrap" {
		t.Fatalf("want bootstrap first, got %v", cands)
	}
}

func TestSessionKeepaliveScriptPatchesNav(t *testing.T) {
	s := sessionKeepaliveScript()
	for _, want := range []string{
		`data-tm-sess`,
		`__tm_s`,
		`tm_ct_session`,
		`tm_device_proof`,
		`X-Ct-Session`,
		`X-Device-Proof`,
		`denyStolen`,
		`addEventListener("submit"`,
		`addEventListener("click"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("keepalive missing %q", want)
		}
	}
}

func TestBindPanelDeviceRejectsCookieShare(t *testing.T) {
	token := "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	panelSess.Store(token, &panelGateSession{username: "sagar", expires: time.Now().Add(time.Hour), proof: "bound-proof"})
	defer panelSess.Delete(token)
	err := bindPanelDevice(token, "missing", "missing")
	if !errors.Is(err, errDeviceCookieShare) {
		t.Fatalf("want cookie_share, got %v", err)
	}
	err = bindPanelDevice(token, "fp", "other-proof")
	if !errors.Is(err, errDeviceCookieShare) {
		t.Fatalf("want cookie_share on mismatch, got %v", err)
	}
}

func TestIsStaticAssetPath(t *testing.T) {
	for _, p := range []string{
		"/assets/5d8cc971b-cc3951233e.css",
		"/images/sas-logo-color-228x30.png",
		"/js/app.js",
		"/favicon.ico",
	} {
		if !isStaticAssetPath(p) {
			t.Fatalf("expected static: %s", p)
		}
	}
	for _, p := range []string{"/", "/r/sas/lookup", "/api/device-bind"} {
		if isStaticAssetPath(p) {
			t.Fatalf("expected non-static: %s", p)
		}
	}
}

func TestClearParentDomainDoesNotExpireHost(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	cfg := Config{PublicScheme: "https"}
	clearParentDomainCtSessionCookies(rec, req, cfg)
	for _, c := range rec.Result().Cookies() {
		if c.Name == "ct_session" && c.Domain == "" && c.MaxAge < 0 {
			t.Fatal("parent clear must not emit host-only Max-Age=0 (wipes live Set-Cookie)")
		}
	}
	foundParent := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == "ct_session" && strings.Contains(c.Domain, "gt4rents.com") {
			foundParent = true
		}
	}
	if !foundParent {
		t.Fatal("expected Domain=gt4rents.com clear")
	}
}
