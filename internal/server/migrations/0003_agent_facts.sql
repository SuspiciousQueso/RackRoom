CREATE TABLE IF NOT EXISTS agent_facts (
       agent_id TEXT PRIMARY KEY,
       updated_at INTEGER NOT NULL,

       os_caption TEXT,
       os_version TEXT,
       os_build TEXT,

       cpu_name TEXT,
       cpu_cores INTEGER,
       cpu_logical INTEGER,

       ram_total_bytes INTEGER,
       ram_free_bytes INTEGER,

       uptime_seconds INTEGER,

       ipv4_primary TEXT,

       disk_total_bytes INTEGER,
       disk_free_bytes INTEGER,

       FOREIGN KEY(agent_id) REFERENCES agents(id)
    );

CREATE INDEX IF NOT EXISTS idx_agent_facts_updated
    ON agent_facts(updated_at DESC);
