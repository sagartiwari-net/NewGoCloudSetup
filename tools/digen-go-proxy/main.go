package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/proxy"
)

const CONFIG_FILE = "config.json"
const COOKIE_FILE = "cookie.txt"
const PROXY_FILE = "proxy.txt"

type contextKey string

const proxyContextKey contextKey = "account_proxy"
const accountIDContextKey contextKey = "account_id"
const sessionTokenContextKey contextKey = "session_token"
const publicHostContextKey contextKey = "public_host"
const publicSchemeContextKey contextKey = "public_scheme"

// Config structure with MySQL support
type Config struct {
	Port          string `json:"port"`
	TargetURL     string `json:"target_url"`
	PublicHost    string `json:"public_host"`
	PublicScheme  string `json:"public_scheme"`
	UserAgent     string `json:"user_agent"`
	WebsiteID     int    `json:"website_id"`
	MySQLHost     string `json:"mysql_host"`
	MySQLPort     string `json:"mysql_port"`
	MySQLUser     string `json:"mysql_user"`
	MySQLPassword string `json:"mysql_password"`
	MySQLDB       string `json:"mysql_db"`
	Cookie        string `json:"cookie"`
	Proxy         string `json:"proxy"`
	PanelDB       string `json:"panel_db"`
	ToolName      string `json:"tool_name"`
	HomePath      string `json:"home_path"`
}

var (
	cfg               Config
	cfgMu             sync.RWMutex
	db                *sql.DB
	cookieJar         = make(map[int]map[string]string) // Scoped by account ID
	cookieJarMu       sync.RWMutex
	currentProxy      *url.URL
	proxyMu           sync.Mutex
	proxyModTime      time.Time
	configModTime     time.Time
	sessionLastSwap   = make(map[string]time.Time)
	sessionLastSwapMu sync.Mutex
)

type Account struct {
	ID        int
	Name      string
	Cookie    string
	UserAgent string
	Proxy     string
}

const BUILD_VERSION = "v2.1-20260606"

