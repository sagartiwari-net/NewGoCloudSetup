package main

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

func cfChallengeBypassJS() string {
	return `/* SEOTester CF challenge bypass */
(function(){try{
  window._cf_chl_opt=window._cf_chl_opt||{};
  if(typeof window.__cfBeacon!=='object'){window.__cfBeacon={u:function(){}};}
}catch(e){}})();`
}

func cfTurnstileBypassJS() string {
	return `(function(){
  window.turnstile={
    render:function(el,p){if(p&&p.callback)setTimeout(function(){p.callback('proxy-bypass');},50);return'mock';},
    reset:function(){},remove:function(){},
    getResponse:function(){return'proxy-bypass';},
    isExpired:function(){return false;},
    execute:function(el,p){if(p&&p.callback)setTimeout(function(){p.callback('proxy-bypass');},50);}
  };
})();`
}

func serveCloudflareBypass(w http.ResponseWriter, path string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	if strings.Contains(path, "turnstile") || path == "/cf-turnstile-bypass.js" {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		fmt.Fprint(w, cfTurnstileBypassJS())
		return
	}
	if strings.Contains(path, "/oneshot/") || strings.HasSuffix(path, ".json") {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprint(w, `{"success":true}`)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	fmt.Fprint(w, cfChallengeBypassJS())
}

var (
	reCFScript      = regexp.MustCompile(`(?is)<script[^>]*\bsrc=["'][^"']*cdn-cgi/challenge-platform[^"']*["'][^>]*>\s*</script>`)
	reCFIframe      = regexp.MustCompile(`(?is)<script>\(function\(\)\{function c\(\)\{var b=a\.contentDocument\|\|a\.contentWindow\.document;if\(b\)\{[^<]*cdn-cgi/challenge-platform[^<]*</script>`)
	reCFInline      = regexp.MustCompile(`(?is)window\.__CF\$cv\$params=\{[^}]*\};var a=document\.createElement\('script'\);a\.src='/cdn-cgi/challenge-platform[^;]*;document\.getElementsByTagName\('head'\)\[0\]\.appendChild\(a\);`)
	reCFTurnstile   = regexp.MustCompile(`(?is)<script[^>]*\bsrc=["'][^"']*challenges\.cloudflare\.com[^"']*["'][^>]*>\s*</script>`)
)

func stripCloudflareChallengeHTML(body []byte, cfg Config) []byte {
	if len(body) == 0 {
		return body
	}
	publicBase := proxyOrigin(cfg)
	s := string(body)
	s = reCFScript.ReplaceAllString(s, "")
	s = reCFIframe.ReplaceAllString(s, "")
	s = reCFInline.ReplaceAllString(s, "/*cf-bypass*/")
	s = reCFTurnstile.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "https://challenges.cloudflare.com/turnstile/v0/api.js", publicBase+"/cf-turnstile-bypass.js")
	s = strings.ReplaceAll(s, "//challenges.cloudflare.com/turnstile/v0/api.js", publicBase+"/cf-turnstile-bypass.js")
	return []byte(s)
}

func isCloudflareBlockPage(body []byte) bool {
	s := strings.ToLower(string(body))
	return strings.Contains(s, "sorry, you have been blocked") ||
		strings.Contains(s, "you are unable to access") ||
		strings.Contains(s, "attention required") && strings.Contains(s, "cloudflare")
}
