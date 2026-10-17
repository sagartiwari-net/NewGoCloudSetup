package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"recloud.dev/cdncache"
)

func TestRewriteKeepsBrowserOnPublicHost(t *testing.T) {
	in := []byte(`<a href="https://semnew1.accesstool.in/analytics/overview/">x</a><script src="https://static.accesstool.in/app.js"></script><script src="https://de.semrush.com/app.js"></script>{"cookie_domain":".accesstool.in","host":"semnew1.accesstool.in","bad":"semnew1.shsm1.gt4rents.com"}`)
	out := string(rewriteText(in, "semnew1.accesstool.in", "https://shsm1.gt4rents.com"))
	if strings.Contains(out, "accesstool.in") || strings.Contains(out, "semrush.com") || strings.Contains(out, "semnew1.shsm1") {
		t.Fatalf("upstream host leaked: %s", out)
	}
	if !strings.Contains(out, "https://shsm1.gt4rents.com/analytics/overview/") {
		t.Fatal("public origin was not applied")
	}
	if !strings.Contains(out, ".shsm1.gt4rents.com") {
		t.Fatal("cookie domain was not rewritten")
	}
}

func TestStaticCachePolicy(t *testing.T) {
	if !isCacheableStaticPath("/__static__/webpack/app.js") {
		t.Fatal("js should cache")
	}
	if isCacheableStaticPath("/analytics/overview/") {
		t.Fatal("html path should not cache")
	}
	req := httptest.NewRequest(http.MethodGet, "/analytics/overview/", nil)
	if cdncache.RequestKey(req, isCacheableStaticPath(req.URL.Path)) != "" {
		t.Fatal("html path must not be cached")
	}
}

func TestCookieJSONBecomesHeader(t *testing.T) {
	raw := `[{"name":"proxy_token","value":"abc","hostOnly":false},{"name":"snav_last_sidebar_tk","value":"toolkit_seo","hostOnly":true}]`
	got := parseCookieFromDB(raw)
	if !strings.Contains(got, "proxy_token=abc") || !strings.Contains(got, "snav_last_sidebar_tk=toolkit_seo") {
		t.Fatal("cookie json did not become a header")
	}
}

func TestUpstreamCookieKeepsOnlyGate(t *testing.T) {
	raw := `[{"name":"snav_last_sidebar_tk","value":"toolkit_seo"},{"name":"ref_code","value":"__default__"},{"name":"refer_source","value":"\"\""},{"name":"sr_gate","value":"gate-token"}]`
	got := upstreamSessionCookie(parseCookieFromDB(raw))
	if got != "sr_gate=gate-token" {
		t.Fatalf("upstream cookie %s", got)
	}
}

func TestUpstreamCookieKeepsBrowserSession(t *testing.T) {
	raw := `[{"name":"snav_last_sidebar_tk","value":"from-panel"},{"name":"ref_code","value":"__default__"},{"name":"refer_source","value":"\"\""},{"name":"sr_gate","value":"gate-token"}]`
	browser := "ct_session=panel-session; SSO-JWT=jwt-value; snav_last_sidebar_tk=from-browser; sr_gate=browser-gate"
	got := cookiesForUpstream(parseCookieFromDB(raw), browser)
	if !strings.HasPrefix(got, "sr_gate=gate-token") {
		t.Fatalf("gate cookie %s", got)
	}
	if !strings.Contains(got, "SSO-JWT=jwt-value") || !strings.Contains(got, "snav_last_sidebar_tk=from-browser") {
		t.Fatalf("browser session was dropped: %s", got)
	}
	if strings.Contains(got, "ct_session") || strings.Contains(got, "from-panel") || strings.Contains(got, "ref_code") || strings.Contains(got, "browser-gate") {
		t.Fatalf("panel junk or panel session leaked: %s", got)
	}
}

func TestSearchRequestsArePatchedBeforeAppScripts(t *testing.T) {
	script := devicePageScript()
	patch := strings.Index(script, "tmPatchRequests(fpNow, proof)")
	finger := strings.Index(script, "tmFingerprint().then")
	if patch < 0 || finger < 0 || patch > finger {
		t.Fatal("search requests can leave before the device patch")
	}
	if strings.Contains(script, "new Request(input, init)") {
		t.Fatal("fetch patch rebuilds the request")
	}
}

