# scripts/bootstrap.ps1
# RackRoom repo bootstrap: creates suite-style scaffolding + starter docs/contracts/migrations/scripts.

[CmdletBinding()]
param(
    [string]$RepoRoot = (Get-Location).Path,
    [switch]$Force
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Ensure-Dir([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path)) {
        New-Item -ItemType Directory -Path $Path | Out-Null
        Write-Host "Created dir: $Path"
    } else {
        Write-Host "Exists dir:  $Path"
    }
}

function Ensure-File([string]$Path, [string]$Content) {
    if ((Test-Path -LiteralPath $Path) -and -not $Force) {
        Write-Host "Exists file: $Path (skipped)"
        return
    }
    $dir = Split-Path -Parent $Path
    if ($dir -and -not (Test-Path -LiteralPath $dir)) {
        New-Item -ItemType Directory -Path $dir | Out-Null
    }
    Set-Content -LiteralPath $Path -Value $Content -Encoding UTF8
    Write-Host "Wrote file:  $Path"
}

function Append-IfMissing([string]$Path, [string]$Marker, [string]$Block) {
    if (-not (Test-Path -LiteralPath $Path)) {
        Ensure-File $Path $Block
        return
    }
    $text = Get-Content -LiteralPath $Path -Raw
    if ($text -match [regex]::Escape($Marker)) {
        Write-Host "Marker already present in $Path (skipped)"
        return
    }
    Add-Content -LiteralPath $Path -Value "`r`n$Block" -Encoding UTF8
    Write-Host "Appended block to: $Path"
}

# --- Directories ---
$contracts = Join-Path $RepoRoot "contracts"
$dbMig     = Join-Path $RepoRoot "db\migrations"
$docs      = Join-Path $RepoRoot "docs"
$scripts   = Join-Path $RepoRoot "scripts"
$webUI     = Join-Path $RepoRoot "web\rmm-ui"
$dataDir   = Join-Path $RepoRoot "data"

Ensure-Dir $contracts
Ensure-Dir $dbMig
Ensure-Dir $docs
Ensure-Dir $scripts
Ensure-Dir $webUI
Ensure-Dir $dataDir

# --- Starter Docs ---
$arch = @"
# RackRoom Architecture

RackRoom is the RMM utility in the MSPGuild suite.

**Slogan:** infrastructure literacy as software

## What RackRoom does (v0 -> v1)
- Agent enrolls into rr-server using a shared enroll token.
- Agent generates/stores an Ed25519 keypair locally.
- Agent signs requests (timestamp + body hash) so server can verify identity.
- Server persists agents, jobs, and results (SQLite in dev; Postgres later).

## Core concepts
- **Agent**: lightweight client on Windows/Linux.
- **Server**: API service that queues work and stores telemetry.
- **Jobs**: command/script execution requests sent to an agent.
- **Results**: stdout/stderr/exit code + timestamps.

## Identity model (Option C)
Agents are primarily identified by their **public key**.
If an agent_id goes stale, the server can re-associate the agent using its pubkey.

## UI + ITASM direction
- Web UI reads from RackRoom via API (or DB views in dev).
- ITASM holds business truth (ownership, lifecycle, docs).
- RackRoom holds raw truth (inventory, heartbeat, execution history).
- A link table maps RackRoom agent_id <-> ITASM asset_id.

"@

Ensure-File (Join-Path $docs "ARCHITECTURE.md") $arch

$roadmap = @"
# RackRoom Roadmap

## v0 (now)
- [x] rr-server runs
- [x] rr-agent enroll + heartbeat + poll + execute job + post result
- [x] Persist agents/jobs/results in SQLite
- [ ] Option C re-association via pubkey across restarts

## v1
- Inventory snapshots + queryable facts
- Auth for admin endpoints
- RBAC + tenancy hooks for MSPGuild
- Installer/service: Windows service + systemd unit
- Web UI: agents list, agent detail, job history, run command

## v2
- Patch policies + lifecycle hooks (PatchDay integration)
- Full monitoring/events (NightWatch integration)
- Baseline audits + drift detection

"@

Ensure-File (Join-Path $docs "ROADMAP.md") $roadmap

# --- OpenAPI Skeleton ---
$openapi = @"
openapi: 3.0.3
info:
  title: RackRoom API
  version: 0.1.0
servers:
  - url: http://localhost:8085
