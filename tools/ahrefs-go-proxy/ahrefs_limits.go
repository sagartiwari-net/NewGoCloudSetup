package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ahrefsLimitHit struct {
	key      string
	label    string
	action   string
	target   string
	amount   int
	reset    int
	family   string
	isExport bool
	page     bool
}

func ahrefsLimitHitFor(r *http.Request, body []byte) (ahrefsLimitHit, bool) {
	path := r.URL.Path
	if path == "/v4/baTable" && r.Method == http.MethodPost {
		n := parseBatchTargetCount(body)
		amount := batchAnalysisCreditCost(n)
		if amount < 1 {
			amount = 1
		}
		hit := ahrefsLimitHit{key: "credits", label: "Credits", action: "Batch analysis", target: path, amount: amount, reset: 1, family: path}
		hit.target = ahrefsUsageLabel(r, body, hit.action)
		return hit, true
	}
	if ahrefsExportPath(path) {
		hit := ahrefsLimitHit{key: "exports", label: "Exports", action: "Export", target: path, amount: 0, reset: 7, family: "export:" + path, isExport: true}
		hit.target = ahrefsUsageLabel(r, body, hit.action)
		return hit, true
	}
	action, family, ok := ahrefsCreditPath(path, r.URL.RawQuery, r.Referer())
	if !ok {
		return ahrefsLimitHit{}, false
	}
	hit := ahrefsLimitHit{key: "credits", label: "Credits", action: action, target: path, amount: 1, reset: 1, family: family}
	hit.target = ahrefsUsageLabel(r, body, action)
	return hit, true
}

func ahrefsUsageLabel(r *http.Request, body []byte, action string) string {
	domain, keyword := "", ""
	note := func(raw string) {
		if strings.TrimSpace(raw) == "" {
			return
		}
		var parsed interface{}
		if json.Unmarshal([]byte(raw), &parsed) == nil {
			ahrefsWalkTarget(parsed, &domain, &keyword)
			return
		}
		if domain == "" {
			domain = ahrefsCleanTarget(raw)
		}
	}
	q := r.URL.Query()
	note(q.Get("input"))
	note(q.Get("target"))
	note(q.Get("keyword"))
	if len(body) > 0 && len(body) < 1<<20 {
		note(string(body))
	}
	subject := domain
	if keyword != "" && !strings.EqualFold(keyword, domain) {
		if subject != "" {
			subject += " · " + keyword
		} else {
			subject = keyword
		}
	}
	if subject == "" {
		return action
	}
	if len(subject) > 180 {
		subject = subject[:180]
	}
	return action + " · " + subject
}

func ahrefsWalkTarget(v interface{}, domain, keyword *string) {
	switch t := v.(type) {
	case map[string]interface{}:
		for key, val := range t {
			text, _ := val.(string)
			switch strings.ToLower(key) {
			case "url", "protocollessurl", "domain":
				if *domain == "" && text != "" {
					*domain = ahrefsCleanTarget(text)
				}
			case "target":
				if *domain == "" && text != "" && !strings.EqualFold(text, "subdomains") && !strings.EqualFold(text, "prefix") && !strings.EqualFold(text, "exact") {
					*domain = ahrefsCleanTarget(text)
				}
			case "keyword", "searchterm":
				if *keyword == "" && text != "" {
					*keyword = strings.TrimSpace(text)
				}
			}
			ahrefsWalkTarget(val, domain, keyword)
		}
	case []interface{}:
		for _, item := range t {
			ahrefsWalkTarget(item, domain, keyword)
		}
	}
}

func ahrefsCleanTarget(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimSuffix(s, "/")
	return s
}

func ahrefsExportPath(path string) bool {
	if strings.Contains(path, "ExportSettings") || strings.Contains(path, "tkGetExport") {
		return false
	}
	return strings.Contains(path, "Export") || strings.Contains(path, "export")
}

