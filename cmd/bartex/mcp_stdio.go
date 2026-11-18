package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/barrier-networks/bartex/harness"
)

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
