package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"testing"
)

func gzipBytes(parts ...string) []byte {
	var buf bytes.Buffer
	for _, part := range parts {
		w := gzip.NewWriter(&buf)
		_, _ = io.WriteString(w, part)
		_ = w.Close()
	}
	return buf.Bytes()
}

func TestReadProxyBodyJoinedGzipMembers(t *testing.T) {
	raw := gzipBytes(`1:"$Sreact.fragment"`+"\n", `0:{"f":[]}`+"\n")
	resp := &http.Response{
		Header: http.Header{"Content-Encoding": []string{"gzip"}},
		Body:   io.NopCloser(bytes.NewReader(raw)),
	}
	plain, wasGzip, err := readProxyBody(resp)
	if err != nil {
		t.Fatal(err)
	}
	if !wasGzip {
		t.Fatal("expected gzip")
	}
	got := string(plain)
	if !strings.Contains(got, `$Sreact.fragment`) || !strings.Contains(got, `{"f":[]}`) {
		t.Fatalf("joined members missing: %q", got)
	}
}

func TestReadProxyBodyPlainLabeledGzip(t *testing.T) {
	const flight = "1:\"$Sreact.fragment\"\n"
	resp := &http.Response{
		Header: http.Header{"Content-Encoding": []string{"gzip"}},
		Body:   io.NopCloser(strings.NewReader(flight)),
	}
	plain, wasGzip, err := readProxyBody(resp)
	if err != nil {
		t.Fatal(err)
	}
	if wasGzip {
		t.Fatal("plain flight must not be marked gzip")
	}
	if string(plain) != flight {
		t.Fatalf("got %q", plain)
	}
	if resp.Header.Get("Content-Encoding") != "" {
		t.Fatal("gzip header left on plain body")
	}
}

func TestRewriteFlightHostsIsNoop(t *testing.T) {
	in := `12:https://app.neilpatel.com/en`
	got := rewriteFlightHosts(in, "app.neilpatel.com", "https://brsg.upays.in", "brsg.upays.in")
	if got != in {
		t.Fatalf("flight must stay exact, got %q", got)
	}
}

func TestRewriteUberHostsOutsideScripts(t *testing.T) {
	in := `<a href="https://ubersuggest.com/en">x</a><script>12:https://ubersuggest.com/en</script>`
	got := rewriteUberHostsOutsideScripts(in, "https://brsg.upays.in", "brsg.upays.in", uberUpstreamHosts("ubersuggest.com"))
	if !strings.Contains(got, `href="https://brsg.upays.in/en"`) {
		t.Fatalf("markup not rewritten: %s", got)
	}
	if !strings.Contains(got, `12:https://ubersuggest.com/en`) {
		t.Fatalf("script body was rewritten: %s", got)
	}
}
