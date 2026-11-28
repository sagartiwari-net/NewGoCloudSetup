package main

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"
)

type barAccount struct {
	ID              int64
	ResellerID      int
	Name            string
	AccessTokenJSON string
	Status          string
	Bearer          string
}

func (s *server) stickyMinutes(resellerID int) int {
	mins := 20
	_ = s.db.QueryRow(`SELECT sticky_minutes FROM ahrefs_bar_reseller_settings WHERE reseller_id = ?`, resellerID).Scan(&mins)
	if mins < 0 {
		mins = 0
	}
	return mins
}

func (s *server) loadAccountByID(id int64) (*barAccount, error) {
	if id <= 0 {
		return nil, nil
	}
	now := s.dbTime(time.Now())
	row := s.db.QueryRow(`
SELECT id, reseller_id, name, access_token_json, status
FROM ahrefs_bar_accounts
WHERE id = ?
  AND status IN ('active','bad','cooldown')
  AND (cooldown_until IS NULL OR cooldown_until = '' OR cooldown_until < ?)`, id, now)
	return scanAccount(row)
}

func scanAccount(row *sql.Row) (*barAccount, error) {
	var a barAccount
	if err := row.Scan(&a.ID, &a.ResellerID, &a.Name, &a.AccessTokenJSON, &a.Status); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	a.Bearer = extractAccessToken(a.AccessTokenJSON)
	if a.Bearer == "" {
		return nil, fmt.Errorf("account %d has no access token", a.ID)
	}
	return &a, nil
}

