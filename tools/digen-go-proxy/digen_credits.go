package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	digenMeterKey    = "credits"
	digenMeterLabel  = "Credits"
	digenCreditLimit = 1000
)

const digenBillKey contextKey = "digen_bill"

type digenQuote struct {
	credits int
	action  string
	path    string
}

type digenBill struct {
	username string
	quote    digenQuote
}

func digenUpstreamPath(path string) string {
	for _, prefix := range []string{"/api-proxy", "/test-api-proxy", "/agent-proxy", "/create-proxy", "/blog-proxy", "/resource-proxy"} {
		if strings.HasPrefix(path, prefix) {
			rest := strings.TrimPrefix(path, prefix)
			if rest == "" {
				return "/"
			}
			if !strings.HasPrefix(rest, "/") {
				rest = "/" + rest
			}
			return rest
		}
	}
	return path
}

func digenBillable(method, path string) bool {
	if method != http.MethodPost {
		return false
	}
	p := strings.ToLower(digenUpstreamPath(path))
	switch {
	case strings.HasSuffix(p, "/job/submit"), strings.HasSuffix(p, "/job/submitv1"):
		return true
	case strings.Contains(p, "/text_to_image"), strings.Contains(p, "/image_to_image"), strings.Contains(p, "/img2img"):
		return true
	case strings.Contains(p, "/sora") && (strings.Contains(p, "submit") || strings.Contains(p, "generat")):
		return true
	default:
		return false
	}
}

func digenQuoteFrom(path string, body []byte) digenQuote {
	up := digenUpstreamPath(path)
	model, seconds, resolution, meme, prompt := digenReadGenerate(body)
	credits := digenCreditsFor(up, model, body, meme)
	return digenQuote{
		credits: credits,
		action:  digenActionLabel(up, model, seconds, resolution, credits, prompt),
		path:    up,
	}
}

func digenCreditsFor(path, model string, body []byte, meme int) int {
	if meme > 0 {
		return meme
	}
	low := strings.ToLower(model + " " + path)
	switch {
	case strings.Contains(low, "veo"):
		return 200
	case strings.Contains(low, "sora"):
		return 400
	case strings.Contains(low, "text_to_image") && !digenBodyHasImage(body):
		return 0
	default:
		return 20
	}
}

func digenActionLabel(path, model string, seconds int, resolution string, credits int, prompt string) string {
	name := strings.TrimSpace(model)
	if name == "" {
		switch {
		case strings.Contains(path, "text_to_image"):
			name = "Text to image"
		case strings.Contains(path, "image_to_image"), strings.Contains(path, "img2img"):
			name = "Image to image"
		case strings.Contains(path, "submitv1"):
			name = "Space video"
		default:
			name = "Video"
		}
	}
	parts := []string{name}
	if seconds > 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}
	if resolution != "" {
		parts = append(parts, resolution)
	}
	label := strings.Join(parts, " · ")
	if credits > 0 {
		label += fmt.Sprintf(" · %d credits", credits)
	}
	if prompt != "" {
		label += " — " + prompt
	}
	if utf8.RuneCountInString(label) > 180 {
		runes := []rune(label)
		label = string(runes[:177]) + "..."
	}
	return label
}

