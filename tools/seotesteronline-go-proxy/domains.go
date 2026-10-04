package main

import (
	"net/url"
	"strings"
)

func defaultSEOTesterExtraDomains() []string {
	return []string{
		"api.seotesteronline.com",
		"socket.seotesteronline.com",
		"proxy.seotesteronline.com",
		"www.seotesteronline.com",
		"seotesteronline.com",
		"suite.seotesteronline.com",
		"feedback.seotesteronline.com",
		"partner.seotesteronline.com",
		"go.seotesteronline.com",
		"stocdn.s3-eu-west-1.amazonaws.com",
		"js.stripe.com",
		"checkout.stripe.com",
		"m.stripe.com",
		"api.stripe.com",
		"static.zdassets.com",
		"assets.calendly.com",
		"fonts.googleapis.com",
		"fonts.gstatic.com",
		"www.googletagmanager.com",
		"www.google-analytics.com",
		"clients1.google.com",
		"suggestqueries.google.com",
		"cdnjs.cloudflare.com",
	}
}

func mergeExtraDomains(cfg []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(cfg)+16)
	add := func(d string) {
		d = trimDomain(d)
		if d == "" || seen[d] {
			return
		}
		seen[d] = true
		out = append(out, d)
	}
	for _, d := range defaultSEOTesterExtraDomains() {
		add(d)
	}
	for _, d := range cfg {
		add(d)
	}
	return out
}

func trimDomain(d string) string {
	d = strings.TrimSpace(d)
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimPrefix(d, "http://")
	d = strings.TrimSuffix(d, "/")
	if i := strings.Index(d, "/"); i >= 0 {
		d = d[:i]
	}
	return d
}

// skipURLRewrite: Firebase / Google auth & Maps must stay on real hosts.
func skipURLRewrite(domain string) bool {
	d := strings.ToLower(trimDomain(domain))
	if d == "" {
		return true
	}
	if strings.HasSuffix(d, ".firebaseio.com") || d == "firebaseio.com" {
		return true
	}
	if strings.HasSuffix(d, ".firebasedatabase.app") || d == "firebasedatabase.app" {
		return true
	}
	if strings.HasSuffix(d, ".firebaseapp.com") {
		return true
	}
	// Google Maps / reCAPTCHA / Google Sign-In — do not proxy.
	// Note: clients1.google.com + suggestqueries.google.com ARE proxied
	// (Keyword Explorer JSONP seeds); www.google.com stays direct.
	if d == "maps.googleapis.com" || d == "www.google.com" || d == "accounts.google.com" {
		return true
	}
	if strings.HasSuffix(d, ".googleapis.com") && strings.Contains(d, "recaptcha") {
		return true
	}
	if d == "www.gstatic.com" || d == "www.recaptcha.net" {
		return true
	}
	return false
}

func upstreamOrigin(cfg Config) string {
	u, err := url.Parse(cfg.TargetURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "https://suite.seotesteronline.com"
	}
	return u.Scheme + "://" + u.Host
}

func addSubdomainRewrites(replacements [][2]string, cfg Config, domain string) [][2]string {
	if skipURLRewrite(domain) {
		return replacements
	}
	origin := proxyOrigin(cfg)
	hostPort := proxyHostPort(cfg)
	wsOrigin := strings.Replace(origin, "http", "ws", 1)
	base := origin + "/ext-proxy/" + domain
	wsBase := wsOrigin + "/ext-proxy/" + domain
	return append(replacements,
		[2]string{"https://" + domain, base},
		[2]string{"http://" + domain, base},
		[2]string{"//" + domain, "//" + hostPort + "/ext-proxy/" + domain},
		[2]string{"wss://" + domain, wsBase},
		[2]string{"ws://" + domain, wsBase},
		[2]string{`https:\/\/` + domain, strings.ReplaceAll(base, "/", `\/`)},
		[2]string{`http:\/\/` + domain, strings.ReplaceAll(base, "/", `\/`)},
		[2]string{`\/\/` + domain, `\/\/` + strings.ReplaceAll(hostPort, "/", `\/`) + `\/ext-proxy\/` + domain},
	)
}
