package guard

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Autumn-27/norma/hook"
)

// CheckToolCall runs the full guard pipeline for a single tool call and reports
// whether it is allowed. It returns nil to allow, or a non-nil error whose text
// is the platform-control block message (the same message the built-in agent
// loop shows the model) to block.
//
// It is the governance seam for callers that are not agent-core hooks — notably
// the BARTEX Tools MCP server, which exposes the governed catalog to an external
// harness. Routing every external tool call through this method means an outside
// loop passes exactly the same scope / deny / RoE-engagement / target-gate /
// approval checks as a worker's own tool calls: the policy lives in one place.
//
// The target gate and RoE engagement read their scope from ctx
// (intercept.WithTargetScope), so the caller MUST attach the task's scope to ctx
// before calling, just as the engine does for a worker run.
func (g *Guard) CheckToolCall(ctx context.Context, toolName string, input json.RawMessage) error {
	if g == nil {
		return nil
	}
	res := g.preToolUse(ctx, hook.Event{ToolName: toolName, Input: input})
	if res.Decision == "block" {
		msg := res.Message
		if msg == "" {
			msg = "platform safety controls blocked execution"
		}
		return errors.New(msg)
	}
	return nil
}
