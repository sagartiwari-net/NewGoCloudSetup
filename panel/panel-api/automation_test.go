package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"panel-api/automation"
	"strings"
	"sync"
	"testing"
	"time"
)

const automationFakeCookie = "FAKECOOKIE_automation_test_value"

type scriptedRecipe struct {
	mu          sync.Mutex
	logins      int
	forceDead   bool
	failLogin   bool
	liveValue   string
	portalValue string
	toolValue   string
}

func (r *scriptedRecipe) Check(ctx context.Context, cookies []cookieJSON) (bool, []cookieJSON, error) {
	if r.forceDead {
		return false, nil, nil
	}
	for _, cookie := range cookies {
		if cookie.Value == r.liveValue || cookie.Value == r.portalValue {
			return true, cookies, nil
		}
	}
	return false, nil, nil
}

func (r *scriptedRecipe) Login(ctx context.Context, username, password string) ([]cookieJSON, error) {
	r.mu.Lock()
	r.logins++
	r.mu.Unlock()
	if r.failLogin {
		return nil, fmt.Errorf("%w %s %s", errAutomationLoginPage, password, automationFakeCookie)
	}
	return []cookieJSON{{
		Domain:   "markhorseotool.com",
		HostOnly: true,
		Name:     "session",
		Path:     "/",
		Secure:   true,
		Value:    r.portalValue,
	}}, nil
}

func (r *scriptedRecipe) PortalHost() string { return "markhorseotool.com" }

func (r *scriptedRecipe) Capture(ctx context.Context, portal []cookieJSON, upstreamURL string) ([]cookieJSON, error) {
	if strings.TrimSpace(upstreamURL) == "" {
		return nil, errAutomationUpstream
	}
	return []cookieJSON{{
		Domain:   "sm01.example",
		HostOnly: true,
		Name:     "tool",
		Path:     "/",
		Secure:   true,
		Value:    r.toolValue,
	}}, nil
}

func (r *scriptedRecipe) loginCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.logins
}

func useAutomationRecipe(t *testing.T, recipe automation.Recipe) {
	t.Helper()
	recipeMu.Lock()
	prev := automationRecipes["markhor"]
	automationRecipes["markhor"] = recipe
	recipeMu.Unlock()
	t.Cleanup(func() {
		recipeMu.Lock()
		automationRecipes["markhor"] = prev
		recipeMu.Unlock()
	})
}

func openAutomationTestDB(t *testing.T) (*server, http.Handler) {
	t.Helper()
	t.Setenv("GT4RENTS_MYSQL_PASSWORD", "")
	t.Setenv("GT4RENTS_AUTOMATION_KEY", "")
	db := openRecloudTestDB(t)
	t.Cleanup(func() { _ = db.Close() })
	ensureAutomationTables(db)
	s := &server{db: db}
	mux := http.NewServeMux()
	s.routes(mux)
	return s, mux
}

func callAuth(t *testing.T, handler http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func insertAutomationCredential(t *testing.T, s *server, password string) int {
	t.Helper()
	enc, err := encryptAutomation(password)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`INSERT INTO automation_credentials (name, provider, username, password_enc, created_at, updated_at)
		VALUES ('Markhor', 'markhor', 'portal-user', ?, ?, ?)`, enc, now, now)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

func insertAutomationJob(t *testing.T, s *server, name string, credentialID, websiteID, accountID int) int {
	t.Helper()
	now := time.Now().UTC()
	res, err := s.db.Exec(`INSERT INTO automation_jobs (name, provider, credential_id, target_website_id, target_account_id, interval_minutes, enabled, run_requested, next_run_at, last_run_at, last_result, last_reason, created_at, updated_at)
		VALUES (?, 'markhor', ?, ?, ?, 30, 1, 0, ?, '', '', '', ?, ?)`,
		name, credentialID, websiteID, accountID, now.Add(-time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

func TestAutomationResellerForbidden(t *testing.T) {
	s, mux := openAutomationTestDB(t)
	expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`INSERT INTO operators (username, password_hash, role, status) VALUES ('seller', 'x', 'reseller', 'active')`)
	if err != nil {
		t.Fatal(err)
	}
	sellerID, _ := res.LastInsertId()
	if _, err := s.db.Exec(`INSERT INTO admin_sessions (token, operator_id, expires_at) VALUES ('reseller-test', ?, ?)`, sellerID, expires); err != nil {
		t.Fatal(err)
	}
	list := callAuth(t, mux, http.MethodGet, "/api/automation/credentials", "", "reseller-test")
	if list.Code != 403 || !bytes.Contains(list.Body.Bytes(), []byte("Master only")) {
		t.Fatalf("list status %d body %s", list.Code, list.Body.String())
	}
	run := callAuth(t, mux, http.MethodPost, "/api/automation/jobs/1/run", "", "reseller-test")
	if run.Code != 403 || !bytes.Contains(run.Body.Bytes(), []byte("Master only")) {
		t.Fatalf("run status %d body %s", run.Code, run.Body.String())
	}
}

func TestAutomationValidCookieSkipsLogin(t *testing.T) {
	s, _ := openAutomationTestDB(t)
	recipe := &scriptedRecipe{liveValue: "live-session-value", portalValue: "portal-session-value", toolValue: "tool-session-value"}
	useAutomationRecipe(t, recipe)
	cred := insertAutomationCredential(t, s, "portal-secret")
	if err := s.replaceAutomationStorage(cred, `[{"name":"session","value":"live-session-value","domain":"markhorseotool.com","path":"/"}]`); err != nil {
		t.Fatal(err)
	}
	var websiteID int
	if err := s.db.QueryRow(`SELECT id FROM websites WHERE kind='recloud'`).Scan(&websiteID); err != nil {
		t.Fatal(err)
	}
	insertAutomationJob(t, s, "Markhor Semrush 1", cred, websiteID, 0)
	s.runDueAutomationJobs()
	if recipe.loginCount() != 0 {
		t.Fatalf("logins = %d", recipe.loginCount())
	}
	var stored, account string
	if err := s.db.QueryRow(`SELECT cookies_json FROM automation_storage WHERE credential_id=?`, cred).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored, "live-session-value") {
		t.Fatal("storage lost the valid session")
	}
	if err := s.db.QueryRow(`SELECT cookie FROM accounts WHERE website_id=?`, websiteID).Scan(&account); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(account, "tool-session-value") {
		t.Fatal("target account did not receive the tool cookie")
	}
}

func TestAutomationDeadCookieLogsInOnce(t *testing.T) {
	s, _ := openAutomationTestDB(t)
	recipe := &scriptedRecipe{liveValue: "live-session-value", portalValue: "portal-session-value", toolValue: "tool-session-value"}
	useAutomationRecipe(t, recipe)
	cred := insertAutomationCredential(t, s, "portal-secret")
	if err := s.replaceAutomationStorage(cred, `[{"name":"session","value":"dead-session-value","domain":"markhorseotool.com","path":"/"}]`); err != nil {
		t.Fatal(err)
	}
	var recloudID, mappedID, accountID int
	if err := s.db.QueryRow(`SELECT id FROM websites WHERE kind='recloud'`).Scan(&recloudID); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT id FROM websites WHERE kind=''`).Scan(&mappedID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE websites SET upstream_url='https://mapped.example' WHERE id=?`, mappedID); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT id FROM accounts WHERE website_id=?`, mappedID).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	insertAutomationJob(t, s, "Markhor Semrush 1", cred, recloudID, 0)
	insertAutomationJob(t, s, "Markhor mapped", cred, mappedID, accountID)
	s.runDueAutomationJobs()
	if recipe.loginCount() != 1 {
		t.Fatalf("logins = %d", recipe.loginCount())
	}
	var stored, recloudCookie, mappedCookie string
	if err := s.db.QueryRow(`SELECT cookies_json FROM automation_storage WHERE credential_id=?`, cred).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored, "portal-session-value") || strings.Contains(stored, "dead-session-value") {
		t.Fatal("storage was not overwritten with the new portal cookie")
	}
	if err := s.db.QueryRow(`SELECT cookie FROM accounts WHERE website_id=?`, recloudID).Scan(&recloudCookie); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT cookie FROM accounts WHERE id=?`, accountID).Scan(&mappedCookie); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(recloudCookie, "tool-session-value") || !strings.Contains(mappedCookie, "tool-session-value") {
		t.Fatal("target accounts did not receive the tool cookie")
	}
}

