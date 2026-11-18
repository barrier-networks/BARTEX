package config

import (
	"os"
	"testing"
)

// A legacy ARTEX_* variable is mirrored to its BARTEX_* equivalent when the
// new name is unset.
func TestApplyLegacyEnvMirrorsLegacy(t *testing.T) {
	t.Setenv("ARTEX_PG_DSN", "postgres://legacy")
	os.Unsetenv("BARTEX_PG_DSN")

	ApplyLegacyEnv()

	if got := os.Getenv("BARTEX_PG_DSN"); got != "postgres://legacy" {
		t.Fatalf("BARTEX_PG_DSN = %q, want mirrored legacy value", got)
	}
}

// The new BARTEX_* name always wins: a legacy value never overwrites it.
func TestApplyLegacyEnvNewNameWins(t *testing.T) {
	t.Setenv("ARTEX_SECRET_KEY", "legacy-key")
	t.Setenv("BARTEX_SECRET_KEY", "new-key")

	ApplyLegacyEnv()

	if got := os.Getenv("BARTEX_SECRET_KEY"); got != "new-key" {
		t.Fatalf("BARTEX_SECRET_KEY = %q, want the new name to win", got)
	}
}

// Non-ARTEX variables are left untouched, and an empty legacy value still maps.
func TestApplyLegacyEnvIgnoresOthersAndKeepsEmpty(t *testing.T) {
	t.Setenv("PATH_LIKE_UNRELATED", "x")
	t.Setenv("ARTEX_EMPTY", "")
	os.Unsetenv("BARTEX_EMPTY")

	ApplyLegacyEnv()

	if _, ok := os.LookupEnv("BARTEX_EMPTY"); !ok {
		t.Fatalf("empty legacy ARTEX_EMPTY should still create BARTEX_EMPTY")
	}
}

// The interim BARRIER_* prefix is mirrored too, and beats ARTEX_* when both
// are set for the same name.
func TestApplyLegacyEnvMirrorsBarrierPrefix(t *testing.T) {
	t.Setenv("BARRIER_LLM_MODEL", "from-barrier")
	t.Setenv("ARTEX_LLM_MODEL", "from-artex")
	os.Unsetenv("BARTEX_LLM_MODEL")
	t.Cleanup(func() { os.Unsetenv("BARTEX_LLM_MODEL") })

	ApplyLegacyEnv()

	if got := os.Getenv("BARTEX_LLM_MODEL"); got != "from-barrier" {
		t.Fatalf("BARTEX_LLM_MODEL = %q, want the BARRIER_ value", got)
	}
}
