// Keep Helium extension auth on the local proxy only.
// - Never talk to official helium10 / h10.com.cn (avoids storing their cookies).
// - Inject ct_session via declarativeNetRequest (SW fetch ignores SameSite=Lax).
(function () {
  "use strict";

  var PROXY_ORIGIN = "__H10_PROXY_ORIGIN__";
  if (!PROXY_ORIGIN || PROXY_ORIGIN.indexOf("__H10_") === 0) return;
  if (typeof chrome === "undefined") return;

  var RULE_ID = 520101;
  var COOKIE_NAME = "ct_session";
  var OFFICIAL_HOST_RE =
    /^(?:https?:)?\/\/(?:[\w-]+\.)*(?:helium10\.com|h10\.com\.cn)(?=[:/?#]|$)/i;

  function proxyCookieHeader(list) {
    if (!list || !list.length) return "";
    for (var i = 0; i < list.length; i++) {
      if (list[i].name === COOKIE_NAME && list[i].value) {
        return COOKIE_NAME + "=" + list[i].value;
      }
    }
    return "";
  }

  async function refreshCookieRule() {
    if (!chrome.cookies || !chrome.declarativeNetRequest) return;
    var header = "";
    try {
      var list = await chrome.cookies.getAll({ url: PROXY_ORIGIN + "/" });
      header = proxyCookieHeader(list);
    } catch (e) {
      header = "";
    }

    try {
      await chrome.declarativeNetRequest.updateSessionRules({
        removeRuleIds: [RULE_ID],
        addRules: header
          ? [
              {
                id: RULE_ID,
                priority: 1,
                action: {
                  type: "modifyHeaders",
                  requestHeaders: [
                    { header: "cookie", operation: "set", value: header }
                  ]
                },
                condition: {
                  urlFilter: PROXY_ORIGIN + "/*",
                  resourceTypes: [
                    "xmlhttprequest",
                    "other",
                    "main_frame",
                    "sub_frame"
                  ]
                }
              }
            ]
          : []
      });
    } catch (e) {}
  }

  function rewriteOfficialUrl(url) {
    if (!url || !OFFICIAL_HOST_RE.test(url)) return url;
    try {
      var u = new URL(url, PROXY_ORIGIN + "/");
      return PROXY_ORIGIN + u.pathname + u.search + u.hash;
    } catch (e) {
      return url;
    }
  }

  // Patch fetch: rewrite official hosts → proxy, never credential official domains.
  if (globalThis.fetch) {
    var nativeFetch = globalThis.fetch.bind(globalThis);
    globalThis.fetch = function (input, init) {
      var url = "";
      try {
        if (typeof input === "string") url = input;
        else if (input && typeof input.url === "string") url = input.url;
      } catch (e) {}

      var rewritten = rewriteOfficialUrl(url);
      if (rewritten && rewritten !== url) {
        if (typeof input === "string") {
          input = rewritten;
        } else if (typeof Request !== "undefined" && input instanceof Request) {
          input = new Request(rewritten, input);
        }
        init = Object.assign({}, init || {}, { credentials: "omit" });
      }

      // Still attach Cookie for proxy URLs as a belt-and-suspenders backup to DNR.
      var finalUrl = rewritten || url;
      if (
        finalUrl &&
        (finalUrl === PROXY_ORIGIN || finalUrl.indexOf(PROXY_ORIGIN + "/") === 0) &&
        chrome.cookies
      ) {
        return chrome.cookies
          .getAll({ url: PROXY_ORIGIN + "/" })
          .then(function (list) {
            var header = proxyCookieHeader(list);
            if (!header) return nativeFetch(input, init);
            var headers = new Headers(
              (init && init.headers) ||
                (input && typeof input === "object" && input.headers) ||
                undefined
            );
            headers.set("Cookie", header);
            var next = Object.assign({}, init || {}, {
              headers: headers,
              credentials: "include"
            });
            if (
              typeof Request !== "undefined" &&
              typeof input !== "string" &&
              input instanceof Request
            ) {
              return nativeFetch(new Request(input, next));
            }
            return nativeFetch(input, next);
          })
          .catch(function () {
            return nativeFetch(input, init);
          });
      }

      return nativeFetch(input, init);
    };
  }

  refreshCookieRule();
  if (chrome.cookies && chrome.cookies.onChanged) {
    chrome.cookies.onChanged.addListener(function (change) {
      var c = change && change.cookie;
      if (!c || c.name !== COOKIE_NAME) return;
      refreshCookieRule();
    });
  }
  setInterval(refreshCookieRule, 15000);
})();
