//go:build darwin

package harness

import (
	"context"
	"strings"
	"testing"
)

func TestSeatbeltProfileInvariants(t *testing.T) {
	p := buildSeatbeltProfile(Confinement{RunDir: "/data/tasks/1/i2"})

	must := []string{
		"(deny default)",
		"(allow file-read*)",
		`(allow file-write* (subpath "/data/tasks/1/i2"))`,
		`(allow network-outbound (remote ip "localhost:*"))`,
	}
	for _, m := range must {
		if !strings.Contains(p, m) {
			t.Errorf("profile missing %q\n%s", m, p)
		}
	}
	// The run dir must NOT be granted broad write beyond a subpath, and there must
	// be no allow-all network rule.
	if strings.Contains(p, `(allow network*)`) || strings.Contains(p, `(allow default)`) {
		t.Errorf("profile grants too much:\n%s", p)
	}
	// No non-loopback remote allowance.
	if strings.Contains(p, `(allow network-outbound (remote ip "*"))`) {
		t.Errorf("profile allows arbitrary egress:\n%s", p)
	}
}

func TestSeatbeltProfileEscapesPaths(t *testing.T) {
	// A path containing a quote must stay inside its string literal.
	p := buildSeatbeltProfile(Confinement{RunDir: `/data/ev"il`})
	if !strings.Contains(p, `(subpath "/data/ev\"il")`) {
		t.Errorf("quote not escaped:\n%s", p)
	}
}

func TestSeatbeltExtraWritable(t *testing.T) {
	p := buildSeatbeltProfile(Confinement{RunDir: "/run", ExtraWritable: []string{"/cache/x"}})
	if !strings.Contains(p, `(allow file-write* (subpath "/run"))`) ||
		!strings.Contains(p, `(allow file-write* (subpath "/cache/x"))`) {
		t.Errorf("extra writable subtree missing:\n%s", p)
	}
}

func TestSeatbeltCommandRequiresRunDir(t *testing.T) {
	s := seatbelt{bin: "/usr/bin/sandbox-exec"}
	if _, err := s.Command(context.Background(), Confinement{}, "/bin/echo", nil, nil); err == nil {
		t.Fatal("want error when RunDir is empty")
	}
	cmd, err := s.Command(context.Background(), Confinement{RunDir: "/run"}, "/bin/echo", []string{"hi"}, []string{"A=1"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Dir != "/run" || len(cmd.Env) != 1 {
		t.Errorf("cmd not configured: dir=%q env=%v", cmd.Dir, cmd.Env)
	}
	// The real program and its args must appear after the inline profile.
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "/bin/echo hi") || !strings.Contains(joined, "-p") {
		t.Errorf("args not assembled: %v", cmd.Args)
	}
}

func TestSeatbeltUnavailableRefuses(t *testing.T) {
	var s seatbelt // bin == "" => unavailable
	if s.Available() {
		t.Fatal("empty seatbelt should be unavailable")
	}
	if _, err := s.Command(context.Background(), Confinement{RunDir: "/run"}, "/bin/echo", nil, nil); err != ErrSandboxUnavailable {
		t.Fatalf("want ErrSandboxUnavailable, got %v", err)
	}
}
