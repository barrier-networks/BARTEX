//go:build darwin

package harness

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// seatbelt confines a subprocess with macOS sandbox-exec (Seatbelt / SBPL).
type seatbelt struct{ bin string }

func newSandbox() Sandbox {
	// sandbox-exec is a system binary; LookPath also lets a host without it fall
	// through to the fail-safe refusal.
	p, err := exec.LookPath("sandbox-exec")
	if err != nil {
		return seatbelt{}
	}
	return seatbelt{bin: p}
}

func (s seatbelt) Available() bool { return s.bin != "" }
func (seatbelt) Name() string      { return "seatbelt" }

func (s seatbelt) Command(ctx context.Context, c Confinement, bin string, args, env []string) (*exec.Cmd, error) {
	if !s.Available() {
		return nil, ErrSandboxUnavailable
	}
	if strings.TrimSpace(c.RunDir) == "" {
		return nil, fmt.Errorf("seatbelt: RunDir is required")
	}
	profile := buildSeatbeltProfile(c)
	// -p takes the profile inline, avoiding a temp .sb file the sandboxed process
	// could otherwise see or race. The profile is passed before the program.
	full := append([]string{"-p", profile, bin}, args...)
	cmd := exec.CommandContext(ctx, s.bin, full...)
	cmd.Dir = c.RunDir
	cmd.Env = env
	return cmd, nil
}

// buildSeatbeltProfile renders the SBPL policy. The policy is deny-by-default
// and grants the minimum a CLI needs to run while enforcing the two invariants
// the harness depends on:
//
//   - Filesystem: broad READ (dyld, shared libraries, the CLI binary, its config
//     and the proxy CA all load via the system read allowance), but WRITE only
//     inside the run directory (plus any explicit extra subtrees) and the
//     per-process temp dirs. The CLI cannot modify the rest of the host.
//
//   - Network: outbound is allowed ONLY to loopback. The recording/scope proxy
//     listens on loopback and the governed MCP endpoint is on loopback, so the
//     CLI can reach both — but it cannot open a direct connection to a target or
//     resolver. Every target request is therefore forced through the proxy,
//     which enforces scope and captures evidence. Non-loopback egress is denied.
//
// writeSubpath escapes each path for SBPL; a path with an embedded quote or
// newline cannot smuggle policy because the value is a quoted string literal and
// control characters are rejected by the caller (validatePath).
func buildSeatbeltProfile(c Confinement) string {
	var b strings.Builder
	b.WriteString("(version 1)\n")
	b.WriteString("(deny default)\n")
	// Diagnostics: let denials surface in the system log without allowing them.
	b.WriteString("(allow process-info* (target self))\n")
	// Run the program and its children.
	b.WriteString("(allow process-fork)\n")
	b.WriteString("(allow process-exec)\n")
	b.WriteString("(allow signal (target children))\n")
	// Read broadly (libraries, binaries, config, CA bundle). Write narrowly.
	b.WriteString("(allow file-read*)\n")
	b.WriteString("(allow sysctl-read)\n")
	b.WriteString("(allow mach-lookup)\n")
	b.WriteString("(allow ipc-posix-shm)\n")
	for _, p := range append([]string{c.RunDir}, c.ExtraWritable...) {
		fmt.Fprintf(&b, "(allow file-write* (subpath %s))\n", sbplString(p))
	}
	// Per-process temp + device scratch the toolchain expects.
	b.WriteString("(allow file-write* (subpath \"/private/tmp\"))\n")
	b.WriteString("(allow file-write* (subpath \"/private/var/tmp\"))\n")
	b.WriteString("(allow file-write* (subpath \"/private/var/folders\"))\n")
	b.WriteString("(allow file-write-data (literal \"/dev/null\") (literal \"/dev/dtracehelper\") (literal \"/dev/tty\"))\n")
	// Network: loopback only, so all real egress must traverse the proxy.
	b.WriteString("(allow network-outbound (remote ip \"localhost:*\"))\n")
	b.WriteString("(allow network-outbound (remote unix-socket))\n")
	b.WriteString("(allow network-bind (local ip \"localhost:*\"))\n")
	return b.String()
}

// sbplString renders a Go string as an SBPL double-quoted string literal,
// backslash-escaping the two characters that are special inside one.
func sbplString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}
