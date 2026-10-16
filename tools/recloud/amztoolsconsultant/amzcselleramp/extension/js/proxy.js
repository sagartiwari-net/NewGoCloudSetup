!function(){
  const MSG="refresh_amember_session";
  let loaded=false;

  function extensionAlive(){
    try{return!!chrome.runtime?.id}catch(e){return false}
  }

  function tryContextReload(){
    try{
      if(sessionStorage.getItem("proxyContextReloaded")==="1")return false;
      sessionStorage.setItem("proxyContextReloaded","1");
      window.location.reload();
      return true;
    }catch(e){return false}
  }

  function showRefresh(){
    const auth=document.querySelector("#authMessage");
    const icon=document.querySelector("#authIcon");
    const title=document.querySelector("#authTitle");
    const copy=document.querySelector("#authCopy");
    const link=document.querySelector("#authLink");
    const note=document.querySelector("#authNote");
    const frame=document.querySelector("#appFrame");
    if(frame){frame.style.display="none";frame.removeAttribute("src")}
    if(auth)auth.style.display="flex";
    if(icon)icon.textContent="i";
    if(title)title.textContent="Refresh Required";
    if(copy)copy.textContent="The extension was reloaded while this Amazon page was open.";
    if(link){
      link.textContent="Refresh Amazon Page";
      link.href="#";
      link.onclick=function(e){e.preventDefault();window.top.location.reload()};
    }
    if(note)note.textContent="Refresh the Amazon tab, then open SellerAmp again.";
  }

  async function boot(){
    const appUrl=new URLSearchParams(window.location.search).get("appUrl");
    const frame=document.querySelector("#appFrame");
    const auth=document.querySelector("#authMessage");
    if(!appUrl){console.error("[Proxy] No appUrl parameter found");return}
    if(!frame){console.error('[Proxy] No iframe with id "appFrame" found');return}

    if(!extensionAlive()){
      if(!tryContextReload())showRefresh();
      return;
    }

    // Always load the SAS panel. Auth is handled by the recloud proxy
    // (premium cookie injection + extension_api_paths). Never show the
    // old third-party login gate.
    if(auth)auth.style.display="none";
    frame.style.display="block";
    if(!loaded){
      frame.src=appUrl;
      loaded=true;
    }

    try{
      await chrome.runtime.sendMessage({type:MSG});
      try{sessionStorage.removeItem("proxyContextReloaded")}catch(e){}
    }catch(e){
      const msg=e?.message||String(e);
      if(msg.includes("Extension context invalidated")){
        if(!tryContextReload())showRefresh();
      }
    }
  }

  document.addEventListener("visibilitychange",()=>{
    if(document.visibilityState==="visible"&&!loaded)boot();
  });
  if(document.readyState==="loading")document.addEventListener("DOMContentLoaded",boot);
  else boot();
}();
