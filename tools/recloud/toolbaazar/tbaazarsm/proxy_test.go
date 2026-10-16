package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"recloud.dev/cdncache"
)

func TestRewriteKeepsBrowserOnPublicHost(t *testing.T) {
	in := []byte(`<a href="https://sm.verygoodtool.com/analytics/overview/">x</a><script src="https://cdn.sm.verygoodtool.com/asset.js"></script><script src="https://static.verygoodtool.com/app.js"></script><script src="https://de.semrush.com/app.js"></script>{"cookie_domain":".verygoodtool.com","host":"sm.verygoodtool.com"}`)
	out := string(rewriteText(in, "sm.verygoodtool.com", "https://tbaazarsm.gt4rents.com"))
	if strings.Contains(out, "verygoodtool.com") || strings.Contains(out, "semrush.com") {
		t.Fatalf("upstream host leaked: %s", out)
	}
	if !strings.Contains(out, "https://tbaazarsm.gt4rents.com/analytics/overview/") || !strings.Contains(out, "https://tbaazarsm.gt4rents.com/asset.js") {
		t.Fatal("public origin was not applied")
	}
	if !strings.Contains(out, ".tbaazarsm.gt4rents.com") {
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

func TestSetCookieDropsUpstreamSession(t *testing.T) {
	if _, ok := rewriteSetCookie("gateway_bootstrap=secret; Path=/; HttpOnly", "sm.verygoodtool.com", "https://tbaazarsm.gt4rents.com"); ok {
		t.Fatal("gateway bootstrap must not reach the browser")
	}
	if _, ok := rewriteSetCookie("user_pool=pool_2; Path=/", "sm.verygoodtool.com", "https://tbaazarsm.gt4rents.com"); ok {
		t.Fatal("user pool must not reach the browser")
	}
	if _, ok := rewriteSetCookie("tsoe-biao-a1rc=secret; Path=/; HttpOnly; Secure", "sm.verygoodtool.com", "https://tbaazarsm.gt4rents.com"); ok {
		t.Fatal("gateway session cookie must not reach the browser")
	}
	got, ok := rewriteSetCookie("snav_last_sidebar_tk=toolkit_seo; Domain=.semrush.com; Path=/", "sm.verygoodtool.com", "https://tbaazarsm.gt4rents.com")
	if !ok || !strings.Contains(got, "Domain=tbaazarsm.gt4rents.com") || strings.Contains(got, "semrush.com") {
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
		PublicHost:   "tbaazarsm.gt4rents.com",
		PublicScheme: "http",
		ToolName:     "Toolbaazar Semrush",
	}
	req := httptest.NewRequest(http.MethodGet, "http://tbaazarsm.gt4rents.com/login?url=%2F&host=answer.tbaazarsm.gt4rents.com&ssl=on", nil)
	req.Header.Set("Accept", "text/html")
	req.Host = "tbaazarsm.gt4rents.com"
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
	in := []byte(`<html><head></head><body><ul class="experiment-sidebar-navigationList_f1guxiff"><a data-name="support-lnk">Support</a></ul><a class="docs-lnk">Docs</a></body></html>`)
	out := string(injectDeviceHTML(in, "sagar"))
	if !strings.Contains(out, `data-tm-device`) {
		t.Fatal("device script missing")
	}
	if strings.Contains(out, `data-tm-hide-notice`) || strings.Contains(out, `data-tm-viewer-name`) {
		t.Fatal("unrelated hide or viewer script was injected")
	}
	if !strings.Contains(out, `docs-lnk`) || strings.Contains(out, `docs-lnk{display:none`) {
		t.Fatal("docs link was hidden")
	}
}

func TestLogoutPath(t *testing.T) {
	if !isLogoutPath("/sso/logout") || !isLogoutPath("/login") || isLogoutPath("/analytics/overview/") {
		t.Fatal("logout path detection")
	}
	if !bodyLooksLoggedOut([]byte(`<h1>Access Denied</h1><p>This section is restricted.</p>`)) {
		t.Fatal("restricted page should count as logged out")
	}
	if !bodyLooksLoggedOut([]byte(`<h1>Member Login</h1><a>Forgot password?</a>`)) {
		t.Fatal("markhor login page should count as logged out")
	}
	if bodyLooksLoggedOut([]byte(`<a class="srf-login-btn">Log In</a>`)) {
		t.Fatal("a login button alone is not logout")
	}
	if !bodyLooksLoggedOut([]byte(`<h1>Session Expired</h1><p>Your session has expired. Please access the dashboard again.</p>`)) {
		t.Fatal("toolbaazar session page should count as logged out")
	}
	if bodyLooksLoggedOut([]byte(`<title>Semrush Projects</title><script>console.log('Session expired, redirecting to logout...');</script>`)) {
		t.Fatal("semrush session timer is not a signed-out page")
	}
}

func TestGatewayPoolFollowsUpstream(t *testing.T) {
	const id = 620001
	t.Cleanup(func() { derivedCookies.Delete(id) })
	req := httptest.NewRequest(http.MethodGet, "http://tbaazarsm.gt4rents.com/", nil)
	req.Header.Set("Cookie", "ct_session=panel-session; user_pool=pool_9; gateway_bootstrap=from-browser")
	acc := ToolAccount{ID: id, Cookie: "gateway_bootstrap=seed; user_pool=pool_2"}
	got := cookiesForUpstream(acc, req)
	if !strings.Contains(got, "gateway_bootstrap=seed") || !strings.Contains(got, "user_pool=pool_2") {
		t.Fatalf("saved bootstrap and pool were not sent: %s", got)
	}
	if strings.Contains(got, "pool_9") || strings.Contains(got, "from-browser") {
		t.Fatalf("browser pool replaced the saved session: %s", got)
	}
	if strings.Contains(got, "ct_session") || strings.Contains(got, "panel-session") {
		t.Fatalf("panel session leaked upstream: %s", got)
	}
	refreshed := absorbSetCookies(got, http.Header{"Set-Cookie": []string{
		"gateway_bootstrap=fresh; Path=/; HttpOnly",
		"user_pool=pool_7; Path=/",
	}})
	rememberUpstreamCookies(acc, refreshed)
	again := cookiesForUpstream(acc, req)
	if !strings.Contains(again, "gateway_bootstrap=fresh") || !strings.Contains(again, "user_pool=pool_7") {
		t.Fatalf("gateway refresh did not win: %s", again)
	}
	if strings.Contains(again, "gateway_bootstrap=seed") || strings.Contains(again, "pool_9") || strings.Contains(again, "pool_2") {
		t.Fatalf("stale pool or bootstrap was sent: %s", again)
	}
	replaced := ToolAccount{ID: id, Cookie: "gateway_bootstrap=newer"}
	next := cookiesForUpstream(replaced, req)
	if !strings.Contains(next, "gateway_bootstrap=newer") {
		t.Fatalf("updated panel cookie was ignored: %s", next)
	}
	if strings.Contains(next, "pool_7") || strings.Contains(next, "gateway_bootstrap=fresh") {
		t.Fatalf("old pool survived a new bootstrap: %s", next)
	}
}

func TestBrowserSessionCookiesAreForwarded(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://tbaazarsm.gt4rents.com/gateway/experimentation/properties", nil)
	req.Header.Set("Cookie", "ct_session=panel-session; grauth=from-browser; csrf_token=tok; PHPSESSID=guest")
	got := cookiesForUpstream(ToolAccount{Cookie: "PHPSESSID=session"}, req)
	if !strings.Contains(got, "PHPSESSID=session") || !strings.Contains(got, "grauth=from-browser") || !strings.Contains(got, "csrf_token=tok") {
		t.Fatalf("browser session cookies were dropped: %s", got)
	}
	if strings.Contains(got, "PHPSESSID=guest") {
		t.Fatalf("browser session replaced the panel cookie: %s", got)
	}
	if strings.Contains(got, "ct_session") || strings.Contains(got, "panel-session") {
		t.Fatalf("panel session leaked upstream: %s", got)
	}
}

func TestSessionCookieFollowsUpstream(t *testing.T) {
	got := absorbSetCookies("PHPSESSID=session", http.Header{"Set-Cookie": []string{"grauth=fresh; Path=/; Secure", "csrf_token=tok; Path=/"}})
	if !strings.Contains(got, "PHPSESSID=session") || !strings.Contains(got, "grauth=fresh") || !strings.Contains(got, "csrf_token=tok") {
		t.Fatalf("cookies were not merged: %s", got)
	}
	next, ok := sameSiteRequestURI("https://sm.verygoodtool.com/docs?x=1", "sm.verygoodtool.com", "https://tbaazarsm.gt4rents.com")
	if !ok || next != "/docs?x=1" {
		t.Fatalf("same-site redirect %q %v", next, ok)
	}
	if _, ok := sameSiteRequestURI("https://example.com/docs", "sm.verygoodtool.com", "https://tbaazarsm.gt4rents.com"); ok {
		t.Fatal("off-site redirect must stay with the browser")
	}
	if _, ok := sameSiteRequestURI("/cdn-cgi/challenge-platform/h/g/scripts/precursor/main.js", "sm.verygoodtool.com", "https://tbaazarsm.gt4rents.com"); ok {
		t.Fatal("cloudflare challenge must not be followed")
	}

	oldWait := loginRetryWait
	loginRetryWait = 0
	t.Cleanup(func() { loginRetryWait = oldWait })
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.SetCookie(w, &http.Cookie{Name: "grauth", Value: "fresh", Path: "/"})
			http.Redirect(w, r, "/docs", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=session") || !strings.Contains(r.Header.Get("Cookie"), "grauth=fresh") {
			_, _ = w.Write([]byte(`<h1>Member Login</h1><a>Forgot password?</a>`))
			return
		}
		_, _ = w.Write([]byte(`<html><body>My docs</body></html>`))
	}))
	defer upstream.Close()
	cfg := Config{TargetURL: upstream.URL, HomePath: "/", PublicHost: "tbaazarsm.gt4rents.com", PublicScheme: "http", ToolName: "Toolbaazar Semrush"}
	req := httptest.NewRequest(http.MethodGet, "http://tbaazarsm.gt4rents.com/", nil)
	req.Header.Set("Accept", "text/html")
	req.Host = "tbaazarsm.gt4rents.com"
	rr := httptest.NewRecorder()
	proxyUpstream(rr, req, cfg, ToolAccount{Cookie: "PHPSESSID=session"})
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "My docs") {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
}

