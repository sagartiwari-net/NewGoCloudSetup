package azad

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"panel-api/automation"
	"strings"
	"testing"
	"time"
)

func TestAzadPageSignals(t *testing.T) {
	login := `<form><input name="amember_login"><input name="amember_pass"></form>`
	if !azadShowsLogin(login, "https://members.azadseo.com/login") {
		t.Fatal("login form was not detected")
	}
	dash := `<a href="/logout">Logout</a><a href="https://semrush2.azadseo.com/?tool_access_at=1">Semrush</a>`
	if !azadShowsDashboard(dash, "https://members.azadseo.com/member") {
		t.Fatal("member dashboard was not detected")
	}
	if azadShowsDashboard(login, "https://members.azadseo.com/member") {
		t.Fatal("login form was treated as a dashboard")
	}
	got := azadLaunchURL(dash, "semrush2.azadseo.com")
	if !strings.Contains(got, "semrush2.azadseo.com") {
		t.Fatalf("launch = %s", got)
	}
}

func TestAzadHTTPLoginAndSemrushCapture(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`<input name="amember_login"><input name="login_attempt_id" value="attempt-1">`))
			return
		}
		if r.FormValue("amember_login") == "" || r.FormValue("amember_pass") == "" || r.FormValue("login_attempt_id") != "attempt-1" {
			http.Error(w, "bad login", http.StatusForbidden)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "PHPSESSID", Value: "portal-session", Path: "/", HttpOnly: true})
		http.Redirect(w, r, "/member", http.StatusFound)
	})
	launchHref := ""
	mux.HandleFunc("/member", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			_, _ = w.Write([]byte(`<input name="amember_login">login to your account`))
			return
		}
		_, _ = w.Write([]byte(`<a href="/logout">Logout</a><a href="` + launchHref + `">Semrush</a>`))
	})
	portal := httptest.NewServer(mux)
	defer portal.Close()
	tool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			http.Error(w, "login to your account", http.StatusOK)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "proxy_token", Value: "tool-session", Path: "/", HttpOnly: true})
		http.SetCookie(w, &http.Cookie{Name: "amember_nr", Value: "portal-token", Path: "/"})
		_, _ = w.Write([]byte(`<a href="/logout">Logout</a> semrush ready`))
	}))
	defer tool.Close()
	launchHref = tool.URL + "/start?tool_access_at=9"

	previousBase, previousLogin, previousMember := azadBaseURL, azadLoginURL, azadMemberURL
	azadBaseURL = portal.URL
	azadLoginURL = portal.URL + "/login"
	azadMemberURL = portal.URL + "/member"
	t.Cleanup(func() {
		azadBaseURL, azadLoginURL, azadMemberURL = previousBase, previousLogin, previousMember
	})

	cookies, err := Recipe{}.Login(context.Background(), "azad-user", "azad-secret")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, cookie := range cookies {
		if cookie.Name == "PHPSESSID" && cookie.Value == "portal-session" {
			found = true
		}
		if strings.Contains(cookie.Value, "azad-secret") {
			t.Fatal("password was stored in a cookie")
		}
	}
	if !found {
		t.Fatal("homepage cookie was not stored")
	}
	valid, _, err := Recipe{}.Check(context.Background(), cookies)
	if err != nil || !valid {
		t.Fatalf("check valid=%v err=%v", valid, err)
	}

	captured, err := Recipe{}.Capture(context.Background(), cookies, tool.URL)
	if err != nil {
		t.Fatal(err)
	}
	gotTool := false
	for _, cookie := range captured {
		if cookie.Name == "PHPSESSID" || strings.Contains(strings.ToLower(cookie.Name), "amember") {
			t.Fatalf("portal cookie %s was written to semrush", cookie.Name)
		}
		if cookie.Name == "proxy_token" && cookie.Value == "tool-session" {
			gotTool = true
			if cookie.Domain != ".xemrush.site" {
				t.Fatalf("proxy cookie domain = %s", cookie.Domain)
			}
		}
	}
	if !gotTool {
		var names []string
		for _, cookie := range captured {
			names = append(names, cookie.Name)
		}
		t.Fatalf("semrush cookie missing, got %s", strings.Join(names, ","))
	}
}

