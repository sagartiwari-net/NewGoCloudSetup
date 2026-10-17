package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"panel-api/automation"
	"panel-api/automation/ahrefs"
	"panel-api/automation/amz"
	"panel-api/automation/azad"
	"panel-api/automation/erank"
	"panel-api/automation/markhor"
	"panel-api/automation/toolbaazar"
	"panel-api/automation/zik"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const automationJobTimeout = 120 * time.Second

// cookieJSON stays as the name this file already uses. The type lives in the
// shared automation package so each portal folder can use it.
type cookieJSON = automation.Cookie

var (
	errAutomationLoginPage = automation.ErrLoginPage
	errAutomationRejected  = automation.ErrRejected
	errAutomationNoCookie  = automation.ErrNoCookie
	errAutomationDecrypt   = automation.ErrDecrypt
	errAutomationCSRF      = automation.ErrCSRF
	errAutomationTimeout   = automation.ErrTimeout
	errAutomationDenied    = automation.ErrDenied
	errAutomationUpstream  = automation.ErrUpstream
	errAutomationHomepage  = errors.New("enable the homepage job first")
)

func cookiesToJSON(cookies []cookieJSON) (string, error) { return automation.CookiesToJSON(cookies) }
func parseStoredCookies(raw string) []cookieJSON         { return automation.ParseStoredCookies(raw) }
func cookieValues(cookies []cookieJSON) []string         { return automation.CookieValues(cookies) }

var (
	recipeMu          sync.Mutex
	automationRecipes = map[string]automation.Recipe{
		"markhor":            markhor.Recipe{},
		"azad":               azad.Recipe{},
		"toolbaazar":         toolbaazar.Recipe{},
		"amztoolsconsultant": amz.Recipe{},
		"ahrefs":             ahrefs.Recipe{},
		"zikaanalytics":      zik.Recipe{},
		"erank":              erank.Recipe{},
	}
	automationProviderNames = map[string]string{
		"markhor": "Markhor", "azad": "Azad", "toolbaazar": "Toolbaazar",
		"amztoolsconsultant": "Amz Tools Consultant",
		"ahrefs":             "Ahrefs",
		"zikaanalytics":      "ZikAnalytics",
		"erank":              "eRank",
	}
	automationCredLocks sync.Map
	automationKeyWarn   sync.Once
	automationMySQLIDs  sync.Once
)

func recipeFor(id string) (automation.Recipe, bool) {
	recipeMu.Lock()
	defer recipeMu.Unlock()
	recipe, ok := automationRecipes[strings.TrimSpace(id)]
	return recipe, ok
}

func knownAutomationProvider(id string) bool {
	_, ok := recipeFor(id)
	return ok
}

func automationCredLock(id int) *sync.Mutex {
	value, _ := automationCredLocks.LoadOrStore(id, &sync.Mutex{})
	return value.(*sync.Mutex)
}

func automationKey() []byte {
	raw := strings.TrimSpace(os.Getenv("GT4RENTS_AUTOMATION_KEY"))
	if raw == "" {
		if strings.TrimSpace(os.Getenv("GT4RENTS_MYSQL_PASSWORD")) != "" {
			automationKeyWarn.Do(func() {
				log.Printf("[automation] GT4RENTS_AUTOMATION_KEY is empty; using the built-in key")
			})
		}
		raw = "panel-automation-sqlite-test-key"
	} else {
		automationKeyWarn.Do(func() {
			log.Printf("[automation] GT4RENTS_AUTOMATION_KEY is set")
		})
	}
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

func encryptAutomation(plain string) (string, error) {
	block, err := aes.NewCipher(automationKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plain), nil)), nil
}

func decryptAutomation(enc string) (string, error) {
	plain, err := openAutomationSecret(enc, automationKey())
	if err == nil {
		return plain, nil
	}
	if strings.TrimSpace(os.Getenv("GT4RENTS_AUTOMATION_KEY")) == "" {
		return "", err
	}
	sum := sha256.Sum256([]byte("panel-automation-sqlite-test-key"))
	plain, fallbackErr := openAutomationSecret(enc, sum[:])
	if fallbackErr != nil {
		return "", err
	}
	log.Printf("[automation] credential opened with the previous key")
	return plain, nil
}

func openAutomationSecret(enc string, key []byte) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(enc))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("short secret")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (s *server) ensureAutomationTables() {
	if s != nil {
		ensureAutomationTables(s.db)
	}
}

