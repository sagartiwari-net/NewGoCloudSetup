package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// rewriteH10ChromeStoreURLs replaces every official Helium10 Chrome Web Store
// URL (plain + JSON-escaped) with our /ext-install page.
func rewriteH10ChromeStoreURLs(body []byte, publicBase string) []byte {
	if len(body) == 0 {
		return body
	}
	ext := strings.TrimRight(publicBase, "/") + "/ext-install"
	id := h10ChromeWebStoreExtID
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`https?://chromewebstore\.google\.com/detail/(?:[^"'\\\s/]+/)?` + id + `[^"'\\\s]*`),
		regexp.MustCompile(`https?://chrome\.google\.com/webstore/detail/(?:[^"'\\\s/]+/)?` + id + `[^"'\\\s]*`),
		regexp.MustCompile(`https?:\\/\\/chromewebstore\.google\.com\\/detail\\/(?:[^"'\\\s/]+\\/)?` + id + `[^"'\\\s]*`),
		regexp.MustCompile(`https?:\\/\\/chrome\.google\.com\\/webstore\\/detail\\/(?:[^"'\\\s/]+\\/)?` + id + `[^"'\\\s]*`),
	}
	escapedExt := strings.ReplaceAll(ext, "/", `\/`)
	for i, re := range patterns {
		repl := ext
		if i >= 2 {
			repl = escapedExt
		}
		body = re.ReplaceAll(body, []byte(repl))
	}
	return body
}

// h10ExtInstallGuardScript is a tiny standalone injector (no fmt.Sprintf) so a
// broken patcher cannot leave Chrome Web Store "Install Now" links live.
func h10ExtInstallGuardScript() string {
	// Keep this light: scanning every <a>.textContent froze Helium's dashboard
	// (context canceled on API posts + infinite skeleton load).
	return `<script data-tm-h10-ext="1">
(function(){
  if (window.__tmH10ExtGuard) return;
  window.__tmH10ExtGuard = true;
  var ID = "njmehopjdpcckochcggncklnlmikcbnb";
  var DEST = "/ext-install";
  var scanning = false;
  function isStore(u){
    if (!u || typeof u !== "string") return false;
    if (u.indexOf(DEST) !== -1) return false;
    if (u.indexOf(ID) !== -1) return true;
    return /chromewebstore\.google\.com|chrome\.google\.com\/webstore/i.test(u) && /helium/i.test(u);
  }
  function fix(a){
    if (!a) return;
    try {
      var cur = a.getAttribute("href") || "";
      if (cur === DEST || a.href.indexOf(DEST) !== -1) {
        if (a.getAttribute("target") === "_blank") a.removeAttribute("target");
        return;
      }
      a.setAttribute("href", DEST);
      a.removeAttribute("target");
      a.setAttribute("rel", "noopener");
    } catch (e) {}
  }
  function scan(){
    if (scanning) return;
    scanning = true;
    try {
      document.querySelectorAll(
        'a[href*="chromewebstore.google.com"],a[href*="chrome.google.com/webstore"],a[href*="'+ID+'"]'
      ).forEach(function(a){
        var h = a.getAttribute("href") || "";
        if (isStore(h) || isStore(a.href)) fix(a);
      });
    } catch (e) {}
    scanning = false;
  }
  function onClick(e){
    var t = e.target;
    if (!t || !t.closest) return;
    var a = t.closest("a");
    if (!a) return;
    var h = a.getAttribute("href") || a.href || "";
    if (!isStore(h) && !isStore(a.href)) return;
    e.preventDefault();
    e.stopPropagation();
    if (e.stopImmediatePropagation) e.stopImmediatePropagation();
    fix(a);
    window.location.href = DEST;
  }
  document.addEventListener("click", onClick, true);
  var _open = window.open;
  window.open = function(url){
    if (isStore(String(url || ""))) url = DEST;
    return _open.apply(this, [url].concat([].slice.call(arguments, 1)));
  };
  scan();
  setInterval(scan, 2000);
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", scan);
  try {
    new MutationObserver(function(muts){
      for (var i = 0; i < muts.length; i++) {
        var m = muts[i];
        if (m.type === "attributes" && m.attributeName === "href" && m.target && m.target.tagName === "A") {
          var a = m.target;
          var h = a.getAttribute("href") || "";
          if (isStore(h) || isStore(a.href)) fix(a);
          continue;
        }
        if (m.addedNodes && m.addedNodes.length) { scan(); return; }
      }
    }).observe(document.documentElement, { childList: true, subtree: true, attributes: true, attributeFilter: ["href"] });
  } catch (e) {}
})();
</script>`
}

