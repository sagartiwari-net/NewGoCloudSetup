package main

func writecreamAccountHideHTML() string {
	const css = `div.header-widget:has(> div.header-notifications.user-menu){display:none!important}`
	return `<style id="tm-wc-hide">` + css + `</style><script>(function(){var css=` + "`" + css + "`" + `;function ensure(){if(document.getElementById("tm-wc-hide"))return;var s=document.createElement("style");s.id="tm-wc-hide";s.textContent=css;(document.head||document.documentElement).appendChild(s);}ensure();setInterval(ensure,1000);})();</script>`
}
