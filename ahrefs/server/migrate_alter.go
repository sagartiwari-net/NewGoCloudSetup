package main

import (
	"database/sql"
	"fmt"
	"log"
)

func ensurePhase2Columns(db *sql.DB, driver string) error {
	alters := []struct {
		table string
		col   string
		ddl   string // sqlite column def
		mysql string
	}{
		{"ahrefs_bar_portals", "shared_secret", "TEXT NOT NULL DEFAULT ''", "VARCHAR(128) NOT NULL DEFAULT ''"},
		{"ahrefs_bar_sessions", "email", "TEXT NOT NULL DEFAULT ''", "VARCHAR(255) NOT NULL DEFAULT ''"},
		{"ahrefs_bar_sessions", "name", "TEXT NOT NULL DEFAULT ''", "VARCHAR(255) NOT NULL DEFAULT ''"},
		{"ahrefs_bar_sessions", "uid", "INTEGER NOT NULL DEFAULT 0", "BIGINT NOT NULL DEFAULT 0"},
		{"ahrefs_bar_sessions", "portal_url", "TEXT NOT NULL DEFAULT ''", "VARCHAR(512) NOT NULL DEFAULT ''"},
	}
	for _, a := range alters {
		ok, err := columnExists(db, driver, a.table, a.col)
		if err != nil {
			return err
		}
		if ok {
			continue
		}
		def := a.ddl
		if driver == "mysql" {
			def = a.mysql
		}
		q := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", a.table, a.col, def)
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("alter %s.%s: %w", a.table, a.col, err)
		}
		log.Printf("[migrate] added %s.%s", a.table, a.col)
	}
	return nil
}

func columnExists(db *sql.DB, driver, table, col string) (bool, error) {
	if driver == "mysql" {
		var name string
		err := db.QueryRow(`
SELECT COLUMN_NAME FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`, table, col).Scan(&name)
		if err == sql.ErrNoRows {
			return false, nil
		}
		return err == nil, err
	}
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == col {
			return true, nil
		}
	}
	return false, rows.Err()
}