// listHealthyAccounts returns RR-ordered healthy accounts for a reseller.
func (s *server) listHealthyAccounts(resellerID int) ([]barAccount, error) {
	now := s.dbTime(time.Now())
	// Revive cooled-down accounts back to active.
	_, _ = s.db.Exec(`
UPDATE ahrefs_bar_accounts
SET status = 'active', last_error = '', updated_at = ?
WHERE reseller_id = ? AND status IN ('bad','cooldown')
  AND cooldown_until IS NOT NULL AND cooldown_until <> '' AND cooldown_until < ?`,
		now, resellerID, now)

	rows, err := s.db.Query(`
SELECT id, reseller_id, name, access_token_json, status
FROM ahrefs_bar_accounts
WHERE reseller_id = ? AND status = 'active'
  AND (cooldown_until IS NULL OR cooldown_until = '' OR cooldown_until < ?)
ORDER BY CASE WHEN last_used_at IS NULL OR last_used_at = '' THEN 0 ELSE 1 END, last_used_at ASC, id ASC`,
		resellerID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []barAccount
	for rows.Next() {
		var a barAccount
		if err := rows.Scan(&a.ID, &a.ResellerID, &a.Name, &a.AccessTokenJSON, &a.Status); err != nil {
			return nil, err
		}
		a.Bearer = extractAccessToken(a.AccessTokenJSON)
		if a.Bearer == "" {
			continue
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// poolUnavailableDetail explains empty healthy pool. Auth-dead tokens
// (expired / revoked Ahrefs subscription) get a clear admin message —
// not a spinner / generic "Error fetching data".
func (s *server) poolUnavailableDetail(resellerID int) (sec int, retryAt, message string) {
	now := time.Now().UTC()
	nowStr := s.dbTime(now)

	var total, authDead int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM ahrefs_bar_accounts WHERE reseller_id = ?`, resellerID).Scan(&total)
	_ = s.db.QueryRow(`
SELECT COUNT(*) FROM ahrefs_bar_accounts
WHERE reseller_id = ?
  AND status IN ('bad','cooldown','disabled')
  AND (
    last_error LIKE '%401%' OR last_error LIKE '%403%' OR last_error LIKE '%Unauthorized%'
    OR last_error LIKE '%expired%' OR last_error LIKE '%revoked%' OR last_error LIKE '%invalid_token%'
    OR last_error LIKE '%subscription%'
  )`, resellerID).Scan(&authDead)

	if total == 0 || (total > 0 && authDead >= total) {
		message = "Subscription expired. Contact to admin."
		return
	}

	var until sql.NullString
	_ = s.db.QueryRow(`
SELECT MIN(cooldown_until) FROM ahrefs_bar_accounts
WHERE reseller_id = ? AND status IN ('bad','cooldown')
  AND cooldown_until IS NOT NULL AND cooldown_until <> '' AND cooldown_until > ?`,
		resellerID, nowStr).Scan(&until)
	if until.Valid && strings.TrimSpace(until.String) != "" {
		if t, err := parseDBTime(until.String); err == nil && t.After(now) {
			sec = int(t.Sub(now).Seconds())
			if sec < 1 {
				sec = 1
			}
			retryAt = s.dbTime(t)
			message = "Ahrefs account cooling down. Try again in " + formatWait(sec) + "."
			return
		}
	}
	// No healthy token left — treat as admin/subscription issue for clear UX.
	message = "Subscription expired. Contact to admin."
	return
}

// pickAccountOrder returns sticky-first then RR list (excluding sticky duplicate).
func (s *server) pickAccountOrder(sess *barSession) ([]barAccount, error) {
	all, err := s.listHealthyAccounts(sess.ResellerID)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, nil
	}

	var ordered []barAccount
	seen := map[int64]bool{}

	if sess.StickyAccountID.Valid && sess.StickyAccountID.Int64 > 0 {
		if stickyUntilOK(sess.StickyUntil) {
			for _, a := range all {
				if a.ID == sess.StickyAccountID.Int64 {
					ordered = append(ordered, a)
					seen[a.ID] = true
					break
				}
			}
		}
	}
	var deferred []barAccount
	for _, a := range all {
		if seen[a.ID] {
			continue
		}
		if isAccountSoftSkipped(a.ID) {
			deferred = append(deferred, a)
			continue
		}
		ordered = append(ordered, a)
	}
	ordered = append(ordered, deferred...)
	return ordered, nil
}

func stickyUntilOK(raw sql.NullString) bool {
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return false
	}
	t, err := parseDBTime(raw.String)
	if err != nil {
		return false
	}
	return time.Now().Before(t)
}

func (s *server) touchAccountUsed(id int64) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.db.Exec(`UPDATE ahrefs_bar_accounts SET last_used_at = ?, updated_at = ? WHERE id = ?`, now, now, id)
}

func (s *server) setSessionSticky(token string, accountID int64, minutes int) {
	if minutes <= 0 || accountID <= 0 || token == "" {
		return
	}
	until := time.Now().UTC().Add(time.Duration(minutes) * time.Minute).Format(time.RFC3339)
	_, _ = s.db.Exec(`
UPDATE ahrefs_bar_sessions
SET sticky_account_id = ?, sticky_until = ?
WHERE token = ?`, accountID, until, token)
}

func (s *server) clearSessionSticky(token string) {
	_, _ = s.db.Exec(`UPDATE ahrefs_bar_sessions SET sticky_account_id = NULL, sticky_until = NULL WHERE token = ?`, token)
}

// markAccountBad parks a pool token after hard auth failures (401/403).
// Do NOT call this for upstream 429 / RateLimit — that is Ahrefs traffic
// shaping on a still-valid token. Parking it bricks every user (Metrics
// included) even when their hourly/daily meters still have room. User
// quotas are enforced by chargeMeters; pool 429s are soft-skipped only.
func (s *server) markAccountBad(id int64, reason string) {
	reason = strings.TrimSpace(reason)
	if len(reason) > 500 {
		reason = reason[:500]
	}
	now := time.Now().UTC()
	// Auth failures (expired Ahrefs subscription) stay parked longer.
	wait := 15 * time.Minute
	low := strings.ToLower(reason)
	if strings.Contains(low, "401") || strings.Contains(low, "403") || strings.Contains(low, "unauthorized") {
		wait = 6 * time.Hour
	}
	cooldown := s.dbTime(now.Add(wait))
	_, _ = s.db.Exec(`
UPDATE ahrefs_bar_accounts
SET status = 'bad', last_error = ?, cooldown_until = ?, updated_at = ?
WHERE id = ?`, reason, cooldown, s.dbTime(now), id)
}

func (s *server) noteAccountUpstream(id int64, reason string) {
	reason = strings.TrimSpace(reason)
	if len(reason) > 500 {
		reason = reason[:500]
	}
	now := s.dbTime(time.Now())
	_, _ = s.db.Exec(`
UPDATE ahrefs_bar_accounts
SET last_error = ?, updated_at = ?
WHERE id = ?`, reason, now, id)
}

var (
	softSkipMu    sync.Mutex
	softSkipUntil = map[int64]time.Time{}
)

// noteAccountSoftSkip briefly deprioritizes an account after upstream 429
// so RR can prefer another token. Never empties the pool.
func noteAccountSoftSkip(id int64) {
	softSkipMu.Lock()
	softSkipUntil[id] = time.Now().Add(20 * time.Second)
	softSkipMu.Unlock()
}

func isAccountSoftSkipped(id int64) bool {
	softSkipMu.Lock()
	defer softSkipMu.Unlock()
	t, ok := softSkipUntil[id]
	return ok && time.Now().Before(t)
}

// seedDevAccounts ensures two accounts for failover tests in dev.
// A = MOCK:401 (always fails)  B = MOCK (ok). Real token env overrides B only.
func (s *server) seedDevAccount() error {
	if !s.cfg.DevAuth {
		return nil
	}
	goodJSON := strings.TrimSpace(s.cfg.AccountJSON)
	if goodJSON == "" {
		goodJSON = `{"accessToken":"MOCK","expiresIn":3600}`
	}
	badJSON := `{"accessToken":"MOCK:401","expiresIn":3600}`

	if err := s.upsertDevNamedAccount("Dev Account A (bad)", badJSON); err != nil {
		return err
	}
	if err := s.upsertDevNamedAccount("Dev Account B (good)", goodJSON); err != nil {
		return err
	}
	// Disable legacy single-account seed so RR failover tests hit A then B.
	_, _ = s.db.Exec(`
UPDATE ahrefs_bar_accounts
SET status = 'disabled', updated_at = ?
WHERE reseller_id = 1 AND name = 'Dev Account'`, time.Now().UTC().Format(time.RFC3339))
	return nil
}

func (s *server) upsertDevNamedAccount(name, tokenJSON string) error {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM ahrefs_bar_accounts WHERE reseller_id = 1 AND name = ? LIMIT 1`, name).Scan(&id)
	if err == nil {
		_, err = s.db.Exec(`
UPDATE ahrefs_bar_accounts
SET access_token_json = ?, status = 'active', last_error = '', cooldown_until = NULL, updated_at = ?
WHERE id = ?`, tokenJSON, time.Now().UTC().Format(time.RFC3339), id)
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO ahrefs_bar_accounts (reseller_id, name, access_token_json, status)
VALUES (1, ?, ?, 'active')`, name, tokenJSON)
	return err
}

func (s *server) listAccountsPublic(resellerID int) ([]map[string]any, error) {
	rows, err := s.db.Query(`
SELECT id, name, status, COALESCE(last_error,''), cooldown_until, last_used_at
FROM ahrefs_bar_accounts WHERE reseller_id = ? ORDER BY id ASC`, resellerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, status, lastErr string
		var cooldown, lastUsed sql.NullString
		if err := rows.Scan(&id, &name, &status, &lastErr, &cooldown, &lastUsed); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id":             id,
			"name":           name,
			"status":         status,
			"last_error":     lastErr,
			"cooldown_until": nullStr(cooldown),
			"last_used_at":   nullStr(lastUsed),
		})
	}
	return out, rows.Err()
}

func nullStr(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}
