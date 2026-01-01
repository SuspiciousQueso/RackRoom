# scripts/dev.ps1
# Dev helper: run rr-server then rr-agent in separate shells.

param(
  [string]$Addr = ":8085",
  [string]$DbPath = ".\data\rackroom.db",
  [string]$EnrollToken = "ENROLL-DEV-CHANGE-ME",
  [string]$AgentConfig = ".\agent.json"
)

$env:RR_ADDR = $Addr
$env:RR_DB_PATH = (Resolve-Path $DbPath).Path
$env:RR_ENROLL_TOKEN = $EnrollToken

Write-Host "Starting rr-server on $Addr (DB: $DbPath)"
Start-Process powershell -ArgumentList "-NoExit", "-Command", "go run .\cmd\rr-server"

Start-Sleep -Seconds 1

Write-Host "Starting rr-agent using $AgentConfig"
Start-Process powershell -ArgumentList "-NoExit", "-Command", "go run .\cmd\rr-agent --config $AgentConfig"
