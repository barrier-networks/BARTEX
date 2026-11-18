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
	reason, err := s.runHarnessSession(ctx, t, name, intent.ID, agent.IntentPrompt(intent), emit)
	return reason, agent.WriteCounts{}, err
}

// runHarnessTask runs ONE whole-engagement harness session: the CLI is given the
// task's goal and situation and the plan+execute governed catalog, and drives the
// engagement itself (shape goals/intents, enumerate, execute, report findings).
// It is the whole-task-handoff backend used when a task's effective backend is a
// harness. activityNode is the task-root node the streamed log lines anchor to.
func (s *Server) runHarnessTask(ctx context.Context, t *Task, name string, activityNode int64, emit func(db.Activity)) (nh.TerminalReason, error) {
	return s.runHarnessSession(ctx, t, name, activityNode, s.harnessTaskPrompt(t), emit)
}

// runHarnessSession is the shared launcher for both the per-intent and the
// whole-task harness paths: it resolves the task's effective profile, mints a
// task-scoped MCP token, builds the sandboxed invocation, streams stdout/stderr
// as activities, and maps the exit/ctx to a terminal reason.
func (s *Server) runHarnessSession(ctx context.Context, t *Task, name string, activityNode int64, prompt string, emit func(db.Activity)) (nh.TerminalReason, error) {
	pid := s.effectiveHarnessProfileID(t)
	if pid == nil {
		return nh.ReasonModelError, errors.New("no harness profile for task")
	}
	prof, err := s.m.pg.HarnessProfileByID(*pid)
	if err != nil || prof == nil {
		return nh.ReasonModelError, errors.New("harness profile not found")
	}

	sandbox := harness.NewSandbox()
	if prof.DisableNativeSandbox && !sandbox.Available() {
		// Fail-safe: the CLI runs with its own sandbox off, so ours MUST confine
		// it. No sandbox ⇒ do not launch (the design doc's hard rule).
		emitHarnessLine(emit, activityNode, name, true, "refusing to launch: no OS sandbox available to confine the harness")
		return nh.ReasonModelError, harness.ErrSandboxUnavailable
	}

	token, _, err := s.mcpTokens.mint(t.ID)
	if err != nil {
		return nh.ReasonModelError, err
	}
	runDir := agent.EnsureRunDir(s.m.DataDir(), taskID64(t), activityNode)

	l := harness.Launch{
		Kind:                 prof.Kind,
		Command:              prof.Command,
		Args:                 prof.Args,
		Env:                  prof.Env,
		Prompt:               prompt,
		RunDir:               runDir,
		SelfExe:              selfExe(),
		Transport:            prof.MCPTransport,
		ProxyEnv:             agent.ProxyEnv(s.m.ProxyAddr(), s.m.ProxyCACert()),
		MCPEndpoint:          s.mcpEndpoint(),
		MCPToken:             token,
		DisableNativeSandbox: prof.DisableNativeSandbox,
		MaxDuration:          time.Duration(prof.MaxDurationSeconds) * time.Second,
	}

	emitHarnessLine(emit, activityNode, name, false, "launching harness "+prof.Kind+" ("+prof.Command+") in "+sandbox.Name()+" sandbox")
	lw := &harnessLogWriter{emit: emit, intentID: activityNode, worker: name}
	runErr := l.Run(ctx, sandbox, lw)
	lw.flush()

	switch {
	case ctx.Err() != nil:
		return nh.ReasonAbortedTools, ctx.Err()
	case runErr != nil:
		emitHarnessLine(emit, activityNode, name, true, "harness exited with error: "+runErr.Error())
		return nh.ReasonModelError, runErr
	default:
		emitHarnessLine(emit, activityNode, name, false, "harness session completed")
		return nh.ReasonCompleted, nil
	}
}

// harnessTaskPrompt is the whole-engagement instruction handed to the CLI. The
// goal and description are authoritative; the governed MCP tools are how it plans
// and writes back.
func (s *Server) harnessTaskPrompt(t *Task) string {
	var b strings.Builder
	b.WriteString("You are conducting an authorized security engagement. Plan and carry it out yourself using the BARTEX MCP tools (the \"bartex\" server) for all graph state — set and complete goals, add intents, record facts and assets, and report confirmed findings with report_finding.\n\n")
	b.WriteString("Goal:\n")
	b.WriteString(strings.TrimSpace(t.Goal))
	b.WriteString("\n\n")
	if d := strings.TrimSpace(t.Description); d != "" {
		b.WriteString("Scope / description:\n")
		b.WriteString(d)
		b.WriteString("\n\n")
	}
	b.WriteString("Rules: stay strictly within the authorized scope; confirm every finding with concrete evidence before calling report_finding; do only read/enumeration unless the scope explicitly authorizes active testing. When every goal is satisfied, mark it with goal_met and stop. Your own shell/tools run sandbox-confined with all network egress forced through the recording proxy.")
	return b.String()
}

// effectiveHarnessProfileID resolves a task's execution backend: its explicit
// harness_profile_id when set, else the global default (System → Harness), else
// nil (the built-in loop).
func (s *Server) effectiveHarnessProfileID(t *Task) *int64 {
	if t != nil && t.HarnessProfileID != nil {
		return t.HarnessProfileID
	}
	if s.m == nil || s.m.pg == nil {
		return nil
	}
	id, _ := s.m.pg.DefaultHarnessProfileID()
	return id
}

// taskUsesHarness reports whether a task runs on a whole-task harness backend
// (explicit or global default) rather than the built-in LLM loop.
func (s *Server) taskUsesHarness(t *Task) bool {
	return s.effectiveHarnessProfileID(t) != nil
}

func taskID64(t *Task) int64 {
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
