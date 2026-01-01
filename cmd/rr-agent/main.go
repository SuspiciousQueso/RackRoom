package main

import (
	"context"
	"flag"
	"log"
	"time"

	"rackroom/internal/agent"
)

func main() {
	configPath := flag.String("config", "./agent.json", "path to agent config json")
	flag.Parse()

	a, err := agent.New(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	if err := a.EnrollIfNeeded(ctx); err != nil {
		log.Fatal(err)
	}
	log.Printf("rr-agent enrolled/ready as agent_id=%s", a.Cfg.AgentID)

	heartbeatTicker := time.NewTicker(time.Duration(a.Cfg.HeartbeatSeconds) * time.Second)
	pollTicker := time.NewTicker(time.Duration(a.Cfg.PollSeconds) * time.Second)

	for {
		select {
		case <-heartbeatTicker.C:
			if err := a.SendHeartbeat(ctx); err != nil {
				log.Printf("heartbeat error: %v", err)
			}
		case <-pollTicker.C:
			jobs, err := a.PollJobs(ctx)
			if err != nil {
				log.Printf("poll error: %v", err)
				continue
			}
			for _, job := range jobs {
				log.Printf("running job %s: %s", job.JobID, job.Command)
				res := a.RunJob(ctx, job)
				if err := a.PostResult(ctx, res); err != nil {
					log.Printf("post result error: %v", err)
				}
			}
		}
	}
}
