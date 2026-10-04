package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPanelDeviceProofKillsCopiedSession(t *testing.T) {
	token, _, err := issuePanelSession("sagar", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{PanelDB: "panel.db", ToolName: "ChatGPT", HomePath: "/"}

	bindReq := httptest.NewRequest(http.MethodPost, "/api/device-bind", nil)
	bindReq.AddCookie(&http.Cookie{Name: "ct_session", Value: token})
	bindReq.Header.Set("X-Device-Fp", "fp-mac")
	bindReq.Header.Set("X-Device-Proof", "proof-chrome")
	bindRec := httptest.NewRecorder()
	deviceBindHandler(bindRec, bindReq)
	if bindRec.Code != http.StatusOK {
		t.Fatalf("bind status %d body %s", bindRec.Code, bindRec.Body.String())
	}

	same := httptest.NewRequest(http.MethodGet, "/", nil)
	same.Header.Set("Accept", "text/html")
	same.Header.Set("X-Device-Fp", "fp-mac")
	same.Header.Set("X-Device-Proof", "proof-chrome")
	same.AddCookie(&http.Cookie{Name: "ct_session", Value: token})
	sameRec := httptest.NewRecorder()
	if rejectPanelDevice(sameRec, same, cfg) {
		t.Fatal("matching proof was rejected")
	}

	copied := httptest.NewRequest(http.MethodGet, "/", nil)
	copied.Header.Set("Accept", "text/html")
	copied.Header.Set("Sec-Fetch-Mode", "navigate")
	copied.Header.Set("Sec-Fetch-Dest", "document")
	copied.AddCookie(&http.Cookie{Name: "ct_session", Value: token})
	copiedRec := httptest.NewRecorder()
	if rejectPanelDevice(copiedRec, copied, cfg) {
		t.Fatal("refresh cannot send device headers and must still open the tool")
	}
	if _, err := panelSessionUsername(copied); err != nil {
		t.Fatal("session was killed before the copied profile proved it had no device secret")
	}

	bad := httptest.NewRequest(http.MethodPost, "/api/device-bind", nil)
	bad.AddCookie(&http.Cookie{Name: "ct_session", Value: token})
	bad.Header.Set("X-Device-Fp", "missing")
	bad.Header.Set("X-Device-Proof", "missing")
	badRec := httptest.NewRecorder()
	deviceBindHandler(badRec, bad)
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("missing proof bind status %d", badRec.Code)
	}

	again := httptest.NewRequest(http.MethodGet, "/", nil)
	again.Header.Set("Accept", "text/html")
	again.Header.Set("X-Device-Fp", "fp-mac")
	again.Header.Set("X-Device-Proof", "proof-chrome")
	again.AddCookie(&http.Cookie{Name: "ct_session", Value: token})
	againRec := httptest.NewRecorder()
	if _, err := panelSessionUsername(again); err == nil {
		t.Fatal("original session still alive after copied profile")
	}
	if rejectPanelDevice(againRec, again, cfg) {
		t.Fatal("dead session should be handled by the auth gate, not the device gate")
	}
}

func TestDirectVisitStillDeniedWithoutSession(t *testing.T) {
	cfg := loadConfig()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	if _, err := panelSessionUsername(req); err == nil {
		t.Fatal("anonymous request looked authenticated")
	}
	if rejectPanelDevice(rec, req, cfg) {
		t.Fatal("device gate should not run without a session")
	}
}

func TestPanelCreditDefaultAndCustom(t *testing.T) {
	state, err := panelCreditStateFor("credit-probe-user")
	if err != nil {
		t.Fatal(err)
	}
	if state.limit != 100 {
		t.Fatalf("default credits = %d", state.limit)
	}
	if !panelCreditsAllow("credit-probe-user") {
		t.Fatal("unused user was blocked")
	}
	panelCreditsCharge("credit-probe-user", "/backend-api/f/conversation")
	db, err := openPanelDB(loadConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(`DELETE FROM usage_events WHERE username=?`, "credit-probe-user")
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM usage_events WHERE username=? AND limit_key='credits'`, "credit-probe-user").Scan(&n)
	if n != 1 {
		t.Fatalf("charged events = %d", n)
	}
}

func TestPanelEndDropsTrackedSession(t *testing.T) {
	token, _, err := issuePanelSession("sagar-ended", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	raw, ok := panelSess.Load(token)
	if !ok {
		t.Fatal("missing memory session")
	}
	sess := raw.(*panelGateSession)
	sess.mu.Lock()
	sess.tracked = true
	sess.mu.Unlock()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	req.AddCookie(&http.Cookie{Name: "ct_session", Value: token})
	if _, err := panelSessionUsername(req); err == nil {
		t.Fatal("session stayed open after the panel row was gone")
	}
	if _, ok := panelSess.Load(token); ok {
		t.Fatal("memory session kept after panel end")
	}
}
