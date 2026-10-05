package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// sscChromeScript hides ONLY the Ant user dropdown items
// (My account / Billing / Team Members / Logout) and shows the panel username
// in the header account email slot — never generic span.email (breaks SPA).
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
    // display:none only — never visibility:hidden (can blank whole trees / fight SPA).
    el.style.setProperty('display','none','important');
    el.setAttribute('aria-hidden','true');
    if (el.dataset) el.dataset.tmSscHide = '1';
  }
  function norm(s){
    return (s || '').replace(/\s+/g,' ').trim().toLowerCase();
  }
  function run(){
    try {
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
        // Narrow selectors only — bare "span.email" / ".user-email" rewrites half the app.
        var sel = [
          '.header-account-triggers .user-email',
          '.header-account-triggers span.email',
          '.ant-dropdown-trigger .user-email',
          '.ant-dropdown-trigger span.email',
          'div.header-account-triggers div.user-email'
        ].join(',');
        document.querySelectorAll(sel).forEach(function(el){
          if (el.children && el.children.length) return;
          if (el.textContent !== TM_USER) el.textContent = TM_USER;
        });
      }
    } catch (e) {}
  }
  var scheduled = 0;
  function schedule(){
    if (scheduled) return;
    scheduled = setTimeout(function(){ scheduled = 0; run(); }, 400);
  }
  run();
  try {
    new MutationObserver(schedule).observe(document.documentElement, {childList:true, subtree:true});
  } catch (e2) {}
  setInterval(run, 5000);
})();
</script>`, userJS)
}
