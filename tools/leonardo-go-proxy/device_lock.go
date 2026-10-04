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
	if cfg.BypassAuth {
		return true
	}
	host := strings.ToLower(cfg.PublicHost)
	return strings.Contains(host, "127.0.0.1") || strings.Contains(host, "localhost")
}

func browserSubresource(r *http.Request) bool {
	dest := strings.ToLower(r.Header.Get("Sec-Fetch-Dest"))
	switch dest {
	case "image", "style", "font", "script", "empty", "worker", "sharedworker", "serviceworker":
		return true
	}
	if strings.Contains(strings.ToLower(r.URL.Path), "service_worker") || strings.HasSuffix(strings.ToLower(r.URL.Path), "/sw.js") {
		return true
	}
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
  var page = "<!DOCTYPE html><html lang=\"en\"><head><meta charset=\"UTF-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1.0\"><title>Access Denied</title><style>*{box-sizing:border-box;margin:0;padding:0}body{min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;background:#eef3f8;color:#0f172a;font-family:system-ui,-apple-system,Segoe UI,sans-serif}.card{width:min(440px,100%);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px 36px;text-align:center}.ring{width:78px;height:78px;margin:0 auto 22px;border-radius:50%;background:conic-gradient(#3b82f6 0 70deg,#e7eef8 70deg 360deg);display:grid;place-items:center}.lock{width:64px;height:64px;border-radius:50%;background:#fff;display:grid;place-items:center;font-size:26px}h1{font-size:28px;line-height:1.2;font-weight:800;letter-spacing:-.03em;margin-bottom:12px}.msg{color:#64748b;font-size:15px;line-height:1.55}.foot{margin-top:18px;color:#94a3b8;font-size:13px}</style></head><body><div class=\"card\"><div class=\"ring\"><div class=\"lock\">🔒</div></div><h1>Access Denied</h1><p class=\"msg\">Open this tool again from your access link.</p><p class=\"foot\">Your session ended or this browser is not authorized</p></div></body></html>";
  try {
    document.open("text/html","replace");
    document.write(page);
    document.close();
  } catch (e) {
    try { document.documentElement.innerHTML = page; } catch (e2) {
      location.replace("/__tm_access_denied");
    }
  }
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
    fetch("/api/device-bind", {
      method: "POST",
      credentials: "same-origin",
      headers: { "X-Device-Fp": "missing", "X-Device-Proof": "missing" }
    }).finally(tmDeny);
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
    if (!navigator.serviceWorker) return;
    navigator.serviceWorker.register("/tm-device-sw.js", { scope: "/" }).then(function () {
      return navigator.serviceWorker.ready;
    }).then(function () {
      if (navigator.serviceWorker.controller) {
        navigator.serviceWorker.controller.postMessage({ fp: fp, proof: proof });
      }
    }).catch(function () {});
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
      return navigator.serviceWorker.register("/tm-device-sw.js", { scope: "/" }).then(function () {
        return navigator.serviceWorker.ready;
      }).then(function () {
        if (navigator.serviceWorker.controller) return dev;
        return new Promise(function (resolve, reject) {
          var timer = setTimeout(function () { reject(new Error("sw")); }, 4000);
          navigator.serviceWorker.addEventListener("controllerchange", function () {
            clearTimeout(timer);
            resolve(dev);
          }, { once: true });
        });
      }).then(function () {
        if (navigator.serviceWorker.controller) {
          navigator.serviceWorker.controller.postMessage({ fp: dev.fp, proof: dev.proof });
        }
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
  return crypto.subtle.digest("SHA-256", new TextEncoder().encode(raw)).then(function (buf) {
    return Array.from(new Uint8Array(buf)).map(function (b) { return b.toString(16).padStart(2, "0"); }).join("");
  });
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
