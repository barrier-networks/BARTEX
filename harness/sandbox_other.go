//go:build !darwin

package harness

import (
	"context"
	"os/exec"
)

// unsupportedSandbox is the fail-safe stub on platforms without a wired sandbox
// mechanism yet (Linux bubblewrap/firejail is future work). Available() is false,
// so the launcher refuses to spawn rather than run a harness unconfined.
type unsupportedSandbox struct{}

func newSandbox() Sandbox { return unsupportedSandbox{} }

func (unsupportedSandbox) Available() bool { return false }
func (unsupportedSandbox) Name() string    { return "none" }

func (unsupportedSandbox) Command(context.Context, Confinement, string, []string, []string) (*exec.Cmd, error) {
	return nil, ErrSandboxUnavailable
}
