package main

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type config struct {
	Addr            string
	DSN             string // empty → local sqlite; mysql DSN → production
	SQLitePath      string
	Timezone        string
	SessionTTL      time.Duration
	CORSExtra       []string
	AmemberLitePath string
	DevAuth         bool   // local fake tm-check + seed portal
	DevCheckSecret  string
	MockUpstream    bool   // fake ahrefs.com responses (default on in dev)
	AccountJSON     string // optional real Ahrefs token JSON for Dev Account seed
	SeedDevAccounts bool   // insert MOCK A/B failover pair (disable when sharing panel.db)
	// Phase 8
	RequireFingerprint bool
	TelegramBotToken   string
	TelegramChatID     string
	SpikeCredits       int // billable credits in window → alert (0 = off)
	SpikeWindowMin     int
	VerifyRatePerMin   int // per client IP; 0 = off
	// Shared file cache (35GB urls + 15GB keywords)
	FileCacheDir     string
	FileCacheURLGB   int
	FileCacheKWGB    int
	FileCacheTTLHours int
	FileCacheEnabled bool
}

func loadConfig() config {
	devAuth := envOr("AHREFS_BAR_DEV_AUTH", "1") == "1"
	mockDefault := "0"
	if devAuth {
		mockDefault = "1"
	}
	cfg := config{
		Addr:               envOr("AHREFS_BAR_ADDR", "127.0.0.1:8088"),
		DSN:                strings.TrimSpace(os.Getenv("AHREFS_BAR_DSN")),
		SQLitePath:         envOr("AHREFS_BAR_SQLITE", "data/ahrefs_bar.db"),
		Timezone:           envOr("AHREFS_BAR_TZ", "Asia/Kolkata"),
		SessionTTL:         time.Duration(envInt("AHREFS_BAR_SESSION_HOURS", 12)) * time.Hour,
		AmemberLitePath:    strings.TrimSpace(os.Getenv("AMEMBER_LITE_PATH")),
		DevAuth:            devAuth,
		DevCheckSecret:     envOr("AHREFS_BAR_DEV_SECRET", "local-dev-secret"),
		MockUpstream:       envOr("AHREFS_BAR_MOCK_UPSTREAM", mockDefault) == "1",
		AccountJSON:        strings.TrimSpace(os.Getenv("AHREFS_BAR_ACCOUNT_JSON")),
		// Default ON — stolen tb.* tokens without matching device FP are rejected.
		RequireFingerprint: envOr("AHREFS_BAR_REQUIRE_FP", "1") == "1",
		TelegramBotToken:   strings.TrimSpace(os.Getenv("AHREFS_BAR_TELEGRAM_BOT_TOKEN")),
		TelegramChatID:     strings.TrimSpace(os.Getenv("AHREFS_BAR_TELEGRAM_CHAT_ID")),
		SpikeCredits:       envInt("AHREFS_BAR_SPIKE_CREDITS", 80),
		SpikeWindowMin:     envInt("AHREFS_BAR_SPIKE_WINDOW_MIN", 5),
		VerifyRatePerMin:    envInt("AHREFS_BAR_VERIFY_RATE", 60),
		FileCacheDir:        envOr("AHREFS_BAR_FILE_CACHE_DIR", "data/ahrefs-file-cache"),
		FileCacheURLGB:      envInt("AHREFS_BAR_FILE_CACHE_URL_GB", 35),
		FileCacheKWGB:       envInt("AHREFS_BAR_FILE_CACHE_KW_GB", 15),
		FileCacheTTLHours:   envInt("AHREFS_BAR_FILE_CACHE_TTL_HOURS", 72),
		FileCacheEnabled:    envOr("AHREFS_BAR_FILE_CACHE", "1") == "1",
	}
	// Same secret shape as panel-api: GT4RENTS_MYSQL_* → DSN when AHREFS_BAR_DSN empty.
	if cfg.DSN == "" {
		if pass := strings.TrimSpace(os.Getenv("GT4RENTS_MYSQL_PASSWORD")); pass != "" {
			host := envOr("GT4RENTS_MYSQL_HOST", "127.0.0.1")
			port := envOr("GT4RENTS_MYSQL_PORT", "3306")
			user := envOr("GT4RENTS_MYSQL_USER", "gt4rents")
			name := envOr("GT4RENTS_MYSQL_DB", "gt4rents")
			cfg.DSN = user + ":" + pass + "@tcp(" + host + ":" + port + ")/" + name +
				"?charset=utf8mb4&parseTime=true&loc=Local"
		}
	}
	// When sharing panel.db, set AHREFS_BAR_SEED_DEV_ACCOUNTS=0 so panel-managed tokens win.
	seedDefault := "0"
	if devAuth {
		seedDefault = "1"
	}
	cfg.SeedDevAccounts = envOr("AHREFS_BAR_SEED_DEV_ACCOUNTS", seedDefault) == "1"
	if extra := strings.TrimSpace(os.Getenv("AHREFS_BAR_CORS")); extra != "" {
		for _, p := range strings.Split(extra, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				cfg.CORSExtra = append(cfg.CORSExtra, p)
			}
		}
	}
	return cfg
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
