package main

import "testing"

func TestIsFlaticonSecurityFilterHTML(t *testing.T) {
	soft := []byte(`<html><body><h1>Security check</h1><p>Just a quick check. We're making sure it's you.</p><p>This keeps your account and your work protected. Just a moment...</p></body></html>`)
	if !isFlaticonSecurityFilterHTML(soft) {
		t.Fatal("soft security check page should be detected")
	}
	hard := []byte(`<title>That request didn't go through. Our security filter flagged something.</title>`)
	if !isFlaticonSecurityFilterHTML(hard) {
		t.Fatal("hard security filter page should be detected")
	}
	ok := []byte(`<html><body><h1>Flaticon</h1><p>Download free icons</p></body></html>`)
	if isFlaticonSecurityFilterHTML(ok) {
		t.Fatal("normal page must not be flagged")
	}
}
