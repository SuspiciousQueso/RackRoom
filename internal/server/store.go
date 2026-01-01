package server

import (
	"sync"
	"time"

	"rackroom/internal/shared"
)

type AgentRecord struct {
	AgentID   string
	PublicKey string // base64
	Info      shared.AgentInfo
	Tags      []string
	LastSeen  time.Time
}

type Store struct {
	mu sync.Mutex

	EnrollToken string
	Agents      map[string]*AgentRecord
	JobsByAgent map[string][]shared.Job
	Results     []shared.JobResult
}

func NewStore(enrollToken string) *Store {
	return &Store{
		EnrollToken: enrollToken,
		Agents:      map[string]*AgentRecord{},
		JobsByAgent: map[string][]shared.Job{},
		Results:     []shared.JobResult{},
	}
}

func (s *Store) UpsertAgent(rec *AgentRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Agents[rec.AgentID] = rec
}

func (s *Store) GetAgent(agentID string) (*AgentRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.Agents[agentID]
	return rec, ok
}

func (s *Store) QueueJob(agentID string, job shared.Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.JobsByAgent[agentID] = append(s.JobsByAgent[agentID], job)
}

func (s *Store) DequeueJobs(agentID string, max int) []shared.Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	jobs := s.JobsByAgent[agentID]
	if len(jobs) == 0 {
		return nil
	}
	if max <= 0 || max > len(jobs) {
		max = len(jobs)
	}
	out := jobs[:max]
	s.JobsByAgent[agentID] = jobs[max:]
	return out
}

func (s *Store) AddResult(res shared.JobResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Results = append(s.Results, res)
}