func TestAutomationLoginFailureKeepsCookies(t *testing.T) {
	s, mux := openAutomationTestDB(t)
	recipe := &scriptedRecipe{failLogin: true, portalValue: "portal-session-value", toolValue: "tool-session-value"}
	useAutomationRecipe(t, recipe)
	cred := insertAutomationCredential(t, s, "portal-secret")
	if err := s.replaceAutomationStorage(cred, `[{"name":"session","value":"`+automationFakeCookie+`","domain":"markhorseotool.com","path":"/"}]`); err != nil {
		t.Fatal(err)
	}
	var websiteID int
	if err := s.db.QueryRow(`SELECT id FROM websites WHERE kind='recloud'`).Scan(&websiteID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`INSERT INTO accounts (website_id, name, cookie, user_agent, proxy, status, show_limit, description, automation_task_uid, automation_ingest_key, cookie_updated_at, last_used_at)
		VALUES (?, 'seat', 'old-account-cookie', '', '', 'active', 1, '', '', '', ?, '')`, websiteID, now); err != nil {
		t.Fatal(err)
	}
	jobID := insertAutomationJob(t, s, "Markhor Semrush 1", cred, websiteID, 0)
	s.runDueAutomationJobs()
	if recipe.loginCount() != 1 {
		t.Fatalf("logins = %d", recipe.loginCount())
	}
	var stored, account, reason string
	if err := s.db.QueryRow(`SELECT cookies_json FROM automation_storage WHERE credential_id=?`, cred).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored, automationFakeCookie) {
		t.Fatal("login failure wiped storage")
	}
	if err := s.db.QueryRow(`SELECT cookie FROM accounts WHERE website_id=?`, websiteID).Scan(&account); err != nil {
		t.Fatal(err)
	}
	if account != "old-account-cookie" {
		t.Fatal("login failure changed the account cookie")
	}
	if err := s.db.QueryRow(`SELECT last_reason FROM automation_jobs WHERE id=?`, jobID).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != "login page still showing" {
		t.Fatalf("reason %q", reason)
	}
	logs := callAuth(t, mux, http.MethodGet, "/api/automation/logs?page=1&pageSize=20", "", "recloud-test")
	if logs.Code != 200 {
		t.Fatalf("logs status %d", logs.Code)
	}
	body := logs.Body.String()
	if strings.Contains(body, automationFakeCookie) || strings.Contains(body, "portal-secret") || strings.Contains(body, "old-account-cookie") {
		t.Fatal("log list included a cookie or password")
	}
	if !strings.Contains(body, "login page still showing") {
		t.Fatal("missing error log")
	}
}

