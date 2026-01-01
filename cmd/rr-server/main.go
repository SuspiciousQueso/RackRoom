package main

import (
	"log"
	"net/http"
	"os"

	"rackroom/internal/server"
)

func main() {
	enrollToken := os.Getenv("RR_ENROLL_TOKEN")
	if enrollToken == "" {
		enrollToken = "ENROLL-DEV-CHANGE-ME"
	}

	store := server.NewStore(enrollToken)
	api := &server.API{Store: store}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/enroll", api.Enroll)
	mux.HandleFunc("/v1/heartbeat", api.RequireAgentAuth(api.Heartbeat))
	mux.HandleFunc("/v1/jobs/poll", api.PollJobs)
	mux.HandleFunc("/v1/job_result", api.RequireAgentAuth(api.JobResult))
	mux.HandleFunc("/v1/jobs/submit", api.SubmitJob)

	addr := ":8080"
	log.Printf("rr-server listening on %s (enroll token via RR_ENROLL_TOKEN)", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