func main() {
	log.Printf("🚀 [STARTING] Digen Reverse Proxy (Build: %s)", BUILD_VERSION)

	// Load configuration
	loadConfig()

	// Connect to MySQL if configured
	if cfg.MySQLHost != "" && cfg.MySQLUser != "" && cfg.MySQLDB != "" {
		initDB()
	} else {
		log.Println("[DB] ℹ️ MySQL configuration is empty. Running in Static/Testing Mode!")
	}

	// Create reverse proxy
	target, err := url.Parse(cfg.TargetURL)
	if err != nil {
		log.Fatalf("[FATAL] Invalid target URL: %v", err)
	}

	reverseProxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			cfgMu.RLock()
			defer cfgMu.RUnlock()

			// Retrieve headers passed from HTTP middleware
			resolvedCookie := req.Header.Get("X-Resolved-Cookie")
			resolvedUserAgent := req.Header.Get("X-Resolved-UA")

			req.Header.Del("X-Resolved-Cookie")
			req.Header.Del("X-Resolved-UA")

			// Apply in-memory Cloudflare cookies
			if resolvedCookie != "" {
				accIDVal, _ := req.Context().Value(accountIDContextKey).(int)
				resolvedCookie = mergeCookiesWithJar(resolvedCookie, accIDVal)
			}

			// Determine target host dynamically
			targetHost := target.Host
			targetScheme := target.Scheme

			if strings.HasPrefix(req.URL.Path, "/api-proxy") {
				req.URL.Path = strings.TrimPrefix(req.URL.Path, "/api-proxy")
				targetHost = "api.digen.ai"
				targetScheme = "https"
			} else if strings.HasPrefix(req.URL.Path, "/test-api-proxy") {
				req.URL.Path = strings.TrimPrefix(req.URL.Path, "/test-api-proxy")
				targetHost = "test.api.digen.ai"
				targetScheme = "https"
			} else if strings.HasPrefix(req.URL.Path, "/agent-proxy") {
				req.URL.Path = strings.TrimPrefix(req.URL.Path, "/agent-proxy")
				targetHost = "agent.digen.ai"
				targetScheme = "https"
			} else if strings.HasPrefix(req.URL.Path, "/create-proxy") {
				req.URL.Path = strings.TrimPrefix(req.URL.Path, "/create-proxy")
				targetHost = "create.digen.ai"
				targetScheme = "https"
			} else if strings.HasPrefix(req.URL.Path, "/blog-proxy") {
				req.URL.Path = strings.TrimPrefix(req.URL.Path, "/blog-proxy")
				targetHost = "blog.digen.ai"
				targetScheme = "https"
			} else if strings.HasPrefix(req.URL.Path, "/resource-proxy") {
				req.URL.Path = strings.TrimPrefix(req.URL.Path, "/resource-proxy")
				targetHost = "resource.digen.ai"
				targetScheme = "https"
			}

			// Set target URL
			req.URL.Scheme = targetScheme
			req.URL.Host = targetHost

			// Preserve request path or override host
			requestPath := req.URL.Path
			req.Header.Set("Host", targetHost)
			req.URL.Path = requestPath
			req.Host = targetHost

			// Set User-Agent and synchronize Client Hints (Sec-Ch-Ua)
			req.Header.Set("User-Agent", resolvedUserAgent)
			setClientHintHeaders(req, resolvedUserAgent)

			// Set Session Cookies
			isSessionDomain := targetHost == "digen.ai" || strings.HasSuffix(targetHost, ".digen.ai")
			if isSessionDomain && resolvedCookie != "" {
				req.Header.Set("Cookie", resolvedCookie)
			} else {
				req.Header.Del("Cookie")
			}

			// Spoof headers to look like a real browser
			req.Header.Set("Accept-Language", "en-US,en;q=0.9")
			req.Header.Set("Accept-Encoding", "gzip")
			req.Header.Set("Cache-Control", "no-cache")
			req.Header.Set("Pragma", "no-cache")
		},

		Transport: &roundTripper{
			base: buildChromeHTTPClient(),
		},

		ModifyResponse: func(resp *http.Response) error {
			cfgMu.RLock()
			defer cfgMu.RUnlock()

			publicHost := cfg.PublicHost
			publicScheme := cfg.PublicScheme
			if resp.Request != nil {
				ctx := resp.Request.Context()
				if val, ok := ctx.Value(publicHostContextKey).(string); ok && val != "" {
					publicHost = val
				}
				if val, ok := ctx.Value(publicSchemeContextKey).(string); ok && val != "" {
					publicScheme = val
				}
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					if bill, ok := ctx.Value(digenBillKey).(digenBill); ok {
						digenCharge(cfg, bill)
					}
				}
			}

			// Rewrite Set-Cookie domains
			cookies := resp.Header["Set-Cookie"]
			if len(cookies) > 0 {
				// Only update short-lived live cookie jar on successful (200) or redirecting (3xx) responses.
				if resp.StatusCode == 200 || (resp.StatusCode >= 300 && resp.StatusCode < 400) {
					accIDVal, _ := resp.Request.Context().Value(accountIDContextKey).(int)
					updateCookieJar(cookies, accIDVal)
				}
				var rewrittenCookies []string
				for _, c := range cookies {
					rewritten := c
					rewritten = strings.ReplaceAll(rewritten, "domain=.digen.ai", "")
					rewritten = strings.ReplaceAll(rewritten, "domain=digen.ai", "")
					rewritten = strings.ReplaceAll(rewritten, "; Secure", "")
					rewritten = strings.ReplaceAll(rewritten, ";Secure", "")
					rewritten = strings.ReplaceAll(rewritten, "; secure", "")
					rewrittenCookies = append(rewrittenCookies, rewritten)
				}
				resp.Header["Set-Cookie"] = rewrittenCookies
			}

			// Rewrite 3xx redirect locations
			if resp.StatusCode >= 300 && resp.StatusCode < 400 {
				location := resp.Header.Get("Location")
				if location != "" {
					lowerLoc := strings.ToLower(location)
					if (strings.Contains(lowerLoc, "sign-in") || strings.Contains(lowerLoc, "login") || strings.Contains(lowerLoc, "auth") || strings.Contains(lowerLoc, "pricing") || strings.Contains(lowerLoc, "subscribe")) &&
						!strings.Contains(lowerLoc, "/api/auth-handshake") &&
						!strings.Contains(lowerLoc, "/access") {

						if db != nil && resp.Request != nil {
							ctx := resp.Request.Context()
							sessionToken, _ := ctx.Value(sessionTokenContextKey).(string)
							accIDVal, _ := ctx.Value(accountIDContextKey).(int)
							websiteIDVal := cfg.WebsiteID

							// Resolve website ID dynamically using public host from context
							var resolvedWebID int
							publicHostVal, _ := ctx.Value(publicHostContextKey).(string)
							if publicHostVal == "" {
								publicHostVal = resp.Request.Host
							}
							errW := db.QueryRow("SELECT id FROM ahrefs_websites WHERE domain = ?", getHostWithoutPort(publicHostVal)).Scan(&resolvedWebID)
							if errW == nil {
								websiteIDVal = resolvedWebID
							}

							if sessionToken != "" {
								swapCount := 0
								if swapCookie, errS := resp.Request.Cookie("digen_swap_count"); errS == nil {
									fmt.Sscanf(swapCookie.Value, "%d", &swapCount)
								}

								if swapCount < 3 {
									nextAcc, errSwap := swapActiveAccount(sessionToken, int64(accIDVal), websiteIDVal)
									if errSwap == nil {
										var fromAccName string
										if accIDVal != 0 {
											_ = db.QueryRow("SELECT name FROM ahrefs_accounts WHERE id = ?", accIDVal).Scan(&fromAccName)
										}
										_, _ = db.Exec("INSERT INTO ahrefs_switch_logs (website_id, session_token, username, from_account_id, from_account_name, to_account_id, to_account_name, reason) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
											websiteIDVal, sessionToken, "", accIDVal, fromAccName, nextAcc.ID, nextAcc.Name, "Digen redirect to "+location)

										// Set swap count cookie
										newSwapCountCookie := &http.Cookie{
											Name:     "digen_swap_count",
											Value:    fmt.Sprintf("%d", swapCount+1),
											Path:     "/",
											MaxAge:   10,
											HttpOnly: true,
										}
										resp.Header.Add("Set-Cookie", newSwapCountCookie.String())

										// Return gorgeous swapping loading screen
										htmlContent := renderSwappingPage("/")
										resp.StatusCode = http.StatusOK
										resp.Header.Set("Content-Type", "text/html; charset=utf-8")
										resp.Body = io.NopCloser(strings.NewReader(htmlContent))
										resp.ContentLength = int64(len(htmlContent))
										resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(htmlContent)))
										resp.Header.Del("Content-Encoding")
										resp.Header.Del("Location")
										return nil
									}
								}
							}

							// If loop protection active or no other active accounts, fallback
							allAccountsOfflinePage := `<!DOCTYPE html><html><head><meta charset="UTF-8"><title>Premium Session Offline</title><style>body{font-family:sans-serif;background:#1a1a2e;color:#eee;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;flex-direction:column;gap:16px}h2{color:#ff6b6b;margin:0}p{color:#aaa;margin:0;font-size:14px}a{color:#4facfe;text-decoration:none;padding:10px 24px;border:1px solid #4facfe;border-radius:6px;margin-top:8px;display:inline-block}a:hover{background:#4facfe;color:#000}</style></head><body><h2>⚠️ Premium Session Offline</h2><p>All premium accounts are currently logged out or limit reached. Please contact support.</p><a href="/">Try Again</a></body></html>`
							resp.StatusCode = http.StatusOK
							resp.Header.Set("Content-Type", "text/html; charset=utf-8")
							resp.Body = io.NopCloser(strings.NewReader(allAccountsOfflinePage))
							resp.ContentLength = int64(len(allAccountsOfflinePage))
							resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(allAccountsOfflinePage)))
							resp.Header.Del("Content-Encoding")
							resp.Header.Del("Location")
							return nil
						}
					}
					// Rewrite location to point back to our domain
					targetURLStr := cfg.TargetURL
					publicURLStr := fmt.Sprintf("%s://%s", publicScheme, publicHost)
					if strings.HasPrefix(location, "https://api.digen.ai") {
						location = strings.Replace(location, "https://api.digen.ai", fmt.Sprintf("%s://%s/api-proxy", publicScheme, publicHost), 1)
					} else if strings.HasPrefix(location, "https://test.api.digen.ai") {
						location = strings.Replace(location, "https://test.api.digen.ai", fmt.Sprintf("%s://%s/test-api-proxy", publicScheme, publicHost), 1)
					} else if strings.HasPrefix(location, "https://agent.digen.ai") {
						location = strings.Replace(location, "https://agent.digen.ai", fmt.Sprintf("%s://%s/agent-proxy", publicScheme, publicHost), 1)
					} else if strings.HasPrefix(location, "https://create.digen.ai") {
						location = strings.Replace(location, "https://create.digen.ai", fmt.Sprintf("%s://%s/create-proxy", publicScheme, publicHost), 1)
					} else if strings.HasPrefix(location, "https://blog.digen.ai") {
						location = strings.Replace(location, "https://blog.digen.ai", fmt.Sprintf("%s://%s/blog-proxy", publicScheme, publicHost), 1)
					} else if strings.HasPrefix(location, "https://resource.digen.ai") {
						location = strings.Replace(location, "https://resource.digen.ai", fmt.Sprintf("%s://%s/resource-proxy", publicScheme, publicHost), 1)
					} else if strings.HasPrefix(location, targetURLStr) {
						location = strings.Replace(location, targetURLStr, publicURLStr, 1)
					}
					resp.Header.Set("Location", location)
				}
			}

			// Decompress if response is compressed
			var reader io.ReadCloser = resp.Body
			encoding := resp.Header.Get("Content-Encoding")
			isGzip := strings.EqualFold(encoding, "gzip")
			if isGzip {
				gzipReader, err := gzip.NewReader(resp.Body)
				if err == nil {
					defer gzipReader.Close()
					reader = gzipReader
				}
			}

			bodyBytes, err := io.ReadAll(reader)
			if err != nil {
				return err
			}
			logDigenUse(resp, bodyBytes)

			isHTML := strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html")
			if !isHTML {
				trimmed := bytes.TrimSpace(bodyBytes)
				lowTrim := bytes.ToLower(trimmed)
				if bytes.HasPrefix(lowTrim, []byte("<!doctype html")) || bytes.HasPrefix(lowTrim, []byte("<html")) {
					isHTML = true
				}
			}
			hostInBody := bytes.Contains(bodyBytes, []byte("digen.ai"))

			// Detect limit/free in body content or status code. Large catalog JSON is not a login page.
			if db != nil && resp.Request != nil && (len(bodyBytes) <= 64*1024 || resp.StatusCode == 401 || resp.StatusCode == 403) && detectDigenLimitOrFree(resp.Request.URL.Path, resp.StatusCode, string(bodyBytes)) {
				ctx := resp.Request.Context()
				sessionToken, _ := ctx.Value(sessionTokenContextKey).(string)
				accIDVal, _ := ctx.Value(accountIDContextKey).(int)
				websiteIDVal := cfg.WebsiteID

				// Resolve website ID dynamically using public host from context
				var resolvedWebID int
				publicHostVal, _ := ctx.Value(publicHostContextKey).(string)
				if publicHostVal == "" {
					publicHostVal = resp.Request.Host
				}
				errW := db.QueryRow("SELECT id FROM ahrefs_websites WHERE domain = ?", getHostWithoutPort(publicHostVal)).Scan(&resolvedWebID)
				if errW == nil {
					websiteIDVal = resolvedWebID
				}

				if sessionToken != "" {
					swapCount := 0
					if swapCookie, errS := resp.Request.Cookie("digen_swap_count"); errS == nil {
						fmt.Sscanf(swapCookie.Value, "%d", &swapCount)
					}

					if swapCount < 3 {
						nextAcc, errSwap := swapActiveAccount(sessionToken, int64(accIDVal), websiteIDVal)
						if errSwap == nil {
							var fromAccName string
							if accIDVal != 0 {
								_ = db.QueryRow("SELECT name FROM ahrefs_accounts WHERE id = ?", accIDVal).Scan(&fromAccName)
							}
							_, _ = db.Exec("INSERT INTO ahrefs_switch_logs (website_id, session_token, username, from_account_id, from_account_name, to_account_id, to_account_name, reason) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
								websiteIDVal, sessionToken, "", accIDVal, fromAccName, nextAcc.ID, nextAcc.Name, "Digen free/limit detected in body of "+resp.Request.URL.Path)

							// Increment swap count cookie
							newSwapCountCookie := &http.Cookie{
								Name:     "digen_swap_count",
								Value:    fmt.Sprintf("%d", swapCount+1),
								Path:     "/",
								MaxAge:   10,
								HttpOnly: true,
							}
							resp.Header.Add("Set-Cookie", newSwapCountCookie.String())

							// For API calls (JSON), return a custom JSON reload message instead of HTML swap page
							contentType := resp.Header.Get("Content-Type")
							isJSON := strings.Contains(contentType, "application/json") || strings.HasPrefix(resp.Request.URL.Path, "/api-proxy") || strings.HasPrefix(resp.Request.URL.Path, "/test-api-proxy")

							if isJSON {
								// Return JSON signaling retry or reload
								jsonResp := `{"error":"account_swapped","message":"Session rotated to premium account, please reload.","reload":true,"code":401}`
								resp.StatusCode = http.StatusUnauthorized
								resp.Header.Set("Content-Type", "application/json; charset=utf-8")
								resp.Body = io.NopCloser(strings.NewReader(jsonResp))
								resp.ContentLength = int64(len(jsonResp))
								resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(jsonResp)))
								resp.Header.Del("Content-Encoding")
								return nil
							} else {
								// Return gorgeous loading screen that reloads/redirects to redirectURL
								redirectURL := resp.Request.URL.Path
								if resp.Request.URL.RawQuery != "" {
									redirectURL += "?" + resp.Request.URL.RawQuery
								}
								htmlContent := renderSwappingPage(redirectURL)
								resp.StatusCode = http.StatusOK
								resp.Header.Set("Content-Type", "text/html; charset=utf-8")
								resp.Body = io.NopCloser(strings.NewReader(htmlContent))
								resp.ContentLength = int64(len(htmlContent))
								resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(htmlContent)))
								resp.Header.Del("Content-Encoding")
								return nil
							}
						}
					}
				}
			}

			// Catalog JSON has no site host. Rewriting those multi-megabyte bodies is what stalls the page.
			skipRewrite := !hostInBody && !isHTML
			if resp.Request != nil {
				switch resp.Request.Host {
				case "agent.digen.ai", "create.digen.ai", "blog.digen.ai", "resource.digen.ai":
					skipRewrite = false
				}
			}
			if !skipRewrite {
				// Subdomain-specific asset path prefixing
				if resp.Request != nil {
					requestHost := resp.Request.Host
					if requestHost == "agent.digen.ai" {
						bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(`"/_next/`), []byte(`"/agent-proxy/_next/`))
						bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(`'/_next/`), []byte(`'/agent-proxy/_next/`))
						bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(`"/assets/`), []byte(`"/agent-proxy/assets/`))
						bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(`'/assets/`), []byte(`'/agent-proxy/assets/`))
						bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(`"/favicon`), []byte(`"/agent-proxy/favicon`))
						bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(`'/favicon`), []byte(`'/agent-proxy/favicon`))
					} else if requestHost == "create.digen.ai" {
						bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(`"/_next/`), []byte(`"/create-proxy/_next/`))
						bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(`'/_next/`), []byte(`'/create-proxy/_next/`))
						bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(`"/assets/`), []byte(`"/create-proxy/assets/`))
						bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(`'/assets/`), []byte(`'/create-proxy/assets/`))
						bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(`"/favicon`), []byte(`"/create-proxy/favicon`))
						bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(`'/favicon`), []byte(`'/create-proxy/favicon`))
					} else if requestHost == "blog.digen.ai" {
						bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(`"/_next/`), []byte(`"/blog-proxy/_next/`))
						bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(`'/_next/`), []byte(`'/blog-proxy/_next/`))
						bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(`"/assets/`), []byte(`"/blog-proxy/assets/`))
						bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(`'/assets/`), []byte(`'/blog-proxy/assets/`))
					} else if requestHost == "resource.digen.ai" {
						bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(`"/_next/`), []byte(`"/resource-proxy/_next/`))
						bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(`'/_next/`), []byte(`'/resource-proxy/_next/`))
						bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(`"/assets/`), []byte(`"/resource-proxy/assets/`))
						bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(`'/assets/`), []byte(`'/resource-proxy/assets/`))
					}
				}

				// Rewrite HTML/JS body content to replace target domain and subdomains with proxy paths
				bodyBytes = bytes.ReplaceAll(bodyBytes, []byte("https://api.digen.ai"), []byte(fmt.Sprintf("%s://%s/api-proxy", publicScheme, publicHost)))
				bodyBytes = bytes.ReplaceAll(bodyBytes, []byte("https://test.api.digen.ai"), []byte(fmt.Sprintf("%s://%s/test-api-proxy", publicScheme, publicHost)))
				bodyBytes = bytes.ReplaceAll(bodyBytes, []byte("https://agent.digen.ai"), []byte(fmt.Sprintf("%s://%s/agent-proxy", publicScheme, publicHost)))
				bodyBytes = bytes.ReplaceAll(bodyBytes, []byte("https://create.digen.ai"), []byte(fmt.Sprintf("%s://%s/create-proxy", publicScheme, publicHost)))
				bodyBytes = bytes.ReplaceAll(bodyBytes, []byte("https://blog.digen.ai"), []byte(fmt.Sprintf("%s://%s/blog-proxy", publicScheme, publicHost)))
				bodyBytes = bytes.ReplaceAll(bodyBytes, []byte("https://resource.digen.ai"), []byte(fmt.Sprintf("%s://%s/resource-proxy", publicScheme, publicHost)))

				targetURLParsed, _ := url.Parse(cfg.TargetURL)
				if targetURLParsed != nil {
					targetDomain := targetURLParsed.Host
					bodyBytes = bytes.ReplaceAll(bodyBytes, []byte("https://"+targetDomain), []byte(fmt.Sprintf("%s://%s", publicScheme, publicHost)))
					bodyBytes = bytes.ReplaceAll(bodyBytes, []byte("http://"+targetDomain), []byte(fmt.Sprintf("%s://%s", publicScheme, publicHost)))
					bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(targetDomain), []byte(publicHost))
				}

				if isHTML {
					cfgMu.RLock()
					panelOn := usesPanelAccountMode(cfg)
					cfgMu.RUnlock()
					if panelOn {
						bodyBytes = injectDeviceHTML(bodyBytes)
						bodyBytes = injectDigenCreditHTML(bodyBytes)
						bodyBytes = injectDigenHeaderHide(bodyBytes)
						resp.Header.Set("Cache-Control", "no-store")
					}
				}
			}

			// Compress body back to Gzip if it was compressed originally
			var finalBody []byte = bodyBytes
			if isGzip {
				var buf bytes.Buffer
				gzipWriter := gzip.NewWriter(&buf)
				if _, err := gzipWriter.Write(bodyBytes); err == nil {
					gzipWriter.Close()
					finalBody = buf.Bytes()
					resp.Header.Set("Content-Encoding", "gzip")
				} else {
					resp.Header.Del("Content-Encoding")
				}
			}

			resp.Body = io.NopCloser(bytes.NewReader(finalBody))
			resp.ContentLength = int64(len(finalBody))
			resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(finalBody)))

			return nil
		},
	}

	reverseProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if marked, px := panelUpstreamProxy(r.Context()); marked && px != "" {
			log.Printf("[PROXY] dial failed %s: %v", r.URL.Path, err)
			renderProxyProblem(w, r)
			return
		}
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	// Setup HTTP Handlers
	mux := http.NewServeMux()

	// aMember API and Auth access endpoints
	mux.HandleFunc("/api/auth-handshake", authHandshakeHandler)
	mux.HandleFunc("/access", func(w http.ResponseWriter, r *http.Request) {
		cfgMu.RLock()
		current := cfg
		cfgMu.RUnlock()
		if usesPanelAccountMode(current) {
			servePanelAccess(w, r, current)
			return
		}
		accessHandler(w, r)
	})
	mux.HandleFunc("/api/device-bind", deviceBindHandler)
	mux.HandleFunc("/api/user-limits", digenUserLimits)
	mux.HandleFunc("/__tm_access_denied", serveAccessDeniedHTML)
	mux.HandleFunc("/tm-device-sw.js", serveDeviceSW)

	// Debug/diagnostic endpoint
	mux.HandleFunc("/debug/check", func(w http.ResponseWriter, r *http.Request) {
		cfgMu.RLock()
		defer cfgMu.RUnlock()
		var dbOk bool
		var websiteCount int
		var resolvedWebID int
		var resolvedDomain string
		if db != nil {
			if db.Ping() == nil {
				dbOk = true
			}
			_ = db.QueryRow("SELECT COUNT(*) FROM ahrefs_websites").Scan(&websiteCount)
			errW := db.QueryRow("SELECT id, domain FROM ahrefs_websites WHERE domain = ?", r.Host).Scan(&resolvedWebID, &resolvedDomain)
			if errW != nil {
				resolvedDomain = "NOT FOUND: " + errW.Error()
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"build_version":        BUILD_VERSION,
			"config_website_id":    cfg.WebsiteID,
			"config_public_host":   cfg.PublicHost,
			"config_public_scheme": cfg.PublicScheme,
			"config_port":          cfg.Port,
			"request_host":         r.Host,
			"resolved_web_id":      resolvedWebID,
			"resolved_domain":      resolvedDomain,
			"db_connected":         dbOk,
			"website_count":        websiteCount,
			"x_forwarded_proto":    r.Header.Get("X-Forwarded-Proto"),
			"cookie_header":        r.Header.Get("Cookie"),
		})
	})

	// Catch-all Reverse Proxy handler
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Cookie")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		publicHost := r.Host
		publicScheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			publicScheme = "https"
		} else {
			cfgMu.RLock()
			publicScheme = cfg.PublicScheme
			cfgMu.RUnlock()
		}

		cfgMu.RLock()
		websiteID := cfg.WebsiteID
		panelCfg := cfg
		cfgMu.RUnlock()
		panelOn := usesPanelAccountMode(panelCfg)
		if panelOn {
			switch r.URL.Path {
			case "/access":
				servePanelAccess(w, r, panelCfg)
				return
			case "/api/device-bind":
				deviceBindHandler(w, r)
				return
			case "/api/user-limits":
				digenUserLimits(w, r)
				return
			case "/tm-device-sw.js":
				serveDeviceSW(w, r)
				return
			}
		}

		// The live panel does not use this MySQL lookup. Waiting on it stalls every request, including device-bind.
		if db != nil && !panelOn {
			var resolvedWebID int
			errW := db.QueryRow("SELECT id FROM ahrefs_websites WHERE domain = ?", getHostWithoutPort(r.Host)).Scan(&resolvedWebID)
			if errW == nil {
				websiteID = resolvedWebID
			}
		}
		var panelAcc ToolAccount
		if panelOn {
			var handled bool
			var next *http.Request
			panelAcc, next, handled = preparePanelRequest(w, r, panelCfg)
			if handled {
				return
			}
			r = next
		}

		// Check if local static testing mode is active (cookie.txt exists)
		var staticCookie string
		if !panelOn {
			if cookieData, err := os.ReadFile(COOKIE_FILE); err == nil {
				staticCookie = string(cookieData)
			}
		}
		isStaticMode := !panelOn && (staticCookie != "" || db == nil)

		var resolvedCookie, resolvedUserAgent, resolvedProxy string
		var accID int
		var activeAccountName string

		var currentUser string
		sessionCookie, errC := r.Cookie("ct_session")
		var isAuthed bool

		if panelOn {
			resolvedCookie = panelAcc.Cookie
			resolvedUserAgent = panelAcc.UserAgent
			resolvedProxy = panelAcc.Proxy
			isAuthed = true
		} else if isStaticMode {
			if staticCookie != "" {
				resolvedCookie = staticCookie
			} else {
				cfgMu.RLock()
				resolvedCookie = cfg.Cookie
				cfgMu.RUnlock()
			}
			cfgMu.RLock()
			resolvedUserAgent = cfg.UserAgent
			resolvedProxy = cfg.Proxy
			cfgMu.RUnlock()
			isAuthed = true
		} else if db != nil {
			if errC == nil && sessionCookie != nil && sessionCookie.Value != "" {
				var expiresAt time.Time
				var sessionWebsiteID int
				var assignedAccountID sql.NullInt64

				log.Printf("[AUTH-DEBUG] Checking session cookie token: %s", sessionCookie.Value)

				errSession := db.QueryRow(
					"SELECT username, expires_at, website_id, assigned_account_id FROM ahrefs_sessions WHERE session_token = ?",
					sessionCookie.Value,
				).Scan(&currentUser, &expiresAt, &sessionWebsiteID, &assignedAccountID)

				if errSession != nil {
					log.Printf("[AUTH-DEBUG] Session query failed for token %s: %v", sessionCookie.Value, errSession)
				} else {
					websiteID = sessionWebsiteID
					now := time.Now()
					isBefore := now.Before(expiresAt)
					log.Printf("[AUTH-DEBUG] Session found: User=%s, WebsiteID=%d, ExpiresAt=%v, Now=%v, Valid=%t",
						currentUser, sessionWebsiteID, expiresAt, now, isBefore)

					if isBefore {
						isAuthed = true

						// Extend session sliding window
						sessionDuration := 30
						var dbSessionDuration int
						if errDb := db.QueryRow("SELECT session_duration FROM ahrefs_websites WHERE id = ?", websiteID).Scan(&dbSessionDuration); errDb == nil && dbSessionDuration > 0 {
							sessionDuration = dbSessionDuration
						}
						newExpires := now.Add(time.Duration(sessionDuration) * time.Minute)
						_, _ = db.Exec("UPDATE ahrefs_sessions SET expires_at = ? WHERE session_token = ?", newExpires, sessionCookie.Value)

						// Fetch user status (reject if suspended)
						var userStatus string
						errU := db.QueryRow("SELECT status FROM ahrefs_users WHERE username = ? AND website_id = ?", currentUser, websiteID).Scan(&userStatus)
						if errU == nil && userStatus == "suspended" {
							log.Printf("[AUTH-DEBUG] User %s is suspended", currentUser)
							http.Error(w, "Account Suspended. Contact support.", http.StatusForbidden)
							return
						}
					} else {
						log.Printf("[AUTH-DEBUG] Session is expired! ExpiresAt=%v is before Now=%v", expiresAt, now)
					}

					// Resolve active account
					if assignedAccountID.Valid {
						errAcc := db.QueryRow(
							"SELECT id, name, cookie, user_agent, COALESCE(proxy, '') FROM ahrefs_accounts WHERE id = ? AND website_id = ? AND status = 'active'",
							assignedAccountID.Int64, websiteID,
						).Scan(&accID, &activeAccountName, &resolvedCookie, &resolvedUserAgent, &resolvedProxy)
						if errAcc != nil {
							log.Printf("[AUTH-DEBUG] Failed to load assigned account %d: %v", assignedAccountID.Int64, errAcc)
						} else {
							cookiePreview := ""
							if len(resolvedCookie) > 15 {
								cookiePreview = resolvedCookie[:15] + "..."
							} else {
								cookiePreview = resolvedCookie
							}
							log.Printf("[AUTH-DEBUG] Loaded assigned account %d (%s), Cookie (len=%d): %q", accID, activeAccountName, len(resolvedCookie), cookiePreview)
						}
					}

					if accID == 0 {
						// Auto assign an account
						acc, errAcc := selectActiveAccount(websiteID)
						if errAcc == nil {
							accID = acc.ID
							activeAccountName = acc.Name
							resolvedCookie = acc.Cookie
							resolvedUserAgent = acc.UserAgent
							resolvedProxy = acc.Proxy
							_, _ = db.Exec("UPDATE ahrefs_sessions SET assigned_account_id = ? WHERE session_token = ? AND website_id = ?", accID, sessionCookie.Value, websiteID)
							cookiePreview := ""
							if len(resolvedCookie) > 15 {
								cookiePreview = resolvedCookie[:15] + "..."
							} else {
								cookiePreview = resolvedCookie
							}
							log.Printf("[AUTH-DEBUG] Auto-assigned account %d (%s), Cookie (len=%d): %q", accID, activeAccountName, len(resolvedCookie), cookiePreview)
						} else {
							log.Printf("[AUTH-DEBUG] Auto-assign account failed for websiteID=%d: %v", websiteID, errAcc)
						}
					}
				}
			} else {
				log.Printf("[AUTH-DEBUG] No ct_session cookie received. errC: %v, cookie: %v", errC, sessionCookie)
			}

			// Fallback: If no account resolved but we still need one for public views, or if it failed
			if resolvedCookie == "" {
				acc, errAcc := selectActiveAccount(websiteID)
				if errAcc == nil {
					accID = acc.ID
					activeAccountName = acc.Name
					resolvedCookie = acc.Cookie
					resolvedUserAgent = acc.UserAgent
					resolvedProxy = acc.Proxy
					cookiePreview := ""
					if len(resolvedCookie) > 15 {
						cookiePreview = resolvedCookie[:15] + "..."
					} else {
						cookiePreview = resolvedCookie
					}
					log.Printf("[AUTH-DEBUG] Fallback resolved account %d (%s) for websiteID=%d, Cookie (len=%d): %q", accID, activeAccountName, websiteID, len(resolvedCookie), cookiePreview)
				} else {
					log.Printf("[AUTH-DEBUG] Fallback account resolution failed for websiteID=%d: %v", websiteID, errAcc)
				}
			}
		}

		// Security block: require authentication for non-static assets
		isStaticAsset := isStaticPath(r.URL.Path)
		isExempt := r.URL.Path == "/api/auth-handshake" || r.URL.Path == "/access" ||
			strings.Contains(r.URL.Path, "favicon") ||
			r.URL.Path == "/manifest.json" ||
			r.URL.Path == "/robots.txt" ||
			r.URL.Path == "/sitemap.xml" ||
			strings.HasPrefix(r.URL.Path, "/cdn-cgi/") ||
			strings.HasPrefix(r.URL.Path, "/_next/static/") ||
			strings.HasPrefix(r.URL.Path, "/_next/data/")

		if !isStaticAsset && !isExempt {
			if !isAuthed {
				log.Printf("[AUTH] ❌ No active session found (Path: %s, Host: %s, Cookie-Header: [%s])", r.URL.Path, r.Host, r.Header.Get("Cookie"))
				renderAccessDeniedPage(w)
				return
			}
		}

		// Normalize Cookie & Merge with live Cloudflare cookies in cookieJar
		if resolvedCookie != "" {
			resolvedCookie = parseCookieContent(resolvedCookie)
			resolvedCookie = stripCloudflareCookies(resolvedCookie)
			resolvedCookie = mergeCookiesWithJar(resolvedCookie, accID)
		}

		// Normalize User-Agent
		resolvedUserAgent = strings.TrimSpace(resolvedUserAgent)
		cfgMu.RLock()
		defaultUA := cfg.UserAgent
		cfgMu.RUnlock()
		if resolvedUserAgent == "" {
			resolvedUserAgent = defaultUA
		}

		// Inject cookies into client browser on root path or HTML page requests
		if isAuthed && r.Method == http.MethodGet && (r.URL.Path == "/" || strings.Contains(r.Header.Get("Accept"), "text/html")) {
			if resolvedCookie != "" {
				injectCookiesToBrowser(w, resolvedCookie)
				log.Println("[COOKIE] Injected target domain cookies into client browser session")
			}
		}

		// Pass resolved cookie, UA, and proxy to the reverse proxy Director via request headers or context
		ctx := r.Context()
		ctx = context.WithValue(ctx, publicHostContextKey, publicHost)
		ctx = context.WithValue(ctx, publicSchemeContextKey, publicScheme)
		if resolvedProxy != "" {
			ctx = context.WithValue(ctx, proxyContextKey, resolvedProxy)
		}
		if accID != 0 {
			ctx = context.WithValue(ctx, accountIDContextKey, accID)
		}
		if errC == nil && sessionCookie != nil && sessionCookie.Value != "" {
			ctx = context.WithValue(ctx, sessionTokenContextKey, sessionCookie.Value)
		}
		if panelOn && currentUser == "" {
			if name, err := panelSessionUsername(r); err == nil {
				currentUser = name
			}
		}
		*r = *r.WithContext(ctx)

		r.Header.Set("X-Resolved-Cookie", resolvedCookie)
		r.Header.Set("X-Resolved-UA", resolvedUserAgent)

		// Log proxy details
		if isStaticMode {
			log.Printf("[STATIC-MODE] 🔐 Running in Static Testing Mode! (Proxy: %s)", resolvedProxy)
		} else if accID != 0 {
			log.Printf("[PROXY-LB] 🎯 Route user session '%s' to Account: %s (ID: %d)", currentUser, activeAccountName, accID)
		}

		if panelOn && currentUser != "" && digenBillable(r.Method, r.URL.Path) {
			bodyBytes, _ := io.ReadAll(io.LimitReader(r.Body, 2<<20))
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			r.ContentLength = int64(len(bodyBytes))
			r.Header.Set("Content-Length", fmt.Sprintf("%d", len(bodyBytes)))
			quote := digenQuoteFrom(r.URL.Path, bodyBytes)
			if quote.credits > 0 && !digenCreditsAllow(currentUser, quote.credits) {
				log.Printf("[QUOTA] blocked user=%s need=%d %q", currentUser, quote.credits, quote.action)
				digenRejectLimit(w)
				return
			}
			*r = *r.WithContext(context.WithValue(r.Context(), digenBillKey, digenBill{username: currentUser, quote: quote}))
		}

		reverseProxy.ServeHTTP(w, r)
	})

	serverAddr := ":" + cfg.Port
	log.Printf("📡 [LISTENING] Proxy running on %s (Public: %s://%s)", serverAddr, cfg.PublicScheme, cfg.PublicHost)
	if err := http.ListenAndServe(serverAddr, mux); err != nil {
		log.Fatalf("[FATAL] Server failed: %v", err)
	}
}

