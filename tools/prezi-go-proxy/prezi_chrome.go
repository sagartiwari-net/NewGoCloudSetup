package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// preziChromeScript shows the panel access-link username on Prezi Name
// labels, blocks opening the account dropdown, and hides only the
// UserDropdown popup (Account settings / Invite / Admin / Log out).
func preziChromeScript(panelUsername string) string {
	user := strings.TrimSpace(panelUsername)
	if user == "" || user == "guest_favicon" || user == "local_dev" {
		user = ""
	}
	userJS, _ := json.Marshal(user)
	return fmt.Sprintf(`<style id="tm-prezi-hide-user-dropdown">
[class*="UserDropdown__UserDropdownList"],
[class*="Popup__Container"]:has([class*="UserDropdown__UserDropdownList"]){
  display:none!important;
  visibility:hidden!important;
  pointer-events:none!important;
  opacity:0!important;
  height:0!important;
  overflow:hidden!important;
}
</style>
<script data-tm-prezi-chrome="1">
(function(){
  if (window.__tmPreziChrome) return;
  window.__tmPreziChrome = true;
  var TM_USER = %s;
  function isNameEl(el){
    if (!el || !el.className || typeof el.className !== "string") return false;
    return el.className.indexOf("Name-sc-") !== -1;
  }
  function isUserDropdownPopup(el){
    if (!el || el.nodeType !== 1) return false;
    var cls = (el.className && typeof el.className === "string") ? el.className : "";
    if (cls.indexOf("UserDropdown__UserDropdownList") !== -1) return true;
    if (cls.indexOf("Popup__Container") !== -1 && el.querySelector && el.querySelector('[class*="UserDropdown__UserDropdownList"]')) return true;
    return false;
  }
  function hideUserDropdown(root){
    var scope = root || document;
    if (!scope.querySelectorAll) {
      if (isUserDropdownPopup(scope)) hideEl(scope);
      return;
    }
    scope.querySelectorAll('[class*="UserDropdown__UserDropdownList"]').forEach(function(list){
      hideEl(list);
      var popup = list.closest('[class*="Popup__Container"]');
      if (popup) hideEl(popup);
    });
    scope.querySelectorAll('[class*="Popup__Container"]').forEach(function(popup){
      if (isUserDropdownPopup(popup)) hideEl(popup);
    });
  }
  function hideEl(el){
    if (!el || !el.style) return;
    el.style.setProperty("display", "none", "important");
    el.style.setProperty("visibility", "hidden", "important");
    el.style.setProperty("pointer-events", "none", "important");
    el.style.setProperty("opacity", "0", "important");
    el.setAttribute("aria-hidden", "true");
  }
  function setPanelNames(){
    if (!TM_USER) return;
    document.querySelectorAll('[class*="Name-sc-"]').forEach(function(el){
      if (!isNameEl(el)) return;
      if (el.dataset.tmUser === TM_USER) return;
      el.textContent = TM_USER;
      el.dataset.tmUser = TM_USER;
    });
  }
  function blockOpen(e){
    hideUserDropdown();
    if (e) {
      e.preventDefault();
      e.stopPropagation();
      if (e.stopImmediatePropagation) e.stopImmediatePropagation();
    }
    return false;
  }
  function wireName(el){
    if (!isNameEl(el) || el.dataset.tmPreziBound === "1") return;
    el.dataset.tmPreziBound = "1";
    ["click","mousedown","mouseup","pointerdown","touchstart","keydown"].forEach(function(ev){
      el.addEventListener(ev, blockOpen, true);
    });
    var btn = el.closest("button, [role=button], a");
    if (btn && btn.dataset.tmPreziBound !== "1") {
      btn.dataset.tmPreziBound = "1";
      ["click","mousedown","mouseup","pointerdown","touchstart","keydown"].forEach(function(ev){
        btn.addEventListener(ev, blockOpen, true);
      });
    }
  }
  function run(){
    try {
      hideUserDropdown();
      setPanelNames();
      document.querySelectorAll('[class*="Name-sc-"]').forEach(wireName);
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
            if (n && n.nodeType === 1) hideUserDropdown(n);
          }
        }
      }
      setPanelNames();
      document.querySelectorAll('[class*="Name-sc-"]').forEach(wireName);
    }).observe(document.documentElement, { childList: true, subtree: true });
  } catch (e2) {}
  setInterval(run, 2000);
})();
</script>`, string(userJS))
}
