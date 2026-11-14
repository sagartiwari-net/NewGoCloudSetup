package main

import "testing"

func TestIsBlockedPathHiddenTools(t *testing.T) {
	blocked := []string{
		"/ai-grader",
		"/ai-grader/",
		"/ai-grader/foo",
		"/content-helper",
		"/content-helper?x=1",
		"/content-helper/bar",
		"/bot-analytics",
		"/bot-analytics/",
		"/bot-analytics/overview",
		"/account/settings",
		"/academy",
	}
	for _, p := range blocked {
		if !isBlockedPath(p) {
			t.Fatalf("expected blocked: %s", p)
		}
	}
	allowed := []string{
		"/site-explorer",
		"/site-explorer/overview",
		"/keywords-explorer",
		"/content-explorer",
		"/v4/someApi",
	}
	for _, p := range allowed {
		if isBlockedPath(p) {
			t.Fatalf("expected allowed: %s", p)
		}
	}
}
