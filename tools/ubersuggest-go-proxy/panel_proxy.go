package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/proxy"
)

type panelUpstreamKey struct{}
type panelSessionMemo struct{}
type panelCookieMemo struct{}

func attachPanelUpstream(r *http.Request, accountProxy string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), panelUpstreamKey{}, strings.TrimSpace(accountProxy)))
}

func panelUpstreamProxy(ctx context.Context) (bool, string) {
	v, ok := ctx.Value(panelUpstreamKey{}).(string)
	return ok, strings.TrimSpace(v)
}

func panelSessionFor(req *http.Request, fallback func() string) string {
	if req == nil {
		if fallback != nil {
			return fallback()
		}
		return ""
	}
	raw := req.Header.Get("X-Panel-Session")
	req.Header.Del("X-Panel-Session")
	req.Header.Del("X-Panel-Cookie")
	req.Header.Del("X-Device-Fp")
	req.Header.Del("X-Device-Proof")
	if ua := strings.TrimSpace(req.Header.Get("X-Panel-UA")); ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	req.Header.Del("X-Panel-UA")
	session := raw
	if session == "" {
		if remembered, ok := req.Context().Value(panelSessionMemo{}).(string); ok {
			session = remembered
		}
	}
	if session == "" && fallback != nil {
		session = fallback()
	}
	*req = *req.WithContext(context.WithValue(req.Context(), panelSessionMemo{}, session))
	return session
}

func stampPanelAccount(r *http.Request, acc ToolAccount) *http.Request {
	if acc.Cookie != "" {
		r.Header.Set("X-Panel-Session", acc.Cookie)
		r.Header.Set("X-Panel-Cookie", acc.Cookie)
	}
	if strings.TrimSpace(acc.UserAgent) != "" {
		r.Header.Set("X-Panel-UA", acc.UserAgent)
	}
	return r
}

func applyPanelPremiumCookie(r *http.Request, fileCookie string) string {
	panelCookie := takePanelCookie(r)
	if panelCookie == "" {
		return fileCookie
	}
	kept := make([]string, 0)
	for _, part := range strings.Split(r.Header.Get("Cookie"), ";") {
		part = strings.TrimSpace(part)
		if part == "" || strings.HasPrefix(part, "ct_session=") {
			continue
		}
		kept = append(kept, part)
	}
	if len(kept) == 0 {
		r.Header.Del("Cookie")
	} else {
		r.Header.Set("Cookie", strings.Join(kept, "; "))
	}
	session := uberSessionCookieHeader(panelCookie)
	if session == "" {
		log.Printf("[UBER] account cookie has no usable id/state")
	}
	*r = *r.WithContext(context.WithValue(r.Context(), panelCookieMemo{}, session))
	return session
}

func panelMemoCookie(r *http.Request) string {
	if r == nil {
		return ""
	}
	v, _ := r.Context().Value(panelCookieMemo{}).(string)
	return v
}

func takePanelCookie(r *http.Request) string {
	cookie := r.Header.Get("X-Panel-Cookie")
	ua := strings.TrimSpace(r.Header.Get("X-Panel-UA"))
	r.Header.Del("X-Panel-Cookie")
	r.Header.Del("X-Panel-Session")
	r.Header.Del("X-Panel-UA")
	r.Header.Del("X-Device-Fp")
	r.Header.Del("X-Device-Proof")
	if ua != "" {
		r.Header.Set("User-Agent", ua)
	}
	return cookie
}

func panelParseProxyURL(raw string) *url.URL {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return nil
		}
		return u
	}
	parts := strings.Split(raw, ":")
	if len(parts) < 2 {
		return nil
	}
	u := &url.URL{Scheme: "http", Host: parts[0] + ":" + parts[1]}
	if len(parts) >= 4 {
		u.User = url.UserPassword(parts[2], strings.Join(parts[3:], ":"))
	}
	return u
}

func panelDialTCP(ctx context.Context, addr string) (net.Conn, error) {
	marked, accountProxy := panelUpstreamProxy(ctx)
	if marked && accountProxy != "" {
		return panelDialThrough(ctx, addr, accountProxy)
	}
	return (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, "tcp", addr)
}

