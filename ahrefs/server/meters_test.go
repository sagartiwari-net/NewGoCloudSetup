package main

import (
	"path/filepath"
	"testing"
)

func TestChargeMetersHourlyCap(t *testing.T) {
	dir := t.TempDir()
	cfg := config{
		SQLitePath: filepath.Join(dir, "t.db"),
		Timezone:   "Asia/Kolkata",
		DevAuth:    true,
	}
	db, driver, err := openDB(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrate(db, driver); err != nil {
		t.Fatal(err)
	}
	s := &server{cfg: cfg, db: db, driver: driver}
	if err := s.setUserCustomLimits(1, "u1", 3, 100); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		ok, _, _, err := s.chargeMeters(1, "u1", 1)
		if err != nil || !ok {
			t.Fatalf("charge %d failed ok=%v err=%v", i+1, ok, err)
		}
	}
	ok, snap, reason, err := s.chargeMeters(1, "u1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if ok || reason != "hourly" {
		t.Fatalf("expected hourly block ok=%v reason=%s snap=%+v", ok, reason, snap)
	}
}
