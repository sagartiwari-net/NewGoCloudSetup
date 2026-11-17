package noxtools

import "testing"

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
