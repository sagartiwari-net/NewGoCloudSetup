package main

import "strconv"

// quillbotChromeHTML hides only the account menu that contains the logout
// item, and the separate Log out row. The header email line becomes the
// panel username.
func quillbotChromeHTML(username string) string {
	const css = `.MuiPopover-paper:has([data-testid="logout-option"]){display:none!important}`
	name := strconv.Quote(username)
	return `<style id="tm-quillbot-hide">` + css + `</style><script data-tm-quillbot-chrome="1">(function(){var css=` + "`" + css + "`" + `;var name=` + name + `;function ensure(){if(!document.getElementById("tm-quillbot-hide")){var s=document.createElement("style");s.id="tm-quillbot-hide";s.textContent=css;(document.head||document.documentElement).appendChild(s);}}function hideLogoutRow(){document.querySelectorAll("p.MuiTypography-bodyMedium").forEach(function(p){if((p.textContent||"").trim()!=="Log out")return;var n=p;for(var i=0;i<3;i++){if(!n.parentElement||!n.parentElement.classList.contains("MuiBox-root"))return;n=n.parentElement;}n.style.setProperty("display","none","important");});}function paintName(){if(!name)return;document.querySelectorAll("div.MuiTypography-subtitleMedium.MuiTypography-noWrap").forEach(function(el){if(el.childElementCount!==0)return;if(el.textContent!==name)el.textContent=name;});}function tick(){ensure();hideLogoutRow();paintName();}tick();setInterval(tick,1000);})();</script>`
}
