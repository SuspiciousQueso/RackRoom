package agent

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"rackroom/internal/shared"
)

type Agent struct {
	ConfigPath string
	Cfg        *shared.AgentConfig
	Priv       ed25519.PrivateKey // ed25519 private key bytes
	Client     *http.Client
	invCache   []byte
	lastInvAt  int64
}

func New(configPath string) (*Agent, error) {
	cfg, err := shared.LoadAgentConfig(configPath)
	if err != nil {
		return nil, err
	}
	a := &Agent{
		ConfigPath: configPath,
		Cfg:        cfg,
		Client:     &http.Client{Timeout: 20 * time.Second},
	}
	if cfg.PrivateKeyPath == "" {
		cfg.PrivateKeyPath = defaultKeyPath()
	}
	if err := a.ensureKey(); err != nil {
		return nil, err
	}
	return a, nil
}

func defaultKeyPath() string {
	if runtime.GOOS == "windows" {
		return `C:\ProgramData\RackRoom\agent.key`
	}
	return `/etc/rackroom/agent.key`
}

func (a *Agent) ensureKey() error {
	b, err := os.ReadFile(a.Cfg.PrivateKeyPath)
	if err == nil {
		priv, err := shared.DecodePrivKey(strings.TrimSpace(string(b)))
		if err != nil {
			return err
		}
		a.Priv = priv
		return nil
	}

	// create dirs if needed
	if runtime.GOOS != "windows" {
		_ = os.MkdirAll("/etc/rackroom", 0700)
	} else {
		_ = os.MkdirAll(`C:\ProgramData\RackRoom`, 0700)
	}

	_, privB64, err := shared.GenKeypair()
	if err != nil {
		return err
	}
	if err := os.WriteFile(a.Cfg.PrivateKeyPath, []byte(privB64), 0600); err != nil {
		return err
	}
	priv, err := shared.DecodePrivKey(privB64)
	if err != nil {
		return err
	}
	a.Priv = priv
	return nil
}

func (a *Agent) EnrollIfNeeded(ctx context.Context) error {
	if a.Cfg.AgentID != "" {
		return nil
	}
	if a.Cfg.EnrollToken == "" {
		return errors.New("missing enroll_token and no agent_id")
	}

	pub := a.Priv.Public().(ed25519.PublicKey)
	pubB64 := base64.StdEncoding.EncodeToString(pub)

	req := shared.EnrollRequest{
		EnrollToken: a.Cfg.EnrollToken,
		PublicKey:   pubB64,
		Info: shared.AgentInfo{
			Hostname: hostname(),
			OS:       runtime.GOOS,
			Arch:     runtime.GOARCH,
		},
		Tags: a.Cfg.Tags,
	}
	body, _ := json.Marshal(req)

	url := strings.TrimRight(a.Cfg.ServerURL, "/") + "/v1/enroll"
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.Client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return errors.New("enroll failed: " + string(b))
	}

	var er shared.EnrollResponse
	_ = json.Unmarshal(b, &er)

	a.Cfg.AgentID = er.AgentID
	a.Cfg.EnrollToken = "" // one-time use
	if err := shared.SaveAgentConfig(a.ConfigPath, a.Cfg); err != nil {
		return err
	}
	return nil
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}

func (a *Agent) signedRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	url := strings.TrimRight(a.Cfg.ServerURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	pub := a.Priv.Public().(ed25519.PublicKey)
	req.Header.Set("X-PubKey", base64.StdEncoding.EncodeToString(pub))

	ts := time.Now().Unix()
	tsStr := itoa(ts)

	bodySha := shared.BodySHA256(body)
	sig := shared.Sign(a.Priv, tsStr, method, path, bodySha)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Id", a.Cfg.AgentID)
	req.Header.Set("X-Timestamp", tsStr)
	req.Header.Set("X-Body-Sha256", bodySha)
	req.Header.Set("X-Signature", sig)
	return req, nil
}

func stringMust(b []byte, err error) string {
	if err != nil {
		return ""
	}
	return string(b)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [32]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + (n % 10))
		n /= 10
	}
	return string(buf[i:])
}

func (a *Agent) SendHeartbeat(ctx context.Context) error {
	now := time.Now().Unix()

	// Refresh inventory every 10 minutes (600s)
	if a.invCache == nil || now-a.lastInvAt >= 600 {
		if inv, err := collectInventoryJSON(); err == nil && len(inv) > 0 {
			a.invCache = inv
			a.lastInvAt = now
		}
	}

	hb := shared.HeartbeatRequest{
		AgentID: a.Cfg.AgentID,
		Info: shared.AgentInfo{
			Hostname: hostname(),
			OS:       runtime.GOOS,
			Arch:     runtime.GOARCH,
		},
		Tags:      a.Cfg.Tags,
		Inventory: a.invCache, // <-- []byte (json.RawMessage)
	}

	body, _ := json.Marshal(hb)

	req, err := a.signedRequest(ctx, "POST", "/v1/heartbeat", body)
	if err != nil {
		return err
	}

	resp, err := a.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return errors.New("heartbeat failed: " + string(b))
	}

	return nil
}

func (a *Agent) PollJobs(ctx context.Context) ([]shared.Job, error) {
	// polling endpoint is not signed yet (fine for v0)
	url := strings.TrimRight(a.Cfg.ServerURL, "/") + "/v1/jobs/poll?agent_id=" + a.Cfg.AgentID
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)

	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, errors.New("poll failed: " + string(b))
	}

	var pr shared.JobsPollResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, err
	}
	return pr.Jobs, nil
}

func (a *Agent) RunJob(ctx context.Context, job shared.Job) shared.JobResult {
	start := time.Now().Unix()
	exitCode, out, errOut := execCommand(ctx, job)
	finish := time.Now().Unix()

	return shared.JobResult{
		JobID:      job.JobID,
		AgentID:    a.Cfg.AgentID,
		ExitCode:   exitCode,
		Stdout:     out,
		Stderr:     errOut,
		StartedAt:  start,
		FinishedAt: finish,
	}
}

func execCommand(ctx context.Context, job shared.Job) (int, string, string) {
	timeout := time.Duration(job.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd

	switch strings.ToLower(job.Shell) {
	case "bash":
		cmd = exec.CommandContext(cctx, "bash", "-lc", job.Command)
	case "cmd":
		cmd = exec.CommandContext(cctx, "cmd.exe", "/C", job.Command)
	default:
		// fallback
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(cctx, "cmd.exe", "/C", job.Command)
		} else {
			cmd = exec.CommandContext(cctx, "bash", "-lc", job.Command)
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitCode = 1
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
	}
	return exitCode, stdout.String(), stderr.String()
}

func (a *Agent) PostResult(ctx context.Context, res shared.JobResult) error {
	body, _ := json.Marshal(res)
	req, err := a.signedRequest(ctx, "POST", "/v1/job_result", body)
	if err != nil {
		return err
	}

	resp, err := a.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return errors.New("post result failed: " + string(b))
	}
	return nil
}
