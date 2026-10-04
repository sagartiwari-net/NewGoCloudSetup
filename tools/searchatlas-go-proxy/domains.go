package main

import (
	"net/url"
	"strings"
)

func defaultSearchAtlasExtraDomains() []string {
	return []string{
		"sa.searchatlas.com",
		"vsfgd.searchatlas.com",
		"help.searchatlas.com",
		"searchatlas.com",
		"www.searchatlas.com",
		"api.searchatlas.com",
		"keyword.searchatlas.com",
		"llmvis.searchatlas.com",
		"gsc.searchatlas.com",
		"backlink.searchatlas.com",
		"ca.searchatlas.com",
		"agent.searchatlas.com",
		"status.searchatlas.com",
		"otto-ppc.searchatlas.com",
		"api.builder.searchatlas.com",
		"ucmsi.searchatlas.com",
		"cdn.searchatlas.com",
		"assets.searchatlas.com",
		"static.searchatlas.com",
		"static-assets.searchatlas.com",
		"js.stripe.com",
		"checkout.stripe.com",
		"m.stripe.com",
		"api.stripe.com",
		"widget.intercom.io",
		"api.intercom.io",
		"js.intercomcdn.com",
		"fonts.googleapis.com",
		"fonts.gstatic.com",
		"www.googletagmanager.com",
		"www.google-analytics.com",
		"flagcdn.com",
		"api.userpilot.io",
		"js.userpilot.io",
		"us.i.posthog.com",
		"app.posthog.com",
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
	for _, d := range defaultSearchAtlasExtraDomains() {
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

// skipURLRewrite: Firebase RTDB rejects proxied URLs like
// http://localhost/ext-proxy/foo.firebaseio.com (dots in path → page crash).
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
	return false
}

func upstreamOrigin(cfg Config) string {
	u, err := url.Parse(cfg.TargetURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "https://dashboard.searchatlas.com"
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
