package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"rackroom/internal/shared"

	"github.com/google/uuid"
)

type API struct {
	Store *Store
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

	if req.EnrollToken == "" || req.EnrollToken != a.Store.EnrollToken {
		writeJSON(w, 401, map[string]any{"error": "invalid enroll token"})
		return
	}

	agentID := uuid.NewString()
	rec := &AgentRecord{
		AgentID:   agentID,
		PublicKey: req.PublicKey,
		Info:      req.Info,
		Tags:      req.Tags,
		LastSeen:  time.Now(),
	}
	a.Store.UpsertAgent(rec)

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
		ts := r.Header.Get("X-Timestamp")
		sig := r.Header.Get("X-Signature")
		bodySha := r.Header.Get("X-Body-Sha256")

		if agentID == "" || ts == "" || sig == "" || bodySha == "" {
			writeJSON(w, 401, map[string]any{"error": "missing auth headers"})
			return
		}

		rec, ok := a.Store.GetAgent(agentID)
		if !ok {
			writeJSON(w, 401, map[string]any{"error": "unknown agent"})
			return
		}

		// Basic timestamp sanity window (10 min)
		// (keep it simple; if clock skew becomes annoying, widen it)
		tInt, _ := parseInt64(ts)
		now := time.Now().Unix()
		if tInt == 0 || tInt < now-600 || tInt > now+600 {
			writeJSON(w, 401, map[string]any{"error": "timestamp outside window"})
			return
		}

		pub, err := shared.DecodePubKey(rec.PublicKey)
		if err != nil {
			writeJSON(w, 500, map[string]any{"error": "server key decode failed"})
			return
		}

		path := r.URL.Path
		method := r.Method
		if !shared.Verify(pub, sig, ts, method, path, bodySha) {
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

	rec, ok := a.Store.GetAgent(hb.AgentID)
	if ok {
		rec.Info = hb.Info
		rec.Tags = hb.Tags
		rec.LastSeen = time.Now()
		a.Store.UpsertAgent(rec)
	}

	writeJSON(w, 200, shared.HeartbeatResponse{Ok: true, ServerTime: time.Now().Unix()})
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
	jobs := a.Store.DequeueJobs(agentID, 5)
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
	a.Store.AddResult(res)
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
	a.Store.QueueJob(req.TargetAgentID, job)
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
