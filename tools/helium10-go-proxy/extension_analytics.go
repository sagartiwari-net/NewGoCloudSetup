package main

import (
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	extensionEventsOnce sync.Once
	asinPathRe          = regexp.MustCompile(`(?i)/(?:dp|gp/product|product)/([A-Z0-9]{10})(?:/|$|\?)`)
	asinQueryKeys       = []string{"asin", "asins", "parentAsin", "parent_asin", "productAsin"}
)

type extensionEventRow struct {
	WebsiteID   int
	Username    string
	ToolKey     string
	Source      string
	Action      string
	TargetPath  string
	PageURL     string
	ASIN        string
	Marketplace string
	QueryText   string
	StatusCode  int
	ClientIP    string
	UserAgent   string
}

func ensureExtensionEventsTable(cfg Config) {
	extensionEventsOnce.Do(func() {
		if !usesPanelAccountMode(cfg) {
			return
		}
		db, err := openPanelDB(cfg)
		if err != nil {
			return
		}
		_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS extension_events (
			id INTEGER PRIMARY KEY,
			website_id INTEGER NOT NULL,
			username TEXT NOT NULL,
			tool_key TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT 'extension',
			action TEXT NOT NULL,
			target_path TEXT NOT NULL DEFAULT '',
			page_url TEXT NOT NULL DEFAULT '',
			asin TEXT NOT NULL DEFAULT '',
			marketplace TEXT NOT NULL DEFAULT '',
			query_text TEXT NOT NULL DEFAULT '',
			status_code INTEGER NOT NULL DEFAULT 0,
			client_ip TEXT NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`)
		_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_extension_events_created ON extension_events(created_at)`)
		_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_extension_events_website ON extension_events(website_id, created_at)`)
		_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_extension_events_user ON extension_events(username, created_at)`)
	})
}

func panelWebsiteID(cfg Config) int {
	if !usesPanelAccountMode(cfg) {
		if currentWebsiteID > 0 {
			return currentWebsiteID
		}
		return 0
	}
	db, err := openPanelDB(cfg)
	if err != nil {
		return currentWebsiteID
	}
	var id int
	if err := db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&id); err != nil || id <= 0 {
		return currentWebsiteID
	}
	return id
}

