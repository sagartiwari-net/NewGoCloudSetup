package amz

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"panel-api/automation"
	"strings"
	"testing"
)

func TestDiamondUsesHomepageCookie(t *testing.T) {
	portal := []automation.Cookie{{Name: "PHPSESSID", Value: "portal-session", Domain: "members.amztoolsconsultant.com", Path: "/"}}
	got, err := Recipe{}.Capture(context.Background(), portal, "https://ht01.amztoolsconsultant.com/black-box/products")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Value != "portal-session" || got[0].Domain != ".amztoolsconsultant.com" || got[0].HostOnly {
		t.Fatalf("diamond cookie %#v", got)
	}
	_, err = Recipe{}.Capture(context.Background(), portal, "https://en01.amztoolsconsultant.com/")
	if !errors.Is(err, automation.ErrUpstream) {
		t.Fatalf("other tool err %v", err)
	}
}

func TestLoginAndKeepa(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if r.FormValue("amember_login") != "amz-user" || r.FormValue("login_attempt_id") != "attempt-1" {
				http.Error(w, "bad login", http.StatusBadRequest)
				return
			}
			if r.FormValue("amember_pass") == "wrong" {
				_, _ = w.Write([]byte(`The user name or password is incorrect`))
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "PHPSESSID", Value: "portal-session", Path: "/", HttpOnly: true})
			http.SetCookie(w, &http.Cookie{Name: "amember_nr", Value: "nr-token", Path: "/"})
			http.SetCookie(w, &http.Cookie{Name: "amember_user", Value: "user-token", Path: "/"})
			http.Redirect(w, r, "/member", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(`<input name="amember_login"><input name="amember_pass"><input name="login_attempt_id" value="attempt-1">`))
	})
	mux.HandleFunc("/member", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(`<a href="/logout">Logout</a><a href="/page/keepa">Keepa</a>`))
	})
	mux.HandleFunc("/keepa-page", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(`<a href="/cloud/kp01">Launch Keepa Workspace</a>`))
	})
	mux.HandleFunc("/cloud/kp01", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<form method="post" action="/auth/login"><input name="token" value="sso"></form>`))
	})
	mux.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("token") != "sso" {
			http.Error(w, "missing sso", http.StatusBadRequest)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "keepa_token", Value: "tool-session", Path: "/", HttpOnly: true})
		_, _ = w.Write([]byte(`keepa ready`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	previous := []string{baseURL, loginURL, memberURL, keepaPageURL, keepaLaunchURL}
	baseURL = server.URL
	loginURL = server.URL + "/login"
	memberURL = server.URL + "/member"
	keepaPageURL = server.URL + "/keepa-page"
	keepaLaunchURL = server.URL + "/cloud/kp01"
	t.Cleanup(func() {
		baseURL, loginURL, memberURL, keepaPageURL, keepaLaunchURL = previous[0], previous[1], previous[2], previous[3], previous[4]
	})

	if _, err := (Recipe{}).Login(context.Background(), "amz-user", "wrong"); err == nil {
		t.Fatal("wrong password should fail")
	}
	cookies, err := Recipe{}.Login(context.Background(), "amz-user", "amz-secret")
	if err != nil {
		t.Fatal(err)
	}
	if !hasCookie(cookies, "PHPSESSID", "portal-session") || !hasCookie(cookies, "amember_nr", "nr-token") || !hasCookie(cookies, "amember_user", "user-token") {
		t.Fatalf("portal cookies %#v", cookies)
	}
	ok, _, err := Recipe{}.Check(context.Background(), cookies)
	if err != nil || !ok {
		t.Fatalf("check valid=%v err=%v", ok, err)
	}
	got, err := Recipe{}.Capture(context.Background(), cookies, server.URL+"/kp01")
	if err != nil {
		t.Fatal(err)
	}
	if !hasCookie(got, "keepa_token", "tool-session") {
		t.Fatalf("keepa cookies %#v", got)
	}
	if hasCookie(got, "PHPSESSID", "portal-session") {
		t.Fatal("keepa kept the homepage cookie")
	}
}

func TestKeepaPostsLaunchFormBeforeOpeningWorkspace(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/keepa-page", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(`<a href="/session-watch/revoke-copied?next=/cloud/kp01&name=Keepa">watch</a><a href="/cloud/kp01">Launch Keepa Workspace</a>`))
	})
	mux.HandleFunc("/session-watch/revoke-copied", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusFound)
	})
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<input name="amember_login"><input name="amember_pass">`))
	})
	mux.HandleFunc("/cloud/kp01", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<form method="get" action="/amztoolsconsultant-kp01/"><input type="hidden" name="token" value="sso"><button formaction="/auth/login" formmethod="post">Launch</button></form>`))
	})
	mux.HandleFunc("/amztoolsconsultant-kp01/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "auth_session=app01-session") {
			_, _ = w.Write([]byte(`workspace without cookie`))
			return
		}
		_, _ = w.Write([]byte(`keepa ready`))
	})
	mux.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.FormValue("token") != "sso" {
			http.Error(w, "missing sso", http.StatusBadRequest)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "auth_session", Value: "app01-session", Path: "/amztoolsconsultant-kp01", HttpOnly: true})
		http.Redirect(w, r, "/amztoolsconsultant-kp01/", http.StatusFound)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	previous := []string{baseURL, loginURL, memberURL, keepaPageURL, keepaLaunchURL}
	baseURL = server.URL
	loginURL = server.URL + "/login"
	memberURL = server.URL + "/member"
	keepaPageURL = server.URL + "/keepa-page"
	keepaLaunchURL = server.URL + "/cloud/kp01"
	t.Cleanup(func() {
		baseURL, loginURL, memberURL, keepaPageURL, keepaLaunchURL = previous[0], previous[1], previous[2], previous[3], previous[4]
	})

	portal := []automation.Cookie{{Name: "PHPSESSID", Value: "portal-session", Domain: "members.amztoolsconsultant.com", Path: "/"}}
	got, err := Recipe{}.Capture(context.Background(), portal, server.URL+"/kp01")
	if err != nil {
		t.Fatal(err)
	}
	if !hasCookie(got, "auth_session", "app01-session") {
		t.Fatalf("keepa cookies %#v", got)
	}
	if hasCookie(got, "PHPSESSID", "portal-session") {
		t.Fatal("keepa kept the homepage cookie")
	}
	for _, cookie := range got {
		if cookie.Name == "auth_session" && cookie.Path != "/amztoolsconsultant-kp01" {
			t.Fatalf("auth_session path %q", cookie.Path)
		}
	}
}

func TestKeepaRetriesFormOnToolHost(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`Site not found. Use your-domain.com/<site-name>`))
	}))
	defer dead.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/keepa-page", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(`<a href="/cloud/kp01">Launch Keepa Workspace</a>`))
	})
	mux.HandleFunc("/cloud/kp01", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<form method="post" action="` + dead.URL + `/amztoolsconsultant-kp01/auth/login"><input name="signature" value="stale"></form><form method="post" action="/amztoolsconsultant-kp01/auth/login"><input name="signature" value="signed"></form>`))
	})
	mux.HandleFunc("/amztoolsconsultant-kp01/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.FormValue("signature") != "signed" {
			http.Error(w, "missing signature", http.StatusBadRequest)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "auth_session", Value: "tool-session", Path: "/amztoolsconsultant-kp01", HttpOnly: true})
		http.Redirect(w, r, "/amztoolsconsultant-kp01/", http.StatusFound)
	})
	mux.HandleFunc("/amztoolsconsultant-kp01/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`keepa ready`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	previous := []string{baseURL, loginURL, memberURL, keepaPageURL, keepaLaunchURL}
	baseURL = server.URL
	loginURL = server.URL + "/login"
	memberURL = server.URL + "/member"
	keepaPageURL = server.URL + "/keepa-page"
	keepaLaunchURL = server.URL + "/cloud/kp01"
	t.Cleanup(func() {
		baseURL, loginURL, memberURL, keepaPageURL, keepaLaunchURL = previous[0], previous[1], previous[2], previous[3], previous[4]
	})

	portal := []automation.Cookie{{Name: "PHPSESSID", Value: "portal-session", Domain: "members.amztoolsconsultant.com", Path: "/"}}
	got, err := Recipe{}.Capture(context.Background(), portal, server.URL+"/kp01")
	if err != nil {
		t.Fatal(err)
	}
	if !hasCookie(got, "auth_session", "tool-session") {
		t.Fatalf("keepa cookies %#v", got)
	}
}

