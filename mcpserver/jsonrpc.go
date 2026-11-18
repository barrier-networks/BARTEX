// Package mcpserver is a minimal, correct Model Context Protocol (MCP) server
// over Streamable HTTP. It exposes a set of tools to an external MCP client (an
// agentic harness such as Claude Code, Codex or Aider) and is deliberately
// governance-agnostic: the caller supplies a SessionBinder that resolves a
// bearer token to a Toolset, and a GuardedToolset routes every tool call through
// a guard before it executes. This is the substrate for Phase 0 of the
// external-harness execution backend (see docs/harness-execution-backend.md):
// the live server plugs guard.applyIntercept and the governed tool catalog in at
// the SessionBinder / GuardFunc seams, so an external harness's tool calls are
// scoped, approval-gated and evidence-bound exactly like the built-in loop's.
package mcpserver

import "encoding/json"

// rpcRequest is a JSON-RPC 2.0 request. A request with no id is a notification
// and receives no response body.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// rpcResponse is a JSON-RPC 2.0 response. Exactly one of Result / Error is set.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// JSON-RPC 2.0 standard error codes.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)
