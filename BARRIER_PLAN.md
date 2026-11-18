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

### 2.2 Brand identity ✅
- **Palette + type taken from the official Barrier Brand Refinement guidelines
  (Figma).** Typeface stack **Aptos → Calibri → Segoe UI → Arial** (Inter was
  review-only in the guidelines, not the brand face); mono Cascadia/Consolas.
  Status hues stay published-fixed: **Jade `#006951`** success, **Brick
  `#A73030`** error, **Amber `#855400`** warning.
- **One palette for all offerings.** Per the current guidelines, service
  offerings no longer take separate colours: AdversaryOps (and CISOaaS, ROC,
  Assure, …) all share the **core Cobalt/Deep/Glow** palette and differ only by
  **name and logo**. The console uses that core palette (`globals.css`, light +
  dark) with the Cobalt→Deep gradient on the login/setup panels.
- **The console carries the AdversaryOps offering:**
  - Product name/title → **AdversaryOps** (`app-config`, login, setup, sidebar);
    the operator *username* stays `Barrier` (backend-asserted).
  - **Logo: the real AdversaryOps mark**, downloaded from the brand Figma
    (node `99:86`, "Supplied White · original SVG geometry") and used verbatim
    at `web/public/logo.svg` (+ raster) — not a recreation. Painted in the core
    brand colour (Deep on light, white when inverted on dark surfaces), per the
    "paint only" variants. (Downloading it needed `www.figma.com` on the
    environment allowlist, which the user approved.)
- **Note:** the client **report** still carries **Barrier Networks** company
  branding (Cobalt + the earlier mark) rather than AdversaryOps red, on the view
  that a client report is a consultancy deliverable; switch it to AdversaryOps or
  the Barrier Networks master mark on request. `favicon.ico` not yet refreshed.

### 2.3 Skills ✅ (content) / 🟡 (image tooling)
Three authorized-engagement playbooks added under `skills/`, validated by the
shipped-skills parser test:
- `cloud-recon` (net-new) — read-only AWS/Azure/GCP exposure + CSPM-style posture
  enumeration (external + credentialed-read), per-provider `reference.md`.
- `external-infra` (net-new) — host-level perimeter assessment that complements
  `bbot-recon` + `scopesentry` rather than duplicating them.
- `web-api-deep` (upgrade) — advanced authZ / injection / SSRF / business-logic
  and end-to-end exploit chaining extending `api-recon` + `api-security`.

**AWS tooling:** `cloud-recon` now drives **Pacu** (the Barrier Networks fork
`barrier-networks/pacu`) for credentialed AWS assessment — headless `*__enum*`
modules plus `iam__privesc_scan --scan-only` (reports escalation paths without
exercising them). Pacu is installed in the `Dockerfile` (pinned venv at
`/opt/pacu`). Its exploitation / persistence / exfiltration / log-manipulation
modules are explicitly **out of scope** for the read-only skill and run only
under an engagement that authorises them + the platform approval gate (ties into
the RoE layer).

**Follow-up:** other referenced tools (Azure CLI, Prowler, ScoutSuite,
cloud_enum, naabu, fingerprintx, testssl.sh/sslscan) are **not yet in the
container image** — add them to the `Dockerfile` tools stage. The skills tell the
agent to verify a tool before use, so this is a capability gap, not a bug. The
Pacu Dockerfile addition has **not** been image-build-verified here (no Docker
daemon); run the image build in CI.

### 2.4 Scoping / Rules-of-Engagement (RoE) ✅ (Phase 1) / ⏭️ (Phase 2)
**Landed & verified.** An additive engagement layer that can only *subtract*
reach. An engagement binds a task to a client with an authorised scope, a
deny-list, a time window, a status and an intensity cap. Enforcement sits at the
single guard choke point (`guard.applyIntercept`), **after** platform/user/judge
deny (so deny still wins) and **before** the target gate, gated only on an
attached policy — so the window/scope are enforced even when the target-gate
opt-in is off. Fail-closed throughout: not-active / window-closed / deny-match /
no-allow-match / undecidable / unresolvable-policy / unparseable-deny-row all
**block**; `engagement_id IS NULL` skips the step entirely (no regression, proven
by test). Migration `0007` (additive) + `schema.sql` parity; minimal
create/list/get/scope/attach API. Verified: full Go suite green against
PostgreSQL 16.

**Phase 2 (follow-up):** engagement UI (selector + detail page + task badge),
authorisation records surfaced, intensity-cap / permitted-test-type composition
with the noise governor, and the report "RoE recap" section.

### 2.5 Client-ready reporting ✅ (core) / ⏭️ (enrichment)
**Landed & verified.** A Barrier-branded, UK-style client report built from the
existing findings:
- `report/cvss` — a dependency-free CVSS v3.1 base-score implementation
  (vector parse + score + severity band), unit-tested against FIRST reference
  vectors (9.8, 10.0, …). `Resolve` prefers an analyst-set vector and otherwise
  derives an **indicative** score from the finding's severity, labelled as such.
- `report/clientreport.go` + template — a print-friendly HTML report: cover with
  CONFIDENTIAL classification, document control, executive summary with a
  severity breakdown and risk narrative, scope & methodology (OWASP / NIST
  SP 800-115 / NCSC CAF), a findings summary table with CVSS, and detailed
  findings (CVSS, status, verification, PoE, summary, evidence, captured
  traffic). Barrier logo + brand palette.
- `report/pdf.go` — renders the HTML to **PDF via headless Chromium**
  (discovered at `BARRIER_CHROMIUM` → `/opt/pw-browsers/chromium` → PATH); no Go
  dependency added. Verified end-to-end (a real PDF is produced in this
  environment).
- Routes: `GET /api/tasks/{id}/report.html` (always available) and
  `GET /api/tasks/{id}/report.pdf` (501 + guidance when no browser). Document
  framing (client, classification, authoriser, window, scope) comes from query
  parameters; the task id is validated so a report can never span tasks.

**Phase 2 (follow-up):** persist an analyst-set CVSS vector per finding
(migration + DB field + UI calculator) so scores stop being indicative; populate
the captured-request/response **evidence excerpts** in the report (the findings
list query returns binding metadata but not bodies yet); auto-fill the document
meta from the task's engagement (name/window/scope/authorisation); and a
Download-report button in the web UI.

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

1. Drop in the **official Barrier logo mark** (export from the brand Figma —
   blocked by the egress proxy this session) and refresh `favicon.ico`; the
   palette/fonts are already applied from the guidelines.
2. Add the remaining cloud/infra tools to the Dockerfile tools stage (Pacu is
   already wired for AWS; Azure CLI, Prowler, ScoutSuite, cloud_enum, naabu,
   fingerprintx, testssl.sh/sslscan still pending) and build the image in CI to
   verify (the Pacu addition is not yet build-verified here).
3. RoE Phase 2: engagement UI (selector + detail page + task badge),
   authorisation records, intensity-cap / permitted-test-type enforcement, and
   the report "RoE recap" section.
4. Reporting Phase 2: persist analyst-set CVSS vectors (so scores stop being
   indicative), populate request/response evidence excerpts in the report,
   auto-fill document meta from the engagement, Download-report button in the UI,
   and optionally a pure-Go PDF engine for browser-forbidden deployments.
5. Internal cosmetic cleanup (optional): the `artex_token` cookie name and the
   `ARTEX_DATA` marker stem remain for compatibility; rename in lockstep if a
   fully brand-consistent internal surface is wanted.
6. Run the Docker image build + CI before any release/tag.
