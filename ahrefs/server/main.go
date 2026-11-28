package main

import (
	"database/sql"
	"log"
	"net/http"
	"strings"

	"ahrefs-bar-api/filecache"
)

type server struct {
	cfg       config
	db        *sql.DB
	driver    string
	fileCache *filecache.Store
}

func main() {
	cfg := loadConfig()
	db, driver, err := openDB(cfg)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer db.Close()

	if err := migrate(db, driver); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Printf("migrated ok (driver=%s)", driver)

	s := &server{cfg: cfg, db: db, driver: driver}
	fc, err := filecache.Open(filecache.Config{
		Dir:      cfg.FileCacheDir,
		URLCapGB: cfg.FileCacheURLGB,
		KWCapGB:  cfg.FileCacheKWGB,
		TTLHours: cfg.FileCacheTTLHours,
		Enabled:  cfg.FileCacheEnabled,
	})
	if err != nil {
		log.Fatalf("filecache: %v", err)
	}
	s.fileCache = fc
	defer fc.Close()
	log.Printf("%s", fc)

	if err := s.seedDevPortal(); err != nil {
		log.Printf("seedDevPortal warn: %v", err)
	} else if cfg.DevAuth {
		log.Printf("dev auth ON — portal http://127.0.0.1:18090 (panel_session) → /v1/dev/tm-check")
	}
	if cfg.SeedDevAccounts {
		if err := s.seedDevAccount(); err != nil {
			log.Printf("seedDevAccount warn: %v", err)
		} else if cfg.DevAuth {
			mode := "MOCK A=401 + B=ok"
			if !cfg.MockUpstream && cfg.AccountJSON != "" {
				mode = "real token on B"
			}
			log.Printf("dev accounts seeded (%s)", mode)
		}
	} else {
		log.Printf("dev account seed OFF (using shared/panel accounts)")
	}
	log.Printf("sqlite/dsn path=%s", firstNonEmpty(cfg.DSN, cfg.SQLitePath))

	mux := http.NewServeMux()
	s.routes(mux)

	if cfg.RequireFingerprint {
		log.Printf("fingerprint REQUIRED (AHREFS_BAR_REQUIRE_FP=1)")
	} else {
		log.Printf("fingerprint optional (default)")
	}
	if s.telegramEnabled() {
		log.Printf("telegram alerts ON (chat=%s spike>=%d/%dm)", cfg.TelegramChatID, cfg.SpikeCredits, cfg.SpikeWindowMin)
	} else {
		log.Printf("telegram alerts off (set AHREFS_BAR_TELEGRAM_BOT_TOKEN + CHAT_ID)")
	}
	log.Printf("ahrefs-bar-api listening on http://%s", cfg.Addr)
	log.Fatal(http.ListenAndServe(cfg.Addr, s.withCORS(mux)))
}

func (s *server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allow := strings.HasPrefix(origin, "http://localhost:") ||
			strings.HasPrefix(origin, "http://127.0.0.1:") ||
			origin == "https://panel.upays.in" ||
			origin == "https://panel.gt4rents.com" ||
			origin == "chrome-extension://" || // won't match full; handled below
			s.originAllowed(origin)

		if strings.HasPrefix(origin, "chrome-extension://") {
			allow = true
		}
		if allow && origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-TM-Bar-Token, X-TM-Bar-Fresh, X-TM-Bar-Portal, X-TM-Bar-Cookie, X-TM-Bar-FP")
			w.Header().Set("Access-Control-Expose-Headers", "X-TM-Bar-Cache")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) originAllowed(origin string) bool {
	for _, o := range s.cfg.CORSExtra {
		if o == origin || o == "*" {
			return true
		}
	}
	return false
}
