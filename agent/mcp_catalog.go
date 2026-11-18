package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	actool "github.com/Autumn-27/norma/tool"
	"github.com/barrier-networks/bartex/mcpserver"
)

// GuardCheck is the governance seam the catalog runs before every tool call. It
// returns a non-nil error (its text is the platform-control block message) to
// block the call, or nil to allow it. guard.Guard.CheckToolCall satisfies it.
//
// The caller is responsible for attaching the task's target scope to the context
// it passes to Call (intercept.WithTargetScope), exactly as the engine does for a
// worker run, so the gate and the RoE engagement policy see the same scope the
// built-in loop would.
type GuardCheck func(ctx context.Context, toolName string, input json.RawMessage) error

// GuardedCatalog adapts a curated set of BARTEX CoreTools to the MCP server's
// Toolset interface, running GuardCheck before each call. It is how an external
// harness drives the governed catalog: tools/list advertises the catalog; each
// tools/call passes the same scope / deny / RoE / target-gate / approval checks
// as a worker's own call, and on a pass executes the identical CoreTool — so
// assets, facts, findings and traffic bindings are written back exactly as they
// are for the built-in loop.
//
// Only execution is governed; listing is not (a client may always see the
// catalog). A blocked call never reaches the underlying tool and comes back as
// an isError result carrying the guard's reason, indistinguishable to the model
// from any other platform-control block.
type GuardedCatalog struct {
	tools map[string]actool.CoreTool
	order []string
	guard GuardCheck
	// workingDir is handed to each tool as its ToolContext.WorkingDir (the task's
	// run directory). maxOutputChars bounds a single tool result.
	workingDir     string
	maxOutputChars int
}

// NewGuardedCatalog builds a catalog from curated tools. Duplicate names keep the
// first occurrence. guard may be nil (every call allowed) only in tests; the live
// server always passes a real check.
func NewGuardedCatalog(tools []actool.CoreTool, guard GuardCheck, workingDir string, maxOutputChars int) *GuardedCatalog {
	c := &GuardedCatalog{
		tools:          make(map[string]actool.CoreTool, len(tools)),
		guard:          guard,
		workingDir:     workingDir,
		maxOutputChars: maxOutputChars,
	}
	for _, t := range tools {
		if t == nil {
			continue
		}
		name := t.Name()
		if _, seen := c.tools[name]; seen {
			continue
		}
		c.tools[name] = t
		c.order = append(c.order, name)
	}
	return c
}

// List returns the catalog in a stable order (tools/list).
func (c *GuardedCatalog) List(ctx context.Context) []mcpserver.ToolDef {
	defs := make([]mcpserver.ToolDef, 0, len(c.order))
	for _, name := range c.order {
		t := c.tools[name]
		schema, err := json.Marshal(t.InputSchema())
		if err != nil || len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object"}`)
		}
		defs = append(defs, mcpserver.ToolDef{
			Name:        name,
			Description: t.Description(),
			InputSchema: schema,
		})
	}
	return defs
}

// Call runs the guard, then (on a pass) the tool. A guard block and a tool-level
// error both come back as an isError ToolResult; only a missing tool is a
// protocol error, so a client that calls an unknown name learns it is unknown
// rather than silently getting an allow.
func (c *GuardedCatalog) Call(ctx context.Context, name string, arguments json.RawMessage) (mcpserver.ToolResult, error) {
	t, ok := c.tools[name]
	if !ok {
		return mcpserver.ToolResult{}, fmt.Errorf("unknown tool: %q", name)
	}
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	if c.guard != nil {
		if err := c.guard(ctx, name, arguments); err != nil {
			return mcpserver.ToolResult{Text: err.Error(), IsError: true}, nil
		}
	}
	res, err := t.Call(ctx, arguments, &actool.ToolContext{
		WorkingDir:     c.workingDir,
		MaxOutputChars: c.maxOutputChars,
	})
	if err != nil {
		// A tool returning a transport-style error (rare for BARTEX domain tools)
		// is surfaced to the model as an isError result, not a JSON-RPC error, so
		// the harness keeps going rather than treating it as a protocol fault.
		return mcpserver.ToolResult{Text: err.Error(), IsError: true}, nil
	}
	return mcpserver.ToolResult{Text: resultText(res), IsError: res.IsError}, nil
}

// resultText flattens a tool Result's content blocks to the plain text an MCP
// text content block carries. Only text blocks survive — BARTEX domain tools
// return text — and Extra (the privileged host channel) is deliberately dropped.
func resultText(res actool.Result) string {
	var b strings.Builder
	for _, blk := range res.Content {
		if blk.Text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(blk.Text)
	}
	return b.String()
}
