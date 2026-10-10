package main

import (
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
	w.WriteHeader(http.StatusForbidden)
	_, _ = io.WriteString(w, `{"error":"limit_reached","message":"Credit limit reached. This generate was not started.","code":403}`)
}
