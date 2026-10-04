package main

import (
	"fmt"
	"net/http"
)

const deviceProofKey = "tm_device_proof"

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
  var html = document.documentElement;
  html.style.cssText = "visibility:visible;background:#eef3f8;margin:0";
  while (html.firstChild) html.removeChild(html.firstChild);
  var body = document.createElement("body");
  body.style.cssText = "min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;margin:0;background:#eef3f8;color:#0f172a;font-family:system-ui,-apple-system,Segoe UI,sans-serif";
  var card = document.createElement("div");
  card.style.cssText = "width:min(440px,100%);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px 36px;text-align:center";
  var ring = document.createElement("div");
  ring.style.cssText = "width:78px;height:78px;margin:0 auto 22px;border-radius:50%;background:conic-gradient(#3b82f6 0 70deg,#e7eef8 70deg 360deg);display:grid;place-items:center";
  var lock = document.createElement("div");
  lock.textContent = "\uD83D\uDD12";
  lock.style.cssText = "width:64px;height:64px;border-radius:50%;background:#fff;display:grid;place-items:center;font-size:26px";
  ring.appendChild(lock);
  var title = document.createElement("h1");
  title.textContent = "Access Denied";
  title.style.cssText = "display:block;font-size:28px;line-height:1.2;font-weight:800;letter-spacing:-.03em;margin:0 0 12px";
  var msg = document.createElement("p");
  msg.style.cssText = "display:block;color:#64748b;font-size:15px;line-height:1.55;margin:0";
  msg.appendChild(document.createTextNode("You cannot open "));
  var brand = document.createElement("span");
  brand.textContent = "Envato";
  brand.style.cssText = "color:#2563eb;font-weight:700";
  msg.appendChild(brand);
  msg.appendChild(document.createTextNode(" directly. Open it from your access link."));
  var foot = document.createElement("p");
  foot.textContent = "A direct visit is not allowed";
  foot.style.cssText = "display:block;margin-top:18px;color:#94a3b8;font-size:13px";
  card.appendChild(ring);
  card.appendChild(title);
  card.appendChild(msg);
  card.appendChild(foot);
  body.appendChild(card);
  html.appendChild(body);
	window.__tmDenied = true;
	if (window.__tmWatch) clearInterval(window.__tmWatch);
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
self.addEventListener("fetch", function () {
  return;
});
`