func TestSignedKeepaFormStaysOnItsHost(t *testing.T) {
	var toolHit bool
	tool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			toolHit = true
		}
		http.Error(w, "should not be called", http.StatusForbidden)
	}))
	defer tool.Close()
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`Site not found. Use your-domain.com/<site-name>`))
	}))
	defer dead.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("/keepa-page", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<a href="/cloud/kp01">Launch Keepa Workspace</a>`))
	})
	mux.HandleFunc("/cloud/kp01", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<form method="post" action="` + dead.URL + `/amztoolsconsultant-kp01/auth/login"><input name="signature" value="stale"></form>`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	previous := []string{baseURL, loginURL, memberURL, keepaPageURL, keepaLaunchURL}
	baseURL = server.URL
	loginURL = server.URL + "/login"
	memberURL = server.URL + "/member"
	keepaPageURL = server.URL + "/keepa-page"
	keepaLaunchURL = server.URL + "/cloud/kp01"
	t.Cleanup(func() {
		baseURL, loginURL, memberURL, keepaPageURL, keepaLaunchURL = previous[0], previous[1], previous[2], previous[3], previous[4]
	})
	_, err := Recipe{}.Capture(context.Background(), []automation.Cookie{{Name: "PHPSESSID", Value: "portal-session", Domain: ".amztoolsconsultant.com", Path: "/"}}, tool.URL+"/kp01")
	if err == nil {
		t.Fatal("signed form for a missing site should not become a tool cookie")
	}
	if toolHit {
		t.Fatal("signed form was replayed on the tool host")
	}
}

