# scripts/build.ps1
# Build rr-server and rr-agent binaries into .\dist

param(
  [string]$OutDir = ".\dist"
)

$ErrorActionPreference = "Stop"
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

Write-Host "Building rr-server..."
go build -o (Join-Path $OutDir "rr-server.exe") .\cmd\rr-server

Write-Host "Building rr-agent..."
go build -o (Join-Path $OutDir "rr-agent.exe") .\cmd\rr-agent

Write-Host "Done. Output in $OutDir"
