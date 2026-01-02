package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"rackroom/internal/server"
)

func main() {
	// Enroll token (dev default is fine locally; override in env)
	enrollToken := os.Getenv("RR_ENROLL_TOKEN")
	if enrollToken == "" {
		enrollToken = "ENROLL-DEV-CHANGE-ME"
	}

	// Listen address
	addr := os.Getenv("RR_ADDR")
	if addr == "" {
		addr = ":8085"
	}

	// DB path (SQLite)
	dbPath := os.Getenv("RR_DB_PATH")
	if dbPath == "" {
		dbPath = "./data/rackroom.db"
	}

	// Ensure DB directory exists
	dbDir := filepath.Dir(dbPath)
	if dbDir != "." && dbDir != "" {
		if err := os.MkdirAll(dbDir, 0700); err != nil {
			log.Fatalf("failed to create db dir %s: %v", dbDir, err)
		}
	}

	// Open DB + run migrations
	db, err := server.OpenDB(dbPath)
	if err != nil {
		log.Fatalf("failed to open db %s: %v", dbPath, err)
	}

	if err := server.RunMigrations(db); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	store := server.NewSQLiteStore(db)

	api := &server.API{
		Store:       store,
		EnrollToken: enrollToken,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/enroll", api.Enroll)
	// admin (v0 – no auth yet)
	mux.HandleFunc("/v1/admin/agents", api.AdminListAgents)
	mux.HandleFunc("/v1/admin/agents/facts", api.AdminAgentsFacts) // must be before prefix
	mux.HandleFunc("/v1/admin/agents/", api.AdminLatestInventory)  // prefix last
	mux.HandleFunc("/debug/sql", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if len(body) == 0 {
			http.Error(w, "empty body", 400)
			return
		}
		if _, err := db.Exec(string(body)); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	})
	// Signed endpoints
	mux.HandleFunc("/v1/heartbeat", api.RequireAgentAuth(api.Heartbeat))
	mux.HandleFunc("/v1/job_result", api.RequireAgentAuth(api.JobResult))
	// Polling + submit (v0)
	mux.HandleFunc("/v1/jobs/poll", api.PollJobs)
	mux.HandleFunc("/v1/jobs/submit", api.SubmitJob)
	mux.Handle("/", http.FileServer(http.Dir("./web/rmm-ui")))
	log.Printf("rr-server listening on %s", addr)
	log.Printf("db: %s", dbPath)
	log.Printf("enroll token: via RR_ENROLL_TOKEN")

	log.Fatal(http.ListenAndServe(addr, mux))
}
