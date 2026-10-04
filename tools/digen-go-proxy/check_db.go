//go:build ignore

package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	// DSN from local config
	dsn := "toolsmandirefct:jAwaF3rpcXM3tNJh@tcp(127.0.0.1:3306)/toolsmandirefct?parseTime=true"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping DB: %v", err)
	}

	fmt.Println("Successfully connected to MySQL database!")

	var id, toolID, sessionDuration int
	var name, domain, secretKey string
	err = db.QueryRow("SELECT id, tool_id, name, domain, secret_key, session_duration FROM ahrefs_websites WHERE id = 44").
		Scan(&id, &toolID, &name, &domain, &secretKey, &sessionDuration)
	if err != nil {
		log.Fatalf("Failed to query website 44: %v", err)
	}

	fmt.Printf("Website 44 Config:\n")
	fmt.Printf("  ID: %d\n", id)
	fmt.Printf("  Tool ID: %d\n", toolID)
	fmt.Printf("  Name: %s\n", name)
	fmt.Printf("  Domain: %s\n", domain)
	fmt.Printf("  Secret Key: %s\n", secretKey)
	fmt.Printf("  Session Duration: %d minutes\n", sessionDuration)
}
