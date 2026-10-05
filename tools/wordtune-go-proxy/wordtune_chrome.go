package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// wordtuneChromeScript hides ONLY Wordtune avatar-menu items
// (Account / Add to Chrome / Get iOS app / Log out + their separators)
// and replaces the profile email with the panel member username.
func wordtuneChromeScript(panelUsername string) string {
	userJS, _ := json.Marshal(strings.TrimSpace(panelUsername))
	return fmt.Sprintf(`<script data-tm-wordtune-chrome="1">
(function(){
  if (window.__tmWordtuneChrome) return;
  window.__tmWordtuneChrome = true;
  var TM_USER = %s;
  var HIDE_LABELS = {
    "Account": 1,
    "Add to Chrome": 1,
    "Get iOS app": 1,
    "Log out": 1
  };
  function hide(el){
    if (!el || (el.dataset && el.dataset.tmWtHide === "1")) return;
    el.style.setProperty("display","none","important");
    el.style.setProperty("visibility","hidden","important");
    el.style.setProperty("pointer-events","none","important");
    el.setAttribute("aria-hidden","true");
    if (el.dataset) el.dataset.tmWtHide = "1";
  }
  function labelOf(el){
    return String(el && el.textContent || "").replace(/\s+/g," ").trim();
  }
  function looksLikeWordtuneProfileMenu(menu){
    var t = labelOf(menu);
    if (t.length < 20 || t.length > 4000) return false;
    // Require at least two of the hide targets so we do not touch other menus.
    var n = 0;
    if (/Add to Chrome/i.test(t)) n++;
    if (/Get iOS app/i.test(t)) n++;
    if (/\bLog out\b/i.test(t)) n++;
    if (/\bAccount\b/i.test(t)) n++;
    return n >= 2;
  }
  function run(){
    try {
      document.querySelectorAll('[role="menu"]').forEach(function(menu){
        if (!looksLikeWordtuneProfileMenu(menu)) return;

        menu.querySelectorAll('[role="menuitem"]').forEach(function(mi){
          var label = labelOf(mi);
          if (!HIDE_LABELS[label]) return;
          // Parent is the padded wrapper (ds-py-0.5 … px-3) around each menuitem.
          hide(mi.parentElement || mi);
        });

        // Separators between those items only (same profile menu).
        menu.querySelectorAll('[role="separator"]').forEach(hide);
      });

      // Fallback: menuitems may render outside [role=menu] briefly.
      document.querySelectorAll('[role="menuitem"]').forEach(function(mi){
        var label = labelOf(mi);
        if (!HIDE_LABELS[label]) return;
        hide(mi.parentElement || mi);
      });

      if (TM_USER) {
        document.querySelectorAll('[aria-label="User details"]').forEach(function(box){
          var col = box.querySelector(".flex.min-w-0.flex-col");
          var emailP = col ? col.querySelector(":scope > p") : null;
          if (!emailP) {
            // Fallback: first truncated primary line that looks like an email.
            var ps = box.querySelectorAll("p");
            for (var i = 0; i < ps.length; i++) {
              var txt = labelOf(ps[i]);
              if (txt.indexOf("@") !== -1 || /subtitle3/i.test(ps[i].className || "")) {
                emailP = ps[i];
                break;
              }
            }
          }
          if (emailP && emailP.textContent !== TM_USER) {
            emailP.textContent = TM_USER;
          }
        });
      }
    } catch (e) {}
  }
  run();
  try {
    new MutationObserver(run).observe(document.documentElement, {childList:true, subtree:true});
  } catch (e2) {}
  setInterval(run, 800);
})();
</script>`, userJS)
}