func TestMenuClickDoesNotFollowIntoAccessDenied(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/en/analyze/dashboard" {
			if r.Header.Get("X-Forwarded-Host") != "" || r.Header.Get("Origin") != "" {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte("Access Denied"))
				return
			}
			http.Redirect(w, r, "/en/project?domain=semrushtoolz.com", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("Access Denied"))
	}))
	defer upstream.Close()
	cfg := Config{TargetURL: upstream.URL, HomePath: "/en/dashboard", PublicHost: "tbaazarsm.gt4rents.com", PublicScheme: "http", ToolName: "Toolbaazar Semrush"}
	req := httptest.NewRequest(http.MethodGet, "http://tbaazarsm.gt4rents.com/en/analyze/dashboard", nil)
	req.Host = "tbaazarsm.gt4rents.com"
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Origin", "https://tbaazarsm.gt4rents.com")
	req.Header.Set("X-Forwarded-Host", "tbaazarsm.gt4rents.com")
	rr := httptest.NewRecorder()
	proxyUpstream(rr, req, cfg, ToolAccount{Cookie: "PHPSESSID=session"})
	if rr.Code != http.StatusFound {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Location") != "/en/project?domain=semrushtoolz.com" {
		t.Fatalf("location %s", rr.Header().Get("Location"))
	}
}

