// Package harness runs an external agentic CLI as a BARTEX worker's execution
// backend, confined so the governance and evidence invariants still hold when
// BARTEX does not own the loop (see docs/harness-execution-backend.md).
//
// The security model is defence in depth:
//   - the CLI is pointed at the governed BARTEX Tools MCP server (Phase 0), so
//     every BARTEX tool call is still gated and still writes evidence back; and
//   - the subprocess is sandboxed (this file) so its OWN native tools cannot
//     write outside the task run directory or reach the network except through
//     the recording/scope proxy.
//
// The second layer is what makes the first safe against "the CLI had a native
// tool we forgot to disable": even an ungoverned native action is confined and
// its egress is forced through the proxy that enforces scope and captures
// evidence. The design doc's hard rule follows: a CLI that cannot be locked to
// MCP-only tools MUST NOT run without this sandbox.
package harness

import (
	"context"
	"errors"
	"os/exec"
)

// ErrSandboxUnavailable is returned when no sandbox mechanism exists on this
// host. Per the fail-safe rule the launcher refuses to spawn a harness in that
// case rather than run it unconfined.
var ErrSandboxUnavailable = errors.New("no OS sandbox available on this host; refusing to launch an external harness unconfined")

// Confinement describes the box a harness subprocess runs in. RunDir is the only
// directory it may write to (its working directory); every outbound network
// connection is forced to the loopback proxy, so the CLI cannot reach a target
// except through the recording/scope proxy and cannot resolve or dial the
// internet directly.
type Confinement struct {
	// RunDir is the task/intent run directory: the process's working directory
	// and the one writable subtree. Required.
	RunDir string
	// ExtraWritable are additional writable subtrees (e.g. a profile-specified
	// cache). Kept minimal; each one widens the box.
	ExtraWritable []string
	// ExtraReadable are additional readable subtrees beyond the system defaults
	// (the CLI binary, config and the proxy CA are typically read via the broad
	// system read allowance).
	ExtraReadable []string
}

// Sandbox builds a confined command for the host it is compiled for.
type Sandbox interface {
	// Available reports whether this host can actually confine a process. When it
	// is false the launcher must not spawn (ErrSandboxUnavailable).
	Available() bool
	// Name identifies the mechanism (e.g. "seatbelt") for logs and the UI.
	Name() string
	// Command returns an *exec.Cmd that runs bin+args confined per c, with env as
	// the subprocess environment (already including the proxy + MCP settings). It
	// returns ErrSandboxUnavailable when Available() is false. The caller sets
	// Stdin/Stdout/Stderr and starts the command.
	Command(ctx context.Context, c Confinement, bin string, args, env []string) (*exec.Cmd, error)
}

// NewSandbox returns the sandbox for the current OS. On macOS this is Seatbelt
// (sandbox-exec); on other platforms it is an unavailable stub until a mechanism
// (bubblewrap/firejail on Linux) is added, so the launcher fails safe.
func NewSandbox() Sandbox { return newSandbox() }
