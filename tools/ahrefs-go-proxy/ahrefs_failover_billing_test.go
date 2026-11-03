package main

import "testing"

func TestAhrefsLooksLikeCSV(t *testing.T) {
	csv := []byte("Keyword,Volume\nai,100\n")
	if !ahrefsLooksLikeCSV("text/csv", csv) {
		t.Fatal("text/csv")
	}
	if !ahrefsLooksLikeCSV("application/json", append([]byte{0xFF, 0xFE}, csv...)) {
		t.Fatal("utf16 bom")
	}
	jsonBody := []byte("{\n\"ok\":true,\n\"rows\":1\n}\n")
	if ahrefsLooksLikeCSV("application/json", jsonBody) {
		t.Fatal("json must not look like csv")
	}
	htmlBody := []byte("<!doctype html>\n<html></html>\n")
	if ahrefsLooksLikeCSV("text/html", htmlBody) {
		t.Fatal("html must not look like csv")
	}
}

func TestAhrefsExportBillAmount(t *testing.T) {
	csv := []byte("Keyword,Volume\n" + "a,1\nb,2\n")
	// 2 data rows
	amount, ok := ahrefsExportBillAmount(0, "text/csv; charset=utf-8", csv)
	if !ok || amount != 2 {
		t.Fatalf("rows amount=%d ok=%v", amount, ok)
	}
	amount, ok = ahrefsExportBillAmount(200, "text/csv", csv)
	if !ok || amount != 2 {
		t.Fatalf("partial ask amount=%d ok=%v", amount, ok)
	}
	// Build ~400 data rows with ask 200 → treat as double-count, bill 200.
	var b []byte
	b = append(b, []byte("Keyword,Volume\n")...)
	for i := 0; i < 400; i++ {
		b = append(b, 'x', ',', '1', '\n')
	}
	amount, ok = ahrefsExportBillAmount(200, "text/csv", b)
	if !ok || amount != 200 {
		t.Fatalf("double-count guard amount=%d ok=%v", amount, ok)
	}
	amount, ok = ahrefsExportBillAmount(200, "application/json", []byte(`{"size":200}`+"\n\n\n"))
	if ok || amount != 0 {
		t.Fatalf("json export must not bill amount=%d ok=%v", amount, ok)
	}
}

func TestAhrefsRecentCreditRevertRequiresCharge(t *testing.T) {
	// No charge recorded → revert is a no-op (must not panic / invent credits).
	ahrefsRevertRecentCreditOnSwitch("nobody-here-for-test")
}

func TestAhrefsTableExportRewrap(t *testing.T) {
	if !ahrefsTableExportRewrap("google_us_chatgpt_matching-terms_2026-11-03_08-41-51.csv", "/keywords-explorer/google/us/ideas") {
		t.Fatal("matching-terms on KE must skip second bill")
	}
	if !ahrefsTableExportRewrap("", "/keywords-explorer/google/us/overview") {
		t.Fatal("keywords-explorer page must skip second bill")
	}
	if ahrefsTableExportRewrap("ahrefs-chart.csv", "/site-explorer/overview") {
		t.Fatal("real chart export must still bill")
	}
}

func TestExportFilenameFromContentDisposition(t *testing.T) {
	got := exportFilenameFromContentDisposition(`attachment; filename="google_us_chatgpt_matching-terms_2026-11-03_08-41-51.csv"`)
	if got != "google_us_chatgpt_matching-terms_2026-11-03_08-41-51.csv" {
		t.Fatalf("got %q", got)
	}
}
