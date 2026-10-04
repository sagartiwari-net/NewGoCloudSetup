package main

import (
	"strings"
	"testing"
)

func TestForceGPTZeroPremiumPlanProfile(t *testing.T) {
	in := []byte(`{"id":"u1","email":"a@b.com","plan":"Free"}`)
	out := forceGPTZeroPremiumPlan("/rest/v1/profiles?id=eq.u1", in)
	if !strings.Contains(string(out), "Premium (Annual)") {
		t.Fatalf("expected Premium plan, got %s", out)
	}
	if !strings.Contains(string(out), "full_plan") {
		t.Fatalf("expected full_plan injection, got %s", out)
	}
}

func TestForceGPTZeroPremiumPlanIgnoresScanJSON(t *testing.T) {
	in := []byte(`{"scanType":"plagiarism","plan":"Free"}`)
	out := forceGPTZeroPremiumPlan("/v3/scan", in)
	if string(out) != string(in) {
		t.Fatalf("scan JSON should be unchanged, got %s", out)
	}
}

func TestNormalizeGPTZeroPlan(t *testing.T) {
	if normalizeGPTZeroPlan("Free") != "Premium (Annual)" {
		t.Fatal("free")
	}
	if normalizeGPTZeroPlan("Premium") != "Premium (Annual)" {
		t.Fatal("premium")
	}
}

func TestStubSupabaseFeatures(t *testing.T) {
	st, body, ok := maybeStubGPTZeroFeatures("/extra-cdn-0/rest/v1/features", 401, []byte(`{"message":"JWT expired"}`))
	if !ok || st != 200 || !strings.Contains(string(body), "generally_available") {
		t.Fatalf("features stub failed st=%d ok=%v body=%s", st, ok, body)
	}
	st, body, ok = maybeStubGPTZeroFeatures("/extra-cdn-0/rest/v1/features_access?email=eq.a", 401, []byte(`{"message":"JWT expired"}`))
	if !ok || st != 200 || string(body) != "[]" {
		t.Fatalf("features_access stub failed st=%d ok=%v body=%s", st, ok, body)
	}
	if gptzeroLooksLoggedOut(401, "/extra-cdn-0/rest/v1/features", []byte(`{"message":"JWT expired"}`)) {
		t.Fatal("features 401 must not trigger account swap")
	}
}

func TestBuildInjectUpsellDismissAndLayoutGuards(t *testing.T) {
	s := buildInject(
		Config{PublicHost: "gptzero.gt4rents.com", HomePath: "/", BlockedPaths: []string{"/login"}},
		map[string]string{"plan": "Free"},
		"Free",
		"tok",
		"http://gptzero.gt4rents.com",
		"sagar",
	)
	need := []string{
		"dismissUpsells",
		"upgrade-plan-modal-close-button",
		`data-testid="upgrade-plan-modal"`,
		"gzIsLayoutChrome",
		"gzUnhideLayout",
		"Premium (Annual)",
		"trialSubscriptionUsed",
	}
	for _, n := range need {
		if !strings.Contains(s, n) {
			t.Fatalf("inject missing %q", n)
		}
	}
	// Free plan must be upgraded in inject PLAN constant.
	if strings.Contains(s, `var PLAN = "Free"`) {
		t.Fatal("PLAN should not stay Free")
	}
}
