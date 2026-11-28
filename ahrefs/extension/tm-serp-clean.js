/**
 * tm-serp-clean.js — Markhor-style Google SERP URL cleaner (document_start).
 *
 * Sets data-mkhr-clean so official content.js can call tbGetHeaderV3 with a real
 * target. Without this, /goto?url=CAES… → InvalidTarget and domain bars stay empty.
 */
(function tmSerpClean() {
  if (!/(^|\.)google\./i.test(location.hostname)) return;

  var ATTR_DONE = "data-tm-goto";
  var ATTR_CLEAN = "data-mkhr-clean";
  var ATTR_ORIG = "data-tm-orig";
  var SEP_RE = /\s*[›»>→•:\t\r\n|-]\s*/;

  function isGoogleNav(a) {
    if (!a || !a.closest) return true;
    return !!a.closest(
      '#hdtb, #top_nav, #gb, header, [role="navigation"], [role="tablist"], .hdtb-mitem, .hdtb-imb, #nJoHfd'
    );
  }

  function isInternalGoogleNavUrl(href) {
    if (!href) return true;
    if (href.startsWith("#") || href.startsWith("javascript:")) return true;
    if (/^\/(preferences|advanced_search|history|imgres|setprefs)/i.test(href)) return true;
    if (/^https?:\/\/(accounts|policies|myactivity|maps)\.google\./i.test(href)) return true;
    if (/^\/search\?/i.test(href) || /^https?:\/\/(www\.)?google\.[a-z.]+\/search\?/i.test(href)) {
      try {
        var full = href.startsWith("/") ? "https://www.google.com" + href : href;
        var u = new URL(full);
        var t = u.searchParams.get("url") || u.searchParams.get("q");
        if (t && /^https?:\/\//i.test(t) && t.indexOf("google.com/") < 0) return false;
      } catch (e) {}
      return true;
    }
    return false;
  }

  function extractUrlFromQuery(str) {
    if (!str || typeof str !== "string") return null;
    try {
      var full = str.startsWith("/") ? "https://www.google.com" + str : str;
      var u = new URL(full);
      var t = u.searchParams.get("url") || u.searchParams.get("q");
      if (!t) return null;
      if (/^https?:\/\//i.test(t) && t.indexOf("google.com/") < 0) return t.split("#")[0];
      try {
        var d = decodeURIComponent(t);
        if (/^https?:\/\//i.test(d) && d.indexOf("google.com/") < 0) return d.split("#")[0];
      } catch (e) {}
    } catch (e) {}
    return null;
  }

  function tryExtractUrlFromBase64(str) {
    if (!str || typeof str !== "string") return null;
    try {
      var b64 = str.replace(/-/g, "+").replace(/_/g, "/");
      while (b64.length % 4) b64 += "=";
      var text = atob(b64);
      var m = text.match(/https?:\/\/[^\s"'<>\x00-\x1f]+/i);
      if (m && m[0].indexOf("google.com") < 0) {
        return m[0].replace(/[.,;)\]]+$/, "");
      }
      var m2 = text.match(/([a-z0-9-]+\.)+[a-z]{2,}(?::\d+)?(?:\/[^\s"'<>\x00-\x1f]*)?/i);
      if (m2 && m2[0].indexOf(".") > 0 && m2[0].indexOf("google.com") < 0) {
        return "https://" + m2[0].replace(/[.,;)\]]+$/, "");
      }
    } catch (e) {}
    return null;
  }

  function isPlausibleHost(host) {
    if (!host) return false;
    host = String(host).toLowerCase().replace(/\.$/, "");
    if (host.length < 3 || host.length > 253) return false;
    if (host.indexOf("+") >= 0 || host.indexOf(" ") >= 0) return false;
    if (!/^[a-z0-9.-]+\.[a-z]{2,}$/i.test(host)) return false;
    // Reject social/follower junk like "1.3l+followers"
    if (/^\d/.test(host) && !/\.(com|net|org|in|io|co)\b/i.test(host)) return false;
    return true;
  }

  function urlFromBreadcrumb(rawText) {
    if (!rawText) return null;
    var text = String(rawText).trim();
    if (!text) return null;

    var scheme = "https://";
    var startIdx = 0;
    var httpMatch = text.match(/https?:\/\//i);
    if (httpMatch) {
      scheme = httpMatch[0].toLowerCase();
      startIdx = httpMatch.index + httpMatch[0].length;
    } else {
      var domainMatch = text.match(/([a-z0-9.-]+\.[a-z]{2,})/i);
      if (!domainMatch) return null;
      startIdx = domainMatch.index;
    }

    var urlPart = text.slice(startIdx).trim();
    var parts = urlPart
      .split(SEP_RE)
      .map(function (s) {
        return s.trim();
      })
      .filter(Boolean);
    if (!parts.length) return null;

    var host = parts[0].replace(/\/+$/, "").replace(/^https?:\/\//i, "");
    if (!isPlausibleHost(host)) return null;

    var segs = parts.slice(1);
    var cleanSegs = [];
    for (var i = 0; i < segs.length; i++) {
      var s = segs[i];
      if (/[…]|\.\.\./.test(s)) break;
      if (/followers|views|subscribers|rating|reviews/i.test(s)) break;
      try {
        cleanSegs.push(encodeURIComponent(s).replace(/%2F/gi, "/"));
      } catch (e) {
        cleanSegs.push(s);
      }
    }
    if (!cleanSegs.length) return scheme + host + "/";
    return scheme + host + "/" + cleanSegs.join("/");
  }

  function findBreadcrumbText(anchor) {
    if (!anchor || !anchor.closest) return null;
    var card = anchor.closest(
      ".g, .tF2Cxc, .MjjYud, .Ww4FFb, [data-sokoban-container], .N54Ufe, .caqC9e, .wHYlbd, .VkpRyc"
    );
    if (!card) return null;
    var bc = card.querySelector(
      "cite, span[role='text'], div.TbwRce, div.byjcW, span.VuuXzf, span.x2VHHc, " +
        "div.iUh30, span.qLRx3b, div.UPTMgc, [class*='cite']"
    );
    if (bc && bc.textContent && bc.textContent.trim()) return bc.textContent.trim();
    return null;
  }

  function resolveExactUrl(a) {
    var href = a.getAttribute("href") || "";
    var ping = a.getAttribute("ping") || "";
    var dataUrl =
      a.getAttribute("data-url") ||
      a.getAttribute("data-href") ||
      a.getAttribute("data-target-url") ||
      a.getAttribute("data-phref") ||
      a.getAttribute("data-cturl");

    var direct = extractUrlFromQuery(href) || extractUrlFromQuery(ping) || extractUrlFromQuery(dataUrl);
    if (direct) return direct;

    if (dataUrl && /^https?:\/\//i.test(dataUrl) && dataUrl.indexOf("google.com/") < 0) {
      return dataUrl.split("#")[0];
    }

    var b64Match = (href + " " + ping).match(/(?:url=|\/goto\?url=)([A-Za-z0-9_-]{15,})/i);
    if (b64Match) {
      var extracted = tryExtractUrlFromBase64(b64Match[1]);
      if (extracted && extracted.indexOf("google.com") < 0) return extracted.split("#")[0];
    }
    return null;
  }

  function fixAnchor(a) {
    if (!a || a.getAttribute(ATTR_DONE) != null) return;
    if (isGoogleNav(a)) return;

    var href = a.getAttribute("href") || "";
    if (isInternalGoogleNavUrl(href)) return;

    var isGoto = /\/goto\?/i.test(href);
    var isGoogleUrl = /\/url\?/i.test(href) || /google\.[a-z.]+\/url\?/i.test(href);
    var isEncrypted = isGoto || isGoogleUrl || a.hasAttribute("ping") || a.hasAttribute("data-jsarwt");

    if (!isEncrypted && /^https?:\/\//i.test(href) && href.indexOf("google.com/") < 0) {
      a.setAttribute(ATTR_CLEAN, href.split("#")[0]);
      a.setAttribute(ATTR_DONE, "1");
      return;
    }

    var exactUrl = resolveExactUrl(a);
    var bcUrl = urlFromBreadcrumb(findBreadcrumbText(a));
    var target = exactUrl || bcUrl;

    if (!target || /google\.[a-z.]+\/(search|preferences|history)/i.test(target)) {
      a.setAttribute(ATTR_DONE, "0");
      return;
    }

    try {
      var host = new URL(target).hostname;
      if (!isPlausibleHost(host)) {
        a.setAttribute(ATTR_DONE, "0");
        return;
      }
    } catch (e) {
      a.setAttribute(ATTR_DONE, "0");
      return;
    }

    a.setAttribute(ATTR_DONE, "1");
    if (!a.hasAttribute(ATTR_ORIG)) a.setAttribute(ATTR_ORIG, href);
    a.setAttribute(ATTR_CLEAN, target);

    // Exact URL: rewrite href so toolbar + clicks both see a real host
    if (exactUrl) {
      a.setAttribute("href", exactUrl);
      a.removeAttribute("ping");
      a.removeAttribute("data-jsarwt");
    }
  }

  function sweep(root) {
    var scope = root && root.querySelectorAll ? root : document;
    var selectors = [
      'a[href*="/goto?"]',
      'a[href*="/goto"]',
      'a[href*="/url?"]',
      'a[href*="google.com/url"]',
      'a[ping*="/url?"]',
      "a[data-jsarwt]",
      ".g a[href]",
      ".tF2Cxc a[href]",
      ".MjjYud a[href]",
      ".Ww4FFb a[href]",
      "[data-sokoban-container] a[href]",
      "div.yuRUbf > a",
      "h3 a",
    ];
    var anchors = scope.querySelectorAll(selectors.join(", "));
    for (var i = 0; i < anchors.length; i++) fixAnchor(anchors[i]);
  }

  var scheduled = false;
  function schedule() {
    if (scheduled) return;
    scheduled = true;
    requestAnimationFrame(function () {
      scheduled = false;
      try {
        sweep(document);
      } catch (e) {}
    });
  }

  sweep(document);
  schedule();
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", schedule, { once: true });
  }
  setTimeout(schedule, 100);
  setTimeout(schedule, 500);
  setTimeout(schedule, 1500);
  setInterval(schedule, 2000);

  try {
    new MutationObserver(function (muts) {
      for (var i = 0; i < muts.length; i++) {
        var nodes = muts[i].addedNodes;
        for (var j = 0; j < nodes.length; j++) {
          var n = nodes[j];
          if (n && n.nodeType === 1) {
            if (n.tagName === "A") fixAnchor(n);
            else sweep(n);
          }
        }
      }
      schedule();
    }).observe(document.documentElement || document, { childList: true, subtree: true });
  } catch (e) {}

  window.addEventListener("popstate", schedule, true);
  try {
    if (!history.pushState.__tmClean) {
      var wrap = function (fn) {
        return function () {
          var ret = fn.apply(this, arguments);
          schedule();
          return ret;
        };
      };
      history.pushState = wrap(history.pushState);
      history.pushState.__tmClean = true;
      history.replaceState = wrap(history.replaceState);
      history.replaceState.__tmClean = true;
    }
  } catch (e) {}
})();
