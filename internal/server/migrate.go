package server

import (
	"database/sql"
	"embed"
	"log"
	"sort"
)

//go:embed migrations/*.sql
var migFS embed.FS

func RunMigrations(db *sql.DB) error {
	entries, err := migFS.ReadDir("migrations")
	if err != nil {
		return err
	}

	// Run in filename order: 0001_..., 0002_...
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	log.Println("running migrations")
	for _, name := range names {
		sqlBytes, err := migFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(sqlBytes)); err != nil {
			return err
		}
	}
	log.Println("migrations complete")
	return nil
}
