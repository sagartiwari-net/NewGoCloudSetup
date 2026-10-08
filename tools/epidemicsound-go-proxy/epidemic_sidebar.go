package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// epidemicSidebarScript keeps the sidebar account button from opening its
// menu, and replaces only the email line inside that button with the panel
// access-link username. The Pro line and every other control stay as they are.
func epidemicSidebarScript(panelUsername string) string {
	user := strings.TrimSpace(panelUsername)
	if user == "" || user == "guest_favicon" || user == "local_dev" || user == "unknown" {
		user = ""
	}
	userJS, _ := json.Marshal(user)
	return fmt.Sprintf(`<style data-es-sidebar-lock>
button[class*="_sidebarBoxButton_"]{
  pointer-events:none!important;
}
</style>
<script data-es-sidebar-lock="1">
(function(){
  if (window.__tmEsSidebar) return;
  window.__tmEsSidebar = true;
  var TM_USER = %s;
  function accountButton(){
    return document.querySelector('button[class*="_sidebarBoxButton_"]');
  }
  function lock(btn){
    if (!btn) return;
    btn.style.setProperty("pointer-events", "none", "important");
    if (btn.getAttribute("aria-expanded") === "true") btn.setAttribute("aria-expanded", "false");
    if (!TM_USER) return;
    var info = btn.querySelector('[class*="_sidebarFooterUserInfo_"]');
    if (!info) return;
    var email = info.querySelector("p.es-text-button-xs");
    if (!email || email.dataset.tmUser === TM_USER) return;
    email.textContent = TM_USER;
    email.dataset.tmUser = TM_USER;
  }
  function run(){ try { lock(accountButton()); } catch (e) {} }
  run();
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", run);
  try {
    new MutationObserver(run).observe(document.documentElement, {childList:true, subtree:true, characterData:true});
  } catch (e2) {}
  document.addEventListener("pointerdown", function(e){
    var t = e.target;
    if (!t || !t.closest) return;
    if (t.closest('button[class*="_sidebarBoxButton_"]')) {
      e.preventDefault();
      e.stopPropagation();
    }
  }, true);
  setInterval(run, 1000);
})();
</script>`, string(userJS))
}
