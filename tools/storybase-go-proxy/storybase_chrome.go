package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// storybaseChromeScript hides #profile-widget-navigation (Settings /
// Billing / Log out) and replaces only the name inside #sidebar-profile
// (not the avatar) with the panel access-link username.
func storybaseChromeScript(panelUsername string) string {
	user := strings.TrimSpace(panelUsername)
	if user == "" || user == "guest_favicon" || user == "local_dev" {
		user = ""
	}
	userJS, _ := json.Marshal(user)
	return fmt.Sprintf(`<style id="tm-sb-hide-profile-nav">
#profile-widget-navigation{
  display:none!important;
  visibility:hidden!important;
  pointer-events:none!important;
  opacity:0!important;
  height:0!important;
  overflow:hidden!important;
}
</style>
<script data-tm-sb-chrome="1">
(function(){
  if (window.__tmSbChrome) return;
  window.__tmSbChrome = true;
  var TM_USER = %s;
  function hideProfileNav(root){
    var scope = root || document;
    var nav = scope.querySelector ? scope.querySelector("#profile-widget-navigation") : null;
    if (!nav && scope.id === "profile-widget-navigation") nav = scope;
    if (!nav) return;
    nav.style.setProperty("display", "none", "important");
    nav.style.setProperty("visibility", "hidden", "important");
    nav.style.setProperty("pointer-events", "none", "important");
    nav.setAttribute("aria-hidden", "true");
  }
  function setSidebarName(){
    if (!TM_USER) return;
    var wrap = document.getElementById("sidebar-profile");
    if (!wrap) return;
    var kids = wrap.children;
    for (var i = 0; i < kids.length; i++) {
      var el = kids[i];
      if (!el || el.id === "sidebar-avatar") continue;
      if (el.tagName !== "DIV") continue;
      if (el.dataset.tmUser === TM_USER) return;
      el.textContent = TM_USER;
      el.dataset.tmUser = TM_USER;
      return;
    }
  }
  function run(){
    try {
      hideProfileNav();
      setSidebarName();
    } catch (e) {}
  }
  run();
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", run);
  try {
    new MutationObserver(function(muts){
      for (var i = 0; i < muts.length; i++) {
        var m = muts[i];
        if (m.type === "childList") {
          for (var j = 0; j < m.addedNodes.length; j++) {
            var n = m.addedNodes[j];
            if (n && n.nodeType === 1) hideProfileNav(n);
          }
        }
      }
      setSidebarName();
    }).observe(document.documentElement, { childList: true, subtree: true });
  } catch (e2) {}
  setInterval(run, 2000);
})();
</script>`, string(userJS))
}
