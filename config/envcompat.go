package config

import (
	"os"
	"strings"
)

// Current and legacy environment-variable prefixes. The backend reads
// BARTEX_*. It was briefly published reading BARRIER_* and, upstream, ARTEX_*.
// ApplyLegacyEnv bridges both so existing deployments (docker-compose files,
// .env files, shell exports) that still set an older prefix keep working
// unchanged.
const currentEnvPrefix = "BARTEX_"

// legacyEnvPrefixes is ordered by precedence: when the same name is set under
// more than one legacy prefix, the more recent one (BARRIER_) wins.
var legacyEnvPrefixes = []string{"BARRIER_", "ARTEX_"}

// ApplyLegacyEnv mirrors every BARRIER_<NAME> and ARTEX_<NAME> variable to
// BARTEX_<NAME> unless the BARTEX_<NAME> form is already set, in which case
// the new name wins. It must be called once at process startup, before any
// configuration or flag default reads the environment. It never clears the
// legacy variables, so a mixed environment is safe and idempotent.
func ApplyLegacyEnv() {
	env := os.Environ()
	for _, prefix := range legacyEnvPrefixes {
		for _, kv := range env {
			eq := strings.IndexByte(kv, '=')
			if eq < 0 {
				continue
			}
			key := kv[:eq]
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			newKey := currentEnvPrefix + strings.TrimPrefix(key, prefix)
			if _, ok := os.LookupEnv(newKey); ok {
				continue
			}
			_ = os.Setenv(newKey, kv[eq+1:])
		}
	}
}
