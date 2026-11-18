package harness

import (
	"encoding/json"
)

// rpcErrorLine builds a JSON-RPC 2.0 error response that echoes the id of the
// request in reqLine, so a CLI awaiting that id sees a proper error instead of a
// hung turn. When the id cannot be parsed (e.g. a notification) it returns nil,
// since a notification expects no reply.
func rpcErrorLine(reqLine []byte, cause error) []byte {
	var req struct {
		ID json.RawMessage `json:"id"`
	}
	_ = json.Unmarshal(reqLine, &req)
	if len(req.ID) == 0 {
		return nil
	}
	msg := "bartex mcp bridge: " + cause.Error()
	out, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"error":   map[string]any{"code": -32000, "message": msg},
	})
	if err != nil {
		return nil
	}
	return out
}
