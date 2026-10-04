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

func sessionFromRequest(r *http.Request) (string, *panelGateSession, bool) {
	cookie, err := r.Cookie("ct_session")
	if err != nil || cookie.Value == "" {
		return "", nil, false
	}
	raw, ok := panelSess.Load(cookie.Value)
	if !ok {
		return cookie.Value, nil, false
	}
	return cookie.Value, raw.(*panelGateSession), true
}

func bindPanelDevice(sessionToken, fp, proof string) error {
	raw, ok := panelSess.Load(sessionToken)
	if !ok {
		return fmt.Errorf("session not found")
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
		panelSess.Delete(sessionToken)
		return fmt.Errorf("device mismatch")
	}
	if sess.proof == "" {
		sess.fp = fp
		sess.proof = proof
		return nil
	}
	if sess.proof != proof || sess.fp != fp {
		panelSess.Delete(sessionToken)
		return fmt.Errorf("device mismatch")
	}
	return nil
}

// clearPanelDeviceBinding lets the same browser re-bind after Claude clears localStorage on logout.
func clearPanelDeviceBinding(sessionToken string) {
	if sessionToken == "" {
		return
	}
	raw, ok := panelSess.Load(sessionToken)
	if !ok {
		return
	}
	sess := raw.(*panelGateSession)
	sess.mu.Lock()
	sess.fp = ""
	sess.proof = ""
	sess.mu.Unlock()
}

