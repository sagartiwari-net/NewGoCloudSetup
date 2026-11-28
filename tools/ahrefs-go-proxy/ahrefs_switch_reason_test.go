package main

import (
	"strings"
	"testing"
)

func TestAhrefsSwitchReasonFreePlanWall(t *testing.T) {
	body := strings.ToLower(`Domain not verified
You can only check verified domains and their subdomains on a free plan.
Please verify your ownership of this website, or subscribe to a premium plan to use all Ahrefs tools for any website.
Verify ownership
See pricing`)
	reason, ok := ahrefsSwitchReasonFromBody(body)
	if !ok {
		t.Fatal("expected free-plan wall to trigger switch")
	}
	if reason != "domain not verified / plan expired" {
		t.Fatalf("reason=%q", reason)
	}
	if ahrefsUsableResult([]byte(body)) {
		t.Fatal("free-plan wall must not count as usable result")
	}
}

func TestAhrefsSwitchReasonNormalPage(t *testing.T) {
	body := strings.ToLower(`Site Explorer overview for example.com domain rating organic traffic`)
	if _, ok := ahrefsSwitchReasonFromBody(body); ok {
		t.Fatal("normal overview must not switch")
	}
}
