package main

import "net/http"

func (s *server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /v1/healthz", s.handleHealthz)

	mux.HandleFunc("GET /v1/portals", s.handlePortals)
	mux.HandleFunc("POST /v1/session/verify", s.handleSessionVerify)
	mux.HandleFunc("POST /v1/session/logout", s.handleSessionLogout)
	mux.HandleFunc("GET /v1/me", s.handleMe)
	mux.HandleFunc("GET /v1/extension/update", s.handleExtensionUpdate)
	mux.HandleFunc("GET /v1/extension/download.zip", s.handleExtensionDownload)

	if s.cfg.DevAuth {
		mux.HandleFunc("GET /v1/dev/tm-check", s.handleDevTMCheck)
		mux.HandleFunc("POST /v1/dev/tm-check", s.handleDevTMCheck)
		mux.HandleFunc("GET /v1/dev/accounts", s.handleDevAccounts)
		mux.HandleFunc("POST /v1/dev/limits", s.handleDevSetLimits)
		mux.HandleFunc("POST /v1/dev/meters/reset", s.handleDevResetMeters)
	}

	mux.HandleFunc("GET /v4/{endpoint}", s.handleV4)
	mux.HandleFunc("POST /v4/{endpoint}", s.handleV4)
	mux.HandleFunc("OPTIONS /v4/{endpoint}", s.handleV4)
}
