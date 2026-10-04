package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const (
	gzMeterKey   = "scans"
	gzMeterLabel = "Credits"
)

type panelCreditState struct {
	websiteID int
	userID    int
	limit     int
	used      int
	resetDays int
	blocked   bool
}

func creditLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return time.FixedZone("IST", 5*3600+30*60)
	}
	return loc
}

func periodStartIST(resetDays int, now time.Time) time.Time {
	if resetDays <= 0 {
		resetDays = 1
	}
	loc := creditLocation()
	now = now.In(loc)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	if resetDays == 1 {
		return day
	}
	// Multi-day windows align to epoch days in IST.
	epoch := time.Date(2020, 1, 1, 0, 0, 0, 0, loc)
	n := int(day.Sub(epoch).Hours() / 24)
	startN := n - (n % resetDays)
	return epoch.AddDate(0, 0, startN)
}

func creditExpirePassed(raw string) bool {
	raw = strings.TrimSpace(raw)
	loc := creditLocation()
	var day time.Time
	if t, err := time.ParseInLocation("2006-01-02", raw, loc); err == nil {
		day = t
	} else if t, err := time.Parse(time.RFC3339, raw); err == nil {
		t = t.In(loc)
		day = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	} else {
		return false
	}
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	return today.After(day)
}

func normalizeGPTZeroAPIPath(path string) string {
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}
	path = strings.TrimSuffix(path, "/")
	if strings.HasPrefix(path, "/api-proxy/") {
		path = "/" + strings.TrimPrefix(path, "/api-proxy/")
	}
	if strings.HasPrefix(path, "/extra-cdn-") {
		rest := path
		if i := strings.Index(rest[1:], "/"); i >= 0 {
			rest = rest[1+i:]
		}
		path = rest
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

// Billable scan/AI actions — 1 word = 1 credit per call (matches GPTZero UI).
func isBillableGPTZeroPath(method, path string) bool {
	if !strings.EqualFold(method, http.MethodPost) {
		return false
	}
	p := normalizeGPTZeroAPIPath(path)
	switch p {
	case "/v3/scan",
		"/v2/async/plagiarism/text",
		"/v3/async/bibliography-scan/text",
		"/v3/ai-vocab-scan",
		"/v3/ai/text/stream",
		"/v3/instant-writing-feedback":
		return true
	}
	if strings.HasSuffix(p, "/stream_review_documents") {
		return true
	}
	return false
}

var wordSplitRe = regexp.MustCompile(`\s+`)

func countWords(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	parts := wordSplitRe.Split(text, -1)
	n := 0
	for _, p := range parts {
		if p == "" {
			continue
		}
		has := false
		for _, r := range p {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				has = true
				break
			}
		}
		if has {
			n++
		}
	}
	return n
}

func extractTextFromJSON(v interface{}, depth int) string {
	if depth > 8 || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if len(s) >= 20 {
			return s
		}
		return ""
	case map[string]interface{}:
		preferred := []string{"document", "text", "content", "input", "prompt", "body", "raw_text", "source_text"}
		for _, k := range preferred {
			if raw, ok := t[k]; ok {
				if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
					return s
				}
			}
		}
		best := ""
		for k, raw := range t {
			lk := strings.ToLower(k)
			if strings.Contains(lk, "token") || strings.Contains(lk, "password") || strings.Contains(lk, "cookie") {
				continue
			}
			if s := extractTextFromJSON(raw, depth+1); len(s) > len(best) {
				best = s
			}
		}
		return best
	case []interface{}:
		best := ""
		for _, item := range t {
			if s := extractTextFromJSON(item, depth+1); len(s) > len(best) {
				best = s
			}
		}
		return best
	default:
		return ""
	}
}

func creditAmountFromBody(body []byte) int {
	if len(body) == 0 {
		return 1
	}
	var v interface{}
	if json.Unmarshal(body, &v) == nil {
		if s := extractTextFromJSON(v, 0); s != "" {
			if n := countWords(s); n > 0 {
				return n
			}
		}
	}
	// Non-JSON / unparsed — charge at least 1 so free abuse is blocked.
	return 1
}

