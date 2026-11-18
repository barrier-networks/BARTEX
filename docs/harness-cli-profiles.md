# External-harness CLI profiles (research)

Grounding for the Phase 1 launcher (`docs/harness-execution-backend.md`). It
records how the candidate local agentic CLIs are launched non-interactively, how
they connect to the governed BARTEX Tools MCP server (Phase 0, `/api/harness-mcp`), and
how their own sandboxes compose with ours. Version-sensitive flags are flagged
**verify** — confirm against the installed CLI's `--help` / `config.md`, since
these tools move fast.

## 0. "OpenAI Daybreak" is a Codex CLI mode, not a separate tool

Daybreak is OpenAI's cybersecurity initiative; in the terminal it is a **toggle
inside the Codex CLI**, not its own agent. So "use OpenAI Daybreak" means **run
Codex CLI with Daybreak on**.

- Turn on: `/daybreak` interactively, or `-c daybreak=true` for a single run.
- Requires **Codex CLI ≥ 0.163.0**, the OpenAI provider, a signed-in ChatGPT
  account, and a compatible model (**verify** the version/model).
- Tiers: **Blue** (default security work) and **Red** (authorised
  proof-of-concept exploit dev, exploit-chain validation, pen-testing, red
  teaming). Red is the tier that matches a BARTEX engagement; access is granted
  per user/project by OpenAI.

⇒ The **primary harness profile is Codex CLI**; Claude Code is the secondary.

## 1. Codex CLI (primary — carries Daybreak)

- **Non-interactive:** `codex exec "<prompt>"`. Flags beat config; the two axes
  are independent — `--ask-for-approval never` (no pauses) and `--sandbox <mode>`
  (what commands may touch). Unattended pen-test shape:
  `codex exec --ask-for-approval never --sandbox danger-full-access -c daybreak=true "<intent>"`.
- **MCP:** `[mcp_servers.<name>]` in `~/.codex/config.toml` (or project
  `.codex/config.toml` in a trusted dir), or `codex mcp add`.
  - **stdio (stable, our default):** `command` + `args` (+ `env`, `cwd`). Point
    it at our bridge:
    ```toml
    [mcp_servers.bartex]
    command = "bartex"
    args    = ["mcp-stdio"]
    env     = { BARTEX_MCP_ENDPOINT = "http://127.0.0.1:8787/api/harness-mcp", BARTEX_MCP_TOKEN = "<minted>" }
    ```
  - **HTTP (works, rougher):** `url` + `bearer_token_env_var = "BARTEX_MCP_TOKEN"`.
    Streamable HTTP rides the **experimental RMCP client** — enable
    `experimental_use_rmcp_client = true` (older name) / `rmcp_client = true`
    (renamed, #7665) (**verify** which your build wants). Because of this churn,
    the launcher **defaults Codex to the stdio bridge**; HTTP is opt-in.
  - `codex exec` reading `mcp_servers` the same way as interactive mode is
    **unverified in docs** — smoke-test with the installed version before relying
    on it.
- **Native tools:** Codex's hands are its sandboxed shell + `apply_patch`; there
  is no "disable all tools" switch. Under our model we do **not** try to strip
  them — we **confine** them (see §3) and add the governed BARTEX MCP for
  write-back. That is the Option-B posture the design doc mandates.

## 2. Claude Code (secondary)

- **Non-interactive:** `claude -p "<prompt>"` (a.k.a. headless).
- **Disable built-in tools:** `--tools ""` (empty = none; or name an explicit
  set) (**verify**). Also `--disallowedTools` removes tools from context;
  `--allowedTools "mcp__bartex__*"` lets the governed MCP tools run without
  prompting.
- **MCP:** `--mcp-config <file|json>` (stdio or http) plus **`--strict-mcp-config`**
  so only our server is used and no ambient MCP config leaks in.
- **Permissions:** no "disable" value for `--permission-mode`
  (default/plan/acceptEdits/bypassPermissions — set varies by version, **verify**).
  For unattended runs combine `--allowedTools` with the MCP-only tool set; avoid
  `bypassPermissions` outside the sandbox.
- Shape (inference, untested):
  `claude -p "<intent>" --tools "" --mcp-config ./bartex-mcp.json --strict-mcp-config --allowedTools "mcp__bartex__*"`.

## 3. Sandbox composition — the one load-bearing decision

Both Codex and Claude Code ship their own OS sandbox (Codex: Seatbelt / Bubblewrap
/ Landlock / Windows DACL). **Nesting a CLI's Seatbelt inside our Seatbelt stacks
two profiles and is unreliable** (Codex's own profile also blocks network
regardless of its config, per a user report).

**Decision:** BARTEX owns the outer sandbox. Launch the CLI with its **own
sandbox disabled**, inside our Seatbelt confinement:

- Codex: `--sandbox danger-full-access` (or `-c sandbox_mode=danger-full-access`
  — **verify** the key) together with `--ask-for-approval never`. This is exactly
  OpenAI's documented "run with the flag inside a container/VM" pattern — our
  Seatbelt *is* that outer isolation.
- Claude Code: run with permissions open (`--allowedTools`/bypass) **only**
  because it sits inside our Seatbelt.

Our Seatbelt (`harness/sandbox_darwin.go`) then provides the real boundary:
write confined to the run dir, **egress allowed only to loopback**, so the CLI's
shell reaches targets solely through the recording/scope proxy (forced via
`proxyEnv`) and the governed MCP on loopback. If no OS sandbox is available the
launcher refuses to start (fail-safe) — it must never run a CLI with its own
sandbox off and no outer box.

## 4. What the launcher injects per run

1. A freshly minted **task-scoped MCP token** (`POST /api/tasks/{id}/mcp-token`).
2. **MCP wiring** for the chosen transport: stdio → write a temp
   `config.toml`/`--mcp-config` pointing at `bartex mcp-stdio` with
   `BARTEX_MCP_ENDPOINT`+`BARTEX_MCP_TOKEN`; http → the `/api/harness-mcp` URL +
   `bearer_token_env_var`.
3. **Proxy egress env** (`proxyEnv`: `HTTP(S)_PROXY`, CA bundle vars) so the
   CLI's own HTTP is recorded and scope-gated.
4. **Working dir / confinement** = the intent run dir (`EvalSymlinks` it first so
   a `/var`→`/private/var` symlink matches the Seatbelt subpath).
5. The **intent** as the prompt; **turn/time caps** enforced by killing the
   subprocess at budget (coarser than native budgets, per the design doc).
6. The CLI's **own sandbox disabled** (§3); stdout/stderr streamed as the
   session log.

## 5. harness_profiles (Phase 1 schema sketch)

Additive table + nullable `tasks.harness_profile_id` FK:
`name`, `kind` (`codex`|`claude-code`|`custom`), `command`, `args` (jsonb),
`env` (jsonb, **sealed** like `mcp_servers.env` / `colMCPEnv`), `mcp_transport`
(`stdio`|`http`), `disable_native_sandbox` (bool, default true), `daybreak`
(bool), plus turn/time caps. The launcher renders the invocation from the
profile + the per-run injections in §4.
