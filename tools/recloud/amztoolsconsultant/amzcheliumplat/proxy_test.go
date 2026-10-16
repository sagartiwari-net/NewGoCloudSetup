package main

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"recloud.dev/cdncache"
)

func TestRewriteKeepsBrowserOnPublicHost(t *testing.T) {
	in := []byte(`<a href="https://ht06.amztoolsconsultant.com/black-box/products">x</a><script src="https://cdn.ht06.amztoolsconsultant.com/asset.js"></script><script src="https://static.amztoolsconsultant.com/app.js"></script><script src="https://de.semrush.com/app.js"></script>{"cookie_domain":".amztoolsconsultant.com","host":"ht06.amztoolsconsultant.com"}`)
	out := string(rewriteText(in, "ht06.amztoolsconsultant.com", "https://amzcheliumplat.gt4rents.com"))
	if strings.Contains(out, "amztoolsconsultant.com") || strings.Contains(out, "semrush.com") {
		t.Fatalf("upstream host leaked: %s", out)
	}
	if !strings.Contains(out, "https://amzcheliumplat.gt4rents.com/black-box/products") || !strings.Contains(out, "https://amzcheliumplat.gt4rents.com/asset.js") {
		t.Fatal("public origin was not applied")
	}
	if !strings.Contains(out, ".amzcheliumplat.gt4rents.com") {
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
	if _, ok := rewriteSetCookie("PHPSESSID=secret; Path=/amztoolsconsultant-kp01; HttpOnly; Secure", "ht06.amztoolsconsultant.com", "https://amzcheliumplat.gt4rents.com"); ok {
		t.Fatal("auth session must not reach the browser")
	}
	got, ok := rewriteSetCookie("snav_last_sidebar_tk=toolkit_seo; Domain=.semrush.com; Path=/", "ht06.amztoolsconsultant.com", "https://amzcheliumplat.gt4rents.com")
	if !ok || !strings.Contains(got, "Domain=amzcheliumplat.gt4rents.com") || strings.Contains(got, "semrush.com") {
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
		PublicHost:   "amzcheliumplat.gt4rents.com",
		PublicScheme: "http",
		ToolName:     "Amztoolsconsultant Helium10 Platinum",
	}
	req := httptest.NewRequest(http.MethodGet, "http://amzcheliumplat.gt4rents.com/login?url=%2F&host=answer.amzcheliumplat.gt4rents.com&ssl=on", nil)
	req.Header.Set("Accept", "text/html")
	req.Host = "amzcheliumplat.gt4rents.com"
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
	if !strings.Contains(out, `<style data-tm-hide-reset>#ht05-storage-reset-button{display:none!important}</style>`) {
		t.Fatal("reset button was not hidden")
	}
	if !strings.Contains(out, `id="tm-ext-download"`) || !strings.Contains(out, `href="/ext-install"`) {
		t.Fatal("extension download button missing")
	}
	if strings.Contains(out, `data-tm-hide-notice`) || strings.Contains(out, `data-tm-viewer-name`) {
		t.Fatal("unrelated hide or viewer script was injected")
	}
	if !strings.Contains(out, `docs-lnk`) || strings.Contains(out, `docs-lnk{display:none`) {
		t.Fatal("docs link was hidden")
	}
}

func TestExtensionLimitsReturnsPanelUser(t *testing.T) {
	token := "ext-limits-token"
	panelSess.Store(token, &panelGateSession{username: "sagar", expires: time.Now().Add(time.Hour)})
	t.Cleanup(func() { panelSess.Delete(token) })
	req := httptest.NewRequest(http.MethodGet, "https://amzcheliumplat.gt4rents.com/api/user-limits", nil)
	req.AddCookie(&http.Cookie{Name: "ct_session", Value: token})
	rr := httptest.NewRecorder()
	userLimitsHandler(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"username":"sagar"`) {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
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
	if !bodyLooksLoggedOut([]byte(`<h1>Access Denied</h1><p>Direct access is not allowed. Please login to your Members Area to launch the browser.</p>`)) {
		t.Fatal("members area wall should count as logged out")
	}
	if !bodyLooksLoggedOut([]byte(`<h1>Members Area</h1><p>Login to your Account</p><a>Forgot password?</a>`)) {
		t.Fatal("members login should count as logged out")
	}
	if bodyLooksLoggedOut([]byte(`<title>Cloud Browser</title><span id="status-text">Connecting...</span>`)) {
		t.Fatal("cloud browser shell is not logged out")
	}
}

func TestProxyTokenIsSentUpstream(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://amzcheliumplat.gt4rents.com/", nil)
	req.Header.Set("Cookie", "ct_session=panel-session")
	got := cookiesForUpstream(ToolAccount{Cookie: "PHPSESSID=saved"}, req)
	if !strings.Contains(got, "PHPSESSID=saved") {
		t.Fatalf("auth session was not sent upstream: %s", got)
	}
	if strings.Contains(got, "ct_session") || strings.Contains(got, "panel-session") {
		t.Fatalf("panel session leaked upstream: %s", got)
	}
}

func TestBrowserSessionCookiesAreForwarded(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://amzcheliumplat.gt4rents.com/gateway/experimentation/properties", nil)
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
	next, ok := sameSiteRequestURI("https://ht06.amztoolsconsultant.com/docs?x=1", "ht06.amztoolsconsultant.com", "https://amzcheliumplat.gt4rents.com")
	if !ok || next != "/docs?x=1" {
		t.Fatalf("same-site redirect %q %v", next, ok)
	}
	next, ok = sameSiteRequestURI("https://members.amztoolsconsultant.com/protect/new-rewrite?f=77&url=/black-box/products", "ht06.amztoolsconsultant.com", "https://amzcheliumplat.gt4rents.com")
	if !ok || !strings.HasPrefix(next, "https://members.amztoolsconsultant.com/protect/new-rewrite") {
		t.Fatalf("members protect redirect %q %v", next, ok)
	}
	base, err := url.Parse("https://ht06.amztoolsconsultant.com")
	if err != nil || upstreamURL(base, next) != next {
		t.Fatalf("protect hop stayed on the tool host: %s", upstreamURL(base, next))
	}
	if _, ok := sameSiteRequestURI("https://example.com/docs", "ht06.amztoolsconsultant.com", "https://amzcheliumplat.gt4rents.com"); ok {
		t.Fatal("off-site redirect must stay with the browser")
	}
	if _, ok := sameSiteRequestURI("/cdn-cgi/challenge-platform/h/g/scripts/precursor/main.js", "ht06.amztoolsconsultant.com", "https://amzcheliumplat.gt4rents.com"); ok {
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
	cfg := Config{TargetURL: upstream.URL, HomePath: "/", PublicHost: "amzcheliumplat.gt4rents.com", PublicScheme: "http", ToolName: "Amztoolsconsultant Helium10 Platinum"}
	req := httptest.NewRequest(http.MethodGet, "http://amzcheliumplat.gt4rents.com/", nil)
	req.Header.Set("Accept", "text/html")
	req.Host = "amzcheliumplat.gt4rents.com"
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
	cfg := Config{TargetURL: upstream.URL, HomePath: "/en/dashboard", PublicHost: "amzcheliumplat.gt4rents.com", PublicScheme: "http", ToolName: "Amztoolsconsultant Helium10 Platinum"}
	req := httptest.NewRequest(http.MethodGet, "http://amzcheliumplat.gt4rents.com/en/analyze/dashboard", nil)
	req.Host = "amzcheliumplat.gt4rents.com"
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Origin", "https://amzcheliumplat.gt4rents.com")
	req.Header.Set("X-Forwarded-Host", "amzcheliumplat.gt4rents.com")
	rr := httptest.NewRecorder()
	proxyUpstream(rr, req, cfg, ToolAccount{Cookie: "PHPSESSID=session"})
	if rr.Code != http.StatusFound {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Location") != "/en/project?domain=semrushtoolz.com" {
		t.Fatalf("location %s", rr.Header().Get("Location"))
	}
}

func TestBakedExtensionNameAndProxy(t *testing.T) {
	raw, err := bakeExtensionBytes("https://amzcheliumplat.gt4rents.com", "amzcheliumplat.gt4rents.com")
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		t.Helper()
		for _, f := range zr.File {
			if f.Name != name {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer rc.Close()
			buf := new(bytes.Buffer)
			if _, err := buf.ReadFrom(rc); err != nil {
				t.Fatal(err)
			}
			return buf.String()
		}
		t.Fatalf("missing %s", name)
		return ""
	}
	manifest := read("manifest.json")
	if !strings.Contains(manifest, `"name": "Amzc Helium10 Extension"`) {
		t.Fatal("manifest name was not set")
	}
	if !strings.Contains(manifest, "Amzc Helium10 Extension for product research") {
		t.Fatal("manifest description was not set")
	}
	hosts := read("tool-hosts.js")
	if !strings.Contains(hosts, "amzcheliumdia.gt4rents.com") || !strings.Contains(hosts, "amzcheliumplat.gt4rents.com") {
		t.Fatal("extension must list both Helium tool hosts")
	}
	bg := read("background-script.js")
	if strings.Contains(bg, "__H10_PROXY_ORIGIN__") {
		t.Fatal("extension still bakes a single tool origin")
	}
	if !strings.Contains(bg, "isToolHost") {
		t.Fatal("extension does not choose the open tool host")
	}
	req := httptest.NewRequest(http.MethodGet, "https://amzcheliumplat.gt4rents.com/extension.zip", nil)
	req.Host = "amzcheliumplat.gt4rents.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()
	extensionZipHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("zip status %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Disposition"); got != `attachment; filename="Amzc Helium10 Extension.zip"` {
		t.Fatalf("download name %s", got)
	}
}
