package harness

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The bridge forwards a request line to the HTTP endpoint with the bearer token
// and writes the single response line back; a notification (no id) produces no
// output, mirroring the server's 202.
func TestStdioBridgeForwards(t *testing.T) {
	var gotAuth string
	var gotBodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		gotBodies = append(gotBodies, string(body))
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		_ = json.Unmarshal(body, &req)
		if len(req.ID) == 0 { // notification
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(req.ID) + `,"result":{"ok":true}}`))
	}))
	defer srv.Close()

	in := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n" +
			`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"ping"}` + "\n")
	var out strings.Builder
	b := StdioBridge{Endpoint: srv.URL, Token: "tok123"}
	if err := b.Run(context.Background(), in, &out); err != nil {
		t.Fatal(err)
	}

	if gotAuth != "Bearer tok123" {
		t.Errorf("auth header = %q", gotAuth)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 { // two requests → two replies; the notification is silent
		t.Fatalf("want 2 response lines, got %d: %q", len(lines), out.String())
	}
	for _, l := range lines {
		if !strings.Contains(l, `"result"`) {
			t.Errorf("bad response line: %s", l)
		}
	}
}

// A transport failure reaching the server becomes a JSON-RPC error line keyed to
// the request id, not a dropped turn.
func TestStdioBridgeTransportErrorBecomesRPCError(t *testing.T) {
	b := StdioBridge{Endpoint: "http://127.0.0.1:0/api/mcp", Token: "t"}
	var out strings.Builder
	err := b.Run(context.Background(), strings.NewReader(`{"jsonrpc":"2.0","id":9,"method":"tools/list"}`+"\n"), &out)
	if err != nil {
		t.Fatalf("per-message failure must not be fatal: %v", err)
	}
	if !strings.Contains(out.String(), `"error"`) || !strings.Contains(out.String(), `"id":9`) {
		t.Fatalf("want rpc error for id 9, got %q", out.String())
	}
}

func TestStdioBridgeRequiresConfig(t *testing.T) {
	if err := (StdioBridge{}).Run(context.Background(), strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("want error when endpoint/token unset")
	}
}

// A 401 from the server surfaces as an rpc error naming the rejected token.
func TestStdioBridgeUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	var out strings.Builder
	_ = StdioBridge{Endpoint: srv.URL, Token: "bad"}.Run(context.Background(),
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`+"\n"), &out)
	if !strings.Contains(out.String(), "token was rejected") {
		t.Fatalf("want unauthorized rpc error, got %q", out.String())
	}
}
