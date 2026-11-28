package main

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func requestFingerprint(r *http.Request) string {
	return strings.TrimSpace(headerOr(r, "X-TM-Bar-FP", "X-Tb-FP"))
}

// bindOrCheckFingerprint enforces session↔device binding when RequireFingerprint is on.
// Returns false when the caller must reject the request.
func (s *server) bindOrCheckFingerprint(sess *barSession, fp string) (ok bool, code string) {
	fp = strings.TrimSpace(fp)
	if !s.cfg.RequireFingerprint {
		if fp != "" && sess.Fingerprint == "" {
			_, _ = s.db.Exec(`UPDATE ahrefs_bar_sessions SET fingerprint = ? WHERE token = ?`, fp, sess.Token)
			sess.Fingerprint = fp
		}
		return true, ""
	}
	if fp == "" {
		return false, "fingerprint_required"
	}
	if sess.Fingerprint == "" {
		_, _ = s.db.Exec(`UPDATE ahrefs_bar_sessions SET fingerprint = ? WHERE token = ?`, fp, sess.Token)
		sess.Fingerprint = fp
		return true, ""
	}
	if !fingerprintEqual(sess.Fingerprint, fp) {
		return false, "fingerprint_mismatch"
	}
	return true, ""
}

func fingerprintEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// lookupSessionChecked loads session and enforces fingerprint header.
func (s *server) lookupSessionChecked(r *http.Request, token string) (*barSession, string, error) {
	sess, err := s.lookupSession(token)
	if err != nil {
		return nil, "InternalError", err
	}
	if sess == nil {
		return nil, "Unauthorized", nil
	}
	ok, code := s.bindOrCheckFingerprint(sess, requestFingerprint(r))
	if !ok {
		return nil, code, nil
	}
	return sess, "", nil
}
