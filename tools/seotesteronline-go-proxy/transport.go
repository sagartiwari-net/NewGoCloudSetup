package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
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

// dialTCP prefers IPv4. CloudFront IPv6 for api.seotesteronline.com intermittently
// hangs until Dialer Timeout → Proxy ErrorHandler 502 (auth/limits/suggested-plans).
func dialTCP(ctx context.Context, addr string) (net.Conn, error) {
	if marked, px := panelUpstreamProxy(ctx); marked && px != "" {
		return panelDialThrough(ctx, addr, px)
	}
	d := &net.Dialer{Timeout: 12 * time.Second, KeepAlive: 30 * time.Second}
	conn, err := d.DialContext(ctx, "tcp4", addr)
	if err == nil {
		return conn, nil
	}
	return d.DialContext(ctx, "tcp", addr)
}

func dialChrome(ctx context.Context, addr string) (*uTLSConn, error) {
	return dialChromeALPN(ctx, addr, nil)
}

func dialChromeHTTP1(ctx context.Context, addr string) (*uTLSConn, error) {
	host, _, _ := net.SplitHostPort(addr)
	tcpConn, err := dialTCP(ctx, addr)
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
	tcpConn, err := dialTCP(ctx, addr)
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

// RoundTrip uses HTTP/1.1 with connection reuse. The old probe-dial-then-close
// path doubled TLS handshakes per request and made concurrent SPA boots flaky.
func (rt *chromeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt.h1.RoundTrip(req)
}

var (
	sharedTransportOnce sync.Once
	sharedTransport     http.RoundTripper
)

func getSharedTransport() http.RoundTripper {
	sharedTransportOnce.Do(func() {
		sharedTransport = buildChromeTransport()
	})
	return sharedTransport
}

func buildChromeTransport() http.RoundTripper {
	dialTLS := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialChromeHTTP1(ctx, addr)
	}
	h1 := &http.Transport{
		DialTLSContext:        dialTLS,
		DialContext:           func(ctx context.Context, network, addr string) (net.Conn, error) { return dialTCP(ctx, addr) },
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   64,
		MaxConnsPerHost:       128,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   12 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    false,
		ForceAttemptHTTP2:     false,
		DisableKeepAlives:     false,
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