func digenReadGenerate(body []byte) (model string, seconds int, resolution string, meme int, prompt string) {
	if len(body) == 0 {
		return
	}
	var v any
	if json.Unmarshal(body, &v) != nil {
		return
	}
	walkJSON(v, func(key string, val any) {
		k := strings.ToLower(strings.TrimSpace(key))
		switch {
		case model == "" && digenModelKey(k):
			if s, ok := digenShortString(val, 60); ok && !strings.Contains(strings.ToLower(s), "http") {
				model = s
			}
		case seconds == 0 && (k == "duration" || k == "seconds" || k == "video_duration" || k == "videoduration" || k == "length"):
			if n, ok := digenPositiveInt(val, 60); ok {
				seconds = n
			}
		case resolution == "" && (k == "resolution" || k == "quality" || k == "size"):
			resolution = digenResolution(val)
		case digenMemeKey(k):
			if n, ok := digenPositiveInt(val, 100000); ok && n > meme {
				meme = n
			}
		case prompt == "" && digenPromptKey(k):
			if s, ok := digenShortString(val, 80); ok && !strings.Contains(strings.ToLower(s), "http") {
				prompt = s
			}
		case model == "":
			if s, ok := digenShortString(val, 60); ok {
				low := strings.ToLower(s)
				if strings.Contains(low, "veo") || strings.Contains(low, "real motion") || strings.Contains(low, "sora") || strings.Contains(low, "flux") || strings.Contains(low, "nano") || strings.Contains(low, "minimax") || strings.Contains(low, "kling") {
					model = s
				}
			}
		}
	})
	return
}

func digenModelKey(k string) bool {
	switch k {
	case "model", "model_name", "modelname", "engine", "engine_name", "enginename", "play_mode", "playmode", "template", "template_name", "templatename":
		return true
	default:
		return false
	}
}

func digenMemeKey(k string) bool {
	switch k {
	case "credit", "credits", "meme", "memes", "cost", "price", "points", "point", "consume", "consume_credit", "credit_cost", "creditcost", "creditsperuse", "credits_per_use":
		return true
	default:
		return false
	}
}

func digenPromptKey(k string) bool {
	switch k {
	case "prompt", "text", "query", "description", "positive", "caption", "user_prompt", "userprompt":
		return true
	default:
		return false
	}
}

func digenShortString(val any, max int) (string, bool) {
	s, ok := val.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if s == "" || utf8.RuneCountInString(s) < 2 {
		return "", false
	}
	runes := []rune(s)
	if len(runes) > max {
		s = string(runes[:max-3]) + "..."
	}
	return s, true
}

func digenPositiveInt(val any, max int) (int, bool) {
	n := 0
	switch t := val.(type) {
	case float64:
		n = int(t + 0.5)
	case string:
		fmt.Sscanf(strings.TrimSpace(t), "%d", &n)
	default:
		return 0, false
	}
	if n <= 0 || n > max {
		return 0, false
	}
	return n, true
}

func digenResolution(val any) string {
	if s, ok := digenShortString(val, 16); ok {
		low := strings.ToLower(s)
		if strings.Contains(low, "p") || strings.Contains(low, "k") || strings.Contains(low, "720") || strings.Contains(low, "1080") {
			return s
		}
		return ""
	}
	if n, ok := digenPositiveInt(val, 4320); ok && (n == 480 || n == 720 || n == 1080 || n == 1440 || n == 2160) {
		return fmt.Sprintf("%dp", n)
	}
	return ""
}

func digenBodyHasImage(body []byte) bool {
	low := strings.ToLower(string(body))
	return strings.Contains(low, "image_url") || strings.Contains(low, "imageurl") || strings.Contains(low, "\"image\"") || strings.Contains(low, "init_image")
}

func walkJSON(v any, fn func(key string, val any)) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			fn(k, val)
			walkJSON(val, fn)
		}
	case []any:
		for _, val := range t {
			walkJSON(val, fn)
		}
	}
}

func digenPeriodStart(resetDays int, now time.Time) time.Time {
	if resetDays <= 0 {
		resetDays = 1
	}
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		loc = time.FixedZone("IST", 5*3600+30*60)
	}
	now = now.In(loc)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	if resetDays == 1 {
		return day
	}
	epoch := time.Date(2020, 1, 1, 0, 0, 0, 0, loc)
	n := int(day.Sub(epoch).Hours() / 24)
	startN := n - (n % resetDays)
	return epoch.AddDate(0, 0, startN)
}

type digenCreditState struct {
	websiteID int
	userID    int
	limit     int
	used      int
	resetDays int
	blocked   bool
}

