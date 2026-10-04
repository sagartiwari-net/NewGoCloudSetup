package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type semrushCredit struct {
	action string
	family string
	target string
}

func semrushClassify(r *http.Request) (semrushCredit, bool) {
	if r == nil {
		return semrushCredit{}, false
	}
	method := r.Method
	p := strings.ToLower(r.URL.Path)
	q := semrushSubject(r)
	doc := semrushDocument(r)
	ref := strings.ToLower(r.Referer())
	onHome := strings.Contains(ref, "/home") || p == "/home" || p == "/home/"
	if onHome {
		return semrushCredit{}, false
	}
	if strings.HasPrefix(p, "/siteaudit") && !semrushSiteAuditReport(p) {
		return semrushCredit{}, false
	}
	switch {
	case method == http.MethodPost && strings.Contains(p, "/dpa/rpc") && strings.Contains(ref, "/analytics/overview"):
		return semrushCredit{"Domain overview", "domain-overview:" + strings.ToLower(q), semrushPretty("Domain Overview", q)}, true
	case method == http.MethodPost && strings.Contains(p, "/topic-research/api/researches/launch"):
		return semrushCredit{"Topic research", "topic-research:" + q, semrushPretty("Topic research", q)}, true
	case method == http.MethodPost && strings.Contains(p, "/spectrum/v1/gap/keywordslist"):
		return semrushCredit{"Keyword gap", "keyword-gap:" + q, semrushPretty("Keyword gap", q)}, true
	case method == http.MethodPost && strings.Contains(p, "/analytics/competitors/rpc"):
		return semrushCredit{"Competitive analysis", "competitors:" + q, semrushPretty("Competitive analysis", q)}, true
	case doc && strings.Contains(p, "/analytics/overview") && q != "":
		return semrushCredit{"Domain overview", "domain-overview:" + q, semrushPretty("Domain Overview", q)}, true
	case doc && strings.Contains(p, "/analytics/organic/positions") && q != "":
		return semrushCredit{"Organic rankings", "organic-positions:" + q, semrushPretty("Organic rankings", q)}, true
	case doc && strings.Contains(p, "/analytics/toppages") && q != "":
		return semrushCredit{"Top pages", "top-pages:" + q, semrushPretty("Top pages", q)}, true
	case method == http.MethodPost && strings.Contains(p, "/kwogw/v2/webapi") && !strings.Contains(p, "mini-kwogw") && strings.Contains(ref, "/analytics/keywordoverview"):
		phrase := semrushPhrase(r)
		if phrase == "" {
			return semrushCredit{}, false
		}
		return semrushCredit{"Keyword overview", "keyword-overview:" + strings.ToLower(phrase), semrushPretty("Keyword overview", phrase)}, true
	case doc && strings.Contains(p, "/analytics/keywordoverview") && firstQuery(r, "q") != "":
		return semrushCredit{"Keyword overview", "keyword-overview:" + strings.ToLower(firstQuery(r, "q")), semrushPretty("Keyword overview", firstQuery(r, "q"))}, true
	case doc && strings.Contains(p, "/analytics/keywordmagic") && q != "":
		return semrushCredit{"Keyword Magic", "keyword-magic:" + q, semrushPretty("Keyword Magic", q)}, true
	case doc && strings.Contains(p, "/analytics/keywordgap"):
		return semrushCredit{"Keyword gap", "keyword-gap-page:" + q, semrushPretty("Keyword gap", q)}, true
	case doc && strings.Contains(p, "/analytics/backlinks") && q != "":
		return semrushCredit{"Backlinks", "backlinks:" + q, semrushPretty("Backlinks", q)}, true
	case doc && strings.Contains(p, "/analytics/refdomains"):
		return semrushCredit{"Referring domains", "refdomains:" + q, semrushPretty("Referring domains", q)}, true
	case doc && semrushSiteAuditReport(p):
		return semrushCredit{"Site Audit", "site-audit:" + p, semrushPretty("Site Audit", semrushAuditName(p))}, true
	case strings.Contains(ref, "/analytics/traffic") && strings.Contains(p, "pagegroupsapigateway/"):
		return semrushCredit{"Page groups", "ta-page-groups", semrushPretty("Page groups", trafficSubject(r))}, true
	case strings.Contains(ref, "/analytics/traffic") && (strings.Contains(p, "getaudience") || strings.Contains(p, "listaudience")):
		return semrushCredit{"Audience overlap", "ta-audience", semrushPretty("Audience overlap", trafficSubject(r))}, true
	case strings.Contains(ref, "/analytics/traffic") && strings.Contains(p, "listsubfolders"):
		return semrushCredit{"Subfolders", "ta-subfolders", semrushPretty("Subfolders", trafficSubject(r))}, true
	case strings.Contains(ref, "/analytics/traffic") && strings.Contains(p, "listsubdomains"):
		return semrushCredit{"Subdomains", "ta-subdomains", semrushPretty("Subdomains", trafficSubject(r))}, true
	case strings.Contains(ref, "/analytics/traffic") && strings.Contains(p, "listdestinations"):
		return semrushCredit{"Destinations", "ta-destinations", semrushPretty("Destinations", trafficSubject(r))}, true
	case strings.Contains(ref, "/analytics/traffic") && (strings.Contains(p, "listsources") || strings.Contains(p, "getsources")):
		return semrushCredit{"Sources", "ta-sources", semrushPretty("Sources", trafficSubject(r))}, true
	case strings.Contains(ref, "/analytics/traffic") && (strings.Contains(p, "listpages") || strings.Contains(p, "listkeywords")):
		return semrushCredit{"Pages", "ta-pages", semrushPretty("Pages", trafficSubject(r))}, true
	case strings.Contains(ref, "/analytics/traffic") && strings.Contains(p, "getmarket"):
		return semrushCredit{"Market", "ta-market", semrushPretty("Market", trafficSubject(r))}, true
	case strings.Contains(ref, "/analytics/traffic") && strings.Contains(p, "gettarget"):
		return semrushCredit{"Traffic overview", "ta-overview", semrushPretty("Traffic overview", trafficSubject(r))}, true
	case doc && strings.Contains(p, "/tracking/landscape/"):
		return semrushCredit{"Prompt tracking", "prompt-tracking:" + p, semrushPretty("Prompt tracking", semrushAuditName(p))}, true
	default:
		return semrushCredit{}, false
	}
}

