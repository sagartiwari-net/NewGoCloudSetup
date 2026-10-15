package toolbaazar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"panel-api/automation"
	"strings"
	"testing"
)

func TestExtractAccessPrefersSemrushLink(t *testing.T) {
	page := `<a href="/content/p/id/128/" class="btn btn-primary">Semrush</a>` +
		`<a class="btn-primary" href="https://other.example/?backup=1">Other</a>` +
		`<a class="btn-primary" href="https://s2new.toolbaazar.com?backup=1">Access Semrush</a>`
	got := extractAccess(page, toolTarget{contentID: semrushContent, fallback: semrushAccess})
	if got != "https://s2new.toolbaazar.com?backup=1" {
		t.Fatalf("semrush access %s", got)
	}
	audit := `<a class="btn-primary" href="https://audit.example/open">Access Audit</a>`
	got = extractAccess(audit, toolTarget{contentID: auditContent})
	if got != "https://audit.example/open" {
		t.Fatalf("audit access %s", got)
	}
	if extractAccess(`<a href="/member">home</a>`, toolTarget{contentID: semrushContent, fallback: semrushAccess}) != semrushAccess {
		t.Fatal("semrush should use the known access link when the button is missing")
	}
}

func TestTargetForUpstream(t *testing.T) {
	if targetFor("https://sm.verygoodtool.com/").contentID != semrushContent {
		t.Fatal("semrush upstream")
	}
	if targetFor("https://site.verygoodtool.com/siteaudit").contentID != auditContent {
		t.Fatal("audit upstream")
	}
}

func TestLoginAndCapture(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if r.FormValue("amember_login") != "tb-user" || r.FormValue("amember_pass") == "" {
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
		_, _ = w.Write([]byte(`<form name="login"><input name="amember_login"><input name="amember_pass"></form>`))
	})
	mux.HandleFunc("/member", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(`<a href="/logout">Logout</a><a href="/content/p/id/128/" class="btn btn-primary">Semrush</a>`))
	})
	mux.HandleFunc("/content/p/id/128/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(`<a class="btn-primary" href="/semrush?backup=1">Access Semrush</a>`))
	})
	mux.HandleFunc("/content/p/id/132/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(`<a class="btn-primary" href="/audit?backup=1">Access Audit</a>`))
	})
	mux.HandleFunc("/semrush", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "gateway_bootstrap", Value: "gate-semrush", Path: "/", HttpOnly: true, Secure: true})
		http.SetCookie(w, &http.Cookie{Name: "user_pool", Value: "pool-semrush", Path: "/"})
		_, _ = w.Write([]byte(`semrush ready`))
	})
	mux.HandleFunc("/audit", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "gateway_bootstrap", Value: "gate-audit", Path: "/", HttpOnly: true})
		http.SetCookie(w, &http.Cookie{Name: "user_pool", Value: "pool-audit", Path: "/"})
		_, _ = w.Write([]byte(`audit ready`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	previousBase, previousLogin, previousMember := baseURL, loginURL, memberURL
	baseURL = server.URL
	loginURL = server.URL + "/login"
	memberURL = server.URL + "/member"
	t.Cleanup(func() {
		baseURL, loginURL, memberURL = previousBase, previousLogin, previousMember
	})

	rejected, err := Recipe{}.Login(context.Background(), "tb-user", "wrong")
	if err == nil || len(rejected) > 0 {
		t.Fatal("wrong password should be rejected")
	}
	cookies, err := Recipe{}.Login(context.Background(), "tb-user", "tb-secret")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, cookie := range cookies {
		if cookie.Name == "PHPSESSID" && cookie.Value == "portal-session" {
			found = true
		}
		if strings.Contains(cookie.Value, "tb-secret") {
			t.Fatal("password stored in a cookie")
		}
	}
	if !found {
		t.Fatal("portal cookie missing")
	}
	ok, _, err := Recipe{}.Check(context.Background(), cookies)
	if err != nil || !ok {
		t.Fatalf("check valid=%v err=%v", ok, err)
	}

	semrush, err := Recipe{}.Capture(context.Background(), cookies, "https://sm.verygoodtool.com/")
	if err != nil {
		t.Fatal(err)
	}
	if !hasCookie(semrush, "gateway_bootstrap", "gate-semrush") || !hasCookie(semrush, "user_pool", "pool-semrush") {
		t.Fatalf("semrush cookies %#v", semrush)
	}
	if hasCookie(semrush, "PHPSESSID", "portal-session") {
		t.Fatal("semrush kept the portal session")
	}

	audit, err := Recipe{}.Capture(context.Background(), cookies, "https://site.verygoodtool.com/siteaudit")
	if err != nil {
		t.Fatal(err)
	}
	if !hasCookie(audit, "gateway_bootstrap", "gate-audit") || !hasCookie(audit, "user_pool", "pool-audit") {
		t.Fatalf("audit cookies %#v", audit)
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
