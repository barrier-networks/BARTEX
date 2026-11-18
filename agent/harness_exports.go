package agent

import "github.com/barrier-networks/bartex/db"

// ProxyEnv returns the environment entries that route a child process's HTTP(S)
// through the recording/scope proxy and trust its CA, identical to what the
// built-in worker injects into its Bash subprocesses. The external-harness
// launcher reuses it so a harness CLI's own traffic is recorded and scope-gated.
func ProxyEnv(proxyAddr, caCert string) []string { return proxyEnv(proxyAddr, caCert) }

// IntentPrompt renders the single intent a worker (or an external harness) must
// execute, as the launch prompt. It is the same text the built-in worker places
// in its system prompt.
func IntentPrompt(intent *db.Node) string { return renderIntentTask(intent) }

// EnsureRunDir creates and returns the dedicated run directory for one intent
// (<base>/tasks/<taskID>/i<intentID>), the confined working directory an
// external harness runs in. It mirrors the built-in worker's run-dir layout.
func EnsureRunDir(base string, taskID, intentID int64) string {
	return ensureRunDir(base, taskID, intentID)
}
