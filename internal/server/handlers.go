// Package server implements the RackRoom HTTP API.
//
// handlers.go contains the HTTP handlers ("controllers") for rr-server.
// This file is intentionally stdlib-only (net/http + ServeMux) to keep the
// API surface small and easy to reason about.
//
// High-level flow:
//   - /v1/enroll: agent enrollment (exchange public key + basic info)
//   - /v1/heartbeat: signed agent updates (presence + optional inventory)
//   - /v1/jobs/*: lightweight job queue (poll + submit + result)
//   - /v1/admin/*: human/admin read endpoints (locked behind service key)
//
// Notes:
//   - Agent authentication uses per-agent public keys + request signing.
//   - "Admin" endpoints are intended for internal use (UI/MSPGuild) and
//     must be protected (RequireServiceKey) before exposing the server beyond localhost.

package server

// -----------------------------------------------------------------------------
// Helpers (JSON + request utilities)
// -----------------------------------------------------------------------------

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"rackroom/internal/shared"

	"github.com/google/uuid"
)

// firstN returns the first n characters of a string.
// Used for safe logging of long values (e.g., pubkey prefixes) without dumping secrets.

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

type API struct {
	Store       Store
	EnrollToken string
}

// writeJSON writes a JSON response with a status code.
// This is the standard response helper for API endpoints.

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// readBody reads the request body with a size limit and closes it.
// The limit prevents accidental large payloads from consuming memory.

func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(io.LimitReader(r.Body, 2<<20))
}

// -----------------------------------------------------------------------------
// Agent endpoints (enroll, heartbeat, job polling/results)
// -----------------------------------------------------------------------------

// Enroll registers a new agent with the server.
//
// Expects POST JSON: shared.EnrollRequest (includes EnrollToken, PublicKey, Info, Tags).
// On success, returns shared.EnrollResponse with a new AgentID.
//
// This is intentionally simple for v0: enrollment is authorized by a shared enroll token.
// Later we can swap this for per-tenant enrollment, short-lived tokens, or UI-driven enrollment.

func (api *API) Enroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	body, err := readBody(r)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": "bad body"})
		return
	}

	var req shared.EnrollRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, 400, map[string]any{"error": "bad json"})
		return
	}

	if req.EnrollToken == "" || req.EnrollToken != api.EnrollToken {
		writeJSON(w, 401, map[string]any{"error": "invalid enroll token"})
		return
	}

	agentID, err := api.Store.CreateAgent(req.PublicKey, req.Info, req.Tags)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "db error"})
		return
	}

	writeJSON(w, 200, shared.EnrollResponse{
		AgentID:    agentID,
		ServerTime: time.Now().Unix(),
		Message:    "enrolled",
	})
}

// RequireAgentAuth validates signed agent requests.
//
// Expected headers:
//   - X-Timestamp, X-Signature, X-Body-Sha256
// Optional headers (v0 supports multiple identity paths):
//   - X-Agent-Id: canonical agent id (preferred)
//   - X-PubKey: fallback identity if agent id is missing/unknown
//
// Verification steps:
//   - timestamp sanity window (prevents replay)
//   - lookup agent record by id or pubkey
//   - verify signature against stored public key
//
// If pubkey-based lookup succeeds, the canonical agent id is attached as
// X-Canonical-Agent-Id for downstream handlers.

