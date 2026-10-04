package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	sessionActivateMu         sync.Mutex
	sessionActivatedKey       string
	sessionActivatedAt        time.Time
	sessionActivateFailUntil  time.Time
	sessionActivateTTL        = 5 * time.Minute
	sessionActivateSoftForce  = 45 * time.Second
	sessionActivateFailBackoff = 90 * time.Second
	sessionActivateInFlight   bool
	sessionActivateClientOnce sync.Once
	sessionActivateHTTP       *http.Client
)

type proxyBodyCacheKeyType struct{}

var proxyBodyCacheKey = proxyBodyCacheKeyType{}

func sessionActivateClient() *http.Client {
	sessionActivateClientOnce.Do(func() {
		sessionActivateHTTP = &http.Client{
			Timeout: 8 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        32,
				MaxIdleConnsPerHost: 16,
				IdleConnTimeout:     90 * time.Second,
				ForceAttemptHTTP2:   true,
			},
		}
	})
	return sessionActivateHTTP
}

func cacheRequestBodyForRetry(req *http.Request) {
	if req == nil || req.Body == nil {
		return
	}
	if req.Method == http.MethodGet || req.Method == http.MethodHead {
		return
	}
	if !isSemrushAPIRequest(req) {
		return
	}
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return
	}
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	req.ContentLength = int64(len(bodyBytes))
	*req = *req.WithContext(context.WithValue(req.Context(), proxyBodyCacheKey, bodyBytes))
}

func resolveUpstreamUA(cfg Config, ua string) string {
	ua = strings.TrimSpace(ua)
	if ua != "" {
		return ua
	}
	if cfg.UserAgent != "" {
		return cfg.UserAgent
	}
	return "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36"
}

// Fingerprint MUST use the same User-Agent that is sent upstream.
func semrushDeviceFingerprint(userAgent, cookieStr string) string {
	sso := cookieValueFromString(cookieStr, "sso_token")
	base := userAgent + "|" + sso
	if sso == "" {
		base = userAgent + "|nosso|" + cookieHash(cookieStr)
	}
	sum := sha256.Sum256([]byte(base))
	return hex.EncodeToString(sum[:16])
}

func parseMultiloginRedirect(location string) string {
	if location == "" {
		return ""
	}
	u, err := url.Parse(location)
	if err != nil {
		return ""
	}
	path := u.Path
	if !strings.Contains(strings.ToLower(path), "multilogin") {
		return ""
	}
	redirectTo := u.Query().Get("redirect_to")
	if redirectTo == "" || !strings.HasPrefix(redirectTo, "/") || strings.HasPrefix(redirectTo, "//") {
		return ""
	}
	return redirectTo
}

// ensureSemrushSessionActivated keeps the upstream Semrush session warm.
// HTTP is never done while holding the lock. Force is soft-throttled. Failures back off.
// Production: disabled — classic semrush-go-proxy never called activate; SSO hits from the
// VPS IP were a major trigger for Cloudflare reCAPTCHA on toolsmandi.
func ensureSemrushSessionActivated(cfg Config, cookieStr, userAgent string, force bool) {
	if !cfg.LocalTestMode {
		return
	}
	cookieStr = strings.TrimSpace(cookieStr)
	if cookieStr == "" {
		return
	}
	userAgent = resolveUpstreamUA(cfg, userAgent)
	fp := semrushDeviceFingerprint(userAgent, cookieStr)
	key := fp + "|" + cookieHash(cookieStr)

	sessionActivateMu.Lock()
	now := time.Now()
	if !force && now.Before(sessionActivateFailUntil) {
		sessionActivateMu.Unlock()
		return
	}
	age := now.Sub(sessionActivatedAt)
	fresh := sessionActivatedKey == key && age < sessionActivateTTL
	if fresh {
		if !force || age < sessionActivateSoftForce {
			sessionActivateMu.Unlock()
			return
		}
	}
	if sessionActivateInFlight {
		sessionActivateMu.Unlock()
		return
	}
	sessionActivateInFlight = true
	sessionActivateMu.Unlock()

	ok := doSemrushSessionActivate(cfg, cookieStr, userAgent, fp)

	sessionActivateMu.Lock()
	sessionActivateInFlight = false
	if ok {
		sessionActivatedKey = key
		sessionActivatedAt = time.Now()
		sessionActivateFailUntil = time.Time{}
	} else {
		sessionActivateFailUntil = time.Now().Add(sessionActivateFailBackoff)
	}
	sessionActivateMu.Unlock()
}

