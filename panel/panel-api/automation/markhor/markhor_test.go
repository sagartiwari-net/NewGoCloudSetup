package markhor

import (
	"net/http/cookiejar"
	"net/url"
	"panel-api/automation"
	"testing"
)

func TestPageSignals(t *testing.T) {
	if !markhorShowsLogin(`<input name="username"><input name="password">`, "https://markhorseotool.com/member/") {
		t.Fatal("username and password inputs are a login page")
	}
	if !markhorShowsLogin("Member login. Sign in", "https://markhorseotool.com/login") {
		t.Fatal("member login copy is a login page")
	}
	if markhorShowsLogin(`<input name="username"><input name="password"> Domain Overview`, "https://sm01.markhorseotool.com/analytics/overview/") {
		t.Fatal("semrush tool page was treated as the portal login")
	}
	if !markhorShowsDashboard(`<a href="/logout">Logout</a> My tools`, "https://markhorseotool.com/member/") {
		t.Fatal("member dashboard was not detected")
	}
	if markhorShowsDashboard(`<input name="password">`, "https://markhorseotool.com/member/") {
		t.Fatal("a password field on /member is not a dashboard")
	}
	if !markhorShowsDenied("Access denied. You do not have an active subscription") {
		t.Fatal("access denied was not detected")
	}
}

func TestMarkhorPortalCookieReachesSemrushHost(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	seedMarkhorPortal(jar, []automation.Cookie{{
		Name: "session", Value: "portal-session", Domain: "markhorseotool.com", Path: "/", HostOnly: true,
	}})
	tool, err := url.Parse("https://sm01.markhorseotool.com/")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, cookie := range jar.Cookies(tool) {
		if cookie.Name == "session" && cookie.Value == "portal-session" {
			found = true
		}
	}
	if !found {
		t.Fatal("portal cookie was not sent to sm01.markhorseotool.com")
	}
}
