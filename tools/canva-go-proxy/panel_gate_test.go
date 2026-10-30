package main

import (
	"testing"
	"time"
)

func TestDropPanelSessionsKeepsOtherWebsite(t *testing.T) {
	panelSess.Range(func(key, _ any) bool {
		panelSess.Delete(key)
		return true
	})
	expires := time.Now().Add(time.Hour)
	panelSess.Store("upays", &panelGateSession{username: "sagar1", websiteID: 11, expires: expires})
	panelSess.Store("seogb", &panelGateSession{username: "sagar1", websiteID: 22, expires: expires})
	panelSess.Store("other", &panelGateSession{username: "other", websiteID: 11, expires: expires})

	dropPanelSessionsFor("sagar1", 22)

	if _, ok := panelSess.Load("upays"); !ok {
		t.Fatal("owner host session was dropped")
	}
	if _, ok := panelSess.Load("seogb"); ok {
		t.Fatal("same-host session was not replaced")
	}
	if _, ok := panelSess.Load("other"); !ok {
		t.Fatal("other username was dropped")
	}
}
