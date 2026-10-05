package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// sscChromeScript hides ONLY the Ant user dropdown items
// (My account / Billing / Team Members / Logout) and shows the panel username
// in .user-email / span.email instead of the premium account email.
func sscChromeScript(panelUsername string) string {
	userJS, _ := json.Marshal(strings.TrimSpace(panelUsername))
	return fmt.Sprintf(`<script data-tm-ssc-chrome="1">
(function(){
  if (window.__tmSscChrome) return;
  window.__tmSscChrome = true;
  var TM_USER = %s;
  var HIDE = {
    'my account': 1,
    'billing': 1,
    'team members': 1,
    'logout': 1,
    'log out': 1
  };
  function hide(el){
    if (!el || (el.dataset && el.dataset.tmSscHide === '1')) return;
    el.style.setProperty('display','none','important');
    el.style.setProperty('visibility','hidden','important');
    el.setAttribute('aria-hidden','true');
    if (el.dataset) el.dataset.tmSscHide = '1';
  }
  function norm(s){
    return (s || '').replace(/\s+/g,' ').trim().toLowerCase();
  }
  function run(){
    try {
      // Exact Ant Design user dropdown items only — do not touch other menus.
      document.querySelectorAll('.ant-dropdown .ant-dropdown-menu-item, .ant-dropdown-menu .ant-dropdown-menu-item').forEach(function(li){
        var t = norm(li.textContent);
        if (HIDE[t]) hide(li);
      });
      document.querySelectorAll('.ant-dropdown .ant-dropdown-menu-item-divider').forEach(function(div){
        var menu = div.parentElement;
        if (!menu) return;
        var items = menu.querySelectorAll('.ant-dropdown-menu-item');
        var hit = 0;
        items.forEach(function(li){ if (HIDE[norm(li.textContent)]) hit++; });
        if (hit >= 2) hide(div);
      });

      if (TM_USER) {
        document.querySelectorAll('div.user-email, span.email, .user-email').forEach(function(el){
          if (el.textContent !== TM_USER) el.textContent = TM_USER;
        });
      }
    } catch (e) {}
  }
  run();
  try {
    new MutationObserver(run).observe(document.documentElement, {childList:true, subtree:true});
  } catch (e2) {}
  setInterval(run, 1000);
})();
</script>`, userJS)
}