func panelDialThrough(ctx context.Context, addr, rawProxy string) (net.Conn, error) {
	proxyURL := panelParseProxyURL(rawProxy)
	if proxyURL == nil {
		return nil, fmt.Errorf("proxy")
	}
	switch strings.ToLower(proxyURL.Scheme) {
	case "socks5", "socks5h":
		var auth *proxy.Auth
		if proxyURL.User != nil {
			pass, _ := proxyURL.User.Password()
			auth = &proxy.Auth{User: proxyURL.User.Username(), Password: pass}
		}
		host := proxyURL.Host
		if !strings.Contains(host, ":") {
			host += ":1080"
		}
		dialer, err := proxy.SOCKS5("tcp", host, auth, &net.Dialer{Timeout: 20 * time.Second})
		if err != nil {
			return nil, err
		}
		if cd, ok := dialer.(proxy.ContextDialer); ok {
			return cd.DialContext(ctx, "tcp", addr)
		}
		return dialer.Dial("tcp", addr)
	case "http", "https":
		host := proxyURL.Host
		if !strings.Contains(host, ":") {
			if proxyURL.Scheme == "https" {
				host += ":443"
			} else {
				host += ":80"
			}
		}
		conn, err := (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, "tcp", host)
		if err != nil {
			return nil, err
		}
		connect := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", addr, addr)
		if proxyURL.User != nil {
			pass, _ := proxyURL.User.Password()
			creds := base64.StdEncoding.EncodeToString([]byte(proxyURL.User.Username() + ":" + pass))
			connect += "Proxy-Authorization: Basic " + creds + "\r\n"
		}
		connect += "\r\n"
		if _, err = conn.Write([]byte(connect)); err != nil {
			conn.Close()
			return nil, err
		}
		resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			conn.Close()
			return nil, err
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			conn.Close()
			return nil, fmt.Errorf("proxy")
		}
		return conn, nil
	default:
		return nil, fmt.Errorf("proxy")
	}
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

var (
	proxyTransportMu sync.Mutex
	proxyTransports  = map[string]*http.Transport{}
)

// panelProxyTransport reuses connections for one proxy.
// A fresh dial on every image request was opening dozens of tunnels at once and the proxy answered 502.
func panelProxyTransport(rawProxy string) *http.Transport {
	rawProxy = strings.TrimSpace(rawProxy)
	proxyTransportMu.Lock()
	defer proxyTransportMu.Unlock()
	if tr, ok := proxyTransports[rawProxy]; ok {
		return tr
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return panelDialThrough(ctx, addr, rawProxy)
		},
		DisableCompression:    true,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		TLSHandshakeTimeout:   20 * time.Second,
		ResponseHeaderTimeout: 45 * time.Second,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   32,
		MaxConnsPerHost:       32,
		IdleConnTimeout:       90 * time.Second,
	}
	proxyTransports[rawProxy] = tr
	return tr
}

var (
	proxyH2Mu sync.Mutex
	proxyH2   = map[string]*http2.Transport{}
	h2Off     sync.Map
)

func panelProxyH2(rawProxy string) *http2.Transport {
	rawProxy = strings.TrimSpace(rawProxy)
	proxyH2Mu.Lock()
	defer proxyH2Mu.Unlock()
	if tr, ok := proxyH2[rawProxy]; ok {
		return tr
	}
	tr := &http2.Transport{
		DisableCompression: true,
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			rawConn, err := panelDialThrough(ctx, addr, rawProxy)
			if err != nil {
				return nil, err
			}
			host, _, splitErr := net.SplitHostPort(addr)
			if splitErr != nil {
				host = addr
			}
			tlsConn := tls.Client(rawConn, &tls.Config{
				ServerName:         host,
				NextProtos:         []string{"h2"},
				InsecureSkipVerify: true,
			})
			if err = tlsConn.HandshakeContext(ctx); err != nil {
				rawConn.Close()
				return nil, err
			}
			return tlsConn, nil
		},
	}
	proxyH2[rawProxy] = tr
	return tr
}

func panelProxyRoundTrip(req *http.Request, rawProxy string) (*http.Response, error) {
	if _, off := h2Off.Load(rawProxy); !off {
		resp, err := panelProxyH2(rawProxy).RoundTrip(req)
		if err == nil {
			return resp, nil
		}
		if req.Body == nil || req.GetBody != nil {
			h2Off.Store(rawProxy, true)
			log.Printf("[UBER] http2 upstream failed, using http/1.1: %v", err)
			if req.GetBody != nil {
				body, bodyErr := req.GetBody()
				if bodyErr == nil {
					req.Body = body
				}
			}
			return panelProxyTransport(rawProxy).RoundTrip(req)
		}
		return nil, err
	}
	return panelProxyTransport(rawProxy).RoundTrip(req)
}
