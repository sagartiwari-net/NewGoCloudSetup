package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSaveUserUpsertsInsteadOfDuplicating(t *testing.T) {
	t.Setenv("GT4RENTS_MYSQL_PASSWORD", "")
	db := openRecloudTestDB(t)
	t.Cleanup(func() { _ = db.Close() })
	s := &server{db: db}

	res, err := db.Exec(`INSERT INTO tools (name, category, limits_json) VALUES ('AhrefsTest', 'seo', '[{"key":"credits","label":"Credits","reset_days":1},{"key":"exports","label":"Exports","reset_days":7}]')`)
	if err != nil {
		t.Fatal(err)
	}
	toolID, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled)
		VALUES (?, 'Tanveer Ahrefs', 'tanveer-ahrefs.test', 'secret', 30, '{"credits":30,"exports":10000}', 1)`, toolID)
	if err != nil {
		t.Fatal(err)
	}
	websiteID, _ := res.LastInsertId()

	// Auto-login style default row with usage already billed.
	res, err = db.Exec(`INSERT INTO panel_users (website_id, username, status) VALUES (?, 'sagar', 'active')`, websiteID)
	if err != nil {
		t.Fatal(err)
	}
	autoID, _ := res.LastInsertId()
	_, err = db.Exec(`INSERT INTO user_meters (user_id, "key", label, reset_days, used, "limit", is_custom)
		VALUES (?, 'credits', 'Credits', 1, 6, 30, 0), (?, 'exports', 'Exports', 7, 0, 10000, 0)`, autoID, autoID)
	if err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]any{
		"id":                     0,
		"website_id":             websiteID,
		"username":               "sagar",
		"status":                 "active",
		"custom_limit_expire_at": "2026-11-27",
		"limit_visibility":       "show",
		"meters": []map[string]any{
			{"key": "credits", "limit": 5, "used": 0},
			{"key": "exports", "limit": 100, "used": 0},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer recloud-test")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.saveUser(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM panel_users WHERE website_id=? AND username='sagar'`, websiteID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("panel_users rows=%d want 1 (no duplicate)", n)
	}
	var limit, used, custom int
	var visibility string
	if err := db.QueryRow(`SELECT m."limit", m.used, m.is_custom, u.limit_visibility
		FROM panel_users u JOIN user_meters m ON m.user_id=u.id AND m."key"='credits'
		WHERE u.website_id=? AND u.username='sagar'`, websiteID).Scan(&limit, &used, &custom, &visibility); err != nil {
		t.Fatal(err)
	}
	if limit != 5 || custom != 1 {
		t.Fatalf("credits limit=%d custom=%d want 5/1", limit, custom)
	}
	if used != 6 {
		t.Fatalf("credits used=%d want 6 (preserved from auto row)", used)
	}
	if visibility != "show" {
		t.Fatalf("limit_visibility=%q want show", visibility)
	}
}

func TestEnsurePanelUsersUniqueMergesDuplicates(t *testing.T) {
	t.Setenv("GT4RENTS_MYSQL_PASSWORD", "")
	db := openRecloudTestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	res, err := db.Exec(`INSERT INTO tools (name, category, limits_json) VALUES ('AhrefsDup', 'seo', '[{"key":"credits","label":"Credits","reset_days":1}]')`)
	if err != nil {
		t.Fatal(err)
	}
	toolID, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled)
		VALUES (?, 'Dup Site', 'dup-ahrefs.test', 'secret', 30, '{"credits":30}', 1)`, toolID)
	if err != nil {
		t.Fatal(err)
	}
	websiteID, _ := res.LastInsertId()
	// Schema now ships a unique index — drop it so we can seed legacy duplicates.
	_, _ = db.Exec(`DROP INDEX IF EXISTS idx_panel_users_site_user`)
	res, err = db.Exec(`INSERT INTO panel_users (website_id, username, status) VALUES (?, 'sagar', 'active')`, websiteID)
	if err != nil {
		t.Fatal(err)
	}
	autoID, _ := res.LastInsertId()
	_, _ = db.Exec(`INSERT INTO user_meters (user_id, "key", label, reset_days, used, "limit", is_custom) VALUES (?, 'credits', 'Credits', 1, 8, 30, 0)`, autoID)
	res, err = db.Exec(`INSERT INTO panel_users (website_id, username, status, custom_limit_expire_at, limit_visibility) VALUES (?, 'sagar', 'active', '2026-11-27', 'show')`, websiteID)
	if err != nil {
		t.Fatal(err)
	}
	customID, _ := res.LastInsertId()
	_, _ = db.Exec(`INSERT INTO user_meters (user_id, "key", label, reset_days, used, "limit", is_custom) VALUES (?, 'credits', 'Credits', 1, 0, 5, 1)`, customID)

	ensurePanelUsersUnique(db)

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM panel_users WHERE website_id=? AND username='sagar'`, websiteID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("after dedupe rows=%d want 1", n)
	}
	var keepID, limit, used, custom int
	var vis string
	if err := db.QueryRow(`SELECT u.id, m."limit", m.used, m.is_custom, u.limit_visibility
		FROM panel_users u JOIN user_meters m ON m.user_id=u.id AND m."key"='credits'
		WHERE u.website_id=? AND u.username='sagar'`, websiteID).Scan(&keepID, &limit, &used, &custom, &vis); err != nil {
		t.Fatal(err)
	}
	if keepID != int(customID) {
		t.Fatalf("keep_id=%d want custom id=%d", keepID, customID)
	}
	if limit != 5 || custom != 1 || used != 8 || vis != "show" {
		t.Fatalf("got limit=%d used=%d custom=%d vis=%q want 5/8/1/show", limit, used, custom, vis)
	}
	_ = autoID
}