func doSemrushSessionActivate(cfg Config, cookieStr, userAgent, fp string) bool {
	sso := cookieValueFromString(cookieStr, "sso_token")
	jwt := cookieValueFromString(cookieStr, "SSO-JWT")
	if sso == "" && jwt == "" {
		log.Printf("[SESSION] ⚠️ skip activate — missing sso_token and SSO-JWT")
		return false
	}

	body := fmt.Sprintf(`{"user-agent-hash":"%s"}`, fp)
	req, err := http.NewRequest(http.MethodPost, "https://www.semrush.com/sso/user-sessions/activate", bytes.NewReader([]byte(body)))
	if err != nil {
		log.Printf("[SESSION] activate build request failed: %v", err)
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", "https://www.semrush.com")
	req.Header.Set("Referer", "https://www.semrush.com/")
	req.Header.Set("Custom-User-Hash", fp)
	req.Header.Set("Cookie", cookieStr)
	req.Header.Set("User-Agent", userAgent)

	resp, err := sessionActivateClient().Do(req)
	if err != nil {
		log.Printf("[SESSION] activate request failed: %v", err)
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		log.Printf("[SESSION] ✅ activated upstream session (status=%d fp=%s... sso=%t jwt=%t)", resp.StatusCode, fp[:8], sso != "", jwt != "")
		return true
	}
	log.Printf("[SESSION] ⚠️ activate failed status=%d fp=%s... sso=%t jwt=%t", resp.StatusCode, fp[:8], sso != "", jwt != "")
	return false
}

func cookieHash(cookieStr string) string {
	// Hash only stable auth cookies — full Cookie header order used to change every
	// request (Go map iteration), so activate cache never hit and every API waited on SSO.
	sso := cookieValueFromString(cookieStr, "sso_token")
	jwt := cookieValueFromString(cookieStr, "SSO-JWT")
	base := "sso=" + sso + "|jwt=" + jwt
	if sso == "" && jwt == "" {
		base = cookieStr
	}
	sum := sha256.Sum256([]byte(base))
	return hex.EncodeToString(sum[:8])
}

// ensureSemrushSessionActivatedAsync warms the session without blocking the proxy request.
func ensureSemrushSessionActivatedAsync(cfg Config, cookieStr, userAgent string) {
	cookieStr = strings.TrimSpace(cookieStr)
	if cookieStr == "" {
		return
	}
	go ensureSemrushSessionActivated(cfg, cookieStr, userAgent, false)
}

func shouldWarmSemrushSession(path string) bool {
	p := strings.ToLower(path)
	switch {
	case strings.Contains(p, "/sso/user-sessions/activate"):
		return false // this request itself activates upstream
	case strings.HasPrefix(p, "/static-proxy"), strings.HasPrefix(p, "/cdn-proxy"),
		strings.HasPrefix(p, "/secure-proxy"), strings.HasPrefix(p, "/ai-proxy"):
		return false
	case strings.Contains(p, "/smrwv/"), strings.Contains(p, "webvital"),
		strings.HasSuffix(p, ".js"), strings.HasSuffix(p, ".css"),
		strings.HasSuffix(p, ".png"), strings.HasSuffix(p, ".jpg"),
		strings.HasSuffix(p, ".woff2"), strings.HasSuffix(p, ".svg"):
		return false
	case strings.Contains(p, "/search-bar/"):
		return false
	default:
		return true
	}
}

func applySemrushSessionHeaders(req *http.Request, cfg Config, cookieStr, userAgent string) {
	ua := resolveUpstreamUA(cfg, userAgent)
	fp := semrushDeviceFingerprint(ua, cookieStr)
	if fp == "" {
		return
	}
	req.Header.Set("Custom-User-Hash", fp)
}

// Raw JS only — wrapped once by blockScript in main.go (no nested <script> tags).
func cookieValueFromString(cookieStr, name string) string {
	for _, part := range strings.Split(cookieStr, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, name+"=") {
			return strings.TrimPrefix(part, name+"=")
		}
	}
	return ""
}

