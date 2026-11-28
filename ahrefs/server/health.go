package main

import (
	"net/http"
	"time"
)

func (s *server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	dbOK := true
	if err := s.db.Ping(); err != nil {
		dbOK = false
	}

	status := http.StatusOK
	if !dbOK {
		status = http.StatusServiceUnavailable
	}

	writeJSON(w, status, map[string]any{
		"success":   dbOK,
		"service":   "ahrefs-bar-api",
		"phase":         5,
		"driver":        s.driver,
		"dev_auth":      s.cfg.DevAuth,
		"mock_upstream": s.cfg.MockUpstream,
		"time":          time.Now().Format(time.RFC3339),
		"timezone":      s.cfg.Timezone,
		"db_ok":         dbOK,
		"listen":        s.cfg.Addr,
		"endpoints":     []string{"GET /healthz", "GET /v1/portals", "POST /v1/session/verify", "GET /v1/me", "GET|POST /v4/{endpoint}"},
	})
}
