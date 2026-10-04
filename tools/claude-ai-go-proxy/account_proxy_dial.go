package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

type contextKey string

const proxyContextKey contextKey = "account_proxy"

func requestWithAccountProxy(r *http.Request, cfg Config) *http.Request {
	if !usesPanelAccountMode(cfg) {
		return r
	}
	_, acc := resolvePremiumCookies(r, cfg)
	if acc == nil || strings.TrimSpace(acc.Proxy) == "" {
		return r
	}
	return r.WithContext(context.WithValue(r.Context(), proxyContextKey, acc.Proxy))
}

func dialClaudeTCP(ctx context.Context, addr string) (net.Conn, error) {
	if ctxPx, ok := ctx.Value(proxyContextKey).(string); ok && strings.TrimSpace(ctxPx) != "" {
		px, err := parseAccountProxy(ctxPx)
		if err != nil {
			return nil, err
		}
		conn, err := dialThroughAccountProxy(ctx, addr, px)
		if err != nil {
			return nil, fmt.Errorf("proxy dial %s: %w", px.Host, err)
		}
		return conn, nil
	}
	conn, err := (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("TCP dial: %w", err)
	}
	return conn, nil
}

func parseAccountProxy(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("proxy dial invalid endpoint")
	}
	switch strings.ToLower(u.Scheme) {
	case "socks5", "socks5h", "http", "https":
		return u, nil
	default:
		return nil, fmt.Errorf("proxy dial unsupported scheme")
	}
}

func dialThroughAccountProxy(ctx context.Context, targetAddr string, proxyURL *url.URL) (net.Conn, error) {
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
			return cd.DialContext(ctx, "tcp", targetAddr)
		}
		return dialer.Dial("tcp", targetAddr)
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
		connect := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", targetAddr, targetAddr)
		if proxyURL.User != nil {
			pass, _ := proxyURL.User.Password()
			token := base64.StdEncoding.EncodeToString([]byte(proxyURL.User.Username() + ":" + pass))
			connect += "Proxy-Authorization: Basic " + token + "\r\n"
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
			return nil, fmt.Errorf("HTTP proxy CONNECT refused: %s", resp.Status)
		}
		return conn, nil
	default:
		return nil, fmt.Errorf("unsupported proxy scheme")
	}
}
