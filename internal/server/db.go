package server

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

func OpenDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON;`); err != nil {
		return nil, err
	}

	if err := migrate(db); err != nil {
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS agents (
			id TEXT PRIMARY KEY,
			public_key TEXT NOT NULL UNIQUE,
			hostname TEXT NOT NULL,
			os TEXT NOT NULL,
			arch TEXT NOT NULL,
			tags_json TEXT NOT NULL DEFAULT '[]',
			created_at INTEGER NOT NULL,
			last_seen INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS jobs (
			id TEXT PRIMARY KEY,
			target_agent_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			shell TEXT NOT NULL,
			command TEXT NOT NULL,
			timeout_seconds INTEGER NOT NULL,
			status TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			started_at INTEGER,
			finished_at INTEGER,
			FOREIGN KEY(target_agent_id) REFERENCES agents(id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_jobs_target_status ON jobs(target_agent_id, status, created_at);`,
		`CREATE TABLE IF NOT EXISTS job_results (
			job_id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			exit_code INTEGER NOT NULL,
			stdout TEXT NOT NULL,
			stderr TEXT NOT NULL,
			started_at INTEGER NOT NULL,
			finished_at INTEGER NOT NULL,
			FOREIGN KEY(job_id) REFERENCES jobs(id),
			FOREIGN KEY(agent_id) REFERENCES agents(id)
		);`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}
