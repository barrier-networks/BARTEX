package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// HarnessProfile describes how to launch an external agentic CLI (Codex/Daybreak,
// Claude Code, …) as a worker's execution backend. See
// docs/harness-execution-backend.md and docs/harness-cli-profiles.md.
type HarnessProfile struct {
	ID      int64    `json:"id"`
	Name    string   `json:"name"`
	Kind    string   `json:"kind"` // codex | claude-code | custom
	Command string   `json:"command"`
	Args    []string `json:"args"`
	// Env is the launch environment (KEY=VALUE map). Sealed at rest. Never
	// serialized to the UI in clear — EnvKeys lists the key names for display.
	Env     map[string]string `json:"-"`
	EnvKeys []string          `json:"env_keys,omitempty"`

	MCPTransport         string `json:"mcp_transport"`          // stdio | http
	DisableNativeSandbox bool   `json:"disable_native_sandbox"` // run the CLI with its own sandbox off, inside ours
	Daybreak             bool   `json:"daybreak"`               // Codex Daybreak mode
	MaxTurns             int    `json:"max_turns"`              // 0 = unlimited
	MaxDurationSeconds   int    `json:"max_duration_seconds"`   // 0 = unlimited
}

const harnessCols = `id, name, kind, command, args, mcp_transport, disable_native_sandbox, daybreak, max_turns, max_duration_seconds`

// ListHarnessProfiles returns every profile without decrypting env (only key
// names, for the UI).
func (d *DB) ListHarnessProfiles() ([]*HarnessProfile, error) {
	rows, err := d.Query(`SELECT ` + harnessCols + `, env FROM harness_profiles ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*HarnessProfile
	for rows.Next() {
		var p HarnessProfile
		if err := scanHarnessWithEnv(rows, &p, false); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

// HarnessProfileByID returns one profile with its env decrypted (the launcher
// needs the real values), or nil when not found.
func (d *DB) HarnessProfileByID(id int64) (*HarnessProfile, error) {
	var p HarnessProfile
	err := scanHarnessWithEnv(d.QueryRow(`SELECT `+harnessCols+`, env FROM harness_profiles WHERE id=$1`, id), &p, true)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &p, err
}

// scanHarnessWithEnv scans a row including the env column and, when decrypt is
// true, opens the sealed env into p.Env; it always fills p.EnvKeys for display.
func scanHarnessWithEnv(row interface{ Scan(...any) error }, p *HarnessProfile, decrypt bool) error {
	var args, env []byte
	if err := row.Scan(&p.ID, &p.Name, &p.Kind, &p.Command, &args,
		&p.MCPTransport, &p.DisableNativeSandbox, &p.Daybreak, &p.MaxTurns, &p.MaxDurationSeconds, &env); err != nil {
		return err
	}
	if len(args) > 0 {
		_ = json.Unmarshal(args, &p.Args)
	}
	m, err := openEnvMap(colHarnessEnv, string(env))
	if err != nil {
		return err
	}
	p.EnvKeys = sortedKeys(m)
	if decrypt {
		p.Env = m
	}
	return nil
}

// SaveHarnessProfile inserts (id==0) or updates a profile. On update, a nil Env
// keeps the stored env; a non-nil Env (even empty) replaces it. The env is
// sealed before it is written.
func (d *DB) SaveHarnessProfile(p *HarnessProfile) (int64, error) {
	p.Name = strings.TrimSpace(p.Name)
	p.Command = strings.TrimSpace(p.Command)
	if p.Name == "" || p.Command == "" {
		return 0, fmt.Errorf("harness profile needs a name and a command")
	}
	if p.Kind == "" {
		p.Kind = "custom"
	}
	if p.MCPTransport == "" {
		p.MCPTransport = "stdio"
	}
	args, err := json.Marshal(p.Args)
	if err != nil {
		return 0, err
	}

	if p.ID == 0 {
		sealed, err := sealEnvMap(colHarnessEnv, p.Env)
		if err != nil {
			return 0, err
		}
		var id int64
		err = d.QueryRow(`INSERT INTO harness_profiles
			(name, kind, command, args, env, mcp_transport, disable_native_sandbox, daybreak, max_turns, max_duration_seconds)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
			p.Name, p.Kind, p.Command, args, sealed, p.MCPTransport, p.DisableNativeSandbox, p.Daybreak, p.MaxTurns, p.MaxDurationSeconds).Scan(&id)
		return id, err
	}

	// Update. Only touch env when the caller supplied one.
	if p.Env != nil {
		sealed, err := sealEnvMap(colHarnessEnv, p.Env)
		if err != nil {
			return 0, err
		}
		_, err = d.Exec(`UPDATE harness_profiles SET
			name=$1, kind=$2, command=$3, args=$4, env=$5, mcp_transport=$6,
			disable_native_sandbox=$7, daybreak=$8, max_turns=$9, max_duration_seconds=$10
			WHERE id=$11`,
			p.Name, p.Kind, p.Command, args, sealed, p.MCPTransport, p.DisableNativeSandbox, p.Daybreak, p.MaxTurns, p.MaxDurationSeconds, p.ID)
		return p.ID, err
	}
	_, err = d.Exec(`UPDATE harness_profiles SET
		name=$1, kind=$2, command=$3, args=$4, mcp_transport=$5,
		disable_native_sandbox=$6, daybreak=$7, max_turns=$8, max_duration_seconds=$9
		WHERE id=$10`,
		p.Name, p.Kind, p.Command, args, p.MCPTransport, p.DisableNativeSandbox, p.Daybreak, p.MaxTurns, p.MaxDurationSeconds, p.ID)
	return p.ID, err
}

