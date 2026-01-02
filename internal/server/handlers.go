package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"rackroom/internal/shared"

	"github.com/google/uuid"
)

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

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(io.LimitReader(r.Body, 2<<20))
}

func (a *API) Enroll(w http.ResponseWriter, r *http.Request) {
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

	if req.EnrollToken == "" || req.EnrollToken != a.EnrollToken {
		writeJSON(w, 401, map[string]any{"error": "invalid enroll token"})
		return
	}

	agentID, err := a.Store.CreateAgent(req.PublicKey, req.Info, req.Tags)
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

// middleware-ish auth for signed requests
func (a *API) RequireAgentAuth(next http.HandlerFunc) http.HandlerFunc {
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
			rec, err = a.Store.GetAgentByID(agentID)
			if err != nil {
				writeJSON(w, 500, map[string]any{"error": "db error"})
				return
			}
		}

		if rec == nil && pubKeyB64 != "" {
			rec, err = a.Store.GetAgentByPubKey(pubKeyB64)
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

func (a *API) Heartbeat(w http.ResponseWriter, r *http.Request) {
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

	if err := a.Store.UpdateAgentSeen(hb.AgentID, hb.Info, hb.Tags); err != nil {
		writeJSON(w, 500, map[string]any{"error": "db error"})
		return
	}
	if len(hb.Inventory) > 0 {
		_ = a.Store.AddInventorySnapshot(hb.AgentID, string(hb.Inventory))

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

			_ = a.Store.UpsertAgentFacts(AgentFacts{
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

func (a *API) PollJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		writeJSON(w, 400, map[string]any{"error": "missing agent_id"})
		return
	}

	jobs, err := a.Store.DequeueJobs(agentID, 5)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "db error"})
		return
	}

	writeJSON(w, 200, shared.JobsPollResponse{Jobs: jobs})
}

func (a *API) JobResult(w http.ResponseWriter, r *http.Request) {
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

	if err := a.Store.AddResult(res); err != nil {
		writeJSON(w, 500, map[string]any{"error": "db error"})
		return
	}

	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *API) SubmitJob(w http.ResponseWriter, r *http.Request) {
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

	if err := a.Store.QueueJob(req.TargetAgentID, job); err != nil {
		writeJSON(w, 500, map[string]any{"error": "db error"})
		return
	}

	writeJSON(w, 200, map[string]any{"ok": true, "job_id": job.JobID})
}

// helpers
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
func (a *API) AdminListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}

	agents, err := a.Store.ListAgents(200)
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
	for _, a := range agents {
		out = append(out, row{
			AgentID:  a.AgentID,
			Hostname: a.Info.Hostname,
			OS:       a.Info.OS,
			Arch:     a.Info.Arch,
			Tags:     a.Tags,
			LastSeen: a.LastSeen,
		})
	}

	writeJSON(w, 200, map[string]any{"agents": out})
}

func (a *API) AdminLatestInventory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}

	// Expected:
	// /v1/admin/agents/{agent_id}/inventory/latest
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

	payload, err := a.Store.GetLatestInventorySnapshot(agentID)
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

func (a *API) AdminAgentsFacts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}

	facts, err := a.Store.ListAgentFacts(200)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "db error"})
		return
	}

	writeJSON(w, 200, map[string]any{"facts": facts})
}
