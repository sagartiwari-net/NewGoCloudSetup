package main

import "strings"
import "testing"

func TestInjectSemrushBoot(t *testing.T) {
	got := injectSemrushBoot(`<head><title>x</title>`, `<script data-semrush-proxy-boot></script>`)
	if !strings.Contains(got, "<head>\n<script data-semrush-proxy-boot>") {
		t.Fatalf("plain head: %s", got)
	}
	got = injectSemrushBoot(`<HEAD class="x"><title>x</title>`, `<script data-semrush-proxy-boot></script>`)
	if !strings.Contains(got, `<HEAD class="x">`+"\n"+`<script data-semrush-proxy-boot>`) {
		t.Fatalf("attr head: %s", got)
	}
	again := injectSemrushBoot(got, `<script data-semrush-proxy-boot>second</script>`)
	if strings.Count(again, "data-semrush-proxy-boot") != 1 {
		t.Fatalf("injected twice: %s", again)
	}
	if !strings.Contains(loadExportToolJS(), "semrush-export-container") {
		t.Fatal("custom export script missing")
	}
}
