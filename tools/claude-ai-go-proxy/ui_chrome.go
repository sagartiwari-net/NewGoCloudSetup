package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// claudeUserChromeScript disables the Claude account user-menu popup (only that menu)
// and shows the panel access username on the footer user button instead of the
// upstream Claude account display name.
func claudeUserChromeScript(panelUsername string) string {
	userJS, _ := json.Marshal(strings.TrimSpace(panelUsername))
	return fmt.Sprintf(`<script data-tm-claude-user="1">
(function(){
  if (window.__tmClaudeUserChrome) return;
  window.__tmClaudeUserChrome = true;
  var TM_USER = %s;

  function isUserMenuBtn(t) {
    return !!(t && t.closest && t.closest('[data-testid="user-menu-button"]'));
  }
  function block(e) {
    if (!isUserMenuBtn(e.target)) return;
    e.preventDefault();
    e.stopPropagation();
    if (e.stopImmediatePropagation) e.stopImmediatePropagation();
  }
  ['pointerdown','mousedown','mouseup','click','auxclick','keydown','keyup','touchstart','touchend'].forEach(function(type){
    document.addEventListener(type, block, true);
  });

  function hideEl(el) {
    if (!el || (el.dataset && el.dataset.tmHideUserMenu === '1')) return;
    el.style.setProperty('display', 'none', 'important');
    el.setAttribute('aria-hidden', 'true');
    if (el.dataset) el.dataset.tmHideUserMenu = '1';
  }
  // Only the account user menu — identified by Claude's user-menu-* testids.
  function hideUserMenu(root) {
    if (!root || !root.querySelectorAll) return;
    var markers = root.querySelectorAll(
      '[data-testid="user-menu-header"],[data-testid="user-menu-settings"],[data-testid="user-menu-usage"]'
    );
    for (var i = 0; i < markers.length; i++) {
      var menu = markers[i].closest('[data-cds="Menu"]') || markers[i].closest('[role="menu"]');
      if (!menu) continue;
      var wrap = menu.closest('[role="presentation"][data-open]') ||
        menu.closest('[data-open][data-side]') ||
        menu;
      hideEl(wrap);
      if (wrap !== menu) hideEl(menu);
    }
    var btns = root.querySelectorAll('[data-testid="user-menu-button"][aria-expanded="true"]');
    for (var b = 0; b < btns.length; b++) {
      try { btns[b].setAttribute('aria-expanded', 'false'); } catch (e) {}
    }
  }

  function setPanelName(root) {
    if (!TM_USER || !root || !root.querySelectorAll) return;
    var btns = root.querySelectorAll('[data-testid="user-menu-button"]');
    for (var i = 0; i < btns.length; i++) {
      var btn = btns[i];
      var trail = btn.querySelector('.df-user-menu-trailing');
      if (!trail) continue;
      var nameSpan = trail.querySelector('span.text-secondary');
      if (!nameSpan) {
        var spans = trail.querySelectorAll(':scope > span, span');
        for (var s = 0; s < spans.length; s++) {
          var sp = spans[s];
          if (sp.getAttribute('aria-hidden') === 'true') continue;
          if (sp.classList && sp.classList.contains('df-footer-suffix-text')) continue;
          if (sp.getAttribute('data-cds') === 'Icon') continue;
          nameSpan = sp;
          break;
        }
      }
      if (!nameSpan) continue;
      if (nameSpan.dataset && nameSpan.dataset.tmUserSet === TM_USER) continue;
      nameSpan.textContent = TM_USER;
      if (nameSpan.dataset) nameSpan.dataset.tmUserSet = TM_USER;
      var img = btn.querySelector('img[alt]');
      if (img) img.setAttribute('alt', TM_USER);
    }
  }

  function run() {
    try {
      hideUserMenu(document);
      setPanelName(document);
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