func semrushPhrase(r *http.Request) string {
	if q := firstQuery(r, "q", "phrase"); q != "" {
		return q
	}
	if ref := r.Referer(); ref != "" {
		if u, err := url.Parse(ref); err == nil && strings.Contains(strings.ToLower(u.Path), "keywordoverview") {
			if q := strings.TrimSpace(u.Query().Get("q")); q != "" {
				return q
			}
		}
	}
	body, _ := r.Context().Value(proxyBodyCacheKey).([]byte)
	if len(body) == 0 || len(body) > 1<<20 {
		return ""
	}
	text := string(body)
	for _, key := range []string{`"phrase":"`, `"keyword":"`, `"q":"`} {
		if i := strings.Index(text, key); i >= 0 {
			rest := text[i+len(key):]
			if end := strings.IndexByte(rest, '"'); end > 0 && end < 180 {
				return rest[:end]
			}
		}
	}
	return ""
}

func semrushSubject(r *http.Request) string {
	if q := firstQuery(r, "q", "phrase", "keyword", "domain"); q != "" {
		return q
	}
	if ref := r.Referer(); ref != "" {
		if u, err := url.Parse(ref); err == nil {
			if q := strings.TrimSpace(u.Query().Get("q")); q != "" {
				return q
			}
		}
	}
	body, _ := r.Context().Value(proxyBodyCacheKey).([]byte)
	if len(body) == 0 || len(body) > 1<<20 {
		return ""
	}
	text := string(body)
	for _, key := range []string{`"q":"`, `"phrase":"`, `"domain":"`, `"query":"`, `"search":"`} {
		if i := strings.Index(text, key); i >= 0 {
			rest := text[i+len(key):]
			if end := strings.IndexByte(rest, '"'); end > 0 && end < 160 {
				return rest[:end]
			}
		}
	}
	return ""
}