func TestAzadSemrushKeepsOnlyTheProxyCookie(t *testing.T) {
	portal := []automation.Cookie{
		{Name: "PHPSESSID", Value: "portal-session", Domain: ".azadseo.com"},
		{Name: "amember_nr", Value: "member-token", Domain: ".azadseo.com"},
	}
	mapped := []automation.Cookie{
		{Name: "PHPSESSID", Value: "portal-session", Domain: ".xemrush.site"},
		{Name: "amember_nr", Value: "member-token", Domain: ".xemrush.site"},
	}
	if got := semrushFreshCookies(mapped, portal); len(got) != 0 {
		t.Fatal("portal cookies were treated as the semrush cookie")
	}
	mapped = append(mapped, automation.Cookie{Name: "proxy_token", Value: "tool-session", Domain: ".xemrush.site"})
	got := semrushFreshCookies(mapped, portal)
	if len(got) != 1 || got[0].Name != "proxy_token" || got[0].Domain != ".xemrush.site" {
		t.Fatalf("fresh semrush cookies = %+v", got)
	}
}

func TestAzadSemrush5IsSeparateFromSemrush2(t *testing.T) {
	html := `<a href="https://semrush2.azadseo.com/?tool_access_at=1">2</a><a href="https://export1sem.azadseo.com/?tool_access_at=1791993330">5</a>`
	got5 := azadExtractSemrushURL(html, "export1sem.azadseo.com")
	if got5 != "https://export1sem.azadseo.com/?tool_access_at=1791993330" {
		t.Fatalf("semrush 5 launch = %s", got5)
	}
	got2 := azadExtractSemrushURL(html, "semrush2.azadseo.com")
	if !strings.Contains(got2, "semrush2.azadseo.com") || strings.Contains(got2, "export1sem") {
		t.Fatalf("semrush 2 launch = %s", got2)
	}
	five := semrushTargetFor("6.semrush.com.in")
	if five.proxyHost != "export1sem.azadseo.com" || five.cookieDomain != ".6.semrush.com.in" {
		t.Fatalf("semrush 5 target = %+v", five)
	}
	two := semrushTargetFor("semrush2.xemrush.site")
	if two.proxyHost != "semrush2.azadseo.com" || two.cookieDomain != ".xemrush.site" {
		t.Fatalf("semrush 2 target = %+v", two)
	}
}

func TestAzadGrammarlyIsSeparate(t *testing.T) {
	html := `<a href="https://semrush2.azadseo.com/?tool_access_at=1">2</a><a href="https://export1sem.azadseo.com/?tool_access_at=9">5</a><a href="https://grammarly2.azadseo.com/?tool_access_at=1791993330">g</a>`
	got := azadExtractSemrushURL(html, "grammarly2.azadseo.com")
	if got != "https://grammarly2.azadseo.com/?tool_access_at=1791993330" {
		t.Fatalf("grammarly launch = %s", got)
	}
	if strings.Contains(azadExtractSemrushURL(html, "semrush2.azadseo.com"), "grammarly2") {
		t.Fatal("semrush 2 picked the grammarly link")
	}
	target := semrushTargetFor("testingg.one")
	if target.proxyHost != "grammarly2.azadseo.com" || target.cookieDomain != ".testingg.one" {
		t.Fatalf("grammarly target = %+v", target)
	}
}

func TestAzadJasperIsSeparate(t *testing.T) {
	html := `<a href="https://grammarly2.azadseo.com/?tool_access_at=1">g</a><a href="https://jasperai1.azadseo.com/?tool_access_at=1792024853">j</a>`
	got := azadExtractSemrushURL(html, "jasperai1.azadseo.com")
	if got != "https://jasperai1.azadseo.com/?tool_access_at=1792024853" {
		t.Fatalf("jasper launch = %s", got)
	}
	if strings.Contains(azadExtractSemrushURL(html, "grammarly2.azadseo.com"), "jasperai1") {
		t.Fatal("grammarly picked the jasper link")
	}
	target := semrushTargetFor("jasperpro.premiium.click")
	if target.proxyHost != "jasperai1.azadseo.com" || !target.keepSetDomain || target.cookieDomain != ".jasperpro.premiium.click" || target.otherDomain != ".premiium.click" {
		t.Fatalf("jasper target = %+v", target)
	}
	if got := domainFromSetCookie(http.Cookie{Domain: ".jasperpro.premiium.click"}, "proxy_token", target.cookieDomain, target.otherDomain); got != ".jasperpro.premiium.click" {
		t.Fatalf("proxy domain = %s", got)
	}
	if got := domainFromSetCookie(http.Cookie{Domain: "premiium.click"}, "_dd_s", target.cookieDomain, target.otherDomain); got != ".premiium.click" {
		t.Fatalf("dd domain = %s", got)
	}
}

