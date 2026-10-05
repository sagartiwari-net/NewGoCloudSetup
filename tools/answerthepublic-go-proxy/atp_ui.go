package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// atpHideChromeCSS hides Upgrade / marketing nav / help-sidebar / avatar chrome.
func atpHideChromeCSS() string {
	return strings.TrimSpace(`
[data-testid="nav-upgrade-button"],
[data-testid="nav-learn-button"],
[data-testid="nav-use-cases-button"],
[data-testid="nav-services-button"],
[data-testid="nav-notifications-bell"],
[data-testid="nav-avatar-button"],
[data-slot="dropdown-menu-trigger"]:has([data-testid="nav-avatar-button"]),
[data-slot="dropdown-menu-trigger"]:has([data-testid="nav-learn-button"]),
[data-slot="dropdown-menu-trigger"]:has([data-testid="nav-use-cases-button"]),
[data-slot="dropdown-menu-trigger"]:has([data-testid="nav-services-button"]),
a[href*="discord.gg"],
a[href*="answerthepublic.zendesk.com"],
button:has(.tabler-icon-logout-2),
button:has(svg.tabler-icon-logout-2),
div[data-testid="email-verification-banner"] {
  display: none !important;
}
`)
}

// atpUIChromeScript hides marketing chrome and inserts a panel-username row
// under the "Buy credits" sidebar button.
func atpUIChromeScript(panelUsername string) string {
	userJS, _ := json.Marshal(strings.TrimSpace(panelUsername))
	return fmt.Sprintf(`<script>
(function(){
  if (window.__atpUIChrome) return;
  window.__atpUIChrome = true;
  var TM_USER = %s;
  function norm(s) {
    return String(s || '').replace(/\s+/g, ' ').trim().toLowerCase();
  }
  function hide(el) {
    if (!el || (el.dataset && el.dataset.tmHide === '1')) return;
    el.style.setProperty('display', 'none', 'important');
    el.setAttribute('aria-hidden', 'true');
    if (el.dataset) el.dataset.tmHide = '1';
  }
  function hideChrome(root) {
    if (!root || !root.querySelectorAll) return;
    var sels = [
      '[data-testid="nav-upgrade-button"]',
      '[data-testid="nav-learn-button"]',
      '[data-testid="nav-use-cases-button"]',
      '[data-testid="nav-services-button"]',
      '[data-testid="nav-notifications-bell"]',
      '[data-testid="nav-avatar-button"]',
      'a[href*="discord.gg"]',
      'a[href*="answerthepublic.zendesk.com"]'
    ];
    for (var s = 0; s < sels.length; s++) {
      var nodes = root.querySelectorAll(sels[s]);
      for (var i = 0; i < nodes.length; i++) {
        var el = nodes[i];
        // Only hide the control itself (or its menu trigger) — never a broad
        // .relative ancestor (that can wipe the whole dashboard main pane).
        if (sels[s].indexOf('nav-avatar') !== -1 || sels[s].indexOf('nav-learn') !== -1 ||
            sels[s].indexOf('nav-use-cases') !== -1 || sels[s].indexOf('nav-services') !== -1) {
          hide(el.closest('[data-slot="dropdown-menu-trigger"]') || el);
        } else {
          hide(el);
        }
      }
    }
    // EN language trigger next to upgrade cluster
    var triggers = root.querySelectorAll('[data-slot="dropdown-menu-trigger"]');
    for (var t = 0; t < triggers.length; t++) {
      var tx = norm(triggers[t].textContent);
      if (tx === 'en' || tx.indexOf('en') === 0 && tx.length <= 4) hide(triggers[t]);
    }
    // Sidebar help box: Watch tutorial / Send Feedback / Community / Support
    // Also hide Sign Out.
    var buttons = root.querySelectorAll('button, a');
    for (var b = 0; b < buttons.length; b++) {
      var label = norm(buttons[b].textContent);
      if (label === 'watch tutorial' || label === 'send feedback' || label === 'community' || label === 'support') {
        var box = buttons[b].closest('.shrink-0') || buttons[b].closest('.rounded-xl') || buttons[b].parentElement;
        if (box) hide(box);
      }
      if (label === 'sign out' || label === 'logout' || label === 'log out') {
        hide(buttons[b].closest('div.flex.flex-col') || buttons[b].parentElement || buttons[b]);
        hide(buttons[b]);
      }
    }
    var logoutIcons = root.querySelectorAll('svg.tabler-icon-logout-2, .tabler-icon-logout-2');
    for (var l = 0; l < logoutIcons.length; l++) {
      hide(logoutIcons[l].closest('button') || logoutIcons[l].parentElement);
    }
  }
  // Replace "Manage account" with non-clickable panel username.
  function replaceManageAccount(root) {
    if (!root || !root.querySelectorAll) return;
    var links = root.querySelectorAll('a[href*="/account"]');
    for (var i = 0; i < links.length; i++) {
      var a = links[i];
      var label = norm(a.textContent);
      if (label.indexOf('manage account') === -1 && label !== TM_USER.toLowerCase()) continue;
      if (a.dataset && a.dataset.tmPanelUserLink === '1') {
        var sp = a.querySelector('[data-tm-panel-user-label]');
        if (sp && TM_USER && sp.textContent !== TM_USER) sp.textContent = TM_USER;
        continue;
      }
      a.setAttribute('data-tm-panel-user-link', '1');
      a.removeAttribute('href');
      a.setAttribute('role', 'presentation');
      a.style.cursor = 'default';
      a.style.pointerEvents = 'none';
      a.onclick = function(e){ e.preventDefault(); e.stopPropagation(); return false; };
      var span = a.querySelector('span');
      if (span && TM_USER) {
        span.textContent = TM_USER;
        span.setAttribute('data-tm-panel-user-label', '1');
      } else if (TM_USER) {
        a.appendChild(document.createTextNode(TM_USER));
      }
      var svg = a.querySelector('svg');
      if (svg) {
        svg.innerHTML = '<path d="M12 12a4 4 0 1 0 -4 -4a4 4 0 0 0 4 4"></path><path d="M4 20a8 8 0 0 1 16 0"></path>';
      }
    }
  }
  function ensureUserBtn(root) {
    if (!TM_USER || !root || !root.querySelectorAll) return;
    var buttons = root.querySelectorAll('button');
    var buy = null;
    for (var i = 0; i < buttons.length; i++) {
      if (norm(buttons[i].textContent) === 'buy credits') { buy = buttons[i]; break; }
    }
    if (!buy) return;
    var parent = buy.parentElement;
    if (!parent) return;
    var existing = parent.querySelector('[data-tm-panel-user]');
    if (existing) {
      var span = existing.querySelector('[data-tm-panel-user-label]');
      if (span && span.textContent !== TM_USER) span.textContent = TM_USER;
      return;
    }
    var btn = buy.cloneNode(true);
    btn.setAttribute('data-tm-panel-user', '1');
    btn.setAttribute('type', 'button');
    btn.removeAttribute('aria-label');
    btn.style.cursor = 'default';
    btn.onclick = function(e){ e.preventDefault(); e.stopPropagation(); return false; };
    // Swap icon text / keep coins-like look but show username
    var label = btn.querySelector('span');
    if (label) {
      label.textContent = TM_USER;
      label.setAttribute('data-tm-panel-user-label', '1');
    } else {
      btn.textContent = TM_USER;
    }
    // Prefer a simple user glyph if an svg exists
    var svg = btn.querySelector('svg');
    if (svg) {
      svg.innerHTML = '<path d="M12 12a4 4 0 1 0 -4 -4a4 4 0 0 0 4 4"></path><path d="M4 20a8 8 0 0 1 16 0"></path>';
    }
    if (buy.nextSibling) parent.insertBefore(btn, buy.nextSibling);
    else parent.appendChild(btn);
  }
  function run() {
    try {
      hideChrome(document);
      replaceManageAccount(document);
      ensureUserBtn(document);
    } catch (e) {}
  }
  run();
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', run);
  try {
    new MutationObserver(function(){ run(); }).observe(document.documentElement, { childList: true, subtree: true });
  } catch (e) {}
  setInterval(run, 1500);
})();
</script>`, string(userJS))
}