func TestUpstreamRequestDropsProxyHeaders(t *testing.T) {
	src := httptest.NewRequest(http.MethodGet, "https://shsm1.gt4rents.com/analytics/overview/?searchType=domain", nil)
	src.Header.Set("X-Forwarded-Host", "shsm1.gt4rents.com")
	src.Header.Set("X-Forwarded-For", "203.0.113.8")
	src.Header.Set("X-Real-IP", "203.0.113.8")
	src.Header.Set("Forwarded", "host=shsm1.gt4rents.com")
	src.Header.Set("CF-Connecting-IP", "203.0.113.8")
	src.Header.Set("User-Agent", "Mozilla/5.0")
	dst, err := http.NewRequest(http.MethodGet, "https://semnew1.accesstool.in/analytics/overview/", nil)
	if err != nil {
		t.Fatal(err)
	}
	copyRequestHeaders(dst, src)
	for _, h := range []string{"X-Forwarded-Host", "X-Forwarded-For", "X-Real-IP", "Forwarded", "CF-Connecting-IP"} {
		if dst.Header.Get(h) != "" {
			t.Fatalf("forwarded %s to upstream", h)
		}
	}
	if dst.Header.Get("User-Agent") != "Mozilla/5.0" {
		t.Fatal("browser header was dropped")
	}
}

func TestSetCookieDropsUpstreamSession(t *testing.T) {
	if _, ok := rewriteSetCookie("sr_gate=secret; Path=/; Secure", "semnew1.accesstool.in", "https://shsm1.gt4rents.com"); ok {
		t.Fatal("upstream session cookie must not reach the browser")
	}
	got, ok := rewriteSetCookie("snav_last_sidebar_tk=toolkit_seo; Domain=.semrush.com; Path=/", "semnew1.accesstool.in", "https://shsm1.gt4rents.com")
	if !ok || !strings.Contains(got, "Domain=shsm1.gt4rents.com") || strings.Contains(got, "semrush.com") {
		t.Fatal("sidebar cookie domain was not rewritten")
	}
}

func TestLoginPageRetriesThenOpensHome(t *testing.T) {
	oldWait := loginRetryWait
	loginRetryWait = 0
	t.Cleanup(func() { loginRetryWait = oldWait })

	var hits int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if hits < loginAttempts {
			_, _ = w.Write([]byte(`<h1>Access Denied</h1><p>This section is restricted.</p>`))
			return
		}
		if r.URL.Path != "/analytics/overview/" {
			t.Errorf("retry path %s", r.URL.RequestURI())
		}
		_, _ = w.Write([]byte(`<html><body>overview ready</body></html>`))
	}))
	defer upstream.Close()

	cfg := Config{
		TargetURL:    upstream.URL,
		HomePath:     "/analytics/overview/?searchType=domain",
		PublicHost:   "shsm1.gt4rents.com",
		PublicScheme: "http",
		ToolName:     "SEO Shop Semrush 1",
	}
	req := httptest.NewRequest(http.MethodGet, "http://shsm1.gt4rents.com/login?url=%2F&host=semnew1.shsm1.gt4rents.com&ssl=on", nil)
	req.Header.Set("Accept", "text/html")
	req.Host = "shsm1.gt4rents.com"
	rr := httptest.NewRecorder()
	proxyUpstream(rr, req, cfg, ToolAccount{Cookie: "PHPSESSID=fresh"})
	if hits != loginAttempts {
		t.Fatalf("upstream hits=%d", hits)
	}
	if rr.Code != http.StatusFound {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Location") != cfg.HomePath {
		t.Fatalf("location %s", rr.Header().Get("Location"))
	}
	if strings.Contains(rr.Body.String(), "restricted") {
		t.Fatal("upstream restricted page was shown")
	}
}

func TestOnlyLayoutNotificationIsHidden(t *testing.T) {
	in := []byte(`<html><head></head><body><div class="srf-layout__notification"><p>out of date</p></div><a class="srf-login-btn">Log In</a></body></html>`)
	out := string(injectDeviceHTML(in))
	if !strings.Contains(out, `.srf-layout__notification{display:none!important}`) {
		t.Fatal("notification hide style missing")
	}
	if !strings.Contains(out, `srf-login-btn`) {
		t.Fatal("login button was removed")
	}
	if strings.Contains(out, `srf-login-btn{display:none`) {
		t.Fatal("login button was hidden")
	}
}

func TestLogoutPath(t *testing.T) {
	if !isLogoutPath("/sso/logout") || !isLogoutPath("/login") || isLogoutPath("/analytics/overview/") {
		t.Fatal("logout path detection")
	}
	if !bodyLooksLoggedOut([]byte(`<h1>Access Denied</h1><p>This section is restricted.</p>`)) {
		t.Fatal("restricted page should count as logged out")
	}
}