func stripMetaCSP(body []byte) []byte {
	re := regexp.MustCompile(`(?is)<meta[^>]+http-equiv\s*=\s*["']?Content-Security-Policy["']?[^>]*>`)
	return re.ReplaceAll(body, nil)
}

func injectIntoHTMLHead(body []byte, snippet string) []byte {
	if len(body) == 0 || snippet == "" {
		return body
	}
	if bytes.Contains(body, []byte(`data-tm-h10-ext="1"`)) && strings.Contains(snippet, `data-tm-h10-ext`) {
		return body
	}
	lower := bytes.ToLower(body)
	if h := bytes.Index(lower, []byte("<head")); h >= 0 {
		if gt := bytes.IndexByte(lower[h:], '>'); gt >= 0 {
			at := h + gt + 1
			out := make([]byte, 0, len(body)+len(snippet))
			out = append(out, body[:at]...)
			out = append(out, snippet...)
			out = append(out, body[at:]...)
			return out
		}
	}
	if i := bytes.Index(lower, []byte("</head>")); i >= 0 {
		out := make([]byte, 0, len(body)+len(snippet))
		out = append(out, body[:i]...)
		out = append(out, snippet...)
		out = append(out, body[i:]...)
		return out
	}
	return append([]byte(snippet), body...)
}

const extensionTemplateDir = "extension"

func resolveExtensionDir() (string, error) {
	candidates := []string{extensionTemplateDir}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), extensionTemplateDir))
		// Go binaries are sometimes in a temp dir when run via `go run`.
		if eval, err2 := filepath.EvalSymlinks(exe); err2 == nil {
			candidates = append(candidates, filepath.Join(filepath.Dir(eval), extensionTemplateDir))
		}
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, extensionTemplateDir))
	}
	seen := map[string]bool{}
	for _, dir := range candidates {
		dir = filepath.Clean(dir)
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		st, err := os.Stat(dir)
		if err == nil && st.IsDir() {
			return dir, nil
		}
	}
	return "", fmt.Errorf("extension template not found (looked next to binary and cwd)")
}

func publicOriginFromRequest(r *http.Request, cfg Config) (origin, hostOnly string) {
	hostWithPort := strings.TrimSpace(cfg.PublicHost)
	if hostWithPort == "" {
		hostWithPort = r.Host
		if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
			hostWithPort = strings.Split(fwd, ",")[0]
			hostWithPort = strings.TrimSpace(hostWithPort)
		}
	}
	hostOnly = hostWithPort
	if h, _, err := net.SplitHostPort(hostWithPort); err == nil {
		hostOnly = h
	} else if strings.HasPrefix(hostWithPort, "[") {
		// rare IPv6 without port
		hostOnly = strings.Trim(hostWithPort, "[]")
	}
	scheme := strings.TrimSpace(cfg.PublicScheme)
	if scheme == "" {
		scheme = "https"
		if r.TLS == nil && !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "http"
		}
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	// Keep port in origin (e.g. http://127.0.0.1:5201). Stripping it breaks the
	// extension, which then opens http://127.0.0.1/user/signup with no listener.
	origin = scheme + "://" + hostWithPort
	return origin, hostOnly
}

