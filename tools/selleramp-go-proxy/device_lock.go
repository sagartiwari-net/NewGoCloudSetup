package main

import (
	"bytes"
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"
	"time"
)

const deviceProofKey = "tm_device_proof"

func loadPanelSessionToken(token string) (*panelGateSession, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, false
	}
	raw, ok := panelSess.Load(token)
	if ok {
		return raw.(*panelGateSession), true
	}
	restored, restoreErr := restorePanelSession(token)
	if restoreErr != nil {
		return nil, false
	}
	panelSess.Store(token, restored)
	return restored, true
}

const tmSessionStorageKey = "tm_ct_session"

// ctSessionCandidates returns every ct_session value on the request.
// Prefer ?__tm_s= (boot bootstrap) and X-Ct-Session over jar cookies — SellerAmp
// leaves ~60 cookies on this host so new ct_session Set-Cookie is often dropped
// while a dead orphan keeps being sent (enter ok → session not found).
func ctSessionCandidates(r *http.Request) []string {
	seen := map[string]bool{}
	var out []string
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	add(r.URL.Query().Get("__tm_s"))
	add(r.Header.Get("X-Ct-Session"))
	for _, c := range r.Cookies() {
		if c != nil && c.Name == "ct_session" {
			add(c.Value)
		}
	}
	return out
}

// resolveCtSession picks the first candidate that is a live panel session.
func resolveCtSession(r *http.Request) (string, *panelGateSession, bool) {
	var last string
	for _, tok := range ctSessionCandidates(r) {
		last = tok
		if sess, ok := loadPanelSessionToken(tok); ok {
			return tok, sess, true
		}
	}
	return last, nil, false
}

func sessionFromRequest(r *http.Request) (string, *panelGateSession, bool) {
	return resolveCtSession(r)
}

