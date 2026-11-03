package main

import (
	"testing"
	"time"
)

func TestRefreshLimitsRollsDailyMeter(t *testing.T) {
	t.Setenv("GT4RENTS_MYSQL_PASSWORD", "")
	db := openRecloudTestDB(t)
	t.Cleanup(func() { _ = db.Close() })
	_, _ = db.Exec(`ALTER TABLE user_meters ADD COLUMN period_start TEXT NOT NULL DEFAULT ''`)
	s := &server{db: db}

	res, err := db.Exec(`INSERT INTO tools (name, category, limits_json) VALUES ('MaintTool', 'seo', '[]')`)
	if err != nil {
		t.Fatal(err)
	}
	toolID, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled)
		VALUES (?, 'Maint Site', 'maint.test', 'secret', 30, '{"credits":10}', 1)`, toolID)
	if err != nil {
		t.Fatal(err)
	}
	websiteID, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO panel_users (website_id, username, status) VALUES (?, 'maint-user', 'active')`, websiteID)
	if err != nil {
		t.Fatal(err)
	}
	userID, _ := res.LastInsertId()
	oldStart := periodStartIST(1, time.Now().Add(-48*time.Hour)).UTC().Format(time.RFC3339)
	res, err = db.Exec(`INSERT INTO user_meters (user_id, "key", label, reset_days, used, "limit", is_custom, period_start)
		VALUES (?, 'credits', 'Credits', 1, 9, 10, 0, ?)`, userID, oldStart)
	if err != nil {
		t.Fatal(err)
	}
	meterID, _ := res.LastInsertId()

	s.refreshLimits()

	var used int
	var start string
	if err := db.QueryRow(`SELECT used, period_start FROM user_meters WHERE id=?`, meterID).Scan(&used, &start); err != nil {
		t.Fatal(err)
	}
	wantStart := periodStartIST(1, time.Now()).UTC().Format(time.RFC3339)
	if start != wantStart {
		t.Fatalf("period_start=%q want %q", start, wantStart)
	}
	if used != 0 {
		t.Fatalf("used=%d want 0 after period roll with no today events", used)
	}
}

func TestRunPurgeForceBypassesThrottle(t *testing.T) {
	t.Setenv("GT4RENTS_MYSQL_PASSWORD", "")
	db := openRecloudTestDB(t)
	t.Cleanup(func() { _ = db.Close() })
	s := &server{db: db}

	purgeMu.Lock()
	lastPurge = time.Now()
	purgeMu.Unlock()

	before := lastPurge
	s.runPurge(false)
	purgeMu.Lock()
	if lastPurge != before {
		purgeMu.Unlock()
		t.Fatal("throttled purge should not move lastPurge")
	}
	purgeMu.Unlock()

	mark := time.Now()
	time.Sleep(5 * time.Millisecond)
	s.runPurge(true)
	purgeMu.Lock()
	after := lastPurge
	purgeMu.Unlock()
	if !after.After(mark) {
		t.Fatal("force purge should refresh lastPurge")
	}
}