func shouldSkipExtensionAnalyticsPath(path string) bool {
	p := strings.ToLower(path)
	if p == "" || p == "/" || p == "/favicon.ico" {
		return true
	}
	if strings.HasPrefix(p, "/cdn-proxy/") || strings.HasPrefix(p, "/extra-cdn-") {
		return true
	}
	if strings.HasPrefix(p, "/cdn-cgi/") || strings.HasPrefix(p, "/_next/") {
		return true
	}
	for _, ext := range []string{".js", ".css", ".map", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".woff", ".woff2", ".ttf", ".eot", ".webp", ".mp4", ".webm"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

func extensionEventSource(r *http.Request, path string) string {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if strings.HasPrefix(origin, "chrome-extension://") {
		return "extension"
	}
	ref := strings.ToLower(r.Header.Get("Referer"))
	if strings.Contains(ref, "amazon.") || strings.Contains(ref, "chrome-extension://") {
		return "extension"
	}
	if strings.HasPrefix(path, "/extension/") || strings.Contains(path, "chrome-extension") {
		return "extension"
	}
	if strings.HasPrefix(path, "/research-tools/") || strings.HasPrefix(path, "/control-center/") {
		return "extension"
	}
	return "web"
}

func classifyHeliumAction(path string) (action string, ok bool) {
	p := strings.ToLower(path)
	switch {
	case strings.HasPrefix(p, "/extension/check-login"), strings.Contains(p, "/chrome-extension"):
		return "extension_login_check", true
	case strings.Contains(p, "/xray") && strings.Contains(p, "keyword"):
		return "xray_keywords", true
	case strings.Contains(p, "/xray") && (strings.Contains(p, "inventory") || strings.Contains(p, "stock")):
		return "xray_inventory", true
	case strings.Contains(p, "/xray") || strings.Contains(p, "ce_xray") || strings.Contains(p, "product-idea"):
		return "xray", true
	case strings.Contains(p, "inventory-grader") || strings.Contains(p, "inventory_grader"):
		return "inventory_grader", true
	case strings.Contains(p, "/cerebro") || strings.Contains(p, "cerebro"):
		return "cerebro", true
	case strings.Contains(p, "/magnet") || strings.Contains(p, "magnet"):
		return "magnet", true
	case strings.Contains(p, "black-box") || strings.Contains(p, "blackbox"):
		return "black_box", true
	case strings.Contains(p, "keyword-tracker") || strings.Contains(p, "keyword_tracker"):
		return "keyword_tracker", true
	case strings.Contains(p, "listing-analyzer") || strings.Contains(p, "listing_analyzer"):
		return "listing_analyzer", true
	case strings.Contains(p, "review-insights") || strings.Contains(p, "reviews"):
		return "reviews", true
	case strings.Contains(p, "profit") || strings.Contains(p, "refund"):
		return "profits", true
	case strings.Contains(p, "market-tracker") || strings.Contains(p, "market_tracker"):
		return "market_tracker", true
	case strings.Contains(p, "alerts"):
		return "alerts", true
	case strings.HasPrefix(p, "/research-tools/"):
		return "research_tools", true
	case strings.HasPrefix(p, "/control-center/"):
		return "control_center", true
	case strings.HasPrefix(p, "/api/v1/"):
		return "api_v1", true
	case strings.HasPrefix(p, "/extension/"):
		return "extension", true
	case strings.HasPrefix(p, "/black-box/"), strings.HasPrefix(p, "/cerebro/"), strings.HasPrefix(p, "/magnet/"):
		return "tool_page", true
	default:
		return "", false
	}
}

func extractASIN(path string, q url.Values) string {
	for _, key := range asinQueryKeys {
		if v := strings.TrimSpace(q.Get(key)); v != "" {
			v = strings.ToUpper(strings.Split(v, ",")[0])
			if len(v) == 10 {
				return v
			}
		}
	}
	if m := asinPathRe.FindStringSubmatch(path); len(m) == 2 {
		return strings.ToUpper(m[1])
	}
	return ""
}

func extractMarketplace(q url.Values, ref string) string {
	for _, key := range []string{"marketplace", "marketplaceId", "store", "storeId", "locale"} {
		if v := strings.TrimSpace(q.Get(key)); v != "" {
			return v
		}
	}
	ref = strings.ToLower(ref)
	for _, host := range []string{"amazon.com", "amazon.in", "amazon.co.uk", "amazon.de", "amazon.ca", "amazon.com.au", "amazon.co.jp", "amazon.fr", "amazon.it", "amazon.es", "amazon.com.mx", "amazon.com.br"} {
		if strings.Contains(ref, host) {
			return host
		}
	}
	return ""
}

func extractQueryText(q url.Values) string {
	for _, key := range []string{"keyword", "keywords", "q", "query", "search", "phrase", "term"} {
		if v := strings.TrimSpace(q.Get(key)); v != "" {
			if len(v) > 200 {
				return v[:200]
			}
			return v
		}
	}
	return ""
}

func maybeLogExtensionEvent(cfg Config, r *http.Request, username, path string, statusCode int) {
	if !usesPanelAccountMode(cfg) {
		return
	}
	username = strings.TrimSpace(username)
	if username == "" || username == "public_asset" || username == "local_dev" {
		return
	}
	if shouldSkipExtensionAnalyticsPath(path) {
		return
	}
	action, ok := classifyHeliumAction(path)
	if !ok {
		return
	}
	q := r.URL.Query()
	ref := r.Header.Get("Referer")
	pageURL := strings.TrimSpace(ref)
	if len(pageURL) > 500 {
		pageURL = pageURL[:500]
	}
	row := extensionEventRow{
		WebsiteID:   panelWebsiteID(cfg),
		Username:    username,
		ToolKey:     "helium10",
		Source:      extensionEventSource(r, path),
		Action:      action,
		TargetPath:  path,
		PageURL:     pageURL,
		ASIN:        extractASIN(path+"?"+r.URL.RawQuery, q),
		Marketplace: extractMarketplace(q, ref),
		QueryText:   extractQueryText(q),
		StatusCode:  statusCode,
		ClientIP:    realClientIP(r),
		UserAgent:   r.UserAgent(),
	}
	if row.WebsiteID <= 0 {
		return
	}
	if len(row.UserAgent) > 300 {
		row.UserAgent = row.UserAgent[:300]
	}
	if len(row.TargetPath) > 400 {
		row.TargetPath = row.TargetPath[:400]
	}
	go writeExtensionEvent(cfg, row)
}

func writeExtensionEvent(cfg Config, row extensionEventRow) {
	ensureExtensionEventsTable(cfg)
	db, err := openPanelDB(cfg)
	if err != nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = db.Exec(`INSERT INTO extension_events (
		website_id, username, tool_key, source, action, target_path, page_url, asin, marketplace, query_text, status_code, client_ip, user_agent, created_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		row.WebsiteID, row.Username, row.ToolKey, row.Source, row.Action, row.TargetPath, row.PageURL,
		row.ASIN, row.Marketplace, row.QueryText, row.StatusCode, row.ClientIP, row.UserAgent, now,
	)
	if err != nil {
		log.Printf("[ANALYTICS] extension event insert failed: %v", err)
	}
}