// ── CONFIG LOADER ────────────────────────────────────────────────────────────

func loadConfig() Config {
	data, err := os.ReadFile(CONFIG_FILE)
	if err != nil {
		log.Printf("[CONFIG] ⚠️ Read config.json error: %v — using default config", err)
		cfg = Config{
			Port:         "7850",
			TargetURL:    "https://digen.ai",
			PublicHost:   "localhost:7850",
			PublicScheme: "http",
			UserAgent:    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36",
			WebsiteID:    10,
			ToolName:     "Digen",
		}
		return cfg
	}
	cfgMu.Lock()
	defer cfgMu.Unlock()
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("[FATAL] Failed to parse config.json: %v", err)
	}
	if cfg.Port == "" {
		cfg.Port = "7850"
	}
	if cfg.TargetURL == "" {
		cfg.TargetURL = "https://digen.ai"
	}
	if cfg.PublicHost == "" {
		cfg.PublicHost = "localhost:7850"
	}
	if cfg.PublicScheme == "" {
		cfg.PublicScheme = "http"
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"
	}
	if cfg.ToolName == "" {
		cfg.ToolName = "Digen"
	}
	log.Printf("[CONFIG] ✅ Loaded configuration (Target: %s, Port: %s)", cfg.TargetURL, cfg.Port)
	return cfg
}

