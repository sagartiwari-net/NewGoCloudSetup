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

func TestRewriteFlightHostsSkipsBareSlash(t *testing.T) {
	in := `href":"https://app.neilpatel.com/en" "/app.neilpatel.com-token"`
	got := rewriteFlightHosts(in, "app.neilpatel.com", "https://ubersuggest.gt4rents.com", "ubersuggest.gt4rents.com")
	if !strings.Contains(got, "https://ubersuggest.gt4rents.com/en") {
		t.Fatalf("origin not rewritten: %s", got)
	}
	if !strings.Contains(got, "/app.neilpatel.com-token") {
		t.Fatalf("bare token was rewritten: %s", got)
	}
}
