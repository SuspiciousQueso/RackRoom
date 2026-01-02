package server

type WinInventory struct {
	CollectedAt int64  `json:"collected_at"`
	Hostname    string `json:"hostname"`

	OS struct {
		Caption string `json:"caption"`
		Version string `json:"version"`
		Build   string `json:"build"`
	} `json:"os"`

	CPU struct {
		Name    string `json:"name"`
		Cores   int64  `json:"cores"`
		Logical int64  `json:"logical"`
	} `json:"cpu"`

	Memory struct {
		TotalBytes int64 `json:"total_bytes"`
		FreeBytes  int64 `json:"free_bytes"`
	} `json:"memory"`

	UptimeSeconds int64 `json:"uptime_seconds"`

	Disks []struct {
		DeviceID   string `json:"DeviceID"`
		Size       int64  `json:"Size"`
		Free       int64  `json:"Free"`
		FileSystem string `json:"FileSystem"`
	} `json:"disks"`

	IPv4 []string `json:"ipv4"`
}
