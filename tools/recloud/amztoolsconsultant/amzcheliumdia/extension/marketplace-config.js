/**
 * Marketplace Patch for SoftZilla Domains
 * Patches vendor code to recognize amztoolsconsultant.com custom domains
 */

(function() {
  if (typeof window === "undefined") return;

  // Only run on amztoolsconsultant.com pages
  if (!window.location.hostname.endsWith('.amztoolsconsultant.com')) return;

  // Patch the marketplace detection after vendor code loads
  // This will be called by the vendor code when checking domains
  const originalFetch = window.fetch;
  
  // Store the current hostname for reference
  window.__CUSTOM_DOMAIN_HOST__ = window.location.hostname;
  
  // Extract just the domain part (e.g., ht07 from ht07.amztoolsconsultant.com)
  const match = window.location.hostname.match(/^(ht\d+)\.amztoolsconsultant\.com$/);
  if (match) {
    window.__CUSTOM_DOMAIN_PREFIX__ = match[1];
  }

  // This will be used by our runtime patches
  window.__IS_SOFTZILLA_CUSTOM__ = true;
})();