func TestAutomationLogPurgeKeepsStorage(t *testing.T) {
	s, _ := openAutomationTestDB(t)
	cred := insertAutomationCredential(t, s, "portal-secret")
	old := time.Now().UTC().Add(-4 * time.Hour).Format(time.RFC3339)
	fresh := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`INSERT INTO automation_storage (credential_id, cookies_json, updated_at) VALUES (?, ?, ?)`, cred, `[{"name":"session","value":"`+automationFakeCookie+`"}]`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO automation_logs (job_id, job_name, credential_id, credential_name, level, message, created_at) VALUES (1, 'Old', ?, 'Markhor', 'info', 'old row', ?)`, cred, old); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO automation_logs (job_id, job_name, credential_id, credential_name, level, message, created_at) VALUES (1, 'New', ?, 'Markhor', 'info', 'fresh row', ?)`, cred, fresh); err != nil {
		t.Fatal(err)
	}
	var websiteID int
	_ = s.db.QueryRow(`SELECT id FROM websites WHERE kind='recloud'`).Scan(&websiteID)
	insertAutomationJob(t, s, "Keep me", cred, websiteID, 0)
	purgeMu.Lock()
	lastPurge = time.Time{}
	purgeMu.Unlock()
	s.purgeOldRows()
	var logs int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM automation_logs`).Scan(&logs); err != nil {
		t.Fatal(err)
	}
	if logs != 1 {
		t.Fatalf("logs left = %d", logs)
	}
	var message, stored string
	if err := s.db.QueryRow(`SELECT message FROM automation_logs`).Scan(&message); err != nil {
		t.Fatal(err)
	}
	if message != "fresh row" {
		t.Fatalf("kept log %q", message)
	}
	if err := s.db.QueryRow(`SELECT cookies_json FROM automation_storage WHERE credential_id=?`, cred).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored, automationFakeCookie) {
		t.Fatal("purge deleted storage")
	}
	var jobs, creds int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM automation_jobs`).Scan(&jobs)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM automation_credentials`).Scan(&creds)
	if jobs != 1 || creds != 1 {
		t.Fatalf("jobs %d credentials %d", jobs, creds)
	}
}

func TestAutomationClearLogsKeepsStorage(t *testing.T) {
	s, mux := openAutomationTestDB(t)
	cred := insertAutomationCredential(t, s, "portal-secret")
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`INSERT INTO automation_storage (credential_id, cookies_json, updated_at) VALUES (?, ?, ?)`, cred, `[{"name":"session","value":"`+automationFakeCookie+`"}]`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO automation_logs (job_id, job_name, credential_id, credential_name, level, message, created_at) VALUES (1, 'Markhor', ?, 'Markhor', 'error', 'login page still showing', ?)`, cred, now); err != nil {
		t.Fatal(err)
	}
	cleared := callAuth(t, mux, http.MethodDelete, "/api/automation/logs", "", "recloud-test")
	if cleared.Code != 200 {
		t.Fatalf("clear status %d body %s", cleared.Code, cleared.Body.String())
	}
	var logs int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM automation_logs`).Scan(&logs); err != nil {
		t.Fatal(err)
	}
	if logs != 0 {
		t.Fatalf("logs left = %d", logs)
	}
	var stored string
	if err := s.db.QueryRow(`SELECT cookies_json FROM automation_storage WHERE credential_id=?`, cred).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored, automationFakeCookie) {
		t.Fatal("clear deleted storage")
	}
}

func TestAutomationListsHideCookies(t *testing.T) {
	s, mux := openAutomationTestDB(t)
	cred := insertAutomationCredential(t, s, "portal-secret")
	if err := s.replaceAutomationStorage(cred, `[{"name":"session","value":"`+automationFakeCookie+`"}]`); err != nil {
		t.Fatal(err)
	}
	var websiteID int
	if err := s.db.QueryRow(`SELECT id FROM websites WHERE kind='recloud'`).Scan(&websiteID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`INSERT INTO accounts (website_id, name, cookie, user_agent, proxy, status, show_limit, description, automation_task_uid, automation_ingest_key, cookie_updated_at, last_used_at)
		VALUES (?, 'seat', ?, '', '', 'active', 1, '', '', '', ?, '')`, websiteID, automationFakeCookie, now); err != nil {
		t.Fatal(err)
	}
	recloud := callAuth(t, mux, http.MethodGet, "/api/recloud?page=1&pageSize=20", "", "recloud-test")
	storage := callAuth(t, mux, http.MethodGet, "/api/automation/storage?page=1&pageSize=20", "", "recloud-test")
	jobs := callAuth(t, mux, http.MethodGet, "/api/automation/jobs?page=1&pageSize=20", "", "recloud-test")
	if recloud.Code != 200 || storage.Code != 200 || jobs.Code != 200 {
		t.Fatalf("recloud %d storage %d jobs %d", recloud.Code, storage.Code, jobs.Code)
	}
	if bytes.Contains(recloud.Body.Bytes(), []byte(automationFakeCookie)) || bytes.Contains(storage.Body.Bytes(), []byte(automationFakeCookie)) || bytes.Contains(jobs.Body.Bytes(), []byte(automationFakeCookie)) {
		t.Fatal("list included a cookie")
	}
	if !bytes.Contains(storage.Body.Bytes(), []byte("Markhor")) {
		t.Fatal("storage list dropped the credential")
	}
}

func TestAutomationShareCookieFrom(t *testing.T) {
	s, _ := openAutomationTestDB(t)
	s.ensureCookieLinks()
	recipe := &scriptedRecipe{portalValue: "portal-session-value", toolValue: "tool-session-value"}
	useAutomationRecipe(t, recipe)
	cred := insertAutomationCredential(t, s, "portal-secret")
	if err := s.replaceAutomationStorage(cred, `[{"name":"session","value":"dead-session-value","domain":"markhorseotool.com","path":"/"}]`); err != nil {
		t.Fatal(err)
	}
	var recloudID, followerID int
	if err := s.db.QueryRow(`SELECT id FROM websites WHERE kind='recloud'`).Scan(&recloudID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`INSERT INTO accounts (website_id, name, cookie, user_agent, proxy, status, show_limit, description, automation_task_uid, automation_ingest_key, cookie_updated_at, last_used_at)
		VALUES (?, 'seat', 'old-account-cookie', '', '', 'active', 1, '', '', '', ?, '')`, recloudID, now)
	if err != nil {
		t.Fatal(err)
	}
	sourceID, _ := res.LastInsertId()
	if err := s.db.QueryRow(`SELECT id FROM accounts WHERE website_id<>?`, recloudID).Scan(&followerID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE accounts SET cookie='follower-before' WHERE id=?`, followerID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO account_cookie_links (follower_account_id, source_account_id, created_at) VALUES (?,?,?)`, followerID, sourceID, now); err != nil {
		t.Fatal(err)
	}
	insertAutomationJob(t, s, "Markhor Semrush 1", cred, recloudID, 0)
	s.runDueAutomationJobs()
	var sourceCookie, followerCookie string
	if err := s.db.QueryRow(`SELECT cookie FROM accounts WHERE id=?`, sourceID).Scan(&sourceCookie); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT cookie FROM accounts WHERE id=?`, followerID).Scan(&followerCookie); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sourceCookie, "tool-session-value") || followerCookie != sourceCookie {
		t.Fatal("follower did not receive the written cookie")
	}
	recipe.forceDead = true
	recipe.failLogin = true
	if _, err := s.db.Exec(`UPDATE automation_jobs SET next_run_at=?, run_requested=1 WHERE credential_id=?`, time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), cred); err != nil {
		t.Fatal(err)
	}
	s.runDueAutomationJobs()
	var afterSource, afterFollower, stored string
	if err := s.db.QueryRow(`SELECT cookie FROM accounts WHERE id=?`, sourceID).Scan(&afterSource); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT cookie FROM accounts WHERE id=?`, followerID).Scan(&afterFollower); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT cookies_json FROM automation_storage WHERE credential_id=?`, cred).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if afterSource != sourceCookie || afterFollower != followerCookie {
		t.Fatal("failed login cleared a cookie")
	}
	if !strings.Contains(stored, "portal-session-value") {
		t.Fatal("failed login wiped storage")
	}
}

