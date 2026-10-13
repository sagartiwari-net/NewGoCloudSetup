package main

import (
	"net/http"

	proxysec "toolsmandi.com/proxy-security"
)

var (
	ipTracker        proxysec.IPTracker
	accountIPTracker proxysec.AccountIPTracker
	scoreEngine      *proxysec.ScoreEngine
)

func toolCtx(cfg Config) proxysec.ToolContext {
	return proxysec.ToolContext{
		PublicHost:     cfg.PublicHost,
		PublicScheme:   cfg.PublicScheme,
		MemberAreaURL:  cfg.MemberAreaURL,
		BypassSecurity: usesCookieFileMode(cfg),
	}
}

func normalizeSessionSecurity(cfg Config) proxysec.Config {
	return proxysec.Normalize(cfg.SessionSecurity, toolCtx(cfg))
}

func securityEnabled(cfg Config) bool {
	return proxysec.Enabled(cfg.SessionSecurity, toolCtx(cfg), currentWebsiteSecurityEnabled)
}

func realClientIP(r *http.Request) string {
	return proxysec.RealClientIP(r)
}

func requestHost(r *http.Request) string {
	return proxysec.RequestHost(r)
}

func isInternalRequestHost(host string) bool {
	return proxysec.IsInternalRequestHost(host)
}

func hostMatchesPublic(reqHost, publicHost string, allowed []string) bool {
	if proxysec.HostMatchesPublic(reqHost, publicHost, allowed) {
		return true
	}
	return panelHostAllowed(publicHost, reqHost)
}

func checkSessionIP(sessionToken, currentIP, sessionIP string, ipCfg proxysec.IPProtectionConfig) (updatedIP string, allow bool, kill bool) {
	return proxysec.CheckSessionIP(&ipTracker, sessionToken, currentIP, sessionIP, ipCfg)
}

func applySecurityHeaders(w http.ResponseWriter, cfg Config) {
	proxysec.ApplySecurityHeaders(w, cfg.SessionSecurity, toolCtx(cfg), currentWebsiteSecurityEnabled)
}

func corsOrigin(cfg Config) string {
	return proxysec.CorsOrigin(cfg.SessionSecurity, toolCtx(cfg))
}

func buildDomainCheckJS(cfg Config) string {
	hosts := panelToolDomains(cfg.PublicHost)
	if len(hosts) > 0 {
		ss := cfg.SessionSecurity
		ss.DomainCheck.AllowedHosts = append(append([]string{}, ss.DomainCheck.AllowedHosts...), hosts...)
		cfg.SessionSecurity = ss
	}
	return proxysec.BuildDomainCheckJS(cfg.SessionSecurity, toolCtx(cfg), currentWebsiteSecurityEnabled)
}

func buildSecurityHeartbeatJS(cfg Config) string {
	return proxysec.BuildSecurityHeartbeatJS(cfg.SessionSecurity, toolCtx(cfg), currentWebsiteSecurityEnabled)
}

func recordSecurityEvent(r *http.Request, username, eventType, details string) {
	wid := currentWebsiteID
	if id := panelWebsiteIDForRequest(r); id > 0 {
		wid = id
	}
	proxysec.RecordEvent(writeSecurityEvent, wid, r, username, eventType, details)
}

func writeSecurityEvent(e proxysec.SecurityEvent) {
	if !dbConnected || e.WebsiteID == 0 {
		return
	}
	_, _ = db.Exec(
		`INSERT INTO ahrefs_security_logs (website_id, username, client_ip, event_type, attempted_url, details, user_agent)
		 VALUES (?,?,?,?,?,?,?)`,
		e.WebsiteID, e.Username, e.ClientIP, e.EventType, e.AttemptedURL, e.Details, e.UserAgent,
	)
}

func initScoreEngine(cfg Config) {
	if scoreEngine != nil {
		scoreEngine.Close()
	}
	ss := normalizeSessionSecurity(cfg)
	scoreEngine = proxysec.InitScoreEngine(ss, toolCtx(cfg))
}

func computeRiskScore(r *http.Request, cfg Config, hasDeviceCookie bool, concurrentIPFlag bool) int {
	return proxysec.ComputeScore(scoreEngine, r, cfg.SessionSecurity, toolCtx(cfg), currentWebsiteSecurityEnabled, hasDeviceCookie, concurrentIPFlag)
}
