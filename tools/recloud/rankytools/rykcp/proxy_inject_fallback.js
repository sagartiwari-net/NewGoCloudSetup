(function(){
  window.__ranky_origFetch = window.fetch.bind(window);
  window.__RANKY_USER = {"uid":0,"username":"","name":"User"};
  var base = "__TM_ORIGIN__";
  var domains = ['https://chatgpt.com','https://cdn.oaistatic.com','https://auth0.openai.com','https://chat.openai.com'];
  function isUpstream(u){ try{ var o = new URL(u, location.href); return domains.some(function(d){return o.href.indexOf(d)===0;}); }catch(e){return false;} }
  function rewrite(u){
    try{
      if(typeof window.__rtRewriteSandboxUrl === 'function'){
        var sb = window.__rtRewriteSandboxUrl(u);
        if(sb !== u) return sb;
      }
      var o = new URL(u, location.href);
      return base + o.pathname + o.search + o.hash;
    }catch(e){return u;}
  }
  var _fetch = window.__ranky_origFetch;
  window.fetch = function(input, init){
    try{
      var url = typeof input === 'string' ? input : input.url;
      if(/\/backend-api\/accounts\/[^/]+\/invites/i.test(String(url||''))){
        var m = ((init&&init.method)||(input&&input.method)||'GET').toUpperCase();
        if(m==='GET'||m==='HEAD') return Promise.resolve(new Response('{"items":[],"invites":[],"total":0}',{status:200,headers:{'content-type':'application/json'}}));
        return Promise.resolve(new Response('{"error":"forbidden"}',{status:403,headers:{'content-type':'application/json'}}));
      }
      if(isUpstream(url)){
        if(typeof input === 'string') input = rewrite(url);
        else input = new Request(rewrite(url), input);
      }
    }catch(e){}
    return _fetch(input, init);
  };
  (function(){
    var OldXHR = window.XMLHttpRequest;
    function NewXHR(){
      var xhr = new OldXHR();
      var _open = xhr.open;
      xhr.open = function(method, url){
        try{ if(isUpstream(url)) url = rewrite(url); }catch(e){}
        var args = Array.prototype.slice.call(arguments);
        args[1] = url;
        return _open.apply(this, args);
      };
      return xhr;
    }
    NewXHR.prototype = OldXHR.prototype;
    window.XMLHttpRequest = NewXHR;
  })();

  console.log('[Proxy] Secondary injector active (v5.8)');

  var css = document.createElement('style'); css.id = 'ranky-css';
  css.textContent = `
    button[data-testid="start-group-chat-from-conversation-button"]{display:none!important}
    [class*="px-3.5"][class*="pt-1"][class*="pb-3"]{display:none!important;visibility:hidden!important;height:0!important;overflow:hidden!important;pointer-events:none!important;margin:0!important;padding:0!important}
    [data-testid="accounts-profile-button"]{display:none!important}
    .__menu-item-trailing-btn{display:none!important}
    .__menu-item:has(svg.lucide-sparkle),.__menu-item:has([aria-label="Upgrade"]),.__menu-item:has(.btn-secondary){display:none!important}
    .__menu-item .rounded-full .text-xs{display:none!important}
    [aria-label="Open profile menu"],nav .sticky.bottom-0,#history{display:none!important;visibility:hidden!important;height:0!important;overflow:hidden!important;opacity:0!important;pointer-events:none!important;position:absolute!important;left:-9999px!important}
    a[href="/codex/cloud"],a[href^="/codex/cloud"]{display:none!important;visibility:hidden!important;height:0!important;overflow:hidden!important;pointer-events:none!important;position:absolute!important;left:-9999px!important}
    aside.app-shell-left-panel,aside[class*="app-shell-left-panel"],aside[class*="LeftPanel"],#app-shell-sidebar,nav[aria-label="App navigation"],[data-app-navigation-rail]{display:none!important;visibility:hidden!important;width:0!important;min-width:0!important;max-width:0!important;overflow:hidden!important;opacity:0!important;pointer-events:none!important;position:absolute!important;left:-9999px!important}
    .ranky-brand{position:fixed!important;bottom:15px!important;right:15px!important;color:#888!important;font-size:11px!important;z-index:999999!important}
    .ranky-brand a{color:#10a37f!important;font-weight:bold!important;text-decoration:none!important}
  `;
  document.head.appendChild(css);
  var _hiding=false;
  var _textScanAt=0;
  var hs='display:none!important;visibility:hidden!important;width:0!important;min-width:0!important;max-width:0!important;height:0!important;overflow:hidden!important;opacity:0!important;position:absolute!important;left:-99999px!important;top:-99999px!important;pointer-events:none!important;';
  function hideEl(el){
    if(!el||el.id==='ranky-sidebar'||(el.closest&&el.closest('#ranky-sidebar')))return;
    if(el.getAttribute&&el.getAttribute('data-rt-hid')==='1')return;
    try{ el.setAttribute('data-rt-hid','1'); }catch(e){}
    el.style.cssText=hs;
  }
  function hideElements(){if(_hiding)return;if(!document.body)return;_hiding=true;try{
    document.querySelectorAll('#__next nav,#__next>div>div>nav,#__next>div>nav').forEach(function(el){hideEl(el);});
    document.querySelectorAll('nav[aria-label="Sidebar"],nav[aria-label="Chat history"],#stage-slideover-sidebar,#thread-left-sidebar').forEach(function(el){hideEl(el);});
    document.querySelectorAll('aside.app-shell-left-panel,aside[class*="app-shell-left-panel"],aside[class*="LeftPanel"],#app-shell-sidebar,nav[aria-label="App navigation"],[data-app-navigation-rail],[data-app-action-sidebar-scroll]').forEach(function(el){if(el.closest&&el.closest('#ranky-sidebar'))return;hideEl(el);var w=el.querySelector&&el.querySelector('div[style*="min-width"],div[style*="width"]');if(w&&!(w.closest&&w.closest('#ranky-sidebar'))){if(w.getAttribute('data-rt-hid')!=='1'){w.setAttribute('data-rt-hid','1');w.style.cssText='width:0!important;min-width:0!important;max-width:0!important;overflow:hidden!important';}}});
    document.querySelectorAll('nav[aria-label="Scheduled task folders"]').forEach(function(el){if(el.closest&&el.closest('#ranky-sidebar'))return;var comp=(el.closest&&el.closest('[role="complementary"]'));if(comp)hideEl(comp);hideEl(el);});
    document.querySelectorAll('[role="complementary"]').forEach(function(el){if(el.id==='ranky-sidebar')return;if(el.closest&&el.closest('#ranky-sidebar'))return;if(el.querySelector&&el.querySelector('#ranky-sidebar'))return;var isLeft=el.querySelector('nav[aria-label="Chat history"],nav[aria-label="Sidebar"],nav[aria-label="App navigation"],#app-shell-sidebar,[data-app-navigation-rail],[data-app-action-sidebar-scroll]');if(!isLeft)return;hideEl(el);});
    document.querySelectorAll('button[aria-label="Open sidebar"],button[aria-label="Close sidebar"],button[data-testid="open-sidebar-button"],button[data-testid="close-sidebar-button"]').forEach(function(el){hideEl(el);});
    var _now=Date.now();
    if(_now-_textScanAt>8000){ _textScanAt=_now;
    document.querySelectorAll('[data-testid="accounts-profile-button"]').forEach(function(el){el.style.cssText=hs;});
    document.querySelectorAll('[aria-label="Open profile menu"]').forEach(function(el){el.style.cssText=hs;});
    document.querySelectorAll('nav .sticky').forEach(function(el){el.style.cssText=hs;});
    /* ranky-move-savectx: move native "Save context" button to top-right */
    document.querySelectorAll('button').forEach(function(b){
      var t=((b.textContent||'').replace(/s+/g,' ')).trim();
      var al=((b.getAttribute&&(b.getAttribute('aria-label')||''))||'');
      if(t==='Save context'||al==='Save context'){
        if(b.closest&&b.closest('#ranky-sidebar'))return;
        b.style.setProperty('position','fixed','important');
        b.style.setProperty('top','8px','important');
        b.style.setProperty('right','70px','important');
        b.style.setProperty('left','auto','important');
        b.style.setProperty('bottom','auto','important');
        b.style.setProperty('margin','0','important');
        b.style.setProperty('z-index','2147481000','important');
      }
    });
    document.querySelectorAll('a[href*="upgrade"]').forEach(function(el){el.style.cssText=hs;});
    document.querySelectorAll('a[href="/codex/cloud"],a[href^="/codex/cloud"]').forEach(function(el){el.style.cssText=hs;});
    var h=document.getElementById('history');if(h)h.style.cssText=hs;
    document.querySelectorAll('.__menu-item').forEach(function(el){var sp=el.querySelector('span');if(sp&&sp.textContent.trim()==='Recent files')el.style.cssText=hs;});
    document.querySelectorAll('.__menu-item').forEach(function(el){if((el.textContent||'').trim()==='Recent files')el.style.cssText=hs;});
    document.querySelectorAll('button,span').forEach(function(el){var t=((el.textContent)||'').replace(/s+/g,' ').trim(); if(t.length>90)return; if(t==='Invite team members'||t.indexOf('Invite members to')===0){var b=el.closest('button'); b=(b&&b.parentElement)||el.closest('[role="dialog"]')||el.parentElement; if(b)b.style.cssText=hs;}});
    }
    
  }finally{_hiding=false;}}
  hideElements();
  var _hideTimer=null;
  function scheduleHide(){
    if(_hideTimer)return;
    _hideTimer=setTimeout(function(){_hideTimer=null; hideElements();}, 1200);
  }
  new MutationObserver(function(){scheduleHide();}).observe(document.body||document.documentElement,{childList:true,subtree:true});
  setInterval(scheduleHide,4000);
  // Block client-side navigation to restricted paths
  var _pushState = history.pushState;
  history.pushState = function(state, title, url){
    if(typeof url === "string" && (/^\/library/i.test(url) || /^\/images/i.test(url))) { window.location.href = "/"; return; }
    return _pushState.apply(this, arguments);
  };
  var _replaceState = history.replaceState;
  history.replaceState = function(state, title, url){
    if(typeof url === "string" && (/^\/library/i.test(url) || /^\/images/i.test(url))) { window.location.href = "/"; return; }
    return _replaceState.apply(this, arguments);
  };
  window.addEventListener("popstate", function(){ if(/^\/library|^\/images/i.test(location.pathname)) window.location.href = "/"; });
})();