// Injects SSO-JWT for gRPC-web (auth-data-jwt header). Browser cookies for 127.0.0.1
// do not include Semrush session data, so widget APIs never authenticate without this.
func buildGrpcWebFix(ssoJWT string) string {
	if strings.TrimSpace(ssoJWT) == "" {
		return ""
	}
	return fmt.Sprintf(`
(function(){
  var JWT = %q;
  var _setHeader = XMLHttpRequest.prototype.setRequestHeader;
  XMLHttpRequest.prototype.setRequestHeader = function(name, value) {
    var n = String(name || "").toLowerCase();
    if (n === "auth-data-jwt") {
      var v = value == null ? "" : String(value);
      if (!v || v === "null" || v === "undefined") value = JWT;
    }
    return _setHeader.call(this, name, value);
  };
})();
`, ssoJWT)
}

func buildSemrushSessionBootstrapJS(deviceFP string) string {
	return fmt.Sprintf(`
(function(){
  var FP = %q;
  function activateSession() {
    return fetch("/sso/user-sessions/activate", {
      method: "POST",
      credentials: "same-origin",
      headers: {"Content-Type":"application/json","Custom-User-Hash": FP},
      body: JSON.stringify({"user-agent-hash": FP})
    }).catch(function(){});
  }
  activateSession();
  if (window.location.pathname.indexOf("/multilogin") !== -1) {
    activateSession().then(function(){
      var p = new URLSearchParams(window.location.search);
      var to = p.get("redirect_to");
      if (to && to.charAt(0) === "/") window.location.replace(to);
    });
  }
})();
`, deviceFP)
}

func isSemrushAPIRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	p := strings.ToLower(r.URL.Path)
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return true
	}
	return strings.Contains(p, "/api/") || strings.Contains(p, "/widget/") || strings.Contains(p, "/rpc")
}

func retrySemrushUpstream(orig *http.Request, cfg Config, cookieStr string) (*http.Response, error) {
	if orig == nil || orig.URL == nil {
		return nil, fmt.Errorf("no request")
	}
	ua := resolveUpstreamUA(cfg, orig.Header.Get("User-Agent"))
	ensureSemrushSessionActivated(cfg, cookieStr, ua, true)

	upstreamURL := "https://www.semrush.com" + orig.URL.Path
	if orig.URL.RawQuery != "" {
		upstreamURL += "?" + orig.URL.RawQuery
	}

	var body io.Reader
	if orig.Method != http.MethodGet && orig.Method != http.MethodHead {
		if cached, ok := orig.Context().Value(proxyBodyCacheKey).([]byte); ok && len(cached) > 0 {
			body = bytes.NewReader(cached)
		}
	}

	req, err := http.NewRequestWithContext(orig.Context(), orig.Method, upstreamURL, body)
	if err != nil {
		return nil, err
	}
	req.Host = "www.semrush.com"

	for k, vals := range orig.Header {
		lk := strings.ToLower(k)
		if lk == "host" || lk == "cookie" || strings.HasPrefix(lk, "x-proxy-") {
			continue
		}
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("Cookie", cookieStr)
	req.Header.Set("User-Agent", ua)
	applySemrushSessionHeaders(req, cfg, cookieStr, ua)

	client := &http.Client{
		Timeout: 45 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        64,
			MaxIdleConnsPerHost: 32,
			IdleConnTimeout:     90 * time.Second,
			ForceAttemptHTTP2:   true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther || resp.StatusCode == http.StatusTemporaryRedirect {
		loc := strings.ToLower(resp.Header.Get("Location"))
		if strings.Contains(loc, "multilogin") {
			resp.Body.Close()
			return nil, fmt.Errorf("multilogin persists after activate")
		}
	}
	return resp, nil
}
