package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const extensionTemplateDir = "extension"

func extensionPublicOrigin(r *http.Request, cfg Config) (origin, host string) {
	// Prefer the browser Host so localhost:7853 installs bind to the URL in use.
	host = strings.TrimSpace(r.Host)
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	if host == "" {
		host = strings.TrimSpace(cfg.PublicHost)
	}
	scheme := strings.TrimSpace(cfg.PublicScheme)
	if scheme == "" {
		scheme = "http"
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
	}
	if fwdProto := r.Header.Get("X-Forwarded-Proto"); fwdProto != "" {
		scheme = strings.TrimSpace(strings.Split(fwdProto, ",")[0])
	}
	origin = scheme + "://" + host
	return origin, host
}

func applyExtensionCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Cookie, Authorization")
	w.Header().Set("Vary", "Origin")
}

// sessionsExtHandler — extension authenticate() hits /sessions_ext (and /jssessions).
// Panel mode loads the session's pinned panel.db account; cookie-file / local_test
// falls back to cookie.txt. MySQL ahrefs_* assign is only used without panel_db.
func sessionsExtHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	applyExtensionCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	cookieStr, source, err := resolveExtensionAuthCookies(r, cfg)
	if err != nil {
		code := "not_logged_in"
		status := http.StatusUnauthorized
		if err.Error() == "no_active_account" {
			code = "no_active_account"
			status = http.StatusServiceUnavailable
		}
		extensionAuthError(w, status, code)
		log.Printf("[EXT] /sessions_ext → %s: %v", code, err)
		return
	}

	cmap := parseCookieMap(cookieStr)
	token := cmap["auth_token"]
	if token == "" {
		token = cmap["STYXKEY_auth_token"]
	}
	if token == "" {
		extensionAuthError(w, http.StatusUnauthorized, "not_logged_in")
		log.Printf("[EXT] /sessions_ext → not_logged_in (empty auth_token source=%s)", source)
		return
	}

	payload := map[string]any{
		"success":     true,
		"daily_token": token,
		"user_id":     cmap["userId"],
		"email":       cmap["userEmail"],
	}
	// Plant auth + datadome on the proxy host so follow-up extension API calls
	// (same-origin to public_host) carry cookies — not only Authorization JWT.
	if n := writeBrowserAuthCookies(w, r, cfg, cookieStr); n > 0 {
		log.Printf("[EXT] set %d proxy-host auth cookies source=%s", n, source)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(payload)
	log.Printf("[EXT] /sessions_ext → ok source=%s token_len=%d user=%s", source, len(token), cmap["userId"])
}

func resolveExtensionAuthCookies(r *http.Request, cfg Config) (cookieStr, source string, err error) {
	// Prefer auth cookies already on the proxy host (set when user opened the dashboard).
	if browser := parseCookieMap(r.Header.Get("Cookie")); browser["auth_token"] != "" || browser["STYXKEY_auth_token"] != "" {
		parts := make([]string, 0, 4)
		for _, name := range []string{"auth_token", "STYXKEY_auth_token", "userId", "userEmail"} {
			if v := browser[name]; v != "" {
				parts = append(parts, name+"="+v)
			}
		}
		return strings.Join(parts, "; "), "browser_cookie", nil
	}

	if usesPanelAccountMode(cfg) {
		username, authErr := getAuthenticatedUser(r, cfg)
		if authErr != nil {
			if cfg.LocalTestMode {
				if ck := loadCookiesFromFile(cfg.CookieFile); ck != "" {
					log.Printf("[EXT] panel session missing (%v); local_test fallback cookie.txt", authErr)
					return ck, "cookie_file_fallback", nil
				}
			}
			return "", "", fmt.Errorf("unauthorized: %w", authErr)
		}
		sessionCookie, cErr := r.Cookie("ct_session")
		if cErr != nil || sessionCookie.Value == "" {
			if cfg.LocalTestMode {
				if ck := loadCookiesFromFile(cfg.CookieFile); ck != "" {
					return ck, "cookie_file_fallback", nil
				}
			}
			return "", "", fmt.Errorf("missing session")
		}
		account, accErr := loadPanelSessionAccount(cfg, sessionCookie.Value)
		if accErr != nil {
			return "", "", fmt.Errorf("no_active_account")
		}
		log.Printf("[EXT] panel account '%s' for user=%s", account.Name, username)
		return parseCookieFromDB(account.Cookie), "panel_account", nil
	}

	if usesCookieFileMode(cfg) || cfg.LocalTestMode {
		ck := loadCookiesFromFile(cfg.CookieFile)
		if ck == "" {
			return "", "", fmt.Errorf("cookie file empty")
		}
		return ck, "cookie_file", nil
	}

	username, authErr := getAuthenticatedUser(r, cfg)
	if authErr != nil {
		return "", "", fmt.Errorf("unauthorized: %w", authErr)
	}
	sessionCookie, cErr := r.Cookie("ct_session")
	if cErr != nil || sessionCookie.Value == "" {
		return "", "", fmt.Errorf("missing session")
	}
	account, found := getSessionAssignedAccount(sessionCookie.Value)
	if !found {
		var assignErr error
		account, assignErr = autoAssignNextAccount(sessionCookie.Value)
		if assignErr != nil {
			log.Printf("[EXT] MySQL assign failed for %s: %v", username, assignErr)
			return "", "", fmt.Errorf("no_active_account")
		}
	}
	return parseCookieFromDB(account.Cookie), "mysql_account", nil
}

