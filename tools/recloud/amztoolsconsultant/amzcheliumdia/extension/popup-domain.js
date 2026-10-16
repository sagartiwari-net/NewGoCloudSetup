/**
 * Popup Domain Control Panel
 * Handles the custom domain UI in the popup
 */

(function() {
  const DOMAIN_STORAGE_KEY = "ht_custom_domain";
  const NO_DOMAIN_LABEL = "Domain not detected";
  const NO_DOMAIN_INSTRUCTION = "Please open the Helium 10 web app from your members area first, then return here and log in to launch the extension tools.";
  const AUTH_ACTION_PATTERN = /^(?:login|log in(?:to)?|signup|sign up)\b/i;

  function removeLoggedOutAuthActions() {
    const popupText = document.body?.innerText || "";
    const isLoggedOut = popupText.includes("Please log in to launch the extension tools.") ||
      popupText.includes(NO_DOMAIN_INSTRUCTION);
    if (!isLoggedOut) return;

    document.querySelectorAll("button, a, [role='button']").forEach((element) => {
      const label = (element.textContent || "").replace(/\s+/g, " ").trim().toLowerCase();
      if (AUTH_ACTION_PATTERN.test(label)) {
        element.remove();
      }
    });

    document.querySelectorAll("div, span, p").forEach((element) => {
      if ((element.textContent || "").trim().toUpperCase() === "OR") {
        element.remove();
      }
    });
  }

  // The bundled popup is rendered asynchronously. Replace the generic logout
  // hint with a clear domain-setup instruction while no htXX tab is known.
  function improveLoggedOutMessage() {
    chrome.storage.local.get([DOMAIN_STORAGE_KEY], (result) => {
      if (result[DOMAIN_STORAGE_KEY]) return;
      const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
      const matches = [];
      let node;
      while ((node = walker.nextNode())) {
        if (node.nodeValue && node.nodeValue.includes("Please log in to launch the extension tools.")) {
          matches.push(node);
        }
      }
      matches.forEach((textNode) => {
        textNode.nodeValue = textNode.nodeValue.replace(
          "Please log in to launch the extension tools.",
          NO_DOMAIN_INSTRUCTION
        );
      });
      removeLoggedOutAuthActions();
    });
  }

  function updateDomainDisplay() {
    chrome.storage.local.get([DOMAIN_STORAGE_KEY], (result) => {
      const domain = result[DOMAIN_STORAGE_KEY] || null;
      const domainSpan = document.getElementById("current-domain");
      if (domainSpan) {
        domainSpan.textContent = domain ? domain + ".amztoolsconsultant.com" : NO_DOMAIN_LABEL;
        domainSpan.style.color = domain ? "#0066cc" : "#777";
        domainSpan.style.fontWeight = "bold";
      }
    });
  }

  function setupEventListeners() {
    // Fetch Current Domain button
    const fetchBtn = document.getElementById("fetch-domain-btn");
    if (fetchBtn) {
      fetchBtn.addEventListener("click", () => {
        updateDomainDisplay();
        const msg = document.getElementById("domain-message");
        if (msg) {
          msg.textContent = "Domain refreshed!";
          msg.style.color = "#00aa00";
          setTimeout(() => {
            msg.textContent = "";
          }, 2000);
        }
      });
    }

    // Clear Data button
    const clearBtn = document.getElementById("clear-data-btn");
    if (clearBtn) {
      clearBtn.addEventListener("click", () => {
        chrome.storage.local.remove([DOMAIN_STORAGE_KEY], () => {
          updateDomainDisplay();
          const msg = document.getElementById("domain-message");
          if (msg) {
            msg.textContent = "Domain detection cleared";
            msg.style.color = "#ff6600";
            setTimeout(() => {
              msg.textContent = "";
            }, 2000);
          }
        });
      });
    }
  }

  // Wait for DOM to be fully ready
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", () => {
      setTimeout(() => {
        updateDomainDisplay();
        setupEventListeners();
        improveLoggedOutMessage();
      }, 100);
    });
  } else {
    // DOM already loaded
    setTimeout(() => {
      updateDomainDisplay();
      setupEventListeners();
      improveLoggedOutMessage();
    }, 100);
  }

  new MutationObserver(improveLoggedOutMessage).observe(document.documentElement, {
    childList: true,
    subtree: true,
  });

  // Listen for storage changes
  chrome.storage.onChanged.addListener((changes, areaName) => {
    if (areaName === "local" && DOMAIN_STORAGE_KEY in changes) {
      updateDomainDisplay();
    }
  });
})();
