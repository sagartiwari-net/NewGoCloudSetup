package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// rewriteClaudeBootstrapNames replaces Claude account display-name fields with the
// panel access username so the SPA renders the right label from bootstrap.
// Only the account object is re-encoded; the rest of the bootstrap is left intact.
func rewriteClaudeBootstrapNames(full []byte, panelUser string) []byte {
	panelUser = strings.TrimSpace(panelUser)
	if panelUser == "" || len(full) == 0 {
		return full
	}
	keyIdx := bytes.Index(full, []byte(`"account"`))
	if keyIdx < 0 {
		return full
	}
	colon := bytes.IndexByte(full[keyIdx:], ':')
	if colon < 0 {
		return full
	}
	start := keyIdx + colon + 1
	for start < len(full) && (full[start] == ' ' || full[start] == '\n' || full[start] == '\t' || full[start] == '\r') {
		start++
	}
	if start >= len(full) || full[start] == 'n' { // null
		return full
	}
	dec := json.NewDecoder(bytes.NewReader(full[start:]))
	var accRaw json.RawMessage
	if err := dec.Decode(&accRaw); err != nil || len(accRaw) == 0 {
		return full
	}
	end := start + int(dec.InputOffset())

	var acc map[string]interface{}
	if err := json.Unmarshal(accRaw, &acc); err != nil {
		return full
	}
	changed := applyPanelDisplayName(acc, panelUser)
	if userObj, ok := acc["user"].(map[string]interface{}); ok {
		if applyPanelDisplayName(userObj, panelUser) {
			changed = true
		}
	}
	if profile, ok := acc["profile"].(map[string]interface{}); ok {
		if applyPanelDisplayName(profile, panelUser) {
			changed = true
		}
	}
	if !changed {
		return full
	}
	newAcc, err := json.Marshal(acc)
	if err != nil {
		return full
	}
	out := make([]byte, 0, len(full)-len(accRaw)+len(newAcc))
	out = append(out, full[:start]...)
	out = append(out, newAcc...)
	out = append(out, full[end:]...)
	return out
}

func applyPanelDisplayName(m map[string]interface{}, panelUser string) bool {
	if m == nil {
		return false
	}
	changed := false
	for _, key := range []string{
		"display_name", "displayName",
		"full_name", "fullName",
		"name", "nickname", "preferred_name", "preferredName",
		"first_name", "firstName",
	} {
		v, ok := m[key]
		if !ok {
			continue
		}
		s, ok := v.(string)
		if !ok || strings.TrimSpace(s) == "" {
			continue
		}
		if s == panelUser {
			continue
		}
		m[key] = panelUser
		changed = true
	}
	return changed
}

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

  function nameSpans(root) {
    var out = [];
    if (!root || !root.querySelectorAll) return out;
    var nodes = root.querySelectorAll(
      '[data-testid="user-menu-button"] span.whitespace-nowrap.text-secondary,' +
      '[data-testid="user-menu-button"] .df-user-menu-trailing span.text-secondary,' +
      '[data-testid="user-menu-button"] .df-user-menu-trailing > span.whitespace-nowrap'
    );
    for (var i = 0; i < nodes.length; i++) out.push(nodes[i]);
    return out;
  }

  function setPanelName(root) {
    if (!TM_USER || !root) return;
    var spans = nameSpans(root);
    for (var i = 0; i < spans.length; i++) {
      var el = spans[i];
      if (!el) continue;
      // Always re-apply — React often restores the upstream name after our first write.
      if (el.textContent !== TM_USER) el.textContent = TM_USER;
    }
    var btns = root.querySelectorAll ? root.querySelectorAll('[data-testid="user-menu-button"]') : [];
    for (var b = 0; b < btns.length; b++) {
      var img = btns[b].querySelector('img[alt]');
      if (img && img.getAttribute('alt') !== TM_USER) img.setAttribute('alt', TM_USER);
    }
  }

  function loadUser(done) {
    if (TM_USER) { done(); return; }
    try {
      fetch('/api/user-limits', { credentials: 'same-origin', cache: 'no-store' })
        .then(function(r){ return r.json(); })
        .then(function(d){
          if (d && d.username) TM_USER = String(d.username).trim();
          done();
        })
        .catch(function(){ done(); });
    } catch (e) { done(); }
  }

  function run() {
    try {
      hideUserMenu(document);
      setPanelName(document);
    } catch (e) {}
  }

  loadUser(function(){
    run();
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', run);
    try {
      new MutationObserver(function(){ run(); }).observe(document.documentElement, {
        childList: true, subtree: true, characterData: true
      });
    } catch (e) {}
    setInterval(run, 400);
  });
})();
</script>`, string(userJS))
}
