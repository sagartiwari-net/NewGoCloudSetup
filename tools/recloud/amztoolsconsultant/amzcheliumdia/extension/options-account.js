(() => {
  const DOMAIN_STORAGE_KEY = "ht_custom_domain";
  const PANEL_ID = "linked-helium-account-panel";

  function readLinkedAccount(status) {
    chrome.storage.local.get([DOMAIN_STORAGE_KEY], (result) => {
      const domain = result[DOMAIN_STORAGE_KEY];
      status.textContent = /^ht\d+$/.test(domain || "")
        ? `Currently linked: ${domain}.amztoolsconsultant.com`
        : "No Helium 10 account is currently linked.";
    });
  }

  function createPanel(anchor) {
    if (document.getElementById(PANEL_ID)) return;

    const panel = document.createElement("section");
    panel.id = PANEL_ID;
    panel.style.cssText = "margin-top:16px;padding:16px;border:1px solid #d7e3f4;border-radius:8px;background:#f8fbff;max-width:560px;font-family:-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;";

    const title = document.createElement("strong");
    title.textContent = "Linked Helium 10 account";
    title.style.cssText = "display:block;margin-bottom:6px;color:#123b68;font-size:16px;";

    const help = document.createElement("p");
    help.textContent = "To switch accounts, first close every open Helium 10 members-area tab. Then clear the linked account and open the other members-area link.";
    help.style.cssText = "margin:0 0 10px;color:#465a70;line-height:1.45;font-size:14px;";

    const status = document.createElement("p");
    status.style.cssText = "margin:0 0 12px;color:#123b68;font-size:14px;font-weight:600;";

    const button = document.createElement("button");
    button.type = "button";
    button.textContent = "Clear linked account";
    button.style.cssText = "border:1px solid #0a6fe8;border-radius:5px;background:#fff;color:#0a6fe8;padding:8px 12px;font-size:14px;font-weight:600;cursor:pointer;";
    button.addEventListener("click", () => {
      if (!window.confirm("Clear the currently linked Helium 10 account? Close all Helium 10 tabs first so it is not linked again automatically.")) return;
      button.disabled = true;
      chrome.runtime.sendMessage({ type: "CLEAR_LINKED_ACCOUNT" }, (response) => {
        button.disabled = false;
        if (chrome.runtime.lastError || !response?.cleared) {
          status.textContent = "Could not clear the linked account. Please try again.";
          return;
        }
        status.textContent = "Linked account cleared. Open the other members-area link to connect it.";
      });
    });

    panel.append(title, help, status, button);
    anchor.insertAdjacentElement("afterend", panel);
    readLinkedAccount(status);
  }

  function attachAccountPanel() {
    const clearCacheButton = Array.from(document.querySelectorAll("button")).find((button) =>
      (button.textContent || "").trim().toLowerCase() === "clear cache"
    );
    if (clearCacheButton) createPanel(clearCacheButton.parentElement || clearCacheButton);
  }

  const observer = new MutationObserver(attachAccountPanel);
  observer.observe(document.documentElement, { childList: true, subtree: true });
  attachAccountPanel();
})();
