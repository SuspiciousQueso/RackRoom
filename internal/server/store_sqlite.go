package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"rackroom/internal/shared"
)

type SQLiteStore struct {
	DB *sql.DB
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{DB: db}
}

func (s *SQLiteStore) CreateAgent(publicKey string, info shared.AgentInfo, tags []string) (string, error) {
	// If pubkey already exists, return existing agent id (idempotent enroll)
	if rec, _ := s.GetAgentByPubKey(publicKey); rec != nil {
		_ = s.UpdateAgentSeen(rec.AgentID, info, tags)
		return rec.AgentID, nil
	}

	agentID := newUUID()
	now := time.Now().Unix()
	tagsJSON, _ := json.Marshal(tags)

	_, err := s.DB.Exec(
		`INSERT INTO agents (id, public_key, hostname, os, arch, tags_json, created_at, last_seen)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		agentID, publicKey, info.Hostname, info.OS, info.Arch, string(tagsJSON), now, now,
	)
	return agentID, err
}

func (s *SQLiteStore) GetAgentByID(agentID string) (*AgentRecord, error) {
	row := s.DB.QueryRow(
		`SELECT id, public_key, hostname, os, arch, tags_json, last_seen
		 FROM agents WHERE id = ?`, agentID,
	)

	var rec AgentRecord
	var tagsJSON string
	if err := row.Scan(&rec.AgentID, &rec.PublicKey, &rec.Info.Hostname, &rec.Info.OS, &rec.Info.Arch, &tagsJSON, &rec.LastSeen); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	_ = json.Unmarshal([]byte(tagsJSON), &rec.Tags)
	return &rec, nil
}

func (s *SQLiteStore) GetAgentByPubKey(publicKey string) (*AgentRecord, error) {
	row := s.DB.QueryRow(
		`SELECT id, public_key, hostname, os, arch, tags_json, last_seen
		 FROM agents WHERE public_key = ?`, publicKey,
	)

	var rec AgentRecord
	var tagsJSON string
	if err := row.Scan(&rec.AgentID, &rec.PublicKey, &rec.Info.Hostname, &rec.Info.OS, &rec.Info.Arch, &tagsJSON, &rec.LastSeen); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	_ = json.Unmarshal([]byte(tagsJSON), &rec.Tags)
	return &rec, nil
}

func (s *SQLiteStore) UpdateAgentSeen(agentID string, info shared.AgentInfo, tags []string) error {
	now := time.Now().Unix()
	tagsJSON, _ := json.Marshal(tags)

	_, err := s.DB.Exec(
		`UPDATE agents
		 SET hostname=?, os=?, arch=?, tags_json=?, last_seen=?
		 WHERE id=?`,
		info.Hostname, info.OS, info.Arch, string(tagsJSON), now, agentID,
	)
	return err
}

func (s *SQLiteStore) QueueJob(agentID string, job shared.Job) error {
	now := time.Now().Unix()

	_, err := s.DB.Exec(
		`INSERT INTO jobs (id, target_agent_id, kind, shell, command, timeout_seconds, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'queued', ?)`,
		job.JobID, agentID, job.Kind, job.Shell, job.Command, job.TimeoutSeconds, now,
	)
	return err
}

func (s *SQLiteStore) DequeueJobs(agentID string, max int) ([]shared.Job, error) {
	if max <= 0 {
		max = 5
	}

	// Grab queued jobs
	rows, err := s.DB.Query(
		`SELECT id, kind, shell, command, timeout_seconds
		 FROM jobs
		 WHERE target_agent_id = ? AND status = 'queued'
		 ORDER BY created_at
		 LIMIT ?`, agentID, max,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []shared.Job
	for rows.Next() {
		var j shared.Job
		if err := rows.Scan(&j.JobID, &j.Kind, &j.Shell, &j.Command, &j.TimeoutSeconds); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}

	// Mark as running (simple; v0 doesn’t track per-agent concurrency)
	now := time.Now().Unix()
	for _, j := range jobs {
		_, _ = s.DB.Exec(`UPDATE jobs SET status='running', started_at=? WHERE id=?`, now, j.JobID)
	}

	return jobs, nil
}

func (s *SQLiteStore) AddResult(res shared.JobResult) error {
	// Store result
	_, err := s.DB.Exec(
		`INSERT OR REPLACE INTO job_results (job_id, agent_id, exit_code, stdout, stderr, started_at, finished_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		res.JobID, res.AgentID, res.ExitCode, res.Stdout, res.Stderr, res.StartedAt, res.FinishedAt,
	)
	if err != nil {
		return err
	}

	// Update job status
	status := "done"
	if res.ExitCode != 0 {
		status = "failed"
	}
	_, _ = s.DB.Exec(`UPDATE jobs SET status=?, finished_at=? WHERE id=?`, status, res.FinishedAt, res.JobID)
	return nil
}
func (s *SQLiteStore) AddInventorySnapshot(agentID string, payloadJSON string) error {
	now := time.Now().Unix()
	id := newUUID()

	_, err := s.DB.Exec(
		`INSERT INTO agent_inventory_snapshots (id, agent_id, created_at, payload_json)
		 VALUES (?, ?, ?, ?)`,
		id, agentID, now, payloadJSON,
	)
	return err
}

func (s *SQLiteStore) GetLatestInventorySnapshot(agentID string) (string, error) {
	row := s.DB.QueryRow(
		`SELECT payload_json
		 FROM agent_inventory_snapshots
		 WHERE agent_id=?
		 ORDER BY created_at DESC
		 LIMIT 1`,
		agentID,
	)

	var payload string
	if err := row.Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return payload, nil
}

func (s *SQLiteStore) ListAgents(limit int) ([]AgentRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.DB.Query(
		`SELECT id, public_key, hostname, os, arch, tags_json, last_seen
		 FROM agents
		 ORDER BY last_seen DESC
		 LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AgentRecord
	for rows.Next() {
		var rec AgentRecord
		var tagsJSON string
		if err := rows.Scan(&rec.AgentID, &rec.PublicKey, &rec.Info.Hostname, &rec.Info.OS, &rec.Info.Arch, &tagsJSON, &rec.LastSeen); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(tagsJSON), &rec.Tags)
		out = append(out, rec)
	}
	return out, nil
}

func (s *SQLiteStore) UpsertAgentFacts(f AgentFacts) error {
	_, err := s.DB.Exec(
		`INSERT INTO agent_facts (
			agent_id, updated_at,
			os_caption, os_version, os_build,
			cpu_name, cpu_cores, cpu_logical,
			ram_total_bytes, ram_free_bytes,
			uptime_seconds, ipv4_primary,
			disk_total_bytes, disk_free_bytes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_id) DO UPDATE SET
			updated_at=excluded.updated_at,
			os_caption=excluded.os_caption,
			os_version=excluded.os_version,
			os_build=excluded.os_build,
			cpu_name=excluded.cpu_name,
			cpu_cores=excluded.cpu_cores,
			cpu_logical=excluded.cpu_logical,
			ram_total_bytes=excluded.ram_total_bytes,
			ram_free_bytes=excluded.ram_free_bytes,
			uptime_seconds=excluded.uptime_seconds,
			ipv4_primary=excluded.ipv4_primary,
			disk_total_bytes=excluded.disk_total_bytes,
			disk_free_bytes=excluded.disk_free_bytes
		`,
		f.AgentID, f.UpdatedAt,
		f.OSCaption, f.OSVersion, f.OSBuild,
		f.CPUName, f.CPUCores, f.CPULogical,
		f.RAMTotalBytes, f.RAMFreeBytes,
		f.UptimeSeconds, f.IPv4Primary,
		f.DiskTotalBytes, f.DiskFreeBytes,
	)
	return err
}
func (s *SQLiteStore) ListAgentFacts(limit int) ([]AgentFacts, error) {
	if limit <= 0 {
		limit = 200
	}

	rows, err := s.DB.Query(
		`SELECT agent_id, updated_at,
		        os_caption, os_version, os_build,
		        cpu_name, cpu_cores, cpu_logical,
		        ram_total_bytes, ram_free_bytes,
		        uptime_seconds, ipv4_primary,
		        disk_total_bytes, disk_free_bytes
		   FROM agent_facts
		   ORDER BY updated_at DESC
		   LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AgentFacts
	for rows.Next() {
		var f AgentFacts
		if err := rows.Scan(
			&f.AgentID, &f.UpdatedAt,
			&f.OSCaption, &f.OSVersion, &f.OSBuild,
			&f.CPUName, &f.CPUCores, &f.CPULogical,
			&f.RAMTotalBytes, &f.RAMFreeBytes,
			&f.UptimeSeconds, &f.IPv4Primary,
			&f.DiskTotalBytes, &f.DiskFreeBytes,
		); err != nil {
			return nil, err
		}
		out = append(out, f)
	}

	return out, nil
}

func (s *SQLiteStore) ListAgentFactsView(limit int) ([]AgentFactsView, error) {
	if limit <= 0 {
		limit = 200
	}

	rows, err := s.DB.Query(
		`SELECT
			a.id,
			a.hostname,
			a.tags_json,
			a.last_seen,

			COALESCE(f.os_caption, ''),
			COALESCE(f.os_version, ''),
			COALESCE(f.os_build, ''),

			COALESCE(f.cpu_name, ''),
			COALESCE(f.cpu_cores, 0),
			COALESCE(f.cpu_logical, 0),

			COALESCE(f.ram_total_bytes, 0),
			COALESCE(f.ram_free_bytes, 0),

			COALESCE(f.uptime_seconds, 0),
			COALESCE(f.ipv4_primary, ''),

			COALESCE(f.disk_total_bytes, 0),
			COALESCE(f.disk_free_bytes, 0),

			COALESCE(f.updated_at, 0)
		FROM agents a
		LEFT JOIN agent_facts f ON f.agent_id = a.id
		ORDER BY a.last_seen DESC
		LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AgentFactsView
	for rows.Next() {
		var v AgentFactsView
		var tagsJSON string
		if err := rows.Scan(
			&v.AgentID,
			&v.Hostname,
			&tagsJSON,
			&v.LastSeen,

			&v.OSCaption,
			&v.OSVersion,
			&v.OSBuild,

			&v.CPUName,
			&v.CPUCores,
			&v.CPULogical,

			&v.RAMTotalBytes,
			&v.RAMFreeBytes,

			&v.UptimeSeconds,
			&v.IPv4Primary,

			&v.DiskTotalBytes,
			&v.DiskFreeBytes,

			&v.UpdatedAt,
		); err != nil {
			return nil, err
		}

		_ = json.Unmarshal([]byte(tagsJSON), &v.Tags)
		out = append(out, v)
	}

	return out, nil
}
