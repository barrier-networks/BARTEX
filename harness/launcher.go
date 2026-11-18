package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Launch fully describes one external-harness run. The caller (the server) opens
// the profile's sealed env, mints a task-scoped MCP token, and supplies the
// recording-proxy env, so this package stays free of db/agent dependencies and
// is unit-testable without a live engagement.
//
// The invocation itself comes from the profile (Command + Args): BARTEX does not
// hard-code the fast-moving per-CLI flags (see docs/harness-cli-profiles.md).
// The launcher only injects the standard BARTEX pieces by substituting
// placeholders in Args and adding environment:
//
//	{{PROMPT}}       → the intent text (the one thing the CLI must act on)
//	{{MCP_CONFIG}}   → path to the generated MCP config file (stdio, claude-code)
//	{{MCP_URL}} / {{MCP_ENDPOINT}} → the governed MCP endpoint URL
//	{{MCP_TOKEN}}    → the task-scoped bearer token (for inline -c overrides)
//	{{SELF_EXE}}     → path to the bartex binary (the stdio bridge)
//	{{RUN_DIR}}      → the confined working directory
//
// and env: the proxy variables (forced egress) plus BARTEX_MCP_ENDPOINT /
// BARTEX_MCP_TOKEN (read by `bartex mcp-stdio` and by http-transport CLIs).
type Launch struct {
	Kind      string            // codex | claude-code | custom (selects MCP config format)
	Command   string            // the CLI binary (name on PATH or absolute path)
	Args      []string          // profile args, with placeholders substituted
	Env       map[string]string // profile env (already decrypted by the caller)
	Prompt    string            // the intent
	RunDir    string            // confined working directory (the intent run dir)
	SelfExe   string            // path to the bartex binary, for the stdio bridge
	Transport string            // stdio | http

	ProxyEnv    []string // recording-proxy env (HTTP(S)_PROXY, CA bundle …)
	MCPEndpoint string   // http://127.0.0.1:8787/api/mcp
	MCPToken    string   // task-scoped bearer token

	// DisableNativeSandbox records that the CLI runs with its own OS sandbox off,
	// so BARTEX's sandbox MUST confine it — the launcher refuses to start without
	// an available outer sandbox (fail-safe).
	DisableNativeSandbox bool
	MaxDuration          time.Duration // 0 = unlimited; enforced by killing at the deadline
}

// Build assembles the sandboxed *exec.Cmd and a cleanup func (removes the temp
// MCP config). It refuses when no outer sandbox is available, because an external
// harness must never run unconfined (its native tools would bypass scope and the
// run dir). The caller sets Stdout/Stderr and runs the command.
func (l Launch) Build(ctx context.Context, sb Sandbox) (*exec.Cmd, func(), error) {
	if sb == nil || !sb.Available() {
		return nil, nil, ErrSandboxUnavailable
	}
	if strings.TrimSpace(l.Command) == "" {
		return nil, nil, fmt.Errorf("harness launch: empty command")
	}
	if strings.TrimSpace(l.RunDir) == "" {
		return nil, nil, fmt.Errorf("harness launch: empty run dir")
	}
	// Canonicalize the run dir so a /var→/private/var symlink matches the sandbox
	// subpath rule (macOS) and the CLI's own path checks.
	runDir := l.RunDir
	if resolved, err := filepath.EvalSymlinks(runDir); err == nil {
		runDir = resolved
	}

	cleanup := func() {}
	subst := map[string]string{
		"{{PROMPT}}":       l.Prompt,
		"{{RUN_DIR}}":      runDir,
		"{{MCP_URL}}":      l.MCPEndpoint,
		"{{MCP_ENDPOINT}}": l.MCPEndpoint,
		"{{MCP_TOKEN}}":    l.MCPToken,
		"{{SELF_EXE}}": func() string {
			if l.SelfExe != "" {
				return l.SelfExe
			}
			return "bartex"
		}(),
	}
	if l.Transport == "stdio" {
		cfgPath, cfg, err := l.writeMCPConfig(runDir)
		if err != nil {
			return nil, nil, err
		}
		cleanup = func() { _ = os.Remove(cfg) }
		subst["{{MCP_CONFIG}}"] = cfgPath
	}

	args := substituteArgs(l.Args, subst)
	env := l.buildEnv(runDir)

	cmd, err := sb.Command(ctx, Confinement{RunDir: runDir}, l.Command, args, env)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return cmd, cleanup, nil
}

