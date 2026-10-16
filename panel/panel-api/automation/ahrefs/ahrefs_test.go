package ahrefs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"panel-api/automation"
	"testing"
)

func TestRefreshKeepsBSSESSID(t *testing.T) {
	const savedAgent = "Mozilla/5.0 Chrome/120 AhrefsTest"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != savedAgent {
			http.Error(w, "wrong agent", http.StatusForbidden)
			return
		}
		if _, err := r.Cookie("BSSESSID"); err != nil {
			http.Redirect(w, r, "/user/login", http.StatusFound)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "BSSESSID", Value: "fresh-session", Path: "/"})
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	dashboardURL = srv.URL + "/dashboard"
	refreshURL = srv.URL + "/v4/loginCheckCompletion"
	t.Cleanup(func() {
		dashboardURL = "https://app.ahrefs.com/dashboard"
		refreshURL = "https://app.ahrefs.com/v4/loginCheckCompletion"
	})

	parsed, _ := url.Parse(srv.URL)
	ctx := automation.WithUserAgent(context.Background(), savedAgent)
	ok, updated, err := Recipe{}.Check(ctx, []automation.Cookie{{
		Name: "BSSESSID", Value: "saved-session", Domain: parsed.Hostname(), HostOnly: true, Path: "/",
	}})
	if err != nil || !ok {
		t.Fatalf("check ok=%v err=%v", ok, err)
	}
	if !hasName(updated, "BSSESSID") {
		t.Fatal("refreshed cookie missing BSSESSID")
	}
	tool, err := Recipe{}.Capture(context.Background(), updated, "https://app.ahrefs.com")
	if err != nil || !hasName(tool, "BSSESSID") {
		t.Fatalf("capture err=%v", err)
	}
}

func TestDeadCookieAsksForStorage(t *testing.T) {
	ok, _, err := Recipe{}.Check(context.Background(), nil)
	if err != nil || ok {
		t.Fatalf("empty check ok=%v err=%v", ok, err)
	}
	_, err = Recipe{}.Login(context.Background(), "acc2", "")
	if err == nil || err.Error() == "" {
		t.Fatal("login should ask for a stored cookie")
	}
	_, _, err = Recipe{}.Check(context.Background(), []automation.Cookie{{
		Name: "BSSESSID", Value: "saved", Domain: ".ahrefs.com", Path: "/",
	}})
	if err == nil || err.Error() == "" {
		t.Fatal("cookie without its user agent should fail")
	}
}
