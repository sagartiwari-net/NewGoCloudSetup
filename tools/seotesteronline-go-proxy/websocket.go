package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
)

func isWebSocket(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") &&
		strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

// buildWSPath builds the upstream WebSocket path (query string preserved).
// SEOTester auth is JWT in localStorage.token (Bearer); WS uses browser hydrate.
func buildWSPath(r *http.Request, targetPath string, cfg Config, session string) string {
	wsPath := targetPath
	if r.URL.RawQuery != "" {
		if strings.Contains(wsPath, "?") {
			wsPath += "&" + r.URL.RawQuery
		} else {
			wsPath += "?" + r.URL.RawQuery
		}
	}
	return wsPath
}

func handleWebSocket(w http.ResponseWriter, r *http.Request, cfg Config, targetHost string, targetPath string, getSession func() string) {
	targetAddr := targetHost + ":443"
	conn, err := dialChromeHTTP1(r.Context(), targetAddr)
	if err != nil {
		log.Printf("[WS] dial failed %s: %v", targetHost, err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	session := ""
	if getSession != nil {
		session = getSession()
	}

	zgOrigin := upstreamOrigin(cfg)
	wsPath := buildWSPath(r, targetPath, cfg, session)

	// Get Sec-WebSocket-Key from browser (case-insensitive header lookup)
	wsKey := r.Header.Get("Sec-Websocket-Key")
	if wsKey == "" {
		wsKey = r.Header.Get("Sec-WebSocket-Key")
	}

	wsProto := r.Header.Get("Sec-Websocket-Protocol")
	if wsProto == "" {
		wsProto = r.Header.Get("Sec-WebSocket-Protocol")
	}

	// Build the upstream WebSocket upgrade request with proper Chrome-like headers
	var reqBuf bytes.Buffer
	fmt.Fprintf(&reqBuf, "GET %s HTTP/1.1\r\n", wsPath)
	fmt.Fprintf(&reqBuf, "Host: %s\r\n", targetHost)

	// --- Critical WebSocket headers ---
	if wsKey != "" {
		fmt.Fprintf(&reqBuf, "Sec-WebSocket-Key: %s\r\n", wsKey)
	}
	fmt.Fprintf(&reqBuf, "Sec-WebSocket-Version: 13\r\n")

	// Forward subprotocol if browser sent one
	if wsProto != "" {
		fmt.Fprintf(&reqBuf, "Sec-WebSocket-Protocol: %s\r\n", wsProto)
	}

	// IMPORTANT: Do NOT forward permessage-deflate extension.
	// The proxy acts as a raw TCP tunnel - if we negotiate compression,
	// the browser and upstream may use different compression contexts
	// causing data corruption. Keep it uncompressed for reliable proxying.
	// (We intentionally omit Sec-WebSocket-Extensions)

	// --- Required upgrade headers ---
	fmt.Fprintf(&reqBuf, "Connection: Upgrade\r\n")
	fmt.Fprintf(&reqBuf, "Upgrade: websocket\r\n")

	// --- Origin ---
	fmt.Fprintf(&reqBuf, "Origin: %s\r\n", zgOrigin)

	// --- Referer ---
	ref := r.Header.Get("Referer")
	if ref == "" {
		ref = zgOrigin + "/"
	} else {
		ref = strings.ReplaceAll(ref, proxyOrigin(cfg), zgOrigin)
	}
	fmt.Fprintf(&reqBuf, "Referer: %s\r\n", ref)

	// --- Chrome-like browser headers ---
	ua := cfg.UserAgent
	if ua == "" {
		ua = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	}
	fmt.Fprintf(&reqBuf, "User-Agent: %s\r\n", ua)
	fmt.Fprintf(&reqBuf, "Accept-Language: en-US,en;q=0.9\r\n")
	fmt.Fprintf(&reqBuf, "Cache-Control: no-cache\r\n")
	fmt.Fprintf(&reqBuf, "Pragma: no-cache\r\n")

	// Cookie: session file only — never merge browser localhost jar
	// (shared across ports → huge SketchGenius/etc cookies break WS upgrade).
	if cookieHdr, _ := parseSessionStorage(session); cookieHdr != "" && len(cookieHdr) < 4096 {
		fmt.Fprintf(&reqBuf, "Cookie: %s\r\n", cookieHdr)
	}

	reqBuf.WriteString("\r\n")

	if cfg.DebugLog {
		log.Printf("[WS] upstream request to %s%s:\n%s", targetHost, wsPath, reqBuf.String())
	}

	if _, err = conn.Write(reqBuf.Bytes()); err != nil {
		conn.Close()
		log.Printf("[WS] handshake write failed: %v", err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, r)
	if err != nil {
		conn.Close()
		log.Printf("[WS] handshake read failed: %v", err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	if resp.StatusCode != http.StatusSwitchingProtocols {
		// Read and log the error body for debugging
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		log.Printf("[WS] upgrade rejected %s%s -> %d | body: %s",
			targetHost, wsPath, resp.StatusCode, string(body))

		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		conn.Close()
		return
	}

	// --- Build the 101 response to send back to the browser client ---
	// We must strip the Sec-WebSocket-Extensions from the upstream response
	// so the browser doesn't try to decompress data that isn't compressed.
	resp.Header.Del("Sec-Websocket-Extensions")
	resp.Header.Del("Sec-WebSocket-Extensions")

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		resp.Body.Close()
		conn.Close()
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		resp.Body.Close()
		conn.Close()
		log.Printf("[WS] hijack failed: %v", err)
		return
	}

	var respBuf bytes.Buffer
	if err := resp.Write(&respBuf); err != nil {
		clientConn.Close()
		conn.Close()
		log.Printf("[WS] serialize upgrade failed: %v", err)
		return
	}
	resp.Body.Close()

	if _, err = clientConn.Write(respBuf.Bytes()); err != nil {
		clientConn.Close()
		conn.Close()
		log.Printf("[WS] write upgrade to client failed: %v", err)
		return
	}

	log.Printf("[WS] tunnel OK %s%s", targetHost, wsPath)
	upstream := &bufferedConn{Conn: conn, r: br}
	errChan := make(chan error, 2)
	go func() { _, err := io.Copy(upstream, clientConn); errChan <- err }()
	go func() { _, err := io.Copy(clientConn, upstream); errChan <- err }()
	<-errChan
	clientConn.Close()
	conn.Close()
}

func normalizeWSForLocalHTTP(u string, cfg Config) string {
	if cfg.PublicScheme != "http" {
		return u
	}
	hostPort := proxyHostPort(cfg)
	u = strings.ReplaceAll(u, "wss://"+hostPort, "ws://"+hostPort)
	u = strings.ReplaceAll(u, "wss://127.0.0.1", "ws://127.0.0.1")
	u = strings.ReplaceAll(u, "wss://localhost", "ws://localhost")
	return u
}
