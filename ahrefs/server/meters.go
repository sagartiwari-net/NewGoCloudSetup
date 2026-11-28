package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type meterSnapshot struct {
	HourlyLimit     int
	DailyLimit      int
	HourlyUsed      int
	DailyUsed       int
	HourlyRemaining int
	DailyRemaining  int
}

func (s *server) meterSnapshot(resellerID int, login string) (meterSnapshot, error) {
	hLim, dLim, hUsed, dUsed, err := s.resolveLimits(resellerID, login)
	if err != nil {
		return meterSnapshot{}, err
	}
	return meterSnapshot{
		HourlyLimit:     hLim,
		DailyLimit:      dLim,
		HourlyUsed:      hUsed,
		DailyUsed:       dUsed,
		HourlyRemaining: max(0, hLim-hUsed),
		DailyRemaining:  max(0, dLim-dUsed),
	}, nil
}

// chargeMeters atomically reserves credits. Returns false if either window is exhausted.
// Rolling hourly: sum of ahrefs_bar_usage_hourly buckets in last 60 minutes.
// Daily: IST calendar day row.
func (s *server) chargeMeters(resellerID int, login string, credits int) (ok bool, snap meterSnapshot, reason string, err error) {
	if credits <= 0 {
		snap, err = s.meterSnapshot(resellerID, login)
		return true, snap, "", err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return false, snap, "", err
	}
	defer func() { _ = tx.Rollback() }()

	hLim, dLim := 50, 150
	_ = tx.QueryRow(`SELECT hourly_limit, daily_limit FROM ahrefs_bar_reseller_settings WHERE reseller_id = ?`, resellerID).
		Scan(&hLim, &dLim)

	var ch, cd, custom int
	qerr := tx.QueryRow(`
SELECT hourly_limit, daily_limit, is_custom FROM ahrefs_bar_user_limits
WHERE reseller_id = ? AND amember_login = ?`, resellerID, login).Scan(&ch, &cd, &custom)
	if qerr == nil && custom == 1 {
		hLim, dLim = ch, cd
	} else if qerr != nil && qerr != sql.ErrNoRows {
		return false, snap, "", qerr
	}

	since := time.Now().UTC().Add(-60 * time.Minute).Format(time.RFC3339)
	var hUsed int
	if err := tx.QueryRow(`
SELECT COALESCE(SUM(used),0) FROM ahrefs_bar_usage_hourly
WHERE reseller_id = ? AND amember_login = ? AND bucket_start >= ?`, resellerID, login, since).Scan(&hUsed); err != nil {
		return false, snap, "", err
	}

	dayIST := time.Now().In(mustLoc(s.cfg.Timezone)).Format("2006-01-02")
	var dUsed int
	if err := tx.QueryRow(`
SELECT COALESCE(used,0) FROM ahrefs_bar_usage_daily
WHERE reseller_id = ? AND amember_login = ? AND day_ist = ?`, resellerID, login, dayIST).Scan(&dUsed); err != nil && err != sql.ErrNoRows {
		return false, snap, "", err
	}

	if hUsed+credits > hLim {
		snap = meterSnapshot{hLim, dLim, hUsed, dUsed, max(0, hLim-hUsed), max(0, dLim-dUsed)}
		return false, snap, "hourly", nil
	}
	if dUsed+credits > dLim {
		snap = meterSnapshot{hLim, dLim, hUsed, dUsed, max(0, hLim-hUsed), max(0, dLim-dUsed)}
		return false, snap, "daily", nil
	}

	// Bucket = floor to current UTC hour for simpler upsert; remaining uses rolling SUM.
	bucket := time.Now().UTC().Truncate(time.Hour).Format(time.RFC3339)
	if err := upsertHourly(tx, s.driver, resellerID, login, bucket, credits); err != nil {
		return false, snap, "", err
	}
	if err := upsertDaily(tx, s.driver, resellerID, login, dayIST, credits); err != nil {
		return false, snap, "", err
	}

	if err := tx.Commit(); err != nil {
		return false, snap, "", err
	}

	snap = meterSnapshot{
		HourlyLimit:     hLim,
		DailyLimit:      dLim,
		HourlyUsed:      hUsed + credits,
		DailyUsed:       dUsed + credits,
		HourlyRemaining: max(0, hLim-(hUsed+credits)),
		DailyRemaining:  max(0, dLim-(dUsed+credits)),
	}
	return true, snap, "", nil
}

func upsertHourly(tx *sql.Tx, driver string, resellerID int, login, bucket string, credits int) error {
	if driver == "mysql" {
		_, err := tx.Exec(`
INSERT INTO ahrefs_bar_usage_hourly (reseller_id, amember_login, bucket_start, used)
VALUES (?, ?, ?, ?)
ON DUPLICATE KEY UPDATE used = used + VALUES(used)`, resellerID, login, bucket, credits)
		return err
	}
	_, err := tx.Exec(`
INSERT INTO ahrefs_bar_usage_hourly (reseller_id, amember_login, bucket_start, used)
VALUES (?, ?, ?, ?)
ON CONFLICT(reseller_id, amember_login, bucket_start) DO UPDATE SET used = used + excluded.used`,
		resellerID, login, bucket, credits)
	return err
}

