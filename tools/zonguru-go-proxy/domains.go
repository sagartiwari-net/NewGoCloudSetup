package main

import (
	"net/url"
	"strings"
)

func defaultZonGuruExtraDomains() []string {
	return []string{
		"www.zonguru.com",
		"zonguru.com",
		"track.zonguru.com",
		"help.zonguru.com",
		"zonguru.turbodash.co",
		"js.stripe.com",
		"checkout.stripe.com",
		"widget.intercom.io",
		"api.intercom.io",
		"js.intercomcdn.com",
		"fonts.googleapis.com",
		"fonts.gstatic.com",
		"www.googletagmanager.com",
		"www.google-analytics.com",
		"public.profitwell.com",
		"assets.loginwithamazon.com",
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
	for _, d := range defaultZonGuruExtraDomains() {
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

func upstreamOrigin(cfg Config) string {
	u, err := url.Parse(cfg.TargetURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "https://my.zonguru.com"
	}
	return u.Scheme + "://" + u.Host
}

func addSubdomainRewrites(replacements [][2]string, cfg Config, domain string) [][2]string {
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
