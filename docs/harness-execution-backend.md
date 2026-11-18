# Design: external-harness execution backend

Status: **design + Phase 0 (substrate) started.** This document scopes letting a
BARTEX worker intent be executed by an **external agentic harness** (Claude
Code, Codex, Aider, …) instead of the built-in `norma/agentcore` loop, without
losing the governance and evidence guarantees that are the point of BARTEX.

---

## 1. Motivation & non-goal

- **Motivation:** teams already run mature agentic CLIs locally and want to point
  BARTEX at them — to reuse their tooling and model access, and to pick the
  loop that suits a job. Combined with the local-model work (Ollama /
  OpenAI-compatible endpoints), this lets BARTEX run against "the same backend
  my Claude Code / Codex setup uses."
- **Non-goal:** replacing `norma/agentcore` as the default. The built-in loop
  stays; the external harness is an **opt-in alternative backend** for a worker.

## 2. The one constraint that decides the design

BARTEX's value is **not the loop** — it is the governance and evidence *around*
it. Today every worker runs through `agentcore.NewSession(opts)` with two
enforcement layers:

1. **Tool-call layer** — `guard.applyIntercept` (the agent-core `PreToolUse`
   hook) fires on *every* tool call: built-in deny → user rules / judge →
   **RoE engagement gate** (migration `0007`) → target gate (scope / pins /
   approval) → budgets. Tools are a **curated catalog**
   (`agent.builtinToolsByAgent()` deliberately excludes the SDK's raw
   Read/Write/Bash).
2. **Network layer** — the recording MITM proxy + egress gate captures
   request/response, binds it to findings (the "receipts"), and enforces scope
   and the noise governor at the wire.

An external harness **replaces layer 1 with its own ungoverned loop and its own
native tools** (its own Bash, file edits, web). Dropped in naively that is not a
feature — it is an **ungoverned offensive agent** acting outside scope, approval,
RoE and evidence capture. So the engineering problem is **"keep every governance
and evidence invariant when we do not own the loop,"** not "launch Claude Code."

## 3. Current architecture (grounded)

- Worker: `agent/worker.go` → `agentcore.Options{…}` → `agentcore.NewSession`.
  Hooks (`harness.HookRunner`) carry `guard.applyIntercept`; tools come from
  `agent.ToolSet`; the model from the resolved LLM profile.
- Tools: `tool.CoreTool` (name, description, `InputSchema()`, execute). Curated
  per agent; plus custom tools and **consumed** MCP tools (`mcphttp`, norma's
  `mcp` **client**). BARTEX does **not** currently expose an MCP server.
- Governance choke point: `guard/guard.go:applyIntercept`, threaded via
  `intercept.TargetScope` (incl. the new `Engagement` policy).
- Evidence: the `intercept` recording proxy + `egress` + traffic-binding to
  findings.

## 4. Options

| Approach | How the harness acts | Governance | Evidence |
|---|---|---|---|
| **A — Governed tools over MCP** | BARTEX **exposes** its curated catalog as an **MCP server**; the harness connects and its **native tools are disabled** — it may only call BARTEX tools | **Full** — every action still hits `applyIntercept` + RoE + target gate | **Full** — tools write assets/findings/traffic-bindings as today |
| **B — Sandboxed harness behind the proxy** | Harness keeps its own tools, but runs in a locked sandbox with **all egress forced through the recording/scope proxy**, FS confined to the task workdir | **Network-level only** (scope + capture at the wire; argv-approval / budgets weaker) | **Full at the network layer**; thinner graph write-back |
| **C — A + B (recommended)** | Governed MCP tools as the surface, **plus** the egress proxy as a hard backstop | **Full**, defence-in-depth | **Full** |

Option A is feasible because the tool catalog is already an abstraction and
norma already has MCP *client* plumbing — we invert it to a *server*. Option B is
what makes A safe against "the harness had a native tool we forgot to disable."

## 5. Recommended design (C)

- **BARTEX Tools MCP server** (Phase 0): expose the governed catalog
  (recon / http / `insert_assets` / `record_fact` / `report_finding` /
  scoped-bash / …) over MCP (Streamable HTTP; stdio later). Each call runs the
  **same** `applyIntercept` hook and the **same** write-back, so scope, deny,
  RoE engagement, target pins, budgets and evidence binding all hold unchanged.
  Independently useful: any MCP client (incl. Claude Code) can then drive a
  **governed** BARTEX engagement.
