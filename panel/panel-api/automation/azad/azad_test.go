package azad

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAzadPageSignals(t *testing.T) {
	login := `<form><input name="amember_login"><input name="amember_pass"></form>`
	if !azadShowsLogin(login, "https://members.azadseo.com/login") {
		t.Fatal("login form was not detected")
	}
	dash := `<a href="/logout">Logout</a><a href="https://semrush2.azadseo.com/?tool_access_at=1">Semrush</a>`
	if !azadShowsDashboard(dash, "https://members.azadseo.com/member") {
		t.Fatal("member dashboard was not detected")
	}
	if azadShowsDashboard(login, "https://members.azadseo.com/member") {
		t.Fatal("login form was treated as a dashboard")
	}
	got := azadLaunchURL(dash, "semrush2.azadseo.com")
	if !strings.Contains(got, "semrush2.azadseo.com") {
		t.Fatalf("launch = %s", got)
	}
}

func TestAzadHTTPLoginAndSemrushCapture(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`<input name="amember_login"><input name="login_attempt_id" value="attempt-1">`))
			return
		}
		if r.FormValue("amember_login") == "" || r.FormValue("amember_pass") == "" || r.FormValue("login_attempt_id") != "attempt-1" {
			http.Error(w, "bad login", http.StatusForbidden)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "PHPSESSID", Value: "portal-session", Path: "/", HttpOnly: true})
		http.Redirect(w, r, "/member", http.StatusFound)
	})
	mux.HandleFunc("/member", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			_, _ = w.Write([]byte(`<input name="amember_login">login to your account`))
			return
		}
		_, _ = w.Write([]byte(`<a href="/logout">Logout</a><a href="` + r.Host + `-semrush">x</a><a href="https://semrush2.azadseo.com/start?tool_access_at=9">Semrush</a>`))
	})
	portal := httptest.NewServer(mux)
	defer portal.Close()
	tool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			http.Error(w, "login to your account", http.StatusOK)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "semrush_session", Value: "tool-session", Path: "/"})
		_, _ = w.Write([]byte(`<a href="/logout">Logout</a> semrush ready`))
	}))
	defer tool.Close()

	previousBase, previousLogin, previousMember := azadBaseURL, azadLoginURL, azadMemberURL
	azadBaseURL = portal.URL
	azadLoginURL = portal.URL + "/login"
	azadMemberURL = portal.URL + "/member"
	t.Cleanup(func() {
		azadBaseURL, azadLoginURL, azadMemberURL = previousBase, previousLogin, previousMember
	})

	cookies, err := Recipe{}.Login(context.Background(), "azad-user", "azad-secret")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, cookie := range cookies {
		if cookie.Name == "PHPSESSID" && cookie.Value == "portal-session" {
			found = true
		}
		if strings.Contains(cookie.Value, "azad-secret") {
			t.Fatal("password was stored in a cookie")
		}
	}
	if !found {
		t.Fatal("homepage cookie was not stored")
	}
	valid, _, err := Recipe{}.Check(context.Background(), cookies)
	if err != nil || !valid {
		t.Fatalf("check valid=%v err=%v", valid, err)
	}

	// Point the member page link at this test tool by serving the launch from the tool host.
	// The recipe reads the semrush2 link, so replace that host check by using the tool URL directly.
	captured, err := Recipe{}.Capture(context.Background(), cookies, tool.URL)
	if err != nil {
		t.Fatal(err)
	}
	gotTool := false
	for _, cookie := range captured {
		if cookie.Name == "semrush_session" && cookie.Value == "tool-session" {
			gotTool = true
		}
	}
	if !gotTool {
		var names []string
		for _, cookie := range captured {
			names = append(names, cookie.Name)
		}
		t.Fatalf("semrush cookie missing, got %s", strings.Join(names, ","))
	}
}