func ahrefsCreditPath(path, rawQuery, referer string) (action, family string, ok bool) {
	if strings.Contains(referer, "/site-explorer/overview") {
		switch path {
		case "/v4/seGetMetrics", "/v4/seGetDomainRating", "/v4/seBacklinksStats", "/v4/seTrafficSummary", "/v4/seGetPageInfo":
			return "Site Explorer overview", "se-overview", true
		}
	}
	switch path {
	case "/v4/keGetTopPositionsHistory":
		if !strings.Contains(rawQuery, "consumeLimitFor") {
			return "", "", false
		}
		return "Keyword overview", "ke-overview", true
	case "/v4/keIdeas":
		return "Keyword ideas", "ke-ideas", true
	case "/v4/keGetTrafficByDomains":
		return "Keyword traffic domains", "ke-traffic-domains", true
	case "/v4/keGetTrafficByPages":
		return "Keyword traffic pages", "ke-traffic-pages", true
	case "/v4/keAdsByDomains":
		return "Keyword ads", "ke-ads", true
	case "/v4/seGetOrganicKeywords", "/v4/seGetOrganicKeywordsSigned":
		return "Organic keywords", "se-organic", true
	case "/v4/seGetOrganicPositions", "/v4/seGetPositionsHistory", "/v4/seSerpOverview":
		return "Organic positions", "se-positions", true
	case "/v4/seGetTopPages":
		return "Top pages", "se-top-pages", true
	case "/v4/seGetOrganicCompetitors":
		return "Organic competitors", "se-competitors", true
	case "/v4/seBacklinks":
		return "Backlinks", path, true
	case "/v4/seBacklinksGroup":
		return "Backlinks group", path, true
	case "/v4/seBrokenBacklinks":
		return "Broken backlinks", path, true
	case "/v4/seRefdomains":
		return "Referring domains", path, true
	case "/v4/seAnchors":
		return "Anchors", path, true
	case "/v4/seAuthors":
		return "Authors", path, true
	case "/v4/seRefIPs":
		return "Referring IPs", path, true
	case "/v4/seLinkedAnchors":
		return "Linked anchors", path, true
	case "/v4/seGetPositionsMovements":
		return "Position movements", path, true
	case "/v4/seGetSiteStructure":
		return "Site structure", path, true
	case "/v4/seGetHtmlSnapshot":
		return "HTML snapshot", path, true
	case "/v4/seGetPaidKeywordsV2":
		return "Paid keywords", path, true
	case "/v4/seGetAdsV2":
		return "Ads", path, true
	case "/v4/ceSearchResults":
		return "Content search", path, true
	case "/v4/ceAuthorsReport":
		return "Content authors", path, true
	case "/v4/ceWebsitesReport":
		return "Content websites", path, true
	case "/v4/ceLanguagesReport":
		return "Content languages", path, true
	case "/v4/ceSearchDetailsKeywords":
		return "Content detail", "ce-detail", true
	case "/v4/cmCreateDocument":
		return "Content Helper document", path, true
	case "/v4/cmSelectCompetitors":
		return "Content Helper competitors", path, true
	case "/v4/caGetKeywords":
		return "Keyword comparison", path, true
	case "/v4/caLinkIntersect":
		return "Link Intersect", path, true
	case "/v4/brGetOverviewStats":
		return "Brand overview", path, true
	case "/v4/brGetSearchDemand":
		return "Brand search demand", path, true
	case "/v4/brGetWebVisibility":
		return "Brand web", path, true
	case "/v4/brGetYoutubeVisibility":
		return "Brand YouTube", path, true
	case "/v4/brGetTikTokResults":
		return "Brand TikTok", path, true
	case "/v4/brGetRedditResults":
		return "Brand Reddit", path, true
	case "/v4/brGetReportsShareOfVoice":
		return "Brand share of voice", path, true
	case "/v4/saGetProject":
		return "Open site audit", path, true
	case "/v4/saDeTable":
		return "Site audit pages", path, true
	case "/v4/saGetUrlDetails":
		return "Site audit URL", path, true
	case "/v4/saGetCrawlLog":
		return "Site audit crawl log", path, true
	case "/v4/saTimeframesMetrics":
		return "Site audit timeframe", path, true
	default:
		return "", "", false
	}
}

func ahrefsExportAsk(r *http.Request, body []byte) int {
	if r != nil {
		if n := ahrefsParamSize(r.URL.Query().Get("input")); n > 0 {
			return n
		}
	}
	if n := ahrefsParamSize(string(body)); n > 0 {
		return n
	}
	return 0
}

func ahrefsParamSize(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	var v interface{}
	if json.Unmarshal([]byte(raw), &v) != nil {
		return 0
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return 0
	}
	params, ok := m["params"].(map[string]interface{})
	if !ok {
		return 0
	}
	n, ok := params["size"].(float64)
	if !ok || n < 1 {
		return 0
	}
	return int(n)
}