func TestAutomationNextRunUsesLastRun(t *testing.T) {
	last := time.Date(2026, 10, 14, 7, 7, 46, 0, time.UTC)
	saved := last.Add(2 * time.Minute)
	got := automationNextRun(last.Format(time.RFC3339), 5, saved)
	want := last.Add(5 * time.Minute)
	if !got.Equal(want) {
		t.Fatalf("next = %s, want %s", got, want)
	}

	late := last.Add(10 * time.Minute)
	got = automationNextRun(last.Format(time.RFC3339), 5, late)
	want = late.Add(5 * time.Minute)
	if !got.Equal(want) {
		t.Fatalf("overdue next = %s, want %s", got, want)
	}

	got = automationNextRun("", 5, saved)
	if !got.Equal(saved.Add(5 * time.Minute)) {
		t.Fatal("a job with no last run should count from the save")
	}

	got = automationNextRun(last.Format(time.RFC3339), 0, saved)
	if !got.Equal(saved) {
		t.Fatal("timer off should not schedule another run")
	}
}

func TestAutomationTimerOffKeepsLogoutAndManual(t *testing.T) {
	s, mux := openAutomationTestDB(t)
	recipe := &scriptedRecipe{liveValue: "live-session-value", portalValue: "portal-session-value", toolValue: "tool-session-value"}
	useAutomationRecipe(t, recipe)
	cred := insertAutomationCredential(t, s, "portal-secret")
	if err := s.replaceAutomationStorage(cred, `[{"name":"session","value":"live-session-value","domain":"markhorseotool.com","path":"/"}]`); err != nil {
		t.Fatal(err)
	}
	var websiteID int
	if err := s.db.QueryRow(`SELECT id FROM websites WHERE kind='recloud'`).Scan(&websiteID); err != nil {
		t.Fatal(err)
	}
	jobID := insertAutomationJob(t, s, "Markhor Semrush 1", cred, websiteID, 0)
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	if _, err := s.db.Exec(`UPDATE automation_jobs SET interval_minutes=0, next_run_at=?, last_run_at=? WHERE id=?`, past, past, jobID); err != nil {
		t.Fatal(err)
	}
	s.runDueAutomationJobs()
	var lastRun string
	if err := s.db.QueryRow(`SELECT last_run_at FROM automation_jobs WHERE id=?`, jobID).Scan(&lastRun); err != nil {
		t.Fatal(err)
	}
	if lastRun != past {
		t.Fatal("timer off job ran on the interval")
	}

	if _, err := s.db.Exec(`UPDATE automation_jobs SET run_requested=1 WHERE id=?`, jobID); err != nil {
		t.Fatal(err)
	}
	s.runDueAutomationJobs()
	if err := s.db.QueryRow(`SELECT last_run_at FROM automation_jobs WHERE id=?`, jobID).Scan(&lastRun); err != nil {
		t.Fatal(err)
	}
	if lastRun == past {
		t.Fatal("timer off blocked Run now")
	}

	if _, err := s.db.Exec(`UPDATE automation_jobs SET interval_minutes=0, last_run_at=?, next_run_at=?, run_requested=0, queued_cause='', last_trigger_at='' WHERE id=?`, past, past, jobID); err != nil {
		t.Fatal(err)
	}
	if got := s.enqueueAutomationTrigger(jobID, websiteID); got != "accepted" {
		t.Fatalf("logout trigger %s", got)
	}
	s.startQueuedAutomation()
	if err := s.db.QueryRow(`SELECT last_run_at FROM automation_jobs WHERE id=?`, jobID).Scan(&lastRun); err != nil {
		t.Fatal(err)
	}
	if lastRun == past {
		t.Fatal("timer off blocked a logout trigger")
	}

	if _, err := s.db.Exec(`UPDATE automation_jobs SET enabled=0, run_requested=1, last_run_at=? WHERE id=?`, past, jobID); err != nil {
		t.Fatal(err)
	}
	s.runDueAutomationJobs()
	if err := s.db.QueryRow(`SELECT last_run_at FROM automation_jobs WHERE id=?`, jobID).Scan(&lastRun); err != nil {
		t.Fatal(err)
	}
	if lastRun != past {
		t.Fatal("disabled job ran from Run now")
	}
	blocked := callAuth(t, mux, http.MethodPost, fmt.Sprintf("/api/automation/jobs/%d/run", jobID), "", "recloud-test")
	if blocked.Code != 400 {
		t.Fatalf("disabled run status %d", blocked.Code)
	}

	sessionID := insertAutomationJob(t, s, "Azad homepage", cred, 0, 0)
	later := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	if _, err := s.db.Exec(`UPDATE automation_jobs SET job_kind='session', target_website_id=0, interval_minutes=0, next_run_at=?, last_run_at='' WHERE id=?`, later, sessionID); err != nil {
		t.Fatal(err)
	}
	s.runDueAutomationJobs()
	var sessionRun string
	if err := s.db.QueryRow(`SELECT COALESCE(last_run_at,'') FROM automation_jobs WHERE id=?`, sessionID).Scan(&sessionRun); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(sessionRun) != "" {
		t.Fatal("timer off homepage ran before anyone asked")
	}
}

