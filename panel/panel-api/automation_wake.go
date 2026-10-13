package main

import (
	"context"
	"database/sql"
	"net/url"
	"strings"
	"sync"
	"time"

	"panel-api/automation"
)

const automationTriggerGap = 5 * time.Minute

var automationWakeMu sync.Mutex

func automationTriggerStamp(db *sql.DB, jobID int, now string) string {
	var cause, previous string
	_ = db.QueryRow(`SELECT run_cause, last_trigger_at FROM automation_jobs WHERE id=?`, jobID).Scan(&cause, &previous)
	if cause == "trigger" {
		return now
	}
	return previous
}

func automationTriggerCooling(last string, now time.Time) bool {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(last))
	if err != nil {
		return false
	}
	return now.Sub(parsed) < automationTriggerGap
}

func automationUpstreamHost(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

func automationHostMatches(host, portal string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	portal = strings.ToLower(strings.TrimSpace(portal))
	if host == "" || portal == "" {
		return false
	}
	return host == portal || strings.HasSuffix(host, "."+portal)
}

func automationPortalHost(provider string) string {
	recipe, ok := recipeFor(provider)
	if !ok {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(recipe.PortalHost()))
}

func (s *server) jobsForLogout(websiteID int) []int {
	var upstream string
	if err := s.db.QueryRow(`SELECT COALESCE(upstream_url, '') FROM websites WHERE id=?`, websiteID).Scan(&upstream); err != nil {
		return nil
	}
	host := automationUpstreamHost(upstream)
	rows, err := s.db.Query(`SELECT id, provider, target_website_id FROM automation_jobs WHERE enabled=1 ORDER BY id`)
	if err != nil {
		return nil
	}
	type jobPick struct {
		id       int
		provider string
		target   int
	}
	var jobs []jobPick
	for rows.Next() {
		var job jobPick
		if rows.Scan(&job.id, &job.provider, &job.target) == nil {
			jobs = append(jobs, job)
		}
	}
	rows.Close()
	explicit := map[int]bool{}
	listed := map[int]bool{}
	siteRows, err := s.db.Query(`SELECT job_id, website_id FROM automation_job_sites`)
	if err == nil {
		for siteRows.Next() {
			var jobID, siteID int
			if siteRows.Scan(&jobID, &siteID) != nil {
				continue
			}
			listed[jobID] = true
			if siteID == websiteID {
				explicit[jobID] = true
			}
		}
		siteRows.Close()
	}
	counts := map[string]int{}
	for _, job := range jobs {
		if automationHostMatches(host, automationPortalHost(job.provider)) {
			counts[job.provider]++
		}
	}
	var out []int
	seen := map[int]bool{}
	for _, job := range jobs {
		take := explicit[job.id] || job.target == websiteID
		if !take && !listed[job.id] && counts[job.provider] == 1 && automationHostMatches(host, automationPortalHost(job.provider)) {
			take = true
		}
		if take && !seen[job.id] {
			seen[job.id] = true
			out = append(out, job.id)
		}
	}
	return out
}

func (s *server) drainAutomationSignals() {
	s.ensureAutomationTables()
	rows, err := s.db.Query(`SELECT id, website_id FROM automation_signals WHERE state='pending' ORDER BY id`)
	if err != nil {
		return
	}
	type signal struct {
		id        int
		websiteID int
	}
	var pending []signal
	for rows.Next() {
		var item signal
		if rows.Scan(&item.id, &item.websiteID) == nil {
			pending = append(pending, item)
		}
	}
	rows.Close()
	for _, item := range pending {
		for _, jobID := range s.jobsForLogout(item.websiteID) {
			s.enqueueAutomationTrigger(jobID, item.websiteID)
		}
		_, _ = s.db.Exec(`UPDATE automation_signals SET state='done' WHERE id=?`, item.id)
	}
}

func (s *server) enqueueAutomationTrigger(jobID, websiteID int) string {
	automationWakeMu.Lock()
	defer automationWakeMu.Unlock()
	job, ok := s.loadAutomationJob(jobID)
	if !ok || job.Enabled != 1 {
		return "missing"
	}
	var running, requested int
	var queued, lastTrigger string
	if err := s.db.QueryRow(`SELECT running, run_requested, queued_cause, last_trigger_at FROM automation_jobs WHERE id=?`, jobID).Scan(&running, &requested, &queued, &lastTrigger); err != nil {
		return "missing"
	}
	now := time.Now().UTC()
	var latest string
	_ = s.db.QueryRow(`SELECT COALESCE(MAX(last_trigger_at), '') FROM automation_jobs WHERE credential_id=?`, job.CredentialID).Scan(&latest)
	if automationTriggerCooling(latest, now) || automationTriggerCooling(lastTrigger, now) {
		s.writeAutomationLog(job, "info", "logout trigger skipped, last automatic run was under 5 minutes", nil)
		return "cooldown"
	}
	if running == 1 || queued != "" || requested == 1 {
		s.writeAutomationLog(job, "info", "logout trigger skipped, this job is already running or waiting", nil)
		return "duplicate"
	}
	var other int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM automation_jobs WHERE credential_id=? AND id<>? AND enabled=1 AND (running=1 OR queued_cause<>'' OR run_requested=1)`, job.CredentialID, jobID).Scan(&other)
	stamp := now.Format(time.RFC3339)
	res, err := s.db.Exec(`UPDATE automation_jobs SET queued_cause='trigger', wake_website_id=?, updated_at=? WHERE id=? AND running=0 AND queued_cause='' AND run_requested=0`,
		websiteID, stamp, jobID)
	if err != nil {
		return "missing"
	}
	if n, _ := res.RowsAffected(); n == 0 {
		s.writeAutomationLog(job, "info", "logout trigger skipped, this job is already running or waiting", nil)
		return "duplicate"
	}
	if other > 0 {
		s.writeAutomationLog(job, "info", "logout trigger waiting for the other job on this login", nil)
		return "queued"
	}
	s.writeAutomationLog(job, "info", "logout trigger started", nil)
	return "accepted"
}

func (s *server) startQueuedAutomation() {
	s.ensureAutomationTables()
	rows, err := s.db.Query(`SELECT id, credential_id, queued_cause FROM automation_jobs
		WHERE enabled=1 AND running=0 AND queued_cause<>'' ORDER BY updated_at, id`)
	if err != nil {
		return
	}
	type queuedJob struct {
		id    int
		cred  int
		cause string
	}
	var queued []queuedJob
	for rows.Next() {
		var item queuedJob
		if rows.Scan(&item.id, &item.cred, &item.cause) == nil {
			queued = append(queued, item)
		}
	}
	rows.Close()
	seen := map[int]bool{}
	var wg sync.WaitGroup
	now := time.Now().UTC()
	for _, item := range queued {
		if seen[item.cred] {
			continue
		}
		var busy int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM automation_jobs WHERE credential_id=? AND (running=1 OR run_requested=1)`, item.cred).Scan(&busy)
		if busy > 0 {
			continue
		}
		if item.cause == "trigger" {
			var latest string
			_ = s.db.QueryRow(`SELECT COALESCE(MAX(last_trigger_at), '') FROM automation_jobs WHERE credential_id=?`, item.cred).Scan(&latest)
			if automationTriggerCooling(latest, now) {
				continue
			}
		}
		seen[item.cred] = true
		wg.Add(1)
		go func(credentialID, jobID int) {
			defer wg.Done()
			lock := automationCredLock(credentialID)
			lock.Lock()
			defer lock.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), automationJobTimeout)
			defer cancel()
			s.runAutomationJob(ctx, jobID)
		}(item.cred, item.id)
	}
	wg.Wait()
}

