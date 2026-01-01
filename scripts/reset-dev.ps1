# scripts/reset-dev.ps1
# Wipes local dev DB and agent state

$ErrorActionPreference = "Stop"

$db = ".\data\rackroom.db"
$agentCfg = ".\agent.json"

if (Test-Path $db) {
    Remove-Item $db -Force
    Write-Host "Deleted $db"
} else {
    Write-Host "No DB to delete"
}

if (Test-Path $agentCfg) {
    $cfg = Get-Content $agentCfg | ConvertFrom-Json
    $cfg.agent_id = ""
    $cfg | ConvertTo-Json -Depth 5 | Set-Content $agentCfg
    Write-Host "Cleared agent_id in agent.json"
}

Write-Host "Dev state reset. Re-run rr-server and rr-agent."
