package shared

import (
	"encoding/json"
	"os"
)

type AgentConfig struct {
	ServerURL        string   `json:"server_url"`
	EnrollToken      string   `json:"enroll_token"`
	AgentID          string   `json:"agent_id"`
	PrivateKeyPath   string   `json:"private_key_path"`
	HeartbeatSeconds int      `json:"heartbeat_seconds"`
	PollSeconds      int      `json:"poll_seconds"`
	InventorySeconds int      `json:"inventory_seconds"`
	Tags             []string `json:"tags"`
}

func LoadAgentConfig(path string) (*AgentConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c AgentConfig
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c.HeartbeatSeconds <= 0 {
		c.HeartbeatSeconds = 30
	}
	if c.PollSeconds <= 0 {
		c.PollSeconds = 10
	}
	if c.InventorySeconds <= 0 {
		c.InventorySeconds = 3600
	}
	return &c, nil
}

func SaveAgentConfig(path string, c *AgentConfig) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600)
}
