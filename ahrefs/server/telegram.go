package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

type alertKey struct {
	kind       string
	resellerID int
}

var (
	alertMu   sync.Mutex
	alertLast = map[alertKey]time.Time{}
)

func (s *server) telegramEnabled() bool {
	return s.cfg.TelegramBotToken != "" && s.cfg.TelegramChatID != ""
}

func (s *server) notifyTelegram(text string) {
	if !s.telegramEnabled() {
		return
	}
	go func() {
		payload, _ := json.Marshal(map[string]any{
			"chat_id":    s.cfg.TelegramChatID,
			"text":       text,
			"parse_mode": "HTML",
		})
		url := "https://api.telegram.org/bot" + s.cfg.TelegramBotToken + "/sendMessage"
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 8 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[telegram] send error: %v", err)
			return
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 300 {
			log.Printf("[telegram] status %d", resp.StatusCode)
		}
	}()
}

func (s *server) alertOnce(kind string, resellerID int, cooldown time.Duration, text string) {
	if !s.telegramEnabled() {
		return
	}
	key := alertKey{kind: kind, resellerID: resellerID}
	now := time.Now()
	alertMu.Lock()
	if t, ok := alertLast[key]; ok && now.Sub(t) < cooldown {
		alertMu.Unlock()
		return
	}
	alertLast[key] = now
	alertMu.Unlock()
	s.notifyTelegram(text)
}

func (s *server) alertPoolEmpty(resellerID int) {
	s.alertOnce("pool_empty", resellerID, 15*time.Minute,
		"<b>Ahrefs Bar</b>\nPool empty / no healthy accounts\nreseller_id="+itoa(resellerID))
}

func (s *server) alertUsageSpike(resellerID int, credits int, window time.Duration) {
	s.alertOnce("spike", resellerID, 10*time.Minute,
		"<b>Ahrefs Bar</b>\nUsage spike: "+itoa(credits)+" credits in "+window.String()+"\nreseller_id="+itoa(resellerID))
}

func (s *server) checkUsageSpike(resellerID int) {
	if !s.telegramEnabled() || s.cfg.SpikeCredits <= 0 {
		return
	}
	window := time.Duration(s.cfg.SpikeWindowMin) * time.Minute
	if window <= 0 {
		window = 5 * time.Minute
	}
	since := time.Now().UTC().Add(-window).Format(time.RFC3339)
	var sum int
	_ = s.db.QueryRow(`
SELECT COALESCE(SUM(credits),0) FROM ahrefs_bar_usage_log
WHERE reseller_id=? AND created_at>=? AND credits>0`, resellerID, since).Scan(&sum)
	if sum >= s.cfg.SpikeCredits {
		s.alertUsageSpike(resellerID, sum, window)
	}
}
