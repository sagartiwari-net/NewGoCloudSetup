package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func (s *server) openAccess(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	var body struct {
		WebsiteID int    `json:"website_id"`
		Username  string `json:"username"`
		ProductID string `json:"product_id"`
		ClientIP  string `json:"client_ip"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	body.Username = strings.TrimSpace(body.Username)
	body.ProductID = strings.TrimSpace(body.ProductID)
	if body.WebsiteID == 0 || body.Username == "" || body.ProductID == "" {
		writeJSON(w, 200, map[string]any{"allowed": false, "error": "Choose a tool and enter a username and product id."})
		return
	}
	if !s.owns(op, body.WebsiteID) {
		writeJSON(w, 200, map[string]any{"allowed": false, "error": "This tool is not in your account."})
		return
	}
	var name, domain string
	err := s.db.QueryRow(`SELECT name, domain FROM websites WHERE id=?`, body.WebsiteID).Scan(&name, &domain)
	if err != nil {
		writeJSON(w, 200, map[string]any{"allowed": false, "error": "Tool not found."})
		return
	}
	if !s.productMapped(body.WebsiteID, body.ProductID) {
		writeJSON(w, 200, map[string]any{"allowed": false, "error": "Product id " + body.ProductID + " is not mapped to " + name + "."})
		return
	}
	if msg := s.userBlock(body.WebsiteID, body.Username); msg != "" {
		writeJSON(w, 200, map[string]any{"allowed": false, "error": msg})
		return
	}
	ip := publicIP(body.ClientIP)
	if ip != "" {
		var blocked int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM blocked_ips WHERE client_ip=? AND (website_id=? OR website_id=0)`, ip, body.WebsiteID).Scan(&blocked)
		if blocked > 0 {
			writeJSON(w, 200, map[string]any{"allowed": false, "error": "This IP is blocked for " + name + "."})
			return
		}
	}
	buf := make([]byte, 24)
	_, _ = rand.Read(buf)
	token := hex.EncodeToString(buf)
	now := time.Now().UTC()
	expires := now.Add(2 * time.Minute).Format(time.RFC3339)
	for attempt := 0; attempt < 5; attempt++ {
		_, err = s.db.Exec(`INSERT INTO access_tokens (token, website_id, username, product_id, client_ip, expires_at, created_at) VALUES (?,?,?,?,?,?,?)`,
			token, body.WebsiteID, body.Username, body.ProductID, ip, expires, now.Format(time.RFC3339))
		if err == nil || (!strings.Contains(err.Error(), "locked") && !strings.Contains(err.Error(), "busy")) {
			break
		}
		time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if ip != "" {
		kind, org, location := classifyIP(ip)
		_, _ = s.db.Exec(`INSERT INTO host_reports (website_id, username, client_ip, ip_type, org, location, created_at) VALUES (?,?,?,?,?,?,?)`,
			body.WebsiteID, body.Username, ip, kind, org, location, now.Format(time.RFC3339))
	}
	// TOOL_PUBLIC_SCHEME=https after SSL; default http so pre-SSL access links work.
	// Browser/nginx still decide asset scheme via X-Forwarded-Proto on the tool itself.
	scheme := strings.TrimSpace(os.Getenv("TOOL_PUBLIC_SCHEME"))
	if scheme != "https" {
		scheme = "http"
	}
	openURL := scheme + "://" + domain + "/access?token=" + url.QueryEscape(token)
	writeJSON(w, 200, map[string]any{
		"allowed":    true,
		"open_url":   openURL,
		"username":   body.Username,
		"product_id": body.ProductID,
		"tool":       name,
	})
}

func (s *server) productMapped(websiteID int, productID string) bool {
	rows, err := s.db.Query(`SELECT product_ids_json FROM products WHERE website_id=?`, websiteID)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if rows.Scan(&raw) != nil {
			continue
		}
		var ids []string
		if json.Unmarshal([]byte(raw), &ids) != nil {
			continue
		}
		for _, id := range ids {
			if strings.TrimSpace(id) == productID {
				return true
			}
		}
	}
	return false
}

func (s *server) userBlock(websiteID int, username string) string {
	var status string
	err := s.db.QueryRow(`SELECT status FROM panel_users WHERE website_id=? AND username=?`, websiteID, username).Scan(&status)
	if err != nil {
		return ""
	}
	if status == "suspended" {
		return username + " is suspended on this tool."
	}
	return ""
}

