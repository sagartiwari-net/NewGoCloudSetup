package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (s *server) listTelegramInbox(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok || op.Role != "master" {
		if ok {
			writeErr(w, 403, "Master only")
		}
		return
	}
	rows, err := s.db.Query(`SELECT chat_id, username, display_name, last_text, updated_at FROM telegram_inbox ORDER BY updated_at DESC LIMIT 100`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var chat, username, name, text, at string
		if rows.Scan(&chat, &username, &name, &text, &at) != nil {
			continue
		}
		items = append(items, map[string]any{
			"chat_id": chat, "username": username, "display_name": name, "last_text": text, "updated_at": at,
		})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *server) pollTelegramInbox() {
	client := &http.Client{Timeout: 35 * time.Second}
	for {
		err := s.pullTelegramUpdates(client)
		if err == nil {
			continue
		}
		if err != errTelegramIdle {
			log.Printf("[TG] inbox: %s", err.Error())
		}
		time.Sleep(5 * time.Second)
	}
}

var errTelegramIdle = errors.New("no bot token")

func (s *server) pullTelegramUpdates(client *http.Client) error {
	var token string
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE "key"='bot_token'`).Scan(&token)
	token = strings.TrimSpace(token)
	if token == "" {
		return errTelegramIdle
	}
	fingerprint := tokenFingerprint(token)
	offset := 0
	var savedBot, offsetRaw string
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE "key"='telegram_offset_bot'`).Scan(&savedBot)
	if savedBot == fingerprint {
		_ = s.db.QueryRow(`SELECT value FROM settings WHERE "key"='telegram_update_offset'`).Scan(&offsetRaw)
		offset, _ = strconv.Atoi(strings.TrimSpace(offsetRaw))
	}
	query := url.Values{}
	query.Set("timeout", "25")
	query.Set("offset", strconv.Itoa(offset))
	query.Set("allowed_updates", `["message"]`)
	resp, err := client.Get("https://api.telegram.org/bot" + token + "/getUpdates?" + query.Encode())
	if err != nil {
		return errors.New("could not reach Telegram")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		s.deleteTelegramWebhook(client, token)
		return errors.New("cleared a previous webhook")
	}
	var parsed struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      []struct {
			UpdateID int `json:"update_id"`
			Message  struct {
				Text string `json:"text"`
				From struct {
					ID        int64  `json:"id"`
					IsBot     bool   `json:"is_bot"`
					Username  string `json:"username"`
					FirstName string `json:"first_name"`
					LastName  string `json:"last_name"`
				} `json:"from"`
				Chat struct {
					ID   int64  `json:"id"`
					Type string `json:"type"`
				} `json:"chat"`
			} `json:"message"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return errors.New("could not read Telegram")
	}
	if resp.StatusCode == http.StatusUnauthorized || strings.Contains(strings.ToLower(parsed.Description), "unauthorized") {
		return errors.New("bot token was rejected")
	}
	if !parsed.OK {
		if parsed.Description == "" {
			return errors.New("Telegram rejected the inbox check")
		}
		return errors.New(parsed.Description)
	}
	for _, update := range parsed.Result {
		next := update.UpdateID + 1
		message := update.Message
		if message.Chat.ID != 0 && !message.From.IsBot {
			if err := s.saveTelegramInbox(token, message.Chat.ID, message.Chat.Type, message.From.Username, message.From.FirstName, message.From.LastName, message.Text); err != nil {
				return err
			}
		}
		s.saveTelegramOffset(fingerprint, next)
	}
	return nil
}

func (s *server) saveTelegramInbox(token string, chatID int64, chatType, username, first, last, text string) error {
	id := strconv.FormatInt(chatID, 10)
	name := strings.TrimSpace(strings.TrimSpace(first) + " " + strings.TrimSpace(last))
	text = strings.TrimSpace(text)
	if text == "" {
		text = "(no text)"
	}
	if letters := []rune(text); len(letters) > 200 {
		text = string(letters[:200])
	}
	var existing int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM telegram_inbox WHERE chat_id=?`, id).Scan(&existing)
	now := time.Now().UTC().Format(time.RFC3339)
	var err error
	if existing == 0 {
		_, err = s.db.Exec(`INSERT INTO telegram_inbox (chat_id, username, display_name, last_text, updated_at) VALUES (?,?,?,?,?)`, id, username, name, text, now)
	} else {
		_, err = s.db.Exec(`UPDATE telegram_inbox SET username=?, display_name=?, last_text=?, updated_at=? WHERE chat_id=?`, username, name, text, now, id)
	}
	if err != nil {
		return err
	}
	if existing == 0 {
		log.Printf("[TG] inbox chat %s", id)
		if chatType == "private" {
			_ = postTelegram(token, id, "Received. Your Telegram id is "+id+".")
		}
	}
	return nil
}

func (s *server) saveTelegramOffset(fingerprint string, offset int) {
	_, _ = s.db.Exec(`INSERT INTO settings ("key", value) VALUES ('telegram_offset_bot', ?) ON DUPLICATE KEY UPDATE value=VALUES(value)`, fingerprint)
	_, _ = s.db.Exec(`INSERT INTO settings ("key", value) VALUES ('telegram_update_offset', ?) ON DUPLICATE KEY UPDATE value=VALUES(value)`, strconv.Itoa(offset))
}

func (s *server) deleteTelegramWebhook(client *http.Client, token string) {
	form := url.Values{}
	form.Set("drop_pending_updates", "false")
	resp, err := client.PostForm("https://api.telegram.org/bot"+token+"/deleteWebhook", form)
	if err == nil {
		resp.Body.Close()
	}
}

func tokenFingerprint(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:8])
}
