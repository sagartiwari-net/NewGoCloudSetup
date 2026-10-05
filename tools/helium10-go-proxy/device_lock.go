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
		// Do not kill the panel session here — page scripts can race during reload.
		return fmt.Errorf("missing client proof")
	}
	if sess.proof == "" {
		sess.fp = fp
		sess.proof = proof
		return nil
	}
	// Proof is the stable device secret. Canvas FP can change between reloads.
	if sess.proof != proof {
		// Reject this bind only. Killing the session here races with Access + open tabs
		// and falsely logs people out (breaks Helium extension login).
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
	// Local bypass_auth: never hide the page behind device-lock (blank white screen).
	if !usesPanelAccountMode(cfg) || cfg.BypassAuth {
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
	// Empty headers: we skip devicePageScript on Helium HTML (SPA blank risk), so
	// most XHR never send X-Device-*. Blocking them 401s the app → /user/signin →
	// infinite account-switch loop. Allow empty headers; mismatch still kills.
	if fp == "" && proof == "" {
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

func isHeliumExtensionPath(path string) bool {
	p := strings.ToLower(strings.Split(path, "?")[0])
	return strings.HasPrefix(p, "/extra-cdn-") ||
		strings.HasPrefix(p, "/cdn-proxy/") ||
		strings.HasPrefix(p, "/cdn-cgi/") ||
		strings.HasPrefix(p, "/extension/") ||
		strings.HasPrefix(p, "/api/v1/") ||
		strings.HasPrefix(p, "/api/v1/chrome-extension") ||
		strings.Contains(p, "chrome-extension") ||
		strings.HasPrefix(p, "/global-config/") ||
		strings.HasPrefix(p, "/prp/") ||
		strings.HasPrefix(p, "/notification/") ||
		strings.HasPrefix(p, "/authhub/") ||
		// Helium SPA XHR used by members UI + extension; empty device headers are normal.
		strings.HasPrefix(p, "/black-box/") ||
		strings.HasPrefix(p, "/cerebro/") ||
		strings.HasPrefix(p, "/magnet/") ||
		strings.HasPrefix(p, "/research-tools/") ||
		strings.HasPrefix(p, "/control-center/")
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
		renderAccessDeniedPage(w, cfg)
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
// Never leave users on a permanent blank page if bind/fingerprint hangs.
setTimeout(function () { try { tmReveal(); } catch (e) {} }, 2500);
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
    // Helium: skip service worker — it races with Access/reloads and kills sessions.
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
      function doBind() {
        return fetch("/api/device-bind", {
          method: "POST",
          credentials: "same-origin",
          headers: { "X-Device-Fp": dev.fp, "X-Device-Proof": dev.proof }
        });
      }
      // Service workers require a secure context. On plain HTTP skip SW and bind directly.
      if (!navigator.serviceWorker || !window.isSecureContext) {
        return doBind();
      }
      return navigator.serviceWorker.register("/tm-device-sw.js", { scope: "/" }).then(function () {
        return navigator.serviceWorker.ready;
      }).then(function () {
        if (navigator.serviceWorker.controller) return dev;
        return new Promise(function (resolve) {
          var timer = setTimeout(function () { resolve(dev); }, 1500);
          navigator.serviceWorker.addEventListener("controllerchange", function () {
            clearTimeout(timer);
            resolve(dev);
          }, { once: true });
        });
      }).then(function () {
        try {
          if (navigator.serviceWorker.controller) {
            navigator.serviceWorker.controller.postMessage({ fp: dev.fp, proof: dev.proof });
          }
        } catch (e) {}
        return doBind();
      }).catch(function () { return doBind(); });
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
