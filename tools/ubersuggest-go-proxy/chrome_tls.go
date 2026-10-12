package main

import (
	"context"
	"crypto/tls"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

// uTLSConn lets the HTTP/2 client read the Chrome handshake result.
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

func dialChrome(ctx context.Context, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
		addr = net.JoinHostPort(addr, "443")
	}
	tcpConn, err := panelDialTCP(ctx, addr)
	if err != nil {
		return nil, err
	}
	uConn := utls.UClient(tcpConn, &utls.Config{ServerName: host}, utls.HelloChrome_120)
	if err = uConn.HandshakeContext(ctx); err != nil {
		tcpConn.Close()
		return nil, err
	}
	return &uTLSConn{uConn}, nil
}

type chromeRoundTripper struct {
	h2 *http2.Transport
	h1 *http.Transport
}

func newChromeRoundTripper() *chromeRoundTripper {
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialChrome(ctx, addr)
	}
	h1 := &http.Transport{
		DialTLSContext:      dial,
		ForceAttemptHTTP2:   false,
		TLSHandshakeTimeout: 20 * time.Second,
		IdleConnTimeout:     90 * time.Second,
	}
	h1.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	h2 := &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			return dialChrome(ctx, addr)
		},
	}
	return &chromeRoundTripper{h2: h2, h1: h1}
}

func (rt *chromeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Del("Connection")
	req.Header.Del("Upgrade")
	req.Header.Del("Proxy-Connection")
	req.Header.Del("Keep-Alive")
	req.Header.Del("TE")
	req.Header.Del("Trailer")
	req.Header.Del("Transfer-Encoding")
	req.Close = false
	host := req.Header.Get("X-Proxy-Host")
	scheme := req.Header.Get("X-Proxy-Scheme")
	req.Header.Del("X-Proxy-Host")
	req.Header.Del("X-Proxy-Scheme")

	resp, err := rt.h2.RoundTrip(req)
	if err != nil {
		log.Printf("[UBER] chrome h2 %s: %v", req.URL.Path, err)
		resp, err = rt.h1.RoundTrip(req)
	}
	if resp != nil && resp.Request != nil {
		if host != "" {
			resp.Request.Header.Set("X-Proxy-Host", host)
		}
		if scheme != "" {
			resp.Request.Header.Set("X-Proxy-Scheme", scheme)
		}
	}
	return resp, err
}

func uberUpstreamRoute(ctx context.Context) string {
	_, px := panelUpstreamProxy(ctx)
	px = strings.TrimSpace(px)
	if px == "" {
		return "server-ip"
	}
	if u := panelParseProxyURL(px); u != nil && u.Host != "" {
		return "proxy " + u.Host
	}
	return "proxy"
}
