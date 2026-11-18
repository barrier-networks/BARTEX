-- External-harness execution backend (docs/harness-execution-backend.md).
-- Additive: a task with a NULL harness_profile_id runs on the built-in
-- norma/agentcore loop exactly as before. A profile describes how to launch an
-- external agentic CLI (Codex/Daybreak, Claude Code, …) as the worker backend,
-- pointed at the governed BARTEX Tools MCP server and confined by the sandbox.

CREATE TABLE IF NOT EXISTS harness_profiles (
    id                     BIGSERIAL PRIMARY KEY,
    name                   TEXT NOT NULL,
    kind                   TEXT NOT NULL DEFAULT 'custom'
                             CHECK (kind IN ('codex','claude-code','custom')),
    command                TEXT NOT NULL,
    args                   JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- env is sealed at rest (see db/secrets.go colHarnessEnv): the plaintext map
    -- lives under the "__artex_enc" key as ciphertext, like mcp_servers.env.
    env                    JSONB NOT NULL DEFAULT '{}'::jsonb,
    mcp_transport          TEXT NOT NULL DEFAULT 'stdio'
                             CHECK (mcp_transport IN ('stdio','http')),
    -- Disable the CLI's own OS sandbox so it does not nest inside ours (BARTEX
    -- owns the outer Seatbelt box). Default true: the launcher refuses to run a
    -- CLI with its own sandbox off and no outer sandbox available.
    disable_native_sandbox BOOLEAN NOT NULL DEFAULT true,
    -- Enable the Codex Daybreak mode for this profile (ignored by other kinds).
    daybreak               BOOLEAN NOT NULL DEFAULT false,
    -- Coarse budgets enforced by killing the subprocess (external token
    -- accounting is not visible to us). 0 = unlimited.
    max_turns              INTEGER NOT NULL DEFAULT 0,
    max_duration_seconds   INTEGER NOT NULL DEFAULT 0,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

DROP TRIGGER IF EXISTS trg_harness_profiles_upd ON harness_profiles;
CREATE TRIGGER trg_harness_profiles_upd BEFORE UPDATE ON harness_profiles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Per-task backend selection. NULL = built-in loop (unchanged legacy behaviour).
ALTER TABLE tasks
    ADD COLUMN IF NOT EXISTS harness_profile_id BIGINT
        REFERENCES harness_profiles(id) ON DELETE SET NULL;