func TestProbeDiamondRejectsLoginPage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/black-box/products", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(`<a href="/logout">Logout</a> helium products`))
	})
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<input name="amember_login"><input name="amember_pass"><a href="/page/keepa">Keepa</a>`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	previousHome := diamondHomeURL
	previousBase := baseURL
	diamondHomeURL = server.URL + "/black-box/products"
	baseURL = server.URL
	t.Cleanup(func() {
		diamondHomeURL = previousHome
		baseURL = previousBase
	})

	portal := []automation.Cookie{{Name: "PHPSESSID", Value: "portal-session", Domain: ".amztoolsconsultant.com", Path: "/"}}
	if err := ProbeDiamond(context.Background(), portal); err != nil {
		t.Fatal(err)
	}
	if err := ProbeDiamond(context.Background(), []automation.Cookie{{Name: "PHPSESSID", Value: "dead", Domain: ".amztoolsconsultant.com", Path: "/"}}); err == nil {
		t.Fatal("dead cookie was accepted")
	}
}

func TestPageKindNamesTheDashboard(t *testing.T) {
	login := pageKind(`<form><input name="amember_login"><input name="amember_pass"></form>`, "https://members.amztoolsconsultant.com/login")
	dash := pageKind(`<div class="am-member-main"><a href="/logout">Logout</a></div>`, "https://members.amztoolsconsultant.com/member")
	if login != "login-form" || dash != "dashboard" {
		t.Fatalf("login=%s dashboard=%s", login, dash)
	}
}

func TestPHPSESSIDAloneIsNotASession(t *testing.T) {
	ok, _, err := Recipe{}.Check(context.Background(), []automation.Cookie{{Name: "PHPSESSID", Value: "only-session", Domain: ".amztoolsconsultant.com", Path: "/"}})
	if err != nil || ok {
		t.Fatalf("php session alone valid=%v err=%v", ok, err)
	}
}

func TestLoginPageMentioningKeepaIsNotASession(t *testing.T) {
	page := `<a href="/logout">Logout</a><a href="/page/keepa">Keepa</a><input name="amember_login"><input name="amember_pass">`
	if !showsLogin(page, "https://members.amztoolsconsultant.com/login") || showsDashboard(page, "https://members.amztoolsconsultant.com/login") {
		t.Fatal("login page counted as a live session")
	}
	member := `<a href="/logout">Logout</a><div class="am-member-main">plans</div>`
	if !showsDashboard(member, "https://members.amztoolsconsultant.com/member") {
		t.Fatal("member page was rejected")
	}
}

func hasCookie(cookies []automation.Cookie, name, value string) bool {
	for _, cookie := range cookies {
		if cookie.Name == name && cookie.Value == value {
			return true
		}
	}
	return false
}