func digenCreditStateFor(cfg Config, username string) (digenCreditState, error) {
	state := digenCreditState{resetDays: 1, limit: digenCreditLimit}
	db, err := openPanelDB(cfg)
	if err != nil {
		return state, err
	}
	if err := db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&state.websiteID); err != nil {
		return state, err
	}
	var status, expire, periodStart string
	var meterID, meterLimit, meterUsed, resetDays, custom int
	err = db.QueryRow(`SELECT u.id, u.status, COALESCE(u.custom_limit_expire_at, ''),
		COALESCE(m.id, 0), COALESCE(m."limit", 0), COALESCE(m.used, 0), COALESCE(m.reset_days, 1), COALESCE(m.is_custom, 0), COALESCE(m.period_start, '')
		FROM panel_users u
		LEFT JOIN user_meters m ON m.user_id=u.id AND m.key=?
		WHERE u.website_id=? AND u.username=?`, digenMeterKey, state.websiteID, username).
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
	state.used = meterUsed
	state.limit = digenCreditLimit
	if custom == 1 && !digenExpirePassed(expire) && meterLimit > 0 {
		state.limit = meterLimit
	}
	if meterID > 0 && custom == 0 && meterLimit != digenCreditLimit {
		_, _ = db.Exec(`UPDATE user_meters SET "limit"=? WHERE id=? AND is_custom=0`, digenCreditLimit, meterID)
	}
	start := digenPeriodStart(state.resetDays, time.Now()).UTC().Format(time.RFC3339)
	if meterID > 0 && (periodStart == "" || periodStart < start) {
		var used int
		_ = db.QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM usage_events WHERE website_id=? AND username=? AND limit_key=? AND timestamp>=?`,
			state.websiteID, username, digenMeterKey, start).Scan(&used)
		_, _ = db.Exec(`UPDATE user_meters SET used=?, period_start=? WHERE id=?`, used, start, meterID)
		state.used = used
	}
	return state, nil
}

func digenExpirePassed(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		loc = time.FixedZone("IST", 5*3600+30*60)
	}
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

func digenCreditsAllow(username string, amount int) bool {
	if amount < 1 {
		return true
	}
	state, err := digenCreditStateFor(loadConfig(), username)
	if err != nil {
		log.Printf("[QUOTA] check failed user=%s: %v", username, err)
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

func digenCharge(cfg Config, bill digenBill) {
	if bill.username == "" {
		return
	}
	state, err := digenCreditStateFor(cfg, bill.username)
	if err != nil || state.websiteID == 0 {
		log.Printf("[QUOTA] charge skipped user=%s: %v", bill.username, err)
		return
	}
	db, err := openPanelDB(cfg)
	if err != nil {
		return
	}
	amount := bill.quote.credits
	if amount < 0 {
		amount = 0
	}
	now := time.Now().UTC().Format(time.RFC3339)
	start := digenPeriodStart(state.resetDays, time.Now()).UTC().Format(time.RFC3339)
	_, err = db.Exec(`INSERT INTO usage_events (website_id, username, limit_key, limit_label, reset_days, action, target_path, amount, timestamp)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		state.websiteID, bill.username, digenMeterKey, digenMeterLabel, state.resetDays, bill.quote.action, bill.quote.path, amount, now)
	if err != nil {
		log.Printf("[QUOTA] usage insert failed user=%s: %v", bill.username, err)
		return
	}
	if amount == 0 {
		log.Printf("[QUOTA] user=%s free %q", bill.username, bill.quote.action)
		return
	}
	if state.userID == 0 {
		res, err := db.Exec(`INSERT INTO panel_users (website_id, username, status, custom_limit_expire_at) VALUES (?,?,?,NULL)`,
			state.websiteID, bill.username, "active")
		if err != nil {
			log.Printf("[QUOTA] user create failed user=%s: %v", bill.username, err)
			return
		}
		id, _ := res.LastInsertId()
		state.userID = int(id)
		_, _ = db.Exec(`INSERT INTO user_meters (user_id, key, label, reset_days, used, "limit", is_custom, period_start) VALUES (?,?,?,?,?,?,0,?)`,
			state.userID, digenMeterKey, digenMeterLabel, state.resetDays, amount, state.limit, start)
	} else {
		res, err := db.Exec(`UPDATE user_meters SET used = used + ?, period_start=CASE WHEN period_start='' OR period_start<? THEN ? ELSE period_start END WHERE user_id=? AND key=?`,
			amount, start, start, state.userID, digenMeterKey)
		changed := int64(0)
		if err == nil && res != nil {
			changed, _ = res.RowsAffected()
		}
		if err != nil || changed == 0 {
			_, _ = db.Exec(`INSERT INTO user_meters (user_id, key, label, reset_days, used, "limit", is_custom, period_start) VALUES (?,?,?,?,?,?,0,?)`,
				state.userID, digenMeterKey, digenMeterLabel, state.resetDays, state.used+amount, state.limit, start)
		}
	}
	log.Printf("[QUOTA] user=%s -%d %q used=%d/%d", bill.username, amount, bill.quote.action, state.used+amount, state.limit)
}

