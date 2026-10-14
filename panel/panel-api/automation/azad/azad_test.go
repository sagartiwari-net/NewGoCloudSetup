package azad

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"panel-api/automation"
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
	launchHref := ""
	mux.HandleFunc("/member", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			_, _ = w.Write([]byte(`<input name="amember_login">login to your account`))
			return
		}
		_, _ = w.Write([]byte(`<a href="/logout">Logout</a><a href="` + launchHref + `">Semrush</a>`))
	})
	portal := httptest.NewServer(mux)
	defer portal.Close()
	tool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			http.Error(w, "login to your account", http.StatusOK)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "proxy_token", Value: "tool-session", Path: "/", HttpOnly: true})
		http.SetCookie(w, &http.Cookie{Name: "amember_nr", Value: "portal-token", Path: "/"})
		_, _ = w.Write([]byte(`<a href="/logout">Logout</a> semrush ready`))
	}))
	defer tool.Close()
	launchHref = tool.URL + "/start?tool_access_at=9"

	previousBase, previousLogin, previousMember, previousProxy := azadBaseURL, azadLoginURL, azadMemberURL, azadProxyHost
	azadBaseURL = portal.URL
	azadLoginURL = portal.URL + "/login"
	azadMemberURL = portal.URL + "/member"
	toolURL, err := url.Parse(tool.URL)
	if err != nil {
		t.Fatal(err)
	}
	azadProxyHost = toolURL.Host
	t.Cleanup(func() {
		azadBaseURL, azadLoginURL, azadMemberURL, azadProxyHost = previousBase, previousLogin, previousMember, previousProxy
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

	captured, err := Recipe{}.Capture(context.Background(), cookies, tool.URL)
	if err != nil {
		t.Fatal(err)
	}
	gotTool := false
	for _, cookie := range captured {
		if cookie.Name == "PHPSESSID" || strings.Contains(strings.ToLower(cookie.Name), "amember") {
			t.Fatalf("portal cookie %s was written to semrush", cookie.Name)
		}
		if cookie.Name == "proxy_token" && cookie.Value == "tool-session" {
			gotTool = true
			if cookie.Domain != ".xemrush.site" {
				t.Fatalf("proxy cookie domain = %s", cookie.Domain)
			}
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

func TestAzadSemrushKeepsOnlyTheProxyCookie(t *testing.T) {
	portal := []automation.Cookie{
		{Name: "PHPSESSID", Value: "portal-session", Domain: ".azadseo.com"},
		{Name: "amember_nr", Value: "member-token", Domain: ".azadseo.com"},
	}
	mapped := []automation.Cookie{
		{Name: "PHPSESSID", Value: "portal-session", Domain: ".xemrush.site"},
		{Name: "amember_nr", Value: "member-token", Domain: ".xemrush.site"},
	}
	if got := semrushFreshCookies(mapped, portal); len(got) != 0 {
		t.Fatal("portal cookies were treated as the semrush cookie")
	}
	mapped = append(mapped, automation.Cookie{Name: "proxy_token", Value: "tool-session", Domain: ".xemrush.site"})
	got := semrushFreshCookies(mapped, portal)
	if len(got) != 1 || got[0].Name != "proxy_token" || got[0].Domain != ".xemrush.site" {
		t.Fatalf("fresh semrush cookies = %+v", got)
	}
}
