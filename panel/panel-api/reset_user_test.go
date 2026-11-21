package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResetUserClearsMetersAndUsageEvents(t *testing.T) {
	t.Setenv("GT4RENTS_MYSQL_PASSWORD", "")
	db := openRecloudTestDB(t)
	t.Cleanup(func() { _ = db.Close() })
	s := &server{db: db}

	res, err := db.Exec(`INSERT INTO tools (name, category, limits_json) VALUES ('EnvatoTest', 'design', '[{"key":"download","label":"Download Limit","reset_days":1}]')`)
	if err != nil {
		t.Fatal(err)
	}
	toolID, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled)
		VALUES (?, 'Envato', 'envt-reset.test', 'secret', 30, '{"download":2}', 1)`, toolID)
	if err != nil {
		t.Fatal(err)
	}
	websiteID, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO panel_users (website_id, username, status) VALUES (?, 'sagar', 'active')`, websiteID)
	if err != nil {
		t.Fatal(err)
	}
	userID, _ := res.LastInsertId()
	_, err = db.Exec(`INSERT INTO user_meters (user_id, "key", label, reset_days, used, "limit", is_custom)
		VALUES (?, 'download', 'Download Limit', 1, 3, 2, 0)`, userID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = db.Exec(`INSERT INTO usage_events (website_id, username, limit_key, limit_label, reset_days, action, target_path, amount, timestamp)
		VALUES (?, 'sagar', 'download', 'Download Limit', 1, 'Download', 'item:abc', 1, ?)`, websiteID, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO usage_events (website_id, username, limit_key, limit_label, reset_days, action, target_path, amount, timestamp)
		VALUES (?, 'sagar', 'download', 'Download Limit', 1, 'Download', 'item:def', 1, ?)`, websiteID, now)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/users/%d/reset", userID), nil)
	req.SetPathValue("id", fmt.Sprintf("%d", userID))
	req.Header.Set("Authorization", "Bearer recloud-test")
	w := httptest.NewRecorder()
	s.resetUser(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var used int
	if err := db.QueryRow(`SELECT used FROM user_meters WHERE user_id=?`, userID).Scan(&used); err != nil {
		t.Fatal(err)
	}
	if used != 0 {
		t.Fatalf("used=%d want 0", used)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM usage_events WHERE website_id=? AND username='sagar'`, websiteID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("usage_events=%d want 0 after reset", n)
	}
}
