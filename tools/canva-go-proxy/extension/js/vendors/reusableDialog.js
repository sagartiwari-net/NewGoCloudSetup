"application/xml"===document.contentType||"text/xml"===document.contentType||"image/svg+xml"===document.contentType||window.sasReusableDialog||(window.sasReusableDialog=!0,window.dialog=function(){let u,v,b,m,p=new Array(["fadeIn","fadeOut"],["slideDown","slideUp"]),w="";const g=()=>{u.classList.add("fadeOut"),v.classList.add(""+p[w][1]),setTimeout(()=>{u.remove()},200)};return{show:(e={})=>{var{title:e=null,content:t="Please entry content",okText:n="ok",cancelText:a="cancel",onOk:l=null,onCancel:d=null,maskDisabled:c=!1,isCancelable:i=!0,animation:s=1}=e,s=(w=s,i?`<div class="btn cancel-btn">${a}</div>`:""),i=e?`<b>${e}</b></br></br>`:"",a=`
    <div class="sasextcl-dialog-wrapper fadeIn">
      <div class="dialog ${p[w][0]}">
        <div class="content">${i}${t}</div>
        <div class="buttons">
          <div class="btn ok-btn">${n}</div>
          ${s}
        </div>
      </div>
    </div>
  `,e=document.createElement("div"),o=(e.innerHTML=a,document.body.appendChild(e),u=document.querySelector(".sasextcl-dialog-wrapper"),v=u.querySelector(".dialog"),b=u.querySelector(".cancel-btn"),m=u.querySelector(".ok-btn"),l),r=d,i=c;m?.addEventListener("click",e=>{g(),o&&o()}),b?.addEventListener("click",e=>{g(),r&&r()}),i&&u.addEventListener("click",e=>{e=e?.target;/sasextcl-dialog-wrapper/.test(e.className)&&g()});return u},hide:g}}());