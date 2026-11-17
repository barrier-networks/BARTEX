package config

import (
	"os"
	"strings"
)

// Legacy and current environment-variable prefixes. Before the rebrand the
// backend read ARTEX_*; it now reads BARRIER_*. ApplyLegacyEnv bridges the two
// so existing deployments (docker-compose files, .env files, shell exports)
// that still set ARTEX_* keep working unchanged.
const (
	legacyEnvPrefix  = "ARTEX_"
	currentEnvPrefix = "BARRIER_"
)

// ApplyLegacyEnv mirrors every ARTEX_<NAME> variable to BARRIER_<NAME> unless
// the BARRIER_<NAME> form is already set, in which case the new name wins. It
// must be called once at process startup, before any configuration or flag
// default reads the environment. It never clears the legacy variables, so a
// mixed environment is safe and idempotent.
func ApplyLegacyEnv() {
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		key := kv[:eq]
		if !strings.HasPrefix(key, legacyEnvPrefix) {
			continue
		}
		newKey := currentEnvPrefix + strings.TrimPrefix(key, legacyEnvPrefix)
		if _, ok := os.LookupEnv(newKey); ok {
			continue
		}
		_ = os.Setenv(newKey, kv[eq+1:])
	}
}
