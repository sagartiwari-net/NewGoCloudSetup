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
	if !hasCookie(cookies, "PHPSESSID", "portal-session") {
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
		_, _ = w.Write([]byte(`<a href="/amztoolsconsultant-kp01/">Open</a><form method="get" action="/amztoolsconsultant-kp01/"><input type="hidden" name="token" value="sso"><button formaction="/auth/login" formmethod="post">Launch</button></form>`))
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

func hasCookie(cookies []automation.Cookie, name, value string) bool {
	for _, cookie := range cookies {
		if cookie.Name == name && cookie.Value == value {
			return true
		}
	}
	return false
}