func digenRejectLimit(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-TM-Limit", "credit")
	w.WriteHeader(http.StatusForbidden)
	_, _ = io.WriteString(w, `{"error":"limit_reached","message":"Limit khatam hai. Is generate ke liye itne credits nahi bache.","code":403}`)
}

func digenUserLimits(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	user, err := panelSessionUsername(r)
	if err != nil || user == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"unauthorized"}`)
		return
	}
	state, stateErr := digenCreditStateFor(loadConfig(), user)
	if stateErr != nil || state.limit == 0 {
		state.limit = digenCreditLimit
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"show_limit":   true,
		"username":     user,
		"credit_limit": state.limit,
		"credit_used":  state.used,
		"credit_label": digenMeterLabel,
	})
}

func injectDigenHeaderHide(body []byte) []byte {
	if bytes.Contains(body, []byte("data-tm-digen-hide")) {
		return body
	}
	// Only the meme pill and the header avatar. Other header buttons stay.
	// A style tag can be ignored by the page policy, so the script removes those two nodes.
	style := []byte(`<style data-tm-digen-hide>.credits-pill-wrap,.header-actions__item:has(.credits-pill-wrap),.header-actions__item:has(.avatar-wrapper){display:none!important}</style><script data-tm-digen-hide>
(function(){
  function hide(){
    var pills = document.querySelectorAll(".credits-pill-wrap");
    for (var i = 0; i < pills.length; i++) {
      var pillItem = pills[i].closest(".header-actions__item") || pills[i];
      if (pillItem.parentNode) pillItem.parentNode.removeChild(pillItem);
    }
    var avatars = document.querySelectorAll(".avatar-wrapper");
    for (var j = 0; j < avatars.length; j++) {
      var avatarItem = avatars[j].closest(".header-actions__item");
      if (avatarItem && avatarItem.parentNode) avatarItem.parentNode.removeChild(avatarItem);
    }
  }
  hide();
  if (document.documentElement) {
    new MutationObserver(hide).observe(document.documentElement, { childList: true, subtree: true });
  }
})();
</script>`)
	lower := bytes.ToLower(body)
	if i := bytes.Index(lower, []byte("</head>")); i >= 0 {
		out := make([]byte, 0, len(body)+len(style))
		out = append(out, body[:i]...)
		out = append(out, style...)
		out = append(out, body[i:]...)
		return out
	}
	return append(body, style...)
}

func injectDigenCreditHTML(body []byte) []byte {
	if bytes.Contains(body, []byte("data-tm-credits")) {
		return body
	}
	script := []byte(digenLimitWidgetHTML())
	lower := bytes.ToLower(body)
	if i := bytes.Index(lower, []byte("</head>")); i >= 0 {
		out := make([]byte, 0, len(body)+len(script))
		out = append(out, body[:i]...)
		out = append(out, script...)
		out = append(out, body[i:]...)
		return out
	}
	return append(body, script...)
}

func digenLimitWidgetHTML() string {
	return `<script data-tm-credits="1">
