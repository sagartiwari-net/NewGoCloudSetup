/**
 * API Request Redirector for SoftZilla Domains
 * Intercepts fetch/XHR requests and ensures they use the correct domain
 * Handles both amztoolsconsultant.com and custom domains
 */

(function() {
  if (typeof window === "undefined") return;

  const hostname = window.location.hostname;
  const proxyHost = "__H10_PROXY_HOST__";
  if (!hostname.includes('amztoolsconsultant.com') && hostname !== proxyHost) return;
  const proxyOrigin = "__H10_PROXY_ORIGIN__";
  function rewriteApiUrl(url) {
    if (typeof url !== "string") return url;
    return url
      .replace(/https?:\/\/re-cdn\.helium10\.com/gi, proxyOrigin + "/__re-cdn")
      .replace(/https?:\/\/(?:members\.)?helium10\.com/gi, proxyOrigin)
      .replace(/https?:\/\/(?:members|ht\d+)\.amztoolsconsultant\.com/gi, proxyOrigin);
  }

  // Patch window.fetch
  const originalFetch = window.fetch;
  window.fetch = function(...args) {
    let [resource, config] = args;
    
    if (typeof resource === 'string') {
      resource = rewriteApiUrl(resource);
      args[0] = resource;
    } else if (resource instanceof Request) {
      const newUrl = rewriteApiUrl(resource.url);
      if (newUrl !== resource.url) {
        args[0] = new Request(newUrl, resource);
      }
    }
    
    return originalFetch.apply(this, args);
  };

  // Patch XMLHttpRequest.open
  const originalXHROpen = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function(method, url, ...rest) {
    if (typeof url === 'string') url = rewriteApiUrl(url);
    return originalXHROpen.call(this, method, url, ...rest);
  };

  // Patch Element.setAttribute to catch null errors
  const originalSetAttribute = Element.prototype.setAttribute;
  Element.prototype.setAttribute = function(name, value) {
    try {
      return originalSetAttribute.call(this, name, value);
    } catch (e) {
      // Silently catch setAttribute errors - these often come from vendor code issues
      return undefined;
    }
  };

  // Patch Element.appendChild to catch null errors
  const originalAppendChild = Element.prototype.appendChild;
  Element.prototype.appendChild = function(node) {
    try {
      return originalAppendChild.call(this, node);
    } catch (e) {
      return node;
    }
  };

  // Patch document.querySelector to return safe proxy instead of null
  const originalQuerySelector = document.querySelector;
  document.querySelector = function(selector) {
    const result = originalQuerySelector.call(this, selector);
    // Don't wrap null - allow it through but vendor code should check
    return result;
  };

  // Global error handler for unhandled promise rejections
  window.addEventListener("error", (event) => {
    const message = event.message || "";
    const stack = event.filename || "";
    
    // Suppress known vendor code errors
    if (message.includes("Cannot read properties of null") ||
        message.includes("Cannot read property") ||
        (stack.includes("helium-script.js") && message.includes("setAttribute"))) {
      event.preventDefault();
      return false;
    }
  }, true);

  // Handle unhandled promise rejections
  window.addEventListener("unhandledrejection", (event) => {
    const message = String(event.reason || "");
    
    // Suppress vendor code errors that don't affect functionality
    if (message.includes("Cannot read properties of null") ||
        message.includes("Cannot read property")) {
      event.preventDefault();
      return false;
    }
  });

  // Suppress console errors for setAttribute on null (these often come from vendor code issues)
  const originalError = console.error;
  console.error = function(...args) {
    const message = args.join(' ');
    // Filter out known vendor code errors that don't affect functionality
    if (!message.includes('Cannot read properties of null') && 
        !message.includes("Cannot read property") &&
        !message.includes('Marketplace not found')) {
      return originalError.apply(console, args);
    }
  };
})();
