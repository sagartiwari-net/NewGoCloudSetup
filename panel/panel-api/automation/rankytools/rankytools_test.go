package rankytools

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"panel-api/automation"
	"testing"
)

func TestRankyLoginAndMemberSignals(t *testing.T) {
	if !showsLogin(`<input name="amember_login"><input name="amember_pass">`, "https://member.rankytools.com/login") {
		t.Fatal("login form must count as login")
	}
	if showsLogin(`Your Membership Information <a href="/logout">Logout</a>`, "https://member.rankytools.com/member") {
		t.Fatal("member page must not count as login")
	}
	if !showsMember(`<title>Your Membership Information</title>`, "https://member.rankytools.com/member") {
		t.Fatal("membership title must count as member")
	}
	if !hasActivePlan(`<div id="member-main-subscriptions"><ul class="am-list-subscriptions"><li>SEO Premium Plan next bill 12/26/26</li></ul></div>`) {
		t.Fatal("SEO Premium Plan must count as active")
	}
	if hasActivePlan(`<div id="member-main-subscriptions">You have no active subscriptions</div>`) {
		t.Fatal("empty subscription must fail plan check")
	}
	if !rejected(`<div class="am-error">Invalid username or password</div>`) {
		t.Fatal("invalid password must reject")
	}
}

func TestRankyPortalCookieSeed(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	seedPortal(jar, []automation.Cookie{{
		Name: "PHPSESSID", Value: "portal-session", Domain: "member.rankytools.com", Path: "/", HostOnly: true,
	}, {
		Name: "amember_nr", Value: "nr-session", Domain: ".rankytools.com", Path: "/",
	}})
	tool, err := url.Parse("https://sr.rankytools.com/")
	if err != nil {
		t.Fatal(err)
	}
	got := jar.Cookies(tool)
	names := map[string]string{}
	for _, c := range got {
		names[c.Name] = c.Value
	}
	if names["PHPSESSID"] != "portal-session" {
		t.Fatalf("PHPSESSID not sent to tool host: %#v", names)
	}
	if names["amember_nr"] != "nr-session" {
		t.Fatalf("amember_nr not sent to tool host: %#v", names)
	}
}

func TestRankyExportFilters(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	seedPortal(jar, []automation.Cookie{
		{Name: "PHPSESSID", Value: "abc", Domain: ".rankytools.com", Path: "/", Session: true},
		{Name: "mh_vid", Value: "ignore", Domain: "member.rankytools.com", Path: "/", HostOnly: true},
	})
	record := &cookieRecord{}
	record.note([]*http.Cookie{{
		Name: "PHPSESSID", Value: "abc", Domain: ".rankytools.com", Path: "/", HttpOnly: true, Secure: true,
	}})
	got := exportCookies(jar, record)
	if len(got) != 1 || got[0].Name != "PHPSESSID" {
		t.Fatalf("expected only PHPSESSID, got %#v", got)
	}
	if got[0].Domain != ".rankytools.com" || got[0].HostOnly {
		t.Fatalf("domain not widened: %#v", got[0])
	}
}
