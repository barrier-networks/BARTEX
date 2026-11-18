package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	actool "github.com/Autumn-27/norma/tool"
)

func echoTool() actool.CoreTool {
	return actool.Build(actool.Spec{
		Name:        "echo",
		Description: "echoes its text argument",
		Schema:      map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}},
		Run: func(_ context.Context, in json.RawMessage, _ *actool.ToolContext) (actool.Result, error) {
			var p struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(in, &p)
			return actool.Text("echo:" + p.Text), nil
		},
	})
}

// An allowed call reaches the tool and returns its text; the guard sees the tool
// name and arguments.
func TestGuardedCatalogAllows(t *testing.T) {
	var sawName string
	c := NewGuardedCatalog([]actool.CoreTool{echoTool()}, func(_ context.Context, name string, _ json.RawMessage) error {
		sawName = name
		return nil
	}, "", 0)

	res, err := c.Call(context.Background(), "echo", json.RawMessage(`{"text":"hi"}`))
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if res.IsError || res.Text != "echo:hi" {
		t.Fatalf("result = %+v", res)
	}
	if sawName != "echo" {
		t.Errorf("guard saw name %q", sawName)
	}
}

// A guard block never reaches the tool and comes back as an isError result
// carrying the block reason — not a transport error.
func TestGuardedCatalogBlocks(t *testing.T) {
	ran := false
	tool := actool.Build(actool.Spec{
		Name: "echo", Description: "d", Schema: map[string]any{"type": "object"},
		Run: func(context.Context, json.RawMessage, *actool.ToolContext) (actool.Result, error) {
			ran = true
			return actool.Text("should not run"), nil
		},
	})
	c := NewGuardedCatalog([]actool.CoreTool{tool}, func(context.Context, string, json.RawMessage) error {
		return errors.New("[BARTEX platform control, not target defense] ENGAGEMENT_OUT_OF_SCOPE: blocked")
	}, "", 0)

	res, err := c.Call(context.Background(), "echo", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("block must be an isError result, not a transport error: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Text, "ENGAGEMENT_OUT_OF_SCOPE") {
		t.Fatalf("result = %+v", res)
	}
	if ran {
		t.Error("blocked call still reached the tool")
	}
}

// An unknown tool is a protocol error (so a client learns the name is unknown,
// rather than silently receiving an allow).
func TestGuardedCatalogUnknownTool(t *testing.T) {
	c := NewGuardedCatalog([]actool.CoreTool{echoTool()}, func(context.Context, string, json.RawMessage) error {
		t.Fatal("guard must not run for an unknown tool")
		return nil
	}, "", 0)
	if _, err := c.Call(context.Background(), "nope", json.RawMessage(`{}`)); err == nil {
		t.Fatal("want error for unknown tool")
	}
}

// List advertises the catalog with each tool's schema.
func TestGuardedCatalogList(t *testing.T) {
	c := NewGuardedCatalog([]actool.CoreTool{echoTool()}, nil, "", 0)
	defs := c.List(context.Background())
	if len(defs) != 1 || defs[0].Name != "echo" || len(defs[0].InputSchema) == 0 {
		t.Fatalf("defs = %+v", defs)
	}
}
