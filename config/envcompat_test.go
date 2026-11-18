package config

import (
	"os"
	"testing"
)

// A legacy ARTEX_* variable is mirrored to its BARRIER_* equivalent when the
// new name is unset.
func TestApplyLegacyEnvMirrorsLegacy(t *testing.T) {
	t.Setenv("ARTEX_PG_DSN", "postgres://legacy")
	os.Unsetenv("BARRIER_PG_DSN")

	ApplyLegacyEnv()

	if got := os.Getenv("BARRIER_PG_DSN"); got != "postgres://legacy" {
		t.Fatalf("BARRIER_PG_DSN = %q, want mirrored legacy value", got)
	}
}

// The new BARRIER_* name always wins: a legacy value never overwrites it.
func TestApplyLegacyEnvNewNameWins(t *testing.T) {
	t.Setenv("ARTEX_SECRET_KEY", "legacy-key")
	t.Setenv("BARRIER_SECRET_KEY", "new-key")

	ApplyLegacyEnv()

	if got := os.Getenv("BARRIER_SECRET_KEY"); got != "new-key" {
		t.Fatalf("BARRIER_SECRET_KEY = %q, want the new name to win", got)
	}
}

// Non-ARTEX variables are left untouched, and an empty legacy value still maps.
func TestApplyLegacyEnvIgnoresOthersAndKeepsEmpty(t *testing.T) {
	t.Setenv("PATH_LIKE_UNRELATED", "x")
	t.Setenv("ARTEX_EMPTY", "")
	os.Unsetenv("BARRIER_EMPTY")

	ApplyLegacyEnv()

	if _, ok := os.LookupEnv("BARRIER_EMPTY"); !ok {
		t.Fatalf("empty legacy ARTEX_EMPTY should still create BARRIER_EMPTY")
	}
}
