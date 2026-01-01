package agent

import (
	"bytes"
	"os/exec"
)

func collectWindowsInventoryJSON() ([]byte, error) {
	// PowerShell emits JSON we can forward directly to server.
	// Keep it simple and stable: OS, CPU, RAM, disks, IPs, uptime.
	script := `
$os = Get-CimInstance Win32_OperatingSystem
$cpu = Get-CimInstance Win32_Processor | Select-Object -First 1
$disks = Get-CimInstance Win32_LogicalDisk -Filter "DriveType=3" | ForEach-Object {
  [pscustomobject]@{
    DeviceID = $_.DeviceID
    Size = [int64]$_.Size
    Free = [int64]$_.FreeSpace
    FileSystem = $_.FileSystem
  }
}
$ips = Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue | Where-Object {$_.IPAddress -ne "127.0.0.1"} |
  Select-Object -ExpandProperty IPAddress

[pscustomobject]@{
  collected_at = [int64]([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())
  hostname = $env:COMPUTERNAME
  os = @{
    caption = $os.Caption
    version = $os.Version
    build = $os.BuildNumber
  }
  cpu = @{
    name = $cpu.Name
    cores = $cpu.NumberOfCores
    logical = $cpu.NumberOfLogicalProcessors
  }
  memory = @{
    total_bytes = [int64]$os.TotalVisibleMemorySize * 1024
    free_bytes  = [int64]$os.FreePhysicalMemory * 1024
  }
  uptime_seconds = [int64]((Get-Date) - $os.LastBootUpTime).TotalSeconds
  disks = $disks
  ipv4 = $ips
} | ConvertTo-Json -Depth 6 -Compress
`

	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