func ensureAutomationTables(db *sql.DB) {
	if db == nil {
		return
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS automation_credentials (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			provider TEXT NOT NULL,
			username TEXT NOT NULL,
			password_enc TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS automation_storage (
			credential_id INTEGER PRIMARY KEY,
			cookies_json TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			user_agent TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS automation_jobs (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			provider TEXT NOT NULL,
			credential_id INTEGER NOT NULL,
			target_website_id INTEGER NOT NULL,
			target_account_id INTEGER NOT NULL DEFAULT 0,
			interval_minutes INTEGER NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 0,
			run_requested INTEGER NOT NULL DEFAULT 0,
			next_run_at TEXT NOT NULL,
			last_run_at TEXT NOT NULL DEFAULT '',
			last_result TEXT NOT NULL DEFAULT '',
			last_reason TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS automation_logs (
			id INTEGER PRIMARY KEY,
			job_id INTEGER NOT NULL,
			job_name TEXT NOT NULL,
			credential_id INTEGER NOT NULL,
			credential_name TEXT NOT NULL,
			level TEXT NOT NULL,
			message TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_automation_logs_created ON automation_logs(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_automation_jobs_due ON automation_jobs(enabled, next_run_at)`,
		`CREATE TABLE IF NOT EXISTS automation_job_sites (
			job_id INTEGER NOT NULL,
			website_id INTEGER NOT NULL,
			PRIMARY KEY (job_id, website_id)
		)`,
		`CREATE TABLE IF NOT EXISTS automation_signals (
			id INTEGER PRIMARY KEY,
			website_id INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			state TEXT NOT NULL
		)`,
	}
	for _, statement := range statements {
		_, _ = db.Exec(statement)
	}
	for _, statement := range []string{
		`ALTER TABLE automation_jobs ADD COLUMN last_trigger_at VARCHAR(40) NOT NULL DEFAULT ''`,
		`ALTER TABLE automation_jobs ADD COLUMN queued_cause VARCHAR(16) NOT NULL DEFAULT ''`,
		`ALTER TABLE automation_jobs ADD COLUMN running INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE automation_jobs ADD COLUMN run_cause VARCHAR(16) NOT NULL DEFAULT ''`,
		`ALTER TABLE automation_jobs ADD COLUMN wake_website_id INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE automation_jobs ADD COLUMN job_kind VARCHAR(16) NOT NULL DEFAULT 'tool'`,
		`ALTER TABLE automation_storage ADD COLUMN user_agent VARCHAR(2048) NOT NULL DEFAULT ''`,
	} {
		if _, err := db.Exec(statement); err != nil {
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "duplicate column") || strings.Contains(msg, "already exists") {
				continue
			}
			log.Printf("[automation] column update failed: %v", err)
		}
	}
	if strings.TrimSpace(os.Getenv("GT4RENTS_MYSQL_PASSWORD")) == "" {
		return
	}
	automationMySQLIDs.Do(func() {
		for _, table := range []string{"automation_credentials", "automation_jobs", "automation_logs", "automation_signals"} {
			_, _ = db.Exec(`ALTER TABLE ` + table + ` MODIFY COLUMN id INTEGER NOT NULL AUTO_INCREMENT`)
		}
	})
}

func automationPublicReason(err error) string {
	var page *automation.PageError
	if errors.As(err, &page) {
		return page.Public()
	}
	switch {
	case err == nil:
		return ""
	case errors.Is(err, errAutomationLoginPage):
		return errAutomationLoginPage.Error()
	case errors.Is(err, errAutomationRejected):
		return errAutomationRejected.Error()
	case errors.Is(err, errAutomationNoCookie):
		return errAutomationNoCookie.Error()
	case errors.Is(err, errAutomationDecrypt):
		return errAutomationDecrypt.Error()
	case errors.Is(err, errAutomationCSRF):
		return errAutomationCSRF.Error()
	case errors.Is(err, errAutomationTimeout):
		return errAutomationTimeout.Error()
	case errors.Is(err, errAutomationDenied):
		return errAutomationDenied.Error()
	case errors.Is(err, errAutomationUpstream):
		return errAutomationUpstream.Error()
	case errors.Is(err, errAutomationHomepage):
		return errAutomationHomepage.Error()
	case errors.Is(err, automation.ErrStoredCookie):
		return automation.ErrStoredCookie.Error()
	case errors.Is(err, automation.ErrCookieRejected):
		return automation.ErrCookieRejected.Error()
	case errors.Is(err, automation.ErrUserAgent):
		return automation.ErrUserAgent.Error()
	default:
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline") || strings.Contains(msg, "timed out") {
			return errAutomationTimeout.Error()
		}
		return errAutomationLoginPage.Error()
	}
}

func scrubAutomation(message string, secrets []string) string {
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if len(secret) < 4 {
			continue
		}
		message = strings.ReplaceAll(message, secret, "")
	}
	message = strings.Join(strings.Fields(message), " ")
	if len(message) > 240 {
		message = message[:240]
	}
	return strings.TrimSpace(message)
}

type automationJobRow struct {
	ID              int
	Name            string
	Provider        string
	CredentialID    int
	CredentialName  string
	TargetWebsiteID int
	TargetAccountID int
	IntervalMinutes int
	Enabled         int
	Kind            string
}

type automationSite struct {
	ID       int
	ToolID   int
	Name     string
	Domain   string
	Kind     string
	Upstream string
}

func (s *server) pollAutomation() {
	for {
		s.drainAutomationSignals()
		s.runDueAutomationJobs()
		s.startQueuedAutomation()
		time.Sleep(2 * time.Second)
	}
}

func (s *server) runDueAutomationJobs() {
	s.ensureAutomationTables()
	now := time.Now().UTC().Format(time.RFC3339)
	rows, err := s.db.Query(`SELECT id, credential_id FROM automation_jobs
		WHERE enabled=1 AND (run_requested=1 OR (interval_minutes>0 AND next_run_at<=?) OR (interval_minutes>0 AND job_kind='session' AND TRIM(COALESCE(last_run_at,''))=''))
		ORDER BY credential_id, CASE WHEN job_kind='session' THEN 0 ELSE 1 END, id`, now)
	if err != nil {
		return
	}
	groups := map[int][]int{}
	var order []int
	for rows.Next() {
		var id, cred int
		if rows.Scan(&id, &cred) != nil {
			continue
		}
		if _, ok := groups[cred]; !ok {
			order = append(order, cred)
		}
		groups[cred] = append(groups[cred], id)
	}
	rows.Close()
	var wg sync.WaitGroup
	for _, cred := range order {
		wg.Add(1)
		ids := groups[cred]
		go func(credentialID int, jobIDs []int) {
			defer wg.Done()
			lock := automationCredLock(credentialID)
			lock.Lock()
			defer lock.Unlock()
			for _, id := range jobIDs {
				ctx, cancel := context.WithTimeout(context.Background(), automationJobTimeout)
				s.runAutomationJob(ctx, id)
				cancel()
			}
		}(cred, ids)
	}
	wg.Wait()
}

func (s *server) runFreshSessionJob(ctx context.Context, credentialID int) {
	var id int
	var lastRun string
	err := s.db.QueryRow(`SELECT id, COALESCE(last_run_at,'') FROM automation_jobs
		WHERE credential_id=? AND job_kind='session' AND enabled=1 AND interval_minutes>0
		ORDER BY id LIMIT 1`, credentialID).Scan(&id, &lastRun)
	if err != nil || id <= 0 || strings.TrimSpace(lastRun) != "" {
		return
	}
	s.runAutomationJob(ctx, id)
}

func (s *server) runAutomationJob(ctx context.Context, id int) {
	job, ok := s.loadAutomationJob(id)
	if !ok || job.Enabled != 1 {
		return
	}
	if job.Kind != "session" {
		s.runFreshSessionJob(ctx, job.CredentialID)
	}
	if !s.claimAutomationJob(job) {
		return
	}
	log.Printf("[automation] job %d %q start provider=%s kind=%s", job.ID, job.Name, job.Provider, job.Kind)
	var wakeWebsiteID int
	_ = s.db.QueryRow(`SELECT wake_website_id FROM automation_jobs WHERE id=?`, job.ID).Scan(&wakeWebsiteID)
	var site automationSite
	if job.Kind != "session" {
		var siteOK bool
		site, siteOK = s.loadAutomationSite(job.TargetWebsiteID)
		if !siteOK || (site.Kind == "recloud" && strings.TrimSpace(site.Upstream) == "") {
			s.failAutomation(job, errAutomationUpstream, nil)
			return
		}
	}
	var username, passwordEnc string
	err := s.db.QueryRow(`SELECT username, password_enc FROM automation_credentials WHERE id=?`, job.CredentialID).Scan(&username, &passwordEnc)
	if err != nil {
		log.Printf("[automation] job %d credential row missing", job.ID)
		s.failAutomation(job, errAutomationLoginPage, nil)
		return
	}
	password := ""
	if strings.TrimSpace(passwordEnc) != "" {
		password, err = decryptAutomation(passwordEnc)
		if err != nil {
			log.Printf("[automation] job %d credential could not be decrypted", job.ID)
			s.failAutomation(job, errAutomationDecrypt, nil)
			return
		}
	}
	if strings.TrimSpace(password) == "" && job.Provider != "ahrefs" {
		log.Printf("[automation] job %d credential could not be decrypted", job.ID)
		s.failAutomation(job, errAutomationDecrypt, nil)
		return
	}
	secrets := []string{}
	if password != "" {
		secrets = append(secrets, password)
	}
	recipe, ok := recipeFor(job.Provider)
	if !ok {
		s.failAutomation(job, errAutomationLoginPage, secrets)
		return
	}
	userAgent := ""
	accountID := job.TargetAccountID
	if job.Provider == "ahrefs" && job.Kind != "session" {
		if accountID <= 0 {
			accountID = s.firstAutomationAccount(site.ID)
		}
		userAgent = s.readAccountUserAgent(accountID)
		if userAgent == "" {
			s.failAutomation(job, automation.ErrUserAgent, secrets)
			return
		}
	}
	ctx = automation.WithUserAgent(ctx, userAgent)
	stored := s.readAutomationCookies(job.CredentialID)
	if job.Provider == "ahrefs" && !cookieNamed(stored, "BSSESSID") {
		if fromAccount := s.readAccountCookies(accountID); cookieNamed(fromAccount, "BSSESSID") {
			stored = fromAccount
		}
	}
	secrets = append(secrets, cookieValues(stored)...)
	log.Printf("[automation] job %d stored cookies %d [%s]", job.ID, len(stored), automation.Names(stored))
	portal := stored
	if len(stored) > 0 {
		valid, updated, checkErr := recipe.Check(ctx, stored)
		if checkErr != nil {
			log.Printf("[automation] job %d session check failed: %s", job.ID, automationPublicReason(checkErr))
			s.failAutomation(job, checkErr, secrets)
			return
		}
		if valid {
			if len(updated) == 0 {
				updated = stored
			}
			if err := s.saveAutomationCookies(job.CredentialID, updated); err != nil {
				s.failAutomation(job, errAutomationLoginPage, secrets)
				return
			}
			s.writeAutomationLog(job, "info", "session still valid", secrets)
			log.Printf("[automation] job %d session still valid", job.ID)
			portal = updated
		} else if portal, err = s.loginForAutomationJob(ctx, recipe, job, username, password, secrets); err != nil {
			s.failAutomation(job, err, append(secrets, cookieValues(portal)...))
			return
		}
	} else if portal, err = s.loginForAutomationJob(ctx, recipe, job, username, password, secrets); err != nil {
		s.failAutomation(job, err, secrets)
		return
	}
	secrets = append(secrets, cookieValues(portal)...)
	if job.Kind == "session" {
		s.writeAutomationLog(job, "info", "stored homepage cookie", secrets)
		s.finishAutomation(job, "ok", "stored homepage cookie")
		return
	}
	if strings.TrimSpace(site.Upstream) == "" {
		site.Upstream = "https://" + strings.TrimSpace(recipe.PortalHost())
	}
	log.Printf("[automation] job %d capture %s", job.ID, automation.Where(site.Upstream))
	tool, err := recipe.Capture(ctx, portal, site.Upstream)
	if err != nil {
		s.failAutomation(job, err, secrets)
		return
	}
	if len(tool) == 0 {
		s.failAutomation(job, errAutomationLoginPage, secrets)
		return
	}
	raw, err := automationAccountBody(recipe, tool)
	if err != nil || strings.TrimSpace(raw) == "" {
		s.failAutomation(job, errAutomationLoginPage, append(secrets, cookieValues(tool)...))
		return
	}
	accountID, err = s.writeAutomationAccountCookie(site, accountID, raw)
	if err != nil {
		s.failAutomation(job, errAutomationLoginPage, append(secrets, raw))
		return
	}
	s.shareCookieFrom(accountID)
	message := "wrote cookie to " + strings.TrimSpace(site.Name)
	s.writeAutomationLog(job, "info", message, append(secrets, raw))
	if job.Provider == "ahrefs" {
		if extra := s.writeAhrefsCopies(job, site, raw, recipe.PortalHost()); extra != "" {
			message = message + "; " + extra
		}
	}
	if wakeWebsiteID > 0 && wakeWebsiteID != site.ID {
		if wakeMessage := s.captureAutomationWake(ctx, recipe, job, portal, wakeWebsiteID, secrets); wakeMessage != "" {
			message = message + "; " + wakeMessage
		}
	}
	s.finishAutomation(job, "ok", message)
}

func automationAccountBody(recipe automation.Recipe, cookies []cookieJSON) (string, error) {
	type documenter interface {
		Document(cookies []automation.Cookie) (string, error)
	}
	if doc, ok := recipe.(documenter); ok {
		return doc.Document(cookies)
	}
	return cookiesToJSON(cookies)
}

func (s *server) writeAhrefsCopies(job automationJobRow, target automationSite, raw, portal string) string {
	names := []string{}
	for _, id := range s.automationWakeIDs(job.ID) {
		if id == target.ID {
			continue
		}
		site, ok := s.loadAutomationSite(id)
		if !ok {
			continue
		}
		sameTool := target.ToolID > 0 && site.ToolID == target.ToolID
		if !sameTool && !automationHostMatches(upstreamHost(site.Upstream), portal) {
			continue
		}
		accountID := 0
		if site.Kind != "recloud" {
			accountID = s.firstAutomationAccount(site.ID)
			if accountID <= 0 {
				continue
			}
		}
		accountID, err := s.writeAutomationAccountCookie(site, accountID, raw)
		if err != nil {
			continue
		}
		s.shareCookieFrom(accountID)
		names = append(names, strings.TrimSpace(site.Name))
	}
	if len(names) == 0 {
		return ""
	}
	return "wrote cookie to " + strings.Join(names, ", ")
}

func upstreamHost(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func (s *server) loginForAutomationJob(ctx context.Context, recipe automation.Recipe, job automationJobRow, username, password string, secrets []string) ([]cookieJSON, error) {
	if job.Kind != "session" && job.Provider == "amztoolsconsultant" && !s.amzKeepaJob(job) {
		log.Printf("[automation] job %d cookie expired, running the homepage job", job.ID)
		if err := s.runAmzHomepage(ctx, job.CredentialID); err != nil {
			return nil, err
		}
		s.writeAutomationLog(job, "info", "homepage logged in", secrets)
		portal := s.readAutomationCookies(job.CredentialID)
		valid, updated, err := recipe.Check(ctx, portal)
		if err != nil {
			return nil, err
		}
		if !valid {
			return nil, errAutomationLoginPage
		}
		if len(updated) > 0 {
			portal = updated
		}
		return portal, nil
	}
	log.Printf("[automation] job %d logging in", job.ID)
	return s.automationLogin(ctx, recipe, job, username, password, secrets)
}

func (s *server) amzKeepaJob(job automationJobRow) bool {
	site, ok := s.loadAutomationSite(job.TargetWebsiteID)
	if !ok {
		return false
	}
	return strings.Contains(strings.ToLower(site.Upstream), "browseanywhere.com")
}

func (s *server) runAmzHomepage(ctx context.Context, credentialID int) error {
	var id int
	var before string
	err := s.db.QueryRow(`SELECT id, COALESCE(last_run_at,'') FROM automation_jobs
		WHERE credential_id=? AND job_kind='session' AND enabled=1 ORDER BY id LIMIT 1`, credentialID).Scan(&id, &before)
	if err != nil || id <= 0 {
		return errAutomationHomepage
	}
	if _, err := s.db.Exec(`UPDATE automation_jobs SET run_requested=1 WHERE id=? AND running=0`, id); err != nil {
		return errAutomationHomepage
	}
	s.runAutomationJob(ctx, id)
	var after, result string
	if err := s.db.QueryRow(`SELECT COALESCE(last_run_at,''), COALESCE(last_result,'') FROM automation_jobs WHERE id=?`, id).Scan(&after, &result); err != nil {
		return errAutomationLoginPage
	}
	if after == before || result != "ok" {
		return errAutomationLoginPage
	}
	return nil
}

func (s *server) automationLogin(ctx context.Context, recipe automation.Recipe, job automationJobRow, username, password string, secrets []string) ([]cookieJSON, error) {
	cookies, err := recipe.Login(ctx, username, password)
	if err != nil {
		return nil, err
	}
	if len(cookies) == 0 {
		return nil, errAutomationLoginPage
	}
	if err := s.saveAutomationCookies(job.CredentialID, cookies); err != nil {
		return nil, errAutomationLoginPage
	}
	s.writeAutomationLog(job, "info", "logged in", append(secrets, cookieValues(cookies)...))
	log.Printf("[automation] job %d logged in [%s]", job.ID, automation.Names(cookies))
	return cookies, nil
}

func automationKeptInterval(minutes int) int {
	if minutes == 0 {
		return 0
	}
	if minutes < 5 {
		return 5
	}
	if minutes > 1440 {
		return 1440
	}
	return minutes
}

func (s *server) automationRunInterval(job automationJobRow) int {
	current := job.IntervalMinutes
	_ = s.db.QueryRow(`SELECT interval_minutes FROM automation_jobs WHERE id=?`, job.ID).Scan(&current)
	return automationKeptInterval(current)
}

func automationNextRun(lastRun string, interval int, now time.Time) time.Time {
	if interval == 0 {
		return now
	}
	if interval < 5 {
		interval = 5
	}
	fromSave := now.Add(time.Duration(interval) * time.Minute)
	last, err := time.Parse(time.RFC3339, strings.TrimSpace(lastRun))
	if err != nil {
		return fromSave
	}
	fromLast := last.Add(time.Duration(interval) * time.Minute)
	if fromLast.Before(now) {
		return fromSave
	}
	return fromLast
}

func (s *server) claimAutomationJob(job automationJobRow) bool {
	now := time.Now().UTC()
	interval := automationKeptInterval(job.IntervalMinutes)
	next := now.Add(time.Duration(interval) * time.Minute).Format(time.RFC3339)
	var queued string
	var requested, running int
	if err := s.db.QueryRow(`SELECT queued_cause, run_requested, running FROM automation_jobs WHERE id=?`, job.ID).Scan(&queued, &requested, &running); err != nil || running == 1 {
		return false
	}
	cause := "schedule"
	if queued == "trigger" || queued == "manual" {
		cause = queued
	} else if requested == 1 {
		cause = "manual"
	}
	res, err := s.db.Exec(`UPDATE automation_jobs SET run_requested=0, queued_cause='', running=1, run_cause=?, next_run_at=?, updated_at=?
		WHERE id=? AND enabled=1 AND running=0 AND (run_requested=1 OR queued_cause<>'' OR (interval_minutes>0 AND next_run_at<=?) OR (interval_minutes>0 AND job_kind='session' AND TRIM(COALESCE(last_run_at,''))=''))`,
		cause, next, now.Format(time.RFC3339), job.ID, now.Format(time.RFC3339))
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n == 1
}

func (s *server) finishAutomation(job automationJobRow, result, reason string) {
	reason = scrubAutomation(reason, nil)
	if reason == "" {
		reason = "session still valid"
	}
	now := time.Now().UTC()
	interval := s.automationRunInterval(job)
	next := now.Add(time.Duration(interval) * time.Minute).Format(time.RFC3339)
	stamp := now.Format(time.RFC3339)
	triggerAt := automationTriggerStamp(s.db, job.ID, stamp)
	_, _ = s.db.Exec(`UPDATE automation_jobs SET last_run_at=?, last_result=?, last_reason=?, next_run_at=?, run_requested=0, running=0, run_cause='', wake_website_id=0, last_trigger_at=?, updated_at=? WHERE id=?`,
		stamp, result, reason, next, triggerAt, stamp, job.ID)
	log.Printf("[automation] job %d %s: %s", job.ID, result, reason)
}

func (s *server) failAutomation(job automationJobRow, err error, secrets []string) {
	reason := scrubAutomation(automationPublicReason(err), secrets)
	if reason == "" {
		reason = automationPublicReason(err)
	}
	s.writeAutomationLog(job, "error", reason, secrets)
	now := time.Now().UTC()
	interval := s.automationRunInterval(job)
	next := now.Add(time.Duration(interval) * time.Minute).Format(time.RFC3339)
	stamp := now.Format(time.RFC3339)
	triggerAt := automationTriggerStamp(s.db, job.ID, stamp)
	_, _ = s.db.Exec(`UPDATE automation_jobs SET last_run_at=?, last_result='failed', last_reason=?, next_run_at=?, run_requested=0, running=0, run_cause='', wake_website_id=0, last_trigger_at=?, updated_at=? WHERE id=?`,
		stamp, reason, next, triggerAt, stamp, job.ID)
	log.Printf("[automation] job %d failed: %s", job.ID, reason)
}

func (s *server) writeAutomationLog(job automationJobRow, level, message string, secrets []string) {
	message = scrubAutomation(message, secrets)
	if message == "" {
		if level == "error" {
			message = errAutomationLoginPage.Error()
		} else {
			message = "session still valid"
		}
	}
	if level != "info" && level != "error" {
		level = "info"
	}
	_, _ = s.db.Exec(`INSERT INTO automation_logs (job_id, job_name, credential_id, credential_name, level, message, created_at)
		VALUES (?,?,?,?,?,?,?)`,
		job.ID, job.Name, job.CredentialID, job.CredentialName, level, message, time.Now().UTC().Format(time.RFC3339))
}

func (s *server) loadAutomationJob(id int) (automationJobRow, bool) {
	var job automationJobRow
	err := s.db.QueryRow(`SELECT j.id, j.name, j.provider, j.credential_id, COALESCE(c.name, ''),
		j.target_website_id, j.target_account_id, j.interval_minutes, j.enabled, COALESCE(j.job_kind, 'tool')
		FROM automation_jobs j LEFT JOIN automation_credentials c ON c.id=j.credential_id
		WHERE j.id=?`, id).Scan(&job.ID, &job.Name, &job.Provider, &job.CredentialID, &job.CredentialName,
		&job.TargetWebsiteID, &job.TargetAccountID, &job.IntervalMinutes, &job.Enabled, &job.Kind)
	return job, err == nil
}

func (s *server) loadAutomationSite(id int) (automationSite, bool) {
	var site automationSite
	err := s.db.QueryRow(`SELECT id, tool_id, name, domain, COALESCE(kind, ''), COALESCE(upstream_url, '') FROM websites WHERE id=?`, id).
		Scan(&site.ID, &site.ToolID, &site.Name, &site.Domain, &site.Kind, &site.Upstream)
	return site, err == nil
}

func (s *server) firstAutomationAccount(websiteID int) int {
	var id int
	if err := s.db.QueryRow(`SELECT id FROM accounts WHERE website_id=? ORDER BY id ASC LIMIT 1`, websiteID).Scan(&id); err != nil {
		return 0
	}
	return id
}

func (s *server) readAccountUserAgent(accountID int) string {
	if accountID <= 0 {
		return ""
	}
	var ua string
	err := s.db.QueryRow(`SELECT CASE WHEN TRIM(COALESCE(a.user_agent, '')) != '' THEN a.user_agent ELSE COALESCE(ua.user_agent, '') END
		FROM accounts a LEFT JOIN user_agents ua ON ua.id=a.user_agent_id WHERE a.id=?`, accountID).Scan(&ua)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(ua)
}

func cookieNamed(cookies []cookieJSON, name string) bool {
	for _, cookie := range cookies {
		if strings.EqualFold(cookie.Name, name) && strings.TrimSpace(cookie.Value) != "" {
			return true
		}
	}
	return false
}

func (s *server) readAccountCookies(accountID int) []cookieJSON {
	if accountID <= 0 {
		return nil
	}
	var raw string
	if err := s.db.QueryRow(`SELECT COALESCE(cookie, '') FROM accounts WHERE id=?`, accountID).Scan(&raw); err != nil {
		return nil
	}
	return parseStoredCookies(raw)
}

func (s *server) readAutomationCookies(credentialID int) []cookieJSON {
	var raw string
	if err := s.db.QueryRow(`SELECT cookies_json FROM automation_storage WHERE credential_id=?`, credentialID).Scan(&raw); err != nil {
		return nil
	}
	return parseStoredCookies(raw)
}

func (s *server) saveAutomationCookies(credentialID int, cookies []cookieJSON) error {
	raw, err := cookiesToJSON(cookies)
	if err != nil {
		return err
	}
	return s.replaceAutomationStorage(credentialID, raw)
}

func (s *server) replaceAutomationStorage(credentialID int, raw string) error {
	var ua string
	_ = s.db.QueryRow(`SELECT COALESCE(user_agent, '') FROM automation_storage WHERE credential_id=?`, credentialID).Scan(&ua)
	return s.writeAutomationStorage(credentialID, raw, ua)
}

func (s *server) writeAutomationStorage(credentialID int, raw, userAgent string) error {
	raw = strings.TrimSpace(raw)
	userAgent = strings.TrimSpace(userAgent)
	if raw == "" {
		return errAutomationLoginPage
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM automation_storage WHERE credential_id=?`, credentialID); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO automation_storage (credential_id, cookies_json, updated_at, user_agent) VALUES (?,?,?,?)`,
		credentialID, raw, time.Now().UTC().Format(time.RFC3339), userAgent); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *server) writeAmzDiamondCookie(portal []cookieJSON) (string, error) {
	cookies := amz.HomepageCookies(portal)
	raw, err := cookiesToJSON(cookies)
	if err != nil {
		return "", err
	}
	rows, err := s.db.Query(`SELECT id FROM websites WHERE kind='recloud' AND upstream_url LIKE ?`, "https://ht01.amztoolsconsultant.com%")
	if err != nil {
		return "", err
	}
	var ids []int
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	var wrote []string
	for _, id := range ids {
		site, ok := s.loadAutomationSite(id)
		if !ok {
			continue
		}
		accountID, err := s.writeAutomationAccountCookie(site, 0, raw)
		if err != nil {
			return "", err
		}
		s.shareCookieFrom(accountID)
		wrote = append(wrote, strings.TrimSpace(site.Name))
	}
	if len(wrote) == 0 {
		return "", nil
	}
	return "wrote homepage cookie to " + strings.Join(wrote, ", "), nil
}

func (s *server) writeAutomationAccountCookie(site automationSite, accountID int, cookie string) (int, error) {
	cookie = strings.TrimSpace(cookie)
	if cookie == "" {
		return 0, errAutomationLoginPage
	}
	if site.Kind == "recloud" {
		return s.saveRecloudCookie(site.ID, cookie)
	}
	if accountID <= 0 {
		return 0, errAutomationLoginPage
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`UPDATE accounts SET cookie=?, status='active', cookie_updated_at=?, failure_count=0 WHERE id=? AND website_id=?`,
		cookie, now, accountID, site.ID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return 0, errAutomationLoginPage
	}
	return accountID, nil
}

func (s *server) automationProviders() []any {
	recipeMu.Lock()
	defer recipeMu.Unlock()
	ids := make([]string, 0, len(automationRecipes))
	for id := range automationRecipes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	items := []any{}
	for _, id := range ids {
		label := automationProviderNames[id]
		if label == "" {
			label = id
		}
		items = append(items, map[string]any{"id": id, "label": label})
	}
	return items
}

func (s *server) listAutomationCredentials(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.master(w, r); !ok {
		return
	}
	s.ensureAutomationTables()
	page, size, q := pageQuery(r)
	where := "1=1"
	args := []any{}
	if q != "" {
		where += " AND (name LIKE ? OR provider LIKE ? OR username LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM automation_credentials WHERE `+where, args...).Scan(&total); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	page = clampPage(page, size, total)
	qargs := append(append([]any{}, args...), size, (page-1)*size)
	rows, err := s.db.Query(`SELECT id, name, provider, username, updated_at, CASE WHEN TRIM(password_enc)='' THEN 0 ELSE 1 END
		FROM automation_credentials WHERE `+where+` ORDER BY name LIMIT ? OFFSET ?`, qargs...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, has int
		var name, provider, username, updated string
		if rows.Scan(&id, &name, &provider, &username, &updated, &has) != nil {
			continue
		}
		items = append(items, map[string]any{
			"id": id, "name": name, "provider": provider, "username": username,
			"has_password": has == 1, "updated_at": updated,
		})
	}
	s.writePage(w, page, size, total, items)
}

func (s *server) saveAutomationCredential(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.master(w, r); !ok {
		return
	}
	s.ensureAutomationTables()
	var body struct {
		ID       int    `json:"id"`
		Name     string `json:"name"`
		Provider string `json:"provider"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Provider = strings.TrimSpace(body.Provider)
	body.Username = strings.TrimSpace(body.Username)
	if body.Name == "" {
		writeErr(w, 400, "Enter a name")
		return
	}
	if !knownAutomationProvider(body.Provider) {
		writeErr(w, 400, "Choose a provider")
		return
	}
	if body.Username == "" {
		writeErr(w, 400, "Enter a username")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if body.ID == 0 {
		enc := ""
		if strings.TrimSpace(body.Password) == "" {
			if body.Provider != "ahrefs" {
				writeErr(w, 400, "Enter a password")
				return
			}
		} else {
			var err error
			enc, err = encryptAutomation(body.Password)
			if err != nil {
				writeErr(w, 500, "Could not save the password")
				return
			}
		}
		if _, err := s.db.Exec(`INSERT INTO automation_credentials (name, provider, username, password_enc, created_at, updated_at) VALUES (?,?,?,?,?,?)`,
			body.Name, body.Provider, body.Username, enc, now, now); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ok"})
		return
	}
	var exists int
	if err := s.db.QueryRow(`SELECT id FROM automation_credentials WHERE id=?`, body.ID).Scan(&exists); err != nil {
		writeErr(w, 404, "Credential not found")
		return
	}
	if strings.TrimSpace(body.Password) == "" {
		_, err := s.db.Exec(`UPDATE automation_credentials SET name=?, provider=?, username=?, updated_at=? WHERE id=?`,
			body.Name, body.Provider, body.Username, now, body.ID)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	} else {
		enc, err := encryptAutomation(body.Password)
		if err != nil {
			writeErr(w, 500, "Could not save the password")
			return
		}
		_, err = s.db.Exec(`UPDATE automation_credentials SET name=?, provider=?, username=?, password_enc=?, updated_at=? WHERE id=?`,
			body.Name, body.Provider, body.Username, enc, now, body.ID)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) deleteAutomationCredential(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.master(w, r); !ok {
		return
	}
	s.ensureAutomationTables()
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if id <= 0 {
		writeErr(w, 400, "Choose a credential")
		return
	}
	rows, err := s.db.Query(`SELECT name FROM automation_jobs WHERE credential_id=? ORDER BY name`, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var names []string
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil && strings.TrimSpace(name) != "" {
			names = append(names, name)
		}
	}
	rows.Close()
	if len(names) > 0 {
		writeErr(w, 400, "Jobs still use this credential: "+strings.Join(names, ", "))
		return
	}
	_, _ = s.db.Exec(`DELETE FROM automation_storage WHERE credential_id=?`, id)
	_, _ = s.db.Exec(`DELETE FROM automation_credentials WHERE id=?`, id)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) listAutomationStorage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.master(w, r); !ok {
		return
	}
	s.ensureAutomationTables()
	page, size, q := pageQuery(r)
	where := "1=1"
	args := []any{}
	if q != "" {
		where += " AND (c.name LIKE ? OR c.provider LIKE ? OR c.username LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM automation_credentials c WHERE `+where, args...).Scan(&total); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	page = clampPage(page, size, total)
	qargs := append(append([]any{}, args...), size, (page-1)*size)
	rows, err := s.db.Query(`SELECT c.id, c.name, c.provider, COALESCE(st.updated_at, ''),
		(SELECT COUNT(*) FROM automation_jobs j WHERE j.credential_id=c.id)
		FROM automation_credentials c
		LEFT JOIN automation_storage st ON st.credential_id=c.id
		WHERE `+where+` ORDER BY c.name LIMIT ? OFFSET ?`, qargs...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, jobs int
		var name, provider, updated string
		if rows.Scan(&id, &name, &provider, &updated, &jobs) != nil {
			continue
		}
		items = append(items, map[string]any{
			"credential_id": id, "name": name, "provider": provider, "updated_at": updated, "job_count": jobs,
		})
	}
	s.writePage(w, page, size, total, items)
}

func (s *server) getAutomationStorage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.master(w, r); !ok {
		return
	}
	s.ensureAutomationTables()
	id, _ := strconv.Atoi(r.PathValue("credentialId"))
	if id <= 0 {
		writeErr(w, 400, "Choose a credential")
		return
	}
	var exists int
	if err := s.db.QueryRow(`SELECT id FROM automation_credentials WHERE id=?`, id).Scan(&exists); err != nil {
		writeErr(w, 404, "Credential not found")
		return
	}
	var cookie, updated, userAgent, provider string
	_ = s.db.QueryRow(`SELECT COALESCE(provider, '') FROM automation_credentials WHERE id=?`, id).Scan(&provider)
	_ = s.db.QueryRow(`SELECT cookies_json, updated_at, COALESCE(user_agent, '') FROM automation_storage WHERE credential_id=?`, id).Scan(&cookie, &updated, &userAgent)
	writeJSON(w, 200, map[string]any{"credential_id": id, "provider": provider, "cookie": cookie, "user_agent": userAgent, "updated_at": updated})
}

func (s *server) saveAutomationStorage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.master(w, r); !ok {
		return
	}
	s.ensureAutomationTables()
	var body struct {
		CredentialID int    `json:"credential_id"`
		Cookie       string `json:"cookie"`
		UserAgent    string `json:"user_agent"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	if body.CredentialID <= 0 {
		writeErr(w, 400, "Choose a credential")
		return
	}
	var exists int
	if err := s.db.QueryRow(`SELECT id FROM automation_credentials WHERE id=?`, body.CredentialID).Scan(&exists); err != nil {
		writeErr(w, 404, "Credential not found")
		return
	}
	if strings.TrimSpace(body.Cookie) == "" {
		writeErr(w, 400, "Enter a cookie")
		return
	}
	if err := s.writeAutomationStorage(body.CredentialID, body.Cookie, strings.TrimSpace(body.UserAgent)); err != nil {
		writeErr(w, 500, "Could not save the cookie")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) listAutomationJobs(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.master(w, r); !ok {
		return
	}
	s.ensureAutomationTables()
	page, size, q := pageQuery(r)
	where := "1=1"
	args := []any{}
	if q != "" {
		where += " AND (j.name LIKE ? OR c.name LIKE ? OR w.name LIKE ? OR w.domain LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like, like, like)
	}
	from := ` FROM automation_jobs j
		LEFT JOIN automation_credentials c ON c.id=j.credential_id
		LEFT JOIN websites w ON w.id=j.target_website_id
		LEFT JOIN accounts a ON a.id=j.target_account_id
		WHERE `
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) `+from+where, args...).Scan(&total); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	page = clampPage(page, size, total)
	qargs := append(append([]any{}, args...), size, (page-1)*size)
	rows, err := s.db.Query(`SELECT j.id, j.name, j.provider, j.credential_id, COALESCE(c.name, ''),
		j.target_website_id, COALESCE(w.name, ''), COALESCE(w.domain, ''), j.target_account_id, COALESCE(a.name, ''),
		j.interval_minutes, j.enabled, COALESCE(j.job_kind, 'tool'), j.last_run_at, j.last_result, j.last_reason, j.next_run_at `+from+where+
		` ORDER BY j.name LIMIT ? OFFSET ?`, qargs...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, credID, websiteID, accountID, interval, enabled int
		var name, provider, credName, siteName, domain, accountName, kind, lastRun, lastResult, lastReason, nextRun string
		if rows.Scan(&id, &name, &provider, &credID, &credName, &websiteID, &siteName, &domain, &accountID, &accountName,
			&interval, &enabled, &kind, &lastRun, &lastResult, &lastReason, &nextRun) != nil {
			continue
		}
		if kind != "session" {
			kind = "tool"
		}
		label := websiteChoiceLabel(siteName, domain)
		if kind == "session" {
			label = "Homepage"
		}
		items = append(items, map[string]any{
			"id": id, "name": name, "provider": provider,
			"credential_id": credID, "credential_name": credName,
			"target_website_id": websiteID, "target_label": label,
			"job_kind":          kind,
			"target_account_id": accountID, "target_account_name": accountName,
			"wake_website_ids": s.automationWakeIDs(id),
			"interval_minutes": interval, "enabled": enabled == 1,
			"last_run_at": lastRun, "last_result": lastResult, "last_reason": lastReason, "next_run_at": nextRun,
		})
	}
	s.writePage(w, page, size, total, items)
}

func (s *server) saveAutomationJob(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.master(w, r); !ok {
		return
	}
	s.ensureAutomationTables()
	var body struct {
		ID              int    `json:"id"`
		Name            string `json:"name"`
		Provider        string `json:"provider"`
		CredentialID    int    `json:"credential_id"`
		TargetWebsiteID int    `json:"target_website_id"`
		TargetAccountID int    `json:"target_account_id"`
		IntervalMinutes int    `json:"interval_minutes"`
		Enabled         bool   `json:"enabled"`
		WakeWebsiteIDs  []int  `json:"wake_website_ids"`
		JobKind         string `json:"job_kind"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Provider = strings.TrimSpace(body.Provider)
	if body.Name == "" {
		writeErr(w, 400, "Enter a name")
		return
	}
	if !knownAutomationProvider(body.Provider) {
		writeErr(w, 400, "Choose a provider")
		return
	}
	if body.IntervalMinutes != 0 && (body.IntervalMinutes < 5 || body.IntervalMinutes > 1440) {
		writeErr(w, 400, "Use 0 to turn the timer off, or 5 to 1440 minutes")
		return
	}
	var credProvider string
	if err := s.db.QueryRow(`SELECT provider FROM automation_credentials WHERE id=?`, body.CredentialID).Scan(&credProvider); err != nil {
		writeErr(w, 400, "Choose a credential")
		return
	}
	if credProvider != body.Provider {
		writeErr(w, 400, "Credential provider does not match the job")
		return
	}
	body.JobKind = strings.TrimSpace(body.JobKind)
	if body.JobKind != "session" {
		body.JobKind = "tool"
	}
	if body.JobKind == "session" && (body.Provider == "ahrefs" || body.Provider == "zikaanalytics" || body.Provider == "erank") {
		writeErr(w, 400, "This provider only updates a tool")
		return
	}
	var site automationSite
	if body.JobKind == "session" {
		body.TargetWebsiteID = 0
		body.TargetAccountID = 0
		body.WakeWebsiteIDs = nil
	} else {
		var siteOK bool
		site, siteOK = s.loadAutomationSite(body.TargetWebsiteID)
		if !siteOK {
			writeErr(w, 400, "Choose a website")
			return
		}
	}
	if body.JobKind == "session" || site.Kind == "recloud" {
		body.TargetAccountID = 0
	} else {
		var accountWebsite int
		if err := s.db.QueryRow(`SELECT website_id FROM accounts WHERE id=?`, body.TargetAccountID).Scan(&accountWebsite); err != nil || accountWebsite != site.ID {
			writeErr(w, 400, "Choose an account")
			return
		}
	}
	enabled := 0
	if body.Enabled {
		enabled = 1
	}
	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339)
	if body.ID == 0 {
		next := now.Add(time.Duration(body.IntervalMinutes) * time.Minute).Format(time.RFC3339)
		if body.JobKind == "session" {
			next = stamp
		}
		res, err := s.db.Exec(`INSERT INTO automation_jobs (name, provider, credential_id, target_website_id, target_account_id, interval_minutes, enabled, run_requested, next_run_at, last_run_at, last_result, last_reason, created_at, updated_at, job_kind)
			VALUES (?,?,?,?,?,?,?,0,?,'','','',?,?,?)`,
			body.Name, body.Provider, body.CredentialID, body.TargetWebsiteID, body.TargetAccountID, body.IntervalMinutes, enabled, next, stamp, stamp, body.JobKind)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		newID, _ := res.LastInsertId()
		s.saveAutomationWakeSites(int(newID), body.TargetWebsiteID, body.WakeWebsiteIDs)
		writeJSON(w, 200, map[string]string{"status": "ok"})
		return
	}
	var prevInterval int
	var prevNext, lastRun string
	if err := s.db.QueryRow(`SELECT interval_minutes, next_run_at, last_run_at FROM automation_jobs WHERE id=?`, body.ID).Scan(&prevInterval, &prevNext, &lastRun); err != nil {
		writeErr(w, 404, "Job not found")
		return
	}
	next := prevNext
	if prevInterval != body.IntervalMinutes {
		next = automationNextRun(lastRun, body.IntervalMinutes, now).Format(time.RFC3339)
	}
	if _, err := s.db.Exec(`UPDATE automation_jobs SET name=?, provider=?, credential_id=?, target_website_id=?, target_account_id=?, interval_minutes=?, enabled=?, next_run_at=?, updated_at=?, job_kind=? WHERE id=?`,
		body.Name, body.Provider, body.CredentialID, body.TargetWebsiteID, body.TargetAccountID, body.IntervalMinutes, enabled, next, stamp, body.JobKind, body.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.saveAutomationWakeSites(body.ID, body.TargetWebsiteID, body.WakeWebsiteIDs)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) deleteAutomationJob(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.master(w, r); !ok {
		return
	}
	s.ensureAutomationTables()
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if id <= 0 {
		writeErr(w, 400, "Choose a job")
		return
	}
	_, _ = s.db.Exec(`DELETE FROM automation_job_sites WHERE job_id=?`, id)
	_, _ = s.db.Exec(`DELETE FROM automation_jobs WHERE id=?`, id)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) runAutomationJobNow(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.master(w, r); !ok {
		return
	}
	s.ensureAutomationTables()
	id, _ := strconv.Atoi(r.PathValue("id"))
	var enabled int
	if err := s.db.QueryRow(`SELECT enabled FROM automation_jobs WHERE id=?`, id).Scan(&enabled); err != nil {
		writeErr(w, 404, "Job not found")
		return
	}
	if enabled != 1 {
		writeErr(w, 400, "Enable the job first")
		return
	}
	if _, err := s.db.Exec(`UPDATE automation_jobs SET run_requested=1 WHERE id=?`, id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	go s.runDueAutomationJobs()
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) listAutomationLogs(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.master(w, r); !ok {
		return
	}
	s.ensureAutomationTables()
	page, size, q := pageQuery(r)
	where := "1=1"
	args := []any{}
	if q != "" {
		where += " AND (message LIKE ? OR job_name LIKE ? OR credential_name LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	if jobID, _ := strconv.Atoi(r.URL.Query().Get("jobId")); jobID > 0 {
		where += " AND job_id=?"
		args = append(args, jobID)
	}
	level := strings.TrimSpace(r.URL.Query().Get("level"))
	if level == "info" || level == "error" {
		where += " AND level=?"
		args = append(args, level)
	}
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM automation_logs WHERE `+where, args...).Scan(&total); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	page = clampPage(page, size, total)
	qargs := append(append([]any{}, args...), size, (page-1)*size)
	rows, err := s.db.Query(`SELECT id, created_at, job_id, job_name, credential_id, credential_name, level, message
		FROM automation_logs WHERE `+where+` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, qargs...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, jobID, credID int
		var created, jobName, credName, level, message string
		if rows.Scan(&id, &created, &jobID, &jobName, &credID, &credName, &level, &message) != nil {
			continue
		}
		items = append(items, map[string]any{
			"id": id, "created_at": created, "job_id": jobID, "job_name": jobName,
			"credential_id": credID, "credential_name": credName, "level": level, "message": message,
		})
	}
	s.writePage(w, page, size, total, items)
}

func (s *server) clearAutomationLogs(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.master(w, r); !ok {
		return
	}
	s.ensureAutomationTables()
	if _, err := s.db.Exec(`DELETE FROM automation_logs`); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) automationOptions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.master(w, r); !ok {
		return
	}
	s.ensureAutomationTables()
	credRows, err := s.db.Query(`SELECT id, name, provider, username FROM automation_credentials ORDER BY name`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	credentials := []any{}
	for credRows.Next() {
		var id int
		var name, provider, username string
		if credRows.Scan(&id, &name, &provider, &username) != nil {
			continue
		}
		credentials = append(credentials, map[string]any{"id": id, "name": name, "provider": provider, "username": username})
	}
	credRows.Close()
	jobRows, err := s.db.Query(`SELECT id, name FROM automation_jobs ORDER BY name`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	jobs := []any{}
	for jobRows.Next() {
		var id int
		var name string
		if jobRows.Scan(&id, &name) != nil {
			continue
		}
		jobs = append(jobs, map[string]any{"id": id, "name": name})
	}
	jobRows.Close()
	siteRows, err := s.db.Query(`SELECT id, tool_id, name, domain, COALESCE(kind, '') FROM websites ORDER BY name`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	type siteAccounts struct {
		ID       int
		ToolID   int
		Name     string
		Domain   string
		Kind     string
		Accounts []any
	}
	sites := map[int]*siteAccounts{}
	var siteOrder []int
	for siteRows.Next() {
		item := &siteAccounts{Accounts: []any{}}
		if siteRows.Scan(&item.ID, &item.ToolID, &item.Name, &item.Domain, &item.Kind) != nil {
			continue
		}
		sites[item.ID] = item
		siteOrder = append(siteOrder, item.ID)
	}
	siteRows.Close()
	accountRows, err := s.db.Query(`SELECT id, website_id, name FROM accounts ORDER BY id`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for accountRows.Next() {
		var id, websiteID int
		var name string
		if accountRows.Scan(&id, &websiteID, &name) != nil {
			continue
		}
		if site := sites[websiteID]; site != nil {
			site.Accounts = append(site.Accounts, map[string]any{"id": id, "name": name})
		}
	}
	accountRows.Close()
	websites := []any{}
	for _, id := range siteOrder {
		site := sites[id]
		websites = append(websites, map[string]any{
			"id": site.ID, "tool_id": site.ToolID, "name": site.Name, "domain": site.Domain, "kind": site.Kind, "accounts": site.Accounts,
		})
	}
	if credentials == nil {
		credentials = []any{}
	}
	if jobs == nil {
		jobs = []any{}
	}
	writeJSON(w, 200, map[string]any{
		"providers":   s.automationProviders(),
		"credentials": credentials,
		"websites":    websites,
		"jobs":        jobs,
	})
}
