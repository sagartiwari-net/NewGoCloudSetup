"use strict";import{positionConstants}from"../../constants/positionConstants.js";import{urlConstants}from"../../constants/urlConstants.js";const getPositionsToolbar=async s=>{var{PANEL_POSITIONS:i,AMAZON_EMBEDDED:o,AMAZON_EMBEDDED_WIDE:a}=positionConstants,{homeURL:t,historyUrl:n,settingsUrl:e,contactUrl:l}=urlConstants,r=chrome.runtime.getURL,t=`
<li id="home-btn" >
  <a href="${t}" target="sasFrame" ><img src="${r("images/house-chimney.svg")}" title="Home" alt="home"/>
</li>`,n=`
<li id="history-btn" >
  <a href="${n}" target="sasFrame" ><img src="${r("images/clock-rotate-left.svg")}" title="History" alt="history"/>
</li>`,e=`
<li id="settings-btn" >
  <a href="${e}" target="_blank"><img src="${r("images/gear.svg")}" title="Settings" alt="settings"/>
</li>`,l=`
<li  id="contact-btn">
  <a href="${l}" target="_blank">
    <img src="${r("images/envelope.svg")}" title="Contact" alt="contact"/>
  </a>
</li>`;let g='<nav class="sasdklgc-menu"><ul id="position-buttons" class="sasdklgc-toolbar-buttons">';for(let t=0;t<i.length;t++)(s||i[t].id!=o&&i[t].id!=a)&&(g=g+'<li id="li-pos-'+i[t].id+'" data-position="'+i[t].id+'"><img src="'+r("images/"+i[t].img)+'" title="'+i[t].desc+'" /></li>');return g=(g+='</ul><ul id="url-buttons" class="sasdklgc-toolbar-buttons">')+(t+n+e+l)+"</ul></nav>"};export{getPositionsToolbar};