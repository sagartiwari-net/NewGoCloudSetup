package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

type uTLSConn struct{ *utls.UConn }

func (c *uTLSConn) ConnectionState() tls.ConnectionState {
	cs := c.UConn.ConnectionState()
	return tls.ConnectionState{
		Version:            cs.Version,
		HandshakeComplete:  cs.HandshakeComplete,
		DidResume:          cs.DidResume,
		CipherSuite:        cs.CipherSuite,
		NegotiatedProtocol: cs.NegotiatedProtocol,
		ServerName:         cs.ServerName,
	}
}

func dialChrome(ctx context.Context, addr string) (*uTLSConn, error) {
	return dialChromeALPN(ctx, addr, nil)
}

func dialChromeHTTP1(ctx context.Context, addr string) (*uTLSConn, error) {
	host, _, _ := net.SplitHostPort(addr)
	tcpConn, err := panelDialTCP(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("TCP dial: %w", err)
	}
	spec, err := utls.UTLSIdToSpec(utls.HelloChrome_120)
	if err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("uTLS spec: %w", err)
	}
	for i, ext := range spec.Extensions {
		if alpn, ok := ext.(*utls.ALPNExtension); ok {
			alpn.AlpnProtocols = []string{"http/1.1"}
			spec.Extensions[i] = alpn
		}
	}
	uConn := utls.UClient(tcpConn, &utls.Config{ServerName: host, InsecureSkipVerify: false}, utls.HelloCustom)
	if err := uConn.ApplyPreset(&spec); err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("uTLS preset: %w", err)
	}
	if err := uConn.HandshakeContext(ctx); err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("uTLS handshake: %w", err)
	}
	return &uTLSConn{uConn}, nil
}

func dialChromeALPN(ctx context.Context, addr string, nextProtos []string) (*uTLSConn, error) {
	host, _, _ := net.SplitHostPort(addr)
	tcpConn, err := panelDialTCP(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("TCP dial: %w", err)
	}
	tlsCfg := &utls.Config{ServerName: host, InsecureSkipVerify: false}
	if len(nextProtos) > 0 {
		tlsCfg.NextProtos = nextProtos
	}
	uConn := utls.UClient(tcpConn, tlsCfg, utls.HelloChrome_120)
	if err := uConn.HandshakeContext(ctx); err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("uTLS handshake: %w", err)
	}
	return &uTLSConn{uConn}, nil
}

type chromeRoundTripper struct {
	h2 *http2.Transport
	h1 *http.Transport
}

func (rt *chromeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	addr := req.URL.Host
	if !strings.Contains(addr, ":") {
		if req.URL.Scheme == "https" {
			addr += ":443"
		} else {
			addr += ":80"
		}
	}
	conn, err := dialChrome(req.Context(), addr)
	if err != nil {
		return nil, err
	}
	proto := conn.ConnectionState().NegotiatedProtocol
	conn.Close()
	if proto == "h2" {
		return rt.h2.RoundTripOpt(req, http2.RoundTripOpt{})
	}
	return rt.h1.RoundTrip(req)
}

func buildChromeTransport() http.RoundTripper {
	dialTLS := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialChrome(ctx, addr)
	}
	h1 := &http.Transport{
		DialTLSContext:      dialTLS,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 20 * time.Second,
		DisableCompression:  false,
		ForceAttemptHTTP2:   false,
	}
	h2 := &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			return dialChrome(ctx, addr)
		},
		DisableCompression: false,
	}
	return &chromeRoundTripper{h2: h2, h1: h1}
}

func applyBrowserHeaders(req *http.Request, cfg Config) {
	req.Header.Set("User-Agent", cfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Sec-CH-UA", `"Chromium";v="131", "Google Chrome";v="131", "Not_A Brand";v="24"`)
	req.Header.Set("Sec-CH-UA-Mobile", "?0")
	req.Header.Set("Sec-CH-UA-Platform", `"macOS"`)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Del("X-Forwarded-For")
	req.Header.Del("X-Real-IP")
	req.Header.Del("Forwarded")
	req.Header.Del("Via")
}
