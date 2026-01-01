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

