package main

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"strconv"
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
		restored, restoreErr := restorePanelSession(cookie.Value)
		if restoreErr != nil {
			return cookie.Value, nil, false
		}
		panelSess.Store(cookie.Value, restored)
		return cookie.Value, restored, true
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
	if fp == "" || proof == "" || fp == "missing" || proof == "missing" {
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
	if fp == storedFp && proof == storedProof {
		return false
	}
	if browserSubresource(r) {
		return false
	}
	if fp == "" && proof == "" {
		// App calls after PHPSESSID login often omit these headers. A 401 here
		// looks like the tool failed to open. The HTML page still binds the
		// device; a different proof below still ends the session.
		return false
	}
	panelSess.Delete(token)
	recordCookieShare(cfg, r, token)
	log.Printf("[DEVICE] session killed user=%s", username)
	if isDocumentNavigation(r) || strings.Contains(r.Header.Get("Accept"), "text/html") {
		renderAccessDeniedPage(w, cfg)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error":"device_mismatch","message":"Open this tool again from your access link."}`)
	}
	return true
}

func recordCookieShare(cfg Config, r *http.Request, token string) {
	panelSess.Delete(token)
	db, err := openPanelDB(cfg)
	if err != nil {
		return
	}
	var websiteID int
	var username, ip string
	err = db.QueryRow(`SELECT website_id, username, COALESCE(client_ip, '') FROM live_sessions WHERE session_token=?`, token).Scan(&websiteID, &username, &ip)
	_, _ = db.Exec(`DELETE FROM live_sessions WHERE session_token=?`, token)
	if err != nil {
		return
	}
	path := ""
	ua := ""
	if r != nil {
		path = r.URL.RequestURI()
		ua = r.UserAgent()
		if ip == "" {
			ip = realClientIP(r)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = db.Exec(`INSERT INTO security_events (website_id, username, client_ip, event_type, attempted_url, details, user_agent, created_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		websiteID, username, ip, "cookie_share", path,
		"Copied the session cookie into another browser. Both browsers were signed out.", ua, now)
	log.Printf("[PANEL] cookie share recorded user=%s", username)
}

func deviceBindHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	token, _, ok := sessionFromRequest(r)
	if !ok {
		renderAccessDeniedPage(w, cfg)
		return
	}
	fp := strings.TrimSpace(r.Header.Get("X-Device-Fp"))
	proof := strings.TrimSpace(r.Header.Get("X-Device-Proof"))
	err := bindPanelDevice(token, fp, proof)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
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
	if db, dbErr := openPanelDB(cfg); dbErr == nil {
		_, _ = db.Exec(`UPDATE live_sessions SET fingerprint=? WHERE session_token=?`, fp, token)
	}
	fmt.Fprintf(w, `{"status":"ok"}`)
}

func serveDeviceSW(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Service-Worker-Allowed", "/")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(deviceSWSource))
}

func viewerNameScript(username string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return ""
	}
	return `<script data-tm-viewer-name>(function(){var name=` + strconv.Quote(username) + `;var sel='span.min-w-0.truncate.text-default > span.min-w-0.flex-1.truncate';function apply(){document.querySelectorAll(sel).forEach(function(el){if(el.childElementCount)return;if(el.textContent!==name)el.textContent=name;});}apply();if(document.documentElement){try{new MutationObserver(apply).observe(document.documentElement,{childList:true,subtree:true,characterData:true});}catch(e){}}})();</script>`
}

func injectDeviceHTML(body []byte, username string) []byte {
	script := []byte(devicePageScript() + `<style data-tm-hide-notice>.srf-layout__notification{display:none!important}ul[class*="experiment-sidebar-navigationList"]{display:none!important}</style>` + viewerNameScript(username))
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
	return append(script, body...)
}

func deviceBootScript(home string) string {
	return `<script>` + deviceSharedJS() + `
(function () {
  var home = ` + fmt.Sprintf("%q", home) + `;
  function fail() {
    var title = document.querySelector("h1");
    var msg = document.querySelector(".msg");
    if (title) title.textContent = "Access Denied";
    if (msg) msg.textContent = "This browser could not verify the device. Open the tool again from your access link.";
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
    window.location.replace(home);
  }).catch(function () { fail(); });
})();
</script>`
}

func devicePageScript() string {
	return `<style data-tm-device>html{visibility:hidden !important}</style><script data-tm-device>` + deviceSharedJS() + `
function tmDeny() {
  if (window.__tmDenied) return;
  window.__tmDenied = true;
  location.replace("/__tm_access_denied");
}
function tmReveal() {
  document.documentElement.style.visibility = "visible";
  var lock = document.querySelector("style[data-tm-device]");
  if (lock) lock.remove();
}
(function () {
  if (navigator.serviceWorker) {
    navigator.serviceWorker.register = function () { return Promise.reject(new Error("sw-disabled")); };
    navigator.serviceWorker.getRegistrations().then(function (regs) {
      regs.forEach(function (r) { r.unregister(); });
    }).catch(function () {});
  }
  var proof = "";
  try { proof = localStorage.getItem("` + deviceProofKey + `") || ""; } catch (e) {}
  if (!proof) {
    fetch("/api/device-bind", {
      method: "POST",
      credentials: "same-origin",
      headers: { "X-Device-Fp": "missing", "X-Device-Proof": "missing" }
    }).finally(tmDeny);
    return;
  }
  tmReveal();
  tmFingerprint().then(function (fp) {
    try { sessionStorage.setItem("tm_device_fp", fp); localStorage.setItem("tm_device_fp", fp); } catch (e) {}
    return tmStore(fp, proof).then(function () {
      tmPatchRequests(fp, proof);
      return fetch("/api/device-bind", {
        method: "POST",
        credentials: "same-origin",
        headers: { "X-Device-Fp": fp, "X-Device-Proof": proof }
      });
    });
  }).then(function (res) {
    if (!res.ok) tmDeny();
  }).catch(function () { tmDeny(); });
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
  var raw = [navigator.userAgent || "", navigator.platform || "", navigator.language || "", String(navigator.hardwareConcurrency || 0), String(screen.width) + "x" + String(screen.height), zone, sample].join("|");
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
  var origFetch = window.fetch;
  if (origFetch) {
    window.fetch = function (input, init) {
      var url = typeof input === "string" ? input : (input && input.url) || "";
      var same = false;
      try { same = new URL(url, location.href).origin === location.origin; } catch (e) {}
      if (same && typeof input !== "string" && input && input.headers && !init) {
        try {
          input.headers.set("X-Device-Fp", fp);
          input.headers.set("X-Device-Proof", proof);
        } catch (e) {}
        return origFetch.call(this, input);
      }
      if (same) {
        init = init || {};
        var headers = new Headers(init.headers || undefined);
        if (!headers.get("X-Device-Fp")) headers.set("X-Device-Fp", fp);
        if (!headers.get("X-Device-Proof")) headers.set("X-Device-Proof", proof);
        init.headers = headers;
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

const deviceSWSource = `
self.addEventListener("install", function () { self.skipWaiting(); });
self.addEventListener("activate", function (event) { event.waitUntil(self.clients.claim()); });
self.addEventListener("fetch", function () {});
`