func extensionAuthError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": false,
		"error":   code,
	})
}

// isauthRedirectHandler — old consultant login gate → our dashboard.
func isauthRedirectHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusFound)
}

func bakeExtension(origin, host string) ([]byte, error) {
	hostNoPort := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostNoPort = h
	}
	replacer := strings.NewReplacer(
		"__JS_PROXY_ORIGIN__", origin,
		"__JS_PROXY_MATCH__", "*://"+hostNoPort+"/*",
	)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err := filepath.WalkDir(extensionTemplateDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			return nil
		}
		rel, err := filepath.Rel(extensionTemplateDir, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".js" || ext == ".json" || ext == ".html" || ext == ".css" {
			raw = []byte(replacer.Replace(string(raw)))
		}
		dst, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		_, err = dst.Write(raw)
		return err
	})
	if err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func extensionZipHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	origin, host := extensionPublicOrigin(r, cfg)
	data, err := bakeExtension(origin, host)
	if err != nil {
		log.Printf("[EXT] package build failed: %v", err)
		http.Error(w, "Failed to build extension package", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="jungle-scout-extension.zip"`)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Header().Set("Cache-Control", "no-store")
	log.Printf("[EXT] served extension for %s (%d bytes)", origin, len(data))
	_, _ = w.Write(data)
}

func extensionInstallPageHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, `<!doctype html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Install Jungle Scout Extension</title>
<style>
*{box-sizing:border-box}body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;background:#f3f6f8;color:#14202b;font-family:system-ui,-apple-system,Segoe UI,sans-serif}
.card{width:100%;max-width:560px;padding:34px;background:#fff;border-radius:18px;box-shadow:0 16px 44px rgba(0,0,0,.09)}
.brand{color:#00a8a8;font-size:14px;font-weight:700;margin-bottom:10px}h1{margin:0 0 10px;font-size:27px}.lead,ol,.note{color:#64717d;line-height:1.6}
.btn{display:block;margin:24px 0;padding:14px 20px;border-radius:10px;background:#00a8a8;color:#fff;text-align:center;text-decoration:none;font-weight:700}
h2{font-size:15px}code,.copy{padding:2px 8px;border-radius:5px;background:#eef1f4;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:13px}
.copy{cursor:pointer;border:1px solid #d7dee6;color:#0f172a;user-select:all}
.copy:hover{background:#e4eaf0}.copy.copied{background:#dcfce7;border-color:#86efac}
.note{font-size:12px}ol{padding-left:20px}li{margin:8px 0}a{color:#0f766e}
</style></head><body><main class="card">
<div class="brand">Jungle Scout</div>
<h1>Install Chrome Extension</h1>
<p class="lead">Download the extension package bound to this Jungle Scout proxy. Log in uses this same host — not third-party member sites.</p>
<a class="btn" href="/extension.zip">Download Extension (.zip)</a>
<h2>Installation steps</h2>
<ol>
<li>Remove any older Jungle Scout custom extension.</li>
<li>Unzip the download.</li>
<li>Open <button type="button" class="copy" id="copy-ext" title="Click to copy">chrome://extensions</button> (click to copy), then paste into the address bar.</li>
<li>Enable Developer mode.</li>
<li>Choose Load unpacked and select the folder.</li>
<li>Open <a href="/">the dashboard</a> once, then click Log in in the extension.</li>
</ol>
<p class="note">Auth verify URL: <code>/sessions_ext</code> on this proxy.</p>
</main>
<script>
(function(){
  var el=document.getElementById('copy-ext');
  if(!el) return;
  el.addEventListener('click', function(){
    var text='chrome://extensions';
    function done(){
      el.classList.add('copied');
      el.textContent='Copied!';
      setTimeout(function(){ el.classList.remove('copied'); el.textContent=text; }, 1200);
    }
    if(navigator.clipboard&&navigator.clipboard.writeText){
      navigator.clipboard.writeText(text).then(done).catch(function(){
        try{ window.prompt('Copy this URL:', text); }catch(e){}
      });
      return;
    }
    try{ window.prompt('Copy this URL:', text); }catch(e){}
  });
})();
</script>
</body></html>`)
}
