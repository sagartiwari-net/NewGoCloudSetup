package markhor

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"panel-api/automation"
	"testing"
	"time"
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
	if markhorShowsLogin(`<a href="/logout">Logout</a><input name="password">`, "https://markhorseotool.com/member/") {
		t.Fatal("member page with logout was treated as logged out")
	}
	if !markhorRejected(`<div class="alert alert-danger">Invalid username or password.</div>`) {
		t.Fatal("rejected password was not detected")
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

func TestMarkhorExportKeepsSetCookieFlags(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	seedMarkhorPortal(jar, []automation.Cookie{
		{Name: "PHPSESSID", Value: "portal-session", Domain: "markhorseotool.com", Path: "/", HostOnly: true, Session: true},
		{Name: "mht_session", Value: "member-session", Domain: "markhorseotool.com", Path: "/", HostOnly: true, Session: true},
	})
	record := &markhorRecord{}
	expires := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	record.note([]*http.Cookie{
		{
			Name: "PHPSESSID", Value: "portal-session", Domain: ".markhorseotool.com", Path: "/",
			HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, Expires: expires,
		},
		{
			Name: "mht_session", Value: "member-session", Domain: ".markhorseotool.com", Path: "/",
			HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, Expires: expires,
		},
	})
	got := markhorExportCookies(jar, record, []string{"https://markhorseotool.com/member/"})
	if len(got) != 2 {
		t.Fatalf("exported %d cookies", len(got))
	}
	for _, cookie := range got {
		if cookie.Domain != ".markhorseotool.com" || cookie.HostOnly || !cookie.HTTPOnly || !cookie.Secure || cookie.Session {
			t.Fatalf("%s lost Set-Cookie flags: %+v", cookie.Name, cookie)
		}
		if cookie.SameSite == nil || *cookie.SameSite != "lax" {
			t.Fatalf("%s sameSite = %v", cookie.Name, cookie.SameSite)
		}
		if cookie.ExpirationDate == nil || int64(*cookie.ExpirationDate) != expires.Unix() {
			t.Fatalf("%s expiry = %v", cookie.Name, cookie.ExpirationDate)
		}
	}
}
