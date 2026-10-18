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
	whWordsKey      = "words"
	whWordsLabel    = "Words"
	whWordsDefault  = 10000
	whImagesKey     = "images"
	whImagesLabel   = "Images"
	whImagesDefault = 50
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

func writeHumanBill(method, path string, body []byte) (key, label string, amount int) {
	if !strings.EqualFold(method, http.MethodPost) {
		return "", "", 0
	}
	switch normalizeGPTZeroAPIPath(path) {
	case "/api/humanize", "/api/detect":
		n := wordsInJSONText(body)
		if n < 1 {
			return "", "", 0
		}
		return whWordsKey, whWordsLabel, n
	case "/api/detect-image":
		return whImagesKey, whImagesLabel, 1
	default:
		return "", "", 0
	}
}

func wordsInJSONText(body []byte) int {
	if len(body) == 0 {
		return 0
	}
	var v map[string]interface{}
	if json.Unmarshal(body, &v) != nil {
		return 0
	}
	raw, _ := v["text"].(string)
	return countWords(raw)
}

func panelMeterFallback(key string) int {
	if key == whImagesKey {
		return whImagesDefault
	}
	return whWordsDefault
}

func panelMeterState(username, key string) (panelCreditState, error) {
	state := panelCreditState{resetDays: 1, limit: panelMeterFallback(key)}
	cfg := loadConfig()
	db, err := openPanelDB(cfg)
	if err != nil {
		return state, err
	}
	var rawDefaults string
	state.websiteID = panelWebsiteIDForUser(db, cfg.PublicHost, username)
	if state.websiteID <= 0 {
		return state, fmt.Errorf("website not found")
	}
	if err := db.QueryRow(`SELECT COALESCE(default_limits_json, '{}') FROM websites WHERE id=?`, state.websiteID).Scan(&rawDefaults); err != nil {
		return state, err
	}
	defaults := map[string]int{}
	_ = json.Unmarshal([]byte(rawDefaults), &defaults)
	if n, ok := defaults[key]; ok {
		state.limit = n
	}

	var status, expire, periodStart string
	var meterID, meterLimit, meterUsed, resetDays, custom int
	err = db.QueryRow(`SELECT u.id, u.status, COALESCE(u.custom_limit_expire_at, ''),
		COALESCE(m.id, 0), COALESCE(m."limit", 0), COALESCE(m.used, 0), COALESCE(m.reset_days, 1), COALESCE(m.is_custom, 0), COALESCE(m.period_start, '')
		FROM panel_users u
		LEFT JOIN user_meters m ON m.user_id=u.id AND m.key=?
		WHERE u.website_id=? AND u.username=?`, key, state.websiteID, username).
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
	if meterID > 0 && (custom == 0 || customActive) {
		state.limit = meterLimit
	}

	// Align used with the current IST day.
	start := periodStartIST(state.resetDays, time.Now()).UTC().Format(time.RFC3339)
	if meterID > 0 && (periodStart == "" || periodStart < start) {
		var used int
		_ = db.QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM usage_events WHERE website_id=? AND username=? AND limit_key=? AND timestamp>=?`,
			state.websiteID, username, key, start).Scan(&used)
		_, _ = db.Exec(`UPDATE user_meters SET used=?, period_start=? WHERE id=?`, used, start, meterID)
		state.used = used
	}
	return state, nil
}

func panelMeterAllow(username, key string, amount int) bool {
	if amount < 1 {
		return true
	}
	state, err := panelMeterState(username, key)
	if err != nil {
		log.Printf("[CREDIT] check failed user=%s key=%s: %v", username, key, err)
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

func panelMeterCharge(username, key, label, path string, amount int) {
	if amount < 1 {
		return
	}
	state, err := panelMeterState(username, key)
	if err != nil || state.websiteID == 0 {
		log.Printf("[CREDIT] charge skipped user=%s key=%s: %v", username, key, err)
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
		state.websiteID, username, key, label, state.resetDays, "use", path, amount, now)
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
		_, _ = db.Exec(`INSERT INTO user_meters (user_id, "key", label, reset_days, used, "limit", is_custom, period_start) VALUES (?,?,?,?,?,?,0,?)`,
			state.userID, key, label, state.resetDays, amount, state.limit, start)
	} else {
		res, err := db.Exec(`UPDATE user_meters SET used = used + ?, period_start=CASE WHEN period_start='' OR period_start<? THEN ? ELSE period_start END WHERE user_id=? AND "key"=?`,
			amount, start, start, state.userID, key)
		changed := int64(0)
		if err == nil && res != nil {
			changed, _ = res.RowsAffected()
		}
		if err != nil || changed == 0 {
			_, _ = db.Exec(`INSERT INTO user_meters (user_id, "key", label, reset_days, used, "limit", is_custom, period_start) VALUES (?,?,?,?,?,?,0,?)`,
				state.userID, key, label, state.resetDays, state.used+amount, state.limit, start)
		}
	}
	log.Printf("[CREDIT] +%d %s user=%s used~%d/%d path=%s", amount, key, username, state.used+amount, state.limit, path)
}

func panelAssignedShowLimit(r *http.Request, cfg Config, username string) bool {
	db, err := openPanelDB(cfg)
	if err != nil {
		return false
	}
	var websiteID, accountID int
	websiteID = panelWebsiteIDForUser(db, cfg.PublicHost, username)
	if websiteID == 0 {
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

func rejectWriteHumanLimit(w http.ResponseWriter, r *http.Request, cfg Config, key, label string) {
	log.Printf("[CREDIT] blocked path=%s key=%s", r.URL.Path, key)
	name := toolDisplayName(cfg)
	message := "You have used the daily " + label + " limit for " + name + ". The limit resets at midnight (12:00 AM IST)."
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-TM-Limit", key)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
		return
	}
	writeLightCard(w, http.StatusForbidden, lightCard{
		Title:   "Daily Limit Reached",
		Heading: "Daily Limit Reached",
		Message: "You have used the daily " + label + " limit for <span class=\"brand\">" + name + "</span>. The limit resets at midnight (12:00 AM IST).",
		Footer:  "This limit applies to the current access",
	})
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
	words, _ := panelMeterState(username, whWordsKey)
	images, _ := panelMeterState(username, whImagesKey)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"username":     username,
		"credit_limit": words.limit,
		"credit_used":  words.used,
		"credit_label": whWordsLabel,
		"image_limit":  images.limit,
		"image_used":   images.used,
		"image_label":  whImagesLabel,
		"tool_name":    toolDisplayName(cfg),
		"show_limit":   panelAssignedShowLimit(r, cfg, username),
	})
}

func limitWidgetScript(toolName string) string {
	name := strings.TrimSpace(toolName)
	if name == "" {
		name = "WriteHuman"
	}
	return fmt.Sprintf(`<script data-tm-credits="1">
(function() {
    var WORD_LABEL = %q;
    var IMAGE_LABEL = %q;
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
            + '<div class="head"><div class="title" id="tm-title">Limits<' + '/div><button class="x" id="tm-close" type="button" aria-label="Close">×<' + '/button><' + '/div>'
            + '<div class="ringwrap"><svg class="ring" viewBox="0 0 120 120"><circle class="track" cx="60" cy="60" r="46"><' + '/circle><circle class="fill" id="tm-ring" cx="60" cy="60" r="46"><' + '/circle><' + '/svg>'
            + '<div class="center"><div class="num" id="tm-left">0<' + '/div><div class="sub" id="tm-sub">words left<' + '/div><' + '/div><' + '/div>'
            + '<div class="meter"><div class="meter-top"><span id="tm-meter-label">Words<' + '/span><span class="count" id="tm-count">0 / 0<' + '/span><' + '/div><div class="bar"><div class="barfill" id="tm-bar"><' + '/div><' + '/div><' + '/div>'
            + '<div class="meter" style="margin-top:8px"><div class="meter-top"><span id="tm-meter-label-2">Images<' + '/span><span class="count" id="tm-count-2">0 / 0<' + '/span><' + '/div><div class="bar"><div class="barfill" id="tm-bar-2"><' + '/div><' + '/div><' + '/div>'
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
        root.getElementById('tm-title').textContent = 'Limits';
        root.getElementById('tm-meter-label').textContent = d.credit_label || WORD_LABEL;
        root.getElementById('tm-meter-label-2').textContent = d.image_label || IMAGE_LABEL;
        var tab = root.getElementById('tm-tab');
        var ring = root.getElementById('tm-ring');
        var bar = root.getElementById('tm-bar');
        var used = Number(d.credit_used) || 0;
        var limit = Number(d.credit_limit);
        var unlimited = !(limit >= 0);
        var left = unlimited ? used : Math.max(0, limit - used);
        var ratio = unlimited ? 1 : (limit === 0 ? 0 : left / limit);
        root.getElementById('tm-left').textContent = unlimited ? '∞' : String(left);
        root.getElementById('tm-sub').textContent = unlimited ? 'unlimited' : 'words left';
        root.getElementById('tm-count').textContent = unlimited ? (String(used) + ' used') : (String(used) + ' / ' + String(limit));
        var imageUsed = Number(d.image_used) || 0;
        var imageLimit = Number(d.image_limit);
        var imageUnlimited = !(imageLimit >= 0);
        var imageLeft = imageUnlimited ? imageUsed : Math.max(0, imageLimit - imageUsed);
        var imageRatio = imageUnlimited ? 1 : (imageLimit === 0 ? 0 : imageLeft / imageLimit);
        root.getElementById('tm-count-2').textContent = imageUnlimited ? (String(imageUsed) + ' used') : (String(imageUsed) + ' / ' + String(imageLimit));
        var imageBar = root.getElementById('tm-bar-2');
        imageBar.style.width = Math.round(imageRatio * 100) + '%%';
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
    function showLimitCard(which) {
        if (document.getElementById('tm-limit-screen')) return;
        var label = which === 'images' ? IMAGE_LABEL : WORD_LABEL;
        var el = document.createElement('div');
        el.id = 'tm-limit-screen';
        el.style.cssText = 'position:fixed;inset:0;z-index:2147483647;background:#eef3f8;color:#0f172a;display:flex;align-items:center;justify-content:center;padding:24px;font-family:system-ui,-apple-system,Segoe UI,sans-serif;';
        el.innerHTML = '<div style="width:min(440px,92vw);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px 36px;text-align:center;">'
            + '<div style="width:78px;height:78px;margin:0 auto 22px;border-radius:50%%;background:#fff;display:grid;place-items:center;font-size:28px;border:1px solid #e6ebf2;">&#128274;<' + '/div>'
            + '<h1 style="font-size:28px;line-height:1.2;font-weight:800;letter-spacing:-.03em;margin:0 0 12px;">Daily Limit Reached<' + '/h1>'
            + '<p style="color:#64748b;font-size:15px;line-height:1.55;margin:0;">You have used the daily ' + label + ' limit for ' + TOOL_NAME + '. The limit resets at midnight (12:00 AM IST).<' + '/p>'
            + '<p style="margin:22px 0 0;color:#94a3b8;font-size:13px;">This limit applies to the current access<' + '/p><' + '/div>';
        (document.body || document.documentElement).appendChild(el);
    }
    function meterFor(url) {
        var u = String(url || '');
        try { u = new URL(u, location.origin).pathname; } catch (e) {}
        if (u === '/api/detect-image') return 'images';
        if (u === '/api/humanize' || u === '/api/detect') return 'words';
        return '';
    }
    function noCreditsLeft(url) {
        if (!latest || !latest.show_limit) return false;
        var which = meterFor(url);
        var limit = which === 'images' ? Number(latest.image_limit) : Number(latest.credit_limit);
        var used = which === 'images' ? Number(latest.image_used) || 0 : Number(latest.credit_used) || 0;
        if (!which || !(limit >= 0)) return false;
        return used >= limit;
    }
    function isBillable(url) {
        return meterFor(url) !== '';
    }
    function blockedResponse(which) {
        showLimitCard(which);
        var label = which === 'images' ? IMAGE_LABEL : WORD_LABEL;
        return new Response(JSON.stringify({error:'You have used the daily ' + label + ' limit for ' + TOOL_NAME + '. The limit resets at midnight (12:00 AM IST).'}), {
            status: 403,
            headers: { 'Content-Type': 'application/json', 'X-TM-Limit': which || 'words' }
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
        if (method === 'POST' && noCreditsLeft(target) && isBillable(target)) return Promise.resolve(blockedResponse(meterFor(target)));
        var done = orig.apply(this, arguments);
        if (method === 'POST' && isBillable(target)) {
            return done.then(function(res) {
                if (res && res.headers && res.headers.get('X-TM-Limit')) {
                    showLimitCard(res.headers.get('X-TM-Limit'));
                    return res;
                }
                if (res && res.ok) setTimeout(updateBadge, 400);
                return res;
            });
        }
        return done;
    };
})();
</script>`, whWordsLabel, whImagesLabel, name)
}