func panelCreditStateFor(username string) (panelCreditState, error) {
	state := panelCreditState{resetDays: 1, limit: 10000}
	cfg := loadConfig()
	db, err := openPanelDB(cfg)
	if err != nil {
		return state, err
	}
	var rawDefaults string
	if err := db.QueryRow(`SELECT id, COALESCE(default_limits_json, '{}') FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&state.websiteID, &rawDefaults); err != nil {
		return state, err
	}
	defaults := map[string]int{}
	_ = json.Unmarshal([]byte(rawDefaults), &defaults)
	if n, ok := defaults[gzMeterKey]; ok {
		state.limit = n
	} else if n, ok := defaults["credits"]; ok {
		state.limit = n
	}

	var status, expire, periodStart string
	var meterID, meterLimit, meterUsed, resetDays, custom int
	err = db.QueryRow(`SELECT u.id, u.status, COALESCE(u.custom_limit_expire_at, ''),
		COALESCE(m.id, 0), COALESCE(m."limit", 0), COALESCE(m.used, 0), COALESCE(m.reset_days, 1), COALESCE(m.is_custom, 0), COALESCE(m.period_start, '')
		FROM panel_users u
		LEFT JOIN user_meters m ON m.user_id=u.id AND m.key=?
		WHERE u.website_id=? AND u.username=?`, gzMeterKey, state.websiteID, username).
		Scan(&state.userID, &status, &expire, &meterID, &meterLimit, &meterUsed, &resetDays, &custom, &periodStart)
	if err != nil {
		return state, nil
	}
	if status == "suspended" {
		state.blocked = true
	}
	if resetDays > 0 {
		state.resetDays = resetDays
	}
	customActive := custom == 1
	if expire != "" && creditExpirePassed(expire) {
		customActive = false
	}
	state.used = meterUsed
	if meterID > 0 {
		state.limit = meterLimit
	} else if customActive {
		state.limit = meterLimit
	}

	// Align used with current IST period (daily by default).
	start := periodStartIST(state.resetDays, time.Now()).UTC().Format(time.RFC3339)
	if meterID > 0 && (periodStart == "" || periodStart < start) {
		var used int
		_ = db.QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM usage_events WHERE website_id=? AND username=? AND limit_key=? AND timestamp>=?`,
			state.websiteID, username, gzMeterKey, start).Scan(&used)
		_, _ = db.Exec(`UPDATE user_meters SET used=?, period_start=? WHERE id=?`, used, start, meterID)
		state.used = used
	}
	return state, nil
}

func panelCreditsAllowAmount(username string, amount int) bool {
	if amount < 1 {
		amount = 1
	}
	state, err := panelCreditStateFor(username)
	if err != nil {
		log.Printf("[CREDIT] check failed user=%s: %v", username, err)
		return true
	}
	if state.blocked || state.limit == 0 {
		return false
	}
	if state.limit < 0 {
		return true
	}
	return state.used+amount <= state.limit
}

