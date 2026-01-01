-- 0002_inventory.sql
CREATE TABLE IF NOT EXISTS agent_inventory_snapshots (
     id TEXT PRIMARY KEY,
     agent_id TEXT NOT NULL,
     created_at INTEGER NOT NULL,
     payload_json TEXT NOT NULL,
     FOREIGN KEY(agent_id) REFERENCES agents(id)
    );

CREATE INDEX IF NOT EXISTS idx_inventory_agent_created
    ON agent_inventory_snapshots(agent_id, created_at DESC);
