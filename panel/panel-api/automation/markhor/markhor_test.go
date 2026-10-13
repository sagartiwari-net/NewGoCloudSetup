package markhor

import "testing"

func TestPageSignals(t *testing.T) {
	if !markhorShowsLogin(`<input name="username"><input name="password">`, "https://markhorseotool.com/member/") {
		t.Fatal("username and password inputs are a login page")
	}
	if !markhorShowsLogin("Member login. Sign in", "https://markhorseotool.com/member/") {
		t.Fatal("member login copy is a login page")
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
