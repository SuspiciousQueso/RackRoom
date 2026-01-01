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

