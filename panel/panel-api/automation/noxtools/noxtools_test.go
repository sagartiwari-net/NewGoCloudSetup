package noxtools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"panel-api/automation"
	"strings"
	"testing"
)

func TestShowsMemberRequiresMarkers(t *testing.T) {
	if showsMember(`<html>ok</html>`, "https://noxtools.com/") {
		t.Fatal("bare homepage should not count as member")
	}
	if !showsMember(`<div id="member-main-subscriptions"></div>`, "https://noxtools.com/secure/member") {
		t.Fatal("member markers should pass")
	}
	if showsMember(`<input name="amember_login"><input name="amember_pass">`, "https://noxtools.com/secure/member") {
		t.Fatal("login form should fail")
	}
}

func TestTargetForSemrushAndSiteaudit(t *testing.T) {
	sem, err := targetFor("https://1.semrush.com.in/analytics/overview/?searchType=domain")
	if err != nil || sem.kind != "semrush" || sem.serverN != 1 || sem.cookieDomain != ".1.semrush.com.in" {
		t.Fatalf("semrush target = %+v err=%v", sem, err)
	}
	if !strings.Contains(sem.fallbackLaunch, "server1.php") {
		t.Fatalf("fallback = %s", sem.fallbackLaunch)
	}
	aud, err := targetFor("https://siteaudit1.semrush.com.in/siteaudit/")
	if err != nil || aud.kind != "siteaudit" || aud.serverN != 1 || aud.cookieDomain != ".siteaudit1.semrush.com.in" {
		t.Fatalf("audit target = %+v err=%v", aud, err)
	}
	if _, err := targetFor("https://example.com/"); err == nil {
		t.Fatal("expected upstream error")
	}
}

func TestExtractServerLaunchPrefersServer1Button(t *testing.T) {
	html := `
<a class="nt-sem-btn" href="https://semrush.noxtools.com/server2.php">Server 2</a>
<a class="nt-sem-btn" href="https://semrush.noxtools.com/server1.php" rel="noopener noreferrer" target="_blank">Server 1</a>
`
	target := captureTarget{proxyHost: "semrush.noxtools.com", serverN: 1, finalHost: "1.semrush.com.in"}
	got := extractServerLaunch(html, target)
	if !strings.Contains(got, "server1.php") {
		t.Fatalf("launch = %s", got)
	}
}

func TestNoxtoolsSemrush1Capture(t *testing.T) {
	toolFinal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "proxy_token", Value: "sem-proxy", Path: "/", HttpOnly: true})
		http.SetCookie(w, &http.Cookie{Name: "uxx", Value: "sem-uxx", Path: "/", HttpOnly: true})
		_, _ = w.Write([]byte(`ok analytics`))
	}))
	defer toolFinal.Close()

	toolLaunch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "proxy_token", Value: "sem-proxy", Path: "/", HttpOnly: true, Secure: true})
		http.SetCookie(w, &http.Cookie{Name: "uxx", Value: "sem-uxx", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
		http.SetCookie(w, &http.Cookie{Name: "snav_last_sidebar_tk", Value: "toolkit_seo", Path: "/"})
		http.SetCookie(w, &http.Cookie{Name: "PHPSESSID", Value: "portal-session", Path: "/"})
		http.Redirect(w, r, toolFinal.URL+"/analytics/overview/?searchType=domain", http.StatusFound)
	}))
	defer toolLaunch.Close()

	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			_, _ = w.Write([]byte(`<input name="amember_login"><input name="amember_pass">`))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/secure/page/semrush") {
			_, _ = w.Write([]byte(`<a class="nt-sem-btn" href="` + toolLaunch.URL + `/server1.php">Server 1</a>`))
			return
		}
		_, _ = w.Write([]byte(`<div id="member-main-subscriptions"></div><a href="/logout">Logout</a>`))
	}))
	defer portal.Close()

	prevBase, prevLogin, prevMember, prevSem := baseURL, loginURL, memberURL, semrushPage
	baseURL = portal.URL
	loginURL = portal.URL + "/login"
	memberURL = portal.URL + "/secure/member"
	semrushPage = portal.URL + "/secure/page/semrush"
	t.Cleanup(func() {
		baseURL, loginURL, memberURL, semrushPage = prevBase, prevLogin, prevMember, prevSem
	})

	cookies, err := Recipe{}.Capture(context.Background(), []automation.Cookie{{
		Name: "PHPSESSID", Value: "portal-session", Domain: ".noxtools.com", Path: "/", HTTPOnly: true, Session: true,
	}}, "https://1.semrush.com.in/")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, c := range cookies {
		names[c.Name] = c.Value
		if c.Name == "PHPSESSID" {
			t.Fatal("portal session leaked into tool cookie")
		}
		if c.Name == "proxy_token" && c.Domain != ".1.semrush.com.in" {
			t.Fatalf("proxy_token domain = %s", c.Domain)
		}
	}
	if names["proxy_token"] != "sem-proxy" || names["uxx"] != "sem-uxx" {
		t.Fatalf("cookies = %#v", names)
	}
}

func TestNoxtoolsSiteaudit1Capture(t *testing.T) {
	toolFinal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "proxy_token", Value: "aud-proxy", Path: "/", HttpOnly: true})
		_, _ = w.Write([]byte(`ok siteaudit`))
	}))
	defer toolFinal.Close()

	toolLaunch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "proxy_token", Value: "aud-proxy", Path: "/", HttpOnly: true})
		http.Redirect(w, r, toolFinal.URL+"/siteaudit/", http.StatusFound)
	}))
	defer toolLaunch.Close()

	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			_, _ = w.Write([]byte(`<input name="amember_login"><input name="amember_pass">`))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/secure/page/siteaudit") {
			_, _ = w.Write([]byte(`<a class="nt-sem-btn" href="` + toolLaunch.URL + `/server1.php">Server 1</a>`))
			return
		}
		_, _ = w.Write([]byte(`<div id="member-main-subscriptions"></div>`))
	}))
	defer portal.Close()

	prevBase, prevMember, prevAudit := baseURL, memberURL, auditPage
	baseURL = portal.URL
	memberURL = portal.URL + "/secure/member"
	auditPage = portal.URL + "/secure/page/siteaudit"
	t.Cleanup(func() {
		baseURL, memberURL, auditPage = prevBase, prevMember, prevAudit
	})

	cookies, err := Recipe{}.Capture(context.Background(), []automation.Cookie{{
		Name: "PHPSESSID", Value: "portal-session", Domain: ".noxtools.com", Path: "/",
	}}, "https://siteaudit1.semrush.com.in/siteaudit/")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range cookies {
		if c.Name == "proxy_token" && c.Value == "aud-proxy" && c.Domain == ".siteaudit1.semrush.com.in" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing audit proxy_token: %#v", cookies)
	}
}