// ── DATABASE INSERTS ──────────────────────────────────────────────────────────

func initDB() {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=Asia%%2FKolkata",
		cfg.MySQLUser, cfg.MySQLPassword, cfg.MySQLHost, cfg.MySQLPort, cfg.MySQLDB)
	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Printf("[DB] ❌ Database connection failed: %v", err)
		return
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(10 * time.Minute)
	if err := db.Ping(); err != nil {
		log.Printf("[DB] ⚠️ Database ping failed: %v", err)
		return
	}
	log.Printf("[DB] Connected to MySQL successfully! Database: %s ✅", cfg.MySQLDB)
}

func injectCookiesToBrowser(w http.ResponseWriter, cookieStr string) {
	if cookieStr == "" {
		return
	}
	parts := strings.Split(cookieStr, ";")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		idx := strings.IndexByte(p, '=')
		if idx > 0 {
			name := strings.TrimSpace(p[:idx])
			val := strings.TrimSpace(p[idx+1:])
			w.Header().Add("Set-Cookie", fmt.Sprintf("%s=%s; Path=/; Max-Age=31536000", name, val))
		}
	}
}

// ── COOKIE PARSER AND MERGER ──────────────────────────────────────────────────

// parseCookieContent supports raw string or JSON format cookies.
func parseCookieContent(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// GoAuto {"referer"?: "...", "cookies":[...]} — referer is optional.
	if strings.HasPrefix(raw, "{") {
		var wrap struct {
			Cookies json.RawMessage `json:"cookies"`
		}
		if err := json.Unmarshal([]byte(raw), &wrap); err == nil {
			if inner := strings.TrimSpace(string(wrap.Cookies)); strings.HasPrefix(inner, "[") {
				raw = inner
			}
		}
	}
	if !strings.HasPrefix(raw, "[") {
		return raw
	}
	// Try parsing JSON format cookies
	var cookieList []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(raw), &cookieList); err != nil {
		log.Printf("[COOKIE] ⚠️ JSON parse error: %v — falling back to raw string", err)
		return raw
	}
	log.Printf("[COOKIE] Parsed %d cookies from JSON config/file", len(cookieList))
	var pairs []string
	for _, c := range cookieList {
		if c.Name != "" {
			pairs = append(pairs, fmt.Sprintf("%s=%s", c.Name, c.Value))
		}
	}
	return strings.Join(pairs, "; ")
}