func insertMarkhorWebsite(t *testing.T, s *server, name, domain string) int {
	t.Helper()
	res, err := s.db.Exec(`INSERT INTO tools (name, category, limits_json) VALUES (?, 'recloud', '[]')`, name)
	if err != nil {
		t.Fatal(err)
	}
	toolID, _ := res.LastInsertId()
	res, err = s.db.Exec(`INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url)
		VALUES (?, ?, ?, 'secret', 30, '{}', 1, 'recloud', 'https://sm01.markhorseotool.com')`, toolID, name, domain)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

func TestAutomationLogoutTrigger(t *testing.T) {
	s, _ := openAutomationTestDB(t)
	recipe := &scriptedRecipe{liveValue: "live-session-value", portalValue: "portal-session-value", toolValue: "tool-session-value"}
	useAutomationRecipe(t, recipe)
	cred := insertAutomationCredential(t, s, "portal-secret")
	if err := s.replaceAutomationStorage(cred, `[{"name":"session","value":"live-session-value","domain":"markhorseotool.com","path":"/"}]`); err != nil {
		t.Fatal(err)
	}
	first := insertMarkhorWebsite(t, s, "Markhor Semrush", "markhsm1.example")
	second := insertMarkhorWebsite(t, s, "Markhor Grammarly", "markhgram.example")
	jobA := insertAutomationJob(t, s, "Markhor Home", cred, first, 0)
	now := time.Now().UTC().Format(time.RFC3339)
	for _, websiteID := range []int{first, second, first} {
		if _, err := s.db.Exec(`INSERT INTO automation_signals (website_id, created_at, state) VALUES (?,?,'pending')`, websiteID, now); err != nil {
			t.Fatal(err)
		}
	}
	s.drainAutomationSignals()
	var queued string
	var queuedCount int
	if err := s.db.QueryRow(`SELECT queued_cause FROM automation_jobs WHERE id=?`, jobA).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != "trigger" {
		t.Fatalf("queued = %q", queued)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM automation_jobs WHERE credential_id=? AND queued_cause='trigger'`, cred).Scan(&queuedCount); err != nil {
		t.Fatal(err)
	}
	if queuedCount != 1 {
		t.Fatalf("queued jobs = %d", queuedCount)
	}
	var skipped int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM automation_logs WHERE message LIKE 'logout trigger skipped%'`).Scan(&skipped); err != nil {
		t.Fatal(err)
	}
	if skipped != 2 {
		t.Fatalf("skipped = %d", skipped)
	}
	s.startQueuedAutomation()
	if recipe.loginCount() != 0 {
		t.Fatalf("logins = %d", recipe.loginCount())
	}
	var triggerAt string
	if err := s.db.QueryRow(`SELECT last_trigger_at FROM automation_jobs WHERE id=?`, jobA).Scan(&triggerAt); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(triggerAt) == "" {
		t.Fatal("trigger did not stamp the cooldown")
	}
	if _, err := s.db.Exec(`INSERT INTO automation_signals (website_id, created_at, state) VALUES (?,?,'pending')`, second, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	s.drainAutomationSignals()
	if err := s.db.QueryRow(`SELECT queued_cause FROM automation_jobs WHERE id=?`, jobA).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != "" {
		t.Fatal("a trigger inside 5 minutes was queued")
	}

	_, _ = s.db.Exec(`UPDATE automation_jobs SET last_trigger_at='', last_run_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339), jobA)
	if _, err := s.db.Exec(`INSERT INTO automation_signals (website_id, created_at, state) VALUES (?,?,'pending')`, first, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	s.drainAutomationSignals()
	if err := s.db.QueryRow(`SELECT queued_cause FROM automation_jobs WHERE id=?`, jobA).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != "trigger" {
		t.Fatal("a manual run should not block the next logout trigger")
	}

	jobB := insertAutomationJob(t, s, "Markhor Grammarly", cred, second, 0)
	_, _ = s.db.Exec(`UPDATE automation_jobs SET running=1, queued_cause='' WHERE id=?`, jobA)
	if _, err := s.db.Exec(`INSERT INTO automation_signals (website_id, created_at, state) VALUES (?,?,'pending')`, second, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO automation_signals (website_id, created_at, state) VALUES (?,?,'pending')`, second, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	s.drainAutomationSignals()
	if err := s.db.QueryRow(`SELECT queued_cause FROM automation_jobs WHERE id=?`, jobB).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != "trigger" {
		t.Fatalf("different job queued = %q", queued)
	}
	var waiting int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM automation_logs WHERE job_id=? AND message LIKE 'logout trigger waiting%'`, jobB).Scan(&waiting); err != nil {
		t.Fatal(err)
	}
	if waiting != 1 {
		t.Fatalf("waiting logs = %d", waiting)
	}
	_, _ = s.db.Exec(`UPDATE automation_jobs SET running=0, last_trigger_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339), jobA)
	s.startQueuedAutomation()
	if err := s.db.QueryRow(`SELECT queued_cause, last_trigger_at FROM automation_jobs WHERE id=?`, jobB).Scan(&queued, &triggerAt); err != nil {
		t.Fatal(err)
	}
	if queued != "" || strings.TrimSpace(triggerAt) == "" {
		t.Fatalf("the other tool did not refresh, queued=%q trigger=%q", queued, triggerAt)
	}
	var leaked int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM automation_logs WHERE message LIKE '%' || ? || '%'`, automationFakeCookie).Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	if leaked != 0 {
		t.Fatal("automation log contains a cookie")
	}
}

func TestAutomationSessionJobStoresHomepageOnly(t *testing.T) {
	s, _ := openAutomationTestDB(t)
	recipe := &scriptedRecipe{liveValue: "live-session-value", portalValue: "portal-session-value", toolValue: "tool-session-value"}
	useAutomationRecipe(t, recipe)
	cred := insertAutomationCredential(t, s, "portal-secret")
	if err := s.replaceAutomationStorage(cred, `[{"name":"session","value":"live-session-value","domain":"markhorseotool.com","path":"/"}]`); err != nil {
		t.Fatal(err)
	}
	var websiteID int
	if err := s.db.QueryRow(`SELECT id FROM websites WHERE kind='recloud'`).Scan(&websiteID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`INSERT INTO accounts (website_id, name, cookie, user_agent, proxy, status, show_limit, description, automation_task_uid, automation_ingest_key, cookie_updated_at, last_used_at)
		VALUES (?, 'tool', 'old-tool-cookie', '', '', 'active', 1, '', '', '', ?, '')`, websiteID, now); err != nil {
		t.Fatal(err)
	}
	jobID := insertAutomationJob(t, s, "Azad homepage", cred, websiteID, 0)
	if _, err := s.db.Exec(`UPDATE automation_jobs SET job_kind='session', target_website_id=0 WHERE id=?`, jobID); err != nil {
		t.Fatal(err)
	}
	s.runDueAutomationJobs()
	if recipe.loginCount() != 0 {
		t.Fatalf("logins = %d", recipe.loginCount())
	}
	var stored, toolCookie, reason string
	if err := s.db.QueryRow(`SELECT cookies_json FROM automation_storage WHERE credential_id=?`, cred).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored, "live-session-value") {
		t.Fatal("homepage cookie was not kept")
	}
	if err := s.db.QueryRow(`SELECT cookie FROM accounts WHERE website_id=?`, websiteID).Scan(&toolCookie); err != nil {
		t.Fatal(err)
	}
	if toolCookie != "old-tool-cookie" {
		t.Fatalf("session job wrote a tool cookie: %s", toolCookie)
	}
	if err := s.db.QueryRow(`SELECT last_reason FROM automation_jobs WHERE id=?`, jobID).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != "stored homepage cookie" {
		t.Fatalf("reason = %s", reason)
	}
}

func TestAutomationFreshHomepageRunsBeforeItsInterval(t *testing.T) {
	s, _ := openAutomationTestDB(t)
	recipe := &scriptedRecipe{liveValue: "live-session-value", portalValue: "portal-session-value", toolValue: "tool-session-value"}
	useAutomationRecipe(t, recipe)
	cred := insertAutomationCredential(t, s, "portal-secret")
	if err := s.replaceAutomationStorage(cred, `[{"name":"session","value":"live-session-value","domain":"azadseo.com","path":"/"}]`); err != nil {
		t.Fatal(err)
	}
	jobID := insertAutomationJob(t, s, "Azad homepage", cred, 0, 0)
	later := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	if _, err := s.db.Exec(`UPDATE automation_jobs SET job_kind='session', target_website_id=0, next_run_at=?, last_run_at='' WHERE id=?`, later, jobID); err != nil {
		t.Fatal(err)
	}
	s.runDueAutomationJobs()
	var reason, lastRun string
	if err := s.db.QueryRow(`SELECT last_reason, last_run_at FROM automation_jobs WHERE id=?`, jobID).Scan(&reason, &lastRun); err != nil {
		t.Fatal(err)
	}
	if reason != "stored homepage cookie" || strings.TrimSpace(lastRun) == "" {
		t.Fatalf("homepage did not run, reason=%s last=%s", reason, lastRun)
	}
}

func TestAmzToolAsksHomepageToLogin(t *testing.T) {
	s, _ := openAutomationTestDB(t)
	recipe := &scriptedRecipe{liveValue: "live-session-value", portalValue: "portal-session-value", toolValue: "tool-session-value"}
	recipeMu.Lock()
	prev := automationRecipes["amztoolsconsultant"]
	automationRecipes["amztoolsconsultant"] = recipe
	recipeMu.Unlock()
	t.Cleanup(func() {
		recipeMu.Lock()
		automationRecipes["amztoolsconsultant"] = prev
		recipeMu.Unlock()
	})
	cred := insertAutomationCredential(t, s, "portal-secret")
	if _, err := s.db.Exec(`UPDATE automation_credentials SET provider='amztoolsconsultant' WHERE id=?`, cred); err != nil {
		t.Fatal(err)
	}
	if err := s.replaceAutomationStorage(cred, `[{"name":"PHPSESSID","value":"dead","domain":".amztoolsconsultant.com","path":"/"}]`); err != nil {
		t.Fatal(err)
	}
	site := insertMarkhorWebsite(t, s, "Helium10 Diamond", "amzcheliumdia.example")
	toolID := insertAutomationJob(t, s, "Amz diamond", cred, site, 0)
	if _, err := s.db.Exec(`UPDATE automation_jobs SET provider='amztoolsconsultant', run_requested=1 WHERE id=?`, toolID); err != nil {
		t.Fatal(err)
	}
	s.runAutomationJob(context.Background(), toolID)
	if recipe.loginCount() != 0 {
		t.Fatalf("tool logged in itself %d", recipe.loginCount())
	}
	var reason string
	if err := s.db.QueryRow(`SELECT last_reason FROM automation_jobs WHERE id=?`, toolID).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != "enable the homepage job first" {
		t.Fatalf("reason %q", reason)
	}
	homeID := insertAutomationJob(t, s, "Amz homepage", cred, 0, 0)
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	if _, err := s.db.Exec(`UPDATE automation_jobs SET job_kind='session', provider='amztoolsconsultant', target_website_id=0, last_run_at=? WHERE id=?`, past, homeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE automation_jobs SET run_requested=1 WHERE id=?`, toolID); err != nil {
		t.Fatal(err)
	}
	s.runAutomationJob(context.Background(), toolID)
	if recipe.loginCount() != 1 {
		t.Fatalf("homepage logins %d", recipe.loginCount())
	}
	if err := s.db.QueryRow(`SELECT last_reason FROM automation_jobs WHERE id=?`, homeID).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != "stored homepage cookie" {
		t.Fatalf("homepage reason %q", reason)
	}
	if err := s.db.QueryRow(`SELECT last_reason FROM automation_jobs WHERE id=?`, toolID).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reason, "wrote cookie to") {
		t.Fatalf("diamond reason %q", reason)
	}
	if _, err := s.db.Exec(`UPDATE automation_jobs SET run_requested=1 WHERE id=?`, toolID); err != nil {
		t.Fatal(err)
	}
	s.runAutomationJob(context.Background(), toolID)
	if recipe.loginCount() != 1 {
		t.Fatalf("valid cookie logged in again %d", recipe.loginCount())
	}
}