func (s *server) captureAutomationWake(ctx context.Context, recipe automation.Recipe, job automationJobRow, portal []cookieJSON, websiteID int, secrets []string) string {
	wake, ok := s.loadAutomationSite(websiteID)
	if !ok || wake.Kind != "recloud" || strings.TrimSpace(wake.Upstream) == "" {
		return ""
	}
	tool, err := recipe.Capture(ctx, portal, wake.Upstream)
	if err != nil {
		s.writeAutomationLog(job, "error", "could not refresh "+strings.TrimSpace(wake.Name), secrets)
		return ""
	}
	if len(tool) == 0 {
		tool = portal
	}
	raw, err := cookiesToJSON(tool)
	if err != nil || strings.TrimSpace(raw) == "" {
		s.writeAutomationLog(job, "error", "could not refresh "+strings.TrimSpace(wake.Name), secrets)
		return ""
	}
	accountID, err := s.writeAutomationAccountCookie(wake, 0, raw)
	if err != nil {
		s.writeAutomationLog(job, "error", "could not refresh "+strings.TrimSpace(wake.Name), append(secrets, raw))
		return ""
	}
	s.shareCookieFrom(accountID)
	message := "wrote cookie to " + strings.TrimSpace(wake.Name)
	s.writeAutomationLog(job, "info", message, append(secrets, raw))
	return message
}

func (s *server) automationWakeIDs(jobID int) []int {
	rows, err := s.db.Query(`SELECT website_id FROM automation_job_sites WHERE job_id=? ORDER BY website_id`, jobID)
	if err != nil {
		return []int{}
	}
	defer rows.Close()
	ids := []int{}
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func (s *server) saveAutomationWakeSites(jobID, targetID int, ids []int) {
	if jobID <= 0 {
		return
	}
	_, _ = s.db.Exec(`DELETE FROM automation_job_sites WHERE job_id=?`, jobID)
	seen := map[int]bool{targetID: true}
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		var kind string
		if err := s.db.QueryRow(`SELECT COALESCE(kind, '') FROM websites WHERE id=?`, id).Scan(&kind); err != nil || kind != "recloud" {
			continue
		}
		_, _ = s.db.Exec(`INSERT INTO automation_job_sites (job_id, website_id) VALUES (?,?)`, jobID, id)
	}
}