// DeleteHarnessProfile removes a profile. tasks.harness_profile_id is ON DELETE
// SET NULL, so any task using it reverts to the built-in backend.
func (d *DB) DeleteHarnessProfile(id int64) error {
	_, err := d.Exec(`DELETE FROM harness_profiles WHERE id=$1`, id)
	return err
}

// defaultHarnessSettingKey holds the global default execution-backend profile id
// ("" / absent = built-in loop). Read on task dispatch to resolve the effective
// backend when a task has no explicit harness_profile_id.
const defaultHarnessSettingKey = "default_harness_profile_id"

// DefaultHarnessProfileID returns the global default execution-backend profile
// id, or nil when none is set (built-in loop is the default).
func (d *DB) DefaultHarnessProfileID() (*int64, error) {
	v, ok, err := d.GetSetting(defaultHarnessSettingKey)
	if err != nil || !ok || strings.TrimSpace(v) == "" {
		return nil, err
	}
	id, perr := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if perr != nil {
		return nil, nil // malformed → treat as unset rather than fail dispatch
	}
	return &id, nil
}

// SetDefaultHarnessProfileID sets (or, with nil, clears) the global default
// execution backend. A non-nil id must reference an existing profile.
func (d *DB) SetDefaultHarnessProfileID(id *int64) error {
	if id == nil {
		return d.SetSetting(defaultHarnessSettingKey, "")
	}
	p, err := d.HarnessProfileByID(*id)
	if err != nil {
		return err
	}
	if p == nil {
		return fmt.Errorf("harness profile %d not found", *id)
	}
	return d.SetSetting(defaultHarnessSettingKey, fmt.Sprintf("%d", *id))
}

// sealEnvMap marshals an env map to canonical JSON and seals it for column c. A
// nil/empty map stores an empty object (nothing to hide).
func sealEnvMap(c secretColumn, env map[string]string) (string, error) {
	if len(env) == 0 {
		return "{}", nil
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return "", err
	}
	return sealSecret(c, string(raw))
}

// openEnvMap decrypts a sealed env column into a map. An empty/`{}`/null value
// yields an empty map.
func openEnvMap(c secretColumn, stored string) (map[string]string, error) {
	stored = strings.TrimSpace(stored)
	if stored == "" || stored == "{}" || stored == "null" {
		return map[string]string{}, nil
	}
	plain, err := openSecret(c, stored)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	if strings.TrimSpace(plain) == "" {
		return m, nil
	}
	if err := json.Unmarshal([]byte(plain), &m); err != nil {
		return nil, err
	}
	return m, nil
}

func sortedKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// stable, cheap: insertion-ordered sort
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
