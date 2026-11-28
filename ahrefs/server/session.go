package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const sessionTokenPrefix = "tb."

type barSession struct {
	Token           string
	ResellerID      int
	PortalID        sql.NullInt64
	AmemberLogin    string
	Email           string
	Name            string
	UID             int64
	Fingerprint     string
	ExpiresAt       time.Time
	PortalURL       string
	StickyAccountID sql.NullInt64
	StickyUntil     sql.NullString
}

func isBarSessionToken(token string) bool {
	return strings.HasPrefix(token, sessionTokenPrefix) && len(token) >= 20
}

// dbTime formats timestamps for the active driver. MySQL DATETIME rejects RFC3339
// (…T…Z) under STRICT mode — that surfaced as mint_failed after tm-check OK.
func (s *server) dbTime(t time.Time) string {
	t = t.UTC()
	if s.driver == "mysql" {
		return t.Format("2006-01-02 15:04:05")
	}
	return t.Format(time.RFC3339)
}

func (s *server) lookupSession(token string) (*barSession, error) {
	if !isBarSessionToken(token) {
		return nil, nil
	}
	row := s.db.QueryRow(`
SELECT token, reseller_id, portal_id, amember_login,
       COALESCE(email,''), COALESCE(name,''), COALESCE(uid,0),
       fingerprint, expires_at, COALESCE(portal_url,''),
       sticky_account_id, sticky_until
FROM ahrefs_bar_sessions
WHERE token = ?`, token)

	var sess barSession
	var expStr string
	if err := row.Scan(&sess.Token, &sess.ResellerID, &sess.PortalID, &sess.AmemberLogin,
		&sess.Email, &sess.Name, &sess.UID, &sess.Fingerprint, &expStr, &sess.PortalURL,
		&sess.StickyAccountID, &sess.StickyUntil); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	exp, err := parseDBTime(expStr)
	if err != nil {
		return nil, err
	}
	if time.Now().After(exp) {
		return nil, nil
	}
	sess.ExpiresAt = exp
	_, _ = s.db.Exec(`UPDATE ahrefs_bar_sessions SET last_seen = ? WHERE token = ?`, s.dbTime(time.Now()), token)
	return &sess, nil
}

func (s *server) mintSession(resellerID int, portalID int64, portalURL, login, email, name string, uid int64, fingerprint string) (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := sessionTokenPrefix + hex.EncodeToString(b)
	now := time.Now().UTC()
	exp := now.Add(s.cfg.SessionTTL)
	if name == "" {
		name = login
	}
	_, err := s.db.Exec(`
INSERT INTO ahrefs_bar_sessions
  (token, reseller_id, portal_id, amember_login, email, name, uid, fingerprint, expires_at, last_seen, portal_url, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		token, resellerID, nullInt64(portalID), login, email, name, uid, fingerprint,
		s.dbTime(exp), s.dbTime(now), portalURL, s.dbTime(now),
	)
	if err != nil {
		return "", fmt.Errorf("insert session: %w", err)
	}
	// opportunistic cleanup
	_, _ = s.db.Exec(`DELETE FROM ahrefs_bar_sessions WHERE expires_at < ?`, s.dbTime(now.Add(-24*time.Hour)))
	return token, nil
}

func (s *server) revokeSession(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM ahrefs_bar_sessions WHERE token = ?`, token)
	return err
}

func nullInt64(v int64) any {
	if v <= 0 {
		return nil
	}
	return v
}

func parseDBTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	formats := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		time.RFC3339Nano,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, raw); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("bad time: %s", raw)
}

func (s *server) resolveLimits(resellerID int, login string) (hourly, daily, hourlyUsed, dailyUsed int, err error) {
	hourly, daily = 50, 150
	_ = s.db.QueryRow(`SELECT hourly_limit, daily_limit FROM ahrefs_bar_reseller_settings WHERE reseller_id = ?`, resellerID).
		Scan(&hourly, &daily)

	var ch, cd int
	var custom int
	err = s.db.QueryRow(`
SELECT hourly_limit, daily_limit, is_custom FROM ahrefs_bar_user_limits
WHERE reseller_id = ? AND amember_login = ?`, resellerID, login).Scan(&ch, &cd, &custom)
	if err == nil && custom == 1 {
		hourly, daily = ch, cd
	} else if err != nil && err != sql.ErrNoRows {
		return 0, 0, 0, 0, err
	}

	// Rolling 60 minutes usage
	since := time.Now().UTC().Add(-60 * time.Minute).Format(time.RFC3339)
	_ = s.db.QueryRow(`
SELECT COALESCE(SUM(used),0) FROM ahrefs_bar_usage_hourly
WHERE reseller_id = ? AND amember_login = ? AND bucket_start >= ?`, resellerID, login, since).Scan(&hourlyUsed)

	dayIST := time.Now().In(mustLoc(s.cfg.Timezone)).Format("2006-01-02")
	_ = s.db.QueryRow(`
SELECT COALESCE(used,0) FROM ahrefs_bar_usage_daily
WHERE reseller_id = ? AND amember_login = ? AND day_ist = ?`, resellerID, login, dayIST).Scan(&dailyUsed)

	return hourly, daily, hourlyUsed, dailyUsed, nil
}

func mustLoc(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.FixedZone("IST", 5*3600+30*60)
	}
	return loc
}
