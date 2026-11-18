//go:build !darwin

package harness

import (
	"context"
	"os"
	"os/exec"
)

// On platforms without a wired per-process sandbox (Linux lacks a Seatbelt
// equivalent here), BARTEX relies on the deployment boundary instead: when it
// runs inside its own container, the container IS the isolation (the design
// doc's "run the CLI with its own sandbox off inside a container/VM" path), and
// egress is still forced through the recording proxy by the launch environment.
//
// That mode is opt-in through BARTEX_HARNESS_ALLOW_UNSANDBOXED (the Docker image
// sets it). Without it, no sandbox is available and the launcher fails safe:
// it will not run an external harness unconfined on a bare host.
type containerSandbox struct{ allowed bool }

func newSandbox() Sandbox {
	return containerSandbox{allowed: os.Getenv("BARTEX_HARNESS_ALLOW_UNSANDBOXED") == "1"}
}

func (c containerSandbox) Available() bool { return c.allowed }
func (containerSandbox) Name() string      { return "container" }

func (c containerSandbox) Command(ctx context.Context, cf Confinement, bin string, args, env []string) (*exec.Cmd, error) {
	if !c.allowed {
		return nil, ErrSandboxUnavailable
	}
	// No per-process OS sandbox: the container is the boundary. The process still
	// runs in the confined working directory, and env carries the forced-proxy
	// settings so its egress is recorded and scope-gated.
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = cf.RunDir
	cmd.Env = env
	return cmd, nil
}
