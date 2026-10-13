package main

import (
	"bufio"
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type bufferedConn struct {
	net.Conn
	reader io.Reader
}

func (b *bufferedConn) Read(p []byte) (int, error) { return b.reader.Read(p) }

func proxyWebSocket(w http.ResponseWriter, r *http.Request, acc ToolAccount, base *url.URL) {
	addr := base.Host
	if !strings.Contains(addr, ":") {
		if base.Scheme == "http" {
			addr += ":80"
		} else {
			addr += ":443"
		}
	}
	dialer := &net.Dialer{Timeout: 20 * time.Second}
	var upstream net.Conn
	var err error
	if base.Scheme == "http" {
		upstream, err = dialer.DialContext(r.Context(), "tcp", addr)
	} else {
		upstream, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: base.Hostname(), MinVersion: tls.VersionTLS12})
	}
	if err != nil {
		log.Printf("[WS] dial %s: %v", addr, err)
		http.Error(w, "WebSocket upstream unavailable", http.StatusBadGateway)
		return
	}
	defer upstream.Close()

	jar := cookiesForUpstream(acc, r)
	var req strings.Builder
	req.WriteString("GET " + r.URL.RequestURI() + " HTTP/1.1\r\n")
	req.WriteString("Host: " + base.Host + "\r\n")
	req.WriteString("Upgrade: websocket\r\n")
	req.WriteString("Connection: Upgrade\r\n")
	for _, name := range []string{"Sec-WebSocket-Key", "Sec-WebSocket-Version", "Sec-WebSocket-Extensions", "Sec-WebSocket-Protocol"} {
		if v := r.Header.Get(name); v != "" {
			req.WriteString(name + ": " + v + "\r\n")
		}
	}
	if jar != "" {
		req.WriteString("Cookie: " + jar + "\r\n")
	}
	ua := r.UserAgent()
	if ua == "" {
		ua = "Mozilla/5.0"
	}
	req.WriteString("User-Agent: " + ua + "\r\n")
	req.WriteString("Origin: " + base.Scheme + "://" + base.Host + "\r\n")
	req.WriteString("\r\n")
	if _, err = io.WriteString(upstream, req.String()); err != nil {
		log.Printf("[WS] write %s: %v", r.URL.Path, err)
		http.Error(w, "WebSocket upstream write failed", http.StatusBadGateway)
		return
	}

	reader := bufio.NewReader(upstream)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		log.Printf("[WS] read %s: %v", r.URL.Path, err)
		http.Error(w, "WebSocket upstream response failed", http.StatusBadGateway)
		return
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		resp.Body.Close()
		log.Printf("[WS] rejected %s status=%d", r.URL.Path, resp.StatusCode)
		http.Error(w, "WebSocket upstream rejected", http.StatusBadGateway)
		return
	}
	resp.Body.Close()

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "WebSocket hijack unsupported", http.StatusInternalServerError)
		return
	}
	client, clientReader, err := hj.Hijack()
	if err != nil {
		log.Printf("[WS] hijack %s: %v", r.URL.Path, err)
		return
	}
	defer client.Close()

	var head strings.Builder
	head.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	for key, values := range resp.Header {
		for _, value := range values {
			head.WriteString(key + ": " + value + "\r\n")
		}
	}
	head.WriteString("\r\n")
	if _, err = io.WriteString(client, head.String()); err != nil {
		log.Printf("[WS] client %s: %v", r.URL.Path, err)
		return
	}

	var up io.ReadWriter = upstream
	if reader.Buffered() > 0 {
		up = &bufferedConn{Conn: upstream, reader: io.MultiReader(reader, upstream)}
	}
	var down io.ReadWriter = client
	if clientReader.Reader.Buffered() > 0 {
		down = &bufferedConn{Conn: client, reader: io.MultiReader(clientReader.Reader, client)}
	}
	log.Printf("[WS] open %s", r.URL.Path)
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(up, down); done <- struct{}{} }()
	go func() { _, _ = io.Copy(down, up); done <- struct{}{} }()
	<-done
	log.Printf("[WS] closed %s", r.URL.Path)
}
