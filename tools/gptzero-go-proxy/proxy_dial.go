package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

// parseProxyString converts panel Proxy Manager / account proxy strings to *url.URL.
// Supported:
//
//	socks5://user:pass@host:port
//	http://user:pass@host:port  (https:// treated as CONNECT too)
//	host:port:user:pass
//	host:port
func parseProxyString(s string) *url.URL {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return nil
		}
		scheme := strings.ToLower(u.Scheme)
		if scheme != "socks5" && scheme != "socks5h" && scheme != "http" && scheme != "https" {
			log.Printf("[PROXY] unknown scheme %q — skipping", scheme)
			return nil
		}
		return u
	}
	parts := strings.SplitN(s, ":", 4)
	switch len(parts) {
	case 2:
		u, _ := url.Parse(fmt.Sprintf("socks5://%s:%s", parts[0], parts[1]))
		return u
	case 4:
		u, _ := url.Parse(fmt.Sprintf("socks5://%s:%s@%s:%s",
			url.QueryEscape(parts[2]), url.QueryEscape(parts[3]), parts[0], parts[1]))
		return u
	}
	log.Printf("[PROXY] unrecognized format %q — skipping", s)
	return nil
}

func dialThroughProxy(ctx context.Context, targetAddr string, proxyURL *url.URL) (net.Conn, error) {
	switch strings.ToLower(proxyURL.Scheme) {
	case "socks5", "socks5h":
		return dialThroughSocks5(ctx, targetAddr, proxyURL)
	case "http", "https":
		return dialThroughHTTPProxy(ctx, targetAddr, proxyURL)
	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s", proxyURL.Scheme)
	}
}

func dialThroughSocks5(ctx context.Context, targetAddr string, proxyURL *url.URL) (net.Conn, error) {
	var auth *proxy.Auth
	if proxyURL.User != nil {
		pass, _ := proxyURL.User.Password()
		auth = &proxy.Auth{User: proxyURL.User.Username(), Password: pass}
	}
	proxyHost := proxyURL.Host
	if !strings.Contains(proxyHost, ":") {
		proxyHost += ":1080"
	}
	base := &net.Dialer{Timeout: 20 * time.Second}
	d, err := proxy.SOCKS5("tcp", proxyHost, auth, base)
	if err != nil {
		return nil, fmt.Errorf("socks5 dialer init: %w", err)
	}
	if cd, ok := d.(proxy.ContextDialer); ok {
		return cd.DialContext(ctx, "tcp", targetAddr)
	}
	return d.Dial("tcp", targetAddr)
}

func dialThroughHTTPProxy(ctx context.Context, targetAddr string, proxyURL *url.URL) (net.Conn, error) {
	proxyHost := proxyURL.Host
	if !strings.Contains(proxyHost, ":") {
		if proxyURL.Scheme == "https" {
			proxyHost += ":443"
		} else {
			proxyHost += ":80"
		}
	}
	conn, err := (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, "tcp", proxyHost)
	if err != nil {
		return nil, fmt.Errorf("connect to HTTP proxy: %w", err)
	}
	connectLine := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", targetAddr, targetAddr)
	if proxyURL.User != nil {
		pass, _ := proxyURL.User.Password()
		creds := base64.StdEncoding.EncodeToString([]byte(proxyURL.User.Username() + ":" + pass))
		connectLine += "Proxy-Authorization: Basic " + creds + "\r\n"
	}
	connectLine += "\r\n"
	if _, err := conn.Write([]byte(connectLine)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_ = conn.Close()
		return nil, fmt.Errorf("HTTP proxy CONNECT refused: %s", resp.Status)
	}
	return conn, nil
}

var (
	upstreamClients   sync.Map // proxy key → *http.Client
	directClientOnce  sync.Once
	directClientValue *http.Client
)

func upstreamHTTPClient(proxyStr string) *http.Client {
	key := strings.TrimSpace(proxyStr)
	if key == "" {
		directClientOnce.Do(func() {
			directClientValue = buildUpstreamClient("")
		})
		return directClientValue
	}
	if v, ok := upstreamClients.Load(key); ok {
		return v.(*http.Client)
	}
	c := buildUpstreamClient(key)
	actual, _ := upstreamClients.LoadOrStore(key, c)
	return actual.(*http.Client)
}

func buildUpstreamClient(proxyStr string) *http.Client {
	px := parseProxyString(proxyStr)
	tr := &http.Transport{
		Proxy:               nil,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 16,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 20 * time.Second,
		ResponseHeaderTimeout: 90 * time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if px != nil {
				c, err := dialThroughProxy(ctx, addr, px)
				if err != nil {
					return nil, fmt.Errorf("proxy dial %s: %w", px.Host, err)
				}
				return c, nil
			}
			return (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, network, addr)
		},
	}
	return &http.Client{
		Transport: tr,
		Timeout:   120 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func isProxyDialError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "proxy dial") ||
		strings.Contains(msg, "socks5") ||
		strings.Contains(msg, "http proxy") ||
		strings.Contains(msg, "connect refused")
}
