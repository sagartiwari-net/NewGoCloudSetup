package main

import (
	"testing"
	"time"
)

func TestDeviceHeaderMatchesDuplicateStamp(t *testing.T) {
	if !deviceHeaderMatches("abc", "abc") {
		t.Fatal("exact match")
	}
	if !deviceHeaderMatches("abc, abc", "abc") {
		t.Fatal("xhr duplicate stamp")
	}
	if deviceHeaderMatches("other", "abc") {
		t.Fatal("different proof must not match")
	}
	if deviceHeaderMatches("abc, other", "abc") {
		t.Fatal("mixed values are a copied session")
	}
}

func TestDropPanelSessionsKeepsOtherWebsite(t *testing.T) {
	panelSess.Range(func(key, _ any) bool {
		panelSess.Delete(key)
		return true
	})
	owner := &panelGateSession{username: "sagar", websiteID: 11, expires: time.Now().Add(time.Hour)}
	friend := &panelGateSession{username: "sagar", websiteID: 22, expires: time.Now().Add(time.Hour)}
	other := &panelGateSession{username: "amit", websiteID: 22, expires: time.Now().Add(time.Hour)}
	panelSess.Store("owner-token", owner)
	panelSess.Store("friend-token", friend)
	panelSess.Store("other-token", other)

	dropPanelSessionsFor("sagar", 22)

	if _, ok := panelSess.Load("owner-token"); !ok {
		t.Fatal("owner website session was dropped")
	}
	if _, ok := panelSess.Load("friend-token"); ok {
		t.Fatal("same-website session should be replaced")
	}
	if _, ok := panelSess.Load("other-token"); !ok {
		t.Fatal("other username was dropped")
	}
}

func TestDropPanelSessionsSkipsZeroWebsite(t *testing.T) {
	panelSess.Range(func(key, _ any) bool {
		panelSess.Delete(key)
		return true
	})
	owner := &panelGateSession{username: "sagar", websiteID: 11, expires: time.Now().Add(time.Hour)}
	panelSess.Store("owner-token", owner)
	dropPanelSessionsFor("sagar", 0)
	if _, ok := panelSess.Load("owner-token"); !ok {
		t.Fatal("zero website id must not sign out other websites")
	}
}