// clearParentDomainCtSessionCookies drops Domain=.gt4rents.com leftovers that
// shadow the host-only session cookie. Never clears the host-only cookie in the
// same response as a new Set-Cookie — browsers may process Max-Age=0 after the
// live value and wipe it (enter ok → session not found with orphan tok=…).
func clearParentDomainCtSessionCookies(w http.ResponseWriter, r *http.Request, cfg Config) {
	secure := cookieSecure(r, cfg)
	for _, domain := range []string{"gt4rents.com", ".gt4rents.com"} {
		http.SetCookie(w, &http.Cookie{
			Name:     "ct_session",
			Value:    "",
			Path:     "/",
			Domain:   domain,
			MaxAge:   -1,
			Expires:  time.Unix(0, 0),
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

// clearStaleCtSessionCookies expires every ct_session scope (host + parent).
// Use only on Access Denied when the cookie is known-dead — not alongside a set.
func clearStaleCtSessionCookies(w http.ResponseWriter, r *http.Request, cfg Config) {
	secure := cookieSecure(r, cfg)
	http.SetCookie(w, &http.Cookie{
		Name: "ct_session", Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(0, 0),
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
	clearParentDomainCtSessionCookies(w, r, cfg)
}

// expireProxyHostJunkCookies clears non-session cookies the browser already sent
// for this host (SellerAmp SPA pollution).
func expireProxyHostJunkCookies(w http.ResponseWriter, r *http.Request, cfg Config) {
	expireProxyHostJunkCookiesN(w, r, cfg, 24)
}

func expireProxyHostJunkCookiesN(w http.ResponseWriter, r *http.Request, cfg Config, maxExpire int) {
	if r == nil {
		return
	}
	if maxExpire <= 0 {
		maxExpire = 24
	}
	secure := cookieSecure(r, cfg)
	seen := map[string]bool{}
	n := 0
	for _, c := range r.Cookies() {
		if c == nil || n >= maxExpire {
			break
		}
		name := strings.TrimSpace(c.Name)
		if name == "" || name == "ct_session" || strings.HasPrefix(name, "tm_device") || seen[name] {
			continue
		}
		seen[name] = true
		n++
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			Expires:  time.Unix(0, 0),
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

// commitPanelSessionCookie best-effort stores ct_session after a live bootstrap.
func commitPanelSessionCookie(w http.ResponseWriter, r *http.Request, cfg Config, sessionToken string) {
	sess, ok := loadPanelSessionToken(sessionToken)
	if !ok {
		return
	}
	sess.mu.Lock()
	exp := sess.expires
	sess.mu.Unlock()
	clearParentDomainCtSessionCookies(w, r, cfg)
	// Free as many jar slots as practical before writing ct_session last.
	expireProxyHostJunkCookiesN(w, r, cfg, 80)
	setCtSessionCookie(w, r, cfg, sessionToken, exp)
}

// isStaticAssetPath is CSS/JS/images/fonts — browser requests these without
// ?__tm_s= or X-Ct-Session, and a full cookie jar often drops ct_session.
func isStaticAssetPath(path string) bool {
	p := strings.ToLower(strings.TrimSpace(path))
	if p == "" {
		return false
	}
	if strings.Contains(p, "favicon") {
		return true
	}
	prefixes := []string{"/assets/", "/images/", "/img/", "/fonts/", "/static/", "/css/", "/js/", "/media/", "/build/"}
	for _, pre := range prefixes {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	exts := []string{".css", ".js", ".mjs", ".map", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".woff", ".woff2", ".ttf", ".eot", ".otf"}
	for _, ext := range exts {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

func bindPanelDevice(sessionToken, fp, proof string) error {
	raw, ok := panelSess.Load(sessionToken)
	if !ok {
		restored, restoreErr := restorePanelSession(sessionToken)
		if restoreErr != nil {
			return fmt.Errorf("session not found")
		}
		panelSess.Store(sessionToken, restored)
		raw = restored
	}
	sess := raw.(*panelGateSession)
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if time.Now().After(sess.expires) {
		panelSess.Delete(sessionToken)
		return fmt.Errorf("session expired")
	}
	if fp == "" || proof == "" {
		return fmt.Errorf("missing proof")
	}
	if fp == "missing" || proof == "missing" {
		// Do not kill the panel session here — page scripts can race during reload.
		return fmt.Errorf("missing client proof")
	}
	if sess.proof == "" {
		sess.fp = fp
		sess.proof = proof
		return nil
	}
	// Proof is the stable device secret. Canvas FP can change between reloads/tabs.
	if sess.proof != proof {
		// Reject this bind only. Killing the session races with Access + open tabs.
		return fmt.Errorf("device mismatch")
	}
	sess.fp = fp
	return nil
}

func browserSubresource(r *http.Request) bool {
	switch strings.ToLower(r.Header.Get("Sec-Fetch-Dest")) {
	case "image", "style", "font", "script":
		return true
	}
	return false
}

func isDocumentNavigation(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	mode := r.Header.Get("Sec-Fetch-Mode")
	dest := r.Header.Get("Sec-Fetch-Dest")
	if mode == "navigate" || dest == "document" || dest == "iframe" {
		return true
	}
	return mode == "" && dest == "" && strings.Contains(r.Header.Get("Accept"), "text/html")
}

// rejectPanelDevice blocks a copied cookie jar. A document request with a missing
// or different proof deletes the session for every profile that holds it.
func rejectPanelDevice(w http.ResponseWriter, r *http.Request, cfg Config) bool {
	if !usesPanelAccountMode(cfg) {
		return false
	}
	token, sess, ok := sessionFromRequest(r)
	if !ok || sess == nil {
		return false
	}
	fp := strings.TrimSpace(r.Header.Get("X-Device-Fp"))
	proof := strings.TrimSpace(r.Header.Get("X-Device-Proof"))
	sess.mu.Lock()
	bound := sess.proof != ""
	storedFp, storedProof := sess.fp, sess.proof
	username := sess.username
	sess.mu.Unlock()
	if !bound {
		return false
	}
	// Proof match is enough — canvas FP often changes between tabs/reloads.
	if proof != "" && proof == storedProof {
		if fp != "" && fp != storedFp {
			sess.mu.Lock()
			sess.fp = fp
			sess.mu.Unlock()
		}
		return false
	}
	if fp == storedFp && proof == storedProof {
		return false
	}
	if browserSubresource(r) {
		return false
	}
	if fp == "" && proof == "" {
		// A normal refresh is a document load and cannot send the device headers.
		// The page script checks this browser's saved proof. Images and files cannot
		// send those headers either, so they are allowed above.
		if isDocumentNavigation(r) {
			return false
		}
		log.Printf("[DEVICE] required path=%s", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error":"device_required","message":"Open this tool again from your access link."}`)
		return true
	}
	panelSess.Delete(token)
	recordCookieShare(cfg, r, token)
	log.Printf("[DEVICE] session killed user=%s missing=%v", username, proof == "")
	if isDocumentNavigation(r) || strings.Contains(r.Header.Get("Accept"), "text/html") {
		renderAccessDeniedPage(w, cfg)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error":"device_mismatch","message":"Open this tool again from your access link."}`)
	}
	return true
}

func renderDeviceGate(w http.ResponseWriter, cfg Config) {
	name := html.EscapeString(toolDisplayName(cfg))
	writeLightCard(w, http.StatusOK, lightCard{
		Title:   "Authenticating",
		Heading: "Authenticating...",
		Message: "You are using <span class=\"brand\">" + name + "</span>. Please wait a moment while we verify your secure access request.",
		Badge:   "Verifying your request...",
		Footer:  "Secure session initialization in progress",
		Spin:    true,
		ExtraScript: `<script>` + deviceSharedJS() + `
(function () {
  var proof = "";
  try { proof = localStorage.getItem("` + deviceProofKey + `") || ""; } catch (e) {}
  if (!proof) {
    fetch("/api/device-bind", {
      method: "POST",
      credentials: "same-origin",
      headers: { "X-Device-Fp": "missing", "X-Device-Proof": "missing" }
    }).finally(function () {
      var title = document.querySelector("h1");
      var msg = document.querySelector(".msg");
      var pill = document.querySelector(".pill");
      var foot = document.querySelector(".foot");
      if (title) title.textContent = "Access Denied";
      if (msg) msg.textContent = "You cannot open this tool from a copied session. Open it again from your access link.";
      if (pill) pill.remove();
      if (foot) foot.textContent = "A direct visit is not allowed";
    });
    return;
  }
  tmFingerprint().then(function (fp) {
    try { sessionStorage.setItem("tm_device_fp", fp); localStorage.setItem("tm_device_fp", fp); } catch (e) {}
    return tmStore(fp, proof).then(function () {
      tmPatchRequests(fp, proof);
      if (!navigator.serviceWorker) {
        window.location.reload();
        return new Promise(function () {});
      }
      return navigator.serviceWorker.register("/tm-device-sw.js", { scope: "/" }).then(function () {
        return navigator.serviceWorker.ready;
      }).then(function () {
        if (navigator.serviceWorker.controller) {
          navigator.serviceWorker.controller.postMessage({ fp: fp, proof: proof });
          window.location.reload();
          return new Promise(function () {});
        }
        return new Promise(function (resolve, reject) {
          var timer = setTimeout(function () { reject(new Error("sw")); }, 4000);
          navigator.serviceWorker.addEventListener("controllerchange", function () {
            clearTimeout(timer);
            if (navigator.serviceWorker.controller) {
              navigator.serviceWorker.controller.postMessage({ fp: fp, proof: proof });
            }
            window.location.reload();
            resolve();
          }, { once: true });
        });
      });
    });
  }).catch(function () {
    var title = document.querySelector("h1");
    var msg = document.querySelector(".msg");
    if (title) title.textContent = "Access Denied";
    if (msg) msg.textContent = "Open this tool again from your access link.";
  });
})();
</script>`,
	})
}

func deviceBindHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if !usesPanelAccountMode(cfg) {
		http.NotFound(w, r)
		return
	}
	token, _, ok := sessionFromRequest(r)
	if !ok {
		log.Printf("[DEVICE] bind: no session cookie/memory (has_ct=%v hdr=%v)", token != "", r.Header.Get("X-Ct-Session") != "")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error":"no_session","message":"Open this tool again from your access link."}`)
		return
	}
	err := bindPanelDevice(token, strings.TrimSpace(r.Header.Get("X-Device-Fp")), strings.TrimSpace(r.Header.Get("X-Device-Proof")))
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		log.Printf("[DEVICE] bind failed: %v", err)
		// Never wipe the live session from /api/device-bind races.
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `{"error":"device_bind_failed","message":%q}`, err.Error())
		return
	}
	// Re-assert cookie so an orphan jar entry is overwritten before goHome.
	if raw, ok := panelSess.Load(token); ok {
		sess := raw.(*panelGateSession)
		sess.mu.Lock()
		exp := sess.expires
		sess.mu.Unlock()
		clearParentDomainCtSessionCookies(w, r, cfg)
		setCtSessionCookie(w, r, cfg, token, exp)
	}
	log.Printf("[DEVICE] proof stored")
	fmt.Fprintf(w, `{"status":"ok"}`)
}


func serveAccessDeniedHTML(w http.ResponseWriter, r *http.Request) {
	writeLightCard(w, http.StatusForbidden, lightCard{
		Title:   "Access Denied",
		Heading: "Access Denied",
		Message: "Open this tool again from your access link.",
		Footer:  "Your session ended or this browser is not authorized",
	})
}

func serveDeviceSW(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Service-Worker-Allowed", "/")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(deviceSWSource))
}

func devicePageScript() string {
	return `<style data-tm-device>html{visibility:hidden !important}</style><script data-tm-device>` + deviceSharedJS() + `
function tmDeny() {
  if (window.__tmDenied) return;
  window.__tmDenied = true;
  if (window.__tmWatch) clearInterval(window.__tmWatch);
  // Never put raw end-style/end-head/end-script sequences in this inline script
  // (comments included) — HTML parsers can treat them as real closers.
  location.replace("/__tm_access_denied");
}
function tmReveal() {
  document.documentElement.style.visibility = "visible";
  var lock = document.querySelector("style[data-tm-device]");
  if (lock) lock.remove();
}
setTimeout(function () { try { tmReveal(); } catch (e) {} }, 2500);
function tmWatch(fp, proof) {
  if (window.__tmWatch) return;
  window.__tmWatch = setInterval(function () {
    fetch("/api/device-bind", {
      method: "POST",
      credentials: "same-origin",
      headers: { "X-Device-Fp": fp, "X-Device-Proof": proof }
    }).then(function (res) {
      // 401 = session gone. 403 bind races must not kick a live user out.
      if (res.status === 401) tmDeny();
    }).catch(function () {});
  }, 2000);
}
(function () {
  try { sessionStorage.removeItem("tm_acct_try"); } catch (e) {}
  var proof = "";
  var fp = "";
  try { proof = localStorage.getItem("` + deviceProofKey + `") || ""; } catch (e) {}
  try { fp = localStorage.getItem("tm_device_fp") || sessionStorage.getItem("tm_device_fp") || ""; } catch (e) {}
  function afterBind(fp, proof) {
    tmReveal();
    tmPatchRequests(fp, proof);
    tmWatch(fp, proof);
    if (navigator.serviceWorker) {
      navigator.serviceWorker.getRegistrations().then(function (regs) {
        regs.forEach(function (r) { r.unregister(); });
      }).catch(function () {});
    }
  }
  // First visit: mint proof (never bind "missing" — that was instant Access Denied).
  if (!proof) {
    tmEnsureProof().then(function (next) {
      proof = next;
      return tmFingerprint().then(function (fp) {
        try { sessionStorage.setItem("tm_device_fp", fp); localStorage.setItem("tm_device_fp", fp); } catch (e) {}
        return tmStore(fp, proof).catch(function () { return null; }).then(function () {
          return fetch("/api/device-bind", {
            method: "POST",
            credentials: "same-origin",
            headers: { "X-Device-Fp": fp, "X-Device-Proof": proof }
          }).then(function () { afterBind(fp, proof); });
        });
      });
    }).catch(function () { tmReveal(); });
    return;
  }
  tmReveal();
  if (fp && proof) { tmPatchRequests(fp, proof); tmWatch(fp, proof); }
  else if (window.fetch) {
    window.__tmOrigFetch = window.fetch;
    window.fetch = function () {
      var self = this;
      var args = arguments;
      return tmFingerprint().then(function (next) {
        try { sessionStorage.setItem("tm_device_fp", next); localStorage.setItem("tm_device_fp", next); } catch (e) {}
        tmPatchRequests(next, proof);
        return window.fetch.apply(self, args);
      });
    };
  }
  tmFingerprint().then(function (fp) {
    try { sessionStorage.setItem("tm_device_fp", fp); localStorage.setItem("tm_device_fp", fp); } catch (e) {}
    return tmStore(fp, proof).catch(function () { return null; }).then(function () {
      return fetch("/api/device-bind", {
        method: "POST",
        credentials: "same-origin",
        headers: { "X-Device-Fp": fp, "X-Device-Proof": proof }
      }).then(function () { return fp; });
    });
  }).then(function (fp) {
    afterBind(fp, proof);
  }).catch(function () { tmReveal(); });
})();
</script>`
}

func deviceBootScript(home, sessionToken, enterNonce string) string {
	_ = enterNonce // kept for signature compatibility; bootstrap uses ?__tm_s=
	return `<script>` + deviceSharedJS() + `
(function () {
  var home = ` + fmt.Sprintf("%q", home) + `;
  var sess = ` + fmt.Sprintf("%q", sessionToken) + `;
  var started = Date.now();
  function goNext() {
    var wait = 400 - (Date.now() - started);
    try {
      if (sess) {
        sessionStorage.setItem("` + tmSessionStorageKey + `", sess);
        sessionStorage.removeItem("tm_ct_retry");
      }
    } catch (e) {}
    // Cookie jar is often full on this host — pass session in the URL once.
    var next = home || "/";
    if (sess) {
      next += (next.indexOf("?") >= 0 ? "&" : "?") + "__tm_s=" + encodeURIComponent(sess);
    }
    setTimeout(function () { window.location.replace(next); }, wait > 0 ? wait : 0);
  }
  function tryBind() {
    return tmEnsureProof().then(function (proof) {
      return tmFingerprint().then(function (fp) {
        try { sessionStorage.setItem("tm_device_fp", fp); localStorage.setItem("tm_device_fp", fp); } catch (e) {}
        return { fp: fp, proof: proof };
      });
    }).then(function (dev) {
      return tmStore(dev.fp, dev.proof).catch(function () { return null; }).then(function () {
        var headers = { "X-Device-Fp": dev.fp, "X-Device-Proof": dev.proof };
        if (sess) headers["X-Ct-Session"] = sess;
        return fetch("/api/device-bind", {
          method: "POST",
          credentials: "same-origin",
          headers: headers
        });
      });
    });
  }
  if (navigator.serviceWorker) {
    navigator.serviceWorker.getRegistrations().then(function (regs) {
      regs.forEach(function (r) { r.unregister(); });
    }).catch(function () {});
  }
  tryBind().catch(function () {}).finally(goNext);
})();
</script>`
}

func deviceSharedJS() string {
	return `
function tmEnsureProof() {
  return new Promise(function (resolve, reject) {
    try {
      var key = "` + deviceProofKey + `";
      var existing = localStorage.getItem(key);
      if (existing) { resolve(existing); return; }
      var bytes = new Uint8Array(32);
      crypto.getRandomValues(bytes);
      var proof = Array.from(bytes).map(function (b) { return b.toString(16).padStart(2, "0"); }).join("");
      localStorage.setItem(key, proof);
      resolve(proof);
    } catch (e) { reject(e); }
  });
}
function tmFingerprint() {
  var canvas = document.createElement("canvas");
  canvas.width = 220;
  canvas.height = 30;
  var g = canvas.getContext("2d");
  var sample = "";
  if (g) {
    g.textBaseline = "top";
    g.font = "14px Arial";
    g.fillStyle = "#f60";
    g.fillRect(0, 0, 220, 30);
    g.fillStyle = "#069";
    g.fillText("tm-fp", 2, 2);
    sample = canvas.toDataURL().slice(-48);
  }
  var zone = "";
  try { zone = Intl.DateTimeFormat().resolvedOptions().timeZone || ""; } catch (e) {}
  var raw = [
    navigator.userAgent || "",
    navigator.platform || "",
    navigator.language || "",
    String(navigator.hardwareConcurrency || 0),
    String(screen.width) + "x" + String(screen.height),
    zone,
    sample
  ].join("|");
  function weakHash(s){var h=0;for(var i=0;i<s.length;i++){h=((h<<5)-h)+s.charCodeAt(i);h|=0;}var out="";for(var j=0;j<8;j++){out+=((h>>> (j*4)) & 15).toString(16);h=(h*1664525+1013904223)|0;}while(out.length<64)out+=out;return out.slice(0,64);}
  if (window.crypto && crypto.subtle && window.isSecureContext) {
    return crypto.subtle.digest("SHA-256", new TextEncoder().encode(raw)).then(function (buf) {
      return Array.from(new Uint8Array(buf)).map(function (b) { return b.toString(16).padStart(2, "0"); }).join("");
    });
  }
  return Promise.resolve(weakHash(raw));
}
function tmOpenDB() {
  return new Promise(function (resolve, reject) {
    var req = indexedDB.open("tm_device", 1);
    req.onupgradeneeded = function () {
      if (!req.result.objectStoreNames.contains("kv")) req.result.createObjectStore("kv");
    };
    req.onsuccess = function () { resolve(req.result); };
    req.onerror = function () { reject(req.error); };
  });
}
function tmStore(fp, proof) {
  return tmOpenDB().then(function (db) {
    return new Promise(function (resolve, reject) {
      var tx = db.transaction("kv", "readwrite");
      tx.objectStore("kv").put(fp, "fp");
      tx.objectStore("kv").put(proof, "proof");
      tx.oncomplete = function () { resolve(); };
      tx.onerror = function () { reject(tx.error); };
    });
  });
}
function tmPatchRequests(fp, proof) {
  if (window.__tmDevicePatched) return;
  window.__tmDevicePatched = true;
  function tmSess() {
    try { return sessionStorage.getItem("` + tmSessionStorageKey + `") || ""; } catch (e) { return ""; }
  }
  var origFetch = window.__tmOrigFetch || window.fetch;
  if (origFetch) {
    window.fetch = function (input, init) {
      var url = typeof input === "string" ? input : (input && input.url) || "";
      var same = false;
      try { same = new URL(url, location.href).origin === location.origin; } catch (e) {}
      if (same) {
        init = init || {};
        var headers = new Headers(init.headers || (input && input.headers) || undefined);
        if (!headers.get("X-Device-Fp")) headers.set("X-Device-Fp", fp);
        if (!headers.get("X-Device-Proof")) headers.set("X-Device-Proof", proof);
        var sess = tmSess();
        if (sess && !headers.get("X-Ct-Session")) headers.set("X-Ct-Session", sess);
        init.headers = headers;
        if (typeof input !== "string") {
          return origFetch.call(this, new Request(input, init));
        }
      }
      return origFetch.call(this, input, init);
    };
  }
  var origOpen = XMLHttpRequest.prototype.open;
  var origSend = XMLHttpRequest.prototype.send;
  XMLHttpRequest.prototype.open = function (method, url) {
    this.__tmURL = url;
    return origOpen.apply(this, arguments);
  };
  XMLHttpRequest.prototype.send = function () {
    try {
      var same = new URL(this.__tmURL, location.href).origin === location.origin;
      if (same) {
        this.setRequestHeader("X-Device-Fp", fp);
        this.setRequestHeader("X-Device-Proof", proof);
        var sess = tmSess();
        if (sess) this.setRequestHeader("X-Ct-Session", sess);
      }
    } catch (e) {}
    return origSend.apply(this, arguments);
  };
  try {
    if (location.search.indexOf("__tm_s=") >= 0 && window.history && history.replaceState) {
      var u = new URL(location.href);
      u.searchParams.delete("__tm_s");
      history.replaceState({}, "", u.pathname + u.search + u.hash);
    }
  } catch (e) {}
}
`
}


func injectDeviceHTML(body []byte) []byte {
	script := []byte(devicePageScript())
	if bytes.Contains(body, []byte("data-tm-device")) {
		return body
	}
	lower := bytes.ToLower(body)
	if h := bytes.Index(lower, []byte("<head")); h >= 0 {
		if gt := bytes.IndexByte(lower[h:], '>'); gt >= 0 {
			at := h + gt + 1
			out := make([]byte, 0, len(body)+len(script))
			out = append(out, body[:at]...)
			out = append(out, script...)
			out = append(out, body[at:]...)
			return out
		}
	}
	if i := bytes.Index(lower, []byte("</head>")); i >= 0 {
		out := make([]byte, 0, len(body)+len(script))
		out = append(out, body[:i]...)
		out = append(out, script...)
		out = append(out, body[i:]...)
		return out
	}
	return append(body, script...)
}

const deviceSWSource = `
var memFp = "";
var memProof = "";
self.addEventListener("install", function () { self.skipWaiting(); });
self.addEventListener("activate", function (event) { event.waitUntil(self.clients.claim()); });
self.addEventListener("message", function (event) {
  var data = event.data || {};
  if (data.fp) memFp = String(data.fp);
  if (data.proof) memProof = String(data.proof);
});
function openDB() {
  return new Promise(function (resolve, reject) {
    var req = indexedDB.open("tm_device", 1);
    req.onupgradeneeded = function () {
      if (!req.result.objectStoreNames.contains("kv")) req.result.createObjectStore("kv");
    };
    req.onsuccess = function () { resolve(req.result); };
    req.onerror = function () { reject(req.error); };
  });
}
function readKey(db, key) {
  return new Promise(function (resolve) {
    try {
      var tx = db.transaction("kv", "readonly");
      var req = tx.objectStore("kv").get(key);
      req.onsuccess = function () { resolve(req.result || ""); };
      req.onerror = function () { resolve(""); };
    } catch (e) { resolve(""); }
  });
}
function stampDevice(request, target) {
  return (async function () {
    var fp = memFp;
    var proof = memProof;
    if (!fp || !proof) {
      try {
        var db = await openDB();
        if (!fp) fp = await readKey(db, "fp");
        if (!proof) proof = await readKey(db, "proof");
        memFp = fp || "";
        memProof = proof || "";
      } catch (e) {}
    }
    var headers = new Headers();
    try {
      request.headers.forEach(function (v, k) {
        try { headers.set(k, v); } catch (e) {}
      });
    } catch (e) {}
    if (fp) headers.set("X-Device-Fp", fp);
    if (proof) headers.set("X-Device-Proof", proof);
    var init = { method: request.method, headers: headers, credentials: "include", redirect: "follow" };
    if (request.method !== "GET" && request.method !== "HEAD") {
      init.body = await request.arrayBuffer();
    }
    return fetch(target, init);
  })();
}
self.addEventListener("fetch", function (event) {
  if (event.request.mode === "navigate") return;
});
`
