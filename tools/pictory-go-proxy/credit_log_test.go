package main

import (
	"strings"
	"testing"
)

func TestPictoryBodySummaryKeepsCreditNumbersAndHidesText(t *testing.T) {
	body := []byte(`{"email":"person@example.com","idToken":"abc.def.ghi","text":"one two three","aiCredits":600,"genAiCreditsUsed":12.5,"quota":{"videoSeconds":180}}`)
	got := pictoryBodySummary(body)
	for _, want := range []string{"aiCredits=600", "genAiCreditsUsed=12.5", "quota.videoSeconds=180", "words=3"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
	for _, hidden := range []string{"example.com", "abc.def", "one two"} {
		if strings.Contains(got, hidden) {
			t.Fatalf("leaked %s in %s", hidden, got)
		}
	}
}

func TestShouldLogPictoryUseSkipsStaticAndLogsSpend(t *testing.T) {
	if shouldLogPictoryUse("GET", "front-end-prod.pictory.ai", "/static/js/main.js", nil) {
		t.Fatal("static js should not log")
	}
	if shouldLogPictoryUse("GET", "api.amplitude.com", "/track", []byte(`{"ok":true}`)) {
		t.Fatal("analytics should not log")
	}
	if !shouldLogPictoryUse("POST", "api.pictory.ai", "/pictoryapis/v1/video", []byte(`{"ok":true}`)) {
		t.Fatal("post should log")
	}
	if !shouldLogPictoryUse("GET", "api.pictory.ai", "/user", []byte(`{"currentQuota":{"aiCredits":10}}`)) {
		t.Fatal("quota response should log")
	}
	if shouldLogPictoryUse("GET", "api.pictory.ai", "/projects", []byte(`{"items":[{"id":"p1"}]}`)) {
		t.Fatal("plain project list should not log")
	}
}
