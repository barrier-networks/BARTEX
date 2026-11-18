package mcpserver

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// protocolVersion is the MCP protocol revision this server implements.
const protocolVersion = "2024-11-05"

// maxBodyBytes caps a single JSON-RPC request body.
const maxBodyBytes = 1 << 20

// SessionBinder resolves an inbound bearer token to the Toolset for that session,
// or returns ok=false to reject the request. The live server binds a token to a
// Barrier task (and its engagement/scope) and returns a GuardedToolset, so every
// tool call is governed for that task. A binder MUST compare tokens in constant
// time.
type SessionBinder func(token string) (Toolset, bool)

// Server is a minimal MCP server over Streamable HTTP. It is transport- and
// governance-agnostic: all policy lives behind the SessionBinder (which Toolset,
// and whether to admit the token) and the GuardedToolset (whether a call runs).
type Server struct {
	name    string
	version string
	bind    SessionBinder
}

// New returns an MCP server. name/version are reported in initialize; bind
// resolves each request's bearer token to a Toolset.
func New(name, version string, bind SessionBinder) *Server {
	return &Server{name: name, version: version, bind: bind}
}

// ServeHTTP implements the Streamable HTTP transport: the client POSTs a
// JSON-RPC request and receives a JSON response. Every request must carry a
// bearer token the SessionBinder admits.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ts, ok := s.bind(bearerToken(r))
	if !ok || ts == nil {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeRPCError(w, nil, codeParseError, "could not read request body")
		return
	}
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeRPCError(w, nil, codeParseError, "invalid JSON")
		return
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		writeRPCError(w, req.ID, codeInvalidRequest, "not a JSON-RPC 2.0 request")
		return
	}
	isNotification := len(req.ID) == 0

	switch req.Method {
	case "initialize":
		// Assign a session id purely for protocol compliance; the Toolset is
		// always derived from the bearer token, so the server holds no session
		// state and cannot be desynchronised.
		w.Header().Set("Mcp-Session-Id", newSessionID())
		writeRPCResult(w, req.ID, map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": s.name, "version": s.version},
		})

	case "notifications/initialized", "notifications/cancelled", "ping":
		if req.Method == "ping" && !isNotification {
			writeRPCResult(w, req.ID, map[string]any{})
			return
		}
		w.WriteHeader(http.StatusAccepted)

	case "tools/list":
		defs := ts.List(r.Context())
		if defs == nil {
			defs = []ToolDef{}
		}
		writeRPCResult(w, req.ID, map[string]any{"tools": defs})

	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || strings.TrimSpace(p.Name) == "" {
			writeRPCError(w, req.ID, codeInvalidParams, "tools/call requires a tool name")
			return
		}
		res, err := ts.Call(r.Context(), p.Name, p.Arguments)
		if err != nil {
			// A transport/internal failure (not a tool- or guard-level block,
			// which come back as an isError result below).
			writeRPCError(w, req.ID, codeInternalError, err.Error())
			return
		}
		writeRPCResult(w, req.ID, map[string]any{
			"content": []map[string]any{{"type": "text", "text": res.Text}},
			"isError": res.IsError,
		})

	default:
		if isNotification {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		writeRPCError(w, req.ID, codeMethodNotFound, "method not found: "+req.Method)
	}
}

// bearerToken extracts the token from an Authorization: Bearer <token> header.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

func newSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "0"
	}
	return hex.EncodeToString(b[:])
}

func writeRPCResult(w http.ResponseWriter, id json.RawMessage, result any) {
	writeJSON(w, rpcResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func writeRPCError(w http.ResponseWriter, id json.RawMessage, code int, msg string) {
	writeJSON(w, rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
