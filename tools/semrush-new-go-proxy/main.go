package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func getConfigFile() string {
	if p := strings.TrimSpace(os.Getenv("CONFIG_FILE")); p != "" {
		return p
	}
	return "config.json"
}

func cookieFilePath(cfg Config) string {
	if strings.TrimSpace(cfg.CookieFile) != "" {
		return cfg.CookieFile
	}
	return "cookie.txt"
}

// Config structures
type Config struct {
	Port          string `json:"port"`
	TargetURL     string `json:"target_url"`
	PublicHost    string `json:"public_host"`
	PublicScheme  string `json:"public_scheme"`
	UserAgent     string `json:"user_agent"`
	WebsiteID     int    `json:"website_id"`
	CookieFile    string `json:"cookie_file"`
	LocalTestMode bool   `json:"local_test_mode"`
	BindLocalhost bool   `json:"bind_localhost"`
	DebugLogging  bool   `json:"debug_logging"`
	PanelDB       string `json:"panel_db"`
	// MySQL configs
	MySQLHost     string `json:"mysql_host"`
	MySQLPort     string `json:"mysql_port"`
	MySQLUser     string `json:"mysql_user"`
	MySQLPassword string `json:"mysql_password"`
	MySQLDB       string `json:"mysql_db"`
	// UpstreamProxy routes Semrush HTTPS via residential/datacenter proxy (http/socks5).
	// Prefer proxy.txt on server (not committed). Example: socks5://user:pass@host:1080
	UpstreamProxy string `json:"upstream_proxy"`
}

var (
	currentConfig Config
	configModTime time.Time

	// DB handles
	db *sql.DB

	// Regexp for stripping Subresource Integrity (SRI)
	integrityRegex      = regexp.MustCompile(`(?i)\s*integrity=(?:"[^"]*"|'[^']*')`)
	webpackAttrRegex    = regexp.MustCompile(`(?i)(?:"integrity"|'integrity')\s*:`)
	webpackSetAttrRegex = regexp.MustCompile(`(?i)setAttribute\(\s*[\'"]integrity[\'"]`)

	// Session → account cookie cache (avoids 2 MySQL round-trips per request)
	sessionCookieCacheMu sync.RWMutex
	sessionCookieCache   = make(map[string]sessionCookieCacheEntry)

	exportToolJSMu    sync.Mutex
	exportToolJS      string
	exportToolJSMtime time.Time
)

type sessionCookieCacheEntry struct {
	accID     int
	cookie    string
	userAgent string
	proxy     string
	expires   time.Time
}

// loadConfig loads the configuration from config.json with hot-reloading
func loadConfig() Config {
	configFile := getConfigFile()
	info, err := os.Stat(configFile)
	if err != nil {
		return currentConfig
	}
	if !info.ModTime().After(configModTime) {
		return currentConfig
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		log.Printf("[CONFIG] Error reading config file: %v", err)
		return currentConfig
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("[CONFIG] Error parsing config JSON: %v", err)
		return currentConfig
	}

	if cfg.Port == "" {
		cfg.Port = "7850"
	}
	if cfg.TargetURL == "" {
		cfg.TargetURL = "https://www.semrush.com"
	}
	if cfg.PublicHost == "" {
		cfg.PublicHost = "localhost:" + cfg.Port
	}
	if cfg.PublicScheme == "" {
		cfg.PublicScheme = "http"
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"
	}

	// Default Semrush WebsiteID if missing
	if cfg.WebsiteID == 0 {
		cfg.WebsiteID = 3
	}
	if cfg.CookieFile == "" {
		cfg.CookieFile = "cookie.txt"
	}

	// Default MySQL connections only when not in local cookie-file mode
	if !cfg.LocalTestMode && cfg.MySQLHost != "" {
		if cfg.MySQLPort == "" {
			cfg.MySQLPort = "3306"
		}
		if cfg.MySQLUser == "" {
			cfg.MySQLUser = "root"
		}
		if cfg.MySQLDB == "" {
			cfg.MySQLDB = "toolsmandirefct"
		}
	}

	currentConfig = cfg
	configModTime = info.ModTime()
	log.Printf("[CONFIG] Config loaded successfully (Port: %s, Host: %s, WebsiteID: %d) ✅", cfg.Port, cfg.PublicHost, cfg.WebsiteID)
	return cfg
}

// mergeCookies merges incoming client cookies with the premium cookies from cookie.txt.
// Premium cookies override incoming client cookies if there are conflicts.
func mergeCookies(clientCookieStr, premiumCookieStr string) string {
	if premiumCookieStr == "" {
		return clientCookieStr
	}
	if clientCookieStr == "" {
		return premiumCookieStr
	}

	// Parse client cookies
	cookies := make(map[string]string)
	parts := strings.Split(clientCookieStr, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		subparts := strings.SplitN(part, "=", 2)
		if len(subparts) == 2 {
			cookies[subparts[0]] = subparts[1]
		}
	}

	// Overwrite/Merge with premium cookies
	premiumParts := strings.Split(premiumCookieStr, ";")
	for _, part := range premiumParts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		subparts := strings.SplitN(part, "=", 2)
		if len(subparts) == 2 {
			cookies[subparts[0]] = subparts[1]
		}
	}

	// Stable order so Cookie header hash / caching stays consistent across requests.
	names := make([]string, 0, len(cookies))
	for k := range cookies {
		names = append(names, k)
	}
	sort.Strings(names)
	merged := make([]string, 0, len(names))
	for _, k := range names {
		merged = append(merged, fmt.Sprintf("%s=%s", k, cookies[k]))
	}
	return strings.Join(merged, "; ")
}

func resolveWebsiteID(publicHost string) int {
	if wid := lookupWebsiteIDByHost(publicHost); wid > 0 {
		log.Printf("[DB] Resolved website_id = %d for domain '%s' ✅", wid, normalizeHost(publicHost))
		return wid
	}
	cfg := loadConfig()
	if cfg.WebsiteID > 0 {
		return cfg.WebsiteID
	}
	log.Printf("[DB] ⚠️ Domain '%s' not registered — using website_id = 1", normalizeHost(publicHost))
	return 1
}

type Account struct {
	ID        int
	Name      string
	Cookie    string
	UserAgent string
	Proxy     string
}

func selectActiveAccount(websiteID int) (Account, error) {
	var acc Account
	query := "SELECT id, name, cookie, user_agent, COALESCE(proxy,'') FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' ORDER BY last_used_at ASC LIMIT 1"
	err := db.QueryRow(query, websiteID).Scan(&acc.ID, &acc.Name, &acc.Cookie, &acc.UserAgent, &acc.Proxy)
	if err != nil {
		return acc, err
	}

	// Update last_used_at to rotate account usage
	_, _ = db.Exec("UPDATE ahrefs_accounts SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?", acc.ID)
	return acc, nil
}

func cachedSessionAccount(sessionToken string) (sessionCookieCacheEntry, bool) {
	sessionCookieCacheMu.RLock()
	ent, ok := sessionCookieCache[sessionToken]
	sessionCookieCacheMu.RUnlock()
	if !ok || time.Now().After(ent.expires) || ent.cookie == "" {
		return sessionCookieCacheEntry{}, false
	}
	return ent, true
}

func putSessionAccountCache(sessionToken string, accID int, cookie, ua, proxyStr string) {
	if sessionToken == "" || cookie == "" {
		return
	}
	sessionCookieCacheMu.Lock()
	sessionCookieCache[sessionToken] = sessionCookieCacheEntry{
		accID:     accID,
		cookie:    cookie,
		userAgent: ua,
		proxy:     proxyStr,
		expires:   time.Now().Add(45 * time.Second),
	}
	sessionCookieCacheMu.Unlock()
}

func invalidateSessionAccountCache(sessionToken string) {
	if sessionToken == "" {
		return
	}
	sessionCookieCacheMu.Lock()
	delete(sessionCookieCache, sessionToken)
	sessionCookieCacheMu.Unlock()
}

func loadExportToolJS() string {
	info, err := os.Stat("export-tool.js")
	if err != nil {
		return ""
	}
	exportToolJSMu.Lock()
	defer exportToolJSMu.Unlock()
	if exportToolJS != "" && !info.ModTime().After(exportToolJSMtime) {
		return exportToolJS
	}
	data, err := os.ReadFile("export-tool.js")
	if err != nil {
		return exportToolJS
	}
	exportToolJS = string(data)
	exportToolJSMtime = info.ModTime()
	return exportToolJS
}

func bodyNeedsSemrushRewrite(body []byte) bool {
	return bytes.Contains(body, []byte("semrush.com")) ||
		bytes.Contains(body, []byte("integrity=")) ||
		bytes.Contains(body, []byte(`"integrity"`))
}