func TestAzadSemrush5CaptureWritesProxyCookie(t *testing.T) {
	mux := http.NewServeMux()
	launchHref := ""
	seenAccess := ""
	mux.HandleFunc("/member", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			_, _ = w.Write([]byte(`<input name="amember_login">login to your account`))
			return
		}
		_, _ = w.Write([]byte(`<a href="/logout">Logout</a><a href="https://semrush2.azadseo.com/?tool_access_at=1">2</a><a href="` + launchHref + `">5</a>`))
	})
	portal := httptest.NewServer(mux)
	defer portal.Close()
	tool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAccess = r.URL.Query().Get("tool_access_at")
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			http.Error(w, "login to your account", http.StatusOK)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "proxy_token",
			Value:    "tool-session",
			Path:     "/",
			HttpOnly: true,
			Expires:  time.Unix(1823530692, 0),
		})
		http.SetCookie(w, &http.Cookie{Name: "amember_nr", Value: "portal-token", Path: "/"})
		_, _ = w.Write([]byte(`<a href="/logout">Logout</a> semrush ready`))
	}))
	defer tool.Close()
	launchHref = tool.URL + "/?tool_access_at=1791993330"

	previousBase, previousLogin, previousMember := azadBaseURL, azadLoginURL, azadMemberURL
	azadBaseURL = portal.URL
	azadLoginURL = portal.URL + "/login"
	azadMemberURL = portal.URL + "/member"
	t.Cleanup(func() {
		azadBaseURL, azadLoginURL, azadMemberURL = previousBase, previousLogin, previousMember
	})
	toolURL, err := url.Parse(tool.URL)
	if err != nil {
		t.Fatal(err)
	}
	cookies := []automation.Cookie{{
		Name: "PHPSESSID", Value: "portal-session", Domain: toolURL.Hostname(), Path: "/", HostOnly: true, HTTPOnly: true,
	}}
	captured, err := captureSemrush(context.Background(), cookies, semrushTarget{
		proxyHost:    toolURL.Host,
		cookieDomain: ".6.semrush.com.in",
	})
	if err != nil {
		t.Fatal(err)
	}
	if seenAccess != "1791993330" {
		t.Fatalf("tool_access_at = %s", seenAccess)
	}
	raw, err := automation.CookiesToJSON(captured)
	if err != nil {
		t.Fatal(err)
	}
	var written []automation.Cookie
	if err := json.Unmarshal([]byte(raw), &written); err != nil {
		t.Fatal(err)
	}
	if len(written) != 1 {
		t.Fatalf("wrote %d cookies", len(written))
	}
	cookie := written[0]
	if cookie.Name != "proxy_token" || cookie.Value != "tool-session" || cookie.Domain != ".6.semrush.com.in" {
		t.Fatalf("cookie = %+v", cookie)
	}
	if cookie.HostOnly || !cookie.HTTPOnly || cookie.Path != "/" || cookie.Session || cookie.SameSite != nil || cookie.StoreID != nil {
		t.Fatalf("cookie shape = %+v", cookie)
	}
	if cookie.ExpirationDate == nil || *cookie.ExpirationDate != 1823530692 {
		t.Fatalf("expiration = %v", cookie.ExpirationDate)
	}
}

