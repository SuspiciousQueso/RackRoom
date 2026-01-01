package shared

type EnrollRequest struct {
	EnrollToken string    `json:"enroll_token"`
	PublicKey   string    `json:"public_key"` // base64
	Info        AgentInfo `json:"info"`
	Tags        []string  `json:"tags,omitempty"`
}

type EnrollResponse struct {
	AgentID    string `json:"agent_id"`
	ServerTime int64  `json:"server_time"`
	Message    string `json:"message"`
}

type AgentInfo struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
}

type HeartbeatRequest struct {
	AgentID   string         `json:"agent_id"`
	Info      AgentInfo      `json:"info"`
	Inventory map[string]any `json:"inventory,omitempty"`
	Tags      []string       `json:"tags,omitempty"`
}

type HeartbeatResponse struct {
	Ok         bool  `json:"ok"`
	ServerTime int64 `json:"server_time"`
}

type Job struct {
	JobID          string `json:"job_id"`
	Kind           string `json:"kind"`  // "command"
	Shell          string `json:"shell"` // "bash" | "cmd" | "pwsh" (later)
	Command        string `json:"command"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type JobsPollResponse struct {
	Jobs []Job `json:"jobs"`
}

type JobResult struct {
	JobID      string `json:"job_id"`
	AgentID    string `json:"agent_id"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	StartedAt  int64  `json:"started_at"`
	FinishedAt int64  `json:"finished_at"`
}

type SubmitJobRequest struct {
	TargetAgentID  string `json:"target_agent_id"`
	Kind           string `json:"kind"`
	Shell          string `json:"shell"`
	Command        string `json:"command"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}