func browserSubresource(r *http.Request) bool {
	switch strings.ToLower(r.Header.Get("Sec-Fetch-Dest")) {
	case "image", "style", "font", "script", "audio", "video", "empty":
		return true
	}
	path := strings.ToLower(r.URL.Path)
	if strings.HasPrefix(path, "/cdn-cgi/") ||
		strings.HasPrefix(path, "/edge-api/") ||
		path == "/favicon.ico" ||
		strings.HasSuffix(path, ".js") ||
		strings.HasSuffix(path, ".css") ||
		strings.HasSuffix(path, ".woff2") ||
		strings.HasSuffix(path, ".ico") ||
		strings.HasSuffix(path, ".png") ||
		strings.HasSuffix(path, ".svg") ||
		strings.HasSuffix(path, ".webp") {
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
	if strings.EqualFold(strings.TrimSpace(cfg.PublicScheme), "http") {
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
	if fp == storedFp && proof == storedProof {
		return false
	}
	if browserSubresource(r) || (fp == "" && proof == "") {
		return false
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
      var clearSW = navigator.serviceWorker
        ? navigator.serviceWorker.getRegistrations().then(function (regs) {
            return Promise.all(regs.map(function (r) { return r.unregister(); }));
          })
        : Promise.resolve();
      return clearSW.then(function () {
        window.location.reload();
        return new Promise(function () {});
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
		renderAccessDeniedPage(w, cfg)
		return
	}
	err := bindPanelDevice(token, strings.TrimSpace(r.Header.Get("X-Device-Fp")), strings.TrimSpace(r.Header.Get("X-Device-Proof")))
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		log.Printf("[DEVICE] bind failed: %v", err)
		if err.Error() == "device mismatch" {
			recordCookieShare(cfg, r, token)
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":"device_mismatch","message":"Open this tool again from your access link."}`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `{"error":"device_bind_failed"}`)
		return
	}
	// Avoid log spam from tmWatch re-bind every 2s.
	if r.Header.Get("X-Device-Quiet") != "1" {
		log.Printf("[DEVICE] proof stored")
	}
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
	return `<style data-tm-device></style><script data-tm-device>` + deviceSharedJS() + `
function tmDeny() {
  if (window.__tmDenied) return;
  window.__tmDenied = true;
  if (window.__tmWatch) clearInterval(window.__tmWatch);
  // DOM rebuild only — never document.write/innerHTML a full HTML string (leaks script as text).
  try {
    var html = document.documentElement;
    html.style.cssText = "visibility:visible;background:#eef3f8;margin:0";
    while (html.firstChild) html.removeChild(html.firstChild);
    var body = document.createElement("body");
    body.style.cssText = "min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;margin:0;background:#eef3f8;color:#0f172a;font-family:system-ui,sans-serif";
    var card = document.createElement("div");
    card.style.cssText = "width:min(440px,100%);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px;text-align:center";
    var h1 = document.createElement("h1");
    h1.textContent = "Access Denied";
    h1.style.cssText = "font-size:28px;margin:0 0 12px;font-weight:800";
    var msg = document.createElement("p");
    msg.textContent = "Open this tool again from your access link.";
    msg.style.cssText = "color:#64748b;font-size:15px;margin:0";
    var foot = document.createElement("p");
    foot.textContent = "Your session ended or this browser is not authorized";
    foot.style.cssText = "margin-top:18px;color:#94a3b8;font-size:13px";
    card.appendChild(h1); card.appendChild(msg); card.appendChild(foot);
    body.appendChild(card); html.appendChild(body);
  } catch (e) {
    location.replace("/__tm_access_denied");
  }
}
function tmReveal() {
  document.documentElement.style.visibility = "visible";
  var lock = document.querySelector("style[data-tm-device]");
  if (lock) lock.remove();
}
function tmWatch(fp, proof) {
  if (window.__tmWatch) return;
  window.__tmWatch = setInterval(function () {
    fetch("/api/device-bind", {
      method: "POST",
      credentials: "same-origin",
      headers: { "X-Device-Fp": fp, "X-Device-Proof": proof, "X-Device-Quiet": "1" }
    }).then(function (res) {
      // Network blips / CF challenge must not kill a live Claude session.
      if (res && (res.status === 401 || res.status === 403)) tmDeny();
    }).catch(function () {});
  }, 15000);
}
(function () {
  try { sessionStorage.removeItem("tm_acct_try"); } catch (e) {}
  var proof = "";
  var fp = "";
  try { proof = localStorage.getItem("` + deviceProofKey + `") || ""; } catch (e) {}
  try { fp = localStorage.getItem("tm_device_fp") || sessionStorage.getItem("tm_device_fp") || ""; } catch (e) {}
  if (!proof) {
    tmEnsureProof().then(function (next) {
      proof = next;
      return tmFingerprint().then(function (fp) {
        try { sessionStorage.setItem("tm_device_fp", fp); localStorage.setItem("tm_device_fp", fp); } catch (e) {}
        return tmStore(fp, proof).then(function () {
          return fetch("/api/device-bind", {
            method: "POST",
            credentials: "same-origin",
            headers: { "X-Device-Fp": fp, "X-Device-Proof": proof }
          }).then(function (res) {
            if (!res.ok) { tmDeny(); return; }
            tmReveal();
            tmPatchRequests(fp, proof);
            tmWatch(fp, proof);
          });
        });
      });
    }).catch(tmDeny);
    return;
  }
  tmReveal();
  if (!window.__tmOrigFetch) window.__tmOrigFetch = window.fetch;
  if (fp && proof) { tmPatchRequests(fp, proof); tmWatch(fp, proof); }
  tmFingerprint().then(function (fp) {
    try { sessionStorage.setItem("tm_device_fp", fp); localStorage.setItem("tm_device_fp", fp); } catch (e) {}
    return tmStore(fp, proof).then(function () {
      return fetch("/api/device-bind", {
        method: "POST",
        credentials: "same-origin",
        headers: { "X-Device-Fp": fp, "X-Device-Proof": proof }
      }).then(function () { return fp; });
    });
  }).then(function (fp) {
    tmPatchRequests(fp, proof);
    tmWatch(fp, proof);
    if (navigator.serviceWorker) {
      navigator.serviceWorker.getRegistrations().then(function (regs) {
        regs.forEach(function (r) { r.unregister(); });
      }).catch(function () {});
    }
  }).catch(function () {});
})();
</script>`
}

func deviceBootScript(home string) string {
	return `<script>` + deviceSharedJS() + `
(function () {
  var home = ` + fmt.Sprintf("%q", home) + `;
  var started = Date.now();
  function fail() {
    var title = document.querySelector("h1");
    var msg = document.querySelector(".msg");
    var pill = document.querySelector(".pill");
    if (title) title.textContent = "Access Denied";
    if (msg) msg.textContent = "This browser could not verify the device. Open the tool again from your access link.";
    if (pill) pill.remove();
  }
  tmEnsureProof().then(function (proof) {
    return tmFingerprint().then(function (fp) {
      try { sessionStorage.setItem("tm_device_fp", fp); localStorage.setItem("tm_device_fp", fp); } catch (e) {}
      return { fp: fp, proof: proof };
    });
  }).then(function (dev) {
    return tmStore(dev.fp, dev.proof).then(function () {
      var clearSW = navigator.serviceWorker
        ? navigator.serviceWorker.getRegistrations().then(function (regs) {
            return Promise.all(regs.map(function (r) { return r.unregister(); }));
          })
        : Promise.resolve();
      return clearSW.then(function () {
        return fetch("/api/device-bind", {
          method: "POST",
          credentials: "same-origin",
          headers: { "X-Device-Fp": dev.fp, "X-Device-Proof": dev.proof }
        });
      });
    });
  }).then(function (res) {
    if (!res || !res.ok) throw new Error("bind");
    var wait = 400 - (Date.now() - started);
    return new Promise(function (resolve) { setTimeout(resolve, wait > 0 ? wait : 0); });
  }).then(function () {
    window.location.replace(home);
  }).catch(function () { fail(); });
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
  // Wrap the current fetch (including proxyUrl). Calling a saved native
  // fetch skips URL rewriting and Claude uploads go cross-origin.
  var prevFetch = window.fetch;
  window.fetch = function (input, init) {
    try {
      var url = typeof input === "string" ? input : (input && input.url) || "";
      var same = false;
      try { same = new URL(url, location.href).origin === location.origin; } catch (e0) {}
      if (same) {
        if (input instanceof Request && (init === undefined || init === null)) {
          var reqHeaders = new Headers(input.headers);
          if (!reqHeaders.has("X-Device-Fp")) reqHeaders.set("X-Device-Fp", fp);
          if (!reqHeaders.has("X-Device-Proof")) reqHeaders.set("X-Device-Proof", proof);
          try { input = new Request(input, { headers: reqHeaders }); } catch (e1) {}
        } else {
          init = init ? Object.assign({}, init) : {};
          var headers = new Headers(init.headers || (input && input.headers) || undefined);
          if (!headers.has("X-Device-Fp")) headers.set("X-Device-Fp", fp);
          if (!headers.has("X-Device-Proof")) headers.set("X-Device-Proof", proof);
          init.headers = headers;
        }
      }
    } catch (e) {}
    return prevFetch.call(this, input, init);
  };
  var origOpen = XMLHttpRequest.prototype.open;
  var origSend = XMLHttpRequest.prototype.send;
  XMLHttpRequest.prototype.open = function (method, url) {
    this.__tmURL = url;
    return origOpen.apply(this, arguments);
  };
  XMLHttpRequest.prototype.send = function (body) {
    try {
      var same = new URL(this.__tmURL || "", location.href).origin === location.origin;
      if (same && this.readyState === 1) {
        this.setRequestHeader("X-Device-Fp", fp);
        this.setRequestHeader("X-Device-Proof", proof);
      }
    } catch (e) {}
    return origSend.apply(this, arguments);
  };
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
