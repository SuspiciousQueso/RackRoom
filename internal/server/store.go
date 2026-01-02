package server

import "rackroom/internal/shared"

type AgentFacts struct {
	AgentID   string
	UpdatedAt int64

	OSCaption string
	OSVersion string
	OSBuild   string

	CPUName    string
	CPUCores   int64
	CPULogical int64

	RAMTotalBytes int64
	RAMFreeBytes  int64

	UptimeSeconds int64
	IPv4Primary   string

	DiskTotalBytes int64
	DiskFreeBytes  int64
}
type Store interface {
	// CreateAgent Agents
	CreateAgent(publicKey string, info shared.AgentInfo, tags []string) (agentID string, err error)
	GetAgentByID(agentID string) (*AgentRecord, error)
	GetAgentByPubKey(publicKey string) (*AgentRecord, error)
	UpdateAgentSeen(agentID string, info shared.AgentInfo, tags []string) error
	AddInventorySnapshot(agentID string, payloadJSON string) error
	GetLatestInventorySnapshot(agentID string) (string, error)
	ListAgents(limit int) ([]AgentRecord, error)
	UpsertAgentFacts(f AgentFacts) error
	// QueueJob Jobs
	QueueJob(agentID string, job shared.Job) error
	DequeueJobs(agentID string, max int) ([]shared.Job, error)
	ListAgentFacts(limit int) ([]AgentFacts, error)
	ListAgentFactsView(limit int) ([]AgentFactsView, error)

	// AddResult Results
	AddResult(res shared.JobResult) error
}

type AgentRecord struct {
	AgentID   string
	PublicKey string
	Info      shared.AgentInfo
	Tags      []string
	LastSeen  int64
}
