package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	loginFailLimit       = 5
	loginFailWindow      = 15 * time.Minute
	loginIPPerMinute     = 12
	loginGlobalPerSecond = 8
)

// loginGate stops password guessing and keeps a flood off the slow hash.
// A blocked call returns before the database or bcrypt run.
type loginGate struct {
	mu     sync.Mutex
	now    func() time.Time
	byIP   map[string]*ipLogin
	global []time.Time
}

type ipLogin struct {
	fails        int
	failStart    time.Time
	blockedUntil time.Time
	hits         []time.Time
}

func newLoginGate() *loginGate {
	return &loginGate{now: time.Now, byIP: map[string]*ipLogin{}}
}

func (g *loginGate) allow(ip string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	g.prune(now)
	state := g.byIP[ip]
	if state != nil && now.Before(state.blockedUntil) {
		return false
	}
	if state == nil {
		state = &ipLogin{}
		g.byIP[ip] = state
	}
	state.hits = recent(state.hits, now.Add(-time.Minute))
	if len(state.hits) >= loginIPPerMinute {
		return false
	}
	g.global = recent(g.global, now.Add(-time.Second))
	if len(g.global) >= loginGlobalPerSecond {
		return false
	}
	state.hits = append(state.hits, now)
	g.global = append(g.global, now)
	return true
}

func (g *loginGate) fail(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	state := g.byIP[ip]
	if state == nil {
		state = &ipLogin{}
		g.byIP[ip] = state
	}
	if state.failStart.IsZero() || now.Sub(state.failStart) > loginFailWindow {
		state.fails = 0
		state.failStart = now
	}
	state.fails++
	if state.fails >= loginFailLimit {
		state.blockedUntil = now.Add(loginFailWindow)
		state.fails = 0
		state.failStart = time.Time{}
	}
}

func (g *loginGate) success(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.byIP, ip)
}

func (g *loginGate) prune(now time.Time) {
	if len(g.byIP) < 500 {
		return
	}
	for ip, state := range g.byIP {
		idle := len(state.hits) == 0 || now.Sub(state.hits[len(state.hits)-1]) > loginFailWindow
		if idle && now.After(state.blockedUntil) {
			delete(g.byIP, ip)
		}
	}
}

func recent(times []time.Time, cutoff time.Time) []time.Time {
	out := times[:0]
	for _, item := range times {
		if item.After(cutoff) {
			out = append(out, item)
		}
	}
	return out
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		if host == "" {
			return "unknown"
		}
		return host
	}
	if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
		return real
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	return host
}
