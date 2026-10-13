package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCookieLinkFollowsUntilStopped(t *testing.T) {
	t.Setenv("GT4RENTS_MYSQL_PASSWORD", "")
	db := openRecloudTestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	var sourceID, websiteID int
	if err := db.QueryRow(`SELECT id, website_id FROM accounts WHERE name='kept'`).Scan(&sourceID, &websiteID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`INSERT INTO accounts (website_id, name, cookie, user_agent, proxy, status, show_limit, description, automation_task_uid, automation_ingest_key, cookie_updated_at, last_used_at)
		VALUES (?, 'two', 'old-two', 'ua-two', 'socks5://keep', 'inactive', 1, '', '', '', ?, '')`, websiteID, now)
	if err != nil {
		t.Fatal(err)
	}
	followerID, _ := res.LastInsertId()
	if _, err := db.Exec(`UPDATE accounts SET user_agent=? WHERE id=?`, "ua-source", sourceID); err != nil {
		t.Fatal(err)
	}

	s := &server{db: db}
	mux := http.NewServeMux()
	s.routes(mux)

	expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO operators (username, password_hash, role, status) VALUES ('reseller', 'x', 'reseller', 'active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO admin_sessions (token, operator_id, expires_at) VALUES ('reseller-test', 2, ?)`, expires); err != nil {
		t.Fatal(err)
	}
	denied := httptest.NewRequest(http.MethodGet, "/api/cookie-links", nil)
	denied.Header.Set("Authorization", "Bearer reseller-test")
	deniedRec := httptest.NewRecorder()
	mux.ServeHTTP(deniedRec, denied)
	if deniedRec.Code != 403 {
		t.Fatalf("reseller status %d", deniedRec.Code)
	}

	link := callJSON(t, mux, http.MethodPost, "/api/cookie-links",
		fmt.Sprintf(`{"follower_account_id":%d,"source_account_id":%d}`, followerID, sourceID))
	if link.Code != 200 {
		t.Fatalf("link status %d", link.Code)
	}
	cookie, ua, proxy, status := followerShare(t, db, int(followerID))
	if cookie != "kept-cookie" || ua != "ua-source" || proxy != "socks5://keep" || status != "inactive" {
		t.Fatalf("copy mismatch cookie=%q ua=%q proxy=%q status=%q", cookie, ua, proxy, status)
	}

	save := callJSON(t, mux, http.MethodPost, "/api/accounts", fmt.Sprintf(
		`{"id":%d,"website_id":%d,"name":"kept","cookie":"new-cookie","user_agent":"","status":"active","show_limit":true}`,
		sourceID, websiteID))
	if save.Code != 200 {
		t.Fatalf("save status %d", save.Code)
	}
	cookie, ua, proxy, status = followerShare(t, db, int(followerID))
	if cookie != "new-cookie" || ua != "ua-source" || proxy != "socks5://keep" || status != "inactive" {
		t.Fatalf("fan-out mismatch cookie=%q ua=%q proxy=%q status=%q", cookie, ua, proxy, status)
	}

	if _, err := db.Exec(`UPDATE accounts SET cookie='' WHERE id=?`, sourceID); err != nil {
		t.Fatal(err)
	}
	s.shareCookieFrom(sourceID)
	cookie, _, _, _ = followerShare(t, db, int(followerID))
	if cookie != "new-cookie" {
		t.Fatal("empty source cookie wiped the follower")
	}

	stop := callJSON(t, mux, http.MethodDelete, fmt.Sprintf("/api/cookie-links?follower=%d", followerID), "")
	if stop.Code != 200 {
		t.Fatalf("stop status %d", stop.Code)
	}
	if _, err := db.Exec(`UPDATE accounts SET cookie='later-cookie' WHERE id=?`, sourceID); err != nil {
		t.Fatal(err)
	}
	s.shareCookieFrom(sourceID)
	cookie, _, _, _ = followerShare(t, db, int(followerID))
	if cookie != "new-cookie" {
		t.Fatal("stopped link still copied")
	}

	again := callJSON(t, mux, http.MethodPost, "/api/cookie-links",
		fmt.Sprintf(`{"follower_account_id":%d,"source_account_id":%d}`, followerID, sourceID))
	if again.Code != 200 {
		t.Fatalf("relink status %d", again.Code)
	}
	reverse := callJSON(t, mux, http.MethodPost, "/api/cookie-links",
		fmt.Sprintf(`{"follower_account_id":%d,"source_account_id":%d}`, sourceID, followerID))
	if reverse.Code != 400 {
		t.Fatalf("cycle status %d", reverse.Code)
	}

	list := callJSON(t, mux, http.MethodGet, "/api/cookie-links", "")
	if list.Code != 200 {
		t.Fatalf("list status %d", list.Code)
	}
	var page struct {
		Items []struct {
			Follower int `json:"follower_account_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Follower != int(followerID) {
		t.Fatalf("list items %#v", page.Items)
	}
}

func followerShare(t *testing.T, db *sql.DB, id int) (string, string, string, string) {
	t.Helper()
	var cookie, ua, proxy, status string
	if err := db.QueryRow(`SELECT COALESCE(cookie, ''), COALESCE(user_agent, ''), COALESCE(proxy, ''), COALESCE(status, '') FROM accounts WHERE id=?`, id).
		Scan(&cookie, &ua, &proxy, &status); err != nil {
		t.Fatal(err)
	}
	return cookie, ua, proxy, status
}