func ahrefsLimitMessage(hit ahrefsLimitHit, used, limit, ask int) string {
	left := limit - used
	if left < 0 {
		left = 0
	}
	if hit.isExport {
		if ask > 0 {
			return fmt.Sprintf("This export needs %d rows, but only %d are left.", ask, left)
		}
		return "Weekly export limit reached."
	}
	return "Daily credit limit reached."
}

func ahrefsCSVRows(body []byte) int {
	if len(body) == 0 {
		return 0
	}
	var lines int
	switch {
	case len(body) >= 2 && body[0] == 0xff && body[1] == 0xfe:
		lines = bytes.Count(body, []byte{0x0a, 0x00})
		if body[len(body)-2] != 0x0a || body[len(body)-1] != 0x00 {
			lines++
		}
	case len(body) >= 2 && body[0] == 0xfe && body[1] == 0xff:
		lines = bytes.Count(body, []byte{0x00, 0x0a})
		if body[len(body)-2] != 0x00 || body[len(body)-1] != 0x0a {
			lines++
		}
	default:
		lines = bytes.Count(body, []byte("\n"))
		if body[len(body)-1] != '\n' {
			lines++
		}
	}
	if lines > 0 {
		lines--
	}
	if lines < 0 {
		return 0
	}
	return lines
}

type ahrefsMeter struct {
	used  int
	limit int
}

func ahrefsMeterView(username, key string, fallbackLimit, resetDays int) (ahrefsMeter, int, bool) {
	view := ahrefsMeter{limit: fallbackLimit}
	_ = resetDays
	if username == "" {
		return view, 0, false
	}
	db, err := openAhrefsPanel()
	if err != nil {
		return view, 0, false
	}
	var websiteID int
	var raw string
	err = db.QueryRow(`SELECT id, COALESCE(default_limits_json, '{}') FROM websites WHERE domain IN ('127.0.0.1:5291', ?) LIMIT 1`, loadConfig().PublicHost).Scan(&websiteID, &raw)
	if err != nil || websiteID == 0 {
		return view, 0, false
	}
	limits := map[string]int{}
	_ = json.Unmarshal([]byte(raw), &limits)
	if n, ok := limits[key]; ok {
		view.limit = n
	}
	var userID, meterID, meterLimit, meterUsed, custom int
	err = db.QueryRow(`SELECT u.id, COALESCE(m.id, 0), COALESCE(m."limit", 0), COALESCE(m.used, 0), COALESCE(m.is_custom, 0)
		FROM panel_users u
		LEFT JOIN user_meters m ON m.user_id = u.id AND m.key = ?
		WHERE u.website_id = ? AND u.username = ?`, key, websiteID, username).Scan(&userID, &meterID, &meterLimit, &meterUsed, &custom)
	if err == nil {
		view.used = meterUsed
		if meterID > 0 {
			view.limit = meterLimit
		}
	}
	return view, websiteID, true
}

