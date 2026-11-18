package server

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"time"

	nh "github.com/Autumn-27/norma/harness"
	"github.com/barrier-networks/bartex/agent"
	"github.com/barrier-networks/bartex/db"
	"github.com/barrier-networks/bartex/harness"
)

// runHarnessIntent executes one claimed intent on an external agentic CLI instead
// of the built-in agentcore loop. It is installed on the engine (SetHarnessRunner)
// and called only when a task selects a harness profile, so tasks without one are
// byte-for-byte unaffected.
//
// The CLI is pointed at the governed BARTEX Tools MCP server (its tool calls are
// gated + written back through the same pipeline a worker uses) and confined by
// the Seatbelt sandbox with egress forced through the recording proxy. External
// token accounting is opaque, so the terminal reason is derived from the exit
// status / context, and write-back counts are not re-derived (they are used only
// for logging).
func (s *Server) runHarnessIntent(ctx context.Context, t *Task, name string, intent *db.Node, emit func(db.Activity)) (nh.TerminalReason, agent.WriteCounts, error) {
	if t.HarnessProfileID == nil {
		return nh.ReasonModelError, agent.WriteCounts{}, errors.New("no harness profile on task")
	}
	prof, err := s.m.pg.HarnessProfileByID(*t.HarnessProfileID)
	if err != nil || prof == nil {
		return nh.ReasonModelError, agent.WriteCounts{}, errors.New("harness profile not found")
	}

	sandbox := harness.NewSandbox()
	if prof.DisableNativeSandbox && !sandbox.Available() {
		// Fail-safe: the CLI runs with its own sandbox off, so ours MUST confine
		// it. No sandbox ⇒ do not launch (the design doc's hard rule).
		emitHarnessLine(emit, intent.ID, name, true, "refusing to launch: no OS sandbox available to confine the harness")
		return nh.ReasonModelError, agent.WriteCounts{}, harness.ErrSandboxUnavailable
	}

	token, _, err := s.mcpTokens.mint(t.ID)
	if err != nil {
		return nh.ReasonModelError, agent.WriteCounts{}, err
	}
	runDir := agent.EnsureRunDir(s.m.DataDir(), intentID64(t), intent.ID)

	l := harness.Launch{
		Kind:                 prof.Kind,
		Command:              prof.Command,
		Args:                 prof.Args,
		Env:                  prof.Env,
		Prompt:               agent.IntentPrompt(intent),
		RunDir:               runDir,
		SelfExe:              selfExe(),
		Transport:            prof.MCPTransport,
		ProxyEnv:             agent.ProxyEnv(s.m.ProxyAddr(), s.m.ProxyCACert()),
		MCPEndpoint:          s.mcpEndpoint(),
		MCPToken:             token,
		DisableNativeSandbox: prof.DisableNativeSandbox,
		MaxDuration:          time.Duration(prof.MaxDurationSeconds) * time.Second,
	}

	emitHarnessLine(emit, intent.ID, name, false, "launching harness "+prof.Kind+" ("+prof.Command+") in "+sandbox.Name()+" sandbox")
	lw := &harnessLogWriter{emit: emit, intentID: intent.ID, worker: name}
	runErr := l.Run(ctx, sandbox, lw)
	lw.flush()

	switch {
	case ctx.Err() != nil:
		// Pause/kill/deadline cancelled the run: settle like a worker abort.
		return nh.ReasonAbortedTools, agent.WriteCounts{}, ctx.Err()
	case runErr != nil:
		emitHarnessLine(emit, intent.ID, name, true, "harness exited with error: "+runErr.Error())
		return nh.ReasonModelError, agent.WriteCounts{}, runErr
	default:
		emitHarnessLine(emit, intent.ID, name, false, "harness completed")
		return nh.ReasonCompleted, agent.WriteCounts{}, nil
	}
}

func intentID64(t *Task) int64 {
	id, _ := parseTaskID(t.ID)
	return id
}

// mcpEndpoint is the loopback URL of the governed MCP server the harness points
// at. It is on the same bind address as the API; the sandbox allows loopback
// egress so the CLI (and the stdio bridge) can reach it.
func (s *Server) mcpEndpoint() string {
	base := strings.TrimRight(s.selfBaseURL, "/")
	if base == "" {
		base = "http://127.0.0.1:8787"
	}
	return base + "/api/harness-mcp"
}

// SetSelfBaseURL records this server's own loopback base URL (scheme+host+port,
// no trailing path), so the external-harness launcher can point a CLI at the
// governed MCP endpoint. The binary sets it from the bind address.
func (s *Server) SetSelfBaseURL(url string) { s.selfBaseURL = strings.TrimRight(url, "/") }

func selfExe() string {
	if p, err := os.Executable(); err == nil {
		return p
	}
	return "bartex"
}

// harnessLogWriter turns the harness subprocess's stdout/stderr stream into
// activity rows, one per line, so the run is visible on the task timeline like a
// worker's steps.
type harnessLogWriter struct {
	emit     func(db.Activity)
	intentID int64
	worker   string
	buf      bytes.Buffer
}

func (w *harnessLogWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	for {
		line, rest, found := bytes.Cut(w.buf.Bytes(), []byte{'\n'})
		if !found {
			break
		}
		s := strings.TrimRight(string(line), "\r")
		if strings.TrimSpace(s) != "" {
			emitHarnessLine(w.emit, w.intentID, w.worker, false, s)
		}
		w.buf = *bytes.NewBuffer(append([]byte(nil), rest...))
	}
	return len(p), nil
}

func (w *harnessLogWriter) flush() {
	s := strings.TrimSpace(w.buf.String())
	if s != "" {
		emitHarnessLine(w.emit, w.intentID, w.worker, false, s)
	}
	w.buf.Reset()
}

func emitHarnessLine(emit func(db.Activity), intentID int64, worker string, isErr bool, text string) {
	if emit == nil {
		return
	}
	nid := intentID
	emit(db.Activity{
		NodeID:  &nid,
		Worker:  worker,
		Kind:    "harness",
		IsError: isErr,
		Summary: firstHarnessLine(text, 400),
		Detail:  text,
	})
}

func firstHarnessLine(s string, n int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > n {
		return s[:n]
	}
	return s
}
