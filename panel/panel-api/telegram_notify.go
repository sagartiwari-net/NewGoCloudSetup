package main

import (
	"log"
	"strconv"
	"strings"
	"time"
)

func (s *server) pollTelegramNotices() {
	s.replayRecentTelegramNotices()
	for {
		s.deliverTelegramNotices()
		time.Sleep(5 * time.Second)
	}
}

func (s *server) replayRecentTelegramNotices() {
	var flag string
	if s.db.QueryRow(`SELECT value FROM settings WHERE "key"='telegram_notice_replay'`).Scan(&flag) == nil && flag == "2" {
		return
	}
	cutoff := time.Now().UTC().Add(-20 * time.Minute).Format(time.RFC3339)
	s.rewindNoticeCursor("telegram_cursor_switch", `SELECT COALESCE(MIN(id), 0) FROM switch_events WHERE switched_at >= ?`, cutoff)
	s.rewindNoticeCursor("telegram_cursor_logout", `SELECT COALESCE(MIN(id), 0) FROM logout_events WHERE created_at >= ?`, cutoff)
	s.saveTelegramOffsetValue("telegram_notice_replay", "2")
}

func (s *server) rewindNoticeCursor(key, query, cutoff string) {
	var minID int
	if s.db.QueryRow(query, cutoff).Scan(&minID) != nil || minID <= 0 {
		return
	}
	s.saveTelegramOffsetValue(key, strconv.Itoa(minID-1))
}

func (s *server) deliverTelegramNotices() {
	s.deliverSwitchNotices()
	s.deliverLogoutNotices()
	s.deliverNetworkNotices()
}

func (s *server) noticeCursor(key, table string) int {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE "key"=?`, key).Scan(&raw)
	if err != nil || strings.TrimSpace(raw) == "" {
		var maxID int
		_ = s.db.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM ` + table).Scan(&maxID)
		s.saveTelegramOffsetValue(key, strconv.Itoa(maxID))
		return maxID
	}
	n, _ := strconv.Atoi(strings.TrimSpace(raw))
	return n
}

func (s *server) saveTelegramOffsetValue(key, value string) {
	_, _ = s.db.Exec(`INSERT INTO settings ("key", value) VALUES (?, ?) ON DUPLICATE KEY UPDATE value=VALUES(value)`, key, value)
}

type telegramNotice struct {
	id       int
	website  int
	tool     string
	site     string
	username string
	from     string
	to       string
	reason   string
	ip       string
	kind     string
	org      string
	location string
	at       string
}

func (s *server) deliverSwitchNotices() {
	cursor := s.noticeCursor("telegram_cursor_switch", "switch_events")
	rows, err := s.db.Query(`SELECT e.id, e.website_id, COALESCE(t.name, ''), COALESCE(w.name, ''), e.username, e.from_account_name, e.to_account_name, e.reason, e.switched_at
		FROM switch_events e
		LEFT JOIN websites w ON w.id=e.website_id
		LEFT JOIN tools t ON t.id=w.tool_id
		WHERE e.id>? ORDER BY e.id LIMIT 40`, cursor)
	if err != nil {
		log.Printf("[TG] switch notices: %s", err.Error())
		return
	}
	defer rows.Close()
	for rows.Next() {
		var note telegramNotice
		if rows.Scan(&note.id, &note.website, &note.tool, &note.site, &note.username, &note.from, &note.to, &note.reason, &note.at) != nil {
			continue
		}
		s.sendTelegramNotice("switch", "switch_template", "{tool}: {username} switched from {from} to {to} on {website} at {time}. {reason}", note)
		s.saveTelegramOffsetValue("telegram_cursor_switch", strconv.Itoa(note.id))
	}
}

func (s *server) deliverLogoutNotices() {
	cursor := s.noticeCursor("telegram_cursor_logout", "logout_events")
	rows, err := s.db.Query(`SELECT e.id, e.website_id, COALESCE(t.name, ''), COALESCE(w.name, ''), e.username, e.account_name, e.next_account_name, e.reason, e.client_ip, e.created_at
		FROM logout_events e
		LEFT JOIN websites w ON w.id=e.website_id
		LEFT JOIN tools t ON t.id=w.tool_id
		WHERE e.id>? ORDER BY e.id LIMIT 40`, cursor)
	if err != nil {
		log.Printf("[TG] logout notices: %s", err.Error())
		return
	}
	defer rows.Close()
	for rows.Next() {
		var note telegramNotice
		if rows.Scan(&note.id, &note.website, &note.tool, &note.site, &note.username, &note.from, &note.to, &note.reason, &note.ip, &note.at) != nil {
			continue
		}
		s.sendTelegramNotice("logout", "logout_template", "{tool} logged out for {username} on {website} at {time}.", note)
		s.saveTelegramOffsetValue("telegram_cursor_logout", strconv.Itoa(note.id))
	}
}

