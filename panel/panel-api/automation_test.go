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