paths:
  /v1/enroll:
    post:
      summary: Enroll an agent
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
      responses:
        "200":
          description: OK
  /v1/heartbeat:
    post:
      summary: Agent heartbeat (signed)
      responses:
        "200":
          description: OK
  /v1/jobs/poll:
    get:
      summary: Poll for jobs
      parameters:
        - in: query
          name: agent_id
          schema:
            type: string
          required: true
      responses:
        "200":
          description: OK
  /v1/jobs/submit:
    post:
      summary: Submit a job (admin/dev)
      responses:
        "200":
          description: OK
  /v1/job_result:
    post:
      summary: Post a job result (signed)
      responses:
        "200":
          description: OK
"@

Ensure-File (Join-Path $contracts "openapi.yaml") $openapi

# --- Migration Starter ---
$mig = @"
-- 0001_init.sql
-- Dev SQLite schema for RackRoom (agents, jobs, job_results)

CREATE TABLE IF NOT EXISTS agents (
  id TEXT PRIMARY KEY,
  public_key TEXT NOT NULL UNIQUE,
  hostname TEXT NOT NULL,
  os TEXT NOT NULL,
  arch TEXT NOT NULL,
  tags_json TEXT NOT NULL DEFAULT '[]',
  created_at INTEGER NOT NULL,
  last_seen INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS jobs (
  id TEXT PRIMARY KEY,
  target_agent_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  shell TEXT NOT NULL,
  command TEXT NOT NULL,
  timeout_seconds INTEGER NOT NULL,
  status TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  started_at INTEGER,
  finished_at INTEGER,
  FOREIGN KEY(target_agent_id) REFERENCES agents(id)
);

CREATE INDEX IF NOT EXISTS idx_jobs_target_status
  ON jobs(target_agent_id, status, created_at);

CREATE TABLE IF NOT EXISTS job_results (
  job_id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  exit_code INTEGER NOT NULL,
  stdout TEXT NOT NULL,
  stderr TEXT NOT NULL,
  started_at INTEGER NOT NULL,
  finished_at INTEGER NOT NULL,
  FOREIGN KEY(job_id) REFERENCES jobs(id),
  FOREIGN KEY(agent_id) REFERENCES agents(id)
);
"@

Ensure-File (Join-Path $dbMig "0001_init.sql") $mig

# --- Dev Script ---
$dev = @"
# scripts/dev.ps1
# Dev helper: run rr-server then rr-agent in separate shells.

param(
  [string]`$Addr = ":8085",
  [string]`$DbPath = ".\data\rackroom.db",
  [string]`$EnrollToken = "ENROLL-DEV-CHANGE-ME",
  [string]`$AgentConfig = ".\agent.json"
)

`$env:RR_ADDR = `$Addr
`$env:RR_DB_PATH = `$DbPath
`$env:RR_ENROLL_TOKEN = `$EnrollToken

Write-Host "Starting rr-server on `$Addr (DB: `$DbPath)"
Start-Process powershell -ArgumentList "-NoExit", "-Command", "go run .\cmd\rr-server"

Start-Sleep -Seconds 1

Write-Host "Starting rr-agent using `$AgentConfig"
Start-Process powershell -ArgumentList "-NoExit", "-Command", "go run .\cmd\rr-agent --config `$AgentConfig"
"@


Ensure-File (Join-Path $scripts "dev.ps1") $dev

# --- Build Script ---
$build = @"
# scripts/build.ps1
# Build rr-server and rr-agent binaries into .\dist

param(
  [string]`$OutDir = ".\dist"
)

`$ErrorActionPreference = "Stop"
New-Item -ItemType Directory -Force -Path `$OutDir | Out-Null

Write-Host "Building rr-server..."
go build -o (Join-Path `$OutDir "rr-server.exe") .\cmd\rr-server

Write-Host "Building rr-agent..."
go build -o (Join-Path `$OutDir "rr-agent.exe") .\cmd\rr-agent

Write-Host "Done. Output in `$OutDir"
"@

Ensure-File (Join-Path $scripts "build.ps1") $build

# --- .gitignore additions (optional) ---
$gitignoreBlock = @"
# RackRoom local dev artifacts
/data/*.db
/dist/
"@

Append-IfMissing (Join-Path $RepoRoot ".gitignore") "RackRoom local dev artifacts" $gitignoreBlock

Write-Host "`nBootstrap complete."
Write-Host "Tip: run .\scripts\dev.ps1 to launch server + agent in two shells."
