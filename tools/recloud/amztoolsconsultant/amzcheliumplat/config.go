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
			Port:                   "6302",
			PublicHost:             "amzcheliumplat.gt4rents.com",
			PublicScheme:           "http",
			TargetURL:              "https://ht06.amztoolsconsultant.com",
			HomePath:               "/black-box/products",
			ToolName:               "Amztoolsconsultant Helium10 Platinum",
			SessionDurationMinutes: 30,
		}
		body, err := os.ReadFile("config.json")
		if err != nil {
			log.Printf("[CFG] config.json not read: %v", err)
			return
		}
		if err := json.Unmarshal(body, &cfgVal); err != nil {
			log.Printf("[CFG] config.json invalid: %v", err)
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
		return "Amztoolsconsultant Helium10 Platinum"
	}
	return name
}