func bakeExtensionBytes(origin, hostOnly string) ([]byte, error) {
	extDir, err := resolveExtensionDir()
	if err != nil {
		return nil, err
	}
	hostEsc := strings.ReplaceAll(hostOnly, ".", `\.`)
	replacer := strings.NewReplacer(
		"__H10_PROXY_ORIGIN__", origin,
		"__H10_PROXY_HOST__", hostOnly,
		"__H10_PROXY_HOST_ESC__", hostEsc,
		// Never leave CN / production Helium hosts in the packed extension.
		"https://members.h10.com.cn", origin,
		"http://members.h10.com.cn", origin,
		"https://members.helium10.com", origin,
		"http://members.helium10.com", origin,
		"dev.h10.com.cn", hostOnly,
		"h10.com.cn", hostOnly,
	)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err = filepath.WalkDir(extDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(extDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, ".") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name := filepath.Base(path)
		ext := strings.ToLower(filepath.Ext(name))
		content := raw
		if ext == ".js" || ext == ".html" || ext == ".json" || ext == ".css" || ext == ".txt" {
			content = []byte(replacer.Replace(string(raw)))
		}
		w, err := zw.Create(rel)
		if err != nil {
			return err
		}
		_, err = w.Write(content)
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

func extensionInstallPageHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	origin, _ := publicOriginFromRequest(r, cfg)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1.0">
<title>Install Helium 10 Extension</title>
<link href="https://fonts.googleapis.com/css2?family=DM+Sans:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:'DM Sans',system-ui,sans-serif;background:#eef1f6;color:#0f172a;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px}
.card{width:100%%;max-width:560px;background:#fff;border-radius:24px;box-shadow:0 18px 50px rgba(15,23,42,.08);padding:36px 32px}
.badge{display:inline-block;padding:6px 12px;border-radius:999px;background:#eef2ff;color:#4338ca;font-size:11px;font-weight:700;letter-spacing:.06em;text-transform:uppercase;margin-bottom:14px}
h1{font-size:28px;font-weight:700;letter-spacing:-.02em;margin-bottom:10px}
.lead{color:#64748b;font-size:15px;line-height:1.6;margin-bottom:16px}
.warn{background:#fff7ed;border:1px solid #fdba74;color:#9a3412;border-radius:14px;padding:12px 14px;font-size:13px;line-height:1.5;margin-bottom:18px}
.origin{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px;background:#f1f5f9;padding:8px 10px;border-radius:8px;color:#0f172a;margin-bottom:18px;word-break:break-all}
.btn{display:inline-flex;align-items:center;justify-content:center;gap:8px;width:100%%;padding:14px 18px;border:0;border-radius:14px;background:#0f172a;color:#fff;font-size:15px;font-weight:600;text-decoration:none;cursor:pointer}
.btn:hover{background:#1e293b}
.steps{margin-top:28px;border-top:1px solid #eef2f7;padding-top:20px}
.steps h2{font-size:14px;font-weight:700;margin-bottom:12px;color:#0f172a}
.steps ol{padding-left:18px;color:#64748b;font-size:14px;line-height:1.7}
.steps li{margin-bottom:8px}
.steps code{background:#f1f5f9;padding:1px 6px;border-radius:6px;font-size:12px;color:#334155}
.note{margin-top:18px;font-size:12px;color:#94a3b8;line-height:1.5}
</style>
</head>
<body>
<div class="card">
  <div class="badge">Chrome Extension</div>
  <h1>Install Helium 10</h1>
  <p class="lead">Download a fresh zip every time. An old install opens <code>http://127.0.0.1/user/signin</code> without the port and fails.</p>
  <div class="warn"><strong>Important:</strong> Remove any old Helium extension first, then load this new zip. Do not reuse a previously unzipped folder.</div>
  <div class="origin">This zip is baked for: %s</div>
  <a class="btn" href="/extension.zip">Download Extension (.zip)</a>
  <div class="steps">
    <h2>Install steps</h2>
    <ol>
      <li>Open <code>chrome://extensions</code> and <strong>Remove</strong> the old Helium extension.</li>
      <li>Download and unzip this new file (delete any old unzip folder).</li>
      <li>Enable <strong>Developer mode</strong> (top right).</li>
      <li>Click <strong>Load unpacked</strong> and select the new unzipped folder.</li>
      <li>Open Helium from Panel Access first, then use the extension login button.</li>
    </ol>
  </div>
  <p class="note">Login must open %s/user/signin — if you see 127.0.0.1 without :5201, the old extension is still installed.</p>
</div>
</body>
</html>`, origin, origin)
}

func extensionZipHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	origin, hostOnly := publicOriginFromRequest(r, cfg)
	zipBytes, err := bakeExtensionBytes(origin, hostOnly)
	if err != nil {
		log.Printf("[EXT] ❌ bake failed origin=%s: %v", origin, err)
		http.Error(w, "Failed to build extension package", http.StatusInternalServerError)
		return
	}
	filename := "helium10-extension.zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipBytes)))
	w.Header().Set("Cache-Control", "no-store")
	log.Printf("[EXT] ✅ served extension.zip for origin=%s (%d bytes)", origin, len(zipBytes))
	_, _ = w.Write(zipBytes)
}
