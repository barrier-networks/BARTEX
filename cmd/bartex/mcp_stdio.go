package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/barrier-networks/bartex/harness"
)

// loopbackHostPort turns a bind address (":8787", "0.0.0.0:8787",
// "127.0.0.1:8787") into a host:port a sandboxed subprocess can reach over
// loopback. A wildcard or empty host becomes 127.0.0.1.
func loopbackHostPort(addr string) string {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return "127.0.0.1:8787"
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// runMCPStdio runs the stdio↔HTTP MCP bridge: it forwards MCP JSON-RPC between
// this process's stdin/stdout and the governed BARTEX Tools MCP server. An
// external harness that only speaks MCP over stdio spawns `bartex mcp-stdio`
// with BARTEX_MCP_ENDPOINT and BARTEX_MCP_TOKEN set (the launcher injects both),
// so the same governed /api/mcp endpoint serves stdio and HTTP clients alike.
func runMCPStdio() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	b := harness.StdioBridge{
		Endpoint: os.Getenv("BARTEX_MCP_ENDPOINT"),
		Token:    os.Getenv("BARTEX_MCP_TOKEN"),
	}
	if err := b.Run(ctx, os.Stdin, os.Stdout); err != nil && ctx.Err() == nil {
		log.Printf("[mcp-stdio] %v", err)
		return 1
	}
	return 0
}