func upsertDaily(tx *sql.Tx, driver string, resellerID int, login, day string, credits int) error {
	if driver == "mysql" {
		_, err := tx.Exec(`
INSERT INTO ahrefs_bar_usage_daily (reseller_id, amember_login, day_ist, used)
VALUES (?, ?, ?, ?)
ON DUPLICATE KEY UPDATE used = used + VALUES(used)`, resellerID, login, day, credits)
		return err
	}
	_, err := tx.Exec(`
INSERT INTO ahrefs_bar_usage_daily (reseller_id, amember_login, day_ist, used)
VALUES (?, ?, ?, ?)
ON CONFLICT(reseller_id, amember_login, day_ist) DO UPDATE SET used = used + excluded.used`,
		resellerID, login, day, credits)
	return err
}

func (s *server) refundMeters(resellerID int, login string, credits int) error {
	if credits <= 0 {
		return nil
	}
	bucket := time.Now().UTC().Truncate(time.Hour).Format(time.RFC3339)
	dayIST := time.Now().In(mustLoc(s.cfg.Timezone)).Format("2006-01-02")
	_, err1 := s.db.Exec(`
UPDATE ahrefs_bar_usage_hourly SET used = CASE WHEN used >= ? THEN used - ? ELSE 0 END
WHERE reseller_id = ? AND amember_login = ? AND bucket_start = ?`, credits, credits, resellerID, login, bucket)
	_, err2 := s.db.Exec(`
UPDATE ahrefs_bar_usage_daily SET used = CASE WHEN used >= ? THEN used - ? ELSE 0 END
WHERE reseller_id = ? AND amember_login = ? AND day_ist = ?`, credits, credits, resellerID, login, dayIST)
	if err1 != nil {
		return err1
	}
	return err2
}

func (s *server) setUserCustomLimits(resellerID int, login string, hourly, daily int) error {
	if hourly < 0 || daily < 0 {
		return fmt.Errorf("limits must be >= 0")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if s.driver == "mysql" {
		_, err := s.db.Exec(`
INSERT INTO ahrefs_bar_user_limits (reseller_id, amember_login, hourly_limit, daily_limit, is_custom, updated_at)
VALUES (?, ?, ?, ?, 1, ?)
ON DUPLICATE KEY UPDATE hourly_limit=VALUES(hourly_limit), daily_limit=VALUES(daily_limit), is_custom=1, updated_at=VALUES(updated_at)`,
			resellerID, login, hourly, daily, now)
		return err
	}
	_, err := s.db.Exec(`
INSERT INTO ahrefs_bar_user_limits (reseller_id, amember_login, hourly_limit, daily_limit, is_custom, updated_at)
VALUES (?, ?, ?, ?, 1, ?)
ON CONFLICT(reseller_id, amember_login) DO UPDATE SET
  hourly_limit=excluded.hourly_limit, daily_limit=excluded.daily_limit, is_custom=1, updated_at=excluded.updated_at`,
		resellerID, login, hourly, daily, now)
	return err
}

func (s *server) resetUserMeters(resellerID int, login string) error {
	_, err := s.db.Exec(`DELETE FROM ahrefs_bar_usage_hourly WHERE reseller_id = ? AND amember_login = ?`, resellerID, login)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM ahrefs_bar_usage_daily WHERE reseller_id = ? AND amember_login = ?`, resellerID, login)
	return err
}

func (s *server) refundOn5xx(resellerID int) bool {
	var flag int
	err := s.db.QueryRow(`SELECT refund_on_5xx FROM ahrefs_bar_reseller_settings WHERE reseller_id = ?`, resellerID).Scan(&flag)
	if err != nil {
		return true
	}
	return flag == 1
}

// meterRetryAfter returns seconds until the blocked window can accept more credits.
func (s *server) meterRetryAfter(reason string, resellerID int, login string) (sec int, until time.Time) {
	now := time.Now().UTC()
	if reason == "daily" {
		loc := mustLoc(s.cfg.Timezone)
		local := now.In(loc)
		next := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, loc)
		until = next.UTC()
		sec = int(until.Sub(now).Seconds())
		if sec < 1 {
			sec = 1
		}
		return sec, until
	}
	// Hourly = rolling 60m: wait until the oldest contributing bucket ages out.
	since := now.Add(-60 * time.Minute).Format(time.RFC3339)
	var oldest sql.NullString
	_ = s.db.QueryRow(`
SELECT MIN(bucket_start) FROM ahrefs_bar_usage_hourly
WHERE reseller_id = ? AND amember_login = ? AND bucket_start >= ? AND used > 0`,
		resellerID, login, since).Scan(&oldest)
	if oldest.Valid && strings.TrimSpace(oldest.String) != "" {
		if t, err := time.Parse(time.RFC3339, oldest.String); err == nil {
			until = t.Add(60 * time.Minute)
			if until.After(now) {
				sec = int(until.Sub(now).Seconds())
				if sec < 1 {
					sec = 1
				}
				return sec, until
			}
		}
	}
	until = now.Truncate(time.Hour).Add(time.Hour)
	sec = int(until.Sub(now).Seconds())
	if sec < 1 {
		sec = 1
	}
	return sec, until
}

func meterLimitMessage(reason string, sec int) string {
	wait := formatWait(sec)
	if reason == "daily" {
		return "Today's limit reached. Try again in " + wait + "."
	}
	return "Hourly limit reached. Try again in " + wait + "."
}

func formatWait(sec int) string {
	if sec < 60 {
		return itoa(sec) + "s"
	}
	min := sec / 60
	if min < 60 {
		rem := sec % 60
		if rem == 0 {
			return itoa(min) + "m"
		}
		return itoa(min) + "m " + itoa(rem) + "s"
	}
	h := min / 60
	m := min % 60
	if m == 0 {
		return itoa(h) + "h"
	}
	return itoa(h) + "h " + itoa(m) + "m"
}