(function () {
  if (window.__tmCredits) return;
  window.__tmCredits = true;
  var latest = null;
  var open = false;
  function removeDock() {
    var el = document.getElementById('tm-limit-dock');
    if (el) el.remove();
  }
  function host() {
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
      + '.num{font-size:34px;font-weight:800;letter-spacing:-.04em;color:#14532d}'
      + '.sub{margin-top:2px;color:#4d7c5e;font-size:13px}'
      + '.meter{background:#fff;border-radius:16px;padding:10px 12px}'
      + '.meter-top{display:flex;justify-content:space-between;align-items:center;margin-bottom:8px;color:#166534;font-size:13px}'
      + '.count{font-weight:750}'
      + '.bar{height:6px;border-radius:999px;background:#e7f6ec;overflow:hidden}'
      + '.barfill{height:100%;width:0;border-radius:999px;background:#22c55e;transition:width .35s ease}'
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
    shadow.getElementById('tm-tab').addEventListener('click', function (ev) { ev.stopPropagation(); setOpen(true); });
    shadow.getElementById('tm-close').addEventListener('click', function (ev) { ev.stopPropagation(); setOpen(false); });
    shadow.getElementById('tm-back').addEventListener('click', function () { setOpen(false); });
    shadow.getElementById('tm-drawer').addEventListener('click', function (ev) { ev.stopPropagation(); });
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
    if (!d) return;
    var el = host();
    if (!el) return;
    var root = el.shadowRoot;
    var tab = root.getElementById('tm-tab');
    var ring = root.getElementById('tm-ring');
    var bar = root.getElementById('tm-bar');
    var used = Number(d.credit_used) || 0;
    var limit = Number(d.credit_limit);
    if (!(limit >= 0)) limit = 1000;
    var left = Math.max(0, limit - used);
    var ratio = limit === 0 ? 0 : left / limit;
    root.getElementById('tm-left').textContent = String(left);
    root.getElementById('tm-sub').textContent = 'left';
    root.getElementById('tm-count').textContent = String(used) + ' / ' + String(limit);
    ring.style.strokeDasharray = '289';
    ring.style.strokeDashoffset = String(289 * (1 - ratio));
    bar.style.width = Math.round(ratio * 100) + '%';
    var low = left < 20;
    var warn = left >= 20 && left < 200;
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
      + '<div style="width:78px;height:78px;margin:0 auto 22px;border-radius:50%;background:#fff;display:grid;place-items:center;font-size:28px;border:1px solid #e6ebf2;">&#128274;<' + '/div>'
      + '<h1 style="font-size:28px;line-height:1.2;font-weight:800;letter-spacing:-.03em;margin:0 0 12px;">Limit khatam hai<' + '/h1>'
      + '<p style="color:#64748b;font-size:15px;line-height:1.55;margin:0;">Is generate ke liye itne credits nahi bache. Request nahi gayi. Limit midnight (12:00 AM IST) par reset hoti hai.<' + '/p>'
      + '<p style="margin:22px 0 0;color:#94a3b8;font-size:13px;">This limit applies to the current access<' + '/p><' + '/div>';
    (document.body || document.documentElement).appendChild(el);
  }
  function leftCredits() {
    if (!latest) return null;
    var limit = Number(latest.credit_limit);
    if (!(limit >= 0)) return null;
    return Math.max(0, limit - (Number(latest.credit_used) || 0));
  }
  function canAfford(cost) {
    var left = leftCredits();
    if (left === null) return true;
    return cost <= left;
  }
  function upstreamPath(url) {
    var p = String(url || '').split('?')[0];
    try { p = new URL(p, location.href).pathname; } catch (e) {}
    var marks = ['/api-proxy', '/test-api-proxy', '/agent-proxy', '/create-proxy'];
    for (var i = 0; i < marks.length; i++) {
      var at = p.indexOf(marks[i]);
      if (at >= 0) {
        p = p.slice(at + marks[i].length);
        break;
      }
    }
    if (p.charAt(0) !== '/') p = '/' + p;
    return p.toLowerCase();
  }
  function billable(url) {
    var p = upstreamPath(url);
    if (p.indexOf('/job/submit') !== -1) return true;
    if (p.indexOf('/text_to_image') !== -1 || p.indexOf('/image_to_image') !== -1 || p.indexOf('/img2img') !== -1) return true;
    if (p.indexOf('/sora') !== -1 && (p.indexOf('submit') !== -1 || p.indexOf('generat') !== -1)) return true;
    return false;
  }
  function costFrom(url, text) {
    if (!billable(url)) return 0;
    var meme = 0;
    var low = String(text || '').toLowerCase();
    var re = /"(credit|credits|meme|memes|cost|price|points|point|consume|consume_credit|credit_cost|creditsperuse|credits_per_use)"\s*:\s*(-?\d+(?:\.\d+)?)/gi;
    var m;
    while ((m = re.exec(low))) {
      var n = Math.round(Number(m[2]));
      if (n > meme && n <= 100000) meme = n;
    }
    if (meme > 0) return meme;
    if (low.indexOf('veo') !== -1) return 200;
    if (low.indexOf('sora') !== -1) return 400;
    if (upstreamPath(url).indexOf('text_to_image') !== -1 && low.indexOf('image') === -1) return 0;
    return 20;
  }
  function blockedResponse() {
    showLimitCard();
    return new Response('{"error":"limit_reached","message":"Limit khatam hai"}', {
      status: 403,
      headers: { 'Content-Type': 'application/json', 'X-TM-Limit': 'credit' }
    });
  }
  function updateBadge() {
    window.fetch('/api/user-limits', { credentials: 'same-origin', cache: 'no-store' })
      .then(function (r) { return r.json(); })
      .then(function (d) { latest = d; paint(d); })
      .catch(function () {});
  }
  window.__tmUpdateCredits = updateBadge;
  if (document.body) updateBadge();
  else document.addEventListener('DOMContentLoaded', updateBadge);
  setInterval(updateBadge, 4000);
  var origFetch = window.fetch;
  if (origFetch) {
    window.fetch = function (input, init) {
      var url = typeof input === 'string' ? input : (input && input.url) || '';
      var method = (init && init.method) || (input && input.method) || 'GET';
      if (String(url).indexOf('/api/user-limits') !== -1) return origFetch.apply(this, arguments);
      if (String(method).toUpperCase() !== 'POST' || !billable(url)) {
        var plain = origFetch.apply(this, arguments);
        return plain;
      }
      var self = this;
      var args = arguments;
      var bodyText = '';
      if (init && typeof init.body === 'string') bodyText = init.body;
      var ready = Promise.resolve(bodyText);
      if (!bodyText && input && typeof input !== 'string' && input.clone) {
        ready = input.clone().text().catch(function () { return ''; });
      }
      return ready.then(function (text) {
        var cost = costFrom(url, text);
        if (cost > 0 && !canAfford(cost)) return blockedResponse();
        return origFetch.apply(self, args).then(function (res) {
          if (res && res.headers && res.headers.get('X-TM-Limit')) showLimitCard();
          if (cost > 0) setTimeout(updateBadge, 700);
          return res;
        });
      });
    };
  }
  var origOpen = XMLHttpRequest.prototype.open;
  var origSend = XMLHttpRequest.prototype.send;
  XMLHttpRequest.prototype.open = function (method, url) {
    this.__tmCreditURL = url;
    this.__tmCreditMethod = method;
    return origOpen.apply(this, arguments);
  };
  XMLHttpRequest.prototype.send = function (body) {
    var url = this.__tmCreditURL || '';
    if (String(this.__tmCreditMethod || '').toUpperCase() === 'POST' && billable(url)) {
      var text = typeof body === 'string' ? body : '';
      var cost = costFrom(url, text);
      if (cost > 0 && !canAfford(cost)) {
        showLimitCard();
        return;
      }
    }
    return origSend.apply(this, arguments);
  };
})();
</script>`
}
