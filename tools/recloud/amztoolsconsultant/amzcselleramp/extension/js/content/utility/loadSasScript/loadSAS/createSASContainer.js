"use strict";import{mutableData}from"../../../constants/mutableData.js";import{getPositionsToolbar}from"../../positionService/getPositionsToolbar.js";import{positionConstants}from"../../../constants/positionConstants.js";import{adjustBody}from"../../positionService/adjustBody.js";import{urlConstants}from"../../../constants/urlConstants.js";const createSASContainer=async()=>{const{FLOATING:e,DOCKED_LEFT:s,DOCKED_RIGHT:o}=positionConstants;var t=document.createElement("div"),t=(t.setAttribute("id","SASContainer"),t.setAttribute("class","sasext sasextdklgc sasdklgc-display-none"),$(t).appendTo("body"),`
			<div id="sas-panel-contents">
				<div class="first-row sas-header" >
          <div class="tr-btns">
            <span  style="float:right"><a id="closeSAS" class="topButton"></a></span>
          </div> 
          <div class="t-logo">
            <a href="https://selleramp.com" target="_blank">
              <img class="sas-logo-image" src="${chrome.runtime.getURL("images/sas-logo.svg")}" />
            </a> 
          </div>
          <a href="${urlConstants.accountUrl}" target="_blank">
            <img src=${chrome.runtime.getURL("images/user-regular.svg")} alt="account-icon" class="sas-account-icon"/>
          </a>
				</div>`),a=await getPositionsToolbar(mutableData.gAmazonProductPage);t+=`
    <div class="second-row">
      <div id="loadingMessage">
        <img class="loadingimg" src="${chrome.runtime.getURL("images/exticon.png")}" />
      </div>
      <iframe name="sasFrame" id="sasFrame" allowtransparency="true"></iframe>
    </div>
    <div class="bottom-row"> 
      ${a}
    </div>
  </div>`,$("#SASContainer").html(t),$("#SASContainer").resizable({minHeight:150,minWidth:292,handles:"n, s,e,w",start:function(t,a){a.element.append($("<div/>",{id:"iframe-barrier",css:{position:"absolute",top:0,right:0,bottom:0,left:0,"z-index":10}}))},resize:function(t,a){mutableData.gDockedState!=s&&mutableData.gDockedState!=o||(mutableData.gDockedWidth=a.size.width,$(this).css("left",""),adjustBody())},stop:function(t,a){$("#iframe-barrier",a.element).remove(),mutableData.gDockedState==s||mutableData.gDockedState==o?(mutableData.gDockedWidth=a.size.width,chrome.storage.sync.set({dockedWidth:mutableData.gDockedWidth},function(){chrome.runtime.lastError&&console.error(chrome.runtime.lastError.message)})):mutableData.gDockedState==e&&(mutableData.gFloatingHeight=a.size.height,chrome.storage.sync.set({floatingHeight:mutableData.gFloatingHeight},function(){chrome.runtime.lastError&&console.error(chrome.runtime.lastError.message)}),mutableData.gFloatingWidth=a.size.width,chrome.storage.sync.set({floatingWidth:mutableData.gFloatingWidth},function(){chrome.runtime.lastError&&console.error(chrome.runtime.lastError.message)})),adjustBody()}})};export{createSASContainer};