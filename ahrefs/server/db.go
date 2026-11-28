package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

func openDB(cfg config) (*sql.DB, string, error) {
	if cfg.DSN != "" {
		db, err := sql.Open("mysql", cfg.DSN)
		if err != nil {
			return nil, "", err
		}
		db.SetMaxOpenConns(20)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(30 * time.Minute)
		if err := db.Ping(); err != nil {
			_ = db.Close()
			return nil, "", fmt.Errorf("mysql ping: %w", err)
		}
		if _, err := db.Exec("SET time_zone = '+05:30'"); err != nil {
			log.Printf("[db] mysql time_zone warn: %v", err)
		}
		return db, "mysql", nil
	}

	if err := os.MkdirAll(filepath.Dir(cfg.SQLitePath), 0o755); err != nil {
		return nil, "", err
	}
	dsn := cfg.SQLitePath + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, "", err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, "", fmt.Errorf("sqlite ping: %w", err)
	}
	return db, "sqlite", nil
}

func migrate(db *sql.DB, driver string) error {
	stmts := schemaStatements(driver)
	for i, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate step %d: %w\nSQL: %s", i+1, err, truncate(stmt, 180))
		}
	}
	if err := ensurePhase2Columns(db, driver); err != nil {
		return err
	}
	if err := seedEndpointCosts(db, driver); err != nil {
		return err
	}
	if err := seedOwnerDefaults(db, driver); err != nil {
		return err
	}
	return nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