func (api *API) RequireAgentAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := r.Header.Get("X-Agent-Id")
		pubKeyB64 := r.Header.Get("X-PubKey")
		ts := r.Header.Get("X-Timestamp")
		sig := r.Header.Get("X-Signature")
		bodySha := r.Header.Get("X-Body-Sha256")

		log.Printf("auth: path=%s agent_id=%q pubkey_prefix=%q", r.URL.Path, agentID, firstN(pubKeyB64, 16))

		if ts == "" || sig == "" || bodySha == "" {
			writeJSON(w, 401, map[string]any{"error": "missing auth headers"})
			return
		}

		// Timestamp sanity window (10 min)
		tInt, _ := parseInt64(ts)
		now := time.Now().Unix()
		if tInt == 0 || tInt < now-600 || tInt > now+600 {
			writeJSON(w, 401, map[string]any{"error": "timestamp outside window"})
			return
		}

		// Find agent record by agent_id, else fall back to pubkey (Option C)
		var rec *AgentRecord
		var err error

		if agentID != "" {
			rec, err = api.Store.GetAgentByID(agentID)
			if err != nil {
				writeJSON(w, 500, map[string]any{"error": "db error"})
				return
			}
		}

		if rec == nil && pubKeyB64 != "" {
			rec, err = api.Store.GetAgentByPubKey(pubKeyB64)
			if err != nil {
				writeJSON(w, 500, map[string]any{"error": "db error"})
				return
			}
			if rec != nil {
				// Tell the handler (and optionally the agent) what the canonical agent_id is
				r.Header.Set("X-Canonical-Agent-Id", rec.AgentID)
				w.Header().Set("X-Canonical-Agent-Id", rec.AgentID)
			}
		}

		if rec == nil {
			writeJSON(w, 401, map[string]any{"error": "unknown agent"})
			return
		}

		pub, err := shared.DecodePubKey(rec.PublicKey)
		if err != nil {
			writeJSON(w, 500, map[string]any{"error": "server key decode failed"})
			return
		}

		if !shared.Verify(pub, sig, ts, r.Method, r.URL.Path, bodySha) {
			writeJSON(w, 401, map[string]any{"error": "bad signature"})
			return
		}

		next(w, r)
	}
}

// -----------------------------------------------------------------------------
// Agent endpoints (enroll, heartbeat, job polling/results)
// -----------------------------------------------------------------------------

// Heartbeat records agent presence ("last seen") and optional inventory payload.
//
// Expects POST JSON: shared.HeartbeatRequest.
// If inventory is included, it is stored as a snapshot and v0 "facts" are derived
// (OS version/build, CPU, RAM, disk totals, primary IPv4, etc.).
//
// This endpoint is signed (RequireAgentAuth) because it mutates server state.

func (api *API) Heartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}

	body, err := readBody(r)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": "bad body"})
		return
	}

	var hb shared.HeartbeatRequest
	if err := json.Unmarshal(body, &hb); err != nil {
		writeJSON(w, 400, map[string]any{"error": "bad json"})
		return
	}

	// If middleware re-associated the agent via pubkey (Option C),
	// use the canonical agent ID
	if canon := r.Header.Get("X-Canonical-Agent-Id"); canon != "" {
		hb.AgentID = canon
	}

	if err := api.Store.UpdateAgentSeen(hb.AgentID, hb.Info, hb.Tags); err != nil {
		writeJSON(w, 500, map[string]any{"error": "db error"})
		return
	}
	if len(hb.Inventory) > 0 {
		_ = api.Store.AddInventorySnapshot(hb.AgentID, string(hb.Inventory))

		// Facts extraction (v0)
		var inv WinInventory
		if err := json.Unmarshal(hb.Inventory, &inv); err == nil {
			var diskTotal, diskFree int64
			for _, d := range inv.Disks {
				diskTotal += d.Size
				diskFree += d.Free
			}
			ip := ""
			if len(inv.IPv4) > 0 {
				ip = inv.IPv4[0]
			}

			_ = api.Store.UpsertAgentFacts(AgentFacts{
				AgentID:        hb.AgentID,
				UpdatedAt:      time.Now().Unix(),
				OSCaption:      inv.OS.Caption,
				OSVersion:      inv.OS.Version,
				OSBuild:        inv.OS.Build,
				CPUName:        inv.CPU.Name,
				CPUCores:       inv.CPU.Cores,
				CPULogical:     inv.CPU.Logical,
				RAMTotalBytes:  inv.Memory.TotalBytes,
				RAMFreeBytes:   inv.Memory.FreeBytes,
				UptimeSeconds:  inv.UptimeSeconds,
				IPv4Primary:    ip,
				DiskTotalBytes: diskTotal,
				DiskFreeBytes:  diskFree,
			})
		}
	}

	writeJSON(w, 200, shared.HeartbeatResponse{
		Ok:         true,
		ServerTime: time.Now().Unix(),
	})
}

