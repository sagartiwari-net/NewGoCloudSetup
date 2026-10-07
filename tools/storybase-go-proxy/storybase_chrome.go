package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// storybaseChromeScript hides #profile-widget (avatar + name + Settings /
// Billing / Log out) and, if that shell is ever visible, replaces only the
// name inside #sidebar-profile with the panel access-link username.
func storybaseChromeScript(panelUsername string) string {
	user := strings.TrimSpace(panelUsername)
	if user == "" || user == "guest_favicon" || user == "local_dev" {
		user = ""
	}
	userJS, _ := json.Marshal(user)
	return fmt.Sprintf(`<style id="tm-sb-hide-profile-widget">
#profile-widget,
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
  function hideEl(el){
    if (!el || !el.style) return;
    el.style.setProperty("display", "none", "important");
    el.style.setProperty("visibility", "hidden", "important");
    el.style.setProperty("pointer-events", "none", "important");
    el.setAttribute("aria-hidden", "true");
  }
  function hideProfileWidget(root){
    var scope = root || document;
    var ids = ["profile-widget", "profile-widget-navigation"];
    for (var i = 0; i < ids.length; i++) {
      var el = null;
      if (scope.id === ids[i]) el = scope;
      else if (scope.querySelector) el = scope.querySelector("#" + ids[i]);
      hideEl(el);
    }
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
      hideProfileWidget();
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
            if (n && n.nodeType === 1) hideProfileWidget(n);
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