func TestAzadGrammarlyCaptureWritesProxyCookie(t *testing.T) {
	mux := http.NewServeMux()
	launchHref := ""
	mux.HandleFunc("/member", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			_, _ = w.Write([]byte(`<input name="amember_login">login to your account`))
			return
		}
		_, _ = w.Write([]byte(`<a href="/logout">Logout</a><a href="https://semrush2.azadseo.com/?tool_access_at=1">2</a><a href="` + launchHref + `">g</a>`))
	})
	portal := httptest.NewServer(mux)
	defer portal.Close()
	tool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tool_access_at") != "1791993330" {
			http.Error(w, "bad access", http.StatusBadRequest)
			return
		}
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			http.Error(w, "login to your account", http.StatusOK)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name: "proxy_token", Value: "tool-session", Path: "/", HttpOnly: true,
			Expires: time.Unix(1823560863, 0),
		})
		_, _ = w.Write([]byte(`<a href="/logout">Logout</a> grammarly ready`))
	}))
	defer tool.Close()
	launchHref = tool.URL + "/?tool_access_at=1791993330"

	previousBase, previousLogin, previousMember := azadBaseURL, azadLoginURL, azadMemberURL
	azadBaseURL = portal.URL
	azadLoginURL = portal.URL + "/login"
	azadMemberURL = portal.URL + "/member"
	t.Cleanup(func() {
		azadBaseURL, azadLoginURL, azadMemberURL = previousBase, previousLogin, previousMember
	})
	toolURL, err := url.Parse(tool.URL)
	if err != nil {
		t.Fatal(err)
	}
	captured, err := captureSemrush(context.Background(), []automation.Cookie{{
		Name: "PHPSESSID", Value: "portal-session", Domain: toolURL.Hostname(), Path: "/", HostOnly: true, HTTPOnly: true,
	}}, semrushTarget{proxyHost: toolURL.Host, cookieDomain: ".testingg.one"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := automation.CookiesToJSON(captured)
	if err != nil {
		t.Fatal(err)
	}
	var written []automation.Cookie
	if err := json.Unmarshal([]byte(raw), &written); err != nil {
		t.Fatal(err)
	}
	if len(written) != 1 {
		t.Fatalf("wrote %d cookies", len(written))
	}
	cookie := written[0]
	if cookie.Name != "proxy_token" || cookie.Domain != ".testingg.one" || cookie.Secure || cookie.HostOnly || !cookie.HTTPOnly || cookie.Session {
		t.Fatalf("cookie = %+v", cookie)
	}
	if cookie.SameSite != nil || cookie.StoreID != nil || cookie.Path != "/" {
		t.Fatalf("cookie shape = %+v", cookie)
	}
	if cookie.ExpirationDate == nil || *cookie.ExpirationDate != 1823560863 {
		t.Fatalf("expiration = %v", cookie.ExpirationDate)
	}
}

func TestAzadJasperCaptureKeepsBothCookieDomains(t *testing.T) {
	mux := http.NewServeMux()
	launchHref := ""
	mux.HandleFunc("/member", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			_, _ = w.Write([]byte(`<input name="amember_login">login to your account`))
			return
		}
		_, _ = w.Write([]byte(`<a href="/logout">Logout</a><a href="` + launchHref + `">j</a>`))
	})
	portal := httptest.NewServer(mux)
	defer portal.Close()
	tool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal-session") {
			http.Error(w, "login to your account", http.StatusOK)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name: "proxy_token", Value: "tool-session", Path: "/", HttpOnly: true,
			Expires: time.Unix(1823515674, 0),
		})
		http.SetCookie(w, &http.Cookie{
			Name: "_dd_s", Value: "logs=1", Path: "/", SameSite: http.SameSiteStrictMode,
			Expires: time.Unix(1823560863, 0),
		})
		http.SetCookie(w, &http.Cookie{Name: "amember_nr", Value: "portal-token", Path: "/"})
		_, _ = w.Write([]byte(`<a href="/logout">Logout</a> jasper ready`))
	}))
	defer tool.Close()
	launchHref = tool.URL + "/?tool_access_at=1792024853"

	previousBase, previousLogin, previousMember := azadBaseURL, azadLoginURL, azadMemberURL
	azadBaseURL = portal.URL
	azadLoginURL = portal.URL + "/login"
	azadMemberURL = portal.URL + "/member"
	t.Cleanup(func() {
		azadBaseURL, azadLoginURL, azadMemberURL = previousBase, previousLogin, previousMember
	})
	toolURL, err := url.Parse(tool.URL)
	if err != nil {
		t.Fatal(err)
	}
	captured, err := captureSemrush(context.Background(), []automation.Cookie{{
		Name: "PHPSESSID", Value: "portal-session", Domain: toolURL.Hostname(), Path: "/", HostOnly: true, HTTPOnly: true,
	}}, semrushTarget{
		proxyHost: toolURL.Host, cookieDomain: ".jasperpro.premiium.click", otherDomain: ".premiium.click", keepSetDomain: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := automation.CookiesToJSON(captured)
	if err != nil {
		t.Fatal(err)
	}
	var written []automation.Cookie
	if err := json.Unmarshal([]byte(raw), &written); err != nil {
		t.Fatal(err)
	}
	if len(written) != 2 {
		t.Fatalf("wrote %d cookies", len(written))
	}
	byName := map[string]automation.Cookie{}
	for _, cookie := range written {
		if strings.Contains(strings.ToLower(cookie.Name), "amember") {
			t.Fatalf("portal cookie %s was written", cookie.Name)
		}
		byName[cookie.Name] = cookie
	}
	token := byName["proxy_token"]
	if token.Domain != ".jasperpro.premiium.click" || !token.HTTPOnly || token.SameSite != nil || token.HostOnly || token.Session {
		t.Fatalf("proxy_token = %+v", token)
	}
	dd := byName["_dd_s"]
	if dd.Domain != ".premiium.click" || dd.HTTPOnly || dd.Secure || dd.SameSite == nil || *dd.SameSite != "strict" || dd.HostOnly || dd.Session {
		t.Fatalf("_dd_s = %+v", dd)
	}
}
