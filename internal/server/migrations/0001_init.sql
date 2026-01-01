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
