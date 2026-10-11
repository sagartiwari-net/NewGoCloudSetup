package main

import "strconv"

func jasperChromeHTML(username string) string {
	const css = `div[data-radix-popper-content-wrapper]:has(button[data-testid="profile-menu-logout-item"]){display:none!important}`
	name := strconv.Quote(username)
	return `<style id="tm-jasper-hide">` + css + `</style><script>(function(){var css=` + "`" + css + "`" + `;var name=` + name + `;function ensure(){if(!document.getElementById("tm-jasper-hide")){var s=document.createElement("style");s.id="tm-jasper-hide";s.textContent=css;(document.head||document.documentElement).appendChild(s);}}function set(el){if(el.textContent!==name)el.textContent=name;}function paint(){if(!name)return;document.querySelectorAll("span.mr-auto.flex-1.truncate.text-left.whitespace-nowrap").forEach(set);document.querySelectorAll("span.flex.min-w-0.flex-1.flex-col.text-left > span.truncate.whitespace-nowrap").forEach(set);}ensure();paint();setInterval(function(){ensure();paint();},1000);})();</script>`
}
