/**
 * Domain Manager - Handles dynamic domain detection and storage
 * Supports ht01, ht02, ht03, ht04 subdomains on amztoolsconsultant.com
 */

class DomainManager {
  constructor() {
    this.storageKey = "ht_custom_domain";
    this.cachedDomain = null;
  }

  /**
   * Detect domain from current tab URL
   */
  detectDomainFromUrl(url) {
    try {
      const urlObj = new URL(url);
      const hostname = urlObj.hostname;

      // Check if it's a softzilla domain (ht01.amztoolsconsultant.com, ht07.amztoolsconsultant.com, etc.)
      // Accepts any ht\d+ pattern dynamically
      const match = hostname.match(/^(ht\d+)\.amztoolsconsultant\.com$/);
      if (match) {
        return match[1]; // Returns ht01, ht07, ht99, etc.
      }
    } catch (e) {
      console.error("Error detecting domain from URL:", e);
    }
    return null;
  }

  /**
   * Save domain to chrome.storage
   */
  async saveDomain(domain) {
    // Accept any ht\d+ pattern
    if (!/^ht\d+$/.test(domain)) {
      throw new Error(`Invalid domain: ${domain}. Must match pattern: ht\\d+`);
    }

    return new Promise((resolve, reject) => {
      chrome.storage.local.set({ [this.storageKey]: domain }, () => {
        if (chrome.runtime.lastError) {
          reject(chrome.runtime.lastError);
        } else {
          this.cachedDomain = domain;
          resolve(domain);
        }
      });
    });
  }

  /**
   * Get saved domain from chrome.storage
   */
  async getSavedDomain() {
    if (this.cachedDomain) {
      return this.cachedDomain;
    }

    return new Promise((resolve, reject) => {
      chrome.storage.local.get([this.storageKey], (result) => {
        if (chrome.runtime.lastError) {
          reject(chrome.runtime.lastError);
        } else {
          const domain = result[this.storageKey] || null;
          this.cachedDomain = domain;
          resolve(domain);
        }
      });
    });
  }

  /**
   * Get the base URL for API calls
   */
  async getBaseUrl() {
    return "__H10_PROXY_ORIGIN__";
  }

  /**
   * Get the members link (for signin, APIs, etc.)
   */
  async getMembersLink() {
    const baseUrl = await this.getBaseUrl();
    if (!baseUrl) {
      return url;
    }
    return baseUrl;
  }

  /**
   * Replace members.helium10.com with dynamic domain
   */
  async replaceUrl(url) {
    if (typeof url !== "string") {
      return url;
    }
    const baseUrl = await this.getBaseUrl();
    if (!baseUrl) {
      return url;
    }
    return url
      .replace(/https?:\/\/re-cdn\.helium10\.com/gi, baseUrl + "/__re-cdn")
      .replace(/https?:\/\/(?:members\.)?helium10\.com/gi, baseUrl)
      .replace(/https?:\/\/(?:members|ht\d+)\.amztoolsconsultant\.com/gi, baseUrl);
  }

  /**
   * Get global config URL with dynamic domain
   */
  async getGlobalConfigUrl() {
    const baseUrl = await this.getBaseUrl();
    const now = new Date();
    const timestamp = [
      now.getFullYear(),
      now.getMonth() + 1,
      now.getDate(),
      now.getHours(),
      now.getMinutes() > 30 ? 30 : 0,
    ].join("-");
    return `${baseUrl}/__re-cdn/global-config/configs.json?time=${timestamp}`;
  }

  /**
   * Clear saved domain (reset to default)
   */
  async clearDomain() {
    return new Promise((resolve, reject) => {
      chrome.storage.local.remove([this.storageKey], () => {
        if (chrome.runtime.lastError) {
          reject(chrome.runtime.lastError);
        } else {
          this.cachedDomain = null;
          resolve();
        }
      });
    });
  }
}

// Create singleton instance
const domainManager = new DomainManager();
