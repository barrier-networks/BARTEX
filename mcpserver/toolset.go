package mcpserver

import (
	"context"
	"encoding/json"
)

// ToolDef describes one tool exposed over MCP (tools/list). InputSchema is a
// JSON Schema object for the tool's arguments.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToolResult is the outcome of one tool call. Text is returned to the client as
// a single text content block. IsError marks a tool-level failure (the call ran
// but the tool reports an error, e.g. a governance block) as distinct from a
// protocol/transport error.
type ToolResult struct {
	Text    string
	IsError bool
}

// Toolset is the set of tools one MCP session may list and call. An
// implementation binds the session to whatever context scopes those tools (a
// Barrier task and its engagement/scope).
type Toolset interface {
	List(ctx context.Context) []ToolDef
	Call(ctx context.Context, name string, arguments json.RawMessage) (ToolResult, error)
}

// GuardFunc is the governance seam. It runs before a tool executes and returns a
// non-nil error to BLOCK the call. The live server plugs guard.applyIntercept in
// here so an external harness's tool calls pass the same scope / deny / RoE
// engagement / target-gate / approval checks as the built-in agent loop. The
// returned error text is surfaced to the model as the block reason.
type GuardFunc func(ctx context.Context, toolName string, arguments json.RawMessage) error

// GuardedToolset wraps a base Toolset and runs Guard before every Call. A blocked
// call never reaches the base tool; it returns an isError ToolResult carrying the
// guard's reason (so the model sees a platform-control block, not a transport
// error, and cannot tell the call ever reached a tool). List is not gated — a
// client may always see the catalog; only execution is governed.
type GuardedToolset struct {
	Base  Toolset
	Guard GuardFunc
}

func (g GuardedToolset) List(ctx context.Context) []ToolDef { return g.Base.List(ctx) }

func (g GuardedToolset) Call(ctx context.Context, name string, arguments json.RawMessage) (ToolResult, error) {
	if g.Guard != nil {
		if err := g.Guard(ctx, name, arguments); err != nil {
			return ToolResult{Text: err.Error(), IsError: true}, nil
		}
	}
	return g.Base.Call(ctx, name, arguments)
}
