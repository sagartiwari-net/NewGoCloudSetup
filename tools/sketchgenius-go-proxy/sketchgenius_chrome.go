package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// sketchgeniusChromeScript shows the panel access-link username on the
// Bootstrap nav dropdown toggle (e.g. "Abir Yousif") and blocks the
// account dropdown from opening.
func sketchgeniusChromeScript(panelUsername string) string {
	user := strings.TrimSpace(panelUsername)
	if user == "" || user == "guest_favicon" || user == "local_dev" {
		user = ""
	}
	userJS, _ := json.Marshal(user)
	return fmt.Sprintf(`<style id="tm-sg-hide-dropdown">
a.nav-link.dropdown-toggle + .dropdown-menu,
.nav-item.dropdown > .dropdown-menu,
.navbar .dropdown-menu.show,
.dropdown-menu.show{
  display:none!important;
  visibility:hidden!important;
  pointer-events:none!important;
  opacity:0!important;
}
</style>
<script data-tm-sg-chrome="1">
(function(){
  if (window.__tmSgChrome) return;
  window.__tmSgChrome = true;
  var TM_USER = %s;
  function isUserToggle(a){
    if (!a || !a.classList) return false;
    if (!a.classList.contains("nav-link") || !a.classList.contains("dropdown-toggle")) return false;
    if (a.getAttribute("data-toggle") === "dropdown") return true;
    if (a.getAttribute("data-bs-toggle") === "dropdown") return true;
    return !!a.closest(".nav-item.dropdown, .dropdown");
  }
  function hideMenus(root){
    var scope = root || document;
    scope.querySelectorAll(".dropdown-menu").forEach(function(menu){
      var parent = menu.parentElement;
      if (!parent) return;
      var toggle = parent.querySelector("a.nav-link.dropdown-toggle, a.dropdown-toggle");
      if (!toggle || !isUserToggle(toggle)) return;
      menu.classList.remove("show");
      menu.style.setProperty("display", "none", "important");
      menu.style.setProperty("visibility", "hidden", "important");
      menu.setAttribute("aria-hidden", "true");
    });
  }
  function blockOpen(e){
    hideMenus();
    if (e) {
      e.preventDefault();
      e.stopPropagation();
      if (e.stopImmediatePropagation) e.stopImmediatePropagation();
    }
    return false;
  }
  function wireToggle(a){
    if (!isUserToggle(a) || a.dataset.tmSgBound === "1") return;
    a.dataset.tmSgBound = "1";
    a.removeAttribute("data-toggle");
    a.removeAttribute("data-bs-toggle");
    a.setAttribute("aria-expanded", "false");
    a.classList.remove("show");
    ["click","mousedown","mouseup","pointerdown","touchstart","keydown"].forEach(function(ev){
      a.addEventListener(ev, blockOpen, true);
    });
  }
  function setPanelUser(){
    if (!TM_USER) return;
    document.querySelectorAll("a.nav-link.dropdown-toggle").forEach(function(a){
      if (!isUserToggle(a)) return;
      wireToggle(a);
      if (a.dataset.tmUser === TM_USER) return;
      // Prefer direct text (not nested icons) — replace child text nodes / whole label.
      var hasElementChild = false;
      for (var i = 0; i < a.childNodes.length; i++) {
        if (a.childNodes[i].nodeType === 1) { hasElementChild = true; break; }
      }
      if (!hasElementChild) {
        a.textContent = TM_USER;
      } else {
        var set = false;
        for (var j = 0; j < a.childNodes.length; j++) {
          var n = a.childNodes[j];
          if (n.nodeType === 3 && n.textContent.replace(/\s+/g, "").length) {
            n.textContent = " " + TM_USER + " ";
            set = true;
          }
        }
        if (!set) a.textContent = TM_USER;
      }
      a.dataset.tmUser = TM_USER;
      a.setAttribute("aria-expanded", "false");
      a.classList.remove("show");
      var parent = a.parentElement;
      if (parent) parent.classList.remove("show");
    });
  }
  function run(){
    try {
      hideMenus();
      setPanelUser();
      document.querySelectorAll("a.nav-link.dropdown-toggle").forEach(wireToggle);
    } catch (e) {}
  }
  run();
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", run);
  try {
    new MutationObserver(function(){ run(); }).observe(document.documentElement, { childList: true, subtree: true });
  } catch (e2) {}
  setInterval(run, 2000);
})();
</script>`, string(userJS))
}
