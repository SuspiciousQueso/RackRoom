package server

import "rackroom/internal/shared"

type Store interface {
	// Agents
	CreateAgent(publicKey string, info shared.AgentInfo, tags []string) (agentID string, err error)
	GetAgentByID(agentID string) (*AgentRecord, error)
	GetAgentByPubKey(publicKey string) (*AgentRecord, error)
	UpdateAgentSeen(agentID string, info shared.AgentInfo, tags []string) error
	AddInventorySnapshot(agentID string, payloadJSON string) error
	GetLatestInventorySnapshot(agentID string) (string, error)

	// Jobs
	QueueJob(agentID string, job shared.Job) error
	DequeueJobs(agentID string, max int) ([]shared.Job, error)

	// Results
	AddResult(res shared.JobResult) error
}

type AgentRecord struct {
	AgentID   string
	PublicKey string
	Info      shared.AgentInfo
	Tags      []string
	LastSeen  int64
}
