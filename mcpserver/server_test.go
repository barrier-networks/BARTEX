package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeToolset is an in-memory Toolset: one "echo" tool that returns its "text"
// argument.
type fakeToolset struct{ calls int }

func (f *fakeToolset) List(context.Context) []ToolDef {
	return []ToolDef{{
		Name:        "echo",
		Description: "echo back the text argument",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`),
	}}
}

func (f *fakeToolset) Call(_ context.Context, name string, args json.RawMessage) (ToolResult, error) {
	f.calls++
	if name != "echo" {
		return ToolResult{}, errors.New("unknown tool: " + name)
	}
	var p struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(args, &p)
	return ToolResult{Text: p.Text}, nil
}

// newTestServer returns a server whose only valid token is "good". When
// blockHost is non-empty, the guard blocks any call whose args contain it (a
// stand-in for a scope/RoE block).
func newTestServer(base Toolset, blockMarker string) *Server {
	return New("barrier-test", "0.0.1", func(token string) (Toolset, bool) {
		if token != "good" {
			return nil, false
		}
		return GuardedToolset{Base: base, Guard: func(_ context.Context, _ string, args json.RawMessage) error {
			if blockMarker != "" && strings.Contains(string(args), blockMarker) {
				return errors.New("[Barrier platform control, not target defense] ENGAGEMENT_OUT_OF_SCOPE: blocked")
			}
			return nil
		}}, true
	})
}

func do(t *testing.T, s *Server, token, bodyJSON string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(bodyJSON))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	var out map[string]any
	if rr.Body.Len() > 0 && strings.HasPrefix(rr.Header().Get("Content-Type"), "application/json") {
		_ = json.Unmarshal(rr.Body.Bytes(), &out)
	}
	return rr, out
}

func TestInitialize(t *testing.T) {
	s := newTestServer(&fakeToolset{}, "")
	rr, out := do(t, s, "good", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if rr.Code != 200 {
		t.Fatalf("status %d", rr.Code)
	}
	if rr.Header().Get("Mcp-Session-Id") == "" {
		t.Error("no Mcp-Session-Id assigned")
	}
	result, _ := out["result"].(map[string]any)
	if result["protocolVersion"] != protocolVersion {
		t.Errorf("protocolVersion = %v", result["protocolVersion"])
	}
	si, _ := result["serverInfo"].(map[string]any)
	if si["name"] != "barrier-test" {
		t.Errorf("serverInfo.name = %v", si["name"])
	}
}

func TestAuthRejected(t *testing.T) {
	s := newTestServer(&fakeToolset{}, "")
	for _, tok := range []string{"", "bad"} {
		rr, _ := do(t, s, tok, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("token %q: status %d, want 401", tok, rr.Code)
		}
	}
}

func TestToolsList(t *testing.T) {
	s := newTestServer(&fakeToolset{}, "")
	_, out := do(t, s, "good", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	result, _ := out["result"].(map[string]any)
	tools, _ := result["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools = %v", tools)
	}
	tool, _ := tools[0].(map[string]any)
	if tool["name"] != "echo" || tool["inputSchema"] == nil {
		t.Errorf("tool = %v", tool)
	}
}

func TestToolsCallSuccess(t *testing.T) {
	base := &fakeToolset{}
	s := newTestServer(base, "")
	_, out := do(t, s, "good", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"text":"hi there"}}}`)
	result, _ := out["result"].(map[string]any)
	if result["isError"] != false {
		t.Errorf("isError = %v", result["isError"])
	}
	content, _ := result["content"].([]any)
	first, _ := content[0].(map[string]any)
	if first["text"] != "hi there" {
		t.Errorf("text = %v", first["text"])
	}
	if base.calls != 1 {
		t.Errorf("base called %d times", base.calls)
	}
}

// The governance seam blocks the call before it reaches the base tool, and the
// block is surfaced as an isError result (not a transport error).
func TestGuardBlocksCall(t *testing.T) {
	base := &fakeToolset{}
	s := newTestServer(base, "evil.example.com")
	_, out := do(t, s, "good", `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"echo","arguments":{"text":"ping evil.example.com"}}}`)
	result, _ := out["result"].(map[string]any)
	if result["isError"] != true {
		t.Fatalf("blocked call isError = %v, want true", result["isError"])
	}
	content, _ := result["content"].([]any)
	first, _ := content[0].(map[string]any)
	if !strings.Contains(first["text"].(string), "ENGAGEMENT_OUT_OF_SCOPE") {
		t.Errorf("block text = %v", first["text"])
	}
	if base.calls != 0 {
		t.Errorf("base tool executed despite guard block (%d calls)", base.calls)
	}
}

func TestMethodNotFoundAndBadRequests(t *testing.T) {
	s := newTestServer(&fakeToolset{}, "")
	_, out := do(t, s, "good", `{"jsonrpc":"2.0","id":5,"method":"no/such"}`)
	e, _ := out["error"].(map[string]any)
	if e == nil || int(e["code"].(float64)) != codeMethodNotFound {
		t.Errorf("want method-not-found, got %v", out)
	}
	// invalid JSON
	rr, _ := do(t, s, "good", `{not json`)
	if o := decode(rr.Body.Bytes()); o["error"] == nil {
		t.Errorf("invalid JSON did not error: %v", o)
	}
	// tools/call without a name
	_, out = do(t, s, "good", `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"arguments":{}}}`)
	if e, _ := out["error"].(map[string]any); e == nil || int(e["code"].(float64)) != codeInvalidParams {
		t.Errorf("want invalid-params, got %v", out)
	}
}

func TestNotificationNoBody(t *testing.T) {
	s := newTestServer(&fakeToolset{}, "")
	rr, _ := do(t, s, "good", `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if rr.Code != http.StatusAccepted {
		t.Errorf("notification status %d, want 202", rr.Code)
	}
	if rr.Body.Len() != 0 {
		t.Errorf("notification returned a body: %s", rr.Body.String())
	}
}

func TestNonPostRejected(t *testing.T) {
	s := newTestServer(&fakeToolset{}, "")
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer good")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET status %d, want 405", rr.Code)
	}
}

func decode(b []byte) map[string]any {
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}
