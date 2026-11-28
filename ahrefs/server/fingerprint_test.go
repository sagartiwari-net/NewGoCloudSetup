package main

import "testing"

func TestFingerprintEqual(t *testing.T) {
	if !fingerprintEqual("abc", "abc") {
		t.Fatal("equal")
	}
	if fingerprintEqual("abc", "abd") {
		t.Fatal("mismatch")
	}
	if fingerprintEqual("ab", "abc") {
		t.Fatal("len")
	}
}

func TestBindOrCheckFingerprint(t *testing.T) {
	s := &server{cfg: config{RequireFingerprint: true}, db: nil}
	// without db, empty-bind path will panic on Exec — use mismatch path with pre-set FP
	sess := &barSession{Token: "tb.test", Fingerprint: "tmfp.aaa"}
	ok, code := s.bindOrCheckFingerprint(sess, "")
	if ok || code != "fingerprint_required" {
		t.Fatalf("want required got ok=%v code=%s", ok, code)
	}
	ok, code = s.bindOrCheckFingerprint(sess, "tmfp.bbb")
	if ok || code != "fingerprint_mismatch" {
		t.Fatalf("want mismatch got ok=%v code=%s", ok, code)
	}
	ok, code = s.bindOrCheckFingerprint(sess, "tmfp.aaa")
	if !ok || code != "" {
		t.Fatalf("want match got ok=%v code=%s", ok, code)
	}
}

func TestBindOptionalOff(t *testing.T) {
	s := &server{cfg: config{RequireFingerprint: false}}
	sess := &barSession{Token: "tb.x", Fingerprint: "bound"}
	ok, _ := s.bindOrCheckFingerprint(sess, "other")
	if !ok {
		t.Fatal("optional mode should allow mismatch")
	}
}
