// Keep Helium extension auth on our tool hosts only.
// Diamond and Platinum share this extension. Each host keeps its own ct_session.
// Inject ct_session via declarativeNetRequest (SW fetch ignores SameSite=Lax).
(function () {
  "use strict";

  if (typeof chrome === "undefined") return;

  var RULE_BASE = 630101;
  var COOKIE_NAME = "ct_session";
  var OFFICIAL_HOST_RE =
    /^(?:https?:)?\/\/(?:[\w-]+\.)*(?:helium10\.com|h10\.com\.cn)(?=[:/?#]|$)/i;
  var activeOrigin = "";

  function toolHosts() {
    return globalThis.H10_TOOL_HOSTS || [];
  }

  function proxyCookieHeader(list) {
    if (!list || !list.length) return "";
    for (var i = 0; i < list.length; i++) {
      if (list[i].name === COOKIE_NAME && list[i].value) {
        return COOKIE_NAME + "=" + list[i].value;
      }
    }
    return "";
  }

  function readActiveOrigin() {
    var hosts = toolHosts();
    return new Promise(function (resolve) {
      try {
        chrome.storage.local.get(["ht_custom_domain"], function (stored) {
          var host = stored && stored.ht_custom_domain ? String(stored.ht_custom_domain).toLowerCase() : "";
          if (host && hosts.indexOf(host) >= 0) {
            resolve("https://" + host);
            return;
          }
          resolve("");
        });
      } catch (e) {
        resolve("");
      }
    });
  }

  function orderedOrigins(preferred) {
    var hosts = toolHosts();
    var out = [];
    if (preferred) out.push(preferred);
    for (var i = 0; i < hosts.length; i++) {
      var origin = "https://" + hosts[i];
      if (out.indexOf(origin) < 0) out.push(origin);
    }
    return out;
  }

  async function refreshCookieRule() {
    if (!chrome.cookies || !chrome.declarativeNetRequest) return;
    activeOrigin = await readActiveOrigin();
    var hosts = toolHosts();
    var remove = [];
    var add = [];
    for (var i = 0; i < hosts.length; i++) {
      var id = RULE_BASE + i;
      remove.push(id);
      var origin = "https://" + hosts[i];
      var header = "";
      try {
        var list = await chrome.cookies.getAll({ url: origin + "/" });
        header = proxyCookieHeader(list);
      } catch (e) {
        header = "";
      }
      if (!header) continue;
      add.push({
        id: id,
        priority: 1,
        action: {
          type: "modifyHeaders",
          requestHeaders: [{ header: "cookie", operation: "set", value: header }]
        },
        condition: {
          urlFilter: origin + "/*",
          resourceTypes: ["xmlhttprequest", "other", "main_frame", "sub_frame"]
        }
      });
    }
    try {
      await chrome.declarativeNetRequest.updateSessionRules({
        removeRuleIds: remove,
        addRules: add
      });
    } catch (e) {}
  }

  function rewriteOfficialUrl(url) {
    if (!url || !activeOrigin || !OFFICIAL_HOST_RE.test(url)) return url;
    try {
      var u = new URL(url, activeOrigin + "/");
      return activeOrigin + u.pathname + u.search + u.hash;
    } catch (e) {
      return url;
    }
  }

  function originOf(url) {
    try {
      return globalThis.h10ToolOrigin(new URL(url).hostname) || "";
    } catch (e) {
      return "";
    }
  }

  if (globalThis.fetch) {
    var nativeFetch = globalThis.fetch.bind(globalThis);
    globalThis.fetch = function (input, init) {
      var url = "";
      try {
        if (typeof input === "string") url = input;
        else if (input && typeof input.url === "string") url = input.url;
      } catch (e) {}

      return readActiveOrigin().then(function (origin) {
        activeOrigin = origin || "";
        var rewritten = rewriteOfficialUrl(url);
        var nextInput = input;
        var nextInit = init;
        if (rewritten && rewritten !== url) {
          if (typeof input === "string") {
            nextInput = rewritten;
          } else if (typeof Request !== "undefined" && input instanceof Request) {
            nextInput = new Request(rewritten, input);
          }
          nextInit = Object.assign({}, init || {}, { credentials: "omit" });
        }

        var finalUrl = rewritten || url;
        var toolOrigin = originOf(finalUrl);
        if (toolOrigin && chrome.cookies) {
          return chrome.cookies
            .getAll({ url: toolOrigin + "/" })
            .then(function (list) {
              var header = proxyCookieHeader(list);
              if (!header) return nativeFetch(nextInput, nextInit);
              var headers = new Headers(
                (nextInit && nextInit.headers) ||
                  (nextInput && typeof nextInput === "object" && nextInput.headers) ||
                  undefined
              );
              headers.set("Cookie", header);
              var withCookie = Object.assign({}, nextInit || {}, {
                headers: headers,
                credentials: "include"
              });
              if (
                typeof Request !== "undefined" &&
                typeof nextInput !== "string" &&
                nextInput instanceof Request
              ) {
                return nativeFetch(new Request(nextInput, withCookie));
              }
              return nativeFetch(nextInput, withCookie);
            })
            .catch(function () {
              return nativeFetch(nextInput, nextInit);
            });
        }
        return nativeFetch(nextInput, nextInit);
      });
    };
  }

  refreshCookieRule();
  if (chrome.cookies && chrome.cookies.onChanged) {
    chrome.cookies.onChanged.addListener(function (change) {
      var c = change && change.cookie;
      if (!c || c.name !== COOKIE_NAME) return;
      if (toolHosts().indexOf(String(c.domain || "").replace(/^\./, "")) < 0) return;
      refreshCookieRule();
    });
  }
  if (chrome.storage && chrome.storage.onChanged) {
    chrome.storage.onChanged.addListener(function (changes, area) {
      if (area === "local" && changes.ht_custom_domain) refreshCookieRule();
    });
  }
  setInterval(refreshCookieRule, 15000);

  if (chrome.runtime && chrome.runtime.onMessage) {
    chrome.runtime.onMessage.addListener(function (msg, _sender, sendResponse) {
      try {
        if (!msg || msg.type !== "tm_panel_username") return false;
        readActiveOrigin()
          .then(function (preferred) {
            var origins = orderedOrigins(preferred);
            var i = 0;
            function next() {
              if (i >= origins.length) {
                sendResponse({ ok: false, username: "" });
                return;
              }
              var origin = origins[i++];
              fetch(origin + "/api/user-limits", {
                credentials: "include",
                cache: "no-store"
              })
                .then(function (r) {
                  if (!r.ok) throw new Error("limits " + r.status);
                  return r.json();
                })
                .then(function (d) {
                  var name = d && d.username ? String(d.username).trim() : "";
                  if (!name) {
                    next();
                    return;
                  }
                  sendResponse({ ok: true, username: name });
                })
                .catch(function () {
                  next();
                });
            }
            next();
          })
          .catch(function () {
            try {
              sendResponse({ ok: false, username: "" });
            } catch (e) {}
          });
        return true;
      } catch (e) {
        try {
          sendResponse({ ok: false, username: "" });
        } catch (e2) {}
        return false;
      }
    });
  }
})();