- **`harness` worker backend** (Phase 1+): a *harness profile* (alongside LLM
  profiles) describing how to launch an external CLI (command, args, env, the
  BARTEX MCP endpoint, "native tools off" flags). Worker dispatch spawns it in
  the task's sandboxed workdir, injects the intent as the prompt, points it at
  the BARTEX Tools MCP server, and forces egress through the recording proxy.
- **Budgets/termination:** external token accounting differs — enforce via
  turn/time caps + the noise governor + killing the subprocess at budget;
  documented as **coarser** than native budgets.
- **Transcript/evidence:** the harness transcript is captured as the session
  log; findings/assets/traffic still bind because actions are BARTEX tool calls.

## 6. Security analysis (the crux)

| Invariant | Preserved via | Residual risk / mitigation |
|---|---|---|
| Scope / deny / target gate | A: guard hook on every MCP tool call; B: proxy scope gate | Harness native egress → **B backstop**; deny unknown hosts |
| **RoE engagement window/scope** | Same `applyIntercept` path (`0007` gate) | Thread engagement context into the MCP session (same `TargetScope`) |
| argv-prefix approval | MCP "scoped bash" tool keeps argv gating | A native shell bypasses it → native tools **must** be disabled + sandbox |
| Budgets | Turn/time caps + subprocess kill | Coarser; documented |
| Evidence binding ("receipts") | Tool calls + proxy capture | Full |
| Prompt-injection framing | Re-wrap target output in MCP tool results (untrusted-data note) | External loop may ignore our framing → rely on tools-are-data + sandbox, not the model's compliance |

**The trust boundary moves to the MCP tool surface + sandbox/proxy, not the
model's compliance.** Hard rule: if a CLI cannot be restricted to MCP-only tools,
it **must not** run without Option B's sandbox.

## 7. Data model / API / config (Phase 1+)

- `harness_profiles` table (additive): `name`, `command`, `args` (jsonb), `env`
  (jsonb, secrets sealed), `mcp_mode`, `native_tools_disabled` (bool), `sandbox`
  (bool). A nullable `tasks.harness_profile_id` FK selects the backend per task.
- API: CRUD for harness profiles; task-create accepts an optional
  `harness_profile_id`. UI: a harness selector + a profile editor.

## 8. Phased plan

- **Phase 0 — BARTEX Tools MCP server. ✅ Mounted.** The substrate (the
  self-contained `mcpserver` package — a minimal MCP Streamable-HTTP / JSON-RPC
  server with `initialize` / `tools/list` / `tools/call`, bearer auth and the
  `GuardedToolset` seam) is now wired live:
  - **Guard seam:** `guard.Guard.CheckToolCall(ctx, name, input)` runs the same
    `applyIntercept` pipeline a worker uses and returns the platform-control
    block message as an error — one policy, two callers.
  - **Governed catalog:** `agent.GuardedCatalog` adapts the task's curated
    `WorkerTools()` to the MCP `Toolset`, running `CheckToolCall` before each
    call and executing the identical `CoreTool` on a pass (so assets / facts /
    findings write back exactly as for the built-in loop).
  - **Endpoint + scope:** mounted at `/api/mcp`, outside the operator-JWT
    wrapper, authenticating on a task-scoped bearer token. The binder attaches
    the task's `targetScope(...)` to each call's context, so the target gate and
    the RoE engagement policy enforce pins / approvals / the engagement window.
  - **Tokens:** `POST /api/tasks/{id}/mcp-token` (operator-auth) mints a
    SHA-256-stored, 12 h, task-scoped token; shown once, stored in the harness
    profile's env.
  - **Known Phase-0 limit:** only BARTEX's *domain* tools are governed here. A
    harness's own native recon/exploit tools still bypass layer-1 and are caught
    only by the network backstop — which is why Phase 1 is Option-B-mandatory.
- **Phase 1 — Claude Code backend, MCP-restricted, behind the egress proxy.**
  One harness, minimal, sandboxed; prove governance parity end-to-end on a
  scoped target.
- **Phase 2 — parity + breadth.** Budgets / transcript / evidence polish;
  Codex / Aider backends; harness-profile UI; RoE threading.
- **Phase 3 — hardening.** Sandbox-escape review, per-CLI native-tool lockdown,
  red-team the trust boundary.

## 9. Recommendation

Build **Phase 0 now** — highest leverage, lowest risk, useful on its own (it
lets any MCP client drive a governed engagement) and the substrate everything
else stands on. Treat **Phase 1+ as conditional** on accepting coarser budgets
and the sandbox-mandatory rule; if a CLI cannot be locked to MCP-only tools, it
is Option-B-or-nothing.