func TestOfficialSiteRedirectStaysInsideTheProxy(t *testing.T) {
	oldWait := loginRetryWait
	oldGateway := semrushGatewayURL
	loginRetryWait = 0
	t.Cleanup(func() {
		loginRetryWait = oldWait
		semrushGatewayURL = oldGateway
	})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "gateway_bootstrap=ready") {
			http.Redirect(w, r, "https://toolbaazar.com/", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body>Semrush projects</body></html>`))
	}))
	defer upstream.Close()
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=portal") {
			http.Redirect(w, r, "https://app.toolbaazar.com/login", http.StatusFound)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "gateway_bootstrap", Value: "ready", Path: "/"})
		http.Redirect(w, r, upstream.URL+"/projects", http.StatusFound)
	}))
	defer gateway.Close()
	semrushGatewayURL = gateway.URL + "/?backup=1"

	cfg := Config{TargetURL: upstream.URL, HomePath: "/", PublicHost: "tbaazarsm.gt4rents.com", PublicScheme: "http", ToolName: "Toolbaazar Semrush"}
	req := httptest.NewRequest(http.MethodGet, "http://tbaazarsm.gt4rents.com/", nil)
	req.Host = "tbaazarsm.gt4rents.com"
	req.Header.Set("Accept", "text/html")
	rr := httptest.NewRecorder()
	proxyUpstream(rr, req, cfg, ToolAccount{Cookie: "PHPSESSID=portal"})
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Semrush projects") {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Header().Get("Location"), "toolbaazar.com") {
		t.Fatal("browser was sent to the official site")
	}

	rr = httptest.NewRecorder()
	proxyUpstream(rr, req, cfg, ToolAccount{})
	if strings.Contains(rr.Header().Get("Location"), "toolbaazar.com") || strings.Contains(rr.Body.String(), "toolbaazar.com/") {
		t.Fatalf("dead session still left the proxy: %d %s %s", rr.Code, rr.Header().Get("Location"), rr.Body.String())
	}
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "Signing you in") {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
}