// PollJobs allows an agent to request queued work.
//
// Expects GET with query param: agent_id.
// Returns up to N jobs from the queue in shared.JobsPollResponse.
//
// NOTE: In v0 this is not signed. If you want strict security, wrap this with
// RequireAgentAuth and/or move agent_id into headers so the signature covers identity.

func (api *API) PollJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		writeJSON(w, 400, map[string]any{"error": "missing agent_id"})
		return
	}

	jobs, err := api.Store.DequeueJobs(agentID, 5)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "db error"})
		return
	}

	writeJSON(w, 200, shared.JobsPollResponse{Jobs: jobs})
}

// JobResult accepts an agent's result payload for a previously issued job.
//
// Expects POST JSON: shared.JobResult.
// This endpoint is signed (RequireAgentAuth) because it writes results to storage.
//
// If RequireAgentAuth re-associated identity via pubkey, we use X-Canonical-Agent-Id.

func (api *API) JobResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	body, err := readBody(r)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": "bad body"})
		return
	}
	var res shared.JobResult
	if err := json.Unmarshal(body, &res); err != nil {
		writeJSON(w, 400, map[string]any{"error": "bad json"})
		return
	}

	// If middleware rebounded, use canonical agent id
	if canon := r.Header.Get("X-Canonical-Agent-Id"); canon != "" {
		res.AgentID = canon
	}

	if err := api.Store.AddResult(res); err != nil {
		writeJSON(w, 500, map[string]any{"error": "db error"})
		return
	}

	writeJSON(w, 200, map[string]any{"ok": true})
}

// SubmitJob queues work for a target agent.
//
// Expects POST JSON: shared.SubmitJobRequest.
// This is a v0 admin-style endpoint and should be protected (RequireServiceKey)
// before exposing rr-server beyond localhost.
//
// Later: integrate with FrontDesk/PatchDay (e.g., "run script", "collect facts", etc.).

func (api *API) SubmitJob(w http.ResponseWriter, r *http.Request) {
	// v0 admin endpoint: no auth yet (lock it down later)
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	body, err := readBody(r)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": "bad body"})
		return
	}
	var req shared.SubmitJobRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, 400, map[string]any{"error": "bad json"})
		return
	}
	if strings.TrimSpace(req.TargetAgentID) == "" {
		writeJSON(w, 400, map[string]any{"error": "missing target_agent_id"})
		return
	}

	job := shared.Job{
		JobID:          uuid.NewString(),
		Kind:           req.Kind,
		Shell:          req.Shell,
		Command:        req.Command,
		TimeoutSeconds: req.TimeoutSeconds,
	}
	if job.Kind == "" {
		job.Kind = "command"
	}
	if job.TimeoutSeconds <= 0 {
		job.TimeoutSeconds = 30
	}

	if err := api.Store.QueueJob(req.TargetAgentID, job); err != nil {
		writeJSON(w, 500, map[string]any{"error": "db error"})
		return
	}

	writeJSON(w, 200, map[string]any{"ok": true, "job_id": job.JobID})
}

// parseInt64 parses a base-10 integer string without using strconv.
// Kept tiny for v0; returns 0 if any non-digit is encountered.

func parseInt64(s string) (int64, error) {
	var n int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, nil
		}
		n = n*10 + int64(ch-'0')
	}
	return n, nil
}

// -----------------------------------------------------------------------------
// Admin endpoints (read-only views for UI/MSPGuild)
// -----------------------------------------------------------------------------

