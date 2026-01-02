# scripts/reset-dev.ps1
# Wipes local dev DB and agent state

$ErrorActionPreference = "Stop"

$db = ".\data\rackroom.db*"

if (Test-Path $db) {
    Remove-Item $db -Force
    Write-Host "Deleted $db"
} else {
    Write-Host "No DB to delete"
}

Write-Host "Dev state reset. Re-run rr-server and rr-agent."