func firstQuery(r *http.Request, keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(r.URL.Query().Get(key)); v != "" {
			return v
		}
	}
	return ""
}

func semrushPretty(action, subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return action
	}
	return action + ": " + subject
}

func semrushSiteAuditReport(path string) bool {
	p := strings.Trim(strings.ToLower(path), "/")
	if p == "siteaudit" {
		return false
	}
	return strings.HasPrefix(p, "siteaudit/") && strings.Contains(p, "/campaign")
}

func semrushAuditName(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func trafficSubject(r *http.Request) string {
	if q := firstQuery(r, "q", "domain"); q != "" {
		return q
	}
	if ref := r.Referer(); ref != "" {
		if u, err := url.Parse(ref); err == nil {
			if q := strings.TrimSpace(u.Query().Get("q")); q != "" {
				return q
			}
		}
	}
	return ""
}

func semrushSessionUser(token string) string {
	if token == "" {
		return ""
	}
	db, err := openSemrushPanel()
	if err != nil {
		return ""
	}
	var username string
	_ = db.QueryRow(`SELECT username FROM live_sessions WHERE session_token=?`, token).Scan(&username)
	return username
}

func semrushMeter(username string) (used, limit, websiteID int, ok bool) {
	limit = 30
	db, err := openSemrushPanel()
	if err != nil || username == "" {
		return 0, limit, 0, false
	}
	cfg := loadConfig()
	var raw string
	err = db.QueryRow(`SELECT id, COALESCE(default_limits_json, '{}') FROM websites WHERE domain IN ('127.0.0.1:5141', ?) LIMIT 1`, cfg.PublicHost).Scan(&websiteID, &raw)
	if err != nil || websiteID == 0 {
		return 0, limit, 0, false
	}
	defaults := map[string]int{}
	_ = json.Unmarshal([]byte(raw), &defaults)
	if n, exists := defaults["credits"]; exists {
		limit = n
	}
	var meterLimit, meterUsed, custom int
	err = db.QueryRow(`SELECT COALESCE(m."limit", 0), COALESCE(m.used, 0), COALESCE(m.is_custom, 0)
		FROM panel_users u
		LEFT JOIN user_meters m ON m.user_id = u.id AND m.key = 'credits'
		WHERE u.website_id = ? AND u.username = ?`, websiteID, username).Scan(&meterLimit, &meterUsed, &custom)
	if err == nil {
		used = meterUsed
		if custom == 1 || meterLimit != 0 {
			limit = meterLimit
		}
	}
	return used, limit, websiteID, true
}

func semrushLimitVisible(username, token string) bool {
	db, err := openSemrushPanel()
	if err != nil || username == "" {
		return false
	}
	cfg := loadConfig()
	var websiteID int
	if db.QueryRow(`SELECT id FROM websites WHERE domain IN ('127.0.0.1:5141', ?) LIMIT 1`, cfg.PublicHost).Scan(&websiteID) != nil || websiteID == 0 {
		return false
	}
	var mode string
	_ = db.QueryRow(`SELECT COALESCE(limit_visibility, '') FROM panel_users WHERE website_id=? AND username=?`, websiteID, username).Scan(&mode)
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "show":
		return true
	case "hide":
		return false
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var accountID int
	if token != "" {
		_ = db.QueryRow(`SELECT COALESCE(assigned_account_id, 0) FROM live_sessions WHERE session_token=? AND expires_at>?`, token, now).Scan(&accountID)
	}
	if accountID == 0 {
		return false
	}
	var show int
	if db.QueryRow(`SELECT COALESCE(show_limit, 0) FROM accounts WHERE id=?`, accountID).Scan(&show) != nil {
		return false
	}
	return show == 1
}

func semrushDenyOverLimit(w http.ResponseWriter, r *http.Request) bool {
	hit, ok := semrushClassify(r)
	if !ok {
		return false
	}
	user := semrushSessionUser(semrushSessionToken(r))
	used, limit, _, ready := semrushMeter(user)
	if !ready || limit < 0 || used < limit {
		return false
	}
	_ = hit
	w.Header().Set("Cache-Control", "no-store")
	if semrushDocument(r) || strings.Contains(r.Header.Get("Accept"), "text/html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(semrushLightPage("Daily Limit Reached", "Daily Limit Reached", "Daily credit limit reached.", "", false, "")))
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	fmt.Fprintf(w, `{"error":"limit","message":"Daily credit limit reached.","used":%d,"limit":%d}`, used, limit)
	return true
}

func semrushRecordCredit(r *http.Request) {
	if r == nil {
		return
	}
	hit, ok := semrushClassify(r)
	if !ok {
		return
	}
	token := semrushSessionToken(r)
	user := semrushSessionUser(token)
	if user == "" {
		return
	}
	window := 2 * time.Minute
	if semrushDocument(r) {
		window = 800 * time.Millisecond
	}
	if semrushDuplicateCredit(user+"\n"+hit.family+"\n"+hit.target, window) {
		return
	}
	db, err := openSemrushPanel()
	if err != nil {
		return
	}
	_, limit, websiteID, ready := semrushMeter(user)
	if !ready || websiteID == 0 {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = db.Exec(`INSERT INTO usage_events (website_id, username, limit_key, limit_label, reset_days, action, target_path, amount, timestamp) VALUES (?,?,?,?,?,?,?,?,?)`,
		websiteID, user, "credits", "Credits", 1, hit.action, hit.target, 1, now)
	if err != nil {
		log.Printf("[LIMITS] usage insert failed user=%s action=%s: %v", user, hit.action, err)
		return
	}
	var userID int
	err = db.QueryRow(`SELECT id FROM panel_users WHERE website_id=? AND username=?`, websiteID, user).Scan(&userID)
	if err != nil {
		res, insertErr := db.Exec(`INSERT INTO panel_users (website_id, username, status) VALUES (?,?,?)`, websiteID, user, "active")
		if insertErr != nil {
			return
		}
		id, _ := res.LastInsertId()
		userID = int(id)
	}
	res, err := db.Exec(`UPDATE user_meters SET used = used + 1 WHERE user_id=? AND key='credits'`, userID)
	changed := int64(0)
	if err == nil && res != nil {
		changed, _ = res.RowsAffected()
	}
	if err != nil || changed == 0 {
		_, _ = db.Exec(`INSERT INTO user_meters (user_id, key, label, reset_days, used, "limit", is_custom) VALUES (?,?,?,?,?,?,0)`,
			userID, "credits", "Credits", 1, 1, limit)
	}
	log.Printf("[LIMITS] credits +1 user=%s action=%s", user, hit.action)
}

func semrushUserLimits(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	token := semrushSessionToken(r)
	user := semrushSessionUser(token)
	if user == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"show_limit":false}`))
		return
	}
	used, limit, _, _ := semrushMeter(user)
	fmt.Fprintf(w, `{"show_limit":%t,"credits_used":%d,"credits_limit":%d}`, semrushLimitVisible(user, token), used, limit)
}

const semrushLimitDock = `<script data-semrush-limit>
(function() {
  if (window.__semrushLimitDock) return;
  window.__semrushLimitDock = true;
  var open = false;
  function removeDock() {
    var el = document.getElementById("tm-limit-dock");
    if (el) el.remove();
  }
  function host() {
    var el = document.getElementById("tm-limit-dock");
    if (el) return el;
    el = document.createElement("div");
    el.id = "tm-limit-dock";
    var root = el.attachShadow({ mode: "open" });
    root.innerHTML = ""
      + "<style>"
      + ":host{all:initial}"
      + ".tab{position:fixed;left:16px;bottom:24px;z-index:2147483646;width:48px;height:48px;border:0;border-radius:999px;background:#22c55e;color:#fff;display:grid;place-items:center;box-shadow:0 10px 24px rgba(22,163,74,.35);cursor:pointer}"
      + ".tab svg{width:22px;height:22px;display:block}"
      + ".tab.warn{background:#f59e0b}.tab.low{background:#ef4444}.tab.hide{opacity:0;pointer-events:none}"
      + ".back{position:fixed;inset:0;z-index:2147483646;background:rgba(15,23,42,.18);opacity:0;pointer-events:none;transition:opacity .2s}"
      + ".back.show{opacity:1;pointer-events:auto}"
      + ".card{position:fixed;left:16px;bottom:24px;z-index:2147483647;width:232px;background:#f3fbf6;color:#14532d;border-radius:28px;box-shadow:0 22px 50px rgba(15,23,42,.18);padding:16px 16px 14px;box-sizing:border-box;font:500 14px/1.3 system-ui,sans-serif;transform:translateY(10px) scale(.96);opacity:0;pointer-events:none;transition:transform .22s ease,opacity .22s ease}"
      + ".card.show{transform:none;opacity:1;pointer-events:auto}"
      + ".head{display:flex;align-items:center;justify-content:space-between;margin-bottom:6px}"
      + ".title{font-weight:750;font-size:15px}"
      + ".x{border:0;background:#e7f6ec;color:#166534;width:28px;height:28px;border-radius:999px;cursor:pointer;font:700 16px/1 system-ui,sans-serif}"
      + ".ringwrap{position:relative;width:168px;height:168px;margin:4px auto 8px}"
      + ".ring{width:168px;height:168px;transform:rotate(-90deg)}"
      + ".track{fill:none;stroke:#d9f3e3;stroke-width:10}"
      + ".fill{fill:none;stroke:#22c55e;stroke-width:10;stroke-linecap:round;stroke-dasharray:289;stroke-dashoffset:0;transition:stroke-dashoffset .35s ease,stroke .2s}"
      + ".fill.warn{stroke:#f59e0b}.fill.low{stroke:#ef4444}"
      + ".center{position:absolute;inset:0;display:flex;flex-direction:column;align-items:center;justify-content:center}"
      + ".num{font-size:40px;font-weight:800;letter-spacing:-.04em;color:#14532d}"
      + ".sub{margin-top:2px;color:#4d7c5e;font-size:13px}"
      + ".meter{background:#fff;border-radius:16px;padding:10px 12px}"
      + ".meter-top{display:flex;justify-content:space-between;align-items:center;margin-bottom:8px;color:#166534;font-size:13px}"
      + ".count{font-weight:750}"
      + ".bar{height:6px;border-radius:999px;background:#e7f6ec;overflow:hidden}"
      + ".barfill{height:100%;width:0;border-radius:999px;background:#22c55e;transition:width .35s ease}"
      + ".barfill.warn{background:#f59e0b}.barfill.low{background:#ef4444}"
      + "<" + "/style>"
      + '<button class="tab" id="tm-tab" type="button" aria-label="Credits"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="8" fill="none" stroke="currentColor" stroke-width="2" opacity=".35"><' + '/circle><path d="M12 4a8 8 0 0 1 8 8" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round"><' + '/path><' + '/svg><' + '/button>'
      + '<div class="back" id="tm-back"><' + '/div>'
      + '<aside class="card" id="tm-drawer">'
      + '<div class="head"><div class="title">Credits<' + '/div><button class="x" id="tm-close" type="button" aria-label="Close">×<' + '/button><' + '/div>'
      + '<div class="ringwrap"><svg class="ring" viewBox="0 0 120 120"><circle class="track" cx="60" cy="60" r="46"><' + '/circle><circle class="fill" id="tm-ring" cx="60" cy="60" r="46"><' + '/circle><' + '/svg>'
      + '<div class="center"><div class="num" id="tm-left">0<' + '/div><div class="sub" id="tm-sub">left<' + '/div><' + '/div><' + '/div>'
      + '<div class="meter"><div class="meter-top"><span>Credits<' + '/span><span class="count" id="tm-count">0 / 0<' + '/span><' + '/div><div class="bar"><div class="barfill" id="tm-bar"><' + '/div><' + '/div><' + '/div>'
      + '<' + '/aside>';
    (document.body || document.documentElement).appendChild(el);
    root.getElementById("tm-tab").addEventListener("click", function(ev){ ev.stopPropagation(); setOpen(true); });
    root.getElementById("tm-close").addEventListener("click", function(ev){ ev.stopPropagation(); setOpen(false); });
    root.getElementById("tm-back").addEventListener("click", function(){ setOpen(false); });
    return el;
  }
  function setOpen(next) {
    open = next;
    var el = host();
    if (!el) return;
    var root = el.shadowRoot;
    root.getElementById("tm-drawer").classList.toggle("show", open);
    root.getElementById("tm-back").classList.toggle("show", open);
    root.getElementById("tm-tab").classList.toggle("hide", open);
  }
  function paint(d) {
    if (!d || !d.show_limit) { removeDock(); return; }
    var el = host();
    if (!el) return;
    var root = el.shadowRoot;
    var tab = root.getElementById("tm-tab");
    var ring = root.getElementById("tm-ring");
    var bar = root.getElementById("tm-bar");
    var used = Number(d.credits_used) || 0;
    var limit = Number(d.credits_limit);
    var unlimited = !(limit >= 0);
    var left = unlimited ? used : Math.max(0, limit - used);
    var ratio = unlimited ? 1 : (limit === 0 ? 0 : left / limit);
    root.getElementById("tm-left").textContent = unlimited ? "∞" : String(left);
    root.getElementById("tm-sub").textContent = unlimited ? "unlimited" : "left";
    root.getElementById("tm-count").textContent = unlimited ? (String(used) + " used") : (String(used) + " / " + String(limit));
    ring.style.strokeDasharray = "289";
    ring.style.strokeDashoffset = String(289 * (1 - ratio));
    bar.style.width = Math.round(ratio * 100) + "%";
    var low = !unlimited && left < 3;
    var warn = !unlimited && left >= 3 && left < 10;
    tab.classList.toggle("low", low);
    tab.classList.toggle("warn", warn);
    ring.classList.toggle("low", low);
    ring.classList.toggle("warn", warn);
    bar.classList.toggle("low", low);
    bar.classList.toggle("warn", warn);
  }
  function poll() {
    fetch("/api/user-limits", { credentials: "same-origin", cache: "no-store" })
      .then(function(r){ return r.json(); })
      .then(paint)
      .catch(function(){});
  }
  if (document.body) poll();
  else document.addEventListener("DOMContentLoaded", poll);
  setInterval(poll, 2000);
})();
</script>`

var (
	semrushCreditMu     sync.Mutex
	semrushCreditRecent = map[string]time.Time{}
)

func semrushDuplicateCredit(key string, window time.Duration) bool {
	semrushCreditMu.Lock()
	defer semrushCreditMu.Unlock()
	now := time.Now()
	if at, ok := semrushCreditRecent[key]; ok && now.Sub(at) < window {
		return true
	}
	semrushCreditRecent[key] = now
	if len(semrushCreditRecent) > 400 {
		for k, at := range semrushCreditRecent {
			if now.Sub(at) > 2*time.Minute {
				delete(semrushCreditRecent, k)
			}
		}
	}
	return false
}
