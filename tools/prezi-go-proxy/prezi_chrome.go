package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// preziChromeScript shows the panel access-link username on Prezi Name
// labels, blocks opening the account dropdown (including parent/avatar
// clicks), and removes only the UserDropdown popup.
func preziChromeScript(panelUsername string) string {
	user := strings.TrimSpace(panelUsername)
	if user == "" || user == "guest_favicon" || user == "local_dev" {
		user = ""
	}
	userJS, _ := json.Marshal(user)
	return fmt.Sprintf(`<style id="tm-prezi-hide-user-dropdown">
[class*="UserDropdown__UserDropdownList"],
[class*="Popup__Container"]:has([class*="UserDropdown__UserDropdownList"]),
[class*="Popup__Container"]:has(a[href*="/settings"]),
[class*="Popup__Container"]:has(a[href*="/organizations/manage"]){
  display:none!important;
  visibility:hidden!important;
  pointer-events:none!important;
  opacity:0!important;
  height:0!important;
  max-height:0!important;
  overflow:hidden!important;
}
</style>
<script data-tm-prezi-chrome="1">
(function(){
  if (window.__tmPreziChrome) return;
  window.__tmPreziChrome = true;
  var TM_USER = %s;
  function cls(el){
    if (!el) return "";
    if (typeof el.className === "string") return el.className;
    if (el.classList && el.classList.value) return el.classList.value;
    try { return String(el.getAttribute("class") || ""); } catch (e) { return ""; }
  }
  function hasCls(el, part){
    return cls(el).indexOf(part) !== -1;
  }
  function isNameEl(el){
    return !!(el && el.tagName === "DIV" && hasCls(el, "Name-sc-"));
  }
  function hideEl(el){
    if (!el || !el.style) return;
    el.style.setProperty("display", "none", "important");
    el.style.setProperty("visibility", "hidden", "important");
    el.style.setProperty("pointer-events", "none", "important");
    el.style.setProperty("opacity", "0", "important");
    el.style.setProperty("height", "0", "important");
    el.setAttribute("aria-hidden", "true");
  }
  function killUserDropdown(root){
    var scope = root && root.querySelectorAll ? root : document;
    var lists = scope.querySelectorAll('[class*="UserDropdown__UserDropdownList"]');
    for (var i = 0; i < lists.length; i++) {
      var list = lists[i];
      var popup = list.closest ? list.closest('[class*="Popup__Container"]') : null;
      hideEl(list);
      if (popup) {
        hideEl(popup);
        try { popup.remove(); } catch (e1) {}
      } else {
        try { list.remove(); } catch (e2) {}
      }
    }
    // Fallback: popups that contain account settings / logout links
    var pops = scope.querySelectorAll('[class*="Popup__Container"]');
    for (var j = 0; j < pops.length; j++) {
      var p = pops[j];
      if (!p.querySelector) continue;
      if (p.querySelector('a[href*="/settings"], a[href*="/organizations/manage"], [class*="UserDropdown__UserDropdownList"]')) {
        hideEl(p);
        try { p.remove(); } catch (e3) {}
      }
    }
  }
  function setPanelNames(){
    if (!TM_USER) return;
    var nodes = document.querySelectorAll('[class*="Name-sc-"]');
    for (var i = 0; i < nodes.length; i++) {
      var el = nodes[i];
      if (!isNameEl(el)) continue;
      if (el.textContent !== TM_USER) el.textContent = TM_USER;
      el.dataset.tmUser = TM_USER;
    }
  }
  function isUserMenuEventTarget(t){
    if (!t || t.nodeType !== 1) {
      if (t && t.parentElement) t = t.parentElement;
      else return false;
    }
    var n = t;
    for (var depth = 0; depth < 10 && n; depth++) {
      if (isNameEl(n)) return true;
      if (hasCls(n, "UserDropdown")) return true;
      // Profile cluster: contains a Name-sc label (avatar + name + org)
      try {
        if (n.querySelector && n.querySelector('[class*="Name-sc-"]')) {
          // Avoid treating huge page roots as the menu
          if (n !== document.body && n !== document.documentElement && n.tagName !== "MAIN") {
            var names = n.querySelectorAll('[class*="Name-sc-"]');
            if (names.length > 0 && names.length < 6) return true;
          }
        }
      } catch (e) {}
      n = n.parentElement;
    }
    return false;
  }
  function blockOpen(e){
    killUserDropdown();
    if (e) {
      e.preventDefault();
      e.stopPropagation();
      if (e.stopImmediatePropagation) e.stopImmediatePropagation();
    }
    return false;
  }
  ["click","mousedown","mouseup","pointerdown","touchstart","keydown"].forEach(function(ev){
    document.addEventListener(ev, function(e){
      if (isUserMenuEventTarget(e.target)) blockOpen(e);
    }, true);
  });
  function run(){
    try {
      killUserDropdown();
      setPanelNames();
    } catch (e) {}
  }
  run();
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", run);
  try {
    new MutationObserver(function(){ run(); }).observe(document.documentElement, {
      childList: true,
      subtree: true,
      characterData: true
    });
  } catch (e2) {}
  setInterval(run, 400);
})();
</script>`, string(userJS))
}