func TestAmzKeepaLogsInWithoutHomepage(t *testing.T) {
	s, _ := openAutomationTestDB(t)
	recipe := &scriptedRecipe{liveValue: "live-session-value", portalValue: "portal-session-value", toolValue: "tool-session-value"}
	recipeMu.Lock()
	prev := automationRecipes["amztoolsconsultant"]
	automationRecipes["amztoolsconsultant"] = recipe
	recipeMu.Unlock()
	t.Cleanup(func() {
		recipeMu.Lock()
		automationRecipes["amztoolsconsultant"] = prev
		recipeMu.Unlock()
	})
	cred := insertAutomationCredential(t, s, "portal-secret")
	if _, err := s.db.Exec(`UPDATE automation_credentials SET provider='amztoolsconsultant' WHERE id=?`, cred); err != nil {
		t.Fatal(err)
	}
	if err := s.replaceAutomationStorage(cred, `[{"name":"PHPSESSID","value":"dead","domain":".amztoolsconsultant.com","path":"/"}]`); err != nil {
		t.Fatal(err)
	}
	site := insertMarkhorWebsite(t, s, "Keepa", "amzckeepa.example")
	if _, err := s.db.Exec(`UPDATE websites SET upstream_url=? WHERE id=?`, "https://app03.browseanywhere.com", site); err != nil {
		t.Fatal(err)
	}
	toolID := insertAutomationJob(t, s, "Amz Keepa", cred, site, 0)
	if _, err := s.db.Exec(`UPDATE automation_jobs SET provider='amztoolsconsultant', run_requested=1 WHERE id=?`, toolID); err != nil {
		t.Fatal(err)
	}
	s.runAutomationJob(context.Background(), toolID)
	if recipe.loginCount() != 1 {
		t.Fatalf("keepa logins %d", recipe.loginCount())
	}
	var reason string
	if err := s.db.QueryRow(`SELECT last_reason FROM automation_jobs WHERE id=?`, toolID).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reason, "wrote cookie to") {
		t.Fatalf("keepa reason %q", reason)
	}
	if _, err := s.db.Exec(`UPDATE automation_jobs SET run_requested=1 WHERE id=?`, toolID); err != nil {
		t.Fatal(err)
	}
	s.runAutomationJob(context.Background(), toolID)
	if recipe.loginCount() != 1 {
		t.Fatalf("stored cookie logged in again %d", recipe.loginCount())
	}
}

