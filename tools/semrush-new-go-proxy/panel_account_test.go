package main

import "testing"

func TestSemrushChooseLeastBusySpreadsUsers(t *testing.T) {
	rows := []semrushBusyRow{{ID: 10, Live: 0}, {ID: 20, Live: 0}, {ID: 30, Live: 0}}
	got := make([]int, 0, 6)
	for i := 0; i < 6; i++ {
		id := semrushChooseLeastBusy(rows, nil)
		if id == 0 {
			t.Fatal("expected an account")
		}
		got = append(got, id)
		for j := range rows {
			if rows[j].ID == id {
				rows[j].Live++
			}
		}
	}
	want := []int{10, 20, 30, 10, 20, 30}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("assignment %d: got %v want %v", i, got, want)
		}
	}
}

func TestSemrushChooseLeastBusySkipsLoggedOut(t *testing.T) {
	rows := []semrushBusyRow{{ID: 10, Live: 2}, {ID: 20, Live: 0}, {ID: 30, Live: 1}}
	got := semrushChooseLeastBusy(rows, map[int]bool{20: true})
	if got != 30 {
		t.Fatalf("got %d want 30", got)
	}
}

func TestSemrushChooseLeastBusyAfterAccountRemoved(t *testing.T) {
	rows := []semrushBusyRow{{ID: 10, Live: 2}, {ID: 30, Live: 0}}
	got := semrushChooseLeastBusy(rows, nil)
	if got != 30 {
		t.Fatalf("got %d want 30", got)
	}
}
