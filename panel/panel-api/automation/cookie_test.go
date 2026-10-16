package automation

import "testing"

func TestUserAgentStaysBesideTheCookies(t *testing.T) {
	raw := `{
		"userAgent": "Mozilla/5.0 Chrome/155",
		"cookies": [{"name":"BSSESSID","value":"sess","domain":".ahrefs.com","path":"/"}]
	}`
	if got := UserAgentFromCookie(raw); got != "Mozilla/5.0 Chrome/155" {
		t.Fatalf("user agent %q", got)
	}
	cookies := ParseStoredCookies(raw)
	if len(cookies) != 1 || cookies[0].Name != "BSSESSID" {
		t.Fatalf("cookies %#v", cookies)
	}
	wrapped, err := CookiesWithUserAgent(cookies, "Mozilla/5.0 Chrome/155")
	if err != nil {
		t.Fatal(err)
	}
	if UserAgentFromCookie(wrapped) != "Mozilla/5.0 Chrome/155" {
		t.Fatalf("wrapped lost the user agent: %s", wrapped)
	}
	if len(ParseStoredCookies(wrapped)) != 1 {
		t.Fatal("wrapped cookies missing")
	}
}
