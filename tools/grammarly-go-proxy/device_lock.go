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

func isLocalDevHost(cfg Config) bool {
	// BypassAuth skips device header gate (local cookie.txt / testing).
	// Cookie-share kill still applies when a mismatched proof is sent.
	return cfg.BypassAuth
}

func browserSubresource(r *http.Request) bool {
	dest := strings.ToLower(r.Header.Get("Sec-Fetch-Dest"))
	switch dest {
	case "image", "style", "font", "script", "empty", "worker", "sharedworker", "serviceworker":
		return true
	}
	path := strings.ToLower(r.URL.Path)
	if strings.Contains(path, "service_worker") || strings.HasSuffix(path, "/sw.js") {
		return true
	}
	// fetch/XHR often have empty Dest + cors/same-origin — do not 401 those or
	// Grammarly auth (/auth/v3/user) fails and the SPA refresh-loops via /login.
	mode := strings.ToLower(r.Header.Get("Sec-Fetch-Mode"))
	if dest == "" && (mode == "cors" || mode == "same-origin" || mode == "no-cors") {
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
	if !usesPanelAccountMode(cfg) || isLocalDevHost(cfg) {
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
      // No device SW — reload after bind headers are available via fetch patch.
      window.location.reload();
      return new Promise(function () {});
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
  // Never embed Access Denied HTML here — literal </style></head><body> breaks HTML parsing.
  location.replace("/__tm_access_denied");
}
function tmReveal() {
  // Remove !important lock first — inline visibility without !important cannot win
  // (permanent blank white page on grammarly.gt4rents.com).
  var lock = document.querySelector("style[data-tm-device]");
  if (lock) lock.remove();
  try { document.documentElement.style.setProperty("visibility", "visible", "important"); } catch (e) {}
  try { document.documentElement.style.setProperty("opacity", "1", "important"); } catch (e) {}
  try { if (document.body) document.body.style.setProperty("display", "block", "important"); } catch (e) {}
  try { if (document.body) document.body.style.setProperty("visibility", "visible", "important"); } catch (e) {}
}
// Never leave users on a permanent blank page if bind/fingerprint hangs.
tmReveal();
setTimeout(function () { try { tmReveal(); } catch (e) {} }, 800);
// Keep forcing reveal for a few seconds (SPA can re-hide during boot).
(function () {
  var n = 0;
  var t = setInterval(function () {
    try { tmReveal(); } catch (e) {}
    if (++n > 20) clearInterval(t);
  }, 500);
})();
function tmWatch(fp, proof) {
  if (window.__tmWatch) return;
  window.__tmWatch = setInterval(function () {
    fetch("/api/device-bind", {
      method: "POST",
      credentials: "same-origin",
      headers: { "X-Device-Fp": fp, "X-Device-Proof": proof }
    }).then(function (res) {
      if (!res.ok) tmDeny();
    }).catch(function () {});
  }, 2000);
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
  // Always resolve fp then patch once. Never install a temporary fetch wrapper that
  // re-enters window.fetch after tmPatchRequests (infinite loop → blank SPA).
  var bootFp = fp;
  Promise.resolve(bootFp || tmFingerprint()).then(function (next) {
    bootFp = next || "";
    try { sessionStorage.setItem("tm_device_fp", bootFp); localStorage.setItem("tm_device_fp", bootFp); } catch (e) {}
    return tmStore(bootFp, proof).then(function () {
      return fetch("/api/device-bind", {
        method: "POST",
        credentials: "same-origin",
        headers: { "X-Device-Fp": bootFp, "X-Device-Proof": proof }
      }).then(function () { return bootFp; });
    });
  }).then(function (next) {
    tmPatchRequests(next, proof);
    tmWatch(next, proof);
    // Skip device SW — races with Access/reloads and can blank the SPA.
    if (navigator.serviceWorker) {
      navigator.serviceWorker.getRegistrations().then(function (regs) {
        regs.forEach(function (r) { r.unregister(); });
      }).catch(function () {});
    }
  }).catch(function () { tmReveal(); });
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
  // Never register device SW — it races with Grammarly SPA boots and leaves a blank white page.
  if (navigator.serviceWorker) {
    navigator.serviceWorker.getRegistrations().then(function (regs) {
      regs.forEach(function (r) { r.unregister(); });
    }).catch(function () {});
  }
  tmEnsureProof().then(function (proof) {
    return tmFingerprint().then(function (fp) {
      try { sessionStorage.setItem("tm_device_fp", fp); localStorage.setItem("tm_device_fp", fp); } catch (e) {}
      return { fp: fp, proof: proof };
    });
  }).then(function (dev) {
    return tmStore(dev.fp, dev.proof).then(function () {
      return fetch("/api/device-bind", {
        method: "POST",
        credentials: "same-origin",
        headers: { "X-Device-Fp": dev.fp, "X-Device-Proof": dev.proof }
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
  // Chain CURRENT fetch (URL rewriter). Never jump to window.__tmOrigFetch.
  var origFetch = window.fetch;
  if (origFetch) {
    window.fetch = function (input, init) {
      try {
        if (typeof window.__tmPatchURL === "function") {
          if (typeof input === "string") input = window.__tmPatchURL(input);
          else if (input && typeof input.url === "string") input = new Request(window.__tmPatchURL(input.url), input);
        }
      } catch (e) {}
      var url = typeof input === "string" ? input : (input && input.url) || "";
      var same = false;
      try { same = new URL(url, location.href).origin === location.origin; } catch (e) {}
      if (same) {
        init = init || {};
        var headers = new Headers(init.headers || (input && input.headers) || undefined);
        if (!headers.get("X-Device-Fp")) headers.set("X-Device-Fp", fp);
        if (!headers.get("X-Device-Proof")) headers.set("X-Device-Proof", proof);
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
    try {
      if (typeof window.__tmPatchURL === "function") url = window.__tmPatchURL(url);
    } catch (e) {}
    this.__tmURL = url;
    var args = Array.prototype.slice.call(arguments);
    args[1] = url;
    return origOpen.apply(this, args);
  };
  XMLHttpRequest.prototype.send = function () {
    try {
      var same = new URL(this.__tmURL, location.href).origin === location.origin;
      if (same) {
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