func panelCreditsChargeAmount(username, path string, amount int) {
	if amount < 1 {
		amount = 1
	}
	state, err := panelCreditStateFor(username)
	if err != nil || state.websiteID == 0 {
		log.Printf("[CREDIT] charge skipped user=%s: %v", username, err)
		return
	}
	db, err := openPanelDB(loadConfig())
	if err != nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	start := periodStartIST(state.resetDays, time.Now()).UTC().Format(time.RFC3339)
	_, err = db.Exec(`INSERT INTO usage_events (website_id, username, limit_key, limit_label, reset_days, action, target_path, amount, timestamp)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		state.websiteID, username, gzMeterKey, gzMeterLabel, state.resetDays, "scan", path, amount, now)
	if err != nil {
		log.Printf("[CREDIT] usage insert failed user=%s: %v", username, err)
		return
	}
	if state.userID == 0 {
		res, err := db.Exec(`INSERT INTO panel_users (website_id, username, status, custom_limit_expire_at) VALUES (?,?,?,NULL)`,
			state.websiteID, username, "active")
		if err != nil {
			log.Printf("[CREDIT] user create failed user=%s: %v", username, err)
			return
		}
		id, _ := res.LastInsertId()
		state.userID = int(id)
		_, _ = db.Exec(`INSERT INTO user_meters (user_id, key, label, reset_days, used, "limit", is_custom, period_start) VALUES (?,?,?,?,?,?,0,?)`,
			state.userID, gzMeterKey, gzMeterLabel, state.resetDays, amount, state.limit, start)
	} else {
		res, err := db.Exec(`UPDATE user_meters SET used = used + ?, period_start=CASE WHEN period_start='' OR period_start<? THEN ? ELSE period_start END WHERE user_id=? AND key=?`,
			amount, start, start, state.userID, gzMeterKey)
		changed := int64(0)
		if err == nil && res != nil {
			changed, _ = res.RowsAffected()
		}
		if err != nil || changed == 0 {
			_, _ = db.Exec(`INSERT INTO user_meters (user_id, key, label, reset_days, used, "limit", is_custom, period_start) VALUES (?,?,?,?,?,?,0,?)`,
				state.userID, gzMeterKey, gzMeterLabel, state.resetDays, state.used+amount, state.limit, start)
		}
	}
	log.Printf("[CREDIT] +%d user=%s used~%d/%d path=%s", amount, username, state.used+amount, state.limit, path)
}

func panelAssignedShowLimit(r *http.Request, cfg Config, username string) bool {
	db, err := openPanelDB(cfg)
	if err != nil {
		return false
	}
	var websiteID, accountID int
	if err = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID); err != nil || websiteID == 0 {
		return false
	}
	if username != "" {
		var mode string
		_ = db.QueryRow(`SELECT COALESCE(limit_visibility, '') FROM panel_users WHERE website_id=? AND username=?`, websiteID, username).Scan(&mode)
		switch strings.ToLower(strings.TrimSpace(mode)) {
		case "show":
			return true
		case "hide":
			return false
		}
	}
	token := ctSessionToken(r)
	now := time.Now().UTC().Format(time.RFC3339)
	if token != "" {
		_ = db.QueryRow(`SELECT assigned_account_id FROM live_sessions WHERE session_token=? AND website_id=? AND expires_at>?`, token, websiteID, now).Scan(&accountID)
	}
	if accountID == 0 && username != "" {
		_ = db.QueryRow(`SELECT assigned_account_id FROM live_sessions WHERE username=? AND website_id=? AND expires_at>? ORDER BY id DESC LIMIT 1`, username, websiteID, now).Scan(&accountID)
	}
	if accountID == 0 {
		return false
	}
	var show int
	if err = db.QueryRow(`SELECT COALESCE(show_limit, 0) FROM accounts WHERE id=?`, accountID).Scan(&show); err != nil {
		return false
	}
	return show == 1
}

func rejectCreditLimit(w http.ResponseWriter, r *http.Request, cfg Config) {
	log.Printf("[CREDIT] blocked path=%s", r.URL.Path)
	if wantsLightDenied(r, r.URL.Path) {
		name := toolDisplayName(cfg)
		writeLightCard(w, http.StatusForbidden, lightCard{
			Title:   "Daily Limit Reached",
			Heading: "Daily Limit Reached",
			Message: "You have used the daily " + gzMeterLabel + " limit for <span class=\"brand\">" + name + "</span>. The limit resets at midnight (12:00 AM IST).",
			Footer:  "This limit applies to the current access",
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-TM-Limit", "credit")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusForbidden)
	fmt.Fprintf(w, `{"error":{"message":"Daily limit reached","code":"limit_reached","label":%q}}`, gzMeterLabel)
}

func userLimitsAPIHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	username, err := panelSessionUsername(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "unauthorized"})
		return
	}
	state, _ := panelCreditStateFor(username)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"username":     username,
		"credit_limit": state.limit,
		"credit_used":  state.used,
		"tool_name":    toolDisplayName(cfg),
		"show_limit":   panelAssignedShowLimit(r, cfg, username),
		"credit_label": gzMeterLabel,
	})
}

func limitWidgetScript(toolName string) string {
	name := strings.TrimSpace(toolName)
	if name == "" {
		name = "GPTZero"
	}
	return fmt.Sprintf(`<script data-tm-credits="1">
(function() {
    var CREDIT_LABEL = %q;
    var TOOL_NAME = %q;
    var open = false;
    var latest = null;
    function removeDock() {
        var el = document.getElementById('tm-limit-dock');
        if (el) el.remove();
    }
    function host() {
        if (latest && !latest.show_limit) { removeDock(); return null; }
        var el = document.getElementById('tm-limit-dock');
        if (el) return el;
        el = document.createElement('div');
        el.id = 'tm-limit-dock';
        var root = el.attachShadow({ mode: 'open' });
        root.innerHTML = ''
            + '<style>'
            + ':host{all:initial}'
            + '.tab{position:fixed;left:16px;bottom:24px;z-index:2147483646;width:48px;height:48px;border:0;border-radius:999px;background:#22c55e;color:#fff;display:grid;place-items:center;box-shadow:0 10px 24px rgba(22,163,74,.35);cursor:pointer}'
            + '.tab svg{width:22px;height:22px;display:block}'
            + '.tab.warn{background:#f59e0b}.tab.low{background:#ef4444}.tab.hide{opacity:0;pointer-events:none}'
            + '.back{position:fixed;inset:0;z-index:2147483646;background:rgba(15,23,42,.18);opacity:0;pointer-events:none;transition:opacity .2s}'
            + '.back.show{opacity:1;pointer-events:auto}'
            + '.card{position:fixed;left:16px;bottom:24px;z-index:2147483647;width:232px;background:#f3fbf6;color:#14532d;border-radius:28px;box-shadow:0 22px 50px rgba(15,23,42,.18);padding:16px 16px 14px;box-sizing:border-box;font:500 14px/1.3 system-ui,sans-serif;transform:translateY(10px) scale(.96);opacity:0;pointer-events:none;transition:transform .22s ease,opacity .22s ease}'
            + '.card.show{transform:none;opacity:1;pointer-events:auto}'
            + '.head{display:flex;align-items:center;justify-content:space-between;margin-bottom:6px}'
            + '.title{font-weight:750;font-size:15px}'
            + '.x{border:0;background:#e7f6ec;color:#166534;width:28px;height:28px;border-radius:999px;cursor:pointer;font:700 16px/1 system-ui,sans-serif}'
            + '.ringwrap{position:relative;width:168px;height:168px;margin:4px auto 8px}'
            + '.ring{width:168px;height:168px;transform:rotate(-90deg)}'
            + '.track{fill:none;stroke:#d9f3e3;stroke-width:10}'
            + '.fill{fill:none;stroke:#22c55e;stroke-width:10;stroke-linecap:round;stroke-dasharray:289;stroke-dashoffset:0;transition:stroke-dashoffset .35s ease,stroke .2s}'
            + '.fill.warn{stroke:#f59e0b}.fill.low{stroke:#ef4444}'
            + '.center{position:absolute;inset:0;display:flex;flex-direction:column;align-items:center;justify-content:center}'
            + '.num{font-size:40px;font-weight:800;letter-spacing:-.04em;color:#14532d}'
            + '.sub{margin-top:2px;color:#4d7c5e;font-size:13px}'
            + '.meter{background:#fff;border-radius:16px;padding:10px 12px}'
            + '.meter-top{display:flex;justify-content:space-between;align-items:center;margin-bottom:8px;color:#166534;font-size:13px}'
            + '.count{font-weight:750}'
            + '.bar{height:6px;border-radius:999px;background:#e7f6ec;overflow:hidden}'
            + '.barfill{height:100%%;width:0;border-radius:999px;background:#22c55e;transition:width .35s ease}'
            + '.barfill.warn{background:#f59e0b}.barfill.low{background:#ef4444}'
            + '<' + '/style>'
            + '<button class="tab" id="tm-tab" type="button" aria-label="Credits"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="8" fill="none" stroke="currentColor" stroke-width="2" opacity=".35"><' + '/circle><path d="M12 4a8 8 0 0 1 8 8" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round"><' + '/path><' + '/svg><' + '/button>'
            + '<div class="back" id="tm-back"><' + '/div>'
            + '<aside class="card" id="tm-drawer">'
            + '<div class="head"><div class="title" id="tm-title">Credits<' + '/div><button class="x" id="tm-close" type="button" aria-label="Close">×<' + '/button><' + '/div>'
            + '<div class="ringwrap"><svg class="ring" viewBox="0 0 120 120"><circle class="track" cx="60" cy="60" r="46"><' + '/circle><circle class="fill" id="tm-ring" cx="60" cy="60" r="46"><' + '/circle><' + '/svg>'
            + '<div class="center"><div class="num" id="tm-left">0<' + '/div><div class="sub" id="tm-sub">left<' + '/div><' + '/div><' + '/div>'
            + '<div class="meter"><div class="meter-top"><span id="tm-meter-label">Credits<' + '/span><span class="count" id="tm-count">0 / 0<' + '/span><' + '/div><div class="bar"><div class="barfill" id="tm-bar"><' + '/div><' + '/div><' + '/div>'
            + '<' + '/aside>';
        (document.body || document.documentElement).appendChild(el);
        var shadow = root;
        shadow.getElementById('tm-tab').addEventListener('click', function(ev){ ev.stopPropagation(); setOpen(true); });
        shadow.getElementById('tm-close').addEventListener('click', function(ev){ ev.stopPropagation(); setOpen(false); });
        shadow.getElementById('tm-back').addEventListener('click', function(){ setOpen(false); });
        shadow.getElementById('tm-drawer').addEventListener('click', function(ev){ ev.stopPropagation(); });
        return el;
    }
    function setOpen(next) {
        open = next;
        var el = host();
        if (!el) return;
        var root = el.shadowRoot;
        root.getElementById('tm-drawer').classList.toggle('show', open);
        root.getElementById('tm-back').classList.toggle('show', open);
        root.getElementById('tm-tab').classList.toggle('hide', open);
    }
    function paint(d) {
        if (!d || !d.show_limit) { removeDock(); return; }
        var el = host();
        if (!el) return;
        var root = el.shadowRoot;
        root.getElementById('tm-title').textContent = CREDIT_LABEL;
        root.getElementById('tm-meter-label').textContent = CREDIT_LABEL;
        var tab = root.getElementById('tm-tab');
        var ring = root.getElementById('tm-ring');
        var bar = root.getElementById('tm-bar');
        var used = Number(d.credit_used) || 0;
        var limit = Number(d.credit_limit);
        var unlimited = !(limit >= 0);
        var left = unlimited ? used : Math.max(0, limit - used);
        var ratio = unlimited ? 1 : (limit === 0 ? 0 : left / limit);
        root.getElementById('tm-left').textContent = unlimited ? '∞' : String(left);
        root.getElementById('tm-sub').textContent = unlimited ? 'unlimited' : 'left';
        root.getElementById('tm-count').textContent = unlimited ? (String(used) + ' used') : (String(used) + ' / ' + String(limit));
        ring.style.strokeDasharray = '289';
        ring.style.strokeDashoffset = String(289 * (1 - ratio));
        bar.style.width = Math.round(ratio * 100) + '%%';
        var lowCut = unlimited ? 0 : Math.max(50, Math.floor(limit * 0.03));
        var warnCut = unlimited ? 0 : Math.max(200, Math.floor(limit * 0.10));
        var low = !unlimited && left < lowCut;
        var warn = !unlimited && !low && left < warnCut;
        tab.classList.toggle('low', low);
        tab.classList.toggle('warn', warn);
        ring.classList.toggle('low', low);
        ring.classList.toggle('warn', warn);
        bar.classList.toggle('low', low);
        bar.classList.toggle('warn', warn);
    }
    function showLimitCard() {
        if (document.getElementById('tm-limit-screen')) return;
        var el = document.createElement('div');
        el.id = 'tm-limit-screen';
        el.style.cssText = 'position:fixed;inset:0;z-index:2147483647;background:#eef3f8;color:#0f172a;display:flex;align-items:center;justify-content:center;padding:24px;font-family:system-ui,-apple-system,Segoe UI,sans-serif;';
        el.innerHTML = '<div style="width:min(440px,92vw);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px 36px;text-align:center;">'
            + '<div style="width:78px;height:78px;margin:0 auto 22px;border-radius:50%%;background:#fff;display:grid;place-items:center;font-size:28px;border:1px solid #e6ebf2;">&#128274;<' + '/div>'
            + '<h1 style="font-size:28px;line-height:1.2;font-weight:800;letter-spacing:-.03em;margin:0 0 12px;">Daily Limit Reached<' + '/h1>'
            + '<p style="color:#64748b;font-size:15px;line-height:1.55;margin:0;">You have used the daily ' + CREDIT_LABEL + ' limit for ' + TOOL_NAME + '. The limit resets at midnight (12:00 AM IST).<' + '/p>'
            + '<p style="margin:22px 0 0;color:#94a3b8;font-size:13px;">This limit applies to the current access<' + '/p><' + '/div>';
        (document.body || document.documentElement).appendChild(el);
    }
    function noCreditsLeft() {
        if (!latest || !latest.show_limit) return false;
        var limit = Number(latest.credit_limit);
        if (!(limit >= 0)) return false;
        var used = Number(latest.credit_used) || 0;
        return used >= limit;
    }
    function isBillable(url) {
        var u = String(url || '');
        try { u = new URL(u, location.origin).pathname; } catch (e) {}
        u = u.replace(/\/api-proxy/, '').replace(/\/extra-cdn-\d+/, '');
        if (u === '/v3/scan') return true;
        if (u.indexOf('/v2/async/plagiarism/text') !== -1) return true;
        if (u.indexOf('/v3/async/bibliography-scan/text') !== -1) return true;
        if (u.indexOf('/v3/ai-vocab-scan') !== -1) return true;
        if (u.indexOf('/v3/ai/text/stream') !== -1) return true;
        if (u.indexOf('/v3/instant-writing-feedback') !== -1) return true;
        if (u.indexOf('/stream_review_documents') !== -1) return true;
        return false;
    }
    function blockedResponse() {
        showLimitCard();
        return new Response(JSON.stringify({error:{message:'Daily limit reached',code:'limit_reached'}}), {
            status: 403,
            headers: { 'Content-Type': 'application/json', 'X-TM-Limit': 'credit' }
        });
    }
    function updateBadge() {
        window.fetch('/api/user-limits', { credentials: 'same-origin', cache: 'no-store' })
            .then(function(r){ return r.json(); })
            .then(function(d){
                latest = d;
                paint(d);
                try { if (window.__tmPaintNativeCredits) window.__tmPaintNativeCredits(d); } catch (e) {}
            })
            .catch(function(){});
    }
    window.__tmUpdateCredits = updateBadge;
    function boot() { updateBadge(); }
    if (document.body) boot();
    else document.addEventListener('DOMContentLoaded', boot);
    setInterval(function(){ updateBadge(); }, 4000);
    var orig = window.fetch;
    window.fetch = function(input, init) {
        var url = typeof input === 'string' ? input : (input && input.url) || '';
        var target = String(url);
        var method = 'GET';
        if (init && init.method) method = String(init.method).toUpperCase();
        else if (input && input.method) method = String(input.method).toUpperCase();
        if (method === 'POST' && noCreditsLeft() && isBillable(target)) return Promise.resolve(blockedResponse());
        var done = orig.apply(this, arguments);
        if (method === 'POST' && isBillable(target)) {
            return done.then(function(res) {
                if (res && res.headers && res.headers.get('X-TM-Limit')) {
                    showLimitCard();
                    return new Response(JSON.stringify({error:'limit_reached'}), { status: 403, headers: { 'Content-Type': 'application/json' } });
                }
                if (res && res.ok) setTimeout(updateBadge, 400);
                return res;
            });
        }
        return done;
    };
})();
</script>`, gzMeterLabel, name)
}
