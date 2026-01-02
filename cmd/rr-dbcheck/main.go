package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"rackroom/internal/server"
)

func main() {
	dbPath := os.Getenv("RR_DB_PATH")
	if dbPath == "" {
		dbPath = "./data/rackroom.db"
	}

	db, err := server.OpenDB(dbPath)
	if err != nil {
		log.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' ORDER BY name;`)
	if err != nil {
		log.Fatalf("query failed: %v", err)
	}
	defer rows.Close()

	fmt.Println("Tables:")
	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		fmt.Println(" -", name)
	}

	// Optional: show agent count
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM agents;`).Scan(&n)
	fmt.Println("Agents:", n)

	var inv int
	err = db.QueryRow(`SELECT COUNT(*) FROM agent_inventory_snapshots;`).Scan(&inv)
	if err != nil {
		fmt.Println("Inventory snapshots: ERROR ->", err)
	} else {
		fmt.Println("Inventory snapshots:", inv)
	}
	var facts int
	err = db.QueryRow(`SELECT COUNT(*) FROM agent_facts;`).Scan(&facts)
	if err != nil {
		fmt.Println("Agent facts: ERROR ->", err)
	} else {
		fmt.Println("Agent facts:", facts)
	}
	_ = sql.ErrNoRows // keeps sql imported if your IDE nags
}