func mergeCookiesWithJar(dbCookie string, accID int) string {
	cookieJarMu.RLock()
	defer cookieJarMu.RUnlock()

	cookies := make(map[string]string)
	parts := strings.Split(dbCookie, ";")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		idx := strings.IndexByte(p, '=')
		if idx > 0 {
			name := strings.TrimSpace(p[:idx])
			val := strings.TrimSpace(p[idx+1:])
			cookies[name] = val
		}
	}

	if jar, exists := cookieJar[accID]; exists {
		for k, v := range jar {
			cookies[k] = v
		}
	}

	var merged []string
	for k, v := range cookies {
		merged = append(merged, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(merged, "; ")
}

func updateCookieJar(cookies []string, accID int) {
	cookieJarMu.Lock()
	defer cookieJarMu.Unlock()
	if cookieJar[accID] == nil {
		cookieJar[accID] = make(map[string]string)
	}
	for _, cookieStr := range cookies {
		parts := strings.Split(strings.Split(cookieStr, ";")[0], "=")
		if len(parts) >= 2 {
			name := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(strings.Join(parts[1:], "="))
			if name == "__cf_bm" || name == "_cfuvid" || name == "cf_clearance" {
				cookieJar[accID][name] = val
				log.Printf("[COOKIEJAR] 🔄 Updated live cookie for Account ID %d: %s", accID, name)
			}
		}
	}
}

func stripCloudflareCookies(cookieStr string) string {
	parts := strings.Split(cookieStr, "; ")
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		idx := strings.IndexByte(part, '=')
		if idx > 0 {
			name := strings.TrimSpace(part[:idx])
			lowerName := strings.ToLower(name)
			if lowerName == "__cf_bm" || lowerName == "_cfuvid" || lowerName == "cf_clearance" || lowerName == "__cfruid" || lowerName == "__cf_address" {
				continue
			}
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, "; ")
}

// ── CLIENT HINTS SYNCHRONIZER ────────────────────────────────────────────────

func setClientHintHeaders(req *http.Request, ua string) {
	version := "133"
	platform := `"macOS"`

	if strings.Contains(ua, "Chrome/") {
		parts := strings.Split(ua, "Chrome/")
		if len(parts) > 1 {
			versionParts := strings.Split(parts[1], ".")
			if len(versionParts) > 0 {
				version = versionParts[0]
			}
		}
	}
	if strings.Contains(strings.ToLower(ua), "windows") {
		platform = `"Windows"`
	} else if strings.Contains(strings.ToLower(ua), "linux") {
		platform = `"Linux"`
	} else if strings.Contains(strings.ToLower(ua), "android") {
		platform = `"Android"`
	} else if strings.Contains(strings.ToLower(ua), "iphone") || strings.Contains(strings.ToLower(ua), "ipad") {
		platform = `"iOS"`
	}

	req.Header.Set("Sec-Ch-Ua", fmt.Sprintf(`"Not(A:Brand";v="99", "Google Chrome";v="%s", "Chromium";v="%s"`, version, version))
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	if platform == `"Android"` || platform == `"iOS"` {
		req.Header.Set("Sec-Ch-Ua-Mobile", "?1")
	}
	req.Header.Set("Sec-Ch-Ua-Platform", platform)
}

// ── AMEMBER AUTH HANDLERS ─────────────────────────────────────────────────────

func authHandshakeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if db == nil {
		http.Error(w, "Service Unavailable: Database not connected", http.StatusServiceUnavailable)
		return
	}

	var payload struct {
		Username  string `json:"username"`
		ClientIP  string `json:"client_ip"`
		Timestamp int64  `json:"timestamp"`
		Signature string `json:"signature"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Bad Request: Invalid JSON", http.StatusBadRequest)
		return
	}

	if payload.Username == "" || payload.ClientIP == "" || payload.Signature == "" {
		http.Error(w, "Bad Request: Missing required fields", http.StatusBadRequest)
		return
	}

	// Resolve website ID dynamically from Host header to support multi-domain
	cfgMu.RLock()
	websiteID := cfg.WebsiteID
	cfgMu.RUnlock()
	if db != nil {
		var resolvedWebID int
		errW := db.QueryRow("SELECT id FROM ahrefs_websites WHERE domain = ?", getHostWithoutPort(r.Host)).Scan(&resolvedWebID)
		if errW == nil {
			websiteID = resolvedWebID
		}
	}

	// Validate signature using secret key from database (or config fallback)
	var dbSecretKey string
	if db != nil {
		err := db.QueryRow("SELECT secret_key FROM ahrefs_websites WHERE id = ?", websiteID).Scan(&dbSecretKey)
		if err != nil {
			dbSecretKey = "toolsmandi_digen_secret_xyzjune2026"
		}
	} else {
		dbSecretKey = "toolsmandi_digen_secret_xyzjune2026"
	}

	// Verify HMAC-SHA256 signature
	h := hmac.New(sha256.New, []byte(dbSecretKey))
	h.Write([]byte(fmt.Sprintf("%s:%d", payload.Username, payload.Timestamp)))
	expectedSig := hex.EncodeToString(h.Sum(nil))
	if !hmac.Equal([]byte(payload.Signature), []byte(expectedSig)) {
		log.Printf("[HANDSHAKE] ❌ HMAC signature verification failed for: %s", payload.Username)
		http.Error(w, "Forbidden: Invalid signature", http.StatusForbidden)
		return
	}

	// Generate OTT token
	b := make([]byte, 32)
	rand.Read(b)
	ott := hex.EncodeToString(b)
	expires := time.Now().Add(60 * time.Second)

	_, err := db.Exec(
		"INSERT INTO ahrefs_tokens (token, username, client_ip, expires_at, website_id) VALUES (?,?,?,?,?)",
		ott, payload.Username, payload.ClientIP, expires, websiteID,
	)
	if err != nil {
		log.Printf("[HANDSHAKE] ❌ Database error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	log.Printf("[HANDSHAKE] ✅ Handshake successful: User=%s, websiteID=%d, Token=%s... (Expires=%v)", payload.Username, websiteID, ott[:8], expires.Format(time.RFC3339))

	publicHost := r.Host
	publicScheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		publicScheme = "https"
	} else {
		cfgMu.RLock()
		publicScheme = cfg.PublicScheme
		cfgMu.RUnlock()
	}

	redirectURL := fmt.Sprintf("%s://%s/access?token=%s",
		publicScheme, publicHost, url.QueryEscape(ott))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":       "ok",
		"redirect_url": redirectURL,
	})
}

func accessHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[ACCESS] 📥 /access called: Host=%s, URL=%s, Query=%s", r.Host, r.URL.Path, r.URL.RawQuery)

	username := strings.TrimSpace(r.URL.Query().Get("user"))
	token := strings.TrimSpace(r.URL.Query().Get("token"))

	if token == "" || db == nil {
		log.Printf("[ACCESS] ❌ Rejected: username=%q, token_present=%t, db_nil=%t", username, token != "", db == nil)
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "Access Denied: Invalid credentials.")
		return
	}

	cfgMu.RLock()
	websiteID := cfg.WebsiteID
	cfgMu.RUnlock()
	if db != nil {
		var resolvedWebID int
		hostWithoutPort := getHostWithoutPort(r.Host)
		errW := db.QueryRow("SELECT id FROM ahrefs_websites WHERE domain = ?", hostWithoutPort).Scan(&resolvedWebID)
		if errW == nil {
			websiteID = resolvedWebID
			log.Printf("[ACCESS] 🌐 Resolved websiteID=%d from domain=%s", websiteID, hostWithoutPort)
		} else {
			log.Printf("[ACCESS] ⚠️ Domain '%s' not found in DB, using config websiteID=%d. Error: %v", hostWithoutPort, websiteID, errW)
		}
	}

	var dbUsername, dbClientIP string
	var expiresAt time.Time
	err := db.QueryRow(
		"SELECT username, client_ip, expires_at FROM ahrefs_tokens WHERE token = ? AND website_id = ?",
		token, websiteID,
	).Scan(&dbUsername, &dbClientIP, &expiresAt)

	// If token not found with resolved websiteID, try config websiteID as fallback
	if err != nil {
		cfgMu.RLock()
		fallbackID := cfg.WebsiteID
		cfgMu.RUnlock()
		if fallbackID != websiteID {
			log.Printf("[ACCESS] 🔄 Token not found with websiteID=%d, trying fallback websiteID=%d", websiteID, fallbackID)
			err = db.QueryRow(
				"SELECT username, client_ip, expires_at FROM ahrefs_tokens WHERE token = ? AND website_id = ?",
				token, fallbackID,
			).Scan(&dbUsername, &dbClientIP, &expiresAt)
			if err == nil {
				websiteID = fallbackID
				log.Printf("[ACCESS] ✅ Token found with fallback websiteID=%d", fallbackID)
			}
		}
	}

	// Last resort: try finding the token without website_id filter
	if err != nil {
		log.Printf("[ACCESS] 🔄 Token not found with any websiteID, trying without website_id filter")
		var tokenWebsiteID int
		err = db.QueryRow(
			"SELECT username, client_ip, expires_at, website_id FROM ahrefs_tokens WHERE token = ? ORDER BY created_at DESC LIMIT 1",
			token,
		).Scan(&dbUsername, &dbClientIP, &expiresAt, &tokenWebsiteID)
		if err == nil {
			websiteID = tokenWebsiteID
			log.Printf("[ACCESS] ✅ Token found globally with websiteID=%d", tokenWebsiteID)
		}
	}

	if err != nil || time.Now().After(expiresAt) || (username != "" && dbUsername != username) {
		nowTime := time.Now()
		log.Printf("[ACCESS] ❌ Token validation failed: err=%v, expired=%t (now=%v, expiresAt=%v), username_match=%t (db=%q, req=%q), websiteID=%d",
			err, err == nil && nowTime.After(expiresAt), nowTime.Format(time.RFC3339), expiresAt.Format(time.RFC3339), dbUsername == username, dbUsername, username, websiteID)

		// Dump database diagnostic info to trace if the token exists or has a different website_id
		if db != nil {
			var totalTokens int
			_ = db.QueryRow("SELECT COUNT(*) FROM ahrefs_tokens").Scan(&totalTokens)
			log.Printf("[ACCESS-DIAGNOSTIC] Total tokens currently in DB: %d", totalTokens)

			rows, dbErr := db.Query("SELECT id, token, username, expires_at, website_id FROM ahrefs_tokens ORDER BY created_at DESC LIMIT 5")
			if dbErr == nil {
				defer rows.Close()
				log.Printf("[ACCESS-DIAGNOSTIC] Latest tokens in DB:")
				for rows.Next() {
					var dID, dWebID int
					var dTok, dUser string
					var dExp time.Time
					if scanErr := rows.Scan(&dID, &dTok, &dUser, &dExp, &dWebID); scanErr == nil {
						tokPreview := dTok
						if len(tokPreview) > 8 {
							tokPreview = tokPreview[:8] + "..."
						}
						log.Printf("  -> ID=%d, Token=%s, User=%s, WebsiteID=%d, Expires=%v (now.After=%t)", dID, tokPreview, dUser, dWebID, dExp.Format(time.RFC3339), nowTime.After(dExp))
					}
				}
			} else {
				log.Printf("[ACCESS-DIAGNOSTIC] Failed to query ahrefs_tokens: %v", dbErr)
			}
		}

		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "Access Denied: Expired or invalid token.")
		return
	}

	username = dbUsername

	log.Printf("[ACCESS] ✅ Token validated: User=%s, WebsiteID=%d", username, websiteID)

	// Consume token gracefully: set expiry to 5 seconds from now to allow potential browser double-requests/pre-fetches
	_, _ = db.Exec("UPDATE ahrefs_tokens SET expires_at = ? WHERE token = ?", time.Now().Add(5*time.Second), token)

	// Ensure user exists in local table or create them
	var dbStatus string
	err = db.QueryRow("SELECT status FROM ahrefs_users WHERE username = ? AND website_id = ?", username, websiteID).Scan(&dbStatus)
	if err == sql.ErrNoRows {
		var defCreditLimit int
		if dbErr := db.QueryRow("SELECT COALESCE(default_credit_limit, 50) FROM ahrefs_websites WHERE id = ?", websiteID).Scan(&defCreditLimit); dbErr != nil {
			defCreditLimit = 50
		}
		_, _ = db.Exec("INSERT INTO ahrefs_users (username, website_id, credit_limit) VALUES (?, ?, ?)", username, websiteID, defCreditLimit)
	} else if dbStatus == "suspended" {
		http.Error(w, "Account Suspended. Contact support.", http.StatusForbidden)
		return
	}

	// Generate session token
	sessionBytes := make([]byte, 32)
	rand.Read(sessionBytes)
	sessionToken := hex.EncodeToString(sessionBytes)

	sessionDuration := 30
	var dbSessionDuration int
	if errDb := db.QueryRow("SELECT session_duration FROM ahrefs_websites WHERE id = ?", websiteID).Scan(&dbSessionDuration); errDb == nil && dbSessionDuration > 0 {
		sessionDuration = dbSessionDuration
	}
	sessionExpiry := time.Now().Add(time.Duration(sessionDuration) * time.Minute)

	// Enforce single session
	_, _ = db.Exec("DELETE FROM ahrefs_sessions WHERE username = ? AND website_id = ?", username, websiteID)

	// Auto assign account
	var assignedAccountID sql.NullInt64
	if initialAcc, errAcc := selectActiveAccount(websiteID); errAcc == nil {
		assignedAccountID.Int64 = int64(initialAcc.ID)
		assignedAccountID.Valid = true
		log.Printf("[ACCESS] 🎯 Assigned account: %s (ID: %d) for websiteID=%d", initialAcc.Name, initialAcc.ID, websiteID)
	} else {
		log.Printf("[ACCESS] ⚠️ No active account found for websiteID=%d: %v", websiteID, errAcc)
	}

	_, err = db.Exec(
		"INSERT INTO ahrefs_sessions (session_token, username, client_ip, expires_at, website_id, assigned_account_id) VALUES (?,?,?,?,?,?)",
		sessionToken, username, dbClientIP, sessionExpiry, websiteID, assignedAccountID,
	)
	if err != nil {
		log.Printf("[ACCESS] ❌ Session INSERT failed: %v (websiteID=%d, user=%s)", err, websiteID, username)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Database error: "+err.Error())
		return
	}

	// Log login
	userAgentStr := r.Header.Get("User-Agent")
	_, _ = db.Exec("INSERT INTO ahrefs_login_logs (website_id, username, client_ip, user_agent) VALUES (?, ?, ?, ?)", websiteID, username, dbClientIP, userAgentStr)

	// Detect if connection is over HTTPS
	isSecure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	if !isSecure {
		cfgMu.RLock()
		isSecure = cfg.PublicScheme == "https"
		cfgMu.RUnlock()
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "ct_session",
		Value:    sessionToken,
		Path:     "/",
		Expires:  sessionExpiry,
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
	})

	log.Printf("[ACCESS] ✅ SESSION CREATED: User=%s, WebsiteID=%d, Token=%s, Secure=%t, Expiry=%v, AccountID=%v",
		username, websiteID, sessionToken[:8]+"...", isSecure, sessionExpiry, assignedAccountID)

	http.Redirect(w, r, "/", http.StatusFound)
}

func selectActiveAccount(websiteID int) (Account, error) {
	var acc Account
	if db == nil {
		return acc, fmt.Errorf("database offline")
	}
	query := "SELECT id, name, cookie, user_agent, COALESCE(proxy, '') FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' ORDER BY last_used_at ASC LIMIT 1"
	err := db.QueryRow(query, websiteID).Scan(&acc.ID, &acc.Name, &acc.Cookie, &acc.UserAgent, &acc.Proxy)
	if err != nil {
		return acc, err
	}

	// Update last_used_at to rotate account usage
	_, _ = db.Exec("UPDATE ahrefs_accounts SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?", acc.ID)
	return acc, nil
}

func detectDigenLimitOrFree(path string, statusCode int, body string) bool {
	lowerBody := strings.ToLower(body)

	// 1. Check for explicit plan/subscription checks in JSON responses
	if strings.Contains(path, "user") || strings.Contains(path, "profile") || strings.Contains(path, "subscription") || strings.Contains(path, "billing") || strings.Contains(path, "member") {
		if strings.Contains(lowerBody, `"plan":"free"`) ||
			strings.Contains(lowerBody, `"plan_name":"free"`) ||
			strings.Contains(lowerBody, `"tier":"free"`) ||
			strings.Contains(lowerBody, `"type":"free"`) ||
			strings.Contains(lowerBody, `"plan_type":"free"`) ||
			strings.Contains(lowerBody, `"is_premium":false`) ||
			strings.Contains(lowerBody, `"premium":false`) ||
			strings.Contains(lowerBody, `"plan":"trial"`) ||
			strings.Contains(lowerBody, `"plan_name":"trial"`) {
			log.Printf("[LIMIT-DETECT] 🚫 Free/Trial plan detected in user profile JSON: %s", path)
			return true
		}
	}

	// 2. Check for unauthorized or session expiry in JSON or HTML
	if statusCode == 401 || statusCode == 403 {
		log.Printf("[LIMIT-DETECT] 🚫 Unauthorized/Forbidden status code: %d", statusCode)
		return true
	}

	if strings.Contains(lowerBody, "unauthorized") ||
		strings.Contains(lowerBody, "token expired") ||
		strings.Contains(lowerBody, "invalid token") ||
		strings.Contains(lowerBody, "jwt expired") ||
		strings.Contains(lowerBody, "please log in") ||
		strings.Contains(lowerBody, "session expired") {
		log.Printf("[LIMIT-DETECT] 🚫 Session expired/unauthorized message in body")
		return true
	}

	return false
}

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

func swapActiveAccount(sessionToken string, currentAssignedID int64, websiteID int) (Account, error) {
	var nextAcc Account
	if db == nil {
		return nextAcc, fmt.Errorf("database offline")
	}

	// Strict Cooldown: check if this session already swapped in the last 8 seconds
	sessionLastSwapMu.Lock()
	lastSwapTime, exists := sessionLastSwap[sessionToken]
	if exists && time.Since(lastSwapTime) < 8*time.Second {
		sessionLastSwapMu.Unlock()
		log.Printf("[SWAP] 🛡️ Cooldown active for session %s (swapped %s ago). Skipping rotation trigger.", sessionToken, time.Since(lastSwapTime))
		return nextAcc, fmt.Errorf("swap cooldown active")
	}
	sessionLastSwap[sessionToken] = time.Now()
	sessionLastSwapMu.Unlock()

	var username string
	err := db.QueryRow("SELECT username FROM ahrefs_sessions WHERE session_token = ?", sessionToken).Scan(&username)
	if err != nil {
		return nextAcc, err
	}

	// Check how many active accounts exist for this website ID
	var activeCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM ahrefs_accounts WHERE website_id = ? AND status = 'active'", websiteID).Scan(&activeCount)

	if activeCount <= 1 {
		// Only one active account exists in database!
		errSelectOnly := db.QueryRow("SELECT id, name, cookie, user_agent, COALESCE(proxy, '') FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' LIMIT 1", websiteID).Scan(&nextAcc.ID, &nextAcc.Name, &nextAcc.Cookie, &nextAcc.UserAgent, &nextAcc.Proxy)
		if errSelectOnly != nil {
			return nextAcc, errSelectOnly
		}

		if nextAcc.ID == int(currentAssignedID) {
			log.Printf("[SWAP] 🛡️ Only 1 active account (%s) exists in database. Skipping rotation trigger.", nextAcc.Name)
			return nextAcc, fmt.Errorf("only 1 active account exists and is already active")
		}
	} else {
		// 2+ accounts exist. Fetch next active account that is NOT the current one (to rotate)
		var errSelect error
		if currentAssignedID != 0 {
			errSelect = db.QueryRow("SELECT id, name, cookie, user_agent, COALESCE(proxy, '') FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' AND id != ? ORDER BY last_used_at ASC LIMIT 1", websiteID, currentAssignedID).Scan(&nextAcc.ID, &nextAcc.Name, &nextAcc.Cookie, &nextAcc.UserAgent, &nextAcc.Proxy)
		}
		if currentAssignedID == 0 || errSelect != nil {
			errSelect = db.QueryRow("SELECT id, name, cookie, user_agent, COALESCE(proxy, '') FROM ahrefs_accounts WHERE website_id = ? AND status = 'active' ORDER BY last_used_at ASC LIMIT 1", websiteID).Scan(&nextAcc.ID, &nextAcc.Name, &nextAcc.Cookie, &nextAcc.UserAgent, &nextAcc.Proxy)
		}

		if errSelect != nil {
			return nextAcc, errSelect
		}
	}

	// Update session with new assigned account
	_, _ = db.Exec("UPDATE ahrefs_sessions SET assigned_account_id = ? WHERE session_token = ? AND website_id = ?", nextAcc.ID, sessionToken, websiteID)

	// Update last_used_at to rotate account usage
	if currentAssignedID != 0 {
		_, _ = db.Exec("UPDATE ahrefs_accounts SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?", currentAssignedID)
	}
	_, _ = db.Exec("UPDATE ahrefs_accounts SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?", nextAcc.ID)

	// Clear Cloudflare cookieJar on account switch
	cookieJarMu.Lock()
	delete(cookieJar, int(currentAssignedID))
	cookieJarMu.Unlock()
	log.Printf("[SWAP] 🧹 Cleared cookieJar on account switch (old CF cookies removed for new account %s)", nextAcc.Name)

	return nextAcc, nil
}

func renderAccessDeniedPage(w http.ResponseWriter) {
	name := html.EscapeString(toolDisplayName(loadConfig()))
	writeLightCard(w, http.StatusForbidden, lightCard{
		Title:   "Access Denied",
		Heading: "Access Denied",
		Message: "You cannot open <span class=\"brand\">" + name + "</span> directly. Open it again from your access link.",
		Footer:  "Your session ended or this browser is not authorized",
	})
}

func getHostWithoutPort(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func isStaticPath(path string) bool {
	lowerPath := strings.ToLower(path)
	exts := []string{".js", ".css", ".woff", ".woff2", ".ttf", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".mp4", ".webm", ".webp", ".json", ".map", ".xml", ".txt", ".avif"}
	for _, ext := range exts {
		if strings.HasSuffix(lowerPath, ext) || strings.Contains(lowerPath, ext+"?") {
			return true
		}
	}
	return false
}

// ── PROXY DIALERS AND uTLS CONNECTION CLIENT ──────────────────────────────────

func getProxy() *url.URL {
	proxyMu.Lock()
	defer proxyMu.Unlock()
	info, err := os.Stat(PROXY_FILE)
	if err != nil {
		cfgMu.RLock()
		fallbackProxy := cfg.Proxy
		cfgMu.RUnlock()
		return parseProxyString(fallbackProxy)
	}
	if !info.ModTime().After(proxyModTime) {
		if currentProxy != nil {
			return currentProxy
		}
		cfgMu.RLock()
		fallbackProxy := cfg.Proxy
		cfgMu.RUnlock()
		return parseProxyString(fallbackProxy)
	}
	data, err := os.ReadFile(PROXY_FILE)
	if err != nil {
		cfgMu.RLock()
		fallbackProxy := cfg.Proxy
		cfgMu.RUnlock()
		return parseProxyString(fallbackProxy)
	}
	var proxyStr string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		proxyStr = line
		break
	}
	proxyModTime = info.ModTime()
	if proxyStr == "" {
		cfgMu.RLock()
		fallbackProxy := cfg.Proxy
		cfgMu.RUnlock()
		return parseProxyString(fallbackProxy)
	}
	currentProxy = parseProxyString(proxyStr)
	return currentProxy
}

func parseProxyString(s string) *url.URL {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	// Check if already a valid URL
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err == nil {
			return u
		}
	}

	// Shorthand formats:
	// - IP:PORT:USER:PASS
	// - IP:PORT
	shorthandScheme := "socks5" // Default scheme
	if strings.HasPrefix(s, "http:") {
		shorthandScheme = "http"
		s = strings.TrimPrefix(s, "http:")
	} else if strings.HasPrefix(s, "https:") {
		shorthandScheme = "https"
		s = strings.TrimPrefix(s, "https:")
	} else if strings.HasPrefix(s, "socks5:") {
		shorthandScheme = "socks5"
		s = strings.TrimPrefix(s, "socks5:")
	}

	parts := strings.SplitN(s, ":", 4)
	switch len(parts) {
	case 2:
		raw := fmt.Sprintf("%s://%s:%s", shorthandScheme, parts[0], parts[1])
		u, _ := url.Parse(raw)
		return u
	case 4:
		raw := fmt.Sprintf("%s://%s:%s@%s:%s",
			shorthandScheme,
			url.QueryEscape(parts[2]),
			url.QueryEscape(parts[3]),
			parts[0], parts[1])
		u, _ := url.Parse(raw)
		return u
	}
	return nil
}

func dialThroughProxy(ctx context.Context, targetAddr string, proxyURL *url.URL) (net.Conn, error) {
	switch strings.ToLower(proxyURL.Scheme) {
	case "socks5", "socks5h":
		var auth *proxy.Auth
		if proxyURL.User != nil {
			pass, _ := proxyURL.User.Password()
			auth = &proxy.Auth{User: proxyURL.User.Username(), Password: pass}
		}
		proxyHost := proxyURL.Host
		if !strings.Contains(proxyHost, ":") {
			proxyHost += ":1080"
		}
		baseDialer := &net.Dialer{Timeout: 20 * time.Second}
		socks5Dialer, err := proxy.SOCKS5("tcp", proxyHost, auth, baseDialer)
		if err != nil {
			return nil, fmt.Errorf("socks5 dialer init: %w", err)
		}
		if cd, ok := socks5Dialer.(proxy.ContextDialer); ok {
			return cd.DialContext(ctx, "tcp", targetAddr)
		}
		return socks5Dialer.Dial("tcp", targetAddr)

	case "http", "https":
		proxyHost := proxyURL.Host
		if !strings.Contains(proxyHost, ":") {
			if proxyURL.Scheme == "https" {
				proxyHost += ":443"
			} else {
				proxyHost += ":80"
			}
		}
		conn, err := (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, "tcp", proxyHost)
		if err != nil {
			return nil, fmt.Errorf("connect to HTTP proxy: %w", err)
		}
		connectLine := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", targetAddr, targetAddr)
		if proxyURL.User != nil {
			pass, _ := proxyURL.User.Password()
			creds := base64.StdEncoding.EncodeToString([]byte(proxyURL.User.Username() + ":" + pass))
			connectLine += "Proxy-Authorization: Basic " + creds + "\r\n"
		}
		connectLine += "\r\n"
		if _, err := conn.Write([]byte(connectLine)); err != nil {
			conn.Close()
			return nil, err
		}
		br := bufio.NewReader(conn)
		resp, err := http.ReadResponse(br, nil)
		if err != nil {
			conn.Close()
			return nil, err
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			conn.Close()
			return nil, fmt.Errorf("HTTP proxy CONNECT refused: %s", resp.Status)
		}
		return conn, nil

	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s", proxyURL.Scheme)
	}
}

type uTLSConn struct {
	*utls.UConn
}

func (c *uTLSConn) ConnectionState() tls.ConnectionState {
	cs := c.UConn.ConnectionState()
	return tls.ConnectionState{
		Version:            cs.Version,
		HandshakeComplete:  cs.HandshakeComplete,
		DidResume:          cs.DidResume,
		CipherSuite:        cs.CipherSuite,
		NegotiatedProtocol: cs.NegotiatedProtocol,
		ServerName:         cs.ServerName,
	}
}

func dialChrome(ctx context.Context, addr string) (*uTLSConn, error) {
	host, _, _ := net.SplitHostPort(addr)
	var tcpConn net.Conn
	var err error
	var px *url.URL

	if marked, accountProxy := panelUpstreamProxy(ctx); marked {
		if accountProxy != "" {
			parsed := parseProxyString(accountProxy)
			if parsed == nil {
				return nil, fmt.Errorf("proxy")
			}
			tcpConn, err = dialThroughProxy(ctx, addr, parsed)
			if err != nil {
				return nil, err
			}
		} else {
			tcpConn, err = (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, "tcp4", addr)
			if err != nil {
				return nil, fmt.Errorf("TCP dial failed: %w", err)
			}
		}
	} else {
		if ctxPx, ok := ctx.Value(proxyContextKey).(string); ok && ctxPx != "" {
			px = parseProxyString(ctxPx)
		}
		if px == nil {
			px = getProxy()
		}

		if px != nil {
			tcpConn, err = dialThroughProxy(ctx, addr, px)
			if err != nil {
				log.Printf("[PROXY] ⚠️ Proxy failed (%v) — falling back to direct connection", err)
				tcpConn, err = (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, "tcp4", addr)
				if err != nil {
					return nil, fmt.Errorf("direct dial (after proxy fail): %w", err)
				}
			}
		} else {
			tcpConn, err = (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, "tcp4", addr)
			if err != nil {
				return nil, fmt.Errorf("TCP dial failed: %w", err)
			}
		}
	}

	spec, err := utls.UTLSIdToSpec(utls.HelloChrome_Auto)
	if err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("failed to get uTLS spec: %w", err)
	}

	// Force ALPN to HTTP/1.1 only
	for _, ext := range spec.Extensions {
		if alpn, ok := ext.(*utls.ALPNExtension); ok {
			alpn.AlpnProtocols = []string{"http/1.1"}
		}
	}

	uConn := utls.UClient(tcpConn, &utls.Config{
		ServerName:         host,
		InsecureSkipVerify: true,
	}, utls.HelloCustom)

	if errPreset := uConn.ApplyPreset(&spec); errPreset != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("failed to apply uTLS preset: %w", errPreset)
	}

	if errHandshake := uConn.HandshakeContext(ctx); errHandshake != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("uTLS handshake failed: %w", errHandshake)
	}

	return &uTLSConn{uConn}, nil
}

type roundTripper struct {
	base *http.Transport
}

func (rt *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clean headers right before network transmission
	req.Header.Del("X-Forwarded-For")
	req.Header.Del("X-Real-IP")
	req.Header.Del("X-Forwarded-Proto")
	req.Header.Del("X-Forwarded-Host")
	req.Header.Del("X-Device-Fp")
	req.Header.Del("X-Device-Proof")
	req.Header.Del("X-Panel-Cookie")
	req.Header.Del("X-Panel-Session")
	req.Header.Del("X-Panel-UA")

	return rt.base.RoundTrip(req)
}

func buildChromeHTTPClient() *http.Transport {
	dialTLS := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialChrome(ctx, addr)
	}

	return &http.Transport{
		DialTLSContext:        dialTLS,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   20 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    true,
		ForceAttemptHTTP2:     false,
	}
}
