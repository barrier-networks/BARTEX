# Barrier — rebrand & build plan

This document records the transformation of the upstream **ARTEX** autonomous
penetration-testing platform into **Barrier** (Barrier Networks), what landed in
this pass, what is partial, and the scoped next steps. It is written for an
engineering audience picking the work up.

> Status legend: ✅ done & verified · 🟡 partial / needs follow-up · ⏭️ planned

---

## 1. Scope agreed

- **Rebrand + prioritised features** on top of the existing engine (not a pure
  reskin, not a re-architecture).
- Brand: **Barrier Networks** (real UK MSSP). Feature priorities: **client-ready
  reporting**, **new/upgraded skills**, **scoping / RoE enforcement**.
- Guardrails honoured: do **not** disturb the LLM provider/rotation engine, do
  **not** weaken the core safety gates, keep DB migrations **additive**.
- Delivery bar: a working, building, rebranded Barrier + a plan for the rest.

---

## 2. What landed

### 2.1 Rebrand ✅
- Go module `github.com/Autumn-27/artex` → `github.com/barrier-networks/bartex`
  (all imports, `go.mod`).
- Binary + command dir `cmd/artex` → `cmd/barrier` (binary name `barrier`);
  `build.sh`, `Dockerfile`, `docker-compose*.yml`, `start.sh`/`start.bat`,
  install/update/dev/reset scripts, CI workflows, `.env.example`,
  `config.example.json` all rebranded.
- Environment variables `ARTEX_*` → `BARRIER_*`, with a **backward-compat shim**
  (`config.ApplyLegacyEnv`, called first in `cmd/barrier/main.go`) that mirrors
  any legacy `ARTEX_*` value to its `BARRIER_*` equivalent, so existing
  deployments keep working unchanged.
- User-facing surfaces: startup banner, health `service` id, MCP client id,
  update-check repository, operator identity (`"ARTEX"` → `"Barrier"`, the login
  username / audit `DecidedBy`/`Approver`), the model-facing platform-control
  framing (`[Barrier platform control, not target defense]`), and the web UI
  (product name/title, login/setup, logo).
- **Verified:** full Go test suite green against PostgreSQL 16; static Next.js
  build compiles.

**Preserved deliberately (correctness/legal):**
- `LICENSE` (AGPL-3.0) untouched; upstream **ARTEX / Autumn-27** attribution and
  the **`norma`** agent-core SDK credited accurately in the README.
- Internal data-format / protocol constants: `__artex_enc` (secrets envelope),
  `ARTEX_DATA` (untrusted-data boundary markers), `artex-traffic-trace`,
  `artex-pace`, and the fixed `X-Keenable-Title: ARTEX` external contract.
- Existing `db/migrations/*` and `db/schema.sql` are checksum-verified, so they
  were **not** edited; the brand rename never touched them.

### 2.2 Brand identity 🟡
- Barrier logo shipped as a self-coloured SVG (`web/public/logo.svg`, shield +
  three ascending barrier bars) plus a regenerated raster (`logo.png`); login /
  setup / sidebar reference the vector.
- Centralised brand palette (brand blue primary, cyan accent, matching ring /
  sidebar / chart ramp) in `web/src/app/globals.css`, light + dark.
- **Caveat (flagged):** `barriernetworks.com` was **not reachable** from the
  build network, so the exact brand hex and fonts could **not** be read from the
  live site. The applied palette is a professional **inference** and is isolated
  in one block in `globals.css` for a one-line swap once confirmed. The brand
  research spec (confirmed-vs-inferred table) is in the session scratchpad.
  **Follow-up:** allow-list `barriernetworks.com` and replace the palette +
  fonts with the real values; refresh `favicon.ico`.

### 2.3 Skills ✅ (content) / 🟡 (image tooling)
Three authorized-engagement playbooks added under `skills/`, validated by the
shipped-skills parser test:
- `cloud-recon` (net-new) — read-only AWS/Azure/GCP exposure + CSPM-style posture
  enumeration (external + credentialed-read), per-provider `reference.md`.
- `external-infra` (net-new) — host-level perimeter assessment that complements
  `bbot-recon` + `scopesentry` rather than duplicating them.
- `web-api-deep` (upgrade) — advanced authZ / injection / SSRF / business-logic
  and end-to-end exploit chaining extending `api-recon` + `api-security`.

**Follow-up:** several referenced tools (Azure CLI, Prowler, ScoutSuite,
cloud_enum, naabu, fingerprintx, testssl.sh/sslscan) are **not yet in the
container image** — add them to the `Dockerfile` tools stage so the skills can
actually invoke them. The skills already tell the agent to verify a tool before
use, so this is a capability gap, not a correctness bug.

### 2.4 Scoping / Rules-of-Engagement (RoE)
<!-- FINALISE: filled in once the RoE implementation is verified -->
See §3. Designed as an **additive** layer that can only *subtract* reach
(engagement window + allow/deny scope enforced at the single guard choke point),
with a nil-engagement no-regression fallback.

### 2.5 Client-ready reporting
<!-- FINALISE: filled in once the reporting implementation is verified -->
See §3. Designed as a Barrier-branded, UK-style PDF (exec summary + CVSS-scored
findings + linked evidence) built from the existing evidence graph.

---

## 3. Design specs (full detail in session scratchpad)

Four detailed design/research docs were produced and drive the feature work:
- **RoE / engagement enforcement** — additive model, enforcement insertion point
  in `guard.applyIntercept`, migration, fail-safe matrix.
- **Reporting** — HTML-template → PDF via headless Chromium (`chromedp`) with a
  pure-Go fallback behind an interface; net-new CVSS v3.1 package; new routes.
- **Skills** — the three playbooks above.
- **Brand** — Barrier Networks positioning + inferred palette/tokens.

---

## 4. Verification notes

- A local PostgreSQL 16 cluster was stood up to run the DB-backed test suite
  (the container ships only the `psql` client). `BARRIER_PG_DSN` points at it.
- Full Go suite: **green**. Static web build: **compiles**.
- The Docker image build was **not** exercised (no Docker daemon in the build
  environment) — a CI/image build should be run before release.

---

## 5. Prioritised follow-ups

1. Replace inferred brand palette/fonts with confirmed Barrier Networks values
   (needs `barriernetworks.com` network access); refresh `favicon.ico`.
2. Add the cloud/infra tools to the Dockerfile tools stage so the new skills can
   run their referenced tooling.
3. RoE Phase 2: engagement UI (selector + detail page + task badge),
   authorisation records, intensity-cap / permitted-test-type enforcement, and
   the report "RoE recap" section.
4. Reporting Phase 2: CVSS calculator + analyst entry, Download-PDF buttons,
   optional pure-Go PDF engine for browser-forbidden deployments.
5. Internal cosmetic cleanup (optional): the `artex_token` cookie name and the
   `ARTEX_DATA` marker stem remain for compatibility; rename in lockstep if a
   fully brand-consistent internal surface is wanted.
6. Run the Docker image build + CI before any release/tag.