// AdminListAgents returns a lightweight view of known agents.
//
// Expects GET.
// Returns agent_id, hostname, OS, arch, tags, last_seen.
// Intended for UI/MSPGuild to show inventory/health lists.
//
// Must be protected with RequireServiceKey in real deployments.

func (api *API) AdminListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}

	agents, err := api.Store.ListAgents(200)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "db error"})
		return
	}

	type row struct {
		AgentID  string   `json:"agent_id"`
		Hostname string   `json:"hostname"`
		OS       string   `json:"os"`
		Arch     string   `json:"arch"`
		Tags     []string `json:"tags"`
		LastSeen int64    `json:"last_seen"`
	}

	out := make([]row, 0, len(agents))
	for _, api := range agents {
		out = append(out, row{
			AgentID:  api.AgentID,
			Hostname: api.Info.Hostname,
			OS:       api.Info.OS,
			Arch:     api.Info.Arch,
			Tags:     api.Tags,
			LastSeen: api.LastSeen,
		})
	}

	writeJSON(w, 200, map[string]any{"agents": out})
}

// AdminLatestInventory returns the most recent inventory snapshot for a single agent.
//
// Route:
//   GET /v1/admin/agents/{agent_id}/inventory/latest
//
// This handler is mounted on the "/v1/admin/agents/" prefix and performs
// its own path parsing to extract the agent ID and expected sub-path.
//
// Behavior:
//   - Validates the request path structure
//   - Looks up the latest inventory snapshot for the agent
//   - Returns the snapshot as raw JSON (no re-encoding)
//
// Notes:
//   - Inventory payloads are stored as opaque JSON blobs generated by agents.
//   - This endpoint is intended for internal/admin use (UI, MSPGuild).
//   - Must be protected with RequireServiceKey before exposing publicly.

func (api *API) AdminLatestInventory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	// ServeMux prefix handler:
	// We manually parse the remainder of the path to support sub-routes
	// under /v1/admin/agents/{agent_id}/...

	path := strings.TrimPrefix(r.URL.Path, "/v1/admin/agents/")
	parts := strings.Split(path, "/")

	if len(parts) != 3 || parts[1] != "inventory" || parts[2] != "latest" {
		writeJSON(w, 400, map[string]any{
			"error":    "invalid path",
			"expected": "/v1/admin/agents/{agent_id}/inventory/latest",
		})
		return
	}

	agentID := parts[0]
	if agentID == "" {
		writeJSON(w, 400, map[string]any{"error": "missing agent_id"})
		return
	}

	payload, err := api.Store.GetLatestInventorySnapshot(agentID)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "db error"})
		return
	}
	if payload == "" {
		writeJSON(w, 404, map[string]any{"error": "no inventory"})
		return
	}

	// payload is already JSON — return it raw
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, _ = w.Write([]byte(payload))
}

// AdminAgentsFacts returns the derived "facts" summary for agents.
//
// Expects GET.
// Facts are extracted during Heartbeat inventory ingestion.
// Intended for dashboards and quick asset overview.
//
// Must be protected with RequireServiceKey in real deployments.

func (api *API) AdminAgentsFacts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}

	facts, err := api.Store.ListAgentFacts(200)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "db error"})
		return
	}

	writeJSON(w, 200, map[string]any{"facts": facts})
}

// -----------------------------------------------------------------------------
// Middleware (auth wrappers)
// -----------------------------------------------------------------------------

// RequireServiceKey protects internal endpoints intended for server-to-server use.
//
// The service key is provided via env RR_API_KEY and compared against the request
// header X-RR-Key.
//
// This is used to lock down /v1/admin/* and any debug endpoints.
// It's not meant for agent auth (agents use signed requests via RequireAgentAuth).

func (api *API) RequireServiceKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		want := os.Getenv("RR_API_KEY")
		if want == "" {
			http.Error(w, "RR_API_KEY not set", http.StatusUnauthorized)
			return
		}
		got := r.Header.Get("X-RR-Key")
		if got == "" || got != want {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
