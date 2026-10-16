/**
 * Domain Detection Content Script
 * Runs on amztoolsconsultant.com pages to detect and store the current domain
 * Supports any ht\d+ domain (ht01, ht02, ..., ht99, etc.)
 */

(function() {
  // Only run if we're in a valid environment
  if (typeof chrome === "undefined" || !chrome.runtime) {
    return;
  }

  /**
   * Report the current domain to the background worker. The worker serializes
   * updates so that the most recently refreshed account tab always wins.
   */
  function detectAndStoreDomain() {
    const hostname = window.location.hostname.toLowerCase();
    const onTool = globalThis.h10ToolOrigin && globalThis.h10ToolOrigin(hostname);
    const match = hostname.match(/^(ht\d+)\.amztoolsconsultant\.com$/);
    if (onTool || match) {
      try {
        chrome.runtime.sendMessage({
          type: "LINKED_DOMAIN_REFRESHED",
          url: window.location.href,
          refreshTime: performance.timeOrigin || Date.now(),
        });
      } catch (e) {

      }
    }
  }

  // Run detection on page load
  detectAndStoreDomain();

  // Also run on DOMContentLoaded and history changes
  if (document.readyState !== "loading") {
    detectAndStoreDomain();
  }
  
  document.addEventListener("DOMContentLoaded", detectAndStoreDomain);
  window.addEventListener("load", detectAndStoreDomain);

  // Listen for navigation/history changes
  const originalPushState = history.pushState;
  const originalReplaceState = history.replaceState;

  history.pushState = function(...args) {
    const result = originalPushState.apply(this, args);
    setTimeout(detectAndStoreDomain, 100);
    return result;
  };

  history.replaceState = function(...args) {
    const result = originalReplaceState.apply(this, args);
    setTimeout(detectAndStoreDomain, 100);
    return result;
  };

  window.addEventListener("popstate", () => {
    setTimeout(detectAndStoreDomain, 100);
  });
})();