func TestAmzHomepageCookieLandsOnDiamond(t *testing.T) {
	s, _ := openAutomationTestDB(t)
	id := insertMarkhorWebsite(t, s, "Helium10 Diamond", "amzcheliumdia.example")
	if _, err := s.db.Exec(`UPDATE websites SET upstream_url=? WHERE id=?`, "https://ht01.amztoolsconsultant.com", id); err != nil {
		t.Fatal(err)
	}
	wrote, err := s.writeAmzDiamondCookie([]cookieJSON{{Name: "PHPSESSID", Value: "portal-session", Path: "/"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(wrote, "Helium10 Diamond") {
		t.Fatalf("wrote %q", wrote)
	}
	var cookie string
	if err := s.db.QueryRow(`SELECT cookie FROM accounts WHERE website_id=?`, id).Scan(&cookie); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cookie, "portal-session") || !strings.Contains(cookie, ".amztoolsconsultant.com") {
		t.Fatalf("diamond cookie %s", cookie)
	}
}

func TestOwnToolJobDoesNotNeedUpstream(t *testing.T) {
	s, _ := openAutomationTestDB(t)
	recipe := &scriptedRecipe{liveValue: "live-session-value", portalValue: "portal-session-value", toolValue: "tool-session-value"}
	useAutomationRecipe(t, recipe)
	cred := insertAutomationCredential(t, s, "portal-secret")
	if err := s.replaceAutomationStorage(cred, `[{"name":"session","value":"live-session-value","domain":"markhorseotool.com","path":"/"}]`); err != nil {
		t.Fatal(err)
	}
	res, err := s.db.Exec(`INSERT INTO tools (name, category, limits_json) VALUES ('ZikAnalytics', 'own', '[]')`)
	if err != nil {
		t.Fatal(err)
	}
	toolID, _ := res.LastInsertId()
	res, err = s.db.Exec(`INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url)
		VALUES (?, 'Zik', 'zik.example', 'secret', 30, '{}', 1, '', '')`, toolID)
	if err != nil {
		t.Fatal(err)
	}
	websiteID, _ := res.LastInsertId()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err = s.db.Exec(`INSERT INTO accounts (website_id, name, cookie, user_agent, proxy, status, show_limit, description, automation_task_uid, automation_ingest_key, cookie_updated_at, last_used_at)
		VALUES (?, 'main', '', '', '', 'active', 1, '', '', '', ?, '')`, websiteID, now)
	if err != nil {
		t.Fatal(err)
	}
	accountID, _ := res.LastInsertId()
	jobID := insertAutomationJob(t, s, "Zik", cred, int(websiteID), int(accountID))
	s.runAutomationJob(context.Background(), jobID)
	var reason, cookie string
	if err := s.db.QueryRow(`SELECT last_reason FROM automation_jobs WHERE id=?`, jobID).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(reason, "upstream") {
		t.Fatalf("own tool required an upstream: %s", reason)
	}
	if err := s.db.QueryRow(`SELECT cookie FROM accounts WHERE id=?`, accountID).Scan(&cookie); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cookie, "tool-session-value") {
		t.Fatalf("cookie %s reason %s", cookie, reason)
	}
}

func TestAhrefsCopyWritesTheOtherOwnWebsite(t *testing.T) {
	s, _ := openAutomationTestDB(t)
	res, err := s.db.Exec(`INSERT INTO tools (name, category, limits_json) VALUES ('Ahrefs', 'own', '[]')`)
	if err != nil {
		t.Fatal(err)
	}
	toolID, _ := res.LastInsertId()
	insert := func(name, domain string) (int, int) {
		t.Helper()
		res, err := s.db.Exec(`INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url)
			VALUES (?, ?, ?, 'secret', 30, '{}', 1, '', '')`, toolID, name, domain)
		if err != nil {
			t.Fatal(err)
		}
		websiteID, _ := res.LastInsertId()
		now := time.Now().UTC().Format(time.RFC3339)
		res, err = s.db.Exec(`INSERT INTO accounts (website_id, name, cookie, user_agent, proxy, status, show_limit, description, automation_task_uid, automation_ingest_key, cookie_updated_at, last_used_at)
			VALUES (?, 'main', 'old', 'Mozilla/5.0 AhrefsTest', '', 'active', 1, '', '', '', ?, '')`, websiteID, now)
		if err != nil {
			t.Fatal(err)
		}
		accountID, _ := res.LastInsertId()
		return int(websiteID), int(accountID)
	}
	firstSite, _ := insert("Ahrefs one", "refs.example")
	secondSite, secondAccount := insert("Ahrefs two", "refs2.example")
	cred := insertAutomationCredential(t, s, "unused")
	jobID := insertAutomationJob(t, s, "Ahrefs", cred, firstSite, 0)
	s.saveAutomationWakeSites(jobID, firstSite, []int{secondSite})
	target, ok := s.loadAutomationSite(firstSite)
	if !ok {
		t.Fatal("target missing")
	}
	message := s.writeAhrefsCopies(automationJobRow{ID: jobID}, target, `[{"name":"BSSESSID","value":"fresh"}]`, "ahrefs.com")
	if !strings.Contains(message, "Ahrefs two") {
		t.Fatalf("message %q", message)
	}
	var cookie, agent string
	if err := s.db.QueryRow(`SELECT cookie, COALESCE(user_agent, '') FROM accounts WHERE id=?`, secondAccount).Scan(&cookie, &agent); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cookie, "BSSESSID") || agent != "Mozilla/5.0 AhrefsTest" {
		t.Fatalf("copied cookie %s agent %s", cookie, agent)
	}
	if got := s.readAccountUserAgent(secondAccount); got != "Mozilla/5.0 AhrefsTest" {
		t.Fatalf("mapped user agent %q", got)
	}
	accountCookies := s.readAccountCookies(secondAccount)
	if !cookieNamed(accountCookies, "BSSESSID") {
		t.Fatal("mapped account cookie was not readable")
	}
}