func publicIP(raw string) string {
	parsed := net.ParseIP(strings.TrimSpace(raw))
	if parsed == nil || parsed.IsLoopback() || parsed.IsUnspecified() || parsed.IsPrivate() {
		return ""
	}
	return parsed.String()
}

func requestIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func classifyIP(ip string) (kind, org, location string) {
	kind, org, location, _ = classifyIPDetailed(ip)
	return kind, org, location
}

func classifyIPDetailed(ip string) (kind, org, location string, extra map[string]any) {
	kind = "residential"
	extra = map[string]any{}
	client := http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get("http://ip-api.com/json/" + url.PathEscape(ip) + "?fields=status,country,regionName,city,isp,org,as,asname,proxy,hosting,mobile")
	if err != nil {
		return kind, "", "", extra
	}
	defer resp.Body.Close()
	var info struct {
		Status     string `json:"status"`
		Country    string `json:"country"`
		RegionName string `json:"regionName"`
		City       string `json:"city"`
		ISP        string `json:"isp"`
		Org        string `json:"org"`
		AS         string `json:"as"`
		ASName     string `json:"asname"`
		Proxy      bool   `json:"proxy"`
		Hosting    bool   `json:"hosting"`
		Mobile     bool   `json:"mobile"`
	}
	if json.NewDecoder(resp.Body).Decode(&info) != nil || info.Status != "success" {
		return kind, "", "", extra
	}
	if info.Proxy && info.Hosting {
		kind = "vpn"
	} else if info.Proxy {
		kind = "proxy"
	} else if info.Hosting {
		kind = "hosting"
	}
	org = info.ISP
	if org == "" {
		org = info.Org
	}
	if info.City != "" && info.Country != "" {
		location = info.City + ", " + info.Country
	} else if info.RegionName != "" && info.Country != "" {
		location = info.RegionName + ", " + info.Country
	} else {
		location = info.Country
	}
	extra = map[string]any{
		"country":     info.Country,
		"region":      info.RegionName,
		"city":        info.City,
		"isp":         info.ISP,
		"org":         info.Org,
		"as":          info.AS,
		"asname":      info.ASName,
		"proxy":       info.Proxy,
		"hosting":     info.Hosting,
		"mobile":      info.Mobile,
		"type_label":  ipTypeLabel(kind),
	}
	return kind, org, location, extra
}

func (s *server) accessAlerts(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok || op.Role != "master" {
		if ok {
			writeErr(w, 403, "Master only")
		}
		return
	}
	writeJSON(w, 200, map[string]int{
		"same_tool_count":    settingInt(s.db, "access_same_count", 4),
		"same_tool_minutes":  settingInt(s.db, "access_same_minutes", 30),
		"multi_tool_count":   settingInt(s.db, "access_multi_count", 3),
		"multi_tool_minutes": settingInt(s.db, "access_multi_minutes", 3),
		"repeat_minutes":     settingInt(s.db, "access_repeat_minutes", 30),
		"repeat_hours":       settingInt(s.db, "access_repeat_hours", 3),
		"repeat_min_opens":   settingInt(s.db, "access_repeat_opens", 4),
	})
}

func (s *server) saveAccessAlerts(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok || op.Role != "master" {
		if ok {
			writeErr(w, 403, "Master only")
		}
		return
	}
	var body struct {
		SameCount    int `json:"same_tool_count"`
		SameMinutes  int `json:"same_tool_minutes"`
		MultiCount   int `json:"multi_tool_count"`
		MultiMinutes int `json:"multi_tool_minutes"`
		RepeatEvery  int `json:"repeat_minutes"`
		RepeatHours  int `json:"repeat_hours"`
		RepeatOpens  int `json:"repeat_min_opens"`
	}
	if err := readBody(r, &body); err != nil || body.SameCount < 2 || body.SameMinutes < 1 || body.MultiCount < 2 || body.MultiMinutes < 1 || body.RepeatEvery < 1 || body.RepeatHours < 1 || body.RepeatOpens < 2 {
		writeErr(w, 400, "Use the minimum values shown on the form")
		return
	}
	saveSettingValue(s.db, "access_same_count", body.SameCount)
	saveSettingValue(s.db, "access_same_minutes", body.SameMinutes)
	saveSettingValue(s.db, "access_multi_count", body.MultiCount)
	saveSettingValue(s.db, "access_multi_minutes", body.MultiMinutes)
	saveSettingValue(s.db, "access_repeat_minutes", body.RepeatEvery)
	saveSettingValue(s.db, "access_repeat_hours", body.RepeatHours)
	saveSettingValue(s.db, "access_repeat_opens", body.RepeatOpens)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
