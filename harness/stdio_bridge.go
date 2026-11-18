package harness

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// StdioBridge forwards MCP JSON-RPC between a stdio-only CLI and the governed
// BARTEX Tools MCP server (Phase 0, Streamable HTTP at /api/mcp). A CLI that
// speaks MCP only over a spawned stdio subprocess runs `bartex mcp-stdio`; this
// bridge reads newline-delimited JSON-RPC messages on stdin, POSTs each to the
// HTTP endpoint with the task-scoped bearer token, and writes the response back
// on stdout — so the same governed endpoint serves both transports.
//
// Framing is one JSON message per line (the MCP stdio convention: messages
// contain no embedded newlines). A request (with an id) yields exactly one
// response line; a notification (no id) yields none, mirroring the HTTP server's
// 202-with-empty-body, so the CLI never sees a spurious blank reply.
type StdioBridge struct {
	Endpoint string // e.g. http://127.0.0.1:8787/api/mcp
	Token    string // task-scoped bearer token
	Client   *http.Client
}

// Run pumps stdin→HTTP→stdout until stdin closes or ctx is cancelled. It returns
// the first fatal transport error; a per-message HTTP error is reported inline as
// a JSON-RPC error object so the session continues.
func (b StdioBridge) Run(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	if strings.TrimSpace(b.Endpoint) == "" || strings.TrimSpace(b.Token) == "" {
		return fmt.Errorf("mcp-stdio: endpoint and token are required (BARTEX_MCP_ENDPOINT, BARTEX_MCP_TOKEN)")
	}
	client := b.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	sc := bufio.NewScanner(stdin)
	// MCP messages can be large (a tools/call with a big argument blob); lift the
	// scanner's line cap well above the default 64 KiB.
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		resp, err := b.forward(ctx, client, line)
		if err != nil {
			// Transport failure reaching the BARTEX server: surface as a JSON-RPC
			// error keyed to this message's id, so the CLI sees a proper error
			// rather than a dropped turn.
			resp = rpcErrorLine(line, err)
		}
		if len(resp) == 0 {
			continue // notification: no reply, like the HTTP server's 202
		}
		if _, err := stdout.Write(append(resp, '\n')); err != nil {
			return err
		}
	}
	return sc.Err()
}

func (b StdioBridge) forward(ctx context.Context, client *http.Client, msg []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.Endpoint, bytes.NewReader(msg))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.Token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized: the MCP token was rejected (expired or revoked)")
	}
	// A 202/empty body is a notification ack: no line to emit.
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, nil
	}
	return bytes.TrimSpace(body), nil
}
