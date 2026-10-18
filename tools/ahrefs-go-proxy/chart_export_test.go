package main

import "testing"

func TestChartExportCSV(t *testing.T) {
	raw := []byte("Metric,Referring domains\n2026-09-18,244\n2026-09-19,637\n")
	if got := ahrefsCSVRows(raw); got != 2 {
		t.Fatalf("rows=%d", got)
	}
	out := withUTF8CSVBom(raw)
	if len(out) != len(raw)+3 || out[0] != 0xEF || out[1] != 0xBB || out[2] != 0xBF {
		t.Fatalf("missing utf-8 bom")
	}
	again := withUTF8CSVBom(out)
	if len(again) != len(out) {
		t.Fatalf("bom added twice")
	}
	utf16 := []byte{0xFF, 0xFE, 'A', 0x00}
	if got := withUTF8CSVBom(utf16); len(got) != len(utf16) {
		t.Fatalf("utf-16 should stay as-is")
	}
}