// buildEnv composes the subprocess environment: a minimal base (PATH, HOME, the
// run dir as the temp/working anchor), the profile env, the forced-proxy env,
// and the MCP endpoint + token. Later entries win, so the proxy and MCP settings
// are authoritative over anything the profile tried to set.
func (l Launch) buildEnv(runDir string) []string {
	base := map[string]string{
		"PATH":   os.Getenv("PATH"),
		"HOME":   runDir,
		"TMPDIR": runDir,
		// Non-interactive hygiene for child CLIs.
		"NO_COLOR": "1",
		"CI":       "1",
	}
	// Inherit the CLI's auth/home directories so a UI-authenticated login (stored
	// outside the per-run HOME) is found. HOME is the run dir for confinement, so
	// these must be passed explicitly.
	for _, k := range []string{"CODEX_HOME", "CODEX_API_KEY"} {
		if v := os.Getenv(k); v != "" {
			base[k] = v
		}
	}
	for k, v := range l.Env {
		base[k] = v
	}
	// MCP endpoint + token (read by `bartex mcp-stdio` and http-transport CLIs).
	if l.MCPEndpoint != "" {
		base["BARTEX_MCP_ENDPOINT"] = l.MCPEndpoint
	}
	if l.MCPToken != "" {
		base["BARTEX_MCP_TOKEN"] = l.MCPToken
	}
	out := make([]string, 0, len(base)+len(l.ProxyEnv))
	keys := make([]string, 0, len(base))
	for k := range base {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, k+"="+base[k])
	}
	// Proxy env last so it overrides any proxy var the profile set.
	out = append(out, l.ProxyEnv...)
	return out
}

// writeMCPConfig renders the per-CLI stdio MCP config pointing the harness at
// `bartex mcp-stdio`, and returns the path the CLI should load (via a
// {{MCP_CONFIG}} arg) and the file path to clean up. The bridge reads the
// endpoint + token from the environment the launcher injects, so the config
// carries no secret.
func (l Launch) writeMCPConfig(runDir string) (configRef, filePath string, err error) {
	self := l.SelfExe
	if self == "" {
		self = "bartex"
	}
	switch l.Kind {
	case "claude-code":
		// Claude Code --mcp-config JSON.
		cfg := map[string]any{"mcpServers": map[string]any{
			"bartex": map[string]any{"command": self, "args": []string{"mcp-stdio"}},
		}}
		b, _ := json.MarshalIndent(cfg, "", "  ")
		p := filepath.Join(runDir, ".bartex-mcp.json")
		if err := os.WriteFile(p, b, 0o600); err != nil {
			return "", "", err
		}
		return p, p, nil
	default:
		// Codex / custom: config.toml with an [mcp_servers.bartex] stdio server.
		var b strings.Builder
		b.WriteString("[mcp_servers.bartex]\n")
		fmt.Fprintf(&b, "command = %q\n", self)
		b.WriteString("args = [\"mcp-stdio\"]\n")
		p := filepath.Join(runDir, ".bartex-codex.toml")
		if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
			return "", "", err
		}
		return p, p, nil
	}
}

func substituteArgs(args []string, subst map[string]string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		for k, v := range subst {
			a = strings.ReplaceAll(a, k, v)
		}
		out[i] = a
	}
	return out
}

// Run builds and executes the harness, streaming stdout+stderr to out. A
// MaxDuration budget is enforced by cancelling the context (the sandboxed
// subprocess is killed). The returned error is the process's, or nil on a clean
// exit; a budget kill surfaces as ctx.Err() on the parent.
func (l Launch) Run(ctx context.Context, sb Sandbox, out interface{ Write([]byte) (int, error) }) error {
	if l.MaxDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, l.MaxDuration)
		defer cancel()
	}
	cmd, cleanup, err := l.Build(ctx, sb)
	if err != nil {
		return err
	}
	defer cleanup()
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}
