package main

import (
	"testing"
	"time"
)

func TestLoginGateBlocksRepeatedFailures(t *testing.T) {
	gate := newLoginGate()
	start := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	gate.now = func() time.Time { return start }
	for i := 0; i < loginFailLimit; i++ {
		if !gate.allow("1.2.3.4") {
			t.Fatalf("attempt %d was blocked early", i+1)
		}
		gate.fail("1.2.3.4")
	}
	if gate.allow("1.2.3.4") {
		t.Fatal("ip stayed open after repeated failures")
	}
	if !gate.allow("5.6.7.8") {
		t.Fatal("a different ip was blocked")
	}
	gate.now = func() time.Time { return start.Add(loginFailWindow + time.Second) }
	if !gate.allow("1.2.3.4") {
		t.Fatal("ip stayed blocked after the window")
	}
}

func TestLoginGateSuccessClearsFailures(t *testing.T) {
	gate := newLoginGate()
	gate.now = func() time.Time { return time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC) }
	for i := 0; i < loginFailLimit-1; i++ {
		if !gate.allow("1.2.3.4") {
			t.Fatal("blocked before the limit")
		}
		gate.fail("1.2.3.4")
	}
	gate.success("1.2.3.4")
	gate.now = func() time.Time { return time.Date(2026, 10, 18, 12, 0, 2, 0, time.UTC) }
	for i := 0; i < loginFailLimit-1; i++ {
		if !gate.allow("1.2.3.4") {
			t.Fatal("success did not clear the failure count")
		}
		gate.fail("1.2.3.4")
	}
	if !gate.allow("1.2.3.4") {
		t.Fatal("ip blocked one failure too soon")
	}
}

func TestLoginGateCapsRequestRate(t *testing.T) {
	gate := newLoginGate()
	start := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	for i := 0; i < loginIPPerMinute; i++ {
		at := start.Add(time.Duration(i/loginGlobalPerSecond) * time.Second)
		gate.now = func(stamp time.Time) func() time.Time {
			return func() time.Time { return stamp }
		}(at)
		if !gate.allow("9.9.9.9") {
			t.Fatalf("request %d was capped early", i+1)
		}
	}
	if gate.allow("9.9.9.9") {
		t.Fatal("ip exceeded the per-minute cap")
	}
}