func (s *server) deliverNetworkNotices() {
	cursor := s.noticeCursor("telegram_cursor_host", "host_reports")
	rows, err := s.db.Query(`SELECT h.id, h.website_id, COALESCE(t.name, ''), COALESCE(w.name, ''), h.username, h.client_ip, h.ip_type, h.org, h.location, h.created_at
		FROM host_reports h
		LEFT JOIN websites w ON w.id=h.website_id
		LEFT JOIN tools t ON t.id=w.tool_id
		WHERE h.id>? AND h.ip_type IN ('vpn', 'proxy', 'hosting')
		ORDER BY h.id LIMIT 40`, cursor)
	if err != nil {
		log.Printf("[TG] network notices: %s", err.Error())
		return
	}
	defer rows.Close()
	for rows.Next() {
		var note telegramNotice
		if rows.Scan(&note.id, &note.website, &note.tool, &note.site, &note.username, &note.ip, &note.kind, &note.org, &note.location, &note.at) != nil {
			continue
		}
		note.kind = networkTypeLabel(note.kind)
		s.sendTelegramNotice("network", "network_template", "{username} is using {type} on {tool} ({website}). IP {ip}. {org}. {location}. {time}", note)
		s.saveTelegramOffsetValue("telegram_cursor_host", strconv.Itoa(note.id))
	}
}

func (s *server) noticeRecipients(websiteID int, event string) ([]map[string]string, bool) {
	var routeID int
	var eventsRaw string
	if s.db.QueryRow(`SELECT id, events_json FROM telegram_routes WHERE website_id=?`, websiteID).Scan(&routeID, &eventsRaw) == nil {
		if !strings.Contains(eventsRaw, `"`+event+`"`) {
			return nil, false
		}
		return s.destinationRecipients(`SELECT d.label, d.chat_id, o.username FROM telegram_route_chats c JOIN telegram_destinations d ON d.id=c.destination_id JOIN operators o ON o.id=d.reseller_id WHERE c.route_id=? AND d.enabled=1`, routeID), true
	}
	owned := s.destinationRecipients(`SELECT d.label, d.chat_id, o.username FROM telegram_destinations d JOIN operators o ON o.id=d.reseller_id WHERE d.enabled=1 AND d.reseller_id IN (SELECT operator_id FROM operator_websites WHERE website_id=?)`, websiteID)
	if len(owned) > 0 {
		return owned, true
	}
	return s.destinationRecipients(`SELECT d.label, d.chat_id, o.username FROM telegram_destinations d JOIN operators o ON o.id=d.reseller_id WHERE d.enabled=1`), true
}

func (s *server) destinationRecipients(query string, args ...any) []map[string]string {
	recipients := []map[string]string{}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return recipients
	}
	defer rows.Close()
	for rows.Next() {
		var label, chat, reseller string
		if rows.Scan(&label, &chat, &reseller) != nil {
			continue
		}
		recipients = append(recipients, map[string]string{"label": label, "chat_id": chat, "reseller": reseller})
	}
	return recipients
}

func networkTypeLabel(kind string) string {
	switch kind {
	case "vpn":
		return "VPN"
	case "proxy":
		return "proxy"
	case "hosting":
		return "server"
	default:
		return kind
	}
}

func (s *server) sendTelegramNotice(event, templateKey, fallback string, note telegramNotice) {
	if note.website == 0 || note.site == "" {
		return
	}
	recipients, allowed := s.noticeRecipients(note.website, event)
	if !allowed {
		return
	}
	minutes := 60
	var raw string
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE "key"='repeat_minutes'`).Scan(&raw)
	if n, convErr := strconv.Atoi(raw); convErr == nil && n > 0 {
		minutes = n
	}
	status := "sent"
	var last string
	err := s.db.QueryRow(`SELECT created_at FROM telegram_deliveries WHERE website_id=? AND username=? AND event=? AND status='sent' ORDER BY created_at DESC LIMIT 1`, note.website, note.username, event).Scan(&last)
	if err == nil {
		if parsed, ok := parseTime(last); ok && time.Since(parsed) < time.Duration(minutes)*time.Minute {
			status = "skipped"
		}
	}
	var token, tmpl string
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE "key"='bot_token'`).Scan(&token)
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE "key"=?`, templateKey).Scan(&tmpl)
	if strings.TrimSpace(tmpl) == "" {
		tmpl = fallback
	}
	text := strings.NewReplacer(
		"{tool}", note.tool,
		"{username}", note.username,
		"{website}", note.site,
		"{time}", note.at,
		"{ip}", note.ip,
		"{reason}", note.reason,
		"{count}", "",
		"{from}", note.from,
		"{to}", note.to,
		"{type}", note.kind,
		"{org}", note.org,
		"{location}", note.location,
	).Replace(tmpl)
	errorText := ""
	if status == "sent" {
		switch {
		case strings.TrimSpace(token) == "":
			status = "failed"
			errorText = "Bot token is empty"
		case len(recipients) == 0:
			status = "failed"
			errorText = "No enabled chat on this route"
		default:
			var failures []string
			for _, rec := range recipients {
				if detail := postTelegram(token, rec["chat_id"], text); detail != "" {
					failures = append(failures, rec["label"]+": "+detail)
				}
			}
			if len(failures) > 0 {
				status = "failed"
				errorText = strings.Join(failures, "; ")
			}
		}
	}
	s.insertDelivery(note.website, note.username, event, status, recipients, errorText)
}