func realClientIP(r *http.Request) string {
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Original-Forwarded-For"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func generateSessionToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// sanitizeCookieHeader removes invalid HTTP/2 header characters from a cookie string.
func sanitizeCookieHeader(s string) string {
	replacer := strings.NewReplacer("\r", "", "\n", "", "\x00", "", "\t", " ")
	return strings.TrimSpace(replacer.Replace(s))
}

// browserCookie is one cookie from a JSON array or a GoAuto cookies field.
type browserCookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// cookieEntriesToHeader joins name=value pairs. Empty names are skipped.
func cookieEntriesToHeader(cookies []browserCookie) string {
	parts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		if c.Name == "" || strings.TrimSpace(c.Value) == "" {
			continue
		}
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

// cookieHeaderFromStored accepts:
//   - a raw Cookie header
//   - a browser JSON array (no referer)
//   - a GoAuto object {"referer":"...", "cookies":[...]} — referer is optional
func cookieHeaderFromStored(raw string) string {
	raw = sanitizeCookieHeader(raw)
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	if strings.HasPrefix(trimmed, "{") {
		var wrap struct {
			Cookies []browserCookie `json:"cookies"`
		}
		if err := json.Unmarshal([]byte(trimmed), &wrap); err == nil && len(wrap.Cookies) > 0 {
			return cookieEntriesToHeader(wrap.Cookies)
		}
		return raw
	}

	if strings.HasPrefix(trimmed, "[") {
		var cookies []browserCookie
		if err := json.Unmarshal([]byte(trimmed), &cookies); err != nil {
			log.Printf("[COOKIE] ⚠️ Could not parse JSON cookie array: %v — using raw value", err)
			return raw
		}
		return cookieEntriesToHeader(cookies)
	}
	return raw
}

// parseCookieFromDB converts the stored cookie to HTTP Cookie header format.
func parseCookieFromDB(raw string) string {
	return cookieHeaderFromStored(raw)
}

func renderAccessDeniedPage(w http.ResponseWriter) {
	writeLightCard(w, http.StatusForbidden, lightCard{
		Title:   "Access Denied",
		Heading: "Access Denied",
		Message: "You cannot open <span class=\"brand\">Semrush</span> directly. Open it again from your access link.",
		Footer:  "Your session ended or this browser is not authorized",
	})
}

// loadCookies reads the cookie string from cookie.txt
func loadCookies(cfg Config) string {
	if ck := panelSemrushCookie(cfg.PublicHost); ck != "" {
		return ck
	}
	log.Printf("[COOKIE] panel cookie empty, using %s", cookieFilePath(cfg))
	data, err := os.ReadFile(cookieFilePath(cfg))
	if err != nil {
		return ""
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
		return cookieHeaderFromStored(trimmed)
	}

	// 2. Raw Cookie text string fallback
	lines := strings.Split(trimmed, "\n")
	var cookieParts []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		cookieParts = append(cookieParts, line)
	}
	return strings.Join(cookieParts, "; ")
}

// detectLimitOrLogout scans Semrush response body for free account limits, upgrade CTAs, or logouts.
func detectLimitOrLogout(body string) bool {
	// 1. Upgrade button (using the specific ID)
	if strings.Contains(body, `id="srf-header-upgrade-button"`) ||
		strings.Contains(body, `id='srf-header-upgrade-button'`) ||
		strings.Contains(body, `srf-header-upgrade-button`) {
		return true
	}
	// 2. Used 10 free requests limit
	if strings.Contains(body, `You’ve used 10 free requests`) ||
		strings.Contains(body, `You've used 10 free requests`) {
		return true
	}
	// 3. Register to get 10 free requests
	if strings.Contains(body, `Register to get 10 free requests`) {
		return true
	}
	// 4. Log In button (indicates account is logged out of upstream)
	if strings.Contains(body, `srf-login-btn`) || strings.Contains(body, `auth-popup__btn-login`) {
		return true
	}
	// 5. Already registered? (Only in SSO registration header context to prevent generic page matches)
	if strings.Contains(body, `Already registered?`) &&
		(strings.Contains(body, `sso-header`) || strings.Contains(body, `Register to get`)) {
		return true
	}
	return false
}

// semrushLoggedOutHTML is true only for the Semrush header Log In control
// and the Register / Sign in wall. A keyword or report that contains the
// word login is not a logout.
func semrushLoggedOutHTML(body string) bool {
	if strings.Contains(body, `href="/login/?src=header`) ||
		strings.Contains(body, `id="snav-header-log-in-button"`) ||
		strings.Contains(body, `snav-header__menu-link--login-button`) ||
		strings.Contains(body, `data-ga4-item-location="navigation.header.login"`) ||
		strings.Contains(body, `srf-header__link-button srf-login-btn`) ||
		strings.Contains(body, `data-test="auth-popup__btn-login"`) ||
		strings.Contains(body, `aria-label="Already registered? Log in to Semrush"`) {
		return true
	}
	if strings.Contains(body, `data-test="sso-header"`) && strings.Contains(body, `Register to get`) {
		return true
	}
	if strings.Contains(body, `sso-block`) && (strings.Contains(body, `Register to get 10 free requests`) || strings.Contains(body, `data-test="sso-limit-popup-register-submit"`) || strings.Contains(body, `data-auth="google-auth-button"`)) {
		return true
	}
	return false
}

// renderSwappingPage returns a gorgeous premium loading screen with custom redirect action
func renderSwappingPage(redirectTo string) string {
	script := "window.location.reload();"
	if redirectTo != "" {
		script = fmt.Sprintf("window.location.href = %q;", redirectTo)
	}

	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Rotating Premium Session — ToolsMandi</title>
    <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@300;400;600;800&display=swap" rel="stylesheet">
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: 'Outfit', sans-serif;
            background: radial-gradient(circle at center, #0f172a 0%, #020617 100%);
            color: #ffffff;
            height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            overflow: hidden;
        }
        .container {
            text-align: center;
            padding: 40px 60px;
            background: rgba(255, 255, 255, 0.03);
            border: 1px solid rgba(255, 255, 255, 0.05);
            border-radius: 24px;
            backdrop-filter: blur(20px);
            box-shadow: 0 20px 50px rgba(0, 0, 0, 0.6), 0 0 40px rgba(99, 102, 241, 0.1);
            max-width: 500px;
            width: 90%;
            animation: fadeIn 0.6s ease-out;
        }
        .loader-wrapper {
            position: relative;
            width: 80px;
            height: 80px;
            margin: 0 auto 30px auto;
        }
        .loader {
            width: 100%;
            height: 100%;
            border: 4px solid rgba(99, 102, 241, 0.1);
            border-top-color: #6366f1;
            border-radius: 50%;
            animation: spin 1s cubic-bezier(0.5, 0, 0.5, 1) infinite;
        }
        .loader-inner {
            position: absolute;
            top: 10px;
            left: 10px;
            right: 10px;
            bottom: 10px;
            border: 4px solid rgba(236, 72, 153, 0.1);
            border-top-color: #ec4899;
            border-radius: 50%;
            animation: spin-reverse 1.5s cubic-bezier(0.5, 0, 0.5, 1) infinite;
        }
        .glow {
            position: absolute;
            top: 50%;
            left: 50%;
            transform: translate(-50%, -50%);
            width: 120px;
            height: 120px;
            background: radial-gradient(circle, rgba(99, 102, 241, 0.15) 0%, rgba(99, 102, 241, 0) 70%);
            animation: pulse 2s ease-in-out infinite;
        }
        h1 {
            font-size: 26px;
            font-weight: 800;
            margin-bottom: 12px;
            background: linear-gradient(135deg, #6366f1 0%, #ec4899 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            letter-spacing: -0.5px;
        }
        p {
            font-size: 15px;
            color: #94a3b8;
            line-height: 1.6;
            font-weight: 400;
        }
        .dots {
            display: inline-block;
            margin-left: 2px;
            animation: blink 1.4s infinite both;
        }
        @keyframes spin {
            0% { transform: rotate(0deg); }
            100% { transform: rotate(360deg); }
        }
        @keyframes spin-reverse {
            0% { transform: rotate(360deg); }
            100% { transform: rotate(0deg); }
        }
        @keyframes pulse {
            0%, 100% { transform: translate(-50%, -50%) scale(0.9); opacity: 0.5; }
            50% { transform: translate(-50%, -50%) scale(1.1); opacity: 1; }
        }
        @keyframes fadeIn {
            from { opacity: 0; transform: translateY(10px); }
            to { opacity: 1; transform: translateY(0); }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="loader-wrapper">
            <div class="glow"></div>
            <div class="loader"></div>
            <div class="loader-inner"></div>
        </div>
        <h1>Rotating Session</h1>
        <p>Switching to a fresh premium account for uninterrupted access. Please wait<span class="dots">...</span></p>
    </div>
    <script>
        setTimeout(function() {
            ` + script + `
        }, 1800);
    </script>
</body>
</html>`
}

func rewriteSemrushPublicURL(raw, proxyScheme, proxyHost string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || proxyHost == "" {
		return raw
	}
	if !strings.Contains(strings.ToLower(trimmed), "semrush.com") {
		return raw
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return raw
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "semrush.com" && !strings.HasSuffix(host, ".semrush.com") {
		return raw
	}
	if proxyScheme == "" {
		proxyScheme = "http"
	}
	prefix := ""
	switch host {
	case "static.semrush.com":
		prefix = "/static-proxy"
	case "secure.semrush.com":
		prefix = "/secure-proxy"
	case "cdn.semrush.com":
		prefix = "/cdn-proxy"
	case "ai-visibility-index.semrush.com":
		prefix = "/ai-proxy"
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	if prefix != "" {
		if path == "/" {
			path = prefix + "/"
		} else {
			path = prefix + path
		}
	}
	out := proxyScheme + "://" + proxyHost + path
	if parsed.RawQuery != "" {
		out += "?" + parsed.RawQuery
	}
	if parsed.Fragment != "" {
		out += "#" + parsed.Fragment
	}
	return out
}

var semrushAbsURL = regexp.MustCompile(`https?://(?:[a-zA-Z0-9-]+\.)*semrush\.com[^"'\\\s<>]*`)

// Content Toolkit's router assigns this when a link is another host.
// location.href cannot be hooked, so the assigned URL has to already be local.
var semrushExternalNav = regexp.MustCompile(`window\.location\.href=([A-Za-z_$][\w$]*)\.absoluteURL\|\|([A-Za-z_$][\w$]*)\.to`)

// Protocol-relative //www.semrush.com is not matched by https?://. The char before //
// must not be ":", so an already-absolute https:// URL is left for semrushAbsURL.
var semrushProtoRelURL = regexp.MustCompile(`([^:])(//(?:[a-zA-Z0-9-]+\.)*semrush\.com[^"'\\\s<>]*)`)

func rewriteSemrushHostsInBody(body, proxyScheme, proxyHost string) string {
	if !strings.Contains(strings.ToLower(body), "semrush.com") || proxyHost == "" {
		return body
	}
	if proxyScheme == "" {
		proxyScheme = "http"
	}
	body = semrushAbsURL.ReplaceAllStringFunc(body, func(match string) string {
		return rewriteSemrushPublicURL(match, proxyScheme, proxyHost)
	})
	return semrushProtoRelURL.ReplaceAllStringFunc(body, func(match string) string {
		parts := semrushProtoRelURL.FindStringSubmatch(match)
		if len(parts) < 3 {
			return match
		}
		return parts[1] + rewriteSemrushPublicURL("https:"+parts[2], proxyScheme, proxyHost)
	})
}

func buildLocalHostFix(cfg Config) string {
	if !cfg.LocalTestMode {
		return ""
	}
	fakeHostname := "www.semrush.com"
	if u, err := url.Parse(cfg.TargetURL); err == nil && u.Hostname() != "" {
		fakeHostname = u.Hostname()
	}
	proxyHostname := cfg.PublicHost
	if host, _, err := net.SplitHostPort(cfg.PublicHost); err == nil {
		proxyHostname = host
	}
	return fmt.Sprintf(`
try {
    var REAL_PROXY_ORIGIN = window.location.origin;
    var REAL_PROXY_HOST = window.location.host;
    var REAL_PROXY_PROTOCOL = window.location.protocol;
    var FAKE_NAME = %q;
    var PROXY_NAME = %q;
    function isProxyHost(h) {
        return h === '127.0.0.1' || h === 'localhost' || h === PROXY_NAME;
    }
    // Do not spoof hostname. Content Toolkit reads it and sends the browser
    // to https://www.semrush.com, and cookie code would set domain=semrush.com.
    // gRPC stays on this origin because location.origin is the proxy.
    void FAKE_NAME;
    void isProxyHost;
} catch(e) {}
`, fakeHostname, proxyHostname)
}

func main() {
	cfg := loadConfig()

	if cfg.LocalTestMode || strings.TrimSpace(cfg.MySQLHost) == "" {
		log.Printf("[LOCAL] local_test_mode — using %s (no MySQL auth)", cookieFilePath(cfg))
		db = nil
	} else {
		// Initialize MySQL DB connection if available
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=Asia%%2FKolkata",
			cfg.MySQLUser, cfg.MySQLPassword, cfg.MySQLHost, cfg.MySQLPort, cfg.MySQLDB)
		var err error
		db, err = sql.Open("mysql", dsn)
		if err != nil {
			log.Printf("[DB] ⚠️ MySQL Connection failed to initialize: %v", err)
		} else {
			db.SetMaxOpenConns(50)
			db.SetMaxIdleConns(20)
			db.SetConnMaxLifetime(10 * time.Minute)
			db.SetConnMaxIdleTime(3 * time.Minute)
			if err := db.Ping(); err != nil {
				log.Printf("[DB] ⚠️ Could not connect to MySQL: %v. Database mode is offline.", err)
				db = nil
			} else {
				log.Printf("[DB] Connected to MySQL successfully! Database: %s ✅", cfg.MySQLDB)
				_ = resolveWebsiteID(cfg.PublicHost)
			}
		}
	}

	targetUrl, err := url.Parse(cfg.TargetURL)
	if err != nil {
		log.Fatalf("[FATAL] Invalid target URL: %v", err)
	}

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(targetUrl)
	proxy.Transport = buildUpstreamTransport()
	proxy.FlushInterval = -1
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if rl := getRequestLogger(r.Context()); rl != nil {
			rl.errMsg = err.Error()
			rl.status = http.StatusBadGateway
		}
		log.Printf("[PROXY:ERR] %s %s: %v", r.Method, r.URL.Path, err)
		if strings.Contains(err.Error(), "upstream proxy") {
			if semrushDocument(r) || strings.Contains(r.Header.Get("Accept"), "text/html") {
				renderSemrushProxyProblem(w)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprint(w, `{"error":"proxy_problem","message":"Contact to Admin/Provider to fix it ASAP"}`)
			return
		}
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	// Proxy Director (already handles dynamic DB loading we added)
	proxy.Director = func(req *http.Request) {
		cfg = loadConfig()
		targetHost := "www.semrush.com"
		if strings.HasPrefix(req.URL.Path, "/secure-proxy/") || req.URL.Path == "/secure-proxy" {
			targetHost = "secure.semrush.com"
			req.URL.Path = "/" + strings.TrimPrefix(strings.TrimPrefix(req.URL.Path, "/secure-proxy"), "/")
		} else if strings.HasPrefix(req.URL.Path, "/static-proxy/") || req.URL.Path == "/static-proxy" {
			targetHost = "static.semrush.com"
			req.URL.Path = "/" + strings.TrimPrefix(strings.TrimPrefix(req.URL.Path, "/static-proxy"), "/")
		} else if strings.HasPrefix(req.URL.Path, "/cdn-proxy/") || req.URL.Path == "/cdn-proxy" {
			targetHost = "cdn.semrush.com"
			req.URL.Path = "/" + strings.TrimPrefix(strings.TrimPrefix(req.URL.Path, "/cdn-proxy"), "/")
		} else if strings.HasPrefix(req.URL.Path, "/ai-proxy/") || req.URL.Path == "/ai-proxy" {
			targetHost = "ai-visibility-index.semrush.com"
			req.URL.Path = "/" + strings.TrimPrefix(strings.TrimPrefix(req.URL.Path, "/ai-proxy"), "/")
		}

		// Dynamic public host & scheme detection
		publicHost := req.Host
		if publicHost == "" {
			publicHost = cfg.PublicHost
		}
		publicScheme := cfg.PublicScheme
		if req.TLS != nil {
			publicScheme = "https"
		} else if proto := req.Header.Get("X-Forwarded-Proto"); proto != "" {
			publicScheme = proto
		} else if publicScheme == "" {
			publicScheme = "http"
		}

		req.Header.Set("X-Proxy-Host", publicHost)
		req.Header.Set("X-Proxy-Scheme", publicScheme)

		req.Header.Set("Host", targetHost)
		req.Host = targetHost
		req.URL.Host = targetHost
		req.URL.Scheme = "https"

		// Set Browser Spoof Headers & Database Dynamic Cookie Fetching
		var dbCookie, dbUserAgent, dbProxy string
		var assignedAccountID sql.NullInt64
		var accID int
		websiteID := 0

		if db != nil {
			websiteID = tenantWebsiteID(req.Context())
			if websiteID <= 0 {
				if sessionCookie, errC := req.Cookie("sem_session"); errC == nil && sessionCookie != nil && sessionCookie.Value != "" {
					_ = db.QueryRow("SELECT website_id FROM ahrefs_sessions WHERE session_token = ?", sessionCookie.Value).Scan(&websiteID)
				}
			}
			if websiteID <= 0 {
				websiteID = cfg.WebsiteID
			}
			if websiteID <= 0 {
				websiteID = 1
			}

			sessionToken := ""
			sessionCookie, errC := req.Cookie("sem_session")
			if errC == nil && sessionCookie != nil {
				sessionToken = sessionCookie.Value
			}

			if sessionToken != "" {
				if cached, ok := cachedSessionAccount(sessionToken); ok {
					accID = cached.accID
					dbCookie = cached.cookie
					dbUserAgent = cached.userAgent
					dbProxy = cached.proxy
					assignedAccountID = sql.NullInt64{Int64: int64(cached.accID), Valid: cached.accID > 0}
				}
			}

			if dbCookie == "" && sessionToken != "" {
				_ = db.QueryRow("SELECT assigned_account_id FROM ahrefs_sessions WHERE session_token = ? AND website_id = ?", sessionToken, websiteID).Scan(&assignedAccountID)
			}

			if dbCookie == "" && assignedAccountID.Valid {
				errAcc := db.QueryRow("SELECT id, cookie, user_agent, COALESCE(proxy,'') FROM ahrefs_accounts WHERE id = ? AND website_id = ? AND status = 'active'", assignedAccountID.Int64, websiteID).Scan(&accID, &dbCookie, &dbUserAgent, &dbProxy)
				if errAcc != nil {
					assignedAccountID.Valid = false
				} else if sessionToken != "" {
					putSessionAccountCache(sessionToken, accID, dbCookie, dbUserAgent, dbProxy)
				}
			}

			if dbCookie == "" && !assignedAccountID.Valid {
				acc, errAcc := selectActiveAccount(websiteID)
				if errAcc == nil {
					accID = acc.ID
					dbCookie = acc.Cookie
					dbUserAgent = acc.UserAgent
					dbProxy = acc.Proxy
					if sessionToken != "" {
						_, _ = db.Exec("UPDATE ahrefs_sessions SET assigned_account_id = ? WHERE session_token = ? AND website_id = ?", accID, sessionToken, websiteID)
						putSessionAccountCache(sessionToken, accID, dbCookie, dbUserAgent, dbProxy)
					}
				}
			}
		}

		if dbCookie == "" {
			if c, cErr := req.Cookie("sem_session"); cErr == nil && c.Value != "" {
				if acc, ok := semrushPanelAccount(cfg.PublicHost, c.Value); ok {
					dbCookie = acc.Cookie
					if strings.TrimSpace(dbUserAgent) == "" {
						dbUserAgent = acc.UserAgent
					}
					if dbProxy == "" {
						dbProxy = acc.Proxy
					}
					putSessionAccountCache(c.Value, acc.ID, dbCookie, dbUserAgent, dbProxy)
				}
			}
		}
		if dbCookie == "" && semrushStaticGet(req) {
			dbCookie = panelSemrushCookie(cfg.PublicHost)
		}
		if dbCookie == "" && db != nil {
			dbCookie = loadCookies(cfg)
		}

		if dbProxy == "" && websiteID > 0 {
			dbProxy = getWebsiteProxy(websiteID)
		}
		if dbProxy == "" && cfg.WebsiteID > 0 {
			dbProxy = getWebsiteProxy(cfg.WebsiteID)
		}
		*req = *req.WithContext(withUpstreamProxy(req.Context(), dbProxy))

		dbUserAgent = strings.TrimSpace(dbUserAgent)
		clientUA := req.Header.Get("User-Agent")
		// Prefer stored account UA / config UA — never rotate FP with every browser UA.
		if dbUserAgent == "" {
			dbUserAgent = cfg.UserAgent
		}
		if dbUserAgent == "" {
			dbUserAgent = clientUA
		}

		req.Header.Set("User-Agent", dbUserAgent)
		if dbUserAgent != clientUA {
			req.Header.Del("Sec-Ch-Ua")
			req.Header.Del("Sec-Ch-Ua-Mobile")
			req.Header.Del("Sec-Ch-Ua-Platform")
		}

		req.Header.Set("Accept-Encoding", "gzip")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Del("X-Forwarded-For")
		req.Header.Del("X-Real-IP")
		req.Header.Del("X-Forwarded-Host")
		req.Header.Del("X-Forwarded-Proto")
		req.Header.Del("X-Device-Fp")
		req.Header.Del("X-Device-Proof")

		clientCookies := req.Header.Get("Cookie")
		rememberSemrushSession(req)
		parsedDbCookie := parseCookieFromDB(dbCookie)
		mergedCookies := mergeCookiesForUpstream(cfg, clientCookies, parsedDbCookie)
		if mergedCookies != "" {
			// Never block API/page requests on SSO activate — warm session in background.
			if shouldWarmSemrushSession(req.URL.Path) {
				ensureSemrushSessionActivatedAsync(cfg, mergedCookies, dbUserAgent)
			}
			applySemrushSessionHeaders(req, cfg, mergedCookies, dbUserAgent)
		}
		if rl := getRequestLogger(req.Context()); rl != nil && debugEnabled(cfg) {
			rl.upstream = targetHost
			rl.cookieInfo = summarizeSemrushCookies(clientCookies, mergedCookies)
		}
		if mergedCookies != "" {
			req.Header.Set("Cookie", mergedCookies)
		}

		if strings.Contains(strings.ToLower(req.URL.Path), "/seo/api/") {
			if jwt := cookieValueFromString(mergedCookies, "SSO-JWT"); jwt != "" {
				if v := strings.TrimSpace(req.Header.Get("auth-data-jwt")); v == "" || v == "null" {
					req.Header.Set("auth-data-jwt", jwt)
				}
			}
		}

		cacheRequestBodyForRetry(req)

		if ref := req.Header.Get("Referer"); ref != "" {
			proxyOriginHTTP := "http://" + publicHost
			proxyOriginHTTPS := "https://" + publicHost
			replacedRef := strings.ReplaceAll(ref, proxyOriginHTTP, "https://"+targetHost)
			replacedRef = strings.ReplaceAll(replacedRef, proxyOriginHTTPS, "https://"+targetHost)
			replacedRef = strings.ReplaceAll(replacedRef, cfg.PublicHost, targetHost)
			replacedRef = strings.ReplaceAll(replacedRef, "http://", "https://")
			req.Header.Set("Referer", replacedRef)
		}
		if origin := req.Header.Get("Origin"); origin != "" {
			proxyOriginHTTP := "http://" + publicHost
			proxyOriginHTTPS := "https://" + publicHost
			replacedOrigin := strings.ReplaceAll(origin, proxyOriginHTTP, "https://"+targetHost)
			replacedOrigin = strings.ReplaceAll(replacedOrigin, proxyOriginHTTPS, "https://"+targetHost)
			replacedOrigin = strings.ReplaceAll(replacedOrigin, cfg.PublicHost, targetHost)
			replacedOrigin = strings.ReplaceAll(replacedOrigin, "http://", "https://")
			req.Header.Set("Origin", replacedOrigin)
		}

		rawQuery := req.URL.RawQuery
		if rawQuery != "" {
			encodedProxy := url.QueryEscape(cfg.PublicScheme + "://" + cfg.PublicHost)
			encodedTarget := url.QueryEscape("https://" + targetHost)
			rawQuery = strings.ReplaceAll(rawQuery, encodedProxy, encodedTarget)
			encodedProxyNoScheme := url.QueryEscape(cfg.PublicHost)
			encodedTargetNoScheme := url.QueryEscape(targetHost)
			rawQuery = strings.ReplaceAll(rawQuery, encodedProxyNoScheme, encodedTargetNoScheme)
			rawQuery = strings.ReplaceAll(rawQuery, cfg.PublicHost, targetHost)
			req.URL.RawQuery = rawQuery
		}

		if req.Method == "POST" {
			req.Header.Set("X-Kl-Ajax-Request", "Ajax_Request")
		}
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		cfg = loadConfig()
		credit := resp.StatusCode >= 200 && resp.StatusCode < 300
		defer func() {
			if credit && resp.Request != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
				semrushRecordCredit(resp.Request)
			}
		}()

		// Retrieve dynamic proxy Host and Scheme from request headers
		proxyHost := ""
		proxyScheme := ""
		if resp.Request != nil {
			proxyHost = resp.Request.Header.Get("X-Proxy-Host")
			proxyScheme = resp.Request.Header.Get("X-Proxy-Scheme")
		}
		if proxyHost == "" {
			proxyHost = cfg.PublicHost
		}
		if proxyScheme == "" {
			proxyScheme = cfg.PublicScheme
		}

		if loc := resp.Header.Get("Location"); loc != "" {
			lowerLoc := strings.ToLower(loc)
			if strings.Contains(lowerLoc, "multilogin") {
				fromPath := ""
				if resp.Request != nil {
					fromPath = resp.Request.URL.Path
				}
				log.Printf("[MULTILOGIN] ⚠️ upstream redirect | from=%s → %s", fromPath, loc)
				if rl := getRequestLogger(resp.Request.Context()); rl != nil {
					rl.bodyHint = "MULTILOGIN_REDIRECT:" + truncateLog(loc, 80)
					rl.category = "MULTILOGIN"
				}
				cookieHdr := ""
				if resp.Request != nil {
					cookieHdr = resp.Request.Header.Get("Cookie")
				}
				if cookieHdr == "" {
					cookieHdr = loadCookies(cfg)
				}
				ensureSemrushSessionActivated(cfg, cookieHdr, cfg.UserAgent, true)
				if isSemrushAPIRequest(resp.Request) {
					if newResp, err := retrySemrushUpstream(resp.Request, cfg, cookieHdr); err == nil && newResp != nil {
						if resp.Body != nil {
							resp.Body.Close()
						}
						resp.StatusCode = newResp.StatusCode
						resp.Header = newResp.Header.Clone()
						resp.Body = newResp.Body
						resp.ContentLength = newResp.ContentLength
						resp.Header.Del("Location")
						loc = ""
						log.Printf("[MULTILOGIN] ✅ API retry %s %s → %d", resp.Request.Method, resp.Request.URL.Path, newResp.StatusCode)
						if rl := getRequestLogger(resp.Request.Context()); rl != nil {
							rl.bodyHint = fmt.Sprintf("MULTILOGIN_API_RETRY:%d", newResp.StatusCode)
						}
					} else if err != nil {
						log.Printf("[MULTILOGIN] ⚠️ API retry failed %s %s: %v", resp.Request.Method, resp.Request.URL.Path, err)
					}
				} else if redirectTo := parseMultiloginRedirect(loc); redirectTo != "" {
					fromPath := ""
					if resp.Request != nil {
						fromPath = resp.Request.URL.Path
					}
					if redirectTo == "/" || redirectTo == fromPath {
						redirectTo = "/home/"
					}
					replacedLoc := proxyScheme + "://" + proxyHost + redirectTo
					resp.Header.Set("Location", replacedLoc)
					loc = replacedLoc
					log.Printf("[MULTILOGIN] ✅ auto-activated → sending browser to %s", redirectTo)
					if rl := getRequestLogger(resp.Request.Context()); rl != nil {
						rl.bodyHint = "MULTILOGIN_AUTO:" + redirectTo
					}
				}
			}
			if semrushAccountDeadTarget(loc) && resp.Request != nil {
				page, _ := semrushAccountSwapPage(resp.Request)
				credit = false
				resp.StatusCode = http.StatusOK
				resp.Header.Set("Content-Type", "text/html; charset=utf-8")
				resp.Header.Set("Cache-Control", "no-store")
				resp.Body = io.NopCloser(strings.NewReader(page))
				resp.ContentLength = int64(len(page))
				resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(page)))
				resp.Header.Del("Content-Encoding")
				resp.Header.Del("Location")
				return nil
			}

			// If it redirects to login or SSO pages, swap the account!
			if (strings.Contains(lowerLoc, "/login") || strings.Contains(lowerLoc, "/auth") || strings.Contains(lowerLoc, "/signup")) &&
				!strings.Contains(lowerLoc, "/api/auth-handshake") &&
				!strings.Contains(lowerLoc, "/access") &&
				resp.Request != nil {

				log.Printf("[SWAP] 🔄 Upstream redirected to login page: %s. Swapping account!", loc)
				// Panel mode (local): rotate mapped account from panel.db
				if db == nil {
					page, switched := semrushAccountSwapPage(resp.Request)
					credit = false
					log.Printf("[SWAP] panel login-redirect switch switched=%v", switched)
					resp.StatusCode = http.StatusOK
					resp.Header.Set("Content-Type", "text/html; charset=utf-8")
					resp.Header.Set("Cache-Control", "no-store")
					resp.Body = io.NopCloser(strings.NewReader(page))
					resp.ContentLength = int64(len(page))
					resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(page)))
					resp.Header.Del("Content-Encoding")
					resp.Header.Del("Location")
					return nil
				}
				sessionCookie, errC := resp.Request.Cookie("sem_session")
				if errC == nil && sessionCookie != nil && sessionCookie.Value != "" {
					// Check swap count to prevent loops
					swapCount := 0
					if swapCookie, errS := resp.Request.Cookie("sem_swap_count"); errS == nil {
						fmt.Sscanf(swapCookie.Value, "%d", &swapCount)
					}

					if swapCount >= 3 {
						log.Printf("[SWAP] ⚠️ Loop protection active on redirect! Already swapped %d times. Let login page display.", swapCount)
					} else {
						var currentAssignedID sql.NullInt64
						var username string
						var sessionWebsiteID int
						errSession := db.QueryRow("SELECT username, website_id, assigned_account_id FROM ahrefs_sessions WHERE session_token = ?", sessionCookie.Value).Scan(&username, &sessionWebsiteID, &currentAssignedID)

						if errSession == nil {
							var nextAcc Account
							var errSelect error
							if currentAssignedID.Valid {
								errSelect = db.QueryRow("SELECT id, name, cookie, user_agent FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' AND id != ? ORDER BY last_used_at ASC LIMIT 1", sessionWebsiteID, currentAssignedID.Int64).Scan(&nextAcc.ID, &nextAcc.Name, &nextAcc.Cookie, &nextAcc.UserAgent)
							}
							if !currentAssignedID.Valid || errSelect != nil {
								errSelect = db.QueryRow("SELECT id, name, cookie, user_agent FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' ORDER BY last_used_at ASC LIMIT 1", sessionWebsiteID).Scan(&nextAcc.ID, &nextAcc.Name, &nextAcc.Cookie, &nextAcc.UserAgent)
							}

							if errSelect == nil {
								log.Printf("[SWAP] 🔄 Login redirect detected! Swapping from account ID %v to account ID %d (%s) for session %s (Swap count: %d)",
									currentAssignedID.Int64, nextAcc.ID, nextAcc.Name, sessionCookie.Value, swapCount+1)

								_, _ = db.Exec("UPDATE ahrefs_sessions SET assigned_account_id = ? WHERE session_token = ? AND website_id = ?", nextAcc.ID, sessionCookie.Value, sessionWebsiteID)
								invalidateSessionAccountCache(sessionCookie.Value)

								var fromAccName string
								if currentAssignedID.Valid {
									_ = db.QueryRow("SELECT name FROM ahrefs_accounts WHERE id = ?", currentAssignedID.Int64).Scan(&fromAccName)
									_, _ = db.Exec("UPDATE ahrefs_accounts SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?", currentAssignedID.Int64)
								}
								_, _ = db.Exec("UPDATE ahrefs_accounts SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?", nextAcc.ID)

								// Log the switch event to central ahrefs_switch_logs
								_, _ = db.Exec("INSERT INTO ahrefs_switch_logs (website_id, session_token, username, from_account_id, from_account_name, to_account_id, to_account_name, reason) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
									sessionWebsiteID, sessionCookie.Value, username, currentAssignedID, fromAccName, nextAcc.ID, nextAcc.Name, "Semrush auth redirect detected")

								// Increment and set swap count cookie
								newSwapCountCookie := &http.Cookie{
									Name:     "sem_swap_count",
									Value:    fmt.Sprintf("%d", swapCount+1),
									Path:     "/",
									MaxAge:   10,
									HttpOnly: true,
								}
								resp.Header.Add("Set-Cookie", newSwapCountCookie.String())

								// Return gorgeous loading screen that reloads/redirects to "/"
								htmlContent := renderSwappingPage("/")
								resp.StatusCode = http.StatusOK
								resp.Header.Set("Content-Type", "text/html; charset=utf-8")
								resp.Body = io.NopCloser(strings.NewReader(htmlContent))
								resp.ContentLength = int64(len(htmlContent))
								resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(htmlContent)))
								resp.Header.Del("Content-Encoding")
								resp.Header.Del("Location")
								return nil
							} else {
								log.Printf("[SWAP] ⚠️ Login redirect detected but no alternative active account found in DB for website_id = %d: %v", sessionWebsiteID, errSelect)
							}
						}
					}
				}
			}

			replacedLoc := rewriteSemrushPublicURL(loc, proxyScheme, proxyHost)
			if loc != "" {
				if replacedLoc != loc {
					log.Printf("[REDIRECT] %s → %s", loc, replacedLoc)
				}
				resp.Header.Set("Location", replacedLoc)
			}
		}

		if rl := getRequestLogger(resp.Request.Context()); rl != nil {
			rl.status = resp.StatusCode
		}

		if cfg.LocalTestMode {
			resp.Header.Del("Set-Cookie")
		} else {
			for _, cookie := range resp.Cookies() {
				cookie.Domain = ""
				cookie.Secure = false
			}
		}

		contentType := resp.Header.Get("Content-Type")
		contentTypeLower := strings.ToLower(contentType)
		// grpc-web-text contains the word "text". Rewriting that body corrupts the report.
		// event-stream must stay a live stream; reading it here holds the page on the spinner.
		if strings.Contains(contentTypeLower, "grpc") || strings.Contains(contentTypeLower, "event-stream") {
			return nil
		}
		isText := strings.Contains(contentTypeLower, "text") ||
			strings.Contains(contentTypeLower, "javascript") ||
			strings.Contains(contentTypeLower, "json") ||
			strings.Contains(contentTypeLower, "xml")

		if isText {
			encoding := resp.Header.Get("Content-Encoding")
			isGzip := strings.EqualFold(encoding, "gzip")

			var reader io.Reader
			var err error
			if isGzip {
				gzipReader, err := gzip.NewReader(resp.Body)
				if err != nil {
					return err
				}
				defer gzipReader.Close()
				reader = gzipReader
			} else {
				reader = resp.Body
			}

			bodyBytes, err := io.ReadAll(reader)
			if err != nil {
				return err
			}
			resp.Body.Close()

			isJSON := strings.Contains(contentType, "json")
			isHTML := strings.Contains(contentType, "text/html")

			// Cloudflare/reCAPTCHA: do not rewrite/inject — rewriting broke the challenge page.
			if isHTML && isCloudflareChallengeBody(bodyBytes) {
				log.Printf("[CF] Upstream challenge on %s — pass-through (no rewrite)", resp.Request.URL.Path)
				resp.Header.Del("Content-Encoding")
				resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				resp.ContentLength = int64(len(bodyBytes))
				resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(bodyBytes)))
				return nil
			}

			// Huge Semrush API JSON payloads rarely need domain rewrite. Fast-path pass-through
			// when body has no semrush.com / integrity markers — biggest win for Keyword Gap etc.
			if isJSON && !bodyNeedsSemrushRewrite(bodyBytes) && !isHTML {
				if debugEnabled(cfg) && resp.Request != nil {
					peekLen := len(bodyBytes)
					if peekLen > 240 {
						peekLen = 240
					}
					if hint := bodyDiagnostic(resp.StatusCode, bodyBytes[:peekLen]); hint != "" {
						if rl := getRequestLogger(resp.Request.Context()); rl != nil {
							rl.bodyHint = hint
						}
					}
				}
				resp.Header.Del("Content-Encoding")
				resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				resp.ContentLength = int64(len(bodyBytes))
				resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(bodyBytes)))
				return nil
			}

			bodyStr := string(bodyBytes)

			if debugEnabled(cfg) && resp.Request != nil {
				peekLen := len(bodyBytes)
				if peekLen > 240 {
					peekLen = 240
				}
				if hint := bodyDiagnostic(resp.StatusCode, bodyBytes[:peekLen]); hint != "" {
					if rl := getRequestLogger(resp.Request.Context()); rl != nil {
						rl.bodyHint = hint
					}
				}
			}

			if isHTML && resp.Request != nil && strings.Contains(strings.ToLower(resp.Request.URL.Path), "multilogin") && strings.Contains(bodyStr, "Something went wrong") {
				dest := resp.Request.URL.Query().Get("redirect_to")
				if dest == "" || dest == "/" || !strings.HasPrefix(dest, "/") || strings.HasPrefix(dest, "//") || strings.Contains(strings.ToLower(dest), "multilogin") {
					dest = "/home/"
				}
				log.Printf("[MULTILOGIN] error page on %s → %s", resp.Request.URL.Path, dest)
				resp.StatusCode = http.StatusFound
				resp.Header.Set("Location", dest)
				resp.Header.Set("Cache-Control", "no-store")
				resp.Body = io.NopCloser(strings.NewReader(""))
				resp.ContentLength = 0
				resp.Header.Set("Content-Length", "0")
				resp.Header.Del("Content-Encoding")
				return nil
			}

			if isHTML && resp.Request != nil && semrushLoggedOutHTML(bodyStr) {
				page, switched := semrushAccountSwapPage(resp.Request)
				credit = false
				log.Printf("[SWAP] header or register wall on %s switched=%v", resp.Request.URL.Path, switched)
				resp.StatusCode = http.StatusOK
				resp.Header.Set("Content-Type", "text/html; charset=utf-8")
				resp.Header.Set("Cache-Control", "no-store")
				resp.Body = io.NopCloser(strings.NewReader(page))
				resp.ContentLength = int64(len(page))
				resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(page)))
				resp.Header.Del("Content-Encoding")
				resp.Header.Del("Location")
				return nil
			}

			// Detect limits, register prompts, and upgrade buttons in HTML response
			if isHTML {
				if detectLimitOrLogout(bodyStr) {
					if resp.Request != nil && db == nil {
						page, switched := semrushAccountSwapPage(resp.Request)
						credit = false
						log.Printf("[SWAP] panel limit/logout body switch switched=%v path=%s", switched, resp.Request.URL.Path)
						resp.StatusCode = http.StatusOK
						resp.Header.Set("Content-Type", "text/html; charset=utf-8")
						resp.Header.Set("Cache-Control", "no-store")
						resp.Body = io.NopCloser(strings.NewReader(page))
						resp.ContentLength = int64(len(page))
						resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(page)))
						resp.Header.Del("Content-Encoding")
						resp.Header.Del("Location")
						return nil
					}
					if db != nil && resp.Request != nil {
						sessionCookie, errC := resp.Request.Cookie("sem_session")
						if errC == nil && sessionCookie != nil && sessionCookie.Value != "" {
							// Check swap count to prevent loops
							swapCount := 0
							if swapCookie, errS := resp.Request.Cookie("sem_swap_count"); errS == nil {
								fmt.Sscanf(swapCookie.Value, "%d", &swapCount)
							}

							if swapCount >= 3 {
								log.Printf("[SWAP] ⚠️ Loop protection active! Already swapped %d times. Stopping rotation to avoid ERR_TOO_MANY_REDIRECTS.", swapCount)
								// Let the page render so the user sees the actual limit/login page
							} else {
								var currentAssignedID sql.NullInt64
								var username string
								var sessionWebsiteID int
								errSession := db.QueryRow("SELECT username, website_id, assigned_account_id FROM ahrefs_sessions WHERE session_token = ?", sessionCookie.Value).Scan(&username, &sessionWebsiteID, &currentAssignedID)

								if errSession == nil {
									var nextAcc Account
									var errSelect error
									if currentAssignedID.Valid {
										errSelect = db.QueryRow("SELECT id, name, cookie, user_agent FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' AND id != ? ORDER BY last_used_at ASC LIMIT 1", sessionWebsiteID, currentAssignedID.Int64).Scan(&nextAcc.ID, &nextAcc.Name, &nextAcc.Cookie, &nextAcc.UserAgent)
									}
									if !currentAssignedID.Valid || errSelect != nil {
										errSelect = db.QueryRow("SELECT id, name, cookie, user_agent FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' ORDER BY last_used_at ASC LIMIT 1", sessionWebsiteID).Scan(&nextAcc.ID, &nextAcc.Name, &nextAcc.Cookie, &nextAcc.UserAgent)
									}

									if errSelect == nil {
										log.Printf("[SWAP] 🔄 Limit/Logout detected in response body! Swapping from account ID %v to account ID %d (%s) for session %s (Swap count: %d)",
											currentAssignedID.Int64, nextAcc.ID, nextAcc.Name, sessionCookie.Value, swapCount+1)

										_, _ = db.Exec("UPDATE ahrefs_sessions SET assigned_account_id = ? WHERE session_token = ? AND website_id = ?", nextAcc.ID, sessionCookie.Value, sessionWebsiteID)
										invalidateSessionAccountCache(sessionCookie.Value)

										var fromAccName string
										if currentAssignedID.Valid {
											_ = db.QueryRow("SELECT name FROM ahrefs_accounts WHERE id = ?", currentAssignedID.Int64).Scan(&fromAccName)
											_, _ = db.Exec("UPDATE ahrefs_accounts SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?", currentAssignedID.Int64)
										}
										_, _ = db.Exec("UPDATE ahrefs_accounts SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?", nextAcc.ID)

										// Log the switch event to central ahrefs_switch_logs
										_, _ = db.Exec("INSERT INTO ahrefs_switch_logs (website_id, session_token, username, from_account_id, from_account_name, to_account_id, to_account_name, reason) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
											sessionWebsiteID, sessionCookie.Value, username, currentAssignedID, fromAccName, nextAcc.ID, nextAcc.Name, "Semrush free/limit detection")

										// Return transparent loading screen back to the exact current page path + query
										redirectURL := resp.Request.URL.Path
										if resp.Request.URL.RawQuery != "" {
											redirectURL += "?" + resp.Request.URL.RawQuery
										}

										// Increment and set swap count cookie
										newSwapCountCookie := &http.Cookie{
											Name:     "sem_swap_count",
											Value:    fmt.Sprintf("%d", swapCount+1),
											Path:     "/",
											MaxAge:   10,
											HttpOnly: true,
										}
										resp.Header.Add("Set-Cookie", newSwapCountCookie.String())

										// Return gorgeous loading screen that reloads/redirects to redirectURL
										htmlContent := renderSwappingPage(redirectURL)
										resp.StatusCode = http.StatusOK
										resp.Header.Set("Content-Type", "text/html; charset=utf-8")
										resp.Body = io.NopCloser(strings.NewReader(htmlContent))
										resp.ContentLength = int64(len(htmlContent))
										resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(htmlContent)))
										resp.Header.Del("Content-Encoding")
										resp.Header.Del("Location")
										return nil
									} else {
										log.Printf("[SWAP] ⚠️ Limit/Logout detected but no alternative active account found in DB for website_id = %d: %v", sessionWebsiteID, errSelect)
									}
								}
							}
						}
					}
				} else if db != nil {
					// Clear the swap count cookie on successful page load
					if resp.Request != nil {
						if _, errS := resp.Request.Cookie("sem_swap_count"); errS == nil {
							clearCookie := &http.Cookie{
								Name:     "sem_swap_count",
								Value:    "",
								Path:     "/",
								MaxAge:   -1,
								HttpOnly: true,
							}
							resp.Header.Add("Set-Cookie", clearCookie.String())
						}
					}
				}
			}

			proxySchemeHost := proxyScheme + "://" + proxyHost

			bodyStr = strings.ReplaceAll(bodyStr, "https://www.semrush.com", proxySchemeHost)
			bodyStr = strings.ReplaceAll(bodyStr, "https://semrush.com", proxySchemeHost)
			bodyStr = strings.ReplaceAll(bodyStr, "https://static.semrush.com", proxySchemeHost+"/static-proxy")
			bodyStr = strings.ReplaceAll(bodyStr, "https://secure.semrush.com", proxySchemeHost+"/secure-proxy")
			bodyStr = strings.ReplaceAll(bodyStr, "https://cdn.semrush.com", proxySchemeHost+"/cdn-proxy")
			bodyStr = strings.ReplaceAll(bodyStr, "https://ai-visibility-index.semrush.com", proxySchemeHost+"/ai-proxy")
			bodyStr = strings.ReplaceAll(bodyStr, "//static.semrush.com", "//"+proxyHost+"/static-proxy")
			bodyStr = strings.ReplaceAll(bodyStr, "//secure.semrush.com", "//"+proxyHost+"/secure-proxy")
			bodyStr = strings.ReplaceAll(bodyStr, "//cdn.semrush.com", "//"+proxyHost+"/cdn-proxy")
			bodyStr = strings.ReplaceAll(bodyStr, "//ai-visibility-index.semrush.com", "//"+proxyHost+"/ai-proxy")
			bodyStr = strings.ReplaceAll(bodyStr, `\/static.semrush.com\/`, `\/`+proxyHost+`\/static-proxy\/`)
			bodyStr = strings.ReplaceAll(bodyStr, `\/secure.semrush.com\/`, `\/`+proxyHost+`\/secure-proxy\/`)
			bodyStr = strings.ReplaceAll(bodyStr, `\/cdn.semrush.com\/`, `\/`+proxyHost+`\/cdn-proxy\/`)
			bodyStr = strings.ReplaceAll(bodyStr, `\/ai-visibility-index.semrush.com\/`, `\/`+proxyHost+`\/ai-proxy\/`)
			bodyStr = strings.ReplaceAll(bodyStr, `\/www.semrush.com\/`, `\/`+proxyHost+`\/`)
			bodyStr = strings.ReplaceAll(bodyStr, `/www.semrush.com`, `/`+proxyHost)
			bodyStr = rewriteSemrushHostsInBody(bodyStr, proxyScheme, proxyHost)
			bodyStr = integrityRegex.ReplaceAllString(bodyStr, "")

			if strings.Contains(contentType, "javascript") {
				bodyStr = semrushExternalNav.ReplaceAllString(bodyStr, `window.location.href=(window.proxyUrl?window.proxyUrl($1.absoluteURL||$2.to):($1.absoluteURL||$2.to))`)
				bodyStr = strings.ReplaceAll(bodyStr, ".integrity=", "._ntegrity=")
				bodyStr = webpackAttrRegex.ReplaceAllString(bodyStr, `"_ntegrity":`)
				bodyStr = webpackSetAttrRegex.ReplaceAllString(bodyStr, `setAttribute("_ntegrity"`)
			}

			if strings.Contains(contentType, "text/html") {
				lowerBody := strings.ToLower(bodyStr)
				isMultiloginPage := strings.Contains(lowerBody, "too many active sessions") ||
					(resp.Request != nil && strings.Contains(strings.ToLower(resp.Request.URL.Path), "multilogin"))

				if isMultiloginPage {
					bodyStr = strings.Replace(bodyStr, "</head>", multiloginAutoScript+"</head>", 1)
					if rl := getRequestLogger(resp.Request.Context()); rl != nil {
						rl.bodyHint = "MULTILOGIN_PAGE"
						rl.category = "MULTILOGIN"
					}
				} else {
					customExportJS := loadExportToolJS()
					localHostFix := buildLocalHostFix(cfg)
					sessionCookies := ""
					upstreamUA := cfg.UserAgent
					if resp.Request != nil {
						sessionCookies = resp.Request.Header.Get("Cookie")
						if ua := resp.Request.Header.Get("User-Agent"); ua != "" {
							upstreamUA = ua
						}
					}
					if sessionCookies == "" {
						sessionCookies = loadCookies(cfg)
					}
					sessionFP := semrushDeviceFingerprint(upstreamUA, sessionCookies)
					sessionBootstrap := buildSemrushSessionBootstrapJS(sessionFP)
					grpcWebFix := buildGrpcWebFix(cookieValueFromString(sessionCookies, "SSO-JWT"))
					blockScript := `<script data-semrush-proxy-boot>
					` + localHostFix + grpcWebFix + sessionBootstrap + `

					window.intercomSettings = { app_id: "" };
					window.Intercom = function() { return false; };
					window.Hotjar = function() { return false; };
					window.hj = function() { return false; };

					function blockedPathFromUrl(u) {
						if (!u || typeof u !== "string") return false;
						try {
							var path = u;
							if (u.indexOf("://") !== -1) {
								path = new URL(u, window.location.origin).pathname;
							} else if (u.charAt(0) !== "/") {
								path = new URL(u, window.location.href).pathname;
							}
							path = path.toLowerCase().split("?")[0].split("#")[0];
							var blocked = [
								"/sso/logout",
								"/corporate/account"
							];
							for (var i = 0; i < blocked.length; i++) {
								var b = blocked[i];
								if (path === b || path.indexOf(b + "/") === 0) return true;
							}
						} catch (e) {}
						return false;
					}

					function shouldBlockUrl(u) {
						return blockedPathFromUrl(u) || u.includes("block-sentry") || u.includes("intercom.io") || u.includes("hotjar") || u.includes("_sentry");
					}

					document.addEventListener("click", function(e) {
						var link = e.target && e.target.closest ? e.target.closest("a[href]") : null;
						if (!link) return;
						var href = link.getAttribute("href");
						if (!href || href.charAt(0) === "#") return;
						var proxied = proxyUrl(href);
						if (typeof proxied === "string" && proxied !== href) link.setAttribute("href", proxied);
						if (shouldBlockUrl(href) || shouldBlockUrl(proxied)) {
							e.preventDefault();
							e.stopPropagation();
							e.stopImmediatePropagation();
						}
					}, true);

					function proxyUrl(url) {
						if (typeof url !== "string") return url;
						let u = url.trim();
						if (u.startsWith("https://ai-visibility-index.semrush.com")) {
							return u.replace("https://ai-visibility-index.semrush.com", REAL_PROXY_ORIGIN + "/ai-proxy");
						}
						if (u.startsWith("http://ai-visibility-index.semrush.com")) {
							return u.replace("http://ai-visibility-index.semrush.com", REAL_PROXY_ORIGIN + "/ai-proxy");
						}
						if (u.startsWith("//ai-visibility-index.semrush.com")) {
							return REAL_PROXY_PROTOCOL + u.replace("//ai-visibility-index.semrush.com", "//" + REAL_PROXY_HOST + "/ai-proxy");
						}
						if (u.startsWith("https://cdn.semrush.com")) {
							return u.replace("https://cdn.semrush.com", REAL_PROXY_ORIGIN + "/cdn-proxy");
						}
						if (u.startsWith("http://cdn.semrush.com")) {
							return u.replace("http://cdn.semrush.com", REAL_PROXY_ORIGIN + "/cdn-proxy");
						}
						if (u.startsWith("//cdn.semrush.com")) {
							return REAL_PROXY_PROTOCOL + u.replace("//cdn.semrush.com", "//" + REAL_PROXY_HOST + "/cdn-proxy");
						}
						if (u.startsWith("https://static.semrush.com")) {
							return u.replace("https://static.semrush.com", REAL_PROXY_ORIGIN + "/static-proxy");
						}
						if (u.startsWith("http://static.semrush.com")) {
							return u.replace("http://static.semrush.com", REAL_PROXY_ORIGIN + "/static-proxy");
						}
						if (u.startsWith("//static.semrush.com")) {
							return REAL_PROXY_PROTOCOL + u.replace("//static.semrush.com", "//" + REAL_PROXY_HOST + "/static-proxy");
						}
						if (u.startsWith("https://secure.semrush.com")) {
							return u.replace("https://secure.semrush.com", REAL_PROXY_ORIGIN + "/secure-proxy");
						}
						if (u.startsWith("http://secure.semrush.com")) {
							return u.replace("http://secure.semrush.com", REAL_PROXY_ORIGIN + "/secure-proxy");
						}
						if (u.startsWith("//secure.semrush.com")) {
							return REAL_PROXY_PROTOCOL + u.replace("//secure.semrush.com", "//" + REAL_PROXY_HOST + "/secure-proxy");
						}
						if (u.startsWith("https://www.semrush.com")) {
							return u.replace("https://www.semrush.com", REAL_PROXY_ORIGIN);
						}
						if (u.startsWith("https://semrush.com")) {
							return u.replace("https://semrush.com", REAL_PROXY_ORIGIN);
						}
						if (u.startsWith("http://www.semrush.com")) {
							return u.replace("http://www.semrush.com", REAL_PROXY_ORIGIN);
						}
						if (u.startsWith("http://semrush.com")) {
							return u.replace("http://semrush.com", REAL_PROXY_ORIGIN);
						}
						if (u.startsWith("//www.semrush.com")) {
							return REAL_PROXY_PROTOCOL + u.replace("//www.semrush.com", "//" + REAL_PROXY_HOST);
						}
						if (u.startsWith("//semrush.com")) {
							return REAL_PROXY_PROTOCOL + u.replace("//semrush.com", "//" + REAL_PROXY_HOST);
						}
						try {
							var parsed = new URL(u, REAL_PROXY_ORIGIN);
							var host = parsed.hostname.toLowerCase();
							if (host === "semrush.com" || host.slice(-12) === ".semrush.com") {
								var prefix = "";
								if (host === "static.semrush.com") prefix = "/static-proxy";
								else if (host === "secure.semrush.com") prefix = "/secure-proxy";
								else if (host === "cdn.semrush.com") prefix = "/cdn-proxy";
								else if (host === "ai-visibility-index.semrush.com") prefix = "/ai-proxy";
								return REAL_PROXY_ORIGIN + prefix + parsed.pathname + parsed.search + parsed.hash;
							}
						} catch (e) {}
						return url;
					}

					window.proxyUrl = proxyUrl;
					// location.href is not overridable. This catches the Content Toolkit
					// jump to https://www.semrush.com before the browser leaves the proxy.
					try {
						if (window.navigation && window.navigation.addEventListener) {
							window.navigation.addEventListener("navigate", function(e) {
								var dest = e.destination && e.destination.url;
								if (!dest || !e.cancelable) return;
								var next = proxyUrl(dest);
								if (next === dest) return;
								e.preventDefault();
								if (next !== window.location.href) window.location.replace(next);
							});
						}
					} catch (e) {}
					try {
						var locProto = Location.prototype;
						var hrefDesc = Object.getOwnPropertyDescriptor(locProto, "href");
						if (hrefDesc && hrefDesc.set) {
							var realHrefSet = hrefDesc.set;
							Object.defineProperty(locProto, "href", {
								get: hrefDesc.get,
								set: function(v) { realHrefSet.call(this, proxyUrl(String(v))); },
								configurable: true
							});
						}
						var realAssign = locProto.assign;
						locProto.assign = function(v) { return realAssign.call(this, proxyUrl(String(v))); };
						var realReplace = locProto.replace;
						locProto.replace = function(v) { return realReplace.call(this, proxyUrl(String(v))); };
					} catch (e) {}
					document.addEventListener("submit", function(e) {
						var form = e.target;
						if (!form || !form.action) return;
						var next = proxyUrl(String(form.action));
						if (next !== form.action) form.action = next;
					}, true);
					var realOpen = window.open;
					window.open = function(u, name, specs) {
						if (typeof u === "string") u = proxyUrl(u);
						return realOpen.call(window, u, name, specs);
					};
					
					var __originalFetch = window.__semrushFetch || window.fetch;
					window.__semrushFetch = __originalFetch;
					window.fetch = function(input, init) {
						if (input) {
							if (typeof input === "string") {
								const proxied = proxyUrl(input);
								if (proxied !== input) {
									if (shouldBlockUrl(proxied)) {
										return Promise.resolve(new Response("{}", { status: 200, headers: { "Content-Type": "application/json" } }));
									}
									input = proxied;
								}
							} else if (input instanceof Request) {
								const originalUrl = input.url;
								const proxied = proxyUrl(originalUrl);
								if (proxied !== originalUrl) {
									if (shouldBlockUrl(proxied)) {
										return Promise.resolve(new Response("{}", { status: 200, headers: { "Content-Type": "application/json" } }));
									}
									const newRequest = new Request(proxied, {
										method: input.method,
										headers: input.headers,
										body: input.body,
										mode: input.mode,
										credentials: input.credentials,
										cache: input.cache,
										redirect: input.redirect,
										referrer: input.referrer,
										integrity: input.integrity,
										keepalive: input.keepalive,
										signal: input.signal
									});
									input = newRequest;
								}
							} else if (input instanceof URL) {
								const urlStr = input.toString();
								const proxied = proxyUrl(urlStr);
								if (proxied !== urlStr) {
									input = new URL(proxied);
								}
							}
						}
						return __originalFetch.call(this, input, init);
					};
					
					var __originalSendBeacon = window.__semrushBeacon || window.navigator.sendBeacon;
					window.__semrushBeacon = __originalSendBeacon;
					window.navigator.sendBeacon = function(url, data) {
						if (typeof url === "string") {
							const proxied = proxyUrl(url);
							if (shouldBlockUrl(proxied)) {
								return true;
							}
							url = proxied;
						}
						return __originalSendBeacon.call(window.navigator, url, data);
					};
					
					var __originalXHR = window.__semrushXHR || window.XMLHttpRequest.prototype.open;
					window.__semrushXHR = __originalXHR;
					window.XMLHttpRequest.prototype.open = function(method, url, async, user, password) {
						if (url) {
							let urlStr = (url instanceof URL) ? url.toString() : String(url);
							// gRPC widget APIs — force through proxy even if app-bootstrap cached old origin
							if (urlStr.indexOf("/seo/api/") !== -1 || urlStr.indexOf("semrush.com/seo/api") !== -1) {
								urlStr = proxyUrl(urlStr);
							} else {
								const proxied = proxyUrl(urlStr);
								if (proxied !== urlStr) urlStr = proxied;
							}
							if (shouldBlockUrl(urlStr)) {
								url = "data:application/json,{}";
							} else {
								url = urlStr;
							}
						}
						// Preserve argument arity — passing undefined as async forces sync XHR and breaks gRPC-web responseType.
						var argc = arguments.length;
						if (argc === 2) return __originalXHR.call(this, method, url);
						if (argc === 3) return __originalXHR.call(this, method, url, async);
						if (argc === 4) return __originalXHR.call(this, method, url, async, user);
						return __originalXHR.call(this, method, url, async, user, password);
					};

					function hideCookieConsentBanners() {
						const selectors = [
							"#raspr-hm-root-button", "button.help-menu__button",
							"[id*='cookie' i]", "[class*='cookie' i]", "[id*='consent' i]", "[class*='consent' i]",
							".ch2", ".ch2-container", "#ch2-dialog", "[data-region='c1']",
							"div[style*='z-index: 2147483647']", "div[style*='z-index:99999999']",
							"#onetrust-banner-sdk", ".ot-sdk-container"
						];
						selectors.forEach(sel => {
							try {
								document.querySelectorAll(sel).forEach(el => {
									if (el.id === "semrush-export-container" || el.closest("#semrush-export-container")) return;
									if (el.tagName === "BODY" || el.tagName === "HTML" || el.id === "root" || el.id === "app") return;
									el.style.setProperty("display", "none", "important");
									el.style.setProperty("visibility", "hidden", "important");
									el.style.setProperty("opacity", "0", "important");
									el.style.setProperty("pointer-events", "none", "important");
								});
							} catch(e){}
						});
						
						if (!document.body) return;
						const walk = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT, null, false);
						let node;
						while (node = walk.nextNode()) {
							if (node.nodeValue.includes("We use cookies to run our website") || node.nodeValue.includes("Cookie Policy")) {
								let parent = node.parentElement;
								for (let i = 0; i < 5; i++) {
									if (parent && parent.tagName !== "BODY" && parent.tagName !== "HTML") {
										parent.style.setProperty("display", "none", "important");
										parent.style.setProperty("visibility", "hidden", "important");
										parent.style.setProperty("opacity", "0", "important");
										parent.style.setProperty("pointer-events", "none", "important");
										parent = parent.parentElement;
									}
								}
							}
						}
					}
					
					var observer = new MutationObserver((mutations) => {
						hideCookieConsentBanners();
					});
					observer.observe(document.documentElement, { childList: true, subtree: true });
					
					window.addEventListener("load", hideCookieConsentBanners);
					setInterval(hideCookieConsentBanners, 500);
				</script>
				<style>
					.ch2, .ch2-container, #ch2-dialog, [data-region="c1"], #ch2-dialog-title, #ch2-dialog-description { 
						display: none !important; 
						visibility: hidden !important; 
						opacity: 0 !important; 
						pointer-events: none !important; 
					}
					div.snav-header__end { display: none !important; }
					#raspr-hm-root-button, button.help-menu__button { display: none !important; visibility: hidden !important; pointer-events: none !important; }
				</style>` + semrushLimitDock
					exportScript := ""
					if customExportJS != "" {
						exportScript = "<script>\n" + customExportJS + "\n</script>"
					}

					if !strings.Contains(bodyStr, "data-semrush-proxy-boot") {
						bodyStr = strings.Replace(bodyStr, "<head>", "<head>"+"\n"+blockScript+"\n"+exportScript, 1)
					}
					bodyStr = injectSemrushDeviceScript(bodyStr)
				}
			}

			modifiedBytes := []byte(bodyStr)

			// Always return uncompressed after rewrite — avoids expensive re-gzip on large payloads.
			resp.Header.Del("Content-Encoding")
			resp.Body = io.NopCloser(bytes.NewReader(modifiedBytes))
			resp.ContentLength = int64(len(modifiedBytes))
			resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(modifiedBytes)))
			if resp.StatusCode == http.StatusOK && resp.Request != nil &&
				resp.Header.Get("Set-Cookie") == "" &&
				cdnCacheKeyFromUpstream(resp.Request) != "" &&
				!strings.Contains(contentTypeLower, "text/html") &&
				!strings.Contains(contentTypeLower, "json") {
				resp.Header.Set("Cache-Control", "public, max-age=86400")
				storeCDNCache(resp.Request, resp.StatusCode, contentType, modifiedBytes)
			}
		} else if resp.StatusCode == http.StatusOK && resp.Request != nil &&
			resp.Header.Get("Set-Cookie") == "" &&
			cdnCacheKeyFromUpstream(resp.Request) != "" {
			bodyBytes, err := io.ReadAll(resp.Body)
			if err == nil {
				resp.Body.Close()
				if len(bodyBytes) > 0 && len(bodyBytes) <= cdnCacheMax {
					resp.Header.Set("Cache-Control", "public, max-age=86400")
					storeCDNCache(resp.Request, resp.StatusCode, contentType, bodyBytes)
				}
				resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				resp.ContentLength = int64(len(bodyBytes))
				resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(bodyBytes)))
			}
		}

		return nil
	}

	addr := ":" + cfg.Port
	if cfg.BindLocalhost {
		addr = "127.0.0.1:" + cfg.Port
	}
	listenDisplay := addr
	if strings.HasPrefix(addr, ":") {
		listenDisplay = "127.0.0.1" + addr
	}
	log.Printf("╔══════════════════════════════════════════════════╗")
	log.Printf("║  🚀 SEMrush NEW Go Proxy — www.semrush.com       ║")
	log.Printf("║  Local URL: http://%-28s ║", listenDisplay)
	log.Printf("║  Target   : %s                        ║", cfg.TargetURL)
	log.Printf("║  CDN Proxy: /static-proxy/ /secure-proxy/ /cdn-proxy/ /ai-proxy/ ║")
	if cfg.LocalTestMode {
		log.Printf("║  Mode     : LOCAL TEST (cookie.txt) ✅            ║")
	}
	log.Printf("║  Hot-Reload: %s & cookie.txt ✅                 ║", getConfigFile())
	log.Printf("╚══════════════════════════════════════════════════╝")
	logStartupDiagnostics(cfg)
	logUpstreamProxyStatus(cfg)
	startStatsReporter(cfg)
	startCDNCacheSweep()

	// Pre-activate Semrush session so first page load skips multilogin wall.
	if cfg.LocalTestMode {
		if ck := loadCookies(cfg); ck != "" {
			ensureSemrushSessionActivated(cfg, ck, cfg.UserAgent, true)
		}
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg = loadConfig()
		adoptSemrushDeviceQuery(r)

		rl := &requestLogger{
			start:      time.Now(),
			reqHost:    r.Host,
			method:     r.Method,
			path:       r.URL.Path,
			query:      r.URL.RawQuery,
			origin:     r.Header.Get("Origin"),
			clientIP:   realClientIP(r),
			category:   classifyPath(r.URL.Path),
			cookieInfo: summarizeSemrushCookies(r.Header.Get("Cookie"), ""),
		}
		defer rl.finish(cfg)
		r = r.WithContext(context.WithValue(r.Context(), proxyLogKey, rl))
		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		if r.URL.Path == "/__healthz" {
			px := resolveUpstreamProxy(cfg, getWebsiteProxy(cfg.WebsiteID))
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			if px != nil {
				fmt.Fprintf(w, "ok build=semrush-db-proxy proxy=%s://%s\n", px.Scheme, px.Host)
			} else {
				fmt.Fprintf(w, "ok build=semrush-db-proxy proxy=none — set ahrefs_accounts.proxy OR ahrefs_websites.proxy OR proxy.txt\n")
			}
			return
		}

		if r.URL.Path == "/access" {
			serveSemrushAccess(w, r)
			return
		}
		if r.URL.Path == "/api/device-bind" {
			handleSemrushDeviceBind(w, r)
			return
		}
		if r.URL.Path == "/api/user-limits" {
			semrushUserLimits(w, r)
			rl.status = 200
			return
		}
		if r.URL.Path == "/tm-device-sw.js" {
			serveSemrushDeviceSW(w, r)
			return
		}
		if semrushRequireSession(sr, r) {
			if sr.status != 0 {
				rl.status = sr.status
			}
			return
		}

		// 00. Global Path Blocking Security (Domain-Independent)
		// Prevent users from accessing sensitive profile/billing/subscription URLs
		requestPath := strings.ToLower(r.URL.Path)
		if requestPath == "/multilogin" || strings.HasPrefix(requestPath, "/multilogin/") {
			dest := r.URL.Query().Get("redirect_to")
			if dest == "" || dest == "/" || !strings.HasPrefix(dest, "/") || strings.HasPrefix(dest, "//") || strings.Contains(strings.ToLower(dest), "multilogin") {
				dest = "/home/"
			}
			http.Redirect(sr, r, dest, http.StatusFound)
			rl.status = http.StatusFound
			return
		}
		if requestPath == "/sso/logout" || strings.HasPrefix(requestPath, "/sso/logout/") || strings.Contains(strings.ToLower(r.URL.RawQuery), "disable_hard") {
			serveSemrushAccountSwap(sr, r)
			rl.status = http.StatusOK
			return
		}
		blockedPaths := []string{
			"/accounts/profile",
			"/accounts/subscription-info",
			"/accounts/notifications",
			"/accounts/queries",
			"/accounts/activities",
			"/accounts/tokens",
			"/company/beta-tester-club",
			"/enterprise",
			"/corporate/account",
		}
		for _, blocked := range blockedPaths {
			if requestPath == blocked || strings.HasPrefix(requestPath, blocked+"/") {
				log.Printf("[BLOCK] 🛡️ Global block triggered for sensitive path: %s", r.URL.Path)
				// Relative redirect back to Semrush home dashboard root (works perfectly for all reseller domains!)
				http.Redirect(sr, r, "/", http.StatusFound)
				rl.status = http.StatusFound
				return
			}
		}

		// 0. Server-to-Server Auth Handshake API
		if r.URL.Path == "/api/auth-handshake" {
			if r.Method != http.MethodPost {
				http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
				return
			}

			var payload struct {
				Username      string        `json:"username"`
				ProductIDsRaw []interface{} `json:"product_ids"`
				ClientIP      string        `json:"client_ip"`
				Timestamp     int64         `json:"timestamp"`
				Signature     string        `json:"signature"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				log.Printf("[HANDSHAKE] Bad JSON payload: %v", err)
				http.Error(w, "Bad Request: Invalid JSON", http.StatusBadRequest)
				return
			}

			if payload.Username == "" || payload.ClientIP == "" || payload.Signature == "" {
				http.Error(w, "Bad Request: Missing required fields", http.StatusBadRequest)
				return
			}

			// Determine scheme and host dynamically
			currentHost := resolveHandshakeHost(r, cfg)
			currentScheme := "http"
			if r.TLS != nil {
				currentScheme = "https"
			} else if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
				currentScheme = proto
			} else if cfg.PublicScheme != "" {
				currentScheme = cfg.PublicScheme
			}

			if db == nil {
				log.Printf("[HANDSHAKE] ❌ DB offline — cannot resolve domain '%s'", currentHost)
				http.Error(w, "Service Unavailable: DB Offline", http.StatusServiceUnavailable)
				return
			}

			requestWebsiteID := resolveWebsiteIDForHost(currentHost, cfg)
			if requestWebsiteID <= 0 {
				log.Printf("[HANDSHAKE] ❌ Unknown host '%s' (Host=%q X-Forwarded-Host=%q)", currentHost, r.Host, r.Header.Get("X-Forwarded-Host"))
				http.Error(w, "Forbidden: Unknown domain", http.StatusForbidden)
				return
			}

			// Validate signature using secret key from database (or config fallback)
			var dbSecretKey string
			err := db.QueryRow("SELECT secret_key FROM ahrefs_websites WHERE id = ?", requestWebsiteID).Scan(&dbSecretKey)
			if err != nil {
				// Default Semrush Handshake key fallback
				dbSecretKey = "toolsmandi_semrush_secret_xyz123"
			}

			// HMAC-SHA256 signature check
			mac := hmac.New(sha256.New, []byte(dbSecretKey))
			mac.Write([]byte(fmt.Sprintf("%s:%d", payload.Username, payload.Timestamp)))
			expectedSig := hex.EncodeToString(mac.Sum(nil))

			if !hmac.Equal([]byte(payload.Signature), []byte(expectedSig)) {
				log.Printf("[HANDSHAKE] ❌ HMAC verification failed for user: %s", payload.Username)
				http.Error(w, "Forbidden: Invalid signature", http.StatusForbidden)
				return
			}

			// Reject requests older than 5 minutes
			if time.Now().Unix()-payload.Timestamp > 300 {
				log.Printf("[HANDSHAKE] ❌ Stale request rejected: %s", payload.Username)
				http.Error(w, "Forbidden: Request expired", http.StatusForbidden)
				return
			}

			// Generate One-Time Token (OTT)
			ottBytes := make([]byte, 32)
			if _, err := rand.Read(ottBytes); err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			ott := hex.EncodeToString(ottBytes)
			expiresAt := time.Now().Add(2 * time.Minute)

			// Insert OTT into local database under the resolved requestWebsiteID
			_, err = db.Exec(
				"INSERT INTO ahrefs_tokens (website_id, token, username, client_ip, expires_at) VALUES (?, ?, ?, ?, ?)",
				requestWebsiteID, ott, payload.Username, payload.ClientIP, expiresAt,
			)
			if err != nil {
				log.Printf("[HANDSHAKE] ❌ DB Error inserting OTT: %v", err)
				http.Error(w, "Internal Database Error", http.StatusInternalServerError)
				return
			}

			// Return secure handshake redirect URL
			redirectURL := fmt.Sprintf("%s://%s/access?user=%s&token=%s",
				currentScheme, currentHost, url.QueryEscape(payload.Username), ott)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"redirect_url": redirectURL,
			})
			return
		}

		// 1. OTT access token handshake
		if r.URL.Path == "/access" {
			username := r.URL.Query().Get("user")
			ott := r.URL.Query().Get("token")

			if username == "" || ott == "" {
				http.Error(w, "Bad Request: Missing user or token", http.StatusBadRequest)
				return
			}

			if db == nil {
				http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
				return
			}

			var storedUsername, storedIP string
			var expiresAt time.Time
			var tokenID, tokenWebsiteID int
			err = db.QueryRow(
				"SELECT id, website_id, username, client_ip, expires_at FROM ahrefs_tokens WHERE token = ?",
				ott,
			).Scan(&tokenID, &tokenWebsiteID, &storedUsername, &storedIP, &expiresAt)

			if err == sql.ErrNoRows {
				log.Printf("[ACCESS] ❌ OTT not found for token: %s", ott)
				renderAccessDeniedPage(w)
				return
			} else if err != nil {
				log.Printf("[ACCESS] DB error on OTT lookup: %v", err)
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}

			// Dynamically synchronize the website_id from the authorized token
			accessWebsiteID := tokenWebsiteID

			if time.Now().After(expiresAt) {
				log.Printf("[ACCESS] ❌ Expired OTT for user: %s (expired at %v)", username, expiresAt)
				_, _ = db.Exec("DELETE FROM ahrefs_tokens WHERE id = ?", tokenID)
				renderAccessDeniedPage(w)
				return
			}

			if !strings.EqualFold(strings.TrimSpace(storedUsername), strings.TrimSpace(username)) {
				log.Printf("[ACCESS] ❌ Username mismatch: token was for '%s', got '%s'", storedUsername, username)
				_, _ = db.Exec("DELETE FROM ahrefs_tokens WHERE id = ?", tokenID)
				renderAccessDeniedPage(w)
				return
			}

			if !validateRequestHostMatchesWebsite(r, tokenWebsiteID) {
				log.Printf("[ACCESS] ❌ Host/domain mismatch for website_id=%d host=%s fwd=%s", tokenWebsiteID, r.Host, r.Header.Get("X-Forwarded-Host"))
				_, _ = db.Exec("DELETE FROM ahrefs_tokens WHERE id = ?", tokenID)
				renderAccessDeniedPage(w)
				return
			}

			clientIP := realClientIP(r)
			if !ottIPAllowed(storedIP, clientIP) {
				log.Printf("[ACCESS] ❌ OTT IP rejected for user %s (stored=%s got=%s)", username, storedIP, clientIP)
				_, _ = db.Exec("DELETE FROM ahrefs_tokens WHERE id = ?", tokenID)
				renderAccessDeniedPage(w)
				return
			}

			// Delete OTT only after all checks pass (refresh-safe until success)
			_, _ = db.Exec("DELETE FROM ahrefs_tokens WHERE id = ?", tokenID)

			// Automatically expire custom limits that have passed their expiration date
			_, _ = db.Exec(`
				UPDATE ahrefs_users u
				JOIN ahrefs_websites w ON u.website_id = w.id
				SET u.credit_limit = COALESCE(w.default_credit_limit, 50),
					u.export_limit = COALESCE(w.default_export_limit, 100000),
					u.custom_limit_expire_at = NULL
				WHERE u.custom_limit_expire_at IS NOT NULL AND u.custom_limit_expire_at < NOW()
			`)

			var dbStatus string
			err = db.QueryRow("SELECT status FROM ahrefs_users WHERE username = ? AND website_id = ?", username, accessWebsiteID).Scan(&dbStatus)
			if err == sql.ErrNoRows {
				_, _ = db.Exec("INSERT INTO ahrefs_users (username, website_id, credit_limit, export_limit) VALUES (?, ?, 50, 100000)", username, accessWebsiteID)
			} else if dbStatus == "suspended" {
				http.Error(w, "Account Suspended. Contact support.", http.StatusForbidden)
				return
			}

			sessionToken := generateSessionToken()
			sessionDuration := 30
			var dbSessionDuration int
			if errDb := db.QueryRow("SELECT session_duration FROM ahrefs_websites WHERE id = ?", accessWebsiteID).Scan(&dbSessionDuration); errDb == nil && dbSessionDuration > 0 {
				sessionDuration = dbSessionDuration
			}
			sessionExpires := time.Now().Add(time.Duration(sessionDuration) * time.Minute)

			// Single session enforcement
			_, _ = db.Exec("DELETE FROM ahrefs_sessions WHERE username = ? AND website_id = ?", username, accessWebsiteID)

			var assignedAccountID sql.NullInt64
			if initialAcc, errAcc := selectActiveAccount(accessWebsiteID); errAcc == nil {
				assignedAccountID.Int64 = int64(initialAcc.ID)
				assignedAccountID.Valid = true
			}

			_, err = db.Exec(
				"INSERT INTO ahrefs_sessions (session_token, username, client_ip, expires_at, website_id, assigned_account_id) VALUES (?, ?, ?, ?, ?, ?)",
				sessionToken, storedUsername, clientIP, sessionExpires, accessWebsiteID, assignedAccountID,
			)
			if err != nil {
				log.Printf("[ACCESS] ❌ DB Error inserting session: %v", err)
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}

			userAgentStr := r.Header.Get("User-Agent")
			_, _ = db.Exec("INSERT INTO ahrefs_login_logs (website_id, username, client_ip, user_agent) VALUES (?, ?, ?, ?)", accessWebsiteID, username, realClientIP(r), userAgentStr)

			http.SetCookie(w, &http.Cookie{
				Name:     "sem_session",
				Value:    sessionToken,
				Path:     "/",
				Expires:  sessionExpires,
				HttpOnly: true,
				Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
				SameSite: http.SameSiteLaxMode,
			})

			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		// 2. Legacy MySQL session gate (ahrefs_sessions).
		// gt4rents panel mode already authenticated via semrushRequireSession + panel.db
		// live_sessions — do NOT also require MySQL ahrefs_sessions (tokens live only in panel.db).
		isStatic := strings.HasPrefix(r.URL.Path, "/static-proxy/") || strings.HasPrefix(r.URL.Path, "/secure-proxy/") || strings.HasPrefix(r.URL.Path, "/cdn-proxy/") || strings.HasPrefix(r.URL.Path, "/ai-proxy/")
		panelMode := strings.TrimSpace(cfg.PanelDB) != ""
		if !isStatic && db != nil && !cfg.LocalTestMode && !panelMode {
			var isAuthed bool
			cookie, errC := r.Cookie("sem_session")
			if errC == nil && cookie.Value != "" {
				var username string
				var expiresAt time.Time
				var sessionWebsiteID int
				errS := db.QueryRow("SELECT username, expires_at, website_id FROM ahrefs_sessions WHERE session_token = ?", cookie.Value).Scan(&username, &expiresAt, &sessionWebsiteID)
				if errS == nil && time.Now().Before(expiresAt) {
					if !validateRequestHostMatchesWebsite(r, sessionWebsiteID) {
						log.Printf("[AUTH] ❌ Session host mismatch for website_id=%d host=%s", sessionWebsiteID, r.Host)
					} else {
						isAuthed = true
						r = r.WithContext(withTenantWebsiteID(r.Context(), sessionWebsiteID))
					}
				} else {
					log.Printf("[AUTH] ❌ Session validation failed for token '%s': ErrS=%v, Expired=%v (Path: %s)", cookie.Value, errS, time.Now().After(expiresAt), r.URL.Path)
				}
			} else {
				log.Printf("[AUTH] ❌ No active sem_session cookie found in request (Path: %s)", r.URL.Path)
			}

			if !isAuthed {
				renderAccessDeniedPage(w)
				return
			}
		}

		// 3. Disk CDN cache (Ahrefs-style) + upstream
		cdnKey := cdnCacheKey(r)
		if served, leader := serveCachedCDN(w, r); served {
			rl.status = http.StatusOK
			rl.category = "CDN_CACHE"
			return
		} else if leader && cdnKey != "" {
			// Only the flight leader completes — followers must not close the channel early.
			defer completeCDNFlight(cdnKey)
		}
		if semrushDenyOverLimit(sr, r) {
			if sr.status != 0 {
				rl.status = sr.status
			}
			return
		}
		proxy.ServeHTTP(sr, r)
		if rl.status == 0 {
			rl.status = sr.status
		}
	})

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("[FATAL] Server crash: %v", err)
	}
}
