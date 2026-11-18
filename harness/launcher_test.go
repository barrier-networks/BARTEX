package harness

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// noSandbox is an always-unavailable sandbox for the fail-safe test (the real
// unavailable stub only compiles on non-darwin).
type noSandbox struct{}

func (noSandbox) Available() bool { return false }
func (noSandbox) Name() string    { return "none" }
func (noSandbox) Command(context.Context, Confinement, string, []string, []string) (*exec.Cmd, error) {
	return nil, ErrSandboxUnavailable
}

func TestLaunchRefusesWithoutSandbox(t *testing.T) {
	l := Launch{Command: "codex", RunDir: t.TempDir()}
	if _, _, err := l.Build(context.Background(), noSandbox{}); err != ErrSandboxUnavailable {
		t.Fatalf("want ErrSandboxUnavailable, got %v", err)
	}
}

func TestLaunchSubstitutesAndInjects(t *testing.T) {
	dir := t.TempDir()
	l := Launch{
		Kind:        "codex",
		Command:     "codex",
		Args:        []string{"exec", "--sandbox", "danger-full-access", "-c", "mcp_servers=@{{MCP_CONFIG}}", "{{PROMPT}}"},
		Env:         map[string]string{"OPENAI_API_KEY": "sk-x"},
		Prompt:      "enumerate the target",
		RunDir:      dir,
		SelfExe:     "/usr/local/bin/bartex",
		Transport:   "stdio",
		ProxyEnv:    []string{"HTTPS_PROXY=http://127.0.0.1:8788"},
		MCPEndpoint: "http://127.0.0.1:8787/api/mcp",
		MCPToken:    "tok",
	}
	cmd, cleanup, err := l.Build(context.Background(), newSandbox())
	if err != nil {
		// On a host without sandbox-exec this test can't run the real path.
		if err == ErrSandboxUnavailable {
			t.Skip("no OS sandbox on this host")
		}
		t.Fatal(err)
	}
	defer cleanup()

	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "enumerate the target") {
		t.Errorf("prompt not substituted: %v", cmd.Args)
	}
	cfg := filepath.Join(mustEval(t, dir), ".bartex-codex.toml")
	if !strings.Contains(joined, cfg) {
		t.Errorf("MCP config path not substituted: %v", cmd.Args)
	}
	body, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	if !strings.Contains(string(body), `[mcp_servers.bartex]`) || !strings.Contains(string(body), `/usr/local/bin/bartex`) {
		t.Errorf("bad codex config:\n%s", body)
	}
	env := strings.Join(cmd.Env, "\n")
	for _, want := range []string{"BARTEX_MCP_ENDPOINT=http://127.0.0.1:8787/api/mcp", "BARTEX_MCP_TOKEN=tok", "OPENAI_API_KEY=sk-x", "HTTPS_PROXY=http://127.0.0.1:8788"} {
		if !strings.Contains(env, want) {
			t.Errorf("env missing %q\n%s", want, env)
		}
	}
}

func TestLaunchClaudeCodeConfigJSON(t *testing.T) {
	dir := t.TempDir()
	l := Launch{Kind: "claude-code", Command: "claude", RunDir: dir, Transport: "stdio", SelfExe: "bartex",
		Args: []string{"-p", "{{PROMPT}}", "--mcp-config", "{{MCP_CONFIG}}"}, Prompt: "x"}
	_, cleanup, err := l.Build(context.Background(), newSandbox())
	if err == ErrSandboxUnavailable {
		t.Skip("no OS sandbox on this host")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	body, err := os.ReadFile(filepath.Join(mustEval(t, dir), ".bartex-mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"mcpServers"`) || !strings.Contains(string(body), `"mcp-stdio"`) {
		t.Errorf("bad claude-code config:\n%s", body)
	}
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		return p
	}
	return r
}
