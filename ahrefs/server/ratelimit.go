package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type rateBucket struct {
	count int
	start time.Time
}

var (
	verifyRateMu sync.Mutex
	verifyRates  = map[string]*rateBucket{}
)

func clientIP(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// allowVerifyRate returns false when the client exceeded VerifyRatePerMin.
func (s *server) allowVerifyRate(r *http.Request) bool {
	limit := s.cfg.VerifyRatePerMin
	if limit <= 0 {
		return true
	}
	ip := clientIP(r)
	now := time.Now()
	verifyRateMu.Lock()
	defer verifyRateMu.Unlock()
	b := verifyRates[ip]
	if b == nil || now.Sub(b.start) >= time.Minute {
		verifyRates[ip] = &rateBucket{count: 1, start: now}
		return true
	}
	b.count++
	return b.count <= limit
}
