package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/barrier-networks/bartex/agent"
	"github.com/barrier-networks/bartex/intercept"
	"github.com/barrier-networks/bartex/mcpserver"
)

// mcpTokenTTL is how long a minted MCP session token stays valid. An external
// harness is launched with a fresh token per engagement; a short life bounds the
// blast radius of a leaked token without interrupting a running harness (the
// subprocess holds the token for its lifetime, well under this).
const mcpTokenTTL = 12 * time.Hour

// mcpTokenEntry binds one bearer token (by its hash) to a task.
type mcpTokenEntry struct {
	taskID  string
	expires time.Time
}

// mcpTokenStore maps the SHA-256 of a bearer token to the task it scopes. The
// raw token is never stored, so a store dump does not yield usable tokens, and
// lookup is by hash of the presented token — an attacker cannot time-analyse a
// map keyed on the secret's digest to recover it.
type mcpTokenStore struct {
	mu sync.Mutex
	m  map[string]mcpTokenEntry
}

func newMCPTokenStore() *mcpTokenStore { return &mcpTokenStore{m: map[string]mcpTokenEntry{}} }

func hashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

// mint creates a token scoping the given task and returns the raw token (shown
// once). It also drops any entries that have expired, so the store does not grow
// without bound as engagements come and go.
func (s *mcpTokenStore) mint(taskID string) (string, time.Time, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", time.Time{}, err
	}
	tok := hex.EncodeToString(b[:])
	exp := time.Now().Add(mcpTokenTTL)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, e := range s.m {
		if now.After(e.expires) {
			delete(s.m, k)
		}
	}
	s.m[hashToken(tok)] = mcpTokenEntry{taskID: taskID, expires: exp}
	return tok, exp, nil
}

// resolve returns the task id a live token scopes, or ok=false when the token is
// unknown or expired.
func (s *mcpTokenStore) resolve(tok string) (string, bool) {
	if tok == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[hashToken(tok)]
	if !ok {
		return "", false
	}
	if time.Now().After(e.expires) {
		delete(s.m, hashToken(tok))
		return "", false
	}
	return e.taskID, true
}

// revokeTask drops every token scoping a task (e.g. when it is deleted).
func (s *mcpTokenStore) revokeTask(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, e := range s.m {
		if e.taskID == taskID {
			delete(s.m, k)
		}
	}
}

// mcpSessionToolset wraps the agent GuardedCatalog and attaches the task's target
// scope to the context of every call, exactly as the engine does before a worker
// run. Without this the target gate and the RoE engagement policy would see no
// scope and could not enforce pins, approvals or the engagement window.
type mcpSessionToolset struct {
	base  *agent.GuardedCatalog
	scope intercept.TargetScope
}

func (t mcpSessionToolset) List(ctx context.Context) []mcpserver.ToolDef { return t.base.List(ctx) }

func (t mcpSessionToolset) Call(ctx context.Context, name string, args json.RawMessage) (mcpserver.ToolResult, error) {
	ctx = intercept.WithTargetScope(ctx, t.scope)
	return t.base.Call(ctx, name, args)
}

// bindMCPSession is the mcpserver.SessionBinder: it resolves a bearer token to a
// task and returns a governed catalog scoped to that task. Every tools/call then
// runs through the task's own Guard (the same applyIntercept pipeline a worker
// uses) with the task's scope attached, and executes the identical BARTEX domain
// tools — so deny rules, the RoE engagement gate, the target gate and all
// write-back (assets / facts / findings) behave exactly as for the built-in loop.
func (s *Server) bindMCPSession(token string) (mcpserver.Toolset, bool) {
	taskID, ok := s.mcpTokens.resolve(token)
	if !ok {
		return nil, false
	}
	t, ok := s.m.Task(taskID)
	if !ok || t == nil || t.Store == nil {
		return nil, false
	}

	// Build a task-scoped ToolSet bound to the real stores and the evidence-backed
	// finding recorder, then expose the plan+execute catalog: in whole-task mode
	// the CLI drives the engagement end to end (shape goals/intents, enumerate,
	// execute, and report findings) through these governed tools, while its own
	// native tools do recon/exploitation behind the recording proxy.
	tsx := agent.NewToolSet(t.Store, "mcp")
	tid, _ := parseTaskID(taskID)
	tsx.SetTaskID(tid)
	tsx.SetOwnerNode(0) // task-level session: assets anchor to the task root, not one intent
	tsx.SetFindingRecorder(s.evidenceStore())
	if as := s.m.Assets(); as != nil {
		tsx.SetAssetStore(as, as.Companies())
		tsx.SetCoverageEnabled(t.CoverageEnabled)
	}
	tools := tsx.HarnessLeadTools()
	if !t.CoverageEnabled {
		tools = tsx.DropCoverageTools(tools)
	}

	catalog := agent.NewGuardedCatalog(tools, t.Guard.CheckToolCall, "", 0)
	return mcpSessionToolset{base: catalog, scope: t.targetScope(intercept.DefaultTargetWaitBound)}, true
}

// mcpHandler is the governed BARTEX Tools MCP server. It authenticates on its own
// bearer token (the SessionBinder), so it is mounted outside the operator-JWT
// requireAuth wrapper.
func (s *Server) mcpHandler() http.Handler {
	return mcpserver.New("bartex-tools", BuildVersion, s.bindMCPSession)
}

// mintTaskMCPToken handles POST /api/tasks/{id}/mcp-token: it issues a fresh
// task-scoped MCP bearer token and returns it with the endpoint a harness points
// at. The token is shown once; the caller stores it in the harness profile's env.
func (s *Server) mintTaskMCPToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, ok := s.m.Task(id)
	if !ok || t == nil {
		writeErr(w, 404, "task not found")
		return
	}
	tok, exp, err := s.mcpTokens.mint(t.ID)
	if err != nil {
		writeErr(w, 500, "could not mint token")
		return
	}
	writeJSON(w, 200, map[string]any{
		"token":      tok,
		"endpoint":   "/api/harness-mcp",
		"expires_at": exp.Unix(),
		"task_id":    t.ID,
	})
}