func TestBulkAutomationJobs(t *testing.T) {
	s, mux := openAutomationTestDB(t)
	expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`INSERT INTO operators (username, password_hash, role, status) VALUES ('seller', 'x', 'reseller', 'active')`)
	if err != nil {
		t.Fatal(err)
	}
	sellerID, _ := res.LastInsertId()
	if _, err := s.db.Exec(`INSERT INTO admin_sessions (token, operator_id, expires_at) VALUES ('reseller-bulk', ?, ?)`, sellerID, expires); err != nil {
		t.Fatal(err)
	}
	cred := insertAutomationCredential(t, s, "portal-secret")
	var websiteID int
	if err := s.db.QueryRow(`SELECT id FROM websites WHERE kind='recloud'`).Scan(&websiteID); err != nil {
		t.Fatal(err)
	}
	first := insertAutomationJob(t, s, "Job one", cred, websiteID, 0)
	second := insertAutomationJob(t, s, "Job two", cred, websiteID, 0)
	if _, err := s.db.Exec(`UPDATE automation_jobs SET enabled=0 WHERE id=?`, second); err != nil {
		t.Fatal(err)
	}

	denied := callAuth(t, mux, http.MethodPost, "/api/automation/jobs/bulk", fmt.Sprintf(`{"ids":[%d],"enabled":true}`, first), "reseller-bulk")
	if denied.Code != 403 {
		t.Fatalf("reseller status %d body %s", denied.Code, denied.Body.String())
	}
	empty := callAuth(t, mux, http.MethodPost, "/api/automation/jobs/bulk", `{"ids":[]}`, "recloud-test")
	if empty.Code != 400 {
		t.Fatalf("empty status %d body %s", empty.Code, empty.Body.String())
	}
	badInterval := callAuth(t, mux, http.MethodPost, "/api/automation/jobs/bulk", fmt.Sprintf(`{"ids":[%d],"interval_minutes":3}`, first), "recloud-test")
	if badInterval.Code != 400 {
		t.Fatalf("interval status %d body %s", badInterval.Code, badInterval.Body.String())
	}

	enable := callAuth(t, mux, http.MethodPost, "/api/automation/jobs/bulk", fmt.Sprintf(`{"ids":[%d,%d],"enabled":true}`, first, second), "recloud-test")
	if enable.Code != 200 {
		t.Fatalf("enable status %d body %s", enable.Code, enable.Body.String())
	}
	interval := callAuth(t, mux, http.MethodPost, "/api/automation/jobs/bulk", fmt.Sprintf(`{"ids":[%d,%d],"interval_minutes":23}`, first, second), "recloud-test")
	if interval.Code != 200 {
		t.Fatalf("set interval status %d body %s", interval.Code, interval.Body.String())
	}
	disable := callAuth(t, mux, http.MethodPost, "/api/automation/jobs/bulk", fmt.Sprintf(`{"ids":[%d,%d],"enabled":false}`, first, second), "recloud-test")
	if disable.Code != 200 {
		t.Fatalf("disable status %d body %s", disable.Code, disable.Body.String())
	}

	var enabled, minutes int
	if err := s.db.QueryRow(`SELECT enabled, interval_minutes FROM automation_jobs WHERE id=?`, first).Scan(&enabled, &minutes); err != nil {
		t.Fatal(err)
	}
	if enabled != 0 || minutes != 23 {
		t.Fatalf("first enabled=%d interval=%d", enabled, minutes)
	}
	if err := s.db.QueryRow(`SELECT enabled, interval_minutes FROM automation_jobs WHERE id=?`, second).Scan(&enabled, &minutes); err != nil {
		t.Fatal(err)
	}
	if enabled != 0 || minutes != 23 {
		t.Fatalf("second enabled=%d interval=%d", enabled, minutes)
	}
}
