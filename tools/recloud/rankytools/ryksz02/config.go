package main

import (
	"encoding/json"
	"log"
	"os"
	"strings"
	"sync"
)

type Config struct {
	Port                   string `json:"port"`
	PublicHost             string `json:"public_host"`
	PublicScheme           string `json:"public_scheme"`
	TargetURL              string `json:"target_url"`
	HomePath               string `json:"home_path"`
	ToolName               string `json:"tool_name"`
	PanelDB                string `json:"panel_db"`
	LocalTestMode          bool   `json:"local_test_mode"`
	BypassAuth             bool   `json:"bypass_auth"`
	SessionDurationMinutes int    `json:"session_duration_minutes"`
	MySQLPassword          string `json:"mysql_password"`
}

var (
	cfgOnce sync.Once
	cfgVal  Config
)

func loadConfig() Config {
	cfgOnce.Do(func() {
		cfgVal = Config{
			Port:                   "6509",
			PublicHost:             "127.0.0.1:6509",
			PublicScheme:           "http",
			TargetURL:              "https://sz02.rankytools.com",
			HomePath:               "/private-domains/",
			ToolName:               "RankyTools SpamZilla",
			SessionDurationMinutes: 30,
			LocalTestMode:          true,
			BypassAuth:             true,
		}
		body, err := os.ReadFile("config.json")
		if err != nil {
			log.Printf("[CFG] config.json not read: %v", err)
		} else if err := json.Unmarshal(body, &cfgVal); err != nil {
			log.Printf("[CFG] config.json invalid: %v", err)
		}
		if local, err := os.ReadFile("config.local.json"); err == nil {
			if err := json.Unmarshal(local, &cfgVal); err != nil {
				log.Printf("[CFG] config.local.json invalid: %v", err)
			} else {
				log.Printf("[CFG] loaded config.local.json")
			}
		}
	})
	return cfgVal
}

func usesPanelAccountMode(cfg Config) bool {
	return strings.TrimSpace(cfg.PanelDB) != "" && !cfg.BypassAuth
}

func toolDisplayName(cfg Config) string {
	name := strings.TrimSpace(cfg.ToolName)
	if name == "" {
		return "RankyTools SpamZilla"
	}
	return name
}
