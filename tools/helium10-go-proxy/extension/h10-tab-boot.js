// Runs before amazon-script. Answers product-page probes immediately while
// the heavy amazon bundle is still booting.
(function () {
  "use strict";
  if (window.__H10_TAB_BOOT__) return;
  window.__H10_TAB_BOOT__ = true;

  function productStatus() {
    try {
      var href = String(location.href || "");
      var asin = (href.match(/\/(?:dp|gp\/product|product)\/([A-Z0-9]{10})/i) || [])[1];
      if (asin) return { status: "product", marketplace: null };
      if (/[?&]k=/.test(href) || /\/s(\?|\/|$)/.test(href) || /\/best-sellers\//.test(href)) {
        return { status: "listing", marketplace: null };
      }
    } catch (e) {}
    return { status: null, marketplace: null };
  }

  chrome.runtime.onMessage.addListener(function (msg, _sender, sendResponse) {
    if (!msg || !msg.type) return;
    // Amazon script listener is live — let it own all messages.
    if (window.__H10_AMZ_LISTENING__ || window.__H10_AMZ_READY__) return;

    if (msg.type === "on-amazon-product-page") {
      sendResponse(productStatus());
      return true;
    }
    if (msg.type === "is-files-injected") {
      sendResponse({ status: true });
      return true;
    }
    return;
  });
})();