func ahrefsSaveUsage(username string, hit ahrefsLimitHit) {
	if username == "" || hit.amount < 1 {
		return
	}
	if ahrefsDuplicateBill(username+"\n"+hit.family+"\n"+hit.target, hit.family) {
		return
	}
	db, err := openAhrefsPanel()
	if err != nil {
		log.Printf("[LIMITS] panel open failed: %v", err)
		return
	}
	var websiteID int
	if err = db.QueryRow(`SELECT id FROM websites WHERE domain IN ('127.0.0.1:5291', ?) LIMIT 1`, loadConfig().PublicHost).Scan(&websiteID); err != nil || websiteID == 0 {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = db.Exec(`INSERT INTO usage_events (website_id, username, limit_key, limit_label, reset_days, action, target_path, amount, timestamp) VALUES (?,?,?,?,?,?,?,?,?)`,
		websiteID, username, hit.key, hit.label, hit.reset, hit.action, hit.target, hit.amount, now)
	if err != nil {
		log.Printf("[LIMITS] usage insert failed user=%s action=%s: %v", username, hit.action, err)
		return
	}
	var userID int
	err = db.QueryRow(`SELECT id FROM panel_users WHERE website_id=? AND username=?`, websiteID, username).Scan(&userID)
	if err != nil {
		res, insertErr := db.Exec(`INSERT INTO panel_users (website_id, username, status, custom_limit_expire_at) VALUES (?,?,?,NULL)`, websiteID, username, "active")
		if insertErr != nil {
			return
		}
		id, _ := res.LastInsertId()
		userID = int(id)
	}
	res, err := db.Exec(`UPDATE user_meters SET used = used + ? WHERE user_id=? AND key=?`, hit.amount, userID, hit.key)
	changed := int64(0)
	if err == nil && res != nil {
		changed, _ = res.RowsAffected()
	}
	if err != nil || changed == 0 {
		view, _, _ := ahrefsMeterView(username, hit.key, 0, hit.reset)
		_, _ = db.Exec(`INSERT INTO user_meters (user_id, key, label, reset_days, used, "limit", is_custom) VALUES (?,?,?,?,?,?,0)`,
			userID, hit.key, hit.label, hit.reset, view.used+hit.amount, view.limit)
	}
	log.Printf("[LIMITS] %s +%d user=%s action=%s", hit.key, hit.amount, username, hit.action)
}

type ahrefsBillStamp struct {
	at     time.Time
	page   bool
	paired bool
}

var (
	ahrefsBillMu     sync.Mutex
	ahrefsBillRecent = map[string]ahrefsBillStamp{}
)

func ahrefsDuplicateBill(key, family string) bool {
	ahrefsBillMu.Lock()
	defer ahrefsBillMu.Unlock()
	window := 800 * time.Millisecond
	if family == "se-overview" {
		window = 2 * time.Minute
	}
	if last, ok := ahrefsBillRecent[key]; ok && time.Since(last.at) < window {
		return true
	}
	ahrefsBillRecent[key] = ahrefsBillStamp{at: time.Now()}
	return false
}

func ahrefsUsableResult(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return false
	}
	sample := trimmed
	if len(sample) > 65536 {
		sample = sample[:65536]
	}
	lower := strings.ToLower(string(sample))
	for _, phrase := range []string{
		"suspicious activity",
		"suspicious_activity",
		"temporarily restricted",
		"potential violations",
		"sign in to ahrefs",
		"sign in to your account",
		"domain not verified",
	} {
		if strings.Contains(lower, phrase) {
			return false
		}
	}
	if strings.Contains(lower, "see pricing") && strings.Contains(lower, "/pricing") {
		return false
	}
	if trimmed[0] == '{' && len(trimmed) <= 65536 {
		var obj map[string]interface{}
		if json.Unmarshal(trimmed, &obj) == nil {
			if _, ok := obj["error"]; ok {
				return false
			}
			if _, ok := obj["errors"]; ok {
				return false
			}
		}
	}
	return true
}

func ahrefsReadLimitBody(r *http.Request) []byte {
	if r.Body == nil || r.Method != http.MethodPost {
		return nil
	}
	_, _, credit := ahrefsCreditPath(r.URL.Path, r.URL.RawQuery, r.Referer())
	if !credit && r.URL.Path != "/v4/baTable" && !ahrefsExportPath(r.URL.Path) {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		return nil
	}
	return body
}

func ahrefsBlockLimit(w http.ResponseWriter, r *http.Request, hit ahrefsLimitHit, used, limit, ask int) {
	message := ahrefsLimitMessage(hit, used, limit, ask)
	log.Printf("[LIMITS] blocked %s user path=%s used=%d limit=%d ask=%d", hit.key, r.URL.Path, used, limit, ask)
	if strings.HasPrefix(r.URL.Path, "/v4/") || strings.Contains(r.Header.Get("Accept"), "application/json") {
		code := "daily_credit_limit_reached"
		if hit.isExport {
			code = "weekly_export_limit_reached"
		}
		payload, _ := json.Marshal(map[string]interface{}{"error": code, "message": message, "used": used, "limit": limit, "requested": ask})
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write(payload)
		return
	}
	title := "Daily Limit Reached"
	if hit.isExport {
		title = "Export Limit Reached"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><meta charset="UTF-8"><title>` + title + `</title>
<style>body{min-height:100vh;display:flex;align-items:center;justify-content:center;margin:0;background:#eef3f8;font-family:system-ui,sans-serif;color:#0f172a}
.card{width:min(440px,100%);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px;text-align:center}
h1{font-size:28px;margin:0 0 12px}p{color:#64748b;line-height:1.5}</style></head>
<body><div class="card"><h1>` + title + `</h1><p>` + message + `</p></div></body></html>`))
}
