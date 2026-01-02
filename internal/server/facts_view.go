package server

type AgentFactsView struct {
	AgentID   string `json:"agent_id"`
	Hostname  string `json:"hostname"`
	OSCaption string `json:"os_caption"`
	OSVersion string `json:"os_version"`
	OSBuild   string `json:"os_build"`

	CPUName    string `json:"cpu_name"`
	CPUCores   int64  `json:"cpu_cores"`
	CPULogical int64  `json:"cpu_logical"`

	RAMTotalBytes int64 `json:"ram_total_bytes"`
	RAMFreeBytes  int64 `json:"ram_free_bytes"`

	UptimeSeconds int64  `json:"uptime_seconds"`
	IPv4Primary   string `json:"ipv4_primary"`

	DiskTotalBytes int64 `json:"disk_total_bytes"`
	DiskFreeBytes  int64 `json:"disk_free_bytes"`

	UpdatedAt int64    `json:"updated_at"`
	LastSeen  int64    `json:"last_seen"`
	Tags      []string `json:"tags"`
}
