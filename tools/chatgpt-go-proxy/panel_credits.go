package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

func isBillableChat(r *http.Request, path string) bool {
	return r.Method == http.MethodPost && path == "/backend-api/f/conversation"
}

type panelCreditState struct {
	websiteID int
	userID    int
	limit     int
	used      int
	resetDays int
	blocked   bool
}

func panelCreditStateFor(username string) (panelCreditState, error) {
	state := panelCreditState{resetDays: 1, limit: 100}
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
	if n, ok := defaults["credits"]; ok {
		state.limit = n
	}
	var status, expire string
	var meterID, meterLimit, meterUsed, resetDays, custom int
	err = db.QueryRow(`SELECT u.id, u.status, COALESCE(u.custom_limit_expire_at, ''),
		COALESCE(m.id, 0), COALESCE(m."limit", 0), COALESCE(m.used, 0), COALESCE(m.reset_days, 1), COALESCE(m.is_custom, 0)
		FROM panel_users u
		LEFT JOIN user_meters m ON m.user_id=u.id AND m.key='credits'
		WHERE u.website_id=? AND u.username=?`, state.websiteID, username).Scan(&state.userID, &status, &expire, &meterID, &meterLimit, &meterUsed, &resetDays, &custom)
	if err == nil {
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
	}
	return state, nil
}

func creditLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return time.FixedZone("IST", 5*3600+30*60)
	}
	return loc
}

func creditPeriodStart(resetDays int) time.Time {
	if resetDays < 1 {
		resetDays = 1
	}
	now := time.Now().In(creditLocation())
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return day.AddDate(0, 0, -(resetDays - 1))
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

func panelCreditsAllow(username string) bool {
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
	return state.used < state.limit
}

func panelCreditsCharge(username, path string) {
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
	_, err = db.Exec(`INSERT INTO usage_events (website_id, username, limit_key, limit_label, reset_days, action, target_path, amount, timestamp)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		state.websiteID, username, "credits", "Credits", state.resetDays, "message", path, 1, now)
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
		_, _ = db.Exec(`INSERT INTO user_meters (user_id, key, label, reset_days, used, "limit", is_custom) VALUES (?,?,?,?,?,?,0)`,
			state.userID, "credits", "Credits", state.resetDays, state.used+1, state.limit)
	} else {
		res, err := db.Exec(`UPDATE user_meters SET used = used + 1 WHERE user_id=? AND key='credits'`, state.userID)
		changed := int64(0)
		if err == nil && res != nil {
			changed, _ = res.RowsAffected()
		}
		if err != nil || changed == 0 {
			_, _ = db.Exec(`INSERT INTO user_meters (user_id, key, label, reset_days, used, "limit", is_custom) VALUES (?,?,?,?,?,?,0)`,
				state.userID, "credits", "Credits", state.resetDays, state.used+1, state.limit)
		}
	}
	log.Printf("[CREDIT] 1 credit user=%s used=%d limit=%d", username, state.used+1, state.limit)
}

func rejectIfChatCreditSpent(w http.ResponseWriter, r *http.Request, cfg Config, path, username string) bool {
	if !usesPanelAccountMode(cfg) || !isBillableChat(r, path) || strings.TrimSpace(username) == "" || username == "guest_passthrough" {
		return false
	}
	if panelCreditsAllow(username) {
		return false
	}
	log.Printf("[CREDIT] blocked user=%s path=%s", username, path)
	if strings.HasPrefix(path, "/backend-api/") || strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		label := cfg.CreditLabel
		if label == "" {
			label = "Messages"
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-TM-Limit", "credit")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `{"error":{"message":"Daily limit reached","code":"limit_reached","label":%q}}`, label)
		return true
	}
	renderLimitReachedPage(w, "credit", cfg)
	return true
}